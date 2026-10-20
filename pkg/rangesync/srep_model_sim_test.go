// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build sim

package rangesync_test

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
)

// The heavy runs of the analytical model (srep_model_test.go), behind the
// sim build tag:
//
//	go test ./pkg/rangesync/ -tags=sim -run 'TestSREPModel(VsHarness|G6)' -v -timeout 30m
//
// TestSREPModelVsHarness validates the model against the harness
// (runNeighborhood) in its steady mode, and TestSREPModelG6 sweeps the model
// over G6's candidate constants, the hint, N, p, the upload rate and Delay.
//
// The harness runs with the compiled constants. To measure a G6 candidate
// with it, change GrowthFactor, or BatchFactorNum and BatchFactorDen (which
// FirstBatch reads), in a scratch copy and run TestSREPModelVsHarness there:
// the model follows the compiled constants (srepCompiled).

// srepSimDraws is the number of decoder draws per difference size in the
// tagged runs.
const srepSimDraws = 1000

// srepCompiled returns the model's parameters for the compiled constants.
func srepCompiled(p srepParams) srepParams {
	p.BatchFactor = srepFactor{Num: rangesync.BatchFactorNum, Den: rangesync.BatchFactorDen}
	p.GrowthFactor = rangesync.GrowthFactor
	p.MinBatch = rangesync.MinBatch
	return p
}

// srepLabel names the session constants of p.
func srepLabel(p srepParams) string {
	return fmt.Sprintf("GrowthFactor %d, BatchFactor %v, MinBatch %d", p.GrowthFactor, p.BatchFactor, p.MinBatch)
}

// srepFigures are the figures the model and the harness are compared on,
// per upload unless named otherwise.
type srepFigures struct {
	Share, Metadata         float64 // share of AC-B1's pullsync, bytes
	Status, Reconcile, Want float64 // bytes
	SymbolsPerDiff          float64 // per decoded difference
	DiffPerUpload, DiffP50  float64 // n_Δ per upload, and its median per session
	MoresPerUpload          float64
	SessionsPerUpload       float64 // completed sessions
	ZeroShare               float64 // share of sessions with no difference
	BatchConv, MaxConv      float64 // mean and longest convergence of a batch, seconds
	Duplicates              float64
}

func srepHarnessFigures(o simOptions, res *simResult) srepFigures {
	o = o.withDefaults()
	uploads := float64(o.Batches * o.BatchSize)
	var metaIn, mores float64
	for _, n := range res.Nodes {
		metaIn += n.MetadataIn
		mores += n.MoreRequests
	}
	from := res.Batches[0].At
	spd, _ := res.symbolsPerDifference(from)
	exact := res.exactDiffsFrom(from)
	sum, zero := 0.0, 0
	for _, d := range exact {
		sum += d
		if d == 0 {
			zero++
		}
	}
	status := float64(res.StreamBytes[rangesync.StatusStreamName]) / uploads
	reconcile := float64(res.StreamBytes[rangesync.ReconcileStreamName]) / uploads
	var conv, maxConv float64
	for _, b := range res.Batches {
		conv += b.Convergence.Seconds()
		maxConv = max(maxConv, b.Convergence.Seconds())
	}
	return srepFigures{
		Share: metaIn / uploads / srepPullsyncACB1(o.Nodes), Metadata: metaIn / uploads,
		Status: status, Reconcile: reconcile, Want: metaIn/uploads - status - reconcile,
		SymbolsPerDiff: spd, DiffPerUpload: sum / uploads, DiffP50: quantile(exact, 0.5),
		MoresPerUpload: mores / uploads, SessionsPerUpload: float64(len(exact)) / uploads,
		ZeroShare: float64(zero) / float64(max(len(exact), 1)),
		BatchConv: conv / float64(len(res.Batches)), MaxConv: maxConv,
		Duplicates: float64(res.Duplicates) / uploads,
	}
}

func srepModelFigures(r *srepResult) srepFigures {
	uploads := float64(r.Uploads)
	reconcile := 0.0
	for _, k := range []int{srepFPBytes, srepSymbolBytes, srepMoreBytes, srepItemBytes, srepEmptyBytes} {
		reconcile += r.Bytes[k]
	}
	zero := 0
	for _, d := range r.Diffs {
		if d == 0 {
			zero++
		}
	}
	var conv, maxConv float64
	for _, b := range r.Batches {
		conv += b.Convergence.Seconds()
		maxConv = max(maxConv, b.Convergence.Seconds())
	}
	return srepFigures{
		Share: r.Share(), Metadata: r.MetadataPerUpload(),
		Status: r.Bytes[srepStatusBytes] / uploads, Reconcile: reconcile / uploads, Want: r.Bytes[srepWantBytes] / uploads,
		SymbolsPerDiff: r.SymbolsPerDifference(), DiffPerUpload: r.DiffPerUpload(), DiffP50: r.DiffQuantile(0.5),
		MoresPerUpload: r.MoresPerUpload(), SessionsPerUpload: float64(r.Sessions) / uploads,
		ZeroShare: float64(zero) / float64(max(len(r.Diffs), 1)),
		BatchConv: conv / float64(len(r.Batches)), MaxConv: maxConv,
		Duplicates: float64(r.Duplicates) / uploads,
	}
}

// srepSpread is the mean, minimum and maximum of one figure over runs.
type srepSpread struct{ mean, lo, hi float64 }

func srepSpreadOf(fs []srepFigures, get func(srepFigures) float64) srepSpread {
	xs := make([]float64, len(fs))
	sum := 0.0
	for i, f := range fs {
		xs[i] = get(f)
		sum += xs[i]
	}
	return srepSpread{mean: sum / float64(len(xs)), lo: slices.Min(xs), hi: slices.Max(xs)}
}

func (s srepSpread) format(scale float64, prec int) string {
	return fmt.Sprintf("%.*f (%.*f–%.*f)", prec, s.mean*scale, prec, s.lo*scale, prec, s.hi*scale)
}

// srepFigureNames are the figures a comparison reports, with their scale,
// precision and tolerance: the model's mean must lie within the tolerance
// of the harness's mean (relative, or absolute in the figure's unit when
// abs is set).
var srepFigureNames = []struct {
	name       string
	get        func(srepFigures) float64
	scale      float64
	prec       int
	tol        float64
	abs, check bool
}{
	{name: "share of pullsync %", get: func(f srepFigures) float64 { return f.Share }, scale: 100, prec: 1, tol: 0.10, check: true},
	{name: "symbols per difference", get: func(f srepFigures) float64 { return f.SymbolsPerDiff }, scale: 1, prec: 2, tol: 0.10, check: true},
	{name: "n_Δ per upload", get: func(f srepFigures) float64 { return f.DiffPerUpload }, scale: 1, prec: 1, tol: 0.10, check: true},
	{name: "n_Δ p50 per session", get: func(f srepFigures) float64 { return f.DiffP50 }, scale: 1, prec: 0, tol: 0.15, check: true},
	{name: "Mores per upload", get: func(f srepFigures) float64 { return f.MoresPerUpload }, scale: 1, prec: 2},
	{name: "status B", get: func(f srepFigures) float64 { return f.Status }, scale: 1, prec: 0},
	{name: "reconcile B", get: func(f srepFigures) float64 { return f.Reconcile }, scale: 1, prec: 0},
	{name: "want B", get: func(f srepFigures) float64 { return f.Want }, scale: 1, prec: 0},
	{name: "sessions per upload", get: func(f srepFigures) float64 { return f.SessionsPerUpload }, scale: 1, prec: 2},
	{name: "zero-difference share %", get: func(f srepFigures) float64 { return f.ZeroShare }, scale: 100, prec: 1},
	{name: "batch convergence s", get: func(f srepFigures) float64 { return f.BatchConv }, scale: 1, prec: 2, tol: 1, abs: true, check: true},
	{name: "longest batch convergence s", get: func(f srepFigures) float64 { return f.MaxConv }, scale: 1, prec: 2, tol: 1, abs: true, check: true},
	{name: "duplicates per upload", get: func(f srepFigures) float64 { return f.Duplicates }, scale: 1, prec: 2},
}

// TestSREPModelVsHarness runs the harness in its steady mode (steadyOptions)
// for N = 9 and 17, Delay 0, 50 and 200 ms with hint(R) and 50 ms without,
// three seeds each, and the model with the same options (ten seeds), and
// checks that the model's means lie within the stated tolerances of the
// harness's: 10% for the share of pullsync's metadata, the symbols per
// difference and n_Δ per upload, 15% for the median n_Δ per session, and
// one second for the batches' convergence. The other figures are logged.
// The harness runs one after another, so that no run starves another's
// goroutines (simOptions.Settle).
func TestSREPModelVsHarness(t *testing.T) {
	table := newSREPDecodeTable(srepSimDraws, 7)
	harnessSeeds := []uint64{1, 2, 3}
	const modelSeeds = 10
	for _, row := range []struct {
		n       int
		delay   time.Duration
		noHints bool
	}{
		{n: 9},
		{n: 9, delay: 50 * time.Millisecond},
		{n: 9, delay: 200 * time.Millisecond},
		{n: 9, delay: 50 * time.Millisecond, noHints: true},
		{n: 17},
		{n: 17, delay: 50 * time.Millisecond},
		{n: 17, delay: 200 * time.Millisecond},
		{n: 17, delay: 50 * time.Millisecond, noHints: true},
	} {
		hints := "hints"
		if row.noHints {
			hints = "no hints"
		}
		t.Run(fmt.Sprintf("N=%d/Delay=%v/%s", row.n, row.delay, hints), func(t *testing.T) {
			hs := make([]srepFigures, 0, len(harnessSeeds))
			ms := make([]srepFigures, 0, modelSeeds)
			var label string
			start := time.Now()
			for _, seed := range harnessSeeds {
				o := steadyOptions(row.n, seed)
				o.Delay, o.NoHints = row.delay, row.noHints
				hs = append(hs, srepHarnessFigures(o, runNeighborhood(t, o)))
			}
			wall := time.Since(start)
			for seed := range uint64(modelSeeds) {
				o := steadyOptions(row.n, seed+1)
				o.Delay, o.NoHints = row.delay, row.noHints
				p := srepCompiled(srepParamsOf(o))
				label = srepLabel(p)
				ms = append(ms, srepModelFigures(p.run(t, table)))
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%s; harness %d seeds (%v), model %d seeds; model | harness:", label, len(hs), wall.Round(time.Second), len(ms))
			for _, f := range srepFigureNames {
				m, h := srepSpreadOf(ms, f.get), srepSpreadOf(hs, f.get)
				fmt.Fprintf(&b, "\n  %s: %s | %s", f.name, m.format(f.scale, f.prec), h.format(f.scale, f.prec))
				if !f.check {
					continue
				}
				dev := math.Abs(m.mean - h.mean)
				if !f.abs {
					dev /= math.Max(h.mean, 1e-9)
				}
				if dev > f.tol {
					t.Errorf("%s: model %.3f, harness %.3f, outside the tolerance %v", f.name, m.mean*f.scale, h.mean*f.scale, f.tol)
				}
			}
			t.Log(b.String())
		})
	}
}

// srepConfigs are the session constants G6 decides between: "now" is the
// compiled constants.
var srepConfigs = []struct {
	name        string
	growth      int
	batchFactor srepFactor
}{
	{name: "now", growth: rangesync.GrowthFactor, batchFactor: srepFactor{Num: rangesync.BatchFactorNum, Den: rangesync.BatchFactorDen}},
	{name: "GrowthFactor 2", growth: 2, batchFactor: srepFactor{16, 10}},
	{name: "BatchFactor 2.0", growth: 4, batchFactor: srepFactor{2, 1}},
	{name: "both", growth: 2, batchFactor: srepFactor{2, 1}},
}

// srepSweep runs the model for seeds seeds of p (Seed 1 …) and returns
// their figures.
func srepSweep(t *testing.T, table *srepDecodeTable, p srepParams, seeds int) []srepFigures {
	t.Helper()
	fs := make([]srepFigures, 0, seeds)
	for seed := range uint64(seeds) {
		p.Seed = seed + 1
		fs = append(fs, srepModelFigures(p.run(t, table)))
	}
	return fs
}

// TestSREPModelG6 sweeps the model: G6's candidates with and without
// hint(R) at N = 9 and 17 and Delay 0 to 200 ms in the harness's steady
// mode, with the closed form of §7.8 next to them; then the upload rate
// (Poisson), production sizes (a large reserve, a root of radius 10), the
// initiated-session slots, p and MinBatch. It logs tables for the Decisions
// log.
func TestSREPModelG6(t *testing.T) {
	table := newSREPDecodeTable(srepSimDraws, 8)
	const seeds = 10
	delays := []time.Duration{0, 50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond}

	t.Run("candidates", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("| N | constants | hint | ideal (§7.8 closed form) | ")
		for _, d := range delays {
			fmt.Fprintf(&b, "Delay %v: share %%, symbols/diff, Mores/upload | ", d)
		}
		b.WriteString("\n")
		for _, n := range []int{9, 17} {
			for _, c := range srepConfigs {
				for _, noHints := range []bool{false, true} {
					base := srepParamsOf(steadyOptions(n, 1))
					base.GrowthFactor, base.BatchFactor, base.NoHints = c.growth, c.batchFactor, noHints
					id := base.ideal(t, table, base.uploadRate()*base.MinRoundInterval.Seconds())
					hint := "on"
					if noHints {
						hint = "off"
					}
					fmt.Fprintf(&b, "| %d | %s | %s | %.0f%% (oracle %.0f%%), %.2f, %.2f RTT | ", n, c.name, hint, 100*id.Share(), 100*id.OracleShare(), id.SymbolsPerDiff, id.RoundTrips)
					for _, d := range delays {
						p := base
						p.Delay = d
						fs := srepSweep(t, table, p, seeds)
						fmt.Fprintf(&b, "%s, %s, %s | ",
							srepSpreadOf(fs, func(f srepFigures) float64 { return f.Share }).format(100, 0),
							srepSpreadOf(fs, func(f srepFigures) float64 { return f.SymbolsPerDiff }).format(1, 2),
							srepSpreadOf(fs, func(f srepFigures) float64 { return f.MoresPerUpload }).format(1, 2))
					}
					b.WriteString("\n")
				}
			}
		}
		t.Log("\n" + b.String())
	})

	t.Run("rate", func(t *testing.T) {
		// differences of up to a few hundred at 50 uploads per second: fewer
		// draws per size keep the table's cost down
		table := newSREPDecodeTable(srepSimDraws/4, 9)
		var b strings.Builder
		b.WriteString("| N | constants | uploads/s | n_Δ p50 per session | share % | symbols/diff | batch convergence s | pullsync wire-level B (÷ AC-B1's) |\n")
		for _, n := range []int{9, 17} {
			for _, c := range srepConfigs[:2] {
				for _, rate := range []float64{2, 5, 15.4, 50} {
					p := srepParams{Nodes: n, Replication: 3, Rate: rate, Duration: 60 * time.Second, Delay: 50 * time.Millisecond, Tail: 10 * time.Second}.withDefaults()
					p.GrowthFactor, p.BatchFactor = c.growth, c.batchFactor
					fs := srepSweep(t, table, p, seeds)
					fmt.Fprintf(&b, "| %d | %s | %.1f | %s | %s | %s | %s | %.0f (%.2f) |\n", n, c.name, rate,
						srepSpreadOf(fs, func(f srepFigures) float64 { return f.DiffP50 }).format(1, 0),
						srepSpreadOf(fs, func(f srepFigures) float64 { return f.Share }).format(100, 0),
						srepSpreadOf(fs, func(f srepFigures) float64 { return f.SymbolsPerDiff }).format(1, 2),
						srepSpreadOf(fs, func(f srepFigures) float64 { return f.BatchConv }).format(1, 1),
						srepPullsyncWire(n, rate), srepPullsyncWire(n, rate)/srepPullsyncACB1(n))
				}
			}
		}
		t.Log("\n" + b.String())
	})

	t.Run("production sizes", func(t *testing.T) {
		// a reserve past ScanLimit (the root cache bounds a stream at
		// RootSymbols, and the count varints take two bytes) and a root of
		// radius 10, at the steady load with 50 ms
		var b strings.Builder
		b.WriteString("| N | constants | metadata B per upload | share % | symbols/diff | outside |\n")
		for _, n := range []int{9, 17} {
			for _, c := range srepConfigs {
				o := steadyOptions(n, 1)
				o.Delay, o.Base = 50*time.Millisecond, 1<<20
				p := srepParamsOf(o)
				p.Root, p.GrowthFactor, p.BatchFactor = rangeOf("1011001110"), c.growth, c.batchFactor
				fs := make([]srepFigures, 0, seeds)
				outside := 0
				for seed := range uint64(seeds) {
					p.Seed = seed + 1
					r := p.run(t, table)
					outside += r.Outside
					fs = append(fs, srepModelFigures(r))
				}
				fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %d |\n", n, c.name,
					srepSpreadOf(fs, func(f srepFigures) float64 { return f.Metadata }).format(1, 0),
					srepSpreadOf(fs, func(f srepFigures) float64 { return f.Share }).format(100, 0),
					srepSpreadOf(fs, func(f srepFigures) float64 { return f.SymbolsPerDiff }).format(1, 2), outside)
			}
		}
		t.Log("\n" + b.String())
	})

	t.Run("initiated sessions", func(t *testing.T) {
		// MaxInitiatedSessions (8) against a slot per peer: at N = 17 the
		// lowest overlays open up to 16 sessions per round, in two waves of
		// 8, and the second wave meets peers that fetched from the first.
		// The model does not cap the sessions a node serves
		// (MaxConcurrentSessions), so 16 slots is a bound, not a design.
		var b strings.Builder
		b.WriteString("| N | slots | constants | n_Δ per upload (p(N − p)) | share % | symbols/diff | batch convergence s |\n")
		for _, n := range []int{9, 17} {
			for _, slots := range []int{8, 16} {
				for _, c := range srepConfigs[:2] {
					o := steadyOptions(n, 1)
					o.Delay = 50 * time.Millisecond
					p := srepParamsOf(o)
					p.InitiatedSessions, p.GrowthFactor, p.BatchFactor = slots, c.growth, c.batchFactor
					fs := srepSweep(t, table, p, seeds)
					fmt.Fprintf(&b, "| %d | %d | %s | %s (%d) | %s | %s | %s |\n", n, slots, c.name,
						srepSpreadOf(fs, func(f srepFigures) float64 { return f.DiffPerUpload }).format(1, 1), 3*(n-3),
						srepSpreadOf(fs, func(f srepFigures) float64 { return f.Share }).format(100, 0),
						srepSpreadOf(fs, func(f srepFigures) float64 { return f.SymbolsPerDiff }).format(1, 2),
						srepSpreadOf(fs, func(f srepFigures) float64 { return f.BatchConv }).format(1, 2))
				}
			}
		}
		t.Log("\n" + b.String())
	})

	t.Run("replication and MinBatch", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("| N | p | MinBatch | constants | n_Δ per upload (p(N − p)) | share % | symbols/diff |\n")
		for _, n := range []int{9, 17} {
			for _, rep := range []int{2, 3, 4} {
				for _, minBatch := range []int{2, 4, 8} {
					if rep != 3 && minBatch != rangesync.MinBatch {
						continue
					}
					for _, c := range srepConfigs[:2] {
						o := steadyOptions(n, 1)
						o.Replication, o.Delay = rep, 50*time.Millisecond
						p := srepParamsOf(o)
						p.MinBatch, p.GrowthFactor, p.BatchFactor = minBatch, c.growth, c.batchFactor
						fs := srepSweep(t, table, p, seeds)
						fmt.Fprintf(&b, "| %d | %d | %d | %s | %s (%d) | %s | %s |\n", n, rep, minBatch, c.name,
							srepSpreadOf(fs, func(f srepFigures) float64 { return f.DiffPerUpload }).format(1, 1), rep*(n-rep),
							srepSpreadOf(fs, func(f srepFigures) float64 { return f.Share }).format(100, 0),
							srepSpreadOf(fs, func(f srepFigures) float64 { return f.SymbolsPerDiff }).format(1, 2))
					}
				}
			}
		}
		t.Log("\n" + b.String())
	})
}
