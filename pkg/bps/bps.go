// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bps implements BPS-lite (SWIP-74): a single-publisher live stream
// over a feed, through one broker, one hop.
//
// A cohort is identified by its spec {topic, FEED_TOPIC, principal}. Every
// peer joins with the spec and its own identity and is answered with a random
// per-stream challenge. A stream declaring the principal is pending until its
// AUTH — the empty chunk at index 0 of the session's AUTH feed — authenticates
// it; a publisher stream then sends ordinary single-owner chunks that are updates
// of the session feed on keccak256(topic | challenge). The broker keeps one cursor
// per cohort and delivers every accepted update to every subscriber stream.
package bps

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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
)

// Options are the broker's resource bounds. Zero values take the defaults.
type Options struct {
	MaxSubsPerCohort        int // subscriber streams per cohort
	MaxBrokerCohorts        int // live cohorts per broker
	MaxCohortsPerConnection int // cohorts per peer connection
	MaxPeerStreamsPerCohort int // streams per peer connection per cohort
	QueueCap                int // outbound frames per subscriber stream
	//
	InactiveTimeout time.Duration // reclaims a cohort with no accepted frame
	AuthWaitTimeout time.Duration // disconnects a pending stream that has not authenticated
	//
	ProtoBreachBlocklistDuration time.Duration // blocklist duration for a protocol violation
	InvalidAuthBlocklistDuration time.Duration // blocklist duration for an auth timeout
}

// DefaultOptions are the SWIP-74 recommended bounds.
var DefaultOptions = Options{
	MaxSubsPerCohort:             1024,
	MaxBrokerCohorts:             512,
	MaxCohortsPerConnection:      16,
	MaxPeerStreamsPerCohort:      2,
	InactiveTimeout:              10 * time.Minute,
	AuthWaitTimeout:              30 * time.Second,
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
	if o.InactiveTimeout > 0 {
		d.InactiveTimeout = o.InactiveTimeout
	}
	if o.AuthWaitTimeout > 0 {
		d.AuthWaitTimeout = o.AuthWaitTimeout
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

// Message is a verified DATA delivery: the chunk and the frame fields its id
// was derived from.
type Message struct {
	SOC       []byte
	Challenge []byte
	Index     uint64
}

// Session is a single joined p2p stream to a broker.
type Session interface {
	// Challenge is the challenge the broker issued for this stream. Every
	// chunk published on the session must be signed under it (see ID).
	Challenge() []byte
	// Messages yields the DATA deliveries that verify against the cohort spec
	// and the session cursor. The channel is closed when the stream ends.
	Messages() <-chan Message
	// Cursor is the lowest DATA index the session will deliver next.
	Cursor() uint64
	// Publish writes a chunk of the given kind and index, signed under the
	// session challenge, to the broker. The broker does not reply. Only a
	// session whose identity is the cohort principal may publish.
	Publish(ctx context.Context, kind Kind, index uint64, soc []byte) error
	// Done is closed when the p2p stream ends.
	Done() <-chan struct{}
	// Err reports why the stream ended. Valid after Done is closed.
	Err() error
	// Close closes the stream.
	Close() error
}

// CohortSpec names a cohort: the feed of the principal on the topic. The
// topic binding is always FEED_TOPIC.
type CohortSpec struct {
	Topic     []byte // 32 bytes: the feed topic
	Principal []byte // 20 bytes: the publisher
}

func (c CohortSpec) proto() *pb.CohortSpec {
	return &pb.CohortSpec{Topic: c.Topic, Binding: pb.TopicBinding_FEED_TOPIC, Principal: c.Principal}
}

// JoinRequest describes a cohort join.
type JoinRequest struct {
	Broker   swarm.Address // the broker
	Spec     CohortSpec    // the cohort to join
	Identity []byte        // 20 bytes: the joining stream's identity
	Cursor   uint64        // the lowest DATA index to deliver, the subscriber's cursor for the cohort
}

// Join joins a cohort at the broker. A peer whose join was refused or whose
// stream was reset must back off before rejoining.
func (s *Service) Join(ctx context.Context, req JoinRequest) (Session, error) {
	join := &pb.Join{Cohort: req.Spec.proto(), Identity: req.Identity}
	if err := validateJoin(join); err != nil {
		return nil, err
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
		publisher: bytes.Equal(req.Identity, req.Spec.Principal),
		rx:        make(chan Message),
		done:      make(chan struct{}),
	}
	sess.cursor.Store(req.Cursor)

	go sess.readLoop(stream, r, req.Spec)

	return sess, nil
}

// readLoop delivers the broker's frames until the stream ends.
func (s *session) readLoop(stream p2p.Stream, r protobuf.Reader, spec CohortSpec) {
	var readErr error
	defer func() {
		s.finish(readErr)
		if readErr != nil && s.ctx.Err() == nil {
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
		// a retransmit below the cursor is skipped. The challenge is not
		// compared: it is the publisher stream's, which the subscriber never sees.
		if f.Kind != pb.Kind_DATA || f.Index < s.cursor.Load() {
			continue
		}
		// re-verify end to end: a broker that forwards a chunk not valid
		// for the spec is misbehaving, so the stream is ended
		if err := verify(&f, spec.Topic, spec.Principal); err != nil {
			readErr = fmt.Errorf("%w: %w", errInvalidDelivery, err)
			return
		}
		s.cursor.Store(f.Index + 1)
		select {
		case s.rx <- Message{SOC: f.Soc, Challenge: f.Challenge, Index: f.Index}:
		case <-s.ctx.Done():
			return
		}
	}
}

// session is the concrete Session implementation.
type session struct {
	ctx       context.Context
	cancel    context.CancelFunc
	wmtx      sync.Mutex
	w         protobuf.Writer
	challenge []byte
	publisher bool // the identity is the cohort principal
	cursor    atomic.Uint64
	rx        chan Message
	done      chan struct{}
	err       error
	once      sync.Once
}

func (s *session) Challenge() []byte        { return s.challenge }
func (s *session) Messages() <-chan Message { return s.rx }
func (s *session) Cursor() uint64           { return s.cursor.Load() }
func (s *session) Done() <-chan struct{}    { return s.done }
func (s *session) Err() error               { return s.err }
func (s *session) Close() error {
	s.cancel()
	return nil
}

func (s *session) Publish(ctx context.Context, kind Kind, index uint64, soc []byte) error {
	if !s.publisher {
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
		return s.refuse(ctx, w, stream, pb.Status_REJECTED, err)
	}
	// re-marshal so that the cohort key ignores fields BPS-lite does not define
	join.Cohort = &pb.CohortSpec{Topic: join.Cohort.Topic, Binding: join.Cohort.Binding, Principal: join.Cohort.Principal}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	co, m, err := s.join(p.Address, &join, func() {
		cancel()
		_ = stream.Reset()
	})
	if err != nil {
		status := pb.Status_REJECTED
		if errors.Is(err, errFull) {
			status = pb.Status_FULL
		}
		return s.refuse(ctx, w, stream, status, err)
	}
	// detach on every exit, normal or not
	defer s.leave(p.Address, co, m)

	if err := w.WriteMsgWithContext(ctx, &pb.Ack{Status: pb.Status_OK, Challenge: m.challenge}); err != nil {
		m.reset()
		return fmt.Errorf("write ack: %w", err)
	}

	// the writer side: deliveries to a subscriber stream
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

// refuse answers a Join with a non-OK Ack, which ends the stream.
func (s *Service) refuse(ctx context.Context, w protobuf.Writer, stream p2p.Stream, status pb.Status, cause error) error {
	if err := w.WriteMsgWithContext(ctx, &pb.Ack{Status: status}); err != nil {
		_ = stream.Reset()
		return fmt.Errorf("write ack: %w", err)
	}
	_ = stream.FullClose()
	s.logger.Debug("join refused", "status", status, "error", cause)
	return nil
}

// handleFrame validates a Broadcast arriving at the broker per SWIP-74 and
// accepts, drops or punishes it. A returned error resets the stream.
func (s *Service) handleFrame(co *cohort, m *member, f *pb.Broadcast) error {
	co.mtx.Lock()
	role, cursor := m.role, co.cursor
	co.mtx.Unlock()

	// 1. a publisher stream sends publications and heartbeats, a pending stream
	// its AUTH; anything else is not what the stream may send
	if role == roleSubscriber || role == rolePending && f.Kind != pb.Kind_AUTH {
		s.count(&co.counters.wrongStream, "wrong_stream")
		return fmt.Errorf("%w: %w", errViolation, errWrongStream)
	}
	// 2. a kind BPS-lite does not define is dropped
	if f.Kind != pb.Kind_DATA && f.Kind != pb.Kind_AUTH {
		s.count(&co.counters.unknownKind, "unknown_kind")
		return nil
	}
	// 3. the challenge must be the stream's
	if !bytes.Equal(f.Challenge, m.challenge) {
		s.count(&co.counters.wrongChallenge, "wrong_challenge")
		return nil
	}
	// 4. a DATA frame below the cursor is a retransmit
	if f.Kind == pb.Kind_DATA && f.Index < cursor {
		s.count(&co.counters.retransmit, "retransmit")
		return nil
	}
	// 5. the chunk must be the principal's at the id derived from the frame;
	// an AUTH is moreover the empty chunk at index 0
	if err := verify(f, co.spec.Topic, co.spec.Principal); err != nil {
		s.count(&co.counters.invalidSOC, "invalid_soc")
		return fmt.Errorf("%w: %w", errViolation, err)
	}

	var overflow []*member
	co.mtx.Lock()
	switch {
	case f.Kind == pb.Kind_AUTH:
		// the auth of a pending stream, a heartbeat on a publisher stream
		if m.role == rolePending {
			m.role = rolePublisher
			m.authTimer.Stop()
		}
		co.inactiveSince = time.Now()
	case f.Index < co.cursor:
		// raced with another publisher stream of the principal's
		s.count(&co.counters.retransmit, "retransmit")
	default:
		co.cursor = f.Index + 1
		co.inactiveSince = time.Now()
		s.metrics.Delivered.Inc()
		for sub := range co.members {
			if sub.role != roleSubscriber {
				continue
			}
			select {
			case sub.queue <- f:
			default:
				s.count(&co.counters.queueReset, "queue_reset")
				overflow = append(overflow, sub)
			}
		}
	}
	co.mtx.Unlock()
	for _, sub := range overflow {
		sub.reset()
	}
	return nil
}

// join attaches a stream to the cohort named by the spec, creating the
// cohort if no live one has it. An error wrapping errFull means a capacity
// bound was hit.
func (s *Service) join(peer swarm.Address, join *pb.Join, reset func()) (*cohort, *member, error) {
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

	role := roleSubscriber
	if bytes.Equal(join.Identity, join.Cohort.Principal) {
		role = rolePending
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
			inactiveSince: time.Now(),
		}
		co.inactivity = time.AfterFunc(s.opts.InactiveTimeout, func() { s.reclaim(co) })
		s.cohorts[k] = co
		s.metrics.Cohorts.Set(float64(len(s.cohorts)))
	}

	co.mtx.Lock()
	defer co.mtx.Unlock()
	if role == roleSubscriber && co.subscribers >= s.opts.MaxSubsPerCohort {
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
		challenge: challenge,
		queue:     make(chan *pb.Broadcast, s.opts.QueueCap),
	}
	var once sync.Once
	m.reset = func() { once.Do(reset) }
	if role == rolePending {
		m.authTimer = time.AfterFunc(s.opts.AuthWaitTimeout, func() { s.authTimeout(peer, co, m) })
	} else {
		co.subscribers++
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
	if m.role == roleSubscriber {
		co.subscribers--
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
	roleSubscriber role = iota + 1 // receives, never publishes
	rolePending                    // declared the principal as identity, no AUTH yet
	rolePublisher                  // authenticated by its AUTH: may publish, receives nothing
)

type cohort struct {
	key        string
	spec       *pb.CohortSpec
	inactivity *time.Timer
	counters   counters

	mtx           sync.Mutex
	cursor        uint64 // the lowest DATA index accepted next
	inactiveSince time.Time
	members       map[*member]struct{}
	subscribers   int
}

// shutdown returns the cohort's streams, for the caller to reset.
func (co *cohort) shutdown() []*member {
	co.mtx.Lock()
	defer co.mtx.Unlock()
	ms := make([]*member, 0, len(co.members))
	for m := range co.members {
		ms = append(ms, m)
	}
	return ms
}

// member is one stream attached to a cohort. role is guarded by cohort.mtx.
type member struct {
	role      role
	challenge []byte
	queue     chan *pb.Broadcast
	reset     func()
	authTimer *time.Timer
}
