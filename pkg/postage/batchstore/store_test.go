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

	want, err := postagetest.NewBatch()
	if err != nil {
		t.Fatal(err)
	}

	err = bs.Put(want)
	if err != nil {
		t.Fatal(err)
	}

	got, err := bs.Get(want.ID)
	if err != nil {
		t.Fatal(err)
	}

	compareBatches(t, want, got)

	// TODO: got -> mutate, persist, load, check correct values
}
