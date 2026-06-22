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

// TestHighload_BlockJitterVsRetryDelay verifies variable block timing does not
// break receipt waiting or cause runaway replacements.
//
// Goal: Confirm SendWithRetry tolerates irregular block production with a short
// retry delay without excessive RBF churn.
//
// How it works: Fast blocks with large jitter and a short RetryDelay; sustained
// worker load; checks completion, nonce integrity, and bounded mempool replacements.
func TestHighload_BlockJitterVsRetryDelay(t *testing.T) {
	duration := stressDuration()
	workers := envInt("HIGHLOAD_WORKERS", 10)
	rotateInterval := envDuration("HIGHLOAD_ROTATE", 30*time.Second)
	dumpInterval := envDuration("HIGHLOAD_DUMP", 10*time.Second)

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 60 * time.Millisecond
	cfg.BlockPeriodJitter = 50 * time.Millisecond
	cfg.InitialCongestion = 0.3
	cfg.CongestionStdDev = 0.05
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	cfg.RNGSeed = 42
	cfg.HistoryRetentionBlocks = 2000

	retryCfg := defaultRetryCfg()
	retryCfg.RetryDelay = 80 * time.Millisecond

	baseline := goroutineBaseline()
	env := setupHighload(t, "highload_jitter", cfg, retryCfg, rotateInterval)

	runHighload(t, env, duration, workers, dumpInterval)

	require.Positive(t, env.wl.completed.Load())

	check := checkNonces(collectNonces(t, env.store))
	require.False(t, check.dup, "duplicate nonce under jitter")

	stats := env.sim.Stats()
	require.Less(t, stats.MempoolReplacements, uint64(env.wl.completed.Load())*10,
		"excessive RBF from RetryDelay vs block jitter mismatch")

	assertGoroutinesSettled(t, baseline, 10, 30*time.Second)
	t.Logf("jitter: completed=%d replacements=%d", env.wl.completed.Load(), stats.MempoolReplacements)
}
