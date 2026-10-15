// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/encryption/store"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestDecryptChunkDataShortChunk pins the fuzz corpus entry where chunk data
// shorter than the span made the span slice panic.
func TestDecryptChunkDataShortChunk(t *testing.T) {
	t.Parallel()

	_, err := store.DecryptChunkData([]byte{0x01}, keyFromSeed([]byte("swarm-fuzz-key")))
	if !errors.Is(err, swarm.ErrInvalidChunk) {
		t.Fatalf("got error %v, want %v", err, swarm.ErrInvalidChunk)
	}
}

// TestDecryptChunkDataOverflowingSpan pins the fuzz corpus entry whose
// decrypted span livelocked file.ReferenceCount through uint64 overflow.
func TestDecryptChunkDataOverflowingSpan(t *testing.T) {
	t.Parallel()

	const span = 0x541df008a06ef192

	seed := []byte("swarm-fuzz-key")
	chunkData := encryptChunk(seed, span)

	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := store.DecryptChunkData(chunkData, keyFromSeed(seed))
		done <- result{data, err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("unexpected error: %v", r.err)
		}
		if len(r.data) > swarm.ChunkWithSpanSize {
			t.Fatalf("got %d bytes, want at most %d", len(r.data), swarm.ChunkWithSpanSize)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("DecryptChunkData with span %#x did not return", uint64(span))
	}
}
