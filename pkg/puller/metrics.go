// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package puller

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	SyncWorkerIterCounter m.Counter             // counts the number of syncing iterations
	SyncWorkerCounter     m.Gauge               // count number of syncing jobs
	SyncedCounter         m.CounterMetricVector // number of synced chunks
	SyncWorkerErrCounter  m.Counter             // count number of errors
	MaxUintErrCounter     m.Counter             // how many times we got maxuint as topmost
}

func newMetrics() metrics {
	subsystem := "puller"

	return metrics{
		SyncWorkerIterCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "worker_iterations",
			Help:      "Total worker iterations.",
		}),
		SyncWorkerCounter: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "worker",
			Help:      "Total active worker jobs.",
		}),
		SyncedCounter: m.NewCounterVec(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "synced_chunks",
			Help:      "Total synced chunks.",
		}, []string{"type"}),
		SyncWorkerErrCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "worker_errors",
			Help:      "Total worker errors.",
		}),
		MaxUintErrCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "max_uint_errors",
			Help:      "Total max uint errors.",
		}),
	}
}

func (p *Puller) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(p.metrics)
}
