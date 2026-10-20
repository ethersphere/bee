// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
)

// Checks of the analytical model of the neighborhood (srep_model_test.go):
// that its pieces are those of the protocol (first batch, wire sizes, symbol
// sizes, the More growth, the decoder's thresholds), and short runs at
// §7.8's sizes. The heavy runs, the comparison with the harness and the G6
// sweep, are in srep_model_sim_test.go (-tags=sim).

// srepCheckDraws is the number of decoder draws per difference size in the
// default runs: enough for means within a few percent.
const srepCheckDraws = 128

// TestSREPFirstBatch checks that the model's session constants default to the
// compiled ones (BatchFactorNum / BatchFactorDen, GrowthFactor, MinBatch) and
// that its first batch is rangesync.FirstBatch with them: the model computes
// it with FirstBatch's rule (rangesync.FirstBatchOf), and FirstBatch reads the
// constants, so a change of a constant changes the protocol, the harness and
// the model together. A candidate fraction needs no tenths: 7/4 is ⌈1.75 d⌉ +
// 2.
func TestSREPFirstBatch(t *testing.T) {
	t.Parallel()

	p := srepParams{}.withDefaults()
	if want := (srepFactor{Num: rangesync.BatchFactorNum, Den: rangesync.BatchFactorDen}); p.BatchFactor != want {
		t.Fatalf("model BatchFactor %v, constants %v", p.BatchFactor, want)
	}
	if p.GrowthFactor != rangesync.GrowthFactor || p.MinBatch != rangesync.MinBatch {
		t.Fatalf("model GrowthFactor %d MinBatch %d, constants %d %d", p.GrowthFactor, p.MinBatch, rangesync.GrowthFactor, rangesync.MinBatch)
	}
	for d := range 20001 {
		if got, want := p.firstBatch(d), rangesync.FirstBatch(uint64(d)); uint64(got) != want {
			t.Fatalf("first batch of %d: model %d, FirstBatch %d", d, got, want)
		}
	}
	// G6 candidates: BatchFactor 2.0 is 2d + 2, 1.75 is ⌈1.75 d⌉ + 2
	for _, tc := range []struct {
		f        srepFactor
		d, want  int
		minBatch int
	}{
		{srepFactor{2, 1}, 0, 4, 4},
		{srepFactor{2, 1}, 1, 4, 4},
		{srepFactor{2, 1}, 2, 6, 4},
		{srepFactor{2, 1}, 10, 22, 4},
		{srepFactor{2, 1}, 45, 92, 4},
		{srepFactor{7, 4}, 10, 20, 4},
		{srepFactor{7, 4}, 45, 81, 4},
		{srepFactor{16, 10}, 2, 8, 8},
	} {
		q := p
		q.BatchFactor, q.MinBatch = tc.f, tc.minBatch
		if got := q.firstBatch(tc.d); got != tc.want {
			t.Errorf("BatchFactor %v, MinBatch %d: first batch of %d is %d, want %d", tc.f, tc.minBatch, tc.d, got, tc.want)
		}
	}
	if got := (srepFactor{7, 4}).String(); got != "1.75" {
		t.Errorf("7/4 prints as %q", got)
	}
}

// TestSREPWireSizes checks the model's message sizes against the delimited
// encodings of the same messages built with the wire encoders, which is what
// bytes_metadata counts.
func TestSREPWireSizes(t *testing.T) {
	t.Parallel()

	rng := newRand(5)
	for _, root := range []fingerprint.Range{{}, rangeOf("1011001110")} {
		w := newSREPWire(root, serviceLeafBits)
		size := func(m protobuf.Message) int { return len(mustMarshalDelimited(t, m)) }
		msg := func(p *pb.Part) *pb.Message { return &pb.Message{Parts: []*pb.Part{p}} }
		digest := fingerprint.Summary{Count: 1}
		digest.Digest[0] = 1
		for _, count := range []int{1, 300, 736, 1 << 20} {
			sum := digest
			sum.Count = uint64(count)
			if got, want := w.fp(count), size(msg(fpPart(root, sum))); got != want {
				t.Errorf("root %v count %d: fingerprint %d B, encoded %d B", root, count, got, want)
			}
			st := rangesync.Status{Root: root, Summary: sum, LeafBits: serviceLeafBits, Mapping: fingerprint.DefaultMapperID}
			if got, want := w.status(count), size(rangesync.EncodeStatus(st)); got != want {
				t.Errorf("root %v count %d: status %d B, encoded %d B", root, count, got, want)
			}
			for _, b := range []struct{ start, n int }{{0, 4}, {0, 66}, {66, 198}, {300, 5000}} {
				data := make([]byte, w.symbolData(count, b.start, b.n))
				sy := rangesync.Symbols{Range: root, Count: uint64(count), Start: uint64(b.start), Data: data}
				want := size(msg(&pb.Part{Kind: &pb.Part_Symbols{Symbols: rangesync.EncodeSymbols(sy)}}))
				if got := w.symbols(count, b.start, b.n); got != want {
					t.Errorf("root %v count %d: symbols [%d, +%d) %d B, encoded %d B", root, count, b.start, b.n, got, want)
				}
			}
		}
		for _, upTo := range []int{16, 264, 4096} {
			mo := rangesync.More{Range: root, UpTo: uint64(upTo)}
			if got, want := w.more(upTo), size(msg(&pb.Part{Kind: &pb.Part_More{More: rangesync.EncodeMore(mo)}})); got != want {
				t.Errorf("root %v: more up to %d %d B, encoded %d B", root, upTo, got, want)
			}
		}
		for _, k := range []int{0, 1, 20, 250, 1024} {
			ids := randIDs(rng, root, k)
			it := rangesync.Items{Range: root, IDs: ids}
			if got, want := w.items(k), size(msg(&pb.Part{Kind: &pb.Part_Items{Items: rangesync.EncodeItems(it)}})); got != want {
				t.Errorf("root %v: items of %d IDs %d B, encoded %d B", root, k, got, want)
			}
			if k > rangesync.MaxWantIDs {
				continue
			}
			want := &pb.Want{}
			for _, id := range ids {
				want.IDs = append(want.IDs, rangesync.EncodeID(id))
			}
			if got, enc := w.want(k), size(want); got != enc {
				t.Errorf("want of %d IDs %d B, encoded %d B", k, got, enc)
			}
		}
		if got, want := w.empty(), size(&pb.Message{}); got != want {
			t.Errorf("empty message %d B, encoded %d B", got, want)
		}
	}
}

// TestSREPSymbolBytes checks the model's closed-form size of a run of coded
// symbols (the expected length of the count varints, from the binomial count
// of each symbol) against the encoding of a real random set
// (fingerprint.AppendSymbolData): with the harness's counts every varint is
// nearly always one byte, with a large reserve mostly two.
func TestSREPSymbolBytes(t *testing.T) {
	t.Parallel()

	m := fingerprint.NewFloatMapper()
	w := newSREPWire(fingerprint.Range{}, serviceLeafBits)
	rng := newRand(11)
	for _, count := range []int{736, 20000} {
		const n = 512
		syms := make([]fingerprint.CodedSymbol, n)
		for range count {
			fingerprint.Encode(m, syms, 0, randID(rng, fingerprint.Range{}), 1)
		}
		for _, b := range []struct{ start, n int }{{0, 4}, {0, 64}, {0, 512}, {64, 448}} {
			enc := len(fingerprint.AppendSymbolData(nil, m, uint64(count), uint64(b.start), syms[b.start:b.start+b.n]))
			model := w.symbolData(count, b.start, b.n)
			// one byte per varint at most between a random set and the
			// expectation, and a fraction of a percent over a run
			if diff := math.Abs(float64(model - enc)); diff > math.Max(2, 0.005*float64(enc)) {
				t.Errorf("count %d, symbols [%d, +%d): model %d B, encoded %d B", count, b.start, b.n, model, enc)
			}
		}
	}
}

// TestSREPStream checks the model of one symbol stream: the first batch, a
// More up to GrowthFactor times the symbols received while the decoder needs
// more, and the end of the stream at its capacity.
func TestSREPStream(t *testing.T) {
	t.Parallel()

	p := srepParams{}.withDefaults()
	unlimited := rangesync.SessionSymbolBudget
	for _, tc := range []struct {
		growth, n0, k, capR int
		batches             []int
		decoded             bool
	}{
		{growth: 4, n0: 4, k: 1, capR: unlimited, batches: []int{4}, decoded: true},
		{growth: 4, n0: 4, k: 4, capR: unlimited, batches: []int{4}, decoded: true},
		{growth: 4, n0: 4, k: 5, capR: unlimited, batches: []int{4, 12}, decoded: true},
		{growth: 4, n0: 4, k: 17, capR: unlimited, batches: []int{4, 12, 48}, decoded: true},
		{growth: 2, n0: 4, k: 5, capR: unlimited, batches: []int{4, 4}, decoded: true},
		{growth: 2, n0: 66, k: 70, capR: unlimited, batches: []int{66, 66}, decoded: true},
		{growth: 4, n0: 10, k: 50, capR: 32, batches: []int{10, 22}, decoded: false},
		{growth: 4, n0: 4000, k: 4100, capR: fingerprint.RootSymbols, batches: []int{4000, 96}, decoded: false},
	} {
		p.GrowthFactor = tc.growth
		got := p.stream(tc.n0, tc.k, tc.capR)
		symbols := 0
		for _, b := range tc.batches {
			symbols += b
		}
		if fmt.Sprint(got.batches) != fmt.Sprint(tc.batches) || got.decoded != tc.decoded || got.symbols != symbols || got.mores() != len(tc.batches)-1 {
			t.Errorf("growth %d, first batch %d, threshold %d, cap %d: %+v, want batches %v (%d symbols), decoded %v",
				tc.growth, tc.n0, tc.k, tc.capR, got, tc.batches, symbols, tc.decoded)
		}
	}
}

// TestSREPDecodeTable checks the Monte Carlo thresholds of the real decoder
// against T2.6's measurements (p50 16 at d = 10, 68 at d = 45) and §13's band
// for the mean symbols per difference ([1.3, 1.8]).
func TestSREPDecodeTable(t *testing.T) {
	t.Parallel()

	table := newSREPDecodeTable(srepCheckDraws, 1)
	if ks := table.thresholds(t, 0); len(ks) != 1 || ks[0] != 1 {
		t.Fatalf("thresholds of no difference %v, want [1]", ks)
	}
	for _, tc := range []struct{ d, lo, hi int }{{10, 13, 19}, {45, 62, 74}} {
		ks := table.thresholds(t, tc.d)
		if p50 := ks[len(ks)/2]; p50 < tc.lo || p50 > tc.hi {
			t.Errorf("d = %d: p50 %d symbols, want within [%d, %d] (T2.6)", tc.d, p50, tc.lo, tc.hi)
		}
	}
	for _, d := range []int{10, 45, 100} {
		if r := table.meanThreshold(t, d) / float64(d); r < 1.3 || r > 1.8 {
			t.Errorf("d = %d: %.2f symbols per difference, want within [1.3, 1.8] (§13)", d, r)
		}
	}
	// every threshold is at least the difference: a decoder recovers at most
	// one ID per symbol, and the stop signal is symbol 0 after them
	for _, d := range []int{1, 4, 20} {
		if ks := table.thresholds(t, d); ks[0] < d {
			t.Errorf("d = %d: threshold %d below the difference", d, ks[0])
		}
	}
}

// TestSREPSessionCost checks the expected cost of one session's stream
// (srepParams.sessionCost) per difference size, over the decoder's
// thresholds. With an exact estimate (a hint equal to the difference) the
// first batch FirstBatch(d) = ⌈1.6 d⌉ + 2 lies above the median threshold
// (T2.6: p50 68 and p90 79 at d = 45, against 74), so most first batches
// decode (85% at d = 45) and a stream costs 2.0–2.4 symbols per difference
// for d ≥ 45; BatchFactor 2.0 decodes nearly all of them. The count estimate
// alone (|cA − cB| = |2X − d|, X binomial(d, 1/2)) is far below d, and the
// Mores it takes cost 3.3–3.6 symbols per difference with GrowthFactor 4
// and 2.1–2.4 with 2. It logs the table, with the oracle that stops at the
// threshold.
func TestSREPSessionCost(t *testing.T) {
	t.Parallel()

	table := newSREPDecodeTable(srepCheckDraws, 4)
	w := newSREPWire(fingerprint.Range{}, serviceLeafBits)
	count, capR := 736, rangesync.SessionSymbolBudget
	for _, c := range []struct {
		name         string
		growth       int
		bf           srepFactor
		noMoreAt45   [2]float64
		perDiffExact [2]float64
	}{
		{name: "now", growth: rangesync.GrowthFactor, bf: srepFactor{rangesync.BatchFactorNum, rangesync.BatchFactorDen}, noMoreAt45: [2]float64{0.7, 0.95}, perDiffExact: [2]float64{2, 2.8}},
		{name: "GrowthFactor 2", growth: 2, bf: srepFactor{16, 10}, noMoreAt45: [2]float64{0.7, 0.95}, perDiffExact: [2]float64{1.6, 2.2}},
		{name: "BatchFactor 2.0", growth: 4, bf: srepFactor{2, 1}, noMoreAt45: [2]float64{0.9, 1}, perDiffExact: [2]float64{1.9, 2.4}},
	} {
		p := srepParams{GrowthFactor: c.growth, BatchFactor: c.bf}.withDefaults()
		var rows []string
		for _, d := range []int{1, 4, 10, 20, 45, 80} {
			exact := p.sessionCost(t, table, w, d, d, count, capR)
			var counted float64
			for x, px := range srepBinomial(d, 0.5) {
				counted += px * p.sessionCost(t, table, w, d, max(2*x-d, d-2*x, 1), count, capR).Symbols
			}
			rows = append(rows, fmt.Sprintf("d=%d: exact %.2f (no More %.0f%%, one %.0f%%, %.2f round trips), counts only %.2f, oracle %.2f",
				d, exact.Symbols/float64(d), 100*exact.MoreShares[0], 100*exact.MoreShares[1], exact.RoundTrips(), counted/float64(d), table.meanThreshold(t, d)/float64(d)))
			if d != 45 {
				continue
			}
			if got := exact.MoreShares[0]; got < c.noMoreAt45[0] || got > c.noMoreAt45[1] {
				t.Errorf("%s, d = 45, exact estimate: %.2f of the first batches decode, want within %v", c.name, got, c.noMoreAt45)
			}
			if got := exact.Symbols / 45; got < c.perDiffExact[0] || got > c.perDiffExact[1] {
				t.Errorf("%s, d = 45, exact estimate: %.2f symbols per difference, want within %v", c.name, got, c.perDiffExact)
			}
			if exact.Undecoded != 0 {
				t.Errorf("%s, d = 45: %.2f of the streams reach their capacity", c.name, exact.Undecoded)
			}
		}
		t.Logf("%s: symbols per difference\n  %s", c.name, strings.Join(rows, "\n  "))
	}
}

// TestSREPPullsync checks the pullsync baselines: AC-B1's N(N − 1) offers of
// 102 B per upload (the harness's), §7.8's 100 B, and the wire-level model
// of §2.3–2.4, which adds each Offer entry's framing and the Get, Offer and
// Want of each page.
func TestSREPPullsync(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		n          int
		acb1, s78  float64
		rate       float64
		minW, maxW float64
	}{
		{n: 9, acb1: 7344, s78: 7200, rate: 15.4, minW: 72 * 104, maxW: 72 * 125},
		{n: 17, acb1: 27744, s78: 27200, rate: 15.4, minW: 272 * 104, maxW: 272 * 125},
	} {
		if got := srepPullsyncACB1(tc.n); got != tc.acb1 {
			t.Errorf("N = %d: AC-B1 baseline %v B, want %v", tc.n, got, tc.acb1)
		}
		if got := srepPullsync78(tc.n); got != tc.s78 {
			t.Errorf("N = %d: §7.8 baseline %v B, want %v", tc.n, got, tc.s78)
		}
		if got := srepPullsyncWire(tc.n, tc.rate); got < tc.minW || got > tc.maxW {
			t.Errorf("N = %d: wire-level pullsync %v B per upload, want within [%v, %v]", tc.n, got, tc.minW, tc.maxW)
		}
	}
	// pages fill at higher rates, so the per-page overhead per offer falls
	if lo, hi := srepPullsyncWire(9, 100), srepPullsyncWire(9, 1); lo >= hi {
		t.Errorf("wire-level pullsync %v B at 100/s, not below %v B at 1/s", lo, hi)
	}
}

// TestSREPIdeal checks the closed form of §7.8 (synchronous rounds, each
// pair one session per round on the round's snapshot): n_Δ per upload is
// exactly p(N − p), an oracle that sends exactly the symbols the decoder
// needs costs §4.3.7's per-difference cost, and the current constants cost
// more than that.
func TestSREPIdeal(t *testing.T) {
	t.Parallel()

	table := newSREPDecodeTable(srepCheckDraws, 2)
	for _, n := range []int{9, 17} {
		p := srepParamsOf(steadyOptions(n, 1))
		perRound := p.uploadRate() * p.MinRoundInterval.Seconds()
		id := p.ideal(t, table, perRound)
		if want := float64(p.Replication * (n - p.Replication)); math.Abs(id.DiffPerUpload-want) > 1e-6*want {
			t.Errorf("N = %d: n_Δ per upload %v, want p(N − p) = %v", n, id.DiffPerUpload, want)
		}
		if id.OracleSymbolsPerDiff < 1.3 || id.OracleSymbolsPerDiff > 1.8 {
			t.Errorf("N = %d: oracle %.2f symbols per difference, want within [1.3, 1.8]", n, id.OracleSymbolsPerDiff)
		}
		if id.OracleShare() >= id.Share() {
			t.Errorf("N = %d: the oracle's share %.2f not below the constants' %.2f", n, id.OracleShare(), id.Share())
		}
		if id.SymbolsPerDiff <= id.OracleSymbolsPerDiff {
			t.Errorf("N = %d: %.2f symbols per difference with the constants, not above the oracle's %.2f", n, id.SymbolsPerDiff, id.OracleSymbolsPerDiff)
		}
		p.GrowthFactor = 2
		if g2 := p.ideal(t, table, perRound); g2.SymbolsPerDiff >= id.SymbolsPerDiff {
			t.Errorf("N = %d: GrowthFactor 2 costs %.2f symbols per difference, not below GrowthFactor 4's %.2f", n, g2.SymbolsPerDiff, id.SymbolsPerDiff)
		}
		t.Logf("N = %d: %s", n, id)
	}
}

// TestSREPModel runs the model at §7.8's sizes in the harness's steady mode
// (steadyOptions) with a 50 ms latency and checks its invariants: every
// upload reaches every node within the harness's bound, each node fetches
// each upload it lacked exactly once (no duplicate with claims), the
// sessions cover at least p(N − p) differences per upload, and every stream
// stays within the model's domain (no stream reaches its capacity). It logs
// the model's figures for the current constants and the G6 candidates.
func TestSREPModel(t *testing.T) {
	t.Parallel()

	table := newSREPDecodeTable(srepCheckDraws, 3)
	for _, n := range []int{9, 17} {
		o := steadyOptions(n, 1)
		o.Delay = 50 * time.Millisecond
		p := srepParamsOf(o)
		res := p.run(t, table)
		bound := convergenceBound + (3+(8+2*3)+4)*o.Delay
		if res.MaxConvergence() > bound {
			t.Errorf("N = %d: an upload converged in %v, bound %v", n, res.MaxConvergence(), bound)
		}
		if res.Duplicates != 0 {
			t.Errorf("N = %d: %d duplicate deliveries with claims", n, res.Duplicates)
		}
		if want := res.Uploads * (n - p.Replication); res.Fetched != want {
			t.Errorf("N = %d: %d chunks fetched, want %d (each upload by its N − p lackers)", n, res.Fetched, want)
		}
		if lo := float64(p.Replication * (n - p.Replication)); res.DiffPerUpload() < lo {
			t.Errorf("N = %d: n_Δ per upload %.1f, below p(N − p) = %v", n, res.DiffPerUpload(), lo)
		}
		if res.Outside != 0 {
			t.Errorf("N = %d: %d streams outside the model's domain", n, res.Outside)
		}
		total := 0.0
		for _, b := range res.Bytes {
			total += b
		}
		if math.Abs(total-res.Metadata()) > 1e-6 {
			t.Errorf("N = %d: metadata %v B, parts sum to %v B", n, res.Metadata(), total)
		}
		t.Logf("N = %d: %s", n, res)

		// without claims, two sessions that decode the same chunk before
		// the first fetch stored it fetch it twice
		p.NoClaims = true
		nc := p.run(t, table)
		if nc.Duplicates == 0 {
			t.Errorf("N = %d without claims: no duplicate delivery", n)
		}
		t.Logf("N = %d without claims: %d duplicate deliveries (%.2f per upload)", n, nc.Duplicates, float64(nc.Duplicates)/float64(nc.Uploads))
	}
}
