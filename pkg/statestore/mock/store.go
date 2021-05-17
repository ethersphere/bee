// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.package storage

package mock

import (
	"encoding"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/syndtr/goleveldb/leveldb"
)

var _ storage.StateStorer = (*store)(nil)

const mockSchemaNameKey = "schema_name"

type store struct {
	store map[string][]byte
	mtx   sync.RWMutex
}

func NewStateStore() storage.StateStorer {
	s := &store{
		store: make(map[string][]byte),
	}

	if err := s.Put(mockSchemaNameKey, "mock_schema"); err != nil {
		panic(fmt.Errorf("put schema name: %w", err))
	}

	return s
}

func (s *store) Get(key string, i interface{}) (err error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	data, ok := s.store[key]
	if !ok {
		return storage.ErrNotFound
	}

	if unmarshaler, ok := i.(encoding.BinaryUnmarshaler); ok {
		return unmarshaler.UnmarshalBinary(data)
	}

	return json.Unmarshal(data, i)
}

func (s *store) Put(key string, i interface{}) (err error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var bytes []byte
	if marshaler, ok := i.(encoding.BinaryMarshaler); ok {
		if bytes, err = marshaler.MarshalBinary(); err != nil {
			return err
		}
	} else if bytes, err = json.Marshal(i); err != nil {
		return err
	}

	s.store[key] = bytes
	return nil
}

func (s *store) Delete(key string) (err error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	delete(s.store, key)
	return nil
}

func (s *store) Iterate(prefix string, iterFunc storage.StateIterFunc) (err error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	for k, v := range s.store {
		if !strings.HasPrefix(k, prefix) {
			continue
		}

		val := make([]byte, len(v))
		copy(val, v)
		stop, err := iterFunc([]byte(k), val)
		if err != nil {
			return err
		}

		if stop {
			return nil
		}
	}
	return nil
}

// DB implements StateStorer.DB method.
func (s *store) DB() *leveldb.DB {
	return nil
}

func (s *store) Close() (err error) {
	return nil
}
