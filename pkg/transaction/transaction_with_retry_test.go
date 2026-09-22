// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	signermock "github.com/ethersphere/bee/v2/pkg/crypto/mock"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	"github.com/ethersphere/bee/v2/pkg/transaction/monitormock"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuggestGasFeeForTier(t *testing.T) {
	t.Parallel()

	const (
		baseFee      = int64(1000)
		tipBase      = int64(100)
		marketTip    = int64(200) // tipBase * 2
		prevTip      = int64(1000)
		escalatedTip = int64(1150) // prevTip * 1.15
		baseFeeCap   = int64(2000) // baseFee * 2
	)

	headerOption := func() backendmock.Option {
		return backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
			return &types.Header{BaseFee: big.NewInt(baseFee)}, nil
		})
	}

	feeHistoryOption := func(called *atomic.Int32) backendmock.Option {
		return backendmock.WithSuggestedFeeAndTipsFromHistoryFunc(func(ctx context.Context, lastBlock *big.Int) (*transaction.FeeHistorySuggestedFeeAndTips, error) {
			if called != nil {
				called.Add(1)
			}
			return &transaction.FeeHistorySuggestedFeeAndTips{
				LowTip:        big.NewInt(tipBase),
				MarketTip:     big.NewInt(marketTip),
				AggressiveTip: big.NewInt(tipBase * 3),
			}, nil
		})
	}

	t.Run("previous tip nil uses market tip from fee history", func(t *testing.T) {
		t.Parallel()

		var feeHistoryCalls atomic.Int32
		backend := backendmock.New(headerOption(), feeHistoryOption(&feeHistoryCalls))

		gasFeeCap, gasTipCap, err := transaction.SuggestGasFeeForTier(
			backend, nil, context.Background(), int(transaction.FeeTierMarket), nil,
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

		gasFeeCap, gasTipCap, err := transaction.SuggestGasFeeForTier(
			backend, nil, context.Background(), int(transaction.FeeTierMarket), big.NewInt(prevTip),
		)

		require.NoError(t, err)
		assert.Equal(t, int32(1), feeHistoryCalls.Load(), "fee history is always queried")
		assert.Equal(t, escalatedTip, gasTipCap.Int64())
		assert.Equal(t, baseFeeCap+escalatedTip, gasFeeCap.Int64())
	})

	t.Run("max tx price exceeded returns error", func(t *testing.T) {
		t.Parallel()

		// escalated: 2000+1150=3150
		maxTxPrice := big.NewInt(baseFeeCap + prevTip + 100)

		backend := backendmock.New(headerOption(), feeHistoryOption(nil))

		gasFeeCap, gasTipCap, err := transaction.SuggestGasFeeForTier(
			backend, maxTxPrice, context.Background(), int(transaction.FeeTierMarket), big.NewInt(prevTip),
		)

		assert.ErrorIs(t, err, transaction.ErrTxMaxPriceExceeded)
		assert.Nil(t, gasFeeCap)
		assert.Nil(t, gasTipCap)
	})
}

// capturedBroadcast records the parameters of a transaction as seen by SendTransaction.
type capturedBroadcast struct {
	Hash      common.Hash
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
		Hash:      tx.Hash(),
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

func (s retryTestSetup) retryConfig() transaction.TransactionsRetryConfig {
	return transaction.TransactionsRetryConfig{
		AttemptsPerTier: 3,
		StartTier:       "market",
		EndTier:         "market",
		RetryDelay:      50 * time.Millisecond,
		MaxTxPrice:      big.NewInt(100_000_000),
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
	return backendmock.WithSuggestedFeeAndTipsFromHistoryFunc(func(ctx context.Context, lastBlock *big.Int) (*transaction.FeeHistorySuggestedFeeAndTips, error) {
		if counter != nil {
			counter.Add(1)
		}
		return &transaction.FeeHistorySuggestedFeeAndTips{
			LowTip:        new(big.Int).Set(s.tipBase),
			MarketTip:     new(big.Int).Mul(s.tipBase, big.NewInt(2)),
			AggressiveTip: new(big.Int).Mul(s.tipBase, big.NewInt(3)),
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

func nonceWatchIdle() monitormock.Option {
	return monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error, error) {
		return make(chan struct{}), make(chan error), nil
	})
}

func nonceWatchSignal() monitormock.Option {
	return monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error, error) {
		doneC := make(chan struct{}, 1)
		doneC <- struct{}{}
		return doneC, make(chan error), nil
	})
}

func nonceWatchErr(err error) monitormock.Option {
	return monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error, error) {
		errC := make(chan error, 1)
		errC <- err
		return make(chan struct{}), errC, nil
	})
}

func receiptFound() backendmock.Option {
	return backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
		return &types.Receipt{TxHash: txHash, Status: 1}, nil
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

	var v string
	assert.ErrorIs(t, store.Get(transaction.RetryStateKey(s.nonce), &v), storage.ErrNotFound,
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
		monitormock.New(nonceWatchErr(transaction.ErrTransactionCancelled)),
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

	var v string
	assert.ErrorIs(t, store.Get(transaction.RetryStateKey(s.nonce), &v), storage.ErrNotFound,
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
		failOnPut:   3,
		putErr:      putErr,
		callCount:   &callCount,
	}
	doneC := make(chan struct{}, 1)
	var (
		txHash     common.Hash
		watchCount atomic.Int32
	)

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(nil),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				txHash = tx.Hash()
				return nil
			}),
			receiptFound(),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		failingStore,
		s.chainID,
		monitormock.New(
			monitormock.WithWatchNonceFunc(func(nonce uint64) (<-chan struct{}, <-chan error, error) {
				watchCount.Add(1)
				assert.Equal(t, s.nonce, nonce)
				return doneC, make(chan error), nil
			}),
		),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	_, _, err = svc.SendWithRetry(context.Background(), s.request())
	assert.ErrorIs(t, err, putErr)
	assert.Equal(t, int32(1), watchCount.Load(), "broadcast transaction must remain monitored")

	doneC <- struct{}{}
	require.Eventually(t, func() bool {
		return errors.Is(failingStore.Get(transaction.PendingTransactionKey(txHash), &struct{}{}), storage.ErrNotFound)
	}, time.Second, 10*time.Millisecond)
	assert.NoError(t, failingStore.Get(transaction.StoredTransactionKey(txHash), &transaction.StoredTransaction{}))
}

// First broadcast fails (non-critical, signedTx nil because prepare fails), second succeeds.
// On the first attempt HeaderByNumber fails → prepareTransactionWithRetry fails → broadcastTxWithRetry
// returns (nil, err) with a non-critical error.
// UpdateStates receives nil signedTx → state is not updated, only number of attempt increased in-memory
// After sendWithretry delay, second broadcast attempt succeeds → receipt → exit.
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
					// non-critical error
					return nil, errors.New("temporary RPC error")
				}
				return &types.Header{BaseFee: new(big.Int).Set(s.baseFee)}, nil
			}),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				broadcasts = append(broadcasts, captureTx(tx))
				return nil
			}),
			receiptFound(),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(nonceWatchSignal()),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.NotEqual(t, common.Hash{}, txHash)
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

	var v string
	assert.ErrorIs(t, store.Get(transaction.RetryStateKey(broadcasts[0].Nonce), &v), storage.ErrNotFound,
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
	doneC := make(chan struct{}, 1)

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(&feeHistoryCalls),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				broadcastCount.Add(1)
				broadcasts = append(broadcasts, captureTx(tx))
				if len(broadcasts) == 2 {
					first := broadcasts[0].Hash
					var stored transaction.StoredTransaction
					assert.NoError(t, store.Get(transaction.StoredTransactionKey(first), &stored),
						"superseded stored tx must remain until a result is known")
					var pending struct{}
					assert.NoError(t, store.Get(transaction.PendingTransactionKey(first), &pending),
						"superseded pending tx must remain until a result is known")
					doneC <- struct{}{}
				}
				return nil
			}),
			receiptFound(),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(
			monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error, error) {
				return doneC, make(chan error), nil
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

	escalatedTip := transaction.ApplyMempoolBump(marketTip)
	assert.Equal(t, escalatedTip.Int64(), broadcasts[1].GasTipCap.Int64(),
		"second attempt must use escalated tip (MarketTip * 1.15)")
	assert.Equal(t, s.expectedGasFeeCap(escalatedTip).Int64(), broadcasts[1].GasFeeCap.Int64(),
		"second attempt gasFeeCap = baseFee*2 + escalated tip")

	assert.Equal(t, int32(2), feeHistoryCalls.Load(),
		"fee history is called for each attempt")

	var v string
	assert.ErrorIs(t, store.Get(transaction.RetryStateKey(broadcasts[0].Nonce), &v), storage.ErrNotFound,
		"retry state should be cleaned up on success")

	firstTxHash := broadcasts[0].Hash
	lastTxHash := broadcasts[len(broadcasts)-1].Hash
	assert.ErrorIs(t, store.Get(transaction.StoredTransactionKey(firstTxHash), &struct{}{}), storage.ErrNotFound,
		"superseded stored tx should be removed on success")
	var stored transaction.StoredTransaction
	assert.NoError(t, store.Get(transaction.StoredTransactionKey(lastTxHash), &stored),
		"final stored tx should be kept")
}

func TestSendWithRetry_PreviousHashConfirmed(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var (
		broadcasts []capturedBroadcast
		watchCount atomic.Int32
		doneC      = make(chan struct{}, 1)
	)

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(nil),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				broadcasts = append(broadcasts, captureTx(tx))
				if len(broadcasts) == 2 {
					doneC <- struct{}{}
				}
				return nil
			}),
			backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
				if len(broadcasts) >= 2 && txHash == broadcasts[0].Hash {
					return &types.Receipt{TxHash: txHash, Status: 1}, nil
				}
				return nil, ethereum.NotFound
			}),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(
			monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error, error) {
				watchCount.Add(1)
				return doneC, make(chan error), nil
			}),
		),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Len(t, broadcasts, 2)
	assert.Equal(t, broadcasts[0].Hash, txHash)
	assert.Equal(t, broadcasts[0].Hash, receipt.TxHash)
	assert.Equal(t, int32(1), watchCount.Load(), "nonce is watched once")
}

func TestSendWithRetry_ReplacementHashConfirmed(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var (
		broadcasts []capturedBroadcast
		doneC      = make(chan struct{}, 1)
	)

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(nil),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				broadcasts = append(broadcasts, captureTx(tx))
				if len(broadcasts) == 3 {
					doneC <- struct{}{}
				}
				return nil
			}),
			backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
				if len(broadcasts) == 3 && txHash == broadcasts[1].Hash {
					return &types.Receipt{TxHash: txHash, Status: 1}, nil
				}
				return nil, ethereum.NotFound
			}),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(
			monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error, error) {
				return doneC, make(chan error), nil
			}),
		),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Len(t, broadcasts, 3)
	assert.Equal(t, broadcasts[1].Hash, txHash)

	var stored transaction.StoredTransaction
	assert.ErrorIs(t, store.Get(transaction.StoredTransactionKey(broadcasts[0].Hash), &stored), storage.ErrNotFound)
	assert.NoError(t, store.Get(transaction.StoredTransactionKey(broadcasts[1].Hash), &stored))
	assert.ErrorIs(t, store.Get(transaction.StoredTransactionKey(broadcasts[2].Hash), &stored), storage.ErrNotFound)
}

func TestSendWithRetry_ExternalNonceCancellation(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	doneC := make(chan struct{}, 1)
	var txHash common.Hash

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(nil),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				txHash = tx.Hash()
				doneC <- struct{}{}
				return nil
			}),
			backendmock.WithTransactionReceiptFunc(func(context.Context, common.Hash) (*types.Receipt, error) {
				return nil, ethereum.NotFound
			}),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(
			monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error, error) {
				return doneC, make(chan error), nil
			}),
		),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	_, _, err = svc.SendWithRetry(context.Background(), s.request())

	require.ErrorIs(t, err, transaction.ErrTransactionCancelled)
	var state transaction.RetriedTransaction
	assert.ErrorIs(t, store.Get(transaction.RetryStateKey(s.nonce), &state), storage.ErrNotFound)
	assert.ErrorIs(t, store.Get(transaction.PendingTransactionKey(txHash), &struct{}{}), storage.ErrNotFound)
	assert.ErrorIs(t, store.Get(transaction.StoredTransactionKey(txHash), &transaction.StoredTransaction{}), storage.ErrNotFound)
}

// After receipt timeout at market tier, fees escalate to the aggressive tier on the next broadcast.
func TestSendWithRetry_TierEscalation(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var (
		broadcasts []capturedBroadcast
		doneC      = make(chan struct{}, 1)
	)

	cfg := s.retryConfig()
	cfg.AttemptsPerTier = 1
	cfg.StartTier = "market"
	cfg.EndTier = "aggressive"

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(nil),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				broadcasts = append(broadcasts, captureTx(tx))
				if len(broadcasts) == 2 {
					doneC <- struct{}{}
				}
				return nil
			}),
			receiptFound(),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(
			monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error, error) {
				return doneC, make(chan error), nil
			}),
		),
		0,
		cfg,
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.NotEqual(t, common.Hash{}, txHash)

	require.Len(t, broadcasts, 2, "market then aggressive tier, one attempt each")

	marketTip := s.expectedMarketTip()
	aggressiveTip := new(big.Int).Mul(s.tipBase, big.NewInt(3))
	assert.Equal(t, marketTip.Int64(), broadcasts[0].GasTipCap.Int64(),
		"first broadcast must use market tier tip")
	assert.Equal(t, aggressiveTip.Int64(), broadcasts[1].GasTipCap.Int64(),
		"second broadcast must use aggressive tier tip")
	assert.Equal(t, s.expectedGasFeeCap(marketTip).Int64(), broadcasts[0].GasFeeCap.Int64())
	assert.Equal(t, s.expectedGasFeeCap(aggressiveTip).Int64(), broadcasts[1].GasFeeCap.Int64())
}

// Underpriced replacement keeps watching the pending tx hash instead of switching to the rejected one.
func TestSendWithRetry_UnderpricedKeepsPendingTxHash(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var (
		broadcastCount atomic.Int32
		watchCount     atomic.Int32
		firstTxHash    common.Hash
		doneC          = make(chan struct{}, 1)
	)

	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			s.nonceOption(),
			s.feeHistoryOption(nil),
			s.headerOption(),
			s.estimateGasOption(),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				if broadcastCount.Add(1) == 1 {
					firstTxHash = tx.Hash()
					return nil
				}
				doneC <- struct{}{}
				return errors.New("replacement transaction underpriced")
			}),
			receiptFound(),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(
			monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error, error) {
				watchCount.Add(1)
				return doneC, make(chan error), nil
			}),
		),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Equal(t, firstTxHash, txHash, "must return receipt for the original pending tx")
	assert.Equal(t, int32(2), broadcastCount.Load(), "second broadcast should still be attempted")
	assert.Equal(t, int32(1), watchCount.Load(), "rejected replacement must not be registered")
}

// All attempts exhausted, receipt never found → error.
// Verifies compound escalation chain, nonce immutability, and gasFeeCap on every attempt.
func TestSendWithRetry_AllAttemptsExhausted(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var (
		feeHistoryCalls atomic.Int32
		broadcasts      []capturedBroadcast
		watchCount      atomic.Int32
	)

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
		monitormock.New(
			monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error, error) {
				watchCount.Add(1)
				return make(chan struct{}), make(chan error), nil
			}),
		),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	assert.ErrorIs(t, err, transaction.ErrAllAttemptsExhausted)
	assert.NotEqual(t, common.Hash{}, txHash, "should return last tx hash even on exhaustion")
	assert.Nil(t, receipt)

	require.Len(t, broadcasts, 3, "should have made exactly maxRetries attempts")
	assert.Equal(t, int32(1), watchCount.Load(), "nonce is watched once")

	assertTxDataUnchanged(t, broadcasts)

	tip0 := s.expectedMarketTip()              // tipBase*2 = 200
	tip1 := transaction.ApplyMempoolBump(tip0) // 230
	tip2 := transaction.ApplyMempoolBump(tip1) // 264
	expectedTips := []*big.Int{tip0, tip1, tip2}

	for i, expectedTip := range expectedTips {
		assert.Equal(t, expectedTip.Int64(), broadcasts[i].GasTipCap.Int64(),
			"attempt %d: tip must match compound escalation chain", i)
		assert.Equal(t, s.expectedGasFeeCap(expectedTip).Int64(), broadcasts[i].GasFeeCap.Int64(),
			"attempt %d: gasFeeCap must be baseFee*2 + tip", i)
	}

	assert.Equal(t, int32(3), feeHistoryCalls.Load(),
		"fee history is called for each attempt")

	lastTxHash := broadcasts[len(broadcasts)-1].Hash
	assert.Equal(t, lastTxHash, txHash, "last tx hash must match the final broadcast")

	var retryState transaction.RetriedTransaction
	assert.NoError(t, store.Get(transaction.RetryStateKey(broadcasts[0].Nonce), &retryState),
		"retry state should remain while the session is monitored")

	for _, broadcast := range broadcasts {
		var pending struct{}
		assert.NoError(t, store.Get(transaction.PendingTransactionKey(broadcast.Hash), &pending),
			"pending tx should remain until the retry session has a result")
		var stored transaction.StoredTransaction
		assert.NoError(t, store.Get(transaction.StoredTransactionKey(broadcast.Hash), &stored),
			"stored tx should remain until the retry session has a result")
	}
}

func TestSendWithRetry_ExhaustedPreviousHashConfirmed(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var (
		broadcasts []capturedBroadcast
		doneC      = make(chan struct{}, 1)
	)

	cfg := s.retryConfig()
	cfg.AttemptsPerTier = 2

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
			backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
				if len(broadcasts) >= 2 && txHash == broadcasts[0].Hash {
					return &types.Receipt{TxHash: txHash, Status: 1}, nil
				}
				return nil, ethereum.NotFound
			}),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(
			monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error, error) {
				return doneC, make(chan error), nil
			}),
		),
		0,
		cfg,
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	_, _, err = svc.SendWithRetry(context.Background(), s.request())
	require.ErrorIs(t, err, transaction.ErrAllAttemptsExhausted)
	require.Len(t, broadcasts, 2)

	firstHash := broadcasts[0].Hash
	lastHash := broadcasts[1].Hash
	doneC <- struct{}{}

	require.Eventually(t, func() bool {
		var state transaction.RetriedTransaction
		return errors.Is(store.Get(transaction.RetryStateKey(s.nonce), &state), storage.ErrNotFound)
	}, time.Second, 10*time.Millisecond)

	var stored transaction.StoredTransaction
	assert.NoError(t, store.Get(transaction.StoredTransactionKey(firstHash), &stored),
		"confirmed stored tx should be kept")
	assert.ErrorIs(t, store.Get(transaction.StoredTransactionKey(lastHash), &stored), storage.ErrNotFound,
		"unconfirmed replacement should be removed")
}

// "nonce too low" on a rebroadcast means the nonce was consumed. Receipts are
// looked up newest-first without waiting for another watch registration.
func TestSendWithRetry_NonceTooLow(t *testing.T) {
	t.Parallel()

	t.Run("receipt found stops sendWithRetry and returns it", func(t *testing.T) {
		s := newRetryTestSetup()
		store := storemock.NewStateStore()
		testutil.CleanupCloser(t, store)

		var (
			firstTxHash    common.Hash
			broadcastCount atomic.Int32
		)

		svc, err := transaction.NewService(log.Noop, s.sender,
			backendmock.New(
				s.nonceOption(),
				s.feeHistoryOption(nil),
				s.headerOption(),
				s.estimateGasOption(),
				backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
					if broadcastCount.Add(1) == 1 {
						firstTxHash = tx.Hash()
						return nil
					}
					return errors.New("nonce too low")
				}),
				backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
					if txHash == firstTxHash {
						return &types.Receipt{TxHash: txHash, Status: 1}, nil
					}
					return nil, ethereum.NotFound
				}),
			),
			signermock.New(s.passThroughSigner(), s.signerAddr()),
			store,
			s.chainID,
			monitormock.New(nonceWatchIdle()),
			0,
			s.retryConfig(),
		)
		require.NoError(t, err)
		testutil.CleanupCloser(t, svc)

		txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

		require.NoError(t, err)
		require.NotNil(t, receipt)
		assert.Equal(t, firstTxHash, txHash, "must return the mined tx hash")
		assert.Equal(t, uint64(1), receipt.Status)
		assert.Equal(t, int32(2), broadcastCount.Load(), "exactly one rebroadcast, no further retries after nonce too low")

		var v string
		assert.ErrorIs(t, store.Get(transaction.RetryStateKey(s.nonce), &v), storage.ErrNotFound,
			"retry state should be cleaned up after success")
	})

	t.Run("receipt not found returns nonce too low", func(t *testing.T) {
		s := newRetryTestSetup()
		store := storemock.NewStateStore()
		testutil.CleanupCloser(t, store)

		var (
			firstTxHash    common.Hash
			broadcastCount atomic.Int32
		)

		svc, err := transaction.NewService(log.Noop, s.sender,
			backendmock.New(
				s.nonceOption(),
				s.feeHistoryOption(nil),
				s.headerOption(),
				s.estimateGasOption(),
				backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
					if broadcastCount.Add(1) == 1 {
						firstTxHash = tx.Hash()
						return nil
					}
					return errors.New("nonce too low")
				}),
				backendmock.WithTransactionReceiptFunc(func(context.Context, common.Hash) (*types.Receipt, error) {
					return nil, ethereum.NotFound
				}),
			),
			signermock.New(s.passThroughSigner(), s.signerAddr()),
			store,
			s.chainID,
			monitormock.New(nonceWatchIdle()),
			0,
			s.retryConfig(),
		)
		require.NoError(t, err)
		testutil.CleanupCloser(t, svc)

		txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nonce too low")
		assert.Equal(t, common.Hash{}, txHash)
		assert.Nil(t, receipt)
		assert.Equal(t, int32(2), broadcastCount.Load(), "exactly one rebroadcast, no further retries after nonce too low")
		assert.NotEqual(t, common.Hash{}, firstTxHash)

		var retryState transaction.RetriedTransaction
		assert.NoError(t, store.Get(transaction.RetryStateKey(s.nonce), &retryState),
			"retry state should remain while the session is monitored")
	})
}

// Resume after node restart — transaction is re-sent starting from persisted attempt.
// Verifies nonce, escalated tip, gasFeeCap, and that fee history is NOT called.
func TestSendWithRetry_ResumeAfterRestart(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	previousTip := new(big.Int).Set(s.tipBase)
	firstTxHash := common.HexToHash("0xaaaa")
	secondTxHash := common.HexToHash("0xbbbb")
	lastTxHash := common.HexToHash("0xdeadbeef")

	retryKey := transaction.RetryStateKey(s.nonce)
	require.NoError(t, store.Put(retryKey, transaction.RetriedTransaction{
		Nonce:         s.nonce,
		NonceAssigned: true,
		CurrentHash:   lastTxHash,
		PrevHashes:    []common.Hash{firstTxHash, secondTxHash},
	}))

	for _, hash := range []common.Hash{firstTxHash, secondTxHash, lastTxHash} {
		require.NoError(t, store.Put(transaction.StoredTransactionKey(hash), transaction.StoredTransaction{
			To:          &s.recipient,
			Data:        s.txData,
			GasLimit:    s.gasLimit,
			Value:       s.value,
			Nonce:       s.nonce,
			GasTipCap:   previousTip,
			GasFeeCap:   big.NewInt(5000),
			GasPrice:    big.NewInt(0),
			Created:     time.Now().Unix(),
			Description: "test-resume",
		}))
		require.NoError(t, store.Put(transaction.PendingTransactionKey(hash), struct{}{}))
	}

	var (
		broadcastsMu sync.Mutex
		broadcasts   []capturedBroadcast
		watchedNonce atomic.Uint64
	)
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
				captured := captureTx(tx)
				broadcastsMu.Lock()
				broadcasts = append(broadcasts, captured)
				broadcastsMu.Unlock()
				return nil
			}),
			backendmock.WithTransactionByHashFunc(func(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error) {
				return nil, false, ethereum.NotFound
			}),
			receiptFound(),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(
			monitormock.WithWatchNonceFunc(func(nonce uint64) (<-chan struct{}, <-chan error, error) {
				watchedNonce.Store(nonce)
				doneC := make(chan struct{}, 1)
				doneC <- struct{}{}
				return doneC, make(chan error), nil
			}),
		),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	require.Eventually(t, func() bool {
		broadcastsMu.Lock()
		defer broadcastsMu.Unlock()
		return len(broadcasts) > 0
	}, 5*time.Second, 10*time.Millisecond, "resume should have triggered a broadcast")

	assert.Equal(t, s.nonce, watchedNonce.Load())

	require.NoError(t, svc.Close())

	broadcastsMu.Lock()
	require.Len(t, broadcasts, 1)
	resumed := broadcasts[0]
	gasTipCap := new(big.Int).Set(resumed.GasTipCap)
	gasFeeCap := new(big.Int).Set(resumed.GasFeeCap)
	resumedNonce := resumed.Nonce
	broadcastsMu.Unlock()

	assert.Equal(t, s.nonce, resumedNonce, "resumed transaction must use the same nonce")

	expectedTip := s.expectedMarketTip()
	assert.Equal(t, expectedTip.Int64(), gasTipCap.Int64(),
		"resumed transaction should use market tip from fresh fee history")

	assert.Equal(t, s.expectedGasFeeCap(expectedTip).Int64(), gasFeeCap.Int64(),
		"resumed gasFeeCap must be baseFee*2 + escalated tip")

	assert.Equal(t, int32(1), feeHistoryCalls.Load(),
		"fee history should be called on resume")

	var v string
	assert.Eventually(t, func() bool {
		return errors.Is(store.Get(retryKey, &v), storage.ErrNotFound)
	}, 5*time.Second, 10*time.Millisecond, "retry state should be cleaned up after success")
}

// MaxTxPrice cap prevents escalation beyond the configured limit.
func TestSendWithRetry_MaxTxPriceCap(t *testing.T) {
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
		monitormock.New(nonceWatchIdle()),
		0,
		cfg,
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	_, _, err = svc.SendWithRetry(context.Background(), s.request())
	assert.Error(t, err)
	assert.Len(t, broadcasts, 0,
		"no transaction should be sent when maxTxPrice is below the minimum fee")
}

func TestSendWithRetry_FeePriorityContextOverride(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var broadcasts []capturedBroadcast

	cfg := s.retryConfig()
	cfg.StartTier = "market"
	cfg.EndTier = "aggressive"
	cfg.AttemptsPerTier = 1

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
			receiptFound(),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(nonceWatchSignal()),
		0,
		cfg,
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	ctx := sctx.SetFeePriority(context.Background(), "low")
	_, receipt, err := svc.SendWithRetry(ctx, s.request())
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Len(t, broadcasts, 1)
	assert.Equal(t, s.tipBase.Int64(), broadcasts[0].GasTipCap.Int64(),
		"request fee priority must override node start tier")
}

func TestSendWithRetry_FeePriorityClampedToNodeMax(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	var broadcasts []capturedBroadcast

	cfg := s.retryConfig()
	cfg.StartTier = "low"
	cfg.EndTier = "market"
	cfg.AttemptsPerTier = 1

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
			receiptFound(),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(nonceWatchSignal()),
		0,
		cfg,
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)

	ctx := sctx.SetFeePriority(context.Background(), "aggressive")
	_, receipt, err := svc.SendWithRetry(ctx, s.request())
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Len(t, broadcasts, 1)
	assert.Equal(t, s.expectedMarketTip().Int64(), broadcasts[0].GasTipCap.Int64(),
		"request fee priority above node ceiling must clamp to end tier")
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
