// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depth

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	Network           *prometheus.GaugeVec
	StateGauge        *prometheus.GaugeVec
	WindowSamples     prometheus.Gauge
	LastUpdateAge     prometheus.Gauge
	RPCErrors         prometheus.Counter
	CandidateRejected *prometheus.CounterVec
}

func newMetrics() metrics {
	subsystem := "depth"

	return metrics{
		Network: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "network",
			Help:      "Network depth by source.",
		}, []string{"source"}),
		StateGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "state",
			Help:      "Provider state, 1 for the current state and 0 for the others.",
		}, []string{"state"}),
		WindowSamples: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "window_samples",
			Help:      "Number of samples in the estimator window.",
		}),
		LastUpdateAge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "last_update_age_seconds",
			Help:      "Seconds since the last successful chain poll.",
		}),
		RPCErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "rpc_errors_total",
			Help:      "Number of failed chain RPC calls.",
		}),
		CandidateRejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "candidate_rejected_total",
			Help:      "Number of rejected depth candidates by reason.",
		}, []string{"reason"}),
	}
}
