# Phase 6 — Acceptance testing

**Goal:** show, with automated tests and testnet runs, that `rangesync` can sync a network on its
own and alongside `pullsync`, that switching it on and off is safe, and how it compares with
`pullsync`. The output is an **exit report** that a later, separate proposal for phasing out
`pullsync` will be based on. Nothing in this phase changes defaults or touches `pullsync`.

**Proposal sections:** §3 (goals), §7.4 and §7.8 (cost tables to verify), §8.3 (flag matrix),
§8.4 (`SyncRate`), §10 (metrics), §12 row 6, §13 (coexistence, toggle safety, SyncRate), §14 Q10.

**Depends on:** Phase 4 (and Phase 5 if collision detection should be covered).

---

## T6.0 Draft acceptance criteria (do this now, wave A)

**Why early:** the criteria decide which metrics Phases 1–4 must emit. Agreeing them up front
avoids retrofitting metrics later.

**Scope:** write `proposal/rangesync-rateless-plan/acceptance-criteria.md` with a first draft the
team can refine:

| Area | Criterion (draft) | Metric(s) |
|---|---|---|
| Correctness | every node converges to its neighborhood's union in all T6.1 scenarios | test assertions |
| Convergence | steady state: new chunk on all neighbors within 2 × `RoundInterval`; bootstrap ≤ pullsync's time | `session_rounds`, test timings |
| Bandwidth | metadata bytes per new chunk ≤ 50% of pullsync's (§7.8 predicts ~30%); bootstrap ID metadata ≈ 1× reserve IDs (§7.4) | `bytes_metadata`, puller metrics |
| Duplicates | zero duplicate `rangesync` deliveries; cross-protocol duplicates reported in dual mode | `want_claims`, `cross_protocol_duplicates_total` |
| Decoding | `symbols_per_difference` within 1.35–1.72 + overshoot; `decode_corruption_total` = 0 outside adversarial tests | same |
| Resources | Index ≈ 21 MB steady / ≤ 24 MB peak; rebuild ≤ 30 s at 4.2M; `Apply` adds ≤ 5% to `Put` latency | `index_*`, benchmarks |
| Node integration | radius decreases with the same sequencing as the puller; no early decrease at start-up; redistribution outcomes (`isFullySynced`, sample, commit/reveal) unchanged for `rangesync`-only nodes | `sync_rate`, storageincentives metrics |
| Safety | defaults change nothing; toggle on/off/on is clean (T6.2) | T6.2 tests |

Then cross-check that every metric in the table is produced by some task in Phases 1–4 (add a
Follow-up in the owning phase file where one is missing).

**Size:** S. **Parallel:** wave A (documentation only).

---

## T6.1 Mixed-network acceptance test suite

**Depends on:** Phase 4 (T4.6).

**Scope:** an integration suite (`-tags=integration`) of in-process nodes over `streamtest` (or the
repo's existing multi-node test harness, if one fits better — check first), covering the §8.3 flag
matrix:

| Scenario | Nodes | Pass criteria |
|---|---|---|
| Baseline | all `pullsync`-only | today's behaviour (reference numbers) |
| `rangesync` only | all `rangesync`-only | every node ends with its neighborhood's union; zero duplicate deliveries |
| Mixed | `pullsync`-only + `rangesync`-only + dual | every node converges; a `rangesync`-only node still serves `pullsync` peers |
| Bootstrap | one empty `rangesync`-only node joins | each chunk transferred once; `SyncRate` > 0 then 0 |
| Radius decrease | reserve below threshold | radius drops; only the new half moves; same sequencing as with the puller |
| Stale stamp | same chunk, two stamps | both nodes end with the newer stamp (§7.7) |
| Churn | peers join/leave mid-sync | converges; stripes reassigned |

Collect per scenario: metadata bytes, chunk bytes, duplicate deliveries, convergence time, CPU,
peak Index memory — and compare with the baseline.

**Size:** L.

---

## T6.2 Toggle-safety and upgrade/downgrade tests

**Depends on:** T1.12 (can start early; rerun after Phase 4).

**Scope:** storer + node tests for:
- off → on (no marker) → address index filled (blocking before the reserve workers start, or in
  the background with `Busy` until ready — whichever T1.5 chose); Index equals brute force;
- on → off → reserve changes → on → refill, no stale element ever served;
- on → `db repair` → on → refill;
- crash during fill (kill mid-way) → next start refills (marker written only at the end);
- disable → the `chunkAddr` namespace is dropped in the background and disk usage goes back down;
- default flags after any of the above → no `rangesync` protocol, no address-index writes, no Index
  memory;
- a node downgraded to a build without `rangesync` starts normally with leftover `chunkAddr`
  keys present (they're ignored; ~80 B per chunk). Document in the runbook: to reclaim the space,
  start once with a `rangesync`-aware build and `--rangesync-enable=false` before downgrading.

**Size:** M.

---

## T6.3 Testnet runbook, dashboards, comparison runs

**Depends on:** Phase 4 (Phase 5 if collision detection is in scope).

**Scope:**
- A runbook (`proposal/rangesync-rateless-plan/testnet-runbook.md`): how to run a cohort of nodes
  in each mode (flags), what to watch, how to roll back (`--rangesync-enable=false`,
  `--puller-enable=true`).
- A dashboard definition (Grafana JSON or a metric list) for the §10 metrics plus `sync_rate`,
  `cross_protocol_duplicates_total`, existing puller metrics for comparison, and redistribution
  outcomes (`isFullySynced`, sample duration, commit/reveal success).
- Comparison runs of at least: `pullsync`-only cohort, `rangesync`-only cohort, dual cohort, over
  enough rounds to include a bootstrap and a radius change. Record results in the Decisions log.
- **Cohort design:** a `rangesync`-only node doesn't pull from `pullsync`-only peers. Each
  `rangesync`-only cohort must therefore cover **whole neighborhoods** (every full node in the
  neighborhood runs `rangesync`, alone or in dual mode). Otherwise the node may never become
  fully synced and will skip redistribution rounds, which would read as a `rangesync` failure.
- **Dual-mode bandwidth:** a dual node can pull up to ~2000 chunks/s (1000 per protocol). On
  bandwidth-constrained testnet hosts, lower `rangesync`'s fetch limit for dual cohorts and note it
  in the results.
- Confirm or adjust `CatchUpMinDiff`, cache sizes, `RoundInterval` bounds from real `pairwise_diff`.

**Size:** M (plus wall-clock time on testnet).

---

## T6.4 Acceptance criteria + exit report

**Depends on:** T6.0, T6.1–T6.3.

**Scope (open question 10):** write `proposal/rangesync-rateless-plan/exit-report.md`:
- The final acceptance criteria (T6.0's draft as refined by the team).
- Results from T6.1–T6.3 against each criterion: pass / fail / needs work.
- Open issues and recommended fixes.
- Inputs for the separate phase-out proposal: what a default flip would require, which `pullsync`
  pieces other code still depends on (e.g. `ChunkBinItem` is read by reserve sampling
  `pkg/storer/sample.go`, debug info, `countWithinRadius`, `NeighborhoodsStat` — removing BinIDs is
  not just a sync change), and the compatibility window for `pullsync`-only peers.

**Size:** S–M.

---

## Decisions log
_(sessions append here)_

## Follow-ups
_(sessions append here)_
