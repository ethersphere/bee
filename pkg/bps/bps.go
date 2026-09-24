// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bps exposes a simple hello-world wire protocol
// used to greet other peers.
package bps

import (
	"context"
	"errors"
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

var errNotBroker = errors.New("not a broker")

// Service is the bps protocol service.
type Service struct {
	mtx      sync.Mutex
	fullNode bool
	streamer p2p.Streamer

	// cohort registry - keep track of members, publisher, channels, challenge, streams?
	// this is only relevant for the actual broker. subscribers don't care of manage this state at all.
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

// Join onto a topic at the broker at address. This is called on the subscriber.
// Returns the challenge and a channel that would send the payloads over it.
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

// claim a given topic on a broker (at address) with the provided signature.
// this is called on the subsciber that wants to become a publisher.
// returns the write channel used in order to send later payloads.
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

// handler is the protocol handler on the broker.
func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	if !s.fullNode {
		stream.Reset()
		return errNotBroker
	}
	w, r := protobuf.NewWriterAndReader(stream)

	var join pb.Join
	if err := r.ReadMsgWithContext(ctx, &join); err != nil {
		go stream.FullClose()
		return fmt.Errorf("read sys message: %w", err)
	}

	// peer is trying to join the cohort. accept and return the challenge
	ack := pb.JoinAck{Challenge: []byte{0, 1, 2, 3}}
	if err := w.WriteMsgWithContext(ctx, &ack); err != nil {
		go stream.FullClose()
		return fmt.Errorf("write claim: %w", err)
	}

	// add to the cohort and get a channel to receive the broadcasts on
	ch := make(chan []byte) // replace with the registry channel later

	// the writer side - reads messages off the publisher channel
	// and pushes them to the subscriber stream
	go func() {
		defer stream.FullClose()
		for {
			// await messages, then write to the stream once they come in
			select {
			case msg := <-ch:
				m := pb.Broadcast{Soc: msg}
				if err := w.WriteMsgWithContext(ctx, &m); err != nil {
					s.logger.Error(err, "write broadcast")
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// peer is trying to claim the cohort. if the challenge doesn't add up - kick them off
	//_ = claim.Challenge
	// check if claim signature pk matches the feed owner address on the registry
	// if it does - this is an atomic swap - the current stream becomes the publisher stream
	// and the next challenge changes randomly, so that if needed, the publisher can reclaim
	// later and rejoin + reclaim.

	// writer side - we first try to read a claim. we can wait indefinitely here.
	// once a claim is accepted, we prompte the stream to be a publisher stream
	// then continuously try to read from it broadcast messages and push them over the
	// publisher channel
	ch1 := make(chan []byte) // replace with the cohort send channel later
	go func() {
		defer stream.FullClose()
		claim := pb.Claim{}
		if err := r.ReadMsgWithContext(ctx, &claim); err != nil {
			s.logger.Error(err, "read claim")
			return
		}

		// do the checks on the claim, if it is wrong then we reset the stream

		for {
			// await messages, then write to the cohort channel
			m := pb.Broadcast{}
			if err := r.ReadMsgWithContext(ctx, &m); err != nil {
				s.logger.Error(err, "read broadcast")
				return
			}
			select {
			case ch1 <- m.Soc:
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}
