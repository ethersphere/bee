// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"context"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The per-peer stream limiter below is a copy of pkg/pullsync/streamlimit.go
// (rangesync does not import pullsync).

const (
	// maxPeerStatusStreams is the maximum number of concurrent incoming status
	// streams per peer. An honest peer sends one status at a time; this leaves
	// headroom for streams that are closed by the remote but whose handler has
	// not returned yet.
	maxPeerStatusStreams = 4
	// maxPeerReconcileStreams is the maximum number of concurrent incoming
	// reconcile streams per peer. Only MaxConcurrentSessionsPerPeer of them
	// run a session; the others are answered with Busy.
	maxPeerReconcileStreams = 4
)

// peerStreamLimiter limits the number of concurrent streams per peer using a
// token pool (buffered channel) per peer.
type peerStreamLimiter struct {
	mu    sync.Mutex
	limit int
	pools map[string]chan struct{}
}

func newPeerStreamLimiter(limit int) *peerStreamLimiter {
	return &peerStreamLimiter{
		limit: limit,
		pools: make(map[string]chan struct{}),
	}
}

// acquire tries to take a slot for the peer without blocking. If successful,
// the returned release function must be called to free the slot.
func (l *peerStreamLimiter) acquire(peer swarm.Address) (release func(), ok bool) {
	key := peer.ByteString()

	l.mu.Lock()
	pool, exists := l.pools[key]
	if !exists {
		pool = make(chan struct{}, l.limit)
		l.pools[key] = pool
	}
	l.mu.Unlock()

	select {
	case pool <- struct{}{}:
		return func() { <-pool }, true
	default:
		return nil, false
	}
}

// clear removes the peer's pool. Slots held at that time are released to the
// removed pool.
func (l *peerStreamLimiter) clear(peer swarm.Address) {
	l.mu.Lock()
	delete(l.pools, peer.ByteString())
	l.mu.Unlock()
}

// peerSlots bounds the concurrent outgoing streams of one kind per peer:
// acquire waits for one of limit slots of the peer, and at most maxWaiters
// calls wait. A peer's pool is forgotten once no stream holds or waits for
// one of its slots, so the pools of peers that left do not pile up.
type peerSlots struct {
	mu         sync.Mutex
	limit      int
	maxWaiters int
	pools      map[string]*slotPool
}

// slotPool is the pool of one peer; refs counts its holders and waiters.
type slotPool struct {
	c    chan struct{}
	refs int
}

func newPeerSlots(limit, maxWaiters int) *peerSlots {
	return &peerSlots{limit: limit, maxWaiters: maxWaiters, pools: make(map[string]*slotPool)}
}

// acquire takes one of the peer's slots, waiting for one until ctx ends, and
// returns the release function, which must be called once. It fails at once
// with errFetchQueueFull when maxWaiters calls wait already.
func (l *peerSlots) acquire(ctx context.Context, peer swarm.Address) (release func(), err error) {
	key := peer.ByteString()
	l.mu.Lock()
	pool, ok := l.pools[key]
	if !ok {
		pool = &slotPool{c: make(chan struct{}, l.limit)}
		l.pools[key] = pool
	}
	if pool.refs >= l.limit+l.maxWaiters {
		l.mu.Unlock()
		return nil, errFetchQueueFull
	}
	pool.refs++
	l.mu.Unlock()
	unref := func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if pool.refs--; pool.refs == 0 {
			delete(l.pools, key)
		}
	}
	select {
	case pool.c <- struct{}{}:
		return func() {
			<-pool.c
			unref()
		}, nil
	case <-ctx.Done():
		unref()
		return nil, context.Cause(ctx)
	}
}

// openOutcome is how a fetch stream open ended, for openGate.end.
type openOutcome int

const (
	// openSucceeded: the stream opened.
	openSucceeded openOutcome = iota + 1
	// openFailed: the open failed or timed out for a cause on the peer's
	// side or the way to it.
	openFailed
	// openAborted: this node ended the open, or the peer was gone.
	openAborted
)

// openGate backs off a peer's fetch stream opens after one failed: for
// base after the first failed open, doubling with each further one up to
// max (backoff). While the backoff runs, no open starts; once it ends, one
// open probes the peer at a time. An open that succeeds ends the backoff,
// and clear forgets it (a disconnect). Opens that began before a failure
// count with it, not on their own. The gate scores nothing: a peer whose
// opens fail (its inbound streams are full, say) is only asked less often.
// Its methods read the clock (now) only for a peer with a backoff. It is
// safe for concurrent use.
type openGate struct {
	mu        sync.Mutex
	base, max time.Duration
	peers     map[string]*openState
}

// openState is the backoff of one peer's opens: failures failed opens in a
// row, no open before until, and probing while the open after a backoff
// runs.
type openState struct {
	failures int
	until    time.Time
	probing  bool
}

func newOpenGate(base, maxBackoff time.Duration) *openGate {
	return &openGate{base: base, max: maxBackoff, peers: make(map[string]*openState)}
}

// allowed reports whether an open to peer may start now: its opens have not
// failed since its last open succeeded, or its backoff ended and no open
// probes it.
func (g *openGate) allowed(peer swarm.Address, now func() time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, ok := g.peers[peer.ByteString()]
	return !ok || (!st.probing && !now().Before(st.until))
}

// begin starts an open to peer now if allowed, and reports whether it probes
// the peer (its opens failed before); end must follow with the same probe.
func (g *openGate) begin(peer swarm.Address, now func() time.Time) (probe, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, found := g.peers[peer.ByteString()]
	if !found {
		return false, true
	}
	if st.probing || now().Before(st.until) {
		return false, false
	}
	st.probing = true
	return true, true
}

// end records how an open to peer that begin started ended, now.
func (g *openGate) end(peer swarm.Address, now func() time.Time, outcome openOutcome, probe bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	k := peer.ByteString()
	st, found := g.peers[k]
	if found && probe {
		st.probing = false
	}
	switch outcome {
	case openSucceeded:
		delete(g.peers, k)
	case openFailed:
		switch {
		case !found:
			st = new(openState)
			g.peers[k] = st
		case !probe:
			return // it began before the failure that started the backoff
		}
		st.failures++
		st.until = now().Add(g.backoff(st.failures))
	}
}

// backoff returns the wait after n failed opens in a row.
func (g *openGate) backoff(n int) time.Duration {
	d := g.base
	for i := 1; i < n && d < g.max; i++ {
		d *= 2
	}
	return min(d, g.max)
}

// clear forgets peer's backoff.
func (g *openGate) clear(peer swarm.Address) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.peers, peer.ByteString())
}
