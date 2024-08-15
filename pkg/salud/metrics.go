// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package salud

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	AvgDur                prometheus.Gauge
	PDur                  prometheus.Gauge
	PConns                prometheus.Gauge
	NetworkRadius         prometheus.Gauge
	NeighborhoodRadius    prometheus.Gauge
	Commitment            prometheus.Gauge
	ReserveSizePercentErr prometheus.Gauge
	Healthy               prometheus.Counter
	Unhealthy             prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "salud"

	return metrics{
		AvgDur: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "dur",
			Help:      "Average duration for snapshot response.",
		}),
		PDur: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "pdur",
			Help:      "Percentile of durations for snapshot response.",
		}),
		PConns: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "pconns",
			Help:      "Percentile of connections counts.",
		}),
		NetworkRadius: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "network_radius",
			Help:      "Most common radius across the connected peers.",
		}),
		NeighborhoodRadius: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "neighborhood_radius",
			Help:      "Most common radius across the connected peers.",
		}),
		Healthy: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "healthy",
			Help:      "Count of healthy peers.",
		}),
		Unhealthy: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "unhealthy",
			Help:      "Count of unhealthy peers.",
		}),
		Commitment: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "batch_commitment",
			Help:      "Most common batch commitment.",
		}),
		ReserveSizePercentErr: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "reserve_size_percentage_err",
			Help:      "Percentage error of the reservesize relative to the network average.",
		}),
	}
}

func (s *service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
