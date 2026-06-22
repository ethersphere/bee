// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build scenario

package chainsim_test

import (
	"context"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/chainsim"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/stretchr/testify/require"
)

// TestHighload_RPCChaos verifies SendWithRetry remains stable when RPC calls
// fail intermittently across backend methods.
//
// Goal: Confirm the retry loop survives random RPC faults without deadlocking,
// leaking store keys, or breaking the nonce chain.
//
// How it works: Continuous fault injection runs while many workers send under
// congestion; asserts completions, clean store, and on-chain nonce continuity.
func TestHighload_RPCChaos(t *testing.T) {
	duration := stressDuration()
	workers := envInt("HIGHLOAD_WORKERS", 16)
	rotateInterval := envDuration("HIGHLOAD_ROTATE", 30*time.Second)
	dumpInterval := envDuration("HIGHLOAD_DUMP", 10*time.Second)

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 50 * time.Millisecond
	cfg.BlockPeriodJitter = 25 * time.Millisecond
	cfg.InitialCongestion = 0.5
	cfg.CongestionStdDev = 0.1
	cfg.MaxTxsPerBlock = 2
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	cfg.RNGSeed = 42
	cfg.HistoryRetentionBlocks = 2000

	retryCfg := defaultRetryCfg()
	retryCfg.MaxTxPrice = big.NewInt(100_000_000_000_000)

	opt := defaultHighloadOpts()
	opt.backend = withFeeCapBackend(retryCfg.MaxTxPrice)

	baseline := goroutineBaseline()
	env := setupHighloadWithOpts(t, "highload_rpc_chaos", cfg, retryCfg, rotateInterval, opt)

	calmAt := safeCalmAt(duration)
	stopDrivers := make(chan struct{})
	go rpcFaultDriver(stopDrivers, env.sim, rand.New(rand.NewSource(99)), 500*time.Millisecond, calmAt)

	oc := newOutcomeCounters()
	runHighloadWithWorker(t, env, duration, workers, dumpInterval, func(ctx context.Context, stop <-chan struct{}, svc transaction.Service) {
		instrumentedWorker(ctx, stop, svc, oc, env.wl)
	})
	close(stopDrivers)

	out := oc.snapshot()
	require.Positive(t, oc.completed.Load(), "nothing completed under RPC chaos — possible deadlock")
	require.Empty(t, env.store.KeysWithPrefix(retryPrefix))
	require.Empty(t, env.store.KeysWithPrefix(pendingPrefix))

	assertOnChainNonceChain(t, env.confirmedNonce, collectNonces(t, env.store), true)

	if env.feeCap != nil {
		require.Zero(t, env.feeCap.violations.Load(), "gasFeeCap exceeded MaxTxPrice")
	}

	assertGoroutinesSettled(t, baseline, 10, 60*time.Second)
	t.Logf("rpc-chaos: outcomes=%v completed=%d", out, oc.completed.Load())
}
