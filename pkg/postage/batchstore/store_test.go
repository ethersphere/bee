// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore_test

import (
	"io/ioutil"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	postagetest "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
)

func unreserve([]byte, uint8) error { return nil }
func TestBatchStoreGet(t *testing.T) {
	testBatch := postagetest.MustNewBatch()
	key := batchstore.BatchKey(testBatch.ID)

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil)

	stateStorePut(t, stateStore, key, testBatch)
	got := batchStoreGetBatch(t, batchStore, testBatch.ID)
	postagetest.CompareBatches(t, testBatch, got)
}

func TestBatchStorePut(t *testing.T) {
	testBatch := postagetest.MustNewBatch()
	key := batchstore.BatchKey(testBatch.ID)

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, unreserve)
	batchStore.SetRadiusSetter(noopRadiusSetter{})
	batchStorePutBatch(t, batchStore, testBatch)

	var got postage.Batch
	stateStoreGet(t, stateStore, key, &got)
	postagetest.CompareBatches(t, testBatch, &got)
}

func TestBatchStoreGetChainState(t *testing.T) {
	testChainState := postagetest.NewChainState()

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil)
	batchStore.SetRadiusSetter(noopRadiusSetter{})

	err := batchStore.PutChainState(testChainState)
	if err != nil {
		t.Fatal(err)
	}
	got := batchStore.GetChainState()
	postagetest.CompareChainState(t, testChainState, got)
}

func TestBatchStorePutChainState(t *testing.T) {
	testChainState := postagetest.NewChainState()

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil)
	batchStore.SetRadiusSetter(noopRadiusSetter{})

	batchStorePutChainState(t, batchStore, testChainState)
	var got postage.ChainState
	stateStoreGet(t, stateStore, batchstore.StateKey, &got)
	postagetest.CompareChainState(t, testChainState, &got)
}

func TestBatchStoreReset(t *testing.T) {
	testChainState := postagetest.NewChainState()
	testBatch := postagetest.MustNewBatch()

	path := t.TempDir()
	logger := logging.New(ioutil.Discard, 0)

	// we use the real statestore since the mock uses a mutex,
	// therefore deleting while iterating (in Reset() implementation)
	// leads to a deadlock.
	stateStore, err := leveldb.NewStateStore(path, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer stateStore.Close()

	batchStore, _ := batchstore.New(stateStore, func([]byte, uint8) error { return nil })
	batchStore.SetRadiusSetter(noopRadiusSetter{})
	err = batchStore.Put(testBatch, big.NewInt(15), 8)
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
	if err := st.Get(k, v); err != nil {
		t.Fatalf("store get batch: %v", err)
	}
}

func stateStorePut(t *testing.T, st storage.StateStorer, k string, v interface{}) {
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

func batchStorePutBatch(t *testing.T, st postage.Storer, b *postage.Batch) {
	t.Helper()
	if err := st.Put(b, b.Value, b.Depth); err != nil {
		t.Fatalf("postage storer put: %v", err)
	}
}

func batchStorePutChainState(t *testing.T, st postage.Storer, cs *postage.ChainState) {
	t.Helper()
	if err := st.PutChainState(cs); err != nil {
		t.Fatalf("postage storer put chain state: %v", err)
	}
}

type noopRadiusSetter struct{}

func (n noopRadiusSetter) SetRadius(_ uint8) {}
