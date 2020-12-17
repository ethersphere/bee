// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore_test

import (
	"bytes"
	crand "crypto/rand"
	"io"
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	"github.com/ethersphere/bee/pkg/statestore/mock"
)

func compareBatches(t *testing.T, want, got *postage.Batch) {
	t.Helper()

	if !bytes.Equal(want.ID, got.ID) {
		t.Fatalf("batch ID: want %v, got %v", want.ID, got.ID)
	}
	if want.Value.Uint64() != got.Value.Uint64() {
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

func TestBatchPutGet(t *testing.T) {
	bs := batchstore.New(mock.NewStateStore())

	want := newTestBatch(t)
	bs.Put(want)
	got, err := bs.Get(want.ID)
	if err != nil {
		t.Fatal(err)
	}
	compareBatches(t, want, got)

	// TODO: got -> mutate, persist, load, check correct values
}

// TODO: share this code between postage and batchstore tests?
func newTestBatch(t *testing.T) *postage.Batch {
	t.Helper()

	id := make([]byte, 32)
	_, err := io.ReadFull(crand.Reader, id)
	if err != nil {
		t.Fatal(err)
	}
	value64 := rand.Uint64()
	start64 := rand.Uint64()

	owner := make([]byte, 20)
	_, err = io.ReadFull(crand.Reader, owner)
	if err != nil {
		t.Fatal(err)
	}

	depth := uint8(16)

	return &postage.Batch{
		ID:    id,
		Value: (new(big.Int)).SetUint64(value64),
		Start: start64,
		Owner: owner,
		Depth: depth,
	}
}

// func TestStoreMarshalling(t *testing.T) {
// 	mockstore := mock.NewStateStore()
// 	store, err := batchstore.New(mockstore)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	newPrice := big.NewInt(99)
// 	store.UpdatePrice(newPrice)

// 	store.Settle(1100)

// 	store2, err := batchstore.New(mockstore)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	if store2.Price().Cmp(newPrice) != 0 {
// 		t.Fatal("price not persisted")
// 	}

// 	expTotal := big.NewInt(0 + 99*1100)
// 	if store2.Total().Cmp(expTotal) != 0 {
// 		t.Fatal("value mismatch")
// 	}
// }
