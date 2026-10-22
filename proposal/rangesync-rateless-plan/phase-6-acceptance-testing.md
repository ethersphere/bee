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
- **One architecture per run (D22):** until T2.8 has checked the `FloatMapper` vectors on
  arm64, run every `rangesync` cohort on amd64 hosts only.
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
- **(T6.0) Criteria draft written** in [acceptance-criteria.md](acceptance-criteria.md): 44 rows
  (AC-C/V/B/D/K/R/N/S), each with a metric, a threshold and the test or run that measures it. The
  decisions below were needed to make the draft table measurable:
  - **Independent of G3.** No row depends on Rateless IBLT vs CertainSync beyond the proposal's
    own numbers. If G3 picks CertainSync, AC-K1/AC-K2 must be re-based (T2.0 already says the
    [1.3, 1.8] band would not hold).
  - **Metadata.** Metadata = `status` + `reconcile` + `fetch` `Want` bytes; chunk bytes =
    `Delivery`. `pullsync`/`puller` have no byte counters and can't be changed, so the
    `pullsync` baseline is `bee_pullsync_chunks_offered` × ~102 B (one `pb.Chunk` in an `Offer`),
    which matches §7.8's "100 B per offer".
  - **AC-B1 load condition.** The ≤ 50 % bandwidth row applies at a reference load (median
    `pairwise_diff` ≥ 20 per steady-state session), the regime §7.8 assumes. The low-load regime is
    reported but doesn't gate: there the per-round `MinBatch` overhead dominates, as §7.8 itself
    notes. The draft's "~30 %" matches §7.8 at N = 9 (28–35 %); at N = 17 §7.8 gives 17–22 %.
    Counting `Want` IDs adds ~5 points.
  - **ID size.** Bootstrap metadata uses 70 B per ID (§6.2) instead of §7.4's 64 B: ≤ 1.15 × count
    × 70 B (≈ 340 MB at 2²²), plus ≤ 1 MB of verification.
  - **Decoding.** The draft's "1.35–1.72 + overshoot" was split in two: AC-K1 is the decoder unit
    test, [1.3, 1.8] per §13; AC-K2 is the runtime metric with overshoot, median ≤ 2.0 / p95 ≤ 4.0
    (draft) for ranges with ≥ 20 differences, so the `MinBatch` floor stays out.
  - **Rebuild memory: a proposal inconsistency.** §5.2's rebuild builds a new slab while the old
    one stays live (steps 2–5), so Index memory transiently reaches ~2 × 20.6 MB ≈ 42 MB. §5.3's
    "≈ 24 MB peak" counts only session buffers and the want set. AC-R1 keeps 21/24 MB for steady
    state; AC-R2 bounds the rebuild transient separately. The proposal's §5.3 should get a row for
    it (see Follow-ups).
  - **Duplicates.** "Zero duplicate `rangesync` deliveries" is exact in in-process tests (Put count
    per ID). On testnet `cross_protocol_duplicates_total` also counts pushsync racing a fetch, so
    the testnet row (AC-D2) is an upper bound of ≤ 1 % (draft).
  - **Draft numbers with no basis in the proposal**, to be confirmed by the team: AC-D2 1 %,
    AC-K2, AC-R4 5 % `Put` overhead (the draft's number; §14 Q1 only asks for a benchmark),
    AC-R7 CPU ≤ 1.0 ×, AC-R8 +64 MB RSS, AC-N5 ±10 % sample duration.
  - **Metric naming.** §10 short names are assumed to be exported as `bee_rangesync_<name>`. T1.13
    decides; this is recorded as a Phase 1 follow-up.
  - **Metric owners.** Every §10 metric has an owning task in Phases 1–4 except `audit_mismatch`,
    which is Phase 5 (T5.2) by design and only gates if Phase 5 is in scope (AC-S6). Gaps are
    recorded as follow-ups: Phase 1 (metric definitions; `chunks_fetched_total`), Phase 2
    (`symbols_per_difference` definition, `parts_*{type}` for symbols/more, `index_memory_bytes`
    including symbol caches), Phase 3 (stripe and verification metrics), Phase 4
    (`round_interval_seconds`; `want_claims{result}` values incl. `expired` and `late`).
- **(T6.2) Tests and where they are.** Storer half: `pkg/storer/reserveaddr_toggle_test.go`.
  Node half: `pkg/node/rangesync_toggle_test.go`. Test-only seams in `pkg/storer/export_test.go`:
  `AddrIndexDropStarted`, `CountWithinRadius`. No production code changed.
- **(T6.2) Bullet → tests.** Existing tests (T1.5/T1.12) that already cover a bullet are kept as they
  are. New tests are marked *new*.
  - *off → on, Index equals brute force:* existing `TestReserveAddrIndexFillCanceled` (a reserve
    with no index or marker, then an enabled start: one fill, ready, entries equal the
    `ChunkBinItem`s) and `TestAddrIndexFill`. These check index keys only. *New*
    `TestRangesyncToggle` (node) builds the rangesync Index through the node's wiring after the
    first fill. Its root summary and every range down to 4 bits below the root equal an LtHash
    brute force over the reserve, read through `ReserveIterateChunks` (the bin index). The same
    holds after live puts, restamps and overwrites, which reach the Index through
    `removalObserver` → `Indexer`.
  - *on → off → changes → on, nothing stale served:* existing `TestReserveAddrIndexToggle` (index
    keys only). *New* `TestReserveAddrIndexToggleServed` (storer) removes elements while off in
    each way the reserve does (eviction, newer stamp for the same address, stamp index taken over).
    It runs twice: once with the disabled start's drop completed, and once with it interrupted
    (worst case: every entry of the enabled period written back, no marker). After the refill, the
    whole-space and every 1–3-bit `ReserveIterateAddr` equal the reserve, and
    `ReserveHasID`/`ReserveGetByID` find every element and none of the 15 stale ones. *New*
    `TestRangesyncToggle` checks the same at the protocol surface. An empty peer's session
    (`Reconcile`) wants exactly the node's reserve, the peer fetches all of it from the node, and a
    `Fetch` of the stale IDs gets 0 chunks.
  - *on → db repair → on:* existing `TestReserveRepairDropsAddrIndex` (repairer on an in-memory
    store). *New* `TestReserveAddrIndexRepair` runs what `bee db repair-reserve` runs: the storer
    with the defaults, then `ReserveRepairer`. One chunk loses its data. The test checks one refill,
    entries equal the reserve, and the removed chunk is not served. `TestRangesyncToggle` also
    covers the repair at node level.
  - *crash during fill:* existing `TestReserveAddrIndexFillCanceled` (context canceled, store
    closed cleanly) and `TestReserveAddrIndexUncleanShutdown` (copy taken after the fill). *New*
    `TestReserveAddrIndexCrashDuringFill` copies the files of the open store from the fill hook,
    after 9 of 30 entries were committed: a kill, with no `Close`. The copy has a partial index and
    no marker. Its next enabled start refills and serves exactly the reserve.
  - *disable → background drop, disk usage back down:* existing `TestReserveAddrIndexToggle`,
    `…DropInterrupted`, `…NoReserve`, `…Compaction` (leveldb `SizeOf` of the range → 0). *New*
    `TestReserveAddrIndexDiskUsage` measures the `indexstore` directory (see the numbers below).
  - *defaults after any of the above:* existing `TestReserveAddrIndexDefaultNoWrites` (fresh store),
    `TestSyncWiring`, `TestDisableAddrIndex` (zero writes). *New*
    `TestReserveAddrIndexDefaultsAfterToggle` starts with the defaults after six cases: on; on, off;
    on, reset; on, keep-mode change; on, repair; crash during the fill. The first default start
    deletes the marker and starts a drop exactly when entries are left. Then no entry is written
    through puts or evictions, and the next default start starts no drop and writes nothing. *New*
    `TestRangesyncDefaultsNoIndex` (node) covers full-node defaults and a light node with
    `--rangesync-enable`. No rangesync service is built (so no protocol and no Index), and
    `SetReserveObserver` returns `ErrAddrIndexDisabled`. The leftovers are dropped, and a full
    node's reserve changes write nothing. A rangesync service built on such a storer anyway (a
    wiring error) fails `Start` with `ErrAddrIndexDisabled`.
  - *downgrade:* *new* `TestReserveAddrIndexDowngrade` and a one-off cross-build check (both
    below).
- **(T6.2) Node level without `NewBee`.** A full node always enables the chain backend, so `NewBee`
  cannot be built in a unit test (as in T1.12). `toggleNode` builds the parts the flags change, the
  way `NewBee` does: `ResolveSyncWiring` → storer `ReserveAddrIndex` (reserve only on full nodes) →
  `reconciler.New` + `rangesync.New(…, RangesyncOptions(r))` + `SetService` + `Start` (which
  registers `removalObserver`) → `NewRangesyncCloser`, closed before the storer as in `Shutdown`.
  The storer's context is canceled at stop, as `NewBee`'s is, so goleak sees no stats goroutine.
  The peer's `Fetch` asks for 4 IDs per stream: a `streamtest` stream deadlocks when its reader
  lags more than 16 writes behind and then closes it. This is a harness limitation, see Follow-ups.
- **(T6.2) Disk usage (measured).** Same 4000 chunks; `indexstore` file sizes after a restart each
  time (files on tmpfs; these are file lengths). Never indexed: 2 107 145 B. Indexed: 2 448 657 B
  (+341 512 B, **85.4 B per chunk**, close to §5.1's ~80 B). After a start with the index disabled
  (background drop, then compaction): **2 044 352 B**, 62 793 B below never indexed (the compaction
  also drops older versions of other keys). Two more runs gave 85.3–85.6 B per chunk and the same
  pattern. The test requires ≥ 50 B per chunk indexed, and at most never-indexed + 10 % of the index
  after the disabled start. G4's 4.2M run (≈ 150 B per entry uncompacted, all of it back after drop
  + compaction) is the full-scale number.
- **(T6.2) Downgrade, simulated (automated).** Keep mode is the older build: `startAddrIndex` does
  nothing in keep mode (no read, no write), the reserve opens without the index, and `Close` takes
  the old path. So no new seam was needed. The remaining differences are the `ReserveAddrStore`
  methods (an older build has none) and recovery's one `Has` of the marker (outside the
  `chunkAddr` namespace). Eight entries whose values do not decode are added to the real leftovers
  (32 entries + marker), so any read of the namespace would fail. The simulated older build then:
  - starts (migrations, recovery) and runs `countWithinRadius`;
  - iterates the reserve (`ReserveIterateChunks`, one `SubscribeBin` per bin),
    `ReserveLastBinIDs`, `NeighborhoodsStat`, `DebugInfo` and a `ReserveSample` (all must count
    exactly the reserve);
  - puts, evicts a whole batch and part of another, reads everything again, and restarts.

  There is no error, and the leftovers and the marker stay byte-identical. The upgraded enabled
  start refills (the digest no longer matches) and serves exactly the reserve, none of the 20
  elements the older build removed and none of the garbage. A disabled start on a copy drops
  everything, which is the reclaim path.
- **(T6.2) Downgrade, cross-build (one-off, not committed).**
  - This build made a data dir with an enabled index: 32 elements, 40 `chunkAddr` entries (8 of
    them garbage) and a marker.
  - A detached worktree of `docs/rangesync-rateless-proposal` (74c084b1, no rangesync code; its
    `pkg`/`cmd` have no `chunkAddr` or `addrIndexValid` namespace) ran a throwaway internal test on
    that dir. It ran `New`, `StartReserveWorker` (its `countWithinRadius`) and the reads above, put
    8 chunks, evicted 20, read again and restarted. There was no error, and the 40 entries and the
    marker were unchanged.
  - Back on this build, the enabled start refilled (fills 1). It served exactly the 20 elements and
    none of the 20 stale ones. A disabled start on a copy dropped all entries and the marker.
  - The throwaway files and the worktree were removed.
- **(T6.2) Negative controls (manual, not committed).**
  - (A) No drop in `CheckAddrIndex`: `…ToggleServed/drop interrupted` fails at the surface
    (35 elements served, the reserve holds 20), and so does `…Downgrade/upgrade`. The `drop
    completed` variant passes because the disabled start's drop had already emptied the
    namespace. That is why the interrupted variant was added.
  - (B) Trust any current-version marker, without the digest: `…Downgrade/upgrade` fails (fills 0).
  - (C) `removalObserver` passes no removes to the Indexer: `TestRangesyncToggle` fails (Index root
    count 26 against a brute force of 24).
  - (D) `syncWiring` opens the address index with the defaults: `TestRangesyncToggle` and
    `TestRangesyncDefaultsNoIndex/full node defaults` fail (`SetReserveObserver` succeeds).
- **(T6.2) Finding, not fixed: `ReserveRepairer` deletes a live chunk's `ChunkBinItem`.** This
  predates rangesync: the same code is in `docs/rangesync-rateless-proposal`, and the repro fails
  20/20 there.
  - Step 2 deletes every `ChunkBinItem`. Step 3 renumbers each bin from 1 in parallel, and removes
    a chunk whose data is missing with `RemoveChunkWithItem`, which deletes
    `ChunkBinItem{bin, old bin ID}`. When another chunk of the bin was already given that ID, its
    new `ChunkBinItem` is the one deleted.
  - The repair then fails its own final check (`check failed: chunkBinItem, bin 0, binID 1: not
    found`). Without that check, a reserve chunk would be left that no bin iteration finds
    (pullsync, sampling).
  - It happened in about 2/3 of runs of the first version of `TestReserveAddrIndexRepair`.
  - Deterministic repro: `TestReserveRepairMissingChunk`, skipped with a reason that points here
    (fails 100/100 unskipped). It needs 4·NumCPU+4 chunks of one batch in one bin; the
    highest-addressed one is put first and loses its data.
  - Not fixed: `pkg/storer/migration` is outside the plan and the fix is not additive.
    `TestReserveAddrIndexRepair` puts its damaged chunk alone in its bin, where the bug cannot
    happen.
- **(T6.2) Runs.** No `-race` (no C compiler).
  - `go test ./pkg/pullsync/... ./pkg/puller/... ./pkg/storer/...` and `./pkg/node/...` pass.
  - `./pkg/storer/ ./pkg/storer/migration/` pass with `-count=3 -cpu=1,2,4`, and `./pkg/node/...`
    with `-count=5 -cpu=1,2,4`.
  - The new storer tests pass with `-count=5 -cpu=1,2,4`.
  - `go build ./...`, `go vet` and `golangci-lint` (go1.26.1) on `pkg/storer/...` and
    `pkg/node/...` are clean (0 issues). Only the three files of this task were formatted.
  - The defaults change nothing: there is no production change, and
    `TestReserveAddrIndexDefaultNoWrites`, `TestSyncWiring` and the new defaults tests pass.
- **(T6.2 fix round 1) The node-level stale check can now fail; earlier text corrected.** In the
  first version, the "Bullet → tests" entry said `TestRangesyncToggle` "checks the same at the
  protocol surface". That claim was wrong on the toggle path, for two reasons:
  - the off period waited for the drop, so the refill always started from an empty namespace;
  - the stale `Fetch` (error discarded) and `ReserveGetByID` read the chunk through the
    reserve's own items, keyed by stamp hash, which no stale entry can bring back.

  Changes:
  - Before the refill, the test now writes the enabled period's `chunkAddr` entries back into the
    closed store's leveldb (`putRawEntries`). This is what a disabled start whose drop was
    interrupted leaves.
  - The stale `Fetch` and the node's stale `ReserveGetByID` check are removed.
  - The storer's stale `ReserveGetByID` check stays, documented as a check of the served surface
    rather than of the index.

  Negative control: with no drop in `CheckAddrIndex`, `TestRangesyncToggle` fails at the Index
  root summary (31 against a brute force of 28). With that comparison disabled, it fails at
  `ReserveHasID` of a stale element. With that disabled too, it fails at the session: the peer
  wants 31 IDs, 3 of them stale, and the reserve holds 28. It passes with the drop.
- **(T6.2 fix round 1) Control (A) record corrected.** With the committed test,
  `…ToggleServed/drop interrupted` fails first at `checkIndexEqualsReserve` (index keys, 35 entries
  against a reserve of 20). The "35 elements served" failure at the surface check was measured
  with that line removed for the control run. Both results hold.
- **(T6.2 fix round 1) Keep mode against an older build, and an older build's repair.**
  - With a marker present, start-up recovery (`opts.ReserveAddrIndex || marker`) deletes the
    `chunkAddr` entries of the corrupted chunks it removes, also in keep mode
    (`TestRecoveryDeletesChunkAddrItem`). An older build never writes there. This is safe: the
    removal lowers the count, the marker no longer matches, and the next enabled start refills.
  - An older build's `db repair-reserve` keeps the marker and the entries, because it does not
    know them. Its step 0 deletes the `EpochItem`, so the next start creates a new epoch and the
    next enabled start refills.
  - New case `TestReserveAddrIndexRepair/older build`: this build's repair, then the marker and
    the entries written back, which is what an older repair leaves. Result: one refill, and the
    chunk the repair removed is not served. Controls A (21 index entries against 20) and B
    (fills 0) both make it fail.
- **(T6.2 fix round 1) `ReserveRepairer` finding: repro moved, text corrected.**
  `TestReserveRepairMissingChunk` now lives in `pkg/storer/migration/reserveRepair_test.go`, still
  skipped with the reason. It uses an in-memory store and one batch in one bin. Four missing chunks
  hold bin IDs 1–4 and are handled last. It asserts that the repair returns no error and that
  every `BatchRadiusItem` keeps its `ChunkBinItem`. Unskipped, it fails 1500/1500
  (`-count=500 -cpu=1,2,4`).

  Correction: steps 3 and 4 commit before `checkBinIDs`. So the chunk without its `ChunkBinItem`
  is left whether or not the check fails; the check only makes the command fail. The earlier
  "without that check …" sentence was wrong.
- **(T6.2 fix round 1) Runs.** No `-race` (no C compiler).
  - The changed tests pass with `-count=5 -cpu=1,2,4`: `TestRangesyncToggle`,
    `TestRangesyncDefaultsNoIndex`, the `TestReserveAddrIndex*` tests of
    `reserveaddr_toggle_test.go`, and `pkg/storer/migration`.
  - `go test ./pkg/pullsync/... ./pkg/puller/... ./pkg/storer/... ./pkg/node/...` passes.
  - `go vet`, `GOTOOLCHAIN=go1.26.1 make lint` and `go build ./...` are clean.
  - There is still no production change.
- **(Orchestrator, T6.2 landing, 2026-10-22) T6.2 landed after one fix round.** Tests only; no
  production change. Review: no High; Medium 1 (the node-level stale check could not fail) and
  Medium 2 (the T6.3 runbook paragraph), plus four Lows, fixed in fix round 1, which verified clean
  (no re-review: no High). Where an entry above was corrected, the "(T6.2 fix round 1)" entry
  supersedes it. The pre-existing `ReserveRepairer` bug (also on the pre-rangesync code) is carried
  by the skipped `TestReserveRepairMissingChunk` in pkg/storer/migration and is outside this plan's
  scope (Follow-ups).
- **(T6.1 part 1) Harness: where, and what was checked first.** Three test files in `pkg/node`
  (`package node_test`, `//go:build integration`): `rangesync_acceptance_net_test.go` (transport and
  topology), `rangesync_acceptance_harness_test.go` (nodes, uploads, instrumentation, metrics) and
  `rangesync_acceptance_test.go` (scenarios, results, the comparison table). `pkg/node` because the
  node's wiring helpers (`ResolveSyncWiring`, `RangesyncOptions`, `NewCombinedSyncer`,
  `NewRangesyncCloser`) are exported only to its tests, and `NewBee` cannot be built in a test
  (T6.2 "Node level without `NewBee`"). Existing infrastructure, and why each part was not reused:
  - `pkg/p2p/streamtest`: the lagging-reader deadlock (a writer more than 16 writes ahead blocks
    holding the record lock, T6.2 follow-up). One pullsync page (≤ 250 Deliveries) or one
    rangesync fetch (≤ `MaxWantIDs` = 250 chunks) is enough to hit it. Its handlers also run on
    `context.Background`, so a disconnect cannot end them (part 2's Churn), and it keeps every byte
    of every stream for the whole test.
  - Topology: real kademlia needs a `p2p.Service` that dials, an address book and discovery.
    `pkg/topology/kademlia/mock` uses the list index as the bin and its `EachConnectedPeer` skips
    the first peer; `pkg/topology/mock`'s `SubscribeTopologyChange` returns a nil channel.
  - `pkg/storer/mock`: its `SubscribeBin` serves a fixed list, which is the T1.13 gap (P never
    pulls from D).
  - `pkg/puller` and `pkg/pullsync` tests use a mock pullsync or mock reserves over `streamtest`;
    neither drives the puller loop over a live reserve.
  - The rangesync neighborhood simulation (`neighborhood_sim_test.go`) runs on a fake clock and
    wraps only rangesync. The puller and pullsync run on the real clock (5-min ticker, 250 ms page
    timeout, rate limiters), so they cannot run on its clock, and `package rangesync_test` cannot
    reach `pkg/node`'s wiring.

  The harness therefore has its own parts, all test-only:
  - **`accNet`**: libp2p's stream semantics (copied from the simulation's `simEnd`/`simPipe`) on
    the real clock: an unbounded buffer per direction (no deadlock), `Close`, `FullClose` with the
    remote-EOF wait (30 s, libp2p's `closeDeadline`), `Reset`, handlers on a per-connection
    context that a disconnect cancels, the handler wrapper's `DisconnectError`/`BlockPeerError`
    handling, `p2p.IncompatibleStreamError` for a protocol or version the peer did not add (so a
    reconciler sees a pullsync-only peer as Unsupported), `ErrPeerNotFound` without a connection,
    and byte counters per protocol/stream. `TestAcceptanceNet` checks these semantics.
  - **`accTopology`**: the connected peers with their real proximity order, iterated by bin
    (closest first, and reversed), topology-change signals on connect, disconnect and radius change
    (kademlia's `SetStorageRadius` signals too), and `SetStorageRadius` as the reserve's
    `RadiusSetter`.
  - **A real storer per node**: `storer.New(ctx, "", …)` (in-memory leveldb and sharky, the same
    reserve, address-index and observer code as on disk), as many storer tests do. On-disk storers
    would only add I/O and per-node leveldb buffers.
- **(T6.1 part 1) Node wiring.** Each node is built as `NewBee` builds the parts that the two flags
  change: `ResolveSyncWiring(true, false, rs, puller)` → `storer.New` (`ReserveAddrIndex` from it,
  `RadiusSetter` = the topology, the batch store, `ReserveCapacity` 65536 so nothing is evicted)
  → `pullsync.New(host, store, …, pullsync.DefaultMaxPage)` → with rangesync `reconciler.New(overlay,
  topology, nil, …, reconciler.Options{})`, `rangesync.New(host, store, …, RangesyncOptions(r))`,
  `SetService`, `Start` → `AddProtocol` (pullsync always, rangesync if registered) → with the puller
  `puller.New(overlay, state store, topology, db, pullsync, host, …, puller.Options{})` →
  `NewCombinedSyncer` of the built drivers → `StartReserveWorker(ctx, syncer, radius, nil)`. Shutdown
  follows `Shutdown`: puller, rangesync closer and pullsync closed concurrently, the node contexts
  canceled, then the transport, then the storers and state stores. What differs from `NewBee`:
  - no pushsync, retrieval, PSS or GSOC (uploads are put directly; `unwrap`/`gsocHandler` are no-ops
    for both protocols), and no status protocol (`SetSync`);
  - the radius function returns the configured radius (no salud);
  - the warm-up stabilization is a switch the harness closes once all nodes are connected. In a
    node, kademlia connects peers during the warm-up, so the syncers start with peers connected;
    with `stabilmock.NewSubscriber(true)` they would start before any peer;
  - the state store is `leveldb.NewInMemoryStateStore`.
- **(T6.1 part 1) Uploads, load and network.** One neighborhood of radius 2 (overlays share their
  first 2 bits). Every node starts with the same 4500 base chunks, a neighborhood in sync, so no node
  starts far behind. 4500 is above `fingerprint.ScanLimit` (4096), so a session reads the root's
  symbol stream from the root's cache, as on a full reserve; a range of at most `ScanLimit`
  elements is scanned instead. The level-1 and bucket ranges stay below it here and are scanned
  (their caches need more than 65536 elements per root). A first measurement set used 256 base
  chunks, as the simulation's `Base`, and so scanned every range, the root too; see Results. Then
  300 uploads at 15 per second (the simulation's `steadyOptions` rate). Each is put, as pushsync's
  storer path does (`validStamp` then `ReservePutter().Put`), into the 3 nodes closest to its
  address. Chunks are 4096 B with real stamps: a secp256k1 batch key and the real
  `postage.ValidStamp`, on a batch store mock that holds the batch. The base chunks are validated
  once. The overlays and chunk contents come from a seed (the network size), so the three scenarios
  of one size see the same overlays and uploads. Mixed gives node i the flags P, R, D in turn
  (N = 5: 2P+2R+1D; 9: 3+3+3; 17: 6+6+5). The reference load (median `pairwise_diff` ≥ 20) holds at
  N = 9 and in 2 of 3 runs at N = 5: the median is in the bucket 32–63. At N = 17, and in the third
  N = 5 run, it is in the bucket 16–31, which cannot tell 16–19 from 20–31. A lower load only adds
  per-session overhead to rangesync, so N = 17's AC-B1 pass stands either way. `pairwise_diff_half`'s
  p99 is in the bucket ≤ 64 (≤ 256 at the dual nodes of Mixed at N = 17), below `CatchUpMinDiff`
  (256).
- **(T6.1 part 1) What is measured, and how.** The window runs from the first upload until the
  network is quiet; counters are window deltas summed over the nodes.
  - Metadata (AC definition): Σ `bee_rangesync_bytes_metadata{dir="in"}` + Σ
    `bee_pullsync_chunks_offered` × 102 B, per upload. Wire metadata: the transport's pullsync bytes
    minus the pullsync Delivery bytes (each delivery's `pb.Delivery` size, counted in the harness),
    plus `bytes_metadata{dir="out"}`. Rangesync's split by stream (status, reconcile, fetch minus
    `bytes_chunks`) comes from the transport, whose rangesync total agrees with `bytes_metadata` +
    `bytes_chunks` (to the 0.1 MB the suite logs, in every run).
  - Chunk bytes: pullsync Delivery bytes + `bytes_chunks{dir="in"}`, per upload.
  - Duplicates, exact: the `validStamp` each protocol gets counts the deliveries per node and chunk
    ID before validating. A delivery of an ID the node already holds (put by the harness, or
    delivered before) is a duplicate. Duplicates are split by cause (next entry).
  - Convergence: the putter the protocols get records when each ID first reached each reserve.
    The latency of an upload runs until it is on every node whose root holds it. "Convergence
    after the last upload" depends on the phase: 300 uploads at 15/s take 20 s, 4 rounds of 5 s,
    so the last round often lands right after the last upload. The latency distribution is the
    meaningful number.
  - Quiet (AC-C2): every rangesync node's connected rangesync peers hold its current Status, nodes
    with the same root have equal digests, and the reconcilers' summed `sessions_total` does not
    change for 10 s. It must happen within 120 s of the last upload. The reported time is the last
    session start after the last upload (from a 10 Hz sample).
  - CPU (AC-R7): `bee_process_cpu_seconds_total` from the Prometheus process collector (the metric
    AC-R7 names) over the window, for the whole test process, per 1000 uploads. A `runtime.GC()`
    before the window keeps the setup's garbage out of it. The harness work in the window is the
    same in every scenario (stamp validation and puts of the uploads, the transport's copies), except
    the sampler. The sampler reads three collectors per rangesync node at 10 Hz directly, not
    through a registry `Gather`, to keep its cost negligible. Per-node goroutine CPU via pprof
    labels was not used: handler goroutines run on the transport, and labels would need every
    stream handler started under its node's labels.
  - Index memory: `index_memory_bytes` per rangesync node, sampled at 10 Hz. The peak per node and
    the sum of the peaks are taken over the window; the peak per node is also taken over the whole
    run, which includes the start-up rebuild from radius 0 to 2. The live Go heap (`/gc/heap/live`)
    is reported for information.
- **(T6.1 part 1) Duplicates: a delivery racing a push is not a rangesync duplicate.** In 1 of
  the 9 rangesync-only runs of the first measurement batch, rangesync delivered one chunk to a
  node it was being pushed to: 601 claims granted for 600 needed fetches, no expired claim,
  `cross_protocol_duplicates_total` 1. `claim` re-checks the reserve after the claim, and a claim
  ends only after its Put, so two rangesync fetches of one ID cannot both pass. What remains is a
  push that lands between the claim's check and the delivery. The harness puts an upload into its 3
  holders one after another while sessions run, as pushsync does on a live network. T6.0's note
  ("exact in in-process tests") assumed no such race; it is pushsync racing a fetch, which AC-D2
  covers. The suite now reports four kinds of duplicate deliveries: deliveries of base chunks
  (false differences, must be 0), the first delivery per protocol of a pushed chunk (racing its
  push), a protocol's repeated deliveries, and cross-protocol ones. The rangesync-only test asserts
  AC-D1 exactly: every ID delivered at most once per node by rangesync, and no base chunk. It
  bounds the push races by AC-D2's draft 1 % of the chunks fetched.
  Over all 29 rangesync-only measurement runs, 3 had one delivery racing a push (1 of 600 and 1 of
  4200 fetches); the two after the change passed.
- **(T6.1 part 1) Results.** Three runs per size at `GOMAXPROCS` 4 (`RANGESYNC_ACCEPTANCE_SIZES=5,9,17
  -count=3`, 4500 base chunks), as min–max. B = Baseline (all pullsync-only), R = rangesync-only,
  M = Mixed. "Per upload" is per uploaded chunk, summed over the network.

  | | N = 5 | N = 9 | N = 17 |
  |---|---|---|---|
  | Metadata B per upload (AC definition) B / R / M | 2040 / 1182–1447 / 1587–1643 | 7344 / 4174–4330 / 6343–6413 | 27744 / 10843–11102 / 23583–24074 |
  | R ÷ B (AC-B1) · M ÷ B | 0.58–0.71 · 0.78–0.81 | 0.57–0.59 · 0.86–0.87 | 0.39–0.40 · 0.85–0.87 |
  | Wire metadata R ÷ B (B: Get, Offer, Want, cursors) | 0.54–0.66 (B 2200 B) | 0.53–0.55 (B 7892 B) | 0.36–0.37 (B 29759 B) |
  | R metadata by stream: status / reconcile / fetch Want, B per upload | 16–18 / 1024–1290 / 140 | 59 / 3694–3850 / 421 | 222 / 9637–9897 / 983 |
  | Chunk B per upload B / R / M | 18.2–19.0 K / 8.5 K / 13.8–14.1 K | 52.9–55.2 K / 25.5 K / 45.1–46.0 K | 145–147 K / 59.6 K / 115–116 K |
  | Upload latency median / p95 / max, B | 0.25 / 0.52 / 0.92 s | 0.26 / 0.46 / 0.93 s | 0.26 / 0.39 / 0.67 s |
  | same, R | 2.5 / 4.8 / 5.0 s | 2.6 / 4.8 / 5.1 s | 2.7 / 4.9 / 5.2 s |
  | same, M | 2.2 / 4.7 / 5.0 s | 2.5 / 4.8 / 5.0 s | 2.6 / 4.8 / 5.2 s |
  | Last session after the last upload R / M (AC-C2) | 0.1–10.0 s / 0.1–5.1 s | 0.2–5.1 s / 4.9–5.2 s | 5.1 s / 5.1–5.2 s |
  | Duplicate deliveries B: pullsync repeats (needed deliveries) | 685–737 (600) | 1925–2088 (1800) | 6033–6131 (4200) |
  | Duplicate deliveries R: rangesync repeats / racing a push | 0 / 0 | 0 / 0 | 0 / 0–1 |
  | Duplicate deliveries M: pullsync repeats / cross-protocol / `cross_protocol_duplicates_total` | 371–395 / 0 / 0 | 1374–1438 / 0 / 0 | 3871–3973 / 0 / 0 |
  | CPU s per 1000 uploads B / R / M | 5.1–7.5 / 3.5–4.2 / 5.1–5.6 | 11.6–12.9 / 6.5–8.0 / 10.3–11.2 | 28.5–33.3 / 11.6–14.0 / 22.4–25.1 |
  | CPU R ÷ B (AC-R7) · M ÷ B | 0.47–0.73 · 0.74–0.99 | 0.54–0.62 · 0.85–0.89 | 0.38–0.48 · 0.73–0.86 |
  | Peak Index memory per node; sum R / M | 21.0 MB; 105 / 63 MB | 21.0 MB; 189 / 126 MB | 21.0 MB; 357 / 231 MB |
  | Peak live Go heap of the process B / R / M | 215–241 / 301–317 / 288–308 MB | 423–445 / 618–624 / 536–550 MB | 802–808 / 1160–1203 / 1062–1114 MB |
  | `pairwise_diff` median bucket, R (sessions) | 16–63 (72–94) | 32–63 (288–316) | 16–31 (1090–1162) |
  | IDs sessions found missing per upload, R (p(N − p)) | 4.6–5.9 (6) | 17.5–17.8 (18) | 42.8–45.0 (42) |
  | Reconcile B per ID found missing, R | 218–222 | 207–217 | 217–231 |
  | `symbols_per_difference` median / p95 bucket, R | ≤ 2–2.25 / ≤ 3.5–3.75, > 5 in 1 run | ≤ 2–2.25 / ≤ 3.25–3.75 | ≤ 2.25–2.5 / ≤ 3.5–3.75 |
  | `symbols_per_difference` mean, R | 2.18–2.24; 9.69 in the > 5 run | 2.18–2.20 | 2.36–2.48 |

  - At `GOMAXPROCS` 1 and 2 (`-cpu=1,2`, default sizes) the ratios are: metadata R ÷ B 0.51–0.57
    (N = 5) and 0.61–0.65 (N = 9); CPU R ÷ B 0.57–0.76.
  - CPU numbers vary by up to 1.5× between runs of the same scenario; the other rows are stable.
    Process CPU-seconds depend on the host's load and clock (a shared desktop host here), and
    CPU per upload grows with `GOMAXPROCS`. The ratio within a size is what AC-R7 needs, and it is
    below 1 in every run.
  - Rangesync delivered every chunk exactly once to each node that lacked it: chunk bytes are
    (N − p) × 4258 B per upload, plus one delivery in a run with a push race. Pullsync delivered 2.1–2.5 times the needed chunks, because racing
    offers from the p holders arrive within one 250 ms page of each other.
  - Rangesync's sessions found p(N − p) missing IDs per upload, as §7.8 assumes (only (has, lacks)
    pairs carry an upload). But each ID costs 207–231 reconcile bytes, against §7.8's 110–140 B
    (1.35–1.72 symbols × 82 B). Three causes are visible:
    - `symbols_per_difference` sits at a mean of 2.2–2.5, with medians of ≤ 2–2.5, above §7.8's
      range;
    - the decoding side lists the IDs only it holds as an Items part, which §7.8's table does not
      count (`session.go`, `Handle`: "the IDs only this node holds are sent as Items without
      Reply"; about one Items part per session);
    - a Fingerprint per session.

    Some runs also show a tail of streams with many more symbols than differences: the p95 bucket
    is above 5 in 1 of the 9 rangesync-only runs and 1 of the 9 Mixed runs, with means of 9.7 and
    10.4. The first batch is sized from max(|cL − cR|, hint(R), 1), so a large hint left by an
    earlier session with the same peer would explain the tail. That is not confirmed: rangesync's
    `DisableHints` is in its `export_test.go`, out of `pkg/node`'s reach (Follow-ups).
  - Mixed: the dual nodes pull each upload through pullsync within ~0.25 s, so rangesync carries
    little (6.2–7.5 IDs found missing per upload at N = 9), and Mixed's metadata stays at 0.78–0.87
    of Baseline.
  - The first measurement set, with 256 base chunks (every range scanned, the root too), had four
    runs per size at `GOMAXPROCS` 4. Metadata R ÷ B was 0.58–0.62 (N = 5), 0.52–0.61 (N = 9) and
    0.39–0.46 (N = 17); reconcile B per ID 171–187, 183–223 and 232–263; CPU R ÷ B 0.59–0.73,
    0.51–0.61 and 0.25–0.44. At N = 17 Mixed it showed 1–8 cross-protocol duplicates per run.
    `cross_protocol_duplicates_total` counted 9 of those 15. Pullsync puts a page's chunks only
    after it has read every Delivery of the page, so a rangesync delivery of the same chunk in
    between passes the reserve check and is stored first. The metric only counts a fetched chunk
    already in the reserve. The second set had none at `GOMAXPROCS` 4, and 3 over the four runs at
    1 and 2, of which the metric counted 2.
  - Run times with the final code: see Runs.
- **(T6.1 part 1) Acceptance-criteria rows measured by these three scenarios.** The verdicts are
  from the second set, with the first set and `GOMAXPROCS` 1–2 where they differ.
  - **AC-C1: pass.** Asserted in every scenario and run:
    - every node's reserve inside its root equals the union of its connected peers' reserves inside
      that root;
    - every rangesync node's Index equals a brute force over its reserve: the root summary and
      every range down to 4 bits below it (counts and LtHash digests, as in T6.2's check);
    - the root symbol cache is checked through the protocol. A probe peer holds the node's reserve
      minus 40 random IDs, on a connection the node's topology does not see. Both hold more than
      `ScanLimit` elements, so the session decodes the node's root stream from its cache. It must
      want exactly the 40 IDs, with no corrupt decode.

    The level-1 and bucket caches are not exercised: their ranges stay below `ScanLimit` here.
  - **AC-C2: pass** (asserted ≤ 120 s). The last session starts 0.1–10.0 s after the last upload.
    Then the digests are equal and no session starts during the 10 s tail. "The last
    `pairwise_diff` observations are 0" cannot be observed in-process: with equal digests no
    session runs.
  - **AC-V1: pass at N = 9 and 17** (also at N = 5 and in Mixed). Every upload reached every node
    within 5.3 s. That is below 2 × `MinRoundInterval` (10 s), so it is below 2 × `RoundInterval`
    whatever the adaptive value was (`round_interval_seconds` ranged 5–30 s). Reported, not asserted.
  - **AC-B1: fails at N = 5 (0.58–0.71; 0.51–0.57 at `GOMAXPROCS` 1–2) and N = 9 (0.57–0.59;
    0.61–0.65); passes at N = 17 (0.39–0.40).** The first set agrees (0.58–0.62, 0.52–0.61,
    0.39–0.46). §7.8 predicts 33–40 % at N = 9 and 21–26 % at N = 17 with the `Want` IDs; the gap
    and its causes are under Results. This is a performance result against a draft threshold, not
    a correctness bug, so no production code was changed. It is for T6.4 and the team (Follow-ups).
    Reported, not asserted.
  - **AC-D1: pass** (asserted). In every rangesync-only run, rangesync delivered each ID at most
    once per node and no base chunk; so did the rangesync-only and dual nodes of Mixed. Deliveries
    racing a push: 3 in 29 rangesync-only runs, one each, within AC-D2's draft 1 %.
  - **AC-D3: reported.** Mixed cross-protocol duplicate deliveries were 0 in the second set at
    `GOMAXPROCS` 4, 0–2 per run at 1 and 2 (N = 5, 9), and 1–8 per run at N = 17 in the first set,
    at most 0.51 % of the rangesync fetches. `cross_protocol_duplicates_total` can undercount them
    (see the first set).
  - **AC-D5: reported.** Baseline pullsync duplicates were 685–737 (N = 5), 1925–2088 (N = 9) and
    6033–6131 (N = 17), for 600, 1800 and 4200 needed deliveries: 1.1–1.5 duplicates per needed
    delivery. `bee_pullsync_chunks_delivered` was 1285–1337, 3725–3888 and 10233–10331. On testnet
    that minus the unique chunks added gives the same number.
  - **AC-K2: not measurable as defined; indicative only.** The criterion is over ranges with ≥ 20
    differences. `symbols_per_difference` has no difference label, and the per-stream samples
    (`SessionSample`) are reachable only from rangesync's own tests. Over all decoded streams the
    median is ≤ 2.0 in 4 of the 6 rangesync-only runs at N = 5 and 9, and in the (2.0, 2.5] buckets
    at N = 17. The p95 is ≤ 4.0 in 8 of 9 runs and in the bucket above 5 in the other.
  - **AC-K3: pass** (asserted in rangesync-only and Mixed): `decode_corruption_total` is 0 in every
    run.
  - **AC-R7: pass.** The rangesync-only network used 0.38–0.73 of Baseline's process CPU per upload
    at `GOMAXPROCS` 4, and 0.57–0.76 at 1 and 2. This is steady state only; bootstrap belongs to
    part 2.
  - **AC-S3: pass** (asserted). In Mixed, every rangesync-only node sent pullsync offers and chunks
    to the pullsync-only and dual nodes (`chunks_sent_offered` and `chunks_sent` > 0) and pulled
    nothing through pullsync. `go test ./pkg/pullsync/... ./pkg/puller/...` is unchanged and green.
  - Index memory, for AC-R1 and AC-R2 (T6.3 rows): `index_memory_bytes` is 21.0 MB per node. The
    slab is allocated in full at the default `LeafBits` 12, whatever the reserve size, so this is
    the slab value AC-R1 bounds (≤ 22 MB). The start-up rebuild from radius 0 to 2 peaks at
    42.0 MB, 2 × slab, which is AC-R2's bound.
- **(T6.1 part 1) The split.** It holds as decided. The harness takes per-node radii
  (`accOptions.NodeRadius`), nodes that start empty (`Empty`), the reserve capacity and wake-up
  interval (radius decreases), and has `accNet.connect`/`disconnect` for join and leave; what part 2
  still has to add is in the Follow-ups.
- **(T6.1 part 1) Production code.** None changed. The suite is test-only (`-tags=integration`); no
  test seam was added.
- **(T6.1 part 1) Runs.** No `-race` (no C compiler).
  - First set (256 base chunks): `RANGESYNC_ACCEPTANCE_SIZES=5,9,17 -count=3`. Every scenario passed
    except one rangesync-only run, on the push race before the duplicate classification (see
    above). After that, it passed with `-count=1 -cpu=1,2,4` at the default sizes and once more at
    N = 17.
  - Second set (4500 base chunks): `-count=3` at N = 5, 9 and 17, and `-count=1 -cpu=1,2` at the
    default sizes; all passed. The final code (base chunks validated once) passed the default sizes
    and N = 17 again. `TestAcceptanceNet` passed with `-count=5 -cpu=1,2,4`.
  - Run times with the final code: the default sizes (5, 9) take 5.1 min with a peak RSS of 1.3 GB.
    N = 17 takes 3.1 min with a peak RSS of 3.7 GB (`RANGESYNC_ACCEPTANCE_SIZES=5,9,17`,
    `-timeout 20m`). Per scenario: Baseline 36–46 s (setup 6–14 s); rangesync-only 53–75 s (setup
    11–24 s: the index build and its rebuild at the new radius, the Status exchange, the start-up
    guard); Mixed 52–64 s. `RANGESYNC_ACCEPTANCE_SIZES` defaults to 5,9 so that
    `make test-integration` (`go test` with its default 10-min timeout) stays within it.
  - `go test ./pkg/pullsync/... ./pkg/puller/... ./pkg/storer/... ./pkg/node/...` and
    `go test ./pkg/rangesync/...` pass. The T6.2 flake watch (`TestSyncRateHonestChurn/radius_half`)
    did not show up.
  - `go build ./...` is clean. So are `go vet` and `golangci-lint` (go1.26.1) on `pkg/node/...`,
    with and without `--build-tags integration` (0 issues). Only the three new files were
    formatted (gofumpt, gci).

## Follow-ups
_(sessions append here)_
- **(T6.0) Proposal §5.3:** add the rebuild transient (old + new slab, ≈ 2 × 20.6 MB for up to
  `index_rebuild_seconds`) to the memory budget, or change §5.2 to rebuild in place. AC-R2 assumes
  the former.
- **(T6.0) T6.3:** no Prometheus metric exposes `isFullySynced` or rounds skipped as "not fully
  synced". `pkg/storageincentives/agent.go` only logs "skipping round because node is not fully
  synced" at Info. For AC-N5, T6.3 polls `/redistributionstate` (`isFullySynced`, `isFrozen`,
  `lastFrozenRound`, `lastPlayedRound`) and counts the log lines. A real metric would belong in
  `pkg/storageincentives` or `pkg/node`, outside Phases 1–4; raise it only if polling proves too
  coarse.
- **(T6.0) T6.3:** define a propagation probe for AC-V1 on testnet (upload to one node of a
  `rangesync` cohort, then check its neighbors' reserves), or keep AC-V1 gating only in
  T6.1/T4.6.
- **(T6.2) T6.3, runbook (`testnet-runbook.md`), downgrade paragraph.** A node downgraded to a
  build without `rangesync` starts and runs normally with the leftover `chunkAddr` entries and the
  `addrIndexValid` marker on disk. That build never reads or changes them (verified, see the
  Decisions log). They take ~80 B per chunk: 85 B measured at 4000 chunks, ≈ 350 MB at 4.2M.
  - To reclaim the space, start once with a `rangesync`-aware build and `--rangesync-enable=false`
    before downgrading. That start deletes the marker and drops the entries in the background,
    then compacts the range. Let it run until the drop finishes: at 4.2M, G4 measured ≈ 12–14 s of
    drop and ≈ 19–21 s of compaction. A stop before that resumes the drop at the next such start.
  - If the leftovers stay, upgrading again with `--rangesync-enable=true` costs one refill (the
    marker no longer matches the reserve, ≈ 65–75 s at 4.2M per G4). No stale entry is served.
- **(T6.2) `ReserveRepairer` with missing chunks (outside the plan).** See the Decisions-log
  finding. In step 3, a missing or invalid chunk should not delete `ChunkBinItem{bin, old bin ID}`
  (step 2 already deleted them all). For example, remove its `BatchRadiusItem`, stamp entries and
  data without the `ChunkBinItem`. Then unskip `TestReserveRepairMissingChunk`
  (`pkg/storer/reserveaddr_toggle_test.go`). For a separate issue against `bee db repair-reserve`.
- **(T6.2) T6.1, `streamtest` lagging-reader deadlock.** `record.Write` holds the record lock
  while it sends on a 16-slot signal channel, and `Read` takes a signal only once it caught up. So
  a writer more than 16 writes ahead blocks with the lock held, and a `FullClose`/`Reset` of that
  stream then deadlocks. A rangesync `Fetch` of ~25 chunks over one stream hit it in
  `TestRangesyncToggle`; the test now fetches 4 IDs per stream. A streamtest-based T6.1 suite will
  meet it with large fetches. Bound the IDs per Want there, or fix `pkg/p2p/streamtest` (outside
  the plan).
- **(T6.2) Tests to run with -race on CI.** The CI rule stays: `go test -race ./pkg/rangesync/...`,
  `TestCombinedSyncerBothDrivers`, and the three Phase-1 storer tests `TestIterateAddrNoReserveLock`,
  `TestIterateAddrSnapshot`, `TestSetReserveObserver`. Add these new tests:
  - `TestRangesyncToggle` and `TestRangesyncDefaultsNoIndex` (`pkg/node`): a real storer with the
    rangesync Indexer goroutine, the observer and streamtest sessions/fetches.
  - `TestReserveAddrIndexDefaultsAfterToggle` and `TestReserveAddrIndexDowngrade` (`pkg/storer`):
    the background drop goroutine runs alongside the test's reads, and `SubscribeBin` goroutines.

  The other new storer tests run their steps one after another (the fill blocks in `New`) and
  don't need it.
- **(T6.2 fix round 1) T6.3, runbook (`testnet-runbook.md`), downgrade paragraph, corrected.**
  This entry replaces the earlier T6.2 runbook entry: that one understated the size, implied that
  every interrupted reclaim resumes, and named no completion signal.
  - **What stays on disk.** A node downgraded to a build without `rangesync` starts and runs
    normally with the leftover `chunkAddr` entries and the `addrIndexValid` marker. That build
    never reads or changes them (verified, see the Decisions log). They take space:
    - §5.1 estimates ~80 B per chunk, ≈ 350 MB at 4.2M chunks;
    - T6.2 measured 85 B per chunk at 4000 chunks, after a restart (leveldb tables after the
      journal flush);
    - G4 measured ≈ 600 MiB (≈ 150 B per entry) at 4.2M, right after a fill, uncompacted.

    Plan for 350–600 MiB on a full reserve.
  - **Reclaim before downgrading.** Start once with a `rangesync`-aware build and
    `--rangesync-enable=false`. Keep it running until both the background drop and the compaction
    after it have finished. On G4's host at 4.2M that is ≈ 12–14 s of drop, then ≈ 19–21 s of
    compaction; allow several times that on a loaded node. Check that `localstore/indexstore` got
    smaller (`du`) before stopping.
  - **If it is stopped early.**
    - During the drop: the next start with `--rangesync-enable=false` resumes the drop, then
      compacts.
    - During the compaction: every entry is already deleted, so the next disabled start finds
      none and runs neither drop nor compaction (`hasAddrIndexEntries`). The space then comes back
      only as leveldb rewrites those tables in its own compactions. `bee db compact` compacts
      sharky, not leveldb.
    - To force the reclaim, start once with `--rangesync-enable=true` (a refill, ≈ 41–44 s at
      4.2M), then again with `--rangesync-enable=false`, and let it finish.
  - **Completion signal.** The only one is the Debug log `dropped the disabled reserve address
    index` (with `count`). It is written when the drop ends, before the compaction. Nothing reports
    the end of the compaction; a failure is logged at Debug. Set the `storer` logger to debug
    (`--verbosity` or the `/loggers` API) and wait at least the compaction time after that line,
    or watch the directory size.
  - **Upgrading again with the leftovers** costs one refill (≈ 65–75 s at 4.2M). No stale entry is
    served.
- **(T6.2 fix round 1) Operator-visible end of the reclaim (production change, outside T6.2).**
  Today an operator cannot see when the space is back. Suggested: an Info log after the post-drop
  compaction (entries dropped, drop and compaction durations), or a metric for the
  drop/compaction state. Both would go in `pkg/storer/reserveaddr.go` (`startAddrIndexDrop`,
  `compactAddrIndex`).
- **(T6.2 fix round 1) T6.1 `streamtest` note, corrected.** Once a writer blocks on the full
  16-slot signal channel while it holds the record lock, the reader cannot drain the channel
  either: `Read` calls `bytesSize`, which takes the same lock. So the stream deadlocks for both
  ends: the reader's next `Read` as well as `FullClose`/`Reset`, not only the close.
- **(T6.2 fix round 1) CI -race list:** no change. `TestReserveRepairMissingChunk` is skipped. The
  new `TestReserveAddrIndexRepair/older build` runs its steps one after another.
  `TestRangesyncToggle` is already on the list.
- **(Orchestrator, T6.2 verification) A load-induced flake in `TestSyncRateHonestChurn/radius_half`**
  (pkg/rangesync/reconciler, T4.7 part 3): "timed out waiting for a gated fetch" once in a full
  `go test ./pkg/storer/... ./pkg/node/... ./pkg/pullsync/... ./pkg/puller/... ./pkg/rangesync/...
  -count=1` run on this 4-core host; 30/30 isolated and a second full run passed. Watch it in T6.1's
  runs and on CI; if it recurs, widen the test's wait or move it onto the injected clock.
- **(T6.1 part 1) Tests to run with -race on CI.** `TestAcceptanceNet` and `TestAcceptance`
  (`pkg/node`, `-tags=integration`). `TestAcceptance` runs real storers, pullers, pullsync and
  rangesync services, reconcilers, the reserve workers and the transport's handler goroutines at
  once, and has never run under `-race` (no C compiler here). Suggested:
  `RANGESYNC_ACCEPTANCE_SIZES=5 go test -race -tags=integration -run 'TestAcceptance' -timeout 60m
  ./pkg/node/`. Without `-race`, the default sizes take ~5 min with a peak RSS of 1.3 GB.
- **(T6.1 part 1) `make test-integration` and the 10-min timeout.** It runs `go test -tags=integration
  -v ./...` with `go test`'s default 10-min timeout per package binary. `pkg/node`'s integration
  tests now take ~5 min (the default sizes 5 and 9). That fits, but a slower runner or `-race`
  will need a `-timeout` there or `RANGESYNC_ACCEPTANCE_SIZES=5`.
- **(T6.1 part 1 → T6.4) AC-B1 misses at N = 5 and 9.** Rangesync-only metadata is 0.51–0.71 and
  0.57–0.65 of Baseline's, against ≤ 0.50; N = 17 passes at 0.39–0.40. The first set (every range
  scanned) agrees. The pair model of §7.8 holds: sessions find p(N − p) missing IDs per upload. But
  each ID costs 207–231 reconcile bytes (171–263 in the first set), against §7.8's 110–140 B. For
  the team, either:
  - §7.8's table should count what it leaves out (the decoding side's Items listing of the IDs only
    it holds, and a Fingerprint per session) and the measured symbols per difference (mean
    2.2–2.5), and the threshold be re-based on that; or
  - the protocol should be tuned. The first suspect is the first-batch size from hint(R): a tail of
    streams with many times more symbols than differences, in 1 of 9 rangesync-only and 1 of 9
    Mixed runs (2 of 12 and 7 of 12 in the first set).

  Checking the hint takes a run with hints off. `DisableHints` is in rangesync's `export_test.go`,
  out of `pkg/node`'s reach, so it needs either a test-only way to switch hints off in this harness,
  or a neighborhood-simulation run at this load with `simOptions.NoHints`.
- **(T6.1 part 1 → T6.3, T6.4) `cross_protocol_duplicates_total` undercounts on dual nodes.**
  It counts a fetched chunk that the reserve already holds. Pullsync puts a page's chunks after
  reading the whole page, so when rangesync stores a chunk first and pullsync's delivery of it is
  still in that page, neither protocol counts the duplicate. At N = 17 in the first measurement set,
  the metric counted 9 of 15.
  Rangesync alone cannot see this case, and pullsync cannot change. On testnet, AC-D3's ratio from
  the metric is therefore a lower bound. The dual cohort's exact number is
  `bee_pullsync_chunks_delivered` + `bee_rangesync_fetch_delivered_total` minus the unique chunks
  added.
- **(T6.1 part 1 → T6.4) AC-K2 cannot be measured as defined from the metric.**
  `symbols_per_difference` has no label for the difference size, so the "ranges with ≥ 20
  differences" filter cannot be applied outside rangesync's own tests (`SessionSample`). T6.1
  reports the unfiltered metric (indicative). An exact AC-K2 needs the observation restricted to
  streams with ≥ 20 differences, or a size label (a rangesync metrics change).
- **(T6.1 part 1 → part 2) Harness notes for Bootstrap, Radius decrease, Stale stamp and Churn.**
  - Stale stamp: add a restamp helper, with the same batch and index, a newer timestamp and a new
    signature (`newStamp` shows the signing). Push both stamps to different nodes.
  - Bootstrap: `accOptions.Empty` starts a node with an empty reserve. The node wiring leaves
    `rangesync.Options.LargeDiff` and `reconciler.Options.LargeDiff` at 65536. So an empty node next
    to the 4500 base chunks syncs through ordinary sessions (RequestItems) and never stripes. There
    are two ways to make it stripe, both with a cost:
    - use more than 65536 chunks per node: ~280 MB of chunk data per node, and minutes of setup;
    - set both options in the harness, and record that as a deviation from the node's wiring.
  - AC-V3, AC-N1, AC-N3 (`SyncRate` back to 0): the puller's rate window
    (`DefaultHistRateWindow`) and the reconciler's `SyncRateWindow` are both 10 minutes, so
    "`sync_rate` back to 0" takes ≥ 10 min of real time per run, and so does the reserve worker's
    next radius decrease. Measure the time to the union instead, or gate 10-min runs behind an
    environment variable.
  - A node joining after `start`: `addNode`, then `net.connect` to every node, then
    `stab.stabilize()`. The sampler's node list is fixed at `start` (index memory, sessions, round
    interval); extend it for late nodes.
  - Radius decrease: `NodeRadius`, `ReserveCapacity` and `ReserveWakeUpDuration` exist. Mixed radii
    in one neighborhood exercise T4.6's untested nested-root case (bucket cache, Follow-up
    "(T4.6 part 3 → T6, a radius-aware harness)").
  - Churn: `accNet.disconnect` runs the topology and protocol hooks and resets the open streams;
    `accNet.connect` reconnects.
  - All part-1 runs used `Latency` 0. A run with a one-way latency (`accOptions.Latency`) would show
    how both protocols' upload latency and duplicates depend on it.
