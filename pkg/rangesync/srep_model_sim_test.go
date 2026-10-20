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
//	go test ./pkg/rangesync/ -tags=sim -run 'TestSREPModel(VsHarness|G6)' -v -timeout 60m
//
// TestSREPModelVsHarness validates the model against the harness
// (runNeighborhood) in its steady mode, and TestSREPModelG6 sweeps the model
// over G6's candidate constants, the hint, N, p, the upload rate and Delay.
//
// The harness runs with the compiled constants. To measure a G6 candidate
// with it, change GrowthFactor, or BatchFactorNum and BatchFactorDen (which
// FirstBatch reads), in a scratch copy and run TestSREPModelVsHarness there:
// the model follows the compiled constants (srepCompiled), and the test logs
// the harness's records of them for srepHarnessRef.

// srepSimDraws is the number of decoder draws per difference size in the
// tagged runs.
const srepSimDraws = 1000

// srepCompiled returns the model's parameters for the compiled constants.
func srepCompiled(p srepParams) srepParams {
	return srepConfigCompiled.apply(p)
}

// srepLabel names the session constants of p.
func srepLabel(p srepParams) string {
	return fmt.Sprintf("GrowthFactor %d, BatchFactor %v, MinBatch %d", p.GrowthFactor, p.BatchFactor, p.MinBatch)
}

// srepConfig is a set of session constants.
type srepConfig struct {
	name   string
	growth int
	bf     srepFactor
}

// apply returns p with c's constants and the compiled MinBatch.
func (c srepConfig) apply(p srepParams) srepParams {
	p.GrowthFactor, p.BatchFactor, p.MinBatch = c.growth, c.bf, rangesync.MinBatch
	return p
}

// same reports whether c and d have the same constants.
func (c srepConfig) same(d srepConfig) bool { return c.growth == d.growth && c.bf == d.bf }

// srepConfigCompiled is the compiled constants.
var srepConfigCompiled = srepConfig{
	name:   "now",
	growth: rangesync.GrowthFactor,
	bf:     srepFactor{Num: rangesync.BatchFactorNum, Den: rangesync.BatchFactorDen},
}

// srepConfigs are the session constants G6 decides between: "now" is the
// compiled constants.
var srepConfigs = []srepConfig{
	srepConfigCompiled,
	{name: "GrowthFactor 2", growth: 2, bf: srepFactor{16, 10}},
	{name: "BatchFactor 2.0", growth: 4, bf: srepFactor{2, 1}},
	{name: "both", growth: 2, bf: srepFactor{2, 1}},
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
	// RoundTrips is the mean round trips of a completed session, the open
	// and the close counted (the harness's at a Delay only, from Spans)
	RoundTrips float64
	// Refused counts the sessions answered Busy or over a session limit
	// (the harness's only: the model has no session caps)
	Refused    float64
	ZeroShare  float64 // share of sessions with no difference
	BatchConv  float64 // mean convergence of a batch, seconds
	MaxConv    float64 // longest convergence of a batch, seconds
	Duplicates float64
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
	return srepFigures{
		Share: metaIn / uploads / srepPullsyncACB1(o.Nodes), Metadata: metaIn / uploads,
		Status: status, Reconcile: reconcile, Want: metaIn/uploads - status - reconcile,
		SymbolsPerDiff: spd, DiffPerUpload: sum / uploads, DiffP50: quantile(exact, 0.5),
		MoresPerUpload: mores / uploads, SessionsPerUpload: float64(len(exact)) / uploads,
		RoundTrips: trips / float64(max(sessions, 1)),
		Refused:    (res.Sessions["busy"] + res.Sessions["session_limit"]) / uploads,
		ZeroShare:  float64(zero) / float64(max(len(exact), 1)),
		BatchConv:  conv / float64(len(res.Batches)), MaxConv: maxConv,
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
		// an equal session takes 3 round trips (the open, the opening and
		// its empty answer, the close), a decoded one 4 and one per More
		RoundTrips: float64(3*r.Equal+4*r.Decoded+r.Mores) / float64(max(r.Equal+r.Decoded, 1)),
		ZeroShare:  float64(zero) / float64(max(len(r.Diffs), 1)),
		BatchConv:  conv / float64(len(r.Batches)), MaxConv: maxConv,
		Duplicates: float64(r.Duplicates) / uploads,
	}
}

// srepSpread is the mean, standard deviation, minimum and maximum of one
// figure over n runs.
type srepSpread struct {
	mean, sd, lo, hi float64
	n                int
}

func srepSpreadOf(fs []srepFigures, get func(srepFigures) float64) srepSpread {
	xs := make([]float64, len(fs))
	for i, f := range fs {
		xs[i] = get(f)
	}
	return srepSpreadOfValues(xs)
}

func srepSpreadOfValues(xs []float64) srepSpread {
	s := srepSpread{lo: slices.Min(xs), hi: slices.Max(xs), n: len(xs)}
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

// se returns the standard error of the mean.
func (s srepSpread) se() float64 { return s.sd / math.Sqrt(float64(max(s.n, 1))) }

func (s srepSpread) format(scale float64, prec int) string {
	return fmt.Sprintf("%.*f (%.*f–%.*f)", prec, s.mean*scale, prec, s.lo*scale, prec, s.hi*scale)
}

// formatSD formats the mean, the standard deviation and the range.
func (s srepSpread) formatSD(scale float64, prec int) string {
	return fmt.Sprintf("%.*f ± %.*f (%.*f–%.*f)", prec, s.mean*scale, prec, s.sd*scale, prec, s.lo*scale, prec, s.hi*scale)
}

// srepPaired returns the spread of the per-seed differences b − a of one
// figure, over the seeds both ran (the first min(len(a), len(b))): runs of one
// seed share the uploads' holders, so their difference varies less than
// either.
func srepPaired(a, b []srepFigures, get func(srepFigures) float64) srepSpread {
	xs := make([]float64, min(len(a), len(b)))
	for i := range xs {
		xs[i] = get(b[i]) - get(a[i])
	}
	return srepSpreadOfValues(xs)
}

// srepRow is one row of TestSREPModelVsHarness: N, Delay and whether
// hint(R) is off, in the harness's steady mode.
type srepRow struct {
	n       int
	delay   time.Duration
	noHints bool
}

func (r srepRow) String() string {
	hints := "hints"
	if r.noHints {
		hints = "no hints"
	}
	return fmt.Sprintf("N=%d/Delay=%v/%s", r.n, r.delay, hints)
}

// validated reports whether the row is in the model's validated domain for
// n_Δ, the symbols and the differences between configurations: at a Delay.
// At Delay 0 the harness's goroutines of one instant are not fully in step,
// so a later session can read after a fetch of the same instant: at N = 17
// its n_Δ is 7–11% above the model's, and the model's symbols per
// difference are 2–11% above its own, which partly cancels in the share
// (still checked, within srepShareTol, but no evidence).
func (r srepRow) validated() bool { return r.delay > 0 }

// options returns the harness's options of the row for seed.
func (r srepRow) options(seed uint64) simOptions {
	o := steadyOptions(r.n, seed)
	o.Delay, o.NoHints = r.delay, r.noHints
	return o
}

// srepRows are the rows of TestSREPModelVsHarness.
var srepRows = []srepRow{
	{n: 9},
	{n: 9, delay: 50 * time.Millisecond},
	{n: 9, delay: 200 * time.Millisecond},
	{n: 9, delay: 50 * time.Millisecond, noHints: true},
	{n: 17},
	{n: 17, delay: 50 * time.Millisecond},
	{n: 17, delay: 200 * time.Millisecond},
	{n: 17, delay: 50 * time.Millisecond, noHints: true},
}

// srepHarnessSeeds is the number of harness seeds per row of
// TestSREPModelVsHarness and of the records in srepHarnessRef, and
// srepModelSeeds the model's.
const (
	srepHarnessSeeds = 6
	srepModelSeeds   = 20
)

// srepRef is the harness's record of one row with one set of session
// constants: the mean and standard deviation over the seeds 1 … n of the
// share of AC-B1's pullsync, n_Δ per upload and the symbols per difference.
type srepRef struct {
	growth                                   int
	bf                                       srepFactor
	row                                      srepRow
	n                                        int
	share, shareSD, diff, diffSD, spd, spdSD float64
}

// spread returns a recorded figure as a spread (no range).
func (r srepRef) spread(mean, sd float64) srepSpread {
	return srepSpread{mean: mean, sd: sd, lo: math.NaN(), hi: math.NaN(), n: r.n}
}

// String formats r as an element of srepHarnessRef.
func (r srepRef) String() string {
	delay := "0"
	if r.row.delay > 0 {
		delay = fmt.Sprintf("%d * time.Millisecond", r.row.delay.Milliseconds())
	}
	return fmt.Sprintf("{%d, srepFactor{%d, %d}, srepRow{%d, %s, %v}, %d, %.4f, %.4f, %.2f, %.2f, %.3f, %.3f},",
		r.growth, r.bf.Num, r.bf.Den, r.row.n, delay, r.row.noHints, r.n, r.share, r.shareSD, r.diff, r.diffSD, r.spd, r.spdSD)
}

// srepRefOf returns the record of the harness's figures fs of row with c.
func srepRefOf(c srepConfig, row srepRow, fs []srepFigures) srepRef {
	share := srepSpreadOf(fs, func(f srepFigures) float64 { return f.Share })
	diff := srepSpreadOf(fs, func(f srepFigures) float64 { return f.DiffPerUpload })
	spd := srepSpreadOf(fs, func(f srepFigures) float64 { return f.SymbolsPerDiff })
	return srepRef{growth: c.growth, bf: c.bf, row: row, n: len(fs), share: share.mean, shareSD: share.sd, diff: diff.mean, diffSD: diff.sd, spd: spd.mean, spdSD: spd.sd}
}

// srepRecorded returns the record of row with c in srepHarnessRef.
func srepRecorded(c srepConfig, row srepRow) (srepRef, bool) {
	for _, r := range srepHarnessRef {
		if r.growth == c.growth && r.bf == c.bf && r.row == row {
			return r, true
		}
	}
	return srepRef{}, false
}

// srepHarnessRef holds the harness's records of every row of
// TestSREPModelVsHarness with each of srepConfigs, srepHarnessSeeds seeds
// each, measured in scratch copies with the constants changed
// (TestSREPModelVsHarness logs the records of the compiled constants). A
// change of the protocol that moves the harness's figures fails the record
// check of TestSREPModelVsHarness; record them again then.
var srepHarnessRef = []srepRef{
	// the compiled constants (GrowthFactor 4, BatchFactor 1.6)
	{4, srepFactor{16, 10}, srepRow{9, 0, false}, 6, 0.7806, 0.0261, 18.30, 0.13, 3.088, 0.135},
	{4, srepFactor{16, 10}, srepRow{9, 50 * time.Millisecond, false}, 6, 0.7688, 0.0170, 18.13, 0.05, 3.064, 0.093},
	{4, srepFactor{16, 10}, srepRow{9, 200 * time.Millisecond, false}, 6, 0.7204, 0.0238, 18.06, 0.10, 2.844, 0.127},
	{4, srepFactor{16, 10}, srepRow{9, 50 * time.Millisecond, true}, 6, 0.8044, 0.0176, 18.17, 0.08, 3.226, 0.091},
	{4, srepFactor{16, 10}, srepRow{17, 0, false}, 6, 0.6477, 0.0138, 52.86, 1.53, 3.478, 0.062},
	{4, srepFactor{16, 10}, srepRow{17, 50 * time.Millisecond, false}, 6, 0.6213, 0.0200, 49.64, 0.62, 3.484, 0.112},
	{4, srepFactor{16, 10}, srepRow{17, 200 * time.Millisecond, false}, 6, 0.6623, 0.0397, 54.79, 1.05, 3.355, 0.191},
	{4, srepFactor{16, 10}, srepRow{17, 50 * time.Millisecond, true}, 6, 0.5916, 0.0124, 49.15, 0.75, 3.315, 0.094},
	// GrowthFactor 2
	{2, srepFactor{16, 10}, srepRow{9, 0, false}, 6, 0.6066, 0.0153, 18.49, 0.25, 2.192, 0.044},
	{2, srepFactor{16, 10}, srepRow{9, 50 * time.Millisecond, false}, 6, 0.5951, 0.0048, 18.16, 0.11, 2.189, 0.027},
	{2, srepFactor{16, 10}, srepRow{9, 200 * time.Millisecond, false}, 6, 0.5975, 0.0094, 18.04, 0.14, 2.225, 0.036},
	{2, srepFactor{16, 10}, srepRow{9, 50 * time.Millisecond, true}, 6, 0.5957, 0.0098, 18.19, 0.09, 2.171, 0.048},
	{2, srepFactor{16, 10}, srepRow{17, 0, false}, 6, 0.5051, 0.0289, 53.87, 1.98, 2.483, 0.084},
	{2, srepFactor{16, 10}, srepRow{17, 50 * time.Millisecond, false}, 6, 0.4860, 0.0115, 49.46, 1.03, 2.559, 0.062},
	{2, srepFactor{16, 10}, srepRow{17, 200 * time.Millisecond, false}, 6, 0.5215, 0.0078, 53.89, 0.42, 2.537, 0.070},
	{2, srepFactor{16, 10}, srepRow{17, 50 * time.Millisecond, true}, 6, 0.4430, 0.0092, 48.52, 0.52, 2.316, 0.071},
	// BatchFactor 2.0
	{4, srepFactor{2, 1}, srepRow{9, 0, false}, 6, 0.6913, 0.0200, 18.32, 0.14, 2.637, 0.088},
	{4, srepFactor{2, 1}, srepRow{9, 50 * time.Millisecond, false}, 6, 0.7107, 0.0199, 18.18, 0.09, 2.768, 0.111},
	{4, srepFactor{2, 1}, srepRow{9, 200 * time.Millisecond, false}, 6, 0.7062, 0.0177, 18.04, 0.13, 2.775, 0.089},
	{4, srepFactor{2, 1}, srepRow{9, 50 * time.Millisecond, true}, 6, 0.8076, 0.0246, 18.15, 0.04, 3.245, 0.129},
	{4, srepFactor{2, 1}, srepRow{17, 0, false}, 6, 0.6212, 0.0191, 52.35, 1.32, 3.334, 0.077},
	{4, srepFactor{2, 1}, srepRow{17, 50 * time.Millisecond, false}, 6, 0.6121, 0.0183, 49.59, 0.14, 3.422, 0.113},
	{4, srepFactor{2, 1}, srepRow{17, 200 * time.Millisecond, false}, 6, 0.6378, 0.0165, 54.62, 0.73, 3.225, 0.074},
	{4, srepFactor{2, 1}, srepRow{17, 50 * time.Millisecond, true}, 6, 0.5916, 0.0121, 49.13, 0.87, 3.316, 0.105},
	// both
	{2, srepFactor{2, 1}, srepRow{9, 0, false}, 6, 0.6137, 0.0130, 18.25, 0.18, 2.264, 0.059},
	{2, srepFactor{2, 1}, srepRow{9, 50 * time.Millisecond, false}, 6, 0.6224, 0.0088, 18.13, 0.17, 2.333, 0.042},
	{2, srepFactor{2, 1}, srepRow{9, 200 * time.Millisecond, false}, 6, 0.6397, 0.0084, 18.04, 0.17, 2.440, 0.039},
	{2, srepFactor{2, 1}, srepRow{9, 50 * time.Millisecond, true}, 6, 0.5989, 0.0086, 18.18, 0.11, 2.187, 0.044},
	{2, srepFactor{2, 1}, srepRow{17, 0, false}, 6, 0.5118, 0.0254, 52.53, 2.26, 2.617, 0.068},
	{2, srepFactor{2, 1}, srepRow{17, 50 * time.Millisecond, false}, 6, 0.5229, 0.0076, 49.77, 0.96, 2.797, 0.040},
	{2, srepFactor{2, 1}, srepRow{17, 200 * time.Millisecond, false}, 6, 0.5566, 0.0111, 53.80, 1.01, 2.764, 0.044},
	{2, srepFactor{2, 1}, srepRow{17, 50 * time.Millisecond, true}, 6, 0.4492, 0.0089, 48.75, 0.62, 2.348, 0.052},
}

// The tolerances of TestSREPModelVsHarness: the difference of two means
// passes when it lies within the tolerance plus twice its standard error.
const (
	// srepShareTol bounds the share of AC-B1's pullsync (absolute: 3
	// points).
	srepShareTol = 0.03
	// srepDeltaTol bounds a difference of the share between configurations
	// or rows (absolute: 2 points).
	srepDeltaTol = 0.02
	// srepDiffTol and srepSymbolsTol bound n_Δ per upload and the symbols per
	// difference, each on its own (relative), so that errors in the two
	// cannot cancel in the share beyond them.
	srepDiffTol    = 0.06
	srepSymbolsTol = 0.06
	// srepRecordTol bounds the share's difference between the live harness
	// and its record (absolute: 1.5 points), and srepRecordRelTol n_Δ's and
	// the symbols' (relative).
	srepRecordTol    = 0.015
	srepRecordRelTol = 0.03
)

// srepFigureNames are the figures a comparison reports, with their scale and
// precision, and the tolerance of the checked ones (tol > 0): relative to the
// harness's mean when rel is set, otherwise absolute in the figure's unit. A
// component (n_Δ and the symbols) is checked only at a Delay
// (srepRow.validated).
var srepFigureNames = []struct {
	name      string
	get       func(srepFigures) float64
	scale     float64
	prec      int
	tol       float64
	rel       bool
	component bool
}{
	{name: "share of pullsync %", get: func(f srepFigures) float64 { return f.Share }, scale: 100, prec: 1, tol: srepShareTol},
	{name: "n_Δ per upload", get: func(f srepFigures) float64 { return f.DiffPerUpload }, scale: 1, prec: 1, tol: srepDiffTol, rel: true, component: true},
	{name: "symbols per difference", get: func(f srepFigures) float64 { return f.SymbolsPerDiff }, scale: 1, prec: 2, tol: srepSymbolsTol, rel: true, component: true},
	{name: "n_Δ p50 per session", get: func(f srepFigures) float64 { return f.DiffP50 }, scale: 1, prec: 0, tol: 0.15, rel: true, component: true},
	{name: "batch convergence s", get: func(f srepFigures) float64 { return f.BatchConv }, scale: 1, prec: 2, tol: 1},
	{name: "longest batch convergence s", get: func(f srepFigures) float64 { return f.MaxConv }, scale: 1, prec: 2, tol: 1},
	{name: "Mores per upload", get: func(f srepFigures) float64 { return f.MoresPerUpload }, scale: 1, prec: 2},
	{name: "round trips per session", get: func(f srepFigures) float64 { return f.RoundTrips }, scale: 1, prec: 2},
	{name: "status B", get: func(f srepFigures) float64 { return f.Status }, scale: 1, prec: 0},
	{name: "reconcile B", get: func(f srepFigures) float64 { return f.Reconcile }, scale: 1, prec: 0},
	{name: "want B", get: func(f srepFigures) float64 { return f.Want }, scale: 1, prec: 0},
	{name: "sessions per upload", get: func(f srepFigures) float64 { return f.SessionsPerUpload }, scale: 1, prec: 2},
	{name: "refused sessions per upload", get: func(f srepFigures) float64 { return f.Refused }, scale: 1, prec: 3},
	{name: "zero-difference share %", get: func(f srepFigures) float64 { return f.ZeroShare }, scale: 100, prec: 1},
	{name: "duplicates per upload", get: func(f srepFigures) float64 { return f.Duplicates }, scale: 1, prec: 2},
}

// srepWithin reports whether the difference dev of two means lies within
// tol plus twice its standard error se, and returns that bound.
func srepWithin(dev, se, tol float64) (bound float64, ok bool) {
	bound = tol + 2*se
	return bound, math.Abs(dev) <= bound
}

// srepCompare compares the means of the model's spread m and the harness's
// h with tolerance tol (relative to h's mean when rel is set), fails t if
// check is set and they differ by more, and returns a line for the log: the
// difference (in percent when rel is set) and its bound.
func srepCompare(t *testing.T, what string, m, h srepSpread, tol float64, rel bool, scale float64, prec int, check bool) string {
	t.Helper()
	dev, se, unit := m.mean-h.mean, math.Hypot(m.se(), h.se()), scale
	if rel {
		dev, se, unit = dev/h.mean, se/h.mean, 100
	}
	bound, ok := srepWithin(dev, se, tol)
	if !check {
		return fmt.Sprintf("%+.2f (±%.2f, not checked)", dev*unit, bound*unit)
	}
	if !ok {
		t.Errorf("%s: model %.*f, harness %.*f: difference %.2f outside ±%.2f (tolerance %.2f + 2 × standard error)",
			what, prec, m.mean*scale, prec, h.mean*scale, dev*unit, bound*unit, tol*unit)
	}
	return fmt.Sprintf("%+.2f (±%.2f)", dev*unit, bound*unit)
}

// TestSREPModelVsHarness validates the model against the harness
// (runNeighborhood) in its steady mode (steadyOptions), on srepRows: N = 9
// and 17, Delay 0, 50 and 200 ms with hint(R) and 50 ms without.
//
// The harness runs the compiled constants, srepHarnessSeeds seeds per row,
// one run after another and each torn down before the next, so that no run
// starves another's goroutines (simOptions.Settle); the model runs
// srepModelSeeds seeds of the same options. A figure passes when the
// difference of the two means lies within a tolerance plus twice its
// standard error, from the spread over the seeds of both sides: the harness
// is not exactly repeatable (one seed's share moves by up to 3 points between
// runs), and the bound widens by the spread the run measured. Checked:
//   - the share of AC-B1's pullsync within 3 points;
//   - n_Δ per upload and the symbols per difference within 6% each, so that
//     errors in the two cannot cancel in the share beyond that, and the
//     median n_Δ per session within 15%: at a Delay only (srepRow.validated;
//     at Delay 0 they are logged);
//   - the mean and the longest convergence of a batch within 1 s;
//   - no duplicate delivery on either side.
//
// The differences G6's calls rest on are checked the same way, within 2
// points: hint(R) off against on at 50 ms and Delay 200 ms against 50 ms,
// with both sides measured here, and, at a Delay, each candidate of
// srepConfigs against the compiled constants. The harness needs a build per
// constant set, so for those it uses its records (srepHarnessRef), measured
// in scratch copies with the constants changed: the model runs every
// candidate, and its absolute figures, its differences from the compiled
// constants and its hint(R) differences are checked against the records. The
// live harness must match the record of the compiled constants (the share
// within 1.5 points, n_Δ and the symbols within 3%), so that a record that no
// longer describes the code fails. The test logs the records of the compiled
// constants.
func TestSREPModelVsHarness(t *testing.T) {
	table := newSREPDecodeTable(srepSimDraws, 7)
	share := func(f srepFigures) float64 { return f.Share }

	// the harness with the compiled constants, and the model with every
	// candidate
	harness := make(map[srepRow][]srepFigures)
	walls := make(map[srepRow]time.Duration)
	for _, row := range srepRows {
		start := time.Now()
		for seed := range uint64(srepHarnessSeeds) {
			o := row.options(seed + 1)
			// a subtest of its own, so that the neighborhood is torn down
			// (tb.Cleanup) before the next run
			t.Run(fmt.Sprintf("harness/%v/seed=%d", row, seed+1), func(t *testing.T) {
				harness[row] = append(harness[row], srepHarnessFigures(o, runNeighborhood(t, o)))
			})
		}
		walls[row] = time.Since(start)
	}
	if t.Failed() {
		t.FailNow()
	}
	model := make(map[string]map[srepRow][]srepFigures)
	for _, c := range srepConfigs {
		model[c.name] = make(map[srepRow][]srepFigures)
		for _, row := range srepRows {
			for seed := range uint64(srepModelSeeds) {
				p := c.apply(srepParamsOf(row.options(seed + 1)))
				model[c.name][row] = append(model[c.name][row], srepModelFigures(p.run(t, table)))
			}
		}
	}
	now := model[srepConfigCompiled.name]

	t.Run("compiled", func(t *testing.T) {
		var b strings.Builder
		fmt.Fprintf(&b, "%s; harness %d seeds, model %d seeds; model | harness | model − harness (bound):", srepLabel(srepCompiled(srepParams{})), srepHarnessSeeds, srepModelSeeds)
		for _, row := range srepRows {
			ms, hs := now[row], harness[row]
			fmt.Fprintf(&b, "\n%v (harness %v):", row, walls[row].Round(time.Second))
			for _, f := range srepFigureNames {
				m, h := srepSpreadOf(ms, f.get), srepSpreadOf(hs, f.get)
				fmt.Fprintf(&b, "\n  %s: %s | %s", f.name, m.formatSD(f.scale, f.prec), h.formatSD(f.scale, f.prec))
				if f.tol > 0 {
					fmt.Fprintf(&b, " | %s", srepCompare(t, fmt.Sprintf("%v: %s", row, f.name), m, h, f.tol, f.rel, f.scale, f.prec, !f.component || row.validated()))
				}
			}
			if m, h := srepSpreadOf(ms, func(f srepFigures) float64 { return f.Duplicates }), srepSpreadOf(hs, func(f srepFigures) float64 { return f.Duplicates }); m.hi != 0 || h.hi != 0 {
				t.Errorf("%v: duplicate deliveries per upload, model up to %.2f, harness up to %.2f, want none", row, m.hi, h.hi)
			}
		}
		t.Log(b.String())
	})

	t.Run("rows", func(t *testing.T) {
		// differences between rows, both sides measured here, paired by seed
		var b strings.Builder
		b.WriteString("share of pullsync, model Δ | harness Δ | model Δ − harness Δ (bound), points:")
		for _, n := range []int{9, 17} {
			for _, d := range []struct {
				name string
				a, b srepRow
			}{
				{"hint(R) off − on at 50 ms", srepRow{n: n, delay: 50 * time.Millisecond}, srepRow{n: n, delay: 50 * time.Millisecond, noHints: true}},
				{"Delay 200 ms − 50 ms", srepRow{n: n, delay: 50 * time.Millisecond}, srepRow{n: n, delay: 200 * time.Millisecond}},
			} {
				m, h := srepPaired(now[d.a], now[d.b], share), srepPaired(harness[d.a], harness[d.b], share)
				fmt.Fprintf(&b, "\n  N = %d, %s: %s | %s | %s", n, d.name, m.formatSD(100, 1), h.formatSD(100, 1),
					srepCompare(t, fmt.Sprintf("N = %d, %s", n, d.name), m, h, srepDeltaTol, false, 100, 1, true))
			}
		}
		t.Log(b.String())
	})

	t.Run("records", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("records of the compiled constants (srepHarnessRef):")
		for _, row := range srepRows {
			live := srepRefOf(srepConfigCompiled, row, harness[row])
			fmt.Fprintf(&b, "\n\t%v", live)
			rec, ok := srepRecorded(srepConfigCompiled, row)
			if !ok {
				fmt.Fprintf(&b, " // not recorded")
				continue
			}
			fmt.Fprintf(&b, " // live − recorded: share %s, n_Δ %s, symbols %s",
				srepCompare(t, fmt.Sprintf("%v: live harness against its record: share", row), live.spread(live.share, live.shareSD), rec.spread(rec.share, rec.shareSD), srepRecordTol, false, 100, 1, true),
				srepCompare(t, fmt.Sprintf("%v: live harness against its record: n_Δ per upload", row), live.spread(live.diff, live.diffSD), rec.spread(rec.diff, rec.diffSD), srepRecordRelTol, true, 1, 1, true),
				srepCompare(t, fmt.Sprintf("%v: live harness against its record: symbols per difference", row), live.spread(live.spd, live.spdSD), rec.spread(rec.spd, rec.spdSD), srepRecordRelTol, true, 1, 2, true))
		}
		t.Log(b.String())
	})

	t.Run("configurations", func(t *testing.T) {
		// the candidates against their records, and their differences from
		// the compiled constants against the records'
		var b strings.Builder
		b.WriteString("share of pullsync %, n_Δ per upload, symbols per difference: model | record | model − record (bound); share Δ from the compiled constants, points: model | record | model − record (bound)")
		for _, c := range srepConfigs {
			if c.same(srepConfigCompiled) {
				continue
			}
			for _, row := range srepRows {
				rec, ok := srepRecorded(c, row)
				base, okBase := srepRecorded(srepConfigCompiled, row)
				if !ok || !okBase {
					fmt.Fprintf(&b, "\n%s, %v: no record", c.name, row)
					continue
				}
				what := fmt.Sprintf("%s, %v", c.name, row)
				ms := model[c.name][row]
				m := srepSpreadOf(ms, share)
				nd := srepSpreadOf(ms, func(f srepFigures) float64 { return f.DiffPerUpload })
				spd := srepSpreadOf(ms, func(f srepFigures) float64 { return f.SymbolsPerDiff })
				fmt.Fprintf(&b, "\n%s: %.1f | %.1f | %s; %.1f | %.1f | %s; %.2f | %.2f | %s", what,
					100*m.mean, 100*rec.share, srepCompare(t, what+": share", m, rec.spread(rec.share, rec.shareSD), srepShareTol, false, 100, 1, true),
					nd.mean, rec.diff, srepCompare(t, what+": n_Δ per upload", nd, rec.spread(rec.diff, rec.diffSD), srepDiffTol, true, 1, 1, row.validated()),
					spd.mean, rec.spd, srepCompare(t, what+": symbols per difference", spd, rec.spread(rec.spd, rec.spdSD), srepSymbolsTol, true, 1, 2, row.validated()))
				md := srepPaired(now[row], ms, share)
				hd := srepSpread{mean: rec.share - base.share, sd: math.Hypot(rec.shareSD, base.shareSD), n: min(rec.n, base.n)}
				fmt.Fprintf(&b, "; Δ %+.1f | %+.1f | %s", 100*md.mean, 100*hd.mean, srepCompare(t, what+": share Δ from the compiled constants", md, hd, srepDeltaTol, false, 100, 1, row.validated()))
			}
			// the hint's difference with the candidate
			for _, n := range []int{9, 17} {
				on, off := srepRow{n: n, delay: 50 * time.Millisecond}, srepRow{n: n, delay: 50 * time.Millisecond, noHints: true}
				recOn, okOn := srepRecorded(c, on)
				recOff, okOff := srepRecorded(c, off)
				if !okOn || !okOff {
					continue
				}
				what := fmt.Sprintf("%s, N = %d, hint(R) off − on at 50 ms", c.name, n)
				md := srepPaired(model[c.name][on], model[c.name][off], share)
				hd := srepSpread{mean: recOff.share - recOn.share, sd: math.Hypot(recOn.shareSD, recOff.shareSD), n: min(recOn.n, recOff.n)}
				fmt.Fprintf(&b, "\n%s: Δ %+.1f | %+.1f | %s", what, 100*md.mean, 100*hd.mean, srepCompare(t, what, md, hd, srepDeltaTol, false, 100, 1, true))
			}
		}
		t.Log(b.String())
	})
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
					base.GrowthFactor, base.BatchFactor, base.NoHints = c.growth, c.bf, noHints
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
					p.GrowthFactor, p.BatchFactor = c.growth, c.bf
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
				p.Root, p.GrowthFactor, p.BatchFactor = rangeOf("1011001110"), c.growth, c.bf
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
		// The gain is not validated: it runs through the second wave's
		// hints, where the model's symbols per difference sit above the
		// harness's (TestSREPModelVsHarness), and the harness cannot run 16
		// slots without a scratch edit.
		var b strings.Builder
		b.WriteString("| N | slots | constants | n_Δ per upload (p(N − p)) | share % | symbols/diff | batch convergence s |\n")
		for _, n := range []int{9, 17} {
			for _, slots := range []int{8, 16} {
				for _, c := range srepConfigs[:2] {
					o := steadyOptions(n, 1)
					o.Delay = 50 * time.Millisecond
					p := srepParamsOf(o)
					p.InitiatedSessions, p.GrowthFactor, p.BatchFactor = slots, c.growth, c.bf
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
						p.MinBatch, p.GrowthFactor, p.BatchFactor = minBatch, c.growth, c.bf
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
