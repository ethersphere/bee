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
	DBOpenCount          prometheus.Counter
	GetCount             prometheus.Counter
	GetFailCount         prometheus.Counter
	GetNotFoundCount     prometheus.Counter
	PutCount             prometheus.Counter
	PutFailCount         prometheus.Counter
	HasCount             prometheus.Counter
	HasFailCount         prometheus.Counter
	DeleteCount          prometheus.Counter
	DeleteFailCount      prometheus.Counter
	TotalCount           prometheus.Counter
	TotalFailCount       prometheus.Counter
	CountPrefixCount     prometheus.Counter
	CountPrefixFailCount prometheus.Counter
	CountFromCount       prometheus.Counter
	CountFromFailCount   prometheus.Counter
	IterationCount       prometheus.Counter
	IterationFailCount   prometheus.Counter
	FirstCount           prometheus.Counter
	FirstFailCount       prometheus.Counter
	LastCount            prometheus.Counter
	LastFailCount        prometheus.Counter
	GetBatchCount        prometheus.Counter
	WriteBatchCount      prometheus.Counter
	WriteBatchFailCount  prometheus.Counter
	DBCloseCount         prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "shed"

	return metrics{
		DBOpenCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "open_count",
			Help:      "Number of times the badger DB is opened.",
		}),
		GetCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "get_count",
			Help:      "Number of times a GET operation is performed.",
		}),
		GetFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "get_failure_count",
			Help:      "Number of times a GET operation failed.",
		}),
		GetNotFoundCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "get_not_found_count",
			Help:      "Number of times a GET operation failed.",
		}),
		PutCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "put_count",
			Help:      "Number of times a PUT operation is performed.",
		}),
		PutFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "put_failure_count",
			Help:      "Number of times a PUT operation failed.",
		}),
		HasCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "has_count",
			Help:      "Number of times a HAS operation is performed.",
		}),
		HasFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "has_failure_count",
			Help:      "Number of times a HAS operation failed.",
		}),
		DeleteCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "delete_count",
			Help:      "Number of times a DELETE operation is performed.",
		}),
		DeleteFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "delete_failure_count",
			Help:      "Number of times a DELETE operation failed.",
		}),
		TotalCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_count",
			Help:      "Number of times a COUNT operation is performed.",
		}),
		TotalFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_failure_count",
			Help:      "Number of times a COUNT operation failed.",
		}),
		CountPrefixCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "count_prefix_count",
			Help:      "Number of times a COUNT_PREFIX operation is performed.",
		}),
		CountFromFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "count_from_failure_count",
			Help:      "Number of times a COUNT_FROM operation failed.",
		}),
		CountFromCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "count_from_count",
			Help:      "Number of times a COUNT_FROM operation is performed.",
		}),
		CountPrefixFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "count_prefix_failure_count",
			Help:      "Number of times a COUNT_PREFIX operation failed.",
		}),
		IterationCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "iteration_count",
			Help:      "Number of times a ITERATION operation is performed.",
		}),
		IterationFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "iteration_failure_count",
			Help:      "Number of times a ITERATION operation failed.",
		}),
		FirstCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "first_count",
			Help:      "Number of times a FIRST operation is performed.",
		}),
		FirstFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "first_failure_count",
			Help:      "Number of times a FIRST operation failed.",
		}),
		LastCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "last_count",
			Help:      "Number of times a LAST operation is performed.",
		}),
		LastFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "last_failure_count",
			Help:      "Number of times a LAST operation failed.",
		}),
		GetBatchCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "getbatch_count",
			Help:      "Number of times a GET_BATCH operation is performed.",
		}),
		WriteBatchCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "write_batch_count",
			Help:      "Number of times a WRITE_BATCH operation is performed.",
		}),
		WriteBatchFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "write_batch_failure_count",
			Help:      "Number of times a WRITE_BATCH operation failed.",
		}),
		DBCloseCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "close_count",
			Help:      "Number of times a CLOSE operation is performed.",
		}),
	}
}

func (s *DB) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
