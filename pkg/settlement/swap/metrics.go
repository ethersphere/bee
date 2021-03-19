// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swap

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	TotalReceived    prometheus.Counter
	TotalSent        prometheus.Counter
	ChequesReceived  prometheus.Counter
	ChequesSent      prometheus.Counter
	ChequesRejected  prometheus.Counter
	AvailableBalance prometheus.Gauge
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
		}),
		ChequesReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cheques_received",
			Help:      "Number of cheques received from peers",
		}),
		ChequesSent: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cheques_sent",
			Help:      "Number of cheques sent to peers",
		}),
		ChequesRejected: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cheques_rejected",
			Help:      "Number of cheques rejected",
		}),
		AvailableBalance: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "available_balance",
			Help:      "Currently availeble chequebook balance.",
		}),
	}
}

func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
