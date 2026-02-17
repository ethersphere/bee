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
	MethodCalls                      *prometheus.CounterVec
	MethodCallsDuration              *prometheus.HistogramVec
	ReserveSize                      prometheus.Gauge
	ReserveSizeWithinRadius          prometheus.Gauge
	ReserveCleanup                   prometheus.Counter
	StorageRadius                    prometheus.Gauge
	CacheSize                        prometheus.Gauge
	EvictedChunkCount                prometheus.Counter
	ExpiredChunkCount                prometheus.Counter
	OverCapTriggerCount              prometheus.Counter
	ExpiredBatchCount                prometheus.Counter
	LevelDBStats                     *prometheus.HistogramVec
	ExpiryTriggersCount              prometheus.Counter
	ExpiryRunsCount                  prometheus.Counter
	ReserveMissingBatch              prometheus.Gauge
	ReserveSampleDuration            *prometheus.HistogramVec
	ReserveSampleRunSummary          *prometheus.GaugeVec
	ReserveSampleLastRunTimestamp    prometheus.Gauge
	LevelDBBlockCacheSize            prometheus.Gauge
	LevelDBAliveSnapshots            prometheus.Gauge
	LevelDBAliveIterators            prometheus.Gauge
	LevelDBIOWrite                   prometheus.Gauge
	LevelDBIORead                    prometheus.Gauge
	LevelDBWriteDelayCount           prometheus.Counter
	LevelDBWriteDelayDuration        prometheus.Counter
	LevelDBMemComp                   prometheus.Counter
	LevelDBLevel0Comp                prometheus.Counter
	LevelDBConfigWriteBufferSize     prometheus.Gauge
	LevelDBConfigBlockCacheCapacity  prometheus.Gauge
	LevelDBConfigCompactionL0Trigger prometheus.Gauge
	LevelDBConfigCompactionTableSize prometheus.Gauge
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "localstore"

	return metrics{
		MethodCalls: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "method_calls",
				Help:      "Number of method calls.",
			},
			[]string{"component", "method", "status"},
		),
		MethodCallsDuration: prometheus.NewHistogramVec(
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
		LevelDBStats: prometheus.NewHistogramVec(
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
		ReserveSampleDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "reserve_sample_duration_seconds",
				Help:      "Duration of ReserveSample operations in seconds.",
				Buckets:   []float64{180, 300, 600, 900, 1200, 1500, 1800},
			},
			[]string{"status"},
		),
		ReserveSampleRunSummary: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "reserve_sample_run_summary",
				Help:      "Summary metrics for the last ReserveSample run.",
			},
			[]string{"metric"},
		),
		ReserveSampleLastRunTimestamp: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "reserve_sample_last_run_timestamp",
				Help:      "Unix timestamp of the last ReserveSample run completion.",
			},
		),
		LevelDBBlockCacheSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_block_cache_size",
				Help:      "LevelDB block cache size.",
			},
		),
		LevelDBAliveSnapshots: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_alive_snapshots",
				Help:      "LevelDB alive snapshots.",
			},
		),
		LevelDBAliveIterators: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_alive_iterators",
				Help:      "LevelDB alive iterators.",
			},
		),
		LevelDBIOWrite: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_io_write",
				Help:      "LevelDB IO write.",
			},
		),
		LevelDBIORead: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_io_read",
				Help:      "LevelDB IO read.",
			},
		),
		LevelDBWriteDelayCount: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_write_delay_count",
				Help:      "LevelDB write delay count.",
			},
		),
		LevelDBWriteDelayDuration: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_write_delay_duration_seconds",
				Help:      "LevelDB write delay duration in seconds.",
			},
		),
		LevelDBMemComp: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_mem_comp",
				Help:      "LevelDB mem compaction.",
			},
		),
		LevelDBLevel0Comp: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_level0_comp",
				Help:      "LevelDB level0 compaction.",
			},
		),
		LevelDBConfigWriteBufferSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_config_write_buffer_size",
				Help:      "LevelDB config write buffer size.",
			},
		),
		LevelDBConfigBlockCacheCapacity: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_config_block_cache_capacity",
				Help:      "LevelDB config block cache capacity.",
			},
		),
		LevelDBConfigCompactionL0Trigger: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_config_compaction_l0_trigger",
				Help:      "LevelDB config compaction l0 trigger.",
			},
		),
		LevelDBConfigCompactionTableSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "leveldb_config_compaction_table_size",
				Help:      "LevelDB config compaction table size.",
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
