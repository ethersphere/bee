// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"math/big"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	OutOfFunds                       prometheus.Counter
	CoveringBalance                  prometheus.Gauge
	AvailableBalance                 prometheus.Gauge
	CoveringBalanceCacheHit          prometheus.Counter
	CoveringBalanceCacheMiss         prometheus.Counter
	CoveringBalanceCacheInvalidation prometheus.Counter
}

func newMetrics() metrics {
	subsystem := loggerName

	return metrics{
		OutOfFunds: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "out_of_funds",
			Help:      "Number of cheque issues rejected because the chequebook had insufficient covering balance",
		}),
		CoveringBalance: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "covering_balance",
			Help:      "Chequebook balance plus total paid out, as held in the covering balance cache",
		}),
		AvailableBalance: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "available_balance",
			Help:      "Balance available for new cheques after the last cheque issue or refusal",
		}),
		CoveringBalanceCacheHit: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "covering_balance_cache_hit",
			Help:      "Number of covering balance lookups served from the in-memory cache",
		}),
		CoveringBalanceCacheMiss: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "covering_balance_cache_miss",
			Help:      "Number of covering balance lookups that reloaded balance+totalPaidOut from chain",
		}),
		CoveringBalanceCacheInvalidation: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "covering_balance_cache_invalidation",
			Help:      "Number of covering balance cache invalidations after a confirmed deposit",
		}),
	}
}

func (s *service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}

func setTokenGauge(g prometheus.Gauge, v *big.Int) {
	value, _ := new(big.Float).SetInt(v).Float64()
	g.Set(value)
}
