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
	TotalDebitedAmount                       prometheus.Counter
	TotalCreditedAmount                      prometheus.Counter
	DebitEventsCount                         prometheus.Counter
	CreditEventsCount                        prometheus.Counter
	AccountingDisconnectsEnforceRefreshCount prometheus.Counter
	AccountingNonFatalRefreshFailCount       prometheus.Counter
	AccountingDisconnectsOverdrawCount       prometheus.Counter
	AccountingDisconnectsGhostOverdrawCount  prometheus.Counter
	AccountingDisconnectsReconnectCount      prometheus.Counter
	AccountingBlocksCount                    prometheus.Counter
	AccountingReserveCount                   prometheus.Counter
	TotalOriginatedCreditedAmount            prometheus.Counter
	OriginatedCreditEventsCount              prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "accounting"

	return metrics{
		TotalDebitedAmount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_debited_amount",
			Help:      "Amount of BZZ debited to peers (potential income of the node)",
		}),
		TotalCreditedAmount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_credited_amount",
			Help:      "Amount of BZZ credited to peers (potential cost of the node)",
		}),
		DebitEventsCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "debit_events_count",
			Help:      "Number of occurrences of BZZ debit events towards peers",
		}),
		CreditEventsCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "credit_events_count",
			Help:      "Number of occurrences of BZZ credit events towards peers",
		}),
		AccountingDisconnectsEnforceRefreshCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disconnects_enforce_refresh_count",
			Help:      "Number of occurrences of peers disconnected based on failed refreshment attempts",
		}),
		AccountingNonFatalRefreshFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "non_fatal_refresh_fail_count",
			Help:      "Number of occurrences of refreshments failing for peers because of peer timestamp ahead of ours",
		}),
		AccountingDisconnectsOverdrawCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disconnects_overdraw_count",
			Help:      "Number of occurrences of peers disconnected based on payment thresholds",
		}),
		AccountingDisconnectsGhostOverdrawCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disconnects_ghost_overdraw_count",
			Help:      "Number of occurrences of peers disconnected based on undebitable requests thresholds",
		}),
		AccountingDisconnectsReconnectCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disconnects_reconnect_count",
			Help:      "Number of occurrences of peers disconnected based on early attempt to reconnect",
		}),

		AccountingBlocksCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "accounting_blocks_count",
			Help:      "Number of occurrences of temporarily skipping a peer to avoid crossing their disconnect thresholds",
		}),
		AccountingReserveCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "accounting_reserve_count",
			Help:      "Number of reserve calls",
		}),
		TotalOriginatedCreditedAmount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_originated_credited_amount",
			Help:      "Amount of BZZ credited to peers (potential cost of the node) for originated traffic",
		}),
		OriginatedCreditEventsCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "originated_credit_events_count",
			Help:      "Number of occurrences of BZZ credit events as originator towards peers",
		}),
	}
}

// Metrics returns the prometheus Collector for the accounting service.
func (a *Accounting) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(a.metrics)
}
