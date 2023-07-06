// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"context"
	"errors"
	"github.com/ethersphere/bee/pkg/log"
	"time"

	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// Repository is a collection of stores that provides a unified interface
// to access them. Access to all stores can be guarded by a transaction.
type Repository interface {
	IndexStore() Store
	ChunkStore() ChunkStore

	NewTx(context.Context) (repo Repository, commit func() error, rollback func() error)
}

type repository struct {
	logger  log.Logger
	metrics metrics
	txStart time.Time

	txIndexStore TxStore
	txChunkStore TxChunkStore
}

// IndexStore returns Store.
func (r *repository) IndexStore() Store {
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
		txIndexStore: txIndexStoreWithMetrics{r.txIndexStore.NewTx(NewTxState(ctx)), r.logger, r.metrics},
		txChunkStore: txChunkStoreWithMetrics{r.txChunkStore.NewTx(NewTxState(ctx)), r.metrics},
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

// NewRepository returns a new Repository instance.
func NewRepository(txIndexStore TxStore, txChunkStore TxChunkStore) Repository {
	logger := log.NewLogger("repository").Register()
	metrics := newMetrics()
	return &repository{
		logger:       logger,
		metrics:      metrics,
		txIndexStore: txIndexStoreWithMetrics{txIndexStore, logger, metrics},
		txChunkStore: txChunkStoreWithMetrics{txChunkStore, metrics},
	}
}
