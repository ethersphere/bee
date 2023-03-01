// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"context"
	"crypto/sha256"
	"sync"

	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/hashicorp/go-multierror"
)

// txSharky provides a simple txn functionality over the Sharky store.
// It mainly exist to to support the chunk store Delete operation where
// the Release calls are postponed until Commit or Rollback is called.
type txSharky struct {
	Sharky

	opsMu         sync.Mutex
	writtenLocs   []sharky.Location
	toReleaseLocs map[[32]byte]sharky.Location
	toReleaseSums map[sharky.Location][32]byte
}

func (t *txSharky) Write(ctx context.Context, buf []byte) (sharky.Location, error) {
	var (
		sum = sha256.Sum256(buf)
		loc sharky.Location
		err error
	)

	t.opsMu.Lock()
	defer t.opsMu.Unlock()

	loc, ok := t.toReleaseLocs[sum]
	if ok {
		delete(t.toReleaseLocs, sum)
		delete(t.toReleaseSums, loc)
		return loc, nil
	}

	loc, err = t.Sharky.Write(ctx, buf)
	if err == nil {
		t.writtenLocs = append(t.writtenLocs, loc)
		t.toReleaseLocs[sum] = loc
		t.toReleaseSums[loc] = sum
	}
	return loc, err
}

func (t *txSharky) Release(ctx context.Context, loc sharky.Location) error {
	t.opsMu.Lock()
	defer t.opsMu.Unlock()

	sum, ok := t.toReleaseSums[loc]
	if !ok {
		buf := make([]byte, loc.Length)
		if err := t.Sharky.Read(ctx, loc, buf); err != nil {
			return err
		}
		sum = sha256.Sum256(buf)
		t.toReleaseSums[loc] = sum
	}
	t.toReleaseLocs[sum] = loc

	return nil
}

type txChunkStoreWrapper struct {
	*storage.TxChunkStoreBase

	txStore  storage.TxStore
	txSharky *txSharky

	// Bookkeeping of invasive operations executed
	// on the ChunkStore to support rollback functionality.
	revOps storage.TxRevStack
}

// Put implements the ChunkStore interface.
func (cs *txChunkStoreWrapper) Put(ctx context.Context, chunk swarm.Chunk) error {
	err := cs.TxChunkStoreBase.Put(ctx, chunk)
	if err == nil {
		cs.revOps.Append(&storage.TxRevertOp{
			Origin:   storage.PutOp,
			ObjectID: chunk.Address().String(),
			Revert: func() error {
				return cs.TxChunkStoreBase.Delete(context.Background(), chunk.Address())
			},
		})
	}
	return err
}

func (cs *txChunkStoreWrapper) Commit() error {
	if err := cs.txStore.Commit(); err != nil {
		return err
	}

	for _, loc := range cs.txSharky.toReleaseLocs {
		err := cs.txSharky.Release(context.Background(), loc)
		if err != nil {
			return err
		}
	}

	return nil
}

func (cs *txChunkStoreWrapper) Rollback() error {
	if err := cs.txStore.Rollback(); err != nil {
		return err
	}

	var err *multierror.Error
	for _, loc := range cs.txSharky.writtenLocs {
		err = multierror.Append(err, cs.txSharky.Sharky.Release(context.Background(), loc))
	}
	return err.ErrorOrNil()
}

func (cs *txChunkStoreWrapper) NewTx(state *storage.TxState) storage.TxChunkStore {
	txStore := cs.txStore.NewTx(state)
	txSharky := &txSharky{
		Sharky:        cs.txSharky.Sharky,
		toReleaseLocs: make(map[[32]byte]sharky.Location),
		toReleaseSums: make(map[sharky.Location][32]byte),
	}
	return &txChunkStoreWrapper{
		TxChunkStoreBase: &storage.TxChunkStoreBase{
			TxState: state,
			ChunkStore: &chunkStoreWrapper{
				store:  txStore,
				sharky: txSharky,
			},
		},
		txStore:  txStore,
		txSharky: txSharky,
	}
}

func NewTxChunkStore(txStore storage.TxStore, csSharky Sharky) storage.TxChunkStore {
	txSharky := &txSharky{
		Sharky:        csSharky,
		toReleaseLocs: make(map[[32]byte]sharky.Location),
		toReleaseSums: make(map[sharky.Location][32]byte),
	}
	return &txChunkStoreWrapper{
		TxChunkStoreBase: &storage.TxChunkStoreBase{
			ChunkStore: &chunkStoreWrapper{
				txStore,
				txSharky,
			},
		},
		txStore:  txStore,
		txSharky: txSharky,
	}
}
