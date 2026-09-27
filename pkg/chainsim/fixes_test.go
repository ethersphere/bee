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
		assert.Zero(t, tip.Sign(), "empty block has no priority-fee rewards, got %s", tip)
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
	assert.Zero(t, tips.LowTip.Sign())
	assert.Zero(t, tips.MarketTip.Sign())
	assert.Zero(t, tips.AggressiveTip.Sign())
}

func TestBlockHeaderKeepsExecutionBaseFee(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.InitialBaseFee = big.NewInt(1_000)
	cfg.InitialCongestion = 1
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	sim := chainsim.New(cfg)
	defer sim.Close()

	sim.CommitBlock()

	header, err := sim.HeaderByNumber(context.Background(), big.NewInt(1))
	require.NoError(t, err)
	assert.Equal(t, int64(1_000), header.BaseFee.Int64())
	assert.Positive(t, sim.CurrentBaseFee().Cmp(big.NewInt(1_000)))

	fh, err := sim.FeeHistory(context.Background(), 1, nil, []float64{50})
	require.NoError(t, err)
	require.Len(t, fh.BaseFee, 2)
	assert.Equal(t, int64(1_000), fh.BaseFee[0].Int64())
	assert.Equal(t, sim.CurrentBaseFee().Int64(), fh.BaseFee[1].Int64())
}

func TestFeeHistoryIncludesMinedTransactionTip(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.InitialCongestion = 0
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	sim, _, key := testChain(t, cfg)
	defer sim.Close()

	const tip = int64(2_000_000_000)
	signAndSend(t, sim, key, 0, tip, 5_000_000_000)
	sim.CommitBlock()

	fh, err := sim.FeeHistory(context.Background(), 1, nil, []float64{10, 50, 90})
	require.NoError(t, err)
	require.Len(t, fh.Reward, 1)
	for _, reward := range fh.Reward[0] {
		assert.Equal(t, tip, reward.Int64())
	}
}

func TestHigherTipSuccessorIncludedInSameBlock(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.InitialCongestion = 0
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	sim, sender, key := testChain(t, cfg)
	defer sim.Close()

	low := signAndSend(t, sim, key, 0, 100_000_000, 5_000_000_000)
	high := signAndSend(t, sim, key, 1, 500_000_000, 5_000_000_000)
	sim.CommitBlock()

	nonce, err := sim.NonceAt(context.Background(), sender, nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), nonce)
	assert.Equal(t, 0, sim.MempoolSize())

	lowReceipt, err := sim.TransactionReceipt(context.Background(), low)
	require.NoError(t, err)
	highReceipt, err := sim.TransactionReceipt(context.Background(), high)
	require.NoError(t, err)
	assert.Equal(t, lowReceipt.BlockNumber, highReceipt.BlockNumber)
	assert.Less(t, lowReceipt.TransactionIndex, highReceipt.TransactionIndex,
		"higher-tip nonce 1 must follow nonce 0 in the same block")
}
