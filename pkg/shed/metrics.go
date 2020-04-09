// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shed

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	PutCounter            prometheus.Counter
	PutFailCounter        prometheus.Counter
	GetCounter            prometheus.Counter
	GetFailCounter        prometheus.Counter
	GetNotFoundCounter    prometheus.Counter
	HasCounter            prometheus.Counter
	HasFailCounter        prometheus.Counter
	DeleteCounter         prometheus.Counter
	DeleteFailCounter     prometheus.Counter
	IteratorCounter       prometheus.Counter
	WriteBatchCounter     prometheus.Counter
	WriteBatchFailCounter prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "shed"

	return metrics{
		PutCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "put_count",
			Help:      "Number of times the PUT operation is done.",
		}),
		PutFailCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "put_fail_count",
			Help:      "Number of times the PUT operation failed.",
		}),
		GetCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "get_count",
			Help:      "Number of times the GET operation is done.",
		}),
		GetNotFoundCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "get_not_found_count",
			Help:      "Number of times the GET operation could not find key.",
		}),
		GetFailCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "get_fail_count",
			Help:      "Number of times the GET operation is failed.",
		}),
		HasCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "has_count",
			Help:      "Number of times the HAS operation is done.",
		}),
		HasFailCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "has_fail_count",
			Help:      "Number of times the HAS operation failed.",
		}),
		DeleteCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "delete_count",
			Help:      "Number of times the DELETE operation is done.",
		}),
		DeleteFailCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "delete_fail_count",
			Help:      "Number of times the DELETE operation failed.",
		}),
		IteratorCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "iterator_count",
			Help:      "Number of times the ITERATOR operation is done.",
		}),
		WriteBatchCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "write_batch_count",
			Help:      "Number of times the WRITE_BATCH operation is done.",
		}),
		WriteBatchFailCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "write_batch_fail_count",
			Help:      "Number of times the WRITE_BATCH operation failed.",
		}),
	}
}

func (s *DB) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
