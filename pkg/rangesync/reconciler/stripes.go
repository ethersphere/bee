// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"math/bits"
	"slices"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	// StripeRedundancy is the number of peers PlanStripes assigns to each
	// stripe (§9, "optionally assign each stripe to 2 peers"). The
	// verification round (§7.4) already covers withholding, so the default
	// is 1.
	StripeRedundancy = 1

	// BootstrapRatio is the detection threshold of IsBootstrap: a node is
	// bootstrapping when its own count is below BootstrapRatio times the
	// median count of the peers with the same root.
	BootstrapRatio = 0.5

	// maxStripeBits bounds the depth of the stripes below the striped
	// range, so that a plan holds at most 2^16 stripes.
	maxStripeBits = 16
)

// PeerStatus is a peer with its last Status.
type PeerStatus struct {
	Peer   swarm.Address
	Status rangesync.Status
}

// Stripe is a prefix-aligned part of a striped range and the peers assigned
// to transfer it, in order of preference. A stripe no peer covers has no
// Peers; the caller reports it rather than dropping it.
type Stripe struct {
	Range fingerprint.Range
	Peers []swarm.Address
}

// IsBootstrap reports whether this node, with status own, is bootstrapping
// (§7.4): its count is below BootstrapRatio times the median count of the
// peers whose root equals its own. Peers with other roots are not compared,
// as their counts cover other ranges. With no such peer, or a zero median,
// it returns false. A bootstrapping node whose root differs from every
// neighbor's root is therefore not detected here; LacksSessionRoot covers it
// per peer, and the sessions' large-difference branch (rangesync.Striper)
// per range.
func IsBootstrap(own rangesync.Status, peers []PeerStatus) bool {
	var counts []uint64
	for _, p := range peers {
		if p.Status.Root.Equal(own.Root) {
			counts = append(counts, p.Status.Summary.Count)
		}
	}
	if len(counts) == 0 {
		return false
	}
	slices.Sort(counts)
	median := counts[len(counts)/2]
	return median > 0 && float64(own.Summary.Count) < BootstrapRatio*float64(median)
}

// LacksSessionRoot is the session-root check of the rounds, next to
// IsBootstrap (T4.7 part 1, constraint (d) of AC-B2). It reports whether
// this node, with status own, holds fewer elements than a peer with status
// peer in their session root (rangesync.SessionRoot) by more than threshold,
// and returns the session root. A session this node opened there would open
// at a count far below the peer's, which the peer answers as it answers a
// stripe session: at count 0 it lists all it holds (DecisionSendItems), so
// the caller stripes the session root instead. It compares two counts of
// one range, so an edge peer (§7.6) is compared over the session root only,
// not by the counts of different roots. This node's count is own's when the
// session root is its root, and otherwise (the peer's root lies inside
// this node's) the one summary returns; the peer's is its Status count when
// the session root is its root, and otherwise (this node's root lies inside
// the peer's) its count spread evenly over its root: shifted right by the
// bits the session root lies below the peer's root, an estimate that
// assumes uniformly spread addresses. It reports false for disjoint roots,
// and when summary cannot summarize the range (it returns false).
func LacksSessionRoot(own, peer rangesync.Status, summary func(fingerprint.Range) (uint64, bool), threshold uint64) (root fingerprint.Range, lacks bool) {
	root, ok := rangesync.SessionRoot(own.Root, peer.Root)
	if !ok {
		return root, false
	}
	local, remote := own.Summary.Count, peer.Summary.Count
	if !root.Equal(own.Root) {
		if local, ok = summary(root); !ok {
			return root, false
		}
	}
	if below := int(root.Bits) - int(peer.Root.Bits); below > 0 {
		if below >= 64 {
			remote = 0
		} else {
			remote >>= below
		}
	}
	return root, remote > local && remote-local > threshold
}

// PlanStripes splits r into 2^s prefix-aligned stripes and assigns them to
// peers with StripeRedundancy (see PlanStripesRedundancy). r is the session
// root on bootstrap, or a sub-range of it, such as the new sibling half
// after a radius decrease (§7.5).
func PlanStripes(r fingerprint.Range, peers []PeerStatus) []Stripe {
	return PlanStripesRedundancy(r, peers, StripeRedundancy)
}

// PlanStripesRedundancy splits r into 2^s prefix-aligned stripes and assigns
// stripe i to peer i mod k and, for redundancy above 1, the peers after it,
// where k is the number of peers whose root overlaps r. Only a peer whose
// root covers a stripe is eligible for it (§7.6); an ineligible peer's turn
// passes to the next eligible one. s is the smallest with 2^s >= k, deepened
// so that every stripe lies either inside or outside each peer root at most
// maxStripeBits below r; a peer root deeper than that covers no stripe and the
// peer is never eligible. A stripe with no eligible peer is returned with no
// Peers. redundancy <= 0 is treated as 1; with fewer eligible peers than
// redundancy a stripe gets them all. PlanStripesRedundancy is a pure function.
func PlanStripesRedundancy(r fingerprint.Range, peers []PeerStatus, redundancy int) []Stripe {
	redundancy = max(redundancy, 1)
	r = fingerprint.NewRange(r.Prefix, r.Bits)
	limit := min(maxStripeBits, fingerprint.MaxBits-int(r.Bits))

	var overlapping []PeerStatus
	for _, p := range peers {
		if r.ContainsRange(p.Status.Root) || p.Status.Root.ContainsRange(r) {
			overlapping = append(overlapping, p)
		}
	}
	k := len(overlapping)
	s := 0
	if k > 1 {
		s = bits.Len(uint(k - 1))
	}
	for _, p := range overlapping {
		if d := int(p.Status.Root.Bits) - int(r.Bits); d > s && d <= limit {
			s = d
		}
	}
	s = min(s, limit)

	stripes := make([]Stripe, 1<<s)
	for i := range stripes {
		sr := r.Child(uint8(s), uint64(i))
		st := Stripe{Range: sr}
		for j := 0; j < k && len(st.Peers) < redundancy; j++ {
			p := overlapping[(i+j)%k]
			if p.Status.Root.ContainsRange(sr) {
				st.Peers = append(st.Peers, p.Peer)
			}
		}
		stripes[i] = st
	}
	return stripes
}

// Unassigned returns the stripes with no eligible peer.
func Unassigned(stripes []Stripe) []Stripe {
	var out []Stripe
	for _, s := range stripes {
		if len(s.Peers) == 0 {
			out = append(out, s)
		}
	}
	return out
}

// StripeCollector is a rangesync.Striper that takes over every range a
// session offers (the large-difference branch of §6.3, on the side that
// lacks) and keeps it for a striped transfer. It is safe for concurrent use.
// The zero value transfers nothing; a round session's collector asks its
// reconciler which ranges a transfer already covers (Transferring).
type StripeCollector struct {
	mu     sync.Mutex
	ranges []fingerprint.Range
	// transferring, if not nil, is Reconciler.Transferring.
	transferring func(fingerprint.Range) bool
}

var _ rangesync.TransferStriper = (*StripeCollector)(nil)

// Transferring reports whether a striped transfer of the reconciler covers
// r; false for the zero value.
func (c *StripeCollector) Transferring(r fingerprint.Range) bool {
	return c.transferring != nil && c.transferring(r)
}

// Stripe records r and takes it over.
func (c *StripeCollector) Stripe(r fingerprint.Range, _, _ uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ranges = append(c.ranges, fingerprint.NewRange(r.Prefix, r.Bits))
	return true
}

// Ranges returns the ranges taken over so far, in the order offered.
func (c *StripeCollector) Ranges() []fingerprint.Range {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.ranges)
}
