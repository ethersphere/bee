// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package logging

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
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

func (l *logger) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(l.metrics)
}

func (m metrics) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
		logrus.DebugLevel,
		logrus.TraceLevel,
	}
}

func (m metrics) Fire(e *logrus.Entry) error {
	switch e.Level {
	case logrus.ErrorLevel:
		m.ErrorCount.Inc()
	case logrus.WarnLevel:
		m.WarnCount.Inc()
	case logrus.InfoLevel:
		m.InfoCount.Inc()
	case logrus.DebugLevel:
		m.DebugCount.Inc()
	case logrus.TraceLevel:
		m.TraceCount.Inc()
	}
	return nil
}
