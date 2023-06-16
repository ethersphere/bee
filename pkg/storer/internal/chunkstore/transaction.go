// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
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

	// If the chunk is already written in this transaction then we return
	// the location of the chunk. This is to avoid getting new location for
	// the same chunk in the case Put operation is called after Delete so
	// in the case of Rollback operation everything is consistent.
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

	store    storage.Store
	txSharky *txSharky

	// Bookkeeping of invasive operations executed
	// on the ChunkStore to support rollback functionality.
	revOps storage.TxRevertOpStore[swarm.Address, swarm.Chunk]
}

// Put implements the ChunkStore interface.
func (cs *txChunkStoreWrapper) Put(ctx context.Context, chunk swarm.Chunk) error {
	err := cs.TxChunkStoreBase.Put(ctx, chunk)
	if err == nil {
		err = cs.revOps.Append(&storage.TxRevertOp[swarm.Address, swarm.Chunk]{
			Origin:   storage.PutOp,
			ObjectID: chunk.Address().String(),
			Key:      chunk.Address(),
		})
	}
	return err
}

// Delete implements the ChunkStore interface.
func (cs *txChunkStoreWrapper) Delete(ctx context.Context, address swarm.Address) error {
	chunk, err := cs.TxChunkStoreBase.Get(ctx, address)
	if err != nil {
		return err
	}

	err = cs.TxChunkStoreBase.Delete(ctx, address)
	if err == nil {
		err = cs.revOps.Append(&storage.TxRevertOp[swarm.Address, swarm.Chunk]{
			Origin:   storage.DeleteOp,
			ObjectID: address.String(),
			Val:      chunk,
		})
	}
	return err
}

func (cs *txChunkStoreWrapper) Commit() error {
	if err := cs.TxChunkStoreBase.Done(); err != nil {
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
	if err := cs.TxChunkStoreBase.Rollback(); err != nil {
		return err
	}

	if err := cs.revOps.Revert(); err != nil {
		return fmt.Errorf("txchunkstore: unable to rollback: %w", err)
	}

	var err error
	for _, loc := range cs.txSharky.writtenLocs {
		err = errors.Join(cs.txSharky.Sharky.Release(context.Background(), loc))
	}
	return err
}

func (cs *txChunkStoreWrapper) NewTx(state *storage.TxState) storage.TxChunkStore {
	txSharky := &txSharky{
		Sharky:        cs.txSharky.Sharky,
		toReleaseLocs: make(map[[32]byte]sharky.Location),
		toReleaseSums: make(map[sharky.Location][32]byte),
	}
	chunkStore := &chunkStoreWrapper{
		store:  cs.store,
		sharky: txSharky,
	}
	return &txChunkStoreWrapper{
		TxChunkStoreBase: &storage.TxChunkStoreBase{
			TxState:    state,
			ChunkStore: chunkStore,
		},
		store:    cs.store,
		txSharky: txSharky,
		revOps: storage.NewInMemTxRevertOpStore(
			map[storage.TxOpCode]storage.TxRevertFn[swarm.Address, swarm.Chunk]{
				storage.PutOp: func(key swarm.Address, val swarm.Chunk) error {
					return chunkStore.Delete(context.Background(), key)
				},
				storage.DeleteOp: func(key swarm.Address, val swarm.Chunk) error {
					return chunkStore.Put(context.Background(), val)
				},
			},
		),
	}
}

func NewTxChunkStore(store storage.Store, csSharky Sharky) storage.TxChunkStore {
	txSharky := &txSharky{
		Sharky:        csSharky,
		toReleaseLocs: make(map[[32]byte]sharky.Location),
		toReleaseSums: make(map[sharky.Location][32]byte),
	}
	return &txChunkStoreWrapper{
		TxChunkStoreBase: &storage.TxChunkStoreBase{
			ChunkStore: &chunkStoreWrapper{
				store,
				txSharky,
			},
		},
		store:    store,
		txSharky: txSharky,
	}
}
