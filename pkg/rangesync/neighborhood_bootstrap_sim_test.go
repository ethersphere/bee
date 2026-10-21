// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build sim

package rangesync_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The scaled AC-B2 acceptance of T4.7 part 1, behind the sim build tag:
//
//	go test ./pkg/rangesync/ -tags=sim -run 'TestG6BootstrapScaled|TestG6BootstrapTail' -v -timeout 180m

// bootstrapRow formats one run of a scaled bootstrap for the tables below.
func bootstrapRow(f bootstrapFigures, cells ...string) []string {
	return append(cells,
		fmt.Sprint(f.rank), fmt.Sprintf("%.2f", f.res.CaughtUp.Seconds()),
		fmt.Sprintf("%.0f (%.0f)", f.metaIn/1000, f.bound/1000),
		fmt.Sprintf("%.3f", f.metaIn/((f.bound-1e6)/1.15)),
		fmt.Sprintf("%.0f, %.0f, %.0f", float64(f.own)/1000, float64(f.peers)/1000, float64(f.status)/1000),
		fmt.Sprintf("%d (%d); %d, max %d", f.peerSessions, f.takeovers, f.wants, f.maxWants),
		fmt.Sprintf("%d, %d", f.ownStripes, f.peerStripes),
		fmt.Sprintf("%.0f, %.0f", f.stripesDone, f.stripesTimeout),
		fmt.Sprint(f.res.Duplicates), f.res.Wall.Round(time.Second).String())
}

var bootstrapHeader = []string{
	"rank", "caught up s", "node 0 metadata in KB (AC-B2 bound)", "÷ (Lag × 70 B)",
	"read KB: node 0's sessions, peers', Status", "peers' sessions with node 0 (takeovers); wants, largest",
	"stripe sessions: node 0's, peers'", "stripes done, timed out", "duplicates", "wall",
}

// TestG6BootstrapScaled is the scaled acceptance of the AC-B2 fix: N = 5,
// node 0 empty of Base 70000 or 100000 chunks, LargeDiff Base/64 (1093 and
// 1562), Delay 50 ms, seeds 1–4 for node 0's rank. "lifted" runs with the
// node-wide fetch limit lifted, as TestG6BootstrapPeers, so the transfer
// takes only its message legs on the clock (15–20 s; the takeovers of the
// higher-overlay peers fire only past ~2 × MinRoundInterval + jitter); "2 min"
// adds a node-wide fetch rate on the clock of Base/120 chunks per second, a
// transfer of about two minutes, through which the takeovers fire at every
// Base. Each run checks bootstrapFigures.check.
func TestG6BootstrapScaled(t *testing.T) {
	const seeds = 4
	tbl := &g6Table{header: append([]string{"Base", "fetch", "seed"}, bootstrapHeader...)}
	for _, base := range []int{70000, 100000} {
		for _, rate := range []int{0, base / 120} {
			name := "lifted"
			if rate > 0 {
				name = "2 min"
			}
			for seed := range uint64(seeds) {
				t.Run(fmt.Sprintf("base=%d/fetch=%s/seed=%d", base, name, seed+1), func(t *testing.T) {
					o := scaledBootstrap(base, seed+1)
					o.FetchClockRate = rate
					f := runBootstrap(t, o)
					tbl.add(bootstrapRow(f, fmt.Sprint(base), name, fmt.Sprint(seed+1))...)
					f.check(t)
				})
			}
		}
	}
	t.Logf("%s; N = 5, node 0 empty, LargeDiff Base/64, Delay 50 ms; counted from the start:%s", g6Label(), tbl)
}

// TestG6BootstrapTail measures the tail of a scaled bootstrap at the
// default fetch rate's duration (T4.7 part 1, the orchestrator's point 4):
// a fetch rate on the clock of Base/4200 chunks per second takes the transfer
// ~70 minutes, as 2^22 at 1000 per second, so its tail (the last LargeDiff
// IDs, where node 0's root difference is below LargeDiff and the sessions
// with it descend) lasts ~65 s, 13 minimum round intervals. Base 16000, so
// LargeDiff 250 and 3 chunks per second (the transfer ~89 minutes). Without
// a longer stripe timeout the stripes (4000 IDs at about 0.75 per second)
// outlast StripeTimeout and restart; "3 h" lifts it to show the tail alone.
// "highest" puts node 0 at the highest overlay (rank 4: every peer
// initiates with it every round, the worst case for the tail).
func TestG6BootstrapTail(t *testing.T) {
	const base = 16000
	highest := func(i int, a swarm.Address) swarm.Address {
		if i == 0 {
			b := a.Bytes()
			b[0], b[1] = 0xff, 0xff
			return swarm.NewAddress(b)
		}
		return a
	}
	tbl := &g6Table{header: append([]string{"stripe timeout", "seed"}, bootstrapHeader...)}
	for _, row := range []struct {
		timeout time.Duration
		seed    uint64
		highest bool
	}{
		{0, 1, false}, {0, 2, false}, {3 * time.Hour, 1, false}, {3 * time.Hour, 2, false}, {3 * time.Hour, 1, true},
	} {
		name := "default"
		if row.timeout > 0 {
			name = row.timeout.String()
		}
		seed := fmt.Sprint(row.seed)
		if row.highest {
			seed += ", highest"
		}
		t.Run(fmt.Sprintf("timeout=%s/seed=%s", name, seed), func(t *testing.T) {
			o := scaledBootstrap(base, row.seed)
			o.FetchClockRate = base / 4200
			o.Limit = 4 * time.Hour
			o.Reconciler = func(ro *reconciler.Options) { ro.StripeTimeout = row.timeout }
			if row.highest {
				o.Overlay = highest
			}
			f := runBootstrap(t, o)
			tbl.add(bootstrapRow(f, name, seed)...)
			t.Log(f)
		})
	}
	t.Logf("%s; N = 5, node 0 empty, Base %d, LargeDiff %d, fetch %d per second on the clock, Delay 50 ms:%s", g6Label(), base, base/64, base/4200, tbl)
}
