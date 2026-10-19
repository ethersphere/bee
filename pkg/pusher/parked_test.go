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
