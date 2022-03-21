// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchservice_test

import (
	"bytes"
	"errors"
	"hash"
	"io"
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchservice"
	"github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	mocks "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
)

var (
	testLog    = logging.New(io.Discard, 0)
	errTest    = errors.New("fails")
	testTxHash = make([]byte, 32)
)

type mockListener struct {
}

func (*mockListener) Listen(from uint64, updater postage.EventUpdater, _ *postage.ChainSnapshot) <-chan struct{} {
	return nil
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

func (m *mockBatchListener) HandleCreate(b *postage.Batch) error {
	m.createCount++
	return nil
}

func (m *mockBatchListener) HandleTopUp(_ []byte, _ *big.Int) {
	m.topupCount++
}

func (m *mockBatchListener) HandleDepthIncrease(_ []byte, _ uint8, _ *big.Int) {
	m.diluteCount++
}

func TestBatchServiceCreate(t *testing.T) {
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
		svc, _, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithChainState(testChainState),
			mock.WithUpdateErr(errTest, 0),
		)

		if err := svc.Create(
			testBatch.ID,
			testBatch.Owner,
			testBatch.Value,
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
		// create a owner different from the batch owner
		owner := make([]byte, 32)
		rand.Read(owner)

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
	testBatch := postagetesting.MustNewBatch()
	testNormalisedBalance := big.NewInt(2000000000000)

	t.Run("expect get error", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		svc, _, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithGetErr(errTest, 0),
		)

		if err := svc.TopUp(testBatch.ID, testNormalisedBalance, testTxHash); err == nil {
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

		if err := svc.TopUp(testBatch.ID, testNormalisedBalance, testTxHash); err == nil {
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

		if err := svc.TopUp(testBatch.ID, testNormalisedBalance, testTxHash); err != nil {
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
		owner := make([]byte, 32)
		rand.Read(owner)

		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			owner,
			testBatchListener,
		)
		createBatch(t, batchStore, testBatch)

		want := testNormalisedBalance

		if err := svc.TopUp(testBatch.ID, testNormalisedBalance, testTxHash); err != nil {
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
		owner := make([]byte, 32)
		rand.Read(owner)

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
	svc, store, s := newTestStoreAndService(t)
	if _, err := svc.Start(10, nil); err != nil {
		t.Fatal(err)
	}

	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}

	if err := svc.TransactionEnd(); err != nil {
		t.Fatal(err)
	}

	svc2, err := batchservice.New(s, store, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc2.Start(10, nil); err != nil {
		t.Fatal(err)
	}

	if c := store.ResetCalls(); c != 0 {
		t.Fatalf("expect %d reset calls got %d", 0, c)
	}
}

func TestTransactionError(t *testing.T) {
	svc, store, s := newTestStoreAndService(t)
	if _, err := svc.Start(10, nil); err != nil {
		t.Fatal(err)
	}

	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}

	svc2, err := batchservice.New(s, store, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc2.Start(10, nil); err != nil {
		t.Fatal(err)
	}

	if c := store.ResetCalls(); c != 1 {
		t.Fatalf("expect %d reset calls got %d", 1, c)
	}
}

func TestChecksum(t *testing.T) {
	s := mocks.NewStateStore()
	store := mock.New()
	mockHash := &hs{}
	svc, err := batchservice.New(s, store, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash }, false)
	if err != nil {
		t.Fatal(err)
	}
	testNormalisedBalance := big.NewInt(2000000000000)
	testBatch := postagetesting.MustNewBatch()
	createBatch(t, store, testBatch)

	if err := svc.TopUp(testBatch.ID, testNormalisedBalance, testTxHash); err != nil {
		t.Fatalf("top up: %v", err)
	}
	if m := mockHash.ctr; m != 2 {
		t.Fatalf("expected %d calls got %d", 2, m)
	}
}

func TestChecksumResync(t *testing.T) {
	s := mocks.NewStateStore()
	store := mock.New()
	mockHash := &hs{}
	svc, err := batchservice.New(s, store, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash }, true)
	if err != nil {
		t.Fatal(err)
	}
	testNormalisedBalance := big.NewInt(2000000000000)
	testBatch := postagetesting.MustNewBatch()
	createBatch(t, store, testBatch)

	if err := svc.TopUp(testBatch.ID, testNormalisedBalance, testTxHash); err != nil {
		t.Fatalf("top up: %v", err)
	}
	if m := mockHash.ctr; m != 2 {
		t.Fatalf("expected %d calls got %d", 2, m)
	}

	// now start a new instance and check that the value gets read from statestore
	store2 := mock.New()
	mockHash2 := &hs{}
	_, err = batchservice.New(s, store2, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash2 }, false)
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
	_, err = batchservice.New(s, store3, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash3 }, true)
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
	svc, err := batchservice.New(s, store, testLog, newMockListener(), owner, batchListener, nil, false)
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
