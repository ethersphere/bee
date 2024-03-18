// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/syndtr/goleveldb/leveldb"
	ldbErrors "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	ldbStorage "github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const separator = "/"

// key returns the Item identifier for the leveldb storage.
func key(item storage.Key) []byte {
	return []byte(item.Namespace() + separator + item.ID())
}

// filters is a decorator for a slice of storage.Filters
// that helps with its evaluation.
type filters []storage.Filter

// matchAny returns true if any of the filters match the item.
func (f filters) matchAny(k string, v []byte) bool {
	for _, filter := range f {
		if filter(k, v) {
			return true
		}
	}
	return false
}

// Storer returns the underlying db store.
type Storer interface {
	DB() *leveldb.DB
}

var (
	_ Storer        = (*Store)(nil)
	_ storage.Store = (*Store)(nil)
)

type Store struct {
	db   *leveldb.DB
	path string
}

// New returns a new store the backed by leveldb.
// If path == "", the leveldb will run with in memory backend storage.
func New(path string, opts *opt.Options) (*Store, error) {
	var (
		err error
		db  *leveldb.DB
	)

	if path == "" {
		db, err = leveldb.Open(ldbStorage.NewMemStorage(), opts)
	} else {
		db, err = leveldb.OpenFile(path, opts)
		if ldbErrors.IsCorrupted(err) && !opts.GetReadOnly() {
			db, err = leveldb.RecoverFile(path, opts)
		}
	}

	if err != nil {
		return nil, err
	}

	return &Store{
		db:   db,
		path: path,
	}, nil
}

// DB implements the Storer interface.
func (s *Store) DB() *leveldb.DB {
	return s.db
}

// Close implements the storage.Store interface.
func (s *Store) Close() (err error) {
	return s.db.Close()
}

// Get implements the storage.Store interface.
func (s *Store) Get(item storage.Item) error {
	val, err := s.db.Get(key(item), nil)

	if errors.Is(err, leveldb.ErrNotFound) {
		return storage.ErrNotFound
	}

	if err != nil {
		return err
	}

	if err = item.Unmarshal(val); err != nil {
		return fmt.Errorf("failed decoding value %w", err)
	}

	return nil
}

// Has implements the storage.Store interface.
func (s *Store) Has(k storage.Key) (bool, error) {
	return s.db.Has(key(k), nil)
}

// GetSize implements the storage.Store interface.
func (s *Store) GetSize(k storage.Key) (int, error) {
	val, err := s.db.Get(key(k), nil)

	if errors.Is(err, leveldb.ErrNotFound) {
		return 0, storage.ErrNotFound
	}

	if err != nil {
		return 0, err
	}

	return len(val), nil
}

// Iterate implements the storage.Store interface.
func (s *Store) Iterate(q storage.Query, fn storage.IterateFn) error {
	if err := q.Validate(); err != nil {
		return fmt.Errorf("failed iteration: %w", err)
	}

	var retErr error

	var iter iterator.Iterator
	var prefix string

	defer func() {
		if iter != nil {
			iter.Release()
		}
	}()

	iterOpts := &opt.ReadOptions{
		DontFillCache: true,
	}

	if q.PrefixAtStart {
		prefix = q.Factory().Namespace()
		iter = s.db.NewIterator(util.BytesPrefix([]byte(prefix)), iterOpts)
		exists := iter.Seek([]byte(prefix + separator + q.Prefix))
		if !exists {
			return nil
		}
		_ = iter.Prev()
	} else {
		// this is a small hack to make the iteration work with the
		// old implementation of statestore. this allows us to do a
		// full iteration without looking at the prefix.
		if q.Factory().Namespace() != "" {
			prefix = q.Factory().Namespace() + separator + q.Prefix
		}
		iter = s.db.NewIterator(util.BytesPrefix([]byte(prefix)), iterOpts)
	}

	nextF := iter.Next

	if q.Order == storage.KeyDescendingOrder {
		nextF = func() bool {
			nextF = iter.Prev
			return iter.Last()
		}
	}

	firstSkipped := !q.SkipFirst

	for nextF() {
		keyRaw := iter.Key()
		nextKey := make([]byte, len(keyRaw))
		copy(nextKey, keyRaw)

		valRaw := iter.Value()
		nextVal := make([]byte, len(valRaw))
		copy(nextVal, valRaw)

		key := strings.TrimPrefix(string(nextKey), prefix)

		if filters(q.Filters).matchAny(key, nextVal) {
			continue
		}

		if q.SkipFirst && !firstSkipped {
			firstSkipped = true
			continue
		}

		var (
			res *storage.Result
			err error
		)

		switch q.ItemProperty {
		case storage.QueryItemID, storage.QueryItemSize:
			res = &storage.Result{ID: key, Size: len(nextVal)}
		case storage.QueryItem:
			newItem := q.Factory()
			err = newItem.Unmarshal(nextVal)
			res = &storage.Result{ID: key, Entry: newItem}
		}

		if err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("failed unmarshaling: %w", err))
			break
		}

		if res == nil {
			retErr = errors.Join(retErr, fmt.Errorf("unknown object attribute type: %v", q.ItemProperty))
			break
		}

		if stop, err := fn(*res); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("iterate callback function errored: %w", err))
			break
		} else if stop {
			break
		}
	}

	if err := iter.Error(); err != nil {
		retErr = errors.Join(retErr, err)
	}

	return retErr
}

// Count implements the storage.Store interface.
func (s *Store) Count(key storage.Key) (int, error) {
	keys := util.BytesPrefix([]byte(key.Namespace() + separator))
	iter := s.db.NewIterator(keys, nil)

	var c int
	for iter.Next() {
		c++
	}

	iter.Release()

	return c, iter.Error()
}

// Put implements the storage.Store interface.
func (s *Store) Put(item storage.Item) error {
	value, err := item.Marshal()
	if err != nil {
		return fmt.Errorf("failed serializing: %w", err)
	}

	return s.db.Put(key(item), value, nil)
}

// Delete implements the storage.Store interface.
func (s *Store) Delete(item storage.Item) error {
	// this is a small hack to make the deletion of old entries work. As they
	// don't have a namespace, we need to check for that and use the ID as key without
	// the separator.
	var k []byte
	if item.Namespace() == "" {
		k = []byte(item.ID())
	} else {
		k = key(item)
	}

	return s.db.Delete(k, nil)
}
