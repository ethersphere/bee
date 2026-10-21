// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
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

// TestServedStripeRegisters checks the reconciler as the Striper of the
// sessions it serves: it takes every offered range over and registers it for
// a striped transfer once, but not while the bootstrap plan holds it.
func TestServedStripeRegisters(t *testing.T) {
	t.Parallel()

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	other := addr(0xf1, 0)
	svc := newFakeService(status(1000))
	svc.remote[higher.ByteString()] = status(1000)
	svc.remote[other.ByteString()] = status(1000)
	r := admitted(t, svc, reconciler.Options{}, higher, other)

	rg := root.Child(4, 3)
	if !r.Stripe(rg, 0, rangesync.LargeDiff+1) {
		t.Fatal("offered range declined")
	}
	if got := r.StripeRanges(); !slices.EqualFunc(got, []fingerprint.Range{rg}, fingerprint.Range.Equal) {
		t.Fatalf("registered %v, want %s", got, rg)
	}
	// a range inside a registered one is taken over, not registered again
	if !r.Stripe(rg.Child(4, 1), 0, rangesync.LargeDiff+1) || r.StripeRegistry() != 1 {
		t.Fatalf("registry holds %d ranges after a covered offer, want 1", r.StripeRegistry())
	}

	// bootstrapping (IsBootstrap) before a round registered its root: the
	// bootstrap plan holds the root, nothing is registered
	r.ReleaseStripe(rg)
	svc.set(func() { svc.local = status(0) })
	if !r.Stripe(root, 0, 1000) {
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
	if !r.Stripe(root.Child(1, 1), 0, 1000) || r.StripeRegistry() != 1 {
		t.Fatalf("registry holds %v during the bootstrap, want the root only", r.StripeRanges())
	}

	// closed: taken over, nothing registered
	closed := admitted(t, newFakeService(status(1000)), reconciler.Options{})
	_ = closed.Close()
	if !closed.Stripe(rg, 0, rangesync.LargeDiff+1) || closed.StripeRegistry() != 0 {
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
