// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/pusher"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestParked(t *testing.T) {
	t.Parallel()

	const (
		delay = 5 * time.Minute
		ttl   = time.Hour
	)
	t0 := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	batch := []byte("batch")
	a, b := swarm.RandAddress(t), swarm.RandAddress(t)

	p := pusher.NewParked(delay, ttl)

	if !p.Park(a, batch, t0) {
		t.Fatal("first park: got false, want true")
	}
	if !p.Waiting(a, batch, t0.Add(time.Minute)) {
		t.Fatal("before the delay: not waiting")
	}
	if p.Waiting(a, batch, t0.Add(delay)) {
		t.Fatal("after the delay: still waiting")
	}
	if p.Park(a, batch, t0.Add(6*time.Minute)) {
		t.Fatal("second park: got true, want false")
	}

	p.Forget(a, batch)
	if !p.Park(a, batch, t0) {
		t.Fatal("park after forget: got false, want true")
	}

	// an entry left alone is dropped once it is older than the ttl
	if !p.Park(b, batch, t0.Add(delay+ttl+time.Second)) {
		t.Fatal("park of another chunk: got false, want true")
	}
	if !p.Park(a, batch, t0.Add(delay+ttl+time.Second)) {
		t.Fatal("park after the ttl: got false, want true")
	}
}

// TestParkedPrunesAtMostOncePerDelay tests that park scans for stale entries
// at most once per delay, so a burst of parks does not walk the whole map
// each time while chunksWorker waits on the same lock.
func TestParkedPrunesAtMostOncePerDelay(t *testing.T) {
	t.Parallel()

	const delay = 5 * time.Minute
	t0 := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	batch := []byte("batch")
	a, b := swarm.RandAddress(t), swarm.RandAddress(t)

	// no ttl: an entry is stale as soon as its delay has passed
	p := pusher.NewParked(delay, 0)

	p.Park(a, batch, t0)            // prunes
	p.Park(b, batch, t0.Add(delay)) // prunes; a is not stale yet

	// a is stale now, but the last prune was less than a delay ago
	if p.Park(a, batch, t0.Add(delay+time.Second)) {
		t.Fatal("park within a delay of the last prune: got true, want false")
	}
	// a full delay after the last prune, stale entries are dropped
	if !p.Park(a, batch, t0.Add(2*delay+time.Second)) {
		t.Fatal("park a delay after the last prune: got false, want true")
	}
}
