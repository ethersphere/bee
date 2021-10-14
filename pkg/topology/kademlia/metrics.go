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
	InternalMetricsFlushTotalErrors       prometheus.Counter
	TotalBeforeExpireWaits                prometheus.Counter
	TotalInboundConnections               prometheus.Counter
	TotalInboundDisconnections            prometheus.Counter
	TotalOutboundConnections              prometheus.Counter
	TotalOutboundConnectionAttempts       prometheus.Counter
	TotalOutboundConnectionFailedAttempts prometheus.Counter
	TotalBootNodesConnectionAttempts      prometheus.Counter
	StartAddAddressBookOverlaysTime       prometheus.Histogram
	PeerLatencyEWMA                       prometheus.Histogram
	Flag                                  prometheus.Counter
	Unflag                                prometheus.Counter
	Blocklist                             prometheus.Counter
	ReachabilityStatus                    *prometheus.GaugeVec
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "kademlia"

	return metrics{
		PickCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "pick_calls",
			Help:      "The number of pick method call made.",
		}),
		PickCallsFalse: prometheus.NewCounter(prometheus.CounterOpts{
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
			Help:      "Number of currently known peers.",
		}),
		CurrentlyConnectedPeers: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "currently_connected_peers",
			Help:      "Number of currently connected peers.",
		}),
		InternalMetricsFlushTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "internal_metrics_flush_time",
			Help:      "The time spent flushing the internal metrics about peers to the state-store.",
		}),
		InternalMetricsFlushTotalErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "internal_metrics_flush_total_errors",
			Help:      "Number of total errors occurred during flushing the internal metrics to the state-store.",
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
		}),
		StartAddAddressBookOverlaysTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "start_add_addressbook_overlays_time",
			Help:      "The time spent adding overlays peers from addressbook on kademlia start.",
		}),
		PeerLatencyEWMA: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peer_latency_ewma",
			Help:      "Peer latency EWMA value distribution.",
		}),
		Flag: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "flag",
			Help:      "The nubmer of times peers have been flagged.",
		}),
		Unflag: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "unflag",
			Help:      "The nubmer of times peers have been unflagged.",
		}),
		Blocklist: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "blocklist",
			Help:      "The nubmer of times peers have been blocklisted.",
		}),
		ReachabilityStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "reachability_status",
				Help:      "The reachability status of the node.",
			},
			[]string{"reachability_status"},
		),
	}
}

// Metrics returns set of prometheus collectors.
func (k *Kad) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(k.metrics)
}
