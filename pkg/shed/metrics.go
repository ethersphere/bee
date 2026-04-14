// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shed

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	PutCounter            m.Counter
	PutFailCounter        m.Counter
	GetCounter            m.Counter
	GetFailCounter        m.Counter
	GetNotFoundCounter    m.Counter
	HasCounter            m.Counter
	HasFailCounter        m.Counter
	DeleteCounter         m.Counter
	DeleteFailCounter     m.Counter
	IteratorCounter       m.Counter
	WriteBatchCounter     m.Counter
	WriteBatchFailCounter m.Counter
}

func newMetrics() metrics {
	subsystem := "shed"

	return metrics{
		PutCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "put_count",
			Help:      "Number of times the PUT operation is done.",
		}),
		PutFailCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "put_fail_count",
			Help:      "Number of times the PUT operation failed.",
		}),
		GetCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "get_count",
			Help:      "Number of times the GET operation is done.",
		}),
		GetNotFoundCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "get_not_found_count",
			Help:      "Number of times the GET operation could not find key.",
		}),
		GetFailCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "get_fail_count",
			Help:      "Number of times the GET operation is failed.",
		}),
		HasCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "has_count",
			Help:      "Number of times the HAS operation is done.",
		}),
		HasFailCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "has_fail_count",
			Help:      "Number of times the HAS operation failed.",
		}),
		DeleteCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "delete_count",
			Help:      "Number of times the DELETE operation is done.",
		}),
		DeleteFailCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "delete_fail_count",
			Help:      "Number of times the DELETE operation failed.",
		}),
		IteratorCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "iterator_count",
			Help:      "Number of times the ITERATOR operation is done.",
		}),
		WriteBatchCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "write_batch_count",
			Help:      "Number of times the WRITE_BATCH operation is done.",
		}),
		WriteBatchFailCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "write_batch_fail_count",
			Help:      "Number of times the WRITE_BATCH operation failed.",
		}),
	}
}

func (s *DB) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
