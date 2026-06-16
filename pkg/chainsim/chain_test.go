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

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethersphere/bee/v2/pkg/chainsim"
	signermock "github.com/ethersphere/bee/v2/pkg/crypto/mock"
	"github.com/ethersphere/bee/v2/pkg/log"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testChain(t *testing.T, cfg chainsim.Config) (*chainsim.SimChain, common.Address, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	sender := crypto.PubkeyToAddress(key.PublicKey)

	sim := chainsim.New(cfg)
	sim.SetBalance(sender, big.NewInt(1e18))
	return sim, sender, key
}

func signAndSend(t *testing.T, sim *chainsim.SimChain, key *ecdsa.PrivateKey, nonce uint64, tip, feeCap int64) common.Hash {
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

func TestReceiptDelay_NotAvailableImmediately(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.ReceiptAvailDelay = 2
	cfg.BlockPeriod = time.Second

	sim, sender, key := testChain(t, cfg)
	defer sim.Close()

	hash := signAndSend(t, sim, key, 0, 500_000_000, 5_000_000_000)
	sim.CommitBlock()

	_, pending, err := sim.TransactionByHash(context.Background(), hash)
	require.NoError(t, err)
	assert.False(t, pending)

	_, err = sim.TransactionReceipt(context.Background(), hash)
	assert.ErrorIs(t, err, ethereum.NotFound)

	sim.CommitBlock()
	_, err = sim.TransactionReceipt(context.Background(), hash)
	assert.ErrorIs(t, err, ethereum.NotFound)

	sim.CommitBlock()
	receipt, err := sim.TransactionReceipt(context.Background(), hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Equal(t, hash, receipt.TxHash)
	assert.Equal(t, uint64(1), receipt.Status)

	nonce, err := sim.NonceAt(context.Background(), sender, nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), nonce)
}

func TestReceiptDelay_ZeroMeansInstant(t *testing.T) {
	t.Parallel()

	sim, _, key := testChain(t, chainsim.DefaultConfig())
	defer sim.Close()

	hash := signAndSend(t, sim, key, 0, 500_000_000, 5_000_000_000)
	sim.CommitBlock()

	receipt, err := sim.TransactionReceipt(context.Background(), hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)
}

func TestSimChain_SendWithRetry_InclusionAfterCongestionDrop(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.InitialCongestion = 0.95
	cfg.BlockPeriod = 50 * time.Millisecond

	sim, sender, key := testChain(t, cfg)
	defer sim.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go sim.Run(ctx)

	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	monitor := transaction.NewMonitor(log.Noop, sim, sender, 20*time.Millisecond, 2)
	testutil.CleanupCloser(t, monitor)

	svc, err := transaction.NewService(log.Noop, sender, sim,
		signermock.New(
			signermock.WithSignTxFunc(func(tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
				return types.SignTx(tx, types.LatestSignerForChainID(chainID), key)
			}),
			signermock.WithEthereumAddressFunc(func() (common.Address, error) {
				return sender, nil
			}),
		),
		store,
		cfg.ChainID,
		monitor,
		0,
		transaction.TransactionsRetryConfig{
			RetryDelay:      200 * time.Millisecond,
			AttemptsPerTier: 3,
			StartTier:       "market",
			EndTier:         "aggressive",
			MaxTxPrice:      big.NewInt(100_000_000_000),
		},
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	go func() {
		time.Sleep(300 * time.Millisecond)
		sim.SetCongestion(0.1)
	}()

	recipient := common.HexToAddress("0xabcd")
	txHash, receipt, err := svc.SendWithRetry(ctx, &transaction.TxRequest{
		To:       &recipient,
		Data:     []byte{0xab, 0xcd},
		Value:    big.NewInt(0),
		GasLimit: 50_000,
	})

	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.NotEqual(t, common.Hash{}, txHash)
	assert.Equal(t, uint64(1), receipt.Status)
}

func TestSimChain_MempoolEviction(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.MaxMempoolSize = 1
	cfg.MempoolTTL = 0

	sim, _, key := testChain(t, cfg)
	defer sim.Close()

	first := signAndSend(t, sim, key, 0, 200_000_000, 3_000_000_000)
	second := signAndSend(t, sim, key, 1, 500_000_000, 5_000_000_000)

	_, _, err := sim.TransactionByHash(context.Background(), first)
	require.ErrorIs(t, err, ethereum.NotFound)

	_, pending, err := sim.TransactionByHash(context.Background(), second)
	require.NoError(t, err)
	assert.True(t, pending)
	assert.Equal(t, 1, sim.MempoolSize())
}

func TestSimChain_MempoolTTL(t *testing.T) {
	t.Parallel()

	cfg := chainsim.DefaultConfig()
	cfg.MempoolTTL = 1

	sim, _, key := testChain(t, cfg)
	defer sim.Close()

	hash := signAndSend(t, sim, key, 0, 200_000_000, 3_000_000_000)
	sim.CommitEmptyBlock()
	sim.CommitEmptyBlock()

	_, _, err := sim.TransactionByHash(context.Background(), hash)
	require.ErrorIs(t, err, ethereum.NotFound)
	assert.Equal(t, 0, sim.MempoolSize())
}

func TestSimChain_FeeHistory(t *testing.T) {
	t.Parallel()

	sim, _, key := testChain(t, chainsim.DefaultConfig())
	defer sim.Close()

	signAndSend(t, sim, key, 0, 500_000_000, 5_000_000_000)
	sim.CommitBlock()
	sim.CommitBlock()

	tips, err := sim.SuggestedFeeAndTipsFromHistory(context.Background(), nil)
	require.NoError(t, err)
	assert.True(t, tips.LowTip.Sign() >= 0)
	assert.True(t, tips.MarketTip.Sign() >= 0)
	assert.True(t, tips.AggressiveTip.Sign() >= 0)
}

func TestSimChain_InjectError(t *testing.T) {
	t.Parallel()

	sim, _, _ := testChain(t, chainsim.DefaultConfig())
	defer sim.Close()

	sim.InjectError("BlockNumber", assert.AnError, 1)

	_, err := sim.BlockNumber(context.Background())
	require.Error(t, err)
}
