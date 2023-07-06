// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore

import (
	"context"
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
func (s *txRevertOpStore) Append(ops ...*storage.TxRevertOp[[]byte, []byte]) error {
	if s == nil || len(ops) == 0 {
		return nil
	}

	s.batchMu.Lock()
	defer s.batchMu.Unlock()

	for _, op := range ops {
		if op == nil {
			continue
		}
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

func put(
	reader storage.Reader,
	writer storage.Writer,
	item storage.Item,
) (*storage.TxRevertOp[[]byte, []byte], error) {
	prev := item.Clone()
	var reverseOp *storage.TxRevertOp[[]byte, []byte]
	switch err := reader.Get(prev); {
	case errors.Is(err, storage.ErrNotFound):
		reverseOp = &storage.TxRevertOp[[]byte, []byte]{
			Origin:   storage.PutCreateOp,
			ObjectID: item.String(),
			Key:      key(item),
		}
	case err != nil:
		return nil, err
	default:
		val, err := prev.Marshal()
		if err != nil {
			return nil, err
		}
		reverseOp = &storage.TxRevertOp[[]byte, []byte]{
			Origin:   storage.PutUpdateOp,
			ObjectID: prev.String(),
			Key:      key(prev),
			Val:      val,
		}
	}

	err := writer.Put(item)
	if err == nil {
		return reverseOp, nil
	}
	return nil, err
}

func del(
	reader storage.Reader,
	writer storage.Writer,
	item storage.Item,
) (*storage.TxRevertOp[[]byte, []byte], error) {
	prev := item.Clone()
	var reverseOp *storage.TxRevertOp[[]byte, []byte]
	if err := reader.Get(prev); err == nil {
		val, err := prev.Marshal()
		if err != nil {
			return nil, err
		}
		reverseOp = &storage.TxRevertOp[[]byte, []byte]{
			Origin:   storage.DeleteOp,
			ObjectID: item.String(),
			Key:      key(item),
			Val:      val,
		}
	}

	err := writer.Delete(item)
	if err == nil {
		return reverseOp, nil
	}
	return nil, err
}

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
	s.TxStoreBase.BatchedStore = nil
	s.revOps = nil
}

// Put implements the Store interface.
func (s *TxStore) Put(item storage.Item) error {
	if s.TxState == nil {
		return s.TxStoreBase.Put(item)
	}
	if err := s.IsDone(); err != nil {
		return err
	}

	reverseOp, err := put(s.TxStoreBase, s.TxStoreBase, item)
	if err == nil && reverseOp != nil {
		err = s.revOps.Append(reverseOp)
	}
	return err
}

// Delete implements the Store interface.
func (s *TxStore) Delete(item storage.Item) error {
	if s.TxState == nil {
		return s.TxStoreBase.Delete(item)
	}
	if err := s.IsDone(); err != nil {
		return err
	}

	reverseOp, err := del(s.TxStoreBase, s.TxStoreBase, item)
	if err == nil && reverseOp != nil {
		err = s.revOps.Append(reverseOp)
	}
	return err
}

// Commit implements the Tx interface.
func (s *TxStore) Commit() error {
	if s.TxState == nil {
		return nil
	}
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
	if s.TxState == nil {
		return nil
	}
	defer s.release()

	if err := s.TxStoreBase.Rollback(); err != nil {
		return fmt.Errorf("leveldbstore: unable to rollback: %w", err)
	}

	if err := s.revOps.Revert(); err != nil {
		return fmt.Errorf("leveldbstore: unable to revert operations: %w", err)
	}
	return nil
}

// Batch implements the Batcher interface.
func (s *TxStore) Batch(ctx context.Context) (storage.Batch, error) {
	batch, err := s.TxStoreBase.BatchedStore.Batch(ctx)
	if err != nil {
		return nil, err
	}

	return &txWrappedBatch{
		batch: batch,
		store: s,
		onCommit: func(revOps ...*storage.TxRevertOp[[]byte, []byte]) error {
			return s.revOps.Append(revOps...)
		},
	}, nil
}

type txWrappedBatch struct {
	batch    storage.Batch
	store    *TxStore
	opsMu    sync.Mutex
	ops      []*storage.TxRevertOp[[]byte, []byte]
	onCommit func(revOps ...*storage.TxRevertOp[[]byte, []byte]) error
}

// Put implements the Batch interface.
func (b *txWrappedBatch) Put(item storage.Item) error {
	if err := b.store.IsDone(); err != nil {
		return err
	}

	reverseOp, err := put(b.store, b.batch, item)
	if err == nil && reverseOp != nil {
		b.opsMu.Lock()
		b.ops = append(b.ops, reverseOp)
		b.opsMu.Unlock()
	}
	return err
}

// Delete implements the Batch interface.
func (b *txWrappedBatch) Delete(item storage.Item) error {
	if err := b.store.IsDone(); err != nil {
		return err
	}

	reverseOp, err := del(b.store, b.batch, item)
	if err == nil && reverseOp != nil {
		b.opsMu.Lock()
		b.ops = append(b.ops, reverseOp)
		b.opsMu.Unlock()
	}
	return err
}

func (b *txWrappedBatch) Commit() error {
	if err := b.batch.Commit(); err != nil {
		return err
	}
	b.opsMu.Lock()
	defer b.opsMu.Unlock()
	defer func() {
		b.ops = nil
	}()
	return b.onCommit(b.ops...)
}

// pendingTxNamespace exist for cashing the namespace of pendingTx
var pendingTxNamespace = new(pendingTx).Namespace()

// id returns the key for the stored revert operations.
func id(uuid string) []byte {
	return []byte(storageutil.JoinFields(pendingTxNamespace, uuid))
}

// NewTx implements the TxStore interface.
func (s *TxStore) NewTx(state *storage.TxState) storage.TxStore {
	if s.BatchedStore == nil {
		panic(errors.New("leveldbstore: nil store"))
	}

	batch := new(leveldb.Batch)
	return &TxStore{
		TxStoreBase: &storage.TxStoreBase{
			TxState:      state,
			BatchedStore: s.BatchedStore,
		},
		revOps: &txRevertOpStore{
			id:    id(uuid.NewString()),
			db:    s.BatchedStore.(*Store).db,
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
func NewTxStore(store storage.BatchedStore) *TxStore {
	return &TxStore{TxStoreBase: &storage.TxStoreBase{BatchedStore: store}}
}
