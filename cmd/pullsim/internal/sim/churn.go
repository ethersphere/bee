// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/ethersphere/bee/v2/cmd/pullsim/internal/event"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kadmock "github.com/ethersphere/bee/v2/pkg/topology/kademlia/mock"
)

// minSurvivors is the floor Churn refuses to go below: a one-node network can
// neither sync nor heal, so it is never a useful thing to simulate.
const minSurvivors = 2

// ChurnResult reports the outcome of a churn event.
type ChurnResult struct {
	Departed     []int `json:"departed"`
	Survivors    int   `json:"survivors"`
	Lost         int   `json:"lost"`
	EdgesAdded   int   `json:"edgesAdded"`
	EdgesRemoved int   `json:"edgesRemoved"`
}

// Departed reports whether node i has left the network.
func (n *Network) Departed(i int) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.departedLocked(i)
}

func (n *Network) departedLocked(i int) bool {
	return i >= 0 && i < len(n.departed) && n.departed[i]
}

// Survivors returns the indices of the nodes still in the network, ascending.
func (n *Network) Survivors() []int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.survivorsLocked()
}

func (n *Network) survivorsLocked() []int {
	out := make([]int, 0, len(n.nodes))
	for i := range n.nodes {
		if !n.departed[i] {
			out = append(out, i)
		}
	}
	return out
}

// ChurnRandom departs count nodes chosen from the network's seeded RNG, so a
// seeded run reproduces exactly.
func (n *Network) ChurnRandom(count int) (ChurnResult, error) {
	if count < 1 {
		return ChurnResult{}, fmt.Errorf("count must be >= 1, got %d", count)
	}
	n.mu.Lock()
	alive := n.survivorsLocked()
	if len(alive)-count < minSurvivors {
		n.mu.Unlock()
		return ChurnResult{}, fmt.Errorf("departing %d of %d nodes would leave fewer than %d survivors",
			count, len(alive), minSurvivors)
	}
	perm := n.churnRng.Perm(len(alive))
	n.mu.Unlock()

	pick := make([]int, 0, count)
	for _, p := range perm[:count] {
		pick = append(pick, alive[p])
	}
	sort.Ints(pick)
	return n.Churn(pick)
}

// Churn departs the given nodes: their puller, transport, syncer and reserve are
// closed, the surviving nodes are rewired against a fresh adjacency over the
// surviving address set, and the chunks that no survivor holds are counted as
// lost. Indices are stable: a departed node keeps its slot forever.
func (n *Network) Churn(nodes []int) (ChurnResult, error) {
	// The pre-departure universe has to be sampled before anything is torn
	// down, since Lost is defined against it.
	before := n.universe()

	plan, err := n.planChurn(nodes)
	if err != nil {
		return ChurnResult{}, err
	}

	// Everything below runs without n.mu: closing a component or a link
	// publishes events, which reacquires it.
	n.tearDown(plan.departed)
	n.applyRewire(plan)

	after := n.universe()
	lost := 0
	for key := range before {
		if _, ok := after[key]; !ok {
			lost++
		}
	}

	res := ChurnResult{
		Departed:     plan.departed,
		Survivors:    len(plan.alive),
		Lost:         lost,
		EdgesAdded:   len(plan.added),
		EdgesRemoved: len(plan.removed),
	}
	n.publish(event.Churn{
		Departed: res.Departed, Survivors: res.Survivors, Lost: res.Lost,
		EdgesAdded: res.EdgesAdded, EdgesRemoved: res.EdgesRemoved,
	})
	n.logger.Info("churn", "departed", res.Departed, "survivors", res.Survivors,
		"lost", res.Lost, "edges_added", res.EdgesAdded, "edges_removed", res.EdgesRemoved)
	return res, nil
}

// pair is an unordered edge, always with a < b.
type pair struct{ a, b int }

// churnPlan is the fully computed effect of a churn event. It is produced under
// n.mu and applied outside it.
type churnPlan struct {
	departed []int
	alive    []int
	// added and removed are the undirected delta between the old and the new
	// adjacency; they are what EdgesAdded/EdgesRemoved report.
	added   []pair
	removed []pair
}

// planChurn validates the request, marks the leavers, installs the new
// adjacency and returns the delta to apply. It holds n.mu throughout so no
// snapshot can observe a half-churned network.
func (n *Network) planChurn(nodes []int) (churnPlan, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if len(nodes) == 0 {
		return churnPlan{}, fmt.Errorf("no nodes to depart")
	}
	seen := make(map[int]struct{}, len(nodes))
	for _, i := range nodes {
		if i < 0 || i >= len(n.nodes) {
			return churnPlan{}, fmt.Errorf("node index %d out of range", i)
		}
		if _, dup := seen[i]; dup {
			return churnPlan{}, fmt.Errorf("node index %d repeated", i)
		}
		if n.departed[i] {
			return churnPlan{}, fmt.Errorf("node %d already departed", i)
		}
		seen[i] = struct{}{}
	}
	if len(n.survivorsLocked())-len(nodes) < minSurvivors {
		return churnPlan{}, fmt.Errorf("departing %d nodes would leave fewer than %d survivors",
			len(nodes), minSurvivors)
	}

	departed := append([]int(nil), nodes...)
	sort.Ints(departed)
	for _, i := range departed {
		n.departed[i] = true
	}

	alive := n.survivorsLocked()
	addrs := make([]swarm.Address, len(alive))
	for p, i := range alive {
		addrs[p] = n.nodes[i].Addr
	}

	// Rebuild over the survivors only. The rng is re-seeded from the config
	// seed so the rewire of a seeded run is reproducible.
	sub := buildAdjacency(n.cfg.Topology, addrs, n.cfg.Degree, n.cfg.Bins, n.cfg.Radius,
		rand.New(rand.NewSource(n.cfg.Seed)))

	next := make([][]int, len(n.nodes))
	for p, peers := range sub {
		global := make([]int, 0, len(peers))
		for _, q := range peers {
			global = append(global, alive[q])
		}
		sort.Ints(global)
		next[alive[p]] = global
	}

	plan := churnPlan{departed: departed, alive: alive}
	old := pairSet(n.adj)
	fresh := pairSet(next)
	for p := range old {
		if _, ok := fresh[p]; !ok {
			plan.removed = append(plan.removed, p)
		}
	}
	for p := range fresh {
		if _, ok := old[p]; !ok {
			plan.added = append(plan.added, p)
		}
	}
	sortPairs(plan.removed)
	sortPairs(plan.added)

	n.adj = next
	return plan, nil
}

// pairSet renders an adjacency list as its set of undirected edges.
func pairSet(adj [][]int) map[pair]struct{} {
	out := make(map[pair]struct{})
	for i, peers := range adj {
		for _, j := range peers {
			if i < j {
				out[pair{i, j}] = struct{}{}
			} else if j < i {
				out[pair{j, i}] = struct{}{}
			}
		}
	}
	return out
}

func sortPairs(ps []pair) {
	sort.Slice(ps, func(x, y int) bool {
		if ps[x].a != ps[y].a {
			return ps[x].a < ps[y].a
		}
		return ps[x].b < ps[y].b
	})
}

// tearDown closes the departed nodes, in the order Network.Close establishes as
// deadlock-free: pullers first (cancel client streams), then transports (cancel
// parked handler contexts), then syncers, then reserves. Closing the reserve
// last is what releases the survivors' handlers still parked on it.
func (n *Network) tearDown(departed []int) {
	nodes := make([]*Node, 0, len(departed))
	for _, i := range departed {
		nodes = append(nodes, n.nodes[i])
	}
	closeConcurrent(nodes, func(nd *Node) { _ = nd.Puller.Close() })
	closeConcurrent(nodes, func(nd *Node) { _ = nd.Transport.Close() })
	closeConcurrent(nodes, func(nd *Node) { _ = nd.Syncer.Close() })
	closeConcurrent(nodes, func(nd *Node) { _ = nd.Reserve.Close() })
}

// applyRewire applies the adjacency delta to the transports and hands every
// survivor its new kademlia peer set. Removals run first so no survivor can
// dial a peer it is about to drop.
func (n *Network) applyRewire(plan churnPlan) {
	departed := make(map[int]struct{}, len(plan.departed))
	for _, i := range plan.departed {
		departed[i] = struct{}{}
	}

	drop := func(from, to int) {
		if _, gone := departed[from]; gone {
			return // its whole transport is closed already
		}
		n.nodes[from].Transport.RemoveHandler(n.nodes[to].Addr)
	}
	for _, p := range plan.removed {
		drop(p.a, p.b)
		drop(p.b, p.a)
	}
	for _, p := range plan.added {
		n.nodes[p.a].Transport.SetHandler(n.nodes[p.b].Addr, n.nodes[p.b].Syncer.Protocol())
		n.nodes[p.b].Transport.SetHandler(n.nodes[p.a].Addr, n.nodes[p.a].Syncer.Protocol())
	}

	n.mu.Lock()
	peerSets := make(map[int][]kadmock.AddrTuple, len(plan.alive))
	for _, i := range plan.alive {
		tuples := make([]kadmock.AddrTuple, 0, len(n.adj[i]))
		for _, j := range n.adj[i] {
			tuples = append(tuples, kadmock.AddrTuple{Addr: n.nodes[j].Addr, PO: n.poMatrix[i][j]})
		}
		peerSets[i] = tuples
	}
	n.mu.Unlock()

	for i, tuples := range peerSets {
		n.nodes[i].Kad.SetPeers(tuples)
	}
}

// universe returns the live chunk universe: the union of every surviving
// reserve's entries, keyed on the (address, batchID, stampHash) triple that
// pullsync itself wants on.
func (n *Network) universe() map[string]Entry {
	out := make(map[string]Entry)
	for _, i := range n.Survivors() {
		for _, e := range n.nodes[i].Reserve.Entries() {
			out[presenceKey(e.Address, e.BatchID, e.StampHash)] = e
		}
	}
	return out
}

// deficitSets returns, per surviving node, the presence keys of the live
// universe that the node is responsible for at its current radius but does not
// hold. Departed nodes are absent from the map.
func (n *Network) deficitSets() map[int]map[string]struct{} {
	alive := n.Survivors()

	universe := make(map[string]Entry)
	held := make(map[int]map[string]struct{}, len(alive))
	for _, i := range alive {
		entries := n.nodes[i].Reserve.Entries()
		set := make(map[string]struct{}, len(entries))
		for _, e := range entries {
			key := presenceKey(e.Address, e.BatchID, e.StampHash)
			set[key] = struct{}{}
			universe[key] = e
		}
		held[i] = set
	}

	out := make(map[int]map[string]struct{}, len(alive))
	for _, i := range alive {
		base := n.nodes[i].Addr.Bytes()
		radius := n.nodes[i].Reserve.StorageRadius()
		miss := make(map[string]struct{})
		for key, e := range universe {
			if _, ok := held[i][key]; ok {
				continue
			}
			if swarm.Proximity(base, e.Address.Bytes()) >= radius {
				miss[key] = struct{}{}
			}
		}
		out[i] = miss
	}
	return out
}

// Deficit returns the current per-node deficit counts, indexed by node.
// Departed nodes report 0.
func (n *Network) Deficit() []int {
	sets := n.deficitSets()
	out := make([]int, len(n.nodes))
	for i, s := range sets {
		out[i] = len(s)
	}
	return out
}
