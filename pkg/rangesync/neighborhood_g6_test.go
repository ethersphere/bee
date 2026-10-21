// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
)

// The G6 measurements (T4.6 part 3): what G6 reads off a harness run
// (g6Figures), summarized over seeds (g6Spread). The heavy sweeps are in
// neighborhood_g6_sim_test.go, behind the sim build tag;
// TestNeighborhoodG6Figures runs a short one of each kind in the default
// build.

// g6Quantiles are the median, the 99th percentile (nearest rank) and the
// largest of a set of samples.
type g6Quantiles struct{ P50, P99, Max float64 }

func g6QuantilesOf(xs []float64) g6Quantiles {
	if len(xs) == 0 {
		return g6Quantiles{}
	}
	return g6Quantiles{P50: quantile(xs, 0.5), P99: quantile(xs, 0.99), Max: slices.Max(xs)}
}

// g6Figures are what G6 reads off one harness run: per upload unless named
// otherwise, from the first upload (from the start with a lagging node).
type g6Figures struct {
	// Share is bytes_metadata{in} per upload as a share of AC-B1's pullsync
	// baseline, N(N − 1) × 102 B, and Metadata the bytes.
	Share, Metadata float64
	// Status, Reconcile and Want are the metadata bytes by stream: the
	// Status and reconcile streams' bytes, and the rest (the fetch streams'
	// Wants).
	Status, Reconcile, Want float64
	// SymbolsPerDiff is the symbols received per difference recovered, and
	// MoresPerUpload the More requests.
	SymbolsPerDiff, MoresPerUpload float64
	// RoundTrips is the mean round trips of a completed session the
	// reconcilers opened, the open and the close counted (from Spans, at a
	// Delay only; 0 at Delay 0).
	RoundTrips float64
	// Sessions counts the completed sessions, Refused those answered Busy or
	// over a session limit, and Stale those a recheck ended before they
	// opened (reconciler sessions_total), per upload.
	Sessions, Refused, Stale float64
	// ZeroShare is the share of completed sessions with no difference.
	ZeroShare float64
	// BatchConv and MaxConv are the mean and the longest convergence of a
	// batch, in seconds.
	BatchConv, MaxConv float64
	// Diff holds n_Δ per completed session, from its exact side
	// (simResult.exactDiffsFrom); DiffBoth both sides' samples, as a
	// neighborhood's pairwise_diff aggregates them.
	Diff, DiffBoth g6Quantiles
	// Wants holds the wants per session side (pairwise_diff_half's value),
	// and CatchUp counts the sides with at least CatchUpMinDiff of them: the
	// sessions whose fetches count towards SyncRate.
	Wants   g6Quantiles
	CatchUp int
	// Exhausted counts the streams that exhausted their cache, by level
	// (streams_exhausted_total), and Splits the Fingerprint parts sent
	// beyond the sessions' openings: the children of the ranges split.
	Exhausted map[string]float64
	Splits    float64
	// FetchErrors counts the fetch calls that ended with an error, per
	// upload.
	FetchErrors float64
	// Rounds is the rounds per node and minute of the whole run, and Idle
	// the bytes of the streams opened after the last upload converged (the
	// tail: Status heartbeats, rounds that find nothing) per node and minute
	// of the tail.
	Rounds, Idle float64
	// Duplicates counts duplicate rangesync deliveries, and RateExceeded the
	// Statuses and sessions over a peer's rate on the clock.
	Duplicates, RateExceeded int
}

// g6FiguresOf returns the figures of res, a run with options o.
func g6FiguresOf(o simOptions, res *simResult) g6Figures {
	o = o.withDefaults()
	uploads := float64(o.Batches * o.BatchSize)
	from := res.Batches[0].At
	if o.Lag > 0 {
		from = 0
	}
	var metaIn, mores float64
	for _, n := range res.Nodes {
		metaIn += n.MetadataIn
		mores += n.MoreRequests
	}
	spd, _ := res.symbolsPerDifference(from)
	exact := res.exactDiffsFrom(from)
	zero := 0
	for _, d := range exact {
		if d == 0 {
			zero++
		}
	}
	var wants []float64
	catchUp := 0
	for _, s := range res.PairwiseDiff {
		if s.At >= from {
			wants = append(wants, float64(s.Wants))
			if s.Wants >= rangesync.CatchUpMinDiff {
				catchUp++
			}
		}
	}
	fetchErrors := 0
	for _, sp := range res.Spans {
		if sp.Kind == "fetch" && sp.Err != nil && sp.At >= from {
			fetchErrors++
		}
	}
	var trips float64
	sessions := 0
	if o.Delay > 0 {
		for _, sp := range res.Spans {
			if sp.Kind == "reconcile" && sp.Err == nil && sp.At >= from {
				trips += float64(sp.Duration) / float64(2*o.Delay)
				sessions++
			}
		}
	}
	var conv, maxConv float64
	for _, b := range res.Batches {
		conv += b.Convergence.Seconds()
		maxConv = max(maxConv, b.Convergence.Seconds())
	}
	status := float64(res.StreamBytes[rangesync.StatusStreamName])
	reconcile := float64(res.StreamBytes[rangesync.ReconcileStreamName])
	f := g6Figures{
		Share: metaIn / uploads / srepPullsyncACB1(o.Nodes), Metadata: metaIn / uploads,
		Status: status / uploads, Reconcile: reconcile / uploads, Want: (metaIn - status - reconcile) / uploads,
		SymbolsPerDiff: spd, MoresPerUpload: mores / uploads,
		RoundTrips:  trips / float64(max(sessions, 1)),
		Sessions:    res.Sessions["ok"] / uploads,
		Refused:     (res.Sessions["busy"] + res.Sessions["session_limit"]) / uploads,
		Stale:       res.Sessions["stale"] / uploads,
		FetchErrors: float64(fetchErrors) / uploads,
		ZeroShare:   float64(zero) / float64(max(len(exact), 1)),
		BatchConv:   conv / float64(len(res.Batches)), MaxConv: maxConv,
		Diff: g6QuantilesOf(exact), DiffBoth: g6QuantilesOf(res.diffsFrom(from)),
		Wants: g6QuantilesOf(wants), CatchUp: catchUp,
		Exhausted:  make(map[string]float64),
		Duplicates: res.Duplicates, RateExceeded: res.RateExceeded,
	}
	for _, level := range []string{"root", "level1", "bucket", "deep"} { // streams_exhausted_total{level}
		f.Exhausted[level] = res.Counters["streams_exhausted_total{level="+level+"}"]
	}
	rounds, tail := 0, 0
	for _, nr := range res.Nodes {
		rounds += nr.Rounds
	}
	for _, b := range res.TailBytes {
		tail += b
	}
	f.Rounds = float64(rounds) / float64(o.Nodes) / res.End.Minutes()
	if d := res.End - o.Warmup - res.Converged; d > 0 {
		f.Idle = float64(tail) / float64(o.Nodes) / d.Minutes()
	}
	// every completed session opens with one Fingerprint (a Message with a
	// part per opened range: one, the root, in a round); the rest are the
	// children of split ranges
	f.Splits = max(0, res.Counters["parts_sent{type=fingerprint}"]-res.Sessions["ok"])
	return f
}

// exhausted returns the streams that exhausted their cache, all levels.
func (f g6Figures) exhausted() float64 {
	total := 0.0
	for _, v := range f.Exhausted {
		total += v
	}
	return total
}

// g6Label names the session constants the harness runs with.
func g6Label() string {
	return fmt.Sprintf("GrowthFactor %d, BatchFactor %d/%d, MinBatch %d, MaxInitiatedSessions %d",
		rangesync.GrowthFactor, rangesync.BatchFactorNum, rangesync.BatchFactorDen, rangesync.MinBatch, reconciler.MaxInitiatedSessions)
}

// TestNeighborhoodG6Figures runs a short instance of each kind of G6 run and
// checks what g6FiguresOf reads off it: the steady figures of a small
// neighborhood, the wants of each session side, and a lagging node's
// catch-up (simOptions.Lag) with its counters taken from the start.
func TestNeighborhoodG6Figures(t *testing.T) {
	t.Parallel()

	t.Run("steady", func(t *testing.T) {
		t.Parallel()
		o := simOptions{Nodes: 5, Replication: 3, Seed: 2, Batches: 3, BatchSize: 12, BatchInterval: 1300 * time.Millisecond, Delay: 20 * time.Millisecond}
		res := runNeighborhood(t, o)
		f := g6FiguresOf(o, res)
		t.Logf("%s: %+v", g6Label(), f)
		if f.Share <= 0 || f.SymbolsPerDiff < 1 || f.RoundTrips < 3 || f.Sessions <= 0 {
			t.Errorf("figures %+v", f)
		}
		if got := f.Status + f.Reconcile + f.Want; math.Abs(got-f.Metadata) > 1e-6*f.Metadata {
			t.Errorf("stream bytes %v per upload, metadata %v", got, f.Metadata)
		}
		if f.Duplicates != 0 || f.RateExceeded != 0 || f.exhausted() != 0 {
			t.Errorf("%d duplicates, %d over a rate, %v exhausted streams", f.Duplicates, f.RateExceeded, f.exhausted())
		}
		// a side wants at most the difference it reported, and every stored
		// chunk was a want of the side that fetched it
		stored, wanted := 0, 0
		for _, s := range res.PairwiseDiff {
			if s.Wants > int(s.Session.Difference) {
				t.Errorf("node %d at %v: %d wants, difference %d", s.Node, s.At, s.Wants, s.Session.Difference)
			}
			if s.At >= res.Batches[0].At {
				wanted += s.Wants
			}
		}
		for _, n := range res.Nodes {
			stored += n.Stored
		}
		if wanted < stored {
			t.Errorf("%d wants, %d chunks stored", wanted, stored)
		}
		if f.Wants.Max >= rangesync.CatchUpMinDiff || f.CatchUp != 0 {
			t.Errorf("wants %+v, %d catch-up sides at this load", f.Wants, f.CatchUp)
		}
	})
	t.Run("lag", func(t *testing.T) {
		t.Parallel()
		const lag = 300 // past CatchUpMinDiff, below the bootstrap ratio
		o := simOptions{Nodes: 4, Replication: 2, Seed: 3, Base: 1024, Lag: lag, LagNode: 1, Batches: 1, BatchSize: 4, Delay: 20 * time.Millisecond}
		res := runNeighborhood(t, o)
		f := g6FiguresOf(o, res)
		t.Logf("caught up at %v: %+v; counters %v", res.CaughtUp, f, res.Counters)
		if res.CaughtUp <= 0 || res.CaughtUp > o.withDefaults().Warmup {
			t.Errorf("caught up at %v, want within the warm-up %v", res.CaughtUp, o.withDefaults().Warmup)
		}
		n := res.Nodes[o.LagNode]
		if want := lag + o.BatchSize - n.Uploads; n.Stored != want || n.Deliveries != want {
			t.Errorf("lagging node stored %d, %d deliveries, want %d", n.Stored, n.Deliveries, want)
		}
		if f.CatchUp == 0 || f.Wants.Max < lag/2 {
			t.Errorf("wants %+v, %d catch-up sides, want a catch-up", f.Wants, f.CatchUp)
		}
		if res.Duplicates != 0 {
			t.Errorf("%d duplicate deliveries", res.Duplicates)
		}
		fp := "parts_sent{type=fingerprint}"
		if res.Counters[fp] < res.Sessions["ok"] {
			t.Errorf("%v Fingerprint parts sent, %v sessions", res.Counters[fp], res.Sessions["ok"])
		}
		// the catch-up ended before the upload: its part of the counters
		// holds its sessions' openings, and no more than all of them
		if got := res.CatchUpCounters[fp]; got == 0 || got > res.Counters[fp] {
			t.Errorf("%v Fingerprint parts sent before the upload, %v in all", got, res.Counters[fp])
		}
	})
}
