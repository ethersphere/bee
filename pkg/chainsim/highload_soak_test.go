// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build scenario

package chainsim_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/chainsim"
	"github.com/stretchr/testify/require"
)

// TestHighload_SoakNoLeaks detects goroutine leaks, orphan store keys, and
// unbounded sim history growth over a long calm run.
//
// Goal: Confirm sustained operation does not leak resources or retain unbounded
// chain history beyond configured limits.
//
// How it works: Moderate worker load runs for the full stress duration without
// chaos drivers; checks store hygiene, history retention, and goroutine settlement.
func TestHighload_SoakNoLeaks(t *testing.T) {
	duration := stressDuration()
	workers := envInt("HIGHLOAD_WORKERS", 8)
	rotateInterval := envDuration("HIGHLOAD_ROTATE", 30*time.Second)
	dumpInterval := envDuration("HIGHLOAD_DUMP", 10*time.Second)

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 50 * time.Millisecond
	cfg.BlockPeriodJitter = 20 * time.Millisecond
	cfg.InitialCongestion = 0.2
	cfg.CongestionStdDev = 0.05
	cfg.MaxTxsPerBlock = 4
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	cfg.RNGSeed = 42
	cfg.HistoryRetentionBlocks = 2000

	retryCfg := defaultRetryCfg()

	baseline := goroutineBaseline()
	env := setupHighload(t, "highload_soak", cfg, retryCfg, rotateInterval)

	runHighload(t, env, duration, workers, dumpInterval)

	require.Empty(t, env.store.KeysWithPrefix(retryPrefix))
	require.Empty(t, env.store.KeysWithPrefix(pendingPrefix))

	storedKeys := env.store.KeysWithPrefix(storedPrefix)
	require.Equal(t, env.store.Len(), len(storedKeys), "unexpected non-stored keys remain")

	snap := env.sim.Snapshot()
	require.LessOrEqual(t, len(snap.Blocks), cfg.FeeHistoryDepth+2,
		"blocks not trimmed by FeeHistoryDepth")
	if snap.BlockNum > cfg.HistoryRetentionBlocks {
		maxReceipts := cfg.HistoryRetentionBlocks * uint64(cfg.MaxTxsPerBlock)
		require.LessOrEqual(t, uint64(len(snap.Receipts)), maxReceipts,
			"receipts not trimmed by HistoryRetentionBlocks")
	}

	assertGoroutinesSettled(t, baseline, 10, 60*time.Second)
	t.Logf("soak: completed=%d stored=%d blocks=%d", env.wl.completed.Load(), len(storedKeys), len(snap.Blocks))
}
