// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build scenario

package chainsim_test

import (
	"context"
	"encoding/json"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethersphere/bee/v2/pkg/chainsim"
	signermock "github.com/ethersphere/bee/v2/pkg/crypto/mock"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// env helpers
// ---------------------------------------------------------------------------

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// ---------------------------------------------------------------------------
// workload counters
// ---------------------------------------------------------------------------

type workloadCounters struct {
	completed atomic.Int64
	failed    atomic.Int64
	inFlight  atomic.Int64
}

func (w *workloadCounters) snapshot() workloadSnapshot {
	return workloadSnapshot{
		Completed: w.completed.Load(),
		Failed:    w.failed.Load(),
		InFlight:  w.inFlight.Load(),
	}
}

// ---------------------------------------------------------------------------
// dump structures
// ---------------------------------------------------------------------------

type simSummary struct {
	BlockNum    uint64         `json:"block_num"`
	BaseFee     string         `json:"base_fee"`
	MempoolSize int            `json:"mempool_size"`
	Congestion  float64        `json:"congestion"`
	Stats       chainsim.Stats `json:"stats"`
}

type storeEntryDump struct {
	Key   string `json:"key"`
	Nonce uint64 `json:"nonce,omitempty"`
	Hash  string `json:"tx_hash,omitempty"`
	Desc  string `json:"description,omitempty"`
}

type storeDump struct {
	TotalKeys int              `json:"total_keys"`
	Counts    map[string]int   `json:"counts"`
	Retry     []storeEntryDump `json:"retry"`
	Pending   []string         `json:"pending"`

	StoredCount    int    `json:"stored_count"`
	StoredMinNonce uint64 `json:"stored_min_nonce"`
	StoredMaxNonce uint64 `json:"stored_max_nonce"`
}

type workloadSnapshot struct {
	Completed int64 `json:"completed"`
	Failed    int64 `json:"failed"`
	InFlight  int64 `json:"in_flight"`
}

type minuteDump struct {
	ElapsedSec     int64            `json:"elapsed_sec"`
	Timestamp      string           `json:"timestamp"`
	CurrentLogFile string           `json:"current_log_file"`
	Sim            simSummary       `json:"sim"`
	Store          storeDump        `json:"store"`
	Workload       workloadSnapshot `json:"workload"`
}

// ---------------------------------------------------------------------------
// dump collection helpers
// ---------------------------------------------------------------------------

const (
	storedPrefix  = "transaction_stored_"
	pendingPrefix = "transaction_pending_"
	retryPrefix   = "transaction_retry_"
)

func collectSimSummary(sim *chainsim.SimChain, congestion float64) simSummary {
	return simSummary{
		BlockNum:    sim.BlockCount(),
		BaseFee:     sim.CurrentBaseFee().String(),
		MempoolSize: sim.MempoolSize(),
		Congestion:  congestion,
		Stats:       sim.Stats(),
	}
}

func collectStoreDump(insp storemock.InspectableStore) storeDump {
	d := storeDump{
		TotalKeys: insp.Len(),
		Counts:    map[string]int{},
	}

	retryKeys := insp.KeysWithPrefix(retryPrefix)
	d.Counts[retryPrefix] = len(retryKeys)
	for _, k := range retryKeys {
		var hash common.Hash
		_ = insp.Get(k, &hash)
		d.Retry = append(d.Retry, storeEntryDump{Key: k, Hash: hash.Hex()})
	}

	storedKeys := insp.KeysWithPrefix(storedPrefix)
	d.Counts[storedPrefix] = len(storedKeys)
	d.StoredCount = len(storedKeys)
	first := true
	for _, k := range storedKeys {
		var st transaction.StoredTransaction
		if err := insp.Get(k, &st); err != nil {
			continue
		}
		if first || st.Nonce < d.StoredMinNonce {
			d.StoredMinNonce = st.Nonce
		}
		if first || st.Nonce > d.StoredMaxNonce {
			d.StoredMaxNonce = st.Nonce
		}
		first = false
	}

	pendingKeys := insp.KeysWithPrefix(pendingPrefix)
	d.Counts[pendingPrefix] = len(pendingKeys)
	d.Pending = pendingKeys

	return d
}

// ---------------------------------------------------------------------------
// highload environment
// ---------------------------------------------------------------------------

type highloadEnv struct {
	sim            *chainsim.SimChain
	svc            transaction.Service
	monitor        transaction.Monitor
	store          storemock.InspectableStore
	sender         common.Address
	outDir         string
	logsDir        string
	dumpsDir       string
	writer         *chainsim.RotatingJSONLWriter
	harnessLog     *chainsim.JSONLLogger
	wl             *workloadCounters
	congestion     float64
	cancelSim      context.CancelFunc
	feeCap         *feeCapRecorder
	opts           highloadOpts
	confirmedNonce uint64
}

func setupHighload(t *testing.T, name string, cfg chainsim.Config, retryCfg transaction.TransactionsRetryConfig, rotateInterval time.Duration, opts ...highloadOpts) *highloadEnv {
	t.Helper()
	opt := defaultHighloadOpts()
	if len(opts) > 0 {
		opt = opts[0]
	}
	return setupHighloadWithOpts(t, name, cfg, retryCfg, rotateInterval, opt)
}

func setupHighloadWithOpts(t *testing.T, name string, cfg chainsim.Config, retryCfg transaction.TransactionsRetryConfig, rotateInterval time.Duration, opt highloadOpts) *highloadEnv {
	t.Helper()

	outDir := filepath.Join(scenarioOutputDir(), name)
	logsDir := filepath.Join(outDir, "logs")
	dumpsDir := filepath.Join(outDir, "dumps")
	for _, d := range []string{logsDir, dumpsDir} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	sender := crypto.PubkeyToAddress(key.PublicKey)

	rotWriter, err := chainsim.NewRotatingJSONLWriter(logsDir, rotateInterval)
	require.NoError(t, err)

	simLogger := chainsim.NewJSONLLogger("chainsim", rotWriter)
	svcLogger := chainsim.NewJSONLLogger("sendwithretry", rotWriter)
	harnessLog := chainsim.NewJSONLLogger("harness", rotWriter)

	sim := chainsim.New(cfg)
	sim.SetBalance(sender, new(big.Int).Mul(big.NewInt(1e18), big.NewInt(10000)))
	sim.SetLogger(simLogger)

	simCtx, cancelSim := context.WithCancel(context.Background())
	go sim.Run(simCtx)

	store := storemock.NewStateStore()
	insp := store.(storemock.InspectableStore)

	var backend transaction.Backend = sim
	var feeCap *feeCapRecorder
	if opt.backend != nil {
		backend = opt.backend(sim)
		if rec, ok := backend.(*feeCapRecorder); ok {
			feeCap = rec
		}
	}

	cancellationDepth := opt.cancellationDepth
	if cancellationDepth == 0 {
		cancellationDepth = 2
	}

	monitor := transaction.NewMonitor(svcLogger, backend, sender, 30*time.Millisecond, cancellationDepth)

	svc, err := transaction.NewService(svcLogger, sender, backend,
		signermock.New(
			signermock.WithSignTxFunc(func(tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
				return types.SignTx(tx, types.LatestSignerForChainID(chainID), key)
			}),
			signermock.WithEthereumAddressFunc(func() (common.Address, error) {
				return sender, nil
			}),
		),
		store,
		cfg.ChainID,
		monitor,
		0,
		retryCfg,
	)
	require.NoError(t, err)

	return &highloadEnv{
		sim:        sim,
		svc:        svc,
		monitor:    monitor,
		store:      insp,
		sender:     sender,
		outDir:     outDir,
		logsDir:    logsDir,
		dumpsDir:   dumpsDir,
		writer:     rotWriter,
		harnessLog: harnessLog,
		wl:         &workloadCounters{},
		congestion: cfg.InitialCongestion,
		cancelSim:  cancelSim,
		feeCap:     feeCap,
		opts:       opt,
	}
}

func (e *highloadEnv) writeDump(t *testing.T, name string, elapsed time.Duration) {
	t.Helper()
	d := minuteDump{
		ElapsedSec:     int64(elapsed.Seconds()),
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		CurrentLogFile: e.writer.CurrentFileName(),
		Sim:            collectSimSummary(e.sim, e.congestion),
		Store:          collectStoreDump(e.store),
		Workload:       e.wl.snapshot(),
	}
	data, _ := json.MarshalIndent(d, "", "  ")
	path := filepath.Join(e.dumpsDir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Logf("write dump %s: %v", name, err)
	}

	e.harnessLog.Info("dump_created",
		"dump_file", name,
		"elapsed_sec", d.ElapsedSec,
		"block_num", d.Sim.BlockNum,
	)
}

func (e *highloadEnv) writeFullSnapshot(t *testing.T, name string) {
	t.Helper()
	snap := e.sim.Snapshot()
	data, _ := json.MarshalIndent(snap, "", "  ")
	path := filepath.Join(e.dumpsDir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Logf("write snapshot %s: %v", name, err)
	}
}

func (e *highloadEnv) writeSummary(t *testing.T, assertions map[string]string) {
	t.Helper()
	summary := struct {
		Workload   workloadSnapshot  `json:"workload"`
		FinalStore storeDump         `json:"final_store"`
		Assertions map[string]string `json:"assertions"`
	}{
		Workload:   e.wl.snapshot(),
		FinalStore: collectStoreDump(e.store),
		Assertions: assertions,
	}
	data, _ := json.MarshalIndent(summary, "", "  ")
	path := filepath.Join(e.outDir, "summary.json")
	_ = os.WriteFile(path, data, 0o644)
}

// ---------------------------------------------------------------------------
// worker loop
// ---------------------------------------------------------------------------

type highloadWorkerFn func(ctx context.Context, stop <-chan struct{}, svc transaction.Service)

// highloadWorker issues SendWithRetry calls until stop is signaled. When stop
// fires it does NOT abort an in-flight call: the current SendWithRetry is allowed
// to run to completion (so its receipt lands and its pending_ key is cleaned up),
// and only then does the worker exit. ctx is cancelled solely on hard teardown.
func highloadWorker(ctx context.Context, stop <-chan struct{}, svc transaction.Service, wl *workloadCounters) {
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		default:
		}
		wl.inFlight.Add(1)
		_, receipt, err := svc.SendWithRetry(ctx, defaultHighloadTxRequest("highload"))
		wl.inFlight.Add(-1)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			wl.failed.Add(1)
			continue
		}
		if receipt != nil {
			wl.completed.Add(1)
		}
	}
}

// ---------------------------------------------------------------------------
// run harness: workers + dump loop
// ---------------------------------------------------------------------------

func runHighload(t *testing.T, env *highloadEnv, duration time.Duration, workers int, dumpInterval time.Duration) {
	t.Helper()
	runHighloadWithWorker(t, env, duration, workers, dumpInterval, func(ctx context.Context, stop <-chan struct{}, svc transaction.Service) {
		highloadWorker(ctx, stop, svc, env.wl)
	})
}

func runHighloadWithWorker(t *testing.T, env *highloadEnv, duration time.Duration, workers int, dumpInterval time.Duration, workerFn highloadWorkerFn) {
	t.Helper()

	start := time.Now()

	env.writeFullSnapshot(t, "dump_start.json")

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	stop := make(chan struct{})
	stopTimer := time.AfterFunc(duration, func() { close(stop) })
	defer stopTimer.Stop()

	var workersWg sync.WaitGroup
	for range workers {
		workersWg.Add(1)
		go func() {
			defer workersWg.Done()
			workerFn(runCtx, stop, env.svc)
		}()
	}

	dumpCtx, cancelDump := context.WithCancel(context.Background())
	var dumpWg sync.WaitGroup
	dumpWg.Add(1)
	go func() {
		defer dumpWg.Done()
		ticker := time.NewTicker(dumpInterval)
		defer ticker.Stop()
		seq := 0
		for {
			select {
			case <-dumpCtx.Done():
				return
			case <-ticker.C:
				seq++
				elapsed := time.Since(start)
				var name string
				if dumpInterval >= time.Minute {
					name = dumpNameMin(seq, dumpInterval)
				} else {
					name = dumpNameSec(seq, dumpInterval)
				}
				env.writeDump(t, name, elapsed)
			}
		}
	}()

	workersWg.Wait()

	if n := env.wl.inFlight.Load(); n > 0 {
		t.Logf("warning: %d send with retry calls still in flight after workers stopped", n)
	}

	pendingDrain := env.opts.pendingDrainTimeout
	if pendingDrain == 0 {
		pendingDrain = 30 * time.Second
	}
	pendingDeadline := time.Now().Add(pendingDrain)
	for len(env.store.KeysWithPrefix(pendingPrefix)) > 0 && time.Now().Before(pendingDeadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := len(env.store.KeysWithPrefix(pendingPrefix)); n > 0 {
		t.Logf("warning: %d pending tx still tracked after drain", n)
	}

	t.Logf("workers done: completed=%d failed=%d", env.wl.completed.Load(), env.wl.failed.Load())

	if nonce, err := env.sim.NonceAt(context.Background(), env.sender, nil); err == nil {
		env.confirmedNonce = nonce
	}

	require.NoError(t, env.svc.Close())
	require.NoError(t, env.monitor.Close())
	env.cancelSim()
	env.sim.Close()

	cancelDump()
	dumpWg.Wait()

	env.writeDump(t, "dump_final_light.json", time.Since(start))
	env.writeFullSnapshot(t, "dump_final.json")
	require.NoError(t, env.writer.Close())
}

func dumpNameMin(seq int, interval time.Duration) string {
	mins := seq * int(interval/time.Minute)
	return "dump_" + padInt(mins, 3) + "m.json"
}

func dumpNameSec(seq int, interval time.Duration) string {
	secs := seq * int(interval/time.Second)
	return "dump_" + padInt(secs, 4) + "s.json"
}

func padInt(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// collectNonces reads all StoredTransaction nonces from stored_ keys.
func collectNonces(t *testing.T, insp storemock.InspectableStore) []uint64 {
	t.Helper()
	return collectNoncesFrom(insp)
}

// ---------------------------------------------------------------------------
// Test A: happy path — strict store cleanliness (less greenhouse)
// ---------------------------------------------------------------------------

func TestHighload_HappyPathStoreClean(t *testing.T) {
	duration := stressDuration()
	workers := envInt("HIGHLOAD_WORKERS", 8)
	rotateInterval := envDuration("HIGHLOAD_ROTATE", 30*time.Second)
	dumpInterval := envDuration("HIGHLOAD_DUMP", 10*time.Second)

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 50 * time.Millisecond
	cfg.BlockPeriodJitter = 20 * time.Millisecond
	cfg.InitialCongestion = 0.2
	cfg.CongestionStdDev = 0.05
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	cfg.RNGSeed = 42
	cfg.HistoryRetentionBlocks = 2000

	retryCfg := defaultRetryCfg()

	baseline := goroutineBaseline()
	env := setupHighload(t, "highload_happy", cfg, retryCfg, rotateInterval)

	calmAt := time.Now().Add(duration - quietTail())
	stopDrivers := make(chan struct{})
	go lightReadRPCFaultDriver(stopDrivers, env.sim, rand.New(rand.NewSource(7)), 2*time.Second, calmAt)

	runHighload(t, env, duration, workers, dumpInterval)
	close(stopDrivers)

	assertions := map[string]string{}

	retryKeys := env.store.KeysWithPrefix(retryPrefix)
	if len(retryKeys) == 0 {
		assertions["retry_clean"] = "PASS"
	} else {
		assertions["retry_clean"] = "FAIL: " + strconv.Itoa(len(retryKeys)) + " retry keys remain"
	}
	require.Empty(t, retryKeys, "retry state leak")

	pendingKeys := env.store.KeysWithPrefix(pendingPrefix)
	if len(pendingKeys) == 0 {
		assertions["pending_clean"] = "PASS"
	} else {
		assertions["pending_clean"] = "FAIL: " + strconv.Itoa(len(pendingKeys)) + " pending keys remain"
	}
	require.Empty(t, pendingKeys, "pending tx leak")

	storedKeys := env.store.KeysWithPrefix(storedPrefix)
	completed := int(env.wl.completed.Load())
	if len(storedKeys) == completed {
		assertions["stored_count"] = "PASS: " + strconv.Itoa(completed)
	} else {
		assertions["stored_count"] = "FAIL: stored=" + strconv.Itoa(len(storedKeys)) + " completed=" + strconv.Itoa(completed)
	}
	require.Equal(t, completed, len(storedKeys), "stored count mismatch")

	totalKeys := env.store.Len()
	if totalKeys == len(storedKeys) {
		assertions["no_extra_keys"] = "PASS"
	} else {
		assertions["no_extra_keys"] = "FAIL: total=" + strconv.Itoa(totalKeys) + " stored=" + strconv.Itoa(len(storedKeys))
	}
	require.Equal(t, len(storedKeys), totalKeys, "unexpected non-stored keys remain")

	nonces := collectNonces(t, env.store)
	check := checkNonces(nonces)
	if !check.dup {
		assertions["no_nonce_dup"] = "PASS: " + strconv.Itoa(len(nonces)) + " unique nonces"
	} else {
		assertions["no_nonce_dup"] = "FAIL"
	}
	require.False(t, check.dup, "duplicate nonce detected")

	assertGoroutinesSettled(t, baseline, 10, 30*time.Second)
	assertions["goroutines_settled"] = "PASS"

	env.writeSummary(t, assertions)
	t.Logf("happy path done: %d txs completed, %d stored, %d total keys", completed, len(storedKeys), totalKeys)
}

// ---------------------------------------------------------------------------
// Test B: stress — nonce correctness and retry under pressure (less greenhouse)
// ---------------------------------------------------------------------------

func TestHighload_StressNonceAndRetry(t *testing.T) {
	duration := stressDuration()
	workers := envInt("HIGHLOAD_WORKERS", 16)
	rotateInterval := envDuration("HIGHLOAD_ROTATE", 30*time.Second)
	dumpInterval := envDuration("HIGHLOAD_DUMP", 10*time.Second)

	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 50 * time.Millisecond
	cfg.BlockPeriodJitter = 25 * time.Millisecond
	cfg.InitialCongestion = 0.5
	cfg.CongestionStdDev = 0.15
	cfg.MaxTxsPerBlock = 2
	cfg.MaxMempoolSize = 4096
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	cfg.RNGSeed = 42
	cfg.HistoryRetentionBlocks = 2000

	retryCfg := defaultRetryCfg()
	retryCfg.MaxTxPrice = big.NewInt(100_000_000_000_000)

	opt := defaultHighloadOpts()
	opt.backend = withFeeCapBackend(retryCfg.MaxTxPrice)

	baseline := goroutineBaseline()
	env := setupHighloadWithOpts(t, "highload_stress", cfg, retryCfg, rotateInterval, opt)

	calmAt := time.Now().Add(duration - quietTail())
	stopDrivers := make(chan struct{})
	go congestionWaveDriver(stopDrivers, env.sim, 0.85, 0.3, 0.3, 5*time.Second, calmAt)

	runHighload(t, env, duration, workers, dumpInterval)
	close(stopDrivers)

	assertions := map[string]string{}

	retryKeys := env.store.KeysWithPrefix(retryPrefix)
	if len(retryKeys) == 0 {
		assertions["retry_clean"] = "PASS"
	} else {
		assertions["retry_clean"] = "FAIL: " + strconv.Itoa(len(retryKeys)) + " retry keys remain"
	}
	require.Empty(t, retryKeys, "retry state leak")

	nonces := collectNonces(t, env.store)
	if len(nonces) == 0 {
		t.Fatal("no stored transactions at all")
	}

	check := checkNonces(nonces)
	if !check.dup {
		assertions["no_nonce_dup"] = "PASS: " + strconv.Itoa(len(nonces)) + " unique nonces"
	} else {
		assertions["no_nonce_dup"] = "FAIL"
	}
	require.False(t, check.dup, "duplicate nonce detected")

	if !check.gap {
		assertions["no_nonce_gap"] = "PASS: 0.." + strconv.FormatUint(check.maxNonce, 10)
	} else {
		assertions["no_nonce_gap"] = "FAIL"
	}
	require.False(t, check.gap, "nonce gap detected")

	if env.feeCap != nil {
		require.Zero(t, env.feeCap.violations.Load(), "gasFeeCap exceeded MaxTxPrice")
		assertions["fee_cap_ok"] = "PASS"
	}

	pendingKeys := env.store.KeysWithPrefix(pendingPrefix)
	assertions["pending_count"] = strconv.Itoa(len(pendingKeys))

	completed := env.wl.completed.Load()
	failed := env.wl.failed.Load()
	total := completed + failed
	if total > 0 {
		rate := float64(completed) / float64(total)
		assertions["completion_rate"] = strconv.FormatFloat(rate, 'f', 3, 64)
		require.Greater(t, rate, 0.10, "completion rate too low: workload is deadlocked")
	}
	require.Greater(t, completed, int64(0), "no transactions completed at all")

	assertGoroutinesSettled(t, baseline, 10, 30*time.Second)
	assertions["goroutines_settled"] = "PASS"

	t.Logf("stress done: %d stored nonces, %d pending remain, %d completed, %d failed (rate=%.1f%%)",
		len(nonces), len(pendingKeys), completed, failed, float64(completed)/float64(total)*100)

	env.writeSummary(t, assertions)
}
