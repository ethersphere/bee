// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Scaled bootstraps (T4.7 part 1, AC-B2): the neighborhood simulation with a
// test-settable LargeDiff of Base/64, so that a lacking node's root
// difference stands to LargeDiff as 2^22's does to 65536 (64), its stripes
// (Base/4 IDs over four peers) are past LargeDiff, and so are the level-1
// ranges of its root while it lacks more than a quarter of it. The harness
// geometry otherwise stays below LargeDiff (T4.6 part 3, AC-B2).

// bootstrapIDBytes is the bytes of one ID in an Items page (AC-B2's 70 B).
const bootstrapIDBytes = 70

// bootstrapFigures is what a scaled bootstrap measured for its lacking node
// (simOptions.LagNode).
type bootstrapFigures struct {
	res *simResult
	// rank is the number of the lacking node's peers with a lower overlay:
	// the initiators of its pairs.
	rank int
	// metaIn is the lacking node's bytes_metadata{dir="in"}, and bound AC-B2
	// scaled to the IDs it lacked: 1.15 × Lag × 70 B, plus 1 MB of
	// verification.
	metaIn, bound float64
	// peerSessions counts the sessions its peers opened with it (rounds and
	// takeovers, not stripes), and takeovers those of the peers with a
	// higher overlay, which open only once their wait ran out.
	peerSessions, takeovers int
	// stripesDone and stripesTimeout are stripes_total{done} and
	// {timeout}, summed over the nodes; ownStripes counts the stripe
	// sessions the lacking node opened, and peerStripes those its peers
	// opened, ranges they registered although they hold them.
	stripesDone, stripesTimeout float64
	ownStripes, peerStripes     int
	// own, peers and status are the bytes the lacking node read on the
	// reconcile streams it opened (its stripes, verifications and later
	// sessions), on those its peers opened, and on the Status streams;
	// wants and maxWants the IDs it wanted from its peers' sessions, in
	// all and in the largest (an ID that several sessions list counts in
	// each).
	own, peers, status int
	wants, maxWants    int
	// served is the number of distinct IDs the lacking node wanted from its
	// peers' sessions, lagging those of them it lacked at the start (the
	// rest are uploads), and uploads the uploads of the run.
	served, lagging, uploads int
}

// runBootstrap runs o, a neighborhood with a lagging node, and reads off its
// figures.
func runBootstrap(t *testing.T, o simOptions) bootstrapFigures {
	t.Helper()
	res := runNeighborhood(t, o)
	lag := res.Nodes[o.LagNode]
	f := bootstrapFigures{
		res:            res,
		metaIn:         lag.MetadataIn,
		bound:          1.15*float64(o.Lag)*bootstrapIDBytes + 1e6,
		stripesDone:    res.Counters["stripes_total{result=done}"],
		stripesTimeout: res.Counters["stripes_total{result=timeout}"],
		served:         lag.ServedWants,
		lagging:        lag.ServedLagging,
		uploads:        o.Batches * o.BatchSize,
	}
	lower := make(map[int]bool)
	for i, n := range res.Nodes {
		if i != o.LagNode && bytes.Compare(n.Addr.Bytes(), lag.Addr.Bytes()) < 0 {
			f.rank++
			lower[i] = true
		}
	}
	for i := range res.Nodes {
		if i == o.LagNode {
			continue
		}
		f.own += res.Flows[simFlow{From: i, To: o.LagNode, Opener: o.LagNode, Stream: rangesync.ReconcileStreamName}]
		f.peers += res.Flows[simFlow{From: i, To: o.LagNode, Opener: i, Stream: rangesync.ReconcileStreamName}]
		f.status += res.Flows[simFlow{From: i, To: o.LagNode, Opener: i, Stream: rangesync.StatusStreamName}] +
			res.Flows[simFlow{From: i, To: o.LagNode, Opener: o.LagNode, Stream: rangesync.StatusStreamName}]
	}
	for _, s := range res.PairwiseDiff {
		if s.Node == o.LagNode && !s.Session.Initiator {
			f.wants += s.Wants
			f.maxWants = max(f.maxWants, s.Wants)
		}
	}
	for _, sp := range res.Spans {
		if sp.Kind != "reconcile" {
			continue
		}
		switch {
		case sp.Stripe && sp.Node == o.LagNode:
			f.ownStripes++
		case sp.Stripe:
			f.peerStripes++
		case sp.Peer.Equal(lag.Addr) && sp.Err == nil:
			f.peerSessions++
			if !lower[sp.Node] {
				f.takeovers++
			}
		}
	}
	return f
}

// check checks the figures of a scaled bootstrap: (a) its stripes completed,
// none timed out; (b) the lacking node's metadata is within AC-B2 scaled; no
// peer opened a stripe session (registered a range it holds); no chunk was
// delivered twice.
func (f bootstrapFigures) check(t *testing.T) {
	t.Helper()
	if f.ownStripes == 0 || f.stripesDone == 0 || f.stripesTimeout != 0 {
		t.Errorf("stripes: %d sessions of the lacking node, %.0f done, %.0f timed out", f.ownStripes, f.stripesDone, f.stripesTimeout)
	}
	if f.metaIn > f.bound {
		t.Errorf("lacking node's metadata %.0f B, over AC-B2's %.0f B", f.metaIn, f.bound)
	}
	if f.peerStripes != 0 {
		t.Errorf("peers opened %d stripe sessions", f.peerStripes)
	}
	if f.res.Duplicates != 0 {
		t.Errorf("%d duplicate deliveries", f.res.Duplicates)
	}
}

// checkDeclined checks the headline claim of a bootstrap the lacking node
// detects itself (IsBootstrap: its own transfer of its root, at threshold
// 0): its peers' sessions list it nothing it lacked at the start, only
// uploads, at most each once as a distinct ID.
func (f bootstrapFigures) checkDeclined(t *testing.T) {
	t.Helper()
	if f.lagging != 0 {
		t.Errorf("peers' sessions listed %d IDs the lacking node lacked at the start", f.lagging)
	}
	if f.served > f.uploads {
		t.Errorf("peers' sessions listed %d distinct IDs, more than the %d uploads", f.served, f.uploads)
	}
}

func (f bootstrapFigures) String() string {
	return fmt.Sprintf("rank %d, caught up %.2f s, metadata in %.0f KB of AC-B2's %.0f KB (%.2f × the lacked IDs; read: own sessions %.0f KB, peers' %.0f KB, Status %.0f KB), peers' sessions %d (takeovers %d; wants %d, max %d; distinct %d of %d uploads, lacked %d), stripe sessions %d (peers' %d), stripes done %.0f, timed out %.0f",
		f.rank, f.res.CaughtUp.Seconds(), f.metaIn/1000, f.bound/1000, f.metaIn/((f.bound-1e6)/1.15), float64(f.own)/1000, float64(f.peers)/1000, float64(f.status)/1000,
		f.peerSessions, f.takeovers, f.wants, f.maxWants, f.served, f.uploads, f.lagging, f.ownStripes, f.peerStripes, f.stripesDone, f.stripesTimeout)
}

// atRank returns a simOptions.Overlay that gives node 0 rank peers with a
// lower overlay, the initiators of its pairs: node 0's first byte is 0x80,
// peers 1 to rank have the top bit clear, and the others a first byte above
// 0x80.
func atRank(rank int) func(i int, a swarm.Address) swarm.Address {
	return func(i int, a swarm.Address) swarm.Address {
		b := a.Bytes()
		switch {
		case i == 0:
			b[0] = 0x80
		case i <= rank:
			b[0] &= 0x7f
		default:
			b[0] |= 0x81
		}
		return swarm.NewAddress(b)
	}
}

// scaledBootstrap returns the options of a scaled bootstrap: N = 5, node 0
// empty of Base chunks, LargeDiff Base/64, Delay 50 ms, the node-wide fetch
// limit lifted as in TestG6Bootstrap.
func scaledBootstrap(base int, seed uint64) simOptions {
	return simOptions{
		Nodes: 5, Replication: 3, Seed: seed, Base: base, Lag: base, Batches: 1, BatchSize: 4, Delay: 50 * time.Millisecond,
		LargeDiff:      uint64(base / 64),
		FetchClockRate: base / 120,
		Service:        func(so *rangesync.Options) { so.FetchRate = 1 << 20 },
	}
}

// TestNeighborhoodBootstrapScaled runs an empty node's bootstrap with the
// scaled LargeDiff, at rank 2 (two initiators, two peers that take over):
// its peers' round sessions with it, takeovers included, list nothing it
// lacked (it declines the root as their responder, at threshold 0 through
// the transfer's tail), and its stripes, each past LargeDiff, are listed in
// full.
func TestNeighborhoodBootstrapScaled(t *testing.T) {
	t.Parallel()

	o := scaledBootstrap(8000, 1)
	o.Overlay = atRank(2)
	f := runBootstrap(t, o)
	t.Log(f)
	if f.rank != 2 {
		t.Fatalf("rank %d, want 2", f.rank)
	}
	f.check(t)
	f.checkDeclined(t)
	if f.peerSessions == 0 || f.takeovers == 0 {
		t.Errorf("%d sessions of the peers with the bootstrapping node, %d takeovers; want both", f.peerSessions, f.takeovers)
	}
}

// TestNeighborhoodBootstrapStripeTimeout: stripes that outlast StripeTimeout
// (30 s here, the 2-minute transfer's stripes take ~2 minutes each) keep
// storing, so they run on as one attempt each (StripeTimeout is an
// inactivity timeout, T4.7 part 1 fix round 1): none times out or restarts,
// and nothing is listed twice, so the lacking node stays within AC-B2.
// Before, each restart listed the stripe's remainder again.
func TestNeighborhoodBootstrapStripeTimeout(t *testing.T) {
	t.Parallel()

	o := scaledBootstrap(8000, 5)
	o.Overlay = atRank(1)
	o.Reconciler = func(ro *reconciler.Options) { ro.StripeTimeout = 30 * time.Second }
	f := runBootstrap(t, o)
	t.Log(f)
	if f.res.CaughtUp < 3*30*time.Second {
		t.Fatalf("caught up after %v: the stripes did not outlast the timeout", f.res.CaughtUp)
	}
	f.check(t)
	f.checkDeclined(t)
	// one completed session per stripe and one per verification (a Busy
	// answer is retried in the same attempt): no restart
	completed := 0
	for _, sp := range f.res.Spans {
		if sp.Kind == "reconcile" && sp.Stripe && sp.Node == o.LagNode && sp.Err == nil {
			completed++
		}
	}
	if completed > 2*int(f.stripesDone) {
		t.Errorf("%d completed stripe sessions for %.0f stripes: restarted", completed, f.stripesDone)
	}
}

// TestNeighborhoodBootstrapResponderOnly: the node with the highest overlay,
// the responder of every pair, lacks a level-1 range of its root, 4 ×
// LargeDiff, and is not bootstrapping (it holds 15/16 of the reserve). It
// declines its peers' sessions and transfers the root striped; it does not
// starve, and no peer lists the range to it.
func TestNeighborhoodBootstrapResponderOnly(t *testing.T) {
	t.Parallel()

	const base = 8000
	o := scaledBootstrap(base, 2)
	o.Lag = base / 16
	o.LagRoot = rangeOf("0110")
	o.Overlay = func(i int, a swarm.Address) swarm.Address {
		if i == 0 {
			b := a.Bytes()
			b[0], b[1] = 0xff, 0xff
			return swarm.NewAddress(b)
		}
		return a
	}
	f := runBootstrap(t, o)
	t.Log(f)
	if f.rank != o.Nodes-1 {
		t.Fatalf("rank %d, want the highest overlay", f.rank)
	}
	f.check(t)
}

// TestNeighborhoodBootstrapEdgePeer: an empty node bootstraps among peers one
// of which, an edge peer, is one bit deeper (radius 1, §7.6) and holds half
// the others' count. Nothing compares counts across roots, so the edge peer
// is neither deferred nor starved: its round sessions run, its uploads
// converge, and it registers no transfer (it lacks nothing in its root).
// Whether it serves a stripe depends on the plan's order of the peers
// (PlanStripes passes an ineligible peer's turn on).
func TestNeighborhoodBootstrapEdgePeer(t *testing.T) {
	t.Parallel()

	o := scaledBootstrap(8000, 3)
	const edge = 4
	o.Radius = map[int]uint8{edge: 1}
	o.Batches, o.BatchSize = 2, 16
	f := runBootstrap(t, o)
	t.Log(f)
	f.check(t)
	rounds := 0
	for _, sp := range f.res.Spans {
		if sp.Kind == "reconcile" && !sp.Stripe && sp.Err == nil && (sp.Node == edge || sp.Peer.Equal(f.res.Nodes[edge].Addr)) {
			rounds++
		}
	}
	if rounds == 0 {
		t.Error("no round session of the edge peer")
	}
}

// TestNeighborhoodBootstrapForeignRoot: an empty node whose root no peer
// shares (constraint (d); IsBootstrap does not detect it) transfers its
// session root with its peers striped, in both nestings: its root contains
// its peers' (it has the smaller radius), or its root lies inside theirs
// (the larger radius; their counts in it are estimated). It has the lowest
// overlay, so that it initiates with every peer: its rounds' session-root
// check, not its answers as a responder, keeps the peers from listing.
func TestNeighborhoodBootstrapForeignRoot(t *testing.T) {
	t.Parallel()

	const base = 8000
	half := rangeOf("1")
	// overlay puts node 0 at prefix first, lowest of all, and every peer in
	// the half
	overlay := func(first byte) func(i int, a swarm.Address) swarm.Address {
		return func(i int, a swarm.Address) swarm.Address {
			b := a.Bytes()
			if i == 0 {
				b[0], b[1] = first, 0
			} else {
				b[0] |= 0x80
			}
			return swarm.NewAddress(b)
		}
	}
	for _, tc := range []struct {
		name string
		o    func(o *simOptions)
	}{
		{
			// the peers at radius 1 in one half, the chunks in it
			name: "its root contains the peers'",
			o: func(o *simOptions) {
				o.Radius = map[int]uint8{1: 1, 2: 1, 3: 1, 4: 1}
				o.ChunkRoot = half
				o.Overlay = overlay(0x00)
			},
		},
		{
			// node 0 at radius 1 in the half, the chunks in it
			name: "its root inside the peers'",
			o: func(o *simOptions) {
				o.Radius = map[int]uint8{0: 1}
				o.ChunkRoot = half
				o.Overlay = overlay(0x80)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := scaledBootstrap(base, 4)
			tc.o(&o)
			f := runBootstrap(t, o)
			t.Log(f)
			if f.rank != 0 {
				t.Fatalf("rank %d, want the lowest overlay", f.rank)
			}
			f.check(t)
			lagRoot := fingerprint.NewRange(f.res.Nodes[0].Addr, o.Radius[0])
			if !lagRoot.ContainsRange(half) && !half.ContainsRange(lagRoot) {
				t.Fatalf("node 0's root %s and the chunks' %s do not overlap", lagRoot, half)
			}
		})
	}
}
