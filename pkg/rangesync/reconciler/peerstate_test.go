// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// peerState returns the state of peer in r, or 0 when r does not know it.
func peerState(r *reconciler.Reconciler, peer swarm.Address) reconciler.PeerState {
	s, _ := r.PeerState(peer)
	return s
}

// gatedRound runs a round of h and reports whether it started a session
// with peer, which it then waits for until the peer is Idle again. The
// session is held until the state is read.
func gatedRound(t *testing.T, h *harness, peer swarm.Address) bool {
	t.Helper()
	gate := make(chan struct{})
	h.svc.set(func() { h.svc.reconcileGate = gate })
	n := h.svc.count(h.svc.reconciled, peer)
	h.r.Round()
	started := peerState(h.r, peer) == reconciler.StateReconciling
	close(gate)
	if started {
		waitFor(t, "session", func() bool { return h.svc.count(h.svc.reconciled, peer) == n+1 })
		waitState(t, h.r, peer, reconciler.StateIdle)
	}
	return started
}

// waitAdmitted waits until r follows each of peers: Idle, or in a session,
// fetch or stripe.
func waitAdmitted(t *testing.T, r *reconciler.Reconciler, peers ...swarm.Address) {
	t.Helper()
	for _, p := range peers {
		waitFor(t, "peer admitted", func() bool {
			switch peerState(r, p) {
			case reconciler.StateIdle, reconciler.StateReconciling, reconciler.StateFetching, reconciler.StateStriped:
				return true
			}
			return false
		})
	}
}

// waitState waits until peer is in state want.
func waitState(t *testing.T, r *reconciler.Reconciler, peer swarm.Address, want reconciler.PeerState) {
	t.Helper()
	waitFor(t, "peer "+want.String(), func() bool { return peerState(r, peer) == want })
}

// TestPeerStatePullsyncOnly: a peer that does not speak rangesync ends in
// Unsupported and is ignored: not probed again, no session, no stripe.
func TestPeerStatePullsyncOnly(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.unsupported[higher.ByteString()] = true
	svc.remote[higher.ByteString()] = status(2)
	svc.remote[addr(0xf1, 0).ByteString()] = status(2)
	h := newGossipHarness(t, svc, higher, addr(0xf1, 0))
	h.r.Round()
	if got := peerState(h.r, higher); got != reconciler.StateUnsupported {
		t.Fatalf("pullsync-only peer in %v, want %v", got, reconciler.StateUnsupported)
	}
	for range 3 {
		h.clock.Add(reconciler.StatusMaxInterval)
		h.r.Round()
		h.r.Gossip()
	}
	settle()
	if got := peerState(h.r, higher); got != reconciler.StateUnsupported {
		t.Fatalf("pullsync-only peer in %v, want %v", got, reconciler.StateUnsupported)
	}
	if n := svc.count(svc.sent, higher); n != 1 {
		t.Fatalf("pullsync-only peer probed %d times", n)
	}
	if n := svc.count(svc.reconciled, higher); n != 0 {
		t.Fatalf("%d sessions with a pullsync-only peer", n)
	}
	for _, ps := range h.r.PeerStatuses() {
		if ps.Peer.Equal(higher) {
			t.Fatal("pullsync-only peer offered for stripes")
		}
	}
}

// TestPeerStateDisjointDropped: a peer whose root becomes disjoint from this
// node's after a radius increase is dropped (no session, no stripe) but
// still gets this node's Status; it is followed again once its own radius
// decreases and the roots overlap. (When this node's radius moves back
// instead, the peer's range is the new half of a radius decrease, striped
// rather than reconciled: TestRadius*.)
func TestPeerStateDisjointDropped(t *testing.T) {
	t.Parallel()

	withRoot := func(st rangesync.Status, root fingerprint.Range) rangesync.Status {
		st.Root = root
		return st
	}
	whole := fingerprint.NewRange(swarm.ZeroAddress, 0)
	narrow := fingerprint.NewRange(self, 1)            // 1…
	peerRoot := fingerprint.NewRange(addr(0x00, 0), 1) // 0…: inside whole, disjoint from narrow
	svc := newFakeService(withRoot(status(1), whole))
	svc.remote[higher.ByteString()] = withRoot(status(2), peerRoot)
	h := newGossipHarness(t, svc, higher)
	h.r.Round()
	waitFor(t, "session", func() bool { return svc.count(svc.reconciled, higher) == 1 })
	waitState(t, h.r, higher, reconciler.StateIdle)

	// the radius increases: the roots are disjoint
	h.clock.Add(time.Second)
	svc.set(func() { svc.local = withRoot(status(1), narrow) })
	h.r.Round()
	if got := peerState(h.r, higher); got != reconciler.StateRemoved {
		t.Fatalf("peer with a disjoint root in %v, want %v", got, reconciler.StateRemoved)
	}
	sent := svc.sentStatuses(higher)
	if last := sent[len(sent)-1]; !last.Root.Equal(narrow) {
		t.Fatalf("dropped peer's last Status has root %s, want %s", last.Root, narrow)
	}
	for range 3 {
		h.clock.Add(time.Second)
		h.r.Round()
	}
	settle()
	if n := svc.count(svc.reconciled, higher); n != 1 {
		t.Fatalf("%d sessions with a dropped peer", n-1)
	}
	if got := len(h.r.PeerStatuses()); got != 0 {
		t.Fatalf("%d peers offered for stripes, want 0", got)
	}
	if !h.r.KnowsPeer(higher) {
		t.Fatal("a dropped, connected peer lost its Status gossip entry")
	}

	// the peer's radius decreases: the roots overlap, it is followed again
	h.clock.Add(time.Second)
	svc.set(func() { svc.remote[higher.ByteString()] = withRoot(status(2), whole) })
	h.r.Round()
	waitFor(t, "session after the roots overlap again", func() bool { return svc.count(svc.reconciled, higher) == 2 })
	waitState(t, h.r, higher, reconciler.StateIdle)

	// a disconnect removes it
	h.topo.setPeers()
	h.r.Round()
	if _, ok := h.r.PeerState(higher); ok {
		t.Fatal("disconnected peer still known")
	}
}

// TestPeerStateMappingDropped: a peer with another Mapping is dropped like
// a disjoint one.
func TestPeerStateMappingDropped(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	m := status(2)
	m.Mapping++
	svc.remote[higher.ByteString()] = m
	h := newGossipHarness(t, svc, higher)
	h.r.Round()
	if got := peerState(h.r, higher); got != reconciler.StateRemoved {
		t.Fatalf("peer with another Mapping in %v, want %v", got, reconciler.StateRemoved)
	}
}

// TestPeerStateProbing: a peer stays in Probing until the Statuses are
// exchanged both ways.
func TestPeerStateProbing(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1)) // no Status of the peer yet
	svc.sendErr[higher.ByteString()] = errors.New("stream reset")
	h := newGossipHarness(t, svc, higher)
	h.r.Round()
	if got := peerState(h.r, higher); got != reconciler.StateProbing {
		t.Fatalf("state %v, want %v", got, reconciler.StateProbing)
	}
	svc.set(func() { svc.remote[higher.ByteString()] = status(1) }) // its Status arrives
	h.r.Round()
	if got := peerState(h.r, higher); got != reconciler.StateProbing {
		t.Fatalf("state %v before this node's Status reached the peer, want %v", got, reconciler.StateProbing)
	}
	if got := len(h.r.PeerStatuses()); got != 0 {
		t.Fatalf("%d probing peers offered for stripes, want 0", got)
	}
	svc.set(func() { delete(svc.sendErr, higher.ByteString()) })
	h.clock.Add(reconciler.StatusMinInterval)
	h.r.Round()
	if got := peerState(h.r, higher); got != reconciler.StateIdle {
		t.Fatalf("state %v after the exchange, want %v", got, reconciler.StateIdle)
	}
	if got := h.r.PeerStatuses(); len(got) != 1 || !got[0].Peer.Equal(higher) {
		t.Fatalf("peers offered for stripes %v, want the admitted peer", got)
	}
}

// TestPeerStateSessionBackoff: a peer that answers Busy is retried by the
// next round, then after a backoff that doubles with each Busy or failed
// session in a row; a completed session resets it. Busy is not scored.
func TestPeerStateSessionBackoff(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	h := newGossipHarness(t, svc, higher)
	busy := func(err error) {
		svc.set(func() { svc.reconErr[higher.ByteString()] = err })
	}
	round := func() bool { return gatedRound(t, h, higher) }

	busy(rangesync.ErrPeerBusy)
	if !round() {
		t.Fatal("no first session")
	}
	busy(rangesync.ErrPeerBusy)
	if !round() {
		t.Fatal("first Busy not retried by the next round")
	}
	for i, wait := range []time.Duration{rangesync.MinRoundInterval, 2 * rangesync.MinRoundInterval, 4 * rangesync.MinRoundInterval} {
		if round() {
			t.Fatalf("Busy %d in a row: session within the backoff", i+2)
		}
		h.clock.Add(wait - time.Millisecond)
		if round() {
			t.Fatalf("Busy %d in a row: session %v into a %v backoff", i+2, wait-time.Millisecond, wait)
		}
		h.clock.Add(time.Millisecond)
		if i == 1 {
			busy(errors.New("stream reset")) // an error counts like Busy
		} else {
			busy(rangesync.ErrPeerBusy)
		}
		if !round() {
			t.Fatalf("Busy %d in a row: no session after a %v backoff", i+2, wait)
		}
	}
	if s := h.r.Score(higher); s != 0 {
		t.Fatalf("score %d after Busy answers", s)
	}

	// a completed session resets the backoff
	h.clock.Add(8 * rangesync.MinRoundInterval)
	if !round() {
		t.Fatal("no session after the backoff")
	}
	busy(rangesync.ErrPeerBusy)
	if !round() {
		t.Fatal("no session after a completed one")
	}
	if !round() {
		t.Fatal("first Busy after a completed session not retried by the next round")
	}
}

// TestPeerStateStripeWaitsForSession: a stripe session with a peer waits
// for the peer's round session to end (not for its fetch, which does not
// take the session slot), and a round starts no session with a peer in
// Striped: the two never race for the peer's session slot
// (MaxConcurrentSessionsPerPeer).
func TestPeerStateStripeWaitsForSession(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	svc.wants[higher.ByteString()] = 3
	gate := make(chan struct{})
	svc.reconcileGate = gate
	h := newGossipHarness(t, svc, higher)
	h.r.Round()
	if got := peerState(h.r, higher); got != reconciler.StateReconciling {
		t.Fatalf("state %v, want %v", got, reconciler.StateReconciling)
	}

	done := make(chan reconciler.StripeResult, 1)
	go func() {
		done <- h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})[0]
	}()
	settle()
	if n := len(svc.sessionOpens(higher)); n != 1 {
		t.Fatalf("%d sessions while the round session runs, want 1", n)
	}
	close(gate)
	res := <-done
	if res.Err != nil || res.Stored != 3 {
		t.Fatalf("stripe: stored %d, err %v", res.Stored, res.Err)
	}
	opens := svc.sessionOpens(higher)
	if len(opens) != 2 || len(opens[0]) != 0 || len(opens[1]) != 1 {
		t.Fatalf("sessions %v, want the round session then the stripe session", opens)
	}
	waitState(t, h.r, higher, reconciler.StateIdle)

	// a stripe in flight (its fetch blocks) keeps the peer Striped and out
	// of the rounds
	open := gateFetch(t, svc)
	go func() {
		done <- h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})[0]
	}()
	waitState(t, h.r, higher, reconciler.StateStriped)
	h.r.Round()
	settle()
	if n := len(svc.sessionOpens(higher)); n != 3 {
		t.Fatalf("%d sessions, want 3: a round started a session with a striped peer", n)
	}
	open()
	if res := <-done; res.Err != nil {
		t.Fatalf("stripe: %v", res.Err)
	}
	waitState(t, h.r, higher, reconciler.StateIdle)
	h.r.Round()
	waitFor(t, "round session after the stripe", func() bool { return len(svc.sessionOpens(higher)) == 4 })
	if got := svc.maxConcurrent(higher); got != 1 {
		t.Fatalf("%d concurrent sessions with the peer, want 1", got)
	}
}

// TestPeerStateSessionSlotExclusive runs rounds every 10 ms against peers
// whose sessions hand a large range to the stripe worker: no two sessions
// with one peer ever overlap.
func TestPeerStateSessionSlotExclusive(t *testing.T) {
	t.Parallel()

	other := addr(0xf1, 0)
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	svc := newFakeService(status(1))
	for _, p := range []swarm.Address{higher, other} {
		svc.remote[p.ByteString()] = status(2)
		svc.wants[p.ByteString()] = 2
		svc.offer[p.ByteString()] = root
	}
	svc.reconcileDelay = 5 * time.Millisecond
	h := newHarness(t, svc, higher, other)
	waitFor(t, "stripes", func() bool { return h.r.StripesTotal("done") >= 6 })
	for _, p := range []swarm.Address{higher, other} {
		if got := svc.maxConcurrent(p); got != 1 {
			t.Fatalf("%d concurrent sessions with %s, want 1", got, p)
		}
	}
}

// TestPeerStateStripedDuringBootstrap: the peers that deliver the stripes of
// a bootstrap are in Striped while they do.
func TestPeerStateStripedDuringBootstrap(t *testing.T) {
	t.Parallel()

	other := addr(0xf1, 0)
	svc := newFakeService(status(0))
	for _, p := range []swarm.Address{higher, other} {
		svc.remote[p.ByteString()] = status(5)
		svc.wants[p.ByteString()] = 3
	}
	open := gateFetch(t, svc)
	h := newHarness(t, svc, higher, other)
	waitState(t, h.r, higher, reconciler.StateStriped)
	waitState(t, h.r, other, reconciler.StateStriped)
	open()
	waitFor(t, "stripes done", func() bool { return h.r.StripesTotal("done") >= 2 })
}

// TestRadiusHalfStripedWhenCovered: a new half no neighbor covers is
// striped once a neighbor that covers it appears, even past radiusHold.
func TestRadiusHalfStripedWhenCovered(t *testing.T) {
	t.Parallel()

	h := decreaseUncovered(t)
	h.svc.set(func() { h.svc.local = selfRoot(1, 5) })
	waitFor(t, "decrease seen", h.r.RadiusPending)
	settle()
	if h.r.RadiusStriping() {
		t.Fatal("uncovered half registered")
	}
	h.clock.Add(reconciler.RadiusHold + time.Second)

	cover := addr(0xf2, 0)
	h.svc.set(func() {
		h.svc.remote[cover.ByteString()] = selfRoot(0, 10)
		h.svc.wants[cover.ByteString()] = 2
	})
	h.topo.setPeers(higher, addr(0xf1, 0), cover)
	waitFor(t, "half striped", func() bool { return !h.r.RadiusPending() && countInside(h.svc, sibling(2), cover) > 0 })
}

// TestStatusGossipBackoff: Status sends that keep failing are retried after
// StatusMinInterval, doubling up to StatusMaxInterval; a success resets it.
func TestStatusGossipBackoff(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[lower.ByteString()] = status(1)
	svc.sendErr[lower.ByteString()] = errors.New("stream reset")
	h := newGossipHarness(t, svc, lower)
	g := newGossipLog(h, lower)
	h.r.Round() // the first contact fails
	g.record()
	tick := func(d time.Duration) {
		for end := h.clock.Now().Add(d); h.clock.Now().Before(end); {
			h.clock.Add(time.Second)
			h.r.Gossip()
			g.record()
		}
	}
	tick(5 * time.Minute)

	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second}
	times := g.times[lower.ByteString()]
	for i := 1; i < len(times); i++ {
		w := reconciler.StatusMaxInterval
		if i-1 < len(want) {
			w = want[i-1]
		}
		if d := times[i].Sub(times[i-1]); d != w {
			t.Fatalf("attempt %d %v after the previous one, want %v", i, d, w)
		}
	}
	if len(times) < len(want)+3 {
		t.Fatalf("%d attempts in 5 minutes", len(times))
	}

	// the peer answers again: the next attempt succeeds, the heartbeat
	// follows, and a new failure is retried after StatusMinInterval
	svc.set(func() { delete(svc.sendErr, lower.ByteString()) })
	for n := svc.count(svc.sent, lower); svc.count(svc.sent, lower) == n; {
		h.clock.Add(time.Second)
		h.r.Gossip()
	}
	n := svc.count(svc.sent, lower)
	svc.set(func() { svc.sendErr[lower.ByteString()] = errors.New("stream reset") })
	tick(reconciler.StatusMaxInterval)
	if got := svc.count(svc.sent, lower); got != n+1 {
		t.Fatalf("%d sends in a heartbeat interval, want 1", got-n)
	}
	if got := h.r.Gossip(); got != reconciler.StatusMinInterval {
		t.Fatalf("retry in %v after a first failure, want %v", got, reconciler.StatusMinInterval)
	}
}

// TestStartupGuardBusyPeer: the start-up guard holds while a differing peer
// this node initiates with answers Busy or backs off, and releases once a
// session with it completes.
func TestStartupGuardBusyPeer(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	h := newGossipHarness(t, svc, higher)
	for i := range 2 {
		svc.set(func() { svc.reconErr[higher.ByteString()] = rangesync.ErrPeerBusy })
		if !gatedRound(t, h, higher) {
			t.Fatalf("round %d: no session", i)
		}
		if h.r.GuardReleased() {
			t.Fatalf("guard released after Busy %d", i+1)
		}
	}
	if gatedRound(t, h, higher) {
		t.Fatal("session within the backoff")
	}
	if h.r.GuardReleased() {
		t.Fatal("guard released while the peer backs off")
	}
	h.clock.Add(rangesync.MinRoundInterval)
	if !gatedRound(t, h, higher) {
		t.Fatal("no session after the backoff")
	}
	if !h.r.GuardReleased() {
		t.Fatal("guard held after the session completed")
	}
}

// TestStartupGuardNewPeer: a peer that connects while the start-up guard
// waits, or an Unsupported peer that reconnects with rangesync, keeps the
// guard from releasing until a round has evaluated the peer; the round that a
// topology change triggers waits for MinRoundInterval.
func TestStartupGuardNewPeer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		start []swarm.Address
		join  func(h *harness)
	}{
		{"new peer", []swarm.Address{lower}, func(h *harness) {
			h.topo.setPeers(lower, higher)
		}},
		{"unsupported peer reconnects", []swarm.Address{lower, higher}, func(h *harness) {
			h.svc.set(func() {
				delete(h.svc.unsupported, higher.ByteString())
				h.svc.remote[higher.ByteString()] = status(5)
			})
			h.topo.setPeers(lower, higher) // a topology change of another peer
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newFakeService(status(4))
			svc.remote[lower.ByteString()] = status(5) // initiates: the guard waits for its session
			svc.unsupported[higher.ByteString()] = true
			h := newScheduledHarness(t, svc, tc.start...)
			waitFor(t, "first contact", func() bool { return svc.count(svc.sent, lower) == 1 })
			settle()
			sent := svc.count(svc.sent, higher)
			svc.set(func() { delete(svc.unsupported, higher.ByteString()) })
			svc.set(func() { svc.remote[higher.ByteString()] = status(5) })

			tc.join(h)
			waitFor(t, "first contact with the peer", func() bool { return svc.count(svc.sent, higher) == sent+1 && !h.r.StatusSending(higher) })
			h.r.Succeed(lower) // the lower peer's session
			if h.r.GuardReleased() {
				t.Fatal("guard released before a round evaluated the peer")
			}
			h.clock.Add(reconciler.MinRoundInterval) // the round: a session with the peer
			waitFor(t, "guard released", h.r.GuardReleased)
			if n := svc.count(svc.reconciled, higher); n != 1 {
				t.Fatalf("%d sessions with the peer, want 1", n)
			}
		})
	}
}

// TestPeerStateLocalCauseNoBackoff: a session that did not run for a cause
// on this node (its session cap, its index rebuilding, a stale scope, no
// Status), or because the peer runs its own session with this node (the
// peer's session slot on this node is taken, T4.3), is retried by the next
// round and leaves the peer's Busy streak as it is; the peer's Busy answers
// back off.
func TestPeerStateLocalCauseNoBackoff(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	h := newGossipHarness(t, svc, higher)
	fail := func(err error) {
		svc.set(func() { svc.reconErr[higher.ByteString()] = err })
	}
	for _, err := range []error{
		rangesync.ErrNodeSessionLimit,
		fmt.Errorf("index: %w", fingerprint.ErrBusy),
		fmt.Errorf("session scope: %w", rangesync.ErrDisjointRoots),
		fmt.Errorf("session scope: %w", rangesync.ErrInvalidScope),
		rangesync.ErrNoStatus,
		fmt.Errorf("%w: peer mapping 2", rangesync.ErrMappingMismatch),
		fmt.Errorf("local lookup: %w: reserve has id: leveldb: closed", rangesync.ErrLocal),
		fmt.Errorf("local ids: %w: iterate ids: leveldb: closed", rangesync.ErrLocal),
		rangesync.ErrNodeSessionLimit,
		rangesync.ErrPeerSessionLimit,
		rangesync.ErrPeerSessionLimit,
	} {
		fail(err)
		if !gatedRound(t, h, higher) {
			t.Fatalf("no session after %v", err)
		}
	}
	fail(rangesync.ErrPeerBusy)
	if !gatedRound(t, h, higher) {
		t.Fatal("no session")
	}
	fail(rangesync.ErrPeerBusy)
	if !gatedRound(t, h, higher) {
		t.Fatal("first peer answer in a row not retried by the next round")
	}
	if gatedRound(t, h, higher) {
		t.Fatal("session within the backoff after two peer answers")
	}
}

// TestPeerStateUnsupportedReconnect: a peer in Unsupported whose Status
// arrives reconnected unseen with rangesync: it gets a new machine, this
// node's first contact, and sessions.
func TestPeerStateUnsupportedReconnect(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.unsupported[higher.ByteString()] = true
	h := newGossipHarness(t, svc, higher)
	h.r.Round()
	if got := peerState(h.r, higher); got != reconciler.StateUnsupported {
		t.Fatalf("state %v, want %v", got, reconciler.StateUnsupported)
	}
	h.r.Round()
	if n := svc.count(svc.sent, higher); n != 1 {
		t.Fatalf("unsupported peer probed %d times", n)
	}

	svc.set(func() {
		delete(svc.unsupported, higher.ByteString())
		svc.remote[higher.ByteString()] = status(2)
	})
	h.r.Round()
	if n := svc.count(svc.sent, higher); n != 2 {
		t.Fatalf("%d statuses, want a first contact after the reconnect", n)
	}
	waitFor(t, "session", func() bool { return svc.count(svc.reconciled, higher) == 1 })

	// a Status that was there when the peer was found Unsupported proves
	// nothing; a different one does
	svc2 := newFakeService(status(1))
	svc2.unsupported[higher.ByteString()] = true
	svc2.remote[higher.ByteString()] = status(2)
	h2 := newGossipHarness(t, svc2, higher)
	h2.r.Round()
	h2.r.Round()
	if n := svc2.count(svc2.sent, higher); n != 1 || peerState(h2.r, higher) != reconciler.StateUnsupported {
		t.Fatalf("%d statuses, state %v: re-probed on a Status seen before", n, peerState(h2.r, higher))
	}
	svc2.set(func() {
		delete(svc2.unsupported, higher.ByteString())
		svc2.remote[higher.ByteString()] = status(3)
	})
	h2.r.Round()
	if n := svc2.count(svc2.sent, higher); n != 2 {
		t.Fatalf("%d statuses, want a first contact after a new Status", n)
	}
}

// TestPeerStateStripeDuringFetch: a stripe session starts while the wants of
// the peer's round session are fetched (Fetching -> Striped), and the peer
// is Idle only once both have ended, whichever ends first.
func TestPeerStateStripeDuringFetch(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	for _, tc := range []struct {
		name       string
		roundFirst bool
	}{
		{"round fetch ends first", true},
		{"stripe ends first", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newFakeService(status(1))
			svc.remote[higher.ByteString()] = status(2)
			svc.wants[higher.ByteString()] = 3
			calls := make(chan chan struct{})
			svc.fetchCalls = calls
			h := newGossipHarness(t, svc, higher)
			h.r.Round()
			roundFetch := <-calls
			if got := peerState(h.r, higher); got != reconciler.StateFetching {
				t.Fatalf("state %v during the round fetch, want %v", got, reconciler.StateFetching)
			}
			done := make(chan reconciler.StripeResult, 1)
			go func() {
				done <- h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})[0]
			}()
			stripeFetch := <-calls
			if got := peerState(h.r, higher); got != reconciler.StateStriped {
				t.Fatalf("state %v during the stripe, want %v", got, reconciler.StateStriped)
			}

			if tc.roundFirst {
				close(roundFetch)
				waitFor(t, "round fetch recorded", func() bool { return !h.r.PeerFetching(higher) })
				if got := peerState(h.r, higher); got != reconciler.StateStriped {
					t.Fatalf("state %v after the round fetch, want %v", got, reconciler.StateStriped)
				}
				close(stripeFetch)
				if res := <-done; res.Err != nil {
					t.Fatal(res.Err)
				}
			} else {
				close(stripeFetch)
				if res := <-done; res.Err != nil {
					t.Fatal(res.Err)
				}
				if got := peerState(h.r, higher); got != reconciler.StateFetching {
					t.Fatalf("state %v after the stripe, want %v (round fetch held)", got, reconciler.StateFetching)
				}
				close(roundFetch)
			}
			waitState(t, h.r, higher, reconciler.StateIdle)
		})
	}
}

// TestPeerStateWaiterWoken: a stripe session waiting for a peer's round
// session goes on, untracked, when the peer leaves or is found Unsupported
// (a reconnect the peer set did not show).
func TestPeerStateWaiterWoken(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	for _, tc := range []struct {
		name  string
		event func(h *harness)
		want  reconciler.PeerState // 0: no entry
	}{
		{"disconnect", func(h *harness) {
			h.topo.setPeers()
			h.r.Round()
		}, 0},
		{"unsupported", func(h *harness) {
			h.svc.set(func() { h.svc.unsupported[higher.ByteString()] = true })
			h.clock.Add(reconciler.StatusMaxInterval)
			h.r.Gossip() // the heartbeat finds the peer does not speak rangesync
		}, reconciler.StateUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newFakeService(status(1))
			svc.remote[higher.ByteString()] = status(2)
			gate := make(chan struct{})
			svc.reconcileGate = gate
			h := newGossipHarness(t, svc, higher)
			h.r.Round()
			if got := peerState(h.r, higher); got != reconciler.StateReconciling {
				t.Fatalf("state %v, want %v", got, reconciler.StateReconciling)
			}
			// the round session runs (past its slot and recheck, T4.3)
			waitFor(t, "round session", func() bool { return len(svc.sessionOpens(higher)) == 1 })
			done := make(chan reconciler.StripeResult, 1)
			go func() {
				done <- h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})[0]
			}()
			waitFor(t, "stripe waiting", func() bool { return h.r.StripeWaiters(higher) == 1 })

			tc.event(h)
			waitFor(t, "stripe session", func() bool { return len(svc.sessionOpens(higher)) == 2 })
			if got := peerState(h.r, higher); got != tc.want {
				t.Fatalf("state %v, want %v", got, tc.want)
			}
			close(gate)
			<-done
		})
	}
}

// TestPeerStateReadmittedBackoff: a peer re-admitted after its roots were
// disjoint starts with a new machine: no backoff from before.
func TestPeerStateReadmittedBackoff(t *testing.T) {
	t.Parallel()

	withRoot := func(st rangesync.Status, root fingerprint.Range) rangesync.Status {
		st.Root = root
		return st
	}
	whole := fingerprint.NewRange(swarm.ZeroAddress, 0)
	disjoint := fingerprint.NewRange(addr(0x00, 0), 1)
	svc := newFakeService(withRoot(status(1), fingerprint.NewRange(self, 1)))
	svc.remote[higher.ByteString()] = withRoot(status(2), whole)
	h := newGossipHarness(t, svc, higher)
	for range 2 {
		svc.set(func() { svc.reconErr[higher.ByteString()] = rangesync.ErrPeerBusy })
		if !gatedRound(t, h, higher) {
			t.Fatal("no session")
		}
	}
	if gatedRound(t, h, higher) {
		t.Fatal("session within the backoff")
	}

	svc.set(func() { svc.remote[higher.ByteString()] = withRoot(status(2), disjoint) })
	h.r.Round()
	if got := peerState(h.r, higher); got != reconciler.StateRemoved {
		t.Fatalf("state %v, want %v", got, reconciler.StateRemoved)
	}
	svc.set(func() { svc.remote[higher.ByteString()] = withRoot(status(2), whole) })
	if !gatedRound(t, h, higher) {
		t.Fatal("re-admitted peer still backing off")
	}
}

// reconnectUnseen makes peer reconnect twice between two rounds, unseen by
// the peer set: a heartbeat finds it without rangesync (a new machine, then
// Unsupported), and it then shows status, a Status it had not sent at that
// verdict, so the next round gives it another new machine.
func reconnectUnseen(t *testing.T, h *harness, peer swarm.Address, status rangesync.Status) {
	t.Helper()
	h.svc.set(func() { h.svc.unsupported[peer.ByteString()] = true })
	h.clock.Add(reconciler.StatusMaxInterval)
	h.r.Gossip()
	if got := peerState(h.r, peer); got != reconciler.StateUnsupported {
		t.Fatalf("state %v after the heartbeat, want %v", got, reconciler.StateUnsupported)
	}
	h.svc.set(func() {
		delete(h.svc.unsupported, peer.ByteString())
		h.svc.remote[peer.ByteString()] = status
	})
}

// TestPeerStateResetIgnoresStaleRuns: a peer that reconnects unseen gets a
// new machine; the round session, round fetch and stripe session of the old
// one that still run end without moving the new machine.
func TestPeerStateResetIgnoresStaleRuns(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)

	// The old round fetch ends while the new machine's round fetch runs:
	// the peer stays Fetching, so no session starts before the new fetch
	// ends.
	t.Run("stale fetch", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService(status(10))
		svc.remote[higher.ByteString()] = status(11)
		svc.wants[higher.ByteString()] = 3
		// each completed session hands this range to its Striper, which is
		// registered once the session's fetch is recorded
		svc.offer[higher.ByteString()] = root.Child(1, 0)
		calls := make(chan chan struct{})
		svc.fetchCalls = calls
		h := newGossipHarness(t, svc, higher)
		h.r.Round()
		oldFetch := <-calls
		reconnectUnseen(t, h, higher, status(12))
		h.r.Round()
		newFetch := <-calls
		if got := peerState(h.r, higher); got != reconciler.StateFetching {
			t.Fatalf("state %v during the new fetch, want %v", got, reconciler.StateFetching)
		}

		close(oldFetch)
		waitFor(t, "old fetch recorded", func() bool { return h.r.StripeRegistry() == 1 })
		if got := peerState(h.r, higher); got != reconciler.StateFetching || !h.r.PeerFetching(higher) {
			t.Fatalf("state %v (fetching %v) after the old fetch ended, want %v", got, h.r.PeerFetching(higher), reconciler.StateFetching)
		}
		h.r.Round()
		if n := len(svc.sessionOpens(higher)); n != 2 {
			t.Fatalf("%d sessions, want 2: one started during the new fetch", n)
		}
		close(newFetch)
		waitState(t, h.r, higher, reconciler.StateIdle)
	})

	// The old round session is answered Busy while the new machine's
	// session runs: no backoff, and no second session with the peer.
	t.Run("stale session", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService(status(10))
		svc.remote[higher.ByteString()] = status(11)
		oldGate, newGate := make(chan struct{}), make(chan struct{})
		svc.peerGate[higher.ByteString()] = oldGate
		h := newGossipHarness(t, svc, higher)
		h.r.Round()
		waitFor(t, "old session", func() bool { return len(svc.sessionOpens(higher)) == 1 })
		svc.set(func() {
			svc.peerGate[higher.ByteString()] = newGate
			svc.busyN[higher.ByteString()] = 1 // the old session's answer
		})
		reconnectUnseen(t, h, higher, status(12))
		h.r.Round()
		waitFor(t, "new session", func() bool { return len(svc.sessionOpens(higher)) == 2 })

		close(oldGate)
		waitFor(t, "old session recorded", func() bool { return h.r.SessionsTotal("busy") == 1 })
		if got := peerState(h.r, higher); got != reconciler.StateReconciling {
			t.Fatalf("state %v after the old session ended, want %v", got, reconciler.StateReconciling)
		}
		if n := h.r.PeerFailures(higher); n != 0 {
			t.Fatalf("backoff streak %d from the old session, want 0", n)
		}
		h.r.Round()
		if n := len(svc.sessionOpens(higher)); n != 2 {
			t.Fatalf("%d sessions, want 2: one started during the new session", n)
		}
		close(newGate)
		waitState(t, h.r, higher, reconciler.StateIdle)
	})

	// The old round session gets its slot after the new machine started
	// one: it is stale and does not run, so only one session runs with the
	// peer.
	t.Run("stale session waiting for its slot", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService(status(10))
		blockers := make([]swarm.Address, 0, reconciler.MaxInitiatedSessions+1) // and higher, later
		gates := make([]chan struct{}, reconciler.MaxInitiatedSessions)
		for i := range gates {
			p := addr(0xf1, byte(i))
			blockers = append(blockers, p)
			gates[i] = make(chan struct{})
			svc.peerGate[p.ByteString()] = gates[i]
			svc.remote[p.ByteString()] = status(11)
		}
		t.Cleanup(func() {
			for _, g := range gates[1:] {
				close(g)
			}
		})
		svc.remote[higher.ByteString()] = status(11)
		h := newGossipHarness(t, svc, blockers...)
		h.r.Round()
		waitFor(t, "slots taken", func() bool { return svc.running() == len(blockers) })
		h.topo.setPeers(append(blockers, higher)...)
		h.r.Round() // the old session waits for a slot
		waitState(t, h.r, higher, reconciler.StateReconciling)
		reconnectUnseen(t, h, higher, status(12))
		h.r.Round() // the new session waits behind it
		waitState(t, h.r, higher, reconciler.StateReconciling)

		close(gates[0])
		waitFor(t, "stale session", func() bool { return h.r.SessionsTotal("stale") == 1 })
		waitState(t, h.r, higher, reconciler.StateIdle)
		if n := len(svc.sessionOpens(higher)); n != 1 {
			t.Fatalf("%d sessions with the peer, want 1 (the new machine's)", n)
		}
	})

	// The old stripe session ends while a new one holds the peer: the peer
	// stays Striped.
	t.Run("stale stripe", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService(status(10))
		svc.remote[higher.ByteString()] = status(10) // equal digest: no round session
		oldGate, newGate := make(chan struct{}), make(chan struct{})
		svc.peerGate[higher.ByteString()] = oldGate
		h := newGossipHarness(t, svc, higher)
		h.r.Round()
		waitState(t, h.r, higher, reconciler.StateIdle)
		stripe := []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}}
		oldCtx, cancelOld := context.WithCancel(context.Background())
		oldDone := make(chan struct{})
		go func() {
			defer close(oldDone)
			h.r.TransferStripes(oldCtx, stripe)
		}()
		waitFor(t, "old stripe session", func() bool { return len(svc.sessionOpens(higher)) == 1 })
		svc.set(func() {
			svc.peerGate[higher.ByteString()] = newGate
			svc.local = status(12)
		})
		reconnectUnseen(t, h, higher, status(12))
		h.r.Round()
		waitState(t, h.r, higher, reconciler.StateIdle)
		newDone := make(chan struct{})
		go func() {
			defer close(newDone)
			h.r.TransferStripes(context.Background(), stripe)
		}()
		waitFor(t, "new stripe session", func() bool { return len(svc.sessionOpens(higher)) == 2 })

		cancelOld()
		<-oldDone
		if got := peerState(h.r, higher); got != reconciler.StateStriped {
			t.Fatalf("state %v after the old stripe ended, want %v", got, reconciler.StateStriped)
		}
		close(newGate)
		<-newDone
		waitState(t, h.r, higher, reconciler.StateIdle)
	})
}
