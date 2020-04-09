// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection

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
	GCWriteBatchError        prometheus.Counter
	GCExcludeCounter         prometheus.Counter
	GCExcludeError           prometheus.Counter
	GCExcludedCounter        prometheus.Counter
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
		TotalTimeCollectGarbage: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "time_taken_gc",
			Help:      "Total time taken to collect garbage.",
		}),
		TotalTimeGCExclude: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "time_taken_gc_exclude_index",
			Help:      "Total time taken to exclude gc index.",
		}),
		TotalTimeGet: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "time_taken_get_chunk",
			Help:      "Total time taken to get chunk from DB.",
		}),
		TotalTimeUpdateGC: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "time_taken_in_gc",
			Help:      "Total time taken to in gc.",
		}),
		TotalTimeGetMulti: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "time_taken_get_multi",
			Help:      "Total time taken to get multiple chunks from DB.",
		}),
		TotalTimeHas: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "time_taken_has",
			Help:      "Total time taken to check if the key is present in DB.",
		}),
		TotalTimeHasMulti: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "time_taken_has_multi",
			Help:      "Total time taken to check if multiple keys are present in DB.",
		}),
		TotalTimePut: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "time_taken_put",
			Help:      "Total time taken to put a chunk in DB.",
		}),
		TotalTimeSet: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "time_taken_set",
			Help:      "Total time taken to set chunk in DB.",
		}),
		TotalTimeSubscribePullIteration: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "time_taken_subscribe_pull_iteration",
			Help:      "Total time taken to subsctibe for pull iteration.",
		}),
		TotalTimeSubscribePushIteration: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "time_taken_subscribe_push_iteration",
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
		GCWriteBatchError: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_write_batch_error",
			Help:      "Number of times the GC_WRITE_BATCH operation failed.",
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
		GCExcludedCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_excluded_count",
			Help:      "Number of times the GC_EXCLUDED operation is done.",
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
			Name:      "gc_update",
			Help:      "Number of times the gc is updated.",
		}),
		GCUpdateError: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "fc_update_error",
			Help:      "Number of times the gc update had error.",
		}),

		ModeGet: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_get",
			Help:      "Number of times MODE_GET is invoked.",
		}),
		ModeGetFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_get_failure",
			Help:      "Number of times MODE_GET invocation failed.",
		}),
		ModeGetMulti: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_get_multi",
			Help:      "Number of times MODE_MULTI_GET is invoked.",
		}),
		ModeGetMultiFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_get_failure",
			Help:      "Number of times MODE_GET invocation failed.",
		}),
		ModePut: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_put",
			Help:      "Number of times MODE_PUT is invoked.",
		}),
		ModePutFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_put_failure",
			Help:      "Number of times MODE_PUT invocation failed.",
		}),
		ModeSet: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_set",
			Help:      "Number of times MODE_SET is invoked.",
		}),
		ModeSetFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_set_failure",
			Help:      "Number of times MODE_SET invocation failed.",
		}),
		ModeHas: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_has",
			Help:      "Number of times MODE_HAS is invoked.",
		}),
		ModeHasFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_has",
			Help:      "Number of times MODE_HAS invocation failed.",
		}),
		ModeHasMulti: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_has_multi",
			Help:      "Number of times MODE_HAS_MULTI is invoked.",
		}),
		ModeHasMultiFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mode_has_multi_fail",
			Help:      "Number of times MODE_HAS_MULTI invocation failed.",
		}),
		SubscribePull: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pull",
			Help:      "Number of times Subscribe_pULL is invoked.",
		}),
		SubscribePullStop: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pull_stop",
			Help:      "Number of times Subscribe_pull_stop is invoked.",
		}),
		SubscribePullIteration: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pull_iteration",
			Help:      "Number of times Subscribe_pull_iteration is invoked.",
		}),
		SubscribePullIterationFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pull_iteration_fail",
			Help:      "Number of times Subscribe_pull_iteration_fail is invoked.",
		}),
		LastPullSubscriptionBinID: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "last_pull_subscription_bin_id",
			Help:      "Number of times LastPullSubscriptionBinID is invoked.",
		}),
		SubscribePush: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_push",
			Help:      "Number of times SUBSCRIBE_PUSH is invoked.",
		}),
		SubscribePushIteration: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_push_iteration",
			Help:      "Number of times SUBSCRIBE_PUSH_ITERATION is invoked.",
		}),
		SubscribePushIterationDone: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_push_iteration_done",
			Help:      "Number of times SUBSCRIBE_PUSH_ITERATION_DONE is invoked.",
		}),
		SubscribePushIterationFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_push_iteration_failure",
			Help:      "Number of times SUBSCRIBE_PUSH_ITERATION_FAILURE is invoked.",
		}),

		GCSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_size_gauge",
			Help:      "gc size gauge.",
		}),
		GCStoreTimeStamps: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_time_stamps_gauge",
			Help:      "gc time stamps gauge.",
		}),
		GCStoreAccessTimeStamps: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gc_access_time_stamps_gauge",
			Help:      "gc access time stamps gauge.",
		}),
	}
}

func (s *DB) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
