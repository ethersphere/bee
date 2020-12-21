// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	postagetest "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/statestore/mock"
)

func TestBatchStoreGet(t *testing.T) {
	stateStore := mock.NewStateStore()

	testBatch, err := postagetest.NewBatch()
	if err != nil {
		t.Fatalf("create test batch: %v", err)
	}
	key := batchstore.BatchKey(testBatch.ID)

	err = stateStore.Put(key, testBatch)
	if err != nil {
		t.Fatalf("store batch: %v", err)
	}

	bStore := batchstore.New(stateStore)

	got, err := bStore.Get(testBatch.ID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}

	compareBatches(t, testBatch, got)
}

func TestBatchStorePut(t *testing.T) {
	stateStore := mock.NewStateStore()
	bStore := batchstore.New(stateStore)

	testBatch, err := postagetest.NewBatch()
	if err != nil {
		t.Fatalf("create test batch: %v", err)
	}
	key := batchstore.BatchKey(testBatch.ID)

	err = bStore.Put(testBatch)
	if err != nil {
		t.Fatalf("put batch: %v", err)
	}

	var got postage.Batch
	err = stateStore.Get(key, &got)
	if err != nil {
		t.Fatalf("store get batch: %v", err)
	}

	compareBatches(t, testBatch, &got)
}

func TestBatchStoreGetChainState(t *testing.T) {
	stateStore := mock.NewStateStore()
	bStore := batchstore.New(stateStore)

	testState := postagetest.NewChainState()

	err := stateStore.Put(batchstore.StateKey, testState)
	if err != nil {
		t.Fatalf("stateStore put: %v", err)
	}

	got, err := bStore.GetChainState()
	if err != nil {
		t.Fatalf("get chain state: %v", err)
	}

	compareChainState(t, testState, got)
}

func TestBatchStorePutChainState(t *testing.T) {
	stateStore := mock.NewStateStore()
	bStore := batchstore.New(stateStore)

	testState := postagetest.NewChainState()

	err := bStore.PutChainState(testState)
	if err != nil {
		t.Fatalf("put chain state: %v", err)
	}

	var got postage.ChainState
	err = stateStore.Get(batchstore.StateKey, &got)
	if err != nil {
		t.Fatalf("statestore get: %v", err)
	}

	compareChainState(t, testState, &got)
}

func compareBatches(t *testing.T, want, got *postage.Batch) {
	t.Helper()

	if !bytes.Equal(want.ID, got.ID) {
		t.Fatalf("batch ID: want %v, got %v", want.ID, got.ID)
	}
	if want.Value.Cmp(got.Value) != 0 {
		t.Fatalf("value: want %v, got %v", want.Value, got.Value)
	}
	if want.Start != got.Start {
		t.Fatalf("start: want %v, got %b", want.Start, got.Start)
	}
	if !bytes.Equal(want.Owner, got.Owner) {
		t.Fatalf("owner: want %v, got %v", want.Owner, got.Owner)
	}
	if want.Depth != got.Depth {
		t.Fatalf("depth: want %v, got %v", want.Depth, got.Depth)
	}
}

func compareChainState(t *testing.T, want, got *postage.ChainState) {
	t.Helper()

	if want.Block != got.Block {
		t.Fatalf("block: want %v, got %v", want.Block, got.Block)
	}
	if want.Price.Cmp(got.Price) != 0 {
		t.Fatalf("price: want %v, got %v", want.Price, got.Price)
	}
	if want.Total.Cmp(got.Total) != 0 {
		t.Fatalf("total: want %v, got %v", want.Total, got.Total)
	}
}
