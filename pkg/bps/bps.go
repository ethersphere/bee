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
	protocolName    = "bps"
	protocolVersion = "1.0.0"
	streamName      = "bps"
)

// Interface is the main interface of the bps protocol.
//type Interface interface {
//Join(ctx context.Context, p p2p.Peer, topic []byte, notify <-chan []byte) error
//Publish(ctx context.Context, topic []byte, message []byte) error
//}

// Service is the bps protocol service.
type Service struct {
	mtx      sync.Mutex
	streamer p2p.Streamer
	// cohort registry - keep track of members, publisher, channels, challenge, streams?
	registry *CohortRegistry

	logger log.Logger
}

// New returns a new bps Service.
func New(streamer p2p.Streamer, logger log.Logger) *Service {
	return &Service{
		streamer: streamer,
		registry: NewRegistry(),
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

// Join onto a topic at the remote peer. The channel passed at registration will be notified
// with the payloads once they arrive. Returns the challenge and a channel that would send
// the payloads over it.
func (s *Service) Join(ctx context.Context, address swarm.Address, topic []byte) ([]byte, chan []byte, error) {
	stream, err := s.streamer.NewStream(ctx, address, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, nil, fmt.Errorf("new stream: %w", err)
	}

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

	// from now on we expect only to read updates off this stream
	go func() {
		defer stream.FullClose()

		for {
			msg := pb.Broadcast{}
			if err := r.ReadMsgWithContext(ctx, &msg); err != nil {
				s.logger.Error(err, "read join ack")
				return
			}

			// we might want to do some input validation to see that the broker isn't tricking us

			select {
			case rxCh <- msg.Soc:
			default:
			}

		}
	}()
	return joinAck.Challenge, rxCh, nil
}

// claim a given topic on a broker with the provided signature. returns the write channel used in order
// to send later payloads
func (s *Service) Claim(ctx context.Context, address swarm.Address, topic, sig []byte) (chan []byte, error) {
	stream, err := s.streamer.NewStream(ctx, address, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, fmt.Errorf("new stream: %w", err)
	}

	w, r := protobuf.NewWriterAndReader(stream)
	claim := pb.SystemMessage{SysMessage: &pb.SystemMessage_Claim{
		Claim: &pb.Claim{Sig: topic},
	}}
	if err := w.WriteMsgWithContext(ctx, &claim); err != nil {
		return nil, fmt.Errorf("write claim: %w", err)
	}
	claimAck := pb.ClaimAck{}
	// if the claim is wrong we will just get kicked off with a stream reset and the read will fail
	if err := r.ReadMsgWithContext(ctx, &claimAck); err != nil {
		return nil, fmt.Errorf("read claim ack: %w", err)
	}

	ch := make(chan []byte)

	// from now on we expect only to write updates to this stream
	go func() {
		defer stream.FullClose()

		for {
			// we might want to do some input validation to see that the broker isn't tricking us

			select {
			case v, ok := <-ch:
				// closing the channel causes us to exit and close the stream
				if !ok {
					return
				}
				msg := pb.Broadcast{Soc: v}
				if err := w.WriteMsgWithContext(ctx, &msg); err != nil {
					s.logger.Error(err, "read join ack")
					return
				}
			}
		}
	}()
	return ch, nil
}

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	defer stream.FullClose()

	var sysMsg pb.SystemMessage
	if err := r.ReadMsgWithContext(ctx, &sysMsg); err != nil {
		return fmt.Errorf("read sys message: %w", err)
	}

	if join := sysMsg.GetJoin(); join != nil {
		// peer is trying to join the cohort. accept and return the challenge
		ack := pb.JoinAck{Challenge: []byte{0, 1, 2, 3}}
		if err := w.WriteMsgWithContext(ctx, &ack); err != nil {
			return fmt.Errorf("write claim: %w", err)
		}
		return nil
	}
	if claim := sysMsg.GetClaim(); claim != nil {
		// peer is trying to claim the cohort. if the challenge doesn't add up - kick them off

		return nil
	}

	return nil
}
