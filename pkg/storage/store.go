// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.package storage

package storage

import (
	"context"
	"errors"

	"github.com/dgraph-io/badger"
)

var (
	ErrNotFound       = errors.New("storage: not found")
	ErrNotImplemented = errors.New("storage: not implemented")
	ErrInvalidChunk = errors.New("storage: invalid chunk")
)

type Storer interface {
	Get(ctx context.Context, key []byte) (value []byte, err error)
	Put(ctx context.Context, key []byte, value []byte) (err error)
	Has(ctx context.Context, key []byte) (yes bool, err error)
	Delete(ctx context.Context, key []byte) (err error)
	Count(ctx context.Context) (count int, err error)
	CountPrefix(prefix []byte) (count int, err error)
	CountFrom(prefix []byte) (count int, err error)
	Iterate(startKey []byte, skipStartKey bool, fn func(key []byte, value []byte) (stop bool, err error)) (err error)
	First(prefix []byte) (key []byte, value []byte, err error)
	Last(prefix []byte) (key []byte, value []byte, err error)
	GetBatch(update bool) (txn *badger.Txn)
	WriteBatch(txn *badger.Txn) (err error)
	Close(ctx context.Context) (err error)
}
