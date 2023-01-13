package chunkstore

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/sharky"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/hashicorp/go-multierror"
)

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

func (t *txSharky) newTx() *txSharky {
	return &txSharky{Sharky: t.Sharky}
}

type txChunkStoreWrapper struct {
	*storage.TxChunkStoreBase

	txStore storage.TxStore
	sharky  *txSharky
	doneMu  sync.Mutex
}

func (t *txChunkStoreWrapper) Put(ctx context.Context, chunk swarm.Chunk) error {
	return t.TxChunkStoreBase.Put(ctx, chunk)
}

func (t *txChunkStoreWrapper) Delete(ctx context.Context, address swarm.Address) error {
	return t.TxChunkStoreBase.Delete(ctx, address)
}

func (t *txChunkStoreWrapper) Commit() error {
	t.doneMu.Lock()
	defer t.doneMu.Unlock()

	if err := t.IsDone(); err != nil {
		return err
	}

	for _, v := range t.sharky.releaseOps {
		err := t.sharky.Sharky.Release(context.Background(), v)
		if err != nil {
			return err
		}
	}

	if err := t.txStore.Commit(); err != nil {
		for _, v := range t.sharky.committedLocs {
			err = multierror.Append(err, t.sharky.Sharky.Release(context.Background(), v))
		}
		return err
	}

	t.TxState.Done()
	return nil
}

func (t *txChunkStoreWrapper) Rollback() error {
	t.doneMu.Lock()
	defer t.doneMu.Unlock()

	if err := t.IsDone(); err != nil {
		return err
	}

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
	txStore := t.txStore.NewTx(storage.NewChildTxState(state))
	txSharky := t.sharky.newTx()
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
