// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemchunkstore

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ storage.TxChunkStore = (*TxChunkStore)(nil)

// TxChunkStore is an implementation of in-memory Store
// where all Store operations are done in a transaction.
type TxChunkStore struct {
	*storage.TxChunkStoreBase

	// Bookkeeping of invasive operations executed
	// on the ChunkStore to support rollback functionality.
	revOps storage.TxRevStack
}

// Put implements the Store interface.
func (s *TxChunkStore) Put(ctx context.Context, chunk swarm.Chunk) (err error) {
	err = s.TxChunkStoreBase.Put(ctx, chunk)
	if err == nil {
		s.revOps.Append(&storage.TxRevertOp{
			Origin:   storage.PutOp,
			ObjectID: chunk.Address().String(),
			Revert: func() error {
				return s.TxChunkStoreBase.ChunkStore.Delete(ctx, chunk.Address())
			},
		})
	}
	return err
}

// Delete implements the Store interface.
func (s *TxChunkStore) Delete(ctx context.Context, addr swarm.Address) error {
	chunk, err := s.Get(ctx, addr)
	if err != nil {
		return err
	}
	err = s.TxChunkStoreBase.Delete(ctx, addr)
	if err == nil {
		s.revOps.Append(&storage.TxRevertOp{
			Origin:   storage.DeleteOp,
			ObjectID: addr.String(),
			Revert: func() error {
				return s.TxChunkStoreBase.ChunkStore.Put(ctx, chunk)
			},
		})
	}
	return err
}

// Commit implements the Tx interface.
func (s *TxChunkStore) Commit() error {
	return s.TxState.Done()
}

// Rollback implements the Tx interface.
func (s *TxChunkStore) Rollback() error {
	if err := s.TxState.Done(); err != nil {
		return err
	}

	if err := s.revOps.Revert(); err != nil {
		return fmt.Errorf("inmemchunkstore: unable to rollback: %w", err)
	}
	return nil
}

// NewTx implements the TxStore interface.
func (s *TxChunkStore) NewTx(state *storage.TxState) storage.TxChunkStore {
	return &TxChunkStore{
		TxChunkStoreBase: &storage.TxChunkStoreBase{
			TxState:    state,
			ChunkStore: s.ChunkStore,
		},
	}
}

// NewTxChunkStore returns a new TxChunkStore instance backed by the given chunk store.
func NewTxChunkStore(store storage.ChunkStore) *TxChunkStore {
	return &TxChunkStore{TxChunkStoreBase: &storage.TxChunkStoreBase{ChunkStore: store}}
}
