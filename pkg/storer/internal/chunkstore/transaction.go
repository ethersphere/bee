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

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/storage/storageutil"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

// txSharky provides a simple txn functionality over the Sharky store.
// It mainly exists to support the chunk store Delete operation where
// the Release calls are postponed until Commit or Rollback is called.
type txSharky struct {
	Sharky

	id    []byte
	store leveldbstore.Storer

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

		buf, err = msgpack.Marshal(t.writtenLocs)
		if err == nil {
			err = t.store.DB().Put(t.id, buf, nil)
		}
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

var (
	_ storage.ChunkStore = (*TxChunkStoreWrapper)(nil)
	_ storage.Recoverer  = (*TxChunkStoreWrapper)(nil)
)

type TxChunkStoreWrapper struct {
	*storage.TxChunkStoreBase

	txStore  storage.TxStore
	txSharky *txSharky
}

// release releases the TxChunkStoreWrapper transaction associated resources.
func (cs *TxChunkStoreWrapper) release() {
	cs.TxChunkStoreBase.ChunkStore = nil
	cs.txSharky.toReleaseLocs = nil
	cs.txSharky.toReleaseSums = nil
	cs.txSharky.writtenLocs = nil
	cs.txSharky.Sharky = nil
}

func (cs *TxChunkStoreWrapper) Commit() error {
	defer cs.release()

	var errs error
	if err := cs.txStore.Commit(); err != nil {
		errs = errors.Join(errs, fmt.Errorf("txchunkstore: unable to commit index store transaction: %w", err))
	}

	for _, loc := range cs.txSharky.toReleaseLocs {
		errs = errors.Join(errs, cs.txSharky.Sharky.Release(context.Background(), loc))
	}

	if err := cs.txSharky.store.DB().Delete(cs.txSharky.id, nil); err != nil {
		errs = errors.Join(errs, fmt.Errorf("txchunkstore: unable to delete transaction: %x: %w", cs.txSharky.id, err))
	}
	return errs
}

func (cs *TxChunkStoreWrapper) Rollback() error {
	defer cs.release()

	var errs error
	if err := cs.txStore.Rollback(); err != nil {
		errs = errors.Join(errs, fmt.Errorf("txchunkstore: unable to rollback index store transaction: %w", err))
	}

	if errs == nil {
		for _, loc := range cs.txSharky.writtenLocs {
			errs = errors.Join(errs, cs.txSharky.Sharky.Release(context.Background(), loc))
		}
		if errs != nil {
			return fmt.Errorf("txchunkstore: unable to release locations: %w", errs)
		}
	}

	if err := cs.txSharky.store.DB().Delete(cs.txSharky.id, nil); err != nil {
		errs = errors.Join(errs, fmt.Errorf("txchunkstore: unable to delete transaction: %x: %w", cs.txSharky.id, err))
	}
	return errs
}

var pendingTxNamespace = new(pendingTx).Namespace()

func (cs *TxChunkStoreWrapper) NewTx(state *storage.TxState) storage.TxChunkStore {
	txStore := cs.txStore.NewTx(state)
	txSharky := &txSharky{
		id:            []byte(storageutil.JoinFields(pendingTxNamespace, uuid.NewString())),
		store:         cs.txStore.(*leveldbstore.TxStore).BatchedStore.(leveldbstore.Storer), // TODO: make this independent of the underlying store.
		Sharky:        cs.txSharky.Sharky,
		toReleaseLocs: make(map[[32]byte]sharky.Location),
		toReleaseSums: make(map[sharky.Location][32]byte),
	}
	return &TxChunkStoreWrapper{
		TxChunkStoreBase: &storage.TxChunkStoreBase{
			TxState:    state,
			ChunkStore: New(txStore, txSharky),
		},
		txStore:  txStore,
		txSharky: txSharky,
	}
}

func NewTxChunkStore(txStore storage.TxStore, csSharky Sharky) *TxChunkStoreWrapper {
	return &TxChunkStoreWrapper{
		TxChunkStoreBase: &storage.TxChunkStoreBase{
			ChunkStore: New(txStore, csSharky),
		},
		txStore:  txStore,
		txSharky: &txSharky{Sharky: csSharky},
	}
}
