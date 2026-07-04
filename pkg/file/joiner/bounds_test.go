// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joiner_test

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/file/joiner"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/langos"
)

// TestJoinerBounds reproduces the joiner panic and timeout against the
// actual malformed sub-reference of the tree, read the way the /bzz
// file-serving path reads it (through langos look-ahead) rather than by
// sequentially reading the root.
//
// Chunk1 is the tree root. TestJoinerCrashTest joins Chunk1 directly, but the
// root advertises a span (352) smaller than its own chunk data, so the joiner
// treats it as a single leaf, copies 352 bytes and stops without ever
// descending into the tree - hence no panic there.
//
// The /bzz endpoint instead resolves Chunk1 as a manifest and ends up serving
// the child reference Chunk2, whose header advertises a span (524288 bytes) far
// larger than the data actually present in the tree. Serving it wraps the
// joiner in langos, which reads from offset 0 while peeking the next buffer at
// offset smallFileBufferSize.
//
// Root Cause Details:
//  1. In TestJoinerBounds/langos crash (with langos), the peek's ReadAt at offset 262144
//     descended into the first child chunk Chunk1. The parent's layout expected Chunk1's
//     subtree to cover a section size of 438272 bytes, but Chunk1 is actually a leaf
//     chunk of only 4096 bytes. Because the joiner did not validate that the child's
//     actual span matches the parent's layout expectation, it descended into Chunk1
//     with off=262144, cur=0. Since Chunk1's span (4096) <= len(data) (4096), it was
//     treated as a leaf, calculating dataOffsetStart = off - cur = 262144, and
//     sliced the data out of bounds, crashing the node.
//  2. In TestJoinerBounds/timeout (without langos), reading sequentially from offset 0
//     successfully read the first 4096 bytes from Chunk1. The next read at offset 4096
//     descended into Chunk1 again (expecting it to cover up to 438272 bytes). This
//     calculated dataOffsetStart = 4096, which resulted in copying 0 bytes and
//     returning 0, nil. io.Copy kept retrying indefinitely, causing the hang.
//  3. Note that langos itself functions correctly. Since the root chunk Chunk2
//     advertises a span of 524288 bytes, langos is entirely justified in issuing a
//     read/peek at offset 262144 (which is less than the advertised size). The joiner,
//     as an io.Reader/io.ReaderAt implementation, must return an error when queried
//     past the actual tree boundaries rather than panicking or hanging.
//
// The joiner enforces bounds check limits on leaf chunk reads, returning
// ErrMalformedTrie immediately if the offset is invalid.
func TestJoinerBounds(t *testing.T) {
	t.Run("langos crash", func(t *testing.T) {
		t.Parallel()
		testJoinerBug(t, true)
	})

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()
		testJoinerBug(t, false)
	})
}

func testJoinerBug(t *testing.T, useLangos bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store := inmemchunkstore.New()
	addrs := []string{
		"324881a5e0980e21eec6ab4348767a322260b4d477198aeb2500d9d492a99518", // Chunk1
		"7c512937c1aac8d772cff0d071cbdcda15748c9d422904007342a30dd58ef05d", // Chunk2
		"147a4a13002cfe9033b04bbf64b4de8091645930fa58f170fa146b0a7e0d7695", // Chunk3
		"6650507776a544842c632ed1dde92f8a456a88495308349f147472de3c1620f8", // Chunk4
		"8504f2a107ca940beafc4ce2f6c9a9f0968c62a5b5893ff0e4e1e2983048d276", // Chunk5
		"95378adf59567d5db28e970fc60d2dcf0568f11804fd8aa27d2ee7c9eac24100", // Chunk6
	}

	for _, addrStr := range addrs {
		b64Data, err := os.ReadFile(filepath.Join("testdata/bounds", addrStr+".b64"))
		if err != nil {
			t.Fatal(err)
		}
		data, err := base64.StdEncoding.DecodeString(string(b64Data))
		if err != nil {
			t.Fatal(err)
		}
		addr, err := hex.DecodeString(addrStr)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Put(ctx, swarm.NewChunk(swarm.NewAddress(addr), data)); err != nil {
			t.Fatal(err)
		}
	}

	// Join the malformed child reference directly, as /bzz does after resolving
	// the manifest at Chunk1.
	ref := swarm.MustParseHexAddress("7c512937c1aac8d772cff0d071cbdcda15748c9d422904007342a30dd58ef05d")
	reader, _, err := joiner.New(ctx, store, store, ref, redundancy.DefaultDownloadLevel)
	if err != nil {
		t.Fatal(err)
	}

	if useLangos {
		// smallFileBufferSize mirrors the lookahead buffer size that pkg/api uses when
		// serving sub-10MB files through langos (see lookaheadBufferSize in bzz.go).
		// langos peeks one buffer ahead, i.e. while serving from offset 0 it also issues
		// a ReadAt at this offset.
		const smallFileBufferSize = 8 * 32 * 1024 // 262144

		// Serve it exactly like the API does: wrap the joiner in langos and read it
		// from the start. langos' look-ahead peek issues the panicking ReadAt at
		// offset smallFileBufferSize. Once the joiner rejects such malformed trees
		// this should instead return an error here.
		lr := langos.NewBufferedLangos(reader, smallFileBufferSize)
		if _, err := io.Copy(io.Discard, lr); err == nil {
			t.Fatal("expected error, got nil")
		} else if !errors.Is(err, joiner.ErrMalformedTrie) {
			t.Fatalf("expected ErrMalformedTrie, got: %v", err)
		}
	} else {
		// Sequentially read the entire file. Without the leaf bounds check,
		// this path would hang indefinitely (reproducing the timeout) because
		// it kept retrying to read 0 bytes from the leaf chunk.
		if _, err := io.Copy(io.Discard, reader); err == nil {
			t.Fatal("expected error, got nil")
		} else if !errors.Is(err, joiner.ErrMalformedTrie) {
			t.Fatalf("expected ErrMalformedTrie, got: %v", err)
		}
	}
}
