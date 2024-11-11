// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package transaction provides transaction support for localstore operations.
All writes to the localstore (both indexstore and chunkstore) must be made using a transaction.
The transaction must be committed for the writes to be stored on the disk.

The rules of the transaction is as follows:

-sharky_write 		-> write to disk, keep sharky location in memory
-sharky_release		-> keep location in memory, do not release from the disk
-indexstore write	-> write to batch
-on commit			-> if batch_commit succeeds, release sharky_release locations from the disk
					-> if batch_commit fails or is not called, release all sharky_write location from the disk, do nothing for sharky_release

See the NewTransaction method for more details.
*/

package transaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/prometheus/client_golang/prometheus"
	"resenje.org/multex"
)

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
	ReadOnlyStore
	NewTransaction(context.Context) (Transaction, func())
	Run(context.Context, func(Store) error) error
	Close() error
}

type store struct {
	sharky      *sharky.Store
	bstore      storage.BatchStore
	metrics     metrics
	chunkLocker *multex.Multex
}

func NewStorage(sharky *sharky.Store, bstore storage.BatchStore) Storage {
	return &store{sharky, bstore, newMetrics(), multex.New()}
}

type transaction struct {
	start      time.Time
	batch      storage.Batch
	indexstore storage.IndexStore
	chunkStore *chunkStoreTrx
	sharkyTrx  *sharkyTrx
	metrics    metrics
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

	index := &indexTrx{s.bstore, b, s.metrics}
	sharky := &sharkyTrx{s.sharky, s.metrics, nil, nil}

	t := &transaction{
		start:      time.Now(),
		batch:      b,
		indexstore: index,
		chunkStore: &chunkStoreTrx{index, sharky, s.chunkLocker, make(map[string]struct{}), s.metrics, false},
		sharkyTrx:  sharky,
		metrics:    s.metrics,
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
	return &indexTrx{s.bstore, nil, s.metrics}
}

func (s *store) ChunkStore() storage.ReadOnlyChunkStore {
	indexStore := &indexTrx{s.bstore, nil, s.metrics}
	sharyTrx := &sharkyTrx{s.sharky, s.metrics, nil, nil}
	return &chunkStoreTrx{indexStore, sharyTrx, s.chunkLocker, nil, s.metrics, true}
}

// Run creates a new transaction and gives the caller access to the transaction
// in the form of a callback function. After the callback returns, the transaction
// is committed to the disk. See the NewTransaction method for more details on how transactions operate internally.
// By design, it is best to not batch too many writes to a single transaction, including multiple chunks writes.
// Calls made to the transaction are NOT thread-safe.
func (s *store) Run(ctx context.Context, f func(Store) error) error {
	trx, done := s.NewTransaction(ctx)
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

func (t *transaction) Commit() (err error) {

	defer func() {
		t.metrics.MethodDuration.WithLabelValues("transaction", "success").Observe(time.Since(t.start).Seconds())
	}()

	defer handleMetric("commit", t.metrics)(&err)
	defer func() {
		for addr := range t.chunkStore.lockedAddrs {
			t.chunkStore.globalLocker.Unlock(addr)
		}
		t.chunkStore.lockedAddrs = nil
		t.sharkyTrx.writtenLocs = nil
	}()

	h := handleMetric("batch_commit", t.metrics)
	err = t.batch.Commit()
	h(&err)
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
		h := handleMetric("sharky_release", t.metrics)
		rerr := t.sharkyTrx.sharky.Release(context.TODO(), l)
		h(&rerr)
		if rerr != nil {
			err = errors.Join(err, fmt.Errorf("failed releasing location after commit %s: %w", l, rerr))
		}
	}

	return err
}

// IndexStore gives access to the index store of the transaction.
// Note that no writes are persisted to the disk until the commit is called.
func (t *transaction) IndexStore() storage.IndexStore {
	return t.indexstore
}

// ChunkStore gives access to the chunkstore of the transaction.
// Note that no writes are persisted to the disk until the commit is called.
func (t *transaction) ChunkStore() storage.ChunkStore {
	return t.chunkStore
}

type chunkStoreTrx struct {
	indexStore   storage.IndexStore
	sharkyTrx    *sharkyTrx
	globalLocker *multex.Multex
	lockedAddrs  map[string]struct{}
	metrics      metrics
	readOnly     bool
}

func (c *chunkStoreTrx) Get(ctx context.Context, addr swarm.Address) (ch swarm.Chunk, err error) {
	defer handleMetric("chunkstore_get", c.metrics)(&err)
	unlock := c.lock(addr)
	defer unlock()
	ch, err = chunkstore.Get(ctx, c.indexStore, c.sharkyTrx, addr)
	return ch, err
}
func (c *chunkStoreTrx) Has(ctx context.Context, addr swarm.Address) (_ bool, err error) {
	defer handleMetric("chunkstore_has", c.metrics)(&err)
	unlock := c.lock(addr)
	defer unlock()
	return chunkstore.Has(ctx, c.indexStore, addr)
}
func (c *chunkStoreTrx) Put(ctx context.Context, ch swarm.Chunk) (err error) {
	defer handleMetric("chunkstore_put", c.metrics)(&err)
	unlock := c.lock(ch.Address())
	defer unlock()
	return chunkstore.Put(ctx, c.indexStore, c.sharkyTrx, ch)
}
func (c *chunkStoreTrx) Delete(ctx context.Context, addr swarm.Address) (err error) {
	defer handleMetric("chunkstore_delete", c.metrics)(&err)
	unlock := c.lock(addr)
	defer unlock()
	return chunkstore.Delete(ctx, c.indexStore, c.sharkyTrx, addr)
}
func (c *chunkStoreTrx) Iterate(ctx context.Context, fn storage.IterateChunkFn) (err error) {
	defer handleMetric("chunkstore_iterate", c.metrics)(&err)
	return chunkstore.Iterate(ctx, c.indexStore, c.sharkyTrx, fn)
}

func (c *chunkStoreTrx) Replace(ctx context.Context, ch swarm.Chunk, emplace bool) (err error) {
	defer handleMetric("chunkstore_replace", c.metrics)(&err)
	unlock := c.lock(ch.Address())
	defer unlock()
	return chunkstore.Replace(ctx, c.indexStore, c.sharkyTrx, ch, emplace)
}

func (c *chunkStoreTrx) lock(addr swarm.Address) func() {
	// directly lock
	if c.readOnly {
		c.globalLocker.Lock(addr.ByteString())
		return func() { c.globalLocker.Unlock(addr.ByteString()) }
	}

	// lock chunk only once in the same transaction
	if _, ok := c.lockedAddrs[addr.ByteString()]; !ok {
		c.globalLocker.Lock(addr.ByteString())
		c.lockedAddrs[addr.ByteString()] = struct{}{}
	}

	return func() {} // unlocking the chunk will be done in the Commit()
}

type indexTrx struct {
	store   storage.Reader
	batch   storage.Batch
	metrics metrics
}

func (s *indexTrx) Get(i storage.Item) error           { return s.store.Get(i) }
func (s *indexTrx) Has(k storage.Key) (bool, error)    { return s.store.Has(k) }
func (s *indexTrx) GetSize(k storage.Key) (int, error) { return s.store.GetSize(k) }
func (s *indexTrx) Iterate(q storage.Query, f storage.IterateFn) (err error) {
	defer handleMetric("iterate", s.metrics)(&err)
	return s.store.Iterate(q, f)
}
func (s *indexTrx) Count(k storage.Key) (int, error) { return s.store.Count(k) }
func (s *indexTrx) Put(i storage.Item) error         { return s.batch.Put(i) }
func (s *indexTrx) Delete(i storage.Item) error      { return s.batch.Delete(i) }

type sharkyTrx struct {
	sharky       *sharky.Store
	metrics      metrics
	writtenLocs  []sharky.Location
	releasedLocs []sharky.Location
}

func (s *sharkyTrx) Read(ctx context.Context, loc sharky.Location, buf []byte) (err error) {
	defer handleMetric("sharky_read", s.metrics)(&err)
	return s.sharky.Read(ctx, loc, buf)
}

func (s *sharkyTrx) Write(ctx context.Context, data []byte) (_ sharky.Location, err error) {
	defer handleMetric("sharky_write", s.metrics)(&err)
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

func handleMetric(key string, m metrics) func(*error) {
	t := time.Now()
	return func(err *error) {
		if err != nil && *err != nil {
			m.MethodCalls.WithLabelValues(key, "failure").Inc()
			m.MethodDuration.WithLabelValues(key, "failure").Observe(time.Since(t).Seconds())
		} else {
			m.MethodCalls.WithLabelValues(key, "success").Inc()
			m.MethodDuration.WithLabelValues(key, "success").Observe(time.Since(t).Seconds())
		}
	}
}
