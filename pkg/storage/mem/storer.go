// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mem

import (
	"bytes"
	"context"
	"github.com/dgraph-io/badger"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type MemStore struct {
	store     map[string][]byte
	order     []string
	validator storage.ChunkValidatorFunc
	logger    logging.Logger
}

func NewMemStorer(v storage.ChunkValidatorFunc, logger logging.Logger) (store *MemStore, err error) {
	s := &MemStore{
		store:     make(map[string][]byte),
		order:     make([]string, 0),
		validator: v,
		logger:    logger,
	}
	return s, nil
}

func (m *MemStore) Get(ctx context.Context, key []byte) (value []byte, err error) {
	val, has := m.store[string(key)]
	if !has {
		return nil, storage.ErrNotFound
	}
	return val, nil
}

func (m *MemStore) Put(ctx context.Context, key []byte, value []byte) (err error) {
	if m.validator != nil {
		ch := swarm.NewChunk(swarm.NewAddress(key), swarm.NewData(value))
		if !m.validator(ch) {
			return storage.ErrInvalidChunk
		}
	}
	m.store[string(key)] = value
	m.order = append(m.order, string(key))
	return nil
}

func (m *MemStore) Has(ctx context.Context, key []byte) (yes bool, err error) {
	if _, yes := m.store[(string(key))]; yes {
		return true, nil
	}
	return false, nil
}

func (m *MemStore) Delete(ctx context.Context, key []byte) (err error) {
	_, has := m.store[string(key)]
	if !has {
		return nil
	}
	m.store[string(key)] = nil
	return nil
}

func (m *MemStore) Count(ctx context.Context) (count int, err error) {
	return len(m.store), nil
}

func (m *MemStore) CountPrefix(prefix []byte) (count int, err error) {
	for _, k := range m.order {
		if bytes.HasPrefix([]byte(k), prefix) {
			count++
		}
	}
	return count, nil
}

func (m *MemStore) CountFrom(prefix []byte) (count int, err error) {
	var foundPrefix bool
	for _, k := range m.order {
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
	var startCounting bool
	var skipped bool
	for _, k := range m.order {

		if bytes.Equal(startKey, []byte{0}) {
			startCounting = true
		} else if bytes.HasPrefix([]byte(k), startKey) {
			startCounting = true
		}

		if startCounting && skipStartKey && !skipped {
			skipped = true
			continue
		}

		if startCounting {
			v := m.store[k]
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
	for _, k := range m.order {
		if bytes.HasPrefix([]byte(k), prefix) {
			v := m.store[k]
			return []byte(k), v, nil
		}
	}
	return nil, nil, storage.ErrNotFound
}

func (m *MemStore) Last(prefix []byte) (key []byte, value []byte, err error) {
	for _, k := range m.order {
		if prefix == nil {
			value = m.store[k]
		}
		if bytes.HasPrefix([]byte(k), prefix) {
			value = m.store[k]
		}
	}

	if value != nil {
		return key, value, nil
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
