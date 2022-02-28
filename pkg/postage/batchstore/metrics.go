// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	AvailableCapacity prometheus.Gauge
	Radius            prometheus.Gauge
	StorageRadius     prometheus.Gauge
	UnreserveDuration prometheus.Histogram
	SaveDuration      prometheus.Histogram
}

func newMetrics() metrics {
	subsystem := "batchstore"

	return metrics{
		AvailableCapacity: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "available_capacity",
			Help:      "Available capacity observed by the batchstore.",
		}),
		Radius: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "radius",
			Help:      "Radius of responsibility observed by the batchstore.",
		}),
		StorageRadius: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "storage_radius",
			Help:      "Radius of responsibility communicated to the localstore",
		}),
		UnreserveDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "unreserve_duration",
			Help:      "Duration in seconds for the Unreserve call.",
		}),
		SaveDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "save_batch_duration",
			Help:      "Duration in seconds for the Save call.",
		}),
	}
}

func (s *store) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
