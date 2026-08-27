// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"strings"

	"github.com/ethersphere/bee/v2/pkg/bzz"
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	connectionTransportLabelName = "transport"
	connectionPublicLabelName    = "public"
)

var (
	// TCP/WS/WSS only: bee does not register QUIC today.
	transportLabelValues = []string{
		bzz.TransportTCP.String(),
		bzz.TransportWS.String(),
		bzz.TransportWSS.String(),
		bzz.TransportUnknown.String(),
	}
	publicLabelValues = []string{"true", "false"}
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	CreatedConnectionCount     *prometheus.CounterVec
	HandledConnectionCount     *prometheus.CounterVec
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
	transportHelp := "The 'transport' label is one of: " + strings.Join(transportLabelValues, ", ")
	publicHelp := "The 'public' label is one of: " + strings.Join(publicLabelValues, ", ") + " (true = public remote multiaddr)."

	connectionLabels := []string{connectionTransportLabelName, connectionPublicLabelName}

	createdConnectionCount := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "created_connection_count",
			Help:      "Number of initiated outgoing libp2p connections. " + transportHelp + " " + publicHelp,
		},
		connectionLabels,
	)

	handledConnectionCount := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "handled_connection_count",
			Help:      "Number of handled incoming libp2p connections. " + transportHelp + " " + publicHelp,
		},
		connectionLabels,
	)

	// Ensure all expected label value combinations exist as 0-valued series,
	// so Grafana shows a flat line instead of "No Data".
	for _, transport := range transportLabelValues {
		for _, public := range publicLabelValues {
			createdConnectionCount.WithLabelValues(transport, public).Add(0)
			handledConnectionCount.WithLabelValues(transport, public).Add(0)
		}
	}

	return metrics{
		CreatedConnectionCount: createdConnectionCount,
		HandledConnectionCount: handledConnectionCount,
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
	m.CreatedConnectionCount.WithLabelValues(connectionTransportLabel(addr), connectionPublicLabel(addr)).Inc()
}

func (m metrics) observeHandledConnection(addr ma.Multiaddr) {
	m.HandledConnectionCount.WithLabelValues(connectionTransportLabel(addr), connectionPublicLabel(addr)).Inc()
}

func connectionTransportLabel(addr ma.Multiaddr) string {
	return bzz.ClassifyTransport(addr).String()
}

func connectionPublicLabel(addr ma.Multiaddr) string {
	if manet.IsPublicAddr(addr) {
		return "true"
	}
	return "false"
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
