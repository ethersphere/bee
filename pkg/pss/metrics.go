// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	TotalMessagesSentCounter m.Counter
	MessageMiningDuration    m.Gauge
}

func newMetrics() metrics {
	subsystem := "pss"

	return metrics{
		TotalMessagesSentCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_message_sent",
			Help:      "Total messages sent.",
		}),
		MessageMiningDuration: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mining_duration",
			Help:      "Time duration to mine a message.",
		}),
	}
}

func (s *pss) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
