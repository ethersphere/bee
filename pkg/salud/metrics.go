// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package salud

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	Dur         prometheus.Gauge
	Radius      prometheus.Gauge
	Blocklisted prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "pushsync"

	return metrics{
		Dur: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "dur",
			Help:      "Average duration for snapshot response.",
		}),
		Radius: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "radius",
			Help:      "Most common radius across the connected peers.",
		}),
		Blocklisted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "blocklisted",
			Help:      "Total number of blocklisted peers.",
		}),
	}
}

func (s *service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
