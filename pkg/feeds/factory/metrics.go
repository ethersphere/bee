// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"github.com/prometheus/client_golang/prometheus"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	LookupStarted  *prometheus.CounterVec
	LookupDuration *prometheus.HistogramVec
}

func newMetrics() metrics {
	subsystem := "feeds"

	return metrics{
		LookupStarted: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "lookup_started_total",
				Help:      "Number of feed lookup attempts started.",
			},
			[]string{"type"},
		),
		LookupDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "lookup_duration_seconds",
				Help:      "Histogram of feed lookup durations.",
				Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"type", "result"},
		),
	}
}

func (f *factory) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(f.metrics)
}
