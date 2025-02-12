// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"bytes"
	"context"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// PutterCloserWithReference provides a Putter which can be closed with a root
// swarm reference associated with this session.
type PutterCloserWithReference interface {
	Put(context.Context, transaction.Store, swarm.Chunk) error
	Close(storage.IndexStore, swarm.Address) error
	Cleanup(transaction.Storage) error
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
func NewInmemStorage() transaction.Storage {
	ts := &inmemStorage{
		indexStore: inmemstore.New(),
		chunkStore: inmemchunkstore.New(),
	}

	return ts
}

type inmemStorage struct {
	indexStore storage.IndexStore
	chunkStore storage.ChunkStore
}

func (t *inmemStorage) NewTransaction(ctx context.Context) (transaction.Transaction, func()) {
	return &inmemTrx{t.indexStore, t.chunkStore}, func() {}
}

type inmemTrx struct {
	indexStore storage.IndexStore
	chunkStore storage.ChunkStore
}

func (t *inmemStorage) IndexStore() storage.Reader             { return t.indexStore }
func (t *inmemStorage) ChunkStore() storage.ReadOnlyChunkStore { return t.chunkStore }

func (t *inmemTrx) IndexStore() storage.IndexStore { return t.indexStore }
func (t *inmemTrx) ChunkStore() storage.ChunkStore { return t.chunkStore }
func (t *inmemTrx) Commit() error                  { return nil }

func (t *inmemStorage) Close() error { return nil }
func (t *inmemStorage) Run(ctx context.Context, f func(s transaction.Store) error) error {
	trx, done := t.NewTransaction(ctx)
	defer done()
	return f(trx)
}
