// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redundancy_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
)

// TestComposition pins the fixed-point solve to the agreed spec §3.1 tables.
func TestComposition(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		level     redundancy.Level
		encrypted bool
		m, k, c   int
	}{
		{redundancy.NONE, false, 128, 0, 0},
		{redundancy.NONE, true, 64, 0, 0},
		{redundancy.MEDIUM, false, 114, 9, 3},
		{redundancy.STRONG, false, 103, 20, 3},
		{redundancy.INSANE, false, 92, 30, 3},
		{redundancy.PARANOID, false, 36, 87, 3},
		{redundancy.MEDIUM, true, 57, 9, 2},
		{redundancy.STRONG, true, 51, 20, 2},
		{redundancy.INSANE, true, 46, 31, 2},
		{redundancy.PARANOID, true, 18, 87, 3},
	} {
		m, k, c := tc.level.Composition(tc.encrypted)
		if m != tc.m || k != tc.k || c != tc.c {
			t.Errorf("level %d encrypted %v: got (%d,%d,%d), want (%d,%d,%d)",
				tc.level, tc.encrypted, m, k, c, tc.m, tc.k, tc.c)
		}
	}
}

func TestCarrierRefs(t *testing.T) {
	t.Parallel()
	if got := redundancy.NONE.CarrierRefs(123); got != 0 {
		t.Errorf("NONE must have no carriers, got %d", got)
	}
	for _, tc := range []struct{ children, want int }{
		{123, 5}, // full MEDIUM plain parent: ceil(123/48)+2
		{66, 4},  // full MEDIUM encrypted parent
		{5, 3},   // small partial parent: 1 carrier + 2
	} {
		if got := redundancy.MEDIUM.CarrierRefs(tc.children); got != tc.want {
			t.Errorf("CarrierRefs(%d) = %d, want %d", tc.children, got, tc.want)
		}
	}
}
