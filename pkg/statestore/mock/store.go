// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"encoding"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/storage"
)

// InspectableStore extends StateStorer with methods for test introspection.
type InspectableStore interface {
	storage.StateStorer
	Len() int
	KeysWithPrefix(prefix string) []string
}

var _ storage.StateStorer = (*store)(nil)

type store struct {
	store map[string][]byte
	mtx   sync.RWMutex
}

func NewStateStore() storage.StateStorer {
	s := &store{
		store: make(map[string][]byte),
	}

	return s
}

func (s *store) Get(key string, i any) (err error) {
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

func (s *store) Put(key string, i any) (err error) {
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

// Len returns the total number of entries in the store.
func (s *store) Len() int {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return len(s.store)
}

// KeysWithPrefix returns a sorted slice of keys that start with prefix.
func (s *store) KeysWithPrefix(prefix string) []string {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	var keys []string
	for k := range s.store {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func (s *store) Close() (err error) {
	return nil
}

var _ InspectableStore = (*store)(nil)
