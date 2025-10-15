// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	CreatedConnectionCount     m.Counter
	HandledConnectionCount     m.Counter
	CreatedStreamCount         m.Counter
	ClosedStreamCount          m.Counter
	StreamResetCount           m.Counter
	HandledStreamCount         m.Counter
	BlocklistedPeerCount       m.Counter
	BlocklistedPeerErrCount    m.Counter
	DisconnectCount            m.Counter
	ConnectBreakerCount        m.Counter
	UnexpectedProtocolReqCount m.Counter
	KickedOutPeersCount        m.Counter
	StreamHandlerErrResetCount m.Counter
	HeadersExchangeDuration    m.Histogram
}

func newMetrics() metrics {
	subsystem := "libp2p"

	return metrics{
		CreatedConnectionCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "created_connection_count",
			Help:      "Number of initiated outgoing libp2p connections.",
		}),
		HandledConnectionCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "handled_connection_count",
			Help:      "Number of handled incoming libp2p connections.",
		}),
		CreatedStreamCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "created_stream_count",
			Help:      "Number of initiated outgoing libp2p streams.",
		}),
		ClosedStreamCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "closed_stream_count",
			Help:      "Number of closed outgoing libp2p streams.",
		}),
		StreamResetCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "stream_reset_count",
			Help:      "Number of outgoing libp2p streams resets.",
		}),
		HandledStreamCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "handled_stream_count",
			Help:      "Number of handled incoming libp2p streams.",
		}),
		BlocklistedPeerCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "blocklisted_peer_count",
			Help:      "Number of peers we've blocklisted.",
		}),
		BlocklistedPeerErrCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "blocklisted_peer_err_count",
			Help:      "Number of peers we've been unable to blocklist.",
		}),
		DisconnectCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disconnect_count",
			Help:      "Number of peers we've disconnected from (initiated locally).",
		}),
		ConnectBreakerCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "connect_breaker_count",
			Help:      "Number of times we got a closed breaker while connecting to another peer.",
		}),
		UnexpectedProtocolReqCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "unexpected_protocol_request_count",
			Help:      "Number of requests the peer is not expecting.",
		}),
		KickedOutPeersCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "kickedout_peers_count",
			Help:      "Number of total kicked-out peers.",
		}),
		StreamHandlerErrResetCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "stream_handler_error_reset_count",
			Help:      "Number of total stream handler error resets.",
		}),
		HeadersExchangeDuration: m.NewHistogram(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "headers_exchange_duration",
			Help:      "The duration spent exchanging the headers.",
		}),
	}
}

func (s *Service) Metrics() []m.Collector {
	return append(m.PrometheusCollectorsFromFields(s.metrics), s.handshakeService.Metrics()...)
}

// StatusMetrics exposes metrics that are exposed on the status protocol.
func (s *Service) StatusMetrics() []m.Collector {
	return []m.Collector{
		s.metrics.HeadersExchangeDuration,
	}
}
