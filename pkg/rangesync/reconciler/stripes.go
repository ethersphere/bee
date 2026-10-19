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
// it returns false.
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
// so that every stripe lies either inside or outside each peer root narrower
// than r (at most maxStripeBits below r). A stripe with no eligible peer is
// returned with no Peers. PlanStripesRedundancy is a pure function.
func PlanStripesRedundancy(r fingerprint.Range, peers []PeerStatus, redundancy int) []Stripe {
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
// session offers (the d̂ > LargeDiff branch of §6.3) and keeps it for a
// striped transfer. It is safe for concurrent use.
type StripeCollector struct {
	mu     sync.Mutex
	ranges []fingerprint.Range
}

var _ rangesync.Striper = (*StripeCollector)(nil)

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
