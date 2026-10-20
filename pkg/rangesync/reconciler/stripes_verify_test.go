// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// verifyHarness returns a started reconciler whose peers higher and other
// have a Status covering the root; higher lists deliver IDs, other verify.
func verifyHarness(t *testing.T, deliver, verify int) (*harness, swarm.Address) {
	t.Helper()
	other := addr(0xf1, 0)
	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	svc.remote[other.ByteString()] = status(2)
	svc.wants[higher.ByteString()] = deliver
	svc.wants[other.ByteString()] = verify
	h := newHarness(t, svc, higher, other)
	waitFor(t, "status", func() bool { return svc.count(svc.sent, other) > 0 && svc.count(svc.sent, higher) > 0 })
	waitAdmitted(t, h.r, higher, other)
	return h, other
}

// TestVerifyStripesWithheld: a deliverer that withholds 5% of its stripe
// (does not list it) completes the stripe; the verification session with
// another peer finds and fetches the rest, and the deliverer is scored.
func TestVerifyStripesWithheld(t *testing.T) {
	t.Parallel()

	const stripe = 100
	h, other := verifyHarness(t, stripe*95/100, stripe*5/100)
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)

	res := h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})
	if res[0].Err != nil || !res[0].Peer.Equal(higher) {
		t.Fatalf("transfer: peer %s, err %v", res[0].Peer, res[0].Err)
	}
	if got := h.r.Score(higher); got != 0 {
		t.Fatalf("score before verification %d", got)
	}

	fetched := h.svc.count(h.svc.fetched, other) // round sessions fetch too
	v := h.r.VerifyStripes(context.Background(), res)[0]
	if v.Err != nil || !v.Verifier.Equal(other) || !v.Deliverer.Equal(higher) {
		t.Fatalf("verifier %s, deliverer %s, err %v", v.Verifier, v.Deliverer, v.Err)
	}
	if v.Diff != 5 || v.Stored != 5 {
		t.Fatalf("diff %d, stored %d, want 5", v.Diff, v.Stored)
	}
	if got := h.svc.count(h.svc.fetched, other) - fetched; got < 5 {
		t.Fatalf("fetched %d from verifier, want 5", got)
	}
	if !slices.ContainsFunc(h.svc.sessionOpens(other), func(o []fingerprint.Range) bool { return len(o) == 1 && o[0].Equal(root) }) {
		t.Fatal("no verification session opened with the stripe")
	}
	if got := h.r.Score(higher); got != 1 {
		t.Fatalf("deliverer score %d, want 1", got)
	}
	if got := h.r.Score(other); got != 0 {
		t.Fatalf("verifier score %d, want 0", got)
	}
	if got := h.r.VerificationDiffCount(); got != 1 {
		t.Fatalf("verification_diff observations %d, want 1", got)
	}
}

// TestVerifyStripesHonest: a verification that finds nothing (or no more
// than the tolerance) leaves the deliverer's score alone.
func TestVerifyStripesHonest(t *testing.T) {
	t.Parallel()

	for _, diff := range []int{0, reconciler.VerifyMinDiff} {
		h, _ := verifyHarness(t, 100, diff)
		root := fingerprint.NewRange(swarm.ZeroAddress, 0)
		res := h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})
		v := h.r.VerifyStripes(context.Background(), res)[0]
		if v.Err != nil || v.Diff != diff {
			t.Fatalf("diff %d: got %d, err %v", diff, v.Diff, v.Err)
		}
		if got := h.r.Score(higher); got != 0 {
			t.Fatalf("diff %d: deliverer score %d, want 0", diff, got)
		}
	}
}

// TestVerifyStripesNoOtherPeer: with no peer other than the deliverer
// covering the stripe, verification is skipped (ErrNoVerifier), no session
// is opened and nobody is scored.
func TestVerifyStripesNoOtherPeer(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	svc.wants[higher.ByteString()] = 10
	h := newHarness(t, svc, higher)
	waitFor(t, "status", func() bool { return svc.count(svc.sent, higher) > 0 })

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})
	before := len(svc.sessionOpens(higher))
	v := h.r.VerifyStripes(context.Background(), res)[0]
	if !errors.Is(v.Err, reconciler.ErrNoVerifier) {
		t.Fatalf("got %v, want %v", v.Err, reconciler.ErrNoVerifier)
	}
	if got := len(svc.sessionOpens(higher)); got != before {
		t.Fatalf("%d sessions with the deliverer, want %d", got, before)
	}
	if got := h.r.Score(higher); got != 0 {
		t.Fatalf("score %d", got)
	}
}

// TestVerifyStripesFailedNotVerified: a stripe that did not complete is
// not verified.
func TestVerifyStripesFailedNotVerified(t *testing.T) {
	t.Parallel()

	r := reconciler.New(self, nil, newFakeService(status(0)), log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	v := r.VerifyStripes(context.Background(), []reconciler.StripeResult{{Range: root, Peer: higher, Err: reconciler.ErrStripeTimeout}})[0]
	if !errors.Is(v.Err, reconciler.ErrStripeNotDone) {
		t.Fatalf("got %v, want %v", v.Err, reconciler.ErrStripeNotDone)
	}
}

// TestRoundBootstrapVerifies: the stripe worker verifies each bootstrap
// stripe with the peer that did not deliver it before leaving bootstrap.
func TestRoundBootstrapVerifies(t *testing.T) {
	t.Parallel()

	other := addr(0xf1, 0)
	svc := newFakeService(status(0))
	svc.remote[higher.ByteString()] = status(5)
	svc.remote[other.ByteString()] = status(5)
	h := newHarness(t, svc, higher, other)

	// both halves verified, each with the peer that did not deliver it
	waitFor(t, "verification", func() bool { return h.r.VerificationDiffCount() >= 2 })
}

// TestVerifyStripesLyingVerifier: a verifier that lists IDs it does not
// deliver cannot get an honest deliverer scored; the verifier is scored for
// withholding instead.
func TestVerifyStripesLyingVerifier(t *testing.T) {
	t.Parallel()

	h, other := verifyHarness(t, 100, 50)
	h.svc.mu.Lock()
	h.svc.withhold[other.ByteString()] = true
	h.svc.mu.Unlock()
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})
	v := h.r.VerifyStripes(context.Background(), res)[0]
	if v.Err != nil || v.Diff != 50 || v.Stored != 0 {
		t.Fatalf("diff %d, stored %d, err %v", v.Diff, v.Stored, v.Err)
	}
	if got := h.r.Score(higher); got != 0 {
		t.Fatalf("deliverer score %d, want 0", got)
	}
	if got := h.r.Score(other); got != 1 {
		t.Fatalf("verifier score %d, want 1", got)
	}
}

// TestVerifyStripesReassignedTolerance: the tolerance of a reassigned
// stripe is based on what its last deliverer stored, not on what every
// peer that worked on the stripe stored.
func TestVerifyStripesReassignedTolerance(t *testing.T) {
	t.Parallel()

	first := addr(0xe1, 0)
	h, other := verifyHarness(t, 100, 6)
	h.svc.mu.Lock()
	h.svc.wants[first.ByteString()] = 1000
	h.svc.withholdN[first.ByteString()] = 1 // stores 999 of 1000: withheld
	h.svc.mu.Unlock()
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{first, higher}}})
	if res[0].Err != nil || !res[0].Peer.Equal(higher) || res[0].PeerStored != 100 {
		t.Fatalf("peer %s, peer stored %d, err %v", res[0].Peer, res[0].PeerStored, res[0].Err)
	}
	v := h.r.VerifyStripes(context.Background(), res)[0]
	if v.Err != nil || !v.Verifier.Equal(other) {
		t.Fatalf("verifier %s, err %v", v.Verifier, v.Err)
	}
	if got := h.r.Score(higher); got != 1 {
		t.Fatalf("deliverer score %d, want 1", got)
	}
}

// TestVerifyStripesExcludesTried: a peer that already worked on the stripe
// (here withheld it before it was reassigned) is not chosen as verifier.
func TestVerifyStripesExcludesTried(t *testing.T) {
	t.Parallel()

	first := addr(0xe1, 0)
	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	svc.remote[first.ByteString()] = status(2)
	svc.wants[higher.ByteString()] = 10
	svc.wants[first.ByteString()] = 10
	svc.withhold[first.ByteString()] = true
	h := newHarness(t, svc, higher, first)
	waitFor(t, "status", func() bool { return svc.count(svc.sent, first) > 0 && svc.count(svc.sent, higher) > 0 })

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{first, higher}}})
	if res[0].Err != nil || !res[0].Peer.Equal(higher) || len(res[0].Tried) != 2 {
		t.Fatalf("peer %s, tried %v, err %v", res[0].Peer, res[0].Tried, res[0].Err)
	}
	v := h.r.VerifyStripes(context.Background(), res)[0]
	if !errors.Is(v.Err, reconciler.ErrNoVerifier) {
		t.Fatalf("verifier %s, got %v, want %v", v.Verifier, v.Err, reconciler.ErrNoVerifier)
	}
}

// TestVerifyStripesBusyOnlyPeer: a peer that only answered Busy to a stripe
// (it was then reassigned) ran no session of it, so it is not in Tried and
// may verify the stripe; it is not blamed.
func TestVerifyStripesBusyOnlyPeer(t *testing.T) {
	t.Parallel()

	busy, deliverer := addr(0xe1, 0), addr(0xe2, 0)
	svc := newFakeService(status(1))
	svc.remote[busy.ByteString()] = status(1) // equal digests: no round sessions
	svc.remote[deliverer.ByteString()] = status(1)
	svc.wants[deliverer.ByteString()] = 10
	svc.busyN[busy.ByteString()] = reconciler.MaxStripeBusyRetries + 1 // every answer of one attempt
	h := newStripeHarness(t, svc, time.Hour, time.Millisecond)
	h.topo.setPeers(busy, deliverer)
	h.r.Round()
	waitAdmitted(t, h.r, busy, deliverer)

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{busy, deliverer}}})
	if res[0].Err != nil || !res[0].Peer.Equal(deliverer) {
		t.Fatalf("peer %s, err %v", res[0].Peer, res[0].Err)
	}
	if len(res[0].Tried) != 1 || !res[0].Tried[0].Equal(deliverer) {
		t.Fatalf("tried %v, want only %s", res[0].Tried, deliverer)
	}
	v := h.r.VerifyStripes(context.Background(), res)[0]
	if v.Err != nil || !v.Verifier.Equal(busy) {
		t.Fatalf("verifier %s, err %v; want %s", v.Verifier, v.Err, busy)
	}
	if s := h.r.Score(busy); s != 0 {
		t.Fatalf("score of the Busy peer %d", s)
	}

	// a peer whose session listed IDs ran one, even if it then ended Busy
	// (here this node's index) and the peer answered only Busy after it
	svc2 := newFakeService(status(1))
	svc2.remote[busy.ByteString()] = status(1)
	svc2.remote[deliverer.ByteString()] = status(1)
	svc2.wants[busy.ByteString()] = 3
	svc2.withhold[busy.ByteString()] = true
	svc2.sessionErrAfter[busy.ByteString()] = fmt.Errorf("index: %w", fingerprint.ErrBusy)
	svc2.sessionHook = func(p swarm.Address, n int) {
		if n == 2 {
			svc2.set(func() { svc2.busyN[p.ByteString()] = reconciler.MaxStripeBusyRetries + 1 })
		}
	}
	h2 := newStripeHarness(t, svc2, time.Hour, time.Millisecond)
	h2.topo.setPeers(busy, deliverer)
	h2.r.Round()
	waitAdmitted(t, h2.r, busy, deliverer)
	res = h2.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{busy, deliverer}}})
	if res[0].Err != nil || !res[0].Peer.Equal(deliverer) || len(res[0].Tried) != 2 {
		t.Fatalf("peer %s, tried %v, err %v; want done by %s, both tried", res[0].Peer, res[0].Tried, res[0].Err, deliverer)
	}
	if v, ok := h2.r.VerifierFor(res[0]); ok {
		t.Fatalf("verifier %s, a peer that ran a session of the stripe", v)
	}
}

// TestVerifyStripesCanceled: a verification canceled while it fetches ends
// with the context's error and scores nobody.
func TestVerifyStripesCanceled(t *testing.T) {
	t.Parallel()

	h, other := verifyHarness(t, 100, 50)
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})
	h.svc.mu.Lock()
	h.svc.fetchGate = make(chan struct{}) // never opened
	h.svc.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan reconciler.VerifyResult)
	go func() { done <- h.r.VerifyStripes(ctx, res)[0] }()
	waitFor(t, "verification session", func() bool {
		return slices.ContainsFunc(h.svc.sessionOpens(other), func(o []fingerprint.Range) bool { return len(o) == 1 && o[0].Equal(root) })
	})
	cancel()
	v := <-done
	if !errors.Is(v.Err, context.Canceled) {
		t.Fatalf("got %v, want %v", v.Err, context.Canceled)
	}
	if h.r.Score(higher) != 0 || h.r.Score(other) != 0 {
		t.Fatalf("scores %d, %d, want 0", h.r.Score(higher), h.r.Score(other))
	}
}

// TestVerifyStripesVerifierShortfallTolerated: a verifier that delivers all
// but one listed chunk (rejected, or claimed by another session) is not
// scored: the verifier gets the deliverer's tolerance.
func TestVerifyStripesVerifierShortfallTolerated(t *testing.T) {
	t.Parallel()

	h, other := verifyHarness(t, 100, 50)
	h.svc.mu.Lock()
	h.svc.withholdN[other.ByteString()] = 1
	h.svc.mu.Unlock()
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := h.r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})
	v := h.r.VerifyStripes(context.Background(), res)[0]
	if v.Err != nil || v.Diff != 50 || v.Stored != 49 {
		t.Fatalf("diff %d, stored %d, err %v", v.Diff, v.Stored, v.Err)
	}
	if got := h.r.Score(other); got != 0 {
		t.Fatalf("verifier score %d, want 0", got)
	}
}
