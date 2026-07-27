// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !race

// This test drives a real (if tiny) network through runBenchCell. It is
// excluded from -race runs for the same reason as internal/sim/network_test.go:
// concurrent pullsync handlers trip a benign, pre-existing data race inside the
// resenje.org/singleflight dependency.

package main

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/cmd/pullsim/internal/sim"
	"github.com/ethersphere/bee/v2/pkg/log"
)

// C1b + I6 end to end: a cell runs with a non-zero -bench-minpo, and the row it
// produces carries the settle/warmup windows it was measured under.
func TestRunBenchCellProducesAMeasuredRow(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and warms a real network")
	}

	opts := benchOptions{
		Base: sim.Config{
			Bins: 8, Topology: sim.TopologyFull, Degree: 2, Radius: 0,
			Latency: time.Millisecond, MaxPage: 64, Clusters: 1, Seed: 21,
		},
		Warmup: 3 * time.Second,
		Settle: 3 * time.Second,
		// Non-zero: the chunks must be mined into the origin's neighborhood.
		MinPO:   4,
		Timeout: 60 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Two nodes: one pullsync client per server, so no request coalescing.
	row, err := runBenchCell(ctx, opts, 2, 4, 0, log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "ok" {
		t.Fatalf("got status %q, want ok (row: %+v)", row.Status, row)
	}
	if row.Replicas == 0 || row.NodesReached == 0 {
		t.Errorf("got %d replicas over %d nodes, want the batch to have propagated", row.Replicas, row.NodesReached)
	}
	if row.SpanMs <= 0 {
		t.Errorf("got SpanMs %d, want > 0", row.SpanMs)
	}
	if row.LateReplicas != 0 {
		t.Errorf("got LateReplicas %d, want 0 for a 3s settle window", row.LateReplicas)
	}
	if row.SettleMs != 3000 || row.WarmupMs != 3000 {
		t.Errorf("got settleMs %d warmupMs %d, want 3000/3000", row.SettleMs, row.WarmupMs)
	}
	if row.Chunks != 4 || row.Nodes != 2 {
		t.Errorf("got nodes %d chunks %d, want 2/4", row.Nodes, row.Chunks)
	}

	// The row must survive into CSV with its timing intact.
	csv := row.csv()
	if csv[colStatus] != "ok" || csv[colSpanMs] == "" {
		t.Errorf("ok row lost its span in CSV: %v", csv)
	}
}
