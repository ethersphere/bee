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

package shed

import (
	"context"
	"encoding/binary"

	"github.com/dgraph-io/badger"
	"github.com/ethersphere/bee/pkg/storage"
)

// Uint64Vector provides a way to have multiple counters in the database.
// It transparently encodes uint64 type value to bytes.
type Uint64Vector struct {
	db  *DB
	key []byte
}

// NewUint64Vector returns a new Uint64Vector.
// It validates its name and type against the database schema.
func (db *DB) NewUint64Vector(ctx context.Context, name string) (f Uint64Vector, err error) {
	key, err := db.schemaFieldKey(ctx, name, "vector-uint64")
	if err != nil {
		return f, err
	}
	return Uint64Vector{
		db:  db,
		key: key,
	}, nil
}

// Get retrieves a uint64 value at index i from the database.
// If the value is not found in the database a 0 value
// is returned and no error.
func (f Uint64Vector) Get(ctx context.Context, i uint64) (val uint64, err error) {
	b, err := f.db.Store.Get(ctx, f.indexKey(i))
	if err != nil {
		if err == storage.ErrNotFound {
			return 0, nil
		}
		return 0, err
	}
	return binary.BigEndian.Uint64(b), nil
}

// Put encodes uin64 value and stores it in the database.
func (f Uint64Vector) Put(ctx context.Context, i, val uint64) (err error) {
	return f.db.Store.Put(ctx, f.indexKey(i),encodeUint64(val) )
}

// PutInBatch stores a uint64 value at index i in a batch
// that can be saved later in the database.
func (f Uint64Vector) PutInBatch(batch *badger.Txn, i, val uint64) (err error) {
	return batch.Set(f.indexKey(i), encodeUint64(val))
}

// Inc increments a uint64 value in the database.
// This operation is not goroutine safe.
func (f Uint64Vector) Inc(ctx context.Context, i uint64) (val uint64, err error) {
	val, err = f.Get(ctx, i)
	if err != nil {
		if err == storage.ErrNotFound {
			val = 0
		} else {
			return 0, err
		}
	}
	val++
	return val, f.Put(ctx, i, val)
}

// IncInBatch increments a uint64 value at index i in the batch
// by retreiving a value from the database, not the same batch.
// This operation is not goroutine safe.
func (f Uint64Vector) IncInBatch(ctx context.Context, batch *badger.Txn, i uint64) (val uint64, err error) {
	val, err = f.Get(ctx, i)
	if err != nil {
		if err == storage.ErrNotFound {
			val = 0
		} else {
			return 0, err
		}
	}
	val++
	err = f.PutInBatch(batch, i, val)
	if err != nil {
		return 0, err
	}
	return val, nil
}

// Dec decrements a uint64 value at index i in the database.
// This operation is not goroutine safe.
// The field is protected from overflow to a negative value.
func (f Uint64Vector) Dec(ctx context.Context, i uint64) (val uint64, err error) {
	val, err = f.Get(ctx, i)
	if err != nil {
		if err == storage.ErrNotFound {
			val = 0
		} else {
			return 0, err
		}
	}
	if val != 0 {
		val--
	}
	return val, f.Put(ctx, i, val)
}

// DecInBatch decrements a uint64 value at index i in the batch
// by retreiving a value from the database, not the same batch.
// This operation is not goroutine safe.
// The field is protected from overflow to a negative value.
func (f Uint64Vector) DecInBatch(ctx context.Context, batch *badger.Txn, i uint64) (val uint64, err error) {
	val, err = f.Get(ctx, i)
	if err != nil {
		if err == storage.ErrNotFound {
			val = 0
		} else {
			return 0, err
		}
	}
	if val != 0 {
		val--
	}
	err = f.PutInBatch(batch, i, val)
	if err != nil {
		return 0, err
	}
	return val, nil
}

// indexKey concatenates field prefix and vector index
// returning a unique database key for a specific vector element.
func (f Uint64Vector) indexKey(i uint64) (key []byte) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, i)
	return append(f.key, b...)
}
