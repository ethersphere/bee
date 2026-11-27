// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bps implements BPS-lite (SWIP-74) with the multi-publisher mode
// (SWIP-87). It is a live stream over feeds, relayed by a single broker.
//
// Peers that join with the same topic and principal form a cohort. A peer may
// declare an owner, the address that signs its chunks. Without one, the stream
// only listens. The broker answers each join with a random challenge. The
// principal (the admin) must authenticate with an AUTH chunk before it can
// publish. Any other owner listens until a roster naming it arrives, then sends
// its AUTH to become a publisher. Publishers send signed chunks. The admin also
// sends ROSTER (the set of publishers) and EOS (end of the channel). The broker
// forwards each accepted chunk to every other receiving stream.
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

	// ErrEndOfStream is returned by Session.Err after the admin ended the channel.
	ErrEndOfStream = errors.New("bps: end of stream")
	// ErrNotPublisher is returned by Publish when the session may not publish
	// the chunk. Either it was not granted, it was revoked, or it is not the
	// admin and the chunk is a ROSTER or EOS.
	ErrNotPublisher = errors.New("bps: session is not a publisher")
)

// ForgingBrokerBlocklistDuration is how long a subscriber blocklists a broker
// that forwarded an invalid chunk. An honest broker never does that.
const ForgingBrokerBlocklistDuration = 10 * 24 * time.Hour

// Options are the broker's resource limits. Zero values take the defaults.
type Options struct {
	MaxSubsPerCohort        int // subscriber streams per cohort
	MaxBrokerCohorts        int // live cohorts per broker
	MaxCohortsPerConnection int // cohorts per peer connection
	MaxPeerStreamsPerCohort int // streams per peer connection per cohort
	MaxPublisherStreams     int // publisher streams per address per cohort
	QueueCap                int // outbound frames per receiving stream
	//
	InactiveTimeout  time.Duration // idle time before a cohort is dropped
	AuthWaitTimeout  time.Duration // time a pending stream has to send its AUTH
	FlushTimeout     time.Duration // time to write out the queue of a closing stream
	HandshakeTimeout time.Duration // time a client's Join waits for the Ack
	//
	ProtoBreachBlocklistDuration time.Duration // blocklist duration for a protocol violation
	InvalidAuthBlocklistDuration time.Duration // blocklist duration for an auth timeout
}

// DefaultOptions are the default resource limits.
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

// Service is the bps protocol service. If broker is set, it also accepts
// inbound joins. Otherwise it only joins remote brokers.
type Service struct {
	broker      bool
	streamer    p2p.Streamer
	blocklister p2p.Blocklister
	opts        Options
	metrics     metrics
	logger      log.Logger

	quit chan struct{} // closed by Close

	mtx     sync.Mutex
	cohorts map[string]*cohort        // keyed by marshalled spec
	peers   map[string]map[string]int // peer overlay -> cohort key -> stream count
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

// Close refuses new joins, drops all cohorts and resets their streams. Calling
// it again does nothing.
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

// Interface is the part of the service used by the API.
type Interface interface {
	// Join joins a cohort at a broker and returns the session.
	Join(ctx context.Context, req JoinRequest) (Session, error)
}

// Message is a verified delivery: a DATA chunk from an allowed publisher, or a
// ROSTER or EOS from the admin.
type Message struct {
	Kind      Kind
	Owner     []byte // signer, the principal for ROSTER and EOS
	SOC       []byte
	Challenge []byte
	Index     uint64
	Roster    [][]byte // ROSTER only: publishers other than the principal
}

// Position is how far a subscriber has read a channel. It is passed from one
// session to the next.
type Position struct {
	Cursors      map[string]uint64 // next expected DATA index, keyed by publisher address
	Roster       [][]byte          // latest accepted roster
	RosterCursor uint64            // next expected ROSTER index
	Ended        bool              // an EOS was received
}

func (p Position) clone() Position {
	p.Cursors = maps.Clone(p.Cursors)
	if p.Cursors == nil {
		p.Cursors = make(map[string]uint64)
	}
	p.Roster = slices.Clone(p.Roster)
	return p
}

// Session is a joined p2p stream to a broker.
type Session interface {
	// Challenge returns the challenge the broker issued for this stream. Every
	// chunk published on the session must use it (see ID).
	Challenge() []byte
	// Messages returns the verified deliveries. The channel is never closed.
	// Use Done to learn when the stream has ended. All deliveries, including a
	// final EOS, are handed over before Done is closed.
	Messages() <-chan Message
	// Position returns how far the session has read, to pass to a later Join.
	Position() Position
	// Granted is closed when the session may send its AUTH. For the principal
	// that is at once. For any other owner it is when a roster naming it
	// arrives on this stream. A stream without an owner is never granted.
	Granted() <-chan struct{}
	// Publish sends a chunk of the given kind and index to the broker. The
	// broker does not reply, so Publish runs the broker's checks first and
	// returns ErrNotPublisher, ErrInvalidMessage, ErrInvalidSOC or
	// ErrInvalidRoster for a chunk the broker would reject.
	Publish(ctx context.Context, kind Kind, index uint64, chunk []byte) error
	// Done is closed when the p2p stream has ended.
	Done() <-chan struct{}
	// Err returns why the stream ended, ErrEndOfStream after an EOS. It is
	// valid once Done is closed.
	Err() error
	// Close closes the stream.
	Close() error
}

// CohortSpec identifies a cohort. The topic binding is always FEED_TOPIC.
type CohortSpec struct {
	Topic     []byte // 32-byte feed topic
	Principal []byte // 20-byte admin address
}

func (c CohortSpec) proto() *pb.CohortSpec {
	return &pb.CohortSpec{Topic: c.Topic, Binding: pb.TopicBinding_FEED_TOPIC, Principal: c.Principal}
}

// JoinRequest describes a cohort join.
type JoinRequest struct {
	Broker   swarm.Address
	Spec     CohortSpec
	Owner    []byte   // 20-byte signer address, nil to only listen
	Position Position // from an earlier session
}

// Join joins a cohort at the broker. After a refused join or a reset stream,
// the caller must back off before joining again.
func (s *Service) Join(ctx context.Context, req JoinRequest) (Session, error) {
	join := &pb.Join{Cohort: req.Spec.proto(), Owner: req.Owner}
	if err := validateJoin(join); err != nil {
		return nil, err
	}
	if req.Position.Ended {
		return nil, errChannelEnded
	}
	// bound the handshake only. The session has its own context.
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

// readLoop reads the broker's frames until the stream ends. It is the only
// place that ends the stream and closes done. Close only cancels the context.
func (s *session) readLoop(stream p2p.Stream, r protobuf.Reader) {
	defer close(s.done)
	defer func() {
		switch {
		case s.ctx.Err() != nil:
			// closed by Close. The cancelled read may still be running, and a
			// stream wakes only one reader, so FullClose would start a second
			// read that may never return. Close does not read, and still lets
			// what we wrote reach the broker.
			_ = stream.Close()
		case s.err != nil && !errors.Is(s.err, ErrEndOfStream):
			_ = stream.Reset()
		default:
			// EOS: no read is running, so FullClose is safe
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
			// invalid chunk: end the stream and blocklist the broker
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

// decode verifies a delivery and returns it as a Message. ROSTER and EOS must
// be signed by the principal. The challenge is not checked because it belongs
// to the publisher's stream, which the subscriber does not know. Frames to
// ignore, such as AUTH, unknown kinds or undecodable rosters, are returned with
// KindUnspecified. An error means the chunk is invalid, which an honest broker
// never forwards.
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

// accept updates the position with a decoded delivery and reports whether to
// pass it on. Old or unrostered DATA and old rosters are dropped.
func (s *session) accept(msg Message) (forward bool) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	switch msg.Kind {
	case KindData:
		// drop, but do not treat as forgery: while a revocation propagates,
		// our roster may be newer than the broker's.
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
		// grant only if the latest roster still names us, so the broker
		// has it too
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

// admits reports whether owner is the principal or on the latest roster.
// Must be called with s.mtx held.
func (s *session) admits(owner []byte) bool {
	return bytes.Equal(owner, s.spec.Principal) || contains(s.pos.Roster, owner)
}

func contains(set [][]byte, a []byte) bool {
	return slices.ContainsFunc(set, func(b []byte) bool { return bytes.Equal(a, b) })
}

// session implements Session. Only readLoop ends it.
type session struct {
	svc       *Service
	broker    swarm.Address
	ctx       context.Context
	cancel    context.CancelFunc // stops the read loop
	wmtx      sync.Mutex
	w         protobuf.Writer
	challenge []byte
	spec      CohortSpec
	owner     []byte
	mtx       sync.Mutex
	pos       Position
	authed    bool          // AUTH was sent
	granted   chan struct{} // closed when AUTH may be sent
	isGranted bool          // granted is closed, used by newSession and the read loop only
	rx        chan Message  // unbuffered, so every delivery is handed over before done closes
	done      chan struct{} // closed when the read loop returns
	err       error         // set before done is closed
}

// newSession returns the session for a joined stream.
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
	// the principal needs no roster to authenticate
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

// Close asks the read loop to end the session. It does not block and is safe
// to call more than once, from any goroutine.
func (s *session) Close() error {
	s.cancel()
	return nil
}

func (s *session) Position() Position {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.pos.clone()
}

// grant closes granted if it is not closed yet. Only newSession and the read
// loop call it.
func (s *session) grant() {
	if !s.isGranted {
		s.isGranted = true
		close(s.granted)
	}
}

// forged blocklists the broker for forwarding an invalid chunk.
func (s *session) forged(err error) {
	if s.svc.blocklister == nil {
		return
	}
	if berr := s.svc.blocklister.Blocklist(s.broker, ForgingBrokerBlocklistDuration, err.Error()); berr != nil {
		s.svc.logger.Error(berr, "blocklist broker failed", "broker", s.broker)
	}
}

// Publish checks a frame as the broker would and sends it. Checking here
// returns an error to the caller instead of getting the stream blocklisted.
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
	// a ROSTER or EOS is not applied here. The broker sends it back, and
	// accept applies it then.
	return nil
}

// check runs the broker's checks on a frame, in the broker's order. First
// whether the stream may send it, then the frame itself.
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
	// the broker resets a stream that sends anything before its AUTH
	if !authed && f.Kind != pb.Kind_AUTH {
		return fmt.Errorf("%s before the auth: %w", f.Kind, ErrInvalidMessage)
	}
	// ROSTER and EOS got here only from the principal, so s.owner is right for every kind
	c, err := verify(f, s.spec.Topic, s.owner)
	if err != nil {
		return fmt.Errorf("check %s: %w", f.Kind, err)
	}
	if f.Kind == pb.Kind_ROSTER {
		return checkRoster(payload(c), s.spec.Principal)
	}
	return nil
}

// checkRoster rejects a roster that does not decode, names the admin, or names
// an address twice. Receivers would ignore these, so the sender is told here.
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

// validateJoin checks the field sizes and the topic binding of a Join. The
// owner must be empty or 20 bytes.
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

// handler handles an inbound join on the broker.
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
	// copy the known fields only, so unknown fields do not change the cohort key
	join.Cohort = &pb.CohortSpec{Topic: join.Cohort.Topic, Binding: join.Cohort.Binding, Principal: join.Cohort.Principal}

	co, m, leave, err := s.join(p.Address, &join)
	if err != nil {
		return s.refuse(ctx, w, stream, err)
	}
	defer leave()

	if err := w.WriteMsgWithContext(ctx, &pb.Ack{Status: pb.Status_OK, Challenge: m.challenge}); err != nil {
		_ = stream.Reset()
		return fmt.Errorf("write ack: %w", err)
	}
	return s.serve(ctx, p.Address, stream, w, r, co, m)
}

// serve runs a reader for the peer's frames and a writer for the stream's
// queue. It returns when either stops or the stream is asked to end. It is the
// only place that ends the stream. On a nil error, the queue was written out
// after a close request, so the stream is closed to let the data arrive. On
// any other error the stream is reset.
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
		// a reset request wins over other ready cases
		select {
		case <-m.resetReq:
			return errResetRequested
		default:
		}
		select {
		case <-m.resetReq:
			return errResetRequested

		case <-closeC:
			// the queue is closed. Give the writer the flush timeout to drain it.
			closeC = nil
			t := time.NewTimer(s.opts.FlushTimeout)
			defer t.Stop()
			flushC = t.C

		case <-flushC:
			return errFlushTimeout

		case <-writeDone:
			// nil if the closed queue was drained
			return writeErr

		case <-readC:
			readC = nil
			if closeC == nil {
				// closing: wait for the writer
				continue
			}
			if errors.Is(readErr, errViolation) && s.blocklister != nil {
				if berr := s.blocklister.Blocklist(peer, s.opts.ProtoBreachBlocklistDuration, readErr.Error()); berr != nil {
					s.logger.Error(berr, "blocklist failed", "peer_address", peer)
				}
			}
			return readErr

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

// read handles the peer's frames until a read fails or a frame is a
// violation.
func (s *Service) read(ctx context.Context, r protobuf.Reader, co *cohort, m *member) error {
	for {
		var f pb.Broadcast
		if err := r.ReadMsgWithContext(ctx, &f); err != nil {
			return fmt.Errorf("read frame: %w", err)
		}
		if err := s.handleFrame(co, m, &f); err != nil {
			return fmt.Errorf("%s frame: %w", f.Kind, err)
		}
	}
}

// write sends the queued frames until ctx ends. It returns nil once the queue
// is closed and drained.
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

// refuse answers a Join with a FULL or REJECTED Ack, depending on cause, and
// ends the stream.
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

// handleFrame checks a frame received by the broker, then applies or drops it.
// A returned error is a protocol violation.
func (s *Service) handleFrame(co *cohort, m *member, f *pb.Broadcast) error {
	principal := co.spec.Principal
	admin := bytes.Equal(m.owner, principal)
	role, named, rosterSent, cursor := co.view(m, f.Kind)

	// 0. drop frames from a closing stream. They were sent before the peer
	// saw the roster or EOS that closed it.
	if role == roleRevoked || role == roleEnded {
		s.dropClosing(co, role)
		return nil
	}
	// an AUTH sent after a roster named the spectator, but a newer roster
	// dropped it. This is a race, not a violation: close the stream quietly.
	if role == roleSpectator && f.Kind == pb.Kind_AUTH && !named && rosterSent {
		co.mtx.Lock()
		defer co.mtx.Unlock()
		s.revokeSpectator(co, m)
		return nil
	}
	// 1. the role must allow the kind (see mayEmit)
	if !mayEmit(role, admin, named, f.Kind) {
		s.count(&co.counters.wrongStream, "wrong_stream")
		return fmt.Errorf("%w: %w", errViolation, errWrongStream)
	}
	// 2. drop unknown kinds
	switch f.Kind {
	case pb.Kind_DATA, pb.Kind_AUTH, pb.Kind_EOS, pb.Kind_ROSTER:
	default:
		s.count(&co.counters.unknownKind, "unknown_kind")
		return nil
	}
	// 3. the challenge must match the stream's
	if !bytes.Equal(f.Challenge, m.challenge) {
		s.count(&co.counters.wrongChallenge, "wrong_challenge")
		return nil
	}
	// 4. drop DATA or ROSTER below the cursor as a retransmit
	if (f.Kind == pb.Kind_DATA || f.Kind == pb.Kind_ROSTER) && f.Index < cursor {
		s.count(&co.counters.retransmit, "retransmit")
		return nil
	}
	// 5. verify the chunk. DATA and AUTH must be signed by the stream's owner,
	// ROSTER and EOS by the principal. The signature check runs without the
	// lock, so apply rechecks the state afterwards.
	owner := m.owner
	if isServiceKind(f.Kind) {
		owner = principal
	}
	c, err := verify(f, co.spec.Topic, owner)
	if err != nil {
		s.count(&co.counters.invalidSOC, "invalid_soc")
		return fmt.Errorf("%w: %w", errViolation, err)
	}
	// 6. drop a ROSTER that does not decode
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

// apply applies a verified frame to the cohort. It runs under one lock. It
// ends streams only by sending non-blocking requests to their handlers.
func (s *Service) apply(co *cohort, m *member, f *pb.Broadcast, roster [][]byte) {
	co.mtx.Lock()
	defer co.mtx.Unlock()

	// revoked or ended since handleFrame's checks
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
			s.countQueueResets(co, co.upgrade(m, s.opts.MaxPublisherStreams))
		}
		// on a publisher stream, AUTH is a heartbeat
		co.inactiveSince = time.Now()

	case pb.Kind_DATA:
		k := string(m.owner)
		if f.Index < co.cursors[k] {
			// another stream of the same owner got there first
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
		// send the EOS to every stream, pending ones too, and close them all.
		// The cohort is removed when the last stream leaves. A Join before
		// that creates a new cohort.
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

// mayEmit reports whether a stream in role r may send kind. Publishers send
// DATA and AUTH, and the admin also ROSTER and EOS. Pending streams send only
// AUTH. Spectators send only AUTH, and only once named on the roster. Unknown
// kinds from a publisher pass here and are dropped later.
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

// dropClosing counts a dropped frame from a closing stream.
func (s *Service) dropClosing(co *cohort, r role) {
	if r == roleEnded {
		s.count(&co.counters.ended, "ended")
		return
	}
	s.count(&co.counters.revoked, "revoked")
}

// countQueueResets counts n streams reset for a full queue.
func (s *Service) countQueueResets(co *cohort, n int) {
	for range n {
		s.count(&co.counters.queueReset, "queue_reset")
	}
}

// revokeSpectator closes a spectator whose AUTH arrived after a roster
// removed it. Must be called with co.mtx held.
func (s *Service) revokeSpectator(co *cohort, m *member) {
	if m.role == roleSpectator {
		m.role = roleRevoked
		co.spectators--
	}
	s.count(&co.counters.revoked, "revoked")
	m.requestClose()
}

// join adds a stream to the cohort, creating the cohort if needed. The
// handler must call the returned leave once, when it returns. An error
// wrapping errFull means a limit was reached.
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
		// an ended cohort is replaced by a new one
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

	// the admin's stream starts pending and does not count against
	// MaxSubsPerCohort. Other streams are spectators and do count. If the
	// spectators are full, a rostered owner still gets in as pending. It
	// receives only the roster until it sends its AUTH, so claiming a
	// rostered address does not get a listener past the limit.
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
		// send a rostered owner the roster, which tells it to send its AUTH
		if !bytes.Equal(join.Owner, join.Cohort.Principal) {
			m.queue <- co.rosterFrame
			m.lastRoster = co.rosterFrame
		}
	} else {
		co.spectators++
		// the latest roster is always the first delivery
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

	// leave undoes join, and removes the cohort when its last stream leaves
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

// removeCohort removes the cohort. Must be called with s.mtx and co.mtx held.
func (s *Service) removeCohort(co *cohort) {
	delete(s.cohorts, co.key)
	co.inactivity.Stop()
	s.metrics.Cohorts.Set(float64(len(s.cohorts)))
}

// closeCohort removes the cohort and resets its streams. Must be called with
// s.mtx and co.mtx held.
func (s *Service) closeCohort(co *cohort) {
	s.removeCohort(co)
	for _, m := range co.members() {
		m.requestReset()
	}
}

// forget closes the cohort if no frame was accepted within InactiveTimeout.
// Otherwise it re-arms the timer.
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

// authExpired reports whether the stream was still pending at its auth
// deadline, and if so marks it for closing.
func (s *Service) authExpired(peer swarm.Address, co *cohort, m *member) bool {
	if !co.expire(m) {
		return false
	}
	s.count(&co.counters.authTimeout, "auth_timeout")
	// blocklist only the admin. A rostered owner that got in past a full
	// cohort and never authenticated cost no more than a FULL refusal.
	if s.blocklister != nil && bytes.Equal(m.owner, co.spec.Principal) {
		if err := s.blocklister.Blocklist(peer, s.opts.InvalidAuthBlocklistDuration, "bps auth timeout"); err != nil {
			s.logger.Debug("blocklist failed", "peer_address", peer, "error", err)
		}
	}
	return true
}

// Counters returns the counters of the cohort, if it exists.
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
	roleSpectator role = iota + 1 // receives, sends only AUTH once rostered
	rolePending                   // admin, or rostered owner past a full cohort, waiting for AUTH
	rolePublisher                 // sent AUTH, publishes and receives
	roleRevoked                   // removed or timed out, closing
	roleEnded                     // cohort ended by EOS, closing
)

// receives reports whether streams in role r get deliveries.
func (r role) receives() bool { return r == roleSpectator || r == rolePublisher }

type cohort struct {
	key        string
	spec       *pb.CohortSpec
	inactivity *time.Timer
	counters   counters

	mtx           sync.Mutex
	cursors       map[string]uint64 // next expected DATA index, keyed by publisher
	roster        [][]byte          // latest roster, without the principal
	rosterCursor  uint64            // next expected ROSTER index
	rosterFrame   *pb.Broadcast     // latest ROSTER frame, sent first to new streams
	inactiveSince time.Time
	attached      map[*member]struct{}
	spectators    int    // number of spectator streams
	authSeq       uint64 // orders publisher streams by when they sent AUTH
	ended         bool   // EOS was accepted
}

// newCohort returns an empty cohort. The caller sets the inactivity timer.
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

// view returns, under the lock, the state handleFrame checks before verifying
// the signature.
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

// expire revokes the stream if it is still pending, so a late AUTH is not
// accepted. It reports whether it did.
func (co *cohort) expire(m *member) bool {
	co.mtx.Lock()
	defer co.mtx.Unlock()
	if m.role != rolePending {
		return false
	}
	m.role = roleRevoked
	return true
}

// deliver queues the frame on every receiving stream and returns how many were
// reset for a full queue. DATA skips streams of the sender's owner. ROSTER and
// EOS go back to the sender too, so it sees where in the stream they took
// effect. Queueing does not block, so a slow receiver cannot stall the cohort.
// Must be called with co.mtx held.
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

// upgrade makes a pending or spectator stream a publisher. If the owner now
// has more than maxStreams publisher streams, the oldest are reset. It returns
// 1 if the stream's queue was too full for the roster, else 0. Must be called
// with co.mtx held.
func (co *cohort) upgrade(m *member, maxStreams int) (fullQueues int) {
	if m.role == rolePending {
		// send the latest roster first, unless the stream already has it
		if co.rosterFrame != nil && m.lastRoster != co.rosterFrame && !m.enqueue(co.rosterFrame) && m.requestReset() {
			fullQueues++
		}
	} else {
		// a spectator already has the roster
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

// applyRoster sets the cohort's roster and delivers it. Streams of owners it
// removes get the roster, then are closed. Their cursors are deleted. It
// returns how many streams were reset for a full queue. Must be called with
// co.mtx held.
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
			// delivered above, it may now send its AUTH
			m.rosterSent = true
		case m.role == rolePending && co.rostered(m.owner):
			// pending streams are skipped by deliver
			m.lastRoster = f
			if !m.enqueue(f) && m.requestReset() {
				fullQueues++
			}
		case m.role == rolePending:
			// removed before its AUTH: send the roster, then close
			m.role = roleRevoked
			if !m.enqueue(f) {
				if m.requestReset() {
					fullQueues++
				}
				continue
			}
			m.requestClose()
		case m.role == rolePublisher && !co.rostered(m.owner):
			// removed: close after the roster delivered above is written
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

// member is a stream in a cohort. Only its handler closes or resets the
// stream. Others ask with requestClose or requestReset. The fields from role
// on are guarded by cohort.mtx.
type member struct {
	owner        []byte // nil if the stream only listens
	challenge    []byte
	authDeadline bool               // joined as pending, so the auth timeout applies
	queue        chan *pb.Broadcast // closed by requestClose
	closeReq     chan struct{}      // drain the queue, then close
	resetReq     chan struct{}      // reset now, wins over closeReq

	role       role
	authSeq    uint64
	rosterSent bool          // spectator was sent a roster naming its owner
	lastRoster *pb.Broadcast // last roster queued on a pending stream
	resetting  bool          // reset requested, queue nothing more
	closing    bool          // queue closed, queue nothing more
}

// newMember returns a member for a stream joining in role r.
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

// requestClose closes the queue and asks the handler to drain it and close the
// stream. It does not block, and later calls do nothing. Must be called with
// cohort.mtx held.
func (m *member) requestClose() {
	if m.closing {
		return
	}
	m.closing = true
	close(m.queue)
	request(m.closeReq)
}

// requestReset asks the handler to reset the stream now and stops further
// queueing. It does not block. It reports whether this was the first request.
// Must be called with cohort.mtx held.
func (m *member) requestReset() bool {
	if m.resetting {
		return false
	}
	m.resetting = true
	request(m.resetReq)
	return true
}

// request does a non-blocking send on a one-slot channel. The channel is
// never closed, so repeated requests cannot panic.
func request(c chan struct{}) {
	select {
	case c <- struct{}{}:
	default:
	}
}

// enqueue adds a frame to the queue and reports false if the queue is full.
// On a closing stream it drops the frame and reports true. Must be called with
// cohort.mtx held.
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
