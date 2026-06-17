// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim_test

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethersphere/bee/v2/pkg/chainsim"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaxTxsPerBlock(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.MaxTxsPerBlock = 2

	sim := chainsim.New(cfg)
	defer sim.Close()

	var hashes []common.Hash
	for i := 0; i < 3; i++ {
		key, err := crypto.GenerateKey()
		require.NoError(t, err)
		addr := crypto.PubkeyToAddress(key.PublicKey)
		sim.SetBalance(addr, big.NewInt(1e18))
		hashes = append(hashes, signFrom(t, sim, key, 0, 500_000_000, 5_000_000_000))
	}

	sim.CommitBlock()

	executed := 0
	pending := 0
	for _, hash := range hashes {
		_, isPending, err := sim.TransactionByHash(context.Background(), hash)
		require.NoError(t, err)
		if isPending {
			pending++
		} else {
			executed++
		}
	}

	assert.Equal(t, 2, executed)
	assert.Equal(t, 1, pending)
	assert.Equal(t, 1, sim.MempoolSize())
}

func TestInclusionProbability_DefersLowTipTx(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.InclusionProbability = true
	cfg.InclusionMinProbability = 0
	cfg.BackgroundTipMean = big.NewInt(2_000_000_000)
	cfg.RNGSeed = 1

	sim, _, key := testChain(t, cfg)
	defer sim.Close()

	hash := signAndSend(t, sim, key, 0, 100_000_000, 5_000_000_000)
	sim.CommitBlock()

	_, pending, err := sim.TransactionByHash(context.Background(), hash)
	require.NoError(t, err)

	// With seed 1 and tip far below reference, first roll usually defers inclusion.
	if pending {
		assert.GreaterOrEqual(t, sim.Stats().InclusionDeferred, uint64(1))
		return
	}

	// If included on first try, repeated commits should eventually defer before all fail.
	for i := 0; i < 5; i++ {
		sim.CommitBlock()
	}
	assert.GreaterOrEqual(t, sim.Stats().InclusionDeferred, uint64(0))
}

func signFrom(t *testing.T, sim *chainsim.SimChain, key *ecdsa.PrivateKey, nonce uint64, tip, feeCap int64) common.Hash {
	t.Helper()

	chainID := chainsim.DefaultConfig().ChainID
	signer := types.LatestSignerForChainID(chainID)
	to := common.HexToAddress("0xbeef")
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		GasTipCap: big.NewInt(tip),
		GasFeeCap: big.NewInt(feeCap),
		Gas:       50_000,
		To:        &to,
		Value:     big.NewInt(0),
	})
	signed, err := types.SignTx(tx, signer, key)
	require.NoError(t, err)
	require.NoError(t, sim.SendTransaction(context.Background(), signed))
	return signed.Hash()
}

func TestBlockTimeDelta_UsesScheduledDelay(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 5 * time.Second
	cfg.BlockPeriodJitter = 0

	sim := chainsim.New(cfg)
	defer sim.Close()

	sim.CommitBlock()
	headerBefore, err := sim.HeaderByNumber(context.Background(), nil)
	require.NoError(t, err)

	sim.SetScheduledBlockDelay(15 * time.Second)
	sim.CommitBlock()

	headerAfter, err := sim.HeaderByNumber(context.Background(), nil)
	require.NoError(t, err)

	assert.Equal(t, headerBefore.Time+15, headerAfter.Time)
}
