// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

// metrics groups sharky related prometheus counters.
type metrics struct {
	TotalWriteCalls        m.Counter
	TotalWriteCallsErr     m.Counter
	TotalReadCalls         m.Counter
	TotalReadCallsErr      m.Counter
	TotalReleaseCalls      m.Counter
	TotalReleaseCallsErr   m.Counter
	ShardCount             m.Gauge
	CurrentShardSize       m.GaugeMetricVector
	ShardFragmentation     m.GaugeMetricVector
	LastAllocatedShardSlot m.GaugeMetricVector
	LastReleasedShardSlot  m.GaugeMetricVector
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "sharky"

	return metrics{
		TotalWriteCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_write_calls",
			Help:      "The total write calls made.",
		}),
		TotalWriteCallsErr: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_write_calls_err",
			Help:      "The total write calls ended up with error.",
		}),
		TotalReadCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_read_calls",
			Help:      "The total read calls made.",
		}),
		TotalReadCallsErr: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_read_calls_err",
			Help:      "The total read calls ended up with error.",
		}),
		TotalReleaseCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_release_calls",
			Help:      "The total release calls made.",
		}),
		TotalReleaseCallsErr: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_release_calls_err",
			Help:      "The total release calls ended up with error.",
		}),
		ShardCount: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "shard_count",
			Help:      "The number of shards.",
		}),
		CurrentShardSize: m.NewGaugeVec(
			m.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "current_shard_size",
				Help:      "The current size of the shard derived as: length in bytes/data length per chunk",
			},
			[]string{"current_shard_size"},
		),
		ShardFragmentation: m.NewGaugeVec(
			m.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "shard_fragmentation",
				Help: `
The total fragmentation of the files on disc for current shard. This is obtained by keeping track of the difference
between actual lengths of chunks and the length of slot.
			`,
			}, []string{"shard_fragmentation"},
		),
		LastAllocatedShardSlot: m.NewGaugeVec(
			m.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "last_allocated_shard_slot",
				Help:      "The slot no of the last allocated entry per shard",
			},
			[]string{"shard_slot_no"},
		),
		LastReleasedShardSlot: m.NewGaugeVec(
			m.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "last_released_shard_slot",
				Help:      "The slot no of the last released slot",
			},
			[]string{"shard_slot_no"},
		),
	}
}

// Metrics returns set of prometheus collectors.
func (s *Store) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
