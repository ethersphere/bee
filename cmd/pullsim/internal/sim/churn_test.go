// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim_test

import (
	"reflect"
	"testing"

	"github.com/ethersphere/bee/v2/cmd/pullsim/internal/sim"
	"github.com/ethersphere/bee/v2/pkg/log"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// buildNet builds a network without starting the pullers, so the tests below
// observe wiring rather than protocol traffic.
func buildNet(t *testing.T, cfg sim.Config) *sim.Network {
	t.Helper()
	n, err := sim.BuildNetwork(cfg, log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Close)
	return n
}

// freshAdjacency recomputes the adjacency the network should hold after churn:
// buildAdjacency over the surviving addresses, mapped back to global indices.
func freshAdjacency(n *sim.Network) [][]int {
	alive := n.Survivors()
	addrs := make([]swarm.Address, len(alive))
	for p, i := range alive {
		addrs[p] = n.Nodes()[i].Addr
	}
	cfg := n.Config()
	sub := sim.BuildAdjacency(cfg.Topology, addrs, cfg.Degree, cfg.Bins, cfg.Radius, sim.Rand(cfg.Seed))

	out := make([][]int, len(n.Nodes()))
	for p, peers := range sub {
		global := make([]int, 0, len(peers))
		for _, q := range peers {
			global = append(global, alive[q])
		}
		out[alive[p]] = global
	}
	return out
}

func TestChurnRewiresSurvivors(t *testing.T) {
	t.Parallel()

	n := buildNet(t, sim.Config{
		Nodes: 10, Bins: 8, Topology: sim.TopologyKademlia, Degree: 3, Radius: 4, Seed: 21,
	})

	res, err := n.Churn([]int{2, 5})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Departed, []int{2, 5}) {
		t.Errorf("got departed %v, want [2 5]", res.Departed)
	}
	if res.Survivors != 8 {
		t.Errorf("got %d survivors, want 8", res.Survivors)
	}
	if res.EdgesRemoved == 0 {
		t.Error("got 0 edges removed, want the departed nodes' edges to be dropped")
	}

	for _, i := range []int{2, 5} {
		if !n.Departed(i) {
			t.Errorf("node %d not marked departed", i)
		}
	}
	if got, want := n.Survivors(), []int{0, 1, 3, 4, 6, 7, 8, 9}; !reflect.DeepEqual(got, want) {
		t.Errorf("got survivors %v, want %v", got, want)
	}

	// The adjacency must be exactly a fresh build over the survivors.
	if got, want := n.Adjacency(), freshAdjacency(n); !reflect.DeepEqual(got, want) {
		t.Errorf("adjacency after churn:\n got %v\nwant %v", got, want)
	}
}

func TestChurnDepartedUnreachableBothDirections(t *testing.T) {
	t.Parallel()

	n := buildNet(t, sim.Config{
		Nodes: 8, Bins: 8, Topology: sim.TopologyFull, Radius: 2, Seed: 22,
	})
	nodes := n.Nodes()

	if _, err := n.Churn([]int{3}); err != nil {
		t.Fatal(err)
	}

	gone := nodes[3]
	for _, nd := range nodes {
		if nd.Index == 3 {
			continue
		}
		if nd.Transport.HasHandler(gone.Addr) {
			t.Errorf("survivor %d can still dial departed node 3", nd.Index)
		}
		if gone.Transport.HasHandler(nd.Addr) {
			t.Errorf("departed node 3 can still dial survivor %d", nd.Index)
		}
		for _, p := range nd.Kad.Peers() {
			if p.Addr.Equal(gone.Addr) {
				t.Errorf("survivor %d still lists departed node 3 as a kad peer", nd.Index)
			}
		}
	}
}

func TestChurnSurvivorKadPeersMatchAdjacency(t *testing.T) {
	t.Parallel()

	n := buildNet(t, sim.Config{
		Nodes: 10, Bins: 8, Topology: sim.TopologyRandom, Degree: 3, Radius: 3, Seed: 23,
	})

	if _, err := n.Churn([]int{1, 7}); err != nil {
		t.Fatal(err)
	}

	adj := n.Adjacency()
	for _, i := range n.Survivors() {
		want := make(map[string]struct{}, len(adj[i]))
		for _, j := range adj[i] {
			want[n.Nodes()[j].Addr.ByteString()] = struct{}{}
		}
		got := make(map[string]struct{})
		for _, p := range n.Nodes()[i].Kad.Peers() {
			got[p.Addr.ByteString()] = struct{}{}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("node %d kad peer set does not match its adjacency", i)
		}
	}
}

func TestChurnGuards(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		nodes []int
	}{
		{"empty", nil},
		{"repeated index", []int{2, 2}},
		{"negative index", []int{-1}},
		{"index out of range", []int{5}},
		{"leaves fewer than two survivors", []int{0, 1, 2, 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			n := buildNet(t, sim.Config{
				Nodes: 5, Bins: 4, Topology: sim.TopologyFull, Seed: 24,
			})
			if _, err := n.Churn(tc.nodes); err == nil {
				t.Fatalf("Churn(%v) succeeded, want an error", tc.nodes)
			}
			// A rejected churn must not have departed anything.
			if got := len(n.Survivors()); got != 5 {
				t.Errorf("got %d survivors after a rejected churn, want 5", got)
			}
		})
	}
}

func TestChurnRejectsAlreadyDeparted(t *testing.T) {
	t.Parallel()

	n := buildNet(t, sim.Config{
		Nodes: 5, Bins: 4, Topology: sim.TopologyFull, Seed: 25,
	})
	if _, err := n.Churn([]int{4}); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Churn([]int{4}); err == nil {
		t.Fatal("re-churning node 4 succeeded, want an error")
	}
	if _, err := n.Churn([]int{0, 1, 2}); err == nil {
		t.Fatal("churn leaving 1 survivor succeeded, want an error")
	}
}

func TestChurnRandomIsSeeded(t *testing.T) {
	t.Parallel()

	pick := func() []int {
		n := buildNet(t, sim.Config{
			Nodes: 10, Bins: 4, Topology: sim.TopologyFull, Seed: 26,
		})
		res, err := n.ChurnRandom(3)
		if err != nil {
			t.Fatal(err)
		}
		return res.Departed
	}
	a, b := pick(), pick()
	if len(a) != 3 {
		t.Fatalf("got %d departed, want 3", len(a))
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("ChurnRandom is not reproducible from the seed: %v vs %v", a, b)
	}

	n := buildNet(t, sim.Config{Nodes: 4, Bins: 4, Topology: sim.TopologyFull, Seed: 27})
	if _, err := n.ChurnRandom(3); err == nil {
		t.Error("ChurnRandom leaving 1 survivor succeeded, want an error")
	}
	if _, err := n.ChurnRandom(0); err == nil {
		t.Error("ChurnRandom(0) succeeded, want an error")
	}
}

// TestChurnReportsLost: a chunk held only by a departing node is gone, and is
// counted; a chunk that a survivor also holds is not.
func TestChurnReportsLost(t *testing.T) {
	t.Parallel()

	n := buildNet(t, sim.Config{
		Nodes: 4, Bins: 8, Topology: sim.TopologyFull, Radius: 0, Seed: 28,
	})
	rng := sim.Rand(101)
	nodes := n.Nodes()

	doomed := sim.ChunkAt(rng, nodes[3].Addr, 0)
	if err := nodes[3].Reserve.Inject(doomed); err != nil {
		t.Fatal(err)
	}
	replicated := sim.ChunkAt(rng, nodes[3].Addr, 0)
	for _, i := range []int{0, 3} {
		if err := nodes[i].Reserve.Inject(replicated); err != nil {
			t.Fatal(err)
		}
	}

	res, err := n.Churn([]int{3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Lost != 1 {
		t.Errorf("got Lost %d, want 1", res.Lost)
	}
}

// TestDeficitMaths checks the deficit definition against a hand-built reserve
// set: responsibility is decided by the storage radius, holding the chunk
// clears it, and the key is the (address, batchID, stampHash) triple.
func TestDeficitMaths(t *testing.T) {
	t.Parallel()

	const radius = 4
	n := buildNet(t, sim.Config{
		Nodes: 3, Bins: 8, Topology: sim.TopologyFull, Radius: radius, Seed: 29,
	})
	nodes := n.Nodes()
	rng := sim.Rand(102)

	// A chunk deep inside node 0's responsibility and outside everyone else's.
	near := sim.ChunkAt(rng, nodes[0].Addr, 6)
	for _, i := range []int{1, 2} {
		if po := swarm.Proximity(nodes[i].Addr.Bytes(), near.Address().Bytes()); po >= radius {
			t.Fatalf("test setup: node %d is unexpectedly responsible for the chunk (po %d)", i, po)
		}
	}

	// Two distinct stamps over the same address. swarm.Chunk.WithStamp mutates
	// in place, so each needs its own chunk value.
	stamped1 := swarm.NewChunk(near.Address(), near.Data()).WithStamp(postagetesting.MustNewStamp())
	stamped2 := swarm.NewChunk(near.Address(), near.Data()).WithStamp(postagetesting.MustNewStamp())

	// Held by a node that is not responsible: only node 0 is in deficit.
	if err := nodes[1].Reserve.Inject(stamped1); err != nil {
		t.Fatal(err)
	}
	if got, want := n.Deficit(), []int{1, 0, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got deficit %v, want %v", got, want)
	}

	// A second stamp over the same address is a second entry in the universe:
	// the key is the triple, not the address.
	if err := nodes[1].Reserve.Inject(stamped2); err != nil {
		t.Fatal(err)
	}
	if got, want := n.Deficit(), []int{2, 0, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got deficit %v, want %v (the triple key must count both stamps)", got, want)
	}

	// Already held clears exactly one of the two.
	if err := nodes[0].Reserve.Inject(stamped1); err != nil {
		t.Fatal(err)
	}
	if got, want := n.Deficit(), []int{1, 0, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got deficit %v, want %v", got, want)
	}
	if err := nodes[0].Reserve.Inject(stamped2); err != nil {
		t.Fatal(err)
	}
	if got, want := n.Deficit(), []int{0, 0, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got deficit %v, want %v", got, want)
	}

	// A chunk outside everyone's radius is in the universe but in nobody's
	// deficit.
	var far swarm.Chunk
	for {
		c := sim.ChunkAt(rng, nodes[0].Addr, 0)
		outside := true
		for _, nd := range nodes {
			if swarm.Proximity(nd.Addr.Bytes(), c.Address().Bytes()) >= radius {
				outside = false
				break
			}
		}
		if outside {
			far = c
			break
		}
	}
	if err := nodes[1].Reserve.Inject(far); err != nil {
		t.Fatal(err)
	}
	if got, want := n.Deficit(), []int{0, 0, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got deficit %v, want %v (an out-of-radius chunk is nobody's deficit)", got, want)
	}

	// Departed nodes report zero.
	if err := nodes[2].Reserve.Inject(sim.ChunkAt(rng, nodes[2].Addr, 6)); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Churn([]int{2}); err != nil {
		t.Fatal(err)
	}
	d := n.Deficit()
	if len(d) != 3 {
		t.Fatalf("got deficit length %d, want 3 (indices must stay stable)", len(d))
	}
	if d[2] != 0 {
		t.Errorf("got deficit %d for a departed node, want 0", d[2])
	}
	if _, ok := n.DeficitSets()[2]; ok {
		t.Error("departed node still present in the deficit sets")
	}
}
