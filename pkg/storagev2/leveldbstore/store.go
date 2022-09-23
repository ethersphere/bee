// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/syndtr/goleveldb/leveldb"
	ldbErrors "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	ldbStorage "github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/ethersphere/bee/pkg/storagev2"
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

var _ storage.Store = (*Store)(nil)

type Store struct {
	db      *leveldb.DB
	path    string
	closeLk sync.RWMutex
}

// New returns a new store the backed by leveldb.
// If path == "", the leveldb will run with in memory backend storage.
func New(path string, opts *opt.Options) (*Store, error) {
	var err error
	var db *leveldb.DB

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

	ds := Store{
		db:   db,
		path: path,
	}

	return &ds, nil
}

// Close implements the storage.Store interface.
func (s *Store) Close() (err error) {
	s.closeLk.Lock()
	defer s.closeLk.Unlock()
	return s.db.Close()
}

// Get implements the storage.Store interface.
func (s *Store) Get(item storage.Item) error {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

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
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	return s.db.Has(key(k), nil)
}

// GetSize implements the storage.Store interface.
func (s *Store) GetSize(k storage.Key) (int, error) {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

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
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	if err := q.Validate(); err != nil {
		return fmt.Errorf("failed iteration: %w", err)
	}

	var retErr *multierror.Error

	namespace := q.Factory().Namespace()
	prefix := namespace + separator

	keys := util.BytesPrefix([]byte(prefix))

	iter := s.db.NewIterator(keys, nil)

	nextF := iter.Next

	if q.Order == storage.KeyDescendingOrder {
		nextF = func() bool {
			nextF = iter.Prev
			return iter.Last()
		}
	}

	for nextF() {
		nextKey := string(iter.Key())
		nextVal := iter.Value()

		key := strings.TrimPrefix(nextKey, prefix)

		if filters(q.Filters).matchAny(key, nextVal) {
			continue
		}

		var err error

		var res *storage.Result
		switch q.ItemAttribute {
		case storage.QueryItemID, storage.QueryItemSize:
			res = &storage.Result{ID: key, Size: len(nextVal)}
		case storage.QueryItem:
			newItem := q.Factory()
			err = newItem.Unmarshal(nextVal)
			res = &storage.Result{Entry: newItem}
		}

		if err != nil {
			retErr = multierror.Append(retErr, fmt.Errorf("failed unmarshaling: %w", err))
			break
		}

		if res == nil {
			retErr = multierror.Append(retErr, fmt.Errorf("unknown object attribute type: %v", q.ItemAttribute))
			break
		}

		if stop, err := fn(*res); err != nil {
			retErr = multierror.Append(retErr, fmt.Errorf("iterate callback function errored: %w", err))
			break
		} else if stop {
			break
		}
	}

	iter.Release()

	if err := iter.Error(); err != nil {
		retErr = multierror.Append(retErr, err)
	}

	return retErr.ErrorOrNil()
}

// Count implements the storage.Store interface.
func (s *Store) Count(key storage.Key) (int, error) {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

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
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	value, err := item.Marshal()
	if err != nil {
		return fmt.Errorf("failed serializing: %w", err)
	}

	return s.db.Put(key(item), value, &opt.WriteOptions{Sync: true})
}

// Delete implements the storage.Store interface.
func (s *Store) Delete(item storage.Item) error {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	return s.db.Delete(key(item), &opt.WriteOptions{Sync: true})
}
