// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"context"
	"fmt"
	"slices"

	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storageutil"
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
	if rr, ok := cs.txStore.(storage.Recoverer); ok {
		if err := rr.Recover(); err != nil {
			return fmt.Errorf("chunkstore: recovery: %w", err)
		}
	}
	err := cs.txStore.Iterate(storage.Query{
		Factory:      func() storage.Item { return new(pendingTx) },
		ItemProperty: storage.QueryItem,
	}, func(r storage.Result) (bool, error) {
		item := r.Entry.(*pendingTx)
		item.key = r.ID

		var locations []sharky.Location
		if err := msgpack.Unmarshal(item.val, &locations); err != nil {
			return true, fmt.Errorf("location unmarshal failed: %w", err)
		}

		ctx := context.Background()
		for _, location := range locations {
			if err := cs.txSharky.Sharky.Release(ctx, location); err != nil {
				return true, fmt.Errorf("unable to release location %v for %s: %w", location, r.ID, err)
			}
		}

		if err := cs.txStore.Delete(r.Entry); err != nil {
			return true, fmt.Errorf("unable to delete %s: %w", r.ID, err)
		}

		return false, nil
	})
	if err != nil {
		return fmt.Errorf("chunkstore: recovery: iteration failed: %w", err)
	}

	return nil
}
