// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reacher

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics groups reacher related prometheus counters.
type metrics struct {
	Peers            prometheus.Gauge
	PingAttemptCount prometheus.Counter
	PingErrorCount   prometheus.Counter
	PingDuration     prometheus.Histogram
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "reacher"

	return metrics{
		Peers: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peers",
			Help:      "Number of peers currently in the reacher queue.",
		}),
		PingAttemptCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ping_attempt_count",
			Help:      "Number of ping attempts.",
		}),
		PingErrorCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ping_error_count",
			Help:      "Number of failed ping attempts.",
		}),
		PingDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ping_duration_seconds",
			Help:      "Ping latency distribution in seconds.",
			Buckets:   []float64{.1, .25, .5, 1, 2, 5, 10, 15},
		}),
	}
}

// Metrics returns set of prometheus collectors.
func (r *reacher) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(r.metrics)
}
