// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package puller

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	SyncWorkerIterCounter prometheus.Counter // counts the number of syncing iterations
	SyncWorkerCounter     prometheus.Counter // count number of syncing jobs
	SyncWorkerDoneCounter prometheus.Counter // count number of finished syncing jobs
	SyncWorkerErrCounter  prometheus.Counter // count number of errors
	MaxUintErrCounter     prometheus.Counter // how many times we got maxuint as topmost
}

func newMetrics() metrics {
	subsystem := "puller"

	return metrics{
		SyncWorkerIterCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "worker_iterations",
			Help:      "Total history worker iterations.",
		}),
		SyncWorkerCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "worker",
			Help:      "Total history active worker jobs.",
		}),
		SyncWorkerDoneCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "worker_done",
			Help:      "Total history worker jobs done.",
		}),
		SyncWorkerErrCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "worker_errors",
			Help:      "Total history worker errors.",
		}),
		MaxUintErrCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "max_uint_errors",
			Help:      "Total max uint errors.",
		}),
	}
}

func (p *Puller) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(p.metrics)
}
