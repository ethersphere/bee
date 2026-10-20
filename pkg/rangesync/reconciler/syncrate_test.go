// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"context"
	"encoding/binary"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// pastWindow moves the clock of h past two SyncRate windows, so that every
// chunk counted so far has left the rate.
func pastWindow(h *harness) { h.clock.Add(2*reconciler.SyncRateWindow + 1) }

// TestSyncRateBootstrap: a bootstrap keeps SyncRate above 0 until its stripes
// and their verification have finished; then the stored chunks keep it up
// within the window and it drops to 0 past the window.
func TestSyncRateBootstrap(t *testing.T) {
	t.Parallel()

	other := addr(0xf1, 0)
	svc := newFakeService(status(0))
	svc.remote[higher.ByteString()] = status(5)
	svc.remote[other.ByteString()] = status(5)
	svc.wants[higher.ByteString()] = 10
	svc.wants[other.ByteString()] = 10
	open := gateFetch(t, svc)
	h := newHarness(t, svc, higher, other)

	waitFor(t, "bootstrap registered", func() bool { return h.r.StripeRegistry() > 0 })
	waitFor(t, "guard released", h.r.GuardReleased)
	for range 3 {
		pastWindow(h) // nothing stored yet: only the transfer holds the rate
		if got := h.r.SyncRate(); got <= 0 {
			t.Fatalf("sync rate %v during the bootstrap transfer", got)
		}
		settle()
	}

	open()
	waitFor(t, "verification", func() bool { return h.r.VerificationDiffCount() >= 2 })
	// caught up: same digest everywhere, so no more sessions
	svc.set(func() { svc.local = status(5) })
	waitFor(t, "bootstrap ended", func() bool { return h.r.StripeRegistry() == 0 })
	if got := h.r.SyncRate(); got <= 0 {
		t.Fatalf("sync rate %v within the window of the bootstrap", got)
	}
	settle()
	pastWindow(h)
	if got := h.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v past the window, want 0", got)
	}
}

// TestSyncRateRegisteredStripe: a range registered for a striped transfer
// counts as catch-up in flight while no fetch is running (queued, or between
// its transfer and its verification).
func TestSyncRateRegisteredStripe(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(1)
	h := newHarness(t, svc, higher)
	waitFor(t, "guard released", h.r.GuardReleased)
	if got := h.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v in sync", got)
	}
	rg := fingerprint.NewRange(self, 1)
	h.r.HoldStripe(rg)
	if got := h.r.SyncRate(); got < reconciler.StartupGuardRate {
		t.Fatalf("sync rate %v with a registered stripe, want >= %v", got, reconciler.StartupGuardRate)
	}
	h.r.ReleaseStripe(rg)
	if got := h.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v after the stripe ended", got)
	}
}

// TestSyncRateSteadyStateUploads: live uploads in steady state (small
// differences, in both the sessions this node initiates and those it serves)
// never raise SyncRate above 0.
func TestSyncRateSteadyStateUploads(t *testing.T) {
	t.Parallel()

	lower := addr(0x10, 0) // initiates with self
	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	svc.remote[lower.ByteString()] = status(2)
	svc.wants[higher.ByteString()] = 3
	h := newHarness(t, svc, higher, lower)
	waitFor(t, "status", func() bool { return svc.count(svc.sent, lower) > 0 })
	h.r.Wants(lower, distinctChunkIDs(5)) // lower served a session
	waitFor(t, "guard released", h.r.GuardReleased)

	for range 20 {
		h.r.Wants(lower, distinctChunkIDs(5))
		time.Sleep(10 * time.Millisecond)
		if got := h.r.SyncRate(); got != 0 {
			t.Fatalf("sync rate %v in steady state", got)
		}
	}
	waitFor(t, "fetches", func() bool { return svc.count(svc.fetched, lower) >= 105 && svc.count(svc.fetched, higher) > 0 })
	if got := h.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v in steady state", got)
	}
}

// chunkIDSeq numbers the IDs of distinctChunkIDs.
var chunkIDSeq atomic.Uint32

// distinctChunkIDs returns n ChunkIDs that differ from each other and from
// every other ID it returned (served wants are merged without duplicates).
func distinctChunkIDs(n int) []storer.ChunkID {
	ids := make([]storer.ChunkID, n)
	for i := range ids {
		a := make([]byte, swarm.HashSize)
		binary.BigEndian.PutUint32(a[2:], chunkIDSeq.Add(1))
		a[0] = 0x66
		ids[i] = storer.ChunkID{Address: swarm.NewAddress(a), StampHash: slices.Clone(a)}
	}
	return ids
}

// reserveWorker is the reserve worker's radius decision (pkg/storer
// reserveWorker) for a reserve that stays below its threshold: one tick
// lowers the radius when SyncRate is 0. It reports whether it did.
func reserveWorker(h *harness) bool {
	if h.r.SyncRate() != 0 {
		return false
	}
	h.svc.set(func() {
		if bits := h.svc.local.Root.Bits; bits > 0 {
			h.svc.local = selfRoot(bits-1, 5)
		}
	})
	return true
}

// TestSyncRateRadiusDecreaseSequencing: with the reserve below its threshold
// the radius drops once, SyncRate rises during the new half's transfer, and
// the next decrease waits for it (and for the window), as with the puller.
func TestSyncRateRadiusDecreaseSequencing(t *testing.T) {
	t.Parallel()

	peers := []swarm.Address{higher, addr(0xf1, 0)}
	h := newRadiusFake(t, 2, 2, peers...)
	svc := h.svc
	waitFor(t, "guard released", h.r.GuardReleased)
	open := gateFetch(t, svc)

	if !reserveWorker(h) {
		t.Fatalf("no decrease in sync, sync rate %v", h.r.SyncRate())
	}
	waitFor(t, "new half in flight", func() bool { return countInside(svc, sibling(2), peers...) > 0 })
	for range 3 {
		pastWindow(h)
		if reserveWorker(h) {
			t.Fatal("radius decreased during the new half's transfer")
		}
		settle()
	}

	open()
	waitFor(t, "new half done", func() bool { return h.r.StripeRegistry() == 0 && !h.r.RadiusStriping() })
	var got uint8
	svc.set(func() { got = svc.local.Root.Bits })
	if got != 1 {
		t.Fatalf("root bits %d, want 1", got)
	}
	// the chunks of the transfer keep the rate up within the window
	if reserveWorker(h) {
		t.Fatal("radius decreased within the window of the transfer")
	}
	settle()
	pastWindow(h)
	if !reserveWorker(h) {
		t.Fatalf("no decrease past the window, sync rate %v", h.r.SyncRate())
	}
	waitFor(t, "second half striped", func() bool { return countInside(svc, sibling(1), peers...) > 0 })
}

// decreaseUncovered returns a harness at root bits 2 whose peers have roots
// at bits 2 too, lowered to bits 1: no peer covers the new half.
func decreaseUncovered(t *testing.T) *harness {
	t.Helper()
	peers := []swarm.Address{higher, addr(0xf1, 0)}
	svc := newFakeService(selfRoot(2, 5))
	for _, p := range peers {
		svc.remote[p.ByteString()] = selfRoot(2, 5)
	}
	h := newHarness(t, svc, peers...)
	waitFor(t, "guard released", h.r.GuardReleased)
	return h
}

// TestSyncRateRadiusHalfUncovered: a pending half no peer covers holds
// SyncRate above 0 for at most RadiusHold, then it reads 0.
func TestSyncRateRadiusHalfUncovered(t *testing.T) {
	t.Parallel()

	h := decreaseUncovered(t)
	h.svc.set(func() { h.svc.local = selfRoot(1, 5) })
	waitFor(t, "decrease seen", h.r.RadiusPending)
	settle()
	if h.r.RadiusStriping() {
		t.Fatal("uncovered half registered")
	}
	if got := h.r.SyncRate(); got < reconciler.StartupGuardRate {
		t.Fatalf("sync rate %v with a pending half", got)
	}
	h.clock.Add(reconciler.RadiusHold + time.Second)
	if got := h.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v past the hold, want 0", got)
	}
}

// radiusBlocked lowers the root of a radius harness from bits 2 to 1 while
// block keeps the new half from being registered, checks that SyncRate
// stays above 0, then unblocks and checks the half is transferred and the
// rate reads 0 past the window.
func radiusBlocked(t *testing.T, block, unblock func(h *harness)) {
	t.Helper()
	peers := []swarm.Address{higher, addr(0xf1, 0)}
	h := newRadiusFake(t, 2, 2, peers...)
	waitFor(t, "guard released", h.r.GuardReleased)
	block(h)
	h.svc.set(func() { h.svc.local = selfRoot(1, 5) })
	waitFor(t, "decrease seen", h.r.RadiusPending)
	settle()
	if h.r.RadiusStriping() {
		t.Fatal("blocked half registered")
	}
	if got := h.r.SyncRate(); got < reconciler.StartupGuardRate {
		t.Fatalf("sync rate %v with a blocked half", got)
	}
	unblock(h)
	waitFor(t, "half transferred", func() bool { return !h.r.RadiusPending() && h.r.StripeRegistry() == 0 })
	if countInside(h.svc, sibling(2), peers...) == 0 {
		t.Fatal("half not striped")
	}
	settle()
	pastWindow(h)
	if got := h.r.SyncRate(); got != 0 {
		t.Fatalf("sync rate %v after the half, want 0", got)
	}
}

// TestSyncRateRadiusHalfOverlap: a half that overlaps a registered range
// waits (SyncRate > 0) and is transferred once that range ends.
func TestSyncRateRadiusHalfOverlap(t *testing.T) {
	t.Parallel()

	half := sibling(2)
	radiusBlocked(t,
		func(h *harness) { h.r.HoldStripe(half) },
		func(h *harness) { h.r.ReleaseStripe(half) })
}

// TestSyncRateRadiusHalfRegistryFull: a half dropped by a full registry
// waits (SyncRate > 0) and is transferred once the registry has room.
func TestSyncRateRadiusHalfRegistryFull(t *testing.T) {
	t.Parallel()

	oldHalf := fingerprint.NewRange(self, 2)
	var held []fingerprint.Range
	radiusBlocked(t,
		func(h *harness) {
			for i := range reconciler.MaxStripeRegistry {
				rg := oldHalf.Child(10, uint64(i))
				held = append(held, rg)
				h.r.HoldStripe(rg)
			}
		},
		func(h *harness) {
			for _, rg := range held {
				h.r.ReleaseStripe(rg)
			}
		})
}

// TestSyncRateRadiusHalfRetried: a failed transfer of the new half is
// retried by a later round and completes once the peers deliver.
func TestSyncRateRadiusHalfRetried(t *testing.T) {
	t.Parallel()

	peers := []swarm.Address{higher, addr(0xf1, 0)}
	h := newRadiusFake(t, 2, 2, peers...)
	svc := h.svc
	svc.set(func() {
		for _, p := range peers {
			svc.withhold[p.ByteString()] = true
		}
		svc.local = selfRoot(1, 5)
	})
	waitFor(t, "first attempt failed", func() bool { return h.r.RadiusAttempts() >= 1 })
	svc.set(func() {
		for _, p := range peers {
			svc.withhold[p.ByteString()] = false
		}
	})
	waitFor(t, "half done", func() bool { return !h.r.RadiusPending() })
	if got := h.r.RadiusAttempts(); got < 2 || got > reconciler.MaxRadiusAttempts {
		t.Fatalf("attempts %d, want 2..%d", got, reconciler.MaxRadiusAttempts)
	}
	if got := h.r.StripesTotal("done"); got == 0 {
		t.Fatal("no stripe done after the retry")
	}
}

// TestSyncRateRadiusHalfAttemptsBounded: a half whose transfer keeps failing
// is given up after MaxRadiusAttempts transfers.
func TestSyncRateRadiusHalfAttemptsBounded(t *testing.T) {
	t.Parallel()

	peers := []swarm.Address{higher, addr(0xf1, 0)}
	h := newRadiusFake(t, 2, 2, peers...)
	svc := h.svc
	svc.set(func() {
		for _, p := range peers {
			svc.withhold[p.ByteString()] = true
		}
		svc.local = selfRoot(1, 5)
	})
	waitFor(t, "decrease seen", h.r.RadiusPending)
	waitFor(t, "given up", func() bool { return !h.r.RadiusPending() })
	if got := h.r.RadiusAttempts(); got != reconciler.MaxRadiusAttempts {
		t.Fatalf("attempts %d, want %d", got, reconciler.MaxRadiusAttempts)
	}
}

// sybil returns the overlay of the i-th stalling peer.
func sybil(i int) swarm.Address { return addr(0x20, byte(i)) }

// assertStallsCannotHold starts stalling catch-up fetches on h (start starts
// one from peer, waits until it is in flight and returns the function that
// ends it, storing nothing) and checks that they hold SyncRate above 0 for
// at most CatchUpGrace in all: the first within the grace, then none of
// them over half an hour of the fake clock, staggered, at most four in
// flight, from new overlays and from earlier ones again (reconnects).
func assertStallsCannotHold(t *testing.T, h *harness, start func(peer swarm.Address) (end func())) {
	t.Helper()
	zero := func(when string) {
		t.Helper()
		if got := h.r.SyncRate(); got != 0 {
			t.Fatalf("sync rate %v %s, want 0 (%d catch-up fetches in flight)", got, when, h.r.CatchUpFetches())
		}
	}
	zero("before any catch-up")
	ends := []func(){start(sybil(0))}
	for _, d := range []time.Duration{0, reconciler.CatchUpGrace / 2} {
		h.clock.Add(d)
		if got := h.r.SyncRate(); got < reconciler.StartupGuardRate {
			t.Fatalf("sync rate %v within the grace, want at least %v", got, reconciler.StartupGuardRate)
		}
	}
	h.clock.Add(reconciler.CatchUpGrace)
	zero("past the grace")

	const step = 5 * time.Second
	for i := 1; i <= int(30*time.Minute/step); i++ {
		// sybil(i % 8) last stalled 8 steps ago and ended since
		ends = append(ends, start(sybil(i%8)))
		zero("as a new stall starts")
		h.clock.Add(step)
		zero("while stalls are in flight")
		if len(ends) > 3 {
			ends[0]()
			ends = ends[1:]
			waitFor(t, "stall ended", func() bool { return h.r.CatchUpFetches() == len(ends) })
			zero("after a stall ended")
		}
	}
	for _, end := range ends {
		end()
	}
	waitFor(t, "stalls ended", func() bool { return h.r.CatchUpFetches() == 0 })
	zero("after the stalls")
}

// TestSyncRateStallingPeers (T4.5 part 2c fix round 2): catch-up fetches
// that store nothing hold SyncRate above 0 for at most CatchUpGrace in all,
// node-wide. Peers that stall their fetches, new peers (Sybils with new
// overlays) and peers that come back (reconnects) do not renew the grace,
// however they stagger their fetches; only a chunk that a catch-up stores
// does. Before, each catch-up fetch in flight held SyncRate at
// StartupGuardRate until it ended: a stalled probe for the fetch timeout.
func TestSyncRateStallingPeers(t *testing.T) {
	t.Parallel()

	t.Run("stalled opens", func(t *testing.T) {
		t.Parallel()

		// each fetch waits until the test ends it, then fails storing
		// nothing, as a fetch whose stream open stalls
		svc := newFakeService(status(1))
		svc.fetchCalls = make(chan chan struct{})
		svc.fetchErr = errStalled
		h := newHarness(t, svc) // no neighbor: the guard holds until its timeout
		h.clock.Add(reconciler.StartupGuardTimeout)
		start := func(peer swarm.Address) func() {
			h.r.Wants(peer, distinctChunkIDs(reconciler.CatchUpMinDiff))
			release := <-svc.fetchCalls
			return func() { close(release) }
		}
		assertStallsCannotHold(t, h, start)

		// a catch-up that stores chunks refills the grace once
		svc.set(func() { svc.fetchErr = nil })
		h.r.Wants(addr(0x30, 0), distinctChunkIDs(reconciler.CatchUpMinDiff))
		close(<-svc.fetchCalls) // stores every ID
		waitFor(t, "catch-up stored", func() bool { return h.r.CatchUpFetches() == 0 })
		if got := h.r.SyncRate(); got <= 0 {
			t.Fatalf("sync rate %v after a catch-up stored", got)
		}
		pastWindow(h)
		if got := h.r.SyncRate(); got != 0 {
			t.Fatalf("sync rate %v past the window", got)
		}
		svc.set(func() { svc.fetchErr = errStalled })
		end := start(sybil(0))
		if got := h.r.SyncRate(); got < reconciler.StartupGuardRate {
			t.Fatalf("sync rate %v within the refilled grace", got)
		}
		h.clock.Add(reconciler.CatchUpGrace + reconciler.CatchUpGrace/2)
		if got := h.r.SyncRate(); got != 0 {
			t.Fatalf("sync rate %v past the refilled grace", got)
		}
		end()
	})

	t.Run("held wants", func(t *testing.T) {
		t.Parallel()

		// each fetch claims its IDs and holds them until the test ends it,
		// then they are Undelivered, as a peer that holds its Wants
		svc := newWantService(status(1))
		h := newWantHarness(t, svc)
		h.r.Start(context.Background())
		h.clock.Add(time.Second) // past the start-up guard
		start := func(peer swarm.Address) func() {
			ids := distinctChunkIDs(reconciler.CatchUpMinDiff)
			svc.lists(peer, ids...)
			stall := make(chan struct{})
			svc.wmu.Lock()
			svc.stall[peer.ByteString()] = stall
			svc.wmu.Unlock()
			claimed := svc.claimSignal(peer)
			h.r.Wants(peer, ids)
			waitClosed(t, "claims", claimed)
			return func() { close(stall) }
		}
		assertStallsCannotHold(t, h, start)
		if n := svc.ws.Len(); n != 0 {
			t.Fatalf("%d claims held after the stalls", n)
		}
	})
}
