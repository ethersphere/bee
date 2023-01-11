// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"bytes"
	"context"
	"testing"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemchunkstore"
	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Storage groups the storage.Store and storage.ChunkStore interfaces with context..
type Storage interface {
	Ctx() context.Context
	Store() storage.Store
	ChunkStore() ChunkStore
}

type ChunkStore interface {
	storage.ChunkStore
	storage.GetterWithStamp
}

// PutterCloserWithReference provides a Putter which can be closed with a root
// swarm reference associated with this session.
type PutterCloserWithReference interface {
	storage.Putter
	Close(swarm.Address) error
}

var emptyAddr = make([]byte, swarm.HashSize)

func AddressOrZero(buf []byte) swarm.Address {
	if bytes.Equal(buf, emptyAddr) {
		return swarm.ZeroAddress
	}
	return swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), buf...))
}

func AddressBytesOrZero(addr swarm.Address) []byte {
	if addr.IsZero() {
		return make([]byte, swarm.HashSize)
	}
	return addr.Bytes()
}

func NewTestStorage(t *testing.T) Storage {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ts := &testStorage{
		ctx:        ctx,
		indexStore: inmemstore.New(),
		chunkStore: &chunkStore{inmemchunkstore.New()},
	}
	t.Cleanup(func() {
		if err := ts.Store().Close(); err != nil {
			t.Errorf("Storage().Close(): unexpected error: %v", err)
		}
		if err := ts.ChunkStore().Close(); err != nil {
			t.Errorf("ChunkStore().Close(): unexpected error: %v", err)
		}
	})

	return ts
}

type testStorage struct {
	ctx        context.Context
	indexStore storage.Store
	chunkStore *chunkStore
}

type chunkStore struct {
	storage.ChunkStore
}

func (c *chunkStore) GetWithStamp(ctx context.Context, address swarm.Address, _ []byte) (swarm.Chunk, error) {
	return c.Get(ctx, address)
}

func (c *chunkStore) DeleteWithStamp(ctx context.Context, address swarm.Address, _ []byte) error {
	return c.Delete(ctx, address)
}

func (t *testStorage) Ctx() context.Context   { return t.ctx }
func (t *testStorage) Store() storage.Store   { return t.indexStore }
func (t *testStorage) ChunkStore() ChunkStore { return t.chunkStore }
