// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"context"
	"errors"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	signermock "github.com/ethersphere/bee/v2/pkg/crypto/mock"
	"github.com/ethersphere/bee/v2/pkg/log"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	"github.com/ethersphere/bee/v2/pkg/transaction/monitormock"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuggestGasFeeGasTipCapWithHistory(t *testing.T) {
	t.Parallel()

	const (
		baseFee        = int64(1000)
		tipBase        = int64(100)
		marketTip      = int64(200) // tipBase * 2
		gasIncreasePct = 20
		prevTip        = int64(1000)
		escalatedTip   = int64(1200) // prevTip * 1.2
		baseFeeCap     = int64(2000) // baseFee * 2
	)

	headerOption := func() backendmock.Option {
		return backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
			return &types.Header{BaseFee: big.NewInt(baseFee)}, nil
		})
	}

	feeHistoryOption := func(called *atomic.Int32) backendmock.Option {
		return backendmock.WithGetFeeAndTipsFromFeeHistoryFunc(func(ctx context.Context, lastBlock *big.Int) (*transaction.FeeHistorySuggestedFeeAndTips, error) {
			if called != nil {
				called.Add(1)
			}
			return &transaction.FeeHistorySuggestedFeeAndTips{
				LowTip:        big.NewInt(tipBase),
				MarketTip:     big.NewInt(marketTip),
				AggressiveTip: big.NewInt(tipBase * 3),
				LatestBaseFee: big.NewInt(baseFee),
			}, nil
		})
	}

	t.Run("prevGasTipCap nil uses market tip from fee history", func(t *testing.T) {
		t.Parallel()

		var feeHistoryCalls atomic.Int32
		backend := backendmock.New(headerOption(), feeHistoryOption(&feeHistoryCalls))

		gasFeeCap, gasTipCap, err := transaction.SuggestGasFeeGasTipCapWithHistory(
			backend, gasIncreasePct, nil, context.Background(), nil,
		)

		require.NoError(t, err)
		assert.Equal(t, int32(1), feeHistoryCalls.Load())
		assert.Equal(t, marketTip, gasTipCap.Int64())
		assert.Equal(t, baseFeeCap+marketTip, gasFeeCap.Int64())
	})

	t.Run("escalates previous tip by configured percent", func(t *testing.T) {
		t.Parallel()

		var feeHistoryCalls atomic.Int32
		backend := backendmock.New(headerOption(), feeHistoryOption(&feeHistoryCalls))

		gasFeeCap, gasTipCap, err := transaction.SuggestGasFeeGasTipCapWithHistory(
			backend, gasIncreasePct, nil, context.Background(), big.NewInt(prevTip),
		)

		require.NoError(t, err)
		assert.Equal(t, int32(0), feeHistoryCalls.Load(), "fee history must not be called when previous tip is set")
		assert.Equal(t, escalatedTip, gasTipCap.Int64())
		assert.Equal(t, baseFeeCap+escalatedTip, gasFeeCap.Int64())
	})

	t.Run("max tx price exceeded falls back to previous tip", func(t *testing.T) {
		t.Parallel()

		// escalated: 2000+1200=3200, previous: 2000+1000=3000
		maxTxPrice := big.NewInt(baseFeeCap + prevTip + 100)

		backend := backendmock.New(headerOption())

		gasFeeCap, gasTipCap, err := transaction.SuggestGasFeeGasTipCapWithHistory(
			backend, gasIncreasePct, maxTxPrice, context.Background(), big.NewInt(prevTip),
		)

		require.NoError(t, err)
		assert.Equal(t, prevTip, gasTipCap.Int64(), "must fall back to previous tip without escalation")
		assert.Equal(t, baseFeeCap+prevTip, gasFeeCap.Int64())
	})

	t.Run("max tx price exceeded and previous tip also exceeds limit returns error", func(t *testing.T) {
		t.Parallel()

		maxTxPrice := big.NewInt(baseFeeCap + prevTip - 1)

		backend := backendmock.New(headerOption())

		gasFeeCap, gasTipCap, err := transaction.SuggestGasFeeGasTipCapWithHistory(
			backend, gasIncreasePct, maxTxPrice, context.Background(), big.NewInt(prevTip),
		)

		assert.ErrorIs(t, err, transaction.ErrTxMaxPriceExceeded)
		assert.Nil(t, gasFeeCap)
		assert.Nil(t, gasTipCap)
	})
}

// capturedBroadcast records the parameters of a transaction as seen by SendTransaction.
type capturedBroadcast struct {
	Nonce     uint64
	GasTipCap *big.Int
	GasFeeCap *big.Int
	GasLimit  uint64
	To        *common.Address
	Data      []byte
	Value     *big.Int
}

func captureTx(tx *types.Transaction) capturedBroadcast {
	return capturedBroadcast{
		Nonce:     tx.Nonce(),
		GasTipCap: new(big.Int).Set(tx.GasTipCap()),
		GasFeeCap: new(big.Int).Set(tx.GasFeeCap()),
		GasLimit:  tx.Gas(),
		To:        tx.To(),
		Data:      tx.Data(),
		Value:     new(big.Int).Set(tx.Value()),
	}
}

// assertTxDataUnchanged verifies that nonce, to, data, value, and gas limit
// are identical across all broadcast attempts (only fees should change).
func assertTxDataUnchanged(t *testing.T, broadcasts []capturedBroadcast) {
	t.Helper()
	for i := 1; i < len(broadcasts); i++ {
		assert.Equal(t, broadcasts[0].Nonce, broadcasts[i].Nonce,
			"attempt %d: nonce must not change across retries", i)
		assert.Equal(t, broadcasts[0].To, broadcasts[i].To,
			"attempt %d: To must not change across retries", i)
		assert.Equal(t, broadcasts[0].Data, broadcasts[i].Data,
			"attempt %d: Data must not change across retries", i)
		assert.True(t, broadcasts[0].Value.Cmp(broadcasts[i].Value) == 0,
			"attempt %d: Value must not change across retries (got %s, want %s)", i, broadcasts[i].Value, broadcasts[0].Value)
		assert.Equal(t, broadcasts[0].GasLimit, broadcasts[i].GasLimit,
			"attempt %d: GasLimit must not change across retries", i)
	}
}

// retryTestSetup holds shared constants and helpers for SendWithRetry tests.
type retryTestSetup struct {
	sender    common.Address
	recipient common.Address
	chainID   *big.Int
	nonce     uint64
	txData    []byte
	value     *big.Int
	tipBase   *big.Int // base value for fee tiers: LowTip=tipBase, MarketTip=tipBase*2, AggressiveTip=tipBase*3
	baseFee   *big.Int
	gasLimit  uint64
}

func newRetryTestSetup() retryTestSetup {
	return retryTestSetup{
		sender:    common.HexToAddress("0xddff"),
		recipient: common.HexToAddress("0xabcd"),
		chainID:   big.NewInt(5),
		nonce:     uint64(2),
		txData:    common.Hex2Bytes("abcdee"),
		value:     big.NewInt(1),
		tipBase:   big.NewInt(100),
		baseFee:   big.NewInt(1000),
		gasLimit:  uint64(50000),
	}
}

func (s retryTestSetup) expectedMarketTip() *big.Int {
	return new(big.Int).Mul(s.tipBase, big.NewInt(2))
}

func (s retryTestSetup) expectedGasFeeCap(tip *big.Int) *big.Int {
	return new(big.Int).Add(new(big.Int).Mul(s.baseFee, big.NewInt(2)), tip)
}

func (s retryTestSetup) retryConfig() transaction.ServiceRetryConfig {
	return transaction.ServiceRetryConfig{
		MaxRetries:         3,
		RetryDelay:         50 * time.Millisecond,
		GasIncreasePercent: 20,
		MaxTxPrice:         big.NewInt(100_000_000),
	}
}

func (s retryTestSetup) request() *transaction.TxRequest {
	return &transaction.TxRequest{
		To:       &s.recipient,
		Data:     s.txData,
		Value:    s.value,
		GasLimit: s.gasLimit,
	}
}

func (s retryTestSetup) passThroughSigner() signermock.Option {
	return signermock.WithSignTxFunc(func(tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
		return tx, nil
	})
}

func (s retryTestSetup) signerAddr() signermock.Option {
	return signermock.WithEthereumAddressFunc(func() (common.Address, error) {
		return s.sender, nil
	})
}

func (s retryTestSetup) feeHistoryOption(counter *atomic.Int32) backendmock.Option {
	return backendmock.WithGetFeeAndTipsFromFeeHistoryFunc(func(ctx context.Context, lastBlock *big.Int) (*transaction.FeeHistorySuggestedFeeAndTips, error) {
		if counter != nil {
			counter.Add(1)
		}
		return &transaction.FeeHistorySuggestedFeeAndTips{
			LowTip:        new(big.Int).Set(s.tipBase),
			MarketTip:     new(big.Int).Mul(s.tipBase, big.NewInt(2)),
			AggressiveTip: new(big.Int).Mul(s.tipBase, big.NewInt(3)),
			LatestBaseFee: new(big.Int).Set(s.baseFee),
		}, nil
	})
}

func (s retryTestSetup) headerOption() backendmock.Option {
	return backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
		return &types.Header{BaseFee: new(big.Int).Set(s.baseFee)}, nil
	})
}

func (s retryTestSetup) nonceOption() backendmock.Option {
	var counter atomic.Uint64
	counter.Store(s.nonce)
	return backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
		return counter.Add(1) - 1, nil
	})
}

func (s retryTestSetup) estimateGasOption() backendmock.Option {
	return backendmock.WithEstimateGasFunc(func(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
		return s.gasLimit, nil
	})
}

// receiptWatchOK returns a monitor option that immediately returns a successful receipt.
func receiptWatchOK(status uint64) monitormock.Option {
	return monitormock.WithWatchTransactionFunc(func(txHash common.Hash, nonce uint64) (<-chan types.Receipt, <-chan error, error) {
		ch := make(chan types.Receipt, 1)
		ch <- types.Receipt{TxHash: txHash, Status: status}
		return ch, nil, nil
	})
}

// receiptWatchTimeout returns a monitor option that never returns a receipt (for testing timeout).
func receiptWatchTimeout() monitormock.Option {
	return monitormock.WithWatchTransactionFunc(func(txHash common.Hash, nonce uint64) (<-chan types.Receipt, <-chan error, error) {
		return make(chan types.Receipt), make(chan error), nil
	})
}

// receiptWatchErr returns a monitor option that returns an error on the error channel.
func receiptWatchErr(err error) monitormock.Option {
	return monitormock.WithWatchTransactionFunc(func(txHash common.Hash, nonce uint64) (<-chan types.Receipt, <-chan error, error) {
		ch := make(chan error, 1)
		ch <- err
		return nil, ch, nil
	})
}

// Broadcast returns critical error → immediate exit, verify tx was built correctly.
func TestSendWithRetry_BroadcastCriticalError(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var feeHistoryCalls atomic.Int32
	var broadcasts []capturedBroadcast

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(&feeHistoryCalls),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				broadcasts = append(broadcasts, captureTx(tx))
				return errors.New("execution reverted")
			}),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "execution reverted")
	assert.Equal(t, common.Hash{}, txHash)
	assert.Nil(t, receipt)

	assert.Equal(t, int32(1), feeHistoryCalls.Load(), "fee history must be called once")

	require.Len(t, broadcasts, 1, "exactly one broadcast before critical error")
	marketTip := s.expectedMarketTip()
	assert.Equal(t, marketTip.Int64(), broadcasts[0].GasTipCap.Int64(),
		"initial tip must be MarketTip")
	assert.Equal(t, s.expectedGasFeeCap(marketTip).Int64(), broadcasts[0].GasFeeCap.Int64(),
		"gasFeeCap must be baseFee*2 + MarketTip")
	assert.Equal(t, s.recipient, *broadcasts[0].To)
	assert.Equal(t, s.txData, broadcasts[0].Data)
	assert.Equal(t, s.value.Int64(), broadcasts[0].Value.Int64())
	assert.Equal(t, s.gasLimit, broadcasts[0].GasLimit)

	var rs transaction.RetryState
	assert.ErrorIs(t, store.Get(transaction.RetryStateKey(s.nonce), &rs), storage.ErrNotFound,
		"retry state should be cleaned up after critical error")
}

// WaitForReceipt returns critical error → immediate exit, verify tx params.
func TestSendWithRetry_WaitForReceiptCriticalError(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var feeHistoryCalls atomic.Int32
	var broadcasts []capturedBroadcast

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(&feeHistoryCalls),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				broadcasts = append(broadcasts, captureTx(tx))
				return nil
			}),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(receiptWatchErr(transaction.ErrTransactionCancelled)),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	_, _, err = svc.SendWithRetry(context.Background(), s.request())
	assert.Error(t, err)

	assert.Equal(t, int32(1), feeHistoryCalls.Load(),
		"fee history called once: tip was set after first broadcast, no more calls needed")

	require.Len(t, broadcasts, 1, "exactly one broadcast before critical wait error")
	marketTip := s.expectedMarketTip()
	assert.Equal(t, marketTip.Int64(), broadcasts[0].GasTipCap.Int64(),
		"initial tip must be MarketTip")
	assert.Equal(t, s.expectedGasFeeCap(marketTip).Int64(), broadcasts[0].GasFeeCap.Int64(),
		"gasFeeCap must be baseFee*2 + MarketTip")

	var rs transaction.RetryState
	assert.ErrorIs(t, store.Get(transaction.RetryStateKey(s.nonce), &rs), storage.ErrNotFound,
		"retry state should be cleaned up after critical WaitForReceipt error")
}

// updateStates returns any error → immediate exit.
func TestSendWithRetry_UpdateStateError(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()

	putErr := errors.New("disk write failed")
	callCount := 0
	failingStore := &failOnNthPutStore{
		StateStorer: storemock.NewStateStore(),
		failOnPut:   1,
		putErr:      putErr,
		callCount:   &callCount,
	}

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(nil),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				return nil
			}),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		failingStore,
		s.chainID,
		monitormock.New(),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	_, _, err = svc.SendWithRetry(context.Background(), s.request())
	assert.ErrorIs(t, err, putErr)
}

// First broadcast fails (non-critical, signedTx nil because prepare fails), second succeeds.
// On the first attempt HeaderByNumber fails → prepareTransactionWithRetry fails → broadcastTxWithRetry
// returns (nil, err) with a non-critical error.
// UpdateStates receives nil signedTx → state is not updated, only number of attempt increased in-memory
// After retry delay, second broadcast attempt succeeds → receipt → exit.
func TestSendWithRetry_NonCriticalThenSuccess(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var headerCalls atomic.Int32
	var feeHistoryCalls atomic.Int32
	var broadcasts []capturedBroadcast

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(&feeHistoryCalls),
			s.estimateGasOption(),
			backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
				n := headerCalls.Add(1)
				if n == 1 {
					return nil, errors.New("temporary RPC error")
				}
				return &types.Header{BaseFee: new(big.Int).Set(s.baseFee)}, nil
			}),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				broadcasts = append(broadcasts, captureTx(tx))
				return nil
			}),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(
			monitormock.WithWatchTransactionFunc(func(txHash common.Hash, nonce uint64) (<-chan types.Receipt, <-chan error, error) {
				ch := make(chan types.Receipt, 1)
				ch <- types.Receipt{TxHash: txHash, Status: 1}
				return ch, nil, nil
			}),
		),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	assert.NotEqual(t, common.Hash{}, txHash)
	require.NotNil(t, receipt)
	assert.Equal(t, uint64(1), receipt.Status)

	assert.GreaterOrEqual(t, int(headerCalls.Load()), 2, "should have retried after non-critical error")
	assert.Equal(t, int32(1), feeHistoryCalls.Load(),
		"fee history called once: first attempt failed at HeaderByNumber before reaching fee history")

	require.Len(t, broadcasts, 1, "only one successful broadcast (first attempt failed before SendTransaction)")
	marketTip := s.expectedMarketTip()
	assert.Equal(t, marketTip.Int64(), broadcasts[0].GasTipCap.Int64(),
		"tip must be MarketTip (no previous tip was set since first attempt failed)")
	assert.Equal(t, s.expectedGasFeeCap(marketTip).Int64(), broadcasts[0].GasFeeCap.Int64(),
		"gasFeeCap must be baseFee*2 + MarketTip")
	assert.Equal(t, s.recipient, *broadcasts[0].To)
	assert.Equal(t, s.txData, broadcasts[0].Data)

	var rs transaction.RetryState
	assert.ErrorIs(t, store.Get(transaction.RetryStateKey(broadcasts[0].Nonce), &rs), storage.ErrNotFound,
		"retry state should be cleaned up on success")
	assert.ErrorIs(t, store.Get(transaction.PendingTransactionKey(txHash), &struct{}{}), storage.ErrNotFound,
		"pending tx entry should be cleaned up on success")
}

// First broadcast OK, receipt not found (timeout), second broadcast with escalated gas → receipt found.
// Verifies exact fee values, nonce immutability, and tx data immutability.
func TestSendWithRetry_EscalateGasThenSuccess(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var broadcastCount atomic.Int32
	var feeHistoryCalls atomic.Int32
	var broadcasts []capturedBroadcast

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(&feeHistoryCalls),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				broadcastCount.Add(1)
				broadcasts = append(broadcasts, captureTx(tx))
				return nil
			}),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(
			monitormock.WithWatchTransactionFunc(func(txHash common.Hash, nonce uint64) (<-chan types.Receipt, <-chan error, error) {
				if broadcastCount.Load() <= 1 {
					return make(chan types.Receipt), make(chan error), nil
				}
				ch := make(chan types.Receipt, 1)
				ch <- types.Receipt{TxHash: txHash, Status: 1}
				return ch, nil, nil
			}),
		),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	assert.NotEqual(t, common.Hash{}, txHash)
	require.NotNil(t, receipt)
	assert.Equal(t, uint64(1), receipt.Status)

	require.Len(t, broadcasts, 2, "should have exactly 2 broadcast attempts")

	assertTxDataUnchanged(t, broadcasts)

	marketTip := s.expectedMarketTip()
	assert.Equal(t, marketTip.Int64(), broadcasts[0].GasTipCap.Int64(),
		"first attempt must use MarketTip from fee history")
	assert.Equal(t, s.expectedGasFeeCap(marketTip).Int64(), broadcasts[0].GasFeeCap.Int64(),
		"first attempt gasFeeCap = baseFee*2 + MarketTip")

	escalatedTip := transaction.EscalateGasTip(marketTip, 20)
	assert.Equal(t, escalatedTip.Int64(), broadcasts[1].GasTipCap.Int64(),
		"second attempt must use escalated tip (MarketTip * 1.2)")
	assert.Equal(t, s.expectedGasFeeCap(escalatedTip).Int64(), broadcasts[1].GasFeeCap.Int64(),
		"second attempt gasFeeCap = baseFee*2 + escalated tip")

	assert.Equal(t, int32(1), feeHistoryCalls.Load(),
		"fee history called once: PreviousTip known after first broadcast, retries use escalated tip")

	var rs transaction.RetryState
	assert.ErrorIs(t, store.Get(transaction.RetryStateKey(broadcasts[0].Nonce), &rs), storage.ErrNotFound,
		"retry state should be cleaned up on success")
}

// All attempts exhausted, receipt never found → error.
// Verifies compound escalation chain, nonce immutability, and gasFeeCap on every attempt.
func TestSendWithRetry_AllAttemptsExhausted(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var feeHistoryCalls atomic.Int32
	var broadcasts []capturedBroadcast

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(&feeHistoryCalls),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				broadcasts = append(broadcasts, captureTx(tx))
				return nil
			}),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(receiptWatchTimeout()),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction failed after 3 attempts")
	assert.NotEqual(t, common.Hash{}, txHash, "should return last tx hash even on exhaustion")
	assert.Nil(t, receipt)

	require.Len(t, broadcasts, 3, "should have made exactly maxRetries attempts")

	assertTxDataUnchanged(t, broadcasts)

	tip0 := s.expectedMarketTip()                // tipBase*2 = 200
	tip1 := transaction.EscalateGasTip(tip0, 20) // 240
	tip2 := transaction.EscalateGasTip(tip1, 20) // 288
	expectedTips := []*big.Int{tip0, tip1, tip2}

	for i, expectedTip := range expectedTips {
		assert.Equal(t, expectedTip.Int64(), broadcasts[i].GasTipCap.Int64(),
			"attempt %d: tip must match compound escalation chain", i)
		assert.Equal(t, s.expectedGasFeeCap(expectedTip).Int64(), broadcasts[i].GasFeeCap.Int64(),
			"attempt %d: gasFeeCap must be baseFee*2 + tip", i)
	}

	assert.Equal(t, int32(1), feeHistoryCalls.Load(),
		"fee history called once: PreviousTip known after first broadcast, retries use escalated tip")

	var rs transaction.RetryState
	assert.ErrorIs(t, store.Get(transaction.RetryStateKey(broadcasts[0].Nonce), &rs), storage.ErrNotFound,
		"retry state should be cleaned up after exhaustion")
}

// Resume after node restart — transaction is re-sent starting from persisted attempt.
// Verifies nonce, escalated tip, gasFeeCap, and that fee history is NOT called.
func TestSendWithRetry_ResumeAfterRestart(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	previousTip := new(big.Int).Set(s.tipBase)
	lastTxHash := common.HexToHash("0xdeadbeef")

	priorState := transaction.RetryState{
		Nonce:         s.nonce,
		NonceAssigned: true,
		NextAttempt:   1,
		LastTxHash:    lastTxHash,
		AllTxHashes:   nil,
		GasLimit:      s.gasLimit,
		To:            &s.recipient,
		Data:          s.txData,
		Value:         s.value,
		Description:   "test-resume",
		PreviousTip:   previousTip,
	}

	retryKey := transaction.RetryStateKey(s.nonce)
	require.NoError(t, store.Put(retryKey, priorState))

	require.NoError(t, store.Put(transaction.StoredTransactionKey(lastTxHash), transaction.StoredTransaction{
		To:        &s.recipient,
		Data:      s.txData,
		GasLimit:  s.gasLimit,
		Value:     s.value,
		Nonce:     s.nonce,
		GasTipCap: previousTip,
		GasFeeCap: big.NewInt(5000),
		GasPrice:  big.NewInt(0),
		Created:   time.Now().Unix(),
	}))
	require.NoError(t, store.Put(transaction.PendingTransactionKey(lastTxHash), struct{}{}))

	var broadcasts []capturedBroadcast
	var feeHistoryCalls atomic.Int32

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.feeHistoryOption(&feeHistoryCalls),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
				return s.nonce, nil
			}),
			backendmock.WithNonceAtFunc(func(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
				return s.nonce, nil
			}),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				broadcasts = append(broadcasts, captureTx(tx))
				return nil
			}),
			backendmock.WithTransactionByHashFunc(func(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error) {
				return nil, false, ethereum.NotFound
			}),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(
			monitormock.WithWatchTransactionFunc(func(txHash common.Hash, nonce uint64) (<-chan types.Receipt, <-chan error, error) {
				ch := make(chan types.Receipt, 1)
				ch <- types.Receipt{TxHash: txHash, Status: 1}
				return ch, nil, nil
			}),
		),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	require.Eventually(t, func() bool {
		return len(broadcasts) > 0
	}, 5*time.Second, 10*time.Millisecond, "resume should have triggered a broadcast")

	require.Len(t, broadcasts, 1)
	assert.Equal(t, s.nonce, broadcasts[0].Nonce, "resumed transaction must use the same nonce")

	expectedTip := transaction.EscalateGasTip(previousTip, 20)
	assert.Equal(t, expectedTip.Int64(), broadcasts[0].GasTipCap.Int64(),
		"resumed transaction should use escalated tip (one step from persisted PreviousTip)")

	assert.Equal(t, s.expectedGasFeeCap(expectedTip).Int64(), broadcasts[0].GasFeeCap.Int64(),
		"resumed gasFeeCap must be baseFee*2 + escalated tip")

	assert.Equal(t, int32(0), feeHistoryCalls.Load(),
		"fee history should NOT be called on resume — tip is restored from persisted state")

	var rs transaction.RetryState
	assert.Eventually(t, func() bool {
		return errors.Is(store.Get(retryKey, &rs), storage.ErrNotFound)
	}, 5*time.Second, 10*time.Millisecond, "retry state should be cleaned up after success")
}

// MaxTxPrice cap prevents escalation beyond the configured limit.
func TestSendWithRetry_MaxTxPriceCap(t *testing.T) {
	t.Parallel()

	t.Run("escalation capped to previous tip", func(t *testing.T) {
		t.Parallel()
		s := newRetryTestSetup()
		store := storemock.NewStateStore()
		testutil.CleanupCloser(t, store)

		marketTip := s.expectedMarketTip() // 200
		// Set maxTxPrice so that first attempt (baseFee*2 + MarketTip = 2200) fits,
		// but escalated (baseFee*2 + 240 = 2240) exceeds the cap.
		maxTxPrice := new(big.Int).Add(s.expectedGasFeeCap(marketTip), big.NewInt(10)) // 2210

		cfg := s.retryConfig()
		cfg.MaxTxPrice = maxTxPrice

		var broadcasts []capturedBroadcast

		svc, err := transaction.NewService(log.Noop, s.sender,
			backendmock.New(
				s.nonceOption(),
				s.feeHistoryOption(nil),
				s.headerOption(),
				s.estimateGasOption(),
				backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
					broadcasts = append(broadcasts, captureTx(tx))
					return nil
				}),
			),
			signermock.New(s.passThroughSigner(), s.signerAddr()),
			store,
			s.chainID,
			monitormock.New(receiptWatchTimeout()),
			0,
			cfg,
		)
		require.NoError(t, err)
		testutil.CleanupCloser(t, svc)

		_, _, err = svc.SendWithRetry(context.Background(), s.request())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "transaction failed after 3 attempts")

		require.Len(t, broadcasts, 3, "all 3 attempts should have been sent")

		assertTxDataUnchanged(t, broadcasts)

		for i, bc := range broadcasts {
			assert.Equal(t, marketTip.Int64(), bc.GasTipCap.Int64(),
				"attempt %d: tip must stay at MarketTip (escalation capped by maxTxPrice)", i)
			assert.Equal(t, s.expectedGasFeeCap(marketTip).Int64(), bc.GasFeeCap.Int64(),
				"attempt %d: gasFeeCap must be baseFee*2 + MarketTip (capped)", i)
		}
	})

	t.Run("exceeds limit on first attempt", func(t *testing.T) {
		t.Parallel()
		s := newRetryTestSetup()
		store := storemock.NewStateStore()
		testutil.CleanupCloser(t, store)

		marketTip := s.expectedMarketTip()
		// Set maxTxPrice below baseFee*2 + MarketTip so even the first attempt fails.
		maxTxPrice := new(big.Int).Sub(s.expectedGasFeeCap(marketTip), big.NewInt(1)) // 2199

		cfg := s.retryConfig()
		cfg.MaxTxPrice = maxTxPrice

		var broadcasts []capturedBroadcast

		svc, err := transaction.NewService(log.Noop, s.sender,
			backendmock.New(
				s.nonceOption(),
				s.feeHistoryOption(nil),
				s.headerOption(),
				s.estimateGasOption(),
				backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
					broadcasts = append(broadcasts, captureTx(tx))
					return nil
				}),
			),
			signermock.New(s.passThroughSigner(), s.signerAddr()),
			store,
			s.chainID,
			monitormock.New(receiptWatchTimeout()),
			0,
			cfg,
		)
		require.NoError(t, err)
		testutil.CleanupCloser(t, svc)

		_, _, err = svc.SendWithRetry(context.Background(), s.request())
		assert.Error(t, err)
		assert.Len(t, broadcasts, 0,
			"no transaction should be sent when maxTxPrice is below the minimum fee")
	})
}

// failOnNthPutStore wraps a StateStorer and fails the Nth Put call with putErr.
type failOnNthPutStore struct {
	storage.StateStorer
	failOnPut int
	putErr    error
	callCount *int
}

func (s *failOnNthPutStore) Put(key string, i any) error {
	*s.callCount++
	if *s.callCount >= s.failOnPut {
		return s.putErr
	}
	return s.StateStorer.Put(key, i)
}
