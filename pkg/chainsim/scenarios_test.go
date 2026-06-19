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
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

// scenarioEnv holds everything needed for a scenario run.
type scenarioEnv struct {
	sim     *chainsim.SimChain
	svc     transaction.Service
	monitor transaction.Monitor
	sender  common.Address
	log     *chainsim.JSONLFile
	outDir  string
	cancel  context.CancelFunc
}

func setupScenario(t *testing.T, name string, cfg chainsim.Config, retryCfg transaction.TransactionsRetryConfig) *scenarioEnv {
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

	monitor := transaction.NewMonitor(svcLogger, sim, sender, 30*time.Millisecond, 2)
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

func sendOne(t *testing.T, ctx context.Context, env *scenarioEnv, name string) {
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
	dur := time.Since(start)
	env.writeResult(t, name, txHash, receipt, err, dur)
}

// --- Scenario 1: Happy path ---
func TestScenario_HappyPath(t *testing.T) {
	cfg := fastSimConfig()
	env := setupScenario(t, "01_happy_path", cfg, defaultRetryCfg())
	defer env.teardown(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sendOne(t, ctx, env, "01_happy_path")
}

// --- Scenario 2: Congestion spike then drop ---
func TestScenario_CongestionSpikeDrop(t *testing.T) {
	cfg := fastSimConfig()
	cfg.InitialCongestion = 0.95
	env := setupScenario(t, "02_congestion_spike_drop", cfg, defaultRetryCfg())
	defer env.teardown(t)

	go func() {
		time.Sleep(600 * time.Millisecond)
		env.sim.SetCongestion(0.1)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sendOne(t, ctx, env, "02_congestion_spike_drop")
}

// --- Scenario 3: BaseFee spike mid-retry ---
func TestScenario_BaseFeeSpikeRetry(t *testing.T) {
	cfg := fastSimConfig()
	env := setupScenario(t, "03_basefee_spike", cfg, defaultRetryCfg())
	defer env.teardown(t)

	go func() {
		time.Sleep(200 * time.Millisecond)
		env.sim.SetBaseFee(big.NewInt(100_000_000_000))
		time.Sleep(800 * time.Millisecond)
		env.sim.SetBaseFee(big.NewInt(1_000_000_000))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sendOne(t, ctx, env, "03_basefee_spike")
}

// --- Scenario 4: Transient RPC errors ---
func TestScenario_TransientRPCErrors(t *testing.T) {
	cfg := fastSimConfig()
	env := setupScenario(t, "04_transient_rpc_errors", cfg, defaultRetryCfg())
	defer env.teardown(t)

	go func() {
		time.Sleep(100 * time.Millisecond)
		env.sim.InjectError("TransactionReceipt", context.DeadlineExceeded, 5)
		env.sim.InjectError("HeaderByNumber", context.DeadlineExceeded, 3)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sendOne(t, ctx, env, "04_transient_rpc_errors")
}

// --- Scenario 5: Receipt delay ---
func TestScenario_ReceiptDelay(t *testing.T) {
	cfg := fastSimConfig()
	cfg.ReceiptAvailDelay = 3
	env := setupScenario(t, "05_receipt_delay", cfg, defaultRetryCfg())
	defer env.teardown(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sendOne(t, ctx, env, "05_receipt_delay")
}

// --- Scenario 6: All tiers exhausted (permanent congestion) ---
func TestScenario_AllTiersExhausted(t *testing.T) {
	cfg := fastSimConfig()
	cfg.InitialCongestion = 1.0
	retryCfg := defaultRetryCfg()
	retryCfg.AttemptsPerTier = 2
	env := setupScenario(t, "06_all_tiers_exhausted", cfg, retryCfg)
	defer env.teardown(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sendOne(t, ctx, env, "06_all_tiers_exhausted")
}

// --- Scenario 7: Transaction revert (SetRevertAddress) ---
func TestScenario_TransactionRevert(t *testing.T) {
	cfg := fastSimConfig()
	env := setupScenario(t, "07_tx_revert", cfg, defaultRetryCfg())
	defer env.teardown(t)

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
}

// --- Scenario 8: Random revert rate ---
func TestScenario_RandomRevertRate(t *testing.T) {
	cfg := fastSimConfig()
	cfg.RandomRevertRate = 0.5
	env := setupScenario(t, "08_random_revert_rate", cfg, defaultRetryCfg())
	defer env.teardown(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sendOne(t, ctx, env, "08_random_revert_rate")
}

// --- Scenario 9: Multi-tx burst (10 sequential sends) ---
func TestScenario_MultiBurst(t *testing.T) {
	cfg := fastSimConfig()
	cfg.MaxTxsPerBlock = 3
	env := setupScenario(t, "09_multi_burst", cfg, defaultRetryCfg())
	defer env.teardown(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	type txResult struct {
		idx     int
		hash    common.Hash
		receipt *types.Receipt
		err     error
		dur     time.Duration
	}
	results := make([]txResult, 5)
	for i := range results {
		recipient := common.HexToAddress("0xabcd")
		start := time.Now()
		hash, receipt, err := env.svc.SendWithRetry(ctx, &transaction.TxRequest{
			To:          &recipient,
			Data:        []byte{byte(i)},
			Value:       big.NewInt(0),
			GasLimit:    50_000,
			Description: "09_multi_burst",
		})
		results[i] = txResult{i, hash, receipt, err, time.Since(start)}
	}

	var summaries []scenarioResult
	for _, r := range results {
		sr := scenarioResult{
			Scenario:   "09_multi_burst",
			TxHash:     r.hash.Hex(),
			HasReceipt: r.receipt != nil,
			DurationMs: r.dur.Milliseconds(),
		}
		if r.receipt != nil {
			sr.Status = r.receipt.Status
		}
		if r.err != nil {
			sr.ErrorMsg = r.err.Error()
		}
		summaries = append(summaries, sr)
	}
	data, _ := json.MarshalIndent(summaries, "", "  ")
	writeFile(t, filepath.Join(env.outDir, "result.json"), data)
}

// --- Scenario 10: Probabilistic inclusion with low tip ---
func TestScenario_ProbabilisticInclusion(t *testing.T) {
	cfg := fastSimConfig()
	cfg.InclusionProbability = true
	cfg.InclusionMinProbability = 0.1
	cfg.BackgroundTipMean = big.NewInt(2_000_000_000)
	env := setupScenario(t, "10_probabilistic_inclusion", cfg, defaultRetryCfg())
	defer env.teardown(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sendOne(t, ctx, env, "10_probabilistic_inclusion")
}

// --- Scenario 11: BaseFee floor (empty blocks drive baseFee to minimum) ---
func TestScenario_BaseFeeDrop(t *testing.T) {
	cfg := fastSimConfig()
	cfg.InitialBaseFee = big.NewInt(100_000_000_000)
	cfg.InitialCongestion = 0.0
	env := setupScenario(t, "11_basefee_drop", cfg, defaultRetryCfg())
	defer env.teardown(t)

	time.Sleep(1 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sendOne(t, ctx, env, "11_basefee_drop")
}

// --- Scenario 12: Mempool TTL eviction during retry ---
func TestScenario_MempoolTTLEviction(t *testing.T) {
	cfg := fastSimConfig()
	cfg.MempoolTTL = 2
	cfg.InitialCongestion = 0.99
	retryCfg := defaultRetryCfg()
	retryCfg.RetryDelay = 500 * time.Millisecond
	env := setupScenario(t, "12_mempool_ttl_eviction", cfg, retryCfg)
	defer env.teardown(t)

	go func() {
		time.Sleep(2 * time.Second)
		env.sim.SetCongestion(0.0)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sendOne(t, ctx, env, "12_mempool_ttl_eviction")
}
