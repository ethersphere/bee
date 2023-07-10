// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/pkg/swarm"
)

// ErrTxDone is returned by any operation that is performed on
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
	Batcher

	NewTx(*TxState) TxStore
}

// TxChunkStore represents a Tx ChunkStore where
// all operations are completed in a transaction.
type TxChunkStore interface {
	Tx
	ChunkStore

	NewTx(*TxState) TxChunkStore
}

// TxState is a mix-in for Tx. It provides basic
// functionality for transaction state lifecycle.
type TxState struct {
	// ctx lives for the life of the transaction.
	ctx context.Context

	// cancel is this context cancel function
	// that signals the end of this transaction.
	cancel context.CancelCauseFunc
}

// AwaitDone returns a channel that blocks until the context
// in TxState is canceled or the transaction is done.
func (tx *TxState) AwaitDone() <-chan struct{} {
	if tx == nil {
		c := make(chan struct{})
		close(c)
		return c
	}

	// Wait for either the transaction to be committed or rolled
	// back, or for the associated context to be closed.
	return tx.ctx.Done()
}

// IsDone returns ErrTxDone if the transaction has already been committed
// or rolled back. If the transaction was in progress and the context was
// canceled, it returns the context.Canceled error.
func (tx *TxState) IsDone() error {
	if tx == nil {
		return nil
	}

	return context.Cause(tx.ctx)
}

// Done marks this transaction as complete. It returns ErrTxDone if the
// transaction has already been committed or rolled back or if the transaction
// was in progress and the context was canceled, it returns the context.Canceled
// error.
func (tx *TxState) Done() error {
	if tx == nil {
		return nil
	}

	if tx.ctx.Err() == nil {
		tx.cancel(ErrTxDone)
		return nil
	}
	return context.Cause(tx.ctx)
}

// NewTxState is a convenient constructor for creating instances of TxState.
func NewTxState(ctx context.Context) *TxState {
	ctx, cancel := context.WithCancelCause(ctx)
	return &TxState{ctx: ctx, cancel: cancel}
}

var _ Store = (*TxStoreBase)(nil)
var _ Batcher = (*TxStoreBase)(nil)

// TxStoreBase implements the Store interface where
// the operations are guarded by a transaction.
type TxStoreBase struct {
	*TxState
	BatchedStore

	rolledBack atomic.Bool
}

// Close implements the Store interface.
// The operation is blocked until the
// transaction is not done.
func (s *TxStoreBase) Close() error {
	<-s.AwaitDone()
	return s.BatchedStore.Close()
}

// Get implements the Store interface.
func (s *TxStoreBase) Get(item Item) error {
	if err := s.IsDone(); err != nil {
		return err
	}
	return s.BatchedStore.Get(item)
}

// Has implements the Store interface.
func (s *TxStoreBase) Has(key Key) (bool, error) {
	if err := s.IsDone(); err != nil {
		return false, err
	}
	return s.BatchedStore.Has(key)
}

// GetSize implements the Store interface.
func (s *TxStoreBase) GetSize(key Key) (int, error) {
	if err := s.IsDone(); err != nil {
		return 0, err
	}
	return s.BatchedStore.GetSize(key)
}

// Iterate implements the Store interface.
func (s *TxStoreBase) Iterate(query Query, fn IterateFn) error {
	if err := s.IsDone(); err != nil {
		return err
	}
	return s.BatchedStore.Iterate(query, fn)
}

// Count implements the Store interface.
func (s *TxStoreBase) Count(key Key) (int, error) {
	if err := s.IsDone(); err != nil {
		return 0, err
	}
	return s.BatchedStore.Count(key)
}

// Put implements the Store interface.
func (s *TxStoreBase) Put(item Item) error {
	if err := s.IsDone(); err != nil {
		return err
	}
	return s.BatchedStore.Put(item)
}

// Delete implements the Store interface.
func (s *TxStoreBase) Delete(item Item) error {
	if err := s.IsDone(); err != nil {
		return err
	}
	return s.BatchedStore.Delete(item)
}

func (s *TxStoreBase) Batch(ctx context.Context) (Batch, error) {
	if err := s.IsDone(); err != nil {
		return nil, err
	}

	return s.BatchedStore.Batch(ctx)
}

// Rollback implements the TxStore interface.
func (s *TxStoreBase) Rollback() error {
	if s.rolledBack.CompareAndSwap(false, true) {
		if err := s.Done(); err == nil ||
			errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
	}
	return s.IsDone()
}

var _ ChunkStore = (*TxChunkStoreBase)(nil)

// TxChunkStoreBase implements the ChunkStore interface
// where the operations are guarded by a transaction.
type TxChunkStoreBase struct {
	*TxState
	ChunkStore

	rolledBack atomic.Bool
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
func (s *TxChunkStoreBase) Put(ctx context.Context, chunk swarm.Chunk) error {
	if err := s.IsDone(); err != nil {
		return err
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

// Rollback implements the TxChunkStore interface.
func (s *TxChunkStoreBase) Rollback() error {
	if s.rolledBack.CompareAndSwap(false, true) {
		if err := s.Done(); err == nil ||
			errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
	}
	return s.IsDone()
}

// TxOpCode represents code for tx operations.
type TxOpCode string

const (
	PutOp       TxOpCode = "put"
	PutCreateOp TxOpCode = "putCreate"
	PutUpdateOp TxOpCode = "putUpdate"
	DeleteOp    TxOpCode = "delete"
)

// TxRevertOp represents a reverse operation.
type TxRevertOp[K, V any] struct {
	Origin   TxOpCode
	ObjectID string

	Key K
	Val V
}

// TxRevertFn represents a function that can be invoked
// to reverse the operation that was performed by the
// corresponding TxOpCode.
type TxRevertFn[K, V any] func(K, V) error

// TxRevertOpStore represents a store for TxRevertOp.
type TxRevertOpStore[K, V any] interface {
	// Append appends a Revert operation to the store.
	Append(...*TxRevertOp[K, V]) error
	// Revert executes all the revere operations
	// in the store in reverse order.
	Revert() error
	// Clean cleans the store.
	Clean() error
}

// NoOpTxRevertOpStore is a no-op implementation of TxRevertOpStore.
type NoOpTxRevertOpStore[K, V any] struct{}

func (s *NoOpTxRevertOpStore[K, V]) Append(...*TxRevertOp[K, V]) error { return nil }
func (s *NoOpTxRevertOpStore[K, V]) Revert() error                     { return nil }
func (s *NoOpTxRevertOpStore[K, V]) Clean() error                      { return nil }

// InMemTxRevertOpStore is an in-memory implementation of TxRevertOpStore.
type InMemTxRevertOpStore[K, V any] struct {
	revOpsFn map[TxOpCode]TxRevertFn[K, V]

	mu  sync.Mutex
	ops []*TxRevertOp[K, V]
}

// Append implements TxRevertOpStore.
func (s *InMemTxRevertOpStore[K, V]) Append(ops ...*TxRevertOp[K, V]) error {
	if s == nil || len(ops) == 0 {
		return nil
	}

	s.mu.Lock()
	s.ops = append(s.ops, ops...)
	s.mu.Unlock()
	return nil
}

// Revert implements TxRevertOpStore.
func (s *InMemTxRevertOpStore[K, V]) Revert() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var errs error
	for i := len(s.ops) - 1; i >= 0; i-- {
		op := s.ops[i]
		if op == nil {
			continue
		}
		if fn, ok := s.revOpsFn[op.Origin]; !ok {
			errs = errors.Join(errs, fmt.Errorf(
				"revert operation %q for object %s not found",
				op.Origin,
				op.ObjectID,
			))
		} else if err := fn(op.Key, op.Val); err != nil {
			errs = errors.Join(errs, fmt.Errorf(
				"revert operation %q for object %s failed: %w",
				op.Origin,
				op.ObjectID,
				err,
			))
		}
	}
	s.ops = nil
	return errs
}

// Clean implements TxRevertOpStore.
func (s *InMemTxRevertOpStore[K, V]) Clean() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	s.ops = nil
	s.mu.Unlock()
	return nil
}

// NewInMemTxRevertOpStore is a convenient constructor for creating instances of
// InMemTxRevertOpStore. The revOpsFn map is used to look up the revert function
// for a given TxOpCode.
func NewInMemTxRevertOpStore[K, V any](revOpsFn map[TxOpCode]TxRevertFn[K, V]) *InMemTxRevertOpStore[K, V] {
	return &InMemTxRevertOpStore[K, V]{revOpsFn: revOpsFn}
}
