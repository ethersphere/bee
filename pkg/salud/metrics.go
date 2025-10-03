// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package salud

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	AvgDur                m.Gauge
	PDur                  m.Gauge
	PConns                m.Gauge
	NetworkRadius         m.Gauge
	NeighborhoodRadius    m.Gauge
	Commitment            m.Gauge
	ReserveSizePercentErr m.Gauge
	Healthy               m.Counter
	Unhealthy             m.Counter
	NeighborhoodAvgDur    m.Gauge
	NeighborCount         m.Gauge
}

func newMetrics() metrics {
	subsystem := "salud"

	return metrics{
		AvgDur: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "dur",
			Help:      "Average duration for snapshot response.",
		}),
		PDur: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "pdur",
			Help:      "Percentile of durations for snapshot response.",
		}),
		PConns: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "pconns",
			Help:      "Percentile of connections counts.",
		}),
		NetworkRadius: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "network_radius",
			Help:      "Most common radius across the connected peers.",
		}),
		NeighborhoodRadius: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "neighborhood_radius",
			Help:      "Most common radius across the connected peers.",
		}),
		Healthy: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "healthy",
			Help:      "Count of healthy peers.",
		}),
		Unhealthy: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "unhealthy",
			Help:      "Count of unhealthy peers.",
		}),
		Commitment: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "batch_commitment",
			Help:      "Most common batch commitment.",
		}),
		ReserveSizePercentErr: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "reserve_size_percentage_err",
			Help:      "Percentage error of the reservesize relative to the network average.",
		}),
		// Neighborhood-specific metrics
		NeighborhoodAvgDur: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "neighborhood_dur",
			Help:      "Average duration for snapshot response from neighborhood peers.",
		}),
		NeighborCount: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "neighbors",
			Help:      "Number of neighborhood peers.",
		}),
	}
}

func (s *service) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
