// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package store_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	mock "github.com/ethersphere/bee/v2/pkg/file/pipeline/mock"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/store"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestStoreWriter tests that store writer stores the provided data and calls the next chain writer.
func TestStoreWriter(t *testing.T) {
	t.Parallel()

	mockStore := inmemchunkstore.New()
	mockChainWriter := mock.NewChainWriter()
	ctx := context.Background()
	writer := store.NewStoreWriter(ctx, mockStore, mockChainWriter)

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
		{},
	} {
		args := pipeline.PipeWriteArgs{Ref: tc.ref, Data: tc.data}
		err := writer.ChainWrite(&args)

		if err != nil && tc.expErr != nil && errors.Is(err, tc.expErr) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}

		d, err := mockStore.Get(ctx, swarm.NewAddress(tc.ref))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(tc.data, d.Data()) {
			t.Fatal("data mismatch")
		}
		if calls := mockChainWriter.ChainWriteCalls(); calls != 1 {
			t.Errorf("wanted 1 ChainWrite call, got %d", calls)
		}
	}
}

// TestSum tests that calling Sum on the store writer results in Sum on the next writer in the chain.
func TestSum(t *testing.T) {
	t.Parallel()

	mockChainWriter := mock.NewChainWriter()
	ctx := context.Background()
	writer := store.NewStoreWriter(ctx, nil, mockChainWriter)
	_, err := writer.Sum()
	if err != nil {
		t.Fatal(err)
	}
	if calls := mockChainWriter.SumCalls(); calls != 1 {
		t.Fatalf("wanted 1 Sum call but got %d", calls)
	}
}

// stampingPutter mimics the api putterSessionWrapper: it attaches a stamp to
// the chunk object it is given (WithStamp mutates the chunk in place).
type stampingPutter struct {
	storage.Putter
	stamp swarm.Stamp
}

func (p *stampingPutter) Put(ctx context.Context, ch swarm.Chunk) error {
	return p.Putter.Put(ctx, ch.WithStamp(p.stamp))
}

// TestStoreWriterSurfacesStamp checks that the store writer copies the stamp
// attached by the putter into the pipe write args.
func TestStoreWriterSurfacesStamp(t *testing.T) {
	t.Parallel()

	stamp := postagetesting.MustNewStamp()
	want, err := stamp.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	p := &stampingPutter{Putter: inmemchunkstore.New(), stamp: stamp}
	writer := store.NewStoreWriter(context.Background(), p, nil)

	ch, err := cac.New([]byte("hello world"))
	if err != nil {
		t.Fatal(err)
	}
	args := pipeline.PipeWriteArgs{Ref: ch.Address().Bytes(), Data: ch.Data()}
	if err := writer.ChainWrite(&args); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(args.Stamp, want) {
		t.Fatalf("stamp not surfaced in pipe write args")
	}
}
