// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reacher

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

// metrics groups reacher related prometheus counters.
type metrics struct {
	Peers            m.Gauge
	PingAttemptCount m.Counter
	PingErrorCount   m.Counter
	PingDuration     m.Histogram
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "reacher"

	return metrics{
		Peers: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peers",
			Help:      "Number of peers currently in the reacher queue.",
		}),
		PingAttemptCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ping_attempt_count",
			Help:      "Number of ping attempts.",
		}),
		PingErrorCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ping_error_count",
			Help:      "Number of failed ping attempts.",
		}),
		PingDuration: m.NewHistogram(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ping_duration_seconds",
			Help:      "Ping latency distribution in seconds.",
			Buckets:   []float64{.1, .25, .5, 1, 2, 5, 10, 15},
		}),
	}
}

// Metrics returns set of prometheus collectors.
func (r *reacher) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(r.metrics)
}
