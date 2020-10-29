// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package push_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/file/pipeline"
	mock "github.com/ethersphere/bee/pkg/file/pipeline/mock"
	"github.com/ethersphere/bee/pkg/file/pipeline/store"
	"github.com/ethersphere/bee/pkg/storage"
	storer "github.com/ethersphere/bee/pkg/storage/mock"
)

// TestStoreWriter tests that store writer stores the provided data and calls the next chain writer.
func TestStoreWriter(t *testing.T) {
	mockStore := storer.NewStorer()
	mockChainWriter := mock.NewChainWriter()
	ctx := context.Background()
	writer := store.NewStoreWriter(ctx, mockStore, storage.ModePutUpload, mockChainWriter)

	for _, tc := range []struct {
		name   string
		ref    []byte
		data   []byte
		expErr error
	}{
		{
			name:   "no data",
			expErr: store.ErrInvalidData,
		},
		{
			name: "some data",
			ref:  []byte{0xaa, 0xbb, 0xcc},
			data: []byte("hello world"),
		},
	} {
		args := pipeline.PipeWriteArgs{Ref: tc.ref, Data: tc.data}
		err := writer.ChainWrite(&args)

		if err != nil && tc.expErr != nil && errors.Is(err, tc.expErr) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}

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
