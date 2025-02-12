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
	"github.com/prometheus/client_golang/prometheus"
)

// metrics groups storer related prometheus counters.
type metrics struct {
	MethodCalls             prometheus.CounterVec
	MethodCallsDuration     prometheus.HistogramVec
	ReserveSize             prometheus.Gauge
	ReserveSizeWithinRadius prometheus.Gauge
	ReserveCleanup          prometheus.Counter
	StorageRadius           prometheus.Gauge
	CacheSize               prometheus.Gauge
	EvictedChunkCount       prometheus.Counter
	ExpiredChunkCount       prometheus.Counter
	OverCapTriggerCount     prometheus.Counter
	ExpiredBatchCount       prometheus.Counter
	LevelDBStats            prometheus.HistogramVec
	ExpiryTriggersCount     prometheus.Counter
	ExpiryRunsCount         prometheus.Counter

	ReserveMissingBatch prometheus.Gauge
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "localstore"

	return metrics{
		MethodCalls: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "method_calls",
				Help:      "Number of method calls.",
			},
			[]string{"component", "method", "status"},
		),
		MethodCallsDuration: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "method_calls_duration",
				Help:      "Duration of method calls.",
			},
			[]string{"component", "method"},
		),
		ReserveSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "reserve_size",
				Help:      "Number of chunks in reserve.",
			},
		),
		ReserveMissingBatch: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "reserve_missing_batch",
				Help:      "Number of chunks in reserve with missing batches.",
			},
		),
		ReserveSizeWithinRadius: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "reserve_size_within_radius",
				Help:      "Number of chunks in reserve with proximity >= storage radius.",
			},
		),
		ReserveCleanup: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "reserve_cleanup",
				Help:      "Number of cleaned-up expired chunks.",
			},
		),
		StorageRadius: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "storage_radius",
				Help:      "Radius of responsibility reserve storage.",
			},
		),
		CacheSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "cache_size",
				Help:      "Number of chunks in cache.",
			},
		),
		EvictedChunkCount: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "evicted_count",
				Help:      "Number of chunks evicted from reserve.",
			},
		),
		ExpiredChunkCount: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "expired_count",
				Help:      "Number of chunks expired from reserve due to stamp expirations.",
			},
		),
		OverCapTriggerCount: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "over_cap_trigger_count",
				Help:      "Number of times the reserve was over capacity and triggered an eviction.",
			},
		),
		ExpiredBatchCount: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "expired_batch_count",
				Help:      "Number of batches expired, that were processed.",
			},
		),
		LevelDBStats: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_stats",
				Help:      "LevelDB statistics.",
			},
			[]string{"counter"},
		),
		ExpiryTriggersCount: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "expiry_trigger_count",
				Help:      "Number of batches expiry triggers.",
			},
		),
		ExpiryRunsCount: prometheus.NewCounter(
			prometheus.CounterOpts{
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

// putterWithMetrics wraps storage.Putter and adds metrics.
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
