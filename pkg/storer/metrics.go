// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"
	"time"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// metrics groups storer related prometheus counters.
type metrics struct {
	MethodCalls             m.CounterMetricVector
	MethodCallsDuration     m.HistogramMetricVector
	ReserveSize             m.Gauge
	ReserveSizeWithinRadius m.Gauge
	ReserveCleanup          m.Counter
	StorageRadius           m.Gauge
	CacheSize               m.Gauge
	EvictedChunkCount       m.Counter
	ExpiredChunkCount       m.Counter
	OverCapTriggerCount     m.Counter
	ExpiredBatchCount       m.Counter
	LevelDBStats            m.HistogramMetricVector
	ExpiryTriggersCount     m.Counter
	ExpiryRunsCount         m.Counter

	ReserveMissingBatch m.Gauge
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "localstore"

	return metrics{
		MethodCalls: m.NewCounterVec(
			m.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "method_calls",
				Help:      "Number of method calls.",
			},
			[]string{"component", "method", "status"},
		),
		MethodCallsDuration: m.NewHistogramVec(
			m.HistogramOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "method_calls_duration",
				Help:      "Duration of method calls.",
			},
			[]string{"component", "method"},
		),
		ReserveSize: m.NewGauge(
			m.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "reserve_size",
				Help:      "Number of chunks in reserve.",
			},
		),
		ReserveMissingBatch: m.NewGauge(
			m.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "reserve_missing_batch",
				Help:      "Number of chunks in reserve with missing batches.",
			},
		),
		ReserveSizeWithinRadius: m.NewGauge(
			m.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "reserve_size_within_radius",
				Help:      "Number of chunks in reserve with proximity >= storage radius.",
			},
		),
		ReserveCleanup: m.NewCounter(
			m.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "reserve_cleanup",
				Help:      "Number of cleaned-up expired chunks.",
			},
		),
		StorageRadius: m.NewGauge(
			m.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "storage_radius",
				Help:      "Radius of responsibility reserve storage.",
			},
		),
		CacheSize: m.NewGauge(
			m.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "cache_size",
				Help:      "Number of chunks in cache.",
			},
		),
		EvictedChunkCount: m.NewCounter(
			m.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "evicted_count",
				Help:      "Number of chunks evicted from reserve.",
			},
		),
		ExpiredChunkCount: m.NewCounter(
			m.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "expired_count",
				Help:      "Number of chunks expired from reserve due to stamp expirations.",
			},
		),
		OverCapTriggerCount: m.NewCounter(
			m.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "over_cap_trigger_count",
				Help:      "Number of times the reserve was over capacity and triggered an eviction.",
			},
		),
		ExpiredBatchCount: m.NewCounter(
			m.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "expired_batch_count",
				Help:      "Number of batches expired, that were processed.",
			},
		),
		LevelDBStats: m.NewHistogramVec(
			m.HistogramOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_stats",
				Help:      "LevelDB statistics.",
			},
			[]string{"counter"},
		),
		ExpiryTriggersCount: m.NewCounter(
			m.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "expiry_trigger_count",
				Help:      "Number of batches expiry triggers.",
			},
		),
		ExpiryRunsCount: m.NewCounter(
			m.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "expiry_run_count",
				Help:      "Number of times the expiry worker was fired.",
			},
		),
	}
}

var _ storage.Putter = (*putterWithMetrics)(nil)

// putterWithMetrics wraps storage.Putter and adds metrics.
type putterWithMetrics struct {
	storage.Putter

	metrics   metrics
	component string
}

func (m putterWithMetrics) Put(ctx context.Context, chunk swarm.Chunk) error {
	dur := captureDuration(time.Now())
	err := m.Putter.Put(ctx, chunk)
	m.metrics.MethodCallsDuration.WithLabelValues(m.component, "Put").Observe(dur())
	if err == nil {
		m.metrics.MethodCalls.WithLabelValues(m.component, "Put", "success").Inc()
	} else {
		m.metrics.MethodCalls.WithLabelValues(m.component, "Put", "failure").Inc()
	}
	return err
}

var _ storage.Getter = (*getterWithMetrics)(nil)

// getterWithMetrics wraps storage.Getter and adds metrics.
type getterWithMetrics struct {
	storage.Getter

	metrics   metrics
	component string
}

func (m getterWithMetrics) Get(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
	dur := captureDuration(time.Now())
	chunk, err := m.Getter.Get(ctx, address)
	m.metrics.MethodCallsDuration.WithLabelValues(m.component, "Get").Observe(dur())
	if err == nil || errors.Is(err, storage.ErrNotFound) {
		m.metrics.MethodCalls.WithLabelValues(m.component, "Get", "success").Inc()
	} else {
		m.metrics.MethodCalls.WithLabelValues(m.component, "Get", "failure").Inc()
	}
	return chunk, err
}

// captureDuration returns a function that returns the duration since the given start.
func captureDuration(start time.Time) func() float64 {
	return func() float64 { return time.Since(start).Seconds() }
}
