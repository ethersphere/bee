// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build scenario

package chainsim_test

import (
	"context"
	"math/big"
	"sync"
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

// TestHighload_CrashRecovery verifies SendWithRetry survives periodic service
// restarts without breaking nonce state.
//
// Goal: Confirm resumeRetryTransactions restores in-flight work and nonce
// integrity across repeated process restarts.
//
// How it works: Sim and store keep running while the transaction service is
// torn down and recreated on a schedule; workers resume each epoch; final
// checks cover retry keys and nonce continuity.
func TestHighload_CrashRecovery(t *testing.T) {
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
	cfg.HistoryRetentionBlocks = 5000

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

	for time.Now().Before(deadline) {
		svc, mon := newSvc()
		stop := make(chan struct{})
		var wg sync.WaitGroup
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				instrumentedWorker(context.Background(), stop, svc, oc, nil)
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

	// Final epoch: bring service back up and let resume + block production settle.
	finalSvc, finalMon := newSvc()
	time.Sleep(15 * time.Second)
	require.NoError(t, finalSvc.Close())
	require.NoError(t, finalMon.Close())

	require.Empty(t, insp.KeysWithPrefix(retryPrefix), "orphan retry keys after restarts")

	stored := collectNoncesFrom(insp)
	check := checkNonces(stored)
	require.False(t, check.dup, "duplicate nonce across restarts")
	require.False(t, check.gap, "nonce gap across restarts")

	if check.count > 0 {
		confirmed, err := sim.NonceAt(context.Background(), sender, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, confirmed, check.maxNonce+1, "confirmed nonce behind stored")
	}

	assertGoroutinesSettled(t, baseline, 10, 60*time.Second)
	t.Logf("crash-recovery: outcomes=%v stored=%d", oc.snapshot(), check.count)
}
