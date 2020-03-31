// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package disk

import (
	"context"
	"github.com/dgraph-io/badger"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type diskStore struct {
	db *badger.DB
}

func NewDiskStorer(path string) (store *diskStore, err error) {
	o := badger.DefaultOptions(path)
	o.SyncWrites = false
	o.ValueLogMaxEntries = 50000
	o.NumMemtables = 10
	o.Logger = nil
	_db, err := badger.Open(o)
	if err != nil {
		return nil, err
	}
	return &diskStore{
		db: _db,
	}, nil
}

func (d *diskStore) Get(ctx context.Context, addr swarm.Address) (chunk swarm.Chunk, err error) {
	err = d.db.View(func(txn *badger.Txn) (err error) {
		item, err := txn.Get(addr.Bytes())
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return storage.ErrNotFound
			}
			return err
		}
		return item.Value(func(val []byte) error {
			chunk = swarm.NewChunk(addr, val)
			return nil
		})
	})
	return chunk, err
}

func (d *diskStore) Put(ctx context.Context, chunk swarm.Chunk) error {
	return d.db.Update(func(txn *badger.Txn) (err error) {
		err = txn.Set(chunk.Address().Bytes(), chunk.Data())
		return err
	})
}

func (d *diskStore) Has(ctx context.Context, addr swarm.Address) (yes bool, err error) {
	yes = false
	err = d.db.View(func(txn *badger.Txn) (err error) {
		item, err := txn.Get(addr.Bytes())
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return storage.ErrNotFound
			}
			return err
		}
		return item.Value(func(val []byte) error {
			yes = true
			return nil
		})
	})
	return yes, err
}

func (d *diskStore) Close() (err error) {
	return d.db.Close()
}