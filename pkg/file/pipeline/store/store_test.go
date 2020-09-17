// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package store_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/file/pipeline"
	mock "github.com/ethersphere/bee/pkg/file/pipeline/mock"
	"github.com/ethersphere/bee/pkg/file/pipeline/store"
	"github.com/ethersphere/bee/pkg/storage"
	storer "github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	addr = []byte{0xaa, 0xbb, 0xcc}
	data = []byte("hello world")
)

// TestStoreWriter tests that store writer stores the provided data and calls the next chain writer.
func TestStoreWriter(t *testing.T) {
	mockStore := storer.NewStorer()
	mockChainWriter := mock.NewChainWriter()
	ctx := context.Background()
	writer := store.NewStoreWriter(ctx, mockStore, storage.ModePutUpload, mockChainWriter)

	args := pipeline.PipeWriteArgs{Ref: addr, Data: data}
	err := writer.ChainWrite(&args)
	if err != nil {
		t.Fatal(err)
	}

	d, err := mockStore.Get(ctx, storage.ModeGetRequest, swarm.NewAddress(addr))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, d.Data()) {
		t.Fatal("data mismatch")
	}
	if calls := mockChainWriter.ChainWriteCalls(); calls != 1 {
		t.Errorf("wanted 1 ChainWrite call, got %d", calls)
	}
}

// TestSum tests that calling Sum on the store writer results in Sum on the next writer in the chain.
func TestSum(t *testing.T) {
	mockChainWriter := mock.NewChainWriter()
	ctx := context.Background()
	writer := store.NewStoreWriter(ctx, nil, storage.ModePutUpload, mockChainWriter)
	_, err := writer.Sum()
	if err != nil {
		t.Fatal(err)
	}
	if calls := mockChainWriter.SumCalls(); calls != 1 {
		t.Fatalf("wanted 1 Sum call but got %d", calls)
	}
}
