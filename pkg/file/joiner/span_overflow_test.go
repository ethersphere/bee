// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joiner_test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/file/joiner"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestSubtrieSectionTerminatesOnLargeSize checks subtrieSection with sizes
// read from chunk data that the references cannot hold: it must terminate and
// report the trie as malformed, at the first and at the last reference. A
// large size used to overflow branchSize and loop forever, and a payload with
// no data references made the loop exit unreachable.
func TestSubtrieSectionTerminatesOnLargeSize(t *testing.T) {
	t.Parallel()

	const lastIdx = (swarm.Branches - 1) * swarm.HashSize

	for _, tc := range []struct {
		name        string
		payloadSize int
		subtrieSize int64
	}{
		{"overflowing size", swarm.ChunkSize, 1 << 62},
		{"maximum size", swarm.ChunkSize, 1<<63 - 1},
		{"size too small for the references", swarm.ChunkSize, 1 << 20},
		{"no data references", 0, 1 << 20},
	} {
		for _, startIdx := range []int{0, lastIdx} {
			t.Run(fmt.Sprintf("%s at %d", tc.name, startIdx), func(t *testing.T) {
				t.Parallel()

				type result struct {
					sec int64
					err error
				}
				done := make(chan result, 1)
				go func() {
					sec, err := joiner.SubtrieSection(128, swarm.HashSize, startIdx, tc.payloadSize, 0, tc.subtrieSize)
					done <- result{sec, err}
				}()

				select {
				case r := <-done:
					if !errors.Is(r.err, joiner.ErrMalformedTrie) {
						t.Fatalf("got section %d and error %v, want %v", r.sec, r.err, joiner.ErrMalformedTrie)
					}
				case <-time.After(5 * time.Second):
					// The loop has no context check; the goroutine is reaped at exit.
					t.Fatal("subtrieSection did not terminate")
				}
			})
		}
	}
}

// TestReadInconsistentTrie checks that a root chunk whose span the references
// cannot hold fails at every offset, instead of serving data for some.
func TestReadInconsistentTrie(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmemchunkstore.New()

	leaf, err := cac.New(make([]byte, swarm.ChunkSize))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, leaf); err != nil {
		t.Fatal(err)
	}
	refs := make([]byte, 0, swarm.HashSize*swarm.Branches)
	for range swarm.Branches {
		refs = append(refs, leaf.Address().Bytes()...)
	}

	for _, span := range []uint64{math.MaxInt64, 1 << 20} {
		t.Run(fmt.Sprintf("span %d", span), func(t *testing.T) {
			t.Parallel()

			spanBytes := make([]byte, swarm.SpanSize)
			binary.LittleEndian.PutUint64(spanBytes, span)
			root, err := cac.NewWithDataSpan(append(spanBytes, refs...))
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Put(ctx, root); err != nil {
				t.Fatal(err)
			}
			j, size, err := joiner.New(ctx, store, store, root.Address(), 0)
			if err != nil {
				t.Fatal(err)
			}

			for _, off := range []int64{0, size / 2, size - swarm.ChunkSize} {
				if _, err := j.ReadAt(make([]byte, swarm.ChunkSize), off); !errors.Is(err, joiner.ErrMalformedTrie) {
					t.Fatalf("offset %d: got error %v, want %v", off, err, joiner.ErrMalformedTrie)
				}
			}
		})
	}
}

// TestSubtrieSectionValidTries checks that the overflow guard leaves the
// sections of well-formed tries unchanged.
func TestSubtrieSectionValidTries(t *testing.T) {
	t.Parallel()

	const (
		refs     = swarm.Branches
		level1   = swarm.ChunkSize // span of a leaf
		level2   = refs * level1   // span of a full intermediate chunk of leaves
		lastIdx  = (refs - 1) * swarm.HashSize
		fullSize = refs * level2
	)

	for _, tc := range []struct {
		name        string
		subtrieSize int64
		startIdx    int
		want        int64
	}{
		{"full leaves first", level2, 0, level1},
		{"full leaves last", level2, lastIdx, level1},
		{"short last leaf", (refs-1)*level1 + 1, lastIdx, 1},
		{"short last leaf first", (refs-1)*level1 + 1, 0, level1},
		{"full second level first", fullSize, 0, level2},
		{"short last subtrie", (refs-1)*level2 + 5, lastIdx, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := joiner.SubtrieSection(refs, swarm.HashSize, tc.startIdx, swarm.ChunkSize, 0, tc.subtrieSize)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got section %d, want %d", got, tc.want)
			}
		})
	}
}
