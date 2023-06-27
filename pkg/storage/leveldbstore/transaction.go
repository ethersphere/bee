// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storageutil"
	"github.com/google/uuid"
	"github.com/syndtr/goleveldb/leveldb"
)

var _ storage.TxRevertOpStore[[]byte, []byte] = (*txRevertOpStore)(nil)

// txRevertOpStore is a storage.TxRevertOpStore that
// stores revert operations in the LevelDB instance.
type txRevertOpStore struct {
	id       []byte
	db       *leveldb.DB
	batch    *leveldb.Batch
	batchMu  sync.Mutex
	revOpsFn map[storage.TxOpCode]storage.TxRevertFn[[]byte, []byte]
}

// Append implements storage.TxRevertOpStore.
func (s *txRevertOpStore) Append(op *storage.TxRevertOp[[]byte, []byte]) error {
	if s == nil || op == nil {
		return nil
	}

	s.batchMu.Lock()
	defer s.batchMu.Unlock()

	var errs error
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
	if errs != nil {
		return errs
	}

	return s.db.Put(s.id, s.batch.Dump(), nil)
}

// Revert implements storage.TxRevertOpStore.
func (s *txRevertOpStore) Revert() error {
	if s == nil {
		return nil
	}

	s.batchMu.Lock()
	defer s.batchMu.Unlock()
	defer s.batch.Reset()

	s.batch.Delete(s.id)
	return s.db.Write(s.batch, nil)
}

// Clean implements storage.TxRevertOpStore.
func (s *txRevertOpStore) Clean() error {
	if s == nil {
		return nil
	}

	return s.db.Delete(s.id, nil)
}

var (
	_ storage.TxStore   = (*TxStore)(nil)
	_ storage.Recoverer = (*TxStore)(nil)
)

// TxStore is an implementation of in-memory Store
// where all Store operations are done in a transaction.
type TxStore struct {
	*storage.TxStoreBase

	// Bookkeeping of invasive operations executed
	// on the Store to support rollback functionality.
	revOps storage.TxRevertOpStore[[]byte, []byte]
}

// release releases the TxStore transaction associated resources.
func (s *TxStore) release() {
	s.TxStoreBase.Store = nil
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
		err = s.revOps.Append(reverseOp)
	}
	return err
}

// Delete implements the Store interface.
func (s *TxStore) Delete(item storage.Item) error {
	if err := s.IsDone(); err != nil {
		return err
	}

	prev := item.Clone()
	var reverseOp *storage.TxRevertOp[[]byte, []byte]
	if err := s.TxStoreBase.Get(prev); err == nil {
		val, err := prev.Marshal()
		if err != nil {
			return err
		}
		reverseOp = &storage.TxRevertOp[[]byte, []byte]{
			Origin:   storage.DeleteOp,
			ObjectID: item.String(),
			Key:      key(item),
			Val:      val,
		}
	}

	err := s.TxStoreBase.Delete(item)
	if err == nil {
		err = s.revOps.Append(reverseOp)
	}
	return err
}

// Commit implements the Tx interface.
func (s *TxStore) Commit() error {
	defer s.release()

	if err := s.TxState.Done(); err != nil {
		return err
	}
	if err := s.revOps.Clean(); err != nil {
		return fmt.Errorf("leveldbstore: unable to clean revert operations: %w", err)
	}
	return nil
}

// Rollback implements the Tx interface.
func (s *TxStore) Rollback() error {
	defer s.release()

	if err := s.TxStoreBase.Rollback(); err != nil {
		return fmt.Errorf("leveldbstore: unable to rollback: %w", err)
	}

	if err := s.revOps.Revert(); err != nil {
		return fmt.Errorf("leveldbstore: unable to revert operations: %w", err)
	}
	return nil
}

// pendingTxNamespace exist for cashing the namespace of pendingTx
var pendingTxNamespace = new(pendingTx).Namespace()

// id returns the key for the stored revert operations.
func id(uuid string) []byte {
	return []byte(storageutil.JoinFields(pendingTxNamespace, uuid))
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
		revOps: &txRevertOpStore{
			id:    id(uuid.NewString()),
			db:    s.Store.(*Store).db,
			batch: batch,
			revOpsFn: map[storage.TxOpCode]storage.TxRevertFn[[]byte, []byte]{
				storage.PutCreateOp: func(k, _ []byte) error {
					batch.Delete(k)
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
		},
	}
}

// NewTxStore returns a new TxStore instance backed by the given store.
func NewTxStore(store storage.Store) *TxStore {
	return &TxStore{
		TxStoreBase: &storage.TxStoreBase{Store: store},
		revOps:      new(storage.NoOpTxRevertOpStore[[]byte, []byte]),
	}
}
