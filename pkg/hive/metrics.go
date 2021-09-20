// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	BroadcastPeers      prometheus.Counter
	BroadcastPeersPeers prometheus.Counter
	BroadcastPeersSends prometheus.Counter

	PeersHandler      prometheus.Counter
	PeersHandlerPeers prometheus.Counter
	UnreachablePeers  prometheus.Counter

	SeenNetworkAddresses        prometheus.Counter
	SeenPrivateNetworkAddresses prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "hive"

	return metrics{
		BroadcastPeers: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "broadcast_peers_count",
			Help:      "Number of calls to broadcast peers.",
		}),
		BroadcastPeersPeers: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "broadcast_peers_peer_count",
			Help:      "Number of peers to be sent.",
		}),
		BroadcastPeersSends: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "broadcast_peers_message_count",
			Help:      "Number of individual peer gossip messages sent.",
		}),
		PeersHandler: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peers_handler_count",
			Help:      "Number of peer messages received.",
		}),
		PeersHandlerPeers: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peers_handler_peers_count",
			Help:      "Number of peers received in peer messages.",
		}),
		UnreachablePeers: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "unreachable_peers_count",
			Help:      "Number of peers that are unreachable.",
		}),
		SeenNetworkAddresses: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "seen_network_addresses_count",
			Help:      "Number of seen network addresses.",
		}),
		SeenPrivateNetworkAddresses: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "seen_private_network_addresses_count",
			Help:      "Number of seen private network addresses.",
		}),
	}
}

func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
