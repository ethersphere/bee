// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	Commitment        prometheus.Gauge
	Radius            prometheus.Gauge
	StorageRadius     prometheus.Gauge
	UnreserveDuration prometheus.HistogramVec
	SaveDuration      prometheus.HistogramVec
	ExistsDuration    prometheus.HistogramVec
	GetDuration       prometheus.HistogramVec
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
		StorageRadius: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "storage_radius",
			Help:      "Radius of responsibility communicated to the localstore",
		}),
		UnreserveDuration: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "unreserve_duration",
			Help:      "Duration in seconds for the Unreserve call.",
		}, []string{"beforeLock"}),
		SaveDuration: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "save_batch_duration",
			Help:      "Duration in seconds for the Save call.",
		}, []string{"beforeLock"}),
		ExistsDuration: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "exists_batch_duration",
			Help:      "Duration in seconds for the Exists call.",
		}, []string{"beforeLock"}),
		GetDuration: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "get_batch_duration",
			Help:      "Duration in seconds for the Get call.",
		}, []string{"beforeLock"}),
	}
}

func (s *store) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
