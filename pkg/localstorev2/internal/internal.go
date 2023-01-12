// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"bytes"
	"context"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemchunkstore"
	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/hashicorp/go-multierror"
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

// AddressOrZero returns swarm.ZeroAddress if the buf is of zero bytes. The Zero byte
// buffer is used by the items to serialize their contents and if valid swarm.ZeroAddress
// entries are allowed.
func AddressOrZero(buf []byte) swarm.Address {
	if bytes.Equal(buf, emptyAddr) {
		return swarm.ZeroAddress
	}
	return swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), buf...))
}

// AddressBytesOrZero is a helper which creates a zero buffer of swarm.HashSize. This
// is required during storing the items in the Store as their serialization formats
// are strict.
func AddressBytesOrZero(addr swarm.Address) []byte {
	if addr.IsZero() {
		return make([]byte, swarm.HashSize)
	}
	return addr.Bytes()
}

// NewInmemStorage constructs a inmem Storage implementation which can be used
// for the tests in the internal packages.
func NewInmemStorage() (Storage, func() error) {

	ctx, cancel := context.WithCancel(context.Background())

	ts := &inmemRepository{
		ctx:        ctx,
		indexStore: inmemstore.New(),
		chunkStore: &chunkStore{inmemchunkstore.New()},
	}

	return ts, func() error {
		cancel()
		return multierror.Append(ts.indexStore.Close(), ts.chunkStore.Close()).ErrorOrNil()
	}
}

type inmemRepository struct {
	ctx        context.Context
	indexStore storage.Store
	chunkStore *chunkStore
}

type chunkStore struct {
	storage.ChunkStore
}

// Once the inmemchunkstore supports this, we can remove this.
func (c *chunkStore) GetWithStamp(ctx context.Context, address swarm.Address, _ []byte) (swarm.Chunk, error) {
	return c.Get(ctx, address)
}

func (t *inmemRepository) Ctx() context.Context   { return t.ctx }
func (t *inmemRepository) Store() storage.Store   { return t.indexStore }
func (t *inmemRepository) ChunkStore() ChunkStore { return t.chunkStore }
