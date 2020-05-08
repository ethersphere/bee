// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection

	TotalChunksToBeSentCounter prometheus.Counter
	TotalChunksSynced          prometheus.Counter
	ErrorSettingChunkToSynced  prometheus.Counter
	MarkAndSweepTimer          prometheus.Histogram
}

func newMetrics() metrics {
	subsystem := "pushsync"

	return metrics{
		TotalChunksToBeSentCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_chunk_to_be_sent",
			Help:      "Total chunks to be sent.",
		}),
		TotalChunksSynced: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_chunk_synced",
			Help:      "Total chunks synced succesfully with valid receipts.",
		}),
		ErrorSettingChunkToSynced: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cannot_set_chunk_sync_in_DB",
			Help:      "Total no of times the chunk cannot be synced in DB.",
		}),
		MarkAndSweepTimer: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mark_and_sweep_time_histogram",
			Help:      "Histogram of time spent in mark and sweep.",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 60},
		}),
	}
}

func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
