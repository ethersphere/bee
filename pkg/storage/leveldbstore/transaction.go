// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore

import (
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var _ storage.TxStore = (*TxStore)(nil)

// TxStore is an implementation of in-memory Store
// where all Store operations are done in a transaction.
type TxStore struct {
	*storage.TxStoreBase

	// Bookkeeping of invasive operations executed
	// on the Store to support rollback functionality.
	batch  *leveldb.Batch
	revOps *storage.TxRevertStack[[]byte, []byte]
}

// release releases the TxStore transaction associated resources.
func (s *TxStore) release() {
	s.TxStoreBase.Store = nil
	s.batch = nil
	s.revOps = nil
}

// Put implements the Store interface.
func (s *TxStore) Put(item storage.Item) error {
	if err := s.IsDone(); err != nil {
		return err
	}

	prev := item.Clone()
	var reverseOp *storage.TxRevertOp[[]byte, []byte]
	switch err := s.TxStoreBase.Get(prev); {
	case errors.Is(err, storage.ErrNotFound):
		reverseOp = &storage.TxRevertOp[[]byte, []byte]{
			Origin:   storage.PutCreateOp,
			ObjectID: item.String(),
			Key:      key(item),
		}
	case err != nil:
		return err
	default:
		val, err := prev.Marshal()
		if err != nil {
			return err
		}
		reverseOp = &storage.TxRevertOp[[]byte, []byte]{
			Origin:   storage.PutUpdateOp,
			ObjectID: prev.String(),
			Key:      key(prev),
			Val:      val,
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
		val, err := item.Marshal()
		if err != nil {
			return err
		}
		s.revOps.Append(&storage.TxRevertOp[[]byte, []byte]{
			Origin:   storage.DeleteOp,
			ObjectID: item.String(),
			Key:      key(item),
			Val:      val,
		})
	}
	return err
}

// Commit implements the Tx interface.
func (s *TxStore) Commit() error {
	defer s.release()

	return s.TxState.Done()
}

// Rollback implements the Tx interface.
func (s *TxStore) Rollback() error {
	defer s.release()

	if err := s.TxStoreBase.Rollback(); err != nil {
		return err
	}

	var errs error
	if err := s.revOps.Revert(); err != nil {
		errs = errors.Join(errs, fmt.Errorf("leveldbstore: unable to rollback: %w", err))
	}

	db := s.TxStoreBase.Store.(*Store).db
	if err := db.Write(s.batch, &opt.WriteOptions{Sync: true}); err != nil {
		errs = errors.Join(errs, fmt.Errorf("leveldbstore: unable to write rollback batch: %w", err))
	}
	return errs
}

// NewTx implements the TxStore interface.
func (s *TxStore) NewTx(state *storage.TxState) storage.TxStore {
	if s.Store == nil {
		panic(errors.New("leveldbstore: nil store"))
	}

	batch := new(leveldb.Batch)
	return &TxStore{
		TxStoreBase: &storage.TxStoreBase{
			TxState: state,
			Store:   s.Store,
		},
		// - Create rev-pending tx and update the serialized batch in the DB on every TxRevertStack.Append call
		// - On Commit, delete the serialized batch from the rev-pending tx
		// - On Rollback, write the batch to the DB and delete the serialized batch from the rev-pending tx (in batch)
		// - On start check the rev-pending tx for serialized batch and apply it to the DB
		batch: batch,
		revOps: storage.NewTxRevertStack[[]byte, []byte](
			new(storage.InMemTxRevertOpStore[[]byte, []byte]),
			map[storage.TxOpCode]storage.TxRevertFn[[]byte, []byte]{
				storage.PutCreateOp: func(key, val []byte) error {
					batch.Delete(key)
					return nil
				},
				storage.PutUpdateOp: func(k, v []byte) error {
					batch.Put(k, v)
					return nil
				},
				storage.DeleteOp: func(k, v []byte) error {
					batch.Put(k, v)
					return nil
				},
			},
		),
	}
}

// NewTxStore returns a new TxStore instance backed by the given store.
func NewTxStore(store storage.Store) *TxStore {
	return &TxStore{TxStoreBase: &storage.TxStoreBase{Store: store}}
}
