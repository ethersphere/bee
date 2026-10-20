// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// sessionTimeout bounds one reconcile session on either side, so that a peer
// cannot keep a session (and its slot) open forever with valid messages.
const sessionTimeout = 10 * time.Minute

// openingScope is a scope that accepts every range: checking the opening
// against it checks the strict decoding of its parts and the message limits
// only.
var openingScope = Scope{Root: fingerprint.Range{}, LeafDepth: fingerprint.MaxBits}

// sessionContext returns the context of one session: ctx bounded by
// sessionTimeout and canceled by Close.
func (s *Service) sessionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, sessionTimeout)
	stop := context.AfterFunc(s.ctx, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

// Reconcile runs one reconciliation session with peer as the initiator and
// returns the IDs that the peer listed and this node does not hold (wants),
// with nil batch IDs. The session's scope comes from this node's Status and
// the peer's last Status (LastStatus).
//
// It returns ErrNoStatus without a Status of the peer, ErrMappingMismatch if
// the peer's Mapping differs from this node's, an error wrapping
// fingerprint.ErrBusy until the index is built, ErrPeerBusy if the peer
// answered Busy, and an error wrapping ErrInvalidMessage if the peer broke
// the protocol: the stream is then reset, the peer disconnected and the
// failure reported to the PeerScorer. An error of this node's index or
// reserve (the index not built included) wraps ErrLocal. Other errors (the
// stream failing) are not scored either. A completed session is reported to
// the PeerScorer as a success.
func (s *Service) Reconcile(ctx context.Context, peer swarm.Address) (wants []storer.ChunkID, err error) {
	return s.ReconcileWith(ctx, peer, ReconcileOptions{})
}

// ReconcileOptions are the optional parameters of ReconcileWith.
type ReconcileOptions struct {
	// Open is Config.Open: the ranges of the opening Message, the session
	// root when empty. A striped transfer opens with its stripe.
	Open []fingerprint.Range
	// Striper is Config.Striper: it is offered the ranges with a large
	// difference.
	Striper Striper
	// Wants, if not nil, receives the wants as the session finds them, once
	// per handled Message with wants, instead of ReconcileWith returning
	// them. It is called on the session's goroutine, so a Wants that blocks
	// holds the session back (the peer waits for the next Message).
	Wants func(wants []storer.ChunkID)
}

// ReconcileWith is Reconcile with options. With opts.Wants set it returns no
// wants.
func (s *Service) ReconcileWith(ctx context.Context, peer swarm.Address, opts ReconcileOptions) (wants []storer.ChunkID, err error) {
	remote, ok := s.LastStatus(peer)
	if !ok {
		return nil, ErrNoStatus
	}
	if remote.Mapping != fingerprint.DefaultMapperID {
		return nil, fmt.Errorf("%w: peer mapping %d", ErrMappingMismatch, remote.Mapping)
	}
	st, err := s.LocalStatus()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLocal, err)
	}
	scope, err := NewScope(st, remote)
	if err != nil {
		return nil, fmt.Errorf("session scope: %w", err)
	}
	release, err := s.acquireSession(peer)
	if err != nil {
		return nil, err
	}
	defer release()
	cfg := s.sessionConfig(peer, scope, st, remote)
	cfg.Open, cfg.Striper = opts.Open, opts.Striper
	sess, open, err := NewInitiator(cfg)
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}

	ctx, cancel := s.sessionContext(ctx)
	defer cancel()
	stream, err := s.streamer.NewStream(ctx, peer, nil, ProtocolName, ProtocolVersion, ReconcileStreamName)
	if err != nil {
		return nil, fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		if err != nil && !errors.Is(err, ErrPeerBusy) {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()

	w, r := protobuf.NewWriter(stream), newReader(stream, MaxMessageBytes)
	if err := w.WriteMsgWithContext(ctx, open); err != nil {
		return nil, fmt.Errorf("write opening: %w", err)
	}
	s.metrics.reconcileMessage(dirOut, open)
	var emit func([]fingerprint.ID)
	found := 0
	if opts.Wants != nil {
		emit = func(ids []fingerprint.ID) {
			found += len(ids)
			opts.Wants(toChunkIDs(ids))
		}
	}
	ids, handled, err := s.converse(ctx, sess, nil, r, w, emit)
	s.metrics.sessionStats(sess.Stats())
	switch {
	case errors.Is(err, ErrPeerBusy):
		return nil, err
	case errors.Is(err, ErrInvalidMessage):
		return nil, s.initiatorViolation(peer, err)
	case err != nil:
		return nil, err
	}
	s.metrics.sessionDone(handled, len(ids)+found, sess.Difference())
	if s.sessionDone != nil {
		s.sessionDone(peer, sess, true)
	}
	s.scorer.Succeed(peer)
	return toChunkIDs(ids), nil
}

// sessionConfig returns the Config of a session with peer in scope, where
// local is this node's Status and remote the peer's.
func (s *Service) sessionConfig(peer swarm.Address, scope Scope, local, remote Status) Config {
	cfg := Config{
		Scope:      scope,
		Local:      s.local,
		PeerCount:  remote.Summary.Count,
		Status:     local,
		PeerStatus: remote,
	}
	if s.hints != nil { // nil only in simulations that run without hints
		cfg.Hints = s.hints.peer(peer)
	}
	return cfg
}

// acquireSession takes a node-wide session token and the session slot of
// peer for an outgoing session. Both caps are shared with the incoming
// sessions. It returns ErrNodeSessionLimit or ErrPeerSessionLimit (both
// ErrSessionLimit) when either is taken.
func (s *Service) acquireSession(peer swarm.Address) (release func(), err error) {
	select {
	case s.sessions <- struct{}{}:
	default:
		return nil, ErrNodeSessionLimit
	}
	releasePeer, ok := s.peerSessions.acquire(peer)
	if !ok {
		<-s.sessions
		return nil, ErrPeerSessionLimit
	}
	return func() {
		releasePeer()
		<-s.sessions
	}, nil
}

// initiatorViolation disconnects peer for the violation err and reports it
// to the scorer, unless the peer was reported within violationGrace
// (markViolation); the caller resets the stream. It returns err.
func (s *Service) initiatorViolation(peer swarm.Address, err error) error {
	reported := s.markViolation(peer)
	s.metrics.InvalidMessage.Inc()
	if !reported {
		s.scorer.Fail(peer, err)
	}
	s.logger.Debug("invalid message from peer, disconnecting", "peer_address", peer, "error", err)
	if derr := s.streamer.Disconnect(peer, "rangesync: invalid message"); derr != nil {
		s.logger.Debug("disconnect peer failed", "peer_address", peer, "error", derr)
	}
	return err
}

// converse drives sess until the session ends and returns the wants and the
// number of Messages handled. It starts by handling in, or by reading a
// Message if in is nil. Each Message is answered with the one Handle
// returns; the session ends when Handle returns no Message, or when the peer
// closes the stream after this side sent an empty Message. A Message that
// does not decode or is over MaxMessageBytes is a violation. With emit set,
// the wants of each handled Message are passed to it instead of returned.
func (s *Service) converse(ctx context.Context, sess *Session, in *pb.Message, r protobuf.Reader, w protobuf.Writer, emit func([]fingerprint.ID)) (wants []fingerprint.ID, handled int, err error) {
	sentEmpty := false
	for ; ; handled++ {
		if in == nil {
			in = new(pb.Message)
			if err := readMessage(ctx, r, in); err != nil {
				if errors.Is(err, io.EOF) && sentEmpty {
					// the peer ended the session after this side's empty Message
					return wants, handled, nil
				}
				return nil, handled, fmt.Errorf("read: %w", err)
			}
			s.metrics.reconcileMessage(dirIn, in)
		}
		out, ids, err := sess.Handle(in)
		if err != nil {
			return nil, handled, err
		}
		if emit != nil {
			if len(ids) > 0 {
				emit(ids)
			}
		} else {
			wants = append(wants, ids...)
		}
		if out == nil {
			return wants, handled + 1, nil
		}
		if err := w.WriteMsgWithContext(ctx, out); err != nil {
			return nil, handled + 1, fmt.Errorf("write: %w", err)
		}
		s.metrics.reconcileMessage(dirOut, out)
		sentEmpty = len(out.Parts) == 0
		in = nil
	}
}

// handleReconcile serves the reconcile stream: one session as the responder.
//
// The opening Message is read first (with the MaxMessageBytes limit) and must
// decode strictly. The session is answered with Busy when this node serves
// MaxConcurrentSessions sessions or one with the peer already, while the
// address index is not ready, while the index is not built or the opening
// asks about a range being rebuilt, when no Status of the peer was received,
// when the opening does not fit the scope derived from the two Statuses (one
// of them is stale), and when the peer opens sessions faster than
// PeerSessionBurst and PeerSessionInterval allow. A peer with a different Mapping gets its stream
// reset. A violation of the protocol resets the stream, disconnects the peer
// and is reported to the PeerScorer. A non-zero AuditRound is ignored in
// Phase 1 (salted sessions are Phase 5): the session runs unsalted.
func (s *Service) handleReconcile(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	peer := p.Address
	release, ok := s.reconcileStreams.acquire(peer)
	if !ok {
		return s.streamLimitExceeded(peer, stream, ReconcileStreamName, maxPeerReconcileStreams)
	}
	defer release()

	ctx, cancel := s.sessionContext(ctx)
	defer cancel()

	w, r := protobuf.NewWriter(stream), newReader(stream, MaxMessageBytes)
	open := new(pb.Message)
	if err := readMessage(ctx, r, open); err != nil {
		if errors.Is(err, ErrInvalidMessage) {
			return s.violation(peer, stream, fmt.Errorf("opening: %w", err))
		}
		_ = stream.Reset()
		return fmt.Errorf("read opening: %w", err)
	}
	s.metrics.reconcileMessage(dirIn, open)
	if err := openingScope.CheckMessage(open); err != nil {
		return s.violation(peer, stream, fmt.Errorf("opening: %w", err))
	}

	busy := func(reason string) error {
		s.metrics.Busy.Inc()
		s.logger.Debug("reconcile session busy", "peer_address", peer, "reason", reason)
		m := &pb.Message{Busy: true}
		if err := w.WriteMsgWithContext(ctx, m); err != nil {
			_ = stream.Reset()
			return fmt.Errorf("write busy: %w", err)
		}
		s.metrics.reconcileMessage(dirOut, m)
		return stream.FullClose()
	}

	select {
	case s.sessions <- struct{}{}:
		defer func() { <-s.sessions }()
	default:
		return busy("node session limit")
	}
	releaseSession, ok := s.peerSessions.acquire(peer)
	if !ok {
		return busy("peer session limit")
	}
	defer releaseSession()
	if !s.store.ReserveAddrIndexReady() {
		return busy("address index not ready")
	}
	st, err := s.LocalStatus()
	if err != nil {
		return busy("index not built")
	}
	remote, ok := s.LastStatus(peer)
	if !ok {
		return busy("no status of the peer")
	}
	if remote.Mapping != fingerprint.DefaultMapperID {
		_ = stream.Reset()
		return fmt.Errorf("%w: peer mapping %d", ErrMappingMismatch, remote.Mapping)
	}
	scope, err := NewScope(st, remote)
	if err != nil {
		// Not scored: the two Statuses come from different times and one
		// may be stale (the roots moved apart), so a scope that cannot be
		// built is not proof that the peer broke the protocol. The peer's
		// Status itself was validated when it was received.
		_ = stream.Reset()
		return fmt.Errorf("session scope: %w", err)
	}
	if err := scope.CheckMessage(open); err != nil {
		return busy("opening outside the scope")
	}
	// Last of the Busy checks, so that no other Busy spends a token. The
	// peer's Status is gone once it disconnected, so this rarely recreates
	// the entry disconnected cleared.
	if !s.sessionLimiter.Allow(peer.ByteString(), 1) {
		return busy("peer session rate")
	}

	sess := NewResponder(s.sessionConfig(peer, scope, st, remote))
	wants, handled, err := s.converse(ctx, sess, open, r, w, nil)
	s.metrics.sessionStats(sess.Stats())
	switch {
	case handled == 0 && errors.Is(err, fingerprint.ErrBusy):
		// the opening asks about a range that is being rebuilt
		return busy("range being rebuilt")
	case errors.Is(err, ErrInvalidMessage):
		return s.violation(peer, stream, err)
	case err != nil:
		_ = stream.Reset()
		return err
	}
	s.metrics.sessionDone(handled, len(wants), sess.Difference())
	if s.sessionDone != nil {
		s.sessionDone(peer, sess, false)
	}
	if s.wants != nil && len(wants) > 0 {
		s.wants(peer, toChunkIDs(wants))
	}
	s.scorer.Succeed(peer)
	return stream.FullClose()
}

// toChunkIDs converts IDs to storer.ChunkIDs with nil batch IDs.
func toChunkIDs(ids []fingerprint.ID) []storer.ChunkID {
	if len(ids) == 0 {
		return nil
	}
	out := make([]storer.ChunkID, len(ids))
	for i, id := range ids {
		out[i] = storer.ChunkID{Address: id.Address(), StampHash: id.StampHash()}
	}
	return out
}
