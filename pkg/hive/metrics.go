// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
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

	UnderlayByteSizeExceeded prometheus.Counter
	UnderlayCountExceeded    prometheus.Counter
	ChequebookVerification   *prometheus.CounterVec
	TimestampRejected        *prometheus.CounterVec

	LegacyRecordSkipped prometheus.Counter

	GossipCoalescedFlushes   prometheus.Counter
	GossipCoalesceDropped    prometheus.Counter
	GossipCoalesceBufferSize prometheus.Gauge
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
			Help:      "Number of errors extracting peer underlay.",
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
		UnderlayByteSizeExceeded: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "underlay_byte_size_exceeded_count",
			Help:      "Number of peers dropped due to oversized underlay field.",
		}),
		UnderlayCountExceeded: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "underlay_count_exceeded_count",
			Help:      "Number of peers dropped due to exceeding underlay count limit.",
		}),
		LegacyRecordSkipped: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "legacy_record_skipped_count",
			Help:      "Number of addressbook records skipped during gossip broadcast due to missing timestamp (pre-timestamp legacy entries).",
		}),
		TimestampRejected: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "timestamp_rejected_total",
				Help:      "Number of gossip peer records rejected by timestamp validation. The 'reason' label is one of: invalid, in_future, stale, too_soon.",
			},
			[]string{"reason"},
		),
		GossipCoalescedFlushes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gossip_coalesced_flushes_total",
			Help:      "Number of coalesced outbound gossip flushes dispatched.",
		}),
		GossipCoalesceDropped: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gossip_coalesce_dropped_total",
			Help:      "Number of peer gossip entries dropped during coalesced flush due to outbound rate limiting.",
		}),
		GossipCoalesceBufferSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "gossip_coalesce_buffer_size",
			Help:      "Number of addressees with outbound gossip buffered awaiting coalesced flush.",
		}),
		ChequebookVerification: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "chequebook_verification_total",
				Help:      "Outcomes of chequebook verification during hive gossip ingestion. The 'result' label is one of: success, missing, issuer_mismatch, bytecode_mismatch, insufficient_balance, already_associated, verify_error.",
			},
			[]string{"result"},
		),
	}
}

func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
