// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	BroadcastPeers      m.Counter
	BroadcastPeersPeers m.Counter
	BroadcastPeersSends m.Counter

	PeersHandler      m.Counter
	PeersHandlerPeers m.Counter
	UnreachablePeers  m.Counter

	PingTime        m.Histogram
	PingFailureTime m.Histogram

	PeerConnectAttempts m.Counter
	PeerUnderlayErr     m.Counter
	StorePeerErr        m.Counter
	ReachablePeers      m.Counter
}

func newMetrics() metrics {
	subsystem := "hive"

	return metrics{
		BroadcastPeers: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "broadcast_peers_count",
			Help:      "Number of calls to broadcast peers.",
		}),
		BroadcastPeersPeers: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "broadcast_peers_peer_count",
			Help:      "Number of peers to be sent.",
		}),
		BroadcastPeersSends: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "broadcast_peers_message_count",
			Help:      "Number of individual peer gossip messages sent.",
		}),
		PeersHandler: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peers_handler_count",
			Help:      "Number of peer messages received.",
		}),
		PeersHandlerPeers: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peers_handler_peers_count",
			Help:      "Number of peers received in peer messages.",
		}),
		UnreachablePeers: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "unreachable_peers_count",
			Help:      "Number of peers that are unreachable.",
		}),
		PingTime: m.NewHistogram(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ping_time",
			Help:      "The time spent for pings.",
		}),
		PingFailureTime: m.NewHistogram(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "fail_ping_time",
			Help:      "The time spent for unsuccessful pings.",
		}),
		PeerConnectAttempts: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peer_attempt_count",
			Help:      "Number of attempts made to check peer reachability.",
		}),
		PeerUnderlayErr: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peer_underlay_err_count",
			Help:      "Number of errors extracting peer underlay.",
		}),
		StorePeerErr: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "store_peer_err_count",
			Help:      "Number of peers that could not be stored.",
		}),
		ReachablePeers: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "reachable_peers_count",
			Help:      "Number of peers that are reachable.",
		}),
	}
}

func (s *Service) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
