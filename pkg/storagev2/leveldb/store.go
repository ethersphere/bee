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

	storage "github.com/ethersphere/bee/pkg/storagev2"
)

type store struct {
	db      *leveldb.DB
	path    string
	closeLk sync.RWMutex
}

const separator = "/"

var _ storage.Store = (*store)(nil)

// NewDatastore returns a new datastore backed by leveldb
// for path == "", an in memory backend will be chosen (TODO)
func New(path string, opts *opt.Options) (storage.Store, error) {
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

	ds := store{
		db:   db,
		path: path,
	}

	return &ds, nil
}

func (s *store) Count(key storage.Key) (int, error) {
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

func (s *store) Put(item storage.Item) error {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	value, err := item.Marshal()
	if err != nil {
		return fmt.Errorf("failed serializing: %w", err)
	}

	key := []byte(item.Namespace() + separator + item.ID())
	return s.db.Put(key, value, &opt.WriteOptions{Sync: true})
}

func (s *store) Get(item storage.Item) error {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	key := []byte(item.Namespace() + separator + item.ID())

	val, err := s.db.Get(key, nil)

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

func (s *store) Has(sKey storage.Key) (bool, error) {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	key := []byte(sKey.Namespace() + separator + sKey.ID())

	return s.db.Has(key, nil)
}

func (s *store) GetSize(sKey storage.Key) (int, error) {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	key := []byte(sKey.Namespace() + separator + sKey.ID())

	val, err := s.db.Get(key, nil)

	if errors.Is(err, leveldb.ErrNotFound) {
		return 0, storage.ErrNotFound
	}

	if err != nil {
		return 0, err
	}

	return len(val), nil
}

func (s *store) Delete(item storage.Key) error {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	key := []byte(item.Namespace() + separator + item.ID())

	return s.db.Delete(key, &opt.WriteOptions{Sync: true})
}

func (s *store) Iterate(q storage.Query, fn storage.IterateFn) error {
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
			retErr = multierror.Append(retErr, fmt.Errorf("unknown item attribute type: %v", q.ItemAttribute))
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

func (s *store) Close() (err error) {
	s.closeLk.Lock()
	defer s.closeLk.Unlock()
	return s.db.Close()
}

type filters []storage.Filter

func (f filters) matchAny(k string, v []byte) bool {
	for _, filter := range f {
		if filter(k, v) {
			return true
		}
	}

	return false
}
