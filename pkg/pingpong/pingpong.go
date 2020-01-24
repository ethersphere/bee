// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate sh -c "protoc -I . -I \"$(go list -f '{{ .Dir }}' -m github.com/gogo/protobuf)/protobuf\" --gogofaster_out=. pingpong.proto"
package pingpong

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
)

const (
	protocolName  = "pingpong"
	streamName    = "pingpong"
	streamVersion = "1.0.0"
)

type Service struct {
	streamer p2p.Streamer
	logger   Logger
	metrics  metrics
}

type Options struct {
	Streamer p2p.Streamer
	Logger   Logger
}

type Logger interface {
	Debugf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

func New(o Options) *Service {
	return &Service{
		streamer: o.Streamer,
		logger:   o.Logger,
		metrics:  newMetrics(),
	}
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name: protocolName,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamName,
				Version: streamVersion,
				Handler: s.Handler,
			},
		},
	}
}

func (s *Service) Ping(ctx context.Context, address string, msgs ...string) (rtt time.Duration, err error) {
	stream, err := s.streamer.NewStream(ctx, address, protocolName, streamName, streamVersion)
	if err != nil {
		return 0, fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	w, r := protobuf.NewWriterAndReader(stream)

	var pong Pong
	start := time.Now()
	for _, msg := range msgs {
		if err := w.WriteMsg(&Ping{
			Greeting: msg,
		}); err != nil {
			return 0, fmt.Errorf("stream write: %w", err)
		}
		s.metrics.PingSentCount.Inc()

		if err := r.ReadMsg(&pong); err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}

		s.logger.Debugf("got pong: %q", pong.Response)
		s.metrics.PongReceivedCount.Inc()
	}
	return time.Since(start) / time.Duration(len(msgs)), nil
}

func (s *Service) Handler(peer p2p.Peer, stream p2p.Stream) {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.Close()

	var ping Ping
	for {
		if err := r.ReadMsg(&ping); err != nil {
			if err == io.EOF {
				break
			}
			s.logger.Errorf("pingpong handler: read message: %v\n", err)
			return
		}
		s.logger.Debugf("got ping: %q", ping.Greeting)
		s.metrics.PingReceivedCount.Inc()

		if err := w.WriteMsg(&Pong{
			Response: "{" + ping.Greeting + "}",
		}); err != nil {
			s.logger.Errorf("pingpong handler: write message: %v\n", err)
			return
		}
		s.metrics.PongSentCount.Inc()
	}
}
