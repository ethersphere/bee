// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore_test

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	postagetest "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var noopEvictFn = func([]byte) error { return nil }

func TestBatchStore_Get(t *testing.T) {
	testBatch := postagetest.MustNewBatch()

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil, log.Noop)

	err := batchStore.Save(testBatch)
	if err != nil {
		t.Fatal(err)
	}

	got := batchStoreGetBatch(t, batchStore, testBatch.ID)
	postagetest.CompareBatches(t, testBatch, got)
}

func TestBatchStore_Iterate(t *testing.T) {
	testBatch := postagetest.MustNewBatch()
	key := batchstore.BatchKey(testBatch.ID)

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil, log.Noop)

	stateStorePut(t, stateStore, key, testBatch)

	var got *postage.Batch
	err := batchStore.Iterate(func(b *postage.Batch) (bool, error) {
		got = b
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	postagetest.CompareBatches(t, testBatch, got)
}

func TestBatchStore_IterateStopsEarly(t *testing.T) {
	testBatch1 := postagetest.MustNewBatch()
	key1 := batchstore.BatchKey(testBatch1.ID)

	testBatch2 := postagetest.MustNewBatch()
	key2 := batchstore.BatchKey(testBatch2.ID)

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil, log.Noop)

	stateStorePut(t, stateStore, key1, testBatch1)
	stateStorePut(t, stateStore, key2, testBatch2)

	var iterations = 0
	err := batchStore.Iterate(func(b *postage.Batch) (bool, error) {
		iterations++
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if iterations != 2 {
		t.Fatalf("wanted 2 iteration, got %d", iterations)
	}

	iterations = 0
	err = batchStore.Iterate(func(b *postage.Batch) (bool, error) {
		iterations++
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if iterations > 2 {
		t.Fatalf("wanted 1 iteration, got %d", iterations)
	}

	iterations = 0
	err = batchStore.Iterate(func(b *postage.Batch) (bool, error) {
		iterations++
		return false, errors.New("test error")
	})
	if err == nil {
		t.Fatalf("wanted error")
	}
	if iterations > 2 {
		t.Fatalf("wanted 1 iteration, got %d", iterations)
	}
}

func TestBatchStore_SaveAndUpdate(t *testing.T) {

	testBatch := postagetest.MustNewBatch()
	key := batchstore.BatchKey(testBatch.ID)

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil, log.Noop)

	if err := batchStore.Save(testBatch); err != nil {
		t.Fatalf("storer.Save(...): unexpected error: %v", err)
	}

	// call Unreserve once to increase storage radius of the test batch
	if err := batchStore.Unreserve(func(id []byte, radius uint8) (bool, error) { return false, nil }); err != nil {
		t.Fatalf("storer.Unreserve(...): unexpected error: %v", err)
	}

	//get test batch after save call
	stateStoreGet(t, stateStore, key, testBatch)

	var have postage.Batch
	stateStoreGet(t, stateStore, key, &have)
	postagetest.CompareBatches(t, testBatch, &have)

	// Check for idempotency.
	if err := batchStore.Save(testBatch); err == nil {
		t.Fatalf("storer.Save(...): expected error")
	}

	cnt := 0
	if err := stateStore.Iterate(batchstore.ValueKey(testBatch.Value, testBatch.ID), func(k, v []byte) (stop bool, err error) {
		cnt++
		return false, nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cnt > 1 {
		t.Fatal("storer.Save(...): method is not idempotent")
	}

	// Check update.
	newValue := postagetest.NewBigInt()
	newDepth := uint8(rand.Intn(int(swarm.MaxPO)))
	if err := batchStore.Update(testBatch, newValue, newDepth); err != nil {
		t.Fatalf("storer.Update(...): unexpected error: %v", err)
	}
	stateStoreGet(t, stateStore, key, &have)
	postagetest.CompareBatches(t, testBatch, &have)
}

func TestBatchStore_GetChainState(t *testing.T) {
	testChainState := postagetest.NewChainState()

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil, log.Noop)

	err := batchStore.PutChainState(testChainState)
	if err != nil {
		t.Fatal(err)
	}
	got := batchStore.GetChainState()
	postagetest.CompareChainState(t, testChainState, got)
}

func TestBatchStore_PutChainState(t *testing.T) {
	testChainState := postagetest.NewChainState()

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil, log.Noop)

	batchStorePutChainState(t, batchStore, testChainState)
	var got postage.ChainState
	stateStoreGet(t, stateStore, batchstore.StateKey, &got)
	postagetest.CompareChainState(t, testChainState, &got)
}

func TestBatchStore_SetStorageRadius(t *testing.T) {

	var (
		radius           uint8 = 5
		oldStorageRadius uint8 = 5
		newStorageRadius uint8 = 3
	)

	stateStore := mock.NewStateStore()
	_ = stateStore.Put(batchstore.ReserveStateKey, &postage.ReserveState{Radius: radius})
	batchStore, _ := batchstore.New(stateStore, nil, log.Noop)

	_ = batchStore.SetStorageRadius(func(uint8) uint8 {
		return oldStorageRadius
	})

	_ = batchStore.SetStorageRadius(func(radius uint8) uint8 {
		if radius != oldStorageRadius {
			t.Fatalf("got old radius %d, want %d", radius, oldStorageRadius)
		}
		return newStorageRadius
	})

	got := batchStore.GetReserveState().StorageRadius
	if got != newStorageRadius {
		t.Fatalf("got old radius %d, want %d", got, newStorageRadius)
	}
}

func TestBatchStore_Reset(t *testing.T) {
	testChainState := postagetest.NewChainState()
	testBatch := postagetest.MustNewBatch(
		postagetest.WithValue(15),
		postagetest.WithDepth(8),
	)

	path := t.TempDir()
	logger := log.Noop

	// we use the real statestore since the mock uses a mutex,
	// therefore deleting while iterating (in Reset() implementation)
	// leads to a deadlock.
	stateStore, err := leveldb.NewStateStore(path, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer stateStore.Close()

	batchStore, _ := batchstore.New(stateStore, noopEvictFn, log.Noop)
	err = batchStore.Save(testBatch)
	if err != nil {
		t.Fatal(err)
	}
	err = batchStore.PutChainState(testChainState)
	if err != nil {
		t.Fatal(err)
	}
	err = batchStore.Reset()
	if err != nil {
		t.Fatal(err)
	}
	c := 0
	_ = stateStore.Iterate("", func(k, _ []byte) (bool, error) {
		c++
		return false, nil
	})

	// we expect one key in the statestore since the schema name
	// will always be there.
	if c != 1 {
		t.Fatalf("expected only one key in statestore, got %d", c)
	}
}

func stateStoreGet(t *testing.T, st storage.StateStorer, k string, v interface{}) {
	t.Helper()

	if err := st.Get(k, v); err != nil {
		t.Fatalf("store get batch: %v", err)
	}
}

func stateStorePut(t *testing.T, st storage.StateStorer, k string, v interface{}) {
	t.Helper()

	if err := st.Put(k, v); err != nil {
		t.Fatalf("store put batch: %v", err)
	}
}

func batchStoreGetBatch(t *testing.T, st postage.Storer, id []byte) *postage.Batch {
	t.Helper()

	b, err := st.Get(id)
	if err != nil {
		t.Fatalf("postage storer get: %v", err)
	}
	return b
}

func batchStorePutChainState(t *testing.T, st postage.Storer, cs *postage.ChainState) {
	t.Helper()

	if err := st.PutChainState(cs); err != nil {
		t.Fatalf("postage storer put chain state: %v", err)
	}
}
