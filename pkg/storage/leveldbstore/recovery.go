// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore

import (
	"fmt"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var _ storage.Item = (*pendingTx)(nil)

// pendingTx is a storage.Item that holds a batch of operations.
type pendingTx struct {
	storage.Item

	val *leveldb.Batch
}

// Namespace implements storage.Item.
func (p *pendingTx) Namespace() string {
	return "pending-indexstore-tx"
}

// Unmarshal implements storage.Item.
func (p *pendingTx) Unmarshal(bytes []byte) error {
	p.val = new(leveldb.Batch)
	return p.val.Load(bytes)
}

// Recover attempts to recover from a previous
// crash by reverting all uncommitted transactions.
func (s *TxStore) Recover() error {
	logger := log.NewLogger("node").WithName("tx_store_recovery").Register() // "node" - copies the node.LoggerName in order to avoid circular import.

	batch := new(leveldb.Batch)

	logger.Info("checking for uncommitted transactions")
	err := s.Iterate(storage.Query{
		Factory:      func() storage.Item { return new(pendingTx) },
		ItemProperty: storage.QueryItem,
	}, func(r storage.Result) (bool, error) {
		logger.Info("uncommitted transaction found", "id", r.ID)
		if err := r.Entry.(*pendingTx).val.Replay(batch); err != nil {
			logger.Debug("unable to replay uncommitted transaction", "id", r.ID, "err", err)
			return true, fmt.Errorf("unable to replay batch for %s: %w", r.ID, err)
		}
		batch.Delete(id(r.ID))
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("leveldbstore: recovery: iteration failed: %w", err)
	}

	if batch.Len() == 0 {
		logger.Info("no uncommitted transactions found")
		return nil
	}

	logger.Info("reversing uncommitted transactions", "count", batch.Len())
	if err := s.BatchedStore.(*Store).db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
		return fmt.Errorf("leveldbstore: recovery: unable to write batch: %w", err)
	}
	logger.Info("recovery successful")
	return nil
}
