// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"bytes"
	"errors"

	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Storage groups the storage.Store and storage.ChunkStore interfaces.
type Storage interface {
	IndexStore() storage.Store
	ChunkStore() storage.ChunkStore
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
	ts := &inmemRepository{
		indexStore: inmemstore.New(),
		chunkStore: inmemchunkstore.New(),
	}

	return ts, func() error {
		return errors.Join(ts.indexStore.Close(), ts.chunkStore.Close())
	}
}

type inmemRepository struct {
	indexStore storage.Store
	chunkStore storage.ChunkStore
}

func (t *inmemRepository) IndexStore() storage.Store      { return t.indexStore }
func (t *inmemRepository) ChunkStore() storage.ChunkStore { return t.chunkStore }
