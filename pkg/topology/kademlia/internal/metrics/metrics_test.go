// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metrics

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/swarm"
)

func snapshot(t *testing.T, mc *Collector, sst time.Time, addr swarm.Address) *Snapshot {
	t.Helper()

	ss, err := mc.Snapshot(sst, addr)
	if err != nil {
		t.Fatalf("Snapshot(%q, ...): unexpected error: %v", addr, err)
	}
	if have, want := len(ss), 1; have != want {
		t.Fatalf("Snapshot(%q, ...): length mismatch: have: %d; want: %d", addr, have, want)
	}
	pm, ok := ss[addr.String()]
	if !ok {
		t.Fatalf("Snapshot(%q, ...): missing peer metrics", addr)
	}
	return pm
}

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

	var (
		c    = NewCollector(db)
		addr = swarm.MustParseHexAddress("0123456789")

		t1 = time.Now()               // Login time.
		t2 = t1.Add(10 * time.Second) // Snapshot time.
		t3 = t2.Add(55 * time.Second) // Logout time.
	)

	// Login.
	err = c.Record(addr, PeerLogIn(t1, PeerConnectionDirectionInbound))
	if err != nil {
		t.Fatalf("Record(%q, ...): unexpected error: %v", addr, err)
	}
	ss := snapshot(t, c, t2, addr)
	if have, want := ss.LastSeen, t1; !have.Equal(want) {
		t.Fatalf("Snapshot(%q, ...): last seen metric mismatch: have %s; want %s", addr, have, want)
	}
	if have, want := ss.SessionConnectionDirection, PeerConnectionDirectionInbound; have != want {
		t.Fatalf("Snapshot(%q, ...): session connection duration metric mismatch: have %q; want %q", addr, have, want)
	}
	if have, want := ss.SessionConnectionDuration, t2.Sub(t1); have != want {
		t.Fatalf("Snapshot(%q, ...): last seen metric mismatch: have %s; want %s", addr, have, want)
	}

	// Login when already logged in.
	err = c.Record(addr, PeerLogIn(t1.Add(1*time.Second), PeerConnectionDirectionOutbound))
	if err != nil {
		t.Fatalf("Record(%q, ...): unexpected error: %v", addr, err)
	}
	ss = snapshot(t, c, t2, addr)
	if have, want := ss.LastSeen, t1; !have.Equal(want) {
		t.Fatalf("Snapshot(%q, ...): last seen metric mismatch: have %s; want %s", addr, have, want)
	}
	if have, want := ss.SessionConnectionDirection, PeerConnectionDirectionInbound; have != want {
		t.Fatalf("Snapshot(%q, ...): session connection duration metric mismatch: have %q; want %q", addr, have, want)
	}
	if have, want := ss.SessionConnectionDuration, t2.Sub(t1); have != want {
		t.Fatalf("Snapshot(%q, ...): last seen metric mismatch: have %s; want %s", addr, have, want)
	}

	// Inc session conn retry.
	err = c.Record(addr, IncSessionConnectionRetry(), IncSessionConnectionRetry())
	if err != nil {
		t.Fatalf("Record(%q, ...): unexpected error: %v", addr, err)
	}
	ss = snapshot(t, c, t2, addr)
	if have, want := ss.SessionConnectionRetry, 2; have != want {
		t.Fatalf("Snapshot(%q, ...): session connection retry metric mismatch: have %d; want %d", addr, have, want)
	}

	// Logout.
	err = c.Record(addr, PeerLogOut(t3))
	if err != nil {
		t.Fatalf("Record(%q, ...): unexpected error: %v", addr, err)
	}
	ss = snapshot(t, c, t2, addr)
	if have, want := ss.LastSeen, t3; !have.Equal(want) {
		t.Fatalf("Snapshot(%q, ...): last seen metric mismatch: have %s; want %s", addr, have, want)
	}
	if have, want := ss.ConnectionTotalDuration, t3.Sub(t1); have != want {
		t.Fatalf("Snapshot(%q, ...): connection total duration metric mismatch: have %s; want %s", addr, have, want)
	}
	if have, want := ss.SessionConnectionRetry, 2; have != want {
		t.Fatalf("Snapshot(%q, ...): session connection retry metric mismatch: have %d; want %d", addr, have, want)
	}
	if have, want := ss.SessionConnectionDuration, t3.Sub(t1); have != want {
		t.Fatalf("Snapshot(%q, ...): session connection duration metric mismatch: have %q; want %q", addr, have, want)
	}

	// Finalize.
	err = c.Record(addr, PeerLogIn(t1, PeerConnectionDirectionInbound))
	if err != nil {
		t.Fatalf("Record(%q, ...): unexpected error: %v", addr, err)
	}
	err = c.Finalize(t3)
	if err != nil {
		t.Fatalf("Finalize(%s): unexpected error: %v", t3, err)
	}
	if have, want := len(c.counters), 0; have != want {
		t.Fatalf("Finalize(%s): counters len mismatch: have %d; want %d", t3, have, want)
	}
}
