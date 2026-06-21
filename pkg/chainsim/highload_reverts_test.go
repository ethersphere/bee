// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build scenario

package chainsim_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/chainsim"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/stretchr/testify/require"
)

// TestHighload_RevertsUnderLoad verifies that random on-chain reverts (status=0)
// are handled correctly under load: store is cleaned, nonces stay contiguous.
func TestHighload_RevertsUnderLoad(t *testing.T) {
	duration := stressDuration()
	workers := envInt("HIGHLOAD_WORKERS", 10)
	rotateInterval := envDuration("HIGHLOAD_ROTATE", 30*time.Second)
	dumpInterval := envDuration("HIGHLOAD_DUMP", 10*time.Second)

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 50 * time.Millisecond
	cfg.BlockPeriodJitter = 20 * time.Millisecond
	cfg.InitialCongestion = 0.4
	cfg.CongestionStdDev = 0.1
	cfg.RandomRevertRate = 0.2
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	cfg.RNGSeed = 42
	cfg.HistoryRetentionBlocks = 3000

	retryCfg := defaultRetryCfg()

	baseline := goroutineBaseline()
	env := setupHighload(t, "highload_reverts", cfg, retryCfg, rotateInterval)

	oc := newOutcomeCounters()
	runHighloadWithWorker(t, env, duration, workers, dumpInterval, func(ctx context.Context, stop <-chan struct{}, svc transaction.Service) {
		instrumentedWorker(ctx, stop, svc, oc, env.wl)
	})

	out := oc.snapshot()
	require.Positive(t, out["reverted"], "expected reverts at RandomRevertRate=0.2")
	require.Positive(t, out["success"], "expected successful txs alongside reverts")
	require.Zero(t, out["critical"], "unexpected critical errors")

	require.Empty(t, env.store.KeysWithPrefix(retryPrefix), "retry leak on reverts")
	require.Empty(t, env.store.KeysWithPrefix(pendingPrefix), "pending leak on reverts")

	stored := collectNonces(t, env.store)
	check := checkNonces(stored)
	require.False(t, check.dup, "duplicate nonce")
	require.False(t, check.gap, "nonce gap (reverts still consume nonce)")

	assertGoroutinesSettled(t, baseline, 10, 30*time.Second)
	t.Logf("reverts: outcomes=%v stored=%d", out, check.count)
}
