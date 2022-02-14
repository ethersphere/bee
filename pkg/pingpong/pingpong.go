// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pingpong exposes the simple ping-pong protocol
// which measures round-trip-time with other peers.
package pingpong

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/pingpong/pb"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tracing"
)

const (
	protocolName    = "pingpong"
	protocolVersion = "1.0.0"
	streamName      = "pingpong"
)

type Interface interface {
	Ping(ctx context.Context, address swarm.Address, msgs ...string) (rtt time.Duration, err error)
}

type Service struct {
	streamer p2p.Streamer
	logger   logging.Logger
	tracer   *tracing.Tracer
	metrics  metrics
}

func New(streamer p2p.Streamer, logger logging.Logger, tracer *tracing.Tracer) *Service {
	return &Service{
		streamer: streamer,
		logger:   logger,
		tracer:   tracer,
		metrics:  newMetrics(),
	}
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamName,
				Handler: s.handler,
			},
		},
	}
}

func (s *Service) Ping(ctx context.Context, address swarm.Address, msgs ...string) (rtt time.Duration, err error) {
	span, _, ctx := s.tracer.StartSpanFromContext(ctx, "pingpong-p2p-ping", s.logger)
	defer span.Finish()

	start := time.Now()
	stream, err := s.streamer.NewStream(ctx, address, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return 0, fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.Close()
		}
	}()

	w, r := protobuf.NewWriterAndReader(stream)

	var pong pb.Pong
	for _, msg := range msgs {
		if err := w.WriteMsgWithContext(ctx, &pb.Ping{
			Greeting: msg,
		}); err != nil {
			return 0, fmt.Errorf("write message: %w", err)
		}
		s.metrics.PingSentCount.Inc()

		if err := r.ReadMsgWithContext(ctx, &pong); err != nil {
			if err == io.EOF {
				break
			}
			return 0, fmt.Errorf("read message: %w", err)
		}

		s.metrics.PongReceivedCount.Inc()
	}
	return time.Since(start), nil
}

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	w, r := protobuf.NewWriterAndReader(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.Close()
		}
	}()

	span, _, ctx := s.tracer.StartSpanFromContext(ctx, "pingpong-p2p-handler", s.logger)
	defer span.Finish()

	var ping pb.Ping
	for {
		if err := r.ReadMsgWithContext(ctx, &ping); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read message: %w", err)
		}
		s.metrics.PingReceivedCount.Inc()

		if err := w.WriteMsgWithContext(ctx, &pb.Pong{
			Response: "{" + ping.Greeting + "}",
		}); err != nil {
			return fmt.Errorf("write message: %w", err)
		}
		s.metrics.PongSentCount.Inc()
	}
	return nil
}
