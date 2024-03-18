// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	Commitment        prometheus.Gauge
	Radius            prometheus.Gauge
	UnreserveDuration prometheus.HistogramVec
}

func newMetrics() metrics {
	subsystem := "batchstore"

	return metrics{
		Commitment: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "commitment",
			Help:      "Sum of all batches' commitment.",
		}),
		Radius: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "radius",
			Help:      "Radius of responsibility observed by the batchstore.",
		}),
		UnreserveDuration: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "unreserve_duration",
			Help:      "Duration in seconds for the Unreserve call.",
		}, []string{"beforeLock"}),
	}
}

func (s *store) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
