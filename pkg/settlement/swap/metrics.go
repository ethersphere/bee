// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swap

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	TotalReceived    m.Counter
	TotalSent        m.Counter
	ChequesReceived  m.Counter
	ChequesSent      m.Counter
	ChequesRejected  m.Counter
	AvailableBalance m.Gauge
}

func newMetrics() metrics {
	subsystem := "swap"

	return metrics{
		TotalReceived: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_received",
			Help:      "Amount of tokens received from peers (income of the node)",
		}),
		TotalSent: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_sent",
			Help:      "Amount of tokens sent to peers (costs paid by the node)",
		}),
		ChequesReceived: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cheques_received",
			Help:      "Number of cheques received from peers",
		}),
		ChequesSent: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cheques_sent",
			Help:      "Number of cheques sent to peers",
		}),
		ChequesRejected: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cheques_rejected",
			Help:      "Number of cheques rejected",
		}),
		AvailableBalance: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "available_balance",
			Help:      "Currently available chequebook balance.",
		}),
	}
}

func (s *Service) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
