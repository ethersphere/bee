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

// ForgingBrokerBlocklistDuration is how long a subscriber avoids a broker that
// forwarded a delivery it should have refused — one that does not validate, or
// whose owner the roster does not admit — which no honest broker sends.
const ForgingBrokerBlocklistDuration = 10 * 24 * time.Hour

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

// Service is the bps protocol service. When broker is set it also serves
// cohorts to inbound joins; otherwise it only joins remote brokers.
type Service struct {
	broker      bool
	streamer    p2p.Streamer
	blocklister p2p.Blocklister
	opts        Options
	metrics     metrics
	logger      log.Logger

	quit chan struct{} // closed by Close

	mtx     sync.Mutex
	cohorts map[string]*cohort        // by canonical spec
	peers   map[string]map[string]int // peer overlay -> cohort key -> streams
}

// New returns a new bps Service. The blocklister may be nil.
func New(streamer p2p.Streamer, blocklister p2p.Blocklister, broker bool, logger log.Logger, o Options) *Service {
	return &Service{
		broker:      broker,
		streamer:    streamer,
		blocklister: blocklister,
		opts:        o.withDefaults(),
		metrics:     newMetrics(),
		quit:        make(chan struct{}),
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

// Close refuses every later join, forgets every cohort and asks every stream
// in them to reset. A second call does nothing.
func (s *Service) Close() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	select {
	case <-s.quit:
		return nil
	default:
	}
	close(s.quit)
	for _, co := range s.cohorts {
		co.mtx.Lock()
		s.removeCohort(co)
		for _, m := range co.members() {
			m.requestReset()
		}
		co.mtx.Unlock()
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
	// principal, and for any other identity once a roster naming it has been
	// delivered on this stream — the cue a grantee waits for before its AUTH,
	// which upgrades the stream in place. A publisher whose identity a roster
	// drops finds its stream closed after that roster.
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

	sess := newSession(s, req, w, ack.Challenge)
	go sess.readLoop(stream, r)

	return sess, nil
}

// readLoop runs the session until its stream ends, and is the one place where
// the session ends: as it returns, it alone sets the reason, closes Messages
// and Done and ends the stream — closed after an EOS or a Close, reset on
// anything else. Close only asks, by cancelling the context, which stops a
// read or a delivery in progress.
func (s *session) readLoop(stream p2p.Stream, r protobuf.Reader) {
	defer s.cancel()
	err := s.read(r)
	s.err = err
	close(s.rx)
	close(s.done)
	if err != nil && !errors.Is(err, errEndOfStream) && s.ctx.Err() == nil {
		_ = stream.Reset()
		return
	}
	_ = stream.FullClose()
}

// read delivers the broker's frames until the stream fails, an EOS arrives, a
// delivery does not verify or the session is closed, and returns why.
func (s *session) read(r protobuf.Reader) error {
	for {
		var f pb.Broadcast
		if err := r.ReadMsgWithContext(s.ctx, &f); err != nil {
			return err
		}
		msg, ok, err := s.accept(&f, s.spec)
		if err != nil {
			// a broker that forwards a chunk not valid for the spec, or by an
			// owner the roster does not admit, is misbehaving: the stream is ended
			err = fmt.Errorf("%w: %w", errInvalidDelivery, err)
			s.forged(err)
			return err
		}
		if !ok {
			continue
		}
		select {
		case s.rx <- msg:
		case <-s.ctx.Done():
			return nil
		}
		if msg.Kind == KindEOS {
			return errEndOfStream
		}
	}
}

// accept re-verifies a delivery end to end. It returns false for a frame the
// subscriber drops: an AUTH, a kind it does not know, one below its cursor, a
// roster that does not decode. The challenge is not compared: it is the
// publisher stream's, which the subscriber never sees.
func (s *session) accept(f *pb.Broadcast, spec CohortSpec) (Message, bool, error) {
	msg := Message{SOC: f.Soc, Challenge: f.Challenge, Index: f.Index}
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
		roster, ok := rosterOf(f.Soc)
		if !ok {
			return msg, false, nil
		}
		s.mtx.Lock()
		defer s.mtx.Unlock()
		if f.Index >= s.pos.RosterCursor {
			s.pos.RosterCursor = f.Index + 1
			s.pos.Roster = roster
			msg.Kind, msg.Owner, msg.Roster = KindRoster, spec.Principal, roster
		}
		// the cue: a roster naming the identity, delivered on this stream,
		// while the latest roster still names it — the broker holds it too
		if contains(roster, s.identity) && s.admits(s.identity) {
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

// session is the concrete Session implementation. Its read loop owns its end
// (see readLoop).
type session struct {
	svc       *Service
	broker    swarm.Address
	ctx       context.Context
	cancel    context.CancelFunc // asks the read loop to stop
	wmtx      sync.Mutex
	w         protobuf.Writer
	challenge []byte
	spec      CohortSpec
	identity  []byte
	mtx       sync.Mutex
	pos       Position
	granted   chan struct{} // closed at the cue, by the read loop; at once for the principal
	rx        chan Message  // closed by the read loop as it returns
	done      chan struct{} // closed by the read loop as it returns
	err       error         // set by the read loop before done is closed
}

// newSession returns the session of a stream joined at the broker; the
// principal's may authenticate at once.
func newSession(svc *Service, req JoinRequest, w protobuf.Writer, challenge []byte) *session {
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{
		svc:       svc,
		broker:    req.Broker,
		ctx:       ctx,
		cancel:    cancel,
		w:         w,
		challenge: challenge,
		spec:      req.Spec,
		identity:  req.Identity,
		pos:       req.Position.clone(),
		granted:   make(chan struct{}),
		rx:        make(chan Message),
		done:      make(chan struct{}),
	}
	if bytes.Equal(req.Identity, req.Spec.Principal) {
		close(s.granted)
	}
	return s
}

func (s *session) Challenge() []byte        { return s.challenge }
func (s *session) Messages() <-chan Message { return s.rx }
func (s *session) Granted() <-chan struct{} { return s.granted }
func (s *session) Done() <-chan struct{}    { return s.done }
func (s *session) Err() error               { return s.err }

// Close asks the read loop to end the session. It never blocks, and may be
// called any number of times, from any goroutine.
func (s *session) Close() error {
	s.cancel()
	return nil
}

func (s *session) Position() Position {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.pos.clone()
}

// grant closes granted, the cue to authenticate, unless it is closed already.
// Only the read loop calls it, so that the check and the close cannot race.
func (s *session) grant() {
	select {
	case <-s.granted:
	default:
		close(s.granted)
	}
}

// forged avoids, for long, a broker that forwarded a delivery it should have
// refused.
func (s *session) forged(err error) {
	if s.svc.blocklister == nil {
		return
	}
	if berr := s.svc.blocklister.Blocklist(s.broker, ForgingBrokerBlocklistDuration, err.Error()); berr != nil {
		s.svc.logger.Debug("blocklist broker failed", "broker", s.broker, "error", berr)
	}
}

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
	// a roster or an EOS the admin publishes is delivered back to it, and it
	// admits by it from where it lands in its stream
	return nil
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
	if !s.broker {
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

	co, m, leave, err := s.join(p.Address, &join)
	if err != nil {
		return s.refuse(ctx, w, stream, err)
	}
	// detach on every exit, normal or not
	defer leave()

	if err := w.WriteMsgWithContext(ctx, &pb.Ack{Status: pb.Status_OK, Challenge: m.challenge}); err != nil {
		_ = stream.Reset()
		return fmt.Errorf("write ack: %w", err)
	}
	return s.serve(ctx, p.Address, stream, w, r, co, m)
}

// serve runs the two halves of an attached stream — the reader, handling the
// peer's frames, and the writer, delivering the stream's queue — and is the
// one place where the stream ends: it alone closes or resets it, on its own
// deadlines and on the requests anyone else makes of the member. It returns
// once both halves have stopped.
func (s *Service) serve(ctx context.Context, peer swarm.Address, stream p2p.Stream, w protobuf.Writer, r protobuf.Reader, co *cohort, m *member) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// each half closes its done channel when it stops, the reason left in its
	// err; flush is closed here, once: write what is queued, then stop
	flush := make(chan struct{})
	readDone, writeDone := make(chan struct{}), make(chan struct{})
	var readErr, writeErr error
	go func() { defer close(readDone); readErr = s.read(ctx, r, co, m) }()
	go func() { defer close(writeDone); writeErr = write(ctx, w, m.queue, flush) }()
	readC := readDone // nil once seen, so that the closed channel is not seen again

	// the auth deadline of a stream admitted pending
	var authC <-chan time.Time
	if m.authDeadline {
		t := time.NewTimer(s.opts.AuthWaitTimeout)
		defer t.Stop()
		authC = t.C
	}
	// the flush deadline, from the moment the stream is asked to close
	var flushTimer *time.Timer
	var flushC <-chan time.Time
	defer func() {
		if flushTimer != nil {
			flushTimer.Stop()
		}
	}()

	// end closes or resets the stream and waits for both halves to stop
	end := func(graceful bool) {
		if graceful {
			// closing, not resetting, lets the frames already written arrive
			_ = stream.Close()
		} else {
			_ = stream.Reset()
		}
		cancel()
		<-readDone
		<-writeDone
	}

	for {
		// a reset asked for takes precedence over everything else
		select {
		case <-m.resetReq:
			end(false)
			return nil
		default:
		}
		select {
		case <-m.resetReq:
			end(false)
			return nil

		case <-m.closeReq:
			// an EOS or a revocation: write the queue — the EOS or the reduced
			// roster last — then close, within the flush deadline
			if flushC == nil {
				close(flush)
				flushTimer = time.NewTimer(s.opts.FlushTimeout)
				flushC = flushTimer.C
			}

		case <-flushC:
			// the peer did not take its queue in time, the write in progress
			// included: the timer is not the writer's, so a stuck write cannot
			// hold it off
			end(false)
			return nil

		case <-writeDone:
			// the queue written after a close, or the writer failed
			end(flushC != nil && writeErr == nil)
			return nil

		case <-readC:
			readC = nil
			if flushC != nil {
				// closing: nothing more is read anyway, the flush decides
				continue
			}
			violation := errors.Is(readErr, errViolation)
			if violation && s.blocklister != nil {
				if berr := s.blocklister.Blocklist(peer, s.opts.ProtoBreachBlocklistDuration, readErr.Error()); berr != nil {
					s.logger.Debug("blocklist failed", "peer_address", peer, "error", berr)
				}
			}
			// a violation, or the peer went away
			end(false)
			if violation {
				return readErr
			}
			return nil

		case <-authC:
			authC = nil
			if s.authExpired(peer, co, m) {
				end(false)
				return nil
			}

		case <-ctx.Done():
			end(false)
			return nil
		}
	}
}

// read handles the peer's frames until the stream ends, and returns why: a
// violation, the peer going away, or ctx ending.
func (s *Service) read(ctx context.Context, r protobuf.Reader, co *cohort, m *member) error {
	for {
		var f pb.Broadcast
		if err := r.ReadMsgWithContext(ctx, &f); err != nil {
			return err
		}
		if err := s.handleFrame(co, m, &f); err != nil {
			return err
		}
	}
}

// write delivers the stream's queue until ctx ends; once flush is closed, it
// writes what is still queued and returns nil.
func write(ctx context.Context, w protobuf.Writer, queue <-chan *pb.Broadcast, flush <-chan struct{}) error {
	for {
		select {
		case f := <-queue:
			if err := w.WriteMsgWithContext(ctx, f); err != nil {
				return err
			}
		case <-flush:
			for {
				select {
				case f := <-queue:
					if err := w.WriteMsgWithContext(ctx, f); err != nil {
						return err
					}
				default:
					return nil
				}
			}
		case <-ctx.Done():
			return ctx.Err()
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
// SWIP-87 and accepts, drops or punishes it. A returned error is a violation.
func (s *Service) handleFrame(co *cohort, m *member, f *pb.Broadcast) error {
	principal := co.spec.Principal
	admin := bytes.Equal(m.identity, principal)
	role, named, cued, cursor := co.view(m, f.Kind)

	// 0. a stream being closed — revoked, or ended by an EOS — is not read: what
	// it still sends was sent before it read the roster or the EOS
	if role == roleRevoked || role == roleEnded {
		s.dropClosing(co, role)
		return nil
	}
	// an AUTH from a spectator a roster had named on this stream, which a later
	// roster no longer names, raced its revocation: the stream is closed, the
	// AUTH dropped, nothing punished
	if role == roleSpectator && f.Kind == pb.Kind_AUTH && !named && cued {
		co.mtx.Lock()
		defer co.mtx.Unlock()
		s.revokeSpectator(co, m)
		return nil
	}
	// 1. a publisher stream sends publications and heartbeats — the admin's also
	// its service messages — a pending stream its AUTH, a spectator stream its
	// AUTH once a roster names it; anything else is not what the stream may send
	if !mayEmit(role, admin, named, f.Kind) {
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
	// an AUTH or EOS is moreover the empty chunk at index 0. The signature check
	// runs outside the lock, which is why the frame is applied in a second step.
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
		var ok bool
		if roster, ok = rosterOf(f.Soc); !ok {
			s.count(&co.counters.invalidRoster, "invalid_roster")
			return nil
		}
	}
	s.apply(co, m, f, roster)
	return nil
}

// apply applies a validated frame to the cohort. Every stream it ends, it ends
// by a request to the stream's handler, which never blocks, so it all runs
// under one lock.
func (s *Service) apply(co *cohort, m *member, f *pb.Broadcast, roster [][]byte) {
	co.mtx.Lock()
	defer co.mtx.Unlock()

	// a roster revoking the identity, or an EOS, was accepted since the checks
	if r := m.role; r == roleRevoked || r == roleEnded {
		s.dropClosing(co, r)
		return
	}
	switch f.Kind {
	case pb.Kind_AUTH:
		switch {
		case m.role == roleSpectator && !co.rostered(m.identity):
			// revoked since the checks
			s.revokeSpectator(co, m)
			return
		case m.role != rolePublisher:
			// the auth of a pending stream or of a spectator a roster names
			s.countQueueResets(co, co.upgrade(m, s.opts.MaxPublisherStreams))
		}
		// on a publisher stream, a heartbeat
		co.inactiveSince = time.Now()

	case pb.Kind_DATA:
		k := string(m.identity)
		if f.Index < co.cursors[k] {
			// raced with another publisher stream of the same identity
			s.count(&co.counters.retransmit, "retransmit")
			return
		}
		co.cursors[k] = f.Index + 1
		co.inactiveSince = time.Now()
		s.metrics.Delivered.Inc()
		s.countQueueResets(co, co.deliver(f, m))

	case pb.Kind_ROSTER:
		if f.Index < co.rosterCursor {
			s.count(&co.counters.retransmit, "retransmit")
			return
		}
		co.rosterCursor = f.Index + 1
		co.inactiveSince = time.Now()
		s.countQueueResets(co, co.applyRoster(f, roster, m))

	case pb.Kind_EOS:
		// the channel is ended: every stream is told — pending ones too — and
		// closed, and nothing more is read from any of them; the cohort is
		// forgotten when the last of them has left, and a Join meanwhile
		// creates a fresh one
		co.ended = true
		overflow := co.deliver(f, m)
		for _, o := range co.members() {
			if o.role == rolePending && !o.enqueue(f) && o.requestReset() {
				overflow++
			}
			o.role = roleEnded
			o.requestClose()
		}
		s.countQueueResets(co, overflow)
	}
}

// mayEmit reports whether a stream of the role may send a frame of the kind
// (step 1); named is whether the latest roster names the stream's identity.
// Kinds the broker does not define are left to step 2.
func mayEmit(r role, admin, named bool, kind pb.Kind) bool {
	switch r {
	case rolePublisher:
		return !service(kind) || admin
	case rolePending:
		return kind == pb.Kind_AUTH
	case roleSpectator:
		return named && kind == pb.Kind_AUTH
	}
	return false
}

// dropClosing counts a frame from a stream being closed, which is not read.
func (s *Service) dropClosing(co *cohort, r role) {
	if r == roleEnded {
		s.count(&co.counters.ended, "ended")
		return
	}
	s.count(&co.counters.revoked, "revoked")
}

// countQueueResets counts the streams reset because their queue was full.
func (s *Service) countQueueResets(co *cohort, n int) {
	for range n {
		s.count(&co.counters.queueReset, "queue_reset")
	}
}

// revokeSpectator closes a spectator whose AUTH raced the roster revoking it.
// Must be called with co.mtx held.
func (s *Service) revokeSpectator(co *cohort, m *member) {
	if m.role == roleSpectator {
		m.role = roleRevoked
		co.spectators--
	}
	s.count(&co.counters.revoked, "revoked")
	m.requestClose()
}

// join attaches a stream to the cohort named by the spec, creating the cohort
// if no live one has it, and returns leave, which detaches the stream again:
// the stream's handler calls it once, when it returns. An error wrapping
// errFull means a capacity bound was hit.
func (s *Service) join(peer swarm.Address, join *pb.Join) (co *cohort, m *member, leave func(), err error) {
	key, err := join.Cohort.Marshal()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal cohort spec: %w", err)
	}
	k, pk := string(key), peer.ByteString()

	s.mtx.Lock()
	defer s.mtx.Unlock()
	select {
	case <-s.quit:
		return nil, nil, nil, errSessionClosed
	default:
	}

	pc := s.peers[pk]
	if pc[k] == 0 && len(pc) >= s.opts.MaxCohortsPerConnection {
		return nil, nil, nil, fmt.Errorf("too many cohorts per peer: %w", errFull)
	}
	if pc[k] >= s.opts.MaxPeerStreamsPerCohort {
		return nil, nil, nil, fmt.Errorf("too many streams per peer per cohort: %w", errFull)
	}

	co, ok := s.cohorts[k]
	if ok {
		// a cohort its admin's EOS ended is being forgotten: the Join creates
		// a fresh one
		co.mtx.Lock()
		if co.ended {
			s.removeCohort(co)
			ok = false
		}
		co.mtx.Unlock()
	}
	if !ok {
		if len(s.cohorts) >= s.opts.MaxBrokerCohorts {
			return nil, nil, nil, fmt.Errorf("too many cohorts per broker: %w", errFull)
		}
		co = newCohort(k, join.Cohort)
		co.inactivity = time.AfterFunc(s.opts.InactiveTimeout, func() { s.reclaim(co) })
		s.cohorts[k] = co
		s.metrics.Cohorts.Set(float64(len(s.cohorts)))
	}

	co.mtx.Lock()
	defer co.mtx.Unlock()

	// the admin's identity is pending, outside the fan-out bound; any other is
	// a spectator, within it — except that a rostered identity finding the set
	// full is pending too: it receives the roster that names it and nothing
	// else until its AUTH, so the bound is not bypassed by declaring a public
	// address
	role := roleSpectator
	full := co.spectators >= s.opts.MaxSubsPerCohort
	if bytes.Equal(join.Identity, join.Cohort.Principal) || full && co.rostered(join.Identity) {
		role = rolePending
	}
	if role == roleSpectator && full {
		if len(co.attached) == 0 {
			s.removeCohort(co)
		}
		return nil, nil, nil, fmt.Errorf("too many subscribers per cohort: %w", errFull)
	}

	challenge := make([]byte, ChallengeSize)
	if _, err := rand.Read(challenge); err != nil {
		if len(co.attached) == 0 {
			s.removeCohort(co)
		}
		return nil, nil, nil, fmt.Errorf("challenge: %w", err)
	}
	m = newMember(role, join.Identity, challenge, s.opts.QueueCap)
	if role == rolePending {
		// a rostered identity receives the roster that names it at once: its cue
		if !bytes.Equal(join.Identity, join.Cohort.Principal) {
			m.queue <- co.rosterFrame
			m.given = co.rosterFrame
		}
	} else {
		co.spectators++
		// the latest roster is the first delivery on a receiving stream; one
		// naming the identity is the cue for its AUTH
		if co.rosterFrame != nil {
			m.queue <- co.rosterFrame
			m.cued = co.rostered(join.Identity)
		}
	}
	co.attached[m] = struct{}{}

	if pc == nil {
		pc = make(map[string]int)
		s.peers[pk] = pc
	}
	pc[k]++

	// leave detaches the stream: the peer's stream count, the member, and the
	// cohort, once no stream is attached to it
	leave = func() {
		s.mtx.Lock()
		defer s.mtx.Unlock()
		if pc := s.peers[pk]; pc != nil {
			if pc[k]--; pc[k] <= 0 {
				delete(pc, k)
			}
			if len(pc) == 0 {
				delete(s.peers, pk)
			}
		}

		co.mtx.Lock()
		defer co.mtx.Unlock()
		delete(co.attached, m)
		if m.role == roleSpectator {
			co.spectators--
		}
		if len(co.attached) == 0 && s.cohorts[k] == co {
			s.removeCohort(co)
		}
	}
	return co, m, leave, nil
}

// removeCohort forgets the cohort. Must be called with s.mtx and co.mtx held.
func (s *Service) removeCohort(co *cohort) {
	delete(s.cohorts, co.key)
	co.inactivity.Stop()
	s.metrics.Cohorts.Set(float64(len(s.cohorts)))
}

// reclaim forgets a cohort on which no frame was accepted for the inactivity
// deadline, and asks every stream in it to reset.
func (s *Service) reclaim(co *cohort) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.cohorts[co.key] != co {
		return
	}

	co.mtx.Lock()
	defer co.mtx.Unlock()
	if idle := time.Since(co.inactiveSince); idle < s.opts.InactiveTimeout {
		co.inactivity.Reset(s.opts.InactiveTimeout - idle)
		return
	}
	s.removeCohort(co)
	for _, m := range co.members() {
		m.requestReset()
	}
	s.logger.Debug("cohort reclaimed", "error", errCohortReclaimed)
}

// authExpired ends a stream still pending at its auth deadline, and reports
// whether it was: a stream that authenticated, or began closing, before the
// deadline is left alone.
func (s *Service) authExpired(peer swarm.Address, co *cohort, m *member) bool {
	if !co.expire(m) {
		return false
	}
	s.count(&co.counters.authTimeout, "auth_timeout")
	// a rostered identity admitted past a full fan-out set that does not
	// authenticate has cost what a FULL would have: no blocklist
	if s.blocklister != nil && bytes.Equal(m.identity, co.spec.Principal) {
		if err := s.blocklister.Blocklist(peer, s.opts.InvalidAuthBlocklistDuration, "bps auth timeout"); err != nil {
			s.logger.Debug("blocklist failed", "peer_address", peer, "error", err)
		}
	}
	return true
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
	roleSpectator role = iota + 1 // receives; sends nothing but, once a roster names it, its AUTH
	rolePending                   // declared the principal — or a rostered address at a full fan-out set — no AUTH yet
	rolePublisher                 // authenticated by its AUTH: publishes, and receives what others publish
	roleRevoked                   // a publisher stream a roster dropped, or timed out pending: being closed, not read
	roleEnded                     // a stream of a cohort its admin's EOS ended: being closed, not read
)

// receives reports whether deliveries are enqueued on a stream of the role.
func (r role) receives() bool { return r == roleSpectator || r == rolePublisher }

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
	attached      map[*member]struct{} // the streams attached to the cohort
	spectators    int                  // the fan-out set
	authSeq       uint64               // orders publisher streams by their auth
	ended         bool                 // the admin's EOS was accepted
}

// newCohort returns an empty cohort for the spec, keyed by its canonical
// serialisation. The caller arms its inactivity timer.
func newCohort(key string, spec *pb.CohortSpec) *cohort {
	return &cohort{
		key:           key,
		spec:          spec,
		attached:      make(map[*member]struct{}),
		cursors:       make(map[string]uint64),
		inactiveSince: time.Now(),
	}
}

// members returns the streams attached to the cohort. Must be called with
// co.mtx held.
func (co *cohort) members() []*member {
	ms := make([]*member, 0, len(co.attached))
	for m := range co.attached {
		ms = append(ms, m)
	}
	return ms
}

// rostered reports whether the latest roster names the address. Must be
// called with co.mtx held.
func (co *cohort) rostered(a []byte) bool {
	return contains(co.roster, a)
}

// view returns what the checks before the signature need of a stream: its
// role, whether the roster names its identity, whether a roster naming it was
// enqueued on it, and the cursor of the feed a frame of the kind is on.
func (co *cohort) view(m *member, kind pb.Kind) (r role, named, cued bool, cursor uint64) {
	co.mtx.Lock()
	defer co.mtx.Unlock()
	switch kind {
	case pb.Kind_DATA:
		cursor = co.cursors[string(m.identity)]
	case pb.Kind_ROSTER:
		cursor = co.rosterCursor
	}
	return m.role, co.rostered(m.identity), m.cued, cursor
}

// expire marks a stream still pending at its auth deadline as ending, so that
// an AUTH arriving now is not accepted, and reports whether it was pending.
func (co *cohort) expire(m *member) bool {
	co.mtx.Lock()
	defer co.mtx.Unlock()
	if m.role != rolePending {
		return false
	}
	m.role = roleRevoked
	return true
}

// deliver enqueues an accepted frame on every receiving stream — for DATA,
// every one not bound to the sender's identity — and returns how many streams
// were asked to reset because their queue was full. A ROSTER or EOS reaches
// the sender too: where it sits in the sender's stream is where it took
// effect, and every receiver, the admin included, admits by the rosters it
// receives. The send is non-blocking, so that one slow receiver cannot stall
// the cohort. Must be called with co.mtx held.
func (co *cohort) deliver(f *pb.Broadcast, sender *member) (overflow int) {
	for m := range co.attached {
		if !m.role.receives() || m.resetting || f.Kind == pb.Kind_DATA && bytes.Equal(m.identity, sender.identity) {
			continue
		}
		if !m.enqueue(f) && m.requestReset() {
			overflow++
		}
	}
	return overflow
}

// upgrade makes a pending stream, or a spectator stream a roster names, a
// publisher stream; beyond the bound on the publisher streams one address may
// hold, the oldest of them is asked to reset. It returns 1 if the stream's
// queue could not take the latest roster, and 0 otherwise. Must be called with
// co.mtx held.
func (co *cohort) upgrade(m *member, maxStreams int) (overflow int) {
	if m.role == rolePending {
		// a stream that starts receiving gets the latest roster first, unless
		// it was given it as its cue
		if co.rosterFrame != nil && m.given != co.rosterFrame && !m.enqueue(co.rosterFrame) && m.requestReset() {
			overflow++
		}
	} else {
		// a spectator leaves the fan-out set; it has the roster already
		co.spectators--
	}
	m.role = rolePublisher
	co.authSeq++
	m.authSeq = co.authSeq

	var same []*member
	for o := range co.attached {
		if o.role == rolePublisher && !o.resetting && bytes.Equal(o.identity, m.identity) {
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
		same[oldest].requestReset()
		same = append(same[:oldest], same[oldest+1:]...)
	}
	return overflow
}

// applyRoster makes an accepted roster the cohort's: it is delivered — to its
// sender too — and every publisher stream of an identity it drops is revoked:
// written the roster, then closed, what it sends meanwhile dropped; the
// cursors of dropped identities are forgotten. A spectator it names may send
// its AUTH. It returns how many streams were asked to reset because their
// queue was full. Must be called with co.mtx held.
func (co *cohort) applyRoster(f *pb.Broadcast, roster [][]byte, sender *member) (overflow int) {
	principal := co.spec.Principal
	co.roster = roster
	co.rosterFrame = f
	overflow = co.deliver(f, sender)
	for m := range co.attached {
		if bytes.Equal(m.identity, principal) {
			continue
		}
		switch {
		case m.role == roleSpectator && co.rostered(m.identity):
			// delivered above: the cue for its AUTH
			m.cued = true
		case m.role == rolePending && co.rostered(m.identity):
			// a rostered pending stream is given the latest roster
			m.given = f
			if !m.enqueue(f) && m.requestReset() {
				overflow++
			}
		case m.role == rolePending:
			// revoked before its auth: told, then closed
			m.role = roleRevoked
			if !m.enqueue(f) {
				if m.requestReset() {
					overflow++
				}
				continue
			}
			m.requestClose()
		case m.role == rolePublisher && !co.rostered(m.identity):
			// revoked: it was delivered this roster above; closed once that
			// is written
			m.role = roleRevoked
			m.requestClose()
		}
	}
	for k := range co.cursors {
		if a := []byte(k); !bytes.Equal(a, principal) && !co.rostered(a) {
			delete(co.cursors, k)
		}
	}
	return overflow
}

// member is one stream attached to a cohort. Its handler owns the stream and
// is the only one that closes or resets it; anyone else asks, by
// requestClose or requestReset. The fields after the requests are guarded by
// cohort.mtx.
type member struct {
	identity     []byte
	challenge    []byte
	authDeadline bool // admitted pending: the auth deadline applies
	queue        chan *pb.Broadcast
	closeReq     chan struct{} // one slot: write the queue, then close
	resetReq     chan struct{} // one slot: reset at once; takes precedence

	role      role
	authSeq   uint64
	cued      bool          // a roster naming the identity was enqueued on this spectator stream
	given     *pb.Broadcast // the roster last enqueued on a rostered pending stream
	resetting bool          // a reset was asked for: nothing more is enqueued
}

// newMember returns the member of a stream joining in the role, with its
// outbound queue and its two requests.
func newMember(r role, identity, challenge []byte, queueCap int) *member {
	return &member{
		identity:     identity,
		challenge:    challenge,
		authDeadline: r == rolePending,
		queue:        make(chan *pb.Broadcast, queueCap),
		closeReq:     make(chan struct{}, 1),
		resetReq:     make(chan struct{}, 1),
		role:         r,
	}
}

// requestClose asks the stream's handler to write what is queued, then close
// the stream, within the flush deadline. It never blocks, and asking again
// changes nothing.
func (m *member) requestClose() { request(m.closeReq) }

// requestReset asks the stream's handler to reset the stream at once, and
// keeps anything more from being enqueued on it. It never blocks; it reports
// whether this was the first ask. Must be called with cohort.mtx held.
func (m *member) requestReset() bool {
	if m.resetting {
		return false
	}
	m.resetting = true
	request(m.resetReq)
	return true
}

// request fills a one-slot request channel, unless it is full already. The
// channel is never closed, so no ask can panic, however often it is made.
func request(c chan struct{}) {
	select {
	case c <- struct{}{}:
	default:
	}
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
