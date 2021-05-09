// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package shed provides a simple abstraction components to compose
// more complex operations on storage data organized in fields and indexes.
//
// Only type which holds logical information about swarm storage chunks data
// and metadata is Item. This part is not generalized mostly for
// performance reasons.
package shed

import (
	"bytes"
	"context"
	"errors"

	badger "github.com/dgraph-io/badger/v3"
	"github.com/ethersphere/bee/pkg/logging"
)

const (
	DefaultSyncWrites         = false  // Dont sync the writes to disk, instead delay it as a batch
	DefaultValueThreshold     = 1024   // Anything less than 1K value will be store with the LSM key itself
	DefaultValueLogMaxEntries = 500000 // Max number of entries in a value log
)

var (
	ErrNotFound = errors.New("database: not found")
)

// DB provides abstractions over badgerDB in order to
// implement complex structures using fields and ordered indexes.
// It provides a schema functionality to store fields and indexes
// information about naming and types.
type DB struct {
	path    string
	bdb     *badger.DB
	metrics metrics
	logger  logging.Logger
}

// NewDB opens the badger DB with options that make the DB useful for
// Chunk, State as well as Index stores
func NewDB(path string, logger logging.Logger) (db *DB, err error) {
	o := badger.DefaultOptions(path)
	o.SyncWrites = DefaultSyncWrites
	o.ValueThreshold = DefaultValueThreshold
	o.ValueLogMaxEntries = DefaultValueLogMaxEntries
	o.Logger = nil // Dont enable the badger logs

	if path == "" {
		o.InMemory = true
	}

	database, err := badger.Open(o)
	if err != nil {
		return nil, err
	}

	db = &DB{
		bdb:     database,
		metrics: newMetrics(),
		logger:  logger,
		path:    path,
	}

	if _, err = db.getSchema(); err != nil {
		if err == ErrNotFound {
			// save schema with initialized default fields
			if err = db.putSchema(schema{
				Fields:  make(map[string]fieldSpec),
				Indexes: make(map[byte]indexSpec),
			}); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return db, nil
}

// Put inserts the given key and value in to badger.
func (db *DB) Put(key []byte, value []byte) (err error) {
	return db.bdb.Update(func(txn *badger.Txn) (err error) {
		db.metrics.PutCounter.Inc()
		err = txn.Set(key, value)
		if err != nil {
			db.metrics.PutFailCounter.Inc()
			return err
		}
		return nil
	})

}

// Get retrieves the value given the key.
// if the key is not present a ErrNotFound is returned.
func (db *DB) Get(key []byte) (value []byte, err error) {
	err = db.bdb.View(func(txn *badger.Txn) (err error) {
		item, err := txn.Get(key)
		if err != nil {
			db.metrics.GetFailCounter.Inc()
			if err == badger.ErrKeyNotFound {
				db.metrics.GetNotFoundCounter.Inc()
				return ErrNotFound
			}
			db.metrics.GetFailCounter.Inc()
			return err
		}
		return item.Value(func(val []byte) error {
			value = val
			return nil
		})
	})
	if err == nil {
		db.metrics.GetCounter.Inc()
	}
	return value, err
}

// Has checks if the given key is present in the database.
// it returns a bool indicating true or false OR error if it encounters one during the operation.
func (db *DB) Has(key []byte) (yes bool, err error) {
	yes = false
	err = db.bdb.View(func(txn *badger.Txn) (err error) {
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {

				return nil
			}
			return err
		}
		return item.Value(func(val []byte) error {
			yes = true
			db.metrics.HasCounter.Inc()
			return nil
		})
	})
	if err != nil {
		db.metrics.HasFailCounter.Inc()
		return false, err
	}

	return yes, nil
}

// Delete removed the key and value if a given key is present in the DB.
func (db *DB) Delete(key []byte) (err error) {
	return db.bdb.Update(func(txn *badger.Txn) (err error) {
		db.metrics.DeleteCounter.Inc()
		err = txn.Delete(key)
		if err != nil {
			db.metrics.DeleteFailCounter.Inc()
		}
		return err
	})
}

// Count gives a count of all the keys present in the DB.
func (db *DB) Count(ctx context.Context) (count int, err error) {

	err = db.bdb.View(func(txn *badger.Txn) (err error) {
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
// a nil prefix acts like the total count of the DB
func (db *DB) CountPrefix(prefix []byte) (count int, err error) {

	err = db.bdb.View(func(txn *badger.Txn) (err error) {
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
func (db *DB) CountFrom(prefix []byte) (count int, err error) {

	err = db.bdb.View(func(txn *badger.Txn) (err error) {
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
func (db *DB) Iterate(startKey []byte, skipStartKey bool, reverse bool, fn func(key []byte, value []byte) (stop bool, err error)) (err error) {

	err = db.bdb.View(func(txn *badger.Txn) (err error) {
		o := badger.DefaultIteratorOptions
		o.PrefetchValues = true
		o.PrefetchSize = 1024
		o.Reverse = reverse
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

	return err
}

// First returns the first key which matches the given prefix.
func (db *DB) First(prefix []byte) (key []byte, value []byte, err error) {

	err = db.bdb.View(func(txn *badger.Txn) (err error) {
		o := badger.DefaultIteratorOptions
		o.PrefetchValues = true
		o.PrefetchSize = 1

		i := txn.NewIterator(o)
		defer i.Close()

		i.Seek(prefix)
		key = i.Item().Key()
		if !bytes.HasPrefix(key, prefix) {
			return ErrNotFound
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
func (db *DB) Last(prefix []byte) (key []byte, value []byte, err error) {

	err = db.bdb.View(func(txn *badger.Txn) (err error) {
		o := badger.DefaultIteratorOptions
		o.PrefetchValues = true
		o.PrefetchSize = 1024
		o.Reverse = true // iterate backwards

		i := txn.NewIterator(o)
		defer i.Close()

		// get the next prefix in line
		// since leveldb iterator Seek seeks to the
		// next key if the key that it seeks to is not found
		// and by getting the previous key, the last one for the
		// actual prefix is found
		nextPrefix := incByteSlice(prefix)
		l := len(prefix)

		if l > 0 && nextPrefix != nil {
			// If there is a no key which starts which nextPrefix, badger moves the
			// cursor to the previous key (which should be our key).
			i.Seek(nextPrefix)
			if bytes.HasPrefix(i.Item().Key(), prefix) {
				key = i.Item().Key()
				value, err = i.Item().ValueCopy(nil)
				if err != nil {
					return err
				}
			} else {
				// If there is a key which starts with nextPrefix, we do reverse Next() to
				// reach our key and pick that up.
				i.Next()
				if bytes.HasPrefix(i.Item().Key(), prefix) {
					key = i.Item().Key()
					value, err = i.Item().ValueCopy(nil)
					if err != nil {
						return err
					}
				}
			}
		}

		if key == nil {
			return ErrNotFound
		}
		return nil
	})

	return key, value, err
}

// GetBatch get a new badger transaction to be used for multiple atomic operations.
func (db *DB) GetBatch(update bool) (txn *badger.Txn) {

	// set update to true indicating that data will be added/changed in this transaction.
	return db.bdb.NewTransaction(update)
}

// WriteBatch commits the badger transaction after all the operations are over.
func (db *DB) WriteBatch(txn *badger.Txn) (err error) {
	db.metrics.WriteBatchCounter.Inc()
	err = txn.Commit()
	if err != nil {
		db.metrics.WriteBatchFailCounter.Inc()
		return err
	}
	return nil
}

// Close shuts down the badger DB.
func (db *DB) Close() (err error) {
	return db.bdb.Close()
}
