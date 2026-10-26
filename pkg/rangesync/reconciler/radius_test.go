// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// radiusNet is a node a (overlay 0x00…, initiating) with peers b and c
// (0x80…, 0xc0…, radius 0) holding every chunk, and a's reconciler.
type radiusNet struct {
	a     *stripeNode
	peers []*stripeNode
	r     *reconciler.Reconciler
	topo  *fakeTopology
}

// round runs a round of a's reconciler (its own rounds are hourly, since
// peers answer Busy to sessions faster than PeerSessionInterval, and a
// topology change defers its round as long).
func (n *radiusNet) round() {
	n.r.Round()
}

func newRadiusNet(t *testing.T, radius uint8, own, all []swarm.Chunk) *radiusNet {
	t.Helper()
	n := &radiusNet{}
	routes := make(map[string]p2p.ProtocolSpec)
	for _, b := range []byte{0x80, 0xc0} {
		a := addr(b, 1)
		p := newStripeNode(t, streamtest.New(streamtest.WithBaseAddr(a)), a, all...)
		routes[a.String()] = p.svc.Protocol()
		n.peers = append(n.peers, p)
	}
	aAddr := addr(0x00, 1)
	n.a = newStripeNodeStore(t, streamtest.New(streamtest.WithBaseAddr(aAddr), streamtest.WithPeerProtocols(routes)), aAddr,
		mockstorer.NewReserve(mockstorer.WithAddrIndex(), mockstorer.WithRadius(radius), mockstorer.WithChunks(own...)))
	addrs := make([]swarm.Address, 0, len(n.peers))
	for _, p := range n.peers {
		p.rec.SetProtocols(n.a.svc.Protocol())
		if err := p.svc.SendStatus(context.Background(), n.a.addr); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "status", func() bool { _, ok := n.a.svc.LastStatus(p.addr); return ok })
		addrs = append(addrs, p.addr)
	}
	n.topo = &fakeTopology{peers: addrs}
	n.r = reconciler.New(n.a.addr, n.topo, n.a.svc, log.Noop, reconciler.Options{RoundInterval: time.Hour})
	t.Cleanup(func() { _ = n.r.Close() })
	n.r.Start(context.Background())
	// streamtest's FullClose returns before the remote handler has stored a
	// Status (libp2p's waits for it), so the handler of the first round's
	// Status can still be pending when a later one is sent, and store after
	// it, overwriting a moved root with the old one. Wait until the peers
	// hold it.
	for _, p := range n.peers {
		waitFor(t, "first status", func() bool { _, ok := p.svc.LastStatus(n.a.addr); return ok })
	}
	return n
}

// sessions returns the opening ranges a wrote, and the number of Messages
// it wrote, per reconcile session with the peers.
func (n *radiusNet) sessions(t *testing.T) (opens []fingerprint.Range, msgs []int) {
	t.Helper()
	for _, p := range n.peers {
		for _, rec := range streamRecords(t, n.a, p.addr, rangesync.ReconcileStreamName) {
			ms, err := protobuf.ReadMessages(bytes.NewReader(rec.In()), func() protobuf.Message { return new(pb.Message) })
			if err != nil || len(ms) == 0 {
				continue // still running
			}
			fp := ms[0].(*pb.Message).Parts[0].GetFP()
			if fp == nil {
				t.Fatalf("opening without a fingerprint: %v", ms[0])
			}
			rg, err := rangesync.ParseRange(fp.Range)
			if err != nil {
				t.Fatal(err)
			}
			opens = append(opens, rg)
			msgs = append(msgs, len(ms))
		}
	}
	return opens, msgs
}

// deliveries returns the number of chunks delivered to a.
func (n *radiusNet) deliveries(t *testing.T) int {
	t.Helper()
	d := 0
	for _, p := range n.peers {
		for _, rec := range streamRecords(t, n.a, p.addr, rangesync.FetchStreamName) {
			ms, err := protobuf.ReadMessages(bytes.NewReader(rec.Out()), func() protobuf.Message { return new(pb.Delivery) })
			if err != nil {
				t.Fatal(err)
			}
			d += len(ms)
		}
	}
	return d
}

func newStripeNodeStore(t *testing.T, rec *streamtest.Recorder, a swarm.Address, store *mockstorer.ReserveStore) *stripeNode {
	t.Helper()
	n := &stripeNode{addr: a, store: store, rec: streamtest.NewRecorderDisconnecter(rec)}
	accept := func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
	n.svc = rangesync.New(n.rec, n.store, n.addr, nil, nil, accept, log.Noop, rangesync.Options{LeafBits: 8})
	t.Cleanup(func() { _ = n.svc.Close() })
	if err := n.svc.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "index", func() bool {
		_, err := n.svc.LocalStatus()
		return err == nil
	})
	return n
}

// waitRoot waits until n's Index is rebuilt at a root of bits.
func waitRoot(t *testing.T, n *stripeNode, bits uint8) {
	t.Helper()
	waitFor(t, "rebuild", func() bool {
		st, err := n.svc.LocalStatus()
		return err == nil && st.Root.Bits == bits
	})
}

// TestRadiusDecreaseStripesNewHalf lowers the radius of one of 3 nodes: only
// the new sibling half is transferred (striped), and the old half costs one
// fingerprint exchange per peer.
func TestRadiusDecreaseStripesNewHalf(t *testing.T) {
	t.Parallel()

	const perQuarter = 6
	all := stripedChunks(2, perQuarter)
	var own, fresh []swarm.Chunk
	for _, ch := range all {
		if ch.Address().Bytes()[0] < 0x80 {
			own = append(own, ch)
		} else {
			fresh = append(fresh, ch)
		}
	}
	n := newRadiusNet(t, 1, own, all)
	oldHalf := fingerprint.NewRange(swarm.ZeroAddress, 1)
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	waitFor(t, "first sessions", func() bool { o, _ := n.sessions(t); return len(o) >= 2 })

	n.a.store.SetStorageRadius(0)
	waitRoot(t, n.a, 0)
	n.round()
	waitFor(t, "new half stored", func() bool {
		for _, ch := range fresh {
			if has, _ := n.a.store.ReserveHasID(chunkIDOf(t, ch).Address, chunkIDOf(t, ch).StampHash); !has {
				return false
			}
		}
		return true
	})
	// a round session that raced a stripe session for the peer's single
	// session slot is retried by the next round
	waitFor(t, "verification", func() bool { return n.r.VerificationDiffCount() >= 2 })
	n.round()
	waitFor(t, "old-half sessions after the decrease", func() bool {
		o, _ := n.sessions(t)
		c := 0
		for _, rg := range o {
			if rg.Equal(oldHalf) {
				c++
			}
		}
		return c >= 3 // 2 before, at least 1 after
	})

	if d := n.deliveries(t); d != len(fresh) {
		t.Fatalf("%d deliveries, want %d (new half only)", d, len(fresh))
	}
	opens, msgs := n.sessions(t)
	for i, rg := range opens {
		switch {
		case rg.Equal(root):
			t.Fatalf("session over the whole new root")
		case rg.Equal(oldHalf) && msgs[i] != 1:
			t.Fatalf("old-half session wrote %d messages, want 1", msgs[i])
		case !oldHalf.ContainsRange(rg) && !root.Child(1, 1).ContainsRange(rg):
			t.Fatalf("unexpected session range %s", rg)
		}
	}
}

// TestRadiusIncreaseNoFetch raises the radius of a node holding every chunk:
// the root narrows and nothing is transferred.
func TestRadiusIncreaseNoFetch(t *testing.T) {
	t.Parallel()

	all := stripedChunks(2, 4)
	n := newRadiusNet(t, 0, all, all) // same digests: no session yet

	n.a.store.SetStorageRadius(1)
	waitRoot(t, n.a, 1)
	n.round()
	// a gossip pass may have had the new root in flight during that round,
	// which then skipped the sessions (T4.3); with the hourly rounds here,
	// run one more once the peers have it
	waitFor(t, "new root sent", func() bool {
		for _, p := range n.peers {
			if st, ok := p.svc.LastStatus(n.a.addr); !ok || st.Root.Bits != 1 || n.r.StatusSending(p.addr) {
				return false
			}
		}
		return true
	})
	n.round()
	waitFor(t, "sessions at the new root", func() bool {
		o, _ := n.sessions(t)
		c := 0
		for _, rg := range o {
			if rg.Bits == 1 {
				c++
			}
		}
		return c >= 2
	})
	settle()
	if d := n.deliveries(t); d != 0 {
		t.Fatalf("%d deliveries after a radius increase", d)
	}
	if got := n.r.StripesTotal("done"); got != 0 {
		t.Fatalf("%v stripes after a radius increase", got)
	}
}

// TestRadiusDecreaseBusyRetried: during a radius decrease the local index
// and the peers' are rebuilding; Busy is retried, not scored or reassigned,
// and the new half is striped while the sessions open only the old half.
func TestRadiusDecreaseBusyRetried(t *testing.T) {
	t.Parallel()

	other := addr(0xf1, 0)
	half := func(bits uint8, count uint64) rangesync.Status {
		st := status(count)
		st.Root = fingerprint.NewRange(self, bits)
		return st
	}
	svc := newFakeService(half(1, 5))
	for _, p := range []swarm.Address{higher, other} {
		svc.remote[p.ByteString()] = half(0, 10)
	}
	h := newHarness(t, svc, higher, other)
	waitFor(t, "first sessions", func() bool { return len(svc.sessionOpens(higher)) > 0 })

	root := fingerprint.NewRange(self, 0)
	oldHalf := fingerprint.NewRange(self, 1)
	newHalf := root.Child(1, 1-root.ChildIndex(self, 1))
	svc.set(func() {
		svc.localErr = fingerprint.ErrBusy // rebuilding
		for _, p := range []swarm.Address{higher, other} {
			svc.reconErr[p.ByteString()] = rangesync.ErrPeerBusy
			svc.wants[p.ByteString()] = 2
		}
	})
	settle()
	before := make(map[string]int)
	svc.set(func() {
		for _, p := range []swarm.Address{higher, other} {
			before[p.ByteString()] = len(svc.opens[p.ByteString()])
		}
		svc.localErr = nil
		svc.local = half(0, 5)
		for _, p := range []swarm.Address{higher, other} {
			svc.reconErr[p.ByteString()] = rangesync.ErrPeerBusy // once more
		}
	})

	waitFor(t, "new half striped", func() bool { return h.r.StripesTotal("done") >= 2 })
	waitFor(t, "old-half session", func() bool {
		for _, p := range []swarm.Address{higher, other} {
			for _, open := range svc.sessionOpens(p) {
				if len(open) == 1 && open[0].Equal(oldHalf) {
					return true
				}
			}
		}
		return false
	})
	if got := h.r.StripesTotal("reassigned"); got != 0 {
		t.Fatalf("stripes reassigned %v after Busy, want 0", got)
	}
	for _, p := range []swarm.Address{higher, other} {
		if s := h.r.Score(p); s != 0 {
			t.Fatalf("score of %s = %d after Busy", p, s)
		}
		// until the peer's last stripe or verification session (the
		// transfer ends after it and root sessions resume), every session
		// opens the old half or a part of the new half
		opens := svc.sessionOpens(p)[before[p.ByteString()]:]
		last := -1
		for i, open := range opens {
			if len(open) == 1 && newHalf.ContainsRange(open[0]) {
				last = i
			}
		}
		for _, open := range opens[:last+1] {
			if len(open) == 0 {
				t.Fatalf("root session with %s during the radius decrease", p)
			}
			for _, rg := range open {
				if !oldHalf.ContainsRange(rg) && !newHalf.ContainsRange(rg) {
					t.Fatalf("session over %s during the radius decrease", rg)
				}
			}
		}
	}
}

// selfRoot returns a Status rooted at self with bits and count.
func selfRoot(bits uint8, count uint64) rangesync.Status {
	st := status(count)
	st.Root = fingerprint.NewRange(self, bits)
	return st
}

// newRadiusFake returns a harness whose node has a root of bits and whose
// peers (roots at bits 0) each list wants IDs per session.
func newRadiusFake(t *testing.T, bits uint8, wants int, peers ...swarm.Address) *harness {
	t.Helper()
	svc := newFakeService(selfRoot(bits, 5))
	for _, p := range peers {
		svc.remote[p.ByteString()] = selfRoot(0, 10)
		svc.wants[p.ByteString()] = wants
	}
	h := newHarness(t, svc, peers...)
	waitFor(t, "first sessions", func() bool { return len(svc.sessionOpens(peers[0])) > 0 })
	return h
}

// sibling returns the child of the root of bits-1 around self that does not
// hold self.
func sibling(bits uint8) fingerprint.Range {
	parent := fingerprint.NewRange(self, bits-1)
	return parent.Child(1, 1-parent.ChildIndex(self, 1))
}

// countInside returns the number of sessions with peers that opened a
// single range inside rg.
func countInside(svc *fakeService, rg fingerprint.Range, peers ...swarm.Address) int {
	n := 0
	for _, p := range peers {
		for _, open := range svc.sessionOpens(p) {
			if len(open) == 1 && rg.ContainsRange(open[0]) {
				n++
			}
		}
	}
	return n
}

// gateFetch blocks every Fetch of svc until the returned func is called.
func gateFetch(t *testing.T, svc *fakeService) (open func()) {
	t.Helper()
	gate := make(chan struct{})
	var once sync.Once
	open = func() { once.Do(func() { close(gate) }) }
	svc.set(func() { svc.fetchGate = gate })
	t.Cleanup(open)
	return open
}

// TestRadiusDecreaseThenIncreaseCancels: a radius increase while the new
// half is in flight cancels its transfer (no further stripe session or
// fetch), and a later decrease stripes the half again.
func TestRadiusDecreaseThenIncreaseCancels(t *testing.T) {
	t.Parallel()

	peers := []swarm.Address{higher, addr(0xf1, 0)}
	h := newRadiusFake(t, 1, 2, peers...)
	svc := h.svc
	open := gateFetch(t, svc)
	newHalf := sibling(1)

	svc.set(func() { svc.local = selfRoot(0, 5) })
	waitFor(t, "stripe in flight", func() bool { return countInside(svc, newHalf, peers...) > 0 })
	svc.set(func() { svc.local = selfRoot(1, 5) }) // radius back up
	waitFor(t, "job canceled", func() bool { return !h.r.RadiusStriping() && h.r.StripeRegistry() == 0 })
	n := countInside(svc, newHalf, peers...)
	open()
	settle()
	if got := countInside(svc, newHalf, peers...); got != n {
		t.Fatalf("%d stripe sessions after the cancel", got-n)
	}
	if got := h.r.StripesTotal("done"); got != 0 {
		t.Fatalf("stripes done %v after the radius increase", got)
	}

	svc.set(func() { svc.local = selfRoot(0, 5) }) // decrease again
	// the fourth root sent back to back goes out PeerStatusInterval later
	// (the sender-side rate), and the stripe sessions wait until it has
	// reached their peers (T6.5 part 1): move the clock on
	waitFor(t, "half striped again", func() bool {
		h.clock.Add(100 * time.Millisecond)
		return h.r.StripesTotal("done") >= 2
	})
}

// TestRadiusTwoDecreases: r -> r-1 -> r-2 while the first half is in
// flight: the first half completes, then the second is striped.
func TestRadiusTwoDecreases(t *testing.T) {
	t.Parallel()

	peers := []swarm.Address{higher, addr(0xf1, 0)}
	h := newRadiusFake(t, 2, 2, peers...)
	svc := h.svc
	open := gateFetch(t, svc)

	svc.set(func() { svc.local = selfRoot(1, 5) })
	waitFor(t, "first half in flight", func() bool { return countInside(svc, sibling(2), peers...) > 0 })
	svc.set(func() { svc.local = selfRoot(0, 5) })
	settle()
	open()
	waitFor(t, "second half striped", func() bool { return countInside(svc, sibling(1), peers...) > 0 })
	waitFor(t, "both done", func() bool { return h.r.StripesTotal("done") >= 4 })
	if !opensExactly(svc, fingerprint.NewRange(self, 1), peers...) {
		t.Fatal("no old-half session at the r-2 root")
	}
}

// opensExactly reports whether a session with any of peers opened exactly
// [rg].
func opensExactly(svc *fakeService, rg fingerprint.Range, peers ...swarm.Address) bool {
	for _, p := range peers {
		for _, open := range svc.sessionOpens(p) {
			if len(open) == 1 && open[0].Equal(rg) {
				return true
			}
		}
	}
	return false
}

// TestRadiusJumpTwoBits: a root that widens by 2 bits at once stripes the
// sibling of the new root's child that holds the old root; that child (old
// root plus one new quarter) goes through the old-half session.
func TestRadiusJumpTwoBits(t *testing.T) {
	t.Parallel()

	peers := []swarm.Address{higher, addr(0xf1, 0)}
	h := newRadiusFake(t, 2, 0, peers...)
	svc := h.svc
	svc.set(func() { svc.local = selfRoot(0, 5) })
	waitFor(t, "half striped", func() bool { return h.r.StripesTotal("done") >= 2 })
	waitFor(t, "old-half session", func() bool { return opensExactly(svc, fingerprint.NewRange(self, 1), peers...) })
	if countInside(svc, sibling(2), peers...) != 0 {
		t.Fatal("quarter next to the old root striped; want it in the old-half session")
	}
}

// TestRadiusHalfFailedFallsBack: after a failed transfer of the new half the
// sessions run over the whole root again.
func TestRadiusHalfFailedFallsBack(t *testing.T) {
	t.Parallel()

	peers := []swarm.Address{higher, addr(0xf1, 0)}
	h := newRadiusFake(t, 1, 2, peers...)
	svc := h.svc
	before := len(svc.sessionOpens(higher)) + len(svc.sessionOpens(peers[1]))
	svc.set(func() {
		for _, p := range peers {
			svc.withhold[p.ByteString()] = true
		}
		svc.local = selfRoot(0, 5)
	})
	waitFor(t, "stripe", func() bool { return countInside(svc, sibling(1), peers...) > 0 })
	waitFor(t, "whole-root session", func() bool {
		n := 0
		for _, p := range peers {
			for _, open := range svc.sessionOpens(p) {
				if len(open) == 0 {
					n++
				}
			}
		}
		return n > before
	})
	if h.r.RadiusStriping() {
		t.Fatal("half still registered")
	}
}

// TestRadiusSkipsSessionInsideNewHalf: while the new half is in flight, a
// peer whose root lies inside it gets no session of its own.
func TestRadiusSkipsSessionInsideNewHalf(t *testing.T) {
	t.Parallel()

	inner := addr(0xf2, 0)
	peers := []swarm.Address{higher, addr(0xf1, 0)}
	svc := newFakeService(selfRoot(1, 5))
	for _, p := range peers {
		svc.remote[p.ByteString()] = selfRoot(0, 10)
		svc.wants[p.ByteString()] = 2
	}
	newHalf := sibling(1)
	in := status(3)
	in.Root = newHalf.Child(1, 0)
	svc.remote[inner.ByteString()] = in
	open := gateFetch(t, svc)
	h := newHarness(t, svc, higher, peers[1], inner)
	waitFor(t, "first sessions", func() bool { return len(svc.sessionOpens(higher)) > 0 })
	if n := len(svc.sessionOpens(inner)); n != 0 {
		t.Fatalf("%d sessions with a disjoint peer", n)
	}
	svc.set(func() { svc.local = selfRoot(0, 5) })
	waitFor(t, "stripe in flight", func() bool { return h.r.RadiusStriping() && countInside(svc, newHalf, peers...) > 0 })
	settle()
	for _, o := range svc.sessionOpens(inner) {
		if len(o) == 0 {
			t.Fatal("session with a peer inside the new half while it is striped")
		}
	}
	open()
}

// TestRadiusHalfOverlapRetried: a new half overlapping a registered range is
// refused and registered by a later round.
func TestRadiusHalfOverlapRetried(t *testing.T) {
	t.Parallel()

	peers := []swarm.Address{higher, addr(0xf1, 0)}
	h := newRadiusFake(t, 1, 0, peers...)
	held := sibling(1).Child(1, 0)
	h.r.HoldStripe(held)
	h.svc.set(func() { h.svc.local = selfRoot(0, 5) })
	settle()
	if h.r.RadiusStriping() {
		t.Fatal("half registered over an overlapping range")
	}
	h.r.ReleaseStripe(held)
	waitFor(t, "half striped", func() bool { return h.r.StripesTotal("done") >= 2 })
}

// TestRadiusStripeWaitsForMovedRoot (T6.5 part 1): after a radius decrease,
// a stripe session of the new half with a peer that this node's Status with
// the new root has not reached yet (a gossip send in flight) waits until it
// has, as round sessions do, instead of opening a session the peer refuses
// (it cannot build the scope from its stale root of this node) and having the
// stripe reassigned. The wait is the peer's Busy, off the stripe clock, and
// ends as soon as the Status reaches the peer, before the stripe retry
// interval (the fake clock does not move).
func TestRadiusStripeWaitsForMovedRoot(t *testing.T) {
	t.Parallel()

	const retry = 1234 * time.Millisecond // unlike any other wait on the clock
	other := addr(0xf1, 0)
	peers := []swarm.Address{higher, other}
	svc := newFakeService(selfRoot(1, 5))
	for _, p := range peers {
		svc.remote[p.ByteString()] = selfRoot(0, 10)
		svc.wants[p.ByteString()] = 2
	}
	h := &harness{svc: svc, topo: &fakeTopology{peers: peers}, clock: &fakeClock{now: time.Unix(1_000_000, 0)}}
	// rounds run at Start and when the test runs them: a triggered round
	// waits an hour on the fake clock
	h.r = reconciler.New(self, h.topo, svc, log.Noop, reconciler.Options{
		Now:           h.clock.Now,
		After:         h.clock.After,
		RoundInterval: time.Hour,
	})
	h.r.SetStripeRetryInterval(retry)
	t.Cleanup(func() {
		if err := h.r.Close(); err != nil {
			t.Error(err)
		}
	})
	h.r.Start(context.Background())
	for _, p := range peers {
		waitFor(t, "first session", func() bool { return len(svc.sessionOpens(p)) > 0 })
		waitState(t, h.r, p, reconciler.StateIdle)
	}

	// the radius decreases; the gossip delivers the new root to other at
	// once, and its send to higher stays in flight
	gate := make(chan struct{})
	svc.set(func() {
		svc.sendGate[higher.ByteString()] = gate
		svc.local = selfRoot(0, 5)
		svc.scoped = true
	})
	go h.r.Gossip()
	waitFor(t, "new root in flight to higher", func() bool {
		var ok bool
		svc.set(func() { ok = svc.gated[higher.ByteString()] > 0 && svc.delivered[other.ByteString()].Root.Bits == 0 })
		return ok
	})
	h.r.Round() // registers the new half for the stripe worker
	waitFor(t, "stripe with higher refused or waiting", func() bool {
		return svc.count(svc.refused, higher) > 0 || h.clock.waiting(retry)
	})
	close(gate)
	waitFor(t, "new half striped and verified", func() bool {
		return !h.r.RadiusStriping() && h.r.StripesTotal("done") >= 2
	})

	if n := svc.count(svc.refused, higher); n != 0 {
		t.Fatalf("%d sessions opened with higher before it had the new root", n)
	}
	if got := h.r.StripesTotal("reassigned"); got != 0 {
		t.Fatalf("stripes reassigned %v, want 0", got)
	}
	if countInside(svc, sibling(1), higher) == 0 {
		t.Fatal("no stripe session with higher")
	}
	for _, p := range peers {
		if s := h.r.Score(p); s != 0 {
			t.Fatalf("score of %s = %d", p, s)
		}
	}
}
