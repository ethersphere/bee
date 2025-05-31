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
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	defaultOpenFilesLimit         = uint64(256)
	defaultBlockCacheCapacity     = uint64(1 * 1024 * 1024)
	defaultWriteBufferSize        = uint64(1 * 1024 * 1024)
	defaultDisableSeeksCompaction = false
)

type Options struct {
	BlockCacheCapacity     uint64
	WriteBufferSize        uint64
	OpenFilesLimit         uint64
	DisableSeeksCompaction bool
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

	return NewDBWrap(ldb)
}

// Compact triggers a full database compaction on the underlying
// LevelDB instance. Use with care! This can be very expensive!
func (db *DB) Compact(start, end []byte) error {
	r := util.Range{Start: start, Limit: end}
	return db.ldb.CompactRange(r)
}

// Close closes LevelDB database.
func (db *DB) Close() (err error) {
	close(db.quit)
	return db.ldb.Close()
}
