// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestReferenceCountBranchingFactor asserts that the branching factor used by
// ReferenceCount is >= 2 for every possible redundancy.Level value, which is
// what makes the saturating overflow guards (division by branching) safe and
// what guarantees the search loop makes progress.
func TestReferenceCountBranchingFactor(t *testing.T) {
	t.Parallel()

	for i := 0; i <= math.MaxUint8; i++ {
		l := redundancy.Level(i)
		if got := l.GetMaxShards(); got < 2 {
			t.Fatalf("level %d: GetMaxShards() = %d, want >= 2", i, got)
		}
		if got := l.GetMaxEncShards(); got < 2 {
			t.Fatalf("level %d: GetMaxEncShards() = %d, want >= 2", i, got)
		}
	}
}

// referenceCountPreFix is the algorithm as it was before the overflow fix. It
// reports ok=false whenever any of its arithmetic would overflow uint64 (which
// includes the livelocking inputs), so it can be used as an oracle only where
// the pre-fix result was actually meaningful. A step budget keeps a
// hypothetical non-overflowing runaway from hanging the test.
func referenceCountPreFix(span uint64, level redundancy.Level, encrytedChunk bool) (int, int, bool) {
	const budget = 1 << 22

	maxShards := level.GetMaxShards()
	if encrytedChunk {
		maxShards = level.GetMaxEncShards()
	}
	var (
		branching  = uint64(maxShards)
		branchSize = uint64(swarm.ChunkSize)
	)
	steps := 0
	branchLevel := 1
	for branchSize < span {
		steps++
		if steps > budget || branchSize > math.MaxUint64/branching {
			return 0, 0, false
		}
		branchSize *= branching
		branchLevel++
	}
	referenceSize := uint64(swarm.ChunkSize)
	for i := 1; i < branchLevel-1; i++ {
		steps++
		if steps > budget || referenceSize > math.MaxUint64/branching {
			return 0, 0, false
		}
		referenceSize *= branching
	}
	dataShardAddresses := 1
	spanOffset := referenceSize
	for spanOffset < span {
		steps++
		if steps > budget || spanOffset > math.MaxUint64-referenceSize {
			return 0, 0, false
		}
		spanOffset += referenceSize
		dataShardAddresses++
	}
	parityAddresses := level.GetParities(dataShardAddresses)
	if encrytedChunk {
		parityAddresses = level.GetEncParities(dataShardAddresses)
	}
	return dataShardAddresses, parityAddresses, true
}

func referenceCountSpans(t *testing.T) []uint64 {
	t.Helper()

	spans := []uint64{0, 1, 2, swarm.ChunkSize - 1, swarm.ChunkSize, swarm.ChunkSize + 1, math.MaxUint64 - 1, math.MaxUint64}
	for k := 1; k < 64; k++ {
		v := uint64(1) << k
		spans = append(spans, v-1, v, v+1)
	}
	// values just under/over multiples of the branching powers actually used
	for _, branching := range []uint64{20, 39, 41, 64, 128} {
		v := uint64(swarm.ChunkSize)
		for {
			spans = append(spans, v-1, v, v+1)
			if v > math.MaxUint64/branching {
				break
			}
			v *= branching
		}
	}
	// The deterministic spans above cover every boundary; the random ones only
	// add breadth. Their number is kept small so that the tests, which run
	// over all 256 levels and both encryption flags, stay well within CI time
	// limits under the race detector.
	rnd := rand.New(rand.NewSource(1))
	for i := 0; i < 256; i++ {
		spans = append(spans, rnd.Uint64())
	}
	return spans
}

// TestReferenceCountTerminates drives ReferenceCount over every redundancy
// level, both encryption flags and a large set of adversarial spans, and
// asserts every call returns within a bounded time.
func TestReferenceCountTerminates(t *testing.T) {
	t.Parallel()

	spans := referenceCountSpans(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i <= math.MaxUint8; i++ {
			level := redundancy.Level(i)
			for _, enc := range []bool{false, true} {
				for _, span := range spans {
					data, parity := file.ReferenceCount(span, level, enc)
					if data < 1 {
						t.Errorf("level %d enc %v span %d: data shards = %d, want >= 1", i, enc, span, data)
						return
					}
					if parity < 0 {
						t.Errorf("level %d enc %v span %d: parities = %d", i, enc, span, parity)
						return
					}
				}
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("ReferenceCount did not terminate within 60s")
	}
}

// TestReferenceCountUnchangedForNonOverflowingSpans asserts the fix is
// bit-identical to the pre-fix algorithm for every span the pre-fix algorithm
// handled without overflowing. Spans where the old code wrapped are skipped:
// its answers there were wrapped garbage, not a behaviour worth preserving.
func TestReferenceCountUnchangedForNonOverflowingSpans(t *testing.T) {
	t.Parallel()

	compared := 0
	for i := 0; i <= math.MaxUint8; i++ {
		level := redundancy.Level(i)
		for _, enc := range []bool{false, true} {
			for _, span := range referenceCountSpans(t) {
				wantData, wantParity, ok := referenceCountPreFix(span, level, enc)
				if !ok {
					continue
				}
				compared++
				gotData, gotParity := file.ReferenceCount(span, level, enc)
				if gotData != wantData || gotParity != wantParity {
					t.Fatalf("level %d enc %v span %d: got (%d, %d), want (%d, %d)",
						i, enc, span, gotData, gotParity, wantData, wantParity)
				}
			}
		}
	}
	if compared == 0 {
		t.Fatal("no spans compared")
	}
	t.Logf("compared %d (span, level, enc) combinations against the pre-fix algorithm", compared)
}

// TestReferenceCountRegression pins the span from the fuzz corpus entry that
// livelocked the pre-fix implementation.
func TestReferenceCountRegression(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = file.ReferenceCount(0x541df008a06ef192, redundancy.Level(0), true)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("ReferenceCount(0x541df008a06ef192, NONE, true) did not return")
	}
}
