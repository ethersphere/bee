// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim_test

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/chainsim"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotRoundTrip(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = time.Second
	cfg.MaxMempoolSize = 10

	sim, sender, key := testChain(t, cfg)
	sim.SetRevertAddress(common.HexToAddress("0xdead"))

	hash := signAndSend(t, sim, key, 0, 500_000_000, 5_000_000_000)
	sim.CommitBlock()

	_, pending, err := sim.TransactionByHash(context.Background(), hash)
	require.NoError(t, err)
	assert.False(t, pending)

	snap := sim.Snapshot()
	data, err := json.Marshal(snap)
	require.NoError(t, err)

	var snapFromJSON chainsim.Snapshot
	require.NoError(t, json.Unmarshal(data, &snapFromJSON))

	restored, err := chainsim.Restore(cfg, snapFromJSON)
	require.NoError(t, err)
	defer restored.Close()

	assert.Equal(t, uint64(1), restored.BlockCount())

	bal, err := restored.BalanceAt(context.Background(), sender, nil)
	require.NoError(t, err)
	assert.True(t, bal.Cmp(big.NewInt(0)) >= 0)

	receipt, err := restored.TransactionReceipt(context.Background(), hash)
	require.NoError(t, err)
	assert.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)

	nonce, err := restored.NonceAt(context.Background(), sender, nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), nonce)
}

func TestSnapshotPreservesMempool(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = time.Hour

	sim, _, key := testChain(t, cfg)
	hash := signAndSend(t, sim, key, 0, 500_000_000, 5_000_000_000)

	snap := sim.Snapshot()
	restored, err := chainsim.Restore(cfg, snap)
	require.NoError(t, err)
	defer restored.Close()

	_, pending, err := restored.TransactionByHash(context.Background(), hash)
	require.NoError(t, err)
	assert.True(t, pending)
	assert.Equal(t, 1, restored.MempoolSize())
}
