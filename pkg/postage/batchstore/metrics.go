// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	Commitment        m.Gauge
	Radius            m.Gauge
	UnreserveDuration m.HistogramMetricVector
}

func newMetrics() metrics {
	subsystem := "batchstore"

	return metrics{
		Commitment: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "commitment",
			Help:      "Sum of all batches' commitment.",
		}),
		Radius: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "radius",
			Help:      "Radius of responsibility observed by the batchstore.",
		}),
		UnreserveDuration: m.NewHistogramVec(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "unreserve_duration",
			Help:      "Duration in seconds for the Unreserve call.",
		}, []string{"beforeLock"}),
	}
}

func (s *store) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
