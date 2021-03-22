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
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

var (
	defaultOpenFilesLimit         = uint64(256)
	defaultBlockCacheCapacity     = uint64(32 * 1024 * 1024)
	defaultWriteBufferSize        = uint64(32 * 1024 * 1024)
	defaultDisableSeeksCompaction = false
)

type Options struct {
	BlockCacheCapacity     uint64
	WriteBufferSize        uint64
	OpenFilesLimit         uint64
	DisableSeeksCompaction bool
}

// DB provides abstractions over LevelDB in order to
// implement complex structures using fields and ordered indexes.
// It provides a schema functionality to store fields and indexes
// information about naming and types.
type DB struct {
	ldb     *leveldb.DB
	metrics metrics
	quit    chan struct{} // Quit channel to stop the metrics collection before closing the database
}

// NewDB constructs a new DB and validates the schema
// if it exists in database on the given path.
// metricsPrefix is used for metrics collection for the given DB.
func NewDB(path string, o *Options) (db *DB, err error) {
	if o == nil {
		o = &Options{
			OpenFilesLimit:         defaultOpenFilesLimit,
			BlockCacheCapacity:     defaultBlockCacheCapacity,
			WriteBufferSize:        defaultWriteBufferSize,
			DisableSeeksCompaction: defaultDisableSeeksCompaction,
		}
	}
	var ldb *leveldb.DB
	if path == "" {
		ldb, err = leveldb.Open(storage.NewMemStorage(), nil)
	} else {
		ldb, err = leveldb.OpenFile(path, &opt.Options{
			OpenFilesCacheCapacity: int(o.OpenFilesLimit),
			BlockCacheCapacity:     int(o.BlockCacheCapacity),
			WriteBuffer:            int(o.WriteBufferSize),
			DisableSeeksCompaction: o.DisableSeeksCompaction,
		})
	}

	if err != nil {
		return nil, err
	}

	db = &DB{
		ldb:     ldb,
		metrics: newMetrics(),
	}

	if _, err = db.getSchema(); err != nil {
		if errors.Is(err, leveldb.ErrNotFound) {
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

	// Create a quit channel for the periodic metrics collector and run it
	db.quit = make(chan struct{})

	return db, nil
}

// Put wraps LevelDB Put method to increment metrics counter.
func (db *DB) Put(key, value []byte) (err error) {
	err = db.ldb.Put(key, value, nil)
	if err != nil {
		db.metrics.PutFailCounter.Inc()
		return err
	}
	db.metrics.PutCounter.Inc()
	return nil
}

// Get wraps LevelDB Get method to increment metrics counter.
func (db *DB) Get(key []byte) (value []byte, err error) {
	value, err = db.ldb.Get(key, nil)
	if err != nil {
		if errors.Is(err, leveldb.ErrNotFound) {
			db.metrics.GetNotFoundCounter.Inc()
		} else {
			db.metrics.GetFailCounter.Inc()
		}
		return nil, err
	}
	db.metrics.GetCounter.Inc()
	return value, nil
}

// Has wraps LevelDB Has method to increment metrics counter.
func (db *DB) Has(key []byte) (yes bool, err error) {
	yes, err = db.ldb.Has(key, nil)
	if err != nil {
		db.metrics.HasFailCounter.Inc()
		return false, err
	}
	db.metrics.HasCounter.Inc()
	return yes, nil
}

// Delete wraps LevelDB Delete method to increment metrics counter.
func (db *DB) Delete(key []byte) (err error) {
	err = db.ldb.Delete(key, nil)
	if err != nil {
		db.metrics.DeleteFailCounter.Inc()
		return err
	}
	db.metrics.DeleteCounter.Inc()
	return nil
}

// NewIterator wraps LevelDB NewIterator method to increment metrics counter.
func (db *DB) NewIterator() iterator.Iterator {
	db.metrics.IteratorCounter.Inc()
	return db.ldb.NewIterator(nil, nil)
}

// WriteBatch wraps LevelDB Write method to increment metrics counter.
func (db *DB) WriteBatch(batch *leveldb.Batch) (err error) {
	err = db.ldb.Write(batch, nil)
	if err != nil {
		db.metrics.WriteBatchFailCounter.Inc()
		return err
	}
	db.metrics.WriteBatchCounter.Inc()
	return nil
}

// Close closes LevelDB database.
func (db *DB) Close() (err error) {
	close(db.quit)
	return db.ldb.Close()
}
