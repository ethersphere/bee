// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build scenario

package chainsim_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/chainsim"
	"github.com/stretchr/testify/require"
)

// TestHighload_StuckNonceMempoolDrop verifies that after mempool TTL eviction and
// base-fee storms that exhaust retries, the node recovers: pending/retry keys drain
// and the on-chain nonce catches up with stored transactions.
func TestHighload_StuckNonceMempoolDrop(t *testing.T) {
	duration := stressDuration()
	workers := envInt("HIGHLOAD_WORKERS", 8)
	rotateInterval := envDuration("HIGHLOAD_ROTATE", 30*time.Second)
	dumpInterval := envDuration("HIGHLOAD_DUMP", 10*time.Second)

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 50 * time.Millisecond
	cfg.BlockPeriodJitter = 20 * time.Millisecond
	cfg.InitialCongestion = 0.5
	cfg.CongestionStdDev = 0.1
	cfg.MempoolTTL = 40
	cfg.MaxTxsPerBlock = 2
	cfg.RNGSeed = 42
	cfg.HistoryRetentionBlocks = 2000

	retryCfg := defaultRetryCfg()
	retryCfg.MaxTxPrice = big.NewInt(50_000_000_000)

	baseline := goroutineBaseline()
	env := setupHighload(t, "highload_stuck_nonce", cfg, retryCfg, rotateInterval)

	calmAt := time.Now().Add(duration - quietTail())
	stopDrivers := make(chan struct{})
	go baseFeeStormDriver(stopDrivers, env.sim, big.NewInt(80_000_000_000), big.NewInt(1_000_000_000), 4*time.Second, calmAt)
	go congestionWaveDriver(stopDrivers, env.sim, 1.0, 0.2, 0.0, 5*time.Second, calmAt)

	runHighload(t, env, duration, workers, dumpInterval)
	close(stopDrivers)

	assertions := map[string]string{}

	require.Empty(t, env.store.KeysWithPrefix(retryPrefix), "retry state leak")
	assertions["retry_clean"] = "PASS"

	require.Empty(t, env.store.KeysWithPrefix(pendingPrefix), "pending leak -> possible stuck nonce")
	assertions["pending_clean"] = "PASS"

	confirmed := env.confirmedNonce

	stored := collectNonces(t, env.store)
	assertOnChainNonceChain(t, confirmed, stored, true)
	assertions["no_nonce_dup"] = "PASS (unique chain checked)"

	assertGoroutinesSettled(t, baseline, 10, 60*time.Second)
	assertions["goroutines_settled"] = "PASS"

	env.writeSummary(t, assertions)
	unique := uniqueNonces(stored)
	maxNonce := uint64(0)
	if len(unique) > 0 {
		maxNonce = unique[len(unique)-1]
	}
	t.Logf("stuck-nonce: confirmed=%d stored_unique_max=%d completed=%d", confirmed, maxNonce, env.wl.completed.Load())
}
