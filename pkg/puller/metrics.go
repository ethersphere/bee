// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package puller

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	HistWorkerIterCounter prometheus.Counter // counts the number of historical syncing iterations
	HistWorkerDoneCounter prometheus.Counter // count number of finished historical syncing jobs
	HistWorkerErrCounter  prometheus.Counter // count number of errors
	LiveWorkerIterCounter prometheus.Counter // counts the number of live syncing iterations
	LiveWorkerErrCounter  prometheus.Counter // count number of errors
	MaxUintErrCounter     prometheus.Counter // how many times we got maxuint as topmost
}

func newMetrics() metrics {
	subsystem := "puller"

	return metrics{
		HistWorkerIterCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "hist_worker_iterations",
			Help:      "Total history worker iterations.",
		}),
		HistWorkerDoneCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "hist_worker_done",
			Help:      "Total history worker jobs done.",
		}),
		HistWorkerErrCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "hist_worker_errors",
			Help:      "Total history worker errors.",
		}),
		LiveWorkerIterCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "live_worker_iterations",
			Help:      "Total live worker iterations.",
		}),
		LiveWorkerErrCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "live_worker_errors",
			Help:      "Total live worker errors.",
		}),
		MaxUintErrCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "max_uint_errors",
			Help:      "Total max uint errors.",
		}),
	}
}

func (s *Puller) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
