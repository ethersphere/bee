// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	Handshakes *prometheus.CounterVec
	Cohorts    prometheus.Gauge
	Published  prometheus.Counter
	Dropped    *prometheus.CounterVec

	Broadcast prometheus.Counter
	Invalid   prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "bps"

	return metrics{
		Handshakes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "handshakes",
			Help:      "Number of handshakes answered, by status.",
		}, []string{"status"}),
		Cohorts: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cohorts",
			Help:      "Number of cohorts this node brokers.",
		}),
		Published: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "published",
			Help:      "Number of messages published by local sessions.",
		}),
		Dropped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "dropped",
			Help:      "Number of messages dropped, by reason.",
		}, []string{"reason"}),
		Broadcast: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "broadcast",
			Help:      "Number of messages accepted by this broker and enqueued for fan-out. Peers dropped as too slow to receive them are counted in dropped{reason=slow_peer}.",
		}),
		Invalid: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "invalid",
			Help:      "Number of invalid messages received from publishers.",
		}),
	}
}

// Metrics returns the prometheus collectors of this service.
func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
