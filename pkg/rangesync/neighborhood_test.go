// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
)

// convergenceBound is the most time an upload pushed to p nodes takes to
// reach every node of a neighborhood, on an otherwise quiet network, and
// convergenceRounds the most rounds a node runs meanwhile.
//
// The bound follows §7.8 and the round scheduler (T4.3). An upload changes the
// root of each of its p holders. Within StatusMinInterval the holder's index
// signals the change (its debounce; a timed round may see it sooner): the
// signal sends the holder's Status to every eligible neighbor, unless the
// per-peer schedule defers a changed summary to StatusMinInterval after the
// last one, and triggers the holder's round, which runs no sooner than
// MinRoundInterval after its last round. So:
//   - a lacker with a higher overlay than the holder gets a session from the
//     holder by StatusMinInterval + MinRoundInterval;
//   - a lacker with a lower overlay has the holder's Status by 2 ×
//     StatusMinInterval, which triggers its own round within MinRoundInterval:
//     a session with the holder by 2 × StatusMinInterval + MinRoundInterval.
//
// In that session the lacker decodes the upload and claims it in its want
// set; sessions with the other holders skip it, so each lacker fetches it once
// (§7.8: one round, no redundant fetch). The adaptive interval does not add to
// the bound, since the root-change and Status triggers do not wait for the
// round timer: at its maximum (30 s) it would. A node's rounds are at least
// MinRoundInterval apart, so it runs at most convergenceBound /
// MinRoundInterval rounds after the upload until it converged. The extra
// rounds over §7.8's single round are the Status latency (debounce and
// schedule), which §7.8's round model does not count.
const (
	convergenceBound  = 2*rangesync.StatusMinInterval + rangesync.MinRoundInterval
	convergenceRounds = int(convergenceBound / rangesync.MinRoundInterval)
)

// TestNeighborhoodConverges runs the neighborhood simulation with N = 5 and
// p = 3: random uploads converge within convergenceBound, every node fetches
// each upload it lacks once, and no stream of another protocol is opened.
func TestNeighborhoodConverges(t *testing.T) {
	t.Parallel()

	o := simOptions{Nodes: 5, Replication: 3, Seed: 1}
	res := runNeighborhood(t, o)
	logNeighborhood(t, o, res)
	assertNeighborhood(t, o, res)
}

// TestNeighborhoodSREP runs the neighborhood simulation at §7.8's sizes,
// N = 9 and N = 17, with p = 3.
func TestNeighborhoodSREP(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("neighborhoods of 9 and 17 nodes")
	}

	for _, n := range []int{9, 17} {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			t.Parallel()
			o := simOptions{Nodes: n, Replication: 3, Seed: uint64(n)}
			res := runNeighborhood(t, o)
			logNeighborhood(t, o, res)
			assertNeighborhood(t, o, res)
		})
	}
}

// TestNeighborhoodRoundBounds checks that the simulation's bounds of the
// adaptive round interval reach the reconcilers: after the quiet tail every
// node's interval is at the overridden maximum, and an upload converges
// within the bound of the overridden minimum.
func TestNeighborhoodRoundBounds(t *testing.T) {
	t.Parallel()

	const lo, hi = 2 * time.Second, 8 * time.Second
	o := simOptions{Nodes: 3, Replication: 1, Seed: 3, Batches: 1, BatchSize: 1, MinRoundInterval: lo, MaxRoundInterval: hi}
	res := runNeighborhood(t, o)
	for i, n := range res.Nodes {
		if n.RoundInterval != hi {
			t.Errorf("node %d: round interval %v after the tail, want %v", i, n.RoundInterval, hi)
		}
	}
	if bound := 2*rangesync.StatusMinInterval + lo; res.Batches[0].Convergence > bound {
		t.Errorf("converged in %v, bound %v", res.Batches[0].Convergence, bound)
	}
}

// assertNeighborhood checks the acceptance of T4.6 on res, the result of a
// simulation with options o.
func assertNeighborhood(t *testing.T, o simOptions, res *simResult) {
	t.Helper()
	o = o.withDefaults()

	for i, b := range res.Batches {
		if b.Convergence > convergenceBound {
			t.Errorf("batch %d converged in %v, bound %v", i, b.Convergence, convergenceBound)
		}
		if b.Rounds > convergenceRounds {
			t.Errorf("batch %d took %d rounds of a node, bound %d", i, b.Rounds, convergenceRounds)
		}
	}

	// every node stored each upload it lacked, delivered once
	if res.Duplicates != 0 {
		t.Errorf("%d duplicate rangesync deliveries", res.Duplicates)
	}
	uploads := o.Batches * o.BatchSize
	for i, n := range res.Nodes {
		if lacked := uploads - n.Uploads; n.Stored != lacked || n.Deliveries != lacked {
			t.Errorf("node %d: stored %d, %d deliveries, want the %d uploads it lacked", i, n.Stored, n.Deliveries, lacked)
		}
		// a granted claim whose chunk another fetch stored meanwhile is
		// released unrequested (the reserve check after the claim, T4.4)
		if got := n.WantClaims["granted"]; got < float64(n.Stored) {
			t.Errorf("node %d: want_claims{granted} %v, want at least the %d stored", i, got, n.Stored)
		}
		if got := n.WantClaims["expired"] + n.WantClaims["late"]; got != 0 {
			t.Errorf("node %d: %v expired or late claims", i, got)
		}
	}

	if len(res.Protocols) != 1 || !res.Protocols[rangesync.ProtocolName] {
		t.Errorf("streams of protocols %v, want %s only (no pullsync)", res.Protocols, rangesync.ProtocolName)
	}
	if res.RateExceeded != 0 {
		t.Errorf("%d Statuses or sessions over a peer's rate on the fake clock", res.RateExceeded)
	}
	if got := res.Sessions["error"]; got != 0 {
		t.Errorf("%v sessions failed", got)
	}

	// pairwise_diff: each completed session observed by both sides, below
	// CatchUpMinDiff at this load, and the session differences cover every
	// chunk fetched
	diffs := res.diffsFrom(res.Batches[0].At)
	if want := 2 * res.Sessions["ok"]; float64(len(diffs)) != want {
		t.Errorf("%d pairwise_diff samples, want %v (both sides of each session)", len(diffs), want)
	}
	stored, sum := 0, 0.0
	for _, n := range res.Nodes {
		stored += n.Stored
	}
	for _, d := range diffs {
		sum += d
		if d >= rangesync.CatchUpMinDiff {
			t.Errorf("pairwise_diff sample %v at least CatchUpMinDiff", d)
		}
	}
	if sum < 2*float64(stored) {
		t.Errorf("pairwise_diff samples sum to %v, want at least twice the %d chunks fetched", sum, stored)
	}
	// the histogram the node exposes (Service.Metrics) holds the same samples
	exposed := 0
	for _, n := range res.Nodes {
		exposed += int(n.PairwiseDiff)
	}
	if exposed != len(diffs) {
		t.Errorf("exposed pairwise_diff counts %d samples, want %d", exposed, len(diffs))
	}

	// sessions stop once the uploads converged (AC-C2)
	if quiet := rangesync.StatusMaxInterval + 2*rangesync.MaxRoundInterval; res.LastSession > quiet {
		t.Errorf("a session started %v after convergence, want within %v", res.LastSession, quiet)
	}
}

// logNeighborhood logs the measurements of res.
func logNeighborhood(t *testing.T, o simOptions, res *simResult) {
	t.Helper()
	o = o.withDefaults()
	conv := make([]string, 0, len(res.Batches))
	for _, b := range res.Batches {
		conv = append(conv, fmt.Sprintf("%v/%d", b.Convergence, b.Rounds))
	}
	var meta, chunks float64
	deliveries := 0
	for _, n := range res.Nodes {
		meta += n.MetadataOut
		chunks += n.ChunksOut
		deliveries += n.Deliveries
	}
	uploads := float64(o.Batches * o.BatchSize)
	diffs := res.diffsFrom(res.Batches[0].At)
	zero := 0
	for _, d := range diffs {
		if d == 0 {
			zero++
		}
	}
	t.Logf("N=%d p=%d: convergence/rounds per batch %v; %d deliveries, %d duplicates; metadata %.0f B per upload, chunk bytes %.0f B per upload",
		o.Nodes, o.Replication, conv, deliveries, res.Duplicates, meta/uploads, chunks/uploads)
	t.Logf("sessions %v; pairwise_diff %d samples (%d zero), p50 %v, p99 %v; last session %v after convergence; %d steps, %v",
		res.Sessions, len(diffs), zero, quantile(diffs, 0.5), quantile(diffs, 0.99), res.LastSession, res.Steps, res.Wall.Round(time.Millisecond))
}
