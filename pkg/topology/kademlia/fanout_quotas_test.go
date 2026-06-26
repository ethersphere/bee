// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func sumQuotas(quotas [swarm.MaxBins]int, hi uint8) int {
	total := 0
	for bin := uint8(0); bin < hi; bin++ {
		total += quotas[bin]
	}
	return total
}

func TestFanoutQuotasEmpty(t *testing.T) {
	t.Parallel()

	quotas := fanoutQuotas([swarm.MaxBins]int{}, 12, 2)
	if sum := sumQuotas(quotas, 12); sum != 0 {
		t.Fatalf("want zero budget on empty bins, got %d", sum)
	}
}

func TestFanoutQuotasSkipsEmptyBins(t *testing.T) {
	t.Parallel()

	var counts [swarm.MaxBins]int
	counts[0] = 5
	counts[4] = 5
	counts[8] = 5

	const perBin = 2
	quotas := fanoutQuotas(counts, 12, perBin)

	active := 3
	wantBudget := perBin * active
	if got := sumQuotas(quotas, 12); got != wantBudget {
		t.Fatalf("budget: got %d, want %d", got, wantBudget)
	}
	for bin, c := range counts {
		if c == 0 && quotas[bin] != 0 {
			t.Fatalf("empty bin %d got quota %d", bin, quotas[bin])
		}
		if c > 0 && quotas[bin] == 0 {
			t.Fatalf("non-empty bin %d got zero quota", bin)
		}
	}
}

func TestFanoutQuotasBandwidthNeutral(t *testing.T) {
	t.Parallel()

	const (
		hi     = uint8(12)
		perBin = 2
		peers  = 10
	)

	var counts [swarm.MaxBins]int
	for bin := uint8(0); bin < hi; bin++ {
		counts[bin] = peers
	}

	quotas := fanoutQuotas(counts, hi, perBin)

	active := int(hi)
	wantBudget := perBin * active
	if got := sumQuotas(quotas, hi); got != wantBudget {
		t.Fatalf("budget: got %d, want %d (bandwidth-neutral)", got, wantBudget)
	}
}

func TestFanoutQuotasGradientShape(t *testing.T) {
	t.Parallel()

	const (
		hi     = uint8(12)
		perBin = 2
		peers  = 10
	)

	var counts [swarm.MaxBins]int
	for bin := uint8(0); bin < hi; bin++ {
		counts[bin] = peers
	}

	quotas := fanoutQuotas(counts, hi, perBin)

	// Closer shallow bins (higher index) should receive at least as many recipients
	// as more distant ones when peer counts are equal.
	for bin := uint8(1); bin < hi; bin++ {
		if quotas[bin] < quotas[bin-1] {
			t.Fatalf("quota not non-decreasing: bin %d=%d, bin %d=%d", bin-1, quotas[bin-1], bin, quotas[bin])
		}
	}

	// Documented example: bins 0-5 get 1, bins 6-11 get 3.
	for bin := uint8(0); bin < 6; bin++ {
		if quotas[bin] != 1 {
			t.Fatalf("bin %d: got quota %d, want 1", bin, quotas[bin])
		}
	}
	for bin := uint8(6); bin < hi; bin++ {
		if quotas[bin] != 3 {
			t.Fatalf("bin %d: got quota %d, want 3", bin, quotas[bin])
		}
	}
}

func TestFanoutQuotasCappedByPeerCount(t *testing.T) {
	t.Parallel()

	var counts [swarm.MaxBins]int
	counts[11] = 1 // only one peer in the closest shallow bin

	quotas := fanoutQuotas(counts, 12, 10)

	if quotas[11] != 1 {
		t.Fatalf("bin 11: got quota %d, want 1 (capped by peer count)", quotas[11])
	}
}
