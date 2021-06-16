// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldb

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/syndtr/goleveldb/leveldb"
	ldberr "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var _ storage.StateStorer = (*store)(nil)

// store uses LevelDB to store values.
type store struct {
	db     *leveldb.DB
	logger logging.Logger
}

// NewStateStore creates a new persistent state storage.
func NewStateStore(path string, l logging.Logger) (storage.StateStorer, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		if !ldberr.IsCorrupted(err) {
			return nil, err
		}

		l.Warningf("statestore open failed: %v. attempting recovery", err)
		db, err = leveldb.RecoverFile(path, nil)
		if err != nil {
			return nil, fmt.Errorf("statestore recovery: %w", err)
		}
		l.Warning("statestore recovery ok! you are kindly request to inform us about the steps that preceded the last Bee shutdown.")
	}

	s := &store{
		db:     db,
		logger: l,
	}

	sn, err := s.getSchemaName()
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			_ = s.Close()
			return nil, fmt.Errorf("get schema name: %w", err)
		}
		// new statestore - put schema key with current name
		if err := s.putSchemaName(dbSchemaCurrent); err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("put schema name: %w", err)
		}
		sn = dbSchemaCurrent
	}

	if err = s.migrate(sn); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	del := []string{}
	s.Iterate("addressbook_entry_", func(k, _ []byte) (bool, error) {
		del = append(del, string(k))
		return false, nil
	})
	del = append(del, "overlay")

	for _, v := range del {
		_ = s.Delete(v)
	}

	return s, nil
}

// Get retrieves a value of the requested key. If no results are found,
// storage.ErrNotFound will be returned.
func (s *store) Get(key string, i interface{}) error {
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
func (s *store) Put(key string, i interface{}) (err error) {
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
func (s *store) Delete(key string) (err error) {
	return s.db.Delete([]byte(key), nil)
}

// Iterate entries that match the supplied prefix.
func (s *store) Iterate(prefix string, iterFunc storage.StateIterFunc) (err error) {
	iter := s.db.NewIterator(util.BytesPrefix([]byte(prefix)), nil)
	defer iter.Release()
	for iter.Next() {
		stop, err := iterFunc(iter.Key(), iter.Value())
		if err != nil {
			return err
		}
		if stop {
			break
		}
	}
	return iter.Error()
}

func (s *store) getSchemaName() (string, error) {
	name, err := s.db.Get([]byte(dbSchemaKey), nil)
	if err != nil {
		if errors.Is(err, leveldb.ErrNotFound) {
			return "", storage.ErrNotFound
		}
		return "", err
	}
	return string(name), nil
}

func (s *store) putSchemaName(val string) error {
	return s.db.Put([]byte(dbSchemaKey), []byte(val), nil)
}

// DB implements StateStorer.DB method.
func (s *store) DB() *leveldb.DB {
	return s.db
}

// Close releases the resources used by the store.
func (s *store) Close() error {
	return s.db.Close()
}
