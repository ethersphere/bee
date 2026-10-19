// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
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
	h.r.Wants(lower, make([]storer.ChunkID, 5)) // lower served a session
	waitFor(t, "guard released", h.r.GuardReleased)

	for range 20 {
		h.r.Wants(lower, make([]storer.ChunkID, 5))
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
