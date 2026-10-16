// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics are the protocol metrics of the rangesync service.
type metrics struct {
	// MappingMismatch counts received statuses whose Mapping differs from
	// this node's.
	MappingMismatch prometheus.Counter
	// InvalidMessage counts peers disconnected for breaking the protocol.
	InvalidMessage prometheus.Counter
	// Busy counts sessions answered with Busy.
	Busy prometheus.Counter
	// StreamLimitExceeded counts peers disconnected for opening too many
	// concurrent streams.
	StreamLimitExceeded prometheus.Counter
}

func newMetrics() metrics {
	const subsystem = "rangesync"

	return metrics{
		MappingMismatch: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mapping_mismatch_total",
			Help:      "Received statuses whose symbol mapping differs from this node's.",
		}),
		InvalidMessage: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "invalid_message_total",
			Help:      "Peers disconnected for sending an invalid message.",
		}),
		Busy: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "session_busy_total",
			Help:      "Reconcile sessions answered with Busy.",
		}),
		StreamLimitExceeded: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "stream_limit_exceeded_total",
			Help:      "Peers disconnected for exceeding the concurrent stream limit.",
		}),
	}
}

func (mt metrics) collectors() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(mt)
}
