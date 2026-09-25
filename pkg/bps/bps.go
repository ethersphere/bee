// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bps exposes a simple hello-world wire protocol
// used to greet other peers.
package bps

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "bps"

const (
	protocolName    = "bps"
	protocolVersion = "1.0.0"
	streamName      = "bps"
)

// broadcastBufferSize is the per-member buffer for broadcast messages.
// Delivery is lossy by design (a slow member must not stall the cohort),
// so the buffer absorbs short bursts while the member's stream is busy.
const broadcastBufferSize = 1

var errNotBroker = errors.New("not a broker")

// Service is the bps protocol service.
type Service struct {
	fullNode    bool
	mtx         sync.Mutex
	selfOverlay swarm.Address // this node's overlay. used to verify challenges
	streamer    p2p.Streamer
	cohorts     map[string]*cohort
	// cohort registry - keep track of members, publisher, channels, challenge, streams?
	// this is only relevant for the actual broker. subscribers don't care of manage this state at all.
	// registry *CohortRegistry

	logger log.Logger
}

// New returns a new bps Service.
func New(streamer p2p.Streamer, overlay swarm.Address, fullNode bool, logger log.Logger) *Service {
	return &Service{
		fullNode:    fullNode,
		selfOverlay: overlay,
		streamer:    streamer,
		cohorts:     make(map[string]*cohort),
		logger:      logger.WithName(loggerName).Register(),
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
func (s *Service) Join(ctx context.Context, address swarm.Address, topic []byte) (challenge []byte, rx, tx chan []byte, claim func([]byte), err error) {
	stream, err := s.streamer.NewStream(ctx, address, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("new stream: %w", err)
	}

	w, r := protobuf.NewWriterAndReader(stream)
	joinMsg := pb.Join{Topic: topic}
	if err := w.WriteMsgWithContext(ctx, &joinMsg); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("write join msg: %w", err)
	}
	joinAck := pb.JoinAck{}
	if err := r.ReadMsgWithContext(ctx, &joinAck); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("read join ack: %w", err)
	}

	rxCh := make(chan []byte, broadcastBufferSize)

	// reading should always yield broadcast msgs
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

	// the write loop will only be used in the case of a publisher.
	// for readers this is essentially a noop.
	tx = make(chan []byte)
	claim = func(soc []byte) {
		select {
		case tx <- soc:
		case <-ctx.Done():
			return
		}
	}
	go func() {
		defer stream.FullClose()

		select {
		case cl := <-tx:
			claim := pb.Claim{Soc: cl}
			if err := w.WriteMsgWithContext(ctx, &claim); err != nil {
				s.logger.Error(err, "read join ack")
				return
			}
		case <-ctx.Done():
			return
		}

		for {
			select {
			case bcast := <-tx:
				msg := pb.Broadcast{Soc: bcast}
				if err := w.WriteMsgWithContext(ctx, &msg); err != nil {
					s.logger.Error(err, "write broadcast")
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return joinAck.Challenge, rxCh, tx, claim, nil
}

// handler is the protocol handler on the broker.
func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	if !s.fullNode {
		stream.Reset()
		return errNotBroker
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer stream.FullClose()
	w, r := protobuf.NewWriterAndReader(stream)

	var join pb.Join
	if err := r.ReadMsgWithContext(ctx, &join); err != nil {
		return fmt.Errorf("read sys message: %w", err)
	}

	// add to the cohort and get a channel to receive the broadcasts on
	ch, challenge := s.joinCohort(p.Address, join.Topic)
	// register in the cohort and return the secret
	// peer is trying to join the cohort. accept and return the challenge
	ack := pb.JoinAck{Challenge: challenge}
	if err := w.WriteMsgWithContext(ctx, &ack); err != nil {
		s.left(p.Address, join.Topic)
		return fmt.Errorf("write claim: %w", err)
	}

	defer s.left(p.Address, join.Topic)
	var wg sync.WaitGroup
	// the writer side - reads messages off the publisher channel
	// and pushes them to the subscriber stream
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
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
	// check if claim signature pk matches the feed owner address on the registry
	// if it does - this is an atomic swap - the current stream becomes the publisher stream
	// and the next challenge changes randomly, so that if needed, the publisher can reclaim
	// later and rejoin + reclaim.

	// writer side - we first try to read a claim. we can wait indefinitely here.
	// once a claim is accepted, we prompte the stream to be a publisher stream
	// then continuously try to read from it broadcast messages and push them over the
	// publisher channel
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		claim := pb.Claim{}
		if err := r.ReadMsgWithContext(ctx, &claim); err != nil {
			s.logger.Error(err, "read claim")
			return
		}

		// do the checks on the claim, if it is wrong then we reset the stream
		ch1, err := s.claim(p.Address, join.Topic, claim.Soc)
		if err != nil {
			stream.Reset()
			return
		}
		defer close(ch1)
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
	wg.Wait()
	return nil
}

var errInvalidProof = errors.New("invalid proof")

// join/open operation in one - the peer either joins an existing
// cohort or creates one by joining a previously unregistered topic.
// returns the cohort challenge and the channel that data will be sent over
func (s *Service) joinCohort(overlay swarm.Address, topic []byte) (chan []byte, []byte) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	t := string(topic)
	co, ok := s.cohorts[t]

	m := newMember()
	if ok {
		co.members[overlay.String()] = m
		return m.ch, m.challenge
	}
	members := make(map[string]*member)
	members[overlay.String()] = m
	s.cohorts[t] = &cohort{
		topic:   topic,
		members: members,
	}

	return m.ch, m.challenge
}

func (s *Service) left(overlay swarm.Address, topic []byte) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	t := string(topic)
	co, ok := s.cohorts[t]
	if ok {
		delete(co.members, overlay.String())
		if len(co.members) == 0 {
			delete(s.cohorts, t)
		}
	}
}

func (s *Service) claim(overlay swarm.Address, topic, socBlob []byte) (chan []byte, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	t := string(topic)
	co, ok := s.cohorts[t]
	if !ok {
		return nil, errors.New("tried to claim non-existent topic")
	}
	member, ok := co.members[overlay.String()]
	if !ok {
		return nil, errors.New("no such member")
	}
	chunk := swarm.NewChunk(swarm.NewAddress(topic), socBlob)
	if !soc.Valid(chunk) {
		return nil, errInvalidProof
	}
	innerProof := make([]byte, len(s.selfOverlay.Bytes())+len(member.challenge))
	copy(innerProof, member.challenge)
	copy(innerProof[len(member.challenge):], s.selfOverlay.Bytes())
	proofSoc, _ := soc.FromChunk(chunk)
	if !bytes.Equal(proofSoc.WrappedChunk().Data()[swarm.SpanSize:], innerProof) {
		return nil, errInvalidProof
	}

	publisher := member
	txCh := make(chan []byte)
	go func() {
		for v := range txCh {
			s.mtx.Lock()
			co, ok := s.cohorts[t]
			if !ok {
				s.mtx.Unlock()
				return
			}
			for _, m := range co.members {
				if m == publisher {
					continue
				}
				select {
				case m.ch <- v:
				default:
				}
			}
			s.mtx.Unlock()
		}
	}()
	return txCh, nil
}

type cohort struct {
	topic []byte // topic is the feed topic + owner hash
	// lastSeen uint64 // last feed update index, used to prevent replay and circumvent dedup logic (for now)
	members map[string]*member
}

func newMember() *member {
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		panic(err)
	}

	return &member{
		ch:        make(chan []byte, broadcastBufferSize),
		challenge: challenge,
	}
}

type member struct {
	ch        chan []byte
	challenge []byte
}
