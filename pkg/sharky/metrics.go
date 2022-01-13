// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics groups sharky related prometheus counters.
type metrics struct {
	TotalWriteCalls    prometheus.Counter
	TotalWriteCallsErr prometheus.Counter
	TotalReadCalls     prometheus.Counter
	TotalReadCallsErr  prometheus.Counter
	ShardSize          prometheus.Gauge
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
		ShardSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "shard_size",
			Help:      "The size of the shard.",
		}),
	}
}

// Metrics returns set of prometheus collectors.
func (s *Store) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
