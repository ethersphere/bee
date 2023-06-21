// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/leveldbstore"
	"github.com/vmihailenco/msgpack/v5"
	"golang.org/x/exp/slices"
)

var _ storage.Item = (*pendingTx)(nil)

// pendingTx is a storage.Item that holds a batch of operations.
type pendingTx struct {
	storage.Item

	val []byte
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

// Recover attempts to recover from a previous crash
// by reverting all uncommitted transactions.
func (cs *TxChunkStoreWrapper) Recover(store *leveldbstore.Store) error {
	err := store.Iterate(storage.Query{
		Factory:      func() storage.Item { return new(pendingTx) },
		ItemProperty: storage.QueryItem,
	}, func(r storage.Result) (bool, error) {
		var locations []sharky.Location

		err := msgpack.Unmarshal(r.Entry.(*pendingTx).val, &locations)
		if err == nil {
			return true, fmt.Errorf("location unmarshal failed: %w", err)
		}

		ctx := context.Background()
		for _, location := range locations {
			if err := cs.txSharky.Sharky.Release(ctx, location); err != nil {
				return true, fmt.Errorf("unable to release location %v for %s: %w", location, r.ID, err)
			}
		}

		return false, nil
	})
	if err != nil {
		return fmt.Errorf("chunkstore: recovery: iteration failed: %w", err)
	}
	return nil
}
