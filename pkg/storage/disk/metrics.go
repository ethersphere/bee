// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package disk

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	DBOpenCount              prometheus.Counter
	DiskGetCount             prometheus.Counter
	DiskGetFailCount         prometheus.Counter
	DiskPutCount             prometheus.Counter
	DiskPutFailCount         prometheus.Counter
	DiskHasCount             prometheus.Counter
	DiskHasFailCount         prometheus.Counter
	DiskDeleteCount          prometheus.Counter
	DiskDeleteFailCount      prometheus.Counter
	DiskTotalCount           prometheus.Counter
	DiskTotalFailCount       prometheus.Counter
	DiskCountPrefixCount     prometheus.Counter
	DiskCountPrefixFailCount prometheus.Counter
	DiskCountFromCount       prometheus.Counter
	DiskCountFromFailCount   prometheus.Counter
	DiskIterationCount       prometheus.Counter
	DiskIterationFailCount   prometheus.Counter
	DiskFirstCount           prometheus.Counter
	DiskFirstFailCount       prometheus.Counter
	DiskLastCount            prometheus.Counter
	DiskLastFailCount        prometheus.Counter
	DiskGetBatchCount        prometheus.Counter
	DiskWriteBatchCount      prometheus.Counter
	DiskWriteBatchFailCount  prometheus.Counter
	DiskCloseCount           prometheus.Counter
	DataValidationCount      prometheus.Counter
	DataValidationFailCount  prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "storage_disk"
	return metrics{
		DBOpenCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_db_open_count",
			Help:      "Number of times the badger DB is opened.",
		}),
		DiskGetCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_get_count",
			Help:      "Number of times a GET operation is performed.",
		}),
		DiskGetFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_get_failure_count",
			Help:      "Number of times a GET operation failed.",
		}),
		DiskPutCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_put_count",
			Help:      "Number of times a PUT operation is performed.",
		}),
		DiskPutFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_put_failure_count",
			Help:      "Number of times a PUT operation failed.",
		}),
		DiskHasCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_has_count",
			Help:      "Number of times a HAS operation is performed.",
		}),
		DiskHasFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_has_failure_count",
			Help:      "Number of times a HAS operation failed.",
		}),
		DiskDeleteCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_delete_count",
			Help:      "Number of times a DELETE operation is performed.",
		}),
		DiskDeleteFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_delete_failure_count",
			Help:      "Number of times a DELETE operation failed.",
		}),
		DiskTotalCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_total_count",
			Help:      "Number of times a COUNT operation is performed.",
		}),
		DiskTotalFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_total_failure_count",
			Help:      "Number of times a COUNT operation failed.",
		}),
		DiskCountPrefixCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_count_prefix_count",
			Help:      "Number of times a COUNT_PREFIX operation is performed.",
		}),
		DiskCountFromFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_count_from_failure_count",
			Help:      "Number of times a COUNT_FROM operation failed.",
		}),
		DiskCountFromCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_count_from_count",
			Help:      "Number of times a COUNT_FROM operation is performed.",
		}),
		DiskCountPrefixFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_count_prefix_failure_count",
			Help:      "Number of times a COUNT_PREFIX operation failed.",
		}),

		DiskIterationCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_iteration_count",
			Help:      "Number of times a ITERATION operation is performed.",
		}),
		DiskIterationFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_iteration_failure_count",
			Help:      "Number of times a ITERATION operation failed.",
		}),
		DiskFirstCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_first_count",
			Help:      "Number of times a FIRST operation is performed.",
		}),
		DiskFirstFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_first_failure_count",
			Help:      "Number of times a FIRST operation failed.",
		}),
		DiskLastCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_last_count",
			Help:      "Number of times a LAST operation is performed.",
		}),
		DiskLastFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_last_failure_count",
			Help:      "Number of times a LAST operation failed.",
		}),
		DiskGetBatchCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_getbatch_count",
			Help:      "Number of times a GET_BATCH operation is performed.",
		}),
		DiskWriteBatchCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_write_batch_count",
			Help:      "Number of times a WRITE_BATCH operation is performed.",
		}),
		DiskWriteBatchFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_write_batch_failure_count",
			Help:      "Number of times a WRITE_BATCH operation failed.",
		}),
		DiskCloseCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disk_close_count",
			Help:      "Number of times a CLOSE operation is performed.",
		}),
		DataValidationCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "data_validation_count",
			Help:      "Number of times a DATA_VALIDATION is performed.",
		}),
		DataValidationFailCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "data_validation_fail_count",
			Help:      "Number of times a DATA_VALIDATION operation failed.",
		}),
	}
}

func (d *DiskStore) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(d.metrics)
}
