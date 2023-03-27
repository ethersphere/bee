// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemstore

import (
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/storage"
)

var _ storage.TxStore = (*TxStore)(nil)

// TxStore is an implementation of in-memory Store
// where all Store operations are done in a transaction.
type TxStore struct {
	*storage.TxStoreBase

	// Bookkeeping of invasive operations executed
	// on the Store to support rollback functionality.
	revOps storage.TxRevStack
}

// Put implements the Store interface.
func (s *TxStore) Put(item storage.Item) error {
	prev := item.Clone()

	var reverseOp *storage.TxRevertOp
	switch err := s.TxStoreBase.Get(prev); {
	case errors.Is(err, storage.ErrNotFound):
		reverseOp = &storage.TxRevertOp{
			Origin:   storage.PutCreateOp,
			ObjectID: item.String(),
			Revert: func() error {
				return s.TxStoreBase.Store.Delete(item)
			},
		}
	case err != nil:
		return err
	default:
		reverseOp = &storage.TxRevertOp{
			Origin:   storage.PutUpdateOp,
			ObjectID: prev.String(),
			Revert: func() error {
				return s.TxStoreBase.Store.Put(prev)
			},
		}
	}

	err := s.TxStoreBase.Put(item)
	if err == nil {
		s.revOps.Append(reverseOp)
	}
	return err
}

// Delete implements the Store interface.
func (s *TxStore) Delete(item storage.Item) error {
	err := s.TxStoreBase.Delete(item)
	if err == nil {
		s.revOps.Append(&storage.TxRevertOp{
			Origin:   storage.DeleteOp,
			ObjectID: item.String(),
			Revert: func() error {
				return s.TxStoreBase.Store.Put(item)
			},
		})
	}
	return err
}

// Commit implements the Tx interface.
func (s *TxStore) Commit() error {
	return s.TxState.Done()
}

// Rollback implements the Tx interface.
func (s *TxStore) Rollback() error {
	if err := s.TxState.Done(); err != nil {
		return err
	}

	if err := s.revOps.Revert(); err != nil {
		return fmt.Errorf("inmemstore: unable to rollback: %w", err)
	}
	return nil
}

// NewTx implements the TxStore interface.
func (s *TxStore) NewTx(state *storage.TxState) storage.TxStore {
	return &TxStore{
		TxStoreBase: &storage.TxStoreBase{
			TxState: state,
			Store:   s.Store,
		},
	}
}

// NewTxStore returns a new TxStore instance backed by the given store.
func NewTxStore(store storage.Store) *TxStore {
	return &TxStore{TxStoreBase: &storage.TxStoreBase{Store: store}}
}
