// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestPeerMetricsCollector(t *testing.T) {
	db, err := shed.NewDB("", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	mc := newPeerMetricsCollector(db)
	addr := swarm.NewAddress([]byte{0x1})

	t.Run("last seen", func(t *testing.T) {
		seen := time.Now()

		err := mc.metrics(addr, setLastSeen(seen))
		if err != nil {
			t.Fatalf("metrics(%q, ...): unexpected error: %v", addr, err)
		}

		ss, err := mc.snapshot(addr)
		if err != nil {
			t.Fatalf("snapshot(%q, ...): unexpected error: %v", addr, err)
		}
		if have, want := len(ss), 1; have != want {
			t.Fatalf("snapshot(%q, ...): length mismatch: have: %d; want: %d", addr, have, want)
		}
		pm, ok := ss[addr.String()]
		if !ok {
			t.Fatalf("snapshot(%q, ...): missing peer metrics", addr)
		}
		if have, want := pm.LastSeen, seen; !have.Equal(want) {
			t.Fatalf("snapshot(%q, ...): last seen metric mismatch: have %s; want %s", addr, have, want)
		}
	})

	t.Run("connection total duration", func(t *testing.T) {
		dur := 16 * time.Minute

		err := mc.metrics(addr, setConnectionTotalDuration(dur))
		if err != nil {
			t.Fatalf("metrics(%q, ...): unexpected error: %v", addr, err)
		}

		ss, err := mc.snapshot(addr)
		if err != nil {
			t.Fatalf("snapshot(%q, ...): unexpected error: %v", addr, err)
		}
		if have, want := len(ss), 1; have != want {
			t.Fatalf("snapshot(%q, ...): length mismatch: have: %d; want: %d", addr, have, want)
		}
		pm, ok := ss[addr.String()]
		if !ok {
			t.Fatalf("snapshot(%q, ...): missing peer metrics", addr)
		}
		if have, want := pm.ConnectionTotalDuration, dur; have != want {
			t.Fatalf("snapshot(%q, ...): connection total duration metric mismatch: have %s; want %s", addr, have, want)
		}
	})

	t.Run("increment session connection retry", func(t *testing.T) {
		err := mc.metrics(addr, incSessionConnectionRetry(), incSessionConnectionRetry())
		if err != nil {
			t.Fatalf("metrics(%q, ...): unexpected error: %v", addr, err)
		}

		ss, err := mc.snapshot(addr)
		if err != nil {
			t.Fatalf("snapshot(%q, ...): unexpected error: %v", addr, err)
		}
		if have, want := len(ss), 1; have != want {
			t.Fatalf("snapshot(%q, ...): length mismatch: have: %d; want: %d", addr, have, want)
		}
		pm, ok := ss[addr.String()]
		if !ok {
			t.Fatalf("snapshot(%q, ...): missing peer metrics", addr)
		}
		if have, want := pm.SessionConnectionRetry, 2; have != want {
			t.Fatalf("snapshot(%q, ...): session connection retry metric mismatch: have %d; want %d", addr, have, want)
		}
	})

	t.Run("add session connection duration", func(t *testing.T) {
		dur := 5 * time.Minute

		err := mc.metrics(addr, addSessionConnectionDuration(dur), addSessionConnectionDuration(dur))
		if err != nil {
			t.Fatalf("metrics(%q, ...): unexpected error: %v", addr, err)
		}

		ss, err := mc.snapshot(addr)
		if err != nil {
			t.Fatalf("snapshot(%q, ...): unexpected error: %v", addr, err)
		}
		if have, want := len(ss), 1; have != want {
			t.Fatalf("snapshot(%q, ...): length mismatch: have: %d; want: %d", addr, have, want)
		}
		pm, ok := ss[addr.String()]
		if !ok {
			t.Fatalf("snapshot(%q, ...): missing peer metrics", addr)
		}
		if have, want := pm.SessionConnectionDuration, 2*dur; have != want {
			t.Fatalf("snapshot(%q, ...): session connection duration metric mismatch: have %d; want %d", addr, have, want)
		}
	})

	t.Run("set session connection direction", func(t *testing.T) {
		dir := peerConnectionDirectionInbound

		err := mc.metrics(addr, setSessionConnectionDirection(dir))
		if err != nil {
			t.Fatalf("metrics(%q, ...): unexpected error: %v", addr, err)
		}

		ss, err := mc.snapshot(addr)
		if err != nil {
			t.Fatalf("snapshot(%q, ...): unexpected error: %v", addr, err)
		}
		if have, want := len(ss), 1; have != want {
			t.Fatalf("snapshot(%q, ...): length mismatch: have: %d; want: %d", addr, have, want)
		}
		pm, ok := ss[addr.String()]
		if !ok {
			t.Fatalf("snapshot(%q, ...): missing peer metrics", addr)
		}
		if have, want := pm.SessionConnectionDirection, dir; have != want {
			t.Fatalf("snapshot(%q, ...): session connection duration metric mismatch: have %q; want %q", addr, have, want)
		}
	})
}
