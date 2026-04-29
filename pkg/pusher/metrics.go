// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	TotalToPush       prometheus.Counter
	TotalSynced       prometheus.Counter
	TotalCouldNotSync prometheus.Counter
	TotalErrors       prometheus.Counter
	MarkAndSweepTime  prometheus.Histogram
	SyncTime          prometheus.Histogram
	ErrorTime         prometheus.Histogram
}

func newMetrics() metrics {
	subsystem := "pusher"

	return metrics{
		TotalToPush: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_to_push",
			Help:      "Total chunks to push (chunks may be repeated).",
		}),
		TotalSynced: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_synced",
			Help:      "Total chunks synced successfully with valid receipts.",
		}),
		TotalCouldNotSync: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_could_not_sync",
			Help:      "Total chunks abandoned after exhausting retries with no valid receipt (shallow receipt or no peer in the correct neighbourhood).",
		}),
		TotalErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_errors",
			Help:      "Total errors encountered.",
		}),
		SyncTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sync_time",
			Help:      "Histogram of time spent to sync a chunk.",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 60},
		}),
		ErrorTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "error_time",
			Help:      "Histogram of time spent before giving up on syncing a chunk.",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 60},
		}),
	}
}

func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
