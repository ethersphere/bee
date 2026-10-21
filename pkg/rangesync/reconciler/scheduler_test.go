// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// assertSessions checks, after letting the sessions a round started run,
// that peer had want sessions.
func assertSessions(t *testing.T, svc *fakeService, peer swarm.Address, want int) {
	t.Helper()
	if want > 0 {
		waitFor(t, fmt.Sprintf("%d sessions", want), func() bool { return len(svc.sessionOpens(peer)) >= want })
	}
	settle()
	if got := len(svc.sessionOpens(peer)); got != want {
		t.Fatalf("%d sessions with %s, want %d", got, peer, want)
	}
}

// TestRoundInitiatorRule: of two peers whose digests differ, the round starts
// a session with the higher one only; the lower one initiates.
func TestRoundInitiatorRule(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[lower.ByteString()] = status(2)
	svc.remote[higher.ByteString()] = status(2)
	h := newGossipHarness(t, svc, lower, higher)
	h.r.Round()
	assertSessions(t, svc, higher, 1)
	h.r.Round()
	assertSessions(t, svc, higher, 2)
	assertSessions(t, svc, lower, 0)
}

// TestRoundTakeover: a lower peer whose digest differs and that runs no
// session with this node is taken over after 2 × RoundInterval + jitter
// (jitter below one RoundInterval), not before.
func TestRoundTakeover(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[lower.ByteString()] = status(2)
	h := newGossipHarness(t, svc, lower)
	ri := h.r.CurrentRoundInterval()
	if ri != reconciler.MinRoundInterval {
		t.Fatalf("initial round interval %v, want %v", ri, reconciler.MinRoundInterval)
	}
	h.r.Round() // the mismatch is seen
	h.clock.Add(2*ri - time.Millisecond)
	h.r.Round()
	assertSessions(t, svc, lower, 0)
	h.clock.Add(ri + time.Millisecond)
	h.r.Round()
	assertSessions(t, svc, lower, 1)

	// with the jitter fixed at half a round interval, exactly at the deadline
	svc2 := newFakeService(status(1))
	svc2.remote[lower.ByteString()] = status(2)
	h2 := newGossipHarnessOptions(t, svc2, reconciler.Options{Jitter: func() float64 { return 0.5 }}, lower)
	h2.r.Round()
	h2.clock.Add(2*ri + ri/2 - time.Nanosecond)
	h2.r.Round()
	assertSessions(t, svc2, lower, 0)
	h2.clock.Add(time.Nanosecond)
	h2.r.Round()
	assertSessions(t, svc2, lower, 1)
}

// TestRoundTakeoverInitiatorAlive: a session the lower peer ran with this
// node restarts the wait, and so does a takeover answered with the peer's
// session slot taken (the peer's own session is running), without a backoff.
func TestRoundTakeoverInitiatorAlive(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[lower.ByteString()] = status(2)
	h := newGossipHarness(t, svc, lower)
	ri := h.r.CurrentRoundInterval()
	h.r.Round()
	h.clock.Add(2*ri - time.Second)
	h.r.Succeed(lower) // the initiator's session with this node completed
	h.clock.Add(2*ri - time.Millisecond)
	h.r.Round() // past the first wait, within the restarted one
	assertSessions(t, svc, lower, 0)

	svc.set(func() { svc.reconErr[lower.ByteString()] = rangesync.ErrPeerSessionLimit })
	h.clock.Add(2 * ri)
	h.r.Round()
	assertSessions(t, svc, lower, 1)
	waitFor(t, "idle", func() bool { s, _ := h.r.PeerState(lower); return s == reconciler.StateIdle })
	h.clock.Add(time.Second)
	h.r.Round() // the slot was taken by the peer's session: wait again
	assertSessions(t, svc, lower, 1)
	h.clock.Add(3 * ri)
	h.r.Round()
	assertSessions(t, svc, lower, 2)
	if s := h.r.Score(lower); s != 0 {
		t.Fatalf("score %d", s)
	}
}

// TestRoundTakeoverEqualDigestClears: an equal digest ends the wait; a later
// mismatch waits again from the round that sees it.
func TestRoundTakeoverEqualDigestClears(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[lower.ByteString()] = status(2)
	h := newGossipHarness(t, svc, lower)
	ri := h.r.CurrentRoundInterval()
	h.r.Round()
	h.clock.Add(ri)
	svc.set(func() { svc.remote[lower.ByteString()] = status(1) })
	h.r.Round()
	h.clock.Add(ri)
	svc.set(func() { svc.remote[lower.ByteString()] = status(0) })
	h.r.Round()                          // a new mismatch
	h.clock.Add(2*ri - time.Millisecond) // past the first mismatch's latest deadline
	h.r.Round()
	assertSessions(t, svc, lower, 0)
	h.clock.Add(ri + time.Millisecond)
	h.r.Round()
	assertSessions(t, svc, lower, 1)
}

// TestRoundIntervalAdapts: the round interval moves toward MaxRoundInterval
// while every digest matches and toward MinRoundInterval while differences
// are found; the round_interval_seconds gauge follows it.
func TestRoundIntervalAdapts(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(1)
	h := newGossipHarness(t, svc, higher)
	check := func(want time.Duration) {
		t.Helper()
		h.r.Round()
		if got := h.r.CurrentRoundInterval(); got != want {
			t.Fatalf("round interval %v, want %v", got, want)
		}
		if got := h.r.RoundIntervalGauge(); got != want.Seconds() {
			t.Fatalf("round_interval_seconds %v, want %v", got, want.Seconds())
		}
	}
	for _, want := range []time.Duration{10 * time.Second, 20 * time.Second, 30 * time.Second, 30 * time.Second} {
		check(want)
	}
	svc.set(func() { svc.remote[higher.ByteString()] = status(2) })
	for _, want := range []time.Duration{15 * time.Second, 7500 * time.Millisecond, 5 * time.Second, 5 * time.Second} {
		check(want)
	}

	// a fixed interval does not adapt
	fixed := reconciler.New(self, &fakeTopology{peers: []swarm.Address{higher}}, svc, log.Noop, reconciler.Options{
		Now:           h.clock.Now,
		After:         h.clock.After,
		RoundInterval: time.Minute,
	})
	t.Cleanup(func() { _ = fixed.Close() })
	fixed.Round()
	if got := fixed.CurrentRoundInterval(); got != time.Minute {
		t.Fatalf("fixed round interval %v", got)
	}
}

// TestRoundIntervalBounds: Options.MinRoundInterval and MaxRoundInterval
// override the bounds of the adaptive interval, and the minimum is the floor
// of a triggered round, while the round timer stays on the fake clock.
func TestRoundIntervalBounds(t *testing.T) {
	t.Parallel()

	const lo, hi = 2 * time.Second, 8 * time.Second
	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(1)
	h := newGossipHarnessOptions(t, svc, reconciler.Options{MinRoundInterval: lo, MaxRoundInterval: hi}, higher)
	if got := h.r.CurrentRoundInterval(); got != lo {
		t.Fatalf("initial round interval %v, want %v", got, lo)
	}
	check := func(want time.Duration) {
		t.Helper()
		h.r.Round()
		if got := h.r.CurrentRoundInterval(); got != want {
			t.Fatalf("round interval %v, want %v", got, want)
		}
	}
	for _, want := range []time.Duration{4 * time.Second, hi, hi} {
		check(want)
	}
	svc.set(func() { svc.remote[higher.ByteString()] = status(2) })
	for _, want := range []time.Duration{4 * time.Second, lo, lo} {
		check(want)
	}

	// a started reconciler: a trigger waits for the overridden minimum
	svc2 := newFakeService(status(1))
	svc2.remote[higher.ByteString()] = status(1)
	h2 := &harness{svc: svc2, topo: &fakeTopology{peers: []swarm.Address{higher}}, clock: &fakeClock{now: time.Unix(1_000_000, 0)}}
	h2.r = reconciler.New(self, h2.topo, svc2, log.Noop, reconciler.Options{
		Now: h2.clock.Now, After: h2.clock.After, MinRoundInterval: lo, MaxRoundInterval: hi,
	})
	t.Cleanup(func() {
		if err := h2.r.Close(); err != nil {
			t.Error(err)
		}
	})
	h2.r.Start(context.Background())
	waitFor(t, "first round", func() bool { s, _ := h2.r.PeerState(higher); return s == reconciler.StateIdle })
	svc2.set(func() { svc2.remote[higher.ByteString()] = status(2) })
	svc2.statusChange()
	assertSessions(t, svc2, higher, 0)
	h2.clock.Add(lo)
	assertSessions(t, svc2, higher, 1)
}

// TestRoundSessionCap: a round with more differing peers than
// MaxInitiatedSessions runs at most that many sessions at once; the others
// wait for a slot and run once one frees.
func TestRoundSessionCap(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.reconcileGate = make(chan struct{})
	peers := make([]swarm.Address, 2*reconciler.MaxInitiatedSessions+3)
	for i := range peers {
		peers[i] = addr(0xf0, byte(i))
		svc.remote[peers[i].ByteString()] = status(2)
	}
	h := newGossipHarness(t, svc, peers...)
	h.r.Round()
	waitFor(t, "sessions", func() bool { return svc.running() == reconciler.MaxInitiatedSessions })
	h.r.Round() // the waiting peers are Reconciling: no second session with them
	settle()
	svc.set(func() {
		if svc.inAll != reconciler.MaxInitiatedSessions {
			t.Errorf("%d sessions at once, want %d", svc.inAll, reconciler.MaxInitiatedSessions)
		}
	})
	if h.r.GuardReleased() {
		t.Fatal("start-up guard released with sessions waiting for a slot")
	}
	close(svc.reconcileGate)
	for _, p := range peers {
		waitFor(t, "session", func() bool { return svc.count(svc.reconciled, p) > 0 })
	}
	settle()
	for _, p := range peers {
		if n, m := len(svc.sessionOpens(p)), svc.maxConcurrent(p); n != 1 || m != 1 {
			t.Fatalf("peer %s: %d sessions, %d at once; want 1 and 1", p, n, m)
		}
	}
	svc.set(func() {
		if svc.maxInAll != reconciler.MaxInitiatedSessions {
			t.Errorf("at most %d sessions at once, want %d", svc.maxInAll, reconciler.MaxInitiatedSessions)
		}
	})
}

// TestRoundSessionCapSharedWithStripes: stripe sessions take their slots
// from the same MaxInitiatedSessions as round sessions.
func TestRoundSessionCapSharedWithStripes(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.reconcileGate = make(chan struct{})
	peers := make([]swarm.Address, reconciler.MaxInitiatedSessions)
	for i := range peers {
		peers[i] = addr(0xf0, byte(i))
		svc.remote[peers[i].ByteString()] = status(2)
	}
	h := newGossipHarness(t, svc, peers...)
	h.r.Round()
	waitFor(t, "sessions", func() bool { return svc.running() == reconciler.MaxInitiatedSessions })

	striper := addr(0xf1, 0) // not followed by the reconciler
	done := make(chan []reconciler.StripeResult, 1)
	go func() {
		done <- h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: fingerprint.NewRange(swarm.ZeroAddress, 1), Peers: []swarm.Address{striper}}})
	}()
	settle()
	if n := len(svc.sessionOpens(striper)); n != 0 {
		t.Fatalf("stripe session started with every slot taken (%d)", n)
	}
	close(svc.reconcileGate)
	if res := <-done; res[0].Err != nil {
		t.Fatal(res[0].Err)
	}
	svc.set(func() {
		if svc.maxInAll > reconciler.MaxInitiatedSessions {
			t.Errorf("%d sessions at once, want at most %d", svc.maxInAll, reconciler.MaxInitiatedSessions)
		}
	})
}

// newScheduledHarness returns a started reconciler with the default
// (adaptive) round interval: its round timer and its deferred rounds follow
// the fake clock, so only the first round runs unless the test moves it.
func newScheduledHarness(t *testing.T, svc *fakeService, peers ...swarm.Address) *harness {
	t.Helper()
	h := &harness{
		svc:   svc,
		topo:  &fakeTopology{peers: peers},
		clock: &fakeClock{now: time.Unix(1_000_000, 0)},
	}
	h.r = reconciler.New(self, h.topo, svc, log.Noop, reconciler.Options{Now: h.clock.Now, After: h.clock.After})
	t.Cleanup(func() {
		if err := h.r.Close(); err != nil {
			t.Error(err)
		}
	})
	h.r.Start(context.Background())
	return h
}

// TestStatusChangeTriggersRound: a changed Status of a peer triggers a round,
// MinRoundInterval after the last one.
func TestStatusChangeTriggersRound(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(1)
	h := newScheduledHarness(t, svc, higher)
	waitFor(t, "first round", func() bool { s, _ := h.r.PeerState(higher); return s == reconciler.StateIdle })

	svc.set(func() { svc.remote[higher.ByteString()] = status(2) })
	svc.statusChange()
	assertSessions(t, svc, higher, 0) // within MinRoundInterval of the first round
	h.clock.Add(reconciler.MinRoundInterval)
	assertSessions(t, svc, higher, 1)
}

// TestRootChangeTriggersRound: a root change of this node triggers a round,
// so that the initiator reacts to its own new elements as its peers do.
func TestRootChangeTriggersRound(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(1)
	h := newScheduledHarness(t, svc, higher)
	waitFor(t, "first round", func() bool { s, _ := h.r.PeerState(higher); return s == reconciler.StateIdle })

	h.clock.Add(reconciler.MinRoundInterval)
	svc.set(func() { svc.local = status(2) })
	svc.rootChange()
	assertSessions(t, svc, higher, 1)
}

// TestStatusChangeWakesGossip: a peer that becomes eligible from its side
// gets this node's changed root summary at once, not at the heartbeat.
func TestStatusChangeWakesGossip(t *testing.T) {
	t.Parallel()

	own := fingerprint.NewRange(addr(0x80, 0), 1)
	withRoot := func(st rangesync.Status, root fingerprint.Range) rangesync.Status {
		st.Root = root
		return st
	}
	svc := newFakeService(withRoot(status(1), own))
	svc.remote[higher.ByteString()] = withRoot(status(1), fingerprint.NewRange(addr(0x00, 0), 1)) // disjoint
	h := newScheduledHarness(t, svc, higher)
	waitFor(t, "first contact", func() bool { return svc.count(svc.sent, higher) == 1 })

	h.clock.Add(reconciler.StatusMinInterval)
	svc.set(func() { svc.local = withRoot(status(2), own) })
	svc.rootChange() // not eligible: nothing to send
	settle()
	if n := svc.count(svc.sent, higher); n != 1 {
		t.Fatalf("%d statuses to a peer that is not eligible", n)
	}
	svc.set(func() { svc.remote[higher.ByteString()] = withRoot(status(1), own) })
	svc.statusChange()
	waitFor(t, "push", func() bool { return svc.count(svc.sent, higher) == 2 })
}

// TestRoundWaitsForMovedRoot: after this node's root moved, the round starts
// no session with a peer that does not have the new root yet (its send is in
// flight), and starts it once the send is done.
func TestRoundWaitsForMovedRoot(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(1)
	h := newGossipHarness(t, svc, higher)
	h.r.Round() // first contact, equal digests

	calls := make(chan chan struct{})
	svc.set(func() {
		svc.sendCalls = calls
		svc.local = selfRoot(1, 5) // the radius increased
	})
	h.clock.Add(time.Second)
	gossip := make(chan struct{})
	go func() {
		defer close(gossip)
		h.r.Gossip() // sends the moved root
	}()
	release := <-calls
	h.r.Round()
	assertSessions(t, svc, higher, 0)
	close(release)
	<-gossip
	svc.set(func() { svc.sendCalls = nil })
	h.r.Round()
	assertSessions(t, svc, higher, 1)
}

// TestStripePlanAvoidsBackoff: stripes are planned without the peers that
// back off after Busy sessions, unless they are needed to cover the range.
func TestStripePlanAvoidsBackoff(t *testing.T) {
	t.Parallel()

	busy, ready := addr(0xf0, 1), addr(0xf0, 2)
	svc := newFakeService(status(1))
	svc.remote[busy.ByteString()] = status(2)
	svc.remote[ready.ByteString()] = status(1)
	h := newGossipHarness(t, svc, busy, ready)
	for range 2 { // the second Busy in a row backs off
		svc.set(func() { svc.reconErr[busy.ByteString()] = rangesync.ErrPeerBusy })
		h.r.Round()
		waitFor(t, "busy", func() bool { return svc.count(svc.reconciled, busy) > 0 && svc.count(svc.inSession, busy) == 0 })
		waitFor(t, "idle", func() bool { s, _ := h.r.PeerState(busy); return s == reconciler.StateIdle })
		svc.set(func() { svc.reconciled[busy.ByteString()] = 0 })
	}

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	for _, st := range h.r.PlanStripesFor(root) {
		if len(st.Peers) != 1 || !st.Peers[0].Equal(ready) {
			t.Fatalf("stripe %s planned with %v, want only %s", st.Range, st.Peers, ready)
		}
	}
	// the reassignment fallback and the verifier choose the ready peer too;
	// each call walks the peers in a new (map) order
	deliverer := addr(0xf1, 0)
	for range 20 {
		if p, ok := h.r.NextStripePeerFallback(root, deliverer); !ok || !p.Equal(ready) {
			t.Fatalf("fallback peer %s, want %s", p, ready)
		}
		if p, ok := h.r.VerifierFor(reconciler.StripeResult{Range: root, Peer: deliverer}); !ok || !p.Equal(ready) {
			t.Fatalf("verifier %s, want %s", p, ready)
		}
	}

	// the busy peer is the only one that covers the range: it is planned
	narrow := withRootOf(status(1), fingerprint.NewRange(addr(0xf0, 0), 1))
	svc.set(func() { svc.remote[ready.ByteString()] = narrow })
	other := root.Child(1, 0) // the half ready does not cover
	var planned bool
	for _, st := range h.r.PlanStripesFor(root) {
		if other.ContainsRange(st.Range) {
			planned = planned || len(st.Peers) > 0
		}
	}
	if !planned {
		t.Fatal("no peer planned for the half only the busy peer covers")
	}
}

// TestStripePlanAvoidsOpenBackoff (T4.5 part 2c fix round 2): stripes are
// planned, reassigned and verified without the peers whose fetch stream
// opens back off, where they would only wait, unless they are needed to
// cover the range; once the backoff ends such a peer is planned again.
func TestStripePlanAvoidsOpenBackoff(t *testing.T) {
	t.Parallel()

	backoff, ready := addr(0xf0, 1), addr(0xf0, 2)
	svc := newFakeService(status(1))
	svc.remote[backoff.ByteString()] = status(1)
	svc.remote[ready.ByteString()] = status(1)
	h := newGossipHarness(t, svc, backoff, ready)
	svc.set(func() {
		svc.now = h.clock.Now
		svc.openUntil[backoff.ByteString()] = h.clock.Now().Add(time.Minute)
	})
	h.r.Round()
	waitFor(t, "peers idle", func() bool {
		a, _ := h.r.PeerState(backoff)
		b, _ := h.r.PeerState(ready)
		return a == reconciler.StateIdle && b == reconciler.StateIdle
	})

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	for _, st := range h.r.PlanStripesFor(root) {
		if len(st.Peers) != 1 || !st.Peers[0].Equal(ready) {
			t.Fatalf("stripe %s planned with %v, want only %s", st.Range, st.Peers, ready)
		}
	}
	deliverer := addr(0xf1, 0)
	for range 20 { // each call walks the peers in a new (map) order
		if p, ok := h.r.NextStripePeerFallback(root, deliverer); !ok || !p.Equal(ready) {
			t.Fatalf("fallback peer %s, want %s", p, ready)
		}
		if p, ok := h.r.VerifierFor(reconciler.StripeResult{Range: root, Peer: deliverer}); !ok || !p.Equal(ready) {
			t.Fatalf("verifier %s, want %s", p, ready)
		}
	}

	// the peer whose opens back off is the only one that covers a half
	svc.set(func() { svc.remote[ready.ByteString()] = withRootOf(status(1), fingerprint.NewRange(addr(0xf0, 0), 1)) })
	other := root.Child(1, 0) // the half ready does not cover
	planned := func() bool {
		for _, st := range h.r.PlanStripesFor(root) {
			if other.ContainsRange(st.Range) && len(st.Peers) > 0 && st.Peers[0].Equal(backoff) {
				return true
			}
		}
		return false
	}
	if !planned() {
		t.Fatal("the half only the backed-off peer covers is not planned")
	}
	// once its backoff ends it is planned again, with the other peers
	svc.set(func() { svc.remote[ready.ByteString()] = status(1) })
	h.clock.Add(time.Minute)
	var backoffPlanned bool
	for _, st := range h.r.PlanStripesFor(root) {
		backoffPlanned = backoffPlanned || len(st.Peers) > 0 && st.Peers[0].Equal(backoff)
	}
	if !backoffPlanned {
		t.Fatal("the peer is not planned after its backoff ended")
	}
}

// withRootOf returns st with root.
func withRootOf(st rangesync.Status, root fingerprint.Range) rangesync.Status {
	st.Root = root
	return st
}

// TestStripeSessionCap: at most MaxStripeSessions stripe sessions run at
// once, and a round session still gets a slot while they hold theirs.
func TestStripeSessionCap(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	stripes := make([]reconciler.Stripe, reconciler.MaxStripeSessions+2)
	gate := make(chan struct{})
	for i := range stripes {
		p := addr(0xf1, byte(i)) // not followed by the reconciler
		svc.peerGate[p.ByteString()] = gate
		stripes[i] = reconciler.Stripe{Range: fingerprint.NewRange(swarm.ZeroAddress, 0).Child(3, uint64(i)), Peers: []swarm.Address{p}}
	}
	h := newGossipHarness(t, svc, higher)
	done := make(chan []reconciler.StripeResult, 1)
	go func() { done <- h.r.TransferStripes(context.Background(), stripes) }()
	waitFor(t, "stripe sessions", func() bool { return svc.running() == reconciler.MaxStripeSessions })

	h.r.Round()
	assertSessions(t, svc, higher, 1)
	waitFor(t, "round session", func() bool { return svc.count(svc.reconciled, higher) == 1 })
	if n := svc.running(); n != reconciler.MaxStripeSessions {
		t.Fatalf("%d stripe sessions at once, want %d", n, reconciler.MaxStripeSessions)
	}
	close(gate)
	for _, res := range <-done {
		if res.Err != nil {
			t.Fatal(res.Err)
		}
	}
	svc.set(func() {
		if svc.maxInAll > reconciler.MaxStripeSessions+1 {
			t.Errorf("%d sessions at once, want at most %d", svc.maxInAll, reconciler.MaxStripeSessions+1)
		}
	})
}

// TestStripeTimeoutAfterSlot: the stripe timeout starts once a stripe has
// its slots, so a stripe that waited longer than its timeout for one (here
// round sessions hold every session slot) still has its whole timeout.
func TestStripeTimeoutAfterSlot(t *testing.T) {
	t.Parallel()

	const timeout = time.Hour // only the fake clock gets there
	svc := newFakeService(status(1))
	gates := make([]chan struct{}, reconciler.MaxInitiatedSessions)
	peers := make([]swarm.Address, len(gates))
	for i := range gates {
		peers[i] = addr(0xf0, byte(i+1))
		gates[i] = make(chan struct{})
		svc.peerGate[peers[i].ByteString()] = gates[i]
		svc.remote[peers[i].ByteString()] = status(2)
	}
	t.Cleanup(func() {
		for _, g := range gates[1:] {
			close(g)
		}
	})
	h := newGossipHarnessOptions(t, svc, reconciler.Options{StripeTimeout: timeout}, peers...)
	h.r.Round()
	waitFor(t, "slots taken", func() bool { return svc.running() == reconciler.MaxInitiatedSessions })

	done := make(chan []reconciler.StripeResult, 1)
	go func() { done <- h.r.TransferStripes(context.Background(), stripesTo(addr(0xf1, 0))) }()
	waitFor(t, "stripe slot taken", func() bool { return h.r.StripeSlotsTaken() == 1 })
	h.clock.Add(2 * timeout) // while the stripe waits for a session slot
	close(gates[0])
	select {
	case res := <-done:
		if res[0].Err != nil {
			t.Fatalf("stripe that waited for a slot: %v", res[0].Err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stripe did not end: it timed out while waiting for a slot and was reassigned")
	}
}

// TestRoundIntervalNestedRoots: a peer whose root contains this node's (or
// is contained in it) has a digest of another range, which is not compared:
// its sessions finding nothing let the interval grow, and one finding wants
// shrinks it.
func TestRoundIntervalNestedRoots(t *testing.T) {
	t.Parallel()

	svc := newFakeService(selfRoot(1, 5))
	svc.remote[higher.ByteString()] = selfRoot(0, 10)
	h := newGossipHarness(t, svc, higher)
	round := func(want time.Duration) {
		t.Helper()
		h.r.Round()
		waitFor(t, "session", func() bool { return len(svc.sessionOpens(higher)) > 0 })
		waitState(t, h.r, higher, reconciler.StateIdle)
		if got := h.r.CurrentRoundInterval(); got != want {
			t.Fatalf("round interval %v, want %v", got, want)
		}
	}
	round(10 * time.Second)
	round(20 * time.Second)
	round(30 * time.Second)
	svc.set(func() { svc.wants[higher.ByteString()] = 3 })
	round(30 * time.Second) // adapted before its session found the wants
	round(15 * time.Second)
}

// summaryService is a fakeService that summarizes ranges of this node's
// index with sum (the reconciler's optional rangeSummarizer).
type summaryService struct {
	*fakeService
	sum func(fingerprint.Range) (fingerprint.Summary, error)
}

func (s summaryService) RangeSummary(r fingerprint.Range) (fingerprint.Summary, error) {
	return s.sum(r)
}

// newSummaryHarness is newGossipHarness over summaryService{svc, sum}.
func newSummaryHarness(t *testing.T, svc *fakeService, sum func(fingerprint.Range) (fingerprint.Summary, error), peers ...swarm.Address) *harness {
	t.Helper()
	h := &harness{
		svc:   svc,
		topo:  &fakeTopology{peers: peers},
		clock: &fakeClock{now: time.Unix(1_000_000, 0)},
	}
	h.r = reconciler.New(self, h.topo, summaryService{fakeService: svc, sum: sum}, log.Noop, reconciler.Options{Now: h.clock.Now, After: h.clock.After})
	t.Cleanup(func() {
		if err := h.r.Close(); err != nil {
			t.Error(err)
		}
	})
	return h
}

// TestRoundSessionRootDigest checks the digest comparison over the session
// root. The peer's Status summarizes its own root; when that root lies inside
// this node's, it is the session root, and this node compares its own
// summary of that range (RangeSummary) with the peer's:
//   - equal: no session, and the round interval grows as for equal roots;
//   - different: a session every round, and the interval stays at the floor
//     however little the sessions find, as for equal roots with differing
//     digests (the T4.3 M2 rule over comparable digests);
//   - when the index cannot summarize the range, or this node's root lies
//     inside the peer's (the peer's summary covers more than the session
//     root), the digests are not comparable: a session every round, and the
//     interval adapts on what the sessions find.
func TestRoundSessionRootDigest(t *testing.T) {
	t.Parallel()

	peerRoot := fingerprint.NewRange(higher, 1) // inside this node's root
	remote := withRootOf(status(3), peerRoot)
	rounds := func(t *testing.T, h *harness, sessions bool, intervals ...time.Duration) {
		t.Helper()
		for i, want := range intervals {
			before := len(h.svc.sessionOpens(higher))
			h.r.Round()
			if sessions {
				waitFor(t, "session", func() bool { return len(h.svc.sessionOpens(higher)) > before })
				waitState(t, h.r, higher, reconciler.StateIdle)
			} else if st := peerState(h.r, higher); st != reconciler.StateIdle || len(h.svc.sessionOpens(higher)) != before {
				// the round moves a peer it starts a session with to
				// Reconciling before it returns
				t.Fatalf("round %d: a session with digests equal over the session root (peer %v)", i+1, st)
			}
			if got := h.r.CurrentRoundInterval(); got != want {
				t.Fatalf("round %d: interval %v, want %v", i+1, got, want)
			}
		}
	}
	summaryOf := func(sum fingerprint.Summary, err error) func(fingerprint.Range) (fingerprint.Summary, error) {
		return func(r fingerprint.Range) (fingerprint.Summary, error) {
			if !r.Equal(peerRoot) {
				return fingerprint.Summary{}, fmt.Errorf("unexpected range %v", r)
			}
			return sum, err
		}
	}

	t.Run("equal", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService(status(10))
		svc.remote[higher.ByteString()] = remote
		h := newSummaryHarness(t, svc, summaryOf(remote.Summary, nil), higher)
		rounds(t, h, false, 10*time.Second, 20*time.Second, 30*time.Second)
	})
	t.Run("different", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService(status(10))
		svc.remote[higher.ByteString()] = remote
		h := newSummaryHarness(t, svc, summaryOf(status(4).Summary, nil), higher)
		rounds(t, h, true, reconciler.MinRoundInterval, reconciler.MinRoundInterval, reconciler.MinRoundInterval)
	})
	t.Run("index cannot answer", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService(status(10))
		svc.remote[higher.ByteString()] = remote
		h := newSummaryHarness(t, svc, summaryOf(remote.Summary, fingerprint.ErrBelowLeaf), higher)
		rounds(t, h, true, 10*time.Second, 20*time.Second, 30*time.Second)
	})
	t.Run("this node's root inside the peer's", func(t *testing.T) {
		t.Parallel()
		local := selfRoot(1, 3)
		svc := newFakeService(local)
		svc.remote[higher.ByteString()] = withRootOf(status(3), fingerprint.NewRange(higher, 0))
		h := newSummaryHarness(t, svc, func(fingerprint.Range) (fingerprint.Summary, error) {
			return local.Summary, nil // equal, were it asked
		}, higher)
		rounds(t, h, true, 10*time.Second, 20*time.Second, 30*time.Second)
	})
}

// TestSessionCollision: a session that finds the peer running its own
// session with this node does not back off, and a takeover answered Busy
// (both sides opened at once) waits for the initiator again, in either role
// and order.
func TestSessionCollision(t *testing.T) {
	t.Parallel()

	t.Run("initiator, peer session running here", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService(status(1))
		svc.remote[higher.ByteString()] = status(2)
		h := newGossipHarness(t, svc, higher)
		for i := range 3 {
			svc.set(func() { svc.reconErr[higher.ByteString()] = rangesync.ErrPeerSessionLimit })
			if !gatedRound(t, h, higher) {
				t.Fatalf("no session %d after the peer's own sessions", i+1)
			}
		}
	})
	t.Run("initiator, both opened at once", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService(status(1))
		svc.remote[higher.ByteString()] = status(2)
		h := newGossipHarness(t, svc, higher)
		svc.set(func() { svc.reconErr[higher.ByteString()] = rangesync.ErrPeerBusy })
		if !gatedRound(t, h, higher) {
			t.Fatal("no session")
		}
		if !gatedRound(t, h, higher) { // the first Busy is retried by the next round
			t.Fatal("no session after the collision")
		}
	})
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"takeover, initiator session running here", rangesync.ErrPeerSessionLimit},
		{"takeover, both opened at once", rangesync.ErrPeerBusy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newFakeService(status(1))
			svc.remote[lower.ByteString()] = status(2)
			h := newGossipHarnessOptions(t, svc, reconciler.Options{Jitter: func() float64 { return 0 }}, lower)
			ri := h.r.CurrentRoundInterval()
			h.r.Round()
			h.clock.Add(2 * ri)
			svc.set(func() { svc.reconErr[lower.ByteString()] = tc.err })
			if !gatedRound(t, h, lower) {
				t.Fatal("no takeover")
			}
			h.clock.Add(2*ri - time.Millisecond)
			if gatedRound(t, h, lower) {
				t.Fatal("takeover again before the initiator's new wait")
			}
			h.clock.Add(time.Millisecond)
			if !gatedRound(t, h, lower) {
				t.Fatal("no takeover after the new wait: backed off")
			}
			if s := h.r.Score(lower); s != 0 {
				t.Fatalf("score %d", s)
			}
		})
	}
}

// TestSessionRecheckAfterSlot: a session that waited for a slot does not
// run when the round's decision no longer holds: the digests became equal,
// this node's root moved, the peer left and came back (only the new entry's
// session runs), the initiator ran a session with this node since the
// takeover was decided, or this node is now far behind the peer in their
// session root (the peer's Status counts more than LargeDiff more, or this
// node's own transfer of the root was registered meanwhile and the peer
// holds more: lacksSessionRootLocked, T4.7 part 1 fix round 1 Low 3). It
// ends without a backoff.
func TestSessionRecheckAfterSlot(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		peer   swarm.Address
		change func(h *harness, p swarm.Address, others []swarm.Address)
		want   int // sessions run with the peer once two slots free
	}{
		{"equal digests", higher, func(h *harness, p swarm.Address, _ []swarm.Address) {
			h.svc.set(func() { h.svc.remote[p.ByteString()] = status(1) })
		}, 0},
		{"root moved", higher, func(h *harness, _ swarm.Address, _ []swarm.Address) {
			h.svc.set(func() { h.svc.local = selfRoot(1, 1) })
		}, 0},
		{"new entry", higher, func(h *harness, p swarm.Address, others []swarm.Address) {
			h.topo.setPeers(others...)
			h.r.Round()
			h.topo.setPeers(append(others, p)...)
			h.r.Round() // the new entry's session waits too
		}, 1},
		{"initiator served", lower, func(h *harness, p swarm.Address, _ []swarm.Address) {
			h.clock.Add(time.Second)
			h.r.Succeed(p)
		}, 0},
		{"far behind the peer", higher, func(h *harness, p swarm.Address, _ []swarm.Address) {
			h.svc.set(func() { h.svc.remote[p.ByteString()] = status(2 + rangesync.LargeDiff) })
		}, 0},
		{"own transfer registered", higher, func(h *harness, _ swarm.Address, _ []swarm.Address) {
			h.r.HoldStripe(fingerprint.NewRange(swarm.ZeroAddress, 0))
		}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newFakeService(status(1))
			gates := make([]chan struct{}, reconciler.MaxInitiatedSessions)
			others := make([]swarm.Address, len(gates), len(gates)+1)
			for i := range gates {
				others[i] = addr(0xf0, byte(i+1))
				gates[i] = make(chan struct{})
				svc.peerGate[others[i].ByteString()] = gates[i]
				svc.remote[others[i].ByteString()] = status(2)
			}
			t.Cleanup(func() {
				for _, g := range gates {
					select {
					case <-g:
					default:
						close(g)
					}
				}
			})
			svc.remote[tc.peer.ByteString()] = status(2)
			h := newGossipHarnessOptions(t, svc, reconciler.Options{Jitter: func() float64 { return 0 }}, others...)
			h.r.Round()
			waitFor(t, "slots taken", func() bool { return svc.running() == reconciler.MaxInitiatedSessions })
			h.topo.setPeers(append(others, tc.peer)...)
			h.r.Round()
			if tc.peer.Equal(lower) { // the takeover is due
				h.clock.Add(3 * h.r.CurrentRoundInterval())
				h.r.Round()
			}
			waitState(t, h.r, tc.peer, reconciler.StateReconciling) // waiting for a slot
			tc.change(h, tc.peer, others)
			close(gates[0])
			close(gates[1])
			waitFor(t, "sessions ended", func() bool { return peerState(h.r, tc.peer) != reconciler.StateReconciling })
			settle()
			if n := len(svc.sessionOpens(tc.peer)); n != tc.want {
				t.Fatalf("%d sessions, want %d: a stale decision ran", n, tc.want)
			}
			if got := h.r.SessionsTotal("stale"); got != 1 {
				t.Fatalf("sessions_total{stale} = %v, want 1", got)
			}
			if f := h.r.PeerFailures(tc.peer); f != 0 {
				t.Fatalf("backoff streak %d after a stale session", f)
			}
			if tc.name == "equal digests" { // differing again: the next round runs it at once
				svc.set(func() { svc.remote[tc.peer.ByteString()] = status(2) })
				if !gatedRound(t, h, tc.peer) {
					t.Fatal("no session after a stale one")
				}
			}
		})
	}
}

// TestStatusChangeFloor: a peer whose Status keeps changing triggers at most
// one round per MinRoundInterval.
func TestStatusChangeFloor(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	h := newScheduledHarness(t, svc, higher)
	assertSessions(t, svc, higher, 1) // the first round
	spam := func() {
		for i := range 50 {
			svc.set(func() { svc.remote[higher.ByteString()] = status(uint64(2 * (i % 2))) })
			svc.statusChange()
			time.Sleep(time.Millisecond)
		}
	}
	spam()
	assertSessions(t, svc, higher, 1)
	h.clock.Add(reconciler.MinRoundInterval / 2)
	spam()
	assertSessions(t, svc, higher, 1)
	h.clock.Add(reconciler.MinRoundInterval / 2)
	assertSessions(t, svc, higher, 2)
	spam()
	assertSessions(t, svc, higher, 2)
}

// TestTopologyChangeFloor: a topology change sends a new peer this node's
// Status at once, but runs the round, and so the sessions, no sooner than
// MinRoundInterval after the last one: a peer that answered Busy is not
// retried earlier because another peer connected.
func TestTopologyChangeFloor(t *testing.T) {
	t.Parallel()

	joiner := addr(0xf0, 1)
	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	svc.remote[joiner.ByteString()] = status(2)
	svc.reconErr[higher.ByteString()] = rangesync.ErrPeerBusy // the first session
	h := newScheduledHarness(t, svc, higher)
	assertSessions(t, svc, higher, 1)
	waitFor(t, "first Busy recorded", func() bool { return h.r.PeerFailures(higher) == 1 })

	h.topo.setPeers(higher, joiner)
	waitFor(t, "first contact", func() bool { return svc.count(svc.sent, joiner) == 1 && !h.r.StatusSending(joiner) })
	assertSessions(t, svc, higher, 1) // the first Busy is not retried within the floor
	assertSessions(t, svc, joiner, 0)
	h.clock.Add(reconciler.MinRoundInterval)
	assertSessions(t, svc, higher, 2)
	assertSessions(t, svc, joiner, 1)
}

// TestRoundDueBeforeTopologyChange: a round that is due runs before the
// next topology change is handled, so a stream of topology changes does not
// stretch the round interval. Each trial holds the loop in the topology
// change's own step, makes the round due and signals another change; the
// round runs next. Rounds are told apart by the interval they adapt.
func TestRoundDueBeforeTopologyChange(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(1)
	h := newScheduledHarness(t, svc, higher)
	waitFor(t, "first round", func() bool { return h.r.CurrentRoundInterval() == 2*reconciler.MinRoundInterval })

	for i := range 20 {
		next := status(2) // differs: the round halves the interval
		if i%2 == 1 {
			next = status(1) // equal: the round doubles it
		}
		svc.set(func() { svc.remote[higher.ByteString()] = next })
		var (
			mu      sync.Mutex
			blocked = make(chan struct{})
			release = make(chan struct{})
			seen    []time.Duration
			armed   = true
		)
		h.topo.setOnEach(func() {
			mu.Lock()
			if armed {
				armed = false
				mu.Unlock()
				close(blocked)
				<-release
				return
			}
			seen = append(seen, h.r.CurrentRoundInterval())
			mu.Unlock()
		})
		h.topo.setPeers(higher) // the loop's step for it waits on release
		<-blocked
		h.clock.Add(h.r.CurrentRoundInterval()) // the round is due
		h.topo.setPeers(higher)                 // and another change is signaled
		close(release)
		waitFor(t, "a round and a topology step", func() bool {
			mu.Lock()
			defer mu.Unlock()
			return len(seen) >= 2
		})
		mu.Lock()
		round, step := seen[0], seen[1]
		mu.Unlock()
		if round == step {
			t.Fatalf("trial %d: the topology change was handled before the due round (interval %v at both)", i, round)
		}
		h.topo.setOnEach(nil)
	}
}

// TestTopologyChangeDoesNotHoldRounds: a peer that connects and never
// answers this node's first Status does not hold up the rounds, whose
// sessions with other peers go on at the round floor.
func TestTopologyChangeDoesNotHoldRounds(t *testing.T) {
	t.Parallel()

	silent := addr(0xf3, 0)
	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	svc.sendBlock[silent.ByteString()] = true
	h := newScheduledHarness(t, svc, higher)
	assertSessions(t, svc, higher, 1)

	h.topo.setPeers(higher, silent)
	waitFor(t, "first contact sent", func() bool { return svc.count(svc.sent, silent) == 1 })
	h.clock.Add(reconciler.MinRoundInterval)
	assertSessions(t, svc, higher, 2)
}

// TestRoundIntervalServedWants: wants a peer listed in a session this node
// served count as differences found, like the wants of its own sessions.
func TestRoundIntervalServedWants(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[lower.ByteString()] = status(1) // equal: no differing digest
	h := newScheduledHarness(t, svc, lower)
	waitFor(t, "first round", func() bool { return h.r.CurrentRoundInterval() == 2*reconciler.MinRoundInterval })
	h.r.Wants(lower, make([]storer.ChunkID, 3))
	waitFor(t, "fetch", func() bool { return svc.count(svc.fetched, lower) == 3 })
	h.clock.Add(2 * reconciler.MinRoundInterval) // the round timer
	waitFor(t, "second round", func() bool { return h.r.CurrentRoundInterval() != 2*reconciler.MinRoundInterval })
	if got := h.r.CurrentRoundInterval(); got != reconciler.MinRoundInterval {
		t.Fatalf("round interval %v after served wants, want %v", got, reconciler.MinRoundInterval)
	}
}

// TestRoundTakeoverNestedRoots: the takeover of a lower peer whose root
// differs from this node's waits for MaxRoundInterval rounds, not for this
// node's current interval, which its other peers keep short.
func TestRoundTakeoverNestedRoots(t *testing.T) {
	t.Parallel()

	svc := newFakeService(selfRoot(1, 5))
	svc.remote[lower.ByteString()] = selfRoot(0, 10) // contains this node's root
	svc.remote[higher.ByteString()] = selfRoot(1, 6) // same root, differs: the interval stays short
	h := newGossipHarnessOptions(t, svc, reconciler.Options{Jitter: func() float64 { return 0 }}, lower, higher)
	h.r.Round()
	h.clock.Add(3 * reconciler.MinRoundInterval) // past a wait at the current interval
	h.r.Round()
	if got := h.r.CurrentRoundInterval(); got != reconciler.MinRoundInterval {
		t.Fatalf("round interval %v, want %v", got, reconciler.MinRoundInterval)
	}
	assertSessions(t, svc, lower, 0)
	h.clock.Add(2*reconciler.MaxRoundInterval - 3*reconciler.MinRoundInterval - time.Millisecond)
	h.r.Round()
	assertSessions(t, svc, lower, 0)
	h.clock.Add(time.Millisecond)
	h.r.Round()
	assertSessions(t, svc, lower, 1)
}

// stripesTo returns a stripe per peer, each a different part of the root.
func stripesTo(peers ...swarm.Address) []reconciler.Stripe {
	out := make([]reconciler.Stripe, len(peers))
	for i, p := range peers {
		out[i] = reconciler.Stripe{Range: fingerprint.NewRange(swarm.ZeroAddress, 0).Child(4, uint64(i)), Peers: []swarm.Address{p}}
	}
	return out
}

// TestStripeBusyDoesNotStall: stripes whose peers keep answering Busy free
// their stripe and session slots while they wait to retry, so an honest
// stripe runs at once, also when round sessions hold the other session
// slots.
func TestStripeBusyDoesNotStall(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	busy := make([]swarm.Address, reconciler.MaxStripeSessions)
	for i := range busy {
		busy[i] = addr(0xf1, byte(i))
		svc.busyN[busy[i].ByteString()] = 1 << 30
	}
	honest := addr(0xf2, 0)
	rounds := make([]swarm.Address, reconciler.MaxInitiatedSessions-reconciler.MaxStripeSessions)
	gate := make(chan struct{})
	for i := range rounds {
		rounds[i] = addr(0xf0, byte(i+1))
		svc.peerGate[rounds[i].ByteString()] = gate
		svc.remote[rounds[i].ByteString()] = status(2)
	}
	t.Cleanup(func() { close(gate) })
	h := newGossipHarness(t, svc, rounds...)
	h.r.Round()
	waitFor(t, "round sessions", func() bool { return svc.running() == len(rounds) })
	ctx, cancel := context.WithCancel(context.Background())
	busyDone := make(chan struct{})
	go func() {
		defer close(busyDone)
		h.r.TransferStripes(ctx, stripesTo(busy...))
	}()
	t.Cleanup(func() {
		cancel()
		<-busyDone
	})
	for _, p := range busy {
		waitFor(t, "busy answer", func() bool { return len(svc.sessionOpens(p)) > 0 })
	}

	hctx, hcancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer hcancel()
	if res := h.r.TransferStripes(hctx, stripesTo(honest)); res[0].Err != nil {
		t.Fatalf("honest stripe behind Busy stripes: %v", res[0].Err)
	}
}

// TestStripeBusyRetriesBounded: an attempt that keeps getting Busy gives up
// after MaxStripeBusyRetries retries, so a stripe is never stuck on such a
// peer (runStripe reassigns it).
func TestStripeBusyRetriesBounded(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	p := addr(0xf1, 0)
	svc.busyN[p.ByteString()] = 1 << 30
	h := newStripeHarness(t, svc, time.Hour, time.Millisecond)
	res := h.r.TransferStripes(context.Background(), stripesTo(p))
	if !errors.Is(res[0].Err, rangesync.ErrPeerBusy) {
		t.Fatalf("stripe error %v, want Busy", res[0].Err)
	}
	if n := len(svc.sessionOpens(p)); n != reconciler.MaxStripeBusyRetries+1 {
		t.Fatalf("%d attempts, want %d", n, reconciler.MaxStripeBusyRetries+1)
	}
}

// TestStripeTimeoutPausedWhileBusy: the waits between Busy retries do not
// count towards the stripe timeout, while the time the attempts hold their
// slots adds up across the Busy answers.
func TestStripeTimeoutPausedWhileBusy(t *testing.T) {
	t.Parallel()

	const timeout = 300 * time.Millisecond
	p := addr(0xf1, 0)
	svc := newFakeService(status(1))
	svc.busyN[p.ByteString()] = 5
	h := newStripeHarness(t, svc, timeout, 100*time.Millisecond)
	start := h.clock.Now()
	if res := h.r.TransferStripes(context.Background(), stripesTo(p)); res[0].Err != nil {
		t.Fatalf("stripe: %v", res[0].Err)
	}
	if d := h.clock.Now().Sub(start); d < 5*100*time.Millisecond {
		t.Fatalf("stripe took %v, want 500 ms of retry waits (past the %v timeout)", d, timeout)
	}

	// the time the attempts hold their slots adds up: 100 ms per Busy answer
	svc2 := newFakeService(status(1))
	svc2.busyN[p.ByteString()] = 5
	h2 := newStripeHarness(t, svc2, timeout, 10*time.Millisecond)
	svc2.sessionHook = func(swarm.Address, int) { h2.clock.Add(100 * time.Millisecond) }
	if res := h2.r.TransferStripes(context.Background(), stripesTo(p)); !errors.Is(res[0].Err, reconciler.ErrStripeTimeout) {
		t.Fatalf("stripe: %v, want a timeout after 300 ms of Busy answers", res[0].Err)
	}
}

// TestStripeFetchBoundedByTimeout: the stripe timeout, on the reconciler's
// clock, also bounds the fetch of the listed IDs after the session ended.
func TestStripeFetchBoundedByTimeout(t *testing.T) {
	t.Parallel()

	const timeout = time.Hour // only the fake clock gets there
	svc := newFakeService(status(1))
	p := addr(0xf1, 0)
	svc.wants[p.ByteString()] = 3
	calls := make(chan chan struct{}, 1)
	svc.fetchCalls = calls // the fetch never ends on its own
	h := newStripeHarness(t, svc, timeout, time.Second)
	done := make(chan []reconciler.StripeResult, 1)
	go func() { done <- h.r.TransferStripes(context.Background(), stripesTo(p)) }()
	<-calls // the fetch started
	waitFor(t, "session ended", func() bool { return svc.running() == 0 })
	select {
	case res := <-done:
		t.Fatalf("stripe ended before its timeout: %v", res[0].Err)
	default:
	}

	h.clock.Add(timeout)
	select {
	case res := <-done:
		if !errors.Is(res[0].Err, reconciler.ErrStripeTimeout) {
			t.Fatalf("stripe: %v, want a timeout", res[0].Err)
		}
		if res[0].Listed != 3 {
			t.Fatalf("listed %d, want 3", res[0].Listed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the fetch after the session outlived the stripe timeout")
	}
}

// dripService is a fakeService whose Fetch calls from peer drip wait for the
// test: each sends a channel on calls and waits until the test closes it (or
// the fetch's context ends), then stores its IDs.
type dripService struct {
	*fakeService
	drip  swarm.Address
	calls chan chan struct{}
}

func (s *dripService) Fetch(ctx context.Context, peer swarm.Address, ids []storer.ChunkID) (int, error) {
	if peer.Equal(s.drip) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		release := make(chan struct{})
		select {
		case s.calls <- release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
		select {
		case <-release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	return s.fakeService.Fetch(ctx, peer, ids)
}

// TestStripeInactivityTimeout: StripeTimeout is an inactivity timeout (T4.7
// part 1 fix round 1). A peer that keeps delivering, one chunk per fetch
// just within the timeout, runs its stripe as one attempt past the timeout
// (no restart, so nothing is listed twice); one that keeps delivering
// without end is reassigned once it spent its budget of StripePeerBudget
// timeouts (as many as its restarts gave it before), not restarted; one
// that stalls is reassigned after one timeout.
func TestStripeInactivityTimeout(t *testing.T) {
	t.Parallel()

	const (
		timeout = time.Hour // only the fake clock gets there
		step    = timeout - timeout/10
	)
	a, b := addr(0xf1, 0), addr(0xf1, 1)
	for _, tc := range []struct {
		name string
		// fetches is the number of a's fetches the test releases, each step
		// after the last, then the rest stall; wants is the IDs a lists,
		// one per fetch
		fetches, wants int
		// peer delivered the stripe after clock time in [min, max)
		peer     swarm.Address
		min, max time.Duration
	}{
		{"slow", 6, 6, a, 6 * step, 6*step + 1},
		{"drip", 1000, 1000, b, reconciler.StripePeerBudget * timeout, reconciler.StripePeerBudget*timeout + 2*step},
		{"stall", 0, 6, b, timeout, timeout + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := newFakeService(status(1))
			fake.wants[a.ByteString()] = tc.wants
			fake.wants[b.ByteString()] = 3
			fake.wantBatches = tc.wants
			svc := &dripService{fakeService: fake, drip: a, calls: make(chan chan struct{})}
			clock := &fakeClock{now: time.Unix(1_000_000, 0)}
			r := reconciler.New(self, &fakeTopology{}, svc, log.Noop, reconciler.Options{
				Now:           clock.Now,
				After:         advancingAfter(clock, time.Second),
				StripeTimeout: timeout,
			})
			r.SetStripeRetryInterval(time.Second)
			t.Cleanup(func() { _ = r.Close() })

			stripe := []reconciler.Stripe{{Range: fingerprint.NewRange(swarm.ZeroAddress, 0), Peers: []swarm.Address{a, b}}}
			start := clock.Now()
			done := make(chan reconciler.StripeResult, 1)
			go func() { done <- r.TransferStripes(context.Background(), stripe)[0] }()
			for released := 0; ; {
				select {
				case release := <-svc.calls:
					if released == tc.fetches { // stall
						clock.Add(timeout)
						continue
					}
					clock.Add(step)
					close(release)
					released++
					continue
				case res := <-done:
					took := clock.Now().Sub(start)
					if res.Err != nil || !res.Peer.Equal(tc.peer) {
						t.Fatalf("stripe from %s: %v; want done by %s", res.Peer, res.Err, tc.peer)
					}
					if took < tc.min || took >= tc.max {
						t.Fatalf("done after %v of clock, want [%v, %v)", took, tc.min, tc.max)
					}
					if n := len(fake.sessionOpens(a)); n != 1 {
						t.Fatalf("%d sessions with %s, want 1: restarted", n, a)
					}
					if tc.peer.Equal(a) && res.Listed != tc.wants {
						t.Fatalf("listed %d, want %d", res.Listed, tc.wants)
					}
				case <-time.After(10 * time.Second):
					t.Fatal("stripe not done")
				}
				break
			}
		})
	}
}

// TestStripeLocalBusy: Busy answers for a cause on this node (its index
// rebuilding) do not count towards MaxStripeBusyRetries and do not reassign
// the stripe; they end at the stripe timeout, and the stripe is restarted
// with the same peer (maxStripeRestarts) before it is reassigned.
func TestStripeLocalBusy(t *testing.T) {
	t.Parallel()

	a, b := addr(0xf1, 0), addr(0xf1, 1)
	stripe := []reconciler.Stripe{{Range: fingerprint.NewRange(swarm.ZeroAddress, 0), Peers: []swarm.Address{a, b}}}

	svc := newFakeService(status(1))
	// more than capped attempts with their restarts could take
	svc.localBusyN[a.ByteString()] = (reconciler.MaxStripeRestarts+1)*(reconciler.MaxStripeBusyRetries+1) + 10
	h := newStripeHarness(t, svc, reconciler.StripeTimeout, time.Millisecond)
	res := h.r.TransferStripes(context.Background(), stripe)[0]
	if res.Err != nil || !res.Peer.Equal(a) || len(res.Tried) != 1 {
		t.Fatalf("stripe from %s (tried %v): %v; want done by %s", res.Peer, res.Tried, res.Err, a)
	}

	// always Busy here: bounded by the timeout, restarted, then reassigned
	const timeout = 20 * time.Millisecond
	svc2 := newFakeService(status(1))
	svc2.localBusyN[a.ByteString()] = 1 << 30
	h2 := newStripeHarness(t, svc2, timeout, time.Millisecond)
	start := h2.clock.Now()
	res = h2.r.TransferStripes(context.Background(), stripe)[0]
	if res.Err != nil || !res.Peer.Equal(b) {
		t.Fatalf("stripe from %s: %v; want done by %s", res.Peer, res.Err, b)
	}
	const attempts = reconciler.MaxStripeRestarts + 1
	if d := h2.clock.Now().Sub(start); d < timeout*attempts || d >= timeout*(attempts+1) {
		t.Fatalf("reassigned after %v, want %d attempts of %v", d, attempts, timeout)
	}
}

// TestStripeLocalBusyBudget: Busy answers for a cause on this node that
// every peer gets (here this node's index rebuilding) draw on one restart
// budget per stripe, which reassigning does not reset: the first peer gets
// 1 + maxStripeRestarts attempts, each further peer one.
func TestStripeLocalBusyBudget(t *testing.T) {
	t.Parallel()

	const timeout = 20 * time.Millisecond
	peers := []swarm.Address{addr(0xf1, 0), addr(0xf1, 1), addr(0xf1, 2), addr(0xf1, 3)} // 1 + MaxStripeReassign
	svc := newFakeService(status(1))
	for _, p := range peers {
		svc.localBusyN[p.ByteString()] = 1 << 30
	}
	h := newStripeHarness(t, svc, timeout, time.Millisecond)
	start := h.clock.Now()
	res := h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: fingerprint.NewRange(swarm.ZeroAddress, 0), Peers: peers}})[0]
	if !errors.Is(res.Err, reconciler.ErrStripeTimeout) || !errors.Is(res.Err, fingerprint.ErrBusy) || !res.Peer.Equal(peers[3]) {
		t.Fatalf("stripe from %s: %v; want a timeout held up by this node, from %s", res.Peer, res.Err, peers[3])
	}
	const attempts = 1 + reconciler.MaxStripeRestarts + reconciler.MaxStripeReassign
	if d := h.clock.Now().Sub(start); d < timeout*attempts || d >= timeout*(attempts+1) {
		t.Fatalf("stripe ended after %v, want %d attempts of %v", d, attempts, timeout)
	}
}

// TestStripePeerSessionLimitIsPeerBusy: a session slot on this node that the
// peer's own session holds (rangesync.ErrPeerSessionLimit) is the peer's
// Busy: the peer can hold it, so it counts towards MaxStripeBusyRetries, off
// the stripe clock, and the stripe is reassigned without a score.
func TestStripePeerSessionLimitIsPeerBusy(t *testing.T) {
	t.Parallel()

	const timeout = 100 * time.Millisecond // above the MaxStripeBusyRetries waits of 1 ms
	a, b := addr(0xf1, 0), addr(0xf1, 1)
	svc := newFakeService(status(1))
	svc.busyN[a.ByteString()] = 1 << 30
	svc.busyErr[a.ByteString()] = rangesync.ErrPeerSessionLimit
	h := newStripeHarness(t, svc, timeout, time.Millisecond)
	start := h.clock.Now()
	res := h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: fingerprint.NewRange(swarm.ZeroAddress, 0), Peers: []swarm.Address{a, b}}})[0]
	if res.Err != nil || !res.Peer.Equal(b) {
		t.Fatalf("stripe from %s: %v; want done by %s", res.Peer, res.Err, b)
	}
	if n := len(svc.sessionOpens(a)); n != reconciler.MaxStripeBusyRetries+1 {
		t.Fatalf("%d sessions with the peer holding the slot, want %d", n, reconciler.MaxStripeBusyRetries+1)
	}
	if d := h.clock.Now().Sub(start); d >= timeout {
		t.Fatalf("stripe took %v, want less than its timeout", d)
	}
	if s := h.r.Score(a); s != 0 {
		t.Fatalf("score of the peer %d", s)
	}
}

// TestStripeBusyPauseFetchPending: the stripe clock does not pause for the
// peer's Busy while the attempt still fetches the IDs an earlier session of
// it listed (that session ended on a Busy cause on this node), so the
// stripe timeout bounds that fetch.
func TestStripeBusyPauseFetchPending(t *testing.T) {
	t.Parallel()

	const timeout = 10 * time.Second
	p := addr(0xf1, 0)
	svc := newFakeService(status(1))
	svc.wants[p.ByteString()] = 3
	svc.sessionErrAfter[p.ByteString()] = fmt.Errorf("index: %w", fingerprint.ErrBusy) // the first session's end
	svc.sessionHook = func(peer swarm.Address, n int) {
		if n == 2 { // later sessions: the peer is Busy
			svc.set(func() { svc.busyN[peer.ByteString()] = 1 << 30 })
		}
	}
	calls := make(chan chan struct{}, 1)
	svc.fetchCalls = calls // the fetch never ends on its own
	h := newStripeHarness(t, svc, timeout, time.Second)
	done := make(chan reconciler.StripeResult, 1)
	go func() { done <- h.r.TransferStripes(context.Background(), stripesTo(p))[0] }()
	select {
	case res := <-done:
		if !errors.Is(res.Err, reconciler.ErrStripeTimeout) {
			t.Fatalf("stripe: %v, want a timeout", res.Err)
		}
		if n := len(svc.sessionOpens(p)); n > int(timeout/time.Second)+1 {
			t.Fatalf("%d sessions, want at most %d: Busy waits paused the clock while the fetch ran", n, int(timeout/time.Second)+1)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the fetch outlived the stripe timeout: the clock paused while it ran")
	}
}

// TestStripeClockCheckedAfterWaits: an attempt whose timeout ran out while
// it waited, for its peer's round session to end or for its slots after a
// Busy answer, starts no session after the wait, even when the clock's
// goroutine has not seen the expiry (here it never does).
func TestStripeClockCheckedAfterWaits(t *testing.T) {
	t.Parallel()

	const (
		timeout = time.Hour
		retry   = 7 * time.Second
	)
	newStripes := func(t *testing.T, svc *fakeService, peers ...swarm.Address) (*harness, *stripeWaits) {
		t.Helper()
		h := &harness{svc: svc, topo: &fakeTopology{peers: peers}, clock: &fakeClock{now: time.Unix(1_000_000, 0)}}
		w := newStripeWaits(h.clock, retry, false)
		h.r = reconciler.New(self, h.topo, svc, log.Noop, reconciler.Options{Now: h.clock.Now, After: w.after, StripeTimeout: timeout})
		h.r.SetStripeRetryInterval(retry)
		t.Cleanup(func() {
			w.close()
			_ = h.r.Close()
		})
		return h, w
	}

	t.Run("peer", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService(status(1))
		svc.remote[higher.ByteString()] = status(1)
		h, _ := newStripes(t, svc, higher)
		h.r.Round()
		waitState(t, h.r, higher, reconciler.StateIdle)
		gate := make(chan struct{})
		svc.set(func() {
			svc.remote[higher.ByteString()] = status(2)
			svc.peerGate[higher.ByteString()] = gate
			svc.reconErr[higher.ByteString()] = errors.New("stream reset") // the round session's end
		})
		h.r.Round() // a round session holds the peer
		waitFor(t, "round session", func() bool { return len(svc.sessionOpens(higher)) == 1 })
		done := make(chan reconciler.StripeResult, 1)
		go func() { done <- h.r.TransferStripes(context.Background(), stripesTo(higher))[0] }()
		waitFor(t, "stripe waiting for the peer", func() bool { return h.r.StripeWaiters(higher) == 1 })

		h.clock.Add(timeout)
		close(gate)
		res := <-done
		if !errors.Is(res.Err, reconciler.ErrStripeTimeout) {
			t.Fatalf("stripe: %v, want a timeout", res.Err)
		}
		if n := len(svc.sessionOpens(higher)); n != 1 {
			t.Fatalf("%d sessions, want only the round session: the stripe's started past its timeout", n)
		}
	})

	t.Run("slots", func(t *testing.T) {
		t.Parallel()
		p := addr(0xf1, 0)
		svc := newFakeService(status(1))
		svc.wants[p.ByteString()] = 3
		svc.sessionErrAfter[p.ByteString()] = fmt.Errorf("index: %w", fingerprint.ErrBusy)
		svc.sessionHook = func(peer swarm.Address, n int) {
			if n == 2 { // later sessions: the peer is Busy
				svc.set(func() { svc.busyN[peer.ByteString()] = 1 << 30 })
			}
		}
		calls := make(chan chan struct{}, 1)
		svc.fetchCalls = calls // the fetch runs on, so the clock does not pause for the peer's Busy
		gates := make([]chan struct{}, reconciler.MaxStripeSessions)
		blockers := make([]swarm.Address, len(gates))
		for i := range gates {
			blockers[i] = addr(0xf2, byte(i))
			gates[i] = make(chan struct{})
			svc.peerGate[blockers[i].ByteString()] = gates[i]
		}
		t.Cleanup(func() {
			for _, g := range gates[1:] {
				close(g)
			}
		})
		h, w := newStripes(t, svc)
		done := make(chan reconciler.StripeResult, 1)
		go func() { done <- h.r.TransferStripes(context.Background(), stripesTo(p))[0] }()
		w.recv(t) <- h.clock.Now() // after the first session (Busy here)
		wait := w.recv(t)          // after the second (the peer's Busy)
		go h.r.TransferStripes(context.Background(), stripesTo(blockers...))
		waitFor(t, "stripe slots taken", func() bool { return h.r.StripeSlotsTaken() == reconciler.MaxStripeSessions })
		wait <- h.clock.Now()
		settle() // the stripe waits for its slots

		h.clock.Add(timeout)
		close(gates[0])
		select {
		case res := <-done:
			if !errors.Is(res.Err, reconciler.ErrStripeTimeout) {
				t.Fatalf("stripe: %v, want a timeout", res.Err)
			}
		case c := <-w.waits:
			c <- h.clock.Now()
			t.Fatal("the stripe ran a session past its timeout")
		case <-time.After(5 * time.Second):
			t.Fatal("stripe did not end")
		}
		if n := len(svc.sessionOpens(p)); n != 2 {
			t.Fatalf("%d sessions, want 2", n)
		}
	})
}

// TestStripeBusyReassigns: an attempt whose peer stays Busy past
// MaxStripeBusyRetries is reassigned to another peer; Busy is not scored.
func TestStripeBusyReassigns(t *testing.T) {
	t.Parallel()

	a, b := addr(0xf1, 0), addr(0xf1, 1)
	svc := newFakeService(status(1))
	svc.busyN[a.ByteString()] = 1 << 30
	h := newStripeHarness(t, svc, time.Hour, time.Millisecond)
	res := h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: fingerprint.NewRange(swarm.ZeroAddress, 0), Peers: []swarm.Address{a, b}}})[0]
	if res.Err != nil || !res.Peer.Equal(b) || len(res.Tried) != 1 { // a only answered Busy
		t.Fatalf("stripe from %s (tried %v): %v; want done by %s", res.Peer, res.Tried, res.Err, b)
	}
	if got := h.r.StripesTotal("reassigned"); got != 1 {
		t.Fatalf("stripes_total{reassigned} = %v, want 1", got)
	}
	if s := h.r.Score(a); s != 0 {
		t.Fatalf("score of the Busy peer %d", s)
	}
}
