// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joiner_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/file/joiner"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestSubtrieSectionTerminatesOnLargeSize guards against an unsatisfiable
// loop exit: subtrieSize comes from chunk data, and a large value overflowed
// branchSize, while a payload with no data references made refs non-positive.
func TestSubtrieSectionTerminatesOnLargeSize(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		payloadSize int
		subtrieSize int64
	}{
		{"overflowing size", swarm.ChunkSize, 1 << 62},
		{"maximum size", swarm.ChunkSize, 1<<63 - 1},
		{"no data references", 0, 1 << 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			done := make(chan int64, 1)
			go func() {
				done <- joiner.SubtrieSection(128, swarm.HashSize, 0, tc.payloadSize, 0, tc.subtrieSize)
			}()

			select {
			case sec := <-done:
				// No value is right for a size the references cannot hold, and
				// readAtOffset rejects the child when its span disagrees. The
				// first section must still not have wrapped around.
				if sec <= 0 {
					t.Fatalf("got section %d, want a positive value", sec)
				}
			case <-time.After(5 * time.Second):
				// The loop has no context check; the goroutine is reaped at exit.
				t.Fatal("subtrieSection did not terminate")
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

			if got := joiner.SubtrieSection(refs, swarm.HashSize, tc.startIdx, swarm.ChunkSize, 0, tc.subtrieSize); got != tc.want {
				t.Fatalf("got section %d, want %d", got, tc.want)
			}
		})
	}
}
