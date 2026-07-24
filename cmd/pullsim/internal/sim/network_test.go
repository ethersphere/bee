// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !race

// These tests drive the real pullsync Syncer with many concurrent peers. When
// two peers coalesce the same (bin, start) request on one server, the shared
// resenje.org/singleflight dependency has a benign data race: call.shared is
// written under its mutex (singleflight.go:43) but read lock-free after unlock
// (singleflight.go:86). pullsync discards that "shared" bool, so behavior is
// unaffected, but the race detector reports it. The race lives in the
// dependency, not in cmd/pullsim, and cannot be fixed without changing a
// production module, so these integration tests are excluded from -race runs.
// They still run under `go test ./cmd/pullsim/...` (no -race).

package sim

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func startNetwork(t *testing.T, cfg Config) *Network {
	t.Helper()
	n, err := BuildNetwork(cfg, log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	n.Start(context.Background())
	t.Cleanup(n.Close)
	return n
}

func countHolders(n *Network, addr swarm.Address) int {
	c := 0
	for _, nd := range n.Nodes() {
		if nd.Reserve.HasAddress(addr) {
			c++
		}
	}
	return c
}

func waitPropagation(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("propagation not observed within timeout")
}

// TestNetwork_FullMeshPropagation: radius 0 full mesh, one injected chunk must
// reach every node (single hop, ~1s coalescing).
func TestNetwork_FullMeshPropagation(t *testing.T) {
	t.Parallel()

	n := startNetwork(t, Config{
		Nodes: 5, Bins: 4, Topology: TopologyFull, Radius: 0, Seed: 1,
	})

	addrs, err := n.Inject(0, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	target := addrs[0]

	waitPropagation(t, 15*time.Second, func() bool {
		return countHolders(n, target) == len(n.Nodes())
	})
}

// TestNetwork_RingMultiHop: ring degree 2, propagation must reach every node
// over multiple hops.
func TestNetwork_RingMultiHop(t *testing.T) {
	t.Parallel()

	n := startNetwork(t, Config{
		Nodes: 6, Bins: 4, Topology: TopologyRing, Degree: 2, Radius: 0, Seed: 2,
	})

	addrs, err := n.Inject(0, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	target := addrs[0]

	waitPropagation(t, 30*time.Second, func() bool {
		return countHolders(n, target) == len(n.Nodes())
	})
}

// TestNetwork_RadiusPartition: an out-of-radius chunk must never be stored by a
// node outside its storage radius; the origin (within radius) keeps it.
func TestNetwork_RadiusPartition(t *testing.T) {
	t.Parallel()

	const radius = 8
	n := startNetwork(t, Config{
		Nodes: 5, Bins: 10, Topology: TopologyFull, Radius: radius, Seed: 3,
	})

	// Mine a chunk very close to node 0 (>= radius bits shared with node 0).
	addrs, err := n.Inject(0, 1, 0, radius)
	if err != nil {
		t.Fatal(err)
	}
	target := addrs[0]

	if !n.Nodes()[0].Reserve.HasAddress(target) {
		t.Fatal("origin should hold the injected chunk")
	}

	// Give the pullers time to attempt (and correctly refuse) propagation.
	time.Sleep(4 * time.Second)

	for _, nd := range n.Nodes() {
		within := swarm.Proximity(nd.Addr.Bytes(), target.Bytes()) >= radius
		has := nd.Reserve.HasAddress(target)
		if has && !within {
			t.Fatalf("node %d stored an out-of-radius chunk (proximity %d < radius %d)",
				nd.Index, swarm.Proximity(nd.Addr.Bytes(), target.Bytes()), radius)
		}
	}
}

// TestNetwork_ShutdownAndGoroutines: Close must complete quickly and leave no
// substantial goroutine leak.
func TestNetwork_ShutdownAndGoroutines(t *testing.T) {
	t.Parallel()

	before := runtime.NumGoroutine()

	n, err := BuildNetwork(Config{
		Nodes: 5, Bins: 4, Topology: TopologyFull, Radius: 0, Seed: 4,
	}, log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	n.Start(context.Background())

	if _, err := n.Inject(0, 3, 0, 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)

	done := make(chan struct{})
	go func() {
		n.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("Close did not complete within 8s")
	}

	// Allow lingering goroutines to unwind.
	var after int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		after = runtime.NumGoroutine()
		if after <= before+20 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("goroutine leak: before=%d after=%d", before, after)
}
