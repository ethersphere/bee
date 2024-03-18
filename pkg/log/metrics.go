// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics groups various metrics counters for statistical reasons.
type metrics struct {
	ErrorCount prometheus.Counter
	WarnCount  prometheus.Counter
	InfoCount  prometheus.Counter
	DebugCount prometheus.Counter
	TraceCount prometheus.Counter
}

// Fire implements Hook interface.
func (m metrics) Fire(v Level) error {
	switch v {
	case VerbosityError:
		m.ErrorCount.Inc()
	case VerbosityWarning:
		m.WarnCount.Inc()
	case VerbosityInfo:
		m.InfoCount.Inc()
	case VerbosityDebug:
		m.DebugCount.Inc()
	default:
		m.TraceCount.Inc()
	}
	return nil
}

// newLogMetrics returns pointer to a new metrics instance ready to use.
func newLogMetrics() *metrics {
	const subsystem = "log"

	return &metrics{
		ErrorCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "error_count",
			Help:      "Number ERROR log messages.",
		}),
		WarnCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "warn_count",
			Help:      "Number WARN log messages.",
		}),
		InfoCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "info_count",
			Help:      "Number INFO log messages.",
		}),
		DebugCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "debug_count",
			Help:      "Number DEBUG log messages.",
		}),
		TraceCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "trace_count",
			Help:      "Number TRACE log messages.",
		}),
	}
}
