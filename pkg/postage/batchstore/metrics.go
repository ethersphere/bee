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
	Inner             prometheus.Gauge
	Outer             prometheus.Gauge
	Radius            prometheus.Gauge
	StorageRadius     prometheus.Gauge
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
		Inner: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "inner",
			Help:      "Inner storage tier value observed by the batchstore.",
		}),
		Outer: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "outer",
			Help:      "Outer storage tier value observed by the batchstore.",
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
	}
}

func (s *store) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
