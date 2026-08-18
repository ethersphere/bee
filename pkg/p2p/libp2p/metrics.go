// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"github.com/ethersphere/bee/v2/pkg/bzz"
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	connectionTransportLabelName = "transport"
	connectionTransportHelp      = "The 'transport' label is one of: tcp, ws, wss, quic-v1, quic, unknown."
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	CreatedConnectionCount     *prometheus.CounterVec
	HandledConnectionCount     *prometheus.CounterVec
	PublicAddressConnections   *prometheus.CounterVec
	PrivateAddressConnections  *prometheus.CounterVec
	CreatedStreamCount         prometheus.Counter
	ClosedStreamCount          prometheus.Counter
	StreamResetCount           prometheus.Counter
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
		CreatedConnectionCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "created_connection_count",
				Help:      "Number of initiated outgoing libp2p connections. " + connectionTransportHelp,
			},
			[]string{connectionTransportLabelName},
		),
		HandledConnectionCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "handled_connection_count",
				Help:      "Number of handled incoming libp2p connections. " + connectionTransportHelp,
			},
			[]string{connectionTransportLabelName},
		),
		PublicAddressConnections: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "public_address_connections_total",
				Help:      "Number of libp2p connections whose remote multiaddr is a public address. " + connectionTransportHelp,
			},
			[]string{connectionTransportLabelName},
		),
		PrivateAddressConnections: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "private_address_connections_total",
				Help:      "Number of libp2p connections whose remote multiaddr is a private address. " + connectionTransportHelp,
			},
			[]string{connectionTransportLabelName},
		),
		CreatedStreamCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "created_stream_count",
			Help:      "Number of initiated outgoing libp2p streams.",
		}),
		ClosedStreamCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "closed_stream_count",
			Help:      "Number of closed outgoing libp2p streams.",
		}),
		StreamResetCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "stream_reset_count",
			Help:      "Number of outgoing libp2p streams resets.",
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

func (m metrics) incCreatedConnection(addr ma.Multiaddr) {
	m.CreatedConnectionCount.WithLabelValues(connectionTransportLabel(addr)).Inc()
}

func (m metrics) observeHandledConnection(addr ma.Multiaddr) {
	transport := connectionTransportLabel(addr)
	m.HandledConnectionCount.WithLabelValues(transport).Inc()
	if manet.IsPublicAddr(addr) {
		m.PublicAddressConnections.WithLabelValues(transport).Inc()
		return
	}
	m.PrivateAddressConnections.WithLabelValues(transport).Inc()
}

// connectionTransportLabel returns the Prometheus transport label for a connection
// multiaddr. Live WSS connections are often encoded with the deprecated /wss
// component rather than the /tls/.../ws form used in advertised AutoTLS addresses.
func connectionTransportLabel(addr ma.Multiaddr) string {
	if addr == nil {
		return bzz.TransportUnknown.String()
	}
	if _, err := addr.ValueForProtocol(ma.P_WSS); err == nil {
		return bzz.TransportWSS.String()
	}
	if t := bzz.ClassifyTransport(addr); t != bzz.TransportUnknown {
		return t.String()
	}
	if _, err := addr.ValueForProtocol(ma.P_QUIC_V1); err == nil {
		return "quic-v1"
	}
	if _, err := addr.ValueForProtocol(ma.P_QUIC); err == nil {
		return "quic"
	}
	return bzz.TransportUnknown.String()
}

func (s *Service) Metrics() []prometheus.Collector {
	collectors := append(m.PrometheusCollectorsFromFields(s.metrics), s.handshakeService.Metrics()...)
	if mc, ok := s.reacher.(interface{ Metrics() []prometheus.Collector }); ok {
		collectors = append(collectors, mc.Metrics()...)
	}
	return collectors
}

// StatusMetrics exposes metrics that are exposed on the status protocol.
func (s *Service) StatusMetrics() []prometheus.Collector {
	return []prometheus.Collector{
		s.metrics.HeadersExchangeDuration,
	}
}
