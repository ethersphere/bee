// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics groups sharky related prometheus counters.
type metrics struct {
	TotalWriteCalls        prometheus.Counter
	TotalWriteCallsErr     prometheus.Counter
	TotalReadCalls         prometheus.Counter
	TotalReadCallsErr      prometheus.Counter
	TotalReleaseCalls      prometheus.Counter
	TotalReleaseCallsErr   prometheus.Counter
	ShardCount             prometheus.Gauge
	CurrentShardSize       *prometheus.GaugeVec
	ShardFragmentation     *prometheus.GaugeVec
	LastAllocatedShardSlot *prometheus.GaugeVec
	LastReleasedShardSlot  *prometheus.GaugeVec
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "sharky"

	return metrics{
		TotalWriteCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_write_calls",
			Help:      "The total write calls made.",
		}),
		TotalWriteCallsErr: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_write_calls_err",
			Help:      "The total write calls ended up with error.",
		}),
		TotalReadCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_read_calls",
			Help:      "The total read calls made.",
		}),
		TotalReadCallsErr: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_read_calls_err",
			Help:      "The total read calls ended up with error.",
		}),
		TotalReleaseCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_release_calls",
			Help:      "The total release calls made.",
		}),
		TotalReleaseCallsErr: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_release_calls_err",
			Help:      "The total release calls ended up with error.",
		}),
		ShardCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "shard_count",
			Help:      "The number of shards.",
		}),
		CurrentShardSize: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "current_shard_size",
				Help:      "The current size of the shard derived as: length in bytes/data length per chunk",
			},
			[]string{"current_shard_size"},
		),
		ShardFragmentation: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "shard_fragmentation",
				Help: `
The total fragmentation of the files on disc for current shard. This is obtained by keeping track of the difference
between actual lengths of chunks and the length of slot.
			`,
			}, []string{"shard_fragmentation"},
		),
		LastAllocatedShardSlot: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "last_allocated_shard_slot",
				Help:      "The slot no of the last allocated entry per shard",
			},
			[]string{"shard_slot_no"},
		),
		LastReleasedShardSlot: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
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
func (s *Store) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
