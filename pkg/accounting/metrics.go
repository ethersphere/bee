// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accounting

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	TotalDebitedAmount         prometheus.Counter
	TotalCreditedAmount        prometheus.Counter
	DebitEventsCount           prometheus.Counter
	CreditEventsCount          prometheus.Counter
	AccountingDisconnectsCount prometheus.Counter
	AccointingBlocksCount      prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "accounting"

	return metrics{
		TotalDebitedAmount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_debited_amount",
			Help:      "Amount of bzz debited to peers (potential income of the node)",
		}),
		TotalCreditedAmount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_credited_amount",
			Help:      "Amount of bzz credited to peers (potential cost of the node)",
		}),
		DebitEventsCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "debit_events_count",
			Help:      "Number of occurrences of bzz debit acts towards peers",
		}),
		CreditEventsCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "credit_events_count",
			Help:      "Number of occurrences of bzz credit acts towards peers",
		}),
		AccountingDisconnectsCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "accounting_disconnects_count",
			Help:      "Number of occurrences of disconnected peers based on payment thresholds",
		}),
		AccointingBlocksCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "accounting_blocks_count",
			Help:      "Number of occurrences of temporary skipping a peer to avoid crossing their disconnect thresholds",
		}),
	}
}

func (a *Accounting) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(a.metrics)
}
