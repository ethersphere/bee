// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

// metrics groups kademlia related m counters.
type metrics struct {
	PickCalls                             m.Counter
	PickCallsFalse                        m.Counter
	CurrentDepth                          m.Gauge
	CurrentStorageDepth                   m.Gauge
	CurrentlyKnownPeers                   m.Gauge
	CurrentlyConnectedPeers               m.Gauge
	InternalMetricsFlushTime              m.Histogram
	InternalMetricsFlushTotalErrors       m.Counter
	TotalBeforeExpireWaits                m.Counter
	TotalInboundConnections               m.Counter
	TotalInboundDisconnections            m.Counter
	TotalOutboundConnections              m.Counter
	TotalOutboundConnectionAttempts       m.Counter
	TotalOutboundConnectionFailedAttempts m.Counter
	TotalBootNodesConnectionAttempts      m.Counter
	StartAddAddressBookOverlaysTime       m.Histogram
	PeerLatencyEWMA                       m.Histogram
	Blocklist                             m.Counter
	ReachabilityStatus                    m.GaugeMetricVector
	PeersReachabilityStatus               m.GaugeMetricVector
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "kademlia"

	return metrics{
		PickCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "pick_calls",
			Help:      "The number of pick method call made.",
		}),
		PickCallsFalse: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "pick_calls_false",
			Help:      "The number of pick method call made which returned false.",
		}),
		CurrentDepth: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "current_depth",
			Help:      "The current value of depth.",
		}),
		CurrentStorageDepth: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "current_storage_depth",
			Help:      "The current value of storage depth.",
		}),
		CurrentlyKnownPeers: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "currently_known_peers",
			Help:      "Number of currently known peers.",
		}),
		CurrentlyConnectedPeers: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "currently_connected_peers",
			Help:      "Number of currently connected peers.",
		}),
		InternalMetricsFlushTime: m.NewHistogram(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "internal_metrics_flush_time",
			Help:      "The time spent flushing the internal metrics about peers to the state-store.",
		}),
		InternalMetricsFlushTotalErrors: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "internal_metrics_flush_total_errors",
			Help:      "Number of total errors occurred during flushing the internal metrics to the state-store.",
		}),
		TotalBeforeExpireWaits: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_before_expire_waits",
			Help:      "Total before expire waits made.",
		}),
		TotalInboundConnections: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_inbound_connections",
			Help:      "Total inbound connections made.",
		}),
		TotalInboundDisconnections: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_inbound_disconnections",
			Help:      "Total inbound disconnections made.",
		}),
		TotalOutboundConnections: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_outbound_connections",
			Help:      "Total outbound connections made.",
		}),
		TotalOutboundConnectionAttempts: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_outbound_connection_attempts",
			Help:      "Total outbound connection attempts made.",
		}),
		TotalOutboundConnectionFailedAttempts: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_outbound_connection_failed_attempts",
			Help:      "Total outbound connection failed attempts made.",
		}),
		TotalBootNodesConnectionAttempts: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_bootnodes_connection_attempts",
			Help:      "Total boot-nodes connection attempts made.",
		}),
		StartAddAddressBookOverlaysTime: m.NewHistogram(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "start_add_addressbook_overlays_time",
			Help:      "The time spent adding overlays peers from addressbook on kademlia start.",
		}),
		PeerLatencyEWMA: m.NewHistogram(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peer_latency_ewma",
			Help:      "Peer latency EWMA value distribution.",
		}),
		Blocklist: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "blocklist",
			Help:      "The number of times peers have been blocklisted.",
		}),
		ReachabilityStatus: m.NewGaugeVec(
			m.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "reachability_status",
				Help:      "The reachability status of the node.",
			},
			[]string{"reachability_status"},
		),
		PeersReachabilityStatus: m.NewGaugeVec(
			m.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "peers_reachability_status",
				Help:      "The reachability status of peers.",
			},
			[]string{"peers_reachability_status"},
		),
	}
}

// Metrics returns set of m collectors.
func (k *Kad) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(k.metrics)
}
