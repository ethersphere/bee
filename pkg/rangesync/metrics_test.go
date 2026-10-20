// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
)

// TestPairwiseDiffBuckets checks the buckets of pairwise_diff: 0, then one
// octave of integers per bucket (2^k − 1), so that the bucket of
// CatchUpMinDiff − 1 counts exactly the sessions below CatchUpMinDiff (open
// question 9), up to past 2 × LargeDiff.
func TestPairwiseDiffBuckets(t *testing.T) {
	t.Parallel()

	n := newTestNode(t)
	bounds, counts := n.svc.PairwiseDiffBuckets()
	if len(bounds) == 0 || bounds[0] != 0 {
		t.Fatalf("bounds %v, want 0 first", bounds)
	}
	for k, b := range bounds[1:] {
		if want := float64(uint64(1)<<(k+1) - 1); b != want {
			t.Fatalf("bound %d is %v, want %v", k+1, b, want)
		}
	}
	if !slices.Contains(bounds, float64(rangesync.CatchUpMinDiff-1)) {
		t.Fatalf("no bound at CatchUpMinDiff − 1 = %d: %v", rangesync.CatchUpMinDiff-1, bounds)
	}
	if last := bounds[len(bounds)-1]; last < 2*rangesync.LargeDiff-1 {
		t.Fatalf("last bound %v, want at least 2 × LargeDiff − 1", last)
	}
	for _, c := range counts {
		if c != 0 {
			t.Fatalf("counts %v before any session", counts)
		}
	}
}
