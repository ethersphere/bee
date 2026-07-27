// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func testAddrs(t *testing.T, n int, seed int64) []swarm.Address {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	addrs := make([]swarm.Address, n)
	for i := range addrs {
		addrs[i] = randAddr(rng)
	}
	return addrs
}

// graphDiameter returns the longest shortest-path in hops, or -1 if the graph
// is disconnected.
func graphDiameter(adj [][]int) int {
	worst := 0
	for src := range adj {
		dist := make([]int, len(adj))
		for i := range dist {
			dist[i] = -1
		}
		dist[src] = 0
		queue := []int{src}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, next := range adj[cur] {
				if dist[next] == -1 {
					dist[next] = dist[cur] + 1
					queue = append(queue, next)
				}
			}
		}
		for _, d := range dist {
			if d == -1 {
				return -1
			}
			if d > worst {
				worst = d
			}
		}
	}
	return worst
}

func TestParseTopologyKademlia(t *testing.T) {
	got, err := ParseTopology("kademlia")
	if err != nil {
		t.Fatal(err)
	}
	if got != TopologyKademlia {
		t.Errorf("got %q, want %q", got, TopologyKademlia)
	}
	if _, err := ParseTopology("kademlia-ish"); err == nil {
		t.Error("got nil error for an unknown topology, want an error")
	}
}

func TestKademliaConnectsEveryNeighborhoodPeer(t *testing.T) {
	const (
		bins   = 8
		radius = 4
		n      = 24
	)
	addrs := testAddrs(t, n, 7)
	adj := buildAdjacency(TopologyKademlia, addrs, 2, bins, radius, rand.New(rand.NewSource(1)))

	for i := 0; i < n; i++ {
		peers := make(map[int]struct{}, len(adj[i]))
		for _, j := range adj[i] {
			peers[j] = struct{}{}
		}
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			if edgePO(addrs[i], addrs[j], bins) < radius {
				continue
			}
			if _, ok := peers[j]; !ok {
				t.Fatalf("node %d is not connected to neighborhood peer %d (po %d >= radius %d)",
					i, j, edgePO(addrs[i], addrs[j], bins), radius)
			}
		}
	}
}

func TestKademliaSaturatesBinsBelowRadius(t *testing.T) {
	const (
		bins   = 8
		radius = 6
		degree = 2
		n      = 40
	)
	addrs := testAddrs(t, n, 11)
	adj := buildAdjacency(TopologyKademlia, addrs, degree, bins, radius, rand.New(rand.NewSource(2)))

	// Every node must hold at least one peer in each sub-radius bin that has
	// any candidate at all — that is what gives the graph its log diameter.
	for i := 0; i < n; i++ {
		candidates := make([]int, bins)
		held := make([]int, bins)
		for j := 0; j < n; j++ {
			if i != j {
				candidates[edgePO(addrs[i], addrs[j], bins)]++
			}
		}
		for _, j := range adj[i] {
			held[edgePO(addrs[i], addrs[j], bins)]++
		}
		for po := 0; po < radius; po++ {
			if candidates[po] > 0 && held[po] == 0 {
				t.Errorf("node %d has %d candidates in bin %d but no peer there", i, candidates[po], po)
			}
		}
	}
}

func TestKademliaAtRadiusZeroIsFullMesh(t *testing.T) {
	const n = 12
	addrs := testAddrs(t, n, 3)
	adj := buildAdjacency(TopologyKademlia, addrs, 1, 8, 0, rand.New(rand.NewSource(4)))
	for i := range adj {
		if len(adj[i]) != n-1 {
			t.Fatalf("node %d has %d peers, want %d: radius 0 makes every peer a neighbor", i, len(adj[i]), n-1)
		}
	}
}

func TestKademliaIsReproducibleFromSeed(t *testing.T) {
	addrs := testAddrs(t, 30, 5)
	a := buildAdjacency(TopologyKademlia, addrs, 2, 8, 5, rand.New(rand.NewSource(99)))
	b := buildAdjacency(TopologyKademlia, addrs, 2, 8, 5, rand.New(rand.NewSource(99)))
	for i := range a {
		if len(a[i]) != len(b[i]) {
			t.Fatalf("node %d: peer counts differ (%d vs %d)", i, len(a[i]), len(b[i]))
		}
		for k := range a[i] {
			if a[i][k] != b[i][k] {
				t.Fatalf("node %d: peer %d differs (%d vs %d)", i, k, a[i][k], b[i][k])
			}
		}
	}
}

// The point of the topology: a few peers in every bin beats only-close peers,
// so kademlia's diameter must be well below the ring's at the same node count.
func TestKademliaDiameterBeatsRing(t *testing.T) {
	const (
		n      = 40
		degree = 4
		bins   = 8
		radius = 6
	)
	addrs := testAddrs(t, n, 13)

	kad := buildAdjacency(TopologyKademlia, addrs, degree, bins, radius, rand.New(rand.NewSource(21)))
	ring := buildAdjacency(TopologyRing, addrs, degree, bins, radius, rand.New(rand.NewSource(21)))

	kadD, ringD := graphDiameter(kad), graphDiameter(ring)
	if kadD < 0 {
		t.Fatal("kademlia graph is disconnected")
	}
	if ringD < 0 {
		t.Fatal("ring graph is disconnected")
	}
	if kadD >= ringD {
		t.Errorf("kademlia diameter %d is not below ring diameter %d", kadD, ringD)
	}
}

func TestBuildNetworkAcceptsKademlia(t *testing.T) {
	n, err := BuildNetwork(Config{
		Nodes:    12,
		Bins:     8,
		Radius:   4,
		Topology: TopologyKademlia,
		Degree:   2,
	}, log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()

	for i, peers := range n.adj {
		if len(peers) == 0 {
			t.Fatalf("node %d has no peers", i)
		}
	}
}
