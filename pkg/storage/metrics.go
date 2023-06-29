// Copyright 2023 The Swarm Authors. All rights reserved.
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

// metrics groups repository related prometheus counters.
type metrics struct {
	TxTotalDuration prometheus.Histogram

	IndexStoreCalls         prometheus.CounterVec
	IndexStoreCallsDuration prometheus.HistogramVec

	ChunkStoreCalls         prometheus.CounterVec
	ChunkStoreCallsDuration prometheus.HistogramVec
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "storage"

	return metrics{
		TxTotalDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "tx_total_duration",
				Help:      "Total duration of transaction.",
			},
		),
		IndexStoreCalls: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "index_store_calls",
				Help:      "Number of index store method calls.",
			},
			[]string{"method", "status"},
		),
		IndexStoreCallsDuration: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "index_store_calls_duration",
				Help:      "Duration of index store method calls.",
			},
			[]string{"method"},
		),
		ChunkStoreCalls: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "chunk_store_calls",
				Help:      "Number of chunk store method calls.",
			},
			[]string{"method", "status"},
		),
		ChunkStoreCallsDuration: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "chunk_store_calls_duration",
				Help:      "Duration of chunk store method calls.",
			},
			[]string{"method"},
		),
	}
}

var _ TxStore = (*txIndexStoreWithMetrics)(nil)

// txIndexStoreWithMetrics wraps TxStore and adds metrics.
type txIndexStoreWithMetrics struct {
	TxStore

	metrics metrics
}

// Commit implements the TxStore interface.
func (m txIndexStoreWithMetrics) Commit() error {
	dur := captureDuration(time.Now())
	err := m.TxStore.Commit()
	m.metrics.IndexStoreCallsDuration.WithLabelValues("Commit").Observe(dur())
	if err == nil {
		m.metrics.IndexStoreCalls.WithLabelValues("Commit", "success").Inc()
	} else {
		m.metrics.IndexStoreCalls.WithLabelValues("Commit", "failure").Inc()
	}
	return err
}

// Rollback implements the TxStore interface.
func (m txIndexStoreWithMetrics) Rollback() error {
	dur := captureDuration(time.Now())
	err := m.TxStore.Rollback()
	m.metrics.IndexStoreCallsDuration.WithLabelValues("Rollback").Observe(dur())
	if err == nil {
		m.metrics.IndexStoreCalls.WithLabelValues("Rollback", "success").Inc()
	} else {
		m.metrics.IndexStoreCalls.WithLabelValues("Rollback", "failure").Inc()
	}
	return err
}

// Close implements the TxStore interface.
func (m txIndexStoreWithMetrics) Close() error {
	dur := captureDuration(time.Now())
	err := m.TxStore.Close()
	m.metrics.IndexStoreCallsDuration.WithLabelValues("Close").Observe(dur())
	if err == nil {
		m.metrics.IndexStoreCalls.WithLabelValues("Close", "success").Inc()
	} else {
		m.metrics.IndexStoreCalls.WithLabelValues("Close", "failure").Inc()
	}
	return err
}

// Get implements the TxStore interface.
func (m txIndexStoreWithMetrics) Get(item Item) error {
	dur := captureDuration(time.Now())
	err := m.TxStore.Get(item)
	m.metrics.IndexStoreCallsDuration.WithLabelValues("Get").Observe(dur())
	if err == nil || errors.Is(err, ErrNotFound) {
		m.metrics.IndexStoreCalls.WithLabelValues("Get", "success").Inc()
	} else {
		m.metrics.IndexStoreCalls.WithLabelValues("Get", "failure").Inc()
	}
	return err
}

// Has implements the TxStore interface.
func (m txIndexStoreWithMetrics) Has(key Key) (bool, error) {
	dur := captureDuration(time.Now())
	has, err := m.TxStore.Has(key)
	m.metrics.IndexStoreCallsDuration.WithLabelValues("Has").Observe(dur())
	if err == nil || errors.Is(err, ErrNotFound) {
		m.metrics.IndexStoreCalls.WithLabelValues("Has", "success").Inc()
	} else {
		m.metrics.IndexStoreCalls.WithLabelValues("Has", "failure").Inc()
	}
	return has, err
}

// GetSize implements the TxStore interface.
func (m txIndexStoreWithMetrics) GetSize(key Key) (int, error) {
	dur := captureDuration(time.Now())
	size, err := m.TxStore.GetSize(key)
	m.metrics.IndexStoreCallsDuration.WithLabelValues("GetSize").Observe(dur())
	if err == nil || errors.Is(err, ErrNotFound) {
		m.metrics.IndexStoreCalls.WithLabelValues("GetSize", "success").Inc()
	} else {
		m.metrics.IndexStoreCalls.WithLabelValues("GetSize", "failure").Inc()
	}
	return size, err
}

// Iterate implements the TxStore interface.
func (m txIndexStoreWithMetrics) Iterate(query Query, fn IterateFn) error {
	dur := captureDuration(time.Now())
	err := m.TxStore.Iterate(query, fn)
	m.metrics.IndexStoreCallsDuration.WithLabelValues("Iterate").Observe(dur())
	if err == nil || errors.Is(err, ErrNotFound) {
		m.metrics.IndexStoreCalls.WithLabelValues("Iterate", "success").Inc()
	} else {
		m.metrics.IndexStoreCalls.WithLabelValues("Iterate", "failure").Inc()
	}
	return err
}

// Count implements the TxStore interface.
func (m txIndexStoreWithMetrics) Count(key Key) (int, error) {
	dur := captureDuration(time.Now())
	cnt, err := m.TxStore.Count(key)
	m.metrics.IndexStoreCallsDuration.WithLabelValues("Count").Observe(dur())
	if err == nil || errors.Is(err, ErrNotFound) {
		m.metrics.IndexStoreCalls.WithLabelValues("Count", "success").Inc()
	} else {
		m.metrics.IndexStoreCalls.WithLabelValues("Count", "failure").Inc()
	}
	return cnt, err
}

// Put implements the TxStore interface.
func (m txIndexStoreWithMetrics) Put(item Item) error {
	dur := captureDuration(time.Now())
	err := m.TxStore.Put(item)
	m.metrics.IndexStoreCallsDuration.WithLabelValues("Put").Observe(dur())
	if err == nil || errors.Is(err, ErrNotFound) {
		m.metrics.IndexStoreCalls.WithLabelValues("Put", "success").Inc()
	} else {
		m.metrics.IndexStoreCalls.WithLabelValues("Put", "failure").Inc()
	}
	return err
}

// Delete implements the TxStore interface.
func (m txIndexStoreWithMetrics) Delete(item Item) error {
	dur := captureDuration(time.Now())
	err := m.TxStore.Delete(item)
	m.metrics.IndexStoreCallsDuration.WithLabelValues("Delete").Observe(dur())
	if err == nil || errors.Is(err, ErrNotFound) {
		m.metrics.IndexStoreCalls.WithLabelValues("Delete", "success").Inc()
	} else {
		m.metrics.IndexStoreCalls.WithLabelValues("Delete", "failure").Inc()
	}
	return err
}

var _ TxChunkStore = (*txChunkStoreWithMetrics)(nil)

// txChunkStoreWithMetrics wraps TxChunkStore and adds metrics.
type txChunkStoreWithMetrics struct {
	TxChunkStore

	metrics metrics
}

// Commit implements the TxChunkStore interface.
func (m txChunkStoreWithMetrics) Commit() error {
	dur := captureDuration(time.Now())
	err := m.TxChunkStore.Commit()
	m.metrics.ChunkStoreCallsDuration.WithLabelValues("Commit").Observe(dur())
	if err == nil {
		m.metrics.ChunkStoreCalls.WithLabelValues("Commit", "success").Inc()
	} else {
		m.metrics.ChunkStoreCalls.WithLabelValues("Commit", "failure").Inc()
	}
	return err
}

// Rollback implements the TxChunkStore interface.
func (m txChunkStoreWithMetrics) Rollback() error {
	dur := captureDuration(time.Now())
	err := m.TxChunkStore.Rollback()
	m.metrics.ChunkStoreCallsDuration.WithLabelValues("Rollback").Observe(dur())
	if err == nil {
		m.metrics.ChunkStoreCalls.WithLabelValues("Rollback", "success").Inc()
	} else {
		m.metrics.ChunkStoreCalls.WithLabelValues("Rollback", "failure").Inc()
	}
	return err
}

// Close implements the TxChunkStore interface.
func (m txChunkStoreWithMetrics) Close() error {
	dur := captureDuration(time.Now())
	err := m.TxChunkStore.Close()
	m.metrics.ChunkStoreCallsDuration.WithLabelValues("Close").Observe(dur())
	if err == nil {
		m.metrics.ChunkStoreCalls.WithLabelValues("Close", "success").Inc()
	} else {
		m.metrics.ChunkStoreCalls.WithLabelValues("Close", "failure").Inc()
	}
	return err
}

// Get implements the TxChunkStore interface.
func (m txChunkStoreWithMetrics) Get(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
	dur := captureDuration(time.Now())
	chunk, err := m.TxChunkStore.Get(ctx, address)
	m.metrics.ChunkStoreCallsDuration.WithLabelValues("Get").Observe(dur())
	if err == nil || errors.Is(err, ErrNotFound) {
		m.metrics.ChunkStoreCalls.WithLabelValues("Get", "success").Inc()
	} else {
		m.metrics.ChunkStoreCalls.WithLabelValues("Get", "failure").Inc()
	}
	return chunk, err
}

// Put implements the TxChunkStore interface.
func (m txChunkStoreWithMetrics) Put(ctx context.Context, chunk swarm.Chunk) error {
	dur := captureDuration(time.Now())
	err := m.TxChunkStore.Put(ctx, chunk)
	m.metrics.ChunkStoreCallsDuration.WithLabelValues("Put").Observe(dur())
	if err == nil || errors.Is(err, ErrNotFound) {
		m.metrics.ChunkStoreCalls.WithLabelValues("Put", "success").Inc()
	} else {
		m.metrics.ChunkStoreCalls.WithLabelValues("Put", "failure").Inc()
	}
	return err
}

// Delete implements the TxChunkStore interface.
func (m txChunkStoreWithMetrics) Delete(ctx context.Context, address swarm.Address) error {
	dur := captureDuration(time.Now())
	err := m.TxChunkStore.Delete(ctx, address)
	m.metrics.ChunkStoreCallsDuration.WithLabelValues("Delete").Observe(dur())
	if err == nil || errors.Is(err, ErrNotFound) {
		m.metrics.ChunkStoreCalls.WithLabelValues("Delete", "success").Inc()
	} else {
		m.metrics.ChunkStoreCalls.WithLabelValues("Delete", "failure").Inc()
	}
	return err
}

// Has implements the TxChunkStore interface.
func (m txChunkStoreWithMetrics) Has(ctx context.Context, address swarm.Address) (bool, error) {
	dur := captureDuration(time.Now())
	has, err := m.TxChunkStore.Has(ctx, address)
	m.metrics.ChunkStoreCallsDuration.WithLabelValues("Has").Observe(dur())
	if err == nil || errors.Is(err, ErrNotFound) {
		m.metrics.ChunkStoreCalls.WithLabelValues("Has", "success").Inc()
	} else {
		m.metrics.ChunkStoreCalls.WithLabelValues("Has", "failure").Inc()
	}
	return has, err
}

// Iterate implements the TxChunkStore interface.
func (m txChunkStoreWithMetrics) Iterate(ctx context.Context, fn IterateChunkFn) error {
	dur := captureDuration(time.Now())
	err := m.TxChunkStore.Iterate(ctx, fn)
	m.metrics.ChunkStoreCallsDuration.WithLabelValues("Iterate").Observe(dur())
	if err == nil || errors.Is(err, ErrNotFound) {
		m.metrics.ChunkStoreCalls.WithLabelValues("Iterate", "success").Inc()
	} else {
		m.metrics.ChunkStoreCalls.WithLabelValues("Iterate", "failure").Inc()
	}
	return err
}

// captureDuration returns a function that returns the duration since the given start.
func captureDuration(start time.Time) (elapsed func() float64) {
	return func() float64 { return time.Since(start).Seconds() }
}
