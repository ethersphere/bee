// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// ErrRefused is matched by every RefusalError.
var ErrRefused = errors.New("bps: broker refused the handshake")

// RefusalError is returned when a broker answers a handshake with a non-OK
// status. The status distinguishes a full broker from an unknown topic or a
// rejection, which callers act on differently.
type RefusalError struct {
	Status pb.Status
}

func (e *RefusalError) Error() string {
	return fmt.Sprintf("bps: broker refused the handshake: %s", e.Status)
}

// Is reports whether target is ErrRefused, so callers can match the class
// without naming the status.
func (e *RefusalError) Is(target error) bool {
	return target == ErrRefused
}

// Session is one peer's participation in one cohort: a long-lived stream to
// the broker, plus fan-out to local consumers.
type Session struct {
	svc       *Service
	topic     swarm.Address
	spec      *pb.CohortSpec
	stream    p2p.Stream
	w         protobuf.Writer
	publisher bool

	messages chan *soc.SOC

	// writeMu serialises writes to w. The protobuf writer wraps a varint
	// framing writer with mutable per-write scratch state, so two concurrent
	// Publish calls would interleave bytes on the wire and desynchronise the
	// broker's framing permanently. Publish is part of the exported Publisher
	// surface and its intended consumer is a WebSocket bridge, where
	// concurrent writes are the normal shape, so the lock lives here rather
	// than being every caller's problem.
	writeMu sync.Mutex

	closeOnce sync.Once
	quit      chan struct{}
	// readDone is closed when the read loop this session started actually
	// returns. Close waits on it, so the goroutine's lifetime is owned by
	// the Session that started it rather than by Service.
	readDone chan struct{}
}

// Open fixes a new cohort at peer, or joins an existing one with an identical
// spec — SWIP-60's Open is idempotent, so a client need not know whether it is
// first. A non-nil auth makes the session a publisher.
func (s *Service) Open(ctx context.Context, peer swarm.Address, spec *pb.CohortSpec, auth *pb.PublisherAuth) (*Session, error) {
	if err := ValidateSpec(spec); err != nil {
		return nil, err
	}
	return s.handshake(ctx, peer, swarm.NewAddress(spec.GetTopic()), spec, &pb.Hello{
		Handshake: &pb.Hello_Open{Open: &pb.Open{Cohort: spec, Auth: auth}},
	}, auth != nil)
}

// Subscribe joins an existing cohort at peer, learning its spec from the Ack
// echo. A non-nil auth makes the session a publisher.
func (s *Service) Subscribe(ctx context.Context, peer swarm.Address, topic swarm.Address, auth *pb.PublisherAuth) (*Session, error) {
	return s.handshake(ctx, peer, topic, nil, &pb.Hello{
		Handshake: &pb.Hello_Subscribe{Subscribe: &pb.Subscribe{Topic: topic.Bytes(), Auth: auth}},
	}, auth != nil)
}

// handshake performs the client side of the Hello/Ack exchange. want is the
// spec the caller asked for, or nil when the caller has nothing to compare
// against — see the spec check below.
func (s *Service) handshake(ctx context.Context, peer swarm.Address, topic swarm.Address, want *pb.CohortSpec, hello *pb.Hello, publisher bool) (ss *Session, err error) {
	select {
	case <-s.quit:
		return nil, ErrShutdown
	default:
	}

	// Bound the exchange the same way the broker bounds its own side: with a
	// context.Background() caller, a broker that accepts the stream and never
	// answers would otherwise park the caller forever.
	ctx, cancel := context.WithTimeout(ctx, HandshakeTimeout)
	defer cancel()

	stream, err := s.streamer.NewStream(ctx, peer, nil, ProtocolName, ProtocolVersion, StreamName)
	if err != nil {
		return nil, fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		}
	}()

	w, r := protobuf.NewWriterAndReader(stream)
	if err := w.WriteMsgWithContext(ctx, hello); err != nil {
		return nil, fmt.Errorf("write hello: %w", err)
	}

	var ack pb.Ack
	if err := r.ReadMsgWithContext(ctx, &ack); err != nil {
		return nil, fmt.Errorf("read ack: %w", err)
	}
	if ack.GetStatus() != pb.Status_OK {
		return nil, &RefusalError{Status: ack.GetStatus()}
	}
	// The echoed spec is what every inbound message is verified against, so a
	// broker that echoes nonsense is refused here rather than trusted later.
	if err := ValidateSpec(ack.GetCohort()); err != nil {
		return nil, fmt.Errorf("echoed cohort spec: %w", err)
	}
	if !topic.Equal(swarm.NewAddress(ack.GetCohort().GetTopic())) {
		return nil, fmt.Errorf("echoed topic %x: %w", ack.GetCohort().GetTopic(), ErrSpecMismatch)
	}
	// An Open knows exactly which cohort it asked for, so the echo must match
	// it field for field: adopting the broker's version instead would let a
	// broker substitute the publisher set, the admin, or the closed flag, and
	// that substituted spec is the sole input to verify() for every later
	// message. Subscribe cannot make this check — it learns the spec from the
	// echo and has nothing to compare against — so a subscriber is only ever
	// as trustworthy as its knowledge of the spec. That caveat now applies
	// across every supported binding: under an explicit publisher regime the
	// topic no longer pins the owner (ANCHOR included — see anchorBinding),
	// so a hostile broker can echo a substituted admin or publisher list and
	// have it accepted by a spec-less Subscribe. Subscribe will need the spec
	// supplied out of band by the invite rather than learned from the broker.
	if want != nil && !SpecEqual(want, ack.GetCohort()) {
		return nil, fmt.Errorf("broker echoed a different spec: %w", ErrSpecMismatch)
	}

	ss = &Session{
		svc:       s,
		topic:     topic,
		spec:      ack.GetCohort(),
		stream:    stream,
		w:         w,
		publisher: publisher,
		messages:  make(chan *soc.SOC, OutboundQueueSize),
		quit:      make(chan struct{}),
		readDone:  make(chan struct{}),
	}

	s.sessionsMu.Lock()
	s.sessions[ss] = struct{}{}
	s.sessionsMu.Unlock()

	go func() {
		defer close(ss.readDone)
		ss.read(r)
	}()

	return ss, nil
}

// Topic returns the cohort's topic.
func (ss *Session) Topic() swarm.Address { return ss.topic }

// Spec returns the cohort spec echoed by the broker. Every inbound message is
// verified against it end to end. The returned spec is owned by the session
// and must be treated as read-only: mutating it changes the rules verify
// enforces on every subsequent message.
func (ss *Session) Spec() *pb.CohortSpec { return ss.spec }

// Messages returns the channel of verified inbound messages. It is closed when
// the session ends.
func (ss *Session) Messages() <-chan *soc.SOC { return ss.messages }

// Publish sends a single-owner chunk to the broker. It fails for a read-only
// session, and for a chunk that would not survive the broker's own checks —
// there is no point spending a round trip on a message the broker will drop.
//
// Publish is safe for concurrent use: calls from multiple goroutines are
// serialised on the session's stream, so frames never interleave.
func (ss *Session) Publish(ctx context.Context, s *soc.SOC) error {
	if !ss.publisher {
		return fmt.Errorf("read-only session: %w", ErrNotPublisher)
	}
	select {
	case <-ss.quit:
		return ErrShutdown
	default:
	}

	if err := ss.verify(s); err != nil {
		return err
	}

	m, err := SocToProto(s)
	if err != nil {
		return err
	}
	ss.writeMu.Lock()
	err = ss.w.WriteMsgWithContext(ctx, &pb.Publish{Soc: m})
	ss.writeMu.Unlock()
	if err != nil {
		return fmt.Errorf("write publish: %w", err)
	}
	ss.svc.metrics.Published.Inc()

	return nil
}

// verify checks a message against the cohort spec: it must qualify under the
// topic binding, and its owner must be a legitimate publisher. This is the
// end-to-end check SWIP-60 requires of every subscriber — the broker can
// withhold, never forge.
func (ss *Session) verify(s *soc.SOC) error {
	b, err := bindingFor(ss.spec.GetBinding())
	if err != nil {
		return err
	}
	if err := b.qualifies(ss.spec, s); err != nil {
		return err
	}
	return authorizePublisher(ss.spec, s.OwnerAddress())
}

func (ss *Session) read(r protobuf.Reader) {
	defer close(ss.messages)

	for {
		var bc pb.Broadcast
		if err := r.ReadMsg(&bc); err != nil {
			select {
			case <-ss.quit:
			default:
				ss.svc.logger.Debug("session read", "topic", ss.topic, "error", err)
			}
			return
		}

		m := bc.GetSoc()
		if m == nil {
			// Reserved multihop control frames: unknown to a singlehop peer,
			// ignored rather than fatal, so bps-multihop needs no version bump.
			continue
		}

		s, err := SocFromProto(m)
		if err != nil {
			ss.svc.metrics.Dropped.WithLabelValues("malformed").Inc()
			ss.svc.logger.Debug("session: malformed message", "topic", ss.topic, "error", err)
			continue
		}
		if err := ss.verify(s); err != nil {
			ss.svc.metrics.Dropped.WithLabelValues("unverified").Inc()
			ss.svc.logger.Debug("session: message failed verification", "topic", ss.topic, "error", err)
			continue
		}

		select {
		case ss.messages <- s:
		case <-ss.quit:
			return
		}
	}
}

// Close ends the session and tears down its stream. It does not return until
// the session's own read loop has actually returned, so the goroutine never
// outlives Close — every caller waits on readDone, not just whichever one
// happened to run the teardown.
func (ss *Session) Close() error {
	ss.closeOnce.Do(func() {
		close(ss.quit)
		_ = ss.stream.Reset()

		ss.svc.sessionsMu.Lock()
		delete(ss.svc.sessions, ss)
		ss.svc.sessionsMu.Unlock()
	})
	<-ss.readDone
	return nil
}

// Publisher is the local surface of a cohort session, as downstream consumers
// (the WebSocket bridge, later) see it.
type Publisher interface {
	Topic() swarm.Address
	Spec() *pb.CohortSpec
	// Publish sends a single-owner chunk to the broker. Implementations must
	// be safe for concurrent use.
	Publish(ctx context.Context, s *soc.SOC) error
	Messages() <-chan *soc.SOC
	Close() error
}

var _ Publisher = (*Session)(nil)
