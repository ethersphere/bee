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
	PickCalls                             prometheus.Counter
	PickCallsFalse                        prometheus.Counter
	CurrentDepth                          prometheus.Gauge
	CurrentRadius                         prometheus.Gauge
	CurrentlyKnownPeers                   prometheus.Gauge
	CurrentlyConnectedPeers               prometheus.Gauge
	InternalMetricsFlushTime              prometheus.Histogram
	TotalBeforeExpireWaits                prometheus.Counter
	TotalInboundConnections               prometheus.Counter
	TotalInboundDisconnections            prometheus.Counter
	TotalOutboundConnections              prometheus.Counter
	TotalOutboundConnectionAttempts       prometheus.Counter
	TotalOutboundConnectionFailedAttempts prometheus.Counter
	TotalBootNodesConnectionAttempts      prometheus.Counter
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "kademlia"

	return metrics{
		PickCalls: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "pick_calls",
			Help:      "The number of pick method call made.",
		}),
		PickCallsFalse: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "pick_calls_false",
			Help:      "The number of pick method call made which returned false.",
		}),
		CurrentDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "current_depth",
			Help:      "The current value of depth.",
		}),
		CurrentRadius: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "current_radius",
			Help:      "The current value of radius.",
		}),
		CurrentlyKnownPeers: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "currently_known_peers",
			Help:      "Number of currently known peers",
		}),
		CurrentlyConnectedPeers: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "currently_connected_peers",
			Help:      "Number of currently connected peers",
		}),
		InternalMetricsFlushTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "internal_metrics_flush_time",
			Help:      "Time spent flushing the internal metrics about peers to the state-store",
		}),
		TotalBeforeExpireWaits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_before_expire_waits",
			Help:      "Total before expire waits made.",
		}),
		TotalInboundConnections: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_inbound_connections",
			Help:      "Total inbound connections made.",
		}),
		TotalInboundDisconnections: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_inbound_disconnections",
			Help:      "Total inbound disconnections made.",
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
		}),
		TotalBootNodesConnectionAttempts: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_bootnodes_connection_attempts",
			Help:      "Total boot-nodes connection attempts made.",
		})}
}

// Metrics returns set of prometheus collectors.
func (k *Kad) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(k.metrics)
}
