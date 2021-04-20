// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	TotalTimeGCLock                 prometheus.Counter
	TotalTimeGCFirstItem            prometheus.Counter
	TotalTimeCollectGarbage         prometheus.Counter
	TotalTimeGCExclude              prometheus.Counter
	TotalTimeGet                    prometheus.Counter
	TotalTimeUpdateGC               prometheus.Counter
	TotalTimeGetMulti               prometheus.Counter
	TotalTimeHas                    prometheus.Counter
	TotalTimeHasMulti               prometheus.Counter
	TotalTimePut                    prometheus.Counter
	TotalTimeSet                    prometheus.Counter
	TotalTimeSubscribePullIteration prometheus.Counter
	TotalTimeSubscribePushIteration prometheus.Counter

	GCCounter                prometheus.Counter
	GCErrorCounter           prometheus.Counter
	GCCollectedCounter       prometheus.Counter
	GCCommittedCounter       prometheus.Counter
	GCExcludeCounter         prometheus.Counter
	GCExcludeError           prometheus.Counter
	GCExcludeWriteBatchError prometheus.Counter
	GCUpdate                 prometheus.Counter
	GCUpdateError            prometheus.Counter

	ModeGet                       prometheus.Counter
	ModeGetFailure                prometheus.Counter
	ModeGetMulti                  prometheus.Counter
	ModeGetMultiFailure           prometheus.Counter
	ModePut                       prometheus.Counter
	ModePutFailure                prometheus.Counter
	ModeSet                       prometheus.Counter
	ModeSetFailure                prometheus.Counter
	ModeHas                       prometheus.Counter
	ModeHasFailure                prometheus.Counter
	ModeHasMulti                  prometheus.Counter
	ModeHasMultiFailure           prometheus.Counter
	SubscribePull                 prometheus.Counter
	SubscribePullStop             prometheus.Counter
	SubscribePullIteration        prometheus.Counter
	SubscribePullIterationFailure prometheus.Counter
	LastPullSubscriptionBinID     prometheus.Counter
	SubscribePush                 prometheus.Counter
	SubscribePushIteration        prometheus.Counter
	SubscribePushIterationDone    prometheus.Counter
	SubscribePushIterationFailure prometheus.Counter

	GCSize                  prometheus.Gauge
	GCStoreTimeStamps       prometheus.Gauge
	GCStoreAccessTimeStamps prometheus.Gauge
}

func newMetrics() metrics {
	subsystem := "localstore"

	return metrics{
		TotalTimeGCLock: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_lock_time",
			Help:      "Total time under lock in gc.",
		}),
		TotalTimeGCFirstItem: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_first_item_time",
			Help:      "Total time taken till first item in gc comes out of gcIndex iterator.",
		}),
		TotalTimeCollectGarbage: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_time",
			Help:      "Total time taken to collect garbage.",
		}),
		TotalTimeGCExclude: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_exclude_index_time",
			Help:      "Total time taken to exclude gc index.",
		}),
		TotalTimeGet: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "get_chunk_time",
			Help:      "Total time taken to get chunk from DB.",
		}),
		TotalTimeUpdateGC: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "in_gc_time",
			Help:      "Total time taken to in gc.",
		}),
		TotalTimeGetMulti: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "get_multi_time",
			Help:      "Total time taken to get multiple chunks from DB.",
		}),
		TotalTimeHas: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "has_time",
			Help:      "Total time taken to check if the key is present in DB.",
		}),
		TotalTimeHasMulti: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "has_multi_time",
			Help:      "Total time taken to check if multiple keys are present in DB.",
		}),
		TotalTimePut: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "put_time",
			Help:      "Total time taken to put a chunk in DB.",
		}),
		TotalTimeSet: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "set_time",
			Help:      "Total time taken to set chunk in DB.",
		}),
		TotalTimeSubscribePullIteration: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pull_iteration_time",
			Help:      "Total time taken to subsctibe for pull iteration.",
		}),
		TotalTimeSubscribePushIteration: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_push_iteration_time",
			Help:      "Total time taken to subscribe for push iteration.",
		}),
		GCCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_count",
			Help:      "Number of times the GC operation is done.",
		}),
		GCErrorCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_fail_count",
			Help:      "Number of times the GC operation failed.",
		}),
		GCCollectedCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_collected_count",
			Help:      "Number of times the GC_COLLECTED operation is done.",
		}),
		GCCommittedCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_committed_count",
			Help:      "Number of gc items to commit.",
		}),
		GCExcludeCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_exclude_count",
			Help:      "Number of times the GC_EXCLUDE operation is done.",
		}),
		GCExcludeError: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_exclude_fail_count",
			Help:      "Number of times the GC_EXCLUDE operation failed.",
		}),
		GCExcludeWriteBatchError: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ex_exclude_write_batch_fail_count",
			Help:      "Number of times the GC_EXCLUDE_WRITE_BATCH operation is failed.",
		}),
		GCUpdate: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_update_count",
			Help:      "Number of times the gc is updated.",
		}),
		GCUpdateError: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_update_error_count",
			Help:      "Number of times the gc update had error.",
		}),

		ModeGet: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_get_count",
			Help:      "Number of times MODE_GET is invoked.",
		}),
		ModeGetFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_get_failure_count",
			Help:      "Number of times MODE_GET invocation failed.",
		}),
		ModeGetMulti: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_get_multi_count",
			Help:      "Number of times MODE_MULTI_GET is invoked.",
		}),
		ModeGetMultiFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_get_multi_failure_count",
			Help:      "Number of times MODE_GET invocation failed.",
		}),
		ModePut: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_put_count",
			Help:      "Number of times MODE_PUT is invoked.",
		}),
		ModePutFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_put_failure_count",
			Help:      "Number of times MODE_PUT invocation failed.",
		}),
		ModeSet: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_set_count",
			Help:      "Number of times MODE_SET is invoked.",
		}),
		ModeSetFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_set_failure_count",
			Help:      "Number of times MODE_SET invocation failed.",
		}),
		ModeHas: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_has_count",
			Help:      "Number of times MODE_HAS is invoked.",
		}),
		ModeHasFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_has_failure_count",
			Help:      "Number of times MODE_HAS invocation failed.",
		}),
		ModeHasMulti: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_has_multi_count",
			Help:      "Number of times MODE_HAS_MULTI is invoked.",
		}),
		ModeHasMultiFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_has_multi_failure_count",
			Help:      "Number of times MODE_HAS_MULTI invocation failed.",
		}),
		SubscribePull: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pull_count",
			Help:      "Number of times Subscribe_pULL is invoked.",
		}),
		SubscribePullStop: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pull_stop_count",
			Help:      "Number of times Subscribe_pull_stop is invoked.",
		}),
		SubscribePullIteration: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pull_iteration_count",
			Help:      "Number of times Subscribe_pull_iteration is invoked.",
		}),
		SubscribePullIterationFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pull_iteration_fail_count",
			Help:      "Number of times Subscribe_pull_iteration_fail is invoked.",
		}),
		LastPullSubscriptionBinID: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "last_pull_subscription_bin_id_count",
			Help:      "Number of times LastPullSubscriptionBinID is invoked.",
		}),
		SubscribePush: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_push_count",
			Help:      "Number of times SUBSCRIBE_PUSH is invoked.",
		}),
		SubscribePushIteration: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_push_iteration_count",
			Help:      "Number of times SUBSCRIBE_PUSH_ITERATION is invoked.",
		}),
		SubscribePushIterationDone: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_push_iteration_done_count",
			Help:      "Number of times SUBSCRIBE_PUSH_ITERATION_DONE is invoked.",
		}),
		SubscribePushIterationFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_push_iteration_failure_count",
			Help:      "Number of times SUBSCRIBE_PUSH_ITERATION_FAILURE is invoked.",
		}),

		GCSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_size",
			Help:      "Number of elements in Garbage collection index.",
		}),
		GCStoreTimeStamps: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_time_stamp",
			Help:      "Storage timestamp in Garbage collection iteration.",
		}),
		GCStoreAccessTimeStamps: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_access_time_stamp",
			Help:      "Access timestamp in Garbage collection iteration.",
		}),
	}
}

func (db *DB) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(db.metrics)
}
