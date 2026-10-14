// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics are the index metrics of proposal §10, in the rangesync subsystem.
type metrics struct {
	// RebuildSeconds is the duration of completed rebuilds.
	RebuildSeconds prometheus.Histogram
	// ApplySeconds is the time a commit spends applying its delta.
	ApplySeconds prometheus.Histogram
	// MemoryBytes is the memory held by the slabs, both while a rebuild runs.
	MemoryBytes prometheus.Gauge
	// OutsideRoot counts elements ignored because they lie outside the root.
	OutsideRoot prometheus.Counter
}

func newMetrics() metrics {
	const subsystem = "rangesync"

	return metrics{
		RebuildSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "index_rebuild_seconds",
			Help:      "Duration of completed fingerprint index rebuilds.",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 20, 40, 80},
		}),
		ApplySeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "index_apply_seconds",
			Help:      "Time spent applying the delta of one reserve commit to the fingerprint index.",
			Buckets:   []float64{1e-6, 5e-6, 1e-5, 5e-5, 1e-4, 5e-4, 1e-3, 5e-3},
		}),
		MemoryBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "index_memory_bytes",
			Help:      "Memory held by the fingerprint index slabs, including a slab being rebuilt.",
		}),
		OutsideRoot: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "index_outside_root_total",
			Help:      "Elements ignored by the fingerprint index because they lie outside its root.",
		}),
	}
}

func (mt metrics) collectors() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(mt)
}
