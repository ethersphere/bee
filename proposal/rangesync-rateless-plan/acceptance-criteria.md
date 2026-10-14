# `rangesync` acceptance criteria (draft)

**Status:** first draft from T6.0 (wave A), for the team to refine. T6.4 turns it into the final
criteria and reports results against each row. Proposal references (§n) point into
[`../pullsync-rangesync-rateless-proposal.md`](../pullsync-rangesync-rateless-proposal.md);
open question 10 (§14 Q10) is what this file answers.

**What "accepted" means here:** `rangesync`-only and dual nodes meet every **gating** row below in
the T6.1 suite and in the T6.3 testnet cohorts. Rows marked *informational* are reported but don't
gate. Passing the criteria lets someone write the separate `pullsync` phase-out proposal. It does
not flip any default.

**Independence from open decisions.** These criteria hold whichever way the open decisions go.
They don't depend on the outcome of gate G3 (T2.0 recommends Rateless IBLT, not decided yet)
beyond what the proposal already states: the decoding rows use the proposal's own numbers (§1.1,
§4.3.7, §10, §13). If G3 picks CertainSync, T2.0 already says the [1.3, 1.8] band would not hold,
and AC-K1/AC-K2 must be re-based. Memory rows follow the T1.2 (LtHash16 vs ECMH) and T2.6
(`BucketSymbols`) decisions. Time rows use the constants as fixed at gate G6 (T4.6).

---

## 1. Definitions and measurement conditions

| Term | Meaning |
|---|---|
| Metric names | §10 names live in the `rangesync` subsystem of the `bee` namespace, so `bytes_metadata` is exported as `bee_rangesync_bytes_metadata`. This file uses the short §10 names. The full prefix is an assumption until T1.13 lands; see the Phase 1 follow-up. |
| Cohorts (T6.3) | `pullsync`-only (defaults), `rangesync`-only (`--puller-enable=false --rangesync-enable=true`), dual (both `true`). A `rangesync` cohort covers **whole neighborhoods** (D12) and runs on **amd64 only** (D22). |
| Scenarios (T6.1) | Baseline, `rangesync` only, Mixed, Bootstrap, Radius decrease, Stale stamp, Churn (T6.1 table). |
| Reference load | Uploads into the neighborhood at a rate where the median `pairwise_diff` per steady-state session is **≥ 20**. §7.8 assumes this regime ("tens to hundreds of differences per pair"). The low-load regime (median ≤ 1) is reported separately: there the per-round `MinBatch` overhead (~330 B per pair, §7.8) dominates. |
| Steady state | No bootstrap or radius-decrease stripes running, combined `SyncRate() == 0`, uploads at the reference load. |
| Reference reserve | 2²² ≈ 4.2M chunks, B = 4096 buckets (`LeafBits = 12`), the size §5.2, §5.3 and §7.4 use. |
| ID wire size | ≈ 70 B per `ID` (§6.2). §7.4's "~270 MB" uses 64 B per ID (2²² × 64 B). The criteria use 70 B. |
| Metadata (for AC-B*) | All `rangesync` bytes except chunk payloads: `status`, `reconcile`, and `fetch` `Want` messages. Chunk bytes are `Delivery` messages. The Phase 1 follow-up asks T1.13 to define `bytes_metadata` / `bytes_chunks` this way. |
| `pullsync` metadata baseline | `pullsync`/`puller` have no byte counters and can't be changed. Baseline = `bee_pullsync_chunks_offered` × ~102 B (one `pb.Chunk` entry in an `Offer`: Address 32 + BatchID 32 + StampHash 32 + framing). `Want` bitvectors and `Get` messages are ignored (< 1 B per offer). This matches the "100 B per offer" in §7.8. |
| `RoundInterval` | The adaptive value (5–30 s, T4.3) at the moment of the upload. Bounds are as fixed at G6. |

---

## 2. Criteria

Columns: **ID**, **criterion and threshold**, **metric(s) or signal**, **measured by**, **gating?**.
"Owner" for every metric is in §4.

### 2.1 Correctness

| ID | Criterion and threshold | Metric(s) / signal | Measured by | Gating |
|---|---|---|---|---|
| AC-C1 | At the end of every T6.1 scenario, every node's reserve inside its root equals the union of its neighbors' reserves restricted to that root (in-radius chunks with valid stamps). Its Index equals a brute-force rebuild (counts, digest, symbol caches). | test assertions | T6.1 (all scenarios), T4.6, T1.13 / T2.4 property tests | yes |
| AC-C2 | **Quiescence.** Once uploads stop, all digests in a neighborhood become equal. Within `StatusMaxInterval` + 2 × `RoundInterval` (≤ 120 s with the current bounds), `sessions_total` stops increasing on every node and the last `pairwise_diff` observations are 0. On testnet this stands in for AC-C1, since reserves can't be compared directly. | `sessions_total{result}`, `pairwise_diff` | T6.1, T6.3 (upload pause in each cohort run) | yes |
| AC-C3 | Stale stamp (§7.7): both nodes end with the newer stamp. The older ID is not refetched in later rounds: it enters the rejected cache and `rejected_cache_size` stays bounded. | test assertions, `rejected_cache_size` | T2.4, T4.4, T6.1 Stale stamp | yes |
| AC-C4 | Edge peers (§7.6): a node with a nested root syncs only the overlapping range. Disjoint roots → no session. | test assertions | T1.8, T4.2, T6.1 Mixed / Churn | yes |

### 2.2 Convergence

| ID | Criterion and threshold | Metric(s) / signal | Measured by | Gating |
|---|---|---|---|---|
| AC-V1 | Steady state, no faults: a chunk pushed to p ≈ 3 nodes is on all N nodes of the neighborhood within **2 × `RoundInterval`** (§3 goal 4 promises one round; the second covers status debounce and the initiator rule of §8.2). Measured for N = 9 and 17. | test timings; testnet: `round_interval_seconds` (proposed) + probe uploads | T4.6, T6.1 `rangesync` only / Mixed; T6.3 probe | yes (T6.1); informational on testnet until a probe exists |
| AC-V2 | A steady-state session (`pairwise_diff` < `CatchUpMinDiff`) needs **median ≤ 2, p95 ≤ 3** round trips (§7.2: 1.5 round trips; §7.3: 4 messages for a split). | `session_rounds` | T2.6 worked examples, T4.6, T6.3 | yes |
| AC-V3 | Bootstrap: an empty `rangesync`-only node reaches `sync_rate == 0` and `isFullySynced == true` **no later than** a `pullsync`-only node bootstrapping the same neighborhood (same reserve size and neighbor count). Ratio ≤ 1.0. Both rates use the same 10-min window, so the window lag cancels out. | `sync_rate` vs `bee_puller_pullsync_rate`; `/redistributionstate` `isFullySynced` | T6.1 Bootstrap (vs Baseline), T6.3 | yes |
| AC-V4 | Radius decrease: the new half is complete (`sync_rate` back to 0) no later than the `pullsync`-only node after the same decrease. | `sync_rate`, `bee_localstore_storage_radius` | T6.1 Radius decrease, T6.3 | yes |
| AC-V5 | Idle synced neighborhood (§7.1): no sessions (`sessions_total` flat). Metadata ≤ one `Status` (~100 B) per `StatusMaxInterval` (60 s) per neighbor in each direction, plus at most one per root change, debounced to `StatusMinInterval` (5 s). | `sessions_total`, `bytes_metadata{dir}` | T4.1, T6.3 | yes |

### 2.3 Bandwidth

| ID | Criterion and threshold | Metric(s) / signal | Measured by | Gating |
|---|---|---|---|---|
| AC-B1 | At the reference load, `rangesync` metadata bytes per new neighborhood chunk are **≤ 50 %** of `pullsync`'s on the same load and topology. Prediction (§7.8, symbols only): 28–35 % at N = 9, 17–22 % at N = 17. With `Want` IDs (~70 B per lacking node): ~33–40 % and ~21–26 %. | Σ `bytes_metadata{dir="in"}` over the neighborhood ÷ chunks uploaded, vs Σ `bee_pullsync_chunks_offered` × 102 B ÷ chunks uploaded | T4.6 model + integration, T6.1 Baseline vs `rangesync` only, T6.3 | yes |
| AC-B1b | Low-load regime (median `pairwise_diff` ≤ 1): report metadata per chunk for both protocols. | same | T6.3 | informational |
| AC-B2 | Bootstrap ID metadata ≈ 1× the reserve's IDs (§7.4): `bytes_metadata{dir="in"}` during bootstrap **≤ 1.15 × final reserve count × 70 B**, plus verification **≤ 1 MB** (§7.4 says "a few KB"). For 2²²: ≤ ~340 MB, against ~3.4 GB for `pullsync` with 8 neighbors (§7.4). | `bytes_metadata{dir}`; verification bytes need the Phase 3 stripe/verification metrics (proposed) | T3.2 (order of magnitude), T6.1 Bootstrap, T6.3 | yes |
| AC-B3 | Bootstrap chunk data: each chunk once. `bytes_chunks{dir="in"}` **≤ 1.01 ×** the total size of the chunks stored by the bootstrap. | `bytes_chunks{dir}`, `chunks_fetched_total` (proposed) | T3.2, T6.1 Bootstrap | yes |
| AC-B4 | Radius decrease (§7.5): only the new half moves. The old half costs one fingerprint exchange per peer (< 1 KB). Chunk bytes ≈ size of the new half. | `bytes_metadata`, `bytes_chunks` | T3.4, T6.1 Radius decrease | yes |

### 2.4 Duplicates

| ID | Criterion and threshold | Metric(s) / signal | Measured by | Gating |
|---|---|---|---|---|
| AC-D1 | `rangesync`-only, no faults: **zero** duplicate `rangesync` deliveries (each ID delivered at most once per node; §3 goal 3). With faults (stalls, churn): duplicates ≤ number of expired claims. | test instrumentation (Put count per ID); `want_claims{result}` | T3.2, T4.4 (§7.8 literal test), T4.6, T6.1 `rangesync` only / Bootstrap / Churn | yes |
| AC-D2 | Testnet `rangesync`-only cohort: `cross_protocol_duplicates_total` ÷ chunks fetched **≤ 1 %** (draft). With `pullsync` off, this counter also catches pushsync racing a fetch and any late `rangesync` delivery, so it is an upper bound on AC-D1. | `cross_protocol_duplicates_total`, `chunks_fetched_total` (proposed), `want_claims{result="late"}` (proposed) | T6.3 | yes |
| AC-D3 | Dual mode: report the cross-protocol duplicate ratio. It is accepted by design (§3 non-goal) and kept low by the `ReserveHasID` re-check before `Want`. | `cross_protocol_duplicates_total`, `chunks_fetched_total` (proposed) | T4.5, T6.1 Mixed, T6.3 dual cohort | informational |
| AC-D4 | The want set de-duplicates. With N ≥ 3 and overlapping differences, `want_claims{result="denied"}` > 0, and each denied ID is still fetched exactly once (AC-D1). | `want_claims{result}` | T4.4, T4.6 | yes |
| AC-D5 | Baseline: report `pullsync` duplicate deliveries for the same scenarios (racing offers). On testnet, approximate them as `bee_pullsync_chunks_delivered` minus unique chunks added. | test instrumentation; `bee_pullsync_chunks_delivered`, `bee_localstore_reserve_size` | T6.1 Baseline, T6.3 | informational |

### 2.5 Decoding

| ID | Criterion and threshold | Metric(s) / signal | Measured by | Gating |
|---|---|---|---|---|
| AC-K1 | Decoder cost without overshoot: mean symbols needed per difference within **[1.3, 1.8]** for differences 1 … 10⁴, and symbol 0 empty exactly when decoding is complete (§13). | unit tests | T2.1 (and T2.7 if done) | yes |
| AC-K2 | Runtime cost including overshoot: for ranges with ≥ 20 differences (to leave out the `MinBatch` floor), `symbols_per_difference` **median ≤ 2.0, p95 ≤ 4.0** (draft). The ideal is 1.35–1.72 (§10); the first batch is sized ⌈1.6 d̂⌉ + 2 and `More` grows ×4. Drift above this points at a poor estimate or a decoder problem (§10). Final numbers come from T2.6/T4.6 data (G6). | `symbols_per_difference` | T2.6, T4.6, T6.1, T6.3 | yes |
| AC-K3 | `decode_corruption_total` = **0** on every node in every non-adversarial T6.1 scenario and every testnet cohort. In adversarial tests it increments, and the range still converges through the exact item listing without losing elements (§9). | `decode_corruption_total` | T2.1, T2.4, T2.5, T6.1, T6.3 | yes |
| AC-K4 | Single-architecture cohorts: `mapping_mismatch_total` = **0** (D22). | `mapping_mismatch_total` | T1.9, T6.3 | yes |
| AC-K5 | `more_requests_total`, `streams_exhausted_total{level}`, `pairwise_diff`: reported to tune cache sizes, `RoundInterval` and `CatchUpMinDiff` (Q2, Q3, Q9). The steady-state `pairwise_diff` p99 must sit well below `CatchUpMinDiff` (Q9). | same | T2.6, T4.6, T6.3 | informational (Q9 part gating) |

### 2.6 Resources

| ID | Criterion and threshold | Metric(s) / signal | Measured by | Gating |
|---|---|---|---|---|
| AC-R1 | Index memory at the reference reserve (LtHash16, `BucketSymbols = 32`): slab **≤ 22 MB** (§5.3: 16 KB + 8 MB + 11.5 + 0.7 + 0.36 MB ≈ 20.6 MB). With session buffers (≤ 16 sessions) and the want set at cap: **≤ 24 MB** (§5.3 "≈ 21 MB steady, ≈ 24 MB peak"). Subtract ~8 MB if T1.2 picks ECMH, ~6 MB if T2.6 picks 16 bucket symbols. | `index_memory_bytes` | T1.6, T2.2 memory tests, T6.3 | yes |
| AC-R2 | During a rebuild, transient Index memory ≤ 2 × slab (≈ 42 MB) for at most `index_rebuild_seconds`. See Decisions log: §5.3's 24 MB peak leaves out the double slab that §5.2's rebuild keeps. | `index_memory_bytes` (must count both slabs) | T1.6, T2.2, T6.3 radius change | yes |
| AC-R3 | Rebuild at 4.2M elements **≤ 30 s** wall clock on reference hardware (§5.2 predicts 10–20 s CPU). Rebuilds stay off the hot path: `Put` latency during a rebuild meets AC-R4. | `index_rebuild_seconds` | T2.6 `Rebuild` benchmark, T6.3 | yes |
| AC-R4 | `Apply` adds **≤ 5 %** to median reserve `Put` latency (draft: the proposal has no number, §14 Q1 asks for the measurement). Also report `index_apply_seconds` p99. | `bee_localstore_method_calls_duration{component="reserve",method="Put"}` on vs off; `index_apply_seconds` | T1.2 `BenchmarkReservePut`, T2.6 `Index.Apply` benchmark, T6.3 | yes |
| AC-R5 | Address-index disk ≤ **100 B per reserve chunk** (§5.1: ~80 B, ≈ 350 MB at 4.2M). | `du` of the data dir, on vs off | T1.5, T6.3 | yes |
| AC-R6 | Address-index fill: blocking **≤ 60 s** at 4.2M, otherwise background fill with `Busy` until ready (D5, gate G4). | T1.5 benchmark, start-up logs | T1.5, T6.2 | yes |
| AC-R7 | CPU: `rangesync`-only cohort `rate(bee_process_cpu_seconds_total)` **≤ 1.0 ×** the `pullsync`-only cohort in steady state and during bootstrap (draft, Q10). | `bee_process_cpu_seconds_total` | T6.1 (per scenario), T6.3 | yes |
| AC-R8 | Process memory: `bee_process_resident_memory_bytes`, `rangesync`-only minus `pullsync`-only on the same reserve, **≤ +64 MB** (draft). | `bee_process_resident_memory_bytes`, `go_memstats_heap_inuse_bytes` | T6.3 | yes |
| AC-R9 | Goroutines grow with the number of neighbors, not neighbors × bins (P6). `go_goroutines` on a `rangesync`-only node ≤ the `pullsync`-only node's. | `go_goroutines` | T4.5, T6.3 | yes |

### 2.7 Node integration

| ID | Criterion and threshold | Metric(s) / signal | Measured by | Gating |
|---|---|---|---|---|
| AC-N1 | `SyncRate` (§8.4, §13): `sync_rate` > 0 during bootstrap and new-half transfers. It returns to 0 within the 10-min window after catch-up ends, and stays 0 in steady state with live uploads. | `sync_rate` | T1.11, T3.5, T6.1 Bootstrap / Radius decrease, T6.3 | yes |
| AC-N2 | No early radius decrease at start-up: on an empty `rangesync`-only node, `bee_localstore_storage_radius` never drops before the start-up guard lifts (first status exchange with all eligible neighbors, or 120 s, D9/D18). | `bee_localstore_storage_radius`, `sync_rate` | T1.11, T6.1 Bootstrap, T6.3 | yes |
| AC-N3 | Radius decreases follow the puller's sequencing: one decrease, the new half moves, and the next decrease waits until `sync_rate == 0`. | `bee_localstore_storage_radius`, `sync_rate` | T3.5, T6.1 Radius decrease | yes |
| AC-N4 | Combined syncer: with both drivers, the rate reported in `/status` `pullsyncRate` = puller + `rangesync`, and it is 0 only when both are 0. With defaults it is exactly the puller's value. | `/status` `pullsyncRate`, `bee_puller_pullsync_rate`, `sync_rate` | T1.12, T3.5, T4.5 | yes |
| AC-N5 | Redistribution is unchanged for `rangesync`-only nodes, compared with the `pullsync`-only cohort over the same rounds: (a) `isFullySynced` true in steady state at least as often as the `pullsync` cohort; (b) played ÷ selected rounds (`commit_phases` ÷ `neighborhood_selected`) ≥ the `pullsync` cohort's; (c) `commit_phase_errors` and `reveal_phase_errors` no higher; (d) **no freezes** (`isFrozen` false, `lastFrozenRound` unchanged); (e) sample duration within ±10 % (draft). In T6.1, the reserve sample of a `rangesync`-synced node equals that of a `pullsync`-synced node holding the same set. | `bee_storageincentives_{neighborhood_selected,commit_phases,reveal_phases,commit_phase_errors,reveal_phase_errors,reserve_sample_duration}`, `bee_localstore_reserve_sample_duration_seconds`; `/redistributionstate` `isFullySynced`, `isFrozen`, `lastFrozenRound` | T6.1 (sample equality), T6.3 | yes |

### 2.8 Safety

| ID | Criterion and threshold | Metric(s) / signal | Measured by | Gating |
|---|---|---|---|---|
| AC-S1 | Defaults change nothing: no `rangesync` protocol, no `chunkAddr` writes, no Index memory, puller values identical. | tests | T1.4, T1.12, T1.13, T6.2 | yes |
| AC-S2 | Every T6.2 case passes: off→on, on→off→on with reserve changes, `db repair`, crash mid-fill, disable → namespace dropped and disk back down, defaults afterwards, downgrade with leftover keys. No stale element is ever served. | T6.2 tests | T6.2 | yes |
| AC-S3 | `go test ./pkg/pullsync/... ./pkg/puller/...` unchanged and green. A `rangesync`-only node still serves `pullsync` peers. | tests; `bee_pullsync_chunks_sent` on the `rangesync`-only node > 0 | T1.13, T4.5, T6.1 Mixed | yes |
| AC-S4 | Testnet rollback (`--rangesync-enable=false --puller-enable=true`) brings back `pullsync`-only behaviour and reclaims the address-index disk. | runbook steps, `du` | T6.3 | yes |
| AC-S5 | Adversarial suites green (no panics; violations reset the stream and disconnect, D4). | fuzz + adversarial tests | T1.13, T2.5 | yes |
| AC-S6 | *Only if Phase 5 is done* (required before mainnet, D11): `audit_mismatch{kind}` = 0 on honest cohorts; a planted collision is detected. | `audit_mismatch{kind}` | T5.x, T6.1, T6.3 | yes, if Phase 5 is in scope |

---

## 3. Existing metrics used as the baseline (read-only; `pkg/puller`, `pkg/pullsync` unchanged)

| Package | Exported name | Used in |
|---|---|---|
| `pkg/puller` | `bee_puller_pullsync_rate` (gauge func of `SyncRate()`) | AC-V3, AC-V4, AC-N4 |
| | `bee_puller_synced_chunks{type="historical"\|"live"}` | AC-B1, AC-D5 (chunks pulled) |
| | `bee_puller_worker` (active workers), `bee_puller_worker_iterations`, `bee_puller_worker_errors`, `bee_puller_max_uint_errors` | AC-R9, dashboards |
| `pkg/pullsync` | `bee_pullsync_chunks_offered` (requester side, one per offered chunk) | AC-B1 baseline (× ~102 B) |
| | `bee_pullsync_chunks_wanted`, `bee_pullsync_chunks_delivered` | AC-D5 |
| | `bee_pullsync_chunks_sent_offered`, `bee_pullsync_chunks_sent_wanted`, `bee_pullsync_chunks_sent` (responder side) | AC-S3 |
| | `bee_pullsync_missing_chunks`, `bee_pullsync_received_zero_address`, `bee_pullsync_received_invalid_chunks`, `bee_pullsync_duplicate_ruids`, `bee_pullsync_last_received{bin}`, `bee_pullsync_stream_limit_exceeded` | dashboards |
| `pkg/storer` | `bee_localstore_storage_radius`, `bee_localstore_reserve_size`, `bee_localstore_reserve_size_within_radius`, `bee_localstore_evicted_count` | AC-N2, AC-N3, AC-D5 |
| | `bee_localstore_method_calls_duration{component="reserve",method="Put"}` | AC-R4 |
| | `bee_localstore_reserve_sample_duration_seconds` | AC-N5 |
| `pkg/storageincentives` | `bee_storageincentives_neighborhood_selected`, `_commit_phases`, `_reveal_phases`, `_claim_phases`, `_winner`, `_commit_phase_errors`, `_reveal_phase_errors`, `_is_playing_errors`, `_reserve_sample_duration` | AC-N5 |
| process / Go (`pkg/api/metrics.go`) | `bee_process_cpu_seconds_total`, `bee_process_resident_memory_bytes`, `go_goroutines`, `go_memstats_heap_inuse_bytes` | AC-R7–R9 |
| API (no metric) | `/redistributionstate`: `isFullySynced`, `isFrozen`, `lastFrozenRound`, `lastPlayedRound`; `/status`: `pullsyncRate` (combined rate after T1.12) | AC-V3, AC-N4, AC-N5 |

There is no Prometheus metric for `isFullySynced` or for rounds skipped as "not fully synced". The
agent only logs the skip at Info level (`pkg/storageincentives/agent.go`). T6.3 polls
`/redistributionstate` instead; see the phase 6 Follow-ups.

---

## 4. Metric → owning task

Short names are §10 names unless marked. "Proposed" = no task emits it yet; the follow-up is
recorded in the named phase file.

| Metric | Emitted by | Used in |
|---|---|---|
| `sessions_total{result}` | T1.13 | AC-C2, AC-V5 |
| `session_rounds` | T1.13 (definition: Phase 1 follow-up) | AC-V2 |
| `parts_sent{type}`, `parts_received{type}` | T1.13; `Symbols`/`More` types: Phase 2 follow-up | dashboards |
| `bytes_metadata{dir}`, `bytes_chunks{dir}` | T1.13 (scope: Phase 1 follow-up) | AC-B1–B4, AC-V5 |
| `pairwise_diff` | T1.13 | AC-C2, AC-K5, G6 |
| `index_rebuild_seconds`, `index_apply_seconds` | T1.13 | AC-R3, AC-R4 |
| `index_memory_bytes` | T1.13; symbol caches + rebuild slab: Phase 1/2 follow-ups | AC-R1, AC-R2 |
| `sync_rate` | T1.13 (value from T1.11, T3.5) | AC-V3, AC-V4, AC-N1–N4 |
| `cross_protocol_duplicates_total` | T1.10 (counter), T1.13 (registration) | AC-D2, AC-D3 |
| `mapping_mismatch_total` (not in §10) | T1.9 | AC-K4 |
| `symbols_per_difference` | T2.6 (definition: Phase 2 follow-up) | AC-K2 |
| `more_requests_total`, `streams_exhausted_total{level}` | T2.6 | AC-K5 |
| `decode_corruption_total` | T2.6 (registers), T2.4 (increments) | AC-K3 |
| `want_claims{result}` | T4.4 (result values: Phase 4 follow-up) | AC-D1, AC-D2, AC-D4 |
| `rejected_cache_size` | T4.4 | AC-C3 |
| `audit_mismatch{kind}` | Phase 5 (T5.2), outside Phases 1–4 by design | AC-S6 |
| **`chunks_fetched_total`** (proposed) | none → Phase 1 follow-up (T1.10 / T1.13) | AC-B3, AC-D2, AC-D3 |
| **`want_claims{result="expired"\|"late"}`** (proposed values) | none → Phase 4 follow-up (T4.4) | AC-D1, AC-D2 |
| **`round_interval_seconds`** (proposed) | none → Phase 4 follow-up (T4.3) | AC-V1, T6.3 bounds |
| **stripe / verification metrics** (proposed: `stripes_total{result}`, `verification_diff`) | none → Phase 3 follow-up (T3.2 / T3.3) | AC-B2, T6.1 Churn |

---

## 5. Changes from the T6.0 draft table

| Draft said | This file | Why |
|---|---|---|
| "metadata ≤ 50 % of pullsync's (§7.8 predicts ~30 %)" | ≤ 50 % kept; prediction given per N and with/without `Want` IDs; measured at a defined reference load | §7.8 gives 28–35 % (N = 9) and 17–22 % (N = 17) for symbols only, and only for tens-to-hundreds of differences per pair. Without a load condition the per-round `MinBatch` overhead could fail the row for reasons the proposal already expects. |
| "bootstrap ID metadata ≈ 1× reserve IDs" | ≤ 1.15 × count × 70 B + ≤ 1 MB verification | §6.2 puts an `ID` at ≈ 70 B on the wire; §7.4's 270 MB uses 64 B. |
| "`symbols_per_difference` within 1.35–1.72 + overshoot" | AC-K1 (unit, [1.3, 1.8] from §13) + AC-K2 (runtime, median ≤ 2.0 / p95 ≤ 4.0, draft) | "+ overshoot" wasn't measurable. §13 uses [1.3, 1.8] for the decoder test and §10 uses 1.35–1.72 for the metric; both are kept, each where it applies. |
| "Index ≈ 21 MB steady / ≤ 24 MB peak" | kept (AC-R1); the rebuild transient is a separate row (AC-R2) | §5.2's rebuild keeps the old and new slab at once; §5.3 doesn't count that. See the Decisions log. |
| "rebuild ≤ 30 s at 4.2M" | kept | §5.2 estimates 10–20 s CPU; 30 s wall clock leaves room for I/O. |
| "`Apply` adds ≤ 5 % to `Put` latency" | kept, marked draft | No number in the proposal (§14 Q1 only asks for the benchmark). |
| "zero duplicate `rangesync` deliveries" | exact in tests (AC-D1), ≤ 1 % upper bound on testnet (AC-D2) | On a live node `cross_protocol_duplicates_total` also counts pushsync racing a fetch, so it can't tell intra-`rangesync` duplicates apart. |
| "`session_rounds`, test timings" for convergence | split into AC-V1 (time), AC-V2 (`session_rounds`), AC-C2 (quiescence) | `session_rounds` counts round trips inside a session, not neighborhood rounds. |
| — | added AC-V5, AC-K4, AC-R5–R9, AC-S3–S6 | They cover the rest of §14 Q10 (CPU, memory), D12/D22 cohort rules and §13 coexistence. |

---

## 6. Open items for the team

1. Confirm or replace every number marked *draft* (AC-D2 1 %, AC-K2, AC-R4 5 %, AC-R7, AC-R8,
   AC-N5 ±10 %).
2. Is AC-V1 gating on testnet? It needs a propagation probe (upload to one node, poll neighbors'
   reserves) that T6.3 doesn't define yet.
3. Decide whether "≤ 1.0 × pullsync" (AC-V3, AC-R7) should allow a margin, since testnet runs are
   noisy.
4. AC-S6 applies only if Phase 5 is done before acceptance (D11 requires it before mainnet anyway).
