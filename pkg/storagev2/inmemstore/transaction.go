// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmem

import (
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/storagev2"
	"github.com/hashicorp/go-multierror"
)

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
	key storage.Key

	// fn is the inverse operation to the origin.
	fn func() error
}

var _ storage.TxStore = (*TxStore)(nil)

// TxStore is an implementation of in-memory Store
// where all Store operations are done in a transaction.
type TxStore struct {
	*storage.TxStoreBase

	// Bookkeeping of invasive operations executed
	// on the Store to support rollback functionality.
	opsMu sync.Mutex
	ops   []op
}

// Put implements the Store interface.
func (s *TxStore) Put(item storage.Item) error {
	err := s.TxStoreBase.Put(item)
	if err == nil {
		s.opsMu.Lock()
		s.ops = append(s.ops, op{putOp, item, func() error {
			return s.TxStoreBase.Store.Delete(item)
		}})
		s.opsMu.Unlock()
	}
	return err
}

// Delete implements the Store interface.
func (s *TxStore) Delete(item storage.Item) error {
	err := s.TxStoreBase.Delete(item)
	if err == nil {
		s.opsMu.Lock()
		s.ops = append(s.ops, op{deleteOp, item, func() error {
			return s.TxStoreBase.Store.Put(item)
		}})
		s.opsMu.Unlock()
	}
	return err
}

// Commit implements the Tx interface.
func (s *TxStore) Commit() error {
	if err := s.IsDone(); err != nil {
		return err
	}
	s.TxState.Done()
	return nil
}

// Rollback implements the Tx interface.
func (s *TxStore) Rollback() error {
	if err := s.IsDone(); err != nil {
		return err
	}
	var opErrors *multierror.Error
	for _, op := range s.ops {
		if err := op.fn(); err != nil {
			err = fmt.Errorf(
				"inmemstore: unable to rollback operation %q for item %s/%s: %w",
				op.origin,
				op.key.Namespace(),
				op.key.ID(),
				err,
			)
			opErrors = multierror.Append(opErrors, err)
		}
	}
	s.TxState.Done()
	return opErrors.ErrorOrNil()
}

// NewTxStore returns an implementation of in-memory Store
// where all Store operations are done in a transaction.
func NewTxStore(base *storage.TxStoreBase) *TxStore {
	return &TxStore{TxStoreBase: base}
}
