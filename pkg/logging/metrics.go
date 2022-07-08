// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package logging

import (
	"github.com/ethersphere/bee/pkg/log"
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	ErrorCount prometheus.Counter
	WarnCount  prometheus.Counter
	InfoCount  prometheus.Counter
	DebugCount prometheus.Counter
	TraceCount prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "log"

	return metrics{
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

func (m metrics) Fire(v log.Level) error {
	switch v {
	case log.VerbosityError:
		m.ErrorCount.Inc()
	case log.VerbosityWarning:
		m.WarnCount.Inc()
	case log.VerbosityInfo:
		m.InfoCount.Inc()
	case log.VerbosityDebug:
		m.DebugCount.Inc()
	case log.VerbosityAll:
		m.TraceCount.Inc() // Irrelevant, will be removed.
	}
	return nil
}

func (w *wrapper) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(w.metrics)
}
