// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bps exposes a simple hello-world wire protocol
// used to greet other peers.
package bps

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "bps"

const (
	protocolName    = "bps"
	protocolVersion = "1.0.0"
	streamName      = "bps"
)

var _ Interface = (*Service)(nil)

// Interface is the main interface of the bps protocol.
type Interface interface {
	Greet(ctx context.Context, address swarm.Address, greeting string) (string, error)
}

// Service is the bps protocol service.
type Service struct {
	streamer p2p.Streamer
	logger   log.Logger
}

// New returns a new bps Service.
func New(streamer p2p.Streamer, logger log.Logger) *Service {
	return &Service{
		streamer: streamer,
		logger:   logger.WithName(loggerName).Register(),
	}
}

// Protocol returns the p2p protocol specification for bps.
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

// Greet sends a greeting to the peer at address and returns its response.
func (s *Service) Greet(ctx context.Context, address swarm.Address, greeting string) (string, error) {
	stream, err := s.streamer.NewStream(ctx, address, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return "", fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		go stream.FullClose()
	}()

	w, r := protobuf.NewWriterAndReader(stream)

	if err := w.WriteMsgWithContext(ctx, &pb.Hello{Greeting: greeting}); err != nil {
		return "", fmt.Errorf("write message: %w", err)
	}

	var welcome pb.Welcome
	if err := r.ReadMsgWithContext(ctx, &welcome); err != nil {
		return "", fmt.Errorf("read message: %w", err)
	}

	return welcome.Response, nil
}

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.FullClose()

	var hello pb.Hello
	if err := r.ReadMsgWithContext(ctx, &hello); err != nil {
		return fmt.Errorf("read message: %w", err)
	}

	if err := w.WriteMsgWithContext(ctx, &pb.Welcome{
		Response: fmt.Sprintf("hello, %s", hello.Greeting),
	}); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}
