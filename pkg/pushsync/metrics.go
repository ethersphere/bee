// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	TotalSent               prometheus.Counter
	TotalReceived           prometheus.Counter
	TotalErrors             prometheus.Counter
	TotalReplicated         prometheus.Counter
	TotalReplicatedError    prometheus.Counter
	TotalSendAttempts       prometheus.Counter
	TotalFailedSendAttempts prometheus.Counter
	TotalFailedCacheHits    prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "pushsync"

	return metrics{
		TotalSent: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_sent",
			Help:      "Total chunks sent.",
		}),
		TotalReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_received",
			Help:      "Total chunks received.",
		}),
		TotalErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_errors",
			Help:      "Total no of time error received while sending chunk.",
		}),
		TotalReplicated: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_replication",
			Help:      "Total no of successfully sent replication chunks.",
		}),
		TotalReplicatedError: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_replication_error",
			Help:      "Total no of failed replication chunks.",
		}),
		TotalSendAttempts: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_send_attempts",
			Help:      "Total no of attempts to push chunk.",
		}),
		TotalFailedSendAttempts: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_failed_send_attempts",
			Help:      "Total no of failed attempts to push chunk.",
		}),
		TotalFailedCacheHits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_failed_cache_hits",
			Help:      "Total FailedRequestCache hits",
		}),
	}
}

func (s *PushSync) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
