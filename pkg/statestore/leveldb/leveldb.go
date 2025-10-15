// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldb

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"

	ldberr "github.com/syndtr/goleveldb/leveldb/errors"

	"github.com/syndtr/goleveldb/leveldb"
	ldbs "github.com/syndtr/goleveldb/leveldb/storage"

	"github.com/syndtr/goleveldb/leveldb/util"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "leveldb"

var (
	_ storage.StateStorer = (*Store)(nil)
)

// Store uses LevelDB to store values.
type Store struct {
	db     *leveldb.DB
	logger log.Logger
}

func NewInMemoryStateStore(l log.Logger) (*Store, error) {
	ldb, err := leveldb.Open(ldbs.NewMemStorage(), nil)
	if err != nil {
		return nil, err
	}

	s := &Store{
		db:     ldb,
		logger: l.WithName(loggerName).Register(),
	}

	return s, nil
}

// NewStateStore creates a new persistent state storage.
func NewStateStore(path string, l log.Logger) (*Store, error) {
	l = l.WithName(loggerName).Register()

	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		if !ldberr.IsCorrupted(err) {
			return nil, err
		}

		l.Warning("statestore open failed, attempting recovery", "error", err)
		db, err = leveldb.RecoverFile(path, nil)
		if err != nil {
			return nil, fmt.Errorf("statestore recovery: %w", err)
		}
		l.Warning("statestore recovery done; you are kindly request to inform us about the steps that preceded the last bee shutdown")
	}

	s := &Store{
		db:     db,
		logger: l,
	}

	return s, nil
}

// Get retrieves a value of the requested key. If no results are found,
// storage.ErrNotFound will be returned.
func (s *Store) Get(key string, i interface{}) error {
	data, err := s.db.Get([]byte(key), nil)
	if err != nil {
		if errors.Is(err, leveldb.ErrNotFound) {
			return storage.ErrNotFound
		}
		return err
	}

	if unmarshaler, ok := i.(encoding.BinaryUnmarshaler); ok {
		return unmarshaler.UnmarshalBinary(data)
	}

	return json.Unmarshal(data, i)
}

// Put stores a value for an arbitrary key. BinaryMarshaler
// interface method will be called on the provided value
// with fallback to JSON serialization.
func (s *Store) Put(key string, i interface{}) (err error) {
	var bytes []byte
	if marshaler, ok := i.(encoding.BinaryMarshaler); ok {
		if bytes, err = marshaler.MarshalBinary(); err != nil {
			return err
		}
	} else if bytes, err = json.Marshal(i); err != nil {
		return err
	}

	return s.db.Put([]byte(key), bytes, nil)
}

// Delete removes entries stored under a specific key.
func (s *Store) Delete(key string) (err error) {
	return s.db.Delete([]byte(key), nil)
}

// Iterate entries that match the supplied prefix.
func (s *Store) Iterate(prefix string, iterFunc storage.StateIterFunc) (err error) {
	iter := s.db.NewIterator(util.BytesPrefix([]byte(prefix)), nil)
	defer iter.Release()
	for iter.Next() {
		stop, err := iterFunc(append([]byte(nil), iter.Key()...), append([]byte(nil), iter.Value()...))
		if err != nil {
			return err
		}
		if stop {
			break
		}
	}
	return iter.Error()
}

// Close releases the resources used by the store.
func (s *Store) Close() error {
	return s.db.Close()
}
