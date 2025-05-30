//go:build js
// +build js

package transaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"resenje.org/multex"
)

type store struct {
	sharky      *sharky.Store
	bstore      storage.BatchStore
	chunkLocker *multex.Multex[any]
}

func NewStorage(sharky *sharky.Store, bstore storage.BatchStore) Storage {
	return &store{sharky, bstore, multex.New[any]()}
}

type transaction struct {
	start      time.Time
	batch      storage.Batch
	indexstore storage.IndexStore
	chunkStore *chunkStoreTrx
	sharkyTrx  *sharkyTrx
}

type indexTrx struct {
	store storage.Reader
	batch storage.Batch
}

type sharkyTrx struct {
	sharky       *sharky.Store
	writtenLocs  []sharky.Location
	releasedLocs []sharky.Location
}

type chunkStoreTrx struct {
	indexStore   storage.IndexStore
	sharkyTrx    *sharkyTrx
	globalLocker *multex.Multex[any]
	lockedAddrs  map[string]struct{}
	readOnly     bool
}

// NewTransaction returns a new storage transaction.
// Commit must be called to persist data to the disk.
// The callback function must be the final call of the transaction whether or not any errors
// were returned from the storage ops or commit. Safest option is to do a defer call immediately after
// creating the transaction.
// By design, it is best to not batch too many writes to a single transaction, including multiple chunks writes.
// Calls made to the transaction are NOT thread-safe.
func (s *store) NewTransaction(ctx context.Context) (Transaction, func()) {

	b := s.bstore.Batch(ctx)

	index := &indexTrx{store: s.bstore, batch: b}
	sharky := &sharkyTrx{s.sharky, nil, nil}

	t := &transaction{
		start:      time.Now(),
		batch:      b,
		indexstore: index,
		chunkStore: &chunkStoreTrx{index, sharky, s.chunkLocker, make(map[string]struct{}), false},
		sharkyTrx:  sharky,
	}

	return t, func() {
		// for whatever reason, commit was not called
		// release uncommitted but written sharky locations
		// unlock the locked addresses
		for _, l := range t.sharkyTrx.writtenLocs {
			_ = t.sharkyTrx.sharky.Release(context.TODO(), l)
		}
		for addr := range t.chunkStore.lockedAddrs {
			s.chunkLocker.Unlock(addr)
		}
		t.sharkyTrx.writtenLocs = nil
		t.chunkStore.lockedAddrs = nil
	}
}

func (s *store) IndexStore() storage.Reader {
	return &indexTrx{s.bstore, nil}
}

func (s *store) ChunkStore() storage.ReadOnlyChunkStore {
	indexStore := &indexTrx{s.bstore, nil}
	sharyTrx := &sharkyTrx{s.sharky, nil, nil}
	return &chunkStoreTrx{indexStore, sharyTrx, s.chunkLocker, nil, true}
}

func (c *chunkStoreTrx) Get(ctx context.Context, addr swarm.Address) (ch swarm.Chunk, err error) {
	unlock := c.lock(addr)
	defer unlock()
	ch, err = chunkstore.Get(ctx, c.indexStore, c.sharkyTrx, addr)
	return ch, err
}
func (c *chunkStoreTrx) Has(ctx context.Context, addr swarm.Address) (_ bool, err error) {

	unlock := c.lock(addr)
	defer unlock()
	return chunkstore.Has(ctx, c.indexStore, addr)
}
func (c *chunkStoreTrx) Put(ctx context.Context, ch swarm.Chunk) (err error) {

	unlock := c.lock(ch.Address())
	defer unlock()
	return chunkstore.Put(ctx, c.indexStore, c.sharkyTrx, ch)
}
func (c *chunkStoreTrx) Delete(ctx context.Context, addr swarm.Address) (err error) {

	unlock := c.lock(addr)
	defer unlock()
	return chunkstore.Delete(ctx, c.indexStore, c.sharkyTrx, addr)
}
func (c *chunkStoreTrx) Iterate(ctx context.Context, fn storage.IterateChunkFn) (err error) {

	return chunkstore.Iterate(ctx, c.indexStore, c.sharkyTrx, fn)
}

func (c *chunkStoreTrx) Replace(ctx context.Context, ch swarm.Chunk, emplace bool) (err error) {

	unlock := c.lock(ch.Address())
	defer unlock()
	return chunkstore.Replace(ctx, c.indexStore, c.sharkyTrx, ch, emplace)
}

func (t *transaction) Commit() (err error) {

	defer func() {
		for addr := range t.chunkStore.lockedAddrs {
			t.chunkStore.globalLocker.Unlock(addr)
		}
		t.chunkStore.lockedAddrs = nil
		t.sharkyTrx.writtenLocs = nil
	}()

	err = t.batch.Commit()

	if err != nil {
		// since the batch commit has failed, we must release the written chunks from sharky.
		for _, l := range t.sharkyTrx.writtenLocs {
			if rerr := t.sharkyTrx.sharky.Release(context.TODO(), l); rerr != nil {
				err = errors.Join(err, fmt.Errorf("failed releasing location during commit rollback %s: %w", l, rerr))
			}
		}
		return err
	}

	// the batch commit was successful, we can now release the accumulated locations from sharky.
	for _, l := range t.sharkyTrx.releasedLocs {

		rerr := t.sharkyTrx.sharky.Release(context.TODO(), l)

		if rerr != nil {
			err = errors.Join(err, fmt.Errorf("failed releasing location after commit %s: %w", l, rerr))
		}
	}

	return err
}

func (s *sharkyTrx) Read(ctx context.Context, loc sharky.Location, buf []byte) (err error) {
	return s.sharky.Read(ctx, loc, buf)
}

func (s *sharkyTrx) Write(ctx context.Context, data []byte) (_ sharky.Location, err error) {
	loc, err := s.sharky.Write(ctx, data)
	if err != nil {
		return sharky.Location{}, err
	}

	s.writtenLocs = append(s.writtenLocs, loc)
	return loc, nil
}

func (s *indexTrx) Iterate(q storage.Query, f storage.IterateFn) (err error) {
	return s.store.Iterate(q, f)
}
