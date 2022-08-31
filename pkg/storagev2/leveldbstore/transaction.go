// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore

import (
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/storagev2"
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
	batchMu sync.Mutex
	batch   *leveldb.Batch
}

// Put implements the Store interface.
func (s *TxStore) Put(item storage.Item) error {
	err := s.TxStoreBase.Put(item)
	if err == nil {
		s.batchMu.Lock()
		s.batch.Delete(key(item))
		s.batchMu.Unlock()
	}
	return err
}

// Delete implements the Store interface.
func (s *TxStore) Delete(item storage.Item) error {
	err := s.TxStoreBase.Delete(item)
	if err == nil {
		val, err := item.Marshal()
		if err != nil {
			return fmt.Errorf("marshalling failed: %w", err)
		}
		s.batchMu.Lock()
		s.batch.Put(key(item), val)
		s.batchMu.Unlock()
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
	defer s.TxState.Done()

	s.batchMu.Lock()
	defer s.batchMu.Unlock()
	db := s.TxStoreBase.Store.(*Store).db
	if err := db.Write(s.batch, &opt.WriteOptions{Sync: true}); err != nil {
		return fmt.Errorf("leveldbstore: failed to write batch: %w", err)
	}
	return nil
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
