// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"context"
	"errors"
	"time"

	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/prometheus/client_golang/prometheus"
)

// Repository is a collection of stores that provides a unified interface
// to access them. Access to all stores can be guarded by a transaction.
type Repository interface {
	IndexStore() BatchedStore
	ChunkStore() ChunkStore

	NewTx(context.Context) (repo Repository, commit func() error, rollback func() error)
}

type repository struct {
	metrics metrics
	txStart time.Time

	txIndexStore TxStore
	txChunkStore TxChunkStore
	locker       ChunkLocker
}

// IndexStore returns Store.
func (r *repository) IndexStore() BatchedStore {
	return r.txIndexStore
}

// ChunkStore returns ChunkStore.
func (r *repository) ChunkStore() ChunkStore {
	return r.txChunkStore
}

// NewTx returns a new transaction that guards all the Repository
// stores. The transaction must be committed or rolled back.
func (r *repository) NewTx(ctx context.Context) (Repository, func() error, func() error) {
	repo := &repository{
		metrics:      r.metrics,
		txStart:      time.Now(),
		txIndexStore: txIndexStoreWithMetrics{r.txIndexStore.NewTx(NewTxState(ctx)), r.metrics},
		txChunkStore: txChunkStoreWithMetrics{
			wrapSync(r.txChunkStore.NewTx(NewTxState(ctx)), r.locker),
			r.metrics,
		},
	}

	txs := []Tx{repo.txIndexStore, repo.txChunkStore}

	commit := func() error {
		var err error
		for _, tx := range txs {
			err = tx.Commit()
			if err != nil {
				break
			}
		}
		if !errors.Is(err, ErrTxDone) {
			repo.metrics.TxTotalDuration.Observe(captureDuration(repo.txStart)())
		}
		return err
	}

	rollback := func() error {
		var errs error
		for i := len(txs) - 1; i >= 0; i-- {
			if err := txs[i].Rollback(); err != nil {
				errs = errors.Join(errs, err)
			}
		}
		if !errors.Is(errs, ErrTxDone) {
			repo.metrics.TxTotalDuration.Observe(captureDuration(repo.txStart)())
		}
		return errs
	}

	return repo, commit, rollback
}

// Metrics returns set of prometheus collectors.
func (r *repository) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(r.metrics)
}

type ChunkLocker func(chunk swarm.Address) func()

// NewRepository returns a new Repository instance.
func NewRepository(
	txIndexStore TxStore,
	txChunkStore TxChunkStore,
	locker ChunkLocker,
) Repository {
	metrics := newMetrics()
	return &repository{
		metrics:      metrics,
		txIndexStore: txIndexStoreWithMetrics{txIndexStore, metrics},
		txChunkStore: txChunkStoreWithMetrics{wrapSync(txChunkStore, locker), metrics},
		locker:       locker,
	}
}

type syncChunkStore struct {
	TxChunkStore
	locker ChunkLocker
}

func wrapSync(store TxChunkStore, locker ChunkLocker) TxChunkStore {
	return &syncChunkStore{store, locker}
}

func (s *syncChunkStore) Put(ctx context.Context, chunk swarm.Chunk) error {
	unlock := s.locker(chunk.Address())
	defer unlock()
	return s.TxChunkStore.Put(ctx, chunk)
}

func (s *syncChunkStore) Delete(ctx context.Context, addr swarm.Address) error {
	unlock := s.locker(addr)
	defer unlock()
	return s.TxChunkStore.Delete(ctx, addr)
}
