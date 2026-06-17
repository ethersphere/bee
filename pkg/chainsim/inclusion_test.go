// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInclusionProbability(t *testing.T) {
	t.Parallel()

	ref := big.NewInt(2_000_000_000)

	assert.Equal(t, 1.0, inclusionProbability(big.NewInt(3_000_000_000), ref, 0.05))
	assert.Equal(t, 1.0, inclusionProbability(big.NewInt(2_000_000_000), ref, 0.05))

	half := big.NewInt(1_000_000_000)
	assert.InDelta(t, 0.25, inclusionProbability(half, ref, 0.05), 0.001)

	low := big.NewInt(100_000_000)
	assert.InDelta(t, 0.05, inclusionProbability(low, ref, 0.05), 0.001)
}

func TestNextBlockPeriod_Jitter(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.BlockPeriod = 10 * time.Second
	cfg.BlockPeriodJitter = 2 * time.Second
	cfg.RNGSeed = 99

	sim := New(cfg)
	defer sim.Close()

	sim.mu.Lock()
	defer sim.mu.Unlock()

	for i := 0; i < 20; i++ {
		d := sim.nextBlockPeriod()
		assert.GreaterOrEqual(t, d, 8*time.Second)
		assert.LessOrEqual(t, d, 12*time.Second)
	}
}
