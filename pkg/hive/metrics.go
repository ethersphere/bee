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

	PingTime        prometheus.Histogram
	PingFailureTime prometheus.Histogram

  PeerConnectAttempts prometheus.Counter
	PeerUnderlayErr     prometheus.Counter
	StorePeerErr        prometheus.Counter
	ReachablePeers      prometheus.Counter
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
		PingTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ping_time",
			Help:      "The time spent for pings.",
		}),
		PingFailureTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "fail_ping_time",
			Help:      "The time spent for unsuccessful pings.",
    }),
		PeerConnectAttempts: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peer_attempt_count",
			Help:      "Number of attempts made to check peer reachability.",
		}),
		PeerUnderlayErr: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peer_underlay_err_count",
			Help:      "Number of errors extacting peer underlay.",
		}),
		StorePeerErr: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "store_peer_err_count",
			Help:      "Number of peers that could not be stored.",
		}),
		ReachablePeers: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "reachable_peers_count",
			Help:      "Number of peers that are reachable.",
		}),
	}
}

func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
