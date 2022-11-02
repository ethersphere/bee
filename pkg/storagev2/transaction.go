// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/pkg/swarm"
)

// ErrTxDone is returned by any opCode that is performed on
// a transaction that has already been committed or rolled back.
var ErrTxDone = errors.New("storage: transaction has already been committed or rolled back")

// Tx represents an in-progress Store transaction.
// A transaction must end with a call to Commit or Rollback.
type Tx interface {
	// Commit commits the transaction.
	Commit() error

	// Rollback aborts the transaction.
	Rollback() error
}

// TxStore represents a Tx Store where all
// operations are completed in a transaction.
type TxStore interface {
	Tx
	Store
}

// TxChunkStore represents a Tx ChunkStore where
// all operations are completed in a transaction.
type TxChunkStore interface {
	Tx
	ChunkStore
}

// TxState is a mix-in for Tx. It provides basic
// functionality for transaction state lifecycle.
type TxState struct {
	// once guards cancel and done invariants.
	once sync.Once

	// ctx lives for the life of the transaction.
	ctx context.Context

	// cancel is this context cancel function
	// that signals the end of this transaction.
	cancel context.CancelFunc

	// done transitions from 0 to 1 exactly once, on Commit or Rollback.
	// Once done, all operations should fail with ErrTxDone.
	// The value is treated as a sync/atomic int32.
	done int32
}

// TODO: consider some TxState method private.

// AwaitDone returns a channel that blocks until the context
// in TxState is canceled or the transaction is done.
func (tx *TxState) AwaitDone() <-chan struct{} {
	// Wait for either the transaction to be committed or rolled
	// back, or for the associated context to be closed.
	return tx.ctx.Done()
}

// IsDone returns ErrTxDone if the transaction has already been committed
// or rolled back. If the transaction is still in progress and the context
// is finished, it returns a context error.
func (tx *TxState) IsDone() error {
	select {
	default:
	case <-tx.ctx.Done():
		if atomic.LoadInt32(&tx.done) == 1 {
			return ErrTxDone
		}
		return tx.ctx.Err()
	}
	return nil
}

// Done marks this transaction as complete.
func (tx *TxState) Done() {
	tx.once.Do(func() {
		atomic.StoreInt32(&tx.done, 1)
		tx.cancel()
	})
}

// NewTxState is a convenient constructor for creating instances of TxState.
func NewTxState(ctx context.Context) *TxState {
	ctx, cancel := context.WithCancel(ctx)
	return &TxState{ctx: ctx, cancel: cancel}
}

var _ Store = (*TxStoreBase)(nil)

// TxStoreBase implements the Store interface where
// the operations are guarded by a transaction.
type TxStoreBase struct {
	*TxState
	Store
}

// Close implements the Store interface.
// The operation is blocked until the
// transaction is not done.
func (s *TxStoreBase) Close() error {
	<-s.AwaitDone()
	return s.Store.Close()
}

// Get implements the Store interface.
func (s *TxStoreBase) Get(item Item) error {
	if err := s.IsDone(); err != nil {
		return err
	}
	return s.Store.Get(item)
}

// Has implements the Store interface.
func (s *TxStoreBase) Has(key Key) (bool, error) {
	if err := s.IsDone(); err != nil {
		return false, err
	}
	return s.Store.Has(key)
}

// GetSize implements the Store interface.
func (s *TxStoreBase) GetSize(key Key) (int, error) {
	if err := s.IsDone(); err != nil {
		return 0, err
	}
	return s.Store.GetSize(key)
}

// Iterate implements the Store interface.
func (s *TxStoreBase) Iterate(query Query, fn IterateFn) error {
	if err := s.IsDone(); err != nil {
		return err
	}
	return s.Store.Iterate(query, fn)
}

// Count implements the Store interface.
func (s *TxStoreBase) Count(key Key) (int, error) {
	if err := s.IsDone(); err != nil {
		return 0, err
	}
	return s.Store.Count(key)
}

// Put implements the Store interface.
func (s *TxStoreBase) Put(item Item) error {
	if err := s.IsDone(); err != nil {
		return err
	}
	return s.Store.Put(item)
}

// Delete implements the Store interface.
func (s *TxStoreBase) Delete(item Item) error {
	if err := s.IsDone(); err != nil {
		return err
	}
	return s.Store.Delete(item)
}

var _ ChunkStore = (*TxChunkStoreBase)(nil)

// TxChunkStoreBase implements the ChunkStore interface
// where the operations are guarded by a transaction.
type TxChunkStoreBase struct {
	*TxState
	ChunkStore
}

// Close implements the ChunkStore interface.
// The operation is blocked until the
// transaction is not done.
func (s *TxChunkStoreBase) Close() error {
	<-s.AwaitDone()
	return s.ChunkStore.Close()
}

// Get implements the ChunkStore interface.
func (s *TxChunkStoreBase) Get(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
	if err := s.IsDone(); err != nil {
		return nil, err
	}
	return s.ChunkStore.Get(ctx, address)
}

// Put implements the ChunkStore interface.
func (s *TxChunkStoreBase) Put(ctx context.Context, chunk swarm.Chunk) (bool, error) {
	if err := s.IsDone(); err != nil {
		return false, err
	}
	return s.ChunkStore.Put(ctx, chunk)
}

// Iterate implements the ChunkStore interface.
func (s *TxChunkStoreBase) Iterate(ctx context.Context, fn IterateChunkFn) error {
	if err := s.IsDone(); err != nil {
		return err
	}
	return s.ChunkStore.Iterate(ctx, fn)
}

// Has implements the ChunkStore interface.
func (s *TxChunkStoreBase) Has(ctx context.Context, address swarm.Address) (bool, error) {
	if err := s.IsDone(); err != nil {
		return false, err
	}
	return s.ChunkStore.Has(ctx, address)
}

// Delete implements the ChunkStore interface.
func (s *TxChunkStoreBase) Delete(ctx context.Context, address swarm.Address) error {
	if err := s.IsDone(); err != nil {
		return err
	}
	return s.ChunkStore.Delete(ctx, address)
}
