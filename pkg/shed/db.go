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
	"fmt"

	"github.com/dgraph-io/badger"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
)

const (
	DefaultSyncWrites         = false  // Dont sync the writes to disk, instead delay it as a batch
	DefaultValueThreshold     = 1024   // Anything less than 1K value will be store with the LSM key itself
	DefaultValueLogMaxEntries = 500000 // Max number of entries in a value log
)

var (
	ErrNotFound       = errors.New("storage: not found")
	ErrNotImplemented = errors.New("storage: not implemented")
	ErrInvalidChunk   = errors.New("storage: invalid chunk")
)

// DB provides abstractions over badgerDB in order to
// implement complex structures using fields and ordered indexes.
// It provides a schema functionality to store fields and indexes
// information about naming and types.
type DB struct {
	path      string
	bdb       *badger.DB
	validator storage.ChunkValidatorFunc
	metrics   metrics
	logger    logging.Logger
}

// NewDB opens the badger DB with options that make the DB useful for
// Chunk, State as well as Index stores
func NewDB(path string, logger logging.Logger) (db *DB, err error) {
	o := badger.DefaultOptions(path)
	o.SyncWrites = DefaultSyncWrites
	o.ValueThreshold = DefaultValueThreshold
	o.ValueLogMaxEntries = DefaultValueLogMaxEntries
	o.Logger = nil // Dont enable the badger logs
	_db, err := badger.Open(o)
	if err != nil {
		fmt.Printf("could not open database. Error: %v", err.Error())
		return nil, err
	}

	db = &DB{
		bdb:     _db,
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

	db.metrics.DBOpenCount.Inc()
	logger.Debugf("Opened DB with path : %s, valueThreshold : %d, valueLogMaxEntries : %d",
		path, DefaultValueThreshold, DefaultValueLogMaxEntries)
	return db, nil
}

// Put inserts the given key and value in to badger.
func (db *DB) Put(key []byte, value []byte) (err error) {
	db.logger.Tracef("Put value len : %d for key %s", len(value), string(key))
	return db.bdb.Update(func(txn *badger.Txn) (err error) {
		db.metrics.PutCount.Inc()
		err = txn.Set(key, value)
		if err != nil {
			db.logger.Errorf("Could not insert chunk with key %s in DB", string(key))
			db.metrics.PutFailCount.Inc()
			return err
		}
		db.logger.Tracef(" put success with key %s and value len %d", string(key), len(value))
		return nil
	})
}

// Get retrieves the value given the key.
// if the key is not present a ErrNotFound is returned.
func (db *DB) Get(key []byte) (value []byte, err error) {
	db.logger.Tracef("Get value for key %s", string(key))
	err = db.bdb.View(func(txn *badger.Txn) (err error) {
		item, err := txn.Get(key)
		if err != nil {
			db.metrics.GetFailCount.Inc()
			if err == badger.ErrKeyNotFound {
				db.metrics.GetNotFoundCount.Inc()
				return ErrNotFound
			}
			db.metrics.GetFailCount.Inc()
			return err
		}
		return item.Value(func(val []byte) error {
			value = val
			db.logger.Tracef("got valeu for key %s with length %d", string(key), len(val))
			return nil
		})
	})
	if err != nil {
		db.logger.Errorf("key %s not found in DB", string(key))
		db.logger.Debug("key %s not found in DB. Error : %v", string(key), err.Error())
	} else {
		db.logger.Tracef("got value with len : %d for key : %s", len(value), string(key))
		db.metrics.GetCount.Inc()
	}

	return value, err
}

// Has checks if the given key is present in the database.
// it returns a bool indicating true or false OR error if it encounters one during the operation.
func (db *DB) Has(key []byte) (yes bool, err error) {
	db.logger.Tracef("Has value with key %s", string(key))
	yes = false
	err = db.bdb.View(func(txn *badger.Txn) (err error) {
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return ErrNotFound
			}
			return err
		}
		return item.Value(func(val []byte) error {
			yes = true
			db.metrics.HasCount.Inc()
			return nil
		})
	})
	if err != nil {
		db.logger.Errorf("key %s not found in DB", string(key))
		db.logger.Debugf("key %s not found in DB. Error: %v", string(key), err.Error())
		db.metrics.HasFailCount.Inc()
	}

	if yes {
		db.logger.Tracef("found key %s", string(key))
	} else {
		db.logger.Tracef("could not found key %s", string(key))
	}

	return yes, nil
}

// Delete removed the key and value if a given key is present in the DB.
func (db *DB) Delete(key []byte) (err error) {
	db.logger.Tracef("deleting key %s along with value", string(key))
	return db.bdb.Update(func(txn *badger.Txn) (err error) {
		db.metrics.DeleteCount.Inc()
		err = txn.Delete(key)
		if err != nil {
			db.logger.Errorf("could not delete key %s from DB", string(key))
			db.logger.Debugf("could not delete key %s from DB. Error: %v", string(key), err.Error())
			db.metrics.DeleteFailCount.Inc()
		} else {
			db.logger.Tracef("deleted key %s", string(key))
		}
		return err
	})
}

// Count gives a count of all the keys present in the DB.
func (db *DB) Count(ctx context.Context) (count int, err error) {
	db.logger.Tracef("counting all keys")
	db.metrics.TotalCount.Inc()
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
	if err != nil {
		db.logger.Errorf("error while counting keys in DB")
		db.logger.Debugf("error while doing count. Error : %w", err.Error())
		db.metrics.TotalFailCount.Inc()
	}
	db.logger.Tracef("Total count is %d", count)
	return count, err
}

// CountPrefix gives a count of all the keys that starts with a given key prefix.
// a nil prefix acts like the total count of the DB
func (db *DB) CountPrefix(prefix []byte) (count int, err error) {
	if prefix != nil {
		db.logger.Tracef("counting all keys with prefix %s", string(prefix))
	}
	db.metrics.CountPrefixCount.Inc()
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
	if err != nil {
		db.logger.Errorf("error while doing count. prefix : %v,", string(prefix))
		db.logger.Debugf("error while doing count for prefix : %v, Error : %w", string(prefix), err.Error())
		db.metrics.CountPrefixFailCount.Inc()
	}
	db.logger.Tracef("Total count for prefix %s is %d", string(prefix), count)
	return count, err
}

// CountFrom gives a count of all the keys that start from a given prefix till the end of the DB.
func (db *DB) CountFrom(prefix []byte) (count int, err error) {
	if prefix != nil {
		db.logger.Tracef("counting all keys from prefix %s", string(prefix))
	}

	db.metrics.CountFromCount.Inc()
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
	if err != nil {
		db.logger.Errorf("error doing count from. prefix : %v", string(prefix))
		db.logger.Debugf("error while doing count from. prefix : %v, Error : %w", string(prefix), err.Error())
		db.metrics.CountPrefixFailCount.Inc()
	}
	db.logger.Tracef("Total count from prefix %s is %d", string(prefix), count)
	return count, err
}

// Iterate goes through the entries in the DB starting from the startKey and executing a
// given function to see if it needs to stop the iteration or not. The skipStartKey indicates
// weather to skip the first key or not.
func (db *DB) Iterate(startKey []byte, skipStartKey bool, fn func(key []byte, value []byte) (stop bool, err error)) (err error) {
	if startKey != nil {
		db.logger.Tracef("Iterating with startKey %s and skipping key is %t", string(startKey), skipStartKey)
	}

	db.metrics.IterationCount.Inc()
	err = db.bdb.View(func(txn *badger.Txn) (err error) {
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
	if err != nil {
		db.logger.Errorf("error while doing iteration. startKey : %v, skipStartKey: %t",
			string(startKey), skipStartKey)
		db.logger.Debugf("error while doing iteration. startKey : %v, skipStartKey: %t, Error : %w",
			string(startKey), skipStartKey, err.Error())
		db.metrics.IterationFailCount.Inc()
	}
	return err
}

// First returns the first key which matches the given prefix.
func (db *DB) First(prefix []byte) (key []byte, value []byte, err error) {
	if prefix != nil {
		db.logger.Tracef("Finding first key with prefix %s", string(prefix))
	}
	db.metrics.FirstCount.Inc()
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
	if err != nil {
		db.logger.Errorf("error while finding first key with prefix : %v, Error : %w", string(prefix), err.Error())
		db.logger.Debugf("error while doing first. prefix : %v, Error : %w", string(prefix), err.Error())
		db.metrics.FirstFailCount.Inc()
	} else {
		db.logger.Tracef("first value with prefix %s, key %s, value len %d", string(prefix), string(key), len(value))
	}

	return key, value, err
}

// Last retuns the last key matching the given prefix.
func (db *DB) Last(prefix []byte) (key []byte, value []byte, err error) {
	if prefix != nil {
		db.logger.Tracef("Finding last key with prefix %s", string(prefix))
	}
	db.metrics.LastCount.Inc()
	err = db.bdb.View(func(txn *badger.Txn) (err error) {
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
			return ErrNotFound
		}

		if !bytes.HasPrefix(key, prefix) {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		db.logger.Errorf("error while finding last value for prefix : %v", string(prefix))
		db.logger.Debugf("error while doing last. prefix : %v, Error : %w", string(prefix), err.Error())
		db.metrics.LastFailCount.Inc()
	} else {
		db.logger.Tracef("last value with prefix %s, key %s, value len %d", string(prefix), string(key), len(value))
	}
	return key, value, err
}

// GetBatch get a new badger transaction to be used for multiple atomic operations.
func (db *DB) GetBatch(update bool) (txn *badger.Txn) {
	db.logger.Tracef("getting a transaction with update %t", update)
	db.metrics.GetBatchCount.Inc()
	// set update to true indicating that data will be added/changed in this transaction.
	return db.bdb.NewTransaction(update)
}

// WriteBatch commits the badger transaction after all the operations are over.
func (db *DB) WriteBatch(txn *badger.Txn) (err error) {
	db.metrics.WriteBatchCount.Inc()
	err = txn.Commit()
	if err != nil {
		db.logger.Errorf("error while committing transaction. Error : %w", err.Error())
		db.logger.Debugf("error while committing batch. Error : %w", err.Error())
		db.metrics.WriteBatchFailCount.Inc()
		return err
	}
	txn.Discard()
	db.logger.Tracef("transaction committed successfully")
	return nil
}

// Close shut's down the badger DB.
func (db *DB) Close(ctx context.Context) (err error) {
	db.logger.Tracef("database closed with path %s", db.path)
	db.metrics.DBCloseCount.Inc()
	return db.bdb.Close()
}
