// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package disk

import (
	"bytes"
	"context"
	"github.com/dgraph-io/badger"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// DiskStorer stores the handle for badger DB.
type DiskStore struct {
	db *badger.DB
	validator storage.ChunkValidatorFunc
}

//NewDiskStorer opens the badger DB with options that make the DB useful for
// Chunk, State as well as Index stores
func NewDiskStorer(path string, v storage.ChunkValidatorFunc) (store *DiskStore, err error) {
	o := badger.DefaultOptions(path)
	o.SyncWrites = false         // Dont sync the writes to disk, instead delay it as a batch
	o.ValueThreshold = 1024      // Anything less than 1K value will be store with the key
	o.ValueLogMaxEntries = 50000 // Max number of entries in a value log
	o.Logger = nil               // DOnt enable the badger logs
	_db, err := badger.Open(o)
	if err != nil {
		return nil, err
	}

	ds := &DiskStore{db: _db, validator: v}
	return ds, nil
}

// Get retrieves the value given the key.
// if the key is not present a storage.ErrNotFound is returned.
func (d *DiskStore) Get(ctx context.Context, key []byte) (value []byte, err error) {
	err = d.db.View(func(txn *badger.Txn) (err error) {
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return storage.ErrNotFound
			}
			return err
		}
		return item.Value(func(val []byte) error {
			value = val
			return nil
		})
	})
	return value, err
}

// Put inserts the given key and value in to badger.
func (d *DiskStore) Put(ctx context.Context, key []byte, value []byte) (err error) {
	if d.validator != nil {
		ch := swarm.NewChunk(swarm.NewAddress(key), swarm.NewData(value))
		if !d.validator(ch) {
			return storage.ErrInvalidChunk
		}
	}
	return d.db.Update(func(txn *badger.Txn) (err error) {
		err = txn.Set(key, value)
		return err
	})
}

// Has checks if the given key is present in the database.
// it returns a bool indicating true or false OR error if it encounters one during the operation.
func (d *DiskStore) Has(ctx context.Context, key []byte) (yes bool, err error) {
	yes = false
	err = d.db.View(func(txn *badger.Txn) (err error) {
		item, err := txn.Get(key)
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
	return yes, nil
}

// Delete removed the key and value if a given key is present in the DB.
func (d *DiskStore) Delete(ctx context.Context, key []byte) (err error) {
	return d.db.Update(func(txn *badger.Txn) (err error) {
		return txn.Delete(key)
	})
}

// Count gives a count of all the keys present in the DB.
func (d *DiskStore) Count(ctx context.Context) (count int, err error) {
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

// CountPrefix gives a count of all the keys that starts with a given key prefix.
func (d *DiskStore) CountPrefix(prefix []byte) (count int, err error) {
	err = d.db.View(func(txn *badger.Txn) (err error) {
		o := badger.DefaultIteratorOptions
		o.PrefetchValues = false
		o.PrefetchSize = 1024
		i := txn.NewIterator(o)
		defer i.Close()

		// if prefix is nil, it is equivalent to counting from beginning
		for i.Seek(prefix); i.ValidForPrefix(prefix); i.Next() {
			item := i.Item()
			k := item.Key()
			if prefix != nil {
				if !bytes.HasPrefix(k, prefix) {
					break
				}
			}
			count++
		}
		return nil
	})
	return count, err
}

// CountFrom gives a count of all the keys that start from a given prefix till the end of the DB.
func (d *DiskStore) CountFrom(prefix []byte) (count int, err error) {
	err = d.db.View(func(txn *badger.Txn) (err error) {
		o := badger.DefaultIteratorOptions
		o.PrefetchValues = false
		o.PrefetchSize = 1024
		i := txn.NewIterator(o)
		defer i.Close()

		// if prefix is nil, it is equivalent to counting from beginning
		for i.Seek(prefix); i.Valid(); i.Next() {
			count++
		}
		return nil
	})
	return count, err
}

// Iterate goes through the entries in the DB starting from the startKey and executing a
// given function to see if it needs to stop the iteration or not. The skipStartKey indicates
// weather to skip the first key or not.
func (d *DiskStore) Iterate(startKey []byte, skipStartKey bool, fn func(key []byte, value []byte) (stop bool, err error)) (err error) {
	return d.db.View(func(txn *badger.Txn) (err error) {
		o := badger.DefaultIteratorOptions
		o.PrefetchValues = true
		o.PrefetchSize = 1024
		i := txn.NewIterator(o)
		defer i.Close()

		i.Seek(startKey)
		if !i.Valid() {
			return nil
		}

		if skipStartKey {
			i.Next()
		}

		for ; i.Valid(); i.Next() {
			item := i.Item()
			k := item.Key()
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			stop, err := fn(k, v)
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

// First returns the first key which matches the given prefix.
func (d *DiskStore) First(prefix []byte) (key []byte, value []byte, err error) {
	err = d.db.View(func(txn *badger.Txn) (err error) {
		o := badger.DefaultIteratorOptions
		o.PrefetchValues = true
		o.PrefetchSize = 1

		i := txn.NewIterator(o)
		defer i.Close()

		i.Seek(prefix)
		key = i.Item().Key()
		if !bytes.HasPrefix(key, prefix) {
			return storage.ErrNotFound
		}
		value, err = i.Item().ValueCopy(value)
		if err != nil {
			return err
		}
		return nil
	})
	return key, value, err
}

// Last retuns the last key matching the given prefix.
func (d *DiskStore) Last(prefix []byte) (key []byte, value []byte, err error) {
	err = d.db.View(func(txn *badger.Txn) (err error) {
		o := badger.DefaultIteratorOptions
		o.PrefetchValues = true
		o.PrefetchSize = 1024

		i := txn.NewIterator(o)
		defer i.Close()

		for i.Seek(prefix); i.ValidForPrefix(prefix); i.Next() {
			key = i.Item().Key()
			value, err = i.Item().ValueCopy(value)
			if err != nil {
				return err
			}
		}

		if key == nil {
			return storage.ErrNotFound
		}

		if !bytes.HasPrefix(key, prefix) {
			return storage.ErrNotFound
		}
		return nil
	})
	return key, value, err
}

// GetBatch get a new badger transaction to be used for multiple atomic operations.
func (d *DiskStore) GetBatch(update bool) (txn *badger.Txn) {
	// set update to true indicating that data will be added/changed in this transaction.
	return d.db.NewTransaction(true)
}

// WriteBatch commits the badger transaction after all the operations are over.
func (d *DiskStore) WriteBatch(txn *badger.Txn) (err error) {
	err = txn.Commit()
	if err != nil {
		return err
	}
	txn.Discard()
	return nil
}

// Close shut's down the badger DB.
func (d *DiskStore) Close(ctx context.Context) (err error) {
	return d.db.Close()
}
