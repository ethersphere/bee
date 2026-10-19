// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func stripeRange(prefix byte, bits uint8) fingerprint.Range {
	return fingerprint.NewRange(swarm.NewAddress([]byte{prefix, 0, 0, 0}), bits)
}

func peerAt(i int, root fingerprint.Range, count uint64) reconciler.PeerStatus {
	addr := make([]byte, swarm.HashSize)
	addr[0], addr[1] = byte(i>>8), byte(i)
	return reconciler.PeerStatus{
		Peer:   swarm.NewAddress(addr),
		Status: rangesync.Status{Root: root, Summary: fingerprint.Summary{Count: count}},
	}
}

// checkPartition checks that the stripes partition root in address order.
func checkPartition(t *testing.T, root fingerprint.Range, stripes []reconciler.Stripe) uint8 {
	t.Helper()
	if len(stripes) == 0 {
		t.Fatal("no stripes")
	}
	bits := stripes[0].Range.Bits
	if len(stripes) != 1<<(bits-root.Bits) {
		t.Fatalf("%d stripes at depth %d below %s", len(stripes), bits, root)
	}
	for i, s := range stripes {
		if want := root.Child(bits-root.Bits, uint64(i)); !s.Range.Equal(want) {
			t.Fatalf("stripe %d is %s, want %s", i, s.Range, want)
		}
	}
	return bits - root.Bits
}

func TestPlanStripes(t *testing.T) {
	t.Parallel()

	root := stripeRange(0x40, 2)
	for _, k := range []int{1, 3, 8, 17} {
		t.Run(fmt.Sprintf("k=%d", k), func(t *testing.T) {
			t.Parallel()
			peers := make([]reconciler.PeerStatus, k)
			for i := range peers {
				peers[i] = peerAt(i, root, 1000)
			}
			stripes := reconciler.PlanStripes(root, peers)
			s := checkPartition(t, root, stripes)
			if 1<<s < k || (s > 0 && 1<<(s-1) >= k) {
				t.Fatalf("2^%d stripes for %d peers", s, k)
			}
			for i, st := range stripes {
				if len(st.Peers) != reconciler.StripeRedundancy {
					t.Fatalf("stripe %d has %d peers", i, len(st.Peers))
				}
				if !st.Peers[0].Equal(peers[i%k].Peer) {
					t.Fatalf("stripe %d assigned to %s, want peer %d", i, st.Peers[0], i%k)
				}
			}
		})
	}
}

func TestPlanStripesNarrowerRoots(t *testing.T) {
	t.Parallel()

	root := stripeRange(0x00, 0)
	// peer 0 covers everything; peers 1 and 2 only the 0b10 quarter.
	peers := []reconciler.PeerStatus{
		peerAt(0, root, 1000),
		peerAt(1, stripeRange(0x80, 2), 300),
		peerAt(2, stripeRange(0x80, 2), 300),
	}
	stripes := reconciler.PlanStripes(root, peers)
	s := checkPartition(t, root, stripes)
	if s != 2 {
		t.Fatalf("stripe depth %d, want 2", s)
	}
	for i, st := range stripes {
		if len(st.Peers) != 1 {
			t.Fatalf("stripe %d has %d peers", i, len(st.Peers))
		}
		var ok bool
		for _, p := range peers {
			if p.Peer.Equal(st.Peers[0]) {
				ok = p.Status.Root.ContainsRange(st.Range)
			}
		}
		if !ok {
			t.Fatalf("stripe %s assigned to a peer that does not cover it", st.Range)
		}
	}
	// stripe 2 (0b10) goes to peer 2 (2 mod 3), eligible.
	if !stripes[2].Peers[0].Equal(peers[2].Peer) {
		t.Fatalf("stripe 2 assigned to %s", stripes[2].Peers[0])
	}
	// stripe 1 would be peer 1's, which does not cover it: falls to peer 0.
	if !stripes[1].Peers[0].Equal(peers[0].Peer) {
		t.Fatalf("stripe 1 assigned to %s", stripes[1].Peers[0])
	}
}

func TestPlanStripesUnassigned(t *testing.T) {
	t.Parallel()

	root := stripeRange(0x00, 1)
	peers := []reconciler.PeerStatus{
		peerAt(0, stripeRange(0x00, 3), 10), // 000
		peerAt(1, stripeRange(0x20, 3), 10), // 001
		peerAt(2, stripeRange(0x80, 3), 10), // disjoint from root: ignored
	}
	stripes := reconciler.PlanStripes(root, peers)
	s := checkPartition(t, root, stripes)
	if s != 2 {
		t.Fatalf("stripe depth %d, want 2", s)
	}
	var unassigned int
	for _, st := range stripes {
		if len(st.Peers) == 0 {
			unassigned++
		}
	}
	if unassigned != 2 {
		t.Fatalf("%d unassigned stripes, want 2", unassigned)
	}
	if got := reconciler.Unassigned(stripes); len(got) != 2 {
		t.Fatalf("Unassigned returned %d", len(got))
	}
	if reconciler.PlanStripes(root, nil)[0].Peers != nil {
		t.Fatal("no peers: stripe assigned")
	}
}

func TestPlanStripesRedundancy(t *testing.T) {
	t.Parallel()

	root := stripeRange(0x00, 0)
	peers := []reconciler.PeerStatus{peerAt(0, root, 1), peerAt(1, root, 1), peerAt(2, root, 1)}
	stripes := reconciler.PlanStripesRedundancy(root, peers, 2)
	for i, st := range stripes {
		if len(st.Peers) != 2 || st.Peers[0].Equal(st.Peers[1]) {
			t.Fatalf("stripe %d peers %v", i, st.Peers)
		}
		if !st.Peers[1].Equal(peers[(i+1)%3].Peer) {
			t.Fatalf("stripe %d second peer %s", i, st.Peers[1])
		}
	}
	// redundancy above the eligible peers caps at them
	stripes = reconciler.PlanStripesRedundancy(root, peers[:1], 2)
	if len(stripes) != 1 || len(stripes[0].Peers) != 1 {
		t.Fatalf("got %+v", stripes)
	}
}

// TestPlanStripesSubRange stripes only the new sibling half (T3.4).
func TestPlanStripesSubRange(t *testing.T) {
	t.Parallel()

	newRoot := stripeRange(0x00, 3)
	half := newRoot.Child(1, 1)
	peers := []reconciler.PeerStatus{
		peerAt(0, newRoot, 1), peerAt(1, half, 1), peerAt(2, newRoot.Child(1, 0), 1),
	}
	stripes := reconciler.PlanStripes(half, peers)
	checkPartition(t, half, stripes)
	for _, st := range stripes {
		if !half.ContainsRange(st.Range) {
			t.Fatalf("stripe %s outside %s", st.Range, half)
		}
		if len(st.Peers) != 1 || st.Peers[0].Equal(peers[2].Peer) {
			t.Fatalf("stripe %s peers %v", st.Range, st.Peers)
		}
	}
}

func TestIsBootstrap(t *testing.T) {
	t.Parallel()

	root := stripeRange(0x40, 2)
	own := func(c uint64) rangesync.Status {
		return rangesync.Status{Root: root, Summary: fingerprint.Summary{Count: c}}
	}
	peers := []reconciler.PeerStatus{
		peerAt(0, root, 1000), peerAt(1, root, 900), peerAt(2, root, 10),
		peerAt(3, stripeRange(0x40, 3), 1_000_000), // other root: not compared
	}
	for _, tc := range []struct {
		count uint64
		want  bool
	}{{0, true}, {449, true}, {450, false}, {900, false}} {
		if got := reconciler.IsBootstrap(own(tc.count), peers); got != tc.want {
			t.Errorf("count %d: got %v, want %v", tc.count, got, tc.want)
		}
	}
	if reconciler.IsBootstrap(own(0), nil) {
		t.Error("bootstrap without peers")
	}
	if reconciler.IsBootstrap(own(0), []reconciler.PeerStatus{peerAt(0, root, 0)}) {
		t.Error("bootstrap against empty peers")
	}
}

func TestStripeCollector(t *testing.T) {
	t.Parallel()

	var c reconciler.StripeCollector
	var _ rangesync.Striper = &c
	a, b := stripeRange(0x00, 4), stripeRange(0x10, 4)
	if !c.Stripe(a, 0, rangesync.LargeDiff+1) || !c.Stripe(b, 0, rangesync.LargeDiff+1) {
		t.Fatal("collector declined a range")
	}
	got := c.Ranges()
	if len(got) != 2 || !got[0].Equal(a) || !got[1].Equal(b) {
		t.Fatalf("ranges %v", got)
	}
}

// TestPlanStripesDeepPeer: a peer root more than 16 bits below the range does
// not deepen the stripes and is never eligible.
func TestPlanStripesDeepPeer(t *testing.T) {
	t.Parallel()

	root := stripeRange(0x00, 0)
	deep := fingerprint.NewRange(swarm.NewAddress([]byte{0x80, 0, 0, 0}), 17)
	peers := make([]reconciler.PeerStatus, 0, 2)
	peers = append(peers, peerAt(0, deep, 1))
	stripes := reconciler.PlanStripes(root, peers)
	if s := checkPartition(t, root, stripes); s != 0 {
		t.Fatalf("stripe depth %d, want 0", s)
	}
	if len(reconciler.Unassigned(stripes)) != 1 {
		t.Fatalf("deep peer assigned: %+v", stripes)
	}
	// alongside a covering peer, the deep peer is never used
	peers = append(peers, peerAt(1, root, 1))
	for _, st := range reconciler.PlanStripesRedundancy(root, peers, 2) {
		if len(st.Peers) != 1 || !st.Peers[0].Equal(peers[1].Peer) {
			t.Fatalf("stripe %s peers %v", st.Range, st.Peers)
		}
	}
}

func TestPlanStripesRedundancyBounds(t *testing.T) {
	t.Parallel()

	root := stripeRange(0x00, 0)
	peers := []reconciler.PeerStatus{peerAt(0, root, 1), peerAt(1, root, 1)}
	for _, n := range []int{0, -1} {
		for _, st := range reconciler.PlanStripesRedundancy(root, peers, n) {
			if len(st.Peers) != 1 {
				t.Fatalf("redundancy %d: stripe %s peers %v", n, st.Range, st.Peers)
			}
		}
	}
	for _, st := range reconciler.PlanStripesRedundancy(root, peers, 5) {
		if len(st.Peers) != 2 {
			t.Fatalf("redundancy 5: stripe %s peers %v", st.Range, st.Peers)
		}
	}
}
