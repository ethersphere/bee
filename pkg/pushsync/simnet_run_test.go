// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/addressbook"
	discmock "github.com/ethersphere/bee/v2/pkg/discovery/mock"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	p2pmock "github.com/ethersphere/bee/v2/pkg/p2p/mock"
	"github.com/ethersphere/bee/v2/pkg/stabilization"
	mockstate "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/topology/kademlia"
)

// simPrimary is the configuration of the plausibility check. It was written
// down before the first run and is not changed to fit the result:
// 1,000 full nodes, radius 8 (3.9 nodes per neighborhood; mainnet at radius
// 10 with about 4,500 nodes has 4.4), a node sees 32 % of its peers as
// healthy, no doubling nodes, every forwarder runs the code of v2.8.3, the
// origin is a light node with the code of this branch, 26,496 chunks.
var simPrimary = simParams{seed: 1, nodes: 1000, radius: 8, healthy: 0.32, chunks: 26496}

// simScenarios are the primary configuration and the runs that change inputs
// of it. The populated runs were added after the primary run showed empty
// neighborhoods; peerhealth has a tenth of the chunks, because most of its
// chunks never sync and each of those takes many pushes.
func simScenarios() map[string]simParams {
	with := func(f func(*simParams)) simParams {
		p := simPrimary
		f(&p)
		return p
	}
	return map[string]simParams{
		"primary":       simPrimary,
		"populated":     with(func(p *simParams) { p.populated = true }),
		"peerhealth":    with(func(p *simParams) { p.peerHealth, p.populated, p.chunks = true, true, 2650 }),
		"popdoubling":   with(func(p *simParams) { p.populated, p.doubling = true, 0.11 }),
		"popseed2":      with(func(p *simParams) { p.populated, p.seed = true, 2 }),
		"popmainnet":    with(func(p *simParams) { p.populated, p.nodes, p.radius = true, 4500, 10 }),
		"upgraded10":    with(func(p *simParams) { p.populated, p.upgraded = true, 0.1 }),
		"upgraded50":    with(func(p *simParams) { p.populated, p.upgraded = true, 0.5 }),
		"upgraded100":   with(func(p *simParams) { p.populated, p.upgraded = true, 1 }),
		"popmainnet2":   with(func(p *simParams) { p.populated, p.nodes, p.radius, p.seed = true, 4500, 10, 2 }),
		"popmainnetdbl": with(func(p *simParams) { p.populated, p.nodes, p.radius, p.doubling = true, 4500, 10, 0.11 }),
		// Input of the comparison with commit 0e0baced3, which has neither of
		// the switches: every forwarder runs the code of the branch, and
		// doubling nodes give the shallow receipts.
		"equivalence": with(func(p *simParams) { p.populated, p.upgraded, p.doubling = true, 1, 0.11 }),
	}
}

// TestSimUpload runs the scenarios named in SIMNET (comma separated, or
// "all"), each without and with the first-hop memory of the origin.
func TestSimUpload(t *testing.T) {
	want := os.Getenv("SIMNET")
	if want == "" {
		t.Skip("set SIMNET to a scenario name to run the simulator")
	}
	for name, p := range simScenarios() {
		if want != "all" && !containsField(want, name) {
			continue
		}
		for _, hop := range []bool{false, true} {
			p.hopMemory = hop
			variant := "control"
			if hop {
				variant = "memory"
			}
			t.Run(name+"/"+variant, func(t *testing.T) {
				res, _, _, _ := runSim(t, p)
				t.Logf("\n%s", res)
			})
		}
	}
}

func containsField(list, name string) bool {
	for len(list) > 0 {
		i := 0
		for i < len(list) && list[i] != ',' {
			i++
		}
		if list[:i] == name {
			return true
		}
		if i == len(list) {
			break
		}
		list = list[i+1:]
	}
	return false
}

// TestSimSmoke keeps the simulator working: a small network must sync every
// chunk, and two runs of the control must give the same result per chunk.
func TestSimSmoke(t *testing.T) {
	p := simParams{seed: 7, nodes: 150, radius: 5, healthy: 0.32, chunks: 600}

	first, _, _, _ := runSim(t, p)
	second, _, _, _ := runSim(t, p)
	if first.couldNotSync != 0 || first.otherErrors != 0 {
		t.Fatalf("not synced: %d, other errors: %d", first.couldNotSync, first.otherErrors)
	}
	if first.shallow == 0 {
		t.Fatal("forwarders of v2.8.3 gave no shallow receipt")
	}
	if first.digest != second.digest {
		t.Fatalf("control run is not reproducible: %s, %s", first.digest, second.digest)
	}

	p.hopMemory = true
	memory, _, _, _ := runSim(t, p)
	if memory.couldNotSync != 0 || memory.otherErrors != 0 {
		t.Fatalf("with memory, not synced: %d, other errors: %d", memory.couldNotSync, memory.otherErrors)
	}
	if memory.shallow >= first.shallow {
		t.Fatalf("shallow receipts with memory %d, without %d", memory.shallow, first.shallow)
	}
}

// TestSimTopologyMatchesKademlia checks the static driver of the simulator
// against the real kademlia over the same peer table.
func TestSimTopologyMatchesKademlia(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	randAddr := func() swarm.Address {
		b := make([]byte, swarm.HashSize)
		rng.Read(b)
		return swarm.NewAddress(b)
	}
	// near returns an address that shares po bits with a.
	near := func(a swarm.Address, po int) swarm.Address {
		b := randAddr().Bytes()
		for k := range po {
			b[k/8] = b[k/8]&^(0x80>>(k%8)) | a.Bytes()[k/8]&(0x80>>(k%8))
		}
		b[po/8] = b[po/8]&^(0x80>>(po%8)) | ^a.Bytes()[po/8]&(0x80>>(po%8))
		return swarm.NewAddress(b)
	}

	detector, err := stabilization.NewDetector(stabilization.Config{PeriodDuration: time.Second, NumPeriodsForStabilization: 2})
	if err != nil {
		t.Fatal(err)
	}
	var (
		base = randAddr()
		disc = discmock.NewDiscovery(discmock.WithBroadcastPeers(func(context.Context, swarm.Address, ...swarm.Address) error { return nil }))
		ab   = addressbook.New(mockstate.NewStateStore())
		sim  = &simTopology{base: base}
	)
	kad, err := kademlia.New(base, ab, disc, p2pmock.New(), detector, log.Noop, kademlia.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = kad.Close() })
	kad.UpdateReachability(p2p.ReachabilityStatusPublic)

	for i := range 140 {
		addr := near(base, i%12)
		healthy := rng.Float64() < 0.32
		if err := kad.Connected(context.Background(), p2p.Peer{Address: addr, FullNode: true}, true); err != nil {
			t.Fatal(err)
		}
		kad.Reachable(addr, p2p.ReachabilityStatusPublic)
		kad.UpdatePeerHealth(addr, healthy, time.Millisecond)
		sim.peers = append(sim.peers, simPeer{addr: addr, po: swarm.Proximity(base.Bytes(), addr.Bytes()), healthy: healthy})
	}

	filters := []topology.Select{{}, {Reachable: true}, {Reachable: true, Healthy: true}}

	for _, filter := range filters {
		collect := func(d topology.Driver, rev bool) map[string]uint8 {
			m := make(map[string]uint8)
			each := d.EachConnectedPeer
			if rev {
				each = d.EachConnectedPeerRev
			}
			_ = each(func(a swarm.Address, po uint8) (bool, bool, error) {
				m[a.ByteString()] = po
				return false, false, nil
			}, filter)
			return m
		}
		for _, rev := range []bool{false, true} {
			got, want := collect(sim, rev), collect(kad, rev)
			if len(got) != len(want) {
				t.Fatalf("filter %+v: %d peers, kademlia has %d", filter, len(got), len(want))
			}
			for k, po := range want {
				if got[k] != po {
					t.Fatalf("filter %+v: peer %x in bin %d, kademlia has %d", filter, k, got[k], po)
				}
			}
		}
	}

	for i := range 20000 {
		var addr swarm.Address
		switch i % 3 {
		case 0:
			addr = randAddr()
		case 1:
			addr = near(base, rng.Intn(14))
		default:
			addr = near(sim.peers[rng.Intn(len(sim.peers))].addr, 8+rng.Intn(8))
		}
		var skip []swarm.Address
		for range rng.Intn(4) * rng.Intn(40) {
			skip = append(skip, sim.peers[rng.Intn(len(sim.peers))].addr)
		}
		if i%500 == 0 { // everything skipped
			skip = skip[:0]
			for _, p := range sim.peers {
				skip = append(skip, p.addr)
			}
		}
		filter, self := filters[rng.Intn(len(filters))], rng.Intn(2) == 0

		got, gotErr := sim.ClosestPeer(addr, self, filter, skip...)
		want, wantErr := kad.ClosestPeer(addr, self, filter, skip...)
		if !errors.Is(gotErr, wantErr) || !got.Equal(want) {
			t.Fatalf("query %d (self %v, filter %+v, %d skipped): got %s, %v; kademlia gives %s, %v", i, self, filter, len(skip), got, gotErr, want, wantErr)
		}
	}

	empty := &simTopology{base: base}
	if _, err := empty.ClosestPeer(randAddr(), true, topology.Select{}); !errors.Is(err, topology.ErrNotFound) {
		t.Fatalf("empty table: %v", err)
	}
}
