// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package versioned_test

import (
	"context"
	"fmt"

	"github.com/coreos/go-semver/semver"
	"github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/versioned"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	exampleProtocolName    = "versionedping"
	exampleProtocolVersion = "1.2.0"
	exampleStreamName      = "ping"
)

type exampleMetrics struct {
	HandledStreamVersionCount *prometheus.CounterVec
}

func newExampleMetrics() exampleMetrics {
	return exampleMetrics{
		HandledStreamVersionCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: metrics.Namespace,
				Subsystem: exampleProtocolName,
				Name:      "handled_stream_version_total",
				Help:      "Number of handled streams by protocol version.",
			},
			[]string{"version"},
		),
	}
}

// ExampleService represents a full Bee protocol service (structured like pkg/pingpong)
// supporting 3 version levels:
//   - v1.2.0: Current version
//   - v1.1.0: Legacy version 1.1
//   - v1.0.0: Legacy version 1.0
type ExampleService struct {
	streamer p2p.Streamer
	metrics  exampleMetrics
}

func NewExampleService(streamer p2p.Streamer) *ExampleService {
	return &ExampleService{
		streamer: streamer,
		metrics:  newExampleMetrics(),
	}
}

func (s *ExampleService) Metrics() []prometheus.Collector {
	return metrics.PrometheusCollectorsFromFields(s.metrics)
}

func (s *ExampleService) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    exampleProtocolName,
		Version: exampleProtocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name: exampleStreamName,
				Handler: versioned.NewHandlersFunc(
					[]versioned.Handler{
						{
							Version: semver.New("1.2.0"), // Server handler for >= 1.2.0
							Handler: func(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
								w, r := protobuf.NewWriterAndReader(stream)
								_, _ = w, r
								fmt.Println("Server received ping on v1.2.0 handler")
								return stream.FullClose()
							},
						},
						{
							Version: semver.New("1.1.0"), // Server handler for legacy 1.1.0
							Handler: func(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
								w, r := protobuf.NewWriterAndReader(stream)
								_, _ = w, r
								fmt.Println("Server received ping on v1.1.0 legacy handler")
								return stream.FullClose()
							},
						},
						{
							Version: semver.New("1.0.0"), // Server handler for legacy 1.0.0
							Handler: func(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
								w, r := protobuf.NewWriterAndReader(stream)
								_, _ = w, r
								fmt.Println("Server received ping on v1.0.0 legacy handler")
								return stream.FullClose()
							},
						},
					},
					versioned.WithMetricCounter(s.metrics.HandledStreamVersionCount),
				),
			},
		},
	}
}

func (s *ExampleService) Ping(ctx context.Context, peer swarm.Address) error {
	stream, err := s.streamer.NewStream(ctx, peer, nil, exampleProtocolName, "1.1.0", exampleStreamName)
	if err != nil {
		return err
	}
	defer stream.Close()

	pingClient := versioned.NewHandlersFunc(
		[]versioned.Handler{
			{
				Version: semver.New("1.2.0"), // Current version client (>= 1.2.0)
				Handler: func(ctx context.Context, _ p2p.Peer, stream p2p.Stream) error {
					w, r := protobuf.NewWriterAndReader(stream)
					_, _ = w, r
					fmt.Println("Client sent ping using v1.2.0 format")
					return nil
				},
			},
			{
				Version: semver.New("1.1.0"), // Legacy v1.1.0 client (1.1.0 <= v < 1.2.0)
				Handler: func(ctx context.Context, _ p2p.Peer, stream p2p.Stream) error {
					w, r := protobuf.NewWriterAndReader(stream)
					_, _ = w, r
					fmt.Println("Client sent ping using v1.1.0 legacy format")
					return nil
				},
			},
			{
				Version: semver.New("1.0.0"), // Legacy v1.0.0 client (1.0.0 <= v < 1.1.0)
				Handler: func(ctx context.Context, _ p2p.Peer, stream p2p.Stream) error {
					w, r := protobuf.NewWriterAndReader(stream)
					_, _ = w, r
					fmt.Println("Client sent ping using v1.0.0 legacy format")
					return nil
				},
			},
		},
		versioned.WithMetricCounter(s.metrics.HandledStreamVersionCount),
	)

	return pingClient(ctx, p2p.Peer{Address: peer}, stream)
}

type mockStreamer struct {
	p2p.Streamer
	supportedVersions map[string]bool
}

func (m *mockStreamer) NewStream(_ context.Context, _ swarm.Address, _ p2p.Headers, _, version, _ string) (p2p.Stream, error) {
	if !m.supportedVersions[version] {
		return nil, fmt.Errorf("protocol version not supported: %s", version)
	}
	return mockStream{
		version: version,
	}, nil
}

// Example_versionedProtocol demonstrates constructing a versioned P2P protocol service
// with Prometheus metrics tracking and executing versioned message exchange between client and server nodes.
func Example_versionedProtocol() {
	ctx := context.Background()

	serverSvc := NewExampleService(nil)
	serverSpec := serverSvc.Protocol()
	serverHandler := serverSpec.StreamSpecs[0].Handler

	// Client streamer connects to server where version 1.1.0 is negotiated:
	clientStreamer := &mockStreamer{
		supportedVersions: map[string]bool{
			"1.1.0": true,
		},
	}
	clientSvc := NewExampleService(clientStreamer)

	// Client sends ping request (client-side dispatcher automatically selects v1.1.0 format)
	_ = clientSvc.Ping(ctx, swarm.ZeroAddress)

	// Server receives incoming stream (server-side dispatcher automatically selects v1.1.0 handler)
	incomingStream := mockStream{
		version: "1.1.0",
	}
	_ = serverHandler(ctx, p2p.Peer{Address: swarm.ZeroAddress}, incomingStream)

	// Output:
	// Client sent ping using v1.1.0 legacy format
	// Server received ping on v1.1.0 legacy handler
}
