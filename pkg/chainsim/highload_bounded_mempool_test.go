// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build scenario

package chainsim_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/chainsim"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/stretchr/testify/require"
)

// TestHighload_BoundedMempoolEviction verifies that with a small mempool and high
// background tips, SendWithRetry escalates tiers and eventually includes txs.
func TestHighload_BoundedMempoolEviction(t *testing.T) {
	duration := stressDuration()
	workers := envInt("HIGHLOAD_WORKERS", 10)
	rotateInterval := envDuration("HIGHLOAD_ROTATE", 30*time.Second)
	dumpInterval := envDuration("HIGHLOAD_DUMP", 10*time.Second)

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 50 * time.Millisecond
	cfg.BlockPeriodJitter = 20 * time.Millisecond
	cfg.InitialCongestion = 0.4
	cfg.CongestionStdDev = 0.1
	cfg.MaxMempoolSize = 64
	cfg.MaxTxsPerBlock = 4
	cfg.BackgroundTipMean = big.NewInt(5_000_000_000)
	cfg.BackgroundTipStdDev = big.NewInt(1_000_000_000)
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	cfg.RNGSeed = 42
	cfg.HistoryRetentionBlocks = 3000

	retryCfg := defaultRetryCfg()
	retryCfg.StartTier = "low"
	retryCfg.EndTier = "aggressive"
	retryCfg.AttemptsPerTier = 3
	retryCfg.MaxTxPrice = big.NewInt(100_000_000_000_000)

	baseline := goroutineBaseline()
	env := setupHighload(t, "highload_bounded_mempool", cfg, retryCfg, rotateInterval)

	oc := newOutcomeCounters()
	runHighloadWithWorker(t, env, duration, workers, dumpInterval, func(ctx context.Context, stop <-chan struct{}, svc transaction.Service) {
		instrumentedWorker(ctx, stop, svc, oc, env.wl)
	})

	require.Positive(t, oc.completed.Load(), "escalation did not break through bounded mempool")

	total := oc.completed.Load() + oc.get("attempts_exhausted") + oc.get("max_price_exceeded") + oc.get("other")
	if total > 0 {
		rate := float64(oc.completed.Load()) / float64(total)
		require.Greater(t, rate, 0.10, "completion rate too low under mempool pressure")
	}

	require.Empty(t, env.store.KeysWithPrefix(retryPrefix))
	check := checkNonces(collectNonces(t, env.store))
	require.False(t, check.dup, "duplicate nonce under eviction")

	assertGoroutinesSettled(t, baseline, 10, 60*time.Second)
	t.Logf("bounded-mempool: completed=%d outcomes=%v", oc.completed.Load(), oc.snapshot())
}
