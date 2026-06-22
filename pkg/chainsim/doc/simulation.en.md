# Blockchain simulation (chainsim)

> Russian version: [simulation.md](simulation.md)

## Overview

The `chainsim` package implements a deterministic simulation of a minimal EIP-1559
chain. It is designed to test the `SendWithRetry` component in the `transaction`
package — the mechanism for sending transactions with automatic fee escalation,
retries, and nonce management.

`SimChain` implements the `transaction.Backend` interface, so the simulation can
replace a real RPC provider without changes to the code under test.

---

## Blockchain model

### Block production

The simulation produces blocks at a fixed `BlockPeriod` (default 5 s; tests
typically use 50–80 ms). `Run(ctx)` starts automatic block production until the
context is cancelled. Each block contains:

- Block number (monotonically increasing).
- Timestamp (previous + `BlockPeriod`).
- BaseFee (computed per EIP-1559).
- Gas used (`gasUsed`).
- Hashes of included user transactions.
- Synthetic tips for fee history.

`BlockPeriodJitter` adds a uniform random offset to the block period:
`[-jitter, +jitter]`. This models unstable intervals on a real network.

### Block gas and congestion

Each block has a gas limit `BlockGasLimit` (default 30,000,000). Block gas is
the sum of two components:

1. **User transactions** — use `(1 − congestion) × BlockGasLimit`.
2. **Synthetic background** — uses `congestion × BlockGasLimit`, modelling
   the rest of network traffic.

`InitialCongestion` (0.0–1.0) sets the initial share of background traffic.
0.0 means the network is idle; 1.0 means fully loaded (user transactions cannot
be included).

Congestion can be changed at runtime via `SetCongestion()` to model load spikes
and drops.

### Background stochasticity (CongestionStdDev)

On real Ethereum, block utilization fluctuates around a mean. `CongestionStdDev`
adds Gaussian noise to congestion **every block**:

    bgCongestion = congestion + N(0, CongestionStdDev)

This matters when `congestion ≈ 0.5`: without noise, background gas sits exactly
on the gas target and base fee stops decreasing even with an empty mempool. With
noise, lighter blocks have `gasUsed < target` and base fee decreases — like on a
real network.

Default is `CongestionStdDev = 0` (deterministic behaviour). Use 0.05–0.10 for
stress tests with `congestion > 0.3`.

### EIP-1559 base fee

Base fee is calculated with the standard EIP-1559 formula:

- `gasTarget = BlockGasLimit / 2`
- If `gasUsed > gasTarget`: base fee increases (up to +12.5% per block).
- If `gasUsed < gasTarget`: base fee decreases (up to −12.5% per block).
- If `gasUsed == gasTarget`: base fee is unchanged.

The initial value is `InitialBaseFee` (default 1 gwei). It can be changed at
runtime via `SetBaseFee()`.

### Base fee cap (MaxBaseFee)

`MaxBaseFee` sets an upper bound on base fee after the EIP-1559 calculation. If
the next value exceeds the cap, it is clamped to `MaxBaseFee`.

Default is `nil` (cap disabled).

This is useful in fast stress tests (`BlockPeriod` 50 ms) with sustained
congestion ≥ 0.5: without a cap, base fee can grow exponentially and exceed
`MaxTxPrice` in `SendWithRetry`, causing a false deadlock in the test. A typical
value for Gnosis-like scenarios is **500 gwei** (`500_000_000_000` wei).

### Per-transaction gas limits

`BaseGasUsed` (default 21,000) is the actual gas charged per transaction for
receipt calculation and balance deduction. If 0, the transaction `gasLimit` is
used. This models the difference between `gasLimit` (sender estimate) and
`gasUsed` (actual consumption).

---

## Mempool

### Accepting transactions

On `SendTransaction()`, the simulation checks:

1. **Nonce** — must be ≥ the sender's confirmed nonce. Otherwise —
   `nonce too low`.
2. **GasFeeCap ≥ baseFee** — the transaction must cover the current base fee.
3. **Effective tip ≥ MinMempoolTip** — minimum priority fee to enter the
   mempool (default 0.1 gwei).
4. **Balance** — `gas × gasFeeCap + value ≤ balance`.

### Transaction replacement (RBF)

If the mempool already has a transaction with the same (sender, nonce), a new
transaction can replace it when the **10% bump** rule is satisfied:

- `newTip ≥ oldTip × 1.10`
- `newFeeCap ≥ oldFeeCap × 1.10`

If the bump is insufficient — `replacement transaction underpriced`.

### Size limit

`MaxMempoolSize` limits the number of transactions in the mempool. When full,
the transaction with the lowest effective tip is evicted. If the new transaction
has a lower tip than the evicted one, it is rejected.

Default is 0 (no limit).

### TTL (time to live)

`MempoolTTL` is the lifetime of a transaction in blocks. If a transaction is not
included within `MempoolTTL` blocks after acceptance, it is removed when the
next block is produced.

Default: 10 minutes (computed as `10min / BlockPeriod`). Set `DisabledMempoolTTL`
to disable.

### Block inclusion

When producing a block, **eligible** transactions are selected from the mempool:

1. Transaction nonce must match the sender's confirmed nonce (sequential nonce
   chains from one sender are supported).
2. `GasFeeCap ≥ baseFee` of the current block.
3. Eligible transactions are sorted by descending effective tip.
4. Included until available gas `(1 − congestion) × BlockGasLimit` is exhausted.
5. `MaxTxsPerBlock` — additional cap on user transactions per block (default 0 —
   no limit).

---

## Fee history and synthetic background

### Synthetic tips

Each block generates synthetic tips for fee history, modelling background
network traffic:

- `BackgroundTipMean` — mean tip (default 2 gwei).
- `BackgroundTipStdDev` — standard deviation (default 0.5 gwei).
- Distribution — normal (Box–Muller), clamped at zero from below.

### FeeHistory

`SuggestedFeeAndTipsFromHistory()` returns suggested fees based on fee history
over the last `FeeHistoryDepth` blocks (default 100). Percentiles 10th, 50th,
and 90th are used by `SendWithRetry` to pick a tip tier (market, urgent,
aggressive).

If a block has no real tips (empty or genesis), `MinMempoolTip` is used as a
fallback.

---

## Probabilistic inclusion

With `InclusionProbability` enabled, a transaction whose tip is below the
"reference" tip (mean background tip) is not guaranteed inclusion; it is included
with a probability proportional to its tip relative to the reference.

`InclusionMinProbability` sets the minimum inclusion chance even for zero-tip
transactions (default 0.1 = 10%).

This models low-tip transactions waiting a long time for inclusion on a real
network.

---

## Reverts

### Targeted revert

`SetRevertAddress(addr)` — all transactions to that address revert
(receipt.Status = 0).

### Random revert

`RandomRevertRate` (0.0–1.0) — probability that any executed transaction
reverts. 0 disables; 1.0 reverts all transactions.

---

## Receipt delay

`ReceiptAvailDelay` (in blocks) models delayed receipt availability.
`TransactionReceipt()` returns `NotFound` until the given number of blocks have
passed after inclusion.

This is important for testing interaction with `Monitor`, which polls for
receipts and may falsely treat a transaction as cancelled when
`cancellationDepth < ReceiptAvailDelay`.

---

## Error injection

`InjectError(method, err, count)` forces the given RPC method to return `err` for
the next `count` calls. Supported methods:

- `SendTransaction`
- `TransactionReceipt`
- `HeaderByNumber`
- `BlockNumber`
- `PendingNonceAt`
- `NonceAt`
- `EstimateGas`
- `SuggestedFeeAndTipsFromHistory`
- `SuggestedFeeAndTip`
- `FeeHistory`
- `BalanceAt`

Errors are applied sequentially (FIFO). `InjectError` can be called multiple
times to queue errors.

---

## History retention (HistoryRetentionBlocks)

On long runs (tens of thousands of blocks), receipt, minedTx, and nonceHistory
growth is unbounded. `HistoryRetentionBlocks` limits retention depth:

- Receipts and minedTxs for blocks older than
  `blockNum − HistoryRetentionBlocks` are removed.
- nonceHistory is pruned (the last entry before the cutoff is kept for correct
  interpolation).

Default is 0 (full history). Use 2000–5000 for stress tests longer than 5
minutes.

---

## Determinism

The simulation uses a deterministic PRNG (`RNGSeed`). With the same seed,
configuration, and call order, results are reproducible. This allows exact
scenario replay and debugging.

Default is `RNGSeed = 1`.

---

## Runtime control

Simulation parameters can be changed at runtime via:

- `SetCongestion(ratio)` — load level.
- `SetBaseFee(fee)` — force base fee.
- `SetMinMempoolTip(tip)` — mempool admission floor.
- `SetBackgroundTipMean(tip)` / `SetBackgroundTipStdDev(stdDev)` —
  background tip distribution.
- `SetBalance(addr, bal)` — account balance.
- `SetNonce(addr, nonce)` — confirmed nonce.
- `SetEstimateGas(gas)` — EstimateGas response.
- `SetRevertAddress(addr)` — revert target address.
- `InjectError(method, err, count)` — error injection.

---

## Observability

### Structured logging (JSONL)

`SetLogger(logger)` attaches a structured logger. Each event is a JSON object with
fields `ts`, `source`, `level`, `msg`, `fields`. The logger can be attached to
both `SimChain` and `transaction.Service`:

- `source: "chainsim"` — simulation events (new block, transaction
  accepted/rejected/executed/deferred, TTL eviction).
- `source: "sendwithretry"` — send events (suggest gas fees, broadcast, receipt
  received, finished with error, retry configuration).

### Log rotation (RotatingJSONLWriter)

For long runs, `RotatingJSONLWriter` splits output into time-based files. The
interval is set by `HIGHLOAD_ROTATE` (default 30 s in stress tests). File names:
`events_0000s-0030s.jsonl`, `events_0030s-0060s.jsonl`, etc.

### Statistics

`Stats()` returns cumulative counters:

- `BlocksProduced` — blocks produced.
- `TransactionsReceived` / `Accepted` / `Rejected` — transaction flow.
- `TransactionsExecuted` / `Reverted` — inclusion outcomes.
- `MempoolReplacements` — replacement (RBF) count.
- `MempoolTTLExpirations` — TTL evictions.
- `InclusionDeferred` — deferred inclusions (probabilistic model).
- `RejectionReasons` — map of rejection reasons (nonce_too_low,
  replacement_underpriced, etc.).

### Snapshot

`Snapshot()` returns a full simulation state snapshot: blocks, receipts,
minedTxs, nonces, balances, mempool, configuration.

---

# Testing SendWithRetry with the simulation

## Test architecture

Tests use `SimChain` as a drop-in replacement for a real backend. `SimChain` is
wired to `transaction.NewService()` and `transaction.NewMonitor()`. The code under
test (`SendWithRetry`) does not know it is talking to a simulation.

All tests are built with the `scenario` build tag:

```bash
go test -tags=scenario ./pkg/chainsim/...
```

## Scenario tests (scenarios_test.go)

Each scenario is a separate Go test that:

1. Creates a `SimChain` with the required configuration.
2. Optionally changes parameters at runtime (congestion, base fee, errors).
3. Sends one or more transactions via `SendWithRetry`.
4. Writes artifacts: `events.jsonl`, `state.json`, `stats.json`, `result.json`.

### Scenario list

| # | Scenario | What it tests |
|---|----------|---------------|
| 01 | Happy path | Baseline: transaction succeeds without issues |
| 02 | Congestion spike + drop | Congestion=1.0 blocks inclusion; drops to 0.1 after 600 ms |
| 03 | BaseFee spike mid-retry | BaseFee jumps to 20 gwei; retry escalates through tiers |
| 04 | Transient RPC errors | HeaderByNumber and TransactionReceipt fail (3 and 10 times) |
| 05 | Receipt delay | Receipt appears only 3 blocks after inclusion |
| 06 | All tiers exhausted | Congestion=1.0 permanently; all attempts exhausted |
| 07 | Transaction revert | Target address configured to revert |
| 08 | Random revert rate | RandomRevertRate=1.0; all transactions revert |
| 09 | Multi-tx burst | 5 sequential SendWithRetry calls, MaxTxsPerBlock=3 |
| 10 | Probabilistic inclusion | Inclusion probability depends on tip |
| 11 | BaseFee drop | Initial baseFee=100 gwei, congestion=0 — base fee falls |
| 12 | Mempool TTL eviction | MempoolTTL=2 blocks; tx evicted and re-submitted |

## Stress tests (highload_*_test.go)

Long high-load tests exercise `SendWithRetry` under pressure: many parallel
workers, log rotation, periodic dumps, and chaos drivers. Shared infrastructure
lives in `highload_test.go` (harness) and `stress_helpers_test.go` (drivers,
outcome classification, nonce checks).

Default duration per test is **15 minutes** (`STRESS_DURATION`). The
`run-scenarios.sh` script also sets dumps every **minute** (`HIGHLOAD_DUMP=60s`).

### Stress test list

| Test | File | Artifact directory | What it tests |
|------|------|------------------|---------------|
| `TestHighload_HappyPathStoreClean` | `highload_test.go` | `highload_happy/` | Store cleanliness under calm network and light RPC faults |
| `TestHighload_StressNonceAndRetry` | `highload_test.go` | `highload_stress/` | Nonce and retry under congestion waves, `MaxTxsPerBlock=2` |
| `TestHighload_BoundedMempoolEviction` | `highload_bounded_mempool_test.go` | `highload_bounded_mempool/` | Fee escalation with small mempool and TTL |
| `TestHighload_StuckNonceMempoolDrop` | `highload_stuck_nonce_test.go` | `highload_stuck_nonce/` | Recovery after TTL eviction and base-fee storms |
| `TestHighload_CrashRecovery` | `highload_crash_recovery_test.go` | — | `resumeRetryTransactions` across periodic service restarts |
| `TestHighload_HardCancelMidRetry` | `highload_hard_cancel_test.go` | — | Context cancellation + restarts without duplicate nonces |
| `TestHighload_BlockJitterVsRetryDelay` | `highload_jitter_test.go` | `highload_jitter/` | Block jitter vs short `RetryDelay` |
| `TestHighload_ProbabilisticInclusion` | `highload_probabilistic_test.go` | `highload_probabilistic/` | Eventual inclusion under probabilistic selection |
| `TestHighload_RevertsUnderLoad` | `highload_reverts_test.go` | `highload_reverts/` | Reverts (`RandomRevertRate`) under load |
| `TestHighload_ReceiptDelayNoFalseCancel` | `highload_receipt_delay_test.go` | `highload_receipt_delay_ok/` | No false cancels with correct `cancellationDepth` |
| `TestHighload_ReceiptDelayMisconfig` | `highload_receipt_delay_test.go` | `highload_receipt_delay_bad/` | Negative test: shallow `cancellationDepth` |
| `TestHighload_RPCChaos` | `highload_rpc_chaos_test.go` | `highload_rpc_chaos/` | Random RPC errors on all backend methods |
| `TestHighload_SoakNoLeaks` | `highload_soak_test.go` | `highload_soak/` | Goroutine/store leaks and history pruning |

`CrashRecovery` and `HardCancelMidRetry` use a custom setup without the full
harness (no `dumps/` or `logs/`).

### Key default parameters

**Happy path** (`TestHighload_HappyPathStoreClean`):

- `InitialCongestion=0.2`, `CongestionStdDev=0.05`
- 8 workers, light `lightReadRPCFaultDriver`

**Stress nonce** (`TestHighload_StressNonceAndRetry`):

- `InitialCongestion=0.5`, `CongestionStdDev=0.15`, `MaxTxsPerBlock=2`
- `MaxBaseFee=500 gwei`, `congestionWaveDriver` (0.85 ↔ 0.3)
- 16 workers, completion rate > 10%

### Environment variables for stress tests

| Variable | Default | Description |
|---|---|---|
| `STRESS_DURATION` | `15m` | Duration of each stress test |
| `STRESS_QUIET_TAIL` | `2m` | Quiet tail before final assertions; chaos drivers stop earlier |
| `HIGHLOAD_WORKERS` | 8–16 (per test) | Number of parallel workers |
| `HIGHLOAD_ROTATE` | `30s` | JSONL log rotation interval |
| `HIGHLOAD_DUMP` | `10s` (in code) / `60s` (in `run-scenarios.sh`) | Periodic dump interval |
| `HIGHLOAD_TIMEOUT` | `4h` | `go test` timeout when using `run-scenarios.sh` |
| `CRASH_RESTART_EVERY` | `90s` | Service restart interval in crash/cancel tests |

Dump file names depend on the interval: with `HIGHLOAD_DUMP >= 1m` —
`dump_001m.json`; otherwise — `dump_0010s.json` (seconds).

### Chaos drivers (stress_helpers_test.go)

Used in selected tests to model an unstable network. Stopped during the quiet
tail (`safeCalmAt` / `STRESS_QUIET_TAIL`):

- `congestionWaveDriver` — periodic congestion up/down.
- `baseFeeStormDriver` — base fee spikes.
- `rpcFaultDriver` / `lightReadRPCFaultDriver` — random RPC errors.

### Stress test observability

Stress tests produce a rich set of artifacts:

**Logs** (`logs/`):

- Rotating JSONL files with simulation and `SendWithRetry` events.
- File names: `events_0000s-0030s.jsonl`, `events_0030s-0060s.jsonl`, etc.
  (interval = `HIGHLOAD_ROTATE`, default 30 s).

**Dumps** (`dumps/`):

- `dump_start.json` — full state snapshot before the run.
- `dump_NNNm.json` or `dump_NNNNs.json` — periodic light dumps (sim state,
  store counters, workload progress).
- `dump_final_light.json` — light final dump.
- `dump_final.json` — full state snapshot after completion.

**Summary** (`summary.json`):

- Final workload counters (completed, failed, in_flight).
- Store contents (total_keys, counts by prefix, nonce range).
- Assertion results (PASS/FAIL).

## Running tests

### All scenario tests:

```bash
./pkg/chainsim/run-scenarios.sh
```

### One scenario:

```bash
./pkg/chainsim/run-scenarios.sh TestScenario_CongestionSpikeDrop
```

### Stress tests only:

```bash
./pkg/chainsim/run-scenarios.sh TestHighload
```

The script runs all `TestHighload_*` with `STRESS_DURATION=15m`,
`HIGHLOAD_DUMP=60s`, `HIGHLOAD_TIMEOUT=4h` and writes artifacts to
`pkg/chainsim/highload-results-<timestamp>/`.

### Stress test with custom parameters:

```bash
STRESS_DURATION=30m HIGHLOAD_WORKERS=20 HIGHLOAD_DUMP=60s \
  go test -tags=scenario -run TestHighload_StressNonceAndRetry \
  -timeout=35m ./pkg/chainsim/...
```

### Run with a custom output directory:

```bash
SCENARIO_OUTPUT_DIR=/tmp/my-run \
  go test -tags=scenario -run TestHighload -timeout=10m ./pkg/chainsim/...
```

## What you can test

### Nonce management correctness

- **Nonce race.** Many parallel workers compete for nonces.
  `TestHighload_StressNonceAndRetry` and related tests check for no duplicates
  or gaps.
- **Nonce on replacement.** When escalating tiers, `SendWithRetry` replaces the
  transaction with the same nonce but a higher fee. Scenarios 02, 03.

### Retry logic correctness

- **Tier escalation.** Scenario 03: base fee jumps; retry goes through
  market → urgent → aggressive.
- **Attempts exhausted.** Scenario 06: all tiers exhausted; expect
  `ErrAllAttemptsExhausted`.
- **Price cap exceeded.** Stress tests with high congestion and no `MaxBaseFee`:
  base fee can grow above `MaxTxPrice`. Tests with `MaxBaseFee=500 gwei` model a
  realistic network ceiling.
- **Retry after RPC errors.** Scenario 04: transient errors on `HeaderByNumber`
  and `TransactionReceipt`; retry survives them.

### Storage cleanliness

- **No pending/retry leaks.** `TestHighload_HappyPathStoreClean` and
  `TestHighload_SoakNoLeaks`: after completion only `stored_` entries remain (or
  retry/pending are empty).
- **Stored count = completed.** Number of stored transactions matches successful
  completions.

### Behaviour under congestion

- **Full load.** Scenario 02: congestion=1.0; user transactions are not included.
- **Partial load.** `TestHighload_StressNonceAndRetry`: congestion=0.5,
  `MaxTxsPerBlock=2`; transactions compete for block space.
- **Base fee oscillation.** CongestionStdDev creates realistic swings: base fee
  rises under load and falls when idle.

### Mempool

- **TTL eviction.** Scenario 12: transaction evicted from mempool by TTL and must
  be re-submitted.
- **Replacement (RBF).** The 10% bump rule is exercised on every retry that
  raises the tip.
- **Size limit.** MaxMempoolSize caps the mempool; when full, the lowest-tip
  transaction is evicted.

### Delays and timeouts

- **Receipt delay.** Scenario 05: receipt appears N blocks after inclusion.
- **Monitor interaction.** Monitor `cancellationDepth` must be ≥
  `ReceiptAvailDelay`, otherwise the monitor falsely treats the transaction as
  cancelled.

### Reverts

- **Deterministic revert.** Scenario 07: revert by address.
- **Probabilistic revert.** Scenario 08: RandomRevertRate=1.0.
- **SendWithRetry on revert.** The call completes (receipt received) but
  receipt.Status=0.

### Probabilistic inclusion

- **Low tip.** Scenario 10: below-average tip is included with probability
  proportional to tip/reference.
- **Minimum probability.** InclusionMinProbability guarantees a non-zero
  inclusion chance even at minimum tip.

---

## Simulation limitations

1. **Single sender.** The simulation supports multiple accounts, but
   `transaction.Service` is bound to one sender. Multi-sender tests need
   multiple service instances.

2. **No smart contracts.** `CallContract` and `CodeAt` return
   `ErrNotImplemented`. The simulation only handles simple transfers.

3. **No uncle/ommer blocks.** The chain is linear; there are no reorgs.

4. **No WebSocket subscriptions.** Monitor uses polling on an interval, not
   new-block subscriptions.

5. **Mock state store.** Tests use a mock store (`statestore/mock`) whose
   `Iterate` scans all keys in O(N). With many `stored_` entries (> 50K) this
   degrades throughput. In production (LevelDB), `Iterate` uses prefix-seek and
   is O(matches). This is a test artifact, not a production issue.

6. **Synthetic tips.** Background tips use a normal distribution. On a real
   network the distribution may differ (heavy tails, bimodality). This is an
   acceptable simplification for testing retry logic.

7. **Uniform gas per tx.** `BaseGasUsed` is the same for all transactions. In
   reality different transactions use different gas.

8. **No legacy gasPrice priority.** The simulation only supports EIP-1559
   (Type 2) transactions.

9. **Base fee without market feedback.** Congestion is set independently of base
   fee (load does not automatically drop when blocks are expensive). For long
   fast-block runs use `MaxBaseFee` or keep `InitialCongestion` below the gas
   target (~0.5).
