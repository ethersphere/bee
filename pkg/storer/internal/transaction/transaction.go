// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/segmentio/ksuid"
	"resenje.org/multex"
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

const globalLockerKey = "global"

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
	sharky           *sharky.Store
	bstore           storage.BatchedStore
	metrics          metrics
	chunkStoreLocker *chunkStoreLocker
}

func NewStorage(sharky *sharky.Store, bstore storage.BatchedStore) Storage {
	return &store{sharky, bstore, newMetrics(), &chunkStoreLocker{multex.New(), make(map[string]map[string]struct{})}}
}

type transaction struct {
	batch      storage.Batch
	indexstore *indexTrx
	chunkStore *chunkStoreTrx
	sharkyTrx  *sharkyTrx
	metrics    metrics
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
	sharyTrx := &sharkyTrx{s.sharky, s.metrics, nil, nil}

	t := &transaction{
		batch:      b,
		indexstore: indexTrx,
		chunkStore: &chunkStoreTrx{indexTrx, sharyTrx, s.chunkStoreLocker, s.chunkStoreLocker.newID(), s.metrics, false},
		sharkyTrx:  sharyTrx,
		metrics:    s.metrics,
	}

	return t, func() {
		// for whatever reason, the commit call was not made
		// release uncommitted written sharky locations
		for _, l := range t.sharkyTrx.writtenLocs {
			_ = t.sharkyTrx.sharky.Release(context.TODO(), l)
		}
		t.chunkStore.chunkStoreLocker.unlockTrx(t.chunkStore.id)
	}
}

type readOnly struct {
	indexStore *indexTrx
	chunkStore *chunkStoreTrx
}

func (s *store) ReadOnly() ReadOnlyStore {
	indexStore := &indexTrx{s.bstore, nil}
	sharyTrx := &sharkyTrx{s.sharky, s.metrics, nil, nil}

	return &readOnly{indexStore, &chunkStoreTrx{indexStore, sharyTrx, s.chunkStoreLocker, "", s.metrics, true}}
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
	return t.chunkStore
}

func (t *transaction) Commit() (err error) {

	defer handleMetric("commit", t.metrics)(err)
	defer func() {
		t.chunkStore.chunkStoreLocker.unlockTrx(t.chunkStore.id)
		t.sharkyTrx.writtenLocs = nil
	}()

	h := handleMetric("batch_commit", t.metrics)
	err = t.batch.Commit()
	h(err)
	if err != nil {
		for _, l := range t.sharkyTrx.writtenLocs {
			if rerr := t.sharkyTrx.sharky.Release(context.TODO(), l); rerr != nil {
				err = errors.Join(err, fmt.Errorf("failed releasing location during commit rollback %s: %w", l, rerr))
			}
		}
		return err
	}

	for _, l := range t.sharkyTrx.releasedLocs {
		h := handleMetric("sharky_release", t.metrics)
		if rerr := t.sharkyTrx.sharky.Release(context.TODO(), l); rerr != nil {
			err = errors.Join(err, fmt.Errorf("failed releasing location afer commit %s: %w", l, rerr))
			h(err)
		}
	}

	return err
}

type chunkStoreTrx struct {
	indexStore       *indexTrx
	sharkyTrx        *sharkyTrx
	chunkStoreLocker *chunkStoreLocker
	id               string
	metrics          metrics
	readOnly         bool
}

func (c *chunkStoreTrx) Get(ctx context.Context, addr swarm.Address) (ch swarm.Chunk, err error) {
	defer handleMetric("chunkstore_get", c.metrics)(err)
	defer c.lock(addr)()
	ch, err = chunkstore.Get(ctx, c.indexStore, c.sharkyTrx, addr)
	return ch, err
}
func (c *chunkStoreTrx) Has(ctx context.Context, addr swarm.Address) (_ bool, err error) {
	defer handleMetric("chunkstore_has", c.metrics)(err)
	defer c.lock(addr)()
	return chunkstore.Has(ctx, c.indexStore, addr)
}
func (c *chunkStoreTrx) Put(ctx context.Context, ch swarm.Chunk) (err error) {
	defer handleMetric("chunkstore_put", c.metrics)(err)
	defer c.lock(ch.Address())()
	return chunkstore.Put(ctx, c.indexStore, c.sharkyTrx, ch)
}
func (c *chunkStoreTrx) Delete(ctx context.Context, addr swarm.Address) (err error) {
	defer handleMetric("chunkstore_delete", c.metrics)(err)
	defer c.lock(addr)()
	return chunkstore.Delete(ctx, c.indexStore, c.sharkyTrx, addr)
}
func (c *chunkStoreTrx) Iterate(ctx context.Context, fn storage.IterateChunkFn) error {
	return chunkstore.Iterate(ctx, c.indexStore, c.sharkyTrx, fn)
}

func (c *chunkStoreTrx) lock(addr swarm.Address) func() {
	return func() {
		if c.readOnly {
			c.chunkStoreLocker.lock(addr)
			defer c.chunkStoreLocker.unlock(addr)
		} else {
			c.chunkStoreLocker.lockTrx(addr, c.id)
		}
	}
}

type sharkyTrx struct {
	sharky       *sharky.Store
	metrics      metrics
	writtenLocs  []sharky.Location
	releasedLocs []sharky.Location
}

func (s *sharkyTrx) Read(ctx context.Context, loc sharky.Location, buf []byte) (err error) {
	defer handleMetric("sharky_read", s.metrics)(err)
	return s.sharky.Read(ctx, loc, buf)
}

func (s *sharkyTrx) Write(ctx context.Context, data []byte) (_ sharky.Location, err error) {
	defer handleMetric("sharky_write", s.metrics)(err)
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

type chunkStoreLocker struct {
	mtx         *multex.Multex
	chunkLocked map[string]map[string]struct{} // transactionID -> chunkAddress, one to many mapping
}

// lock globally locks the chunk address.
func (c *chunkStoreLocker) lock(addr swarm.Address) {
	c.mtx.Lock(addr.ByteString())
}

// lock globally unlocks the chunk address.
func (c *chunkStoreLocker) unlock(addr swarm.Address) {
	c.mtx.Unlock(addr.ByteString())
}

// lockTrx globally locks the chunk address, also making sure that the same address is locked only once by the same transaction.
func (c *chunkStoreLocker) lockTrx(addr swarm.Address, id string) {
	c.mtx.Lock(globalLockerKey)
	defer c.mtx.Unlock(globalLockerKey)

	if c.chunkLocked[id] == nil {
		c.chunkLocked[id] = make(map[string]struct{})
	}

	// chunk already globally locked as part of this transaction
	if _, ok := c.chunkLocked[id][addr.ByteString()]; ok {
		return
	}

	c.chunkLocked[id][addr.ByteString()] = struct{}{}

	// acquire global chunk lock
	c.lock(addr)
}

// unlockTrx globally unlocks any chunk addresses that was part of the transaction.
func (c *chunkStoreLocker) unlockTrx(id string) {

	c.mtx.Lock(globalLockerKey)
	defer c.mtx.Unlock(globalLockerKey)

	for addr := range c.chunkLocked[id] {
		c.mtx.Unlock(addr)
	}

	delete(c.chunkLocked, id)
}

func (c *chunkStoreLocker) newID() string {
	return ksuid.New().String()
}

func handleMetric(key string, m metrics) func(err error) {
	t := time.Now()
	return func(err error) {
		if err != nil {
			m.MethodCalls.WithLabelValues(key, "failure").Inc()
			m.MethodDuration.WithLabelValues(key, "failure").Observe(float64(time.Since(t)))
		} else {
			m.MethodCalls.WithLabelValues(key, "success").Inc()
			m.MethodDuration.WithLabelValues(key, "success").Observe(float64(time.Since(t)))
		}
	}
}
