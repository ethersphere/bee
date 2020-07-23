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
	DebitCount        prometheus.Counter
	CreditCount       prometheus.Counter
	DebitEvents       prometheus.Counter
	CreditEvents      prometheus.Counter
	CreditDisconnects prometheus.Counter
	CreditBlocks      prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "accounting"

	return metrics{
		DebitCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_debited_amount",
			Help:      "Amount of bzz debited to peers (potential income of the node)",
		}),
		CreditCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_credited_amount",
			Help:      "Amount of bzz credited to peers (potential cost of the node)",
		}),
		DebitEvents: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_debit_events_count",
			Help:      "Number of occurences of bzz debit acts towards peers",
		}),
		CreditEvents: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_credit_events_count",
			Help:      "Number of occurences of bzz credit acts towards peers",
		}),
		CreditDisconnects: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "credit_related_disconnects_count",
			Help:      "Number of occurences of disconnected peers based on payment thresholds",
		}),
		CreditBlocks: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "credit_related_blocks_count",
			Help:      "Number of occurences of temporary skipping a peer to avoid crossing their disconnect thresholds",
		}),
	}
}

func (a *Accounting) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(a.metrics)
}
