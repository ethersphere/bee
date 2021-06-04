// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	TotalToPush      prometheus.Counter
	TotalSynced      prometheus.Counter
	TotalErrors      prometheus.Counter
	MarkAndSweepTime prometheus.Histogram
	SyncTime         prometheus.Histogram
	ErrorTime        prometheus.Histogram

	ReceiptDepth *prometheus.CounterVec
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
		TotalErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_errors",
			Help:      "Total errors encountered.",
		}),
		MarkAndSweepTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mark_and_sweep_time",
			Help:      "Histogram of time spent in mark and sweep.",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 60},
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
		ReceiptDepth: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "receipt_depth",
				Help:      "Counter of receipts received at different depths.",
			},
			[]string{"depth"},
		),
	}
}

func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
