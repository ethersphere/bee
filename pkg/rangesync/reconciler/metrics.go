// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics are the reconciler's metrics.
type metrics struct {
	// SyncRate is the reconciler's own SyncRate (§10 sync_rate).
	SyncRate prometheus.GaugeFunc
	// Sessions counts initiated sessions by result.
	Sessions *prometheus.CounterVec
	// Failures counts PeerScorer failures.
	Failures prometheus.Counter
}

func newMetrics(syncRate func() float64) metrics {
	const subsystem = "rangesync"

	return metrics{
		SyncRate: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sync_rate",
			Help:      "Catch-up chunks per second fetched by rangesync (reported through SyncRate).",
		}, syncRate),
		Sessions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sessions_total",
			Help:      "Sessions initiated by the reconciler, by result.",
		}, []string{"result"}),
		Failures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peer_failures_total",
			Help:      "Protocol violations reported to the peer score.",
		}),
	}
}

func (m metrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{m.SyncRate, m.Sessions, m.Failures}
}
