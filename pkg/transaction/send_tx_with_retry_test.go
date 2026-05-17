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

func TestEscalateGasTip(t *testing.T) {
	t.Parallel()

	t.Run("nil returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, transaction.EscalateGasTip(nil, 20))
	})

	t.Run("single step x1.2", func(t *testing.T) {
		t.Parallel()
		got := transaction.EscalateGasTip(big.NewInt(1000), 20)
		require.NotNil(t, got)
		assert.Equal(t, int64(1200), got.Int64())
	})

	t.Run("compound steps", func(t *testing.T) {
		t.Parallel()
		tip := big.NewInt(1000)
		tip = transaction.EscalateGasTip(tip, 20) // 1200
		assert.Equal(t, int64(1200), tip.Int64())
		tip = transaction.EscalateGasTip(tip, 20) // 1440
		assert.Equal(t, int64(1440), tip.Int64())
		tip = transaction.EscalateGasTip(tip, 20) // 1728
		assert.Equal(t, int64(1728), tip.Int64())
	})
}

// retryTestSetup holds shared constants and helpers for SendWithRetry tests.
type retryTestSetup struct {
	sender    common.Address
	recipient common.Address
	chainID   *big.Int
	nonce     uint64
	txData    []byte
	value     *big.Int
	lowTip    *big.Int
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
		lowTip:    big.NewInt(100),
		baseFee:   big.NewInt(1000),
		gasLimit:  uint64(50000),
	}
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
			LowTip:        new(big.Int).Set(s.lowTip),
			MarketTip:     new(big.Int).Mul(s.lowTip, big.NewInt(2)),
			AggressiveTip: new(big.Int).Mul(s.lowTip, big.NewInt(3)),
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

// Broadcast returns critical error → immediate exit.
func TestSendWithRetry_BroadcastCriticalError(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var feeHistoryCalls atomic.Int32

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(&feeHistoryCalls),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
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

	var rs transaction.RetryState
	assert.ErrorIs(t, store.Get(transaction.RetryStateKey(s.nonce), &rs), storage.ErrNotFound,
		"retry state should be cleaned up after critical error")
}

// WaitForReceipt returns critical error → immediate exit.
func TestSendWithRetry_WaitForReceiptCriticalError(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var feeHistoryCalls atomic.Int32

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(&feeHistoryCalls),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
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

	var broadcastCount atomic.Int32

	var feeHistoryCalls atomic.Int32

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(&feeHistoryCalls),
			s.estimateGasOption(),
			backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
				n := broadcastCount.Add(1)
				if n == 1 {
					return nil, errors.New("temporary RPC error")
				}
				return &types.Header{BaseFee: new(big.Int).Set(s.baseFee)}, nil
			}),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
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

	assert.GreaterOrEqual(t, int(broadcastCount.Load()), 2, "should have retried after non-critical error")
	assert.Equal(t, int32(2), feeHistoryCalls.Load(),
		"fee history called twice: first attempt failed before tip was set, so second attempt fetches it again")

	var rs transaction.RetryState
	assert.ErrorIs(t, store.Get(transaction.RetryStateKey(s.nonce), &rs), storage.ErrNotFound,
		"retry state should be cleaned up on success")
	assert.ErrorIs(t, store.Get(transaction.PendingTransactionKey(txHash), &struct{}{}), storage.ErrNotFound,
		"pending tx entry should be cleaned up on success")
}

// First broadcast OK, receipt not found (timeout), second broadcast with escalated gas → receipt found.
func TestSendWithRetry_EscalateGasThenSuccess(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var broadcastCount atomic.Int32
	var feeHistoryCalls atomic.Int32
	var capturedTips []*big.Int

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(&feeHistoryCalls),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				broadcastCount.Add(1)
				capturedTips = append(capturedTips, new(big.Int).Set(tx.GasTipCap()))
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

	require.Len(t, capturedTips, 2, "should have exactly 2 broadcast attempts")
	assert.True(t, capturedTips[1].Cmp(capturedTips[0]) > 0,
		"second attempt tip (%s) should be higher than first (%s)", capturedTips[1], capturedTips[0])
	assert.Equal(t, int32(1), feeHistoryCalls.Load(),
		"fee history called once: PreviousTip known after first broadcast, retries use escalated tip")

	var rs transaction.RetryState
	assert.ErrorIs(t, store.Get(transaction.RetryStateKey(s.nonce), &rs), storage.ErrNotFound,
		"retry state should be cleaned up on success")
}

// All attempts exhausted, receipt never found → error.
func TestSendWithRetry_AllAttemptsExhausted(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var broadcastCount atomic.Int32
	var feeHistoryCalls atomic.Int32

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(&feeHistoryCalls),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				broadcastCount.Add(1)
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

	assert.Equal(t, int32(3), broadcastCount.Load(), "should have made exactly maxRetries attempts")
	assert.Equal(t, int32(1), feeHistoryCalls.Load(),
		"fee history called once: PreviousTip known after first broadcast, retries use escalated tip")

	var rs transaction.RetryState
	assert.ErrorIs(t, store.Get(transaction.RetryStateKey(s.nonce), &rs), storage.ErrNotFound,
		"retry state should be cleaned up after exhaustion")
}

// Resume after node restart — transaction is re-sent starting from persisted attempt.
func TestSendWithRetry_ResumeAfterRestart(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	previousTip := new(big.Int).Set(s.lowTip)
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

	var resumedNonce uint64
	var resumedAttemptTip *big.Int

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
				resumedNonce = tx.Nonce()
				resumedAttemptTip = new(big.Int).Set(tx.GasTipCap())
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
		return resumedAttemptTip != nil
	}, 5*time.Second, 10*time.Millisecond, "resume should have triggered a broadcast")

	assert.Equal(t, s.nonce, resumedNonce, "resumed transaction must use the same nonce")

	expectedTip := transaction.EscalateGasTip(previousTip, 20)
	assert.Equal(t, expectedTip.Int64(), resumedAttemptTip.Int64(),
		"resumed transaction should use escalated tip (one step from persisted PreviousTip)")

	assert.Equal(t, int32(0), feeHistoryCalls.Load(),
		"fee history should NOT be called on resume — tip is restored from persisted state")

	var rs transaction.RetryState
	assert.Eventually(t, func() bool {
		return errors.Is(store.Get(retryKey, &rs), storage.ErrNotFound)
	}, 5*time.Second, 10*time.Millisecond, "retry state should be cleaned up after success")
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
