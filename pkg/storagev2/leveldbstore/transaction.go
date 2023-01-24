// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/storagev2"
	"github.com/hashicorp/go-multierror"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// TODO: factor out the common parts of the op struct and the op functionality.

// opCode represents code for Store operations.
type opCode string

const (
	putCreateOp opCode = "putCreate"
	putUpdateOp opCode = "putUpdate"
	deleteOp    opCode = "delete"
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
	batch *leveldb.Batch
	opsMu sync.Mutex
	ops   []op
}

// Put implements the Store interface.
func (s *TxStore) Put(item storage.Item) error {
	prev := item.Clone()

	var reverseOp op
	switch err := s.TxStoreBase.Get(prev); {
	case errors.Is(err, storage.ErrNotFound):
		reverseOp = op{putCreateOp, item, func() error {
			s.batch.Delete(key(item))
			return nil
		}}
	case err != nil:
		return err
	default:
		reverseOp = op{putUpdateOp, prev, func() error {
			val, err := prev.Marshal()
			if err != nil {
				return err
			}
			s.batch.Put(key(prev), val)
			return nil
		}}
	}

	err := s.TxStoreBase.Put(item)
	if err == nil {
		s.opsMu.Lock()
		s.ops = append(s.ops, reverseOp)
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
			val, err := item.Marshal()
			if err != nil {
				return err
			}
			s.batch.Put(key(item), val)
			return nil
		}})
		s.opsMu.Unlock()
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

	var opErrors *multierror.Error
	for i := len(s.ops) - 1; i >= 0; i-- {
		op := s.ops[i]
		if err := op.fn(); err != nil {
			err = fmt.Errorf(
				"leveldbstore: unable to rollback operation %q for item %s/%s: %w",
				op.origin,
				op.key.Namespace(),
				op.key.ID(),
				err,
			)
			opErrors = multierror.Append(opErrors, err)
		}
	}

	db := s.TxStoreBase.Store.(*Store).db
	if err := db.Write(s.batch, &opt.WriteOptions{Sync: true}); err != nil {
		return fmt.Errorf("leveldbstore: failed to write rollback batch: %w", err)
	}
	return opErrors.ErrorOrNil()
}

// NewTx implements the TxStore interface.
func (s *TxStore) NewTx(state *storage.TxState) storage.TxStore {
	return &TxStore{
		TxStoreBase: &storage.TxStoreBase{
			TxState: state,
			Store:   s.TxStoreBase.Store,
		},
		batch: new(leveldb.Batch),
	}
}

// NewTxStore returns a new TxStore instance backed by the given store.
func NewTxStore(store storage.Store) *TxStore {
	return &TxStore{TxStoreBase: &storage.TxStoreBase{Store: store}}
}
