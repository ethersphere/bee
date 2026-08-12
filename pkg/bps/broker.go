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
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// cohort is one topic's brokered state. All fields are guarded by
// Service.cohortsMu except peers and dedup, which have their own mutex so
// fan-out does not block the handshake path.
type cohort struct {
	spec    *pb.CohortSpec
	binding binding

	mu    sync.Mutex
	peers map[*peerStream]struct{}
	dedup map[string]struct{}
	order [][]byte // insertion order, for evicting the oldest dedup entry
}

func newCohort(spec *pb.CohortSpec, b binding) *cohort {
	return &cohort{
		spec:    spec,
		binding: b,
		peers:   make(map[*peerStream]struct{}),
		dedup:   make(map[string]struct{}),
	}
}

// peerStream is one retained stream in a cohort, with a bounded outbound queue
// drained by a single writer goroutine.
type peerStream struct {
	peer      swarm.Address
	publisher bool
	out       chan *pb.Soc
	quit      chan struct{}
	closeOnce sync.Once
}

func newPeerStream(peer swarm.Address, publisher bool) *peerStream {
	return &peerStream{
		peer:      peer,
		publisher: publisher,
		out:       make(chan *pb.Soc, OutboundQueueSize),
		quit:      make(chan struct{}),
	}
}

func (ps *peerStream) close() {
	ps.closeOnce.Do(func() { close(ps.quit) })
}

func (c *cohort) add(ps *peerStream) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.peers[ps] = struct{}{}
}

func (c *cohort) remove(ps *peerStream) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.peers, ps)
}

func (c *cohort) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.peers)
}

// seen reports whether key was already broadcast in this cohort, recording it
// if not. The horizon is bounded: SWIP-60 fixes the dedup rule but not its
// size, and an unbounded set is a memory exhaustion vector. A bounded horizon
// is a memory bound, not a replay defence — replay defence arrives with
// history delivery.
func (c *cohort) seen(key []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.dedup[string(key)]; ok {
		return true
	}
	if len(c.order) >= DedupCacheSize {
		delete(c.dedup, string(c.order[0]))
		c.order = c.order[1:]
	}
	c.dedup[string(key)] = struct{}{}
	c.order = append(c.order, key)

	return false
}

// fanout enqueues m on every peer stream and returns those whose queue was
// full. A peer that cannot keep up is reset rather than allowed to stall the
// cohort: SWIP-60 gives the broker no delivery obligation, and withholding is
// a liveness fault the peer recovers from by reconnecting.
func (c *cohort) fanout(m *pb.Soc) []*peerStream {
	c.mu.Lock()
	defer c.mu.Unlock()

	var dropped []*peerStream
	for ps := range c.peers {
		select {
		case ps.out <- m:
		default:
			dropped = append(dropped, ps)
		}
	}
	return dropped
}

// handler serves an inbound BPS stream. The first frame is a Hello wrapping
// Open or Subscribe; the broker answers with Ack. A successful handshake
// hands the stream to serve, which retains it for the cohort's lifetime and
// becomes its sole owner for teardown from that point on — handler must not
// touch the stream again once serve has been called.
func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	select {
	case <-s.quit:
		_ = stream.Reset()
		return ErrShutdown
	default:
	}

	w, r := protobuf.NewWriterAndReader(stream)

	helloCtx, cancel := context.WithTimeout(ctx, HandshakeTimeout)
	var hello pb.Hello
	err := r.ReadMsgWithContext(helloCtx, &hello)
	cancel()
	if err != nil {
		_ = stream.Reset()
		return fmt.Errorf("read hello: %w", err)
	}

	c, ps, joinErr := s.join(&hello, p.Address)
	status := statusOf(joinErr)
	s.metrics.Handshakes.WithLabelValues(status.String()).Inc()
	if joinErr != nil {
		s.logger.Debug("handshake refused", "peer_address", p.Address, "status", status, "error", joinErr)
	}

	ack := &pb.Ack{Status: status}
	if joinErr == nil {
		ack.Cohort = c.spec
	}
	// The Ack write is bounded by the same budget as the Hello read: a peer
	// that opens a stream, says Hello and then never reads would otherwise
	// hold this handler goroutine until it disconnects.
	ackCtx, ackCancel := context.WithTimeout(ctx, HandshakeTimeout)
	err = w.WriteMsgWithContext(ackCtx, ack)
	ackCancel()
	if err != nil {
		if joinErr == nil {
			// Admitted but never got to serve: undo the registration admit
			// made under cohortsMu, since serve will never run to do it.
			c.remove(ps)
		}
		_ = stream.Reset()
		return fmt.Errorf("write ack: %w", err)
	}
	if joinErr != nil {
		_ = stream.FullClose()
		return nil
	}

	return s.serve(ctx, p, c, ps, stream, w, r)
}

// serve retains an admitted stream for the cohort's lifetime: this goroutine
// drains ps's outbound queue and writes broadcasts, while a second goroutine
// — for publishers — reads Publish frames. serve is the sole owner of the
// stream's teardown once handler hands it off: whichever condition ends the
// session, serve resets the stream itself and waits for its own reader
// goroutine to actually return before returning, so no goroutine started
// here outlives serve, and nothing else — in particular not handler's own
// caller — ever closes this stream concurrently with serve.
func (s *Service) serve(ctx context.Context, p p2p.Peer, c *cohort, ps *peerStream, stream p2p.Stream, w protobuf.Writer, r protobuf.Reader) error {
	defer func() {
		c.remove(ps)
		ps.close()
	}()

	// The write below returns only on completion or on cancellation of the
	// context it is given, and ctx is the libp2p per-stream context, cancelled
	// only when the peer disconnects or p2p shuts down. A peer that holds the
	// connection open but stops draining its flow-control window would
	// therefore park this goroutine inside the write, past ps.quit, past
	// s.quit and past the slow-peer reset, leaving Service.Close to return nil
	// while this stream and its goroutines leak. writeCtx bridges both quit
	// channels into cancellation so the write is actually interruptible.
	//
	// The bridging goroutine always terminates: every path out of serve runs
	// the deferred cancelWrite, which completes writeCtx.Done() and so ends
	// the select even when neither quit channel ever closes.
	writeCtx, cancelWrite := context.WithCancel(ctx)
	bridgeDone := make(chan struct{})
	go func() {
		defer close(bridgeDone)
		select {
		case <-ps.quit:
			cancelWrite()
		case <-s.quit:
			cancelWrite()
		case <-writeCtx.Done():
		}
	}()
	defer func() {
		cancelWrite()
		<-bridgeDone
	}()

	var readerDone chan struct{}
	if ps.publisher {
		readerDone = make(chan struct{})
		go func() {
			defer close(readerDone)
			defer ps.close()
			s.readPublished(p, c, r)
		}()
	}

	var loopErr error
loop:
	for {
		select {
		case m := <-ps.out:
			if err := w.WriteMsgWithContext(writeCtx, &pb.Broadcast{Frame: &pb.Broadcast_Soc{Soc: m}}); err != nil {
				// A write cut short by our own shutdown or by this peer's
				// reset is not a failure to report; only a genuine write
				// error is.
				select {
				case <-ps.quit:
				case <-s.quit:
				default:
					loopErr = fmt.Errorf("write broadcast: %w", err)
				}
				break loop
			}
		case <-ps.quit:
			break loop
		case <-s.quit:
			break loop
		case <-ctx.Done():
			loopErr = ctx.Err()
			break loop
		}
	}

	// Reset first, to unblock a reader goroutine that may be mid-ReadMsg on
	// this same stream, then wait for it to actually stop. A real libp2p
	// stream's FullClose reads from the stream to observe the peer's own
	// close (pkg/p2p/libp2p/stream.go), so calling it here while the reader
	// might still be blocked in a read would be two concurrent readers of
	// one connection; Reset carries no such obligation, which is why it —
	// never FullClose — is what ends a retained stream.
	_ = stream.Reset()
	if readerDone != nil {
		<-readerDone
	}
	return loopErr
}

// readPublished consumes a publisher's Publish frames, validates each against
// the cohort's binding and publisher regime, deduplicates, and fans out.
func (s *Service) readPublished(p p2p.Peer, c *cohort, r protobuf.Reader) {
	for {
		var msg pb.Publish
		if err := r.ReadMsg(&msg); err != nil {
			s.logger.Debug("read publish", "peer_address", p.Address, "error", err)
			return
		}
		s.publish(p, c, msg.GetSoc())
	}
}

func (s *Service) publish(p p2p.Peer, c *cohort, m *pb.Soc) {
	reason, err := s.validate(c, m)
	if err != nil {
		s.metrics.Dropped.WithLabelValues(reason).Inc()
		s.metrics.Invalid.Inc()
		// Per-peer attribution stays in the debug log, deliberately. Labelling
		// a metric by peer address lets any remote peer mint unbounded
		// Prometheus time series, and no other bee metric does it. The
		// blocklisting policy this was meant to feed wants per-peer state in
		// memory, not in the metrics registry.
		s.logger.Debug("dropping message", "peer_address", p.Address, "reason", reason, "error", err)
		return
	}

	for _, ps := range c.fanout(m) {
		s.metrics.Dropped.WithLabelValues("slow_peer").Inc()
		s.logger.Debug("resetting slow peer", "peer_address", ps.peer)
		// Unregister immediately: otherwise this peer stays in c.peers,
		// visible to and re-dropped by, every fanout until its own serve
		// call unwinds and removes it.
		c.remove(ps)
		ps.close()
	}
	s.metrics.Broadcast.Inc()
}

// validate runs the broker checks SWIP-60 requires on Publish: the SOC is
// well-formed and its owner matches its signature, it qualifies under the
// topic binding, its owner is a legitimate publisher, and it is not a
// duplicate. It returns a metric label alongside the error.
func (s *Service) validate(c *cohort, m *pb.Soc) (string, error) {
	if m == nil {
		return "malformed", ErrMalformedSoc
	}
	sc, err := SocFromProto(m)
	if err != nil {
		return "malformed", err
	}
	if err := c.binding.qualifies(c.spec, sc); err != nil {
		return "unqualified", err
	}
	if err := authorizePublisher(c.spec, sc.OwnerAddress()); err != nil {
		return "not_publisher", err
	}
	key, err := c.binding.dedupKey(sc)
	if err != nil {
		return "malformed", err
	}
	if c.seen(key) {
		return "duplicate", errors.New("bps: duplicate message")
	}
	return "", nil
}

// join resolves the handshake against the cohort registry, creating the
// cohort when the frame is an Open for an unserved topic. On success it
// returns the peer's newly registered peerStream.
func (s *Service) join(hello *pb.Hello, peer swarm.Address) (*cohort, *peerStream, error) {
	switch {
	case hello.GetOpen() != nil:
		return s.open(hello.GetOpen(), peer)
	case hello.GetSubscribe() != nil:
		return s.subscribe(hello.GetSubscribe(), peer)
	default:
		return nil, nil, fmt.Errorf("empty hello: %w", ErrInvalidSpec)
	}
}

func (s *Service) open(open *pb.Open, peer swarm.Address) (*cohort, *peerStream, error) {
	spec := open.GetCohort()
	if err := ValidateSpec(spec); err != nil {
		return nil, nil, err
	}
	b, err := bindingFor(spec.GetBinding())
	if err != nil {
		return nil, nil, err
	}

	s.cohortsMu.Lock()
	defer s.cohortsMu.Unlock()

	key := string(spec.GetTopic())
	if existing, ok := s.cohorts[key]; ok {
		// SWIP-60: an Open naming an already-open topic with an identical spec
		// is equivalent to Subscribe; a mismatched spec is refused.
		if !SpecEqual(existing.spec, spec) {
			return nil, nil, ErrSpecMismatch
		}
		ps, err := s.admit(existing, peer, open.GetAuth())
		if err != nil {
			return nil, nil, err
		}
		return existing, ps, nil
	}

	// Only the creation of a *new* cohort is capped; joining an existing one
	// is unaffected. Capacity limits streams per topic and says nothing about
	// how many topics one peer may fix, so without this a single peer can
	// Open unlimited distinct valid specs, each retaining a spec and a dedup
	// horizon that by design nothing ever reclaims. This is a cap, not
	// reclamation: cohorts still outlive their opener.
	if len(s.cohorts) >= s.maxCohorts {
		return nil, nil, fmt.Errorf("cohort limit %d reached: %w", s.maxCohorts, ErrCohortFull)
	}

	c := newCohort(spec, b)
	ps, err := s.admit(c, peer, open.GetAuth())
	if err != nil {
		return nil, nil, err
	}
	s.cohorts[key] = c
	s.metrics.Cohorts.Set(float64(len(s.cohorts)))

	return c, ps, nil
}

func (s *Service) subscribe(sub *pb.Subscribe, peer swarm.Address) (*cohort, *peerStream, error) {
	if len(sub.GetTopic()) != swarm.HashSize {
		return nil, nil, fmt.Errorf("topic length %d: %w", len(sub.GetTopic()), ErrInvalidSpec)
	}

	s.cohortsMu.Lock()
	defer s.cohortsMu.Unlock()

	c, ok := s.cohorts[string(sub.GetTopic())]
	if !ok {
		return nil, nil, ErrUnknownTopic
	}
	ps, err := s.admit(c, peer, sub.GetAuth())
	if err != nil {
		return nil, nil, err
	}
	return c, ps, nil
}

// admit runs the role checks for a peer joining c and, on success, creates
// and registers its peerStream in the same cohortsMu-held section as the
// capacity check, so admission is exact: two handshakes racing for a
// cohort's last slot can no longer both observe room and both be admitted,
// the way they could when registration happened later, in serve. The
// declared PublisherAuth is not a credential — this is an early refusal
// only; the authenticating check is the same authorization run at Publish
// time against the owner recovered from the message signature.
func (s *Service) admit(c *cohort, peer swarm.Address, auth *pb.PublisherAuth) (*peerStream, error) {
	if c.count() >= s.capacity {
		return nil, ErrCohortFull
	}

	publisher := auth != nil
	if publisher {
		if err := authorizePublisher(c.spec, auth.GetOwner()); err != nil {
			return nil, err
		}
	} else if c.spec.GetClosed() {
		// closed restricts admission, not readability: the role a peer claims
		// here is decided entirely by whether it sent a PublisherAuth, and the
		// owner it names is unauthenticated. Anyone who knows the topic and
		// any genesis publisher address — recoverable from any message they
		// have ever observed — can present that address and be admitted as a
		// publisher, and then read the cohort's whole stream. Publish-time
		// authentication still stops them writing, so this is read access
		// only. Confidentiality is payload encryption's job, not the closed
		// flag's; making closed enforceable would need a challenge-response
		// in the handshake, which is a wire-protocol change for SWIP-60.
		return nil, ErrClosedCohort
	}

	ps := newPeerStream(peer, publisher)
	c.add(ps)
	return ps, nil
}
