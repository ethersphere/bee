// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bps implements BPS-lite (SWIP-74) with the explicit multi-publisher
// mode of SWIP-87: a live stream over feeds, through one broker, one hop.
//
// A cohort is identified by its spec {topic, FEED_TOPIC, principal}. Every
// peer joins with the spec and its own identity and is answered with a random
// per-stream challenge. A stream declaring the principal — the admin — or an
// address on the admin's latest roster is pending until its AUTH — the empty
// chunk at index 0 of the session's AUTH feed — authenticates it; a publisher
// stream then sends ordinary single-owner chunks that are updates of its session
// feed on keccak256(topic | challenge), and the admin's publisher stream also its
// service messages: ROSTER, the publisher set, and EOS, the end of the channel.
// The broker keeps a cursor per publisher and per service kind, and delivers
// every accepted frame to every receiving stream not bound to its author.
package bps

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/crypto"
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

var (
	errNotBroker       = errors.New("bps: not a broker")
	errJoinRefused     = errors.New("bps: join refused")
	errInvalidJoin     = errors.New("bps: invalid join")
	errSessionClosed   = errors.New("bps: session closed")
	errProtocolError   = errors.New("bps: protocol error")
	errViolation       = errors.New("bps: protocol violation")
	errWrongStream     = errors.New("bps: frame not what the stream may send")
	errCohortReclaimed = errors.New("bps: cohort reclaimed")
	errFull            = errors.New("bps: capacity bound")
	errNotPublisher    = errors.New("bps: session is not a publisher")
	errInvalidDelivery = errors.New("bps: invalid delivery")
	errEndOfStream     = errors.New("bps: end of stream")
	// ErrEndOfStream is a session's Err after the admin's EOS ended the channel.
	ErrEndOfStream  = errEndOfStream
	errChannelEnded = errors.New("bps: channel ended")
)

// Options are the broker's resource bounds. Zero values take the defaults.
type Options struct {
	MaxSubsPerCohort        int // subscriber streams per cohort
	MaxBrokerCohorts        int // live cohorts per broker
	MaxCohortsPerConnection int // cohorts per peer connection
	MaxPeerStreamsPerCohort int // streams per peer connection per cohort
	MaxPublisherStreams     int // publisher streams per address per cohort
	QueueCap                int // outbound frames per receiving stream
	//
	InactiveTimeout time.Duration // reclaims a cohort with no accepted frame
	AuthWaitTimeout time.Duration // disconnects a pending stream that has not authenticated
	FlushTimeout    time.Duration // bounds writing the queue of a stream being closed
	//
	ProtoBreachBlocklistDuration time.Duration // blocklist duration for a protocol violation
	InvalidAuthBlocklistDuration time.Duration // blocklist duration for an auth timeout
}

// DefaultOptions are the SWIP-74 and SWIP-87 recommended bounds.
var DefaultOptions = Options{
	MaxSubsPerCohort:             1024,
	MaxBrokerCohorts:             512,
	MaxCohortsPerConnection:      16,
	MaxPeerStreamsPerCohort:      2,
	MaxPublisherStreams:          2,
	InactiveTimeout:              10 * time.Minute,
	AuthWaitTimeout:              30 * time.Second,
	FlushTimeout:                 5 * time.Second,
	QueueCap:                     64,
	ProtoBreachBlocklistDuration: 10 * time.Minute,
	InvalidAuthBlocklistDuration: time.Minute,
}

func (o Options) withDefaults() Options {
	d := DefaultOptions
	if o.MaxSubsPerCohort > 0 {
		d.MaxSubsPerCohort = o.MaxSubsPerCohort
	}
	if o.MaxBrokerCohorts > 0 {
		d.MaxBrokerCohorts = o.MaxBrokerCohorts
	}
	if o.MaxCohortsPerConnection > 0 {
		d.MaxCohortsPerConnection = o.MaxCohortsPerConnection
	}
	if o.MaxPeerStreamsPerCohort > 0 {
		d.MaxPeerStreamsPerCohort = o.MaxPeerStreamsPerCohort
	}
	if o.MaxPublisherStreams > 0 {
		d.MaxPublisherStreams = o.MaxPublisherStreams
	}
	if o.InactiveTimeout > 0 {
		d.InactiveTimeout = o.InactiveTimeout
	}
	if o.AuthWaitTimeout > 0 {
		d.AuthWaitTimeout = o.AuthWaitTimeout
	}
	if o.FlushTimeout > 0 {
		d.FlushTimeout = o.FlushTimeout
	}
	if o.QueueCap > 0 {
		d.QueueCap = o.QueueCap
	}
	if o.ProtoBreachBlocklistDuration > 0 {
		d.ProtoBreachBlocklistDuration = o.ProtoBreachBlocklistDuration
	}
	if o.InvalidAuthBlocklistDuration > 0 {
		d.InvalidAuthBlocklistDuration = o.InvalidAuthBlocklistDuration
	}
	return d
}

// Service is the bps protocol service. On a full node it is also the broker.
type Service struct {
	fullNode    bool
	streamer    p2p.Streamer
	blocklister p2p.Blocklister
	opts        Options
	metrics     metrics
	logger      log.Logger

	mtx     sync.Mutex
	cohorts map[string]*cohort        // by canonical spec
	peers   map[string]map[string]int // peer overlay -> cohort key -> streams
	closed  bool
}

// New returns a new bps Service. The blocklister may be nil.
func New(streamer p2p.Streamer, blocklister p2p.Blocklister, fullNode bool, logger log.Logger, o Options) *Service {
	return &Service{
		fullNode:    fullNode,
		streamer:    streamer,
		blocklister: blocklister,
		opts:        o.withDefaults(),
		metrics:     newMetrics(),
		cohorts:     make(map[string]*cohort),
		peers:       make(map[string]map[string]int),
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

// Close reclaims every cohort and resets their streams.
func (s *Service) Close() error {
	s.mtx.Lock()
	s.closed = true
	cohorts := s.cohorts
	s.cohorts = make(map[string]*cohort)
	s.mtx.Unlock()
	for _, co := range cohorts {
		for _, m := range co.shutdown() {
			m.reset()
		}
	}
	return nil
}

// Interface is the bps surface consumed by the API.
type Interface interface {
	// Join joins a cohort at a broker and returns the live session.
	Join(ctx context.Context, req JoinRequest) (Session, error)
}

// Message is a verified delivery — a DATA update of an admitted publisher, or
// one of the admin's ROSTER and EOS — with the frame fields its id was derived
// from and the owner it validated for.
type Message struct {
	Kind      Kind
	Owner     []byte // the publisher; the principal for ROSTER and EOS
	SOC       []byte
	Challenge []byte
	Index     uint64
	Roster    [][]byte // ROSTER only: the publisher set besides the principal
}

// Position is a subscriber's place on a channel, kept across streams and
// sessions: a cursor per publisher, the latest roster with its cursor, and
// whether the channel has ended.
type Position struct {
	Cursors      map[string]uint64 // by the publisher's 20-byte address as a string: the lowest DATA index delivered next
	Roster       [][]byte          // the latest accepted roster
	RosterCursor uint64            // the lowest ROSTER index accepted next
	Ended        bool              // an EOS was accepted: the EOS cursor is past 0
}

func (p Position) clone() Position {
	c := Position{Cursors: make(map[string]uint64, len(p.Cursors)), RosterCursor: p.RosterCursor, Ended: p.Ended}
	for k, v := range p.Cursors {
		c.Cursors[k] = v
	}
	c.Roster = append([][]byte(nil), p.Roster...)
	return c
}

// Session is a single joined p2p stream to a broker.
type Session interface {
	// Challenge is the challenge the broker issued for this stream. Every
	// chunk published on the session must be signed under it (see ID).
	Challenge() []byte
	// Messages yields the deliveries that verify end to end against the cohort
	// spec, the roster and the session's cursors: DATA from the principal or a
	// rostered publisher, and the principal's ROSTER and EOS. The channel is
	// closed when the stream ends; after an EOS, with Err reporting it.
	Messages() <-chan Message
	// Position is the session's place on the channel, to pass to a later Join.
	Position() Position
	// Granted is closed once the session may authenticate: at once for the
	// principal; for any other identity iff it joined knowing it was rostered —
	// the Position it joined with names it — and the first delivery on this
	// stream is a roster naming it: the cue that the broker admitted the stream
	// as pending, which a grantee waits for before its AUTH. A spectator that a
	// later roster names (see Message.Roster) rejoins to publish; a publisher
	// whose identity a roster drops finds its stream closed after that roster.
	Granted() <-chan struct{}
	// Publish writes a chunk of the given kind and index, signed under the
	// session challenge, to the broker. The broker does not reply. Only the
	// principal, or an identity the latest roster names, may publish; only the
	// principal may publish ROSTER and EOS.
	Publish(ctx context.Context, kind Kind, index uint64, soc []byte) error
	// Done is closed when the p2p stream ends.
	Done() <-chan struct{}
	// Err reports why the stream ended. Valid after Done is closed.
	Err() error
	// Close closes the stream.
	Close() error
}

// CohortSpec names a cohort: the feeds on the topic of the principal and of
// whoever its roster names. The topic binding is always FEED_TOPIC.
type CohortSpec struct {
	Topic     []byte // 32 bytes: the feed topic
	Principal []byte // 20 bytes: the admin
}

func (c CohortSpec) proto() *pb.CohortSpec {
	return &pb.CohortSpec{Topic: c.Topic, Binding: pb.TopicBinding_FEED_TOPIC, Principal: c.Principal}
}

// JoinRequest describes a cohort join.
type JoinRequest struct {
	Broker   swarm.Address // the broker
	Spec     CohortSpec    // the cohort to join
	Identity []byte        // 20 bytes: the joining stream's identity
	Position Position      // the subscriber's place on the channel, from an earlier session
}

// Join joins a cohort at the broker. A peer whose join was refused or whose
// stream was reset must back off before rejoining.
func (s *Service) Join(ctx context.Context, req JoinRequest) (Session, error) {
	join := &pb.Join{Cohort: req.Spec.proto(), Identity: req.Identity}
	if err := validateJoin(join); err != nil {
		return nil, err
	}
	// a channel the subscriber has seen end is not rejoined
	if req.Position.Ended {
		return nil, errChannelEnded
	}
	stream, err := s.streamer.NewStream(ctx, req.Broker, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, fmt.Errorf("new stream: %w", err)
	}

	w, r := protobuf.NewWriterAndReader(stream)
	if err := w.WriteMsgWithContext(ctx, join); err != nil {
		_ = stream.Reset()
		return nil, fmt.Errorf("write join: %w", err)
	}
	var ack pb.Ack
	if err := r.ReadMsgWithContext(ctx, &ack); err != nil {
		_ = stream.Reset()
		return nil, fmt.Errorf("read ack: %w", err)
	}
	if ack.Status != pb.Status_OK {
		_ = stream.Reset()
		return nil, fmt.Errorf("join %s: %w", ack.Status, errJoinRefused)
	}
	if len(ack.Challenge) != ChallengeSize {
		_ = stream.Reset()
		return nil, fmt.Errorf("challenge length %d: %w", len(ack.Challenge), errProtocolError)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sess := &session{
		ctx:       ctx,
		cancel:    cancel,
		w:         w,
		challenge: ack.Challenge,
		spec:      req.Spec,
		identity:  req.Identity,
		pos:       req.Position.clone(),
		seat:      contains(req.Position.Roster, req.Identity),
		granted:   make(chan struct{}),
		rx:        make(chan Message),
		done:      make(chan struct{}),
	}
	if bytes.Equal(req.Identity, req.Spec.Principal) {
		sess.grant()
	}

	go sess.readLoop(stream, r, req.Spec)

	return sess, nil
}

// readLoop delivers the broker's frames until the stream ends.
func (s *session) readLoop(stream p2p.Stream, r protobuf.Reader, spec CohortSpec) {
	var readErr error
	defer func() {
		s.finish(readErr)
		if readErr != nil && !errors.Is(readErr, errEndOfStream) && s.ctx.Err() == nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()
	go func() {
		<-s.ctx.Done()
		_ = stream.Reset()
	}()

	for {
		var f pb.Broadcast
		if err := r.ReadMsgWithContext(s.ctx, &f); err != nil {
			readErr = err
			return
		}
		msg, ok, err := s.accept(&f, spec)
		if err != nil {
			// a broker that forwards a chunk not valid for the spec, or by an
			// owner the roster does not admit, is misbehaving: the stream is ended
			readErr = fmt.Errorf("%w: %w", errInvalidDelivery, err)
			return
		}
		if !ok {
			continue
		}
		select {
		case s.rx <- msg:
		case <-s.ctx.Done():
			return
		}
		if msg.Kind == KindEOS {
			readErr = errEndOfStream
			return
		}
	}
}

// accept re-verifies a delivery end to end. It returns false for a frame the
// subscriber drops: an AUTH, a kind it does not know, one below its cursor, a
// roster that does not decode. The challenge is not compared: it is the
// publisher stream's, which the subscriber never sees.
func (s *session) accept(f *pb.Broadcast, spec CohortSpec) (Message, bool, error) {
	msg := Message{SOC: f.Soc, Challenge: f.Challenge, Index: f.Index}
	first := !s.started
	s.started = true
	switch f.Kind {
	case pb.Kind_DATA:
		owner, err := recoverOwner(f, spec.Topic)
		if err != nil {
			return msg, false, err
		}
		s.mtx.Lock()
		defer s.mtx.Unlock()
		if !s.admits(owner) {
			return msg, false, fmt.Errorf("owner %x not rostered", owner)
		}
		k := string(owner)
		if f.Index < s.pos.Cursors[k] {
			return msg, false, nil
		}
		s.pos.Cursors[k] = f.Index + 1
		msg.Kind, msg.Owner = KindData, owner
		return msg, true, nil

	case pb.Kind_ROSTER:
		// the owner of a service chunk is forced to the principal
		if err := verify(f, spec.Topic, spec.Principal); err != nil {
			return msg, false, err
		}
		roster, err := decodeRoster(f.Soc)
		if err != nil {
			return msg, false, nil
		}
		s.mtx.Lock()
		defer s.mtx.Unlock()
		if f.Index >= s.pos.RosterCursor {
			s.pos.RosterCursor = f.Index + 1
			s.pos.Roster = roster
			msg.Kind, msg.Owner, msg.Roster = KindRoster, spec.Principal, roster
		}
		// the cue: the stream's first delivery is a roster naming the
		// identity — the broker admitted it as pending by that roster
		if first && s.seat && contains(roster, s.identity) && s.admits(s.identity) {
			s.grant()
		}
		return msg, msg.Kind == KindRoster, nil

	case pb.Kind_EOS:
		if err := verify(f, spec.Topic, spec.Principal); err != nil {
			return msg, false, err
		}
		s.mtx.Lock()
		s.pos.Ended = true
		s.mtx.Unlock()
		msg.Kind, msg.Owner = KindEOS, spec.Principal
		return msg, true, nil
	}
	return msg, false, nil
}

// admits reports whether owner may publish: the principal, or an address on the
// latest roster. Must be called with s.mtx held.
func (s *session) admits(owner []byte) bool {
	return bytes.Equal(owner, s.spec.Principal) || contains(s.pos.Roster, owner)
}

func contains(set [][]byte, a []byte) bool {
	for _, b := range set {
		if bytes.Equal(a, b) {
			return true
		}
	}
	return false
}

// session is the concrete Session implementation.
type session struct {
	ctx         context.Context
	cancel      context.CancelFunc
	wmtx        sync.Mutex
	w           protobuf.Writer
	challenge   []byte
	spec        CohortSpec
	identity    []byte
	started     bool // a frame was read; owned by readLoop
	seat        bool // joined knowing the identity rostered
	mtx         sync.Mutex
	pos         Position
	granted     chan struct{}
	grantedOnce sync.Once
	rx          chan Message
	done        chan struct{}
	err         error
	once        sync.Once
}

func (s *session) Challenge() []byte        { return s.challenge }
func (s *session) Messages() <-chan Message { return s.rx }
func (s *session) Granted() <-chan struct{} { return s.granted }
func (s *session) Done() <-chan struct{}    { return s.done }
func (s *session) Err() error               { return s.err }
func (s *session) Close() error {
	s.cancel()
	return nil
}

func (s *session) Position() Position {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.pos.clone()
}

func (s *session) grant() { s.grantedOnce.Do(func() { close(s.granted) }) }

func (s *session) Publish(ctx context.Context, kind Kind, index uint64, soc []byte) error {
	principal := bytes.Equal(s.identity, s.spec.Principal)
	select {
	case <-s.granted:
	default:
		return errNotPublisher
	}
	s.mtx.Lock()
	seated := s.admits(s.identity)
	s.mtx.Unlock()
	if !seated || !principal && (kind == KindRoster || kind == KindEOS) {
		return errNotPublisher
	}
	select {
	case <-s.done:
		return errSessionClosed
	default:
	}
	s.wmtx.Lock()
	defer s.wmtx.Unlock()
	f := pb.Broadcast{Soc: soc, Kind: kind.proto(), Challenge: s.challenge, Index: index}
	if err := s.w.WriteMsgWithContext(ctx, &f); err != nil {
		return fmt.Errorf("write broadcast: %w", err)
	}
	// the admin admits by the roster it publishes, which is not delivered back
	if kind == KindRoster {
		if roster, err := decodeRoster(soc); err == nil {
			s.mtx.Lock()
			if index >= s.pos.RosterCursor {
				s.pos.RosterCursor = index + 1
				s.pos.Roster = roster
			}
			s.mtx.Unlock()
		}
	}
	return nil
}

// finish records the terminal error and signals stream completion exactly once.
func (s *session) finish(err error) {
	s.once.Do(func() {
		s.err = err
		close(s.rx)
		close(s.done)
	})
}

// validateJoin rejects a Join whose spec has a value outside SWIP-74 or whose
// identity is not 20 bytes.
func validateJoin(join *pb.Join) error {
	cohortSpec := join.Cohort
	switch {
	case cohortSpec == nil:
		return fmt.Errorf("no cohort: %w", errInvalidJoin)
	case len(cohortSpec.Topic) != swarm.HashSize:
		return fmt.Errorf("topic length %d: %w", len(cohortSpec.Topic), errInvalidJoin)
	case cohortSpec.Binding != pb.TopicBinding_FEED_TOPIC:
		return fmt.Errorf("binding %s: %w", cohortSpec.Binding, errInvalidJoin)
	case len(cohortSpec.Principal) != crypto.AddressSize:
		return fmt.Errorf("principal length %d: %w", len(cohortSpec.Principal), errInvalidJoin)
	case len(join.Identity) != crypto.AddressSize:
		return fmt.Errorf("identity length %d: %w", len(join.Identity), errInvalidJoin)
	}
	return nil
}

// handler is the protocol handler on the broker.
func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	if !s.fullNode {
		_ = stream.Reset()
		return errNotBroker
	}
	w, r := protobuf.NewWriterAndReader(stream)

	var join pb.Join
	if err := r.ReadMsgWithContext(ctx, &join); err != nil {
		_ = stream.Reset()
		return fmt.Errorf("read join: %w", err)
	}
	if err := validateJoin(&join); err != nil {
		return s.refuse(ctx, w, stream, err)
	}
	// re-marshal so that the cohort key ignores fields BPS-lite does not define
	join.Cohort = &pb.CohortSpec{Topic: join.Cohort.Topic, Binding: join.Cohort.Binding, Principal: join.Cohort.Principal}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	co, m, err := s.join(p.Address, &join, func() {
		cancel()
		_ = stream.Reset()
	}, func() {
		// closing, not resetting, lets the frames already written arrive
		_ = stream.Close()
		cancel()
	})
	if err != nil {
		return s.refuse(ctx, w, stream, err)
	}
	// detach on every exit, normal or not
	defer s.leave(p.Address, co, m)

	if err := w.WriteMsgWithContext(ctx, &pb.Ack{Status: pb.Status_OK, Challenge: m.challenge}); err != nil {
		m.reset()
		return fmt.Errorf("write ack: %w", err)
	}

	// the writer side: deliveries to a receiving stream, and the roster a
	// pending stream is named in
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case f := <-m.queue:
				if err := w.WriteMsgWithContext(ctx, f); err != nil {
					m.reset()
					return
				}
			case <-m.ended:
				// an EOS or a revocation: write what is queued — the EOS or the
				// reduced roster last — then close; a peer that does not read
				// within the flush deadline is reset
				flushCtx, flushCancel := context.WithTimeout(ctx, s.opts.FlushTimeout)
				defer flushCancel()
				for {
					select {
					case f := <-m.queue:
						if err := w.WriteMsgWithContext(flushCtx, f); err != nil {
							m.reset()
							return
						}
					default:
						m.close()
						return
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	defer func() { <-writerDone }()

	// the reader side: publications and authentications
	for {
		var f pb.Broadcast
		if err := r.ReadMsgWithContext(ctx, &f); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			m.reset()
			return nil
		}
		if err := s.handleFrame(co, m, &f); err != nil {
			if errors.Is(err, errViolation) && s.blocklister != nil {
				if berr := s.blocklister.Blocklist(p.Address, s.opts.ProtoBreachBlocklistDuration, err.Error()); berr != nil {
					s.logger.Debug("blocklist failed", "peer_address", p.Address, "error", berr)
				}
			}
			m.reset()
			return err
		}
	}
}

// refuse answers a Join with a non-OK Ack and ends the stream. The status
// follows from the cause — FULL for a capacity bound, REJECTED for anything
// else — so that no caller can answer a refusal with OK.
func (s *Service) refuse(ctx context.Context, w protobuf.Writer, stream p2p.Stream, cause error) error {
	status := pb.Status_REJECTED
	if errors.Is(cause, errFull) {
		status = pb.Status_FULL
	}
	if err := w.WriteMsgWithContext(ctx, &pb.Ack{Status: status}); err != nil {
		_ = stream.Reset()
		return fmt.Errorf("write ack: %w", err)
	}
	_ = stream.FullClose()
	s.logger.Debug("join refused", "status", status, "error", cause)
	return nil
}

// handleFrame validates a Broadcast arriving at the broker per SWIP-74 and
// SWIP-87 and accepts, drops or punishes it. A returned error resets the stream.
func (s *Service) handleFrame(co *cohort, m *member, f *pb.Broadcast) error {
	principal := co.spec.Principal
	admin := bytes.Equal(m.identity, principal)

	co.mtx.Lock()
	role := m.role
	var cursor uint64
	switch f.Kind {
	case pb.Kind_DATA:
		cursor = co.cursors[string(m.identity)]
	case pb.Kind_ROSTER:
		cursor = co.rosterCursor
	}
	co.mtx.Unlock()

	// 0. a revoked stream is being closed: what it still sends was sent before
	// it read the roster that revoked it, and is dropped unread
	if role == roleRevoked {
		s.count(&co.counters.revoked, "revoked")
		return nil
	}
	// 1. a publisher stream sends publications and heartbeats — the admin's also
	// its service messages — a pending stream its AUTH; anything else is not
	// what the stream may send
	if !mayEmit(role, admin, f.Kind) {
		s.count(&co.counters.wrongStream, "wrong_stream")
		return fmt.Errorf("%w: %w", errViolation, errWrongStream)
	}
	// 2. a kind this SWIP does not define is dropped
	switch f.Kind {
	case pb.Kind_DATA, pb.Kind_AUTH, pb.Kind_EOS, pb.Kind_ROSTER:
	default:
		s.count(&co.counters.unknownKind, "unknown_kind")
		return nil
	}
	// 3. the challenge must be the stream's
	if !bytes.Equal(f.Challenge, m.challenge) {
		s.count(&co.counters.wrongChallenge, "wrong_challenge")
		return nil
	}
	// 4. a DATA or ROSTER frame below its feed's cursor is a retransmit
	if (f.Kind == pb.Kind_DATA || f.Kind == pb.Kind_ROSTER) && f.Index < cursor {
		s.count(&co.counters.retransmit, "retransmit")
		return nil
	}
	// 5. the chunk must be its owner's at the id derived from the frame: the
	// stream's identity for DATA and AUTH, the principal for the service kinds;
	// an AUTH or EOS is moreover the empty chunk at index 0
	owner := m.identity
	if service(f.Kind) {
		owner = principal
	}
	if err := verify(f, co.spec.Topic, owner); err != nil {
		s.count(&co.counters.invalidSOC, "invalid_soc")
		return fmt.Errorf("%w: %w", errViolation, err)
	}
	// 6. a ROSTER must decode; one that does not changes nothing
	var roster [][]byte
	if f.Kind == pb.Kind_ROSTER {
		var err error
		if roster, err = decodeRoster(f.Soc); err != nil {
			s.count(&co.counters.invalidRoster, "invalid_roster")
			return nil
		}
	}

	var resets, evicted, closes []*member
	co.mtx.Lock()
	// a roster revoking the identity was accepted since the checks
	if m.role == roleRevoked {
		co.mtx.Unlock()
		s.count(&co.counters.revoked, "revoked")
		return nil
	}
	switch f.Kind {
	case pb.Kind_AUTH:
		// the auth of a pending stream, a heartbeat on a publisher stream
		if m.role != rolePublisher {
			resets, evicted = co.upgrade(m, s.opts.MaxPublisherStreams)
		}
		co.inactiveSince = time.Now()

	case pb.Kind_DATA:
		k := string(m.identity)
		if f.Index < co.cursors[k] {
			// raced with another publisher stream of the same identity
			s.count(&co.counters.retransmit, "retransmit")
			break
		}
		co.cursors[k] = f.Index + 1
		co.inactiveSince = time.Now()
		s.metrics.Delivered.Inc()
		resets = co.deliver(f, m)

	case pb.Kind_ROSTER:
		if f.Index < co.rosterCursor {
			s.count(&co.counters.retransmit, "retransmit")
			break
		}
		co.rosterCursor = f.Index + 1
		co.inactiveSince = time.Now()
		resets, closes = co.applyRoster(f, roster, m)

	case pb.Kind_EOS:
		resets = co.deliver(f, m)
		closes = co.end()
	}
	co.mtx.Unlock()

	// the resets of the overflowed streams, outside the lock (see deliver)
	for _, r := range resets {
		s.count(&co.counters.queueReset, "queue_reset")
		r.reset()
	}
	for _, r := range evicted {
		r.reset()
	}
	if f.Kind == pb.Kind_EOS {
		s.endCohort(co, closes)
		return nil
	}
	// the revoked streams are closed once the roster is written to them
	for _, r := range closes {
		r.end()
	}
	return nil
}

// mayEmit reports whether a stream of the role may send a frame of the kind
// (step 1). Kinds the broker does not define are left to step 2.
func mayEmit(r role, admin bool, kind pb.Kind) bool {
	switch r {
	case rolePublisher:
		return !service(kind) || admin
	case rolePending:
		return kind == pb.Kind_AUTH
	}
	return false
}

// endCohort forgets a cohort ended by its admin's EOS and closes its streams
// once their queues — the EOS last — are written.
func (s *Service) endCohort(co *cohort, ms []*member) {
	s.mtx.Lock()
	if s.cohorts[co.key] == co {
		co.mtx.Lock()
		s.removeCohort(co)
		co.mtx.Unlock()
	}
	s.mtx.Unlock()
	for _, m := range ms {
		m.end()
	}
}

// join attaches a stream to the cohort named by the spec, creating the
// cohort if no live one has it. An error wrapping errFull means a capacity
// bound was hit. reset ends the stream abruptly, close after its queue.
func (s *Service) join(peer swarm.Address, join *pb.Join, reset, closeFn func()) (*cohort, *member, error) {
	key, err := join.Cohort.Marshal()
	if err != nil {
		return nil, nil, fmt.Errorf("marshal cohort spec: %w", err)
	}
	k := string(key)

	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.closed {
		return nil, nil, errSessionClosed
	}

	pc := s.peers[peer.ByteString()]
	if pc[k] == 0 && len(pc) >= s.opts.MaxCohortsPerConnection {
		return nil, nil, fmt.Errorf("too many cohorts per peer: %w", errFull)
	}
	if pc[k] >= s.opts.MaxPeerStreamsPerCohort {
		return nil, nil, fmt.Errorf("too many streams per peer per cohort: %w", errFull)
	}

	co, ok := s.cohorts[k]
	if !ok {
		if len(s.cohorts) >= s.opts.MaxBrokerCohorts {
			return nil, nil, fmt.Errorf("too many cohorts per broker: %w", errFull)
		}
		co = &cohort{
			key:           k,
			spec:          join.Cohort,
			members:       make(map[*member]struct{}),
			cursors:       make(map[string]uint64),
			inactiveSince: time.Now(),
		}
		co.inactivity = time.AfterFunc(s.opts.InactiveTimeout, func() { s.reclaim(co) })
		s.cohorts[k] = co
		s.metrics.Cohorts.Set(float64(len(s.cohorts)))
	}

	co.mtx.Lock()
	defer co.mtx.Unlock()

	// the admin's or a rostered identity is pending, outside the fan-out
	// bound; any other is a spectator, within it
	role := roleSpectator
	if bytes.Equal(join.Identity, join.Cohort.Principal) || co.rostered(join.Identity) {
		role = rolePending
	}
	if role == roleSpectator && co.spectators >= s.opts.MaxSubsPerCohort {
		if len(co.members) == 0 {
			s.removeCohort(co)
		}
		return nil, nil, fmt.Errorf("too many subscribers per cohort: %w", errFull)
	}

	challenge := make([]byte, ChallengeSize)
	if _, err := rand.Read(challenge); err != nil {
		if len(co.members) == 0 {
			s.removeCohort(co)
		}
		return nil, nil, fmt.Errorf("challenge: %w", err)
	}
	m := &member{
		role:      role,
		identity:  join.Identity,
		challenge: challenge,
		queue:     make(chan *pb.Broadcast, s.opts.QueueCap),
		ended:     make(chan struct{}),
	}
	var once, endOnce sync.Once
	m.reset = func() { once.Do(reset) }
	m.close = func() { once.Do(closeFn) }
	m.end = func() { endOnce.Do(func() { close(m.ended) }) }
	if role == rolePending {
		m.authTimer = time.AfterFunc(s.opts.AuthWaitTimeout, func() { s.authTimeout(peer, co, m) })
		// a rostered identity receives the roster that names it at once: its cue
		if co.rosterFrame != nil && !bytes.Equal(join.Identity, join.Cohort.Principal) {
			m.queue <- co.rosterFrame
			m.cue = co.rosterFrame
		}
	} else {
		co.spectators++
		// the latest roster is the first delivery on a receiving stream
		if co.rosterFrame != nil {
			m.queue <- co.rosterFrame
		}
	}
	co.members[m] = struct{}{}

	if pc == nil {
		pc = make(map[string]int)
		s.peers[peer.ByteString()] = pc
	}
	pc[k]++
	return co, m, nil
}

// leave detaches a stream. A cohort with no attached streams is reclaimed at once.
func (s *Service) leave(peer swarm.Address, co *cohort, m *member) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if pc := s.peers[peer.ByteString()]; pc != nil {
		if pc[co.key]--; pc[co.key] <= 0 {
			delete(pc, co.key)
		}
		if len(pc) == 0 {
			delete(s.peers, peer.ByteString())
		}
	}

	co.mtx.Lock()
	defer co.mtx.Unlock()
	if _, ok := co.members[m]; !ok {
		return
	}
	delete(co.members, m)
	if m.authTimer != nil {
		m.authTimer.Stop()
	}
	if m.role == roleSpectator {
		co.spectators--
	}
	if len(co.members) == 0 && s.cohorts[co.key] == co {
		s.removeCohort(co)
	}
}

// removeCohort forgets the cohort. Must be called with s.mtx and co.mtx held.
func (s *Service) removeCohort(co *cohort) {
	delete(s.cohorts, co.key)
	co.inactivity.Stop()
	s.metrics.Cohorts.Set(float64(len(s.cohorts)))
}

// reclaim forgets a cohort on which no frame was accepted for the inactivity
// deadline, and resets every stream in it.
func (s *Service) reclaim(co *cohort) {
	s.mtx.Lock()
	if s.cohorts[co.key] != co {
		s.mtx.Unlock()
		return
	}
	co.mtx.Lock()
	if idle := time.Since(co.inactiveSince); idle < s.opts.InactiveTimeout {
		co.inactivity.Reset(s.opts.InactiveTimeout - idle)
		co.mtx.Unlock()
		s.mtx.Unlock()
		return
	}
	s.removeCohort(co)
	co.mtx.Unlock()
	s.mtx.Unlock()

	s.logger.Debug("cohort reclaimed", "error", errCohortReclaimed)
	for _, m := range co.shutdown() {
		m.reset()
	}
}

// authTimeout disconnects a pending stream that has not authenticated in time.
func (s *Service) authTimeout(peer swarm.Address, co *cohort, m *member) {
	co.mtx.Lock()
	pending := m.role == rolePending
	co.mtx.Unlock()
	if !pending {
		return
	}
	s.count(&co.counters.authTimeout, "auth_timeout")
	if s.blocklister != nil {
		if err := s.blocklister.Blocklist(peer, s.opts.InvalidAuthBlocklistDuration, "bps auth timeout"); err != nil {
			s.logger.Debug("blocklist failed", "peer_address", peer, "error", err)
		}
	}
	m.reset()
}

// Counters returns the counters of the live cohort named by the spec.
func (s *Service) Counters(spec CohortSpec) (Counters, bool) {
	key, err := spec.proto().Marshal()
	if err != nil {
		return Counters{}, false
	}
	s.mtx.Lock()
	co, ok := s.cohorts[string(key)]
	s.mtx.Unlock()
	if !ok {
		return Counters{}, false
	}
	return co.counters.snapshot(), true
}

type role int

const (
	roleSpectator role = iota + 1 // receives, sends nothing; a grantee rejoins to publish
	rolePending                   // declared the principal or a rostered address, no AUTH yet
	rolePublisher                 // authenticated by its AUTH: publishes, and receives what others publish
	roleRevoked                   // a pending or publisher stream a roster dropped: being closed, what it sends dropped
)

type cohort struct {
	key        string
	spec       *pb.CohortSpec
	inactivity *time.Timer
	counters   counters

	mtx           sync.Mutex
	cursors       map[string]uint64 // by publisher: the lowest DATA index accepted next
	roster        [][]byte          // the latest accepted roster, the principal excluded
	rosterCursor  uint64            // the lowest ROSTER index accepted next
	rosterFrame   *pb.Broadcast     // the latest accepted ROSTER frame: the first delivery
	inactiveSince time.Time
	members       map[*member]struct{}
	spectators    int    // the fan-out set
	authSeq       uint64 // orders publisher streams by their auth
}

// rostered reports whether the latest roster names the address. Must be
// called with co.mtx held.
func (co *cohort) rostered(a []byte) bool {
	return contains(co.roster, a)
}

// receives reports whether deliveries are enqueued on a stream of the role.
func (r role) receives() bool { return r == roleSpectator || r == rolePublisher }

// deliver enqueues an accepted frame on every receiving stream but the
// sender's — for DATA, but any stream bound to the sender's identity — and
// returns the streams whose queue was full. The admin's other streams receive
// its service messages: a roster is what every receiver admits by. The send is
// non-blocking, so that one slow receiver cannot stall the cohort; a stream
// whose queue is full is returned and reset by the caller after co.mtx is
// released — reset ends the stream, whose handler then calls leave, which
// takes s.mtx and co.mtx, so resetting under the lock would deadlock. Must be
// called with co.mtx held.
func (co *cohort) deliver(f *pb.Broadcast, sender *member) []*member {
	var overflow []*member
	for m := range co.members {
		if !m.role.receives() || m == sender || f.Kind == pb.Kind_DATA && bytes.Equal(m.identity, sender.identity) {
			continue
		}
		if !m.enqueue(f) {
			overflow = append(overflow, m)
		}
	}
	return overflow
}

// upgrade makes a pending stream a publisher stream, and returns the streams
// to reset: one whose queue was full, and the oldest publisher streams of the
// identity beyond the bound. Must be called with co.mtx held.
func (co *cohort) upgrade(m *member, maxStreams int) (resets, evicted []*member) {
	m.authTimer.Stop()
	// a stream that starts receiving gets the latest roster first, unless that
	// is the roster it was given as its cue
	if co.rosterFrame != nil && m.cue != co.rosterFrame && !m.enqueue(co.rosterFrame) {
		resets = append(resets, m)
	}
	m.role = rolePublisher
	co.authSeq++
	m.authSeq = co.authSeq

	var same []*member
	for o := range co.members {
		if o.role == rolePublisher && bytes.Equal(o.identity, m.identity) {
			same = append(same, o)
		}
	}
	for len(same) > maxStreams {
		oldest := 0
		for i, o := range same {
			if o.authSeq < same[oldest].authSeq {
				oldest = i
			}
		}
		evicted = append(evicted, same[oldest])
		same = append(same[:oldest], same[oldest+1:]...)
	}
	return resets, evicted
}

// applyRoster makes an accepted roster the cohort's: it is delivered, enqueued
// at once on every pending stream it names, and every stream of an identity it
// drops is revoked — written the roster, then closed, what it sends meanwhile
// dropped; the cursors of dropped identities are forgotten. It returns the
// streams to reset for a full queue and the revoked streams to close. Must be
// called with co.mtx held.
func (co *cohort) applyRoster(f *pb.Broadcast, roster [][]byte, sender *member) (resets, closes []*member) {
	principal := co.spec.Principal
	co.roster = roster
	co.rosterFrame = f
	resets = co.deliver(f, sender)
	for m := range co.members {
		if bytes.Equal(m.identity, principal) {
			continue
		}
		switch {
		case m.role == rolePending && co.rostered(m.identity):
			// the cue for a pending stream it names
			m.cue = f
			if !m.enqueue(f) {
				resets = append(resets, m)
			}
		case m.role == rolePending:
			// revoked before its auth: told, then closed
			m.authTimer.Stop()
			m.role = roleRevoked
			if !m.enqueue(f) {
				resets = append(resets, m)
				continue
			}
			closes = append(closes, m)
		case m.role == rolePublisher && !co.rostered(m.identity):
			// revoked: it was delivered this roster above, as a receiving
			// stream; closed once that is written
			m.role = roleRevoked
			closes = append(closes, m)
		}
	}
	for k := range co.cursors {
		if a := []byte(k); !bytes.Equal(a, principal) && !co.rostered(a) {
			delete(co.cursors, k)
		}
	}
	return resets, closes
}

// end returns the cohort's streams for an EOS to close. Must be called with
// co.mtx held.
func (co *cohort) end() []*member {
	ms := make([]*member, 0, len(co.members))
	for m := range co.members {
		ms = append(ms, m)
	}
	return ms
}

// shutdown returns the cohort's streams, for the caller to reset.
func (co *cohort) shutdown() []*member {
	co.mtx.Lock()
	defer co.mtx.Unlock()
	return co.end()
}

// member is one stream attached to a cohort. role and authSeq are guarded by
// cohort.mtx.
type member struct {
	role      role
	identity  []byte
	challenge []byte
	queue     chan *pb.Broadcast
	ended     chan struct{} // closed by an EOS: write the queue, then close
	reset     func()
	close     func()
	end       func()
	authTimer *time.Timer
	authSeq   uint64
	cue       *pb.Broadcast // the roster last enqueued on a pending stream
}

// enqueue adds a frame to the stream's outbound queue, and reports false if
// the queue is full.
func (m *member) enqueue(f *pb.Broadcast) bool {
	select {
	case m.queue <- f:
		return true
	default:
		return false
	}
}
