// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchservice_test

import (
	"bytes"
	"errors"
	"io/ioutil"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchservice"
	"github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
)

var (
	testLog = logging.New(ioutil.Discard, 0)
	testErr = errors.New("fails")
)

type mockListener struct {
}

func (*mockListener) Listen(from uint64, updater postage.EventUpdater) {}
func (*mockListener) Close() error                                     { return nil }

func newMockListener() *mockListener {
	return &mockListener{}
}

func TestNewBatchService(t *testing.T) {
	t.Run("expect get error", func(t *testing.T) {
		store := mock.New(
			mock.WithGetErr(testErr, 0),
		)
		_, err := batchservice.New(store, testLog, newMockListener())
		if err == nil {
			t.Fatal("expected get error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		testChainState := postagetesting.NewChainState()
		store := mock.New(
			mock.WithChainState(testChainState),
		)
		_, err := batchservice.New(store, testLog, newMockListener())
		if err != nil {
			t.Fatalf("new batch service: %v", err)
		}
	})
}

func TestBatchServiceCreate(t *testing.T) {
	testBatch := postagetesting.MustNewBatch()
	testChainState := postagetesting.NewChainState()

	t.Run("expect put create put error", func(t *testing.T) {
		svc, _ := newTestStoreAndService(
			t,
			mock.WithChainState(testChainState),
			mock.WithPutErr(testErr, 0),
		)

		if err := svc.Create(
			testBatch.ID,
			testBatch.Owner,
			testBatch.Value,
			testBatch.Depth,
		); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		svc, batchStore := newTestStoreAndService(
			t,
			mock.WithChainState(testChainState),
		)

		if err := svc.Create(
			testBatch.ID,
			testBatch.Owner,
			testBatch.Value,
			testBatch.Depth,
		); err != nil {
			t.Fatalf("got error %v", err)
		}

		got, err := batchStore.Get(testBatch.ID)
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
		if got.Depth != testBatch.Depth {
			t.Fatalf("batch depth: want %v, got %v", got.Depth, testBatch.Depth)
		}
		if got.Start != testChainState.Block {
			t.Fatalf("batch start block different form chain state: want %v, got %v", got.Start, testChainState.Block)
		}
	})

}

func TestBatchServiceTopUp(t *testing.T) {
	testBatch := postagetesting.MustNewBatch()
	testNormalisedBalance := big.NewInt(2000000000000)

	t.Run("expect get error", func(t *testing.T) {
		svc, _ := newTestStoreAndService(
			t,
			// NOTE: we skip the error on the first get call in batchservice.New.
			mock.WithGetErr(testErr, 1),
		)

		if err := svc.TopUp(testBatch.ID, testNormalisedBalance); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("expect put error", func(t *testing.T) {
		svc, batchStore := newTestStoreAndService(
			t,
			mock.WithPutErr(testErr, 1),
		)
		putBatch(t, batchStore, testBatch)

		if err := svc.TopUp(testBatch.ID, testNormalisedBalance); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		svc, batchStore := newTestStoreAndService(t)
		putBatch(t, batchStore, testBatch)

		want := testNormalisedBalance

		if err := svc.TopUp(testBatch.ID, testNormalisedBalance); err != nil {
			t.Fatalf("top up: %v", err)
		}

		got, err := batchStore.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if got.Value.Cmp(want) != 0 {
			t.Fatalf("topped up amount: got %v, want %v", got.Value, want)
		}
	})
}

func TestBatchServiceUpdateDepth(t *testing.T) {
	const testNewDepth = 30
	testNormalisedBalance := big.NewInt(2000000000000)
	testBatch := postagetesting.MustNewBatch()

	t.Run("expect get error", func(t *testing.T) {
		svc, _ := newTestStoreAndService(
			t,
			mock.WithGetErr(testErr, 1),
		)

		if err := svc.UpdateDepth(testBatch.ID, testNewDepth, testNormalisedBalance); err == nil {
			t.Fatal("expected get error")
		}
	})

	t.Run("expect put error", func(t *testing.T) {
		svc, batchStore := newTestStoreAndService(
			t,
			mock.WithPutErr(testErr, 1),
		)
		putBatch(t, batchStore, testBatch)

		if err := svc.UpdateDepth(testBatch.ID, testNewDepth, testNormalisedBalance); err == nil {
			t.Fatal("expected put error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		svc, batchStore := newTestStoreAndService(t)
		putBatch(t, batchStore, testBatch)

		if err := svc.UpdateDepth(testBatch.ID, testNewDepth, testNormalisedBalance); err != nil {
			t.Fatalf("update depth: %v", err)
		}

		val, err := batchStore.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if val.Depth != testNewDepth {
			t.Fatalf("wrong batch depth set: want %v, got %v", testNewDepth, val.Depth)
		}
	})
}

func TestBatchServiceUpdatePrice(t *testing.T) {
	testChainState := postagetesting.NewChainState()
	testChainState.Price = big.NewInt(100000)
	testNewPrice := big.NewInt(20000000)

	t.Run("expect put error", func(t *testing.T) {
		svc, batchStore := newTestStoreAndService(
			t,
			mock.WithChainState(testChainState),
			mock.WithPutErr(testErr, 1),
		)
		putChainState(t, batchStore, testChainState)

		if err := svc.UpdatePrice(testNewPrice); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		svc, batchStore := newTestStoreAndService(
			t,
			mock.WithChainState(testChainState),
		)

		if err := svc.UpdatePrice(testNewPrice); err != nil {
			t.Fatalf("update price: %v", err)
		}

		cs, err := batchStore.GetChainState()
		if err != nil {
			t.Fatalf("batch store get chain state: %v", err)
		}

		if cs.Price.Cmp(testNewPrice) != 0 {
			t.Fatalf("bad price: want %v, got %v", cs.Price, testNewPrice)
		}
	})
}

func newTestStoreAndService(t *testing.T, opts ...mock.Option) (postage.EventUpdater, postage.Storer) {
	t.Helper()

	store := mock.New(opts...)
	svc, err := batchservice.New(store, testLog, newMockListener())
	if err != nil {
		t.Fatalf("new batch service: %v", err)
	}

	return svc, store
}

func putBatch(t *testing.T, store postage.Storer, b *postage.Batch) {
	t.Helper()

	if err := store.Put(b); err != nil {
		t.Fatalf("store put batch: %v", err)
	}
}

func putChainState(t *testing.T, store postage.Storer, cs *postage.ChainState) {
	t.Helper()

	if err := store.PutChainState(cs); err != nil {
		t.Fatalf("store put chain state: %v", err)
	}
}
