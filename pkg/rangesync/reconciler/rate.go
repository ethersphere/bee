// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"sync"
	"time"
)

// rateWindow is pkg/rate's moving-window rate (copied here so that its clock
// can be injected): chunks per second over the current and the previous
// window, the previous one interpolated by how far the current one is along.
type rateWindow struct {
	mu      sync.Mutex
	windows map[int64]int
	size    int64 // window size in milliseconds
	now     func() time.Time
}

func newRateWindow(size time.Duration, now func() time.Time) *rateWindow {
	return &rateWindow{windows: make(map[int64]int), size: size.Milliseconds(), now: now}
}

// Add counts n chunks in the current window.
func (r *rateWindow) Add(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w := r.now().UnixMilli() / r.size
	r.windows[w] += n
	r.cleanupLocked(w)
}

// Rate returns the rate in chunks per second.
func (r *rateWindow) Rate() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now().UnixMilli()
	w := now / r.size
	r.cleanupLocked(w)
	interpolate := 1 - float64(now-w*r.size)/float64(r.size)
	return 1000 * (float64(r.windows[w]) + interpolate*float64(r.windows[w-1])) / float64(r.size)
}

func (r *rateWindow) cleanupLocked(w int64) {
	for k := range r.windows {
		if k <= w-2 {
			delete(r.windows, k)
		}
	}
}

// catchUpGrace is how long, in all, catch-up fetches in flight hold SyncRate
// at StartupGuardRate while no chunk a catch-up stored is in the rate
// window: the time an honest catch-up needs from its start to its first
// stored chunk (a fetch stream open, the claims, the Want and one delivery),
// with room. It is node-wide and much shorter than the fetch timeout, so
// that fetches that store nothing, whoever starts them, cannot hold the rate
// above 0 (graceBudget).
const catchUpGrace = 2 * time.Second

// ownJobGrace is how long, in all, this node's own striped transfers (its
// bootstrap and a radius decrease's new half, stripeJob.own) hold SyncRate at
// StartupGuardRate while none of them stores a chunk: as long as a pending
// half that is not registered yet (radiusHold), so that registering the
// half does not shorten its hold, and longer than any honest pause of an
// own transfer (a peer's Busy answers, at most maxStripeBusyRetries of
// stripeRetryInterval per attempt; this node's index rebuild; a
// verification round), yet shorter than the reserve worker's wake-up (15
// minutes). It is one budget for the node (ownGrace): a peer cannot start an
// own transfer at will, but a Sybil majority of the peers sharing this node's
// root can make it bootstrap again and again (IsBootstrap), and a budget
// that each bootstrap renewed would keep SyncRate above 0 most of the time
// (T4.7 part 3).
const ownJobGrace = radiusHold

// graceBudget is what is left of a grace of size: catchUpGrace for the
// catch-up fetches in flight, ownJobGrace for this node's own striped
// transfers. It drains while what it covers is in flight (registered) and
// is refilled, to size, only when that stores a chunk: never with time, per
// peer or per fetch, so that no number of peers, overlays, reconnects,
// stalled opens or held Wants renews it. The zero value is spent; the
// Reconciler starts with full ones (newGraceBudget). The Reconciler's mu
// guards it.
type graceBudget struct {
	size     time.Duration
	left     time.Duration // at since
	since    time.Time
	draining bool
}

// newGraceBudget returns a full budget of size.
func newGraceBudget(size time.Duration) graceBudget {
	return graceBudget{size: size, left: size}
}

// leftAt returns what is left at now.
func (g *graceBudget) leftAt(now time.Time) time.Duration {
	if !g.draining || !now.After(g.since) {
		return g.left
	}
	return max(0, g.left-now.Sub(g.since))
}

// holds reports whether anything is left at now.
func (g *graceBudget) holds(now time.Time) bool { return g.leftAt(now) > 0 }

// drain makes the budget drain from now while draining, and stop otherwise.
func (g *graceBudget) drain(now time.Time, draining bool) {
	g.left = g.leftAt(now)
	if now.After(g.since) {
		g.since = now
	}
	g.draining = draining
}

// refill fills the budget at now.
func (g *graceBudget) refill(now time.Time) {
	g.left = g.size
	if now.After(g.since) {
		g.since = now
	}
}
