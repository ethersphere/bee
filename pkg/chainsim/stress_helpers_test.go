// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build scenario

package chainsim_test

import (
	"context"
	"math/big"
	"math/rand"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/chainsim"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/stretchr/testify/require"
)

// stressDuration is the default duration for stress tests (15 minutes).
func stressDuration() time.Duration {
	return envDuration("STRESS_DURATION", 15*time.Minute)
}

// quietTail is the calm-down window before the end of a stress run so that
// pending keys and background goroutines can drain before final assertions.
func quietTail() time.Duration {
	return envDuration("STRESS_QUIET_TAIL", 2*time.Minute)
}

func commonRecipient() common.Address {
	return common.HexToAddress("0xabcd")
}

func defaultHighloadTxRequest(desc string) *transaction.TxRequest {
	to := commonRecipient()
	return &transaction.TxRequest{
		To:          &to,
		Data:        []byte{0xab, 0xcd},
		Value:       big.NewInt(0),
		GasLimit:    50_000,
		Description: desc,
	}
}

// ---------------------------------------------------------------------------
// Outcome classification via transaction.RetryOutcomeLabel
// ---------------------------------------------------------------------------

type outcomeCounters struct {
	mu        sync.Mutex
	byLabel   map[string]int64
	completed atomic.Int64
	inFlight  atomic.Int64
}

func newOutcomeCounters() *outcomeCounters {
	return &outcomeCounters{byLabel: map[string]int64{}}
}

func (o *outcomeCounters) record(err error) {
	label := transaction.RetryOutcomeLabel(err)
	o.mu.Lock()
	o.byLabel[label]++
	o.mu.Unlock()
}

func (o *outcomeCounters) get(label string) int64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.byLabel[label]
}

func (o *outcomeCounters) snapshot() map[string]int64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make(map[string]int64, len(o.byLabel))
	for k, v := range o.byLabel {
		out[k] = v
	}
	return out
}

// instrumentedWorker is like highloadWorker but classifies each outcome.
func instrumentedWorker(ctx context.Context, stop <-chan struct{}, svc transaction.Service, oc *outcomeCounters, wl *workloadCounters) {
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		default:
		}
		oc.inFlight.Add(1)
		if wl != nil {
			wl.inFlight.Add(1)
		}
		_, receipt, err := svc.SendWithRetry(ctx, defaultHighloadTxRequest("stress"))
		oc.inFlight.Add(-1)
		if wl != nil {
			wl.inFlight.Add(-1)
		}
		oc.record(err)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if wl != nil {
				wl.failed.Add(1)
			}
			continue
		}
		if receipt != nil {
			oc.completed.Add(1)
			if wl != nil {
				wl.completed.Add(1)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Nonce invariant checks
// ---------------------------------------------------------------------------

type nonceCheck struct {
	count    int
	dup      bool
	gap      bool
	maxNonce uint64
}

func checkNonces(nonces []uint64) nonceCheck {
	sort.Slice(nonces, func(i, j int) bool { return nonces[i] < nonces[j] })
	res := nonceCheck{count: len(nonces)}
	for i := 1; i < len(nonces); i++ {
		if nonces[i] == nonces[i-1] {
			res.dup = true
		}
		if nonces[i] != nonces[i-1]+1 {
			res.gap = true
		}
	}
	if len(nonces) > 0 {
		res.maxNonce = nonces[len(nonces)-1]
	}
	return res
}

// uniqueNonces returns sorted deduplicated nonces.
func uniqueNonces(nonces []uint64) []uint64 {
	if len(nonces) == 0 {
		return nil
	}
	sort.Slice(nonces, func(i, j int) bool { return nonces[i] < nonces[j] })
	out := make([]uint64, 0, len(nonces))
	var prev uint64
	first := true
	for _, n := range nonces {
		if first || n != prev {
			out = append(out, n)
			prev = n
			first = false
		}
	}
	return out
}

// assertOnChainNonceChain verifies the confirmed on-chain nonce matches a contiguous
// stored nonce range. When allowStoredDup is true, duplicate stored entries for the
// same nonce are ignored (can occur under SendTransaction RPC chaos).
func assertOnChainNonceChain(t *testing.T, confirmed uint64, nonces []uint64, allowStoredDup bool) {
	t.Helper()
	if len(nonces) == 0 {
		return
	}
	check := checkNonces(nonces)
	if !allowStoredDup {
		require.False(t, check.dup, "duplicate stored nonce")
		require.False(t, check.gap, "nonce gap in stored")
		require.Equal(t, confirmed, check.maxNonce+1, "confirmed nonce behind stored max")
		return
	}
	unique := uniqueNonces(nonces)
	ucheck := checkNonces(unique)
	require.False(t, ucheck.gap, "nonce gap in unique stored nonces")
	require.Equal(t, confirmed, ucheck.maxNonce+1, "confirmed nonce behind unique stored max")
	if check.dup {
		t.Logf("warning: %d duplicate stored nonce entries under RPC chaos", len(nonces)-len(unique))
	}
}

func collectNoncesFrom(insp storemock.InspectableStore) []uint64 {
	keys := insp.KeysWithPrefix(storedPrefix)
	nonces := make([]uint64, 0, len(keys))
	for _, k := range keys {
		var st transaction.StoredTransaction
		if err := insp.Get(k, &st); err != nil {
			continue
		}
		nonces = append(nonces, st.Nonce)
	}
	sort.Slice(nonces, func(i, j int) bool { return nonces[i] < nonces[j] })
	return nonces
}

// ---------------------------------------------------------------------------
// Goroutine leak detection
// ---------------------------------------------------------------------------

func goroutineBaseline() int {
	settleGoroutines()
	return runtime.NumGoroutine()
}

func settleGoroutines() {
	for range 5 {
		runtime.GC()
		time.Sleep(100 * time.Millisecond)
	}
}

func assertGoroutinesSettled(t *testing.T, baseline, delta int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last int
	for time.Now().Before(deadline) {
		settleGoroutines()
		last = runtime.NumGoroutine()
		if last <= baseline+delta {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("goroutine leak: baseline=%d current=%d delta_allowed=%d", baseline, last, delta)
}

// ---------------------------------------------------------------------------
// Chaos drivers (stop when stop channel closes or calmAt is reached)
// ---------------------------------------------------------------------------

func congestionWaveDriver(stop <-chan struct{}, sim *chainsim.SimChain, high, low, calmLevel float64, period time.Duration, calmAt time.Time) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	up := true
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if time.Now().After(calmAt) {
				sim.SetCongestion(calmLevel)
				return
			}
			if up {
				sim.SetCongestion(high)
			} else {
				sim.SetCongestion(low)
			}
			up = !up
		}
	}
}

func baseFeeStormDriver(stop <-chan struct{}, sim *chainsim.SimChain, stormFee, calmFee *big.Int, period time.Duration, calmAt time.Time) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	storm := true
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if time.Now().After(calmAt) {
				sim.SetBaseFee(calmFee)
				return
			}
			if storm {
				sim.SetBaseFee(stormFee)
			} else {
				sim.SetBaseFee(calmFee)
			}
			storm = !storm
		}
	}
}

func rpcFaultDriver(stop <-chan struct{}, sim *chainsim.SimChain, rng *rand.Rand, period time.Duration, calmAt time.Time) {
	methods := []string{
		"SendTransaction", "TransactionReceipt", "HeaderByNumber",
		"BlockNumber", "PendingNonceAt", "NonceAt", "EstimateGas",
		"SuggestedFeeAndTipsFromHistory", "FeeHistory", "BalanceAt",
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if time.Now().After(calmAt) {
				return
			}
			m := methods[rng.Intn(len(methods))]
			count := 1 + rng.Intn(3)
			sim.InjectError(m, context.DeadlineExceeded, count)
		}
	}
}

func lightReadRPCFaultDriver(stop <-chan struct{}, sim *chainsim.SimChain, rng *rand.Rand, period time.Duration, calmAt time.Time) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if time.Now().After(calmAt) {
				return
			}
			switch rng.Intn(2) {
			case 0:
				sim.InjectError("HeaderByNumber", context.DeadlineExceeded, 1)
			case 1:
				sim.InjectError("TransactionReceipt", context.DeadlineExceeded, 1)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// feeCapRecorder wraps SimChain and records MaxTxPrice violations.
// ---------------------------------------------------------------------------

type feeCapRecorder struct {
	*chainsim.SimChain
	limit      *big.Int
	violations atomic.Int64
}

var _ transaction.Backend = (*feeCapRecorder)(nil)

func newFeeCapRecorder(sim *chainsim.SimChain, limit *big.Int) *feeCapRecorder {
	return &feeCapRecorder{SimChain: sim, limit: limit}
}

func (f *feeCapRecorder) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if f.limit != nil && tx.GasFeeCap().Cmp(f.limit) > 0 {
		f.violations.Add(1)
	}
	return f.SimChain.SendTransaction(ctx, tx)
}

// ---------------------------------------------------------------------------
// highloadOpts configures setupHighload behaviour.
// ---------------------------------------------------------------------------

type highloadOpts struct {
	backend             func(*chainsim.SimChain) transaction.Backend
	cancellationDepth   uint64
	pendingDrainTimeout time.Duration
}

func defaultHighloadOpts() highloadOpts {
	return highloadOpts{
		cancellationDepth:   2,
		pendingDrainTimeout: 30 * time.Second,
	}
}

func withFeeCapBackend(limit *big.Int) func(*chainsim.SimChain) transaction.Backend {
	return func(sim *chainsim.SimChain) transaction.Backend {
		return newFeeCapRecorder(sim, limit)
	}
}

func withCancellationDepth(depth uint64) highloadOpts {
	opt := defaultHighloadOpts()
	opt.cancellationDepth = depth
	return opt
}

// gnosisLikeConfig returns a Gnosis-chain-like preset (slow blocks, realistic gas).
func gnosisLikeConfig() chainsim.Config {
	cfg := chainsim.DefaultConfig()
	cfg.BlockPeriod = 5 * time.Second
	cfg.BlockPeriodJitter = 1 * time.Second
	cfg.BlockGasLimit = 17_000_000
	cfg.InitialBaseFee = big.NewInt(1_000_000_000)
	cfg.BackgroundTipMean = big.NewInt(1_500_000_000)
	cfg.CongestionStdDev = 0.08
	cfg.MempoolTTL = chainsim.DisabledMempoolTTL
	return cfg
}
