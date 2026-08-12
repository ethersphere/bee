// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"context"
	"errors"
	"fmt"
	"sync"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// ErrNoPeer is returned for an attach that would have to dial but names no
// broker underlay. Until broker discovery exists the caller must supply one.
var ErrNoPeer = errors.New("bps: no broker underlay address")

// Connecter dials a broker's underlay address and reports its overlay.
type Connecter interface {
	Connect(ctx context.Context, addrs []ma.Multiaddr) (*bzz.Address, error)
}

// AttachOptions describes one local client joining a topic through the bridge.
type AttachOptions struct {
	// Peer is the broker's underlay address. Required until broker discovery
	// exists, but only consulted when the topic has no session yet.
	Peer  ma.Multiaddr
	Topic swarm.Address
	// Spec non-nil makes the first attach an Open; nil makes it a Subscribe.
	Spec *pb.CohortSpec
	// Owner non-nil (a 20-byte ethereum address) asks for a read-write
	// attachment, and upgrades the topic's session if it is read-only.
	Owner []byte
}

// Attachment is one local sink on a muxed topic session.
type Attachment interface {
	Spec() *pb.CohortSpec
	// Messages is buffered OutboundQueueSize and closed on teardown, so a
	// WebSocket client sees an EOF when the session ends.
	Messages() <-chan *soc.SOC
	Publish(ctx context.Context, s *soc.SOC) error
	Close() error
}

// Bridge multiplexes one p2p session per topic onto many local sinks. Its
// consumer is the WebSocket API, where several browser clients routinely watch
// the same topic and should cost the node one stream, not one each.
//
// Locking: a single mutex guards the whole entry table, and it is held across
// the dial that a first attach or a role upgrade performs. That is the coarse
// of the two options: a slow handshake stalls fan-out for every topic for up
// to HandshakeTimeout, and messages pile up in the sessions' own buffers
// meanwhile. It is chosen anyway because the alternative — a per-entry dialing
// state that other attaches wait on — has to answer what happens when the dial
// fails, when the waiter's context expires first, and when the entry is torn
// down and recreated under a waiter, and that is three chances to leak a
// session or close a channel twice. Correctness first; if dial-time head of
// line blocking ever shows up in practice, the entry table and the sink sets
// can be split onto separate locks without changing the exported surface.
type Bridge struct {
	svc    *Service
	conn   Connecter
	logger log.Logger

	mu      sync.Mutex
	entries map[string]*entry
	closed  bool
}

// entry is the bridge's state for one topic: the single upstream session and
// every local sink fed from it. All fields are guarded by Bridge.mu.
type entry struct {
	topic   swarm.Address
	overlay swarm.Address // learned by the first dial, reused by upgrades
	session *Session
	sinks   map[*attachment]struct{}
	// done is closed when the fan-out goroutine for the current session
	// returns. A role upgrade replaces both together.
	done chan struct{}
	// torndown marks the entry as removed from the table, so the fan-out
	// goroutine knows the sinks were already closed by whoever removed it.
	torndown bool
}

// attachment is one local sink. Its mutable fields are guarded by Bridge.mu.
type attachment struct {
	b         *Bridge
	e         *entry
	publisher bool
	msgs      chan *soc.SOC
	// chClosed guards against closing msgs twice: either the last detach or
	// the fan-out goroutine closes it, whichever reaches it first.
	chClosed bool
	detached bool
}

var _ Attachment = (*attachment)(nil)

// NewBridge returns a bridge over svc, dialing brokers through conn.
func NewBridge(svc *Service, conn Connecter, logger log.Logger) *Bridge {
	return &Bridge{
		svc:     svc,
		conn:    conn,
		logger:  logger.WithName(loggerName).Register(),
		entries: make(map[string]*entry),
	}
}

// Attach joins a topic, opening the upstream session if this is the first
// local client on it and reusing it otherwise. A spec that disagrees with the
// live session's is refused rather than silently ignored: the spec is what
// every inbound message is verified against, so two clients on one session
// must agree on it.
func (b *Bridge) Attach(ctx context.Context, o AttachOptions) (Attachment, error) {
	topic, err := attachTopic(o)
	if err != nil {
		return nil, err
	}
	key := topic.ByteString()

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, ErrShutdown
	}

	e, ok := b.entries[key]
	if !ok {
		e, err = b.dial(ctx, topic, o)
		if err != nil {
			return nil, err
		}
		b.entries[key] = e
		b.start(e)
		return b.sink(e, o), nil
	}

	if o.Spec != nil && !SpecEqual(o.Spec, e.session.Spec()) {
		return nil, fmt.Errorf("topic %s already attached: %w", topic, ErrSpecMismatch)
	}
	if len(o.Owner) > 0 && !e.session.publisher {
		if err := b.upgrade(ctx, e, o); err != nil {
			return nil, err
		}
	}
	return b.sink(e, o), nil
}

// Status delegates to the service: the bridge adds no topics of its own.
func (b *Bridge) Status() []TopicStatus { return b.svc.Status() }

// Close detaches every sink, closes every session and waits for every fan-out
// goroutine the bridge started to return.
func (b *Bridge) Close() error {
	b.mu.Lock()
	b.closed = true
	live := make([]*entry, 0, len(b.entries))
	for key, e := range b.entries {
		delete(b.entries, key)
		b.detachAll(e)
		live = append(live, e)
	}
	b.mu.Unlock()

	for _, e := range live {
		_ = e.session.Close()
		<-e.done
	}
	return nil
}

// dial resolves the broker's overlay and opens the topic's session. It is
// called with b.mu held; see the note on Bridge.
func (b *Bridge) dial(ctx context.Context, topic swarm.Address, o AttachOptions) (*entry, error) {
	if o.Peer == nil {
		return nil, ErrNoPeer
	}
	addr, err := b.conn.Connect(ctx, []ma.Multiaddr{o.Peer})
	if err != nil {
		return nil, fmt.Errorf("connect broker: %w", err)
	}

	ss, err := b.join(ctx, addr.Overlay, topic, o)
	if err != nil {
		return nil, err
	}
	return &entry{
		topic:   topic,
		overlay: addr.Overlay,
		session: ss,
		sinks:   make(map[*attachment]struct{}),
	}, nil
}

// join performs the handshake an attach asks for: Open when it brings a spec,
// Subscribe when it does not.
func (b *Bridge) join(ctx context.Context, overlay, topic swarm.Address, o AttachOptions) (*Session, error) {
	var auth *pb.PublisherAuth
	if len(o.Owner) > 0 {
		auth = &pb.PublisherAuth{Owner: o.Owner}
	}
	if o.Spec != nil {
		return b.svc.Open(ctx, overlay, o.Spec, auth)
	}
	return b.svc.Subscribe(ctx, overlay, topic, auth)
}

// upgrade replaces a read-only session with a read-write one, keeping the
// existing sinks. The role is fixed at handshake time, so a publisher joining
// a topic that was subscribed to read-only cannot be served by the live
// session — it needs one of its own. Called with b.mu held.
func (b *Bridge) upgrade(ctx context.Context, e *entry, o AttachOptions) error {
	// The upgrade dials the overlay the first attach learned rather than
	// asking the Connecter again: the node is already connected to the peer,
	// and a second Connect on a live connection is at best redundant.
	ns, err := b.join(ctx, e.overlay, e.topic, o)
	if err != nil {
		return fmt.Errorf("upgrade topic %s: %w", e.topic, err)
	}

	old, done := e.session, e.done
	e.session = ns
	b.start(e)

	// The old session must be closed without b.mu: its fan-out goroutine takes
	// the lock to look at the sinks, and Session.Close does not return until
	// that goroutine's source of messages is gone. Unlocking here is safe
	// because the entry already names the new session, so a concurrent attach
	// sees the upgraded state, and the old goroutine sees that it has been
	// superseded and returns without touching the sinks. A message the old
	// goroutine was holding at that instant is dropped; the broker delivers to
	// the new session from the moment it admitted it, so the loss window is
	// bounded by the handshake, not by anything ongoing.
	b.mu.Unlock()
	_ = old.Close()
	<-done
	b.mu.Lock()

	// While the lock was released the last remaining sink may have detached
	// and taken the whole entry down, closing the session this call had just
	// swapped in. Attaching to it now would hand the caller a dead sink, so
	// the attach is refused and the caller retries into a fresh entry.
	if e.torndown {
		return ErrShutdown
	}
	return nil
}

// start launches the fan-out goroutine for the entry's current session.
// Called with b.mu held.
func (b *Bridge) start(e *entry) {
	done := make(chan struct{})
	e.done = done
	go b.fanout(e, e.session, done)
}

// fanout delivers one session's messages to every sink on the entry. A sink
// whose buffer is full is dropped past rather than allowed to stall the
// session — one slow WebSocket client must not hold up the others, let alone
// the p2p stream feeding them all.
func (b *Bridge) fanout(e *entry, ss *Session, done chan struct{}) {
	defer close(done)

	for m := range ss.Messages() {
		b.mu.Lock()
		if e.session != ss || e.torndown {
			b.mu.Unlock()
			return
		}
		for a := range e.sinks {
			select {
			case a.msgs <- m:
			default:
				b.svc.metrics.Dropped.WithLabelValues("slow_ws_client").Inc()
			}
		}
		b.mu.Unlock()
	}

	// The session ended on its own — the broker went away, or someone else
	// closed it. Take the topic down so the sinks see an EOF rather than a
	// channel that has simply gone quiet.
	b.mu.Lock()
	if e.session != ss || e.torndown {
		b.mu.Unlock()
		return
	}
	delete(b.entries, e.topic.ByteString())
	b.detachAll(e)
	b.mu.Unlock()

	// Close the dead session too. Its read loop has already returned, but
	// deregistration from the service's session set — and the stream reset —
	// happen only in Session.Close, so skipping it would leave the session
	// visible in Service.Status forever, and the bridge's own sinks are by now
	// all detached, so no later Attachment.Close would ever reach it. Close is
	// idempotent and does not block here, since readDone is already closed.
	// It is called without b.mu: Session.Close takes only the service's
	// sessionsMu, but there is no reason to hold the entry table for it.
	_ = ss.Close()
	b.logger.Debug("bridge: session ended", "topic", e.topic)
}

// sink registers a new attachment on an entry. Called with b.mu held.
func (b *Bridge) sink(e *entry, o AttachOptions) *attachment {
	a := &attachment{
		b:         b,
		e:         e,
		publisher: len(o.Owner) > 0,
		msgs:      make(chan *soc.SOC, OutboundQueueSize),
	}
	e.sinks[a] = struct{}{}
	return a
}

// detachAll closes every sink on an entry and marks it torn down. It does not
// close the session — the caller does that after releasing b.mu. Called with
// b.mu held.
func (b *Bridge) detachAll(e *entry) {
	e.torndown = true
	for a := range e.sinks {
		delete(e.sinks, a)
		a.detached = true
		a.closeCh()
	}
}

// closeCh closes the sink's channel at most once. Both the last detach and the
// fan-out goroutine can reach a sink, so the guard is not theoretical. Called
// with b.mu held.
func (a *attachment) closeCh() {
	if !a.chClosed {
		a.chClosed = true
		close(a.msgs)
	}
}

// Spec returns the cohort spec of the session behind this attachment.
func (a *attachment) Spec() *pb.CohortSpec {
	a.b.mu.Lock()
	defer a.b.mu.Unlock()
	return a.e.session.Spec()
}

func (a *attachment) Messages() <-chan *soc.SOC { return a.msgs }

// Publish sends through the shared session. The role is per attachment, not
// per session: another client upgrading the topic to read-write does not make
// this sink a publisher.
func (a *attachment) Publish(ctx context.Context, s *soc.SOC) error {
	if !a.publisher {
		return fmt.Errorf("read-only attachment: %w", ErrNotPublisher)
	}

	a.b.mu.Lock()
	if a.detached {
		a.b.mu.Unlock()
		return ErrShutdown
	}
	ss := a.e.session
	a.b.mu.Unlock()

	return ss.Publish(ctx, s)
}

// Close detaches this sink, and tears the topic's session down if it was the
// last one on it.
func (a *attachment) Close() error {
	b := a.b

	b.mu.Lock()
	if a.detached {
		b.mu.Unlock()
		return nil
	}
	a.detached = true
	delete(a.e.sinks, a)
	a.closeCh()

	e := a.e
	last := !e.torndown && len(e.sinks) == 0
	if last {
		e.torndown = true
		delete(b.entries, e.topic.ByteString())
	}
	b.mu.Unlock()

	if last {
		_ = e.session.Close()
		<-e.done
	}
	return nil
}

// attachTopic resolves the topic an attach names, from the option or from the
// spec it brings, and refuses the two disagreeing.
func attachTopic(o AttachOptions) (swarm.Address, error) {
	if o.Spec == nil {
		if o.Topic.IsZero() {
			return swarm.ZeroAddress, fmt.Errorf("no topic and no spec: %w", ErrInvalidSpec)
		}
		return o.Topic, nil
	}
	if err := ValidateSpec(o.Spec); err != nil {
		return swarm.ZeroAddress, err
	}
	t := swarm.NewAddress(o.Spec.GetTopic())
	if !o.Topic.IsZero() && !o.Topic.Equal(t) {
		return swarm.ZeroAddress, fmt.Errorf("topic %s is not the spec's: %w", o.Topic, ErrSpecMismatch)
	}
	return t, nil
}
