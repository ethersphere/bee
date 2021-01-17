// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	"math/big"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
)

const (
	batchKeyPrefix = "batchKeyPrefix"
	stateKey       = "stateKey"
)

type store struct {
	store storage.StateStorer // State store backend to persist batches.
}

// New constructs a new postage batch store.
func New(st storage.StateStorer) postage.Storer {
	return &store{st}
}

// Get returns a batch from the batchstore with the given ID.
func (s *store) Get(id []byte) (*postage.Batch, error) {
	b := &postage.Batch{}
	err := s.store.Get(batchKey(id), b)

	return b, err
}

// Put stores a given batch in the batchstore with the given ID.
func (s *store) Put(b *postage.Batch) error {
	return s.store.Put(batchKey(b.ID), b)
}

// PutChainState implements BatchStorer. It stores the chain state in the batch
// store.
func (s *store) PutChainState(state *postage.ChainState) error {
	return s.store.Put(stateKey, state)
}

// GetChainState implements BatchStorer. It returns the stored chain state from
// the batch store.
func (s *store) GetChainState() (*postage.ChainState, error) {
	cs := &postage.ChainState{}
	err := s.store.Get(stateKey, cs)
	if err == storage.ErrNotFound {
		return &postage.ChainState{
			Block: 0,
			Total: big.NewInt(0),
			Price: big.NewInt(0),
		}, nil
	}

	return cs, err
}

// batchKey returns the index key for the batch ID used in the by-ID batch index.
func batchKey(id []byte) string {
	return batchKeyPrefix + string(id)
}
