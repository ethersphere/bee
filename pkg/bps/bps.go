// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bps exposes a simple hello-world wire protocol
// used to greet other peers.
package bps

import (
	"context"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "bps"

const (
	protocolName      = "bps"
	protocolVersion   = "1.0.0"
	streamNameSystem  = "bps-system"
	streamNameMessage = "bps-message"
)

// Interface is the main interface of the bps protocol.
type Interface interface {
	Join(ctx context.Context, p p2p.Peer, topic []byte, notify <-chan []byte) error
	Publish(ctx context.Context, topic []byte, message []byte) error
}

// Service is the bps protocol service.
type Service struct {
	mtx      sync.Mutex
	streamer p2p.Streamer
	// cohort registry - keep track of members, publisher, channels, challenge, streams?
	registry *CohortRegistry

	// joined
	joined map[string]chan []byte

	logger log.Logger
}

// New returns a new bps Service.
func New(streamer p2p.Streamer, logger log.Logger) *Service {
	return &Service{
		streamer: streamer,
		registry: NewRegistry(),
		joined:   make(map[string]chan []byte),
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
				Name:    streamNameSystem,
				Handler: s.handlerSystem,
			}, {
				Name:    streamNameMessage,
				Handler: s.handlerMessage,
			},
		},
	}
}

// Join onto a topic at the remote peer. The channel passed at registration will be notified
// with the payloads once they arrive. Returns the challenge and a channel that would send
// the payloads over it
func (s *Service) Join(ctx context.Context, address swarm.Address, topic []byte) ([]byte, chan []byte, error) {
	stream, err := s.streamer.NewStream(ctx, address, nil, protocolName, protocolVersion, streamNameSystem)
	if err != nil {
		return nil, nil, fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		go stream.FullClose()
	}()

	w, r := protobuf.NewWriterAndReader(stream)
	joinMsg := pb.SystemMessage{SysMessage: &pb.SystemMessage_Join{
		Join: &pb.Join{Topic: topic},
	}}
	if err := w.WriteMsgWithContext(ctx, &joinMsg); err != nil {
		return nil, nil, fmt.Errorf("write join msg: %w", err)
	}
	joinAck := pb.JoinAck{}
	if err := r.ReadMsgWithContext(ctx, &joinAck); err != nil {
		return nil, nil, fmt.Errorf("read join ack: %w", err)
	}
	rxCh := make(chan []byte)
	s.joined[string(topic)] = rxCh
	return joinAck.Challenge, rxCh, nil
}

func (s *Service) handlerSystem(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	_, r := protobuf.NewWriterAndReader(stream)
	defer stream.FullClose()

	var sysMsg pb.SystemMessage
	if err := r.ReadMsgWithContext(ctx, &sysMsg); err != nil {
		return fmt.Errorf("read sys message: %w", err)
	}

	if join := sysMsg.GetJoin(); join != nil {
		// peer is trying to join the cohort. accept and return the challenge

		return nil
	}
	if claim := sysMsg.GetClaim(); claim != nil {
		// peer is trying to claim the cohort. if the challenge doesn't add up - kick them off

		return nil
	}

	return nil
}

func (s *Service) Publish(ctx context.Context, topic []byte, message []byte) error {
	// grabs an existing publish stream or creates it then pushes the message down the pipe
	// the stream stays alive and is reused later on for future messages
	return nil
}

// handlerMessage handles incoming messages. on non-brokers, it tries to find the
// topic in the joined map, then notify the subscribed reader off the channel.
// the stream is reused and kept with a long-running goroutine.
func (s *Service) handlerMessage(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	_, r := protobuf.NewWriterAndReader(stream)
	//defer stream.FullClose() // NO FULL CLOSE HERE
	//
	var msg pb.PayloadMessage
	if err := r.ReadMsgWithContext(ctx, &msg); err != nil {
		return fmt.Errorf("read sys message: %w", err)
	}

	if publish := msg.GetPublish(); publish != nil {
		// we got a publish message - this means we are a broker.
		// if there's a cohort - publish the message to the peers. launch a goroutine
		// and read off the channel and feed it to the different readers
		return nil
	}
	if bcast := msg.GetBroadcast(); bcast != nil {
		// we got a broadcast on this stream, it means we are a subscriber here.
		// find the subscriber channel(s), launch polling goroutine and duly notify
		// when something comes in on the stream

		return nil
	}

	return nil
}
