// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build scenario

package chainsim_test

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethersphere/bee/v2/pkg/chainsim"
	signermock "github.com/ethersphere/bee/v2/pkg/crypto/mock"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/stretchr/testify/require"
)

// scenarioEnv holds everything needed for a scenario run.
type scenarioEnv struct {
	sim     *chainsim.SimChain
	svc     transaction.Service
	monitor transaction.Monitor
	store   storage.StateStorer
	sender  common.Address
	log     *chainsim.JSONLFile
	outDir  string
	cancel  context.CancelFunc
}

func setupScenario(t *testing.T, name string, cfg chainsim.Config, retryCfg transaction.TransactionsRetryConfig) *scenarioEnv {
	t.Helper()
	return setupScenarioWithMonitorDepth(t, name, cfg, retryCfg, 2)
}

func setupScenarioWithMonitorDepth(t *testing.T, name string, cfg chainsim.Config, retryCfg transaction.TransactionsRetryConfig, cancellationDepth uint64) *scenarioEnv {
	t.Helper()

	outDir := filepath.Join(scenarioOutputDir(), name)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	sender := crypto.PubkeyToAddress(key.PublicKey)

	logFile := &chainsim.JSONLFile{}
	simLogger := chainsim.NewJSONLLogger("chainsim", logFile)
	svcLogger := chainsim.NewJSONLLogger("sendwithretry", logFile)

	sim := chainsim.New(cfg)
	sim.SetBalance(sender, new(big.Int).Mul(big.NewInt(1e18), big.NewInt(1000)))
	sim.SetLogger(simLogger)

	ctx, cancel := context.WithCancel(context.Background())
	go sim.Run(ctx)

	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	monitor := transaction.NewMonitor(svcLogger, sim, sender, 30*time.Millisecond, cancellationDepth)
	testutil.CleanupCloser(t, monitor)

	svc, err := transaction.NewService(svcLogger, sender, sim,
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
	if err != nil {
		cancel()
		sim.Close()
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, svc)

	return &scenarioEnv{
		sim:     sim,
		svc:     svc,
		monitor: monitor,
		store:   store,
		sender:  sender,
		log:     logFile,
		outDir:  outDir,
		cancel:  cancel,
	}
}

func (e *scenarioEnv) teardown(t *testing.T) {
	t.Helper()
	e.cancel()
	e.sim.Close()
	e.writeArtifacts(t)
}

func (e *scenarioEnv) writeArtifacts(t *testing.T) {
	t.Helper()

	eventsData, err := e.log.MarshalJSONL()
	if err != nil {
		t.Errorf("marshal events: %v", err)
		return
	}
	writeFile(t, filepath.Join(e.outDir, "events.jsonl"), eventsData)

	snap := e.sim.Snapshot()
	snapData, _ := json.MarshalIndent(snap, "", "  ")
	writeFile(t, filepath.Join(e.outDir, "state.json"), snapData)

	stats := e.sim.Stats()
	statsData, _ := json.MarshalIndent(stats, "", "  ")
	writeFile(t, filepath.Join(e.outDir, "stats.json"), statsData)
}

type scenarioResult struct {
	Scenario    string `json:"scenario"`
	TxHash      string `json:"tx_hash"`
	HasReceipt  bool   `json:"has_receipt"`
	Status      uint64 `json:"status,omitempty"`
	ErrorMsg    string `json:"error,omitempty"`
	DurationMs  int64  `json:"duration_ms"`
	BlocksTotal uint64 `json:"blocks_total"`
}

func (e *scenarioEnv) writeResult(t *testing.T, name string, txHash common.Hash, receipt *types.Receipt, err error, dur time.Duration) {
	t.Helper()
	r := scenarioResult{
		Scenario:    name,
		TxHash:      txHash.Hex(),
		HasReceipt:  receipt != nil,
		DurationMs:  dur.Milliseconds(),
		BlocksTotal: e.sim.BlockCount(),
	}
	if receipt != nil {
		r.Status = receipt.Status
	}
	if err != nil {
		r.ErrorMsg = err.Error()
	}
	data, _ := json.MarshalIndent(r, "", "  ")
	writeFile(t, filepath.Join(e.outDir, "result.json"), data)
	t.Logf("scenario %s: tx=%s receipt=%v status=%d err=%v blocks=%d duration=%s",
		name, r.TxHash, r.HasReceipt, r.Status, err, r.BlocksTotal, dur)
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Errorf("write %s: %v", path, err)
	}
}

func scenarioOutputDir() string {
	d := os.Getenv("SCENARIO_OUTPUT_DIR")
	if d == "" {
		d = "scenario-results"
	}
	return d
}

func defaultRetryCfg() transaction.TransactionsRetryConfig {
	return transaction.TransactionsRetryConfig{
		RetryDelay:      300 * time.Millisecond,
		AttemptsPerTier: 3,
		StartTier:       "market",
		EndTier:         "aggressive",
		MaxTxPrice:      big.NewInt(500_000_000_000),
	}
}

func fastSimConfig() chainsim.Config {
	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 80 * time.Millisecond
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	return cfg
}

// inclusionSimConfig produces non-zero fee-history tips so the first broadcast
// clears MinMempoolTip and can be included.
func inclusionSimConfig() chainsim.Config {
	cfg := fastSimConfig()
	cfg.InitialCongestion = 0.5
	cfg.BackgroundTipStdDev = big.NewInt(0)
	cfg.CongestionStdDev = 0
	return cfg
}

func scenarioRequest(name string) *transaction.TxRequest {
	to := common.HexToAddress("0xabcd")
	return &transaction.TxRequest{
		To:          &to,
		Data:        []byte{0xab, 0xcd},
		Value:       big.NewInt(0),
		GasLimit:    50_000,
		Description: name,
	}
}

func sendOne(t *testing.T, ctx context.Context, env *scenarioEnv, name string) (common.Hash, *types.Receipt, error) {
	t.Helper()
	recipient := common.HexToAddress("0xabcd")
	start := time.Now()
	txHash, receipt, err := env.svc.SendWithRetry(ctx, &transaction.TxRequest{
		To:          &recipient,
		Data:        []byte{0xab, 0xcd},
		Value:       big.NewInt(0),
		GasLimit:    50_000,
		Description: name,
	})
	env.writeResult(t, name, txHash, receipt, err, time.Since(start))
	return txHash, receipt, err
}

func requireIncluded(t *testing.T, hash common.Hash, receipt *types.Receipt, err error) {
	t.Helper()
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, uint64(1), receipt.Status)
	require.Equal(t, hash, receipt.TxHash)
	require.NotEqual(t, common.Hash{}, hash)
}

func requireExhausted(t *testing.T, hash common.Hash, receipt *types.Receipt, err error) {
	t.Helper()
	require.ErrorIs(t, err, transaction.ErrAllAttemptsExhausted)
	require.Nil(t, receipt)
	require.NotEqual(t, common.Hash{}, hash)
}

// TestScenario_HappyPath verifies a single transaction succeeds under normal conditions.
//
// Goal: Confirm the baseline SendWithRetry path works end-to-end on a fast sim.
//
// How it works: One transaction is sent with default retry config and mild congestion.
func TestScenario_HappyPath(t *testing.T) {
	cfg := inclusionSimConfig()
	env := setupScenario(t, "01_happy_path", cfg, defaultRetryCfg())
	defer env.teardown(t)
	waitForPricedHistory(t, env.sim)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	txHash, receipt, err := sendOne(t, ctx, env, "01_happy_path")
	requireIncluded(t, txHash, receipt, err)
	require.Len(t, broadcastEvents(env.log), 1)
	require.Zero(t, rejectionCount(env.sim, "tip_too_low"))
	assertRetryCleaned(t, env.store, 0, txHash)
}

// TestScenario_CongestionSpikeDrop verifies retry succeeds after a temporary
// congestion spike blocks inclusion.
//
// Goal: Confirm SendWithRetry waits and re-broadcasts until congestion drops
// and the transaction is included.
//
// How it works: Congestion starts at maximum, then is lowered mid-flight while
// one transaction is in the retry loop.
func TestScenario_CongestionSpikeDrop(t *testing.T) {
	cfg := inclusionSimConfig()
	cfg.InitialCongestion = 1
	env := setupScenario(t, "02_congestion_spike_drop", cfg, defaultRetryCfg())
	defer env.teardown(t)
	waitForPricedHistory(t, env.sim)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	start := time.Now()
	done := sendAsync(env.svc, ctx, scenarioRequest("02_congestion_spike_drop"))
	waitUntil(t, 5*time.Second, "tx pending while the block is full", func() bool {
		return env.sim.MempoolSize() >= 1 && env.sim.Stats().TransactionsExecuted == 0
	})
	require.Zero(t, env.sim.Stats().TransactionsExecuted)
	env.sim.SetCongestion(0.1)
	got := waitOutcome(t, done, 10*time.Second)
	env.writeResult(t, "02_congestion_spike_drop", got.hash, got.receipt, got.err, time.Since(start))
	requireIncluded(t, got.hash, got.receipt, got.err)
	require.Zero(t, rejectionCount(env.sim, "tip_too_low"))
	assertRetryCleaned(t, env.store, 0, got.hash)
}

// TestScenario_BaseFeeSpikeRetry verifies fee escalation through a base-fee spike.
//
// Goal: Confirm SendWithRetry escalates tiers to cover a temporary base-fee
// increase and succeeds after fees normalize.
//
// How it works: Congestion blocks inclusion while base fee spikes then drops;
// one transaction must escalate and complete when conditions improve.
func TestScenario_BaseFeeSpikeRetry(t *testing.T) {
	cfg := inclusionSimConfig()
	cfg.InitialBaseFee = big.NewInt(5_000_000_000)
	retryCfg := defaultRetryCfg()
	retryCfg.MaxTxPrice = big.NewInt(1_000_000_000_000)
	env := setupScenario(t, "03_basefee_spike", cfg, retryCfg)
	defer env.teardown(t)
	waitForPricedHistory(t, env.sim)
	env.sim.SetCongestion(1)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	start := time.Now()
	done := sendAsync(env.svc, ctx, scenarioRequest("03_basefee_spike"))
	waitUntil(t, 5*time.Second, "tx pending before the base-fee spike", func() bool {
		return env.sim.MempoolSize() >= 1
	})
	first := requireMempoolTx(t, env.sim, env.sender)
	require.Less(t, first.GasFeeCap().Cmp(big.NewInt(20_000_000_000)), 0)
	const spiked = int64(20_000_000_000)
	env.sim.SetBaseFee(big.NewInt(spiked))
	env.sim.SetCongestion(0.2)
	got := waitOutcome(t, done, 10*time.Second)
	env.writeResult(t, "03_basefee_spike", got.hash, got.receipt, got.err, time.Since(start))
	requireIncluded(t, got.hash, got.receipt, got.err)
	mined, pending, err := env.sim.TransactionByHash(ctx, got.hash)
	require.NoError(t, err)
	require.False(t, pending)
	require.GreaterOrEqual(t, mined.GasFeeCap().Cmp(big.NewInt(spiked)), 0, "included tx must cover the spiked base fee")
	require.GreaterOrEqual(t, len(broadcastEvents(env.log)), 2)
	require.Zero(t, rejectionCount(env.sim, "tip_too_low"))
	assertRetryCleaned(t, env.store, 0, got.hash)
}

// TestScenario_TransientRPCErrors verifies SendWithRetry survives intermittent
// RPC failures during fee lookup and receipt polling.
//
// Goal: Confirm transient backend errors do not abort the retry loop permanently.
//
// How it works: Header and receipt RPC calls fail a fixed number of times before
// one transaction is sent through the full retry flow.
func TestScenario_TransientRPCErrors(t *testing.T) {
	cfg := inclusionSimConfig()
	env := setupScenario(t, "04_transient_rpc_errors", cfg, defaultRetryCfg())
	defer env.teardown(t)
	waitForPricedHistory(t, env.sim)

	env.sim.InjectError("HeaderByNumber", context.DeadlineExceeded, 3)
	env.sim.InjectError("TransactionReceipt", context.DeadlineExceeded, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	txHash, receipt, err := sendOne(t, ctx, env, "04_transient_rpc_errors")
	requireIncluded(t, txHash, receipt, err)
	require.GreaterOrEqual(t, len(broadcastEvents(env.log)), 1)
	assertRetryCleaned(t, env.store, 0, txHash)
}

// TestScenario_ReceiptDelay verifies the monitor waits for late receipts when
// cancellation depth is configured correctly.
//
// Goal: Confirm a receipt that appears several blocks late is not treated as
// a false cancellation.
//
// How it works: Receipt availability is delayed; monitor cancellationDepth is
// set above that delay; one transaction is sent and must complete.
func TestScenario_ReceiptDelay(t *testing.T) {
	cfg := inclusionSimConfig()
	cfg.ReceiptAvailDelay = 3
	retryCfg := defaultRetryCfg()
	retryCfg.RetryDelay = 2 * time.Second
	env := setupScenarioWithMonitorDepth(t, "05_receipt_delay", cfg, retryCfg, 5)
	defer env.teardown(t)
	waitForPricedHistory(t, env.sim)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	txHash, receipt, err := sendOne(t, ctx, env, "05_receipt_delay")
	requireIncluded(t, txHash, receipt, err)
	assertRetryCleaned(t, env.store, 0, txHash)
}

// TestScenario_AllTiersExhausted verifies behavior when inclusion never becomes
// possible within the configured retry budget.
//
// Goal: Confirm SendWithRetry stops cleanly after all tiers and attempts are
// exhausted under permanent congestion.
//
// How it works: Congestion stays at maximum with a small attempts-per-tier
// budget; one transaction is expected to fail without hanging.
func TestScenario_AllTiersExhausted(t *testing.T) {
	cfg := inclusionSimConfig()
	cfg.InitialCongestion = 1
	retryCfg := defaultRetryCfg()
	retryCfg.AttemptsPerTier = 2
	env := setupScenario(t, "06_all_tiers_exhausted", cfg, retryCfg)
	defer env.teardown(t)
	waitForPricedHistory(t, env.sim)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	txHash, receipt, err := sendOne(t, ctx, env, "06_all_tiers_exhausted")
	requireExhausted(t, txHash, receipt, err)
	require.Zero(t, env.sim.Stats().TransactionsExecuted)
	require.GreaterOrEqual(t, env.sim.Stats().TransactionsAccepted, uint64(1))
	require.Zero(t, rejectionCount(env.sim, "tip_too_low"))
	_, pending := loadRetry(t, env.store, 0)
	require.True(t, pending, "exhausted tx must stay under watch")
}

// TestScenario_TransactionRevert verifies deterministic on-chain revert handling.
//
// Goal: Confirm a transaction that always reverts is reported correctly and
// does not leave the retry loop in an inconsistent state.
//
// How it works: The recipient address is marked as reverting; one transaction
// is sent and the outcome is recorded.
func TestScenario_TransactionRevert(t *testing.T) {
	cfg := inclusionSimConfig()
	env := setupScenario(t, "07_tx_revert", cfg, defaultRetryCfg())
	defer env.teardown(t)
	waitForPricedHistory(t, env.sim)

	recipient := common.HexToAddress("0xabcd")
	env.sim.SetRevertAddress(recipient)

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	txHash, receipt, err := env.svc.SendWithRetry(ctx, &transaction.TxRequest{
		To:          &recipient,
		Data:        []byte{0xab, 0xcd},
		Value:       big.NewInt(0),
		GasLimit:    50_000,
		Description: "07_tx_revert",
	})
	dur := time.Since(start)
	env.writeResult(t, "07_tx_revert", txHash, receipt, err, dur)

	require.ErrorIs(t, err, transaction.ErrTransactionReverted)
	require.NotNil(t, receipt)
	require.Equal(t, uint64(0), receipt.Status)
	require.Equal(t, txHash, receipt.TxHash)
	require.NotEqual(t, common.Hash{}, txHash)
	require.Len(t, broadcastEvents(env.log), 1, "a reverted receipt must stop further attempts")
	assertRetryCleaned(t, env.store, 0, txHash)
}

// TestScenario_RandomRevertRate verifies handling when every executed
// transaction reverts probabilistically.
//
// Goal: Confirm a 100% revert rate produces a reverted receipt without
// breaking the service flow.
//
// How it works: RandomRevertRate is set to certainty; one transaction is sent.
func TestScenario_RandomRevertRate(t *testing.T) {
	cfg := inclusionSimConfig()
	cfg.RandomRevertRate = 1.0
	env := setupScenario(t, "08_random_revert_rate", cfg, defaultRetryCfg())
	defer env.teardown(t)
	waitForPricedHistory(t, env.sim)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	recipient := common.HexToAddress("0xabcd")
	start := time.Now()
	txHash, receipt, err := env.svc.SendWithRetry(ctx, &transaction.TxRequest{
		To:          &recipient,
		Data:        []byte{0xab, 0xcd},
		Value:       big.NewInt(0),
		GasLimit:    50_000,
		Description: "08_random_revert_rate",
	})
	env.writeResult(t, "08_random_revert_rate", txHash, receipt, err, time.Since(start))

	require.ErrorIs(t, err, transaction.ErrTransactionReverted)
	require.NotNil(t, receipt)
	require.Equal(t, uint64(0), receipt.Status)
	require.Equal(t, txHash, receipt.TxHash)
	require.Len(t, broadcastEvents(env.log), 1)
	assertRetryCleaned(t, env.store, 0, txHash)
}

// TestScenario_MultiBurst verifies sequential nonce assignment across a burst
// of transactions with limited block space.
//
// Goal: Confirm multiple back-to-back SendWithRetry calls maintain correct
// ordering and inclusion under MaxTxsPerBlock pressure.
//
// How it works: Five transactions are sent sequentially on a sim that includes
// at most three user txs per block; all outcomes are recorded.
func TestScenario_MultiBurst(t *testing.T) {
	cfg := inclusionSimConfig()
	cfg.MaxTxsPerBlock = 3
	env := setupScenario(t, "09_multi_burst", cfg, defaultRetryCfg())
	defer env.teardown(t)
	waitForPricedHistory(t, env.sim)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const burstSize = 5
	type burstEntry struct {
		Scenario   string `json:"scenario"`
		Index      int    `json:"index"`
		TxHash     string `json:"tx_hash"`
		HasReceipt bool   `json:"has_receipt"`
		Status     uint64 `json:"status,omitempty"`
		ErrorMsg   string `json:"error,omitempty"`
		DurationMs int64  `json:"duration_ms"`
	}
	entries := make([]burstEntry, burstSize)
	for i := range entries {
		recipient := common.HexToAddress("0xabcd")
		start := time.Now()
		hash, receipt, err := env.svc.SendWithRetry(ctx, &transaction.TxRequest{
			To:          &recipient,
			Data:        []byte{byte(i)},
			Value:       big.NewInt(0),
			GasLimit:    50_000,
			Description: "09_multi_burst",
		})
		e := burstEntry{
			Scenario:   "09_multi_burst",
			Index:      i,
			TxHash:     hash.Hex(),
			HasReceipt: receipt != nil,
			DurationMs: time.Since(start).Milliseconds(),
		}
		if receipt != nil {
			e.Status = receipt.Status
		}
		if err != nil {
			e.ErrorMsg = err.Error()
		}
		entries[i] = e
	}

	wrapper := struct {
		Scenario    string       `json:"scenario"`
		BlocksTotal uint64       `json:"blocks_total"`
		Txs         []burstEntry `json:"txs"`
	}{
		Scenario:    "09_multi_burst",
		BlocksTotal: env.sim.BlockCount(),
		Txs:         entries,
	}
	data, _ := json.MarshalIndent(wrapper, "", "  ")
	writeFile(t, filepath.Join(env.outDir, "result.json"), data)

	require.Len(t, entries, burstSize)
	var prevNonce uint64
	for i, e := range entries {
		require.Empty(t, e.ErrorMsg, "tx %d: %s", i, e.ErrorMsg)
		require.True(t, e.HasReceipt, "tx %d missing receipt", i)
		require.Equal(t, uint64(1), e.Status, "tx %d status", i)
		hash := common.HexToHash(e.TxHash)
		tx, pending, err := env.sim.TransactionByHash(ctx, hash)
		require.NoError(t, err)
		require.False(t, pending, "tx %d still pending", i)
		require.Equal(t, uint64(i), tx.Nonce())
		if i > 0 {
			require.Greater(t, tx.Nonce(), prevNonce)
		}
		prevNonce = tx.Nonce()
		receipt, err := env.sim.TransactionReceipt(ctx, hash)
		require.NoError(t, err)
		require.Equal(t, uint64(1), receipt.Status)
	}
}

// TestScenario_ProbabilisticInclusion verifies eventual inclusion with a low
// starting fee tier under probabilistic block selection.
//
// Goal: Confirm SendWithRetry completes when inclusion is probabilistic and
// network tips are higher than the initial offer.
//
// How it works: Probabilistic inclusion is enabled with a low minimum chance;
// one transaction starts at default tiers and must eventually succeed.
func TestScenario_ProbabilisticInclusion(t *testing.T) {
	const marketTip int64 = 2_000_000_000
	cfg := inclusionSimConfig()
	cfg.InclusionProbability = true
	cfg.InclusionMinProbability = 0
	cfg.BackgroundTipMean = big.NewInt(marketTip)
	env := setupScenario(t, "10_probabilistic_inclusion", cfg, defaultRetryCfg())
	defer env.teardown(t)
	waitForPricedHistory(t, env.sim)
	// Quoted tip stays near the warmed history while the reference tip jumps,
	// so the first blocks must defer the transaction.
	env.sim.SetBackgroundTipMean(big.NewInt(marketTip * 1_000_000))

	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if env.sim.Stats().InclusionDeferred >= 1 {
				env.sim.SetBackgroundTipMean(big.NewInt(marketTip))
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	txHash, receipt, err := sendOne(t, ctx, env, "10_probabilistic_inclusion")
	requireIncluded(t, txHash, receipt, err)
	require.GreaterOrEqual(t, env.sim.Stats().InclusionDeferred, uint64(1))
	require.Zero(t, rejectionCount(env.sim, "tip_too_low"))
	assertRetryCleaned(t, env.store, 0, txHash)
}

// TestScenario_BaseFeeDrop verifies SendWithRetry works after base fee falls
// from a high starting level on mostly empty blocks.
//
// Goal: Confirm fee suggestions remain valid when base fee decays toward the
// EIP-1559 floor before the transaction is sent.
//
// How it works: Sim starts with high base fee and zero congestion; after blocks
// produce low gas usage, one transaction is sent.
func TestScenario_BaseFeeDrop(t *testing.T) {
	initial := big.NewInt(100_000_000_000)
	cfg := inclusionSimConfig()
	cfg.InitialBaseFee = initial
	cfg.InitialCongestion = 0.2
	env := setupScenario(t, "11_basefee_drop", cfg, defaultRetryCfg())
	defer env.teardown(t)
	waitUntil(t, 5*time.Second, "base fee to fall on under-target blocks", func() bool {
		return env.sim.CurrentBaseFee().Cmp(initial) < 0
	})
	waitForPricedHistory(t, env.sim)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	txHash, receipt, err := sendOne(t, ctx, env, "11_basefee_drop")
	requireIncluded(t, txHash, receipt, err)
	require.Zero(t, rejectionCount(env.sim, "tip_too_low"))
	assertRetryCleaned(t, env.store, 0, txHash)
}

// TestScenario_MempoolTTLEviction verifies re-submission after mempool TTL
// eviction during retry.
//
// Goal: Confirm SendWithRetry recovers when a pending transaction is evicted
// from the mempool before inclusion.
//
// How it works: Full congestion and a short mempool TTL evict the first
// broadcast; congestion later drops so the re-submitted tx can be included.
func TestScenario_MempoolTTLEviction(t *testing.T) {
	cfg := fastSimConfig()
	cfg.MempoolTTL = 1
	cfg.InitialCongestion = 1.0
	cfg.BackgroundTipStdDev = big.NewInt(0)
	cfg.CongestionStdDev = 0
	retryCfg := defaultRetryCfg()
	retryCfg.RetryDelay = 400 * time.Millisecond
	retryCfg.StartTier = "market"
	retryCfg.EndTier = "aggressive"
	env := setupScenario(t, "12_mempool_ttl_eviction", cfg, retryCfg)
	defer env.teardown(t)
	waitForPricedHistory(t, env.sim)

	go func() {
		time.Sleep(1200 * time.Millisecond)
		env.sim.SetCongestion(0.0)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	recipient := common.HexToAddress("0xabcd")
	start := time.Now()
	txHash, receipt, err := env.svc.SendWithRetry(ctx, &transaction.TxRequest{
		To:          &recipient,
		Data:        []byte{0xab, 0xcd},
		Value:       big.NewInt(0),
		GasLimit:    50_000,
		Description: "12_mempool_ttl_eviction",
	})
	env.writeResult(t, "12_mempool_ttl_eviction", txHash, receipt, err, time.Since(start))

	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, uint64(1), receipt.Status)
	require.Equal(t, txHash, receipt.TxHash)
	require.GreaterOrEqual(t, env.sim.Stats().MempoolTTLExpirations, uint64(1))
	broadcasts := broadcastEvents(env.log)
	require.GreaterOrEqual(t, len(broadcasts), 2, "evicted tx must be broadcast again")
	require.NotEqual(t, eventHash(t, broadcasts[0]), txHash, "receipt must be the resubmission, not the evicted tx")
	require.Empty(t, storeKeys(env.store, storedTransactionKey(eventHash(t, broadcasts[0]))))
	assertRetryCleaned(t, env.store, 0, txHash)
	require.Empty(t, storeKeys(env.store, pendingTxPrefix))
}
