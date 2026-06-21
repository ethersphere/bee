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

// TestHighload_ProbabilisticInclusion verifies eventual inclusion under probabilistic
// block selection with low starting fee tier.
func TestHighload_ProbabilisticInclusion(t *testing.T) {
	duration := stressDuration()
	workers := envInt("HIGHLOAD_WORKERS", 8)
	rotateInterval := envDuration("HIGHLOAD_ROTATE", 30*time.Second)
	dumpInterval := envDuration("HIGHLOAD_DUMP", 10*time.Second)

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 50 * time.Millisecond
	cfg.BlockPeriodJitter = 20 * time.Millisecond
	cfg.InitialCongestion = 0.3
	cfg.InclusionProbability = true
	cfg.InclusionMinProbability = 0.1
	cfg.BackgroundTipMean = big.NewInt(3_000_000_000)
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	cfg.RNGSeed = 42
	cfg.HistoryRetentionBlocks = 2000

	retryCfg := defaultRetryCfg()
	retryCfg.StartTier = "low"
	retryCfg.EndTier = "aggressive"

	baseline := goroutineBaseline()
	env := setupHighload(t, "highload_probabilistic", cfg, retryCfg, rotateInterval)

	oc := newOutcomeCounters()
	runHighloadWithWorker(t, env, duration, workers, dumpInterval, func(ctx context.Context, stop <-chan struct{}, svc transaction.Service) {
		instrumentedWorker(ctx, stop, svc, oc, env.wl)
	})

	stats := env.sim.Stats()
	require.Positive(t, stats.InclusionDeferred, "probabilistic inclusion condition not exercised")
	require.Positive(t, oc.completed.Load(), "no txs completed under probabilistic inclusion")

	require.Empty(t, env.store.KeysWithPrefix(retryPrefix))
	require.Empty(t, env.store.KeysWithPrefix(pendingPrefix))

	check := checkNonces(collectNonces(t, env.store))
	require.False(t, check.dup)

	assertGoroutinesSettled(t, baseline, 10, 30*time.Second)
	t.Logf("probabilistic: deferred=%d completed=%d outcomes=%v",
		stats.InclusionDeferred, oc.completed.Load(), oc.snapshot())
}
