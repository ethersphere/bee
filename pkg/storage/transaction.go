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

// TxOpCode represents code for tx operations.
type TxOpCode string

const (
	PutOp       TxOpCode = "put"
	PutCreateOp TxOpCode = "putCreate"
	PutUpdateOp TxOpCode = "putUpdate"
	DeleteOp    TxOpCode = "delete"
)

// TxRevertFn represents a function that can be invoked
// to reverse the operation that was performed by the
// corresponding TxOpCode.
type TxRevertFn[K, V any] func(K, V) error

// TxRevertOp represents a reverse operation.
type TxRevertOp[K, V any] struct {
	Origin   TxOpCode
	ObjectID string

	Key K
	Val V
}

// TxRevertStack tracks reverse operations.
type TxRevertStack[K, V any] struct {
	revOpsFn map[TxOpCode]TxRevertFn[K, V]

	mu  sync.Mutex
	ops []*TxRevertOp[K, V]
}

// Append appends a Revert operation to the stack.
func (rs *TxRevertStack[K, V]) Append(op *TxRevertOp[K, V]) {
	if rs == nil {
		return
	}

	rs.mu.Lock()
	rs.ops = append(rs.ops, op)
	rs.mu.Unlock()
}

// Revert executes all the revere operations in the stack in reverse order.
// If an error occurs during the call to the Revert operation, this error
// is captured and execution continues to the top of the stack.
func (rs *TxRevertStack[K, V]) Revert() error {
	if rs == nil {
		return nil
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()

	var errs error
	for i := len(rs.ops) - 1; i >= 0; i-- {
		op := rs.ops[i]
		fn, ok := rs.revOpsFn[op.Origin]
		if !ok {
			errs = errors.Join(errs, fmt.Errorf(
				"revert operation %q for object %s not found",
				op.Origin,
				op.ObjectID,
			))
			continue
		}
		if err := fn(op.Key, op.Val); err != nil {
			errs = errors.Join(errs, fmt.Errorf(
				"revert operation %q for object %s failed: %w",
				op.Origin,
				op.ObjectID,
				err,
			))
		}
	}
	return errs
}

// NewTxRevertStack is a convenient constructor for creating instances of TxRevertStack.
// The revOpsFn map is used to lookup the revert function for a given TxOpCode.
func NewTxRevertStack[K, V any](
	// TODO: add writer for persisting revert ops
	revOpsFn map[TxOpCode]TxRevertFn[K, V],
) *TxRevertStack[K, V] {
	return &TxRevertStack[K, V]{revOpsFn: revOpsFn}
}

var _ Store = (*TxStoreBase)(nil)

// TxStoreBase implements the Store interface where
// the operations are guarded by a transaction.
type TxStoreBase struct {
	*TxState
	Store

	rolledBack atomic.Bool
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
