// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics groups kademlia related prometheus counters.
type metrics struct {
	TotalBeforeExpireWaits                prometheus.Counter
	TotalInboundConnections               prometheus.Counter
	TotalInboundDisconnections            prometheus.Counter
	TotalOutboundConnections              prometheus.Counter
	TotalOutboundConnectionAttempts       prometheus.Counter
	TotalOutboundConnectionFailedAttempts prometheus.Counter
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "kademlia"

	return metrics{
		TotalBeforeExpireWaits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_before_expire_waits",
			Help:      "Total before expire waits made.",
		}),
		TotalInboundDisconnections: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_inbound_disconnections",
			Help:      "Total inbound disconnections made.",
		}),
		TotalInboundConnections: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_inbound_connections",
			Help:      "Total inbound connections made.",
		}),
		TotalOutboundConnections: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_outbound_connections",
			Help:      "Total outbound connections made.",
		}),
		TotalOutboundConnectionAttempts: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_outbound_connection_attempts",
			Help:      "Total outbound connection attempts made.",
		}),
		TotalOutboundConnectionFailedAttempts: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_outbound_connection_failed_attempts",
			Help:      "Total outbound connection failed attempts made.",
		})}
}

// Metrics returns set of prometheus collectors.
func (k *Kad) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(k.metrics)
}
