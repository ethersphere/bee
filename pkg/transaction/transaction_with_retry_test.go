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

// Fee values below follow from retryTestSetup: base fee 1000, tips low=100, market=200,
// aggressive=300. The first market fee cap is 2*1000+200 = 2200, and every replacement
// must clear 15% above the previous tip and fee cap.

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

// broadcastLog collects broadcasts; resumed sessions send from service goroutines.
type broadcastLog struct {
	mu  sync.Mutex
	txs []capturedBroadcast
}

func (l *broadcastLog) add(tx *types.Transaction) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.txs = append(l.txs, captureTx(tx))
	return len(l.txs)
}

func (l *broadcastLog) all() []capturedBroadcast {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]capturedBroadcast(nil), l.txs...)
}

func (l *broadcastLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.txs)
}

func (l *broadcastLog) hash(i int) common.Hash {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i >= len(l.txs) {
		return common.Hash{}
	}
	return l.txs[i].Hash
}

func (l *broadcastLog) hashes() []common.Hash {
	return hashesOf(l.all())
}

func (l *broadcastLog) sendOK() backendmock.Option {
	return backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
		l.add(tx)
		return nil
	})
}

func hashesOf(broadcasts []capturedBroadcast) []common.Hash {
	out := make([]common.Hash, 0, len(broadcasts))
	for _, b := range broadcasts {
		out = append(out, b.Hash)
	}
	return out
}

func feeCaps(broadcasts []capturedBroadcast) []int64 {
	out := make([]int64, 0, len(broadcasts))
	for _, b := range broadcasts {
		out = append(out, b.GasFeeCap.Int64())
	}
	return out
}

func tipCaps(broadcasts []capturedBroadcast) []int64 {
	out := make([]int64, 0, len(broadcasts))
	for _, b := range broadcasts {
		out = append(out, b.GasTipCap.Int64())
	}
	return out
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

func (s retryTestSetup) retryConfig() transaction.TransactionsRetryConfig {
	return transaction.TransactionsRetryConfig{
		AttemptsPerTier: 3,
		StartTier:       "market",
		EndTier:         "market",
		RetryDelay:      50 * time.Millisecond,
		MaxTxPrice:      big.NewInt(100_000_000),

		ReceiptPollInterval: 5 * time.Millisecond,
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

// nonceOption models an RPC pending nonce that does not see this node's unmined transactions.
func (s retryTestSetup) nonceOption() backendmock.Option {
	return backendmock.WithPendingNonceAtFunc(func(context.Context, common.Address) (uint64, error) {
		return s.nonce, nil
	})
}

func (s retryTestSetup) estimateGasOption() backendmock.Option {
	return backendmock.WithEstimateGasFunc(func(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
		return s.gasLimit, nil
	})
}

// newService builds a retry service with a stable pending nonce, fee history, header and gas
// estimate. Options in opts replace those defaults.
func (s retryTestSetup) newService(t *testing.T, store storage.StateStorer, cfg transaction.TransactionsRetryConfig, watch monitormock.Option, opts ...backendmock.Option) transaction.Service {
	t.Helper()
	backendOpts := append([]backendmock.Option{
		s.nonceOption(),
		s.feeHistoryOption(nil),
		s.headerOption(),
		s.estimateGasOption(),
	}, opts...)
	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(backendOpts...),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(watch),
		0,
		cfg,
	)
	require.NoError(t, err)
	testutil.CleanupCloser(t, svc)
	return svc
}

// resumesAfterRestart starts another service on store and reports whether it picks up a
// retry session. The probe cannot broadcast, so it leaves the store as it found it.
func (s retryTestSetup) resumesAfterRestart(t *testing.T, store storage.StateStorer) bool {
	t.Helper()
	var watched atomic.Bool
	svc, err := transaction.NewService(log.Noop, s.sender,
		backendmock.New(
			confirmedNonce(s.nonce),
			backendmock.WithTransactionByHashFunc(func(context.Context, common.Hash) (*types.Transaction, bool, error) {
				return nil, false, errors.New("probe")
			}),
		),
		signermock.New(s.passThroughSigner(), s.signerAddr()),
		store,
		s.chainID,
		monitormock.New(monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error) {
			watched.Store(true)
			return make(chan struct{}), make(chan error)
		})),
		0,
		s.retryConfig(),
	)
	require.NoError(t, err)
	require.NoError(t, svc.Close())
	return watched.Load()
}

// seedRetrySession runs a session that broadcasts n times without a receipt and stops the
// node, leaving the session on store for the next service to resume.
func (s retryTestSetup) seedRetrySession(t *testing.T, store storage.StateStorer, n int) []capturedBroadcast {
	t.Helper()
	cfg := s.retryConfig()
	cfg.AttemptsPerTier = n
	var txs broadcastLog
	svc := s.newService(t, store, cfg, nonceWatchIdle(), txs.sendOK(), receiptNotFound())
	_, _, err := svc.SendWithRetry(context.Background(), s.request())
	require.ErrorIs(t, err, transaction.ErrAllAttemptsExhausted)
	require.NoError(t, svc.Close())
	return txs.all()
}

// assertSessionCleared checks that no hash stays pending, only keep stays stored (none for
// a zero hash), and a restarted node does not resume the session.
func (s retryTestSetup) assertSessionCleared(t *testing.T, svc transaction.Service, store storage.StateStorer, keep common.Hash, hashes ...common.Hash) {
	t.Helper()
	pending := pendingSet(t, svc)
	for _, h := range hashes {
		assert.False(t, pending[h], "tx %s must not stay pending", h)
		assert.Equal(t, h == keep, isStored(t, svc, h), "tx %s stored", h)
	}
	assert.False(t, s.resumesAfterRestart(t, store), "a finished session must not resume after restart")
}

// assertSessionKept checks that every hash stays pending and stored and a restarted node
// resumes the session.
func (s retryTestSetup) assertSessionKept(t *testing.T, svc transaction.Service, store storage.StateStorer, hashes ...common.Hash) {
	t.Helper()
	pending := pendingSet(t, svc)
	for _, h := range hashes {
		assert.True(t, pending[h], "tx %s must stay pending", h)
		assert.True(t, isStored(t, svc, h), "tx %s must stay stored", h)
	}
	assert.True(t, s.resumesAfterRestart(t, store), "an unfinished session must resume after restart")
}

func newRetryStore(t *testing.T) storage.StateStorer {
	t.Helper()
	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)
	return store
}

func pendingSet(t *testing.T, svc transaction.Service) map[common.Hash]bool {
	t.Helper()
	hashes, err := svc.PendingTransactions()
	require.NoError(t, err)
	out := make(map[common.Hash]bool, len(hashes))
	for _, h := range hashes {
		out[h] = true
	}
	return out
}

func noPending(svc transaction.Service) func() bool {
	return func() bool {
		hashes, err := svc.PendingTransactions()
		return err == nil && len(hashes) == 0
	}
}

func isStored(t *testing.T, svc transaction.Service, hash common.Hash) bool {
	t.Helper()
	_, err := svc.StoredTransaction(hash)
	if errors.Is(err, transaction.ErrUnknownTransaction) {
		return false
	}
	require.NoError(t, err)
	return true
}

func confirmedNonce(nonce uint64) backendmock.Option {
	return backendmock.WithNonceAtFunc(func(context.Context, common.Address, *big.Int) (uint64, error) {
		return nonce, nil
	})
}

func chainHead(block uint64) backendmock.Option {
	return backendmock.WithBlockNumberFunc(func(context.Context) (uint64, error) {
		return block, nil
	})
}

func nonceWatchIdle() monitormock.Option {
	return monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error) {
		return make(chan struct{}), make(chan error)
	})
}

func nonceWatchSignal() monitormock.Option {
	return monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error) {
		minedC := make(chan struct{})
		close(minedC)
		return minedC, make(chan error)
	})
}

func nonceWatchOn(minedC <-chan struct{}) monitormock.Option {
	return monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error) {
		return minedC, make(chan error)
	})
}

func nonceWatchErr(err error) monitormock.Option {
	return monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error) {
		errC := make(chan error, 1)
		errC <- err
		return make(chan struct{}), errC
	})
}

func receiptFound() backendmock.Option {
	return backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
		return &types.Receipt{TxHash: txHash, Status: types.ReceiptStatusSuccessful}, nil
	})
}

func receiptNotFound() backendmock.Option {
	return backendmock.WithTransactionReceiptFunc(func(context.Context, common.Hash) (*types.Receipt, error) {
		return nil, ethereum.NotFound
	})
}

// A non-retryable broadcast error stops the session before anything is persisted or watched.
func TestSendWithRetry_BroadcastCriticalError(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	var (
		txs        broadcastLog
		watchCount atomic.Int32
	)
	svc := s.newService(t, store, s.retryConfig(),
		monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error) {
			watchCount.Add(1)
			return make(chan struct{}), make(chan error)
		}),
		backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
			txs.add(tx)
			return errors.New("execution reverted")
		}),
	)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.ErrorContains(t, err, "execution reverted")
	assert.Equal(t, common.Hash{}, txHash)
	assert.Nil(t, receipt)
	require.Equal(t, 1, txs.count(), "a non-retryable error is not retried")
	assert.Zero(t, watchCount.Load(), "a rejected transaction is not watched")
	s.assertSessionCleared(t, svc, store, common.Hash{}, txs.hash(0))
}

// A failed retry state write stops the loop, but the broadcast tx is still watched in the background.
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
	minedC := make(chan struct{})
	var (
		txs        broadcastLog
		watchCount atomic.Int32
	)

	svc := s.newService(t, failingStore, s.retryConfig(),
		monitormock.WithWatchNonceFunc(func(nonce uint64) (<-chan struct{}, <-chan error) {
			watchCount.Add(1)
			assert.Equal(t, s.nonce, nonce)
			return minedC, make(chan error)
		}),
		txs.sendOK(),
		receiptFound(),
	)

	_, _, err := svc.SendWithRetry(context.Background(), s.request())
	assert.ErrorIs(t, err, putErr)
	assert.Equal(t, int32(1), watchCount.Load(), "broadcast transaction must remain monitored")

	close(minedC)
	require.Eventually(t, noPending(svc), time.Second, 10*time.Millisecond)
	require.NoError(t, svc.Close())
	assert.True(t, isStored(t, svc, txs.hash(0)))
}

// A broadcast that fails before SendTransaction leaves nothing to watch: the loop waits the
// retry delay, and the next attempt takes the same nonce.
func TestSendWithRetry_NonCriticalThenSuccess(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	var (
		headerCalls atomic.Int32
		txs         broadcastLog
	)
	svc := s.newService(t, store, s.retryConfig(), nonceWatchSignal(),
		backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
			if headerCalls.Add(1) == 1 {
				return nil, errors.New("temporary RPC error")
			}
			return &types.Header{BaseFee: new(big.Int).Set(s.baseFee)}, nil
		}),
		txs.sendOK(),
		receiptFound(),
	)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Equal(t, int32(2), headerCalls.Load(), "the failed attempt is retried")
	broadcasts := txs.all()
	require.Len(t, broadcasts, 1, "the first attempt failed before SendTransaction")
	assert.Equal(t, s.nonce, broadcasts[0].Nonce)
	assert.Equal(t, broadcasts[0].Hash, txHash)
	s.assertSessionCleared(t, svc, store, txHash, txHash)
}

// An RPC deadline is not a caller cancellation: the next attempt still runs.
func TestSendWithRetry_RPCTimeoutThenSuccess(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	var txs broadcastLog
	minedC := make(chan struct{})

	svc := s.newService(t, store, s.retryConfig(), nonceWatchOn(minedC),
		backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
			if txs.add(tx) == 1 {
				return context.DeadlineExceeded
			}
			close(minedC)
			return nil
		}),
		receiptFound(),
	)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Equal(t, txs.hash(1), txHash)
	require.Equal(t, 2, txs.count(), "deadline on one RPC must not stop the retry loop")
}

// Superseded broadcasts stay on disk until the session has a result; then only the confirmed tx stays.
func TestSendWithRetry_SupersededTxKeptUntilResult(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	var (
		svc    transaction.Service
		txs    broadcastLog
		minedC = make(chan struct{})
	)
	svc = s.newService(t, store, s.retryConfig(), nonceWatchOn(minedC),
		backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
			if txs.add(tx) == 2 {
				first := txs.hash(0)
				assert.True(t, isStored(t, svc, first), "superseded stored tx must remain until a result is known")
				assert.True(t, pendingSet(t, svc)[first], "superseded pending tx must remain until a result is known")
				close(minedC)
			}
			return nil
		}),
		receiptFound(),
	)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, 2, txs.count())
	assert.Equal(t, txs.hash(1), txHash)
	s.assertSessionCleared(t, svc, store, txHash, txs.hashes()...)
}

// The receipt can belong to any broadcast of the session, not only to the latest one.
func TestSendWithRetry_ConfirmedHashFromSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		broadcasts int
		confirmed  int
	}{
		{name: "first of two broadcasts", broadcasts: 2, confirmed: 0},
		{name: "middle of three broadcasts", broadcasts: 3, confirmed: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newRetryTestSetup()
			store := newRetryStore(t)

			var (
				txs        broadcastLog
				watchCount atomic.Int32
				minedC     = make(chan struct{})
			)
			svc := s.newService(t, store, s.retryConfig(),
				monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error) {
					watchCount.Add(1)
					return minedC, make(chan error)
				}),
				backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
					if txs.add(tx) == tc.broadcasts {
						close(minedC)
					}
					return nil
				}),
				backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
					if txs.count() == tc.broadcasts && txHash == txs.hash(tc.confirmed) {
						return &types.Receipt{TxHash: txHash, Status: types.ReceiptStatusSuccessful}, nil
					}
					return nil, ethereum.NotFound
				}),
			)

			txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

			require.NoError(t, err)
			require.NotNil(t, receipt)
			require.Equal(t, tc.broadcasts, txs.count())
			confirmed := txs.hash(tc.confirmed)
			assert.Equal(t, confirmed, txHash)
			assert.Equal(t, confirmed, receipt.TxHash)
			assert.Equal(t, int32(1), watchCount.Load(), "nonce is watched once per session")
			s.assertSessionCleared(t, svc, store, confirmed, txs.hashes()...)
		})
	}
}

// A receipt already cleared the session. A nonce that moved earlier must not start another wait:
// that wait would look the receipt up again and delete the stored transaction if the lookup misses.
func TestSendWithRetry_ReceiptSkipsNonceWatch(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	minedC := make(chan struct{})
	close(minedC)

	var (
		txs      broadcastLog
		receipts atomic.Int32
	)
	svc := s.newService(t, store, s.retryConfig(), nonceWatchOn(minedC),
		txs.sendOK(),
		backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
			if receipts.Add(1) == 1 && txHash == txs.hash(0) {
				return &types.Receipt{TxHash: txHash, Status: types.ReceiptStatusSuccessful}, nil
			}
			return nil, ethereum.NotFound
		}),
	)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())
	require.NoError(t, svc.Close())

	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Equal(t, txs.hash(0), txHash)
	assert.Equal(t, int32(1), receipts.Load(), "receipt is read once")
	s.assertSessionCleared(t, svc, store, txHash, txHash)
}

func TestSendWithRetry_ExternalNonceCancellation(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	var txs broadcastLog
	minedC := make(chan struct{})
	svc := s.newService(t, store, s.retryConfig(), nonceWatchOn(minedC),
		backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
			txs.add(tx)
			close(minedC)
			return nil
		}),
		receiptNotFound(),
		chainHead(100),
		confirmedNonce(s.nonce+1),
	)

	_, _, err := svc.SendWithRetry(context.Background(), s.request())

	require.ErrorIs(t, err, transaction.ErrTransactionCancelled)
	s.assertSessionCleared(t, svc, store, common.Hash{}, txs.hashes()...)
}

// The RPC node that serves receipts lags behind the one that reported the mined nonce.
// The receipt is looked up again instead of cancelling a mined transaction.
func TestSendWithRetry_LaggingReceiptIsNotCancelled(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	cfg := s.retryConfig()
	cfg.CancellationDepth = 12

	var (
		txs      broadcastLog
		lookups  atomic.Int32
		minedC   = make(chan struct{})
		lagCalls = int32(3)
	)
	svc := s.newService(t, store, cfg, nonceWatchOn(minedC),
		backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
			txs.add(tx)
			close(minedC)
			return nil
		}),
		backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
			if lookups.Add(1) <= lagCalls {
				return nil, ethereum.NotFound
			}
			return &types.Receipt{TxHash: txHash, Status: types.ReceiptStatusSuccessful}, nil
		}),
		chainHead(100), // the head does not advance, so cancellationDepth is never reached
		confirmedNonce(s.nonce),
	)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, 1, txs.count())
	assert.Equal(t, txs.hash(0), txHash)
	assert.Equal(t, lagCalls+1, lookups.Load())
	s.assertSessionCleared(t, svc, store, txHash, txHash)
}

// A missing receipt cancels the session only cancellationDepth blocks after the nonce was seen mined.
func TestSendWithRetry_CancelledAfterCancellationDepth(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	const (
		depth     = 12
		startHead = 100
	)
	cfg := s.retryConfig()
	cfg.CancellationDepth = depth

	var (
		txs    broadcastLog
		head   atomic.Uint64
		minedC = make(chan struct{})
	)
	head.Store(startHead)
	svc := s.newService(t, store, cfg, nonceWatchOn(minedC),
		backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
			txs.add(tx)
			close(minedC)
			return nil
		}),
		receiptNotFound(),
		backendmock.WithBlockNumberFunc(func(context.Context) (uint64, error) {
			return head.Add(1), nil
		}),
		confirmedNonce(s.nonce),
	)

	_, _, err := svc.SendWithRetry(context.Background(), s.request())

	require.ErrorIs(t, err, transaction.ErrTransactionCancelled)
	assert.EqualValues(t, startHead+1+depth, head.Load(), "cancelled at the first head cancellationDepth blocks after the nonce was seen mined")
	s.assertSessionCleared(t, svc, store, common.Hash{}, txs.hashes()...)
}

// A failing receipt lookup is not a missing receipt: a mined session found on start stays on
// disk even when its nonce is deep enough to be cancelled.
func TestSendWithRetry_ReceiptLookupErrorKeepsSession(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	hashes := hashesOf(s.seedRetrySession(t, store, 3))

	var lookups atomic.Int32
	svc := s.newService(t, store, s.retryConfig(), nonceWatchIdle(),
		confirmedNonce(s.nonce+1),
		chainHead(100),
		backendmock.WithTransactionReceiptFunc(func(context.Context, common.Hash) (*types.Receipt, error) {
			lookups.Add(1)
			return nil, errors.New("rpc unavailable")
		}),
	)
	// Two full lookup rounds: the first failure did not end the session.
	require.Eventually(t, func() bool { return lookups.Load() >= int32(2*len(hashes)) }, time.Second, 5*time.Millisecond)
	require.NoError(t, svc.Close())

	s.assertSessionKept(t, svc, store, hashes...)
}

// A reverted receipt ends the session: the reverted tx stays stored, the rest is dropped.
func TestSendWithRetry_Reverted(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	var txs broadcastLog
	minedC := make(chan struct{})
	svc := s.newService(t, store, s.retryConfig(), nonceWatchOn(minedC),
		backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
			if txs.add(tx) == 2 {
				close(minedC)
			}
			return nil
		}),
		backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
			if txHash == txs.hash(1) {
				return &types.Receipt{TxHash: txHash, Status: types.ReceiptStatusFailed}, nil
			}
			return nil, ethereum.NotFound
		}),
	)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.ErrorIs(t, err, transaction.ErrTransactionReverted)
	require.NotNil(t, receipt)
	assert.Equal(t, types.ReceiptStatusFailed, receipt.Status)
	assert.Equal(t, txs.hash(1), txHash)
	s.assertSessionCleared(t, svc, store, txHash, txs.hashes()...)
}

// The caller gives up while the nonce is pending. The tx is already in the mempool, so the
// session stays on disk and the background watch finishes it.
func TestSendWithRetry_CallerCancelKeepsSession(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := s.retryConfig()
	cfg.RetryDelay = time.Minute

	var txs broadcastLog
	minedC := make(chan struct{})
	svc := s.newService(t, store, cfg,
		monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error) {
			cancel()
			return minedC, make(chan error)
		}),
		txs.sendOK(),
		receiptFound(),
	)

	_, receipt, err := svc.SendWithRetry(ctx, s.request())

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, receipt)
	require.Equal(t, 1, txs.count())
	s.assertSessionKept(t, svc, store, txs.hash(0))

	close(minedC)
	require.Eventually(t, noPending(svc), time.Second, 10*time.Millisecond)
	require.NoError(t, svc.Close())
	s.assertSessionCleared(t, svc, store, txs.hash(0), txs.hash(0))
}

// After receipt timeout at market tier, fees escalate to the aggressive tier on the next broadcast.
func TestSendWithRetry_TierEscalation(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	cfg := s.retryConfig()
	cfg.AttemptsPerTier = 1
	cfg.StartTier = "market"
	cfg.EndTier = "aggressive"

	var txs broadcastLog
	minedC := make(chan struct{})
	svc := s.newService(t, store, cfg, nonceWatchOn(minedC),
		backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
			if txs.add(tx) == 2 {
				close(minedC)
			}
			return nil
		}),
		receiptFound(),
	)

	_, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	require.NotNil(t, receipt)
	broadcasts := txs.all()
	require.Len(t, broadcasts, 2, "market then aggressive tier, one attempt each")
	assert.Equal(t, []int64{200, 300}, tipCaps(broadcasts), "market tip, then aggressive tip")
	assert.Equal(t, []int64{2200, 2530}, feeCaps(broadcasts),
		"aggressive fee cap 2300 is below the floor 2200+15%")
}

// The base fee rose between attempts: the natural fee cap clears the 15% floor and is used as is.
func TestSendWithRetry_FeeCapFollowsBaseFeeRise(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	var (
		headerCalls atomic.Int32
		txs         broadcastLog
		minedC      = make(chan struct{})
	)
	svc := s.newService(t, store, s.retryConfig(), nonceWatchOn(minedC),
		backendmock.WithHeaderbyNumberFunc(func(context.Context, *big.Int) (*types.Header, error) {
			if headerCalls.Add(1) == 1 {
				return &types.Header{BaseFee: big.NewInt(1000)}, nil
			}
			return &types.Header{BaseFee: big.NewInt(2000)}, nil
		}),
		backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
			if txs.add(tx) == 2 {
				close(minedC)
			}
			return nil
		}),
		receiptFound(),
	)

	_, _, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	broadcasts := txs.all()
	require.Len(t, broadcasts, 2)
	assert.Equal(t, []int64{200, 230}, tipCaps(broadcasts))
	assert.Equal(t, []int64{2200, 4230}, feeCaps(broadcasts), "2*2000+230 is above the floor 2530")
}

// Underpriced replacement keeps watching the pending tx hash instead of switching to the rejected one.
func TestSendWithRetry_UnderpricedKeepsPendingTxHash(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	var (
		svc             transaction.Service
		txs             broadcastLog
		rejectedPending bool
		rejectedStored  bool
		minedC          = make(chan struct{})
	)
	svc = s.newService(t, store, s.retryConfig(), nonceWatchOn(minedC),
		backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
			if txs.add(tx) == 1 {
				return nil
			}
			close(minedC)
			return errors.New("replacement transaction underpriced")
		}),
		backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
			rejected := txs.hash(1)
			rejectedPending = pendingSet(t, svc)[rejected]
			rejectedStored = isStored(t, svc, rejected)
			return &types.Receipt{TxHash: txHash, Status: types.ReceiptStatusSuccessful}, nil
		}),
	)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, 2, txs.count(), "second broadcast should still be attempted")
	assert.Equal(t, txs.hash(0), txHash, "the receipt belongs to the tx the mempool kept")
	assert.False(t, rejectedPending, "a rejected replacement must not be tracked as pending")
	assert.False(t, rejectedStored, "a rejected replacement must not be stored")
	s.assertSessionCleared(t, svc, store, txHash, txs.hashes()...)
}

// An underpriced replacement still raises the next fee-cap floor. The mempool
// refused that cap, so the following attempt must clear 15% above it.
func TestSendWithRetry_FeeCapFloorFollowsLastAttempt(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	var txs broadcastLog
	minedC := make(chan struct{})
	svc := s.newService(t, store, s.retryConfig(), nonceWatchOn(minedC),
		backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
			switch txs.add(tx) {
			case 2:
				return errors.New("replacement transaction underpriced")
			case 3:
				close(minedC)
			}
			return nil
		}),
		receiptFound(),
	)

	_, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	require.NoError(t, err)
	require.NotNil(t, receipt)
	broadcasts := txs.all()
	require.Len(t, broadcasts, 3)
	assert.Equal(t, []int64{200, 230, 264}, tipCaps(broadcasts))
	assert.Equal(t, []int64{2200, 2530, 2909}, feeCaps(broadcasts),
		"underpriced replacement must raise the fee-cap floor")
}

// All attempts exhausted without a receipt: the escalation chain is compound, and the
// session stays for the background watch and a resume after restart.
func TestSendWithRetry_AllAttemptsExhausted(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	var (
		feeHistoryCalls atomic.Int32
		txs             broadcastLog
		watchCount      atomic.Int32
	)
	svc := s.newService(t, store, s.retryConfig(),
		monitormock.WithWatchNonceFunc(func(uint64) (<-chan struct{}, <-chan error) {
			watchCount.Add(1)
			return make(chan struct{}), make(chan error)
		}),
		s.feeHistoryOption(&feeHistoryCalls),
		txs.sendOK(),
		receiptNotFound(),
	)

	txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

	assert.ErrorIs(t, err, transaction.ErrAllAttemptsExhausted)
	assert.Nil(t, receipt)

	broadcasts := txs.all()
	require.Len(t, broadcasts, 3, "should have made exactly maxRetries attempts")
	assert.Equal(t, broadcasts[2].Hash, txHash, "last tx hash must match the final broadcast")
	assert.Equal(t, int32(1), watchCount.Load(), "nonce is watched once")
	assert.Equal(t, int32(3), feeHistoryCalls.Load(), "fee history is queried for each attempt")
	assertTxDataUnchanged(t, broadcasts)
	assert.Equal(t, []int64{200, 230, 264}, tipCaps(broadcasts))
	assert.Equal(t, []int64{2200, 2530, 2909}, feeCaps(broadcasts))
	s.assertSessionKept(t, svc, store, hashesOf(broadcasts)...)
}

func TestSendWithRetry_ExhaustedPreviousHashConfirmed(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	cfg := s.retryConfig()
	cfg.AttemptsPerTier = 2

	var txs broadcastLog
	minedC := make(chan struct{})
	svc := s.newService(t, store, cfg, nonceWatchOn(minedC),
		txs.sendOK(),
		backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
			if txs.count() >= 2 && txHash == txs.hash(0) {
				return &types.Receipt{TxHash: txHash, Status: types.ReceiptStatusSuccessful}, nil
			}
			return nil, ethereum.NotFound
		}),
	)

	_, _, err := svc.SendWithRetry(context.Background(), s.request())
	require.ErrorIs(t, err, transaction.ErrAllAttemptsExhausted)
	require.Equal(t, 2, txs.count())

	close(minedC)
	require.Eventually(t, noPending(svc), time.Second, 10*time.Millisecond)
	require.NoError(t, svc.Close())
	s.assertSessionCleared(t, svc, store, txs.hash(0), txs.hashes()...)
}

// "nonce too low" on a rebroadcast means the nonce was consumed. Receipts are
// looked up newest-first without waiting for another watch registration.
func TestSendWithRetry_NonceTooLow(t *testing.T) {
	t.Parallel()

	t.Run("receipt found stops sendWithRetry and returns it", func(t *testing.T) {
		t.Parallel()
		s := newRetryTestSetup()
		store := newRetryStore(t)

		var txs broadcastLog
		svc := s.newService(t, store, s.retryConfig(), nonceWatchIdle(),
			backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
				if txs.add(tx) == 1 {
					return nil
				}
				return errors.New("nonce too low")
			}),
			backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
				if txHash == txs.hash(0) {
					return &types.Receipt{TxHash: txHash, Status: types.ReceiptStatusSuccessful}, nil
				}
				return nil, ethereum.NotFound
			}),
		)

		txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

		require.NoError(t, err)
		require.NotNil(t, receipt)
		assert.Equal(t, txs.hash(0), txHash, "must return the mined tx hash")
		assert.Equal(t, 2, txs.count(), "exactly one rebroadcast, no further retries after nonce too low")
		s.assertSessionCleared(t, svc, store, txHash, txs.hashes()...)
	})

	t.Run("receipt not found cancels the session", func(t *testing.T) {
		t.Parallel()
		s := newRetryTestSetup()
		store := newRetryStore(t)

		var txs broadcastLog
		svc := s.newService(t, store, s.retryConfig(), nonceWatchIdle(),
			backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
				if txs.add(tx) == 1 {
					return nil
				}
				return errors.New("nonce too low")
			}),
			receiptNotFound(),
			chainHead(100),
		)

		txHash, receipt, err := svc.SendWithRetry(context.Background(), s.request())

		require.ErrorIs(t, err, transaction.ErrTransactionCancelled)
		assert.Equal(t, common.Hash{}, txHash)
		assert.Nil(t, receipt)
		assert.Equal(t, 2, txs.count(), "exactly one rebroadcast, no further retries after nonce too low")
		s.assertSessionCleared(t, svc, store, common.Hash{}, txs.hashes()...)
	})
}

// ErrMonitorClosed is not a cancellation: the session must survive for the resume after restart.
func TestSendWithRetry_MonitorClosedKeepsSession(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	var txs broadcastLog
	svc := s.newService(t, store, s.retryConfig(), nonceWatchErr(transaction.ErrMonitorClosed),
		txs.sendOK(),
		receiptNotFound(),
	)

	_, _, err := svc.SendWithRetry(context.Background(), s.request())
	require.NoError(t, svc.Close())

	require.ErrorIs(t, err, transaction.ErrAllAttemptsExhausted)
	s.assertSessionKept(t, svc, store, txs.hashes()...)
}

// Resume after node restart floors the tip and the fee cap at 15% above the last broadcast of the session.
func TestSendWithRetry_ResumeAfterRestart(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	seeded := s.seedRetrySession(t, store, 3) // the last broadcast has tip 264 and fee cap 2909

	var txs broadcastLog
	svc := s.newService(t, store, s.retryConfig(), nonceWatchSignal(),
		confirmedNonce(s.nonce),
		txs.sendOK(),
		receiptFound(),
	)

	require.Eventually(t, noPending(svc), time.Second, 10*time.Millisecond)
	require.NoError(t, svc.Close())

	resumed := txs.all()
	require.Len(t, resumed, 1)
	assert.Equal(t, s.nonce, resumed[0].Nonce, "resumed transaction must use the same nonce")
	assert.Equal(t, int64(303), resumed[0].GasTipCap.Int64(), "264 + 15%")
	assert.Equal(t, int64(3345), resumed[0].GasFeeCap.Int64(), "2909 + 15%")
	s.assertSessionCleared(t, svc, store, resumed[0].Hash, append(hashesOf(seeded), resumed[0].Hash)...)
}

// The resumed broadcast fails before it is sent. The session tx is already in the network,
// so the nonce is watched before that attempt and the session finishes when the nonce is mined.
func TestSendWithRetry_ResumeWatchesNonceBeforeBroadcast(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	seeded := s.seedRetrySession(t, store, 3)
	confirmed := seeded[2].Hash

	var txs broadcastLog
	svc := s.newService(t, store, s.retryConfig(), nonceWatchSignal(),
		confirmedNonce(s.nonce),
		backendmock.WithHeaderbyNumberFunc(func(context.Context, *big.Int) (*types.Header, error) {
			return nil, errors.New("rpc unavailable")
		}),
		txs.sendOK(),
		backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
			if txHash == confirmed {
				return &types.Receipt{TxHash: txHash, Status: types.ReceiptStatusSuccessful}, nil
			}
			return nil, ethereum.NotFound
		}),
	)

	require.Eventually(t, noPending(svc), time.Second, 10*time.Millisecond)
	require.NoError(t, svc.Close())

	assert.Zero(t, txs.count())
	s.assertSessionCleared(t, svc, store, confirmed, hashesOf(seeded)...)
}

// Sessions that cannot or need not resume are cleaned up on start without a broadcast.
func TestSendWithRetry_ResumeCleanup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		nonceMined    bool
		receiptFor    int // index of the seeded broadcast with a receipt, -1 for none
		dropCurrentTx bool
	}{
		{name: "mined nonce keeps the tx with the receipt", nonceMined: true, receiptFor: 0},
		{name: "mined nonce without a receipt drops the session", nonceMined: true, receiptFor: -1},
		{name: "missing stored tx drops the session", receiptFor: -1, dropCurrentTx: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newRetryTestSetup()
			store := newRetryStore(t)

			hashes := hashesOf(s.seedRetrySession(t, store, 3))
			if tc.dropCurrentTx {
				require.NoError(t, store.Delete(transaction.StoredTransactionKey(hashes[2])))
			}
			confirmed := s.nonce
			if tc.nonceMined {
				confirmed++
			}
			var keep common.Hash
			if tc.receiptFor >= 0 {
				keep = hashes[tc.receiptFor]
			}

			var txs broadcastLog
			svc := s.newService(t, store, s.retryConfig(), nonceWatchIdle(),
				confirmedNonce(confirmed),
				chainHead(100),
				txs.sendOK(),
				backendmock.WithTransactionReceiptFunc(func(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
					if keep != (common.Hash{}) && txHash == keep {
						return &types.Receipt{TxHash: txHash, Status: types.ReceiptStatusSuccessful}, nil
					}
					return nil, ethereum.NotFound
				}),
			)
			require.Eventually(t, noPending(svc), time.Second, 10*time.Millisecond)
			require.NoError(t, svc.Close())

			assert.Zero(t, txs.count())
			s.assertSessionCleared(t, svc, store, keep, hashes...)
		})
	}
}

// Without the confirmed nonce the node cannot tell a finished session from a live one, so it resumes it.
func TestSendWithRetry_ResumeWithoutConfirmedNonce(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	s.seedRetrySession(t, store, 3)

	var txs broadcastLog
	svc := s.newService(t, store, s.retryConfig(), nonceWatchIdle(),
		backendmock.WithNonceAtFunc(func(context.Context, common.Address, *big.Int) (uint64, error) {
			return 0, errors.New("rpc unavailable")
		}),
		txs.sendOK(),
		receiptNotFound(),
	)

	require.Eventually(t, func() bool { return txs.count() > 0 }, time.Second, 10*time.Millisecond)
	require.NoError(t, svc.Close())
	assert.Equal(t, s.nonce, txs.all()[0].Nonce)
}

// Session hashes belong to the resumed session. The plain pending watch must not look them up:
// a "not found" there drops their pending keys and frees the nonce for a new transaction.
func TestSendWithRetry_PendingWatchSkipsRetryHashes(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	seeded := s.seedRetrySession(t, store, 3)

	var lookups atomic.Int32
	svc := s.newService(t, store, s.retryConfig(), nonceWatchIdle(),
		confirmedNonce(s.nonce),
		backendmock.WithHeaderbyNumberFunc(func(context.Context, *big.Int) (*types.Header, error) {
			return nil, errors.New("rpc unavailable")
		}),
		backendmock.WithTransactionByHashFunc(func(context.Context, common.Hash) (*types.Transaction, bool, error) {
			lookups.Add(1)
			return nil, false, ethereum.NotFound
		}),
		receiptNotFound(),
	)
	require.NoError(t, svc.Close())

	assert.Zero(t, lookups.Load())
	pending := pendingSet(t, svc)
	for _, h := range hashesOf(seeded) {
		assert.True(t, pending[h], "session tx %s must stay pending", h)
	}
}

// knownTxs answers TransactionByHash for every broadcast tx, as a node whose mempool holds them.
type knownTxs struct {
	mu  sync.Mutex
	txs map[common.Hash]*types.Transaction
}

func newKnownTxs(seeded []capturedBroadcast) *knownTxs {
	k := &knownTxs{txs: make(map[common.Hash]*types.Transaction)}
	for _, b := range seeded {
		k.txs[b.Hash] = types.NewTx(&types.DynamicFeeTx{Nonce: b.Nonce})
	}
	return k
}

func (k *knownTxs) send(txs *broadcastLog) backendmock.Option {
	return backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
		k.mu.Lock()
		k.txs[tx.Hash()] = tx
		k.mu.Unlock()
		txs.add(tx)
		return nil
	})
}

func (k *knownTxs) byHash() backendmock.Option {
	return backendmock.WithTransactionByHashFunc(func(_ context.Context, hash common.Hash) (*types.Transaction, bool, error) {
		k.mu.Lock()
		defer k.mu.Unlock()
		tx, ok := k.txs[hash]
		if !ok {
			return nil, false, ethereum.NotFound
		}
		return tx, true, nil
	})
}

func (k *knownTxs) nonce(t *testing.T, hash common.Hash) uint64 {
	t.Helper()
	k.mu.Lock()
	defer k.mu.Unlock()
	tx, ok := k.txs[hash]
	require.True(t, ok, "tx %s was not broadcast", hash)
	return tx.Nonce()
}

// A restarted node resumes a session while the agent sends a new tx. The RPC pending nonce
// does not see the session tx; the new tx must still take the next nonce.
func TestSendWithRetry_ResumedAndNewSendUseDistinctNonces(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	known := newKnownTxs(s.seedRetrySession(t, store, 3))

	var txs broadcastLog
	svc := s.newService(t, store, s.retryConfig(), nonceWatchIdle(),
		confirmedNonce(s.nonce),
		known.send(&txs),
		known.byHash(),
		receiptNotFound(),
	)

	txHash, _, err := svc.SendWithRetry(context.Background(), s.request())
	require.ErrorIs(t, err, transaction.ErrAllAttemptsExhausted)
	require.NoError(t, svc.Close())

	assert.Equal(t, s.nonce+1, known.nonce(t, txHash))
}

// Two sends race for a nonce. The second must see the first as pending even while the first
// is still being written to disk and the RPC pending nonce lags behind.
func TestSendWithRetry_ConcurrentSendsUseDistinctNonces(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := &slowStoredTxPutStore{StateStorer: newRetryStore(t), delay: 100 * time.Millisecond}

	cfg := s.retryConfig()
	cfg.AttemptsPerTier = 1

	known := newKnownTxs(nil)
	var txs broadcastLog
	svc := s.newService(t, store, cfg, nonceWatchIdle(),
		known.send(&txs),
		known.byHash(),
		receiptNotFound(),
	)

	var (
		wg     sync.WaitGroup
		hashes [2]common.Hash
	)
	for i := range hashes {
		wg.Go(func() {
			hashes[i], _, _ = svc.SendWithRetry(context.Background(), s.request())
		})
	}
	wg.Wait()
	require.NoError(t, svc.Close())

	assert.ElementsMatch(t,
		[]uint64{s.nonce, s.nonce + 1},
		[]uint64{known.nonce(t, hashes[0]), known.nonce(t, hashes[1])},
	)
}

// MaxTxPrice below the first fee cap: nothing is sent and nothing is stored.
func TestSendWithRetry_MaxTxPriceCap(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	cfg := s.retryConfig()
	cfg.MaxTxPrice = big.NewInt(2199)

	var txs broadcastLog
	svc := s.newService(t, store, cfg, nonceWatchIdle(), txs.sendOK())

	_, _, err := svc.SendWithRetry(context.Background(), s.request())

	require.ErrorIs(t, err, transaction.ErrTxMaxPriceExceeded)
	assert.Zero(t, txs.count(), "no transaction should be sent when maxTxPrice is below the minimum fee")
	assert.Empty(t, pendingSet(t, svc))
	assert.False(t, s.resumesAfterRestart(t, store))
}

// The fee cap reaches MaxTxPrice after the first broadcast was accepted. The escalation stops,
// but the accepted tx is in the mempool: the session stays and the background watch finishes it.
func TestSendWithRetry_MaxTxPriceAfterBroadcast(t *testing.T) {
	t.Parallel()
	s := newRetryTestSetup()
	store := newRetryStore(t)

	cfg := s.retryConfig()
	cfg.MaxTxPrice = big.NewInt(2200) // the first fee cap; the replacement needs 2530

	var txs broadcastLog
	minedC := make(chan struct{})
	svc := s.newService(t, store, cfg, nonceWatchOn(minedC), txs.sendOK(), receiptFound())

	_, _, err := svc.SendWithRetry(context.Background(), s.request())

	require.ErrorIs(t, err, transaction.ErrTxMaxPriceExceeded)
	require.Equal(t, 1, txs.count())
	s.assertSessionKept(t, svc, store, txs.hash(0))

	close(minedC)
	require.Eventually(t, noPending(svc), time.Second, 10*time.Millisecond)
	require.NoError(t, svc.Close())
	s.assertSessionCleared(t, svc, store, txs.hash(0), txs.hash(0))
}

// The request fee priority replaces the node start tier in either direction; the node end tier
// stays the ceiling.
func TestSendWithRetry_FeePriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		startTier string
		endTier   string
		priority  string
		wantTips  []int64
	}{
		{
			name:      "lower start tier still escalates to the node end tier",
			startTier: "market",
			endTier:   "aggressive",
			priority:  "low",
			wantTips:  []int64{100, 200, 300},
		},
		{
			name:      "start tier above the node end tier is clamped",
			startTier: "low",
			endTier:   "market",
			priority:  "aggressive",
			wantTips:  []int64{200},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newRetryTestSetup()
			store := newRetryStore(t)

			cfg := s.retryConfig()
			cfg.StartTier = tc.startTier
			cfg.EndTier = tc.endTier
			cfg.AttemptsPerTier = 1

			var txs broadcastLog
			minedC := make(chan struct{})
			svc := s.newService(t, store, cfg, nonceWatchOn(minedC),
				backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
					if txs.add(tx) == len(tc.wantTips) {
						close(minedC)
					}
					return nil
				}),
				receiptFound(),
			)

			ctx := sctx.SetFeePriority(context.Background(), tc.priority)
			_, receipt, err := svc.SendWithRetry(ctx, s.request())

			require.NoError(t, err)
			require.NotNil(t, receipt)
			assert.Equal(t, tc.wantTips, tipCaps(txs.all()))
		})
	}
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

// slowStoredTxPutStore delays writes of stored transactions, widening the gap between a
// broadcast and its pending record.
type slowStoredTxPutStore struct {
	storage.StateStorer
	delay time.Duration
}

func (s *slowStoredTxPutStore) Put(key string, i any) error {
	if _, ok := i.(transaction.StoredTransaction); ok {
		time.Sleep(s.delay)
	}
	return s.StateStorer.Put(key, i)
}
