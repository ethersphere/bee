// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	OutOfFunds                       prometheus.Counter
	CoveringBalanceCacheHit          prometheus.Counter
	CoveringBalanceCacheMiss         prometheus.Counter
	CoveringBalanceCacheInvalidation prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "chequebook"

	return metrics{
		OutOfFunds: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "out_of_funds",
			Help:      "Number of cheque issues rejected because the chequebook had insufficient covering balance",
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
			Help:      "Number of covering balance cache invalidations (deposit / wait for deposit)",
		}),
	}
}

func (s *service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
