// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/cmd/pullsim/internal/sim"
)

func TestParseGrid(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []int
	}{
		{"10,20,30", []int{10, 20, 30}},
		{" 10 , 20 ", []int{10, 20}},
		{"5", []int{5}},
	} {
		got, err := parseGrid(tc.in)
		if err != nil {
			t.Fatalf("parseGrid(%q): %v", tc.in, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("parseGrid(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("parseGrid(%q) = %v, want %v", tc.in, got, tc.want)
			}
		}
	}
}

func TestParseGridRejectsBadInput(t *testing.T) {
	for _, in := range []string{"", "  ", "10,,20", "10,abc", "0", "-3", "10,0"} {
		if got, err := parseGrid(in); err == nil {
			t.Errorf("parseGrid(%q) = %v, want an error", in, got)
		}
	}
}

// CSV column indices, kept in one place so a header change breaks one spot.
const (
	colNodes = iota
	colChunks
	colRep
	colTopology
	colDegree
	colRadius
	colBins
	colLatencyMs
	colMaxPage
	colClusters
	colSeed
	colSettleMs
	colWarmupMs
	colSpanMs
	colInjectMs
	colTailMs
	colReplicas
	colNodesReached
	colLateReplicas
	colP50
	colP95
	colMax
	colStatus
)

const wantHeader = "nodes,chunks,rep,topology,degree,radius,bins,latencyMs,maxPage,clusters,seed," +
	"settleMs,warmupMs,spanMs,injectMs,tailMs,replicas,nodesReached,lateReplicas," +
	"perDeliveryP50Ms,perDeliveryP95Ms,perDeliveryMaxMs,status"

func okRow() benchRow {
	return benchRow{
		Nodes: 20, Chunks: 10, Rep: 2,
		Topology: "k-nearest", Degree: 6, Radius: 0, Bins: 8,
		LatencyMs: 5, MaxPage: 64, Clusters: 1, Seed: 42,
		SettleMs: 3000, WarmupMs: 5000,
		SpanMs: 1500, InjectMs: 0, TailMs: 1500,
		Replicas: 120, NodesReached: 19, LateReplicas: 0,
		PerDeliveryP50Ms: 400, PerDeliveryP95Ms: 1200, PerDeliveryMaxMs: 1450,
		Status: "ok",
	}
}

func TestBenchRowCSV(t *testing.T) {
	r := okRow()
	got := r.csv()
	header := benchHeader()
	if len(got) != len(header) {
		t.Fatalf("row has %d fields, header has %d", len(got), len(header))
	}
	if got[0] != "20" || got[1] != "10" || got[2] != "2" {
		t.Errorf("got leading fields %v, want [20 10 2]", got[:3])
	}
	if got[colStatus] != "ok" || colStatus != len(header)-1 {
		t.Errorf("got status %q at %d, want \"ok\" last", got[colStatus], colStatus)
	}
	if strings.Join(header, ",") != wantHeader {
		t.Errorf("unexpected header:\n got %s\nwant %s", strings.Join(header, ","), wantHeader)
	}
}

// I6: the settle and warmup windows are part of what makes runs comparable, so
// they have to travel with the row.
func TestBenchRowCSVCarriesSettleAndWarmup(t *testing.T) {
	got := okRow().csv()
	if got[colSettleMs] != "3000" {
		t.Errorf("got settleMs %q, want 3000", got[colSettleMs])
	}
	if got[colWarmupMs] != "5000" {
		t.Errorf("got warmupMs %q, want 5000", got[colWarmupMs])
	}
	if got[colSeed] != "42" {
		t.Errorf("got seed %q, want 42 (column drift)", got[colSeed])
	}
}

// C1a: any status other than "ok" means the timing is not a measurement, so
// spanMs/tailMs/percentiles come out blank rather than as a misleadingly small
// number. The observed counts stay real for every status.
func TestBenchRowCSVBlanksTimingForNonOkStatus(t *testing.T) {
	for _, status := range []string{"timeout", "no-replicas", "not-settled", "truncated", "error"} {
		r := okRow()
		r.Status = status
		r.Replicas, r.NodesReached, r.LateReplicas = 30, 4, 7
		got := r.csv()

		for _, c := range []struct {
			name string
			idx  int
		}{{"spanMs", colSpanMs}, {"tailMs", colTailMs}, {"p50", colP50}, {"p95", colP95}, {"max", colMax}} {
			if got[c.idx] != "" {
				t.Errorf("status %q: %s = %q, want empty", status, c.name, got[c.idx])
			}
		}
		if got[colReplicas] != "30" || got[colNodesReached] != "4" || got[colLateReplicas] != "7" {
			t.Errorf("status %q lost observed counts: %v", status, got)
		}
		if got[colStatus] != status {
			t.Errorf("got status %q, want %q", got[colStatus], status)
		}
	}

	got := okRow().csv()
	if got[colSpanMs] != "1500" || got[colTailMs] != "1500" {
		t.Errorf("ok row lost its span/tail: %v", got)
	}
	if got[colP50] != "400" || got[colP95] != "1200" || got[colMax] != "1450" {
		t.Errorf("ok row percentiles = %v, %v, %v, want 400, 1200, 1450", got[colP50], got[colP95], got[colMax])
	}
}

func TestBenchCellSeedsDiffer(t *testing.T) {
	// Repetitions of the same cell must not be identical runs.
	a := cellSeed(7, 20, 10, 0)
	b := cellSeed(7, 20, 10, 1)
	if a == b {
		t.Errorf("cellSeed collided across reps: %d", a)
	}
	if cellSeed(7, 20, 10, 0) != a {
		t.Error("cellSeed is not deterministic")
	}
}

// C1a/M3/M4: the verdict comes from the measurement, not only from the error.
func TestBenchStatus(t *testing.T) {
	settled := func(b sim.Batch) sim.Batch { b.Settled = true; return b }

	for _, tc := range []struct {
		name string
		b    sim.Batch
		err  error
		want string
	}{
		{
			name: "settled and replicated",
			b:    settled(sim.Batch{Replicas: 40, NodesReached: 9}),
			want: "ok",
		},
		{
			// The blocker: a batch nobody replicated settles perfectly
			// normally, its last put being the origin's own inject, so it
			// reports a span of ~0 and sorts as the fastest cell in the sweep.
			name: "settled but never replicated",
			b:    settled(sim.Batch{Replicas: 0}),
			want: "no-replicas",
		},
		{
			name: "replicas kept arriving after the window",
			b:    settled(sim.Batch{Replicas: 40, LateReplicas: 3}),
			want: "truncated",
		},
		{
			name: "wait returned without settling",
			b:    sim.Batch{Replicas: 40},
			want: "not-settled",
		},
		{
			// M3: only a real deadline is a timeout.
			name: "per-cell deadline",
			b:    sim.Batch{Replicas: 7},
			err:  context.DeadlineExceeded,
			want: "timeout",
		},
		{
			name: "wrapped deadline",
			b:    sim.Batch{},
			err:  fmt.Errorf("wait: %w", context.DeadlineExceeded),
			want: "timeout",
		},
		{
			name: "batch evicted from the tracker",
			b:    sim.Batch{},
			err:  errors.New("batch 3 no longer retained"),
			want: "error",
		},
		{
			name: "network closed mid-wait",
			b:    sim.Batch{Replicas: 4},
			err:  sim.ErrNetworkClosed,
			want: "error",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := benchStatus(tc.b, tc.err); got != tc.want {
				t.Errorf("benchStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

// C1a end to end through the CSV: a zero-replica cell must not be able to look
// like the fastest row in the sweep.
func TestBenchZeroReplicaCellIsNotTheFastestRow(t *testing.T) {
	b := sim.Batch{Settled: true, Replicas: 0}
	r := okRow()
	r.Status = benchStatus(b, nil)
	r.SpanMs, r.TailMs = 0, 0
	r.Replicas, r.NodesReached = 0, 0
	r.PerDeliveryP50Ms, r.PerDeliveryP95Ms, r.PerDeliveryMaxMs = 0, 0, 0

	got := r.csv()
	if got[colStatus] != "no-replicas" {
		t.Fatalf("got status %q, want no-replicas", got[colStatus])
	}
	if got[colSpanMs] != "" || got[colP50] != "" {
		t.Errorf("zero-replica row published a 0 span/percentile: span=%q p50=%q", got[colSpanMs], got[colP50])
	}
	if got[colReplicas] != "0" {
		t.Errorf("got replicas %q, want the real observed 0", got[colReplicas])
	}
}
