// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate sh -c "protoc -I . -I \"$(go list -f '{{ .Dir }}' -m github.com/gogo/protobuf)/protobuf\" --gogofaster_out=. pingpong.proto"

package pingpong

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/janos/bee/pkg/p2p"
	"github.com/janos/bee/pkg/p2p/protobuf"
)

const (
	protocolName  = "pingpong"
	streamName    = "pingpong"
	streamVersion = "1.0.0"
)

type Service struct {
	streamer p2p.Streamer
	metrics  metrics
}

func New(streamer p2p.Streamer) *Service {
	return &Service{
		streamer: streamer,
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

func (s *Service) Handler(p p2p.Peer) {
	w, r := protobuf.NewWriterAndReader(p.Stream)
	defer p.Stream.Close()

	var ping Ping
	for {
		if err := r.ReadMsg(&ping); err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("pingpong handler: read message: %v\n", err)
			return
		}
		log.Printf("got ping: %q\n", ping.Greeting)
		s.metrics.PingReceivedCount.Inc()

		if err := w.WriteMsg(&Pong{
			Response: "{" + ping.Greeting + "}",
		}); err != nil {
			log.Printf("pingpong handler: write message: %v\n", err)
			return
		}
		s.metrics.PongSentCount.Inc()
	}
}

func (s *Service) Ping(ctx context.Context, peerID string, msgs ...string) (rtt time.Duration, err error) {
	stream, err := s.streamer.NewStream(ctx, peerID, protocolName, streamName, streamVersion)
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

		log.Printf("got pong: %q\n", pong.Response)
		s.metrics.PongReceivedCount.Inc()
	}
	return time.Since(start) / time.Duration(len(msgs)), nil
}
