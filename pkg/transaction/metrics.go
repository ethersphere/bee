// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	FeeHistoryCalls       prometheus.Counter
	FeeHistoryParseErrors prometheus.Counter
	FeeHistoryFallbacks   prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "eth_backend"

	return metrics{
		FeeHistoryCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_fee_history",
			Help:      "Count of eth_feeHistory rpc calls",
		}),
		FeeHistoryParseErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "fee_history_parse_errors",
			Help:      "Count of failures to derive suggested fees from fee history response",
		}),
		FeeHistoryFallbacks: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "fee_history_fallbacks",
			Help:      "Count of fee priority suggestions that fell back to the default tip suggestion",
		}),
	}
}

func (t *transactionService) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(t.metrics)
}
