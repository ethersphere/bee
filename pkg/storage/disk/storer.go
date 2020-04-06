// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package disk

import (
	"bytes"
	"context"
	"fmt"
	"github.com/dgraph-io/badger"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	DefaultSyncWrites         = false  // Dont sync the writes to disk, instead delay it as a batch
	DefaultValueThreshold     = 1024   // Anything less than 1K value will be store with the LSM key itself
	DefaultValueLogMaxEntries = 500000 // Max number of entries in a value log
)

// DiskStorer stores the handle for badger DB.
type DiskStore struct {
	path      string
	db        *badger.DB
	validator storage.ChunkValidatorFunc
	metrics   metrics
	logger    logging.Logger
}

// NewDiskStorer opens the badger DB with options that make the DB useful for
// Chunk, State as well as Index stores
func NewDiskStorer(path string, v storage.ChunkValidatorFunc, logger logging.Logger) (store *DiskStore, err error) {
	o := badger.DefaultOptions(path)
	o.SyncWrites = DefaultSyncWrites
	o.ValueThreshold = DefaultValueThreshold
	o.ValueLogMaxEntries = DefaultValueLogMaxEntries
	o.Logger = nil // Dont enable the badger logs
	_db, err := badger.Open(o)
	if err != nil {
		fmt.Errorf("Could not open database. Error: %v", err.Error())
		return nil, err
	}
	ds := &DiskStore{
		path:      path,
		db:        _db,
		validator: v,
		metrics:   newMetrics(),
		logger:    logger,
	}

	ds.metrics.DBOpenCount.Inc()
	logger.Debugf("Opened DB with path : %s, valueThreshold : %d, valueLogMaxEntries : %d",
		path, DefaultValueThreshold, DefaultValueLogMaxEntries)
	return ds, nil
}

// Get retrieves the value given the key.
// if the key is not present a storage.ErrNotFound is returned.
func (d *DiskStore) Get(ctx context.Context, key []byte) (value []byte, err error) {
	d.logger.Tracef("Get value for key %s", string(key))
	err = d.db.View(func(txn *badger.Txn) (err error) {
		item, err := txn.Get(key)
		if err != nil {
			d.metrics.DiskGetFailCount.Inc()
			if err == badger.ErrKeyNotFound {
				return storage.ErrNotFound
			}
			return err
		}
		return item.Value(func(val []byte) error {
			value = val
			d.logger.Tracef("got valeu for key %s with length %d", string(key), len(val))
			return nil
		})
	})
	if err != nil {
		d.logger.Errorf("key %s not found in DB", string(key))
		d.logger.Debug("key %s not found in DB. Error : %v", string(key), err.Error())
	} else {
		d.logger.Tracef("got value with len : %d for key : %s", len(value), string(key))
		d.metrics.DiskGetCount.Inc()
	}

	return value, err
}

// Put inserts the given key and value in to badger.
func (d *DiskStore) Put(ctx context.Context, key []byte, value []byte) (err error) {
	d.logger.Tracef("Put value len : %d for key %s", len(value), string(key))
	if d.validator != nil {
		d.metrics.DataValidationCount.Inc()
		ch := swarm.NewChunk(swarm.NewAddress(key), swarm.NewData(value))
		if !d.validator(ch) {
			d.metrics.DataValidationFailCount.Inc()
			d.logger.Errorf("Invalid chunk with key %s found in DB", ch.Address().String())
			return storage.ErrInvalidChunk
		}
		d.logger.Tracef(" chunk with key %s is valid", string(key))
	}

	return d.db.Update(func(txn *badger.Txn) (err error) {
		d.metrics.DiskPutCount.Inc()
		err = txn.Set(key, value)
		if err != nil {
			d.logger.Errorf("Could not insert chunk with key %s in DB", string(key))
			d.metrics.DiskPutFailCount.Inc()
			return err
		}
		d.logger.Tracef(" put success with key %s and value len %d", string(key), len(value))
		return nil
	})
}

// Has checks if the given key is present in the database.
// it returns a bool indicating true or false OR error if it encounters one during the operation.
func (d *DiskStore) Has(ctx context.Context, key []byte) (yes bool, err error) {
	d.logger.Tracef("Has value with key %s", string(key))
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
			d.metrics.DiskHasCount.Inc()
			return nil
		})
	})
	if err != nil {
		d.logger.Errorf("key %s not found in DB", string(key))
		d.logger.Debugf("key %s not found in DB. Error: %v", string(key), err.Error())
		d.metrics.DiskHasFailCount.Inc()
	}

	if yes {
		d.logger.Tracef("found key %s", string(key))
	} else {
		d.logger.Tracef("could not found key %s", string(key))
	}

	return yes, nil
}

// Delete removed the key and value if a given key is present in the DB.
func (d *DiskStore) Delete(ctx context.Context, key []byte) (err error) {
	d.logger.Tracef("deleting key %s along with value", string(key))
	return d.db.Update(func(txn *badger.Txn) (err error) {
		d.metrics.DiskDeleteCount.Inc()
		err = txn.Delete(key)
		if err != nil {
			d.logger.Errorf("could not delete key %s from DB", string(key))
			d.logger.Debugf("could not delete key %s from DB. Error: %v", string(key), err.Error())
			d.metrics.DiskDeleteFailCount.Inc()
		} else {
			d.logger.Tracef("deleted key %s", string(key))
		}
		return err
	})
}

// Count gives a count of all the keys present in the DB.
func (d *DiskStore) Count(ctx context.Context) (count int, err error) {
	d.logger.Tracef("counting all keys")
	d.metrics.DiskTotalCount.Inc()
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
	if err != nil {
		d.logger.Errorf("error while counting keys in DB")
		d.logger.Debugf("error while doing count. Error : %w", err.Error())
		d.metrics.DiskTotalFailCount.Inc()
	}
	d.logger.Tracef("Total count is %d", count)
	return count, err
}

// CountPrefix gives a count of all the keys that starts with a given key prefix.
func (d *DiskStore) CountPrefix(prefix []byte) (count int, err error) {
	if prefix != nil {
		d.logger.Tracef("counting all keys with prefix %s", string(prefix))
	}

	d.metrics.DiskCountPrefixCount.Inc()
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
	if err != nil {
		d.logger.Errorf("error while doing count. prefix : %v,", string(prefix))
		d.logger.Debugf("error while doing count for prefix : %v, Error : %w", string(prefix), err.Error())
		d.metrics.DiskCountPrefixFailCount.Inc()
	}
	d.logger.Tracef("Total count for prefix %s is %d", string(prefix), count)
	return count, err
}

// CountFrom gives a count of all the keys that start from a given prefix till the end of the DB.
func (d *DiskStore) CountFrom(prefix []byte) (count int, err error) {
	if prefix != nil {
		d.logger.Tracef("counting all keys from prefix %s", string(prefix))
	}

	d.metrics.DiskCountFromCount.Inc()
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
	if err != nil {
		d.logger.Errorf("error doing count from. prefix : %v", string(prefix))
		d.logger.Debugf("error while doing count from. prefix : %v, Error : %w", string(prefix), err.Error())
		d.metrics.DiskCountPrefixFailCount.Inc()
	}
	d.logger.Tracef("Total count from prefix %s is %d", string(prefix), count)
	return count, err
}

// Iterate goes through the entries in the DB starting from the startKey and executing a
// given function to see if it needs to stop the iteration or not. The skipStartKey indicates
// weather to skip the first key or not.
func (d *DiskStore) Iterate(startKey []byte, skipStartKey bool, fn func(key []byte, value []byte) (stop bool, err error)) (err error) {
	if startKey != nil {
		d.logger.Tracef("Iterating with startKey %s and skipping key is %t", string(startKey), skipStartKey)
	}

	d.metrics.DiskIterationCount.Inc()
	err = d.db.View(func(txn *badger.Txn) (err error) {
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
		d.logger.Errorf("error while doing iteration. startKey : %v, skipStartKey: %t",
			string(startKey), skipStartKey)
		d.logger.Debugf("error while doing iteration. startKey : %v, skipStartKey: %t, Error : %w",
			string(startKey), skipStartKey, err.Error())
		d.metrics.DiskIterationFailCount.Inc()
	}
	return err
}

// First returns the first key which matches the given prefix.
func (d *DiskStore) First(prefix []byte) (key []byte, value []byte, err error) {
	if prefix != nil {
		d.logger.Tracef("Finding first key with prefix %s", string(prefix))
	}
	d.metrics.DiskFirstCount.Inc()
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
	if err != nil {
		d.logger.Errorf("error while finding first key with prefix : %v, Error : %w", string(prefix), err.Error())
		d.logger.Debugf("error while doing first. prefix : %v, Error : %w", string(prefix), err.Error())
		d.metrics.DiskFirstFailCount.Inc()
	} else {
		d.logger.Tracef("first value with prefix %s, key %s, value len %d", string(prefix), string(key), len(value))
	}

	return key, value, err
}

// Last retuns the last key matching the given prefix.
func (d *DiskStore) Last(prefix []byte) (key []byte, value []byte, err error) {
	if prefix != nil {
		d.logger.Tracef("Finding last key with prefix %s", string(prefix))
	}
	d.metrics.DiskLastCount.Inc()
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
	if err != nil {
		d.logger.Errorf("error while finding last value for prefix : %v", string(prefix))
		d.logger.Debugf("error while doing last. prefix : %v, Error : %w", string(prefix), err.Error())
		d.metrics.DiskLastFailCount.Inc()
	} else {
		d.logger.Tracef("last value with prefix %s, key %s, value len %d", string(prefix), string(key), len(value))
	}
	return key, value, err
}

// GetBatch get a new badger transaction to be used for multiple atomic operations.
func (d *DiskStore) GetBatch(update bool) (txn *badger.Txn) {
	d.logger.Tracef("getting a transaction with update %t", update)
	d.metrics.DiskGetBatchCount.Inc()
	// set update to true indicating that data will be added/changed in this transaction.
	return d.db.NewTransaction(update)
}

// WriteBatch commits the badger transaction after all the operations are over.
func (d *DiskStore) WriteBatch(txn *badger.Txn) (err error) {
	d.metrics.DiskWriteBatchCount.Inc()
	err = txn.Commit()
	if err != nil {
		d.logger.Errorf("error while committing transaction. Error : %w", err.Error())
		d.logger.Debugf("error while committing batch. Error : %w", err.Error())
		d.metrics.DiskWriteBatchFailCount.Inc()
		return err
	}
	txn.Discard()
	d.logger.Tracef("transaction committed successfully")
	return nil
}

// Close shut's down the badger DB.
func (d *DiskStore) Close(ctx context.Context) (err error) {
	d.logger.Tracef("database closed with path %s", d.path)
	d.metrics.DiskCloseCount.Inc()
	return d.db.Close()
}
