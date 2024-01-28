// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/prometheus/client_golang/prometheus"
)

// TODO(esad): remove contexts from sharky and any other storage call
// TODO(esad): continue metrics

/*
The rules of the transction is as follows:

-sharky_write 	-> write to disk, keep sharky location in memory
-sharky_release -> keep location in memory, do not release from the disk
-store write 	-> write to batch
-on commit		-> if batch_commit succeeds, release sharky_release locations from the disk
				-> if batch_commit fails or is not called, release all sharky_write location from the disk, do nothing for sharky_release
*/

type Transaction interface {
	Store
	Commit() error
}

type Store interface {
	ChunkStore() storage.ChunkStore
	IndexStore() storage.IndexStore
}

type ReadOnlyStore interface {
	IndexStore() storage.Reader
	ChunkStore() storage.ReadOnlyChunkStore
}

type Storage interface {
	NewTransaction() (Transaction, func())
	ReadOnly() ReadOnlyStore
	Run(func(Store) error) error
	Close() error
}

type store struct {
	sharky *sharky.Store
	bstore storage.BatchedStore

	chunkStoreMtx *sync.Mutex

	metrics metrics
}

func NewStorage(sharky *sharky.Store, bstore storage.BatchedStore) Storage {
	return &store{sharky, bstore, &sync.Mutex{}, newMetrics()}
}

type transaction struct {
	batch         storage.Batch
	indexstore    *indexTrx
	chunkStore    *chunkStoreTrx
	sharkyTrx     *sharkyTrx
	chunkStoreMtx *sync.Mutex
	cleanup       bool
	metrics       metrics
}

type chunkStoreTrx struct {
	indexStore *indexTrx
	sharkyTrx  *sharkyTrx
}

func (c *chunkStoreTrx) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	return chunkstore.Get(ctx, c.indexStore, c.sharkyTrx, addr)
}
func (c *chunkStoreTrx) Has(ctx context.Context, addr swarm.Address) (bool, error) {
	return chunkstore.Has(ctx, c.indexStore, addr)
}
func (c *chunkStoreTrx) Put(ctx context.Context, ch swarm.Chunk) error {
	return chunkstore.Put(ctx, c.indexStore, c.sharkyTrx, ch)
}
func (c *chunkStoreTrx) Delete(ctx context.Context, addr swarm.Address) error {
	return chunkstore.Delete(ctx, c.indexStore, c.sharkyTrx, addr)
}
func (c *chunkStoreTrx) Iterate(ctx context.Context, fn storage.IterateChunkFn) error {
	return chunkstore.Iterate(ctx, c.indexStore, c.sharkyTrx, fn)
}

// NewTransaction returns a new storage transaction.
// Commit must be called to persist data to the disk.
// The callback function must be the final call of the transaction whether or not any errors
// were returned from the storage ops or commit. Safest option is to do a defer call immediately after
// creating the transaction.
// Calls made to the transaction are NOT thread-safe.
func (s *store) NewTransaction() (Transaction, func()) {

	b, _ := s.bstore.Batch(context.TODO())
	indexTrx := &indexTrx{s.bstore, b}
	sharyTrx := &sharkyTrx{sharky: s.sharky}

	t := &transaction{
		batch:         b,
		indexstore:    indexTrx,
		chunkStore:    &chunkStoreTrx{indexTrx, sharyTrx},
		sharkyTrx:     sharyTrx,
		chunkStoreMtx: s.chunkStoreMtx,
		metrics:       s.metrics,
	}

	return t, func() {
		// for whatever reason, the commit call was not made
		// release uncommitted written sharky locations
		for _, l := range t.sharkyTrx.writtenLocs {
			_ = t.sharkyTrx.sharky.Release(context.TODO(), l)
		}
		if t.cleanup {
			t.chunkStoreMtx.Unlock()
		}
	}
}

type readOnly struct {
	indexStore *indexTrx
	chunkStore *chunkStoreTrx
}

func (s *store) ReadOnly() ReadOnlyStore {
	indexStore := &indexTrx{store: s.bstore}
	sharyTrx := &sharkyTrx{sharky: s.sharky}
	return &readOnly{indexStore, &chunkStoreTrx{indexStore, sharyTrx}}
}

func (s *store) Run(f func(Store) error) error {
	trx, done := s.NewTransaction()
	defer done()

	err := f(trx)
	if err != nil {
		return err
	}
	return trx.Commit()
}

// Metrics returns set of prometheus collectors.
func (s *store) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}

func (s *store) Close() error {
	return errors.Join(s.bstore.Close(), s.sharky.Close())
}

func (t *readOnly) IndexStore() storage.Reader {
	return t.indexStore
}

func (t *readOnly) ChunkStore() storage.ReadOnlyChunkStore {
	return t.chunkStore
}

type indexTrx struct {
	store storage.Reader
	batch storage.Batch
}

func (s *indexTrx) Get(i storage.Item) error           { return s.store.Get(i) }
func (s *indexTrx) Has(k storage.Key) (bool, error)    { return s.store.Has(k) }
func (s *indexTrx) GetSize(k storage.Key) (int, error) { return s.store.GetSize(k) }
func (s *indexTrx) Iterate(q storage.Query, f storage.IterateFn) error {
	return s.store.Iterate(q, f)
}
func (s *indexTrx) Count(k storage.Key) (int, error) { return s.store.Count(k) }
func (s *indexTrx) Put(i storage.Item) error         { return s.batch.Put(i) }
func (s *indexTrx) Delete(i storage.Item) error      { return s.batch.Delete(i) }

// IndexStore gives acces to the index store of the transaction.
// Note that no writes are persisted to the disk until the commit is called.
// Reads return data from the disk and not what has been written to the transaction before the commit call.
func (t *transaction) IndexStore() storage.IndexStore {
	return t.indexstore
}

// ChunkStore gives acces to the chunkstore of the transaction.
// Note that no writes are persisted to the disk until the commit is called.
// Reads return data from the disk and not what has been written to the transaction before the commit call.
func (t *transaction) ChunkStore() storage.ChunkStore {
	// acquire the global chunkstore lock here
	if !t.cleanup {
		t.cleanup = true
		t.chunkStoreMtx.Lock()
	}
	return t.chunkStore
}

func (t *transaction) Commit() (err error) {

	defer func(ti time.Time) {
		t.sharkyTrx.writtenLocs = nil // clear written locs so that the done callback does not remove them
		if err != nil {
			t.metrics.MethodCalls.WithLabelValues("commit", "failure").Inc()
			t.metrics.MethodDuration.WithLabelValues("commit", "failure").Observe(float64(time.Since(ti)))
		} else {
			t.metrics.MethodCalls.WithLabelValues("commit", "success").Inc()
			t.metrics.MethodDuration.WithLabelValues("commit", "success").Observe(float64(time.Since(ti)))
		}
	}(time.Now())

	err = t.batch.Commit()
	if err != nil {
		for _, l := range t.sharkyTrx.writtenLocs {
			if rerr := t.sharkyTrx.sharky.Release(context.TODO(), l); rerr != nil {
				err = errors.Join(err, fmt.Errorf("failed releasing location during commit rollback %s: %w", l, rerr))
			}
		}
		return err
	}

	for _, l := range t.sharkyTrx.releasedLocs {
		if rerr := t.sharkyTrx.sharky.Release(context.TODO(), l); rerr != nil {
			err = errors.Join(err, fmt.Errorf("failed releasing location afer commit %s: %w", l, rerr))
		}
	}

	return err
}

type sharkyTrx struct {
	sharky       *sharky.Store
	writtenLocs  []sharky.Location
	releasedLocs []sharky.Location
}

func (s *sharkyTrx) Read(ctx context.Context, loc sharky.Location, buf []byte) error {
	return s.sharky.Read(ctx, loc, buf)
}

func (s *sharkyTrx) Write(ctx context.Context, data []byte) (sharky.Location, error) {
	loc, err := s.sharky.Write(ctx, data)
	if err != nil {
		return sharky.Location{}, err
	}

	s.writtenLocs = append(s.writtenLocs, loc)
	return loc, nil
}

func (s *sharkyTrx) Release(ctx context.Context, loc sharky.Location) error {
	s.releasedLocs = append(s.releasedLocs, loc)
	return nil
}
