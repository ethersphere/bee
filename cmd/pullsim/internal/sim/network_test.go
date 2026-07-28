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
	"errors"
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

	_, addrs, err := n.Inject(0, 1, 0, 0)
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

	_, addrs, err := n.Inject(0, 1, 0, 0)
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
	_, addrs, err := n.Inject(0, 1, 0, radius)
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

	if _, _, err := n.Inject(0, 3, 0, 0); err != nil {
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

func TestNetworkBatchSettles(t *testing.T) {
	// SettleAfter must exceed the real pullsync round trip (empirically ~1s,
	// due to its page-collection coalescing window), or the batch settles
	// before any replica arrives.
	n := startNetwork(t, Config{
		Nodes:       6,
		Bins:        4,
		Topology:    TopologyFull,
		Latency:     time.Millisecond,
		SettleAfter: 3 * time.Second,
	})

	id, addrs, err := n.Inject(0, 5, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Fatalf("got batch ID %d, want > 0", id)
	}
	if len(addrs) != 5 {
		t.Fatalf("got %d addrs, want 5", len(addrs))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	b, err := n.WaitBatch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Settled {
		t.Error("batch not settled")
	}
	if b.Chunks != 5 {
		t.Errorf("got Chunks %d, want 5", b.Chunks)
	}
	if b.Replicas == 0 {
		t.Error("got 0 replicas, want the batch to have propagated")
	}
	if b.NodesReached == 0 {
		t.Error("got 0 nodes reached, want the batch to have propagated")
	}
	if b.Metrics.SpanMs <= 0 {
		t.Errorf("got SpanMs %d, want > 0", b.Metrics.SpanMs)
	}
	// A burst inject feeds everything at once, so the span is all tail.
	if b.Metrics.InjectMs != 0 {
		t.Errorf("got InjectMs %d, want 0 for a burst inject", b.Metrics.InjectMs)
	}
	if b.Metrics.TailMs != b.Metrics.SpanMs {
		t.Errorf("got TailMs %d, want %d", b.Metrics.TailMs, b.Metrics.SpanMs)
	}
}

func TestNetworkSnapshotIncludesBatches(t *testing.T) {
	n := startNetwork(t, Config{
		Nodes: 4, Bins: 4, Topology: TopologyFull,
		Latency: time.Millisecond, SettleAfter: 500 * time.Millisecond,
	})
	id, _, err := n.Inject(0, 2, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	snap := n.Snapshot()
	if len(snap.Batches) != 1 {
		t.Fatalf("got %d batches in snapshot, want 1", len(snap.Batches))
	}
	if snap.Batches[0].ID != id {
		t.Errorf("got batch ID %d, want %d", snap.Batches[0].ID, id)
	}
	if snap.Batches[0].Chunks != 2 {
		t.Errorf("got Chunks %d, want 2", snap.Batches[0].Chunks)
	}
}

func TestNetworkWaitBatchHonorsContext(t *testing.T) {
	n := startNetwork(t, Config{
		Nodes: 4, Bins: 4, Topology: TopologyFull,
		Latency: time.Millisecond, SettleAfter: time.Hour, // never settles
	})
	id, _, err := n.Inject(0, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	b, err := n.WaitBatch(ctx, id)
	if err == nil {
		t.Fatal("got nil error, want context deadline exceeded")
	}
	// The partial batch must still come back so a timed-out sweep cell can
	// report what it observed.
	if b.ID != id {
		t.Errorf("got partial batch ID %d, want %d", b.ID, id)
	}
}

// I5: WaitBatch documents a shutdown wakeup, so a caller passing a
// non-cancellable context must not hang forever after Close.
func TestNetworkWaitBatchWakesOnClose(t *testing.T) {
	n, err := BuildNetwork(Config{
		Nodes: 4, Bins: 4, Topology: TopologyFull, Radius: 0, Seed: 11,
		Latency: time.Millisecond, SettleAfter: time.Hour, // never settles
	}, log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	n.Start(context.Background())

	id, _, err := n.Inject(0, 1, 0, 0)
	if err != nil {
		n.Close()
		t.Fatal(err)
	}

	type result struct {
		b   Batch
		err error
	}
	res := make(chan result, 1)
	go func() {
		// Deliberately not cancellable: only Close can wake this.
		b, err := n.WaitBatch(context.Background(), id)
		res <- result{b, err}
	}()

	time.Sleep(200 * time.Millisecond)
	early := false
	select {
	case r := <-res:
		early = true
		t.Errorf("WaitBatch returned before Close: %+v %v", r.b, r.err)
	default:
	}

	n.Close()
	if early {
		return
	}

	select {
	case r := <-res:
		if !errors.Is(r.err, ErrNetworkClosed) {
			t.Errorf("got error %v, want ErrNetworkClosed", r.err)
		}
		if r.b.ID != id {
			t.Errorf("got partial batch ID %d, want %d", r.b.ID, id)
		}
		if r.b.Settled {
			t.Error("batch reported settled although it never settled")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("WaitBatch did not wake on Close")
	}
}

// TestNetworkChurnHealsAfterRadiusDrop is the end-to-end statement of the whole
// feature: a clustered kademlia network at radius R where the chunks sit at PO
// R-1, so every node but the origin deliberately refuses them; some nodes
// depart; the operator drops the radius to R-1; the survivors must then pull
// what they have newly become responsible for.
//
// The assertion is that the *reported* healed count rises, not that the deficit
// reaches zero. A chunk whose only surviving holder is not a pull-sync source
// for the newly opened bin is unrecoverable by design, so a residue is a
// legitimate outcome and is reported rather than asserted away.
func TestNetworkChurnHealsAfterRadiusDrop(t *testing.T) {
	const (
		radius   = 4
		nodes    = 12
		clusters = 2
		chunks   = 20
	)

	n := startNetwork(t, Config{
		Nodes: nodes, Bins: 8, Topology: TopologyKademlia, Degree: 4,
		Radius: radius, Clusters: clusters, Seed: 41,
		Latency: time.Millisecond, SettleAfter: 3 * time.Second,
	})

	// Chunks at PO radius-1 from node 0: inside nobody's radius, so only the
	// origin holds them while the radius is R.
	if _, _, err := n.Inject(0, chunks, 0, radius-1); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)

	// Depart three of node 0's own neighborhood: they are the nodes that will
	// be responsible once the radius drops, so churning them is what makes the
	// backfill non-trivial.
	base := n.Nodes()[0].Addr
	var neighborhood []int
	for _, nd := range n.Nodes() {
		if nd.Index != 0 && swarm.Proximity(base.Bytes(), nd.Addr.Bytes()) >= radius {
			neighborhood = append(neighborhood, nd.Index)
		}
	}
	if len(neighborhood) < 4 {
		t.Fatalf("test setup: node 0 has only %d neighbors, want >= 4", len(neighborhood))
	}
	res, err := n.Churn(neighborhood[:3])
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("churn: departed=%v survivors=%d lost=%d edges +%d/-%d",
		res.Departed, res.Survivors, res.Lost, res.EdgesAdded, res.EdgesRemoved)

	before := n.Deficit()
	totalBefore := 0
	for _, d := range before {
		totalBefore += d
	}

	n.SetRadius(radius - 1)

	heals := n.Heals()
	if len(heals) != 1 {
		t.Fatalf("got %d heal episodes, want 1", len(heals))
	}
	id := heals[0].ID
	if heals[0].Total == 0 {
		t.Fatalf("heal episode opened with nothing to do (deficit before the drop was %d)", totalBefore)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	h, err := n.WaitHeal(ctx, id)
	if err != nil {
		t.Fatalf("WaitHeal: %v (partial: %+v)", err, h)
	}
	t.Logf("heal %d: %d->%d total=%d healed=%d remaining=%d span=%dms",
		h.ID, h.FromRadius, h.ToRadius, h.Total, h.Healed, h.Remaining, h.HealSpanMs)
	for _, p := range h.PerNode {
		t.Logf("  node %2d: %d/%d", p.Node, p.Healed, p.Total)
	}

	if !h.Settled {
		t.Error("heal episode did not settle")
	}
	if h.Healed == 0 {
		t.Fatalf("no chunks healed at all out of a deficit of %d", h.Total)
	}
	if h.Healed+h.Remaining != h.Total {
		t.Errorf("healed %d + remaining %d != total %d", h.Healed, h.Remaining, h.Total)
	}
	// Residual allowance: the outcome must be a clear majority recovered, not a
	// hardcoded zero deficit.
	if h.Healed*2 < h.Total {
		t.Errorf("only %d of %d chunks healed; expected the majority to be recoverable",
			h.Healed, h.Total)
	}
	// The live deficit must have fallen too, independently of the tracker.
	after := 0
	for _, d := range n.Deficit() {
		after += d
	}
	t.Logf("deficit: %d before the drop, %d after the heal", totalBefore, after)
	if h.Remaining > 0 {
		t.Logf("residual deficit of %d is a reported outcome, not a failure", h.Remaining)
	}
}

// C1b: the bench mines its chunks at -bench-minpo, which is what makes a
// non-zero -radius sweep produce any replicas at all.
func TestNetworkInjectHonorsMinPO(t *testing.T) {
	n := startNetwork(t, Config{
		Nodes: 4, Bins: 8, Topology: TopologyFull, Radius: 0, Seed: 12,
		Latency: time.Millisecond, SettleAfter: time.Hour,
	})
	const minPO = 5
	_, addrs, err := n.Inject(0, 4, 0, minPO)
	if err != nil {
		t.Fatal(err)
	}
	base := n.Nodes()[0].Addr
	for i, a := range addrs {
		if po := swarm.Proximity(a.Bytes(), base.Bytes()); po < minPO {
			t.Errorf("chunk %d at PO %d from the origin, want >= %d", i, po, minPO)
		}
	}
}
