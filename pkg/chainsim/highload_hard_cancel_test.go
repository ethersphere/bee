// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build scenario

package chainsim_test

import (
	"context"
	"math/big"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethersphere/bee/v2/pkg/chainsim"
	signermock "github.com/ethersphere/bee/v2/pkg/crypto/mock"
	"github.com/ethersphere/bee/v2/pkg/log"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/stretchr/testify/require"
)

// cancelChurnWorker cancels its context mid-retry to stress context.Canceled handling.
// Each worker gets its own rng seed to avoid data races on shared *rand.Rand.
func cancelChurnWorker(parent context.Context, stop <-chan struct{}, svc transaction.Service, oc *outcomeCounters, seed int64) {
	rng := rand.New(rand.NewSource(seed)) //nolint:gosec
	for {
		select {
		case <-stop:
			return
		case <-parent.Done():
			return
		default:
		}
		ctx, cancel := context.WithCancel(parent)
		delay := time.Duration(200+rng.Intn(600)) * time.Millisecond
		timer := time.AfterFunc(delay, cancel)

		_, _, err := svc.SendWithRetry(ctx, defaultHighloadTxRequest("cancel-churn"))
		timer.Stop()
		cancel()
		oc.record(err)
		if parent.Err() != nil {
			return
		}
	}
}

// TestHighload_HardCancelMidRetry verifies that context cancellation churn and
// service restarts do not corrupt nonce assignment or persistence.
//
// Goal: Ensure aggressive caller-side cancellation mid-retry remains safe when
// combined with periodic service restarts.
//
// How it works: Workers repeatedly cancel SendWithRetry contexts during flight;
// the service is restarted on a timer; asserts no duplicate nonces and a clean
// retry store at the end.
func TestHighload_HardCancelMidRetry(t *testing.T) {
	duration := stressDuration()
	workers := envInt("HIGHLOAD_WORKERS", 8)
	restartEvery := envDuration("CRASH_RESTART_EVERY", 90*time.Second)

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 50 * time.Millisecond
	cfg.BlockPeriodJitter = 20 * time.Millisecond
	cfg.InitialCongestion = 0.4
	cfg.CongestionStdDev = 0.1
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	cfg.RNGSeed = 42
	cfg.HistoryRetentionBlocks = 3000

	retryCfg := defaultRetryCfg()
	retryCfg.MaxTxPrice = big.NewInt(100_000_000_000_000)

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	sender := crypto.PubkeyToAddress(key.PublicKey)

	sim := chainsim.New(cfg)
	sim.SetBalance(sender, new(big.Int).Mul(big.NewInt(1e18), big.NewInt(100000)))
	simCtx, cancelSim := context.WithCancel(context.Background())
	go sim.Run(simCtx)
	defer func() {
		cancelSim()
		sim.Close()
	}()

	store := storemock.NewStateStore()
	insp := store.(storemock.InspectableStore)

	signer := signermock.New(
		signermock.WithSignTxFunc(func(tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
			return types.SignTx(tx, types.LatestSignerForChainID(chainID), key)
		}),
		signermock.WithEthereumAddressFunc(func() (common.Address, error) { return sender, nil }),
	)

	newSvc := func() (transaction.Service, transaction.Monitor) {
		mon := transaction.NewMonitor(log.Noop, sim, sender, 30*time.Millisecond, 2)
		svc, err := transaction.NewService(log.Noop, sender, sim, signer, store, cfg.ChainID, mon, 0, retryCfg)
		require.NoError(t, err)
		return svc, mon
	}

	baseline := goroutineBaseline()
	oc := newOutcomeCounters()
	deadline := time.Now().Add(duration)
	var workerSeed atomic.Int64
	workerSeed.Store(100)

	for time.Now().Before(deadline) {
		svc, mon := newSvc()
		stop := make(chan struct{})
		var wg sync.WaitGroup
		for range workers {
			wg.Add(1)
			seed := workerSeed.Add(1)
			go func() {
				defer wg.Done()
				cancelChurnWorker(context.Background(), stop, svc, oc, seed)
			}()
		}

		epoch := restartEvery
		if rem := time.Until(deadline); rem < epoch {
			epoch = rem
		}
		time.Sleep(epoch)

		close(stop)
		wg.Wait()
		require.NoError(t, svc.Close())
		require.NoError(t, mon.Close())
	}

	finalSvc, finalMon := newSvc()
	time.Sleep(15 * time.Second)
	require.NoError(t, finalSvc.Close())
	require.NoError(t, finalMon.Close())

	require.Empty(t, insp.KeysWithPrefix(retryPrefix))

	stored := collectNoncesFrom(insp)
	check := checkNonces(stored)
	require.False(t, check.dup, "duplicate nonce after cancel churn")

	assertGoroutinesSettled(t, baseline, 10, 60*time.Second)
	t.Logf("hard-cancel: outcomes=%v stored=%d", oc.snapshot(), check.count)
}
