// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"errors"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "bps"

// Wire identity. SWIP-60 names the libp2p stream "pubsub/1.0.0"; bee composes
// the protocol id as /swarm/{name}/{version}/{stream}.
const (
	ProtocolName    = "pubsub"
	ProtocolVersion = "1.0.0"
	StreamName      = "pubsub"
)

const (
	// DefaultCapacity is the per-topic stream limit a broker enforces when
	// none is configured. Capacity is broker policy, never a cohort parameter:
	// a cohort cannot dictate a remote node's connection count.
	DefaultCapacity = 32
	// HandshakeTimeout bounds how long a fresh stream may take to send Hello.
	HandshakeTimeout = 30 * time.Second
	// OutboundQueueSize bounds a peer stream's pending broadcasts. A peer that
	// fills it is reset rather than allowed to stall the cohort.
	OutboundQueueSize = 64
	// DedupCacheSize bounds a cohort's dedup horizon. SWIP-60 fixes the dedup
	// rule but not its horizon; unbounded is a memory exhaustion vector.
	DedupCacheSize = 1024
	// DefaultMaxCohorts is the number of cohorts a broker will fix when none
	// is configured. Cohorts are never reclaimed, so this is what bounds the
	// registry a remote peer can grow by opening topics.
	DefaultMaxCohorts = 128
)

var (
	// ErrCohortFull is returned when a broker is at its per-topic capacity.
	// A singlehop broker refuses and does nothing else: referral to another
	// attachment point belongs to bps-multihop.
	ErrCohortFull = errors.New("bps: cohort at capacity")
	// ErrUnknownTopic is returned for a Subscribe naming a topic the broker
	// does not serve.
	ErrUnknownTopic = errors.New("bps: unknown topic")
	// ErrSpecMismatch is returned for an Open naming an open topic with a
	// different spec.
	ErrSpecMismatch = errors.New("bps: cohort spec mismatch")
	// ErrClosedCohort is returned when a non-publisher tries to join a closed
	// cohort.
	ErrClosedCohort = errors.New("bps: closed cohort admits publishers only")
	// ErrShutdown is returned once the service is closing.
	ErrShutdown = errors.New("bps: shutting down")
)

// Options configures Service at construction.
type Options struct {
	// Capacity is the per-topic stream limit this broker enforces.
	// Zero means DefaultCapacity.
	Capacity int
	// MaxCohorts is the number of distinct cohorts this broker will fix.
	// Zero means DefaultMaxCohorts. An Open that would exceed it is refused
	// with FULL; joining a cohort that already exists is never affected.
	MaxCohorts int
}

// Service implements the BPS protocol. A node is a broker for the topics in
// its cohort registry and a client for the topics in its session map; the two
// are independent, and a node may be both for different topics.
type Service struct {
	streamer   p2p.Streamer
	logger     log.Logger
	metrics    metrics
	capacity   int
	maxCohorts int

	cohortsMu sync.Mutex
	// cohorts never lose entries once created: SWIP-60's cohort outlives its
	// opener, and the last peer leaving does not destroy it. Reclamation of
	// abandoned cohorts is future work, not an oversight.
	cohorts map[string]*cohort

	sessionsMu sync.Mutex
	sessions   map[*Session]struct{}

	quit     chan struct{}
	quitOnce sync.Once
}

// New returns a new BPS service. The streamer may be nil for a node that only
// brokers and never dials.
func New(streamer p2p.Streamer, logger log.Logger, o Options) *Service {
	capacity := o.Capacity
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	maxCohorts := o.MaxCohorts
	if maxCohorts <= 0 {
		maxCohorts = DefaultMaxCohorts
	}
	return &Service{
		streamer:   streamer,
		logger:     logger.WithName(loggerName).Register(),
		metrics:    newMetrics(),
		capacity:   capacity,
		maxCohorts: maxCohorts,
		cohorts:    make(map[string]*cohort),
		sessions:   make(map[*Session]struct{}),
		quit:       make(chan struct{}),
	}
}

// Protocol returns the p2p protocol specification for registration with the
// p2p service.
func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    ProtocolName,
		Version: ProtocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    StreamName,
				Handler: s.handler,
			},
		},
	}
}

// Topics returns the topics this node brokers.
func (s *Service) Topics() []swarm.Address {
	s.cohortsMu.Lock()
	defer s.cohortsMu.Unlock()

	out := make([]swarm.Address, 0, len(s.cohorts))
	for t := range s.cohorts {
		out = append(out, swarm.NewAddress([]byte(t)))
	}
	return out
}

// TopicStatus describes one topic this node participates in, for the API's
// GET /pubsub listing.
type TopicStatus struct {
	Topic  swarm.Address
	Spec   *pb.CohortSpec
	Broker bool // true: this node brokers the topic; false: client session
	Peers  int  // broker: retained streams; client: 1 (the broker link)
}

// Status enumerates every topic this node participates in, brokered and
// client alike. A node that is both broker and client for the same topic
// yields two entries, one for each role.
func (s *Service) Status() []TopicStatus {
	// Sized from the cohorts alone: len(s.sessions) is guarded by sessionsMu,
	// and reading it here, under cohortsMu, races a concurrent Session.Close.
	// The session entries are appended below and grow the slice as needed.
	s.cohortsMu.Lock()
	out := make([]TopicStatus, 0, len(s.cohorts))
	for t, c := range s.cohorts {
		out = append(out, TopicStatus{
			Topic:  swarm.NewAddress([]byte(t)),
			Spec:   c.spec,
			Broker: true,
			Peers:  c.count(),
		})
	}
	s.cohortsMu.Unlock()

	s.sessionsMu.Lock()
	for ss := range s.sessions {
		out = append(out, TopicStatus{
			Topic:  ss.Topic(),
			Spec:   ss.Spec(),
			Broker: false,
			Peers:  1,
		})
	}
	s.sessionsMu.Unlock()

	return out
}

// Close stops the service and waits for its goroutines to terminate.
//
// Close does not itself track broker-side goroutines: each admitted stream's
// serve call owns its own reader and waits for it before returning (see
// broker.go), so nothing here needs to join that goroutine — it belongs to
// whichever p2p layer dispatched the stream's handler, not to Service. What
// Close does own is every live client Session, and Session.Close is
// synchronous with its own read loop, so closing every live session here
// transitively waits for all of them too.
func (s *Service) Close() error {
	// quitOnce, not a bare select-on-quit/default, because Close is exported
	// and callers do call it more than once: two concurrent calls could both
	// take the default branch and both close(s.quit), which panics. Once
	// makes the transition itself safe under concurrent Close calls; only
	// the call that actually closes quit runs teardown, and every later
	// call returns nil immediately, same as before.
	first := false
	s.quitOnce.Do(func() {
		first = true
		close(s.quit)
	})
	if !first {
		return nil
	}

	s.sessionsMu.Lock()
	live := make([]*Session, 0, len(s.sessions))
	for ss := range s.sessions {
		live = append(live, ss)
	}
	s.sessionsMu.Unlock()

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for _, ss := range live {
			_ = ss.Close()
		}
	}()

	select {
	case <-stopped:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("bps: waited 5 seconds to close active goroutines")
	}
}

// statusOf maps a handshake error to the wire status a broker answers with.
func statusOf(err error) pb.Status {
	switch {
	case err == nil:
		return pb.Status_OK
	case errors.Is(err, ErrCohortFull):
		return pb.Status_FULL
	case errors.Is(err, ErrUnknownTopic):
		return pb.Status_UNKNOWN_TOPIC
	default:
		return pb.Status_REJECTED
	}
}
