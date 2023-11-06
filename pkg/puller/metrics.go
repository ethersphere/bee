// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package puller

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	SyncWorkerIterCounter prometheus.Counter    // counts the number of syncing iterations
	SyncWorkerCounter     prometheus.Counter    // count number of syncing jobs
	SyncedCounter         prometheus.CounterVec // number of synced chunks
	SyncWorkerDoneCounter prometheus.Counter    // count number of finished syncing jobs
	SyncWorkerErrCounter  prometheus.Counter    // count number of errors
	MaxUintErrCounter     prometheus.Counter    // how many times we got maxuint as topmost
}

func newMetrics() metrics {
	subsystem := "puller"

	return metrics{
		SyncWorkerIterCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "worker_iterations",
			Help:      "Total worker iterations.",
		}),
		SyncWorkerCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "worker",
			Help:      "Total active worker jobs.",
		}),
		SyncedCounter: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "synced_chunks",
			Help:      "Total synced chunks.",
		}, []string{"type"}),
		SyncWorkerDoneCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "worker_done",
			Help:      "Total worker jobs done.",
		}),
		SyncWorkerErrCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "worker_errors",
			Help:      "Total worker errors.",
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
