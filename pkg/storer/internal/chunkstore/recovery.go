// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"context"
	"fmt"
	"slices"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storageutil"
	"github.com/vmihailenco/msgpack/v5"
)

var _ storage.Item = (*pendingTx)(nil)

// pendingTx is a storage.Item that holds a batch of operations.
type pendingTx struct {
	key string
	val []byte
}

// ID implements storage.Item.
func (p *pendingTx) ID() string {
	return p.key
}

// Namespace implements storage.Item.
func (p *pendingTx) Namespace() string {
	return "pending-chunkstore-tx"
}

// Unmarshal implements storage.Item.
func (p *pendingTx) Unmarshal(bytes []byte) error {
	p.val = slices.Clone(bytes)
	return nil
}

// Marshal implements storage.Item.
func (p *pendingTx) Marshal() ([]byte, error) {
	return p.val, nil
}

// Clone implements storage.Item.
func (p *pendingTx) Clone() storage.Item {
	if p == nil {
		return nil
	}
	return &pendingTx{
		key: p.key,
		val: slices.Clone(p.val),
	}
}

// String implements storage.Item.
func (p *pendingTx) String() string {
	return storageutil.JoinFields(p.Namespace(), p.ID())
}

// Recover attempts to recover from a previous crash
// by reverting all uncommitted transactions.
func (cs *TxChunkStoreWrapper) Recover() error {
	logger := log.NewLogger("node").WithName("tx_chunkstore_recovery").Register() // "node" - copies the node.LoggerName in order to avoid circular import.

	if rr, ok := cs.txStore.(storage.Recoverer); ok {
		if err := rr.Recover(); err != nil {
			return fmt.Errorf("chunkstore: recovery: %w", err)
		}
	}

	var found bool

	logger.Info("checking for uncommitted transactions")
	err := cs.txStore.Iterate(storage.Query{
		Factory:      func() storage.Item { return new(pendingTx) },
		ItemProperty: storage.QueryItem,
	}, func(r storage.Result) (bool, error) {
		found = true

		item := r.Entry.(*pendingTx)
		item.key = r.ID

		var locations []sharky.Location
		if err := msgpack.Unmarshal(item.val, &locations); err != nil {
			return true, fmt.Errorf("location unmarshal failed: %w", err)
		}

		ctx := context.Background()
		logger.Info("sharky unreleased location found", "count", len(locations), "id", r.ID)
		for _, location := range locations {
			logger.Debug("releasing location", "location", location)
			if err := cs.txSharky.Sharky.Release(ctx, location); err != nil {
				logger.Debug("unable to release location", "location", location, "err", err)
				return true, fmt.Errorf("unable to release location %v for %s: %w", location, r.ID, err)
			}
		}
		logger.Info("sharky unreleased location released", "id", r.ID)

		logger.Info("cleaning uncommitted transaction log", "id", r.ID)
		if err := cs.txStore.Delete(r.Entry); err != nil {
			logger.Debug("unable to delete unreleased location", "id", r.ID, "err", err)
			return true, fmt.Errorf("unable to delete %s: %w", r.ID, err)
		}
		logger.Info("uncommitted transaction log cleaned", "id", r.ID)

		return false, nil
	})
	if err != nil {
		return fmt.Errorf("chunkstore: recovery: iteration failed: %w", err)
	}

	if found {
		logger.Info("recovery successful")
	} else {
		logger.Info("no uncommitted transactions found")
	}

	return nil
}
