// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	OfferCounter    prometheus.Counter // number of chunks offered
	WantCounter     prometheus.Counter // number of chunks wanted
	DeliveryCounter prometheus.Counter // number of chunk deliveries
	DbOpsCounter    prometheus.Counter // number of db ops
}

func newMetrics() metrics {
	subsystem := "pullstorage"

	return metrics{
		OfferCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "chunks_offered",
			Help:      "Total chunks offered.",
		}),
		WantCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "chunks_wanted",
			Help:      "Total chunks wanted.",
		}),
		DeliveryCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "chunks_delivered",
			Help:      "Total chunks delivered.",
		}),
		DbOpsCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "db_ops",
			Help:      "Total Db Ops.",
		})}
}

func (s *Syncer) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
