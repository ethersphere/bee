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

// TestHighload_ReceiptDelayNoFalseCancel verifies that with ReceiptAvailDelay=4 and
// cancellationDepth=6 the monitor does not falsely cancel transactions under load.
func TestHighload_ReceiptDelayNoFalseCancel(t *testing.T) {
	duration := stressDuration()
	workers := envInt("HIGHLOAD_WORKERS", 8)
	rotateInterval := envDuration("HIGHLOAD_ROTATE", 30*time.Second)
	dumpInterval := envDuration("HIGHLOAD_DUMP", 10*time.Second)

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 50 * time.Millisecond
	cfg.BlockPeriodJitter = 20 * time.Millisecond
	cfg.InitialCongestion = 0.3
	cfg.ReceiptAvailDelay = 4
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	cfg.RNGSeed = 42
	cfg.HistoryRetentionBlocks = 2000

	retryCfg := defaultRetryCfg()
	retryCfg.RetryDelay = 500 * time.Millisecond

	opt := withCancellationDepth(6)
	baseline := goroutineBaseline()
	env := setupHighload(t, "highload_receipt_delay_ok", cfg, retryCfg, rotateInterval, opt)

	oc := newOutcomeCounters()
	runHighloadWithWorker(t, env, duration, workers, dumpInterval, func(ctx context.Context, stop <-chan struct{}, svc transaction.Service) {
		instrumentedWorker(ctx, stop, svc, oc, env.wl)
	})

	out := oc.snapshot()
	require.LessOrEqual(t, out["cancelled"], int64(0),
		"false cancellations with cancellationDepth >= ReceiptAvailDelay")
	require.Positive(t, oc.completed.Load())

	require.Empty(t, env.store.KeysWithPrefix(retryPrefix))
	check := checkNonces(collectNonces(t, env.store))
	require.False(t, check.dup)

	assertGoroutinesSettled(t, baseline, 10, 30*time.Second)
	t.Logf("receipt-delay-ok: outcomes=%v completed=%d", out, oc.completed.Load())
}

// TestHighload_ReceiptDelayMisconfig is a negative test: cancellationDepth < ReceiptAvailDelay
// should produce false ErrTransactionCancelled outcomes.
func TestHighload_ReceiptDelayMisconfig(t *testing.T) {
	duration := envDuration("STRESS_DURATION", 2*time.Minute)
	workers := envInt("HIGHLOAD_WORKERS", 4)
	rotateInterval := envDuration("HIGHLOAD_ROTATE", 30*time.Second)
	dumpInterval := envDuration("HIGHLOAD_DUMP", 10*time.Second)

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 50 * time.Millisecond
	cfg.InitialCongestion = 0.3
	cfg.ReceiptAvailDelay = 4
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	cfg.RNGSeed = 42

	retryCfg := defaultRetryCfg()
	retryCfg.RetryDelay = 500 * time.Millisecond

	opt := withCancellationDepth(2)
	env := setupHighload(t, "highload_receipt_delay_bad", cfg, retryCfg, rotateInterval, opt)

	oc := newOutcomeCounters()
	runHighloadWithWorker(t, env, duration, workers, dumpInterval, func(ctx context.Context, stop <-chan struct{}, svc transaction.Service) {
		instrumentedWorker(ctx, stop, svc, oc, env.wl)
	})

	out := oc.snapshot()
	require.Positive(t, out["cancelled"],
		"expected false cancellations when cancellationDepth < ReceiptAvailDelay")
	t.Logf("receipt-delay-bad (negative): outcomes=%v", out)
}
