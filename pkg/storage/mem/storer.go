// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mem

import (
	"bytes"
	"context"
	"encoding/hex"

	"github.com/dgraph-io/badger"
	"github.com/ethersphere/bee/pkg/storage"
)

type MemStore struct {
	store map[string][]byte
}

func NewMemStorer() (store *MemStore, err error) {
	s := &MemStore{
		store: make(map[string][]byte),
	}
	return s, nil
}

func (m *MemStore) Get(ctx context.Context, key []byte) (value []byte, err error) {
	value , has := m.store[hex.EncodeToString(key)]
	if !has {
		return nil , storage.ErrNotFound
	}
	return value , nil
}

func (m *MemStore) Put(ctx context.Context, key []byte, value []byte) (err error) {
	m.store[hex.EncodeToString(key)] = value
	return nil
}

func (m *MemStore) Has(ctx context.Context, key []byte) (yes bool, err error) {
	if m.store[hex.EncodeToString(key)] != nil {
		return true, nil
	}
	return false, nil
}

func (m *MemStore) Delete(ctx context.Context, key []byte) (err error) {
	_, has := m.store[hex.EncodeToString(key)]
	if !has {
		return nil
	}
	m.store[hex.EncodeToString(key)] = nil
	return nil
}

func (m *MemStore) Count(ctx context.Context) (count int, err error) {
	return len(m.store), nil
}

func (m *MemStore) CountPrefix(prefix []byte) (count int, err error) {
	for k, _ := range m.store {
		if bytes.HasPrefix([]byte(k), prefix) {
			count++
		}
	}
	return count, nil
}

func (m *MemStore) CountFrom(prefix []byte) (count int, err error) {
	var foundPrefix bool
	for k, _ := range m.store {
		if bytes.HasPrefix([]byte(k), prefix) {
			foundPrefix = true
		}
		if foundPrefix {
			count++
		}
	}
	return count, nil
}

func (m *MemStore) Iterate(startKey []byte, skipStartKey bool, fn func(key []byte, value []byte) (stop bool, err error)) (err error) {
	var startIterating bool
	for k, v := range m.store {
		if bytes.Equal([]byte(k), startKey) {
			startIterating = true
			if skipStartKey {
				continue
			}
		}
		if startIterating {
			stop, err := fn([]byte(k), v)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
		}
	}
	return nil
}

func (m *MemStore) First(prefix []byte) (key []byte, value []byte, err error) {
	for k, v := range m.store {
		if bytes.HasPrefix([]byte(k), prefix) {
			return []byte(k), v, nil
		}
	}
	return nil, nil, storage.ErrNotFound
}

func (m *MemStore) Last(prefix []byte) (key []byte, value []byte, err error) {
	var found bool
	for k, v := range m.store {
		if bytes.HasPrefix([]byte(k), prefix) {
			found = true
		}

		if found && !bytes.HasPrefix([]byte(k), prefix) {
			return []byte(k), v, nil
		}
	}
	return nil, nil, storage.ErrNotFound
}

func (m *MemStore) GetBatch(update bool) (txn *badger.Txn) {
	return nil
}

func (m *MemStore) WriteBatch(txn *badger.Txn) (err error) {
	return storage.ErrNotImplemented
}

func (m *MemStore) Close(ctx context.Context) (err error) {
	m.store = nil
	return nil
}
