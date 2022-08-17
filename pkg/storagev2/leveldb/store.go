// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"

	storageV2 "github.com/ethersphere/bee/pkg/storagev2"
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

	ns := []byte(key.Namespace())

	for iter.Next() {
		iKey := iter.Key()
		if bytes.Equal(iKey[:len(ns)], ns) {
			c++
		}
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

	err = item.Unmarshal(val)
	if err != nil {
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
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	prefix := q.Factory().Namespace() + "/"
	keys := util.BytesPrefix([]byte(prefix))

	iter := s.DB.NewIterator(keys, nil)
	defer iter.Release()

	getNext := func(k string, v []byte) (*storageV2.Result, error) {
		knp := strings.TrimPrefix(k, prefix)
		for _, filter := range q.Filters {
			if filter(knp, v) {
				return nil, nil
			}
		}

		switch q.ItemAttribute {
		case storageV2.QueryItemID, storageV2.QueryItemSize:
			return &storageV2.Result{ID: knp, Size: len(v)}, nil
		case storageV2.QueryItem:
			newItem := q.Factory()
			err := newItem.Unmarshal(v)
			if err != nil {
				return nil, fmt.Errorf("failed unmarshaling: %w", err)
			}
			return &storageV2.Result{Entry: newItem}, nil
		}

		return nil, nil
	}

	for iter.Next() {
		k := string(iter.Key())
		v := iter.Value()

		var res *storageV2.Result

		res, err := getNext(k, v)
		if err != nil || res == nil {
			continue
		}

		res.Entry = q.Factory()

		if err := res.Entry.Unmarshal(iter.Value()); err != nil {
			return err
		}

		if stop, err := fn(*res); err != nil {
			return err
		} else if stop {
			return nil
		}
	}

	return iter.Error()
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
