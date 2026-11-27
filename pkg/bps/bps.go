// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bps implements BPS-lite (SWIP-74) with the explicit multi-publisher
// mode of SWIP-87: a live stream over feeds, through one broker, one hop.
//
// A cohort is identified by its spec {topic, FEED_TOPIC, principal}. Every
// peer joins with the spec and, unless it will only listen, the stream's owner —
// the wallet address signing its feed update SOCs — and is answered with a random
// per-stream challenge. A stream declaring the principal — the admin — is
// pending until its AUTH — the empty chunk at index 0 of the session's AUTH feed
// — authenticates it; any other stream listens, and its AUTH upgrades it once a
// roster naming its owner has been delivered on it. A publisher stream sends
// ordinary single-owner chunks that are updates of its session feed on
// keccak256(topic | challenge), and the admin's publisher stream also its service
// messages: ROSTER, the publisher set, and EOS, the end of the channel. The
// broker keeps a cursor per publisher and per service kind, and delivers every
// accepted frame to every receiving stream not bound to its author.
package bps

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"slices"
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
	errCohortForgotten = errors.New("bps: cohort forgotten")
	errFull            = errors.New("bps: capacity bound")
	errInvalidDelivery = errors.New("bps: invalid delivery")
	errChannelEnded    = errors.New("bps: channel ended")
	errResetRequested  = errors.New("bps: stream reset")
	errFlushTimeout    = errors.New("bps: queue not written out in time")
	errAuthTimeout     = errors.New("bps: no auth in time")

	// ErrEndOfStream is a session's Err after the admin's EOS ended the channel.
	ErrEndOfStream = errors.New("bps: end of stream")
	// ErrNotPublisher refuses a publication the session may not make: not
	// granted, revoked, or a service message from anyone but the admin.
	ErrNotPublisher = errors.New("bps: session is not a publisher")
)

// ForgingBrokerBlocklistDuration is how long a subscriber avoids a broker that
// forwarded a delivery that does not validate, which no honest broker sends.
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
	InactiveTimeout  time.Duration // forgets a cohort with no accepted frame
	AuthWaitTimeout  time.Duration // disconnects a pending stream that has not authenticated
	FlushTimeout     time.Duration // bounds writing the queue of a stream being closed
	HandshakeTimeout time.Duration // bounds a client's Join until the broker's Ack
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
	HandshakeTimeout:             10 * time.Second,
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
	if o.HandshakeTimeout > 0 {
		d.HandshakeTimeout = o.HandshakeTimeout
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
		s.closeCohort(co)
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
	Cursors      map[string]uint64 // by the publisher's 20-byte address as a string: expect the next DATA feed update at this index or above; 0 = none yet
	Roster       [][]byte          // the latest accepted roster
	RosterCursor uint64            // expect the next ROSTER feed update at this index or above; 0 = none yet
	Ended        bool              // an EOS was accepted: the EOS cursor is past 0
}

func (p Position) clone() Position {
	p.Cursors = maps.Clone(p.Cursors)
	if p.Cursors == nil {
		p.Cursors = make(map[string]uint64)
	}
	p.Roster = slices.Clone(p.Roster)
	return p
}

// Session is a single joined p2p stream to a broker.
type Session interface {
	// Challenge is the challenge the broker issued for this stream. Every
	// chunk published on the session must be signed under it (see ID).
	Challenge() []byte
	// Messages yields the deliveries that verify end to end against the cohort
	// spec, the roster and the session's cursors: DATA from the principal or a
	// rostered publisher, and the principal's ROSTER and EOS. It is not closed:
	// Done says when the stream has ended, by when every delivery, an EOS last,
	// has been handed over.
	Messages() <-chan Message
	// Position is the session's place on the channel, to pass to a later Join.
	Position() Position
	// Granted is closed once the session may authenticate: at once for the
	// principal, and for any other owner once a roster naming it has been
	// delivered on this stream, which a grantee waits for before its AUTH,
	// which upgrades the stream in place. A publisher whose owner a roster
	// drops finds its stream closed after that roster. A stream with no owner is
	// never granted: no roster can name it.
	Granted() <-chan struct{}
	// Publish writes a chunk of the given kind and index, signed under the
	// session challenge, to the broker. The broker does not reply, so the
	// session refuses what the broker would: ErrNotPublisher unless the owner
	// is the principal, or granted and on the latest roster, and for a ROSTER or
	// EOS from anyone but the principal; ErrInvalidMessage for anything before
	// the AUTH and for a frame no chunk can make right; ErrInvalidSOC for a chunk
	// that does not validate; ErrInvalidRoster for a roster it will not send.
	Publish(ctx context.Context, kind Kind, index uint64, chunk []byte) error
	// Done is closed when the p2p stream has ended.
	Done() <-chan struct{}
	// Err reports why the stream ended: ErrEndOfStream after an EOS. Valid after
	// Done is closed.
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
	Owner    []byte        // 20 bytes or nil: the owner of the stream's frames, the wallet address signing its feed update SOCs; nil, the stream can only listen
	Position Position      // the subscriber's place on the channel, from an earlier session
}

// Join joins a cohort at the broker. A peer whose join was refused or whose
// stream was reset must back off before rejoining.
func (s *Service) Join(ctx context.Context, req JoinRequest) (Session, error) {
	join := &pb.Join{Cohort: req.Spec.proto(), Owner: req.Owner}
	if err := validateJoin(join); err != nil {
		return nil, err
	}
	// a channel the subscriber has seen end is not rejoined
	if req.Position.Ended {
		return nil, errChannelEnded
	}
	// a broker that never answers must not hold the caller: the handshake is
	// bounded; the session that follows has a context of its own
	ctx, cancel := context.WithTimeout(ctx, s.opts.HandshakeTimeout)
	defer cancel()
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

// readLoop delivers the broker's frames until the stream ends, and is the one
// place where the session ends: on its way out it alone ends the stream — closed
// after an EOS or a Close, reset on anything else — and then closes Done. Close
// only asks, by cancelling the context, which stops a read or a delivery in
// progress.
func (s *session) readLoop(stream p2p.Stream, r protobuf.Reader) {
	defer close(s.done)
	defer func() {
		switch {
		case s.ctx.Err() != nil:
			// a Close: the read it cancelled may still be in progress, and a
			// second read on the stream, as FullClose's, would strand one of
			// them, since a stream wakes a single reader. Close starts none: it
			// wakes the read in progress and half-closes, so what the session
			// wrote still reaches the broker
			_ = stream.Close()
		case s.err != nil && !errors.Is(s.err, ErrEndOfStream):
			_ = stream.Reset()
		default:
			// an EOS: no read is in progress, FullClose's is the only one
			_ = stream.FullClose()
		}
		s.cancel()
	}()

	for {
		var f pb.Broadcast
		if s.err = r.ReadMsgWithContext(s.ctx, &f); s.err != nil {
			return
		}
		msg, err := s.decode(&f)
		if err != nil {
			// end the stream and avoid the broker
			s.err = fmt.Errorf("%w: %w", errInvalidDelivery, err)
			s.forged(s.err)
			return
		}
		if !s.accept(msg) {
			continue
		}
		select {
		case s.rx <- msg:
		case <-s.ctx.Done():
			return
		}
		if msg.Kind == KindEOS {
			s.err = ErrEndOfStream
			return
		}
	}
}

// decode verifies a delivery end to end and returns it as a Message: a DATA
// update with the owner its signature recovers, a ROSTER with its publisher
// set, an EOS. The owner of a ROSTER or an EOS is forced to the principal. The
// challenge is not compared: it is the publisher stream's, which the
// subscriber never sees. A frame the subscriber drops unread — an AUTH, a kind
// it does not know, a roster that does not decode — comes back with
// KindUnspecified. An error is a chunk that does not validate, which an
// honest broker never forwards.
func (s *session) decode(f *pb.Broadcast) (Message, error) {
	msg := Message{SOC: f.Soc, Challenge: f.Challenge, Index: f.Index}
	switch f.Kind {
	case pb.Kind_DATA:
		c, err := parseFrame(f, s.spec.Topic)
		if err != nil {
			return msg, err
		}
		msg.Kind, msg.Owner = KindData, c.OwnerAddress()
	case pb.Kind_ROSTER:
		c, err := verify(f, s.spec.Topic, s.spec.Principal)
		if err != nil {
			return msg, err
		}
		roster, ok := decodeRosterMsg(payload(c))
		if !ok {
			s.svc.metrics.SubscriberDropped.WithLabelValues("invalid_roster").Inc()
			return msg, nil
		}
		msg.Kind, msg.Owner, msg.Roster = KindRoster, s.spec.Principal, roster
	case pb.Kind_EOS:
		if _, err := verify(f, s.spec.Topic, s.spec.Principal); err != nil {
			return msg, err
		}
		msg.Kind, msg.Owner = KindEOS, s.spec.Principal
	}
	return msg, nil
}

// accept applies a decoded delivery to the subscriber's position, and reports
// whether to forward it: not a DATA update below its owner's cursor or by an
// owner the roster does not name, not a roster older than the latest.
func (s *session) accept(msg Message) (forward bool) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	switch msg.Kind {
	case KindData:
		// An owner missing from this subscriber's roster is not proof of a
		// forging broker: rosters differ between brokers while a revocation
		// travels, and the subscriber's may be newer.
		if !s.admits(msg.Owner) {
			s.svc.metrics.SubscriberDropped.WithLabelValues("not_rostered").Inc()
			return false
		}
		k := string(msg.Owner)
		if msg.Index < s.pos.Cursors[k] {
			return false
		}
		s.pos.Cursors[k] = msg.Index + 1
		return true
	case KindRoster:
		forward = msg.Index >= s.pos.RosterCursor
		if forward {
			s.pos.RosterCursor = msg.Index + 1
			s.pos.Roster = msg.Roster
		}
		// a roster naming the owner, delivered on this stream, while the
		// latest one still names it: the broker holds it too
		if contains(msg.Roster, s.owner) && s.admits(s.owner) {
			s.grant()
		}
		return forward
	case KindEOS:
		s.pos.Ended = true
		return true
	}
	return false
}

// admits reports whether owner may publish: the principal, or an address on the
// latest roster. Must be called with s.mtx held.
func (s *session) admits(owner []byte) bool {
	return bytes.Equal(owner, s.spec.Principal) || contains(s.pos.Roster, owner)
}

func contains(set [][]byte, a []byte) bool {
	return slices.ContainsFunc(set, func(b []byte) bool { return bytes.Equal(a, b) })
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
	owner     []byte
	mtx       sync.Mutex
	pos       Position
	authed    bool          // the AUTH was written: anything else may follow
	granted   chan struct{} // closed when the stream may send its AUTH: at once for the principal, else by the read loop on a roster naming the owner
	isGranted bool          // granted is closed; read and written by the read loop alone (and newSession)
	rx        chan Message  // unbuffered: a delivery is handed over before Done can close
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
		owner:     req.Owner,
		pos:       req.Position.clone(),
		granted:   make(chan struct{}),
		rx:        make(chan Message),
		done:      make(chan struct{}),
	}
	// the principal's AUTH needs no roster: the broker still checks it
	if bytes.Equal(req.Owner, req.Spec.Principal) {
		s.grant()
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

// grant closes granted, which tells the owner to authenticate, once. Only
// newSession and then the read loop call it.
func (s *session) grant() {
	if !s.isGranted {
		s.isGranted = true
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

// Publish checks a frame the way the broker will, so that a mistake is told to
// the caller rather than punished at the broker, and writes it.
func (s *session) Publish(ctx context.Context, kind Kind, index uint64, chunk []byte) error {
	f := &pb.Broadcast{Soc: chunk, Kind: kind.proto(), Challenge: s.challenge, Index: index}
	if err := s.check(f); err != nil {
		return err
	}
	select {
	case <-s.done:
		return errSessionClosed
	default:
	}
	s.wmtx.Lock()
	defer s.wmtx.Unlock()
	if err := s.w.WriteMsgWithContext(ctx, f); err != nil {
		return fmt.Errorf("write broadcast: %w", err)
	}
	if f.Kind == pb.Kind_AUTH {
		s.mtx.Lock()
		s.authed = true
		s.mtx.Unlock()
	}
	// a roster or an EOS the admin publishes is delivered back to it, and it
	// admits by it from where it lands in its stream
	return nil
}

// check runs, for a frame about to be published, the broker's checks in the
// broker's order: what the stream may send now, then the frame itself.
func (s *session) check(f *pb.Broadcast) error {
	select {
	case <-s.granted:
	default:
		return ErrNotPublisher
	}
	s.mtx.Lock()
	seated, authed := s.admits(s.owner), s.authed
	s.mtx.Unlock()
	principal := bytes.Equal(s.owner, s.spec.Principal)
	if !seated || !principal && isServiceKind(f.Kind) {
		return ErrNotPublisher
	}
	switch f.Kind {
	case pb.Kind_DATA, pb.Kind_AUTH, pb.Kind_EOS, pb.Kind_ROSTER:
	default:
		return fmt.Errorf("unknown kind: %w", ErrInvalidMessage)
	}
	// a stream may send nothing before its AUTH; the broker would reset it
	if !authed && f.Kind != pb.Kind_AUTH {
		return fmt.Errorf("%s before the auth: %w", f.Kind, ErrInvalidMessage)
	}
	// a service kind has passed only from the principal, so the stream's owner is the chunk's
	c, err := verify(f, s.spec.Topic, s.owner)
	if err != nil {
		return err
	}
	if f.Kind == pb.Kind_ROSTER {
		return checkRoster(payload(c), s.spec.Principal)
	}
	return nil
}

// checkRoster refuses a roster this node will not send. A receiver drops one
// that does not decode, and an entry equal to the admin's, or repeated, changes
// nothing there; a sender has no reason to send either, and can still be told.
func checkRoster(b, principal []byte) error {
	roster, ok := decodeRosterMsg(b)
	if !ok {
		return ErrInvalidRoster
	}
	for i, a := range roster {
		if bytes.Equal(a, principal) {
			return fmt.Errorf("the admin must not be on the roster: %w", ErrInvalidRoster)
		}
		if contains(roster[:i], a) {
			return fmt.Errorf("address %x twice: %w", a, ErrInvalidRoster)
		}
	}
	return nil
}

// validateJoin rejects a Join whose spec has a value outside SWIP-74 or whose
// owner is neither absent nor 20 bytes.
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
	case len(join.Owner) != 0 && len(join.Owner) != crypto.AddressSize:
		return fmt.Errorf("owner length %d: %w", len(join.Owner), errInvalidJoin)
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
// peer's frames, and the writer, delivering the stream's queue — until one of
// them stops or the stream is asked to end, and returns why. Ending the stream
// is its deferred last step, and the only place it happens: a nil error means
// the queue was written out after a close request, and the stream is closed so
// that what was written arrives; any other error resets it.
func (s *Service) serve(ctx context.Context, peer swarm.Address, stream p2p.Stream, w protobuf.Writer, r protobuf.Reader, co *cohort, m *member) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	readDone, writeDone := make(chan struct{}), make(chan struct{})
	var readErr, writeErr error
	go func() { defer close(readDone); readErr = s.read(ctx, r, co, m) }()
	go func() { defer close(writeDone); writeErr = write(ctx, w, m.queue) }()

	var authC, flushC <-chan time.Time
	if m.authDeadline {
		t := time.NewTimer(s.opts.AuthWaitTimeout)
		defer t.Stop()
		authC = t.C
	}
	defer func() {
		if err == nil {
			_ = stream.Close()
		} else {
			_ = stream.Reset()
		}
		cancel()
		<-readDone
		<-writeDone
	}()

	readC, closeC := readDone, m.closeReq
	for {
		// a reset asked for wins over anything else ready at the same time
		select {
		case <-m.resetReq:
			return errResetRequested
		default:
		}
		select {
		case <-m.resetReq:
			return errResetRequested

		case <-closeC:
			// the queue is closed with its last frame in: the writer writes it
			// out and returns; give it the flush deadline
			closeC = nil
			t := time.NewTimer(s.opts.FlushTimeout)
			defer t.Stop()
			flushC = t.C

		case <-flushC:
			return errFlushTimeout

		case <-writeDone:
			// nil once the closed queue is written out
			return writeErr

		case <-readC:
			readC = nil
			if closeC == nil {
				// closing: the writer decides when the stream ends
				continue
			}
			if errors.Is(readErr, errViolation) && s.blocklister != nil {
				if berr := s.blocklister.Blocklist(peer, s.opts.ProtoBreachBlocklistDuration, readErr.Error()); berr != nil {
					s.logger.Error(berr, "blocklist failed", "peer_address", peer)
				}
			}
			return fmt.Errorf("read: %w", readErr)

		case <-authC:
			authC = nil
			if s.authExpired(peer, co, m) {
				return errAuthTimeout
			}

		case <-ctx.Done():
			return ctx.Err()
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

// write delivers the stream's queue until ctx ends, and returns nil once the
// queue is closed and written out.
func write(ctx context.Context, w protobuf.Writer, queue <-chan *pb.Broadcast) error {
	for {
		select {
		case f, ok := <-queue:
			if !ok {
				return nil
			}
			if err := w.WriteMsgWithContext(ctx, f); err != nil {
				return fmt.Errorf("write: %w", err)
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
	admin := bytes.Equal(m.owner, principal)
	role, named, rosterSent, cursor := co.view(m, f.Kind)

	// 0. a stream being closed — revoked, or ended by an EOS — is not read: what
	// it still sends was sent before it read the roster or the EOS
	if role == roleRevoked || role == roleEnded {
		s.dropClosing(co, role)
		return nil
	}
	// an AUTH from a spectator a roster had named on this stream, which a later
	// roster no longer names, raced its revocation: the stream is closed, the
	// AUTH dropped, nothing punished
	if role == roleSpectator && f.Kind == pb.Kind_AUTH && !named && rosterSent {
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
	// stream's owner for DATA and AUTH, the principal for the service kinds;
	// an AUTH or EOS is moreover the empty chunk at index 0. The signature check
	// runs outside the lock, which is why the frame is applied in a second step.
	owner := m.owner
	if isServiceKind(f.Kind) {
		owner = principal
	}
	c, err := verify(f, co.spec.Topic, owner)
	if err != nil {
		s.count(&co.counters.invalidSOC, "invalid_soc")
		return fmt.Errorf("%w: %w", errViolation, err)
	}
	// 6. a ROSTER must decode; one that does not changes nothing
	var roster [][]byte
	if f.Kind == pb.Kind_ROSTER {
		var ok bool
		if roster, ok = decodeRosterMsg(payload(c)); !ok {
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

	// a roster revoking the owner, or an EOS, was accepted since the checks
	if r := m.role; r == roleRevoked || r == roleEnded {
		s.dropClosing(co, r)
		return
	}
	switch f.Kind {
	case pb.Kind_AUTH:
		switch {
		case m.role == roleSpectator && !co.rostered(m.owner):
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
		k := string(m.owner)
		if f.Index < co.cursors[k] {
			// raced with another publisher stream of the same owner
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
		fullQueues := co.deliver(f, m)
		for _, o := range co.members() {
			if o.role == rolePending && !o.enqueue(f) && o.requestReset() {
				fullQueues++
			}
			o.role = roleEnded
			o.requestClose()
		}
		s.countQueueResets(co, fullQueues)
	}
}

// mayEmit reports whether a stream of the role may send a frame of the kind
// (step 1); named is whether the latest roster names the stream's owner.
// Kinds the broker does not define are left to step 2.
func mayEmit(r role, admin, named bool, kind pb.Kind) bool {
	switch r {
	case rolePublisher:
		return !isServiceKind(kind) || admin
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
		co.inactivity = time.AfterFunc(s.opts.InactiveTimeout, func() { s.forget(co) })
		s.cohorts[k] = co
		s.metrics.Cohorts.Set(float64(len(s.cohorts)))
	}

	co.mtx.Lock()
	defer co.mtx.Unlock()

	// a stream owned by the admin is pending, outside the fan-out bound; any other
	// is a spectator, within it — except that a rostered owner finding the set
	// full is pending too: it receives the roster that names it and nothing
	// else until its AUTH, so the bound is not bypassed by declaring a public
	// address. A stream with no owner is a spectator that no roster can name.
	role := roleSpectator
	full := co.spectators >= s.opts.MaxSubsPerCohort
	if bytes.Equal(join.Owner, join.Cohort.Principal) || full && co.rostered(join.Owner) {
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
	m = newMember(role, join.Owner, challenge, s.opts.QueueCap)
	if role == rolePending {
		// a rostered owner receives at once the roster that names it, which tells it to send its AUTH
		if !bytes.Equal(join.Owner, join.Cohort.Principal) {
			m.queue <- co.rosterFrame
			m.lastRoster = co.rosterFrame
		}
	} else {
		co.spectators++
		// the latest roster is the first delivery on a receiving stream; one
		// naming the owner tells it to send its AUTH
		if co.rosterFrame != nil {
			m.queue <- co.rosterFrame
			m.rosterSent = co.rostered(join.Owner)
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

// closeCohort forgets the cohort and asks every stream in it to reset. Must be
// called with s.mtx and co.mtx held.
func (s *Service) closeCohort(co *cohort) {
	s.removeCohort(co)
	for _, m := range co.members() {
		m.requestReset()
	}
}

// forget forgets a cohort on which no frame was accepted for the inactivity
// deadline, and asks every stream in it to reset.
func (s *Service) forget(co *cohort) {
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
	s.closeCohort(co)
	s.logger.Debug("cohort forgotten", "error", errCohortForgotten)
}

// authExpired ends a stream still pending at its auth deadline, and reports
// whether it was: a stream that authenticated, or began closing, before the
// deadline is left alone.
func (s *Service) authExpired(peer swarm.Address, co *cohort, m *member) bool {
	if !co.expire(m) {
		return false
	}
	s.count(&co.counters.authTimeout, "auth_timeout")
	// a rostered owner admitted past a full fan-out set that does not
	// authenticate has cost what a FULL would have: no blocklist
	if s.blocklister != nil && bytes.Equal(m.owner, co.spec.Principal) {
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
	cursors       map[string]uint64 // by publisher: expect the next DATA feed update at this index or above; 0 = none yet
	roster        [][]byte          // the latest accepted roster, the principal excluded
	rosterCursor  uint64            // expect the next ROSTER feed update at this index or above; 0 = none yet
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
// role, whether the roster names its owner, whether a roster naming it was
// enqueued on it, and the cursor of the feed a frame of the kind is on.
func (co *cohort) view(m *member, kind pb.Kind) (r role, named, rosterSent bool, cursor uint64) {
	co.mtx.Lock()
	defer co.mtx.Unlock()
	switch kind {
	case pb.Kind_DATA:
		cursor = co.cursors[string(m.owner)]
	case pb.Kind_ROSTER:
		cursor = co.rosterCursor
	}
	return m.role, co.rostered(m.owner), m.rosterSent, cursor
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
// every one not owned by the sender's owner — and returns how many streams
// were asked to reset because their queue was full. A ROSTER or EOS reaches
// the sender too: where it sits in the sender's stream is where it took
// effect, and every receiver, the admin included, admits by the rosters it
// receives. The send is non-blocking, so that one slow receiver cannot stall
// the cohort. Must be called with co.mtx held.
func (co *cohort) deliver(f *pb.Broadcast, sender *member) (fullQueues int) {
	for m := range co.attached {
		if !m.role.receives() || m.resetting || f.Kind == pb.Kind_DATA && bytes.Equal(m.owner, sender.owner) {
			continue
		}
		if !m.enqueue(f) && m.requestReset() {
			fullQueues++
		}
	}
	return fullQueues
}

// upgrade makes a pending stream, or a spectator stream a roster names, a
// publisher stream; beyond the bound on the publisher streams one address may
// hold, the oldest of them is asked to reset. It returns 1 if the stream's
// queue could not take the latest roster, and 0 otherwise. Must be called with
// co.mtx held.
func (co *cohort) upgrade(m *member, maxStreams int) (fullQueues int) {
	if m.role == rolePending {
		// a stream that starts receiving gets the latest roster first, unless
		// it was sent it before its AUTH
		if co.rosterFrame != nil && m.lastRoster != co.rosterFrame && !m.enqueue(co.rosterFrame) && m.requestReset() {
			fullQueues++
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
		if o.role == rolePublisher && !o.resetting && bytes.Equal(o.owner, m.owner) {
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
	return fullQueues
}

// applyRoster makes an accepted roster the cohort's: it is delivered — to its
// sender too — and every publisher stream of an owner it drops is revoked:
// written the roster, then closed, what it sends meanwhile dropped; the
// cursors of dropped owners are forgotten. A spectator it names may send
// its AUTH. It returns how many streams were asked to reset because their
// queue was full. Must be called with co.mtx held.
func (co *cohort) applyRoster(f *pb.Broadcast, roster [][]byte, sender *member) (fullQueues int) {
	principal := co.spec.Principal
	co.roster = roster
	co.rosterFrame = f
	fullQueues = co.deliver(f, sender)
	for m := range co.attached {
		if bytes.Equal(m.owner, principal) {
			continue
		}
		switch {
		case m.role == roleSpectator && co.rostered(m.owner):
			// delivered above: it may now send its AUTH
			m.rosterSent = true
		case m.role == rolePending && co.rostered(m.owner):
			// a rostered pending stream is given the latest roster
			m.lastRoster = f
			if !m.enqueue(f) && m.requestReset() {
				fullQueues++
			}
		case m.role == rolePending:
			// revoked before its auth: told, then closed
			m.role = roleRevoked
			if !m.enqueue(f) {
				if m.requestReset() {
					fullQueues++
				}
				continue
			}
			m.requestClose()
		case m.role == rolePublisher && !co.rostered(m.owner):
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
	return fullQueues
}

// member is one stream attached to a cohort. Its handler owns the stream and
// is the only one that closes or resets it; anyone else asks, by
// requestClose or requestReset. The fields after the requests are guarded by
// cohort.mtx.
type member struct {
	owner        []byte // nil for a stream that can only listen
	challenge    []byte
	authDeadline bool               // admitted pending: the auth deadline applies
	queue        chan *pb.Broadcast // closed by requestClose, after its last frame
	closeReq     chan struct{}      // one slot: the queue is closed; write it out, then close
	resetReq     chan struct{}      // one slot: reset at once; takes precedence

	role       role
	authSeq    uint64
	rosterSent bool          // a roster naming the owner was enqueued on this spectator stream
	lastRoster *pb.Broadcast // the roster last enqueued on this rostered pending stream
	resetting  bool          // a reset was asked for: nothing more is enqueued
	closing    bool          // the queue is closed: nothing more is enqueued
}

// newMember returns the member of a stream joining in the role, with its
// outbound queue and its two requests.
func newMember(r role, owner, challenge []byte, queueCap int) *member {
	return &member{
		owner:        owner,
		challenge:    challenge,
		authDeadline: r == rolePending,
		queue:        make(chan *pb.Broadcast, queueCap),
		closeReq:     make(chan struct{}, 1),
		resetReq:     make(chan struct{}, 1),
		role:         r,
	}
}

// requestClose closes the stream's queue, so that what is in it is the last
// the stream gets, and asks the stream's handler to write it out and close the
// stream, within the flush deadline. It never blocks, and asking again changes
// nothing. Must be called with cohort.mtx held.
func (m *member) requestClose() {
	if m.closing {
		return
	}
	m.closing = true
	close(m.queue)
	request(m.closeReq)
}

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
// the queue is full. A closing stream takes nothing more, which is not a full
// queue. Must be called with cohort.mtx held.
func (m *member) enqueue(f *pb.Broadcast) bool {
	if m.closing {
		return true
	}
	select {
	case m.queue <- f:
		return true
	default:
		return false
	}
}
