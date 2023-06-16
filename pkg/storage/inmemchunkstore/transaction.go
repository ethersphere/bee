// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemchunkstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ storage.TxChunkStore = (*TxChunkStore)(nil)

// TxChunkStore is an implementation of in-memory Store
// where all Store operations are done in a transaction.
type TxChunkStore struct {
	*storage.TxChunkStoreBase

	// Bookkeeping of invasive operations executed
	// on the ChunkStore to support rollback functionality.
	revOps storage.TxRevertOpStore[swarm.Address, swarm.Chunk]
}

// release releases the TxStore transaction associated resources.
func (s *TxChunkStore) release() {
	s.TxChunkStoreBase.ChunkStore = nil
	s.revOps = nil
}

// Put implements the Store interface.
func (s *TxChunkStore) Put(ctx context.Context, chunk swarm.Chunk) (err error) {
	err = s.TxChunkStoreBase.Put(ctx, chunk)
	if err == nil {
		err = s.revOps.Append(&storage.TxRevertOp[swarm.Address, swarm.Chunk]{
			Origin:   storage.PutOp,
			ObjectID: chunk.Address().String(),
			Key:      chunk.Address(),
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
		err = s.revOps.Append(&storage.TxRevertOp[swarm.Address, swarm.Chunk]{
			Origin:   storage.DeleteOp,
			ObjectID: addr.String(),
			Val:      chunk,
		})
	}
	return err
}

// Commit implements the Tx interface.
func (s *TxChunkStore) Commit() error {
	defer s.release()

	return s.TxState.Done()
}

// Rollback implements the Tx interface.
func (s *TxChunkStore) Rollback() error {
	defer s.release()

	if err := s.TxChunkStoreBase.Rollback(); err != nil {
		return err
	}

	if err := s.revOps.Revert(); err != nil {
		return fmt.Errorf("inmemchunkstore: unable to rollback: %w", err)
	}
	return nil
}

// NewTx implements the TxStore interface.
func (s *TxChunkStore) NewTx(state *storage.TxState) storage.TxChunkStore {
	if s.ChunkStore == nil {
		panic(errors.New("inmemchunkstore: nil store"))
	}

	return &TxChunkStore{
		TxChunkStoreBase: &storage.TxChunkStoreBase{
			TxState:    state,
			ChunkStore: s.ChunkStore,
		},
		revOps: storage.NewInMemTxRevertOpStore(
			map[storage.TxOpCode]storage.TxRevertFn[swarm.Address, swarm.Chunk]{
				storage.PutOp: func(address swarm.Address, _ swarm.Chunk) error {
					return s.ChunkStore.Delete(context.Background(), address)
				},
				storage.DeleteOp: func(_ swarm.Address, chunk swarm.Chunk) error {
					return s.ChunkStore.Put(context.Background(), chunk)
				},
			},
		),
	}
}

// NewTxChunkStore returns a new TxChunkStore instance backed by the given chunk store.
func NewTxChunkStore(store storage.ChunkStore) *TxChunkStore {
	return &TxChunkStore{TxChunkStoreBase: &storage.TxChunkStoreBase{ChunkStore: store}}
}
