// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swap

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	TotalReceived prometheus.Counter
	TotalSent     prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "swap"

	return metrics{
		TotalReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_received",
			Help:      "Amount of tokens received from peers (income of the node)",
		}),
		TotalSent: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_sent",
			Help:      "Amount of tokens sent to peers (costs paid by the node)",
		})}
}

func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
