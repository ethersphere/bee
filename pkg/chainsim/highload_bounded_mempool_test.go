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

// TestHighload_BoundedMempoolEviction verifies fee escalation and eventual
// inclusion when the mempool is small and transactions compete for slots.
//
// Goal: Confirm SendWithRetry escalates tiers and completes transactions under
// mempool pressure from size limits, TTL eviction, and congestion.
//
// How it works: Workers share one sender on a congested sim with a tiny mempool;
// congestion waves add pressure; asserts evictions occur and enough txs complete.
func TestHighload_BoundedMempoolEviction(t *testing.T) {
	duration := stressDuration()
	workers := envInt("HIGHLOAD_WORKERS", 10)
	rotateInterval := envDuration("HIGHLOAD_ROTATE", 30*time.Second)
	dumpInterval := envDuration("HIGHLOAD_DUMP", 10*time.Second)

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 50 * time.Millisecond
	cfg.BlockPeriodJitter = 20 * time.Millisecond
	cfg.InitialCongestion = 0.6
	cfg.CongestionStdDev = 0.1
	cfg.MaxBaseFee = big.NewInt(500_000_000_000) // 500 gwei — realistic Gnosis ceiling
	// Mempool smaller than the number of concurrent pending nonces from workers:
	// workers produce nonce chains, so the mempool will fill up and evict.
	cfg.MaxMempoolSize = 6
	cfg.MaxTxsPerBlock = 2
	cfg.MempoolTTL = 30
	cfg.RNGSeed = 42
	cfg.HistoryRetentionBlocks = 3000

	retryCfg := defaultRetryCfg()
	retryCfg.StartTier = "low"
	retryCfg.EndTier = "aggressive"
	retryCfg.AttemptsPerTier = 3
	retryCfg.MaxTxPrice = big.NewInt(100_000_000_000_000)

	baseline := goroutineBaseline()

	calmAt := safeCalmAt(duration)
	opt := defaultHighloadOpts()
	opt.pendingDrainTimeout = 60 * time.Second
	env := setupHighloadWithOpts(t, "highload_bounded_mempool", cfg, retryCfg, rotateInterval, opt)

	stopDrivers := make(chan struct{})
	go congestionWaveDriver(stopDrivers, env.sim, 0.9, 0.4, 0.1, 3*time.Second, calmAt)

	oc := newOutcomeCounters()
	runHighloadWithWorker(t, env, duration, workers, dumpInterval, func(ctx context.Context, stop <-chan struct{}, svc transaction.Service) {
		instrumentedWorker(ctx, stop, svc, oc, env.wl)
	})
	close(stopDrivers)

	require.Positive(t, oc.completed.Load(), "escalation did not break through bounded mempool")

	total := oc.completed.Load() + oc.get("attempts_exhausted") + oc.get("max_price_exceeded") + oc.get("other")
	if total > 0 {
		rate := float64(oc.completed.Load()) / float64(total)
		require.Greater(t, rate, 0.05, "completion rate too low under mempool pressure")
	}

	stats := env.sim.Stats()
	require.Positive(t, stats.MempoolTTLExpirations+stats.TransactionsRejected,
		"mempool eviction/rejection never triggered — test misconfigured")

	require.Empty(t, env.store.KeysWithPrefix(retryPrefix))
	assertOnChainNonceChain(t, env.confirmedNonce, collectNonces(t, env.store), true)

	assertGoroutinesSettled(t, baseline, 10, 60*time.Second)
	t.Logf("bounded-mempool: completed=%d evictions=%d rejections=%d outcomes=%v",
		oc.completed.Load(), stats.MempoolTTLExpirations, stats.TransactionsRejected, oc.snapshot())
}
