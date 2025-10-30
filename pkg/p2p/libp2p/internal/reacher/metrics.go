// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reacher

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	Pings    m.CounterMetricVector
	PingTime m.HistogramMetricVector
}

func newMetrics() metrics {
	subsystem := "reacher"

	return metrics{
		Pings: m.NewCounterVec(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "pings",
			Help:      "Ping counter.",
		}, []string{"status"}),
		PingTime: m.NewHistogramVec(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ping_timer",
			Help:      "Ping timer.",
		}, []string{"status"}),
	}
}

func (s *reacher) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
