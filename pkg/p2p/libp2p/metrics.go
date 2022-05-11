// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	CreatedConnectionCount     prometheus.Counter
	HandledConnectionCount     prometheus.Counter
	CreatedStreamCount         prometheus.Counter
	HandledStreamCount         prometheus.Counter
	BlocklistedPeerCount       prometheus.Counter
	BlocklistedPeerErrCount    prometheus.Counter
	DisconnectCount            prometheus.Counter
	ConnectBreakerCount        prometheus.Counter
	UnexpectedProtocolReqCount prometheus.Counter
	KickedOutPeersCount        prometheus.Counter
	StreamHandlerErrResetCount prometheus.Counter
	HeadersExchangeDuration    prometheus.Histogram
}

func newMetrics() metrics {
	subsystem := "libp2p"

	return metrics{
		CreatedConnectionCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "created_connection_count",
			Help:      "Number of initiated outgoing libp2p connections.",
		}),
		HandledConnectionCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "handled_connection_count",
			Help:      "Number of handled incoming libp2p connections.",
		}),
		CreatedStreamCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "created_stream_count",
			Help:      "Number of initiated outgoing libp2p streams.",
		}),
		HandledStreamCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "handled_stream_count",
			Help:      "Number of handled incoming libp2p streams.",
		}),
		BlocklistedPeerCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "blocklisted_peer_count",
			Help:      "Number of peers we've blocklisted.",
		}),
		BlocklistedPeerErrCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "blocklisted_peer_err_count",
			Help:      "Number of peers we've been unable to blocklist.",
		}),
		DisconnectCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disconnect_count",
			Help:      "Number of peers we've disconnected from (initiated locally).",
		}),
		ConnectBreakerCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "connect_breaker_count",
			Help:      "Number of times we got a closed breaker while connecting to another peer.",
		}),
		UnexpectedProtocolReqCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "unexpected_protocol_request_count",
			Help:      "Number of requests the peer is not expecting.",
		}),
		KickedOutPeersCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "kickedout_peers_count",
			Help:      "Number of total kicked-out peers.",
		}),
		StreamHandlerErrResetCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "stream_handler_error_reset_count",
			Help:      "Number of total stream handler error resets.",
		}),
		HeadersExchangeDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "headers_exchange_duration",
			Help:      "The duration spent exchanging the headers.",
		}),
	}
}

func (s *Service) Metrics() []prometheus.Collector {
	return append(m.PrometheusCollectorsFromFields(s.metrics), s.handshakeService.Metrics()...)
}
