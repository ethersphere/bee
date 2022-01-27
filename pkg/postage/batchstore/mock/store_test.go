// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
)

func TestBatchStore(t *testing.T) {
	const testCnt = 3

	testBatch := postagetesting.MustNewBatch()
	batchStore := mock.New(
		mock.WithGetErr(errors.New("fails"), testCnt),
		mock.WithUpdateErr(errors.New("fails"), testCnt),
	)

	if err := batchStore.Create(testBatch, big.NewInt(0), 0); err != nil {
		t.Fatal("unexpected error")
	}

	// Update should return error after a number of tries:
	for i := 0; i < testCnt; i++ {
		if err := batchStore.Update(testBatch, big.NewInt(0), 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := batchStore.Update(testBatch, big.NewInt(0), 0); err == nil {
		t.Fatal("expected error")
	}

	// Get should fail on wrong id, and after a number of tries:
	if _, err := batchStore.Get(postagetesting.MustNewID()); err == nil {
		t.Fatal("expected error")
	}
	for i := 0; i < testCnt-1; i++ {
		if _, err := batchStore.Get(testBatch.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := batchStore.Get(postagetesting.MustNewID()); err == nil {
		t.Fatal("expected error")
	}
}

func TestBatchStorePutChainState(t *testing.T) {
	const testCnt = 3

	testChainState := postagetesting.NewChainState()
	batchStore := mock.New(
		mock.WithChainState(testChainState),
		mock.WithUpdateErr(errors.New("fails"), testCnt),
	)

	// PutChainState should return an error after a number of tries:
	for i := 0; i < testCnt; i++ {
		if err := batchStore.PutChainState(testChainState); err != nil {
			t.Fatal(err)
		}
	}
	if err := batchStore.PutChainState(testChainState); err == nil {
		t.Fatal("expected error")
	}
}

func TestBatchStoreWithBatch(t *testing.T) {
	testBatch := postagetesting.MustNewBatch()
	batchStore := mock.New(
		mock.WithBatch(testBatch),
	)

	b, err := batchStore.Get(testBatch.ID)
	if err != nil {
		t.Fatal(err)
	}

	postagetesting.CompareBatches(t, testBatch, b)
}
