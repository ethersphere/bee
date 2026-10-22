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
