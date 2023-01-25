// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/sharky"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/hashicorp/go-multierror"
)

// txSharky provides a simple txn functionality over the Sharky store.
type txSharky struct {
	Sharky

	opsMu         sync.Mutex
	releaseOps    []sharky.Location
	committedLocs []sharky.Location
}

func (t *txSharky) Write(ctx context.Context, data []byte) (sharky.Location, error) {
	loc, err := t.Sharky.Write(ctx, data)
	if err == nil {
		t.opsMu.Lock()
		t.committedLocs = append(t.committedLocs, loc)
		t.opsMu.Unlock()
	}
	return loc, err
}

func (t *txSharky) Release(_ context.Context, loc sharky.Location) error {
	t.opsMu.Lock()
	defer t.opsMu.Unlock()

	t.releaseOps = append(t.releaseOps, loc)

	return nil
}

type txChunkStoreWrapper struct {
	*storage.TxChunkStoreBase

	txStore storage.TxStore
	sharky  *txSharky
}

func (t *txChunkStoreWrapper) Put(ctx context.Context, chunk swarm.Chunk) error {
	return t.TxChunkStoreBase.Put(ctx, chunk)
}

func (t *txChunkStoreWrapper) Delete(ctx context.Context, address swarm.Address) error {
	return t.TxChunkStoreBase.Delete(ctx, address)
}

func (t *txChunkStoreWrapper) Commit() error {
	// First we need to commit the child txn. This will inturn provide locking as
	// only 1 commit is possible on the child txn.
	if err := t.txStore.Commit(); err != nil {
		// due to the current implementation of the txStore, we would have already
		// committed the entries to disk. So this failure should only happen if
		// its a duplicate Commit request. If this assumption changes in future, we
		// would need to handle committed sharky locations here.
		return err
	}

	for _, v := range t.sharky.releaseOps {
		err := t.sharky.Sharky.Release(context.Background(), v)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *txChunkStoreWrapper) Rollback() error {
	if err := t.txStore.Rollback(); err != nil {
		return err
	}

	var err *multierror.Error
	for _, v := range t.sharky.committedLocs {
		err = multierror.Append(err, t.sharky.Sharky.Release(context.Background(), v))
	}
	return err.ErrorOrNil()
}

func (t *txChunkStoreWrapper) NewTx(state *storage.TxState) storage.TxChunkStore {
	txStore := t.txStore.NewTx(state)
	txSharky := &txSharky{Sharky: t.sharky.Sharky}
	return &txChunkStoreWrapper{
		TxChunkStoreBase: &storage.TxChunkStoreBase{
			TxState:    state,
			ChunkStore: New(txStore, txSharky),
		},
		txStore: txStore,
		sharky:  txSharky,
	}
}

func NewTxChunkStore(txStore storage.TxStore, sharky Sharky) storage.TxChunkStore {
	return &txChunkStoreWrapper{
		TxChunkStoreBase: &storage.TxChunkStoreBase{ChunkStore: New(txStore, sharky)},
		txStore:          txStore,
		sharky:           &txSharky{Sharky: sharky},
	}
}
