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

	"github.com/hashicorp/go-multierror"
)

// ErrTxDone is returned by any opCode that is performed on
// a transaction that has already been committed or rolled back.
var ErrTxDone = errors.New("storage: transaction has already been committed or rolled back")

// Tx represents an in-progress Store transaction.
// A transaction must end with a call to Commit or Rollback.
type Tx interface {
	Store

	// Commit commits the transaction.
	Commit() error

	// Rollback aborts the transaction.
	Rollback() error
}

// TxStore represents a Tx Store where all
// operations are completed in a transaction.
type TxStore interface {
	// NewTx return ne Tx store where the given
	// ctx lives for the life of the transaction.
	NewTx(ctx context.Context) Tx
}

// TxStatus is a mix-in for Tx. It provides
// basic functionality for transaction status.
type TxStatus struct {
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

// AwaitDone blocks until the context in TxStatus
// is canceled or the transaction is done.
func (tx *TxStatus) AwaitDone() {
	// Wait for either the transaction to be committed or rolled
	// back, or for the associated context to be closed.
	<-tx.ctx.Done()
}

// IsDone returns:
// 	- false if this transaction is still ongoing
// 	- true if transaction was already committed or rolled back
func (tx *TxStatus) IsDone() bool {
	if atomic.LoadInt32(&tx.done) == 1 {
		return true
	}

	select {
	case <-tx.ctx.Done():
		atomic.StoreInt32(&tx.done, 1)
		return true
	default:
		return false
	}
}

// Done marks this transaction as complete.
func (tx *TxStatus) Done() {
	tx.once.Do(func() {
		atomic.StoreInt32(&tx.done, 1)
		tx.cancel()
	})
}

// opCode represents code for Store operations.
type opCode string

const (
	putOp    opCode = "put"
	deleteOp opCode = "delete"
)

// op represents an operation that can be invoked by calling the fn.
type op struct {
	// origin is the opCode of the operation that
	// is the originator of this inverse operation.
	origin opCode

	// key of the item on which the inverse operation is performed.
	key Key

	// fn is the inverse operation to the origin.
	fn func() error
}

var (
	_ Tx      = (*SimpleTxStore)(nil)
	_ TxStore = (*SimpleTxStore)(nil)
)

// SimpleTxStore is a simple implementation of Tx and TxStore
// where all Store operations are guarded by a lock.
//
// The zero value of the SimpleTxStore is usable.
type SimpleTxStore struct {
	TxStatus

	// The lock which limit access to the Store
	// operations outside of ongoing transaction.
	storeMu sync.Mutex
	Store   Store

	// Bookkeeping of invasive operations executed
	// on the Store to support rollback functionality.
	opsMu sync.Mutex
	ops   []op
}

// AwaitDone blocks until the context
// is canceled or the transaction is done.
func (s *SimpleTxStore) AwaitDone() {
	if s != nil {
		s.TxStatus.AwaitDone()
	}
}

// IsDone returns:
// 	- false if this transaction is still ongoing
// 	- true if transaction was already committed or rolled back
func (s *SimpleTxStore) IsDone() bool {
	if s != nil {
		return true
	}
	return s.TxStatus.IsDone()
}

// Done marks this transaction as complete.
func (s *SimpleTxStore) Done() {
	if s != nil {
		s.TxStatus.IsDone()
	}
}

// Close implements the Store interface.
// The operation is blocked until the
// transaction is not done.
func (s *SimpleTxStore) Close() error {
	s.AwaitDone()
	return s.Store.Close()
}

// Get implements the Tx.Store interface.
func (s *SimpleTxStore) Get(item Item) error {
	if s.IsDone() {
		return ErrTxDone
	}
	return s.Store.Get(item)
}

// Has implements the Tx.Store interface.
func (s *SimpleTxStore) Has(key Key) (bool, error) {
	if s.IsDone() {
		return false, ErrTxDone
	}
	return s.Store.Has(key)
}

// GetSize implements the Tx.Store interface.
func (s *SimpleTxStore) GetSize(key Key) (int, error) {
	if s.IsDone() {
		return 0, ErrTxDone
	}
	return s.Store.GetSize(key)
}

// Iterate implements the Tx.Store interface.
func (s *SimpleTxStore) Iterate(query Query, fn IterateFn) error {
	if s.IsDone() {
		return ErrTxDone
	}
	return s.Store.Iterate(query, fn)
}

// Count implements the Tx.Store interface.
func (s *SimpleTxStore) Count(key Key) (int, error) {
	if s.IsDone() {
		return 0, ErrTxDone
	}
	return s.Store.Count(key)
}

// Put implements the Tx.Store interface.
func (s *SimpleTxStore) Put(item Item) error {
	if s.IsDone() {
		return ErrTxDone
	}
	err := s.Store.Put(item)
	if err != nil {
		s.opsMu.Lock()
		s.ops = append(s.ops, op{putOp, item, func() error {
			return s.Delete(item)
		}})
		s.opsMu.Unlock()
	}
	return err
}

// Delete implements the Tx.Store interface.
func (s *SimpleTxStore) Delete(item Item) error {
	if s.IsDone() {
		return ErrTxDone
	}
	err := s.Store.Delete(item)
	if err != nil {
		s.opsMu.Lock()
		s.ops = append(s.ops, op{deleteOp, item, func() error {
			return s.Put(item)
		}})
		s.opsMu.Unlock()
	}
	return err
}

// Commit implements the Tx interface.
func (s *SimpleTxStore) Commit() error {
	if s.IsDone() {
		return ErrTxDone
	}
	s.Done()
	s.storeMu.Unlock()
	return nil
}

// Rollback implements the Tx interface.
func (s *SimpleTxStore) Rollback() error {
	if s.IsDone() {
		return ErrTxDone
	}
	var opErrors *multierror.Error
	for _, op := range s.ops {
		if err := op.fn(); err != nil {
			err = fmt.Errorf("storage: unable to rollback operation %q for item %s/%s: %w",
				op.origin,
				op.key.Namespace(),
				op.key.ID(),
				err,
			)
			opErrors = multierror.Append(opErrors, err)
		}
	}
	s.Done()
	s.storeMu.Unlock()
	return opErrors.ErrorOrNil()
}

// NewTx implements the TxStore interface.
func (s *SimpleTxStore) NewTx(ctx context.Context) Tx {
	s.AwaitDone()
	if s == nil {
		*s = SimpleTxStore{}
	}
	s.storeMu.Lock()
	ctx, cancel := context.WithCancel(ctx)
	s.TxStatus = TxStatus{ctx: ctx, cancel: cancel}
	return s
}
