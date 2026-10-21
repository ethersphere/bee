// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build sim

package rangesync_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
)

// The model's G6 sweeps (T4.6 part 3), behind the sim build tag:
//
//	go test ./pkg/rangesync/ -tags=sim -run 'TestSREPModelG6Part3' -v -timeout 30m
//
// They cover what the harness needs a build per value for (the session
// constants on a grid), loads past the harness's reach (up to 200 uploads per
// second) and the streamed batches of Q3, which the frozen wire (G2) does
// not carry. The model is validated against the harness at a Delay
// (TestSREPModelVsHarness: the share within 3 points); its figures here are
// the model's alone where the tables say so.

// srepRuns runs the model for seeds seeds of p (Seed 1 …) and returns the
// results.
func srepRuns(t *testing.T, table *srepDecodeTable, p srepParams, seeds int) []*srepResult {
	t.Helper()
	rs := make([]*srepResult, 0, seeds)
	for seed := range uint64(seeds) {
		p.Seed = seed + 1
		rs = append(rs, p.run(t, table))
	}
	return rs
}

// srepIntQuantiles returns the quantiles of xs.
func srepIntQuantiles(xs []int) g6Quantiles {
	fs := make([]float64, len(xs))
	for i, x := range xs {
		fs[i] = float64(x)
	}
	return g6QuantilesOf(fs)
}

// TestSREPModelG6Part3 logs the model's G6 tables:
//   - constants: GrowthFactor 2, 3 and 4 against BatchFactor 1.2, 1.4, 1.6
//     and 2.0, with hint(R) and without, N = 9 and 17, Delay 50 and 200 ms,
//     in the harness's steady mode: the share, the Mores and the round trips
//     of a decoded session;
//   - rate: Poisson uploads from 15 to 200 per second: n_Δ and the wants per
//     session side against CatchUpMinDiff (Q9);
//   - round interval: MinRoundInterval 5 to 15 s at 15 and 50 uploads per
//     second: the share against the convergence;
//   - streamed: the request-driven streams against a responder that streams
//     until told to stop (Q3), from the same sessions' decoder thresholds.
func TestSREPModelG6Part3(t *testing.T) {
	table := newSREPDecodeTable(srepSimDraws, 11)
	const seeds = 10

	t.Run("constants", func(t *testing.T) {
		tbl := &g6Table{header: []string{"N", "Delay", "GrowthFactor", "BatchFactor", "hint", "share %", "symbols/diff", "Mores/upload", "round trips/decoded session", "convergence mean s"}}
		for _, n := range []int{9, 17} {
			for _, delay := range []time.Duration{50 * time.Millisecond, 200 * time.Millisecond} {
				for _, growth := range []int{2, 3, 4} {
					for _, bf := range []srepFactor{{12, 10}, {14, 10}, {16, 10}, {2, 1}} {
						for _, noHints := range []bool{false, true} {
							o := steadyOptions(n, 1)
							o.Delay, o.NoHints = delay, noHints
							p := srepParamsOf(o)
							p.GrowthFactor, p.BatchFactor = growth, bf
							rs := srepRuns(t, table, p, seeds)
							hint := "on"
							if noHints {
								hint = "off"
							}
							tbl.add(fmt.Sprint(n), delay.String(), fmt.Sprint(growth), bf.String(), hint,
								g6SpreadOf(rs, (*srepResult).Share).short(100, 1),
								g6SpreadOf(rs, (*srepResult).SymbolsPerDifference).short(1, 2),
								g6SpreadOf(rs, (*srepResult).MoresPerUpload).short(1, 2),
								g6SpreadOf(rs, (*srepResult).RoundTrips).short(1, 2),
								g6SpreadOf(rs, func(r *srepResult) float64 {
									conv := 0.0
									for _, b := range r.Batches {
										conv += b.Convergence.Seconds()
									}
									return conv / float64(len(r.Batches))
								}).short(1, 2))
						}
					}
				}
			}
		}
		t.Logf("model, steady mode, %d seeds, MinBatch %d; mean (range):%s", seeds, rangesync.MinBatch, tbl)
	})

	t.Run("rate", func(t *testing.T) {
		table := newSREPDecodeTable(srepSimDraws/4, 12)
		tbl := &g6Table{header: []string{"N", "uploads/s", "n_Δ p50, p99, max", "wants p50, p99, max", "sides ≥ CatchUpMinDiff %", "share %"}}
		for _, n := range []int{9, 17} {
			for _, rate := range []float64{15.4, 50, 100, 150, 200} {
				p := srepParams{Nodes: n, Replication: 3, Rate: rate, Duration: 60 * time.Second, Delay: 50 * time.Millisecond, Tail: 10 * time.Second}.withDefaults()
				rs := srepRuns(t, table, p, 4)
				var diffs, wants []int
				for _, r := range rs {
					diffs = append(diffs, r.Diffs...)
					wants = append(wants, r.Wants...)
				}
				over := 0
				for _, w := range wants {
					if w >= rangesync.CatchUpMinDiff {
						over++
					}
				}
				d, w := srepIntQuantiles(diffs), srepIntQuantiles(wants)
				tbl.add(fmt.Sprint(n), fmt.Sprint(rate), fmt.Sprintf("%.0f, %.0f, %.0f", d.P50, d.P99, d.Max), fmt.Sprintf("%.0f, %.0f, %.0f", w.P50, w.P99, w.Max),
					fmt.Sprintf("%.2f", 100*float64(over)/float64(max(len(wants), 1))), g6SpreadOf(rs, (*srepResult).Share).short(100, 1))
			}
		}
		t.Logf("model, Poisson uploads for 60 s, Delay 50 ms, the compiled constants, 4 seeds pooled; n_Δ per completed session, wants per session side:%s", tbl)
	})

	t.Run("round interval", func(t *testing.T) {
		// differences of up to several hundred: fewer draws per size
		table := newSREPDecodeTable(srepSimDraws/4, 13)
		tbl := &g6Table{header: []string{"N", "uploads/s", "GrowthFactor", "MinRoundInterval", "share %", "convergence p50, p99, max s", "n_Δ p50 per session", "wants p99"}}
		for _, n := range []int{9, 17} {
			for _, rate := range []float64{15.4, 50} {
				for _, growth := range []int{4, 2} {
					for _, lo := range []time.Duration{5 * time.Second, 7500 * time.Millisecond, 10 * time.Second, 15 * time.Second} {
						p := srepParams{Nodes: n, Replication: 3, Rate: rate, Duration: 60 * time.Second, Delay: 50 * time.Millisecond, Tail: 10 * time.Second, MinRoundInterval: lo}.withDefaults()
						p.GrowthFactor = growth
						rs := srepRuns(t, table, p, 4)
						var conv []time.Duration
						var diffs, wants []int
						for _, r := range rs {
							conv = append(conv, r.Convergence...)
							diffs = append(diffs, r.Diffs...)
							wants = append(wants, r.Wants...)
						}
						slices.Sort(conv)
						q := func(f float64) float64 { return conv[max(0, int(f*float64(len(conv)))-1)].Seconds() }
						tbl.add(fmt.Sprint(n), fmt.Sprint(rate), fmt.Sprint(growth), lo.String(), g6SpreadOf(rs, (*srepResult).Share).short(100, 1),
							fmt.Sprintf("%.1f, %.1f, %.1f", q(0.5), q(0.99), conv[len(conv)-1].Seconds()),
							fmt.Sprintf("%.0f", srepIntQuantiles(diffs).P50), fmt.Sprintf("%.0f", srepIntQuantiles(wants).P99))
					}
				}
			}
		}
		t.Logf("model, Poisson uploads for 60 s, Delay 50 ms, BatchFactor and MinBatch compiled, 4 seeds pooled; the convergence of each upload:%s", tbl)
	})

	t.Run("streamed", func(t *testing.T) {
		// A streamed responder sends its stream on at line rate until the
		// initiator's stop arrives: one round trip after the initiator decoded,
		// so it overshoots the threshold K by what the line carries in a round
		// trip, and saves the Mores' round trips. The request-driven streams'
		// symbols are the model's; the streamed ones K + rate × RTT per stream.
		const symbolBytes = 82
		type link struct {
			name string
			bps  float64 // bytes per second
			rtt  time.Duration
		}
		links := []link{{"100 KB/s, 100 ms", 100e3, 100 * time.Millisecond}, {"1 MB/s, 100 ms", 1e6, 100 * time.Millisecond}, {"1 MB/s, 400 ms", 1e6, 400 * time.Millisecond}, {"10 MB/s, 100 ms", 10e6, 100 * time.Millisecond}}
		header := make([]string, 0, 5+len(links))
		header = append(header, "N", "constants", "Mores/upload", "Mores per decoded stream (round trips a stream saves)", "symbols per stream: request-driven, oracle (K)")
		for _, l := range links {
			header = append(header, "streamed at "+l.name)
		}
		tbl := &g6Table{header: header}
		for _, n := range []int{9, 17} {
			for _, c := range []struct {
				name   string
				growth int
			}{{"GrowthFactor 4", 4}, {"GrowthFactor 2", 2}} {
				o := steadyOptions(n, 1)
				o.Delay = 50 * time.Millisecond
				p := srepParamsOf(o)
				p.GrowthFactor = c.growth
				rs := srepRuns(t, table, p, seeds)
				var symbols, ks, streams, mores, uploads float64
				for _, r := range rs {
					symbols += float64(r.Symbols)
					for _, k := range r.Thresholds {
						ks += float64(k)
					}
					streams += float64(len(r.Thresholds))
					mores += float64(r.Mores)
					uploads += float64(r.Uploads)
				}
				row := make([]string, 0, 5+len(links))
				row = append(row, fmt.Sprint(n), c.name, fmt.Sprintf("%.2f", mores/uploads), fmt.Sprintf("%.2f", mores/streams),
					fmt.Sprintf("%.0f, %.0f", symbols/streams, ks/streams))
				for _, l := range links {
					over := l.bps * l.rtt.Seconds() / symbolBytes
					extra := (ks + over*streams - symbols) * symbolBytes / uploads
					row = append(row, fmt.Sprintf("%.0f symbols, %+.1f KB/upload", ks/streams+over, extra/1000))
				}
				tbl.add(row...)
			}
		}
		t.Logf("model, steady mode, Delay 50 ms, hint(R) on, %d seeds; streamed: K + rate × RTT / %d B per stream, and its bytes per upload against the request-driven streams':%s", seeds, symbolBytes, tbl)
	})
}
