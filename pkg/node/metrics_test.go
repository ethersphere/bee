// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"sync/atomic"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/prometheus/client_golang/prometheus"
)

// isFullySynced returns the bee_node_is_fully_synced gauge that the node's
// metrics report, as the node registers them.
func isFullySynced(t *testing.T, reg *prometheus.Registry) float64 {
	t.Helper()

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() == "bee_node_is_fully_synced" {
			return mf.GetMetric()[0].GetGauge().GetValue()
		}
	}
	t.Fatal("bee_node_is_fully_synced not reported")
	return 0
}

// TestIsFullySyncedGauge (T6.5 part 2): the is_fully_synced gauge is
// registered with the node's metrics and reads 0 until a full node sets its
// sync check (light and boot nodes never do), then follows the check each
// time it is collected.
func TestIsFullySyncedGauge(t *testing.T) {
	t.Parallel()

	m := node.NewMetrics()
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(node.GetMetrics(m)...)
	if got := isFullySynced(t, reg); got != 0 {
		t.Fatalf("gauge %v with no sync check set, want 0", got)
	}

	var synced atomic.Bool
	node.SetFullySynced(m, synced.Load)
	for _, want := range []bool{false, true, true, false, true} {
		synced.Store(want)
		got := isFullySynced(t, reg)
		if (got == 1) != want || (got != 0 && got != 1) {
			t.Fatalf("gauge %v with the sync check %v", got, want)
		}
	}
}
