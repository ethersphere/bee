// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore

import (
	"context"
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
var _ storageV2.Tx = (*transaction)(nil)

// NewDatastore returns a new datastore backed by leveldb
// for path == "", an in memory backend will be chosen (TODO)
func NewLevelDBStore(path string, opts *opt.Options) (storageV2.Store, error) {
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
	keys := util.BytesPrefix([]byte(key.Namespace() + "/"))

	iter := s.DB.NewIterator(keys, nil)
	defer iter.Release()

	for iter.Next() {
		c++
	}

	return c, iter.Error()
}

func (s *Store) Put(item storageV2.Item) error {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	value, err := item.Marshal()
	if err != nil {
		return fmt.Errorf("failed serializing %w", err)
	}

	key := []byte(item.Namespace() + "/" + item.ID())

	return s.DB.Put(key, value, &opt.WriteOptions{Sync: true})
}

func (s *Store) Get(item storageV2.Item) error {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

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
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	key := []byte(sKey.Namespace() + "/" + sKey.ID())

	return s.DB.Has(key, nil)
}

func (s *Store) GetSize(sKey storageV2.Key) (size int, err error) {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

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
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	key := []byte(item.Namespace() + "/" + item.ID())

	return s.DB.Delete(key, &opt.WriteOptions{Sync: true})
}

func (s *Store) Iterate(q storageV2.Query, fn storageV2.IterateFn) error {
	if err := q.Validate(); err != nil {
		return fmt.Errorf("failed iteration: %w", err)
	}

	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	var retErr *multierror.Error

	namespace := q.Factory().Namespace()
	prefix := namespace + "/"

	keys := util.BytesPrefix([]byte(prefix))

	iter := s.DB.NewIterator(keys, nil)

	switch q.Order {
	case storageV2.KeyAscendingOrder:
		for iter.Next() {
			k := string(iter.Key())
			v := iter.Value()

			stop, err := apply(k, v, q, fn)
			if err != nil {
				retErr = multierror.Append(retErr, err)
			}
			if stop {
				break
			}
		}
	case storageV2.KeyDescendingOrder:
		for ok := iter.Last(); ok; ok = iter.Prev() {
			k := string(iter.Key())
			v := iter.Value()

			stop, err := apply(k, v, q, fn)
			if err != nil {
				retErr = multierror.Append(retErr, err)
			}
			if stop {
				break
			}
		}
	}

	iter.Release()

	if err := iter.Error(); err != nil {
		retErr = multierror.Append(retErr, err)
	}

	return retErr.ErrorOrNil()
}

func apply(k string, v []byte, q storageV2.Query, fn func(storageV2.Result) (bool, error)) (stop bool, err error) {
	namespace := q.Factory().Namespace()
	prefix := namespace + "/"

	knp := strings.TrimPrefix(k, prefix)
	for _, filter := range q.Filters {
		if filter(knp, v) {
			return false, nil
		}
	}

	var res *storageV2.Result
	switch q.ItemAttribute {
	case storageV2.QueryItemID, storageV2.QueryItemSize:
		res = &storageV2.Result{ID: knp, Size: len(v)}
	case storageV2.QueryItem:
		newItem := q.Factory()
		err := newItem.Unmarshal(v)
		if err != nil {
			return true, fmt.Errorf("failed unmarshaling: %w", err)
		}
		res = &storageV2.Result{Entry: newItem}
	}

	if res != nil {
		stop, err = fn(*res)
		if err != nil {
			return true, fmt.Errorf("failed in iterate function: %w", err)
		}
		if stop {
			return true, nil
		}
	}

	return
}

func (s *Store) Close() (err error) {
	s.closeLk.Lock()
	defer s.closeLk.Unlock()
	return s.DB.Close()
}

// A leveldb transaction embedding the accessor backed by the transaction.
type transaction struct {
	st   *Store
	do   *leveldb.Batch
	undo *leveldb.Batch
}

func (t *transaction) Get(storageV2.Item) error {
	panic("leveldb store: get not implemented")
}

func (t *transaction) Has(storageV2.Key) (bool, error) {
	panic("leveldb store: batch check presence not implemented")
}

func (t *transaction) GetSize(storageV2.Key) (int, error) {
	panic("leveldb store: get size not implemented")
}

func (t *transaction) Iterate(storageV2.Query, storageV2.IterateFn) error {
	panic("leveldb store: iterate not implemented")
}

func (t *transaction) Count(storageV2.Key) (int, error) {
	panic("leveldb store: count not implemented")
}

func (t *transaction) Put(storageV2.Item) error {
	panic("leveldb store: batch put not implemented")
}

func (t *transaction) Delete(storageV2.Key) error {
	// add delete entry to do batch
	// get and add put entry to undo batch
	panic("leveldb store: batch delete not implemented")
}

func (t *transaction) Close() error {
	panic("leveldb store: batch close not implemented")
}

func (t *transaction) Commit(ctx context.Context) error {
	panic("leveldb store: batch commit not implemented")
}

func (t *transaction) Rollback(ctx context.Context) error {
	panic("leveldb store: rollback not implemented")
}

func (s *Store) NewTransaction(ctx context.Context, readOnly bool) (storageV2.Tx, error) {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	return &transaction{st: s, do: new(leveldb.Batch), undo: new(leveldb.Batch)}, nil
}
