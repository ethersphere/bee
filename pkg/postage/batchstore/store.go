// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
)

const (
	batchKeyPrefix = "batchKeyPrefix"
	// valueKeyPrefix = "valueKeyPrefix"
	stateKey = "stateKey"
)

// Store is a local store for postage batches
type Store struct {
	store storage.StateStorer // State store backend to persist batches
}

// New constructs a new postage batch store
func New(store storage.StateStorer) postage.BatchStorer {
	return &Store{store}
}

// Get returns a batch from the batchstore with the given ID
func (s *Store) Get(id []byte) (*postage.Batch, error) {
	var b postage.Batch
	err := s.store.Get(batchKey(id), &b)
	return &b, err
}

// Put stores a given batch in the batchstore with the given ID
func (s *Store) Put(b *postage.Batch) error {
	return s.store.Put(batchKey(b.ID), b)
}

// PutChainState implements BatchStorer. It stores the chain state in the batch
// store
func (s *Store) PutChainState(state *postage.ChainState) error {
	return s.store.Put(stateKey, state)
}

// GetChainState implements BatchStorer. It returns the stored chain state from
// the batch store
func (s *Store) GetChainState() (*postage.ChainState, error) {
	var cs postage.ChainState
	err := s.store.Get(stateKey, &cs)
	return &cs, err
}

// batchKey returns the index key for the batch ID used in the by-ID batch index
func batchKey(id []byte) string {
	return batchKeyPrefix + string(id)
}
