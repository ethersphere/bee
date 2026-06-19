// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/chainsim"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGasUsedNeverExceedsGasLimit(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.BlockGasLimit = 30_000_000
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	sim, _, key := testChain(t, cfg)
	sim.SetCongestion(0.8)
	defer sim.Close()

	signAndSend(t, sim, key, 0, 500_000_000, 5_000_000_000)
	sim.CommitBlock()

	header, err := sim.HeaderByNumber(context.Background(), big.NewInt(1))
	require.NoError(t, err)
	assert.LessOrEqual(t, header.GasUsed, header.GasLimit)
}

func TestFeeHistoryFallbackOnGenesisBlock(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.MinMempoolTip = big.NewInt(100_000_000)
	sim := chainsim.New(cfg)
	defer sim.Close()

	fh, err := sim.FeeHistory(context.Background(), 1, nil, []float64{10, 50, 90})
	require.NoError(t, err)
	require.Len(t, fh.Reward, 1)

	for _, tip := range fh.Reward[0] {
		assert.True(t, tip.Cmp(big.NewInt(0)) > 0,
			"expected non-zero tip in fee history for empty block, got %s", tip)
	}
}

func TestDeductCostUsesActualGas(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.BaseGasUsed = 21_000
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	sim, sender, key := testChain(t, cfg)
	defer sim.Close()

	balBefore, err := sim.BalanceAt(context.Background(), sender, nil)
	require.NoError(t, err)

	signAndSend(t, sim, key, 0, 500_000_000, 5_000_000_000)
	sim.CommitBlock()

	balAfter, err := sim.BalanceAt(context.Background(), sender, nil)
	require.NoError(t, err)

	diff := new(big.Int).Sub(balBefore, balAfter)
	maxExpected := new(big.Int).Mul(big.NewInt(50_000), big.NewInt(5_000_000_000))
	assert.True(t, diff.Cmp(maxExpected) < 0,
		"deducted %s, expected less than %s (gasLimit*feeCap)", diff, maxExpected)
}

func TestMultiNonceInclusion(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	sim, sender, key := testChain(t, cfg)
	defer sim.Close()

	signAndSend(t, sim, key, 0, 500_000_000, 5_000_000_000)
	signAndSend(t, sim, key, 1, 500_000_000, 5_000_000_000)
	signAndSend(t, sim, key, 2, 500_000_000, 5_000_000_000)

	sim.CommitBlock()

	nonce, err := sim.NonceAt(context.Background(), sender, nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), nonce)
	assert.Equal(t, 0, sim.MempoolSize())
}

func TestRandomRevertRate(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.RandomRevertRate = 1.0
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	sim, _, key := testChain(t, cfg)
	defer sim.Close()

	signAndSend(t, sim, key, 0, 500_000_000, 5_000_000_000)
	sim.CommitBlock()

	stats := sim.Stats()
	assert.Equal(t, uint64(1), stats.TransactionsReverted)
}

func TestFeeHistorySuggestedFeesOnNewChain(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	sim := chainsim.New(cfg)
	defer sim.Close()

	tips, err := sim.SuggestedFeeAndTipsFromHistory(context.Background(), nil)
	require.NoError(t, err)
	assert.True(t, tips.LowTip.Sign() > 0)
	assert.True(t, tips.MarketTip.Sign() > 0)
	assert.True(t, tips.AggressiveTip.Sign() > 0)
}
