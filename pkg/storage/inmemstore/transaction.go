// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemstore

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
)

var (
	_ storage.TxStore = (*TxStore)(nil)
	_ storage.Batcher = (*TxStore)(nil)
)

func put(
	reader storage.Reader,
	writer storage.Writer,
	item storage.Item,
) (*storage.TxRevertOp[storage.Key, storage.Item], error) {
	prev := item.Clone()
	var reverseOp *storage.TxRevertOp[storage.Key, storage.Item]
	switch err := reader.Get(prev); {
	case errors.Is(err, storage.ErrNotFound):
		reverseOp = &storage.TxRevertOp[storage.Key, storage.Item]{
			Origin:   storage.PutCreateOp,
			ObjectID: item.String(),
			Val:      item,
		}
	case err != nil:
		return nil, err
	default:
		reverseOp = &storage.TxRevertOp[storage.Key, storage.Item]{
			Origin:   storage.PutUpdateOp,
			ObjectID: prev.String(),
			Val:      prev,
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
) (*storage.TxRevertOp[storage.Key, storage.Item], error) {
	prev := item.Clone()
	var reverseOp *storage.TxRevertOp[storage.Key, storage.Item]
	if err := reader.Get(prev); err == nil {
		reverseOp = &storage.TxRevertOp[storage.Key, storage.Item]{
			Origin:   storage.DeleteOp,
			ObjectID: item.String(),
			Val:      prev,
		}
	}

	err := writer.Delete(item)
	if err == nil {
		return reverseOp, nil
	}
	return nil, err
}

// txBatch is a batch that is used in a transaction.
type txBatch struct {
	batch    storage.Batch
	store    *TxStore
	revOpsMu sync.Mutex
	revOps   []*storage.TxRevertOp[storage.Key, storage.Item]
	onCommit func(revOps ...*storage.TxRevertOp[storage.Key, storage.Item]) error
}

// Put implements the Batch interface.
func (b *txBatch) Put(item storage.Item) error {
	if err := b.store.IsDone(); err != nil {
		return err
	}

	reverseOp, err := put(b.store, b.batch, item)
	if err == nil && reverseOp != nil {
		b.revOpsMu.Lock()
		b.revOps = append(b.revOps, reverseOp)
		b.revOpsMu.Unlock()
	}
	return err
}

// Delete implements the Batch interface.
func (b *txBatch) Delete(item storage.Item) error {
	if err := b.store.IsDone(); err != nil {
		return err
	}

	reverseOp, err := del(b.store, b.batch, item)
	if err == nil && reverseOp != nil {
		b.revOpsMu.Lock()
		b.revOps = append(b.revOps, reverseOp)
		b.revOpsMu.Unlock()
	}
	return err
}

// Commit implements the Batch interface.
func (b *txBatch) Commit() error {
	if err := b.batch.Commit(); err != nil {
		return err
	}
	b.revOpsMu.Lock()
	defer b.revOpsMu.Unlock()
	defer func() {
		b.revOps = nil
	}()
	return b.onCommit(b.revOps...)
}

// TxStore is an implementation of in-memory Store
// where all Store operations are done in a transaction.
type TxStore struct {
	*storage.TxStoreBase

	// Bookkeeping of invasive operations executed
	// on the Store to support rollback functionality.
	revOps storage.TxRevertOpStore[storage.Key, storage.Item]
}

// release releases the TxStore transaction associated resources.
func (s *TxStore) release() {
	s.TxStoreBase.BatchedStore = nil
	s.revOps = nil
}

// Put implements the Store interface.
func (s *TxStore) Put(item storage.Item) error {
	if err := s.IsDone(); err != nil {
		return err
	}

	reverseOp, err := put(s.TxStoreBase, s.TxStoreBase, item)
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

	reverseOp, err := del(s.TxStoreBase, s.TxStoreBase, item)
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
		return fmt.Errorf("inmemstore: unable to clean revert operations: %w", err)
	}
	return nil
}

// Rollback implements the Tx interface.
func (s *TxStore) Rollback() error {
	defer s.release()

	if err := s.TxStoreBase.Rollback(); err != nil {
		return fmt.Errorf("inmemstore: unable to rollback: %w", err)
	}

	if err := s.revOps.Revert(); err != nil {
		return fmt.Errorf("inmemstore: unable to revert operations: %w", err)
	}
	return nil
}

// Batch implements the Batcher interface.
func (s *TxStore) Batch(ctx context.Context) (storage.Batch, error) {
	batch, err := s.TxStoreBase.BatchedStore.Batch(ctx)
	if err != nil {
		return nil, err
	}

	return &txBatch{
		batch: batch,
		store: s,
		onCommit: func(revOps ...*storage.TxRevertOp[storage.Key, storage.Item]) error {
			return s.revOps.Append(revOps...)
		},
	}, nil
}

// NewTx implements the TxStore interface.
func (s *TxStore) NewTx(state *storage.TxState) storage.TxStore {
	if s.BatchedStore == nil {
		panic(errors.New("inmemstore: nil store"))
	}

	return &TxStore{
		TxStoreBase: &storage.TxStoreBase{
			TxState:      state,
			BatchedStore: s.BatchedStore,
		},
		revOps: storage.NewInMemTxRevertOpStore(
			map[storage.TxOpCode]storage.TxRevertFn[storage.Key, storage.Item]{
				storage.PutCreateOp: func(_ storage.Key, item storage.Item) error {
					return s.BatchedStore.Delete(item)
				},
				storage.PutUpdateOp: func(_ storage.Key, item storage.Item) error {
					return s.BatchedStore.Put(item)
				},
				storage.DeleteOp: func(_ storage.Key, item storage.Item) error {
					return s.BatchedStore.Put(item)
				},
			},
		),
	}
}

// NewTxStore returns a new TxStore instance backed by the given store.
func NewTxStore(store storage.BatchedStore) *TxStore {
	return &TxStore{
		TxStoreBase: &storage.TxStoreBase{BatchedStore: store},
		revOps:      new(storage.NoOpTxRevertOpStore[storage.Key, storage.Item]),
	}
}
