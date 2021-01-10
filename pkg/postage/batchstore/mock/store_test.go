// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock_test

import (
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
)

func TestBatchStoreGet(t *testing.T) {
	const testCnt = 3

	testBatch := postagetesting.MustNewBatch()
	batchStore := mock.New(
		mock.WithGetErr(errors.New("fails"), 3),
	)
	if err := batchStore.Put(testBatch); err != nil {
		t.Fatal(err)
	}

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

func TestBatchStorePut(t *testing.T) {
	const testCnt = 3

	testBatch := postagetesting.MustNewBatch()
	batchStore := mock.New(
		mock.WithPutErr(errors.New("fails"), 3),
	)

	for i := 0; i < testCnt; i++ {
		if err := batchStore.Put(testBatch); err != nil {
			t.Fatal(err)
		}
	}

	if err := batchStore.Put(testBatch); err == nil {
		t.Fatal("expected error")
	}
}
