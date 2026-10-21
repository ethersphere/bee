// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"slices"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The lacking side declines (candidate A of AC-B2, T4.7 part 1): the
// reconciler as the Striper of the sessions it serves, and the session-root
// check of the rounds.

// summarizingService is a fakeService that summarizes ranges of this node's
// index (RangeSummary), from sums by range.
type summarizingService struct {
	*fakeService
	sums map[string]fingerprint.Summary
}

func (s summarizingService) RangeSummary(r fingerprint.Range) (fingerprint.Summary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sums[r.String()], nil
}

// statusAt returns a Status with root r and count elements.
func statusAt(r fingerprint.Range, count uint64) rangesync.Status {
	st := status(count)
	st.Root = r
	return st
}

// rootOf returns the range of the first bits bits of a.
func rootOf(a swarm.Address, bits uint8) fingerprint.Range {
	return fingerprint.NewRange(a, bits)
}

// admitted returns a reconciler, not started, of node self with svc and the
// connected peers, after one round has admitted them (their Statuses are in
// svc).
func admitted(t *testing.T, svc reconciler.Service, o reconciler.Options, peers ...swarm.Address) *reconciler.Reconciler {
	t.Helper()
	r := reconciler.New(self, &fakeTopology{peers: peers}, svc, log.Noop, o)
	t.Cleanup(func() { _ = r.Close() })
	r.Round()
	for _, p := range peers {
		if st, ok := r.PeerState(p); !ok || st == reconciler.StateProbing {
			t.Fatalf("peer %s not admitted: %v", p, st)
		}
	}
	return r
}

// roundSessions returns the sessions with peer whose Open is not a stripe:
// those over the session root (no Open).
func roundSessions(svc *fakeService, peer swarm.Address) int {
	n := 0
	for _, open := range svc.sessionOpens(peer) {
		if len(open) == 0 {
			n++
		}
	}
	return n
}

// TestServedStripeRegisters checks the reconciler's Striper of the sessions
// it serves (ServedStriper): it takes every offered range over and
// registers it for a striped transfer once, as the peer's claim, but not
// while the bootstrap plan holds it.
func TestServedStripeRegisters(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	other := addr(0xf1, 0)
	svc := newFakeService(status(1000))
	svc.remote[higher.ByteString()] = status(1000)
	svc.remote[other.ByteString()] = status(1000)
	r := admitted(t, svc, reconciler.Options{}, higher, other)

	rg := root.Child(4, 3)
	if !r.ServedStriper(higher).Stripe(rg, 0, rangesync.LargeDiff+1) {
		t.Fatal("offered range declined")
	}
	if got := r.StripeRanges(); !slices.EqualFunc(got, []fingerprint.Range{rg}, fingerprint.Range.Equal) {
		t.Fatalf("registered %v, want %s", got, rg)
	}
	if n := r.PeerStripes(higher); n != 1 {
		t.Fatalf("%d claims of the peer, want 1", n)
	}
	// a range inside a registered one is taken over, not registered again
	if !r.ServedStriper(other).Stripe(rg.Child(4, 1), 0, rangesync.LargeDiff+1) || r.StripeRegistry() != 1 {
		t.Fatalf("registry holds %d ranges after a covered offer, want 1", r.StripeRegistry())
	}

	// bootstrapping (IsBootstrap) before a round registered its root: the
	// bootstrap plan holds the root, nothing is registered
	r.ReleaseStripe(rg)
	svc.set(func() { svc.local = status(0) })
	if !r.ServedStriper(higher).Stripe(root, 0, 1000) {
		t.Fatal("offered root declined while bootstrapping")
	}
	if n := r.StripeRegistry(); n != 0 {
		t.Fatalf("registered %v while bootstrapping", r.StripeRanges())
	}
	// the round then registers the bootstrap transfer of the root
	r.Round()
	if got := r.StripeRanges(); !slices.EqualFunc(got, []fingerprint.Range{root}, fingerprint.Range.Equal) {
		t.Fatalf("registered %v after the round, want the bootstrap of %s", got, root)
	}
	if !r.ServedStriper(higher).Stripe(root.Child(1, 1), 0, 1000) || r.StripeRegistry() != 1 {
		t.Fatalf("registry holds %v during the bootstrap, want the root only", r.StripeRanges())
	}

	// closed: taken over, nothing registered
	closed := admitted(t, newFakeService(status(1000)), reconciler.Options{})
	_ = closed.Close()
	if !closed.ServedStriper(higher).Stripe(rg, 0, rangesync.LargeDiff+1) || closed.StripeRegistry() != 0 {
		t.Fatalf("registered %v after Close", closed.StripeRanges())
	}
}

// TestRoundLackingSessionRoot checks the session-root check of the rounds
// (constraint (d)): this node starts no round session with a peer that holds
// more than LargeDiff elements more in their session root, and registers the
// session root for a striped transfer instead; a peer that holds fewer, or
// whose count is close, gets its session. The scaled LargeDiff (Options) is
// 100.
func TestRoundLackingSessionRoot(t *testing.T) {
	t.Parallel()

	const largeDiff = 100
	whole := fingerprint.NewRange(swarm.ZeroAddress, 0)
	other := addr(0xf1, 0) // self initiates with it too
	for _, tc := range []struct {
		name          string
		local         rangesync.Status
		remote, other rangesync.Status
		sums          map[string]fingerprint.Summary
		// lacks: no round session with higher, and registered is the
		// range registered for a transfer
		lacks      bool
		registered fingerprint.Range
	}{
		{
			// not a bootstrap: the median of the peers' counts is 1200
			name: "same root, far behind one peer", local: status(1000), remote: status(1000 + largeDiff + 1), other: status(1000),
			lacks: true, registered: whole,
		},
		{
			name: "same root, behind by LargeDiff", local: status(1000), remote: status(1000 + largeDiff), other: status(1000),
		},
		{
			name: "same root, ahead", local: status(1000 + largeDiff + 1), remote: status(1000), other: status(1000),
		},
		{
			// constraint (d): no peer shares this node's root; the session
			// root is the peer's, inside this node's
			name: "the peers' roots inside this node's, empty", local: status(0),
			remote: statusAt(rootOf(higher, 1), 1000), other: statusAt(rootOf(other, 1), 1000),
			sums:  map[string]fingerprint.Summary{rootOf(higher, 1).String(): {}},
			lacks: true, registered: rootOf(higher, 1),
		},
		{
			// constraint (c): an edge peer one bit deeper holds half the
			// count, as this node does in the peer's root
			name: "edge peer one bit deeper", local: status(2000),
			remote: statusAt(rootOf(higher, 1), 1000), other: status(2000),
			sums: map[string]fingerprint.Summary{rootOf(higher, 1).String(): {Count: 1000, Digest: [32]byte{9}}},
		},
		{
			// constraint (d), the other nesting: this node's root inside
			// the peers'; their counts in it are estimated from their roots
			name: "this node's root inside the peers', empty", local: statusAt(rootOf(self, 1), 0),
			remote: status(2000), other: status(2000),
			lacks: true, registered: rootOf(self, 1),
		},
		{
			// constraint (c): this node one bit deeper than its edge peers
			name: "this node one bit deeper, synced", local: statusAt(rootOf(self, 1), 1000),
			remote: status(2000), other: status(2000),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := newFakeService(tc.local)
			fake.remote[higher.ByteString()] = tc.remote
			fake.remote[other.ByteString()] = tc.other
			var svc reconciler.Service = fake
			if tc.sums != nil {
				svc = summarizingService{fakeService: fake, sums: tc.sums}
			}
			r := admitted(t, svc, reconciler.Options{LargeDiff: largeDiff}, higher, other)
			if !tc.lacks {
				waitFor(t, "round session", func() bool { return roundSessions(fake, higher) > 0 })
				if got := r.StripeRanges(); len(got) != 0 {
					t.Fatalf("registered %v", got)
				}
				return
			}
			r.Round() // the first round admitted the peers; this one decides again
			settle()
			if n := roundSessions(fake, higher); n != 0 {
				t.Fatalf("%d round sessions with a peer this node lacks far behind", n)
			}
			if got := r.StripeRanges(); !slices.EqualFunc(got, []fingerprint.Range{tc.registered}, fingerprint.Range.Equal) {
				t.Fatalf("registered %v, want %s", got, tc.registered)
			}
		})
	}
}

// TestTransferring checks the threshold-0 rule (Transferring): this node's
// own transfers (its bootstrap, a radius half) cover the ranges inside them,
// together too, and while one covers a session root a round opens no
// session with a peer that holds more there, however few more; a peer's
// claim covers nothing, and the rounds go on (T4.7 part 1 fix round 1).
func TestTransferring(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	var _ rangesync.TransferStriper = (*reconciler.StripeCollector)(nil)

	svc := newFakeService(status(1000))
	svc.remote[higher.ByteString()] = status(1001)
	r := reconciler.New(self, &fakeTopology{peers: []swarm.Address{higher}}, svc, log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })
	if _, ok := r.ServedStriper(higher).(rangesync.TransferStriper); !ok {
		t.Fatal("the served sessions' Striper is not a TransferStriper")
	}
	if r.Transferring(root) {
		t.Fatal("transferring with an empty registry")
	}
	r.HoldStripe(root.Child(1, 1))
	if !r.Transferring(root.Child(4, 9)) || r.Transferring(root.Child(4, 1)) || r.Transferring(root) {
		t.Fatal("a registered half does not cover exactly its ranges")
	}
	// the union: the other half's quarters complete the root
	r.HoldStripe(root.Child(2, 0))
	r.HoldStripe(root.Child(2, 1))
	if !r.Transferring(root) || !r.Transferring(root.Child(1, 0)) {
		t.Fatal("own transfers that cover a range together do not cover it")
	}
	r.ReleaseStripe(root.Child(2, 1))
	if r.Transferring(root) || r.Transferring(root.Child(1, 0)) || !r.Transferring(root.Child(3, 1)) {
		t.Fatal("own transfers with a gap cover the range across it")
	}
	// a peer's claim, also in the gap, covers nothing
	r.HoldPeerStripe(root.Child(2, 1), lower)
	if r.Transferring(root) || r.Transferring(root.Child(3, 2)) {
		t.Fatal("a peer's claim lowers the threshold")
	}
	for _, rg := range []fingerprint.Range{root.Child(1, 1), root.Child(2, 0), root.Child(2, 1)} {
		r.ReleaseStripe(rg)
	}

	// an own transfer of the session root: no session with a peer one ID
	// ahead
	r.HoldStripe(root)
	r.Round()
	settle()
	if n := roundSessions(svc, higher); n != 0 {
		t.Fatalf("%d round sessions with a peer one ID ahead in a transferred root", n)
	}
	r.ReleaseStripe(root)
	// a peer's claim of the root: the session runs
	r.HoldPeerStripe(root, lower)
	r.Round()
	waitFor(t, "round session", func() bool { return roundSessions(svc, higher) > 0 })
}

// TestClaimCaps checks the bounds on a peer's claims (T4.7 part 1 fix round
// 1): a session, served or opened, registers at most MaxSessionStripes of
// the ranges it takes over, and a peer has at most MaxPeerStripes claims
// registered, trimmed parts counted one by one; the ranges past them are
// taken over all the same. Another peer's claims have their own bound, and
// a claim that ended makes room again.
func TestClaimCaps(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	other := addr(0xf1, 0)
	svc := newFakeService(status(1000))
	svc.remote[higher.ByteString()] = status(1000)
	svc.remote[other.ByteString()] = status(1000)
	r := admitted(t, svc, reconciler.Options{}, higher, other)

	// served sessions of one peer, each offering the 16 children of the
	// root (one opening of 16 ranges)
	serve := func(peer swarm.Address) {
		t.Helper()
		s := r.ServedStriper(peer)
		for i := range uint64(16) {
			if !s.Stripe(root.Child(4, i), 0, rangesync.LargeDiff+1) {
				t.Fatalf("range %d declined", i)
			}
		}
	}
	serve(higher)
	if got := r.StripeRanges(); !slices.EqualFunc(got, []fingerprint.Range{root.Child(4, 0)}, fingerprint.Range.Equal) {
		t.Fatalf("one session registered %v, want %d range", got, reconciler.MaxSessionStripes)
	}
	for range 3 {
		serve(higher)
	}
	if n := r.PeerStripes(higher); n != reconciler.MaxPeerStripes {
		t.Fatalf("the peer holds %d claims, want %d", n, reconciler.MaxPeerStripes)
	}
	// the sessions this node opens register the peer's claims too
	if r.EnqueuePeerStripes(higher, []fingerprint.Range{root.Child(4, 15)}) || r.PeerStripes(higher) != reconciler.MaxPeerStripes {
		t.Fatalf("a session past the peer's bound registered: %v", r.StripeRanges())
	}

	// another peer: one of two ranges of a session, then a trimmed root
	// that leaves the peer room for one part only
	if r.EnqueuePeerStripes(other, []fingerprint.Range{root.Child(4, 14), root.Child(4, 13)}) || r.PeerStripes(other) != 1 {
		t.Fatalf("a session registered %v, want %d range", r.StripeRanges(), reconciler.MaxSessionStripes)
	}
	dropped := r.StripesTotal("dropped")
	if r.ServedStriper(other).Stripe(root, 0, 1000); r.PeerStripes(other) != reconciler.MaxPeerStripes {
		t.Fatalf("the other peer holds %d claims after a trimmed root, want %d", r.PeerStripes(other), reconciler.MaxPeerStripes)
	}
	if r.StripesTotal("dropped") == dropped {
		t.Fatal("trimmed parts past the peer's bound not counted as dropped")
	}

	// a claim that ended makes room
	r.ReleaseStripe(root.Child(4, 0))
	if !r.ServedStriper(higher).Stripe(root.Child(4, 12), 0, rangesync.LargeDiff+1) || r.PeerStripes(higher) != reconciler.MaxPeerStripes {
		t.Fatalf("no claim registered after one ended: %v", r.StripeRanges())
	}
	if !slices.ContainsFunc(r.StripeRanges(), root.Child(4, 12).Equal) {
		t.Fatalf("registered %v, want %s among them", r.StripeRanges(), root.Child(4, 12))
	}
}

// TestOwnJobPreemptsClaims: this node's own transfer is registered over a
// peer's claims and runs first (T4.7 part 1 fix round 1). The bootstrap of
// the root drops the claims inside it and ends the claim in flight; a radius
// decrease's new half drops the claims that overlap it and ends the claim in
// flight, and the other claims stay queued behind it.
func TestOwnJobPreemptsClaims(t *testing.T) {
	t.Parallel()

	liar, other := addr(0xf0, 0), addr(0xf1, 0)
	t.Run("bootstrap", func(t *testing.T) {
		t.Parallel()
		root := fingerprint.NewRange(swarm.ZeroAddress, 0)
		svc := newFakeService(status(1000))
		svc.remote[higher.ByteString()] = status(1000)
		svc.remote[other.ByteString()] = status(1000)
		r := admitted(t, svc, reconciler.Options{}, higher, other)
		inFlight := r.HoldPeerStripe(root.Child(2, 0), liar)
		if !r.EnqueuePeerStripes(liar, []fingerprint.Range{root.Child(2, 1)}) || !r.EnqueuePeerStripes(other, []fingerprint.Range{root.Child(2, 2)}) {
			t.Fatal("claims not registered")
		}
		svc.set(func() { svc.local = status(0) }) // bootstrapping
		r.Round()
		assertStripeRanges(t, r, root)
		if got := r.QueuedStripes(); !slices.EqualFunc(got, []fingerprint.Range{root}, fingerprint.Range.Equal) {
			t.Fatalf("queued %v, want the bootstrap only", got)
		}
		if inFlight.Err() == nil {
			t.Fatal("the claim in flight goes on")
		}
		if !r.Transferring(root.Child(3, 5)) {
			t.Fatal("the bootstrap does not lower the threshold")
		}
	})
	t.Run("radius", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService(selfRoot(1, 5))
		for _, p := range []swarm.Address{higher, other} {
			svc.remote[p.ByteString()] = selfRoot(0, 10)
		}
		r := admitted(t, svc, reconciler.Options{}, higher, other)
		oldHalf, newHalf := fingerprint.NewRange(self, 1), sibling(1)
		inFlight := r.HoldPeerStripe(oldHalf.Child(1, 0), liar)
		if !r.EnqueuePeerStripes(liar, []fingerprint.Range{newHalf.Child(1, 1)}) || !r.EnqueuePeerStripes(other, []fingerprint.Range{oldHalf.Child(1, 1)}) {
			t.Fatal("claims not registered")
		}
		svc.set(func() { svc.local = selfRoot(0, 5) }) // the radius decreased
		r.Round()
		if !r.RadiusStriping() {
			t.Fatal("new half not registered over the claims")
		}
		assertStripeRanges(t, r, oldHalf.Child(1, 1), newHalf)
		if got := r.QueuedStripes(); !slices.EqualFunc(got, []fingerprint.Range{newHalf, oldHalf.Child(1, 1)}, fingerprint.Range.Equal) {
			t.Fatalf("queued %v, want the new half first", got)
		}
		if inFlight.Err() == nil {
			t.Fatal("the claim in flight goes on")
		}
	})
	t.Run("full registry", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService(selfRoot(1, 5))
		for _, p := range []swarm.Address{higher, other} {
			svc.remote[p.ByteString()] = selfRoot(0, 10)
		}
		r := admitted(t, svc, reconciler.Options{}, higher, other)
		// claims of no peer, in the old half, up to the bound
		oldHalf := fingerprint.NewRange(self, 1)
		claims := make([]fingerprint.Range, reconciler.MaxStripeRegistry)
		for i := range claims {
			claims[i] = oldHalf.Child(11, uint64(i))
		}
		if !r.EnqueueStripes(claims) || r.StripeRegistry() != reconciler.MaxStripeRegistry {
			t.Fatalf("registry holds %d claims, want %d", r.StripeRegistry(), reconciler.MaxStripeRegistry)
		}
		svc.set(func() { svc.local = selfRoot(0, 5) }) // the radius decreased
		r.Round()
		if !r.RadiusStriping() || r.StripeRegistry() != reconciler.MaxStripeRegistry+1 {
			t.Fatalf("new half not registered past the claims' bound (%d registered)", r.StripeRegistry())
		}
		if got := r.QueuedStripes(); !got[0].Equal(sibling(1)) {
			t.Fatalf("queued first %s, want the new half", got[0])
		}
	})
}

// TestClaimKeepsRounds is the lying peer of the review (T4.7 part 1 fix
// round 1, Medium 1), over real services: a peer's claim of this node's
// whole root, registered and in flight for as long as the test runs (a liar
// whose stripe stalls), stops neither this node's round sessions with an
// honest peer, in either role, nor the reconciling of their small
// difference (a few chunks this node lacks, as pushsync misses leave). Only
// this node's own transfers lower the threshold to 0.
func TestClaimKeepsRounds(t *testing.T) {
	t.Parallel()

	const largeDiff = 100
	for _, tc := range []struct {
		name string
		a, b swarm.Address // a lacks, b holds the chunks
	}{
		{"this node initiates", addr(0x10, 0), addr(0xa0, 0)},
		{"the peer initiates", addr(0xa0, 0), addr(0x10, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base := baseChunks()
			missed := chunktesting.GenerateTestRandomChunks(3)
			o := roundNodeOptions{roundInterval: 20 * time.Millisecond, striper: true, largeDiff: largeDiff}
			o.chunks = base
			a := newRoundNode(t, tc.a, o, tc.b)
			o.chunks = append(base[:len(base):len(base)], missed...)
			b := newRoundNode(t, tc.b, o, tc.a)
			st, err := a.svc.LocalStatus()
			if err != nil {
				t.Fatal(err)
			}
			stalled := a.r.HoldPeerStripe(st.Root, addr(0xf0, 0))
			b.start(t, []*stripeNode{&a.stripeNode}, nil)
			a.start(t, []*stripeNode{&b.stripeNode}, nil)
			waitFor(t, "the small difference synced", func() bool {
				for _, ch := range missed {
					if !a.has(t, ch) {
						return false
					}
				}
				return true
			})
			if stalled.Err() != nil || a.r.StripeRegistry() == 0 {
				t.Fatal("the claim ended")
			}
		})
	}
}
