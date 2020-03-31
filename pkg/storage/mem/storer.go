// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mem

import (
	"context"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type memStore struct {
	store map[string][]byte
}

func NewMemStorer() (store *memStore, err error) {
	s := &memStore{
		store: make(map[string][]byte),
	}
	return s, nil
}

func (m *memStore) Get(ctx context.Context, addr swarm.Address) (chunk swarm.Chunk, err error) {
	v, has := m.store[addr.String()]
	if !has {
		return chunk, storage.ErrNotFound
	}
	chunk = swarm.NewChunk(addr, v)
	return chunk, nil
}

func (m *memStore) Put(ctx context.Context, chunk swarm.Chunk) error {
	m.store[chunk.Address().String()] = chunk.Data()
	return nil
}

func (m *memStore) Has(ctx context.Context, addr swarm.Address) (yes bool, err error) {
	if m.store[addr.String()] != nil {
		return true, nil
	}
	return false, nil
}

func (m *memStore) Close() (err error) {
	m.store = nil
	return nil
}