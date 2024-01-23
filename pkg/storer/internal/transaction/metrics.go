// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	CommitCalls    *prometheus.CounterVec
	CommitDuration *prometheus.HistogramVec
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "transaction"

	return metrics{
		CommitCalls: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "commit_calls",
				Help:      "Number of commit calls.",
			},
			[]string{"status"},
		),
		CommitDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "commit_duration",
				Help:      "The duration each commit call took.",
			},
			[]string{"status"},
		),
	}
}
