// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build sim

package rangesync_test

import (
	"bytes"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
)

// The G6 sweeps of the harness (T4.6 part 3), behind the sim build tag, one
// test per question; each logs markdown tables for the phase-4 Decisions log:
//
//	go test ./pkg/rangesync/ -tags=sim -run 'TestG6' -v -timeout 120m
//
// The harness runs the compiled constants, and every table names them
// (g6Label). G6's candidates were measured in scratch builds with the
// constant changed: GrowthFactor, BatchFactorNum and BatchFactorDen
// (FirstBatch reads them); the initiated-session cap is per node since T4.7
// part 2, and a run can fix it instead (simOptions.InitiatedSessions, for
// example 8, the constant before G6). Runs go one after another, each torn
// down before the next (a subtest each), so that no run starves another's
// goroutines (simOptions.Settle).

// g6Runs runs o(seed) for seeds 1 … seeds, each in a subtest named by name
// and the seed, and returns their figures and the wall time of all of them;
// none when -run filtered them out (the caller skips the row then).
func g6Runs(t *testing.T, name string, seeds int, o func(seed uint64) simOptions) ([]g6Figures, time.Duration) {
	t.Helper()
	start := time.Now()
	var fs []g6Figures
	for seed := range uint64(seeds) {
		opts := o(seed + 1)
		t.Run(fmt.Sprintf("%s/seed=%d", name, seed+1), func(t *testing.T) {
			fs = append(fs, g6FiguresOf(opts, runNeighborhood(t, opts)))
		})
	}
	if t.Failed() {
		t.FailNow()
	}
	return fs, time.Since(start)
}

// g6Share and the other getters read one figure.
var (
	g6Share        = func(f g6Figures) float64 { return f.Share }
	g6SPD          = func(f g6Figures) float64 { return f.SymbolsPerDiff }
	g6Mores        = func(f g6Figures) float64 { return f.MoresPerUpload }
	g6Trips        = func(f g6Figures) float64 { return f.RoundTrips }
	g6Conv         = func(f g6Figures) float64 { return f.BatchConv }
	g6MaxConv      = func(f g6Figures) float64 { return f.MaxConv }
	g6Metadata     = func(f g6Figures) float64 { return f.Metadata }
	g6DiffP50      = func(f g6Figures) float64 { return f.Diff.P50 }
	g6DiffP99      = func(f g6Figures) float64 { return f.Diff.P99 }
	g6DiffMax      = func(f g6Figures) float64 { return f.Diff.Max }
	g6BothP99      = func(f g6Figures) float64 { return f.DiffBoth.P99 }
	g6WantsP50     = func(f g6Figures) float64 { return f.Wants.P50 }
	g6WantsP99     = func(f g6Figures) float64 { return f.Wants.P99 }
	g6WantsMax     = func(f g6Figures) float64 { return f.Wants.Max }
	g6CatchUp      = func(f g6Figures) float64 { return float64(f.CatchUp) }
	g6Refused      = func(f g6Figures) float64 { return f.Refused }
	g6Busy         = func(f g6Figures) float64 { return f.Busy }
	g6SessionLimit = func(f g6Figures) float64 { return f.SessionLimit }
	g6ServedBusy   = func(f g6Figures) float64 { return f.ServedBusy }
	g6Stale        = func(f g6Figures) float64 { return f.Stale }
	g6Zero         = func(f g6Figures) float64 { return f.ZeroShare }
	g6Sessions     = func(f g6Figures) float64 { return f.Sessions }
	g6Exhausted    = func(f g6Figures) float64 { return f.exhausted() }
)

// g6Steady returns the steady mode (steadyOptions: 24 batches 1.3 s apart)
// at rate uploads per second.
func g6Steady(nodes int, seed uint64, rate float64) simOptions {
	o := steadyOptions(nodes, seed)
	o.BatchSize = int(rate*o.BatchInterval.Seconds() + 0.5)
	return o
}

// TestG6Constants measures the session constants the harness is built with
// (Q3, BatchFactor, AC-B1) at the steady load, ~15 uploads per second, for
// N = 9 and 17: Delay 0, 50, 100 and 200 ms with hint(R), and 50 and 200 ms
// without, 6 seeds each. The initiated sessions are capped per node (the
// compiled rule, "per node"); at N = 17 every row also runs with the
// constant cap of 8 before G6 ("8", the control: G6's GrowthFactor 2 rows).
// At N = 9 the two caps run the same sessions: no node has more than 8
// higher-overlay peers, and the per-node cap is at least 8. The share is of
// AC-B1's baseline, N(N − 1) × 102 B per upload; "wire" divides by
// pullsync's bytes on the wire instead (srepPullsyncWire, 1.09× at this
// load). Busy and session-limit answers are the opening side's (reconciler
// sessions_total), Busy given the serving side's (session_busy_total).
func TestG6Constants(t *testing.T) {
	const seeds = 6
	tbl := &g6Table{header: []string{"N", "cap", "Delay", "hint", "share %", "wire %", "symbols/diff", "Mores/upload", "round trips/session", "convergence mean, max s", "n_Δ p50, p99, max", "wants p99, max", "Busy, session limit, Busy given /upload", "stale /upload", "zero %", "wall"}}
	for _, n := range []int{9, 17} {
		caps := []int{0}
		if n == 17 {
			caps = append(caps, 8)
		}
		for _, slots := range caps {
			for _, row := range []struct {
				delay   time.Duration
				noHints bool
			}{{0, false}, {50 * time.Millisecond, false}, {100 * time.Millisecond, false}, {200 * time.Millisecond, false}, {50 * time.Millisecond, true}, {200 * time.Millisecond, true}} {
				hint := "on"
				if row.noHints {
					hint = "off"
				}
				capName := "per node"
				if slots > 0 {
					capName = fmt.Sprint(slots)
				}
				name := fmt.Sprintf("N=%d/cap=%s/Delay=%v/hint=%s", n, capName, row.delay, hint)
				fs, wall := g6Runs(t, name, seeds, func(seed uint64) simOptions {
					o := steadyOptions(n, seed)
					o.Delay, o.NoHints, o.InitiatedSessions = row.delay, row.noHints, slots
					return o
				})
				if len(fs) == 0 {
					continue
				}
				o := steadyOptions(n, 1)
				wire := srepPullsyncACB1(n) / srepPullsyncWire(n, float64(o.BatchSize)/o.BatchInterval.Seconds())
				share := g6SpreadOf(fs, g6Share)
				tbl.add(fmt.Sprint(n), capName, row.delay.String(), hint,
					share.format(100, 1), fmt.Sprintf("%.1f", 100*share.mean*wire),
					g6SpreadOf(fs, g6SPD).short(1, 2), g6SpreadOf(fs, g6Mores).short(1, 2), g6SpreadOf(fs, g6Trips).short(1, 2),
					fmt.Sprintf("%.2f, %.2f", g6SpreadOf(fs, g6Conv).mean, g6SpreadOf(fs, g6MaxConv).hi),
					fmt.Sprintf("%.0f, %.0f, %.0f", g6SpreadOf(fs, g6DiffP50).mean, g6SpreadOf(fs, g6DiffP99).mean, g6SpreadOf(fs, g6DiffMax).hi),
					fmt.Sprintf("%.0f, %.0f", g6SpreadOf(fs, g6WantsP99).mean, g6SpreadOf(fs, g6WantsMax).hi),
					fmt.Sprintf("%.3f, %.3f, %.3f", g6SpreadOf(fs, g6Busy).mean, g6SpreadOf(fs, g6SessionLimit).mean, g6SpreadOf(fs, g6ServedBusy).mean),
					fmt.Sprintf("%.3f", g6SpreadOf(fs, g6Stale).mean),
					g6SpreadOf(fs, g6Zero).short(100, 1), wall.Round(time.Second).String())
				for _, f := range fs {
					if f.Duplicates != 0 || f.RateExceeded != 0 || f.exhausted() != 0 {
						t.Errorf("%s: %d duplicates, %d over a rate, %v exhausted", name, f.Duplicates, f.RateExceeded, f.exhausted())
					}
				}
			}
		}
	}
	t.Logf("%s; steady load, %d seeds; mean ± sd (range):%s", g6Label(), seeds, tbl)
}

// TestG6InitiatedCap measures the per-node initiated-session cap
// (reconciler.InitiatedSessionCap) past N = 17, where a node's sessions, one
// per peer, no longer fit its MaxConcurrentSessions tokens: N = 17, 25 and
// 33 at the steady load (~15 uploads per second), Delay 50 ms, 3 seeds, the
// per-node cap against the constant cap of 8 before G6. Busy and
// session-limit answers are the opening side's, Busy given the serving
// side's; served min is the fewest completed sessions per lower-overlay peer
// that a node served (0 would be a node whose served sessions starved), and
// unpaired the pairs with no completed session (either side opening, which
// then converge through the others), both from the first upload.
func TestG6InitiatedCap(t *testing.T) {
	const seeds = 3
	tbl := &g6Table{header: []string{"N", "cap", "share %", "convergence mean, max s", "sessions/upload", "Busy, session limit, Busy given /upload", "stale /upload", "served min, unpaired pairs", "n_Δ p50, p99", "wants max", "wall"}}
	for _, n := range []int{17, 25, 33} {
		for _, slots := range []int{0, 8} {
			capName := "per node"
			if slots > 0 {
				capName = fmt.Sprint(slots)
			}
			name := fmt.Sprintf("N=%d/cap=%s", n, capName)
			fs, wall := g6Runs(t, name, seeds, func(seed uint64) simOptions {
				o := steadyOptions(n, seed)
				o.Delay, o.InitiatedSessions = 50*time.Millisecond, slots
				return o
			})
			if len(fs) == 0 {
				continue
			}
			tbl.add(fmt.Sprint(n), capName, g6SpreadOf(fs, g6Share).format(100, 1),
				fmt.Sprintf("%.2f, %.2f", g6SpreadOf(fs, g6Conv).mean, g6SpreadOf(fs, g6MaxConv).hi),
				g6SpreadOf(fs, g6Sessions).short(1, 2),
				fmt.Sprintf("%.3f, %.3f, %.3f", g6SpreadOf(fs, g6Busy).mean, g6SpreadOf(fs, g6SessionLimit).mean, g6SpreadOf(fs, g6ServedBusy).mean),
				fmt.Sprintf("%.3f", g6SpreadOf(fs, g6Stale).mean),
				fmt.Sprintf("%.1f, %.0f", g6SpreadOf(fs, func(f g6Figures) float64 { return f.ServedMin }).lo, g6SpreadOf(fs, func(f g6Figures) float64 { return f.Unpaired }).hi),
				fmt.Sprintf("%.0f, %.0f", g6SpreadOf(fs, g6DiffP50).mean, g6SpreadOf(fs, g6DiffP99).mean),
				fmt.Sprintf("%.0f", g6SpreadOf(fs, g6WantsMax).hi),
				wall.Round(time.Second).String())
			for _, f := range fs {
				if f.Duplicates != 0 || f.RateExceeded != 0 || f.ServedMin == 0 {
					t.Errorf("%s: %d duplicates, %d over a rate, served min %v", name, f.Duplicates, f.RateExceeded, f.ServedMin)
				}
			}
		}
	}
	t.Logf("%s; steady load, Delay 50 ms, %d seeds; mean ± sd (range), served min the smallest of the seeds, unpaired the most:%s", g6Label(), seeds, tbl)
}

// TestG6Load measures the differences and the wants per session (Q9, Q2)
// at the reference load (~15 uploads per second) and heavier ones (50 and 100
// per second), N = 9 and 17, Delay 50 ms, 4 seeds; and at 50 per second with a
// reserve past ScanLimit (Base 5000), where the root stream ends at the root
// cache (RootSymbols) instead of going on along the scan path.
func TestG6Load(t *testing.T) {
	const seeds = 4
	tbl := &g6Table{header: []string{"N", "uploads/s", "Base", "n_Δ p50, p99, max (exact side)", "n_Δ p99 (both sides)", "wants p50, p99, max", "sides ≥ CatchUpMinDiff", "share %", "Mores/upload", "convergence mean, max s", "exhausted", "refused, stale /upload", "wall"}}
	for _, n := range []int{9, 17} {
		for _, row := range []struct {
			rate float64
			base int
		}{{15.4, 0}, {50, 0}, {100, 0}, {50, 5000}} {
			seeds := seeds
			if row.base > 0 {
				seeds = 2
			}
			name := fmt.Sprintf("N=%d/rate=%v/base=%d", n, row.rate, row.base)
			fs, wall := g6Runs(t, name, seeds, func(seed uint64) simOptions {
				o := g6Steady(n, seed, row.rate)
				o.Delay, o.Base = 50*time.Millisecond, row.base
				return o
			})
			if len(fs) == 0 {
				continue
			}
			base := row.base
			if base == 0 {
				base = simOptions{}.withDefaults().Base
			}
			tbl.add(fmt.Sprint(n), fmt.Sprint(row.rate), fmt.Sprint(base),
				fmt.Sprintf("%.0f, %.0f, %.0f", g6SpreadOf(fs, g6DiffP50).mean, g6SpreadOf(fs, g6DiffP99).mean, g6SpreadOf(fs, g6DiffMax).hi),
				g6SpreadOf(fs, g6BothP99).short(1, 0),
				fmt.Sprintf("%.0f, %s, %.0f", g6SpreadOf(fs, g6WantsP50).mean, g6SpreadOf(fs, g6WantsP99).short(1, 0), g6SpreadOf(fs, g6WantsMax).hi),
				fmt.Sprintf("%.0f", g6SpreadOf(fs, g6CatchUp).hi),
				g6SpreadOf(fs, g6Share).format(100, 1), g6SpreadOf(fs, g6Mores).short(1, 2),
				fmt.Sprintf("%.2f, %.2f", g6SpreadOf(fs, g6Conv).mean, g6SpreadOf(fs, g6MaxConv).hi),
				fmt.Sprintf("%.0f", g6SpreadOf(fs, g6Exhausted).hi),
				fmt.Sprintf("%.3f, %.3f", g6SpreadOf(fs, g6Refused).mean, g6SpreadOf(fs, g6Stale).mean),
				wall.Round(time.Second).String())
		}
	}
	t.Logf("%s; Delay 50 ms, %d seeds (2 with Base 5000); n_Δ per completed session, wants per session side (pairwise_diff_half's value); a quantile column is the mean over the seeds, a max the largest:%s", g6Label(), seeds, tbl)
}

// TestG6RoundInterval sweeps the bounds of the adaptive round interval
// (reconciler.Options MinRoundInterval and MaxRoundInterval) at N = 9 and 17,
// Delay 50 ms, 3 seeds, under two loads: the steady one (~15 uploads per
// second), and a sparse one (6 batches of 8 uploads 47 s apart, so that the
// interval grows back between them) with a 10-minute quiet tail, whose bytes
// (Status heartbeats, rounds that find nothing) are the idle cost. A
// minimum below 5 s is rate-limited in production (PeerSessionInterval and
// the session backoff stay tied to rangesync.MinRoundInterval): its row
// counts the sessions a peer would refuse (RateExceeded).
func TestG6RoundInterval(t *testing.T) {
	const seeds = 3
	tbl := &g6Table{header: []string{"N", "load", "min, max", "convergence mean, max s", "AC-V1 bound (2 × min) s", "share %", "symbols/diff", "Mores/upload", "n_Δ p50 per session", "metadata B/upload", "sessions/upload", "rounds per node-minute", "idle B per node-minute", "over a peer's rate", "wall"}}
	bounds := []struct{ lo, hi time.Duration }{
		{5 * time.Second, 30 * time.Second},
		{7500 * time.Millisecond, 30 * time.Second},
		{10 * time.Second, 30 * time.Second},
		{5 * time.Second, 15 * time.Second},
		{5 * time.Second, 60 * time.Second},
		{2500 * time.Millisecond, 30 * time.Second},
	}
	for _, n := range []int{9, 17} {
		for _, load := range []string{"steady", "sparse"} {
			for _, bd := range bounds {
				name := fmt.Sprintf("N=%d/%s/min=%v/max=%v", n, load, bd.lo, bd.hi)
				fs, wall := g6Runs(t, name, seeds, func(seed uint64) simOptions {
					o := steadyOptions(n, seed)
					if load == "sparse" {
						o = simOptions{Nodes: n, Replication: 3, Seed: seed, Batches: 6, BatchSize: 8, BatchInterval: 47 * time.Second, Tail: 10 * time.Minute}
					}
					o.Delay, o.MinRoundInterval, o.MaxRoundInterval = 50*time.Millisecond, bd.lo, bd.hi
					return o
				})
				if len(fs) == 0 {
					continue
				}
				idle := "–"
				if load == "sparse" {
					idle = g6SpreadOf(fs, func(f g6Figures) float64 { return f.Idle }).short(1, 0)
				}
				tbl.add(fmt.Sprint(n), load, fmt.Sprintf("%v, %v", bd.lo, bd.hi),
					fmt.Sprintf("%s, %.2f", g6SpreadOf(fs, g6Conv).short(1, 2), g6SpreadOf(fs, g6MaxConv).hi),
					fmt.Sprintf("%.0f", 2*bd.lo.Seconds()),
					g6SpreadOf(fs, g6Share).short(100, 1), g6SpreadOf(fs, g6SPD).short(1, 2), g6SpreadOf(fs, g6Mores).short(1, 2), fmt.Sprintf("%.0f", g6SpreadOf(fs, g6DiffP50).mean),
					g6SpreadOf(fs, g6Metadata).short(1, 0), g6SpreadOf(fs, g6Sessions).short(1, 2),
					g6SpreadOf(fs, func(f g6Figures) float64 { return f.Rounds }).short(1, 1), idle,
					fmt.Sprintf("%.0f", g6SpreadOf(fs, func(f g6Figures) float64 { return float64(f.RateExceeded) }).hi), wall.Round(time.Second).String())
			}
		}
	}
	t.Logf("%s; Delay 50 ms, %d seeds; mean (range); idle: the bytes of the streams opened after the last convergence, per node and minute of the tail; over a peer's rate: the most in a run:%s", g6Label(), seeds, tbl)
}

// TestG6Catchup measures what the caches do in a catch-up (Q2, churn): a
// node of N = 5 rejoins lacking Lag of the base chunks (simOptions.Lag),
// with a reserve past 16 × ScanLimit (Base 70000) so that both the root
// stream (RootSymbols) and the level-1 streams (Level1Symbols) end at their
// caches; with the harness's 8 leaf bits a bucket holds ~270 elements, on
// the scan path. Delay 50 ms, 2 seeds. The catch-up ends long before the 4
// uploads at Warmup; what follows them is the first sessions of each pair
// after it ("after"), whose first batches read the catch-up's hint(R).
func TestG6Catchup(t *testing.T) {
	const (
		seeds = 2
		base  = 70000
	)
	tbl := &g6Table{header: []string{"Lag", "caught up s", "catch-up sessions", "round trips of the longest", "split children (Fingerprint parts past the openings): catch-up, after", "exhausted root, level1, bucket, deep: catch-up, after", "symbols: catch-up, after", "symbols per missing chunk (catch-up)", "metadata KB", "wall"}}
	for _, lag := range []int{300, 1000, 2500, 4000, 6000, 12000} {
		type run struct {
			caught, longest, sessions       float64
			splits, splitsAfter             float64
			ex, exAfter                     [4]float64
			symbols, symbolsAfter, metadata float64
		}
		var runs []run
		start := time.Now()
		for seed := range uint64(seeds) {
			o := simOptions{Nodes: 5, Replication: 3, Seed: seed + 1, Base: base, Lag: lag, Batches: 1, BatchSize: 4, Delay: 50 * time.Millisecond}
			t.Run(fmt.Sprintf("lag=%d/seed=%d", lag, seed+1), func(t *testing.T) {
				res := runNeighborhood(t, o)
				warmup := o.withDefaults().Warmup
				r := run{caught: res.CaughtUp.Seconds()}
				if res.CaughtUp > warmup {
					t.Errorf("caught up at %v, after the uploads at %v", res.CaughtUp, warmup)
				}
				for _, sp := range res.Spans {
					if sp.Kind == "reconcile" && sp.Err == nil && sp.At < warmup {
						r.longest = max(r.longest, float64(sp.Duration)/float64(2*o.Delay))
						r.sessions++
					}
				}
				for _, s := range res.PairwiseDiff {
					if s.At < warmup {
						r.symbols += float64(s.Session.SymbolsReceived)
					} else {
						r.symbolsAfter += float64(s.Session.SymbolsReceived)
					}
				}
				fp := "parts_sent{type=fingerprint}"
				r.splits = res.CatchUpCounters[fp] - r.sessions
				r.splitsAfter = res.Counters[fp] - res.CatchUpCounters[fp] - (res.Sessions["ok"] - r.sessions)
				for i, level := range []string{"root", "level1", "bucket", "deep"} {
					k := "streams_exhausted_total{level=" + level + "}"
					r.ex[i], r.exAfter[i] = res.CatchUpCounters[k], res.Counters[k]-res.CatchUpCounters[k]
				}
				for _, n := range res.Nodes {
					r.metadata += n.MetadataIn
				}
				runs = append(runs, r)
			})
		}
		if t.Failed() {
			t.FailNow()
		}
		if len(runs) == 0 {
			continue
		}
		get := func(f func(run) float64) g6Spread { return g6SpreadOf(runs, f) }
		ex := func(e func(run) [4]float64) string {
			return fmt.Sprintf("%.0f, %.0f, %.0f, %.0f", get(func(r run) float64 { return e(r)[0] }).mean, get(func(r run) float64 { return e(r)[1] }).mean,
				get(func(r run) float64 { return e(r)[2] }).mean, get(func(r run) float64 { return e(r)[3] }).mean)
		}
		tbl.add(fmt.Sprint(lag), get(func(r run) float64 { return r.caught }).short(1, 2),
			get(func(r run) float64 { return r.sessions }).short(1, 0), get(func(r run) float64 { return r.longest }).short(1, 0),
			fmt.Sprintf("%.0f, %.0f", get(func(r run) float64 { return r.splits }).mean, get(func(r run) float64 { return r.splitsAfter }).mean),
			ex(func(r run) [4]float64 { return r.ex })+"; "+ex(func(r run) [4]float64 { return r.exAfter }),
			fmt.Sprintf("%.0f, %.0f", get(func(r run) float64 { return r.symbols }).mean, get(func(r run) float64 { return r.symbolsAfter }).mean),
			fmt.Sprintf("%.2f", get(func(r run) float64 { return r.symbols }).mean/float64(lag)),
			get(func(r run) float64 { return r.metadata / 1000 }).short(1, 0), time.Since(start).Round(time.Second).String())
	}
	t.Logf("%s; N = 5, Base %d, Delay 50 ms, %d seeds; counted from the start; catch-up: before the uploads, after: from them on; mean (range):%s", g6Label(), base, seeds, tbl)
}

// TestG6Bootstrap measures an empty node's bootstrap in a neighborhood of
// N = 17 (the T4.3–T4.5 follow-ups to T4.6: the stripe/round split, Busy-heavy
// stripes, deferred stripes): node 0 starts without any of the 20000 base
// chunks (simOptions.Lag = Base), so it transfers its root striped (16
// stripes, MaxStripeSessions = 4 at a time) while its peers' rounds run
// sessions with it. The node-wide fetch limit is lifted (FetchRate), since the
// harness cannot time it; the holders' delivery limiter (250 chunks per second
// per peer, on the real clock) stays. Delay 50 ms, 2 seeds; node 0's rank is
// the number of its peers with a lower overlay (the initiators of its pairs).
func TestG6Bootstrap(t *testing.T) {
	const (
		seeds = 2
		base  = 20000
	)
	tbl := &g6Table{header: []string{"seed", "rank", "caught up s", "stripes done, reassigned, timeout", "stripe waves (session starts of node 0)", "sessions ok, busy, session_limit, stale, error", "Busy answers", "fetches skipped", "node 0 metadata in KB (AC-B2 bound)", "duplicates", "wall"}}
	for seed := range uint64(seeds) {
		o := simOptions{
			Nodes: 17, Replication: 3, Seed: seed + 1, Base: base, Lag: base, Batches: 1, BatchSize: 4, Delay: 50 * time.Millisecond,
			Service: func(so *rangesync.Options) { so.FetchRate = 1 << 20 },
		}
		t.Run(fmt.Sprintf("seed=%d", seed+1), func(t *testing.T) {
			start := time.Now()
			res := runNeighborhood(t, o)
			rank := 0
			for _, n := range res.Nodes[1:] {
				if bytes.Compare(n.Addr.Bytes(), res.Nodes[0].Addr.Bytes()) < 0 {
					rank++
				}
			}
			var starts []time.Duration
			for _, sp := range res.Spans {
				if sp.Node == 0 && sp.Kind == "reconcile" {
					starts = append(starts, sp.At)
				}
			}
			slices.Sort(starts)
			waves := 0
			for i, at := range starts {
				if i == 0 || at-starts[i-1] > 2*o.Delay {
					waves++
				}
			}
			var skipped []string
			for k, v := range res.Counters {
				if strings.HasPrefix(k, "fetch_skipped_total") && v > 0 {
					skipped = append(skipped, fmt.Sprintf("%s %.0f", strings.TrimPrefix(k, "fetch_skipped_total"), v))
				}
			}
			slices.Sort(skipped)
			n0 := res.Nodes[0]
			tbl.add(fmt.Sprint(seed+1), fmt.Sprint(rank), fmt.Sprintf("%.2f", res.CaughtUp.Seconds()),
				fmt.Sprintf("%.0f, %.0f, %.0f", res.Counters["stripes_total{result=done}"], res.Counters["stripes_total{result=reassigned}"], res.Counters["stripes_total{result=timeout}"]),
				fmt.Sprintf("%d sessions in %d waves", len(starts), waves),
				fmt.Sprintf("%.0f, %.0f, %.0f, %.0f, %.0f", res.Sessions["ok"], res.Sessions["busy"], res.Sessions["session_limit"], res.Sessions["stale"], res.Sessions["error"]),
				fmt.Sprintf("%.0f", res.Counters["session_busy_total{}"]), strings.Join(skipped, ", "),
				fmt.Sprintf("%.0f (%.0f)", n0.MetadataIn/1000, (1.15*base*70+1e6)/1000),
				fmt.Sprint(res.Duplicates), time.Since(start).Round(time.Second).String())
			t.Logf("node 0 want claims %v; counters %v", n0.WantClaims, res.Counters)
		})
	}
	t.Logf("%s; N = 17, Base %d, node 0 empty, Delay 50 ms; counted from the start:%s", g6Label(), base, tbl)
}

// TestG6BootstrapPeers measures, per peer, the metadata an empty node
// receives in a bootstrap past LargeDiff (the T4.6 part 3 review, AC-B2):
// node 0 of N = 5 starts without any of the Base base chunks (70000 and
// 100000, so its root difference is past LargeDiff = 65536, as TestG6Catchup's
// reserve), and stripes its root while its peers' rounds run sessions with it.
// The fetch limit is lifted as in TestG6Bootstrap; Delay 50 ms; seeds 1–4, for
// node 0's rank (its peers with a lower overlay, which initiate their pairs
// with it). Per peer: the bytes node 0 read on the reconcile streams it opened
// (its stripes, and also its verification and post-bootstrap sessions) and on
// those the peer opened (rounds, and the takeovers of the higher-overlay
// peers, which fire once the transfer outlasts their wait: at Base 100000,
// not at 70000), with the IDs node 0 wanted from the latter. The stripes are
// below LargeDiff here (17500–25000 IDs), so this does not exercise the
// production geometry of stripes past it (T4.6 part 3 fix round 2, AC-B2).
func TestG6BootstrapPeers(t *testing.T) {
	const seeds = 4
	tbl := &g6Table{header: []string{"Base", "seed", "rank", "caught up s", "node 0 metadata in KB (AC-B2 bound)", "of it: node 0's sessions (stripes and later), peers' sessions, Status KB", "peers' sessions: KB, sessions, wants (lower-overlay peers first)", "stripes done", "duplicates", "wall"}}
	for _, base := range []int{70000, 100000} {
		for seed := range uint64(seeds) {
			o := simOptions{
				Nodes: 5, Replication: 3, Seed: seed + 1, Base: base, Lag: base, Batches: 1, BatchSize: 4, Delay: 50 * time.Millisecond,
				Service: func(so *rangesync.Options) { so.FetchRate = 1 << 20 },
			}
			t.Run(fmt.Sprintf("base=%d/seed=%d", base, seed+1), func(t *testing.T) {
				start := time.Now()
				res := runNeighborhood(t, o)
				n0 := res.Nodes[0]
				type peer struct {
					i, lower           int // lower: 1 for a peer with a lower overlay
					stripes, sessions  int // bytes node 0 read
					opened, wants, max int
				}
				peers := make([]peer, 0, len(res.Nodes)-1)
				for i, n := range res.Nodes[1:] {
					p := peer{i: i + 1}
					if bytes.Compare(n.Addr.Bytes(), n0.Addr.Bytes()) < 0 {
						p.lower = 1
					}
					p.stripes = res.Flows[simFlow{From: p.i, To: 0, Opener: 0, Stream: rangesync.ReconcileStreamName}]
					p.sessions = res.Flows[simFlow{From: p.i, To: 0, Opener: p.i, Stream: rangesync.ReconcileStreamName}]
					for _, sp := range res.Spans {
						if sp.Kind == "reconcile" && sp.Node == p.i && sp.Peer.Equal(n0.Addr) && sp.Err == nil {
							p.opened++
						}
					}
					for _, s := range res.PairwiseDiff {
						if s.Node == 0 && !s.Session.Initiator && s.Session.Peer.Equal(n.Addr) {
							p.wants += s.Wants
							p.max = max(p.max, s.Wants)
						}
					}
					peers = append(peers, p)
				}
				slices.SortStableFunc(peers, func(a, b peer) int { return b.lower - a.lower })
				rank, stripes, sessions, status := 0, 0, 0, 0
				cells := make([]string, 0, len(peers))
				for _, p := range peers {
					rank += p.lower
					stripes += p.stripes
					sessions += p.sessions
					status += res.Flows[simFlow{From: p.i, To: 0, Opener: p.i, Stream: rangesync.StatusStreamName}] +
						res.Flows[simFlow{From: p.i, To: 0, Opener: 0, Stream: rangesync.StatusStreamName}]
					cells = append(cells, fmt.Sprintf("%.0f, %d, %d (max %d)", float64(p.sessions)/1000, p.opened, p.wants, p.max))
				}
				tbl.add(fmt.Sprint(base), fmt.Sprint(seed+1), fmt.Sprint(rank), fmt.Sprintf("%.2f", res.CaughtUp.Seconds()),
					fmt.Sprintf("%.0f (%.0f)", n0.MetadataIn/1000, (1.15*float64(base)*70+1e6)/1000),
					fmt.Sprintf("%.0f, %.0f, %.0f", float64(stripes)/1000, float64(sessions)/1000, float64(status)/1000),
					strings.Join(cells, "; "),
					fmt.Sprintf("%.0f", res.Counters["stripes_total{result=done}"]),
					fmt.Sprint(res.Duplicates), time.Since(start).Round(time.Second).String())
				if res.Duplicates != 0 {
					t.Errorf("%d duplicate deliveries", res.Duplicates)
				}
				t.Logf("node 0 want claims %v; counters %v", n0.WantClaims, res.Counters)
			})
		}
	}
	t.Logf("%s; N = 5, node 0 empty, Delay 50 ms; counted from the start; bytes node 0 read on the reconcile streams (node 0's sessions: those it opened, its stripes with its verification and post-bootstrap sessions; peers' sessions: those the peer opened) and the Status streams:%s", g6Label(), tbl)
}

// TestG6EqualCounts measures one session over a two-sided difference with
// equal counts and no hint (the T4.6 part 3 review, GrowthFactor): A and B
// share ScanLimit + 1000 IDs (the root is served from its cache) and hold k
// IDs each that the other lacks, so d̂ = 1, the first batch is MinBatch and
// the stream grows by GrowthFactor per More up to RootSymbols. 5 seeds per
// k; the session's Messages with parts, the Mores and the bytes (the means),
// and how many root streams exhausted their cache (a split follows).
func TestG6EqualCounts(t *testing.T) {
	const seeds = 5
	n := fingerprint.ScanLimit + 1000
	root := rangeOf("")
	tbl := &g6Table{header: []string{"k per side (n_Δ = 2k)", "Messages with parts", "Mores", "KB", "root streams exhausted"}}
	for _, k := range []int{13, 50, 200, 500, 1000, 1500} {
		var msgs, mores, kb, exhausted float64
		for seed := range seeds {
			rng := newRand(uint64(seed))
			ex := runExample(t, randIDs(rng, root, n), randIDs(rng, root, k), randIDs(rng, root, k))
			msgs += float64(ex.messages) / seeds
			mores += float64(ex.a.Stats().MoreRequests+ex.b.Stats().MoreRequests) / seeds
			kb += float64(ex.bytes) / 1000 / seeds
			exhausted += float64(ex.a.Stats().ExhaustedByLevel[0] + ex.b.Stats().ExhaustedByLevel[0]) // the root's
		}
		tbl.add(fmt.Sprintf("%d (%d)", k, 2*k), fmt.Sprintf("%.1f", msgs), fmt.Sprintf("%.1f", mores), fmt.Sprintf("%.1f", kb), fmt.Sprintf("%.0f in %d sessions", exhausted, seeds))
	}
	t.Logf("%s; one session, equal counts, no hint, %d seeds; mean:%s", g6Label(), seeds, tbl)
}

// TestG6FetchOpenFails measures the cost of a peer that fails every fetch
// open (the T4.5 part 2c follow-up "A peer that fails every fetch open keeps
// its sessions"): node 0 of N = 9 refuses every fetch stream
// (simOptions.FetchOpenFails) but runs its sessions, at the steady load with
// Delay 50 ms, against the same seeds without it; 3 seeds.
func TestG6FetchOpenFails(t *testing.T) {
	const seeds = 3
	tbl := &g6Table{header: []string{"node 0", "convergence mean, max s", "share %", "sessions/upload", "fetches failed /upload", "duplicates"}}
	for _, fails := range []bool{false, true} {
		name := "serves"
		if fails {
			name = "fails every fetch open"
		}
		fs, _ := g6Runs(t, name, seeds, func(seed uint64) simOptions {
			o := steadyOptions(9, seed)
			o.Delay = 50 * time.Millisecond
			if fails {
				o.FetchOpenFails = []int{0}
			}
			return o
		})
		if len(fs) == 0 {
			continue
		}
		tbl.add(name, fmt.Sprintf("%s, %.2f", g6SpreadOf(fs, g6Conv).short(1, 2), g6SpreadOf(fs, g6MaxConv).hi),
			g6SpreadOf(fs, g6Share).short(100, 1), g6SpreadOf(fs, g6Sessions).short(1, 2),
			g6SpreadOf(fs, func(f g6Figures) float64 { return f.FetchErrors }).short(1, 2),
			fmt.Sprintf("%.0f", g6SpreadOf(fs, func(f g6Figures) float64 { return float64(f.Duplicates) }).hi))
	}
	t.Logf("%s; N = 9, steady load, Delay 50 ms, %d seeds; mean (range):%s", g6Label(), seeds, tbl)
}

// g6Spread is the mean, the standard deviation and the range of one figure
// over runs.
type g6Spread struct{ mean, sd, lo, hi float64 }

func g6SpreadOf[F any](fs []F, get func(F) float64) g6Spread {
	if len(fs) == 0 {
		return g6Spread{}
	}
	xs := make([]float64, len(fs))
	for i, f := range fs {
		xs[i] = get(f)
	}
	s := g6Spread{lo: slices.Min(xs), hi: slices.Max(xs)}
	for _, x := range xs {
		s.mean += x / float64(len(xs))
	}
	if len(xs) > 1 {
		for _, x := range xs {
			s.sd += (x - s.mean) * (x - s.mean)
		}
		s.sd = math.Sqrt(s.sd / float64(len(xs)-1))
	}
	return s
}

// String formats the spread as "mean ± sd (lo–hi)".
func (s g6Spread) format(scale float64, prec int) string {
	return fmt.Sprintf("%.*f ± %.*f (%.*f–%.*f)", prec, s.mean*scale, prec, s.sd*scale, prec, s.lo*scale, prec, s.hi*scale)
}

// short formats the spread as "mean (lo–hi)".
func (s g6Spread) short(scale float64, prec int) string {
	return fmt.Sprintf("%.*f (%.*f–%.*f)", prec, s.mean*scale, prec, s.lo*scale, prec, s.hi*scale)
}

// g6Table is a markdown table of rows, logged once complete.
type g6Table struct {
	header []string
	rows   [][]string
}

func (tb *g6Table) add(cells ...string) { tb.rows = append(tb.rows, cells) }

func (tb *g6Table) String() string {
	var b strings.Builder
	b.WriteString("\n| " + strings.Join(tb.header, " | ") + " |\n|")
	for range tb.header {
		b.WriteString("---|")
	}
	for _, r := range tb.rows {
		b.WriteString("\n| " + strings.Join(r, " | ") + " |")
	}
	return b.String()
}
