// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	postagetest "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
)

func unreserve([]byte, uint8, uint8) error { return nil }
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

	batchStorePutBatch(t, batchStore, testBatch)

	var got postage.Batch
	stateStoreGet(t, stateStore, key, &got)
	postagetest.CompareBatches(t, testBatch, &got)
}

func TestBatchStoreGetChainState(t *testing.T) {
	testChainState := postagetest.NewChainState()

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil)

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

	batchStorePutChainState(t, batchStore, testChainState)
	var got postage.ChainState
	stateStoreGet(t, stateStore, batchstore.StateKey, &got)
	postagetest.CompareChainState(t, testChainState, &got)
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
