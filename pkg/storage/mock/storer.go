// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type mockStorer struct {
	store     map[string][]byte
	validator storage.ChunkValidatorFunc
}

func NewStorer() storage.Storer {
	s := &mockStorer{
		store: make(map[string][]byte),
	}

	return s
}

func NewValidatingStorer(f storage.ChunkValidatorFunc) storage.Storer {
	s := &mockStorer{
		store:     make(map[string][]byte),
		validator: f,
	}

	return s
}

func (m *mockStorer) Get(ctx context.Context, addr swarm.Address) (data []byte, err error) {
	v, has := m.store[addr.String()]
	if !has {
		return nil, storage.ErrNotFound
	}
	return v, nil
}

func (m *mockStorer) Put(ctx context.Context, addr swarm.Address, data []byte) error {
	if m.validator != nil {
		if !m.validator(addr, data) {
			return storage.ErrInvalidChunk
		}
	}
	m.store[addr.String()] = data
	return nil
}
