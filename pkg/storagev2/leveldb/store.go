// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore

import (
	"fmt"
	"strings"
	"sync"

	storageV2 "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/hashicorp/go-multierror"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type Store struct {
	DB      *leveldb.DB
	path    string
	closeLk sync.RWMutex
}

var _ storageV2.Store = (*Store)(nil)

// NewDatastore returns a new datastore backed by leveldb
// for path == "", an in memory backend will be chosen (TODO)
func New(path string, opts *opt.Options) (storageV2.Store, error) {
	var err error
	var db *leveldb.DB

	if path == "" {
		db, err = leveldb.Open(storage.NewMemStorage(), opts)
	} else {
		db, err = leveldb.OpenFile(path, opts)
		if errors.IsCorrupted(err) && !opts.GetReadOnly() {
			db, err = leveldb.RecoverFile(path, opts)
		}
	}

	if err != nil {
		return nil, err
	}

	ds := Store{
		DB:   db,
		path: path,
	}

	return &ds, nil
}

func (s *Store) Count(key storageV2.Key) (c int, err error) {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	keys := util.BytesPrefix([]byte(key.Namespace() + "/"))
	iter := s.DB.NewIterator(keys, nil)

	for iter.Next() {
		c++
	}

	iter.Release()

	return c, iter.Error()
}

func (s *Store) Put(item storageV2.Item) error {
	value, err := item.Marshal()
	if err != nil {
		return fmt.Errorf("failed serializing %w", err)
	}

	key := []byte(item.Namespace() + "/" + item.ID())
	return s.DB.Put(key, value, &opt.WriteOptions{Sync: true})
}

func (s *Store) Get(item storageV2.Item) error {
	key := []byte(item.Namespace() + "/" + item.ID())

	val, err := s.DB.Get(key, nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return storageV2.ErrNotFound
		}
		return err
	}

	if err = item.Unmarshal(val); err != nil {
		return fmt.Errorf("failed decoding value %w", err)
	}

	return nil
}

func (s *Store) Has(sKey storageV2.Key) (exists bool, err error) {
	key := []byte(sKey.Namespace() + "/" + sKey.ID())

	return s.DB.Has(key, nil)
}

func (s *Store) GetSize(sKey storageV2.Key) (size int, err error) {
	key := []byte(sKey.Namespace() + "/" + sKey.ID())

	val, err := s.DB.Get(key, nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return 0, storageV2.ErrNotFound
		}
		return 0, err
	}

	return len(val), nil
}

func (s *Store) Delete(item storageV2.Key) (err error) {
	key := []byte(item.Namespace() + "/" + item.ID())

	return s.DB.Delete(key, &opt.WriteOptions{Sync: true})
}

func (s *Store) Iterate(q storageV2.Query, fn storageV2.IterateFn) error {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	if err := q.Validate(); err != nil {
		return fmt.Errorf("failed iteration: %w", err)
	}

	var retErr *multierror.Error

	namespace := q.Factory().Namespace()
	prefix := namespace + "/"

	keys := util.BytesPrefix([]byte(prefix))

	iter := s.DB.NewIterator(keys, nil)

	nextF := iter.Next

	if q.Order == storageV2.KeyDescendingOrder {
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

		var res *storageV2.Result
		switch q.ItemAttribute {
		case storageV2.QueryItemID, storageV2.QueryItemSize:
			res = &storageV2.Result{ID: key, Size: len(nextVal)}
		case storageV2.QueryItem:
			newItem := q.Factory()
			err = newItem.Unmarshal(nextVal)
			res = &storageV2.Result{Entry: newItem}
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
			retErr = multierror.Append(retErr, fmt.Errorf("failed in iterate function: %w", err))
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

func (s *Store) Close() (err error) {
	s.closeLk.Lock()
	defer s.closeLk.Unlock()
	return s.DB.Close()
}

type filters []storageV2.Filter

func (f filters) matchAny(k string, v []byte) bool {
	for _, filter := range f {
		if filter(k, v) {
			return true
		}
	}

	return false
}
