// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	TotalToPush      m.Counter
	TotalSynced      m.Counter
	TotalErrors      m.Counter
	MarkAndSweepTime m.Histogram
	SyncTime         m.Histogram
	ErrorTime        m.Histogram
}

func newMetrics() metrics {
	subsystem := "pusher"

	return metrics{
		TotalToPush: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_to_push",
			Help:      "Total chunks to push (chunks may be repeated).",
		}),
		TotalSynced: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_synced",
			Help:      "Total chunks synced successfully with valid receipts.",
		}),
		TotalErrors: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_errors",
			Help:      "Total errors encountered.",
		}),
		SyncTime: m.NewHistogram(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sync_time",
			Help:      "Histogram of time spent to sync a chunk.",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 60},
		}),
		ErrorTime: m.NewHistogram(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "error_time",
			Help:      "Histogram of time spent before giving up on syncing a chunk.",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 60},
		}),
	}
}

func (s *Service) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
