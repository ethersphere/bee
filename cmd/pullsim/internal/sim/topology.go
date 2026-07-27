// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Topology names the peer-connection graph shape.
type Topology string

const (
	// TopologyFull connects every node to every other node.
	TopologyFull Topology = "full"
	// TopologyRing connects each node to degree/2 neighbours on each side of
	// the address-sorted ring.
	TopologyRing Topology = "ring"
	// TopologyKNearest connects each node to its degree nearest peers by
	// proximity (symmetrised).
	TopologyKNearest Topology = "k-nearest"
	// TopologyRandom connects each node to ~degree random peers (symmetrised).
	TopologyRandom Topology = "random"
	// TopologyKademlia mirrors Bee's kademlia: up to degree peers in each
	// proximity order bin below the storage radius, and every peer at or above
	// it (the neighborhood is fully connected). Unlike k-nearest, which only
	// links close peers, holding a few peers in every bin gives the graph
	// O(log N) diameter.
	TopologyKademlia Topology = "kademlia"
)

// ParseTopology validates and returns a Topology.
func ParseTopology(s string) (Topology, error) {
	switch Topology(s) {
	case TopologyFull, TopologyRing, TopologyKNearest, TopologyRandom, TopologyKademlia:
		return Topology(s), nil
	default:
		return "", fmt.Errorf("unknown topology %q (want full|ring|k-nearest|random|kademlia)", s)
	}
}

// edgePO returns the proximity order between two addresses, capped at bins-1 so
// it is a valid index into per-bin cursors.
func edgePO(a, b swarm.Address, bins uint8) uint8 {
	po := swarm.Proximity(a.Bytes(), b.Bytes())
	if po > bins-1 {
		po = bins - 1
	}
	return po
}

// buildAdjacency returns, for each node index, the sorted set of peer indices
// it is connected to. The graph is always symmetric.
//
// bins and radius are only read by TopologyKademlia. The graph is built once,
// from the configured radius; a later SetRadius changes sync behaviour but does
// not rewire the topology, so reshaping it needs a rebuild.
func buildAdjacency(topo Topology, addrs []swarm.Address, degree int, bins, radius uint8, rng *rand.Rand) [][]int {
	n := len(addrs)
	sets := make([]map[int]struct{}, n)
	for i := range sets {
		sets[i] = make(map[int]struct{})
	}
	link := func(i, j int) {
		if i == j {
			return
		}
		sets[i][j] = struct{}{}
		sets[j][i] = struct{}{}
	}

	switch topo {
	case TopologyFull:
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				link(i, j)
			}
		}

	case TopologyRing:
		order := make([]int, n)
		for i := range order {
			order[i] = i
		}
		sort.Slice(order, func(a, b int) bool {
			return addrs[order[a]].Compare(addrs[order[b]]) < 0
		})
		side := degree / 2
		if side < 1 {
			side = 1
		}
		for pos := 0; pos < n; pos++ {
			for d := 1; d <= side; d++ {
				link(order[pos], order[(pos+d)%n])
			}
		}

	case TopologyKNearest:
		for i := 0; i < n; i++ {
			others := make([]int, 0, n-1)
			for j := 0; j < n; j++ {
				if j != i {
					others = append(others, j)
				}
			}
			sort.Slice(others, func(a, b int) bool {
				return swarm.Proximity(addrs[i].Bytes(), addrs[others[a]].Bytes()) >
					swarm.Proximity(addrs[i].Bytes(), addrs[others[b]].Bytes())
			})
			k := degree
			if k > len(others) {
				k = len(others)
			}
			for _, j := range others[:k] {
				link(i, j)
			}
		}

	case TopologyKademlia:
		for i := 0; i < n; i++ {
			// Bucket every other node by proximity order. A slice indexed by
			// bin, not a map, so the rng is consumed in a fixed order and the
			// graph stays reproducible from the seed.
			byBin := make([][]int, bins)
			for j := 0; j < n; j++ {
				if j == i {
					continue
				}
				po := edgePO(addrs[i], addrs[j], bins)
				byBin[po] = append(byBin[po], j)
			}
			for po := 0; po < int(bins); po++ {
				peers := byBin[po]
				if uint8(po) >= radius {
					// Neighborhood: everyone this close is a peer.
					for _, j := range peers {
						link(i, j)
					}
					continue
				}
				// Below the radius the bin is merely saturated: shuffle so the
				// choice is not biased by address order, then take degree.
				rng.Shuffle(len(peers), func(a, b int) { peers[a], peers[b] = peers[b], peers[a] })
				k := degree
				if k > len(peers) {
					k = len(peers)
				}
				for _, j := range peers[:k] {
					link(i, j)
				}
			}
		}

	case TopologyRandom:
		for i := 0; i < n; i++ {
			perm := rng.Perm(n)
			added := 0
			for _, j := range perm {
				if added >= degree {
					break
				}
				if j == i {
					continue
				}
				link(i, j)
				added++
			}
		}
	}

	out := make([][]int, n)
	for i := range sets {
		peers := make([]int, 0, len(sets[i]))
		for j := range sets[i] {
			peers = append(peers, j)
		}
		sort.Ints(peers)
		out[i] = peers
	}
	return out
}
