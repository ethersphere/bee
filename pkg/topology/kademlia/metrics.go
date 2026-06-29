// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics groups kademlia related prometheus counters.
type metrics struct {
	PickCalls                             prometheus.Counter
	PickCallsFalse                        prometheus.Counter
	CurrentDepth                          prometheus.Gauge
	CurrentStorageDepth                   prometheus.Gauge
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
	Blocklist                             prometheus.Counter
	ReachabilityStatus                    *prometheus.GaugeVec
	PeersReachabilityStatus               *prometheus.GaugeVec

	AnnounceTotal                   prometheus.Counter
	AnnounceIsNeighborTotal         *prometheus.CounterVec
	AnnounceBinSelectionTotal       *prometheus.CounterVec
	AnnounceBinPeersAvailable       prometheus.Histogram
	AnnounceBinPeersSelected        prometheus.Histogram
	AnnounceBinCoverageRatio        prometheus.Histogram
	AnnouncePeersSentToNewPeer      prometheus.Histogram
	AnnounceOutgoingPeerGossipTotal prometheus.Counter
	AnnounceErrorsTotal             *prometheus.CounterVec
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
		CurrentStorageDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "current_storage_depth",
			Help:      "The current value of storage depth.",
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
		Blocklist: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "blocklist",
			Help:      "The number of times peers have been blocklisted.",
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
		PeersReachabilityStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "peers_reachability_status",
				Help:      "The reachability status of peers.",
			},
			[]string{"peers_reachability_status"},
		),
		AnnounceTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "announce_total",
			Help:      "Number of peer announce operations.",
		}),
		AnnounceIsNeighborTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "announce_is_neighbor_total",
				Help:      "Number of announce operations for neighbor peers.",
			},
			[]string{"is_neighbor"},
		),
		AnnounceBinSelectionTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "announce_bin_selection_total",
				Help:      "Number of bin peer selections during announce. The mode label is one of: full, subset.",
			},
			[]string{"mode"},
		),
		AnnounceBinPeersAvailable: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "announce_bin_peers_available",
			Help:      "Number of connected peers available in a bin before announce selection.",
			Buckets:   []float64{1, 2, 3, 4, 5, 6, 8, 10, 12, 15, 18, 25, 32},
		}),
		AnnounceBinPeersSelected: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "announce_bin_peers_selected",
			Help:      "Number of peers selected from a bin during announce.",
			Buckets:   []float64{1, 2, 3, 4, 5, 6, 8, 10, 12, 15, 18, 25, 32},
		}),
		AnnounceBinCoverageRatio: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "announce_bin_coverage_ratio",
			Help:      "Ratio of selected peers to available peers in a bin during announce.",
			Buckets:   []float64{0.1, 0.2, 0.33, 0.5, 0.66, 0.75, 0.8, 0.9, 1.0},
		}),
		AnnouncePeersSentToNewPeer: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "announce_peers_sent_to_new_peer",
			Help:      "Number of existing peers sent to a newly connected peer in a single announce.",
			Buckets:   []float64{1, 2, 5, 10, 15, 20, 30, 40, 50, 75, 100, 150, 200},
		}),
		AnnounceOutgoingPeerGossipTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "announce_outgoing_peer_gossip_total",
			Help:      "Number of outgoing peer gossip messages enqueued to existing peers during announce.",
		}),
		AnnounceErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "announce_errors_total",
				Help:      "Number of announce errors. The reason label is one of: random_subset, broadcast_to_new.",
			},
			[]string{"reason"},
		),
	}
}

// Metrics returns set of prometheus collectors.
func (k *Kad) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(k.metrics)
}
