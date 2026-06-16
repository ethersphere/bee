// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testChainID = big.NewInt(1337)
	testKey     *ecdsa.PrivateKey
	testSigner  types.Signer
	testSender  common.Address
)

func init() {
	key, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}
	testKey = key
	testSigner = types.LatestSignerForChainID(testChainID)
	testSender = crypto.PubkeyToAddress(testKey.PublicKey)
}

func signedTx(t *testing.T, nonce uint64, gasTipCap, gasFeeCap *big.Int) *types.Transaction {
	t.Helper()
	to := common.HexToAddress("0xabcd")
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   testChainID,
		Nonce:     nonce,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Gas:       21_000,
		To:        &to,
		Value:     big.NewInt(0),
	})
	signed, err := types.SignTx(tx, testSigner, testKey)
	require.NoError(t, err)
	return signed
}

func poolEntryFromTx(t *testing.T, tx *types.Transaction, addedAt uint64) *poolEntry {
	t.Helper()
	sender, err := types.Sender(testSigner, tx)
	require.NoError(t, err)
	return &poolEntry{tx: tx, sender: sender, addedAt: addedAt}
}

func TestMempool_AddAndGet(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 0)
	baseFee := big.NewInt(1_000)
	minTip := big.NewInt(100)
	balance := big.NewInt(1e18)

	tx := signedTx(t, 0, big.NewInt(200), big.NewInt(3_000))
	entry := poolEntryFromTx(t, tx, 0)
	require.NoError(t, pool.add(entry, baseFee, minTip, 0, balance))

	got := pool.getByHash(tx.Hash())
	require.NotNil(t, got)
	assert.Equal(t, tx.Hash(), got.tx.Hash())

	gotByNonce := pool.getBySenderNonce(testSender, 0)
	require.NotNil(t, gotByNonce)
	assert.Equal(t, tx.Hash(), gotByNonce.tx.Hash())
}

func TestMempool_Remove(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 0)
	baseFee := big.NewInt(1_000)
	minTip := big.NewInt(100)
	balance := big.NewInt(1e18)

	tx := signedTx(t, 0, big.NewInt(200), big.NewInt(3_000))
	entry := poolEntryFromTx(t, tx, 0)
	require.NoError(t, pool.add(entry, baseFee, minTip, 0, balance))

	pool.remove(tx.Hash())
	assert.Nil(t, pool.getByHash(tx.Hash()))
	assert.Nil(t, pool.getBySenderNonce(testSender, 0))
}

func TestMempool_RejectNonceTooLow(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 0)
	tx := signedTx(t, 0, big.NewInt(200), big.NewInt(3_000))
	entry := poolEntryFromTx(t, tx, 0)

	err := pool.add(entry, big.NewInt(1_000), big.NewInt(100), 1, big.NewInt(1e18))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonce too low")
}

func TestMempool_RejectLowFeeCap(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 0)
	tx := signedTx(t, 0, big.NewInt(200), big.NewInt(500))
	entry := poolEntryFromTx(t, tx, 0)

	err := pool.add(entry, big.NewInt(1_000), big.NewInt(100), 0, big.NewInt(1e18))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max fee per gas less than block base fee")
}

func TestMempool_RejectLowTip(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 0)
	tx := signedTx(t, 0, big.NewInt(50), big.NewInt(3_000))
	entry := poolEntryFromTx(t, tx, 0)

	err := pool.add(entry, big.NewInt(1_000), big.NewInt(100), 0, big.NewInt(1e18))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max priority fee per gas too low")
}

func TestMempool_RejectInsufficientFunds(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 0)
	tx := signedTx(t, 0, big.NewInt(200), big.NewInt(3_000))
	entry := poolEntryFromTx(t, tx, 0)

	err := pool.add(entry, big.NewInt(1_000), big.NewInt(100), 0, big.NewInt(1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient funds")
}

func TestMempool_ReplacementAccepted(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 0)
	baseFee := big.NewInt(1_000)
	minTip := big.NewInt(100)
	balance := big.NewInt(1e18)

	first := signedTx(t, 0, big.NewInt(200), big.NewInt(3_000))
	require.NoError(t, pool.add(poolEntryFromTx(t, first, 0), baseFee, minTip, 0, balance))

	second := signedTx(t, 0, big.NewInt(300), big.NewInt(4_000))
	require.NoError(t, pool.add(poolEntryFromTx(t, second, 0), baseFee, minTip, 0, balance))

	assert.Nil(t, pool.getByHash(first.Hash()))
	got := pool.getByHash(second.Hash())
	require.NotNil(t, got)
	assert.Equal(t, second.Hash(), got.tx.Hash())
}

func TestMempool_ReplacementUnderpriced_TipTooLow(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 0)
	baseFee := big.NewInt(1_000)
	minTip := big.NewInt(100)
	balance := big.NewInt(1e18)

	first := signedTx(t, 0, big.NewInt(200), big.NewInt(3_000))
	require.NoError(t, pool.add(poolEntryFromTx(t, first, 0), baseFee, minTip, 0, balance))

	second := signedTx(t, 0, big.NewInt(210), big.NewInt(4_000))
	err := pool.add(poolEntryFromTx(t, second, 0), baseFee, minTip, 0, balance)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "replacement transaction underpriced")
	assert.NotNil(t, pool.getByHash(first.Hash()))
}

func TestMempool_ReplacementUnderpriced_FeeCapTooLow(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 0)
	baseFee := big.NewInt(1_000)
	minTip := big.NewInt(100)
	balance := big.NewInt(1e18)

	first := signedTx(t, 0, big.NewInt(200), big.NewInt(3_000))
	require.NoError(t, pool.add(poolEntryFromTx(t, first, 0), baseFee, minTip, 0, balance))

	second := signedTx(t, 0, big.NewInt(300), big.NewInt(3_100))
	err := pool.add(poolEntryFromTx(t, second, 0), baseFee, minTip, 0, balance)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "replacement transaction underpriced")
}

func TestMempool_EvictOnFull(t *testing.T) {
	t.Parallel()

	pool := newMempool(2, 0)
	baseFee := big.NewInt(1_000)
	minTip := big.NewInt(100)
	balance := big.NewInt(1e18)

	low := signedTx(t, 0, big.NewInt(150), big.NewInt(3_000))
	high := signedTx(t, 1, big.NewInt(500), big.NewInt(5_000))
	require.NoError(t, pool.add(poolEntryFromTx(t, low, 0), baseFee, minTip, 0, balance))
	require.NoError(t, pool.add(poolEntryFromTx(t, high, 0), baseFee, minTip, 0, balance))

	newer := signedTx(t, 2, big.NewInt(600), big.NewInt(6_000))
	require.NoError(t, pool.add(poolEntryFromTx(t, newer, 0), baseFee, minTip, 0, balance))

	assert.Nil(t, pool.getByHash(low.Hash()))
	assert.NotNil(t, pool.getByHash(high.Hash()))
	assert.NotNil(t, pool.getByHash(newer.Hash()))
}

func TestMempool_EvictExpired(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 5)
	baseFee := big.NewInt(1_000)
	minTip := big.NewInt(100)
	balance := big.NewInt(1e18)

	tx := signedTx(t, 0, big.NewInt(200), big.NewInt(3_000))
	require.NoError(t, pool.add(poolEntryFromTx(t, tx, 10), baseFee, minTip, 0, balance))

	evicted := pool.evictExpired(16)
	require.Len(t, evicted, 1)
	assert.Nil(t, pool.getByHash(tx.Hash()))
}

func TestMempool_NewTxRejectedWhenFullAndLowTip(t *testing.T) {
	t.Parallel()

	pool := newMempool(2, 0)
	baseFee := big.NewInt(1_000)
	minTip := big.NewInt(100)
	balance := big.NewInt(1e18)

	high1 := signedTx(t, 0, big.NewInt(500), big.NewInt(5_000))
	high2 := signedTx(t, 1, big.NewInt(600), big.NewInt(6_000))
	require.NoError(t, pool.add(poolEntryFromTx(t, high1, 0), baseFee, minTip, 0, balance))
	require.NoError(t, pool.add(poolEntryFromTx(t, high2, 0), baseFee, minTip, 0, balance))

	low := signedTx(t, 2, big.NewInt(150), big.NewInt(3_000))
	err := pool.add(poolEntryFromTx(t, low, 0), baseFee, minTip, 0, balance)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max priority fee per gas too low")
	assert.Equal(t, 2, pool.size())
}

func TestMempool_EligibleSortedByTip(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 0)
	baseFee := big.NewInt(1_000)
	minTip := big.NewInt(100)
	balance := big.NewInt(1e18)

	keyB, err := crypto.GenerateKey()
	require.NoError(t, err)
	senderB := crypto.PubkeyToAddress(keyB.PublicKey)

	makeForSender := func(key *ecdsa.PrivateKey, sender common.Address, nonce uint64, tip int64) *poolEntry {
		to := common.HexToAddress("0xabcd")
		tx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   testChainID,
			Nonce:     nonce,
			GasTipCap: big.NewInt(tip),
			GasFeeCap: big.NewInt(5_000),
			Gas:       21_000,
			To:        &to,
		})
		signed, sErr := types.SignTx(tx, testSigner, key)
		require.NoError(t, sErr)
		return &poolEntry{tx: signed, sender: sender, addedAt: 0}
	}

	require.NoError(t, pool.add(makeForSender(testKey, testSender, 0, 200), baseFee, minTip, 0, balance))
	require.NoError(t, pool.add(makeForSender(keyB, senderB, 0, 500), baseFee, minTip, 0, balance))

	eligible := pool.eligible(map[common.Address]uint64{}, baseFee)
	require.Len(t, eligible, 2)
	assert.True(t, eligible[0].effectiveTip(baseFee).Cmp(eligible[1].effectiveTip(baseFee)) >= 0)
}

func TestMempool_EligibleSkipsWrongNonce(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 0)
	baseFee := big.NewInt(1_000)
	minTip := big.NewInt(100)
	balance := big.NewInt(1e18)

	tx := signedTx(t, 1, big.NewInt(200), big.NewInt(3_000))
	require.NoError(t, pool.add(poolEntryFromTx(t, tx, 0), baseFee, minTip, 0, balance))

	eligible := pool.eligible(map[common.Address]uint64{testSender: 0}, baseFee)
	assert.Empty(t, eligible)
}

func TestMempool_EligibleSkipsLowFeeCap(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 0)
	baseFee := big.NewInt(1_000)
	minTip := big.NewInt(100)
	balance := big.NewInt(1e18)

	tx := signedTx(t, 0, big.NewInt(200), big.NewInt(3_000))
	require.NoError(t, pool.add(poolEntryFromTx(t, tx, 0), baseFee, minTip, 0, balance))

	eligible := pool.eligible(map[common.Address]uint64{}, big.NewInt(4_000))
	assert.Empty(t, eligible)
}

func TestMempool_PendingNonce(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 0)
	baseFee := big.NewInt(1_000)
	minTip := big.NewInt(100)
	balance := big.NewInt(1e18)

	for nonce := uint64(5); nonce <= 7; nonce++ {
		tx := signedTx(t, nonce, big.NewInt(200), big.NewInt(3_000))
		require.NoError(t, pool.add(poolEntryFromTx(t, tx, 0), baseFee, minTip, 5, balance))
	}

	assert.Equal(t, uint64(8), pool.pendingNonce(testSender, 5))
}

func TestMempool_PendingNonceGap(t *testing.T) {
	t.Parallel()

	pool := newMempool(0, 0)
	baseFee := big.NewInt(1_000)
	minTip := big.NewInt(100)
	balance := big.NewInt(1e18)

	tx5 := signedTx(t, 5, big.NewInt(200), big.NewInt(3_000))
	tx7 := signedTx(t, 7, big.NewInt(200), big.NewInt(3_000))
	require.NoError(t, pool.add(poolEntryFromTx(t, tx5, 0), baseFee, minTip, 5, balance))
	require.NoError(t, pool.add(poolEntryFromTx(t, tx7, 0), baseFee, minTip, 5, balance))

	assert.Equal(t, uint64(6), pool.pendingNonce(testSender, 5))
}
