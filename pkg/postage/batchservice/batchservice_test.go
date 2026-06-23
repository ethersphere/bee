// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchservice_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"hash"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/postage/batchservice"
	"github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/v2/pkg/postage/snapshot"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	mocks "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

var (
	testLog    = log.Noop
	errTest    = errors.New("fails")
	testTxHash = common.BytesToHash(make([]byte, 32))
)

type mockListener struct{}

func (*mockListener) Listen(ctx context.Context, from uint64, updater postage.EventUpdater) <-chan error {
	c := make(chan error, 1)
	c <- nil
	return c
}
func (*mockListener) Close() error { return nil }

func newMockListener() *mockListener {
	return &mockListener{}
}

type mockBatchListener struct {
	createCount int
	topupCount  int
	diluteCount int
}

func (m *mockBatchListener) HandleCreate(b *postage.Batch, _ *big.Int) error {
	m.createCount++
	return nil
}

func (m *mockBatchListener) HandleTopUp(_ []byte, _ *big.Int) {
	m.topupCount++
}

func (m *mockBatchListener) HandleDepthIncrease(_ []byte, _ uint8) {
	m.diluteCount++
}

var _ postage.BatchEventListener = (*mockBatchListener)(nil)

func TestBatchServiceCreate(t *testing.T) {
	t.Parallel()

	testChainState := postagetesting.NewChainState()

	validateNoBatch := func(t *testing.T, testBatch *postage.Batch, st *mock.BatchStore) {
		t.Helper()

		got, err := st.Exists(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}
		if got {
			t.Fatalf("expected batch not to exist")
		}
	}

	validateBatch := func(t *testing.T, testBatch *postage.Batch, st *mock.BatchStore) {
		t.Helper()

		got, err := st.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if !bytes.Equal(got.ID, testBatch.ID) {
			t.Fatalf("batch id: want %v, got %v", testBatch.ID, got.ID)
		}
		if !bytes.Equal(got.Owner, testBatch.Owner) {
			t.Fatalf("batch owner: want %v, got %v", testBatch.Owner, got.Owner)
		}
		if got.Value.Cmp(testBatch.Value) != 0 {
			t.Fatalf("batch value: want %v, got %v", testBatch.Value.String(), got.Value.String())
		}
		if got.BucketDepth != testBatch.BucketDepth {
			t.Fatalf("bucket depth: want %v, got %v", got.BucketDepth, testBatch.BucketDepth)
		}
		if got.Depth != testBatch.Depth {
			t.Fatalf("batch depth: want %v, got %v", got.Depth, testBatch.Depth)
		}
		if got.Immutable != testBatch.Immutable {
			t.Fatalf("immutable: want %v, got %v", got.Immutable, testBatch.Immutable)
		}
		if got.Start != testChainState.Block {
			t.Fatalf("batch start block different form chain state: want %v, got %v", got.Start, testChainState.Block)
		}
	}

	t.Run("expect put create put error", func(t *testing.T) {
		testBatch := postagetesting.MustNewBatch()
		testBatchListener := &mockBatchListener{}
		testAmount := postagetesting.NewBigInt()

		svc, _, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithChainState(testChainState),
			mock.WithSaveError(errTest),
		)

		if err := svc.Create(
			testBatch.ID,
			testBatch.Owner,
			testBatch.Value,
			testAmount,
			testBatch.Depth,
			testBatch.BucketDepth,
			testBatch.Immutable,
			testTxHash,
		); err == nil {
			t.Fatalf("expected error")
		}
		if testBatchListener.createCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.createCount)
		}
	})

	t.Run("passes", func(t *testing.T) {
		testBatch := postagetesting.MustNewBatch()
		testBatchListener := &mockBatchListener{}
		testAmount := postagetesting.NewBigInt()

		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithChainState(testChainState),
		)
		testBatch.Value.Add(testChainState.TotalAmount, big.NewInt(0).Mul(testChainState.CurrentPrice, big.NewInt(2)))
		if err := svc.Create(
			testBatch.ID,
			testBatch.Owner,
			testAmount,
			testBatch.Value,
			testBatch.Depth,
			testBatch.BucketDepth,
			testBatch.Immutable,
			testTxHash,
		); err != nil {
			t.Fatalf("got error %v", err)
		}
		if testBatchListener.createCount != 1 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 1, testBatchListener.createCount)
		}

		validateBatch(t, testBatch, batchStore)
	})

	t.Run("passes without recovery", func(t *testing.T) {
		testBatch := postagetesting.MustNewBatch()
		testBatchListener := &mockBatchListener{}
		testAmount := postagetesting.NewBigInt()

		// create a owner different from the batch owner
		owner := testutil.RandBytes(t, 32)

		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			owner,
			testBatchListener,
			mock.WithChainState(testChainState),
		)

		testBatch.Value.Add(testChainState.TotalAmount, big.NewInt(0).Mul(testChainState.CurrentPrice, big.NewInt(2)))

		if err := svc.Create(
			testBatch.ID,
			testBatch.Owner,
			testAmount,
			testBatch.Value,
			testBatch.Depth,
			testBatch.BucketDepth,
			testBatch.Immutable,
			testTxHash,
		); err != nil {
			t.Fatalf("got error %v", err)
		}
		if testBatchListener.createCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.createCount)
		}

		validateBatch(t, testBatch, batchStore)
	})

	t.Run("batch with near-zero val rejected", func(t *testing.T) {
		testBatch := postagetesting.MustNewBatch()
		testBatchListener := &mockBatchListener{}
		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithChainState(testChainState),
		)

		vv := big.NewInt(0).Add(testChainState.CurrentPrice, testChainState.TotalAmount)
		if err := svc.Create(
			testBatch.ID,
			testBatch.Owner,
			testBatch.Value,
			vv,
			testBatch.Depth,
			testBatch.BucketDepth,
			testBatch.Immutable,
			testTxHash,
		); !errors.Is(err, batchservice.ErrZeroValueBatch) {
			t.Fatalf("got error %v", err)
		}
		if testBatchListener.createCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.createCount)
		}

		validateNoBatch(t, testBatch, batchStore)
	})
}

func TestBatchServiceTopUp(t *testing.T) {
	t.Parallel()

	testBatch := postagetesting.MustNewBatch()
	testNormalisedBalance := big.NewInt(2000000000000)
	testTopUpAmount := big.NewInt(1000)
	t.Run("expect get error", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		svc, _, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithGetErr(errTest, 0),
		)

		if err := svc.TopUp(testBatch.ID, testTopUpAmount, testNormalisedBalance, testTxHash); err == nil {
			t.Fatal("expected error")
		}
		if testBatchListener.topupCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.topupCount)
		}
	})

	t.Run("expect update error", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithUpdateErr(errTest, 0),
		)
		createBatch(t, batchStore, testBatch)

		if err := svc.TopUp(testBatch.ID, testTopUpAmount, testNormalisedBalance, testTxHash); err == nil {
			t.Fatal("expected error")
		}
		if testBatchListener.topupCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.topupCount)
		}
	})

	t.Run("passes", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
		)
		createBatch(t, batchStore, testBatch)

		want := testNormalisedBalance

		if err := svc.TopUp(testBatch.ID, testTopUpAmount, testNormalisedBalance, testTxHash); err != nil {
			t.Fatalf("top up: %v", err)
		}

		got, err := batchStore.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if got.Value.Cmp(want) != 0 {
			t.Fatalf("topped up amount: got %v, want %v", got.Value, want)
		}
		if testBatchListener.topupCount != 1 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 1, testBatchListener.topupCount)
		}
	})

	// if a batch with a different owner is topped up we should not see any event fired in the
	// batch service
	t.Run("passes without BatchEventListener update", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		// create an owner different from the batch owner
		owner := testutil.RandBytes(t, 32)

		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			owner,
			testBatchListener,
		)
		createBatch(t, batchStore, testBatch)

		want := testNormalisedBalance

		if err := svc.TopUp(testBatch.ID, testTopUpAmount, testNormalisedBalance, testTxHash); err != nil {
			t.Fatalf("top up: %v", err)
		}

		got, err := batchStore.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if got.Value.Cmp(want) != 0 {
			t.Fatalf("topped up amount: got %v, want %v", got.Value, want)
		}
		if testBatchListener.topupCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.topupCount)
		}
	})
}

func TestBatchServiceUpdateDepth(t *testing.T) {
	t.Parallel()

	const testNewDepth = 30
	testNormalisedBalance := big.NewInt(2000000000000)
	testBatch := postagetesting.MustNewBatch()

	t.Run("expect get error", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		svc, _, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithGetErr(errTest, 0),
		)

		if err := svc.UpdateDepth(testBatch.ID, testNewDepth, testNormalisedBalance, testTxHash); err == nil {
			t.Fatal("expected get error")
		}

		if testBatchListener.diluteCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.diluteCount)
		}
	})

	t.Run("expect put error", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithUpdateErr(errTest, 0),
		)
		createBatch(t, batchStore, testBatch)

		if err := svc.UpdateDepth(testBatch.ID, testNewDepth, testNormalisedBalance, testTxHash); err == nil {
			t.Fatal("expected put error")
		}

		if testBatchListener.diluteCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.diluteCount)
		}
	})

	t.Run("passes", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
		)
		createBatch(t, batchStore, testBatch)

		if err := svc.UpdateDepth(testBatch.ID, testNewDepth, testNormalisedBalance, testTxHash); err != nil {
			t.Fatalf("update depth: %v", err)
		}

		val, err := batchStore.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if val.Depth != testNewDepth {
			t.Fatalf("wrong batch depth set: want %v, got %v", testNewDepth, val.Depth)
		}

		if testBatchListener.diluteCount != 1 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 1, testBatchListener.diluteCount)
		}
	})

	// if a batch with a different owner is diluted we should not see any event fired in the
	// batch service
	t.Run("passes without BatchEventListener update", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		// create an owner different from the batch owner
		owner := testutil.RandBytes(t, 32)

		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			owner,
			testBatchListener,
		)
		createBatch(t, batchStore, testBatch)

		if err := svc.UpdateDepth(testBatch.ID, testNewDepth, testNormalisedBalance, testTxHash); err != nil {
			t.Fatalf("update depth: %v", err)
		}

		val, err := batchStore.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if val.Depth != testNewDepth {
			t.Fatalf("wrong batch depth set: want %v, got %v", testNewDepth, val.Depth)
		}

		if testBatchListener.diluteCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.diluteCount)
		}
	})
}

func TestBatchServiceUpdatePrice(t *testing.T) {
	t.Parallel()

	testChainState := postagetesting.NewChainState()
	testChainState.CurrentPrice = big.NewInt(100000)
	testNewPrice := big.NewInt(20000000)

	t.Run("expect put error", func(t *testing.T) {
		svc, batchStore, _ := newTestStoreAndService(
			t,
			mock.WithChainState(testChainState),
			mock.WithUpdateErr(errTest, 1),
		)
		putChainState(t, batchStore, testChainState)

		if err := svc.UpdatePrice(testNewPrice, testTxHash); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		svc, batchStore, _ := newTestStoreAndService(
			t,
			mock.WithChainState(testChainState),
		)

		if err := svc.UpdatePrice(testNewPrice, testTxHash); err != nil {
			t.Fatalf("update price: %v", err)
		}

		cs := batchStore.GetChainState()
		if cs.CurrentPrice.Cmp(testNewPrice) != 0 {
			t.Fatalf("bad price: want %v, got %v", cs.CurrentPrice, testNewPrice)
		}
	})
}

func TestBatchServiceUpdateBlockNumber(t *testing.T) {
	t.Parallel()

	testChainState := &postage.ChainState{
		Block:        1,
		CurrentPrice: big.NewInt(100),
		TotalAmount:  big.NewInt(100),
	}
	svc, batchStore, _ := newTestStoreAndService(
		t,
		mock.WithChainState(testChainState),
	)

	// advance the block number and expect total cumulative payout to update
	nextBlock := uint64(4)

	if err := svc.UpdateBlockNumber(nextBlock); err != nil {
		t.Fatalf("update price: %v", err)
	}
	nn := big.NewInt(400)
	cs := batchStore.GetChainState()
	if cs.TotalAmount.Cmp(nn) != 0 {
		t.Fatalf("bad price: want %v, got %v", nn, cs.TotalAmount)
	}
}

func TestTransactionOk(t *testing.T) {
	t.Parallel()

	svc, store, s := newTestStoreAndService(t)
	if err := svc.Start(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}

	if err := svc.TransactionEnd(); err != nil {
		t.Fatal(err)
	}

	svc2, _, err := batchservice.New(context.Background(), s, store, testLog, newMockListener(), nil, nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc2.Start(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if c := store.ResetCalls(); c != 0 {
		t.Fatalf("expect %d reset calls got %d", 0, c)
	}
}

func TestTransactionError(t *testing.T) {
	t.Parallel()

	svc, store, s := newTestStoreAndService(t)
	if err := svc.Start(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}

	svc2, _, err := batchservice.New(context.Background(), s, store, testLog, newMockListener(), nil, nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc2.Start(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if c := store.ResetCalls(); c != 1 {
		t.Fatalf("expect %d reset calls got %d", 1, c)
	}
}

// TestResyncControlsReset checks that, with no snapshot, the resync flag passed
// to New determines whether the store is reset, and that the reset happens in
// New (the constructor), with Start never resetting.
func TestResyncControlsReset(t *testing.T) {
	t.Parallel()

	t.Run("resync resets the store in New", func(t *testing.T) {
		t.Parallel()

		s := mocks.NewStateStore()
		store := mock.New()
		svc, _, err := batchservice.New(context.Background(), s, store, testLog, newMockListener(), nil, nil, nil, nil, true)
		if err != nil {
			t.Fatal(err)
		}

		// The reset happens in the constructor.
		if c := store.ResetCalls(); c != 1 {
			t.Fatalf("expect %d reset calls after New got %d", 1, c)
		}

		// Start never resets.
		if err := svc.Start(context.Background(), 10); err != nil {
			t.Fatal(err)
		}
		if c := store.ResetCalls(); c != 1 {
			t.Fatalf("expect %d reset calls got %d", 1, c)
		}
	})

	t.Run("no resync leaves the store intact", func(t *testing.T) {
		t.Parallel()

		s := mocks.NewStateStore()
		store := mock.New()
		svc, _, err := batchservice.New(context.Background(), s, store, testLog, newMockListener(), nil, nil, nil, nil, false)
		if err != nil {
			t.Fatal(err)
		}

		if err := svc.Start(context.Background(), 10); err != nil {
			t.Fatal(err)
		}

		if c := store.ResetCalls(); c != 0 {
			t.Fatalf("expect %d reset calls got %d", 0, c)
		}
	})
}

type recordingListener struct {
	from      uint64
	syncedTo  uint64 // when non-zero, the replay advances the chain state to this block
	listened  bool
	closed    bool
	listenErr error
	closeErr  error
}

func (r *recordingListener) Listen(_ context.Context, from uint64, updater postage.EventUpdater) <-chan error {
	r.listened = true
	r.from = from
	// Mimic the real listener advancing the chain state during replay.
	if r.syncedTo != 0 && r.listenErr == nil {
		_ = updater.UpdateBlockNumber(r.syncedTo)
	}
	c := make(chan error, 1)
	c <- r.listenErr
	return c
}

func (r *recordingListener) Close() error {
	r.closed = true
	return r.closeErr
}

// TestSnapshotRebuild covers the snapshot rebuild path: live sync resumes from
// the block the replay reached (not the snapshot tip), and the store is reset at
// most once even when --resync is set alongside a snapshot (#5495).
func TestSnapshotRebuild(t *testing.T) {
	t.Parallel()

	newSnapshot := func() (*recordingListener, *batchservice.Snapshot) {
		snapListener := &recordingListener{}
		return snapListener, &batchservice.Snapshot{
			Listener:   snapListener,
			StartBlock: 100,
		}
	}

	t.Run("live sync resumes from where the replay stopped, not the snapshot tip", func(t *testing.T) {
		t.Parallel()

		s := mocks.NewStateStore()
		store := mock.New()
		// A valid chain state must exist before the replay advances it.
		putChainState(t, store, &postage.ChainState{Block: 0, TotalAmount: big.NewInt(0), CurrentPrice: big.NewInt(0)})

		// The real replay stops a few blocks below the snapshot tip (the listener
		// trims its tip), so live sync must resume where it actually stopped, not
		// at the tip — otherwise the trimmed blocks are skipped (#5495).
		snapListener := &recordingListener{syncedTo: 4090}
		snapshot := &batchservice.Snapshot{Listener: snapListener, StartBlock: 100}
		liveListener := &recordingListener{}

		svc, loaded, err := batchservice.New(context.Background(), s, store, testLog, liveListener, nil, nil, nil, snapshot, false)
		if err != nil {
			t.Fatal(err)
		}
		if !loaded {
			t.Fatal("expected snapshot to be loaded")
		}
		// The store is empty when a snapshot applies (snapshot only runs when no
		// batch store exists), so without --resync there is nothing to reset.
		if c := store.ResetCalls(); c != 0 {
			t.Fatalf("expect %d reset calls got %d", 0, c)
		}
		if snapListener.from != snapshot.StartBlock+1 {
			t.Fatalf("expect snapshot replay from %d got %d", snapshot.StartBlock+1, snapListener.from)
		}
		if !snapListener.closed {
			t.Fatal("expected snapshot listener to be closed")
		}

		// Live sync resumes from cs.Block+1, where the replay stopped.
		if err := svc.Start(context.Background(), snapshot.StartBlock); err != nil {
			t.Fatal(err)
		}
		if liveListener.from != 4091 {
			t.Fatalf("expect live sync to resume from 4091 (replay stop +1) got %d", liveListener.from)
		}
		if c := store.ResetCalls(); c != 0 {
			t.Fatalf("expect store never reset, got %d", c)
		}
	})

	t.Run("resync alongside a snapshot still resets only once", func(t *testing.T) {
		t.Parallel()

		s := mocks.NewStateStore()
		store := mock.New()
		_, snapshot := newSnapshot()
		liveListener := &recordingListener{}

		svc, loaded, err := batchservice.New(context.Background(), s, store, testLog, liveListener, nil, nil, nil, snapshot, true)
		if err != nil {
			t.Fatal(err)
		}
		if !loaded {
			t.Fatal("expected snapshot to be loaded")
		}
		// --resync resets once during the snapshot rebuild in New.
		if c := store.ResetCalls(); c != 1 {
			t.Fatalf("expect %d reset calls after New got %d", 1, c)
		}

		if err := svc.Start(context.Background(), snapshot.StartBlock); err != nil {
			t.Fatal(err)
		}
		// Start must not reset again: that second reset was the #5495 bug.
		if c := store.ResetCalls(); c != 1 {
			t.Fatalf("expect store reset exactly once with snapshot+resync, got %d", c)
		}
	})
}

// TestSnapshotCornerCases pushes the snapshot rebuild path into its failure and
// boundary scenarios: a failed replay, a dirty shutdown coinciding with a
// snapshot, a chain state already ahead of the snapshot, and a listener that
// errors on close.
func TestSnapshotCornerCases(t *testing.T) {
	t.Parallel()

	newSnapshot := func(listenErr error) (*recordingListener, *batchservice.Snapshot) {
		snapListener := &recordingListener{listenErr: listenErr}
		return snapListener, &batchservice.Snapshot{
			Listener:   snapListener,
			StartBlock: 100,
		}
	}

	t.Run("replay failure falls back to live sync without loading", func(t *testing.T) {
		t.Parallel()

		s := mocks.NewStateStore()
		store := mock.New()
		snapListener, snapshot := newSnapshot(errTest)
		liveListener := &recordingListener{}

		svc, loaded, err := batchservice.New(context.Background(), s, store, testLog, liveListener, nil, nil, nil, snapshot, false)
		if err != nil {
			t.Fatalf("a failed replay must not be fatal: %v", err)
		}
		if loaded {
			t.Fatal("expected snapshot not to be loaded on replay error")
		}
		if !snapListener.closed {
			t.Fatal("expected snapshot listener to be closed even on failure")
		}
		// A partial replay may have dirtied the store, so New resets it once to
		// clean up for the live rebuild.
		if c := store.ResetCalls(); c != 1 {
			t.Fatalf("expect %d reset calls after New got %d", 1, c)
		}

		// Live sync still proceeds, from the requested start block, since no
		// snapshot resume point was recorded; Start never resets.
		if err := svc.Start(context.Background(), snapshot.StartBlock); err != nil {
			t.Fatal(err)
		}
		if liveListener.from != snapshot.StartBlock+1 {
			t.Fatalf("expect live sync from %d got %d", snapshot.StartBlock+1, liveListener.from)
		}
		if c := store.ResetCalls(); c != 1 {
			t.Fatalf("expect store reset exactly once, got %d", c)
		}
	})

	t.Run("replay failure with resync resets once before and once after the replay", func(t *testing.T) {
		t.Parallel()

		s := mocks.NewStateStore()
		store := mock.New()
		_, snapshot := newSnapshot(errTest)
		liveListener := &recordingListener{}

		svc, loaded, err := batchservice.New(context.Background(), s, store, testLog, liveListener, nil, nil, nil, snapshot, true)
		if err != nil {
			t.Fatal(err)
		}
		if loaded {
			t.Fatal("expected snapshot not to be loaded on replay error")
		}
		// resync resets once up front, then the failed replay's cleanup resets
		// again — both in New. Start never resets.
		if c := store.ResetCalls(); c != 2 {
			t.Fatalf("expect %d reset calls after New got %d", 2, c)
		}
		if err := svc.Start(context.Background(), snapshot.StartBlock); err != nil {
			t.Fatal(err)
		}
		if c := store.ResetCalls(); c != 2 {
			t.Fatalf("expect %d reset calls after Start got %d", 2, c)
		}
	})

	t.Run("dirty shutdown with a snapshot resets exactly once", func(t *testing.T) {
		t.Parallel()

		s := mocks.NewStateStore()
		store := mock.New()

		// Simulate a dirty shutdown: a prior service started a transaction and
		// never finished it, leaving the dirty marker set.
		prior, _, err := batchservice.New(context.Background(), s, store, testLog, newMockListener(), nil, nil, nil, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := prior.TransactionStart(); err != nil {
			t.Fatal(err)
		}

		snapListener, snapshot := newSnapshot(nil)
		liveListener := &recordingListener{}

		svc, loaded, err := batchservice.New(context.Background(), s, store, testLog, liveListener, nil, nil, nil, snapshot, false)
		if err != nil {
			t.Fatal(err)
		}
		if !loaded {
			t.Fatal("expected snapshot to be loaded")
		}
		if !snapListener.closed {
			t.Fatal("expected snapshot listener to be closed")
		}
		// The dirty flag triggers one reset during the snapshot rebuild...
		if c := store.ResetCalls(); c != 1 {
			t.Fatalf("expect %d reset calls after New got %d", 1, c)
		}
		// ...and Start must not reset again and wipe the freshly loaded snapshot.
		if err := svc.Start(context.Background(), snapshot.StartBlock); err != nil {
			t.Fatal(err)
		}
		if c := store.ResetCalls(); c != 1 {
			t.Fatalf("expect store reset exactly once, got %d", c)
		}
	})

	t.Run("persisted chain state ahead of the snapshot block wins", func(t *testing.T) {
		t.Parallel()

		s := mocks.NewStateStore()
		store := mock.New()
		_, snapshot := newSnapshot(nil)
		liveListener := &recordingListener{}

		svc, loaded, err := batchservice.New(context.Background(), s, store, testLog, liveListener, nil, nil, nil, snapshot, false)
		if err != nil {
			t.Fatal(err)
		}
		if !loaded {
			t.Fatal("expected snapshot to be loaded")
		}

		// A chain state further ahead than the replay must take precedence so
		// live sync never rewinds and reprocesses events.
		putChainState(t, store, &postage.ChainState{Block: 5000, TotalAmount: big.NewInt(0), CurrentPrice: big.NewInt(0)})

		if err := svc.Start(context.Background(), snapshot.StartBlock); err != nil {
			t.Fatal(err)
		}
		if liveListener.from != 5001 {
			t.Fatalf("expect live sync from 5001 got %d", liveListener.from)
		}
	})

	t.Run("snapshot listener close error is not fatal", func(t *testing.T) {
		t.Parallel()

		s := mocks.NewStateStore()
		store := mock.New()
		snapListener := &recordingListener{closeErr: errTest}
		snapshot := &batchservice.Snapshot{
			Listener:   snapListener,
			StartBlock: 100,
		}

		_, loaded, err := batchservice.New(context.Background(), s, store, testLog, &recordingListener{}, nil, nil, nil, snapshot, false)
		if err != nil {
			t.Fatal(err)
		}
		if !loaded {
			t.Fatal("expected snapshot to be loaded despite a listener close error")
		}
		if !snapListener.closed {
			t.Fatal("expected snapshot listener close to be attempted")
		}
	})
}

// TestSnapshotHandoffNoGap guards the snapshot->RPC handoff (#5495): after a
// snapshot replay, live sync must resume from where the replay stopped
// (cs.Block+1), not the snapshot's nominal tip (maxBlock+1), which would skip the
// blocks the listener trims off the tip.
func TestSnapshotHandoffNoGap(t *testing.T) {
	t.Parallel()

	const maxBlock = uint64(5000)

	// Newest log at maxBlock; a non-matching address makes the listener filter
	// the events out, so it only advances the chain state per page.
	logs := []types.Log{
		{BlockNumber: 10, Address: common.HexToAddress("0x1"), Topics: []common.Hash{}},
		{BlockNumber: maxBlock, Address: common.HexToAddress("0x1"), Topics: []common.Hash{}},
	}
	snap, err := snapshot.New(context.Background(), testLog, rawSnapshotGetter(gzipSnapshot(t, logs)), nil,
		common.Address{}, abi.ABI{}, time.Second, time.Minute, time.Second, 0)
	if err != nil {
		t.Fatalf("snapshot.New: %v", err)
	}

	s := mocks.NewStateStore()
	store := mock.New()
	// Valid chain state so the replay can advance it.
	putChainState(t, store, &postage.ChainState{Block: 0, TotalAmount: big.NewInt(0), CurrentPrice: big.NewInt(0)})

	live := &recordingListener{}
	svc, loaded, err := batchservice.New(context.Background(), s, store, testLog, live, nil, nil, nil, snap, false)
	if err != nil {
		t.Fatalf("batchservice.New: %v", err)
	}
	if !loaded {
		t.Fatal("expected snapshot to be loaded")
	}

	cs := store.GetChainState()
	if cs.Block >= maxBlock {
		t.Fatalf("replay reached %d, expected to stop below the snapshot max block %d", cs.Block, maxBlock)
	}

	if err := svc.Start(context.Background(), 0); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Must resume where the replay stopped, not at the snapshot tip.
	if live.from != cs.Block+1 {
		t.Fatalf("live sync resumed from %d; must resume from cs.Block+1 = %d (resuming higher skips the snapshot's trimmed tail — see #5495)", live.from, cs.Block+1)
	}
}

func gzipSnapshot(t *testing.T, logs []types.Log) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	enc := json.NewEncoder(gz)
	for _, l := range logs {
		if err := enc.Encode(l); err != nil {
			t.Fatalf("encode log: %v", err)
		}
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

type rawSnapshotGetter []byte

func (g rawSnapshotGetter) GetBatchSnapshot() []byte { return g }

func TestChecksum(t *testing.T) {
	t.Parallel()

	s := mocks.NewStateStore()
	store := mock.New()
	mockHash := &hs{}
	svc, _, err := batchservice.New(context.Background(), s, store, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash }, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	testNormalisedBalance := big.NewInt(2000000000000)
	testTopUpAmount := big.NewInt(1000)
	testBatch := postagetesting.MustNewBatch()
	createBatch(t, store, testBatch)

	if err := svc.TopUp(testBatch.ID, testTopUpAmount, testNormalisedBalance, testTxHash); err != nil {
		t.Fatalf("top up: %v", err)
	}
	if m := mockHash.ctr; m != 2 {
		t.Fatalf("expected %d calls got %d", 2, m)
	}
}

func TestChecksumResync(t *testing.T) {
	t.Parallel()

	s := mocks.NewStateStore()
	store := mock.New()
	mockHash := &hs{}
	svc, _, err := batchservice.New(context.Background(), s, store, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash }, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	testNormalisedBalance := big.NewInt(2000000000000)
	testTopUpAmount := big.NewInt(1000)
	testBatch := postagetesting.MustNewBatch()
	createBatch(t, store, testBatch)

	if err := svc.TopUp(testBatch.ID, testTopUpAmount, testNormalisedBalance, testTxHash); err != nil {
		t.Fatalf("top up: %v", err)
	}
	if m := mockHash.ctr; m != 2 {
		t.Fatalf("expected %d calls got %d", 2, m)
	}

	// now start a new instance and check that the value gets read from statestore
	store2 := mock.New()
	mockHash2 := &hs{}
	_, _, err = batchservice.New(context.Background(), s, store2, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash2 }, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if m := mockHash2.ctr; m != 1 {
		t.Fatalf("expected %d calls got %d", 1, m)
	}

	// now start a new instance and check that the value does not get written into the hasher
	// when resyncing
	store3 := mock.New()
	mockHash3 := &hs{}
	_, _, err = batchservice.New(context.Background(), s, store3, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash3 }, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if m := mockHash3.ctr; m != 0 {
		t.Fatalf("expected %d calls got %d", 0, m)
	}
}

func newTestStoreAndServiceWithListener(
	t *testing.T,
	owner []byte,
	batchListener postage.BatchEventListener,
	opts ...mock.Option,
) (postage.EventUpdater, *mock.BatchStore, storage.StateStorer) {
	t.Helper()
	s := mocks.NewStateStore()
	store := mock.New(opts...)
	svc, _, err := batchservice.New(context.Background(), s, store, testLog, newMockListener(), owner, batchListener, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	return svc, store, s
}

func newTestStoreAndService(t *testing.T, opts ...mock.Option) (postage.EventUpdater, *mock.BatchStore, storage.StateStorer) {
	t.Helper()
	return newTestStoreAndServiceWithListener(t, nil, nil, opts...)
}

func createBatch(t *testing.T, store postage.Storer, b *postage.Batch) {
	t.Helper()

	if err := store.Save(b); err != nil {
		t.Fatalf("store create batch: %v", err)
	}
}

func putChainState(t *testing.T, store postage.Storer, cs *postage.ChainState) {
	t.Helper()

	if err := store.PutChainState(cs); err != nil {
		t.Fatalf("store put chain state: %v", err)
	}
}

type hs struct{ ctr uint8 }

func (h *hs) Write(p []byte) (n int, err error) { h.ctr++; return len(p), nil }
func (h *hs) Sum(b []byte) []byte               { return []byte{h.ctr} }
func (h *hs) Reset()                            {}
func (h *hs) Size() int                         { panic("not implemented") }
func (h *hs) BlockSize() int                    { panic("not implemented") }
