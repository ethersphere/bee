// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package disk

import (
	"context"
	"github.com/dgraph-io/badger"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/prometheus/client_golang/prometheus"
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

func (d *diskStore) Delete(ctx context.Context,addr swarm.Address) (err error) {
	return d.db.Update(func(txn *badger.Txn) (err error) {
		return txn.Delete(addr.Bytes())
	})
}

func (d *diskStore) Count(ctx context.Context) (count int, err error) {
	err = d.db.View(func(txn *badger.Txn) (err error) {
		o := badger.DefaultIteratorOptions
		o.PrefetchValues = false
		i := txn.NewIterator(o)
		defer i.Close()
		for i.Rewind(); i.Valid(); i.Next() {
			item := i.Item()
			k := item.KeySize()
			if k < 1 {
				continue
			}
			count++
		}
		return nil
	})
	return count, err
}

func (d *diskStore) Iterate(fn func(chunk swarm.Chunk) (stop bool, err error)) (err error) {
	return d.db.View(func(txn *badger.Txn) (err error) {
		o := badger.DefaultIteratorOptions
		o.PrefetchValues = true
		o.PrefetchSize = 1024
		i := txn.NewIterator(o)
		defer i.Close()
		for i.Rewind(); i.Valid(); i.Next() {
			item := i.Item()
			k := item.Key()
			if len(k) < 1 {
				continue
			}
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			if len(v) == 0 {
				continue
			}
			stop, err := fn(swarm.NewChunk(swarm.NewAddress(k), v))
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
		}
		return nil
	})
}

func (d *diskStore) Close(ctx context.Context) (err error) {
	return d.db.Close()
}