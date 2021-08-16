// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metrics_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/kademlia/internal/metrics"
	"github.com/google/go-cmp/cmp"
)

func snapshot(t *testing.T, mc *metrics.Collector, sst time.Time, addr swarm.Address) *metrics.Snapshot {
	t.Helper()

	ss := mc.Snapshot(sst, addr)
	if have, want := len(ss), 1; have != want {
		t.Fatalf("Snapshot(%q, ...): length mismatch: have: %d; want: %d", addr, have, want)
	}
	cs, ok := ss[addr.ByteString()]
	if !ok {
		t.Fatalf("Snapshot(%q, ...): missing peer metrics", addr)
	}
	return cs
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
		mc   = metrics.NewCollector(db)
		addr = swarm.MustParseHexAddress("0123456789")

		t1 = time.Now()               // Login time.
		t2 = t1.Add(10 * time.Second) // Snapshot time.
		t3 = t2.Add(55 * time.Second) // Logout time.
	)

	// Inc session conn retry.
	mc.Record(addr, metrics.IncSessionConnectionRetry())
	ss := snapshot(t, mc, t2, addr)
	if have, want := ss.SessionConnectionRetry, uint64(1); have != want {
		t.Fatalf("Snapshot(%q, ...): session connection retry counter mismatch: have %d; want %d", addr, have, want)
	}

	// Login.
	mc.Record(addr, metrics.PeerLogIn(t1, metrics.PeerConnectionDirectionInbound))
	ss = snapshot(t, mc, t2, addr)
	if have, want := ss.LastSeenTimestamp, t1.UnixNano(); have != want {
		t.Fatalf("Snapshot(%q, ...): last seen counter mismatch: have %d; want %d", addr, have, want)
	}
	if have, want := ss.SessionConnectionDirection, metrics.PeerConnectionDirectionInbound; have != want {
		t.Fatalf("Snapshot(%q, ...): session connection direction counter mismatch: have %q; want %q", addr, have, want)
	}
	if have, want := ss.SessionConnectionDuration, t2.Sub(t1); have != want {
		t.Fatalf("Snapshot(%q, ...): session connection duration counter mismatch: have %s; want %s", addr, have, want)
	}
	if have, want := ss.ConnectionTotalDuration, t2.Sub(t1); have != want {
		t.Fatalf("Snapshot(%q, ...): connection total duration counter mismatch: have %s; want %s", addr, have, want)
	}

	// Login when already logged in.
	mc.Record(addr, metrics.PeerLogIn(t1.Add(1*time.Second), metrics.PeerConnectionDirectionOutbound))
	ss = snapshot(t, mc, t2, addr)
	if have, want := ss.LastSeenTimestamp, t1.UnixNano(); have != want {
		t.Fatalf("Snapshot(%q, ...): last seen counter mismatch: have %d; want %d", addr, have, want)
	}
	if have, want := ss.SessionConnectionDirection, metrics.PeerConnectionDirectionInbound; have != want {
		t.Fatalf("Snapshot(%q, ...): session connection direction counter mismatch: have %q; want %q", addr, have, want)
	}
	if have, want := ss.SessionConnectionDuration, t2.Sub(t1); have != want {
		t.Fatalf("Snapshot(%q, ...): session connection duration counter mismatch: have %s; want %s", addr, have, want)
	}
	if have, want := ss.ConnectionTotalDuration, t2.Sub(t1); have != want {
		t.Fatalf("Snapshot(%q, ...): connection total duration counter mismatch: have %s; want %s", addr, have, want)
	}

	// Inc session conn retry.
	mc.Record(addr, metrics.IncSessionConnectionRetry())
	ss = snapshot(t, mc, t2, addr)
	if have, want := ss.SessionConnectionRetry, uint64(2); have != want {
		t.Fatalf("Snapshot(%q, ...): session connection retry counter mismatch: have %d; want %d", addr, have, want)
	}

	// Logout.
	mc.Record(addr, metrics.PeerLogOut(t3))
	ss = snapshot(t, mc, t2, addr)
	if have, want := ss.LastSeenTimestamp, t3.UnixNano(); have != want {
		t.Fatalf("Snapshot(%q, ...): last seen counter mismatch: have %d; want %d", addr, have, want)
	}
	if have, want := ss.ConnectionTotalDuration, t3.Sub(t1); have != want {
		t.Fatalf("Snapshot(%q, ...): connection total duration counter mismatch: have %s; want %s", addr, have, want)
	}
	if have, want := ss.SessionConnectionRetry, uint64(2); have != want {
		t.Fatalf("Snapshot(%q, ...): session connection retry counter mismatch: have %d; want %d", addr, have, want)
	}
	if have, want := ss.SessionConnectionDuration, t3.Sub(t1); have != want {
		t.Fatalf("Snapshot(%q, ...): session connection duration counter mismatch: have %q; want %q", addr, have, want)
	}

	// Inspect.
	have := mc.Inspect(addr)
	want := ss
	if diff := cmp.Diff(have, want); diff != "" {
		t.Fatalf("unexpected snapshot diffrence:\n%s", diff)
	}

	// Flush.
	if err := mc.Flush(addr); err != nil {
		t.Fatalf("Flush(): unexpected error: %v", err)
	}

	// Finalize.
	mc.Record(addr, metrics.PeerLogIn(t1, metrics.PeerConnectionDirectionInbound))
	if err := mc.Finalize(t3); err != nil {
		t.Fatalf("Finalize(%s): unexpected error: %v", t3, err)
	}
	if have, want := len(mc.Snapshot(t2, addr)), 0; have != want {
		t.Fatalf("Finalize(%s): counters length mismatch: have %d; want %d", t3, have, want)
	}
}
