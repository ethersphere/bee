// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	TotalSent                       prometheus.Counter
	TotalReceived                   prometheus.Counter
	TotalErrors                     prometheus.Counter
	TotalHandlerErrors              prometheus.Counter
	TotalReplicated                 prometheus.Counter
	TotalReplicatedError            prometheus.Counter
	TotalSendAttempts               prometheus.Counter
	TotalFailedSendAttempts         prometheus.Counter
	TotalSkippedPeers               prometheus.Counter
	TotalOutgoing                   prometheus.Counter
	TotalOutgoingErrors             prometheus.Counter
	InvalidStampErrors              prometheus.Counter
	TotalHandlerReplicationErrors   prometheus.Counter
	TotalReplicationFromDistantPeer prometheus.Counter
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
		TotalHandlerErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_handler_errors",
			Help:      "Total no of error occurred while handling an incoming delivery (either while storing or forwarding).",
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
		TotalSkippedPeers: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_skipped_peers",
			Help:      "Total no of peers skipped",
		}),
		TotalOutgoing: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_outgoing",
			Help:      "Total no of chunks requested to be synced (calls on exported PushChunkToClosest)",
		}),
		TotalOutgoingErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_outgoing_errors",
			Help:      "Total no of errors of entire operation to sync a chunk (multiple attempts included)",
		}),
		InvalidStampErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "invalid_stamps",
			Help:      "No of invalid stamp errors.",
		}),
		TotalHandlerReplicationErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_replication_handlers_errors",
			Help:      "Total no of errors of pushsync handler neighborhood replication.",
		}),
		TotalReplicationFromDistantPeer: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_distant_replications",
			Help:      "Total no of replication requests received from non closest peers",
		}),
	}
}

func (s *PushSync) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
