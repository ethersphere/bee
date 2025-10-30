// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	MethodCalls    m.CounterMetricVector
	MethodDuration m.HistogramMetricVector
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "transaction"

	return metrics{
		MethodCalls: m.NewCounterVec(
			m.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "method_calls",
				Help:      "The number of method calls.",
			},
			[]string{"method", "status"},
		),
		MethodDuration: m.NewHistogramVec(
			m.HistogramOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "method_duration",
				Help:      "The duration each method call took.",
			},
			[]string{"method", "status"},
		),
	}
}
