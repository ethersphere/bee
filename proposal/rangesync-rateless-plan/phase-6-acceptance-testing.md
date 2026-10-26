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

## T6.5 Pre-cluster fixes and observability (added 2026-10-26, before T6.3's cluster runs)

**Depends on:** T4.7, T6.1. **Blocks:** T6.3's cluster runs (the runbook is finished against it).
Placed in Phase 6 because the T6.1 acceptance harness, which reproduces both findings, lives here.

**Scope (two parts, each verified and reviewed separately):**
1. **Part 1, production fixes from T6.1 part 2** (Follow-ups "(T6.1 part 2 → T4, Low) Stripe
   sessions race the moved-root Status", "(T6.1 part 2 → T4/T6.4, finding) Stripe verification scores
   honest deliverers when uploads land in their stripes", and "(T6.1 part 2 fix round 1 → before
   T6.3)"):
   - the stripe worker waits for the moved-root Status to be reached (as round sessions do) before
     it opens stripe sessions after a radius change, so the stripe peer can build the scope;
   - stripe verification does not count against a deliverer the chunks uploaded into its stripe
     after it listed the stripe, while a withholding deliverer is still detected (T3.3, T4.4 and
     T4.5 rules).
2. **Part 2, cluster knobs and observability** (T6.3 docs Follow-ups):
   - a `--rangesync-fetch-rate` flag (chunks per second, default `DefaultFetchRate`), plumbed into
     `rangesync.Options.FetchRate` for full nodes with rangesync enabled;
   - an `isFullySynced` gauge in `pkg/node`, reported in every sync mode so the pullsync-only
     cohort reports it too (an additive metric, no behaviour change);
   - a reason label on the reconciler's `peer_failures_total`;
   - metrics for stripe-job waits (queue wait of claims and own jobs) and first-chunk latency of
     stripe jobs and catch-ups;
   - an Info log when the address-index reclaim (background drop + compaction) finishes.

**Tests:** a regression test per fix that fails before it; in the acceptance harness, the Radius
scenario's stripes are no longer reassigned on the moved-root race and
`TestAcceptanceChurn/N=5/bootstrap_during_uploads` runs unskipped and passes; the flag's wiring and
its default; the new metrics registered and moving; `go test ./pkg/pullsync/... ./pkg/puller/...
./pkg/storer/...` and the defaults unchanged apart from the new metric.

**Size:** M (two parts).

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
- **(T6.1 part 1 fix round 1) Push races are timed (review Medium 1).** The rule of "(T6.1 part 1)
  Duplicates: a delivery racing a push …" excused every first rangesync delivery of a pushed chunk
  to one of its holders, whenever it came, up to ⌈1 % × chunks fetched⌉ per run (6, 18 and 42 at
  N = 5, 9 and 17). A regression in the claim's reserve re-check, or a holder refetching a chunk a
  round after its push, would have passed. Now:
  - the harness records when each protocol's first delivery of an ID reached a node (in the
    counting `validStamp`), and when the push's `Put` to each holder returned;
  - rangesync's first delivery of a pushed chunk races the push only if it lands within
    `accPushRaceWindow` (1 s) of that `Put`, before or after. Past it, it counts as a rangesync
    repeat, which fails AC-D1 (rangesync-only, and Mixed, next entry). `classifyDeliveries` holds
    the rule;
  - pullsync's first deliveries of pushed chunks stay untimed: they are reported, not asserted.

  Why 1 s. A delivery races the push in two ways:
  - the push lands between the claim's re-check and the delivery: the delivery follows the push
    by less than the fetch's latency from that re-check;
  - rangesync stores the chunk first, fetched from a holder the same push reached a moment earlier:
    the delivery precedes the push by less than the push's span over its holders.

  The suite now measures both bounds in every run. The fetch latency runs from the reserve's last
  miss of the ID (`accStore.ReserveHasID` records the misses; before a delivery the last one is the
  claim's re-check, or a later check) to the first rangesync delivery; the push span from the
  push's start to its last holder's `Put`. Over this round's five runs per scenario (Results):
  - fetch latency: median 12–129 ms, p99 45–373 ms, maximum 48–411 ms, the largest at N = 17
    rangesync-only (30 uploads/s on 4 cores);
  - push span: at most 159 ms;
  - the races themselves: 3 (two in one N = 5 rangesync-only run, one at a rangesync-only node of
    an N = 5 Mixed run), at most 61 ms from their push.

  1 s is 2.4× the largest fetch latency and a fifth of `MinRoundInterval`. The margin is
  narrowest at N = 17; a run with a one-way latency must re-check it (Follow-ups).

  What the window cannot tell: a broken re-check that refetches within 1 s of the push looks like a
  race. In rangesync-only the 1 % bound still catches it unless it is rare; in Mixed races are
  reported, not bounded.

  Negative controls:
  - `TestAccDuplicateClassification` (committed) checks the rule: deliveries 100 ms after and
    50 ms before the push race it; one 1 ms past the window is a rangesync repeat, as is a second
    delivery; a base chunk's delivery is a false difference.
  - End to end, with a temporary patch (not committed): once the network was quiet, a holder's
    `ReserveHasID` was made to answer false for one of its pushed uploads, and the holder's
    `rangesync.Service.Fetch` asked another holder for it. The real fetch path claimed the ID,
    re-checked it, sent the Want and stored the delivery. At N = 5 both scenarios failed:
    rangesync-only with "rangesync delivered 1 IDs again (1 of them pushed chunks past 1s of the
    push)" (node 2, 45 s after its push), Mixed with "node 2 (D): rangesync delivered 1 IDs again"
    (35 s after). Without the patch both pass.
- **(T6.1 part 1 fix round 1) Mixed: duplicate and AC-S3 gates on dual nodes (review Medium 2).**
  `checkMixed` asserted only AC-S3 on rangesync-only nodes, AC-K3 and no faults. Yet Mixed is the
  only scenario with dual nodes, where a bug in the index and observer running next to pullsync
  would show as deliveries of base chunks or rangesync repeats. It now asserts, per rangesync-only
  and dual node:
  - no rangesync repeat, with the timed push-race rule (AC-D1);
  - no delivery of a base chunk, by either protocol (a false difference);
  - `bee_pullsync_chunks_sent_offered` > 0 and `bee_pullsync_chunks_sent` > 0, on dual nodes too:
    the pullsync-only nodes pull from them, the T1.13 gap that `pkg/storer/mock` could not show
    (AC-S3);
  - and, on rangesync-only nodes as before, no pullsync delivery.

  Correction to "(T6.1 part 1) Acceptance-criteria rows …", AC-D1: "so did the rangesync-only and
  dual nodes of Mixed" was observed in part 1, not asserted: `checkMixed` had no duplicate check.
  It is asserted from this round. AC-S3 was asserted on rangesync-only nodes only; it now covers
  dual nodes as well.
- **(T6.1 part 1 fix round 1) AC-V1 is asserted.** Correction to the AC-V1 entry ("Reported, not
  asserted"): AC-V1 gates T6.1, so `checkRangesyncOnly` and `checkMixed` now assert it at every
  size: every upload is on every node of its root within 2 × `MinRoundInterval` (10 s), which is
  within 2 × `RoundInterval` whatever the adaptive value was. The slowest upload took 5.0–5.5 s
  (5.46 s at N = 17, rangesync-only), so the margin is ~1.8×.
- **(T6.1 part 1 fix round 1) The root cache path is confirmed.** `checkCaches` relied on both
  sides holding more than `ScanLimit` elements. It now also asserts that, during the probe session:
  - the node answered the probe's root Fingerprint with Symbols parts and no Fingerprint part (no
    split);
  - the probe decoded at least one stream (`symbols_per_difference` observed) and none ran out
    (`streams_exhausted_total` 0);
  - the node made no `ReserveIterateAddr` call over its whole root (the harness's store wrapper
    counts them): a scan of the root, or a listing of it, would make one.

  For the root, `fingerprint.Index.Symbols` serves the first `RootSymbols` symbols from the root
  cache whatever the count; only symbols past them would need a scan. So a decoded root stream with
  no root iteration was served by the cache. It held on every rangesync node of every run below.
- **(T6.1 part 1 fix round 1) Reference load at N = 17.** At 15 uploads/s, a pair's difference per
  round falls with N: a share 2p(N − p)/(N(N − 1)) of the uploads, 37.5 per 5 s round at N = 9 and
  23 at N = 17, where the median `pairwise_diff` fell in the bucket 16–31. That bucket cannot tell
  16–19 from 20–31, so the reference load (median ≥ 20) was not shown. `accRate` keeps 15/s up to
  N = 9 and, above, gives a pair 1.2 × N = 9's expected difference per round (45), rounded up to
  5/s: 30 uploads/s at N = 17, 600 uploads over the same 20 s. Measured once: the median
  `pairwise_diff` is in the bucket 32–63 in rangesync-only (1176 sessions) and 64–127 in Mixed,
  so N = 17 now runs at the reference load. `pairwise_diff_half`'s p99 is ≤ 64 (≤ 256 in Mixed),
  below `CatchUpMinDiff`. Part 1's N = 17 numbers (15/s) stand as a measurement at a lower load
  (median in 16–31).
- **(T6.1 part 1 fix round 1) Biases of the measurement (review Lows 4 and 5).** None changes a
  verdict; each is recorded so that T6.3 and T6.4 read the in-process numbers right.
  - **Wire metadata leaves out per-stream costs.** accNet counts the bytes of the protocol
    messages only. On libp2p every stream also costs multistream-select (the protocol ID both ways),
    yamux frames (12-byte headers for open, data, window updates and close), Noise framing (2 bytes
    of length and a 16-byte tag per frame) and bee's headers exchange (a `pb.Headers` each way,
    `pkg/p2p/libp2p/headers.go`), a few hundred bytes per stream. Pullsync opens a stream per
    `Sync` call (one page) and per cursor request; rangesync one per Status, session and Want, far
    fewer. So the missing bytes are larger on Baseline's side: the "Wire metadata" ratios, and
    AC-B1's misses at N = 5 and 9, are conservative for rangesync.
  - **Clique topology.** Every node is connected to every other and all share one neighborhood.
    A real node also has peers outside its neighborhood: the reconciler tracks every connected peer
    and sends each one its Status at least every `StatusMaxInterval` (60 s, the heartbeat branch of
    `statusDue`, `pkg/rangesync/reconciler/gossip.go`), although no session runs with an
    ineligible peer. With ~100–150 such peers at ~100 B per Status that is ~170–250 B/s in per
    node, a few percent of rangesync's `bytes_metadata{dir="in"}` at the reference load (~7 KB/s
    per node at N = 9). The puller also syncs one bin (the peer's PO) with peers up to
    `maxPODelta` (2) below the radius, but at equal radii their reserves hold nothing in that bin,
    so it carries almost no offers. The suite sees neither; on balance the clique biases AC-B1
    toward rangesync by a few percent.
  - **In-memory storers.** `storer.New(ctx, "", …)` keeps leveldb and sharky in memory, so the
    address index's writes (one `ChunkAddrItem` per put) and its scans cost no disk I/O. Only
    rangesync nodes keep that index, so AC-R7's process CPU leaves out work a disk-backed rangesync
    node does: a bias toward rangesync. T6.3 measures it on disk.
  - **AC-R7 per upload, not as a rate.** AC-R7 compares `rate(bee_process_cpu_seconds_total)`.
    The suite compares CPU over the window per upload, that is total CPU. The rangesync scenarios'
    windows are longer (slower convergence and a later quiet tail), so their rate ratio to Baseline
    is lower than their per-upload ratio: the per-upload comparison is conservative for rangesync.
- **(T6.1 part 1 fix round 1) Results.** The default sizes twice and N = 17 once, at `GOMAXPROCS`
  4 and 4500 base chunks, with the fix-round assertions; min–max over the two runs. N = 17 now runs
  at 30 uploads/s (600 uploads), N = 5 and 9 at 15/s (300), as before. Per upload as in the part-1
  table.

  | | N = 5 (2 runs) | N = 9 (2 runs) | N = 17 (1 run, 30/s) |
  |---|---|---|---|
  | Metadata B per upload (AC definition) B / R / M | 2040 / 1319–1485 / 1624–1626 | 7344 / 4212–4219 / 6561–6635 | 27744 / 9740 / 23039 |
  | R ÷ B (AC-B1) · M ÷ B | 0.65–0.73 · 0.80 | 0.57 · 0.89–0.90 | 0.35 · 0.83 |
  | Wire metadata R ÷ B | 0.60–0.68 (B 2200 B) | 0.53 (B 7889–7891 B) | 0.33 (B 29084 B) |
  | R metadata by stream: status / reconcile / fetch Want, B per upload | 16 / 1162–1329 / 140–141 | 59 / 3732–3740 / 421 | 111 / 8647 / 982 |
  | Chunk B per upload B / R / M | 18.0 K / 8.5 K / 13.7–14.0 K | 53.2–53.9 K / 25.5 K / 45.1–45.2 K | 129 K / 59.6 K / 107 K |
  | Upload latency median / p95 / max, B | 0.26 / 0.53 / 0.93 s | 0.26 / 0.47–0.52 / 0.93–0.94 s | 0.26 / 0.50 / 1.42 s |
  | same, R | 2.5–2.6 / 4.8 / 5.0 s | 2.6–2.7 / 4.8–5.0 / 5.1–5.3 s | 2.9 / 5.1 / 5.5 s |
  | same, M | 2.2 / 4.7 / 5.0 s | 2.5–2.6 / 4.8 / 5.0–5.1 s | 2.6 / 4.9 / 5.3 s |
  | Last session after the last upload R / M (AC-C2) | 0.1–5.1 s / 5.1–5.2 s | 5.1 s / 5.1 s | 5.1 s / 0.1 s |
  | Duplicate deliveries B: pullsync repeats (needed deliveries) | 666–668 (600) | 1948–1996 (1800) | 9820 (8400) |
  | R: rangesync repeats / racing a push (max distance) | 0 / 0–2 (48 ms) | 0 / 0 | 0 / 0 |
  | M: pullsync repeats / cross-protocol / `cross_protocol_duplicates_total` | 367–383 / 0 / 0–1 | 1375–1382 / 0 / 0 | 6595 / 13 / 4 |
  | M: rangesync repeats at R / D nodes; base-chunk deliveries; races | 0 / 0; 0; 0–1 (61 ms) | 0 / 0; 0; 0 | 0 / 0; 0; 0 |
  | Rangesync fetch latency median / p99 / max, R · M | 21–30 / 51–80 / 55–89 · 19–24 / 58–80 / 59–83 ms | 26–89 / 73–259 / 82–276 · 12–33 / 45–90 / 48–100 ms | 129 / 373 / 411 · 50 / 125 / 143 ms |
  | Push span max | 9–25 ms | 3–123 ms | 159 ms |
  | CPU s per 1000 uploads B / R / M | 12.7–13.0 / 8.2–9.3 / 13.1–13.7 | 11.4–28.7 / 6.5–16.8 / 10.2–27.4 | 22.0 / 11.5 / 19.3 |
  | CPU R ÷ B (AC-R7) · M ÷ B | 0.63–0.74 · 1.01–1.08 | 0.57–0.58 · 0.90–0.95 | 0.52 · 0.88 |
  | Peak Index memory per node; sum R / M | 21.0 MB; 105 / 63 MB | 21.0 MB; 189 / 126 MB | 21.0 MB; 357 / 231 MB |
  | Peak live Go heap of the process B / R / M | 216–223 / 297–305 / 289 MB | 438–445 / 630–631 / 534–537 MB | 849 / 1217 / 1076 MB |
  | `pairwise_diff` median bucket, R (sessions) | 32–63 (72–74) | 32–63 (296) | 32–63 (1176) |
  | IDs sessions found missing per upload, R (p(N − p)) | 6.0 (6) | 17.9–18.0 (18) | 43.2 (42) |
  | Reconcile B per ID found missing, R | 195–223 | 207–208 | 200 |
  | `symbols_per_difference` median / p95 bucket; mean, R | ≤ 1.75–2.25 / ≤ 3–3.5; 1.92–2.21 | ≤ 2 / ≤ 3.25; 2.17–2.19 | ≤ 2 / ≤ 3.25; 2.15 |

  - No verdict changes. AC-B1 still fails at N = 5 (0.65–0.73) and N = 9 (0.57), and passes at
    N = 17 (0.35, at the reference load now; 0.39–0.40 at 15/s in part 1). AC-R7 passes
    (0.52–0.74). AC-C1 (with the cache-path check), AC-C2, AC-V1, AC-D1 (rangesync-only and,
    now, the rangesync-only and dual nodes of Mixed), AC-K3 and AC-S3 (now on dual nodes too)
    pass, all asserted.
  - At N = 17 the higher rate lowers rangesync's metadata per upload (9740 B against
    10843–11102 B at 15/s): the per-session overhead (Fingerprint, Items listing, `MinBatch`) is
    spread over more differences. Baseline's per-upload metadata does not depend on the rate.
  - Mixed used 1.01–1.08 of Baseline's CPU at N = 5 (0.74–0.99 in part 1). AC-R7 compares the
    rangesync-only cohort, not Mixed, so this is reported only; a dual node runs both drivers.
  - CPU per upload varied by up to 2.5× between the two runs at N = 9 (Baseline 11.4 and
    28.7 s per 1000 uploads; the host is a shared desktop), while the ratios moved by 0.01–0.05.
  - The dual nodes' `cross_protocol_duplicates_total` counted 4 of the 13 cross-protocol
    duplicates at N = 17, as part 1's undercount note says.
- **(T6.1 part 1 fix round 1) Runs.** No `-race` (no C compiler).
  - The default sizes twice (`go test -tags=integration ./pkg/node/ -run 'TestAcceptance'
    -count=1 -timeout 60m`, run as the compiled test binary to sample its RSS): both passed, in
    6.8 and 5.9 min, peak RSS 1.36–1.38 GB (5.1 min in part 1: the probe session and the new
    bookkeeping add little; most of the spread is the host). That is still inside
    `make test-integration`'s 10-min default timeout, with less room.
  - N = 17 once, alone (`RANGESYNC_ACCEPTANCE_SIZES=17`): passed in 3.1 min, peak RSS 2.6 GB.
  - The end-to-end negative control (temporary patch, above) failed both scenarios as intended;
    the patch was reverted.
  - `go test ./pkg/node/... ./pkg/pullsync/... ./pkg/puller/... ./pkg/storer/...` (default build)
    passes.
  - `go vet ./pkg/node/...` with and without `-tags=integration`, `GOTOOLCHAIN=go1.26.1 make
    lint`, `golangci-lint run --build-tags integration ./pkg/node/...` and `go build ./...` are
    clean. Only the two changed test files were formatted (gofumpt, gci).
  - No production code changed.
- **(Orchestrator, T6.1 part 1 landing, 2026-10-22) Part 1 landed after one fix round.** Tests
  only (pkg/node, `-tags=integration`). Review: no High; Medium 1 (the push-race exception ignored
  timing) and Medium 2 (Mixed had no duplicate gate) and four Lows fixed in fix round 1, which
  verified clean (no re-review: no High). The fix round's verifier re-ran the suite: N = 5 R÷B
  metadata 0.68–0.72, CPU 0.58–0.65; N = 9 metadata 0.57–0.58, CPU 0.50–0.53; N = 17 (30/s)
  metadata 0.35, CPU 0.49; AC-V1 max 5.38 s. The spread between runs (a few points) is run-to-run
  variance; no verdict changes. AC-B1 fails at N = 5 and 9 and passes at N = 17 (T6.4 decides the
  per-N re-basing, as the G6 sheet proposed). T6.1 is ticked after part 2.
- **(T6.1 part 2) Harness additions.** Tests only, in `pkg/node` (`//go:build integration`): the
  part-2 scenarios are in a new file, `rangesync_acceptance_scenarios_test.go`; the harness and the
  transport gained what they need, and part 1's scenarios are unchanged by it except where noted.
  - **Bounded stream window** (`accOptions.Window`, `accPipe`): a writer sends at most the window's
    bytes that its reader has not read, then waits; the reader's window update reaches the writer
    after the latency, so a stream moves at most a window per round trip. 256 KiB
    (`accWindow`, go-yamux's `initialStreamWindow`; go-yamux grows it up to 16 MiB with its
    auto-tuning, which accNet does not model, so the bound is conservative). A reset, a close of
    either end or a disconnect ends a write that waits. `TestAcceptanceNetWindow` checks it, with
    and without a latency.
  - **Per-pair counters** (`accNet.pairStats`): the streams one host opened to another and the
    bytes both wrote on them, per protocol stream, across reconnects. AC-C4 (sessions between
    disjoint roots), AC-B4 (old-half cost per peer) and Churn (the peer delivering a stripe) read
    them.
  - **Late join, leave, rejoin** (`accHarness.join`, `leave`, `rejoin`): `join` builds the node
    then (`addNode` takes an `accNodeSpec` now), adds it to the sampler (which grows), connects it
    to every online node and stabilizes it, as a node starting next to a running neighborhood.
    `leave` disconnects a node from every peer (the transport's teardown: topology and protocol
    hooks, handler contexts canceled, streams reset) and marks it offline: `push` then skips it, as
    pushsync would. Snapshots and results take nodes that joined after a snapshot.
  - **Per-node reserve options**: `WakeUp` (the reserve worker's interval) and `Hold`. A held
    node's reserve worker reads its syncer through `accSyncGate`, which reports a non-zero rate
    until the scenario calls `release`. It stands for the moment the reserve falls below its
    threshold, which the scenario picks; the drivers themselves are untouched (the gate is only
    what the reserve worker reads; `isFullySynced` is not wired in the harness).
  - **Overlays and base per root**: `Overlay` places node i (Radius decrease: two halves);
    every node holds the base chunks inside its own root (in part 1 that is every base chunk, as
    before: same overlays, same chunks).
  - **Checks**: `checkUnion` also fails a node whose reserve holds a chunk outside its root (no
    scenario raises a radius), which is AC-C4's "syncs only the overlapping range" for every node;
    `checkUnionNoProbe` skips the probe sessions. `accTopology` records every radius the reserve
    set (`radiusChanges`).
  - **Helpers**: `restamp` (same batch and index, newer timestamp, new signature), `uploadPhase`,
    `watchRate` (a node's sync rate at 10 Hz), `sampleEqual` (AC-N5), the stale peer of Stale stamp.
  - `pullerLiveWorkers` replaces part 1's worker count in `waitReady`: a peer up to 2 bins below
    the radius has one live worker (the Radius decrease halves are at PO 1). Same value in part 1.
  - Part 1's code changed only where part 2 needed it: `checkCaches` runs its probe session again
    (up to 3 times) when another peer ran a session with the node meanwhile, since the node's
    `parts_sent` and root iterations count every session it serves (a nested-root network keeps
    running such sessions, Follow-ups; it failed the probe in 1 of 3 nested runs before the
    change); the sampler also reads every reconciler's `sync_rate`, and `checkRangesyncOnly` and
    `checkMixed` assert that it stays 0 (AC-N1's steady state, `checkSteadySyncRate`); the result
    counts `streams_exhausted_total`; and the default sizes (next entry).
- **(T6.1 part 2) Deviations from the node's wiring.** Each is a harness setting, recorded here
  as T4.7 part 1 recorded its scaled tests:
  - `LargeDiff` 128 (`accLargeDiff`) instead of 65536, on every rangesync node of every part-2
    scenario (`rangesync.Options.LargeDiff` and `reconciler.Options.LargeDiff`): ~1/35 of the
    4500-chunk reserves, against 1/64 of 2^22 in the node, and above a session's steady-state
    difference at the reference load (~40), so that only a catch-up stripes. Base/64 (70) was
    too close to two rounds' worth of uploads.
  - The bounded window (above) in every part-2 scenario and in the latency run.
  - Reserve-worker intervals: 1 s for the rangesync Bootstrap joiner (AC-N2: any tick that reads
    a sync rate of 0 lowers its radius), 10 s for the Radius decreaser (see the Radius entry),
    15 min in the node (`reserveWakeUpDuration`; the harness's other nodes keep the storer's 30
    min, so they never tick).
  - Reserve capacity 2 × 4500 in Bootstrap, so that the joiner's final count is the threshold:
    below it all through the bootstrap.
- **(T6.1 part 2) Scenario designs.** Each runs on rangesync-only nodes and, where a criterion
  compares the protocols, on pullsync-only nodes with the same seed (same overlays and chunks;
  stamps differ per run), as part 1 compares Baseline with rangesync-only.
  - **Bootstrap** (`TestAcceptanceBootstrap`, N = 5 by default,
    `RANGESYNC_ACCEPTANCE_SCENARIO_SIZES`): N − 1 nodes hold 4500 base chunks, then an empty node
    joins (`join`). Three networks: Baseline (pullsync-only holders and joiner), rangesync
    (rangesync-only holders and joiner), and "rangesync nested root": the same with a joiner one
    bit shallower (radius 1), whose root contains the holders' and that no neighbor shares, T4.7
    part 1's constraint (d), which syncs through claims of the session roots instead of
    `IsBootstrap`. "AC-V3 vs a pullsync-only node bootstrapping on the same network" is read as
    acceptance-criteria.md words it ("the same neighborhood, same reserve size and neighbor
    count"): two networks with the same overlays and chunks. One network with both joiners at once
    was not used: they would load the same holders, the pullsync joiner would also pull from the
    rangesync one, and AC-R7's CPU cannot be split per joiner in one process. The pullsync joiner
    keeps the default reserve-worker interval, so that AC-V3 compares the two as a node runs them.
  - **Radius decrease** (`TestAcceptanceRadius`): a radius-1 neighborhood in two halves of 3
    nodes at radius 2 (`Overlay`), each half holding its ~4500 base chunks. Node 0 (the decreaser,
    `Hold`, reserve-worker interval 10 s) is released once the network is ready; its count is
    below the threshold (capacity 65536), so it lowers its radius to 1 at its next tick, and its
    new half is the other half's root. Its count stays below the threshold after that, so the
    next decrease waits only for its sync rate (AC-N3). The mixed radii then get a phase of
    uploads (10 s at 15/s into the whole radius-1 neighborhood): the decreaser's sessions with the
    other half run on their root, nested in its own (§7.6, AC-C4, the bucket-cache case of T4.6
    part 3).
  - **Stale stamp** (`TestAcceptanceStaleStamp`): 4 nodes; a chunk is pushed with a stamp to node
    0 and with a newer stamp of the same batch and index (`restamp`) to node 1. On rangesync a
    stale peer (a rangesync service on a mock reserve holding the base and the older stamp, with
    no reconciler: it serves sessions and fetches and never fetches) connects to every node with
    the highest overlay, so every node initiates a session with it in every round and meets the
    older stamp again and again. The Baseline (pullsync-only, no stale peer) shows what pullsync
    does with the two pushes.
  - **Churn** (`TestAcceptanceChurn`, N = 5): N − 1 rangesync-only nodes with the base; uploads
    at the reference load for 20 s while node N − 2 leaves at 3 s and rejoins at 8 s (it misses
    ~75 pushes, below `accLargeDiff`, so its catch-up is a session). Then an empty node joins and
    bootstraps striped; when it holds 25 % of its root, the holder whose fetch stream to it moved
    bytes last leaves (it is delivering a stripe), and rejoins 5 s later. A sub-case with the
    joiner joining during the uploads is skipped by default (a finding, Follow-ups;
    `RANGESYNC_ACCEPTANCE_REPRO`).
  - **Latency** (`TestAcceptanceLatency`, skipped unless `RANGESYNC_ACCEPTANCE_LATENCY` is set):
    part 1's Baseline and rangesync-only scenarios, with their checks, a one-way latency on every
    stream and the 256 KiB window.
  - **Long runs** (`RANGESYNC_ACCEPTANCE_LONG`): Bootstrap waits for each joiner's sync rate back at
    0; Radius waits for the decreaser's, then for the second decrease, and checks that it came
    after the last positive rate read; Stale stamp watches the rounds for `RejectTTL` + 1 min. The
    networks of a scenario then run in parallel (only the times to 0 are compared).
- **(T6.1 part 2) Default sizes and the 10-minute package timeout.** This supersedes part 1's
  "`RANGESYNC_ACCEPTANCE_SIZES` defaults to 5,9". With the part-2 scenarios at their default
  sizes, part 1 at 5 and 9 took the package past 10 minutes (part 2 alone is ~5–6 min on this
  host). Part 1 now defaults to N = 5; N = 9 and 17, the sizes AC-V1 names, run with
  `RANGESYNC_ACCEPTANCE_SIZES=5,9,17` (part 1's recorded results came from such runs). The
  part-2 scenarios use a 5-s quiet tail (`accTail`, one `MinRoundInterval` with equal digests and
  no session started) instead of part 1's 10 s, and the pullsync network of Radius decrease skips
  the mixed-radii upload phase (its one development run: 975 B of metadata per upload, latency max
  0.45 s). The default run of `go test -tags=integration ./pkg/node/` costs 8m15s on this host
  (package 493 s: part 1 at N = 5 156 s, Bootstrap 108 s, Radius decrease 118 s, Stale stamp 49 s,
  Churn 62 s), against 5.9–6.8 min before part 2.
- **(T6.1 part 2) The bounded window does not change part 1.** Part 1 ran once with the 256 KiB
  window (a temporary build, not committed) at N = 5 and 9, against runs without it on the same
  code: metadata R ÷ B 0.65 and 0.58 (without: 0.66–0.68 and 0.57), wire metadata 0.61 and 0.54
  (0.61–0.63, 0.53), chunk bytes and duplicates unchanged (rangesync 0 repeats; pullsync 703 and
  1971 against 703–715 and 2037), upload latencies within 10 ms, rangesync fetch latency p99
  30–77 ms (24–82 ms), CPU R ÷ B 0.76 and 0.56 (0.65–0.73, 0.63). Steady-state pages and Wants stay
  below 256 KiB of unread data, so part 1 keeps the unbounded window and its numbers stay
  comparable with part 1's record. The part-2 scenarios and the latency run use it.
- **(T6.1 part 2) Bootstrap results** (N = 5: 4 holders + the joiner, 4500 chunks; min–max over
  the runs listed under Runs, outside the long run, whose networks ran in parallel).

  | | Baseline (P) | rangesync (R) | R, nested root |
  |---|---|---|---|
  | Join to first chunk | 0.09–0.10 s | 5.0 s | 5.0–10.0 s |
  | Join to union (AC-V3 proxy) | 9.5–9.9 s | 9.3–9.5 s (R ÷ B 0.97–1.00) | 9.4–14.9 s |
  | Fetch-limiter bound (4500 at 1000/s) | 4.5 s | 4.5 s | 4.5 s |
  | Metadata in, AC definition (B: offers × 102 B) | 1.84 MB, 408 B per chunk | 0.32 MB, 70.2 B per chunk | 0.32 MB, 70.3 B per chunk |
  | AC-B2 bound (1.15 × 4500 × 70 B + 1 MB) | – | 1.36 MB | 1.36 MB |
  | Of which after the union (verification) | – | < 0.01 MB | < 0.01 MB |
  | Chunk bytes ÷ (chunks × 4258 B) | 2.36–2.43 | 1.000 | 1.000 |
  | Deliveries (repeats) | 10605–10943 (6105–6443) | 4500 (0) | 4500 (0) |
  | Stripes; verified (chunks the verifications delivered) | – | done 4; 4 (0) | done 4; 4 (0) |
  | Joiner's round sessions in the window | – | 0 | 0 |
  | Sync rate from the join to the tail's end (min / max) | 0 / 17.7–18.2 | 1.0 / 7.5 | 1.0 / 7.5 |
  | Radius changes of the joiner | none (30-min interval) | none (1-s interval) | none (1-s interval) |
  | CPU s per 1000 chunks (R ÷ B) | 1.10–1.20 | 0.56–0.68 (0.53–0.60) | 0.67–1.11 |
  | Joiner's index memory, peak | – | 21.0 MB | 21.0 MB |
  | Reserve sample = a holder's (AC-N5) | yes | yes | yes |

  - The rangesync joiner listed each ID once (70.2 B per chunk, 0.17 of the pullsync joiner's
    metadata) and fetched each chunk once; the verification of its 4 stripes found nothing. The
    transfer took 4.3–4.5 s from the first chunk: the node-wide fetch limiter (1000/s) set it, with
    4 stripe peers at 250 chunks/s each. The other 5.0 s are the wait for the joiner's first round
    (the guard holds the sync rate meanwhile, so AC-N2 is not at stake): the time to union ties
    with pullsync at this size (0.97–1.00), where pullsync's duplicates (2.4×, also paced by the
    puller's 1000/s limiter) cost it the same.
  - The nested-root joiner (T4.7 part 1's constraint (d), `IsBootstrap` false: no peer shares its
    root) took the holders' root through session-root claims: the same 70.3 B per chunk, 4 stripes,
    no tail through round sessions (the network is static, so the claim listed everything). In 2
    of its 4 runs it waited one round more (first chunk at 10 s).
  - N = 9 (8 holders, 8 stripes; once): join to union R 9.28 s, nested 9.30 s, B 12.03 s (R ÷ B
    0.77); metadata 70.2 B per chunk against 816 B (0.09); pullsync's chunk bytes 2.91× the chunks
    (8615 repeats for 4500); CPU R ÷ B 0.66; 7 of the 8 stripes verified, 0 chunks found. More
    holders cost pullsync more racing offers and duplicates, while rangesync's transfer stays at the
    fetch limiter.
  - "Join to union" is the AC-V3 proxy: the sync rates themselves read 0 only 10–20 minutes later
    (long runs below).
- **(T6.1 part 2) Radius decrease results** (3 + 3 nodes, a new half of 4525 chunks; min–max over
  the runs listed under Runs: 8 of the rangesync network and 4 of the Baseline's).

  | | Baseline (P) | rangesync (R) |
  |---|---|---|
  | Decrease to first chunk | 0.07–0.11 s | 0.5–5.1 s |
  | Decrease to the new half complete (AC-V4 proxy) | 9.4–10.1 s | 8.8–17.8 s (R ÷ B 1.04–1.76 in the 4 runs with both) |
  | Of which the transfer (first to last chunk) | 9.3–10.0 s | 8.3–12.7 s |
  | Metadata in, AC definition | 1.38 MB, 306 B per chunk | 0.32 MB, 70.1 B per chunk (0.23 of B) |
  | Chunk bytes ÷ (new half × 4258 B) | 1.75–1.83 | 1.000 |
  | Old-half sessions with each old-half peer after the decrease (AC-B4) | – | 0 B (digests equal: no session) |
  | Stripes | – | done 4; reassigned 1–2 in 3 of 8 runs |
  | Sync rate from the transfer's first chunk to the end of the run (min) | 0–0.42 | 1.0 |
  | First positive sync rate after the decrease | 0.15–0.27 s | 0.5–5.1 s |
  | Radii in the default run | 2 > 1 | 2 > 1 |
  | Sessions across disjoint roots once ready; at start-up | 0; 0 | 0; 0–6 |
  | CPU s, decrease phase, outside the long run (R ÷ B) | 3.7–4.6 | 3.4–5.0 (0.74–1.24) |

  Upload phase with the mixed radii (rangesync only, 150 uploads at 15/s; the decreaser at radius 1,
  the others at 2): every upload on every node of its root, latency median 0.36–1.9 s, max
  4.95–9.97 s; at the decreaser (nested root) median 2.0–5.7 s, max 4.95–9.97 s, against 2 ×
  the round interval at the upload of at least 20–30 s (at least 10 s in one run). Metadata
  373–560 B per upload (the Baseline's one development run: 975 B). No streams exhausted, 3–27
  More requests; `symbols_per_difference` mean 2.3–3.6 (p95 bucket ≤ 4–4.75, > 5 in two runs).
  No duplicate, no corrupt decode. The interval bound matters: the slowest upload took 9.94–9.97 s
  in 6 of 8 runs, under 2 × `MinRoundInterval` (10 s) by 30–60 ms, after the decreaser's interval
  had grown to 30 s while its neighborhood was quiet.

  - **AC-V4 fails at this size.** Its causes:
    - in every run, the round lag: until the round that tracks the new root the reconciler does
      nothing (first chunk 0.5–5.1 s after the decrease, against 0.1 s for the puller's first
      page);
    - in every run, the stripe plan: 3 covering peers get 4 stripes (2^s ≥ k), so one serves
      two at the holder's 250 chunks/s, and the transfer takes half the new half ÷ 250/s,
      against the puller's three peers at once (with 1.8× duplicates);
    - and, in 3 of 8 runs, a stripe reassigned on the moved-root race (Follow-ups), which gives
      one peer three of the four stripes (13.6–17.8 s against 8.8–13.3 s without).

    At 2^22 chunks the round lag is negligible; the per-peer imbalance and the race remain.
    Without them the transfer is limited by the stripe peers (k × 250/s) or the node's fetch
    limiter (1000/s), the puller by its 1000/s limiter with its duplicates included: the fastest
    rangesync run (8.8 s) beat every Baseline run. T6.3 measures it at scale.
  - **AC-B4 passes**: chunk bytes are exactly the new half (each chunk once, 1.000), and the old
    half cost nothing: the decreaser's digests over its old-half peers' root were equal, so no
    session ran (0 B, against "one fingerprint exchange per peer, < 1 KB").
  - **AC-N1 passes from the transfer's first chunk**; between the decrease and the round that
    tracks the new root the reconciler's rate reads 0 (0.5–5.1 s; Follow-ups). The puller's reads
    0 too, until its first page (0.15–0.27 s).
  - **AC-N3 passes in the default run** (one decrease, none while the rate is above 0, ≥ 10
    minutes) and in the long run (below).
  - **AC-C4 passes**: no session between the radius-2 nodes of the two halves once ready (their
    roots are disjoint), and no reserve holds a chunk outside its root: the other half's nodes
    synced with the decreaser (whose root contains theirs) only their own root. At start-up, up to
    6 sessions ran across the halves: a harness node's index is built at radius 0 (the storer has
    no persisted radius; the base chunks are put before the warm-up) until the warm-up's rebuild,
    so its first Status carries the radius-0 root. In those, stripe plans of the other half's root
    listed nothing and some openings were refused as outside the scope; no chunk moved, nobody was
    scored. A node restores its persisted radius, so a restart builds its index at it.
- **(T6.1 part 2) Stale stamp results.** In every run every node ended with the newer stamp only,
  0.05–4.95 s (rangesync: at once when the stale peer's connection and the pushes trigger a round,
  otherwise at the next one) and 0.25–0.42 s (pullsync) after the pushes.
  - Rangesync: each node got the older stamp at most twice: stored once when it held neither
    (nodes 2 and 3 in some runs, then replaced by the newer), and rejected once when it held the
    newer (every node, the first fetch Q7 calls unavoidable). After that, every round's session
    with the stale peer (3–6 per node in 20 s) listed it again and none fetched it:
    `want_claims{rejected}` counted 11–20 skips, `rejected_cache_size` stayed at 1 per node.
    AC-C1 holds among the nodes, the Index included.
  - Pullsync (Baseline, the two pushes only): the nodes also end with the newer stamp. Node 0
    pulls it through live sync, since node 1's `Put` of the newer stamp takes a new bin ID, and the
    older one reaches node 1 once and is rejected by its `Put`. §7.7's "today A never learns about
    s2" does not hold for this case, two live pushes (proposal wording, Follow-ups).
- **(T6.1 part 2) Churn results** (N = 5: 4 nodes, 300 uploads at 15/s, then the joiner; N = 9
  once).
  - Every node converged (AC-C1); the joiner's base arrived 12.6–13.3 s after it joined (N = 9:
    11.1 s), with its active stripe peer gone at 25–26 % of its root: `stripes_total` done 4
    (N = 9: 8), reassigned 1.
  - The node that was away 5 s got the last upload it missed 12.0 s after it rejoined (N = 9:
    16.9 s; the round, the Status exchange and the initiator rule after a reconnect); upload
    latency over the window median 2.3–3.3 s, max 7.99–9.8 s.
  - AC-D1 with faults: 0 rangesync repeats, 0 deliveries of a held chunk, 0 races; no claim
    expired, so the bound (repeats ≤ expired claims) held at 0 ≤ 0. `want_claims{denied}`
    313–512 (N = 9: 2384): the want set kept the joiner's stripes and the rounds from fetching an
    ID twice.
  - No honest peer scored: `peer_failures_total` 0, no protocol disconnect or blocklist.
  - The rangesync fetch latency reached 0.93–1.0 s (p99, max) in the stripe phase: a Want of
    250 IDs at the holder's 250 chunks/s takes a second. No push ran then; with uploads during a
    striped transfer, a push racing a stripe fetch could land past `accPushRaceWindow` and count as
    a repeat (Follow-ups).
- **(T6.1 part 2) Latency run** (`RANGESYNC_ACCEPTANCE_SIZES=9 RANGESYNC_ACCEPTANCE_LATENCY=50ms`,
  one-way 50 ms on every stream, 256 KiB window, once). Both scenarios passed with part 1's checks.

  | N = 9, 50 ms | Baseline | rangesync | at latency 0 (part 1, fix round) |
  |---|---|---|---|
  | Upload latency median / p95 / max | 0.41 / 0.67 / 1.08 s | 3.0 / 5.3 / 5.9 s | B 0.26 / 0.47–0.52 / 0.93 s; R 2.6 / 4.8–5.0 / 5.1–5.3 s |
  | AC-V1 (within 10 s) | – | 300 / 300 | 300 / 300 |
  | Metadata per upload, AC definition (R ÷ B) | 7344 B | 4419 B (0.60) | 0.57 |
  | Wire metadata R ÷ B | – | 0.56 | 0.53 |
  | Pullsync repeats (needed deliveries) | 2502 (1800), 1.39 per needed | – | 1948–1996, 1.08–1.11 per needed |
  | Rangesync fetch latency median / p99 / max | – | 58 / 128 / 145 ms | 26–89 / 73–259 / 82–276 ms |
  | More requests; reconcile B per ID found missing | – | 132; 221 | –; 207–208 |
  | CPU R ÷ B | – | 0.57 | 0.57–0.58 |

  - A round trip of 100 ms adds ~0.15 s to pullsync's median upload latency and ~0.4 s to
    rangesync's; AC-V1 keeps a 1.7× margin (5.9 s against 10 s).
  - Pullsync's racing offers grow from 1.1 to 1.4 duplicates per needed delivery; rangesync stays
    at 0. AC-B1 at N = 9 moves from 0.57 to 0.60 (still a fail): 221 reconcile bytes per ID
    found missing against 207–208, with 132 `More` requests.
  - **`accPushRaceWindow` (1 s) re-checked:** the largest rangesync fetch latency was 145 ms at
    50 ms, 6.9× below the window; no delivery raced a push. The window stays 1 s. In striped
    transfers the fetch latency reaches ~1 s (Churn), so the window does not fit pushes during a
    stripe (Follow-ups).
- **(T6.1 part 2) Long runs** (`RANGESYNC_ACCEPTANCE_LONG=1`, once; Bootstrap, Radius decrease and
  Stale stamp in three processes at once, their networks in parallel: only the times to 0 and
  the second decrease are read from them).
  - Bootstrap: the sync rate of each joiner read 0 16m02s (rangesync), 16m08s (nested root) and
    16m12s (pullsync) after its join, 15.8–16.0 min after its last chunk: all three at the same
    10-minute boundary of Unix time. AC-V3 to the letter: R ÷ B 0.99, a tie at the window's
    resolution. No joiner's radius changed after it.
  - Radius decrease: the decreaser's rate read 0 15m58s (rangesync) and 16m06s (pullsync) after
    the decrease, and the second decrease (radius 1 → 0; its count was still below the threshold)
    came 2.9 s and 3.7 s after the last positive rate read, within one 10-s reserve-worker tick:
    AC-N3 holds for both.
  - Stale stamp: in the RejectTTL + 1 min after the convergence each node fetched the older stamp
    once more (the rejected cache entry expired after 10 minutes, and the next session's want
    went through and was rejected again): one fetch per stale ID per node per `RejectTTL`, as Q7
    says.
  - Idle sessions: while waiting, the rangesync joiner ran 0 sessions per minute with its
    same-root peers, the nested-root joiner 8.0 and the radius-1 decreaser 3.4 (Follow-ups).
- **(T6.1 part 2) Items earlier tasks handed to T6.1, measured where the harness can.**
  - **(T4.7 part 1 fix round 1 → T6.1, AC-B2) The tails of the two claimed transfers.** The
    bootstrapping node whose root no neighbor shares (Bootstrap, nested root) synced through
    claims of the session root at 70.3 B per chunk, with no round session after the claim: in a
    static network the claim lists everything, so there is no tail. The responder-only lacking
    node did not arise: the joiners' own transfers (the bootstrap, or the nested joiner's claims)
    ran before a holder's session could leave them a claim to answer. The tail needs uploads
    during the transfer (Churn's skipped sub-case, where the verification also scored honest
    peers), and its 2^22 geometry stays for T6.3.
  - **(T4.7 part 3 → T6.1/T6.3) Claims' first-chunk window.** The nested joiner's sync rate never
    read 0 before its first chunk (5.0–10.0 s after the join): at start-up the guard covers the
    window. After start-up the window was not reached (no scenario makes a running node claim a
    session root); T6.3.
  - **(T4.6 part 3 → T6) Nested session roots and the bucket cache.** In the upload phase with the
    mixed radii the decreaser's sessions with the other half run on their root, nested in its own:
    no stream exhausted, 3–27 More requests per phase, `symbols_per_difference` mean 2.3–3.6. The
    per-session difference stayed small (~20–40), below where `BucketSymbols` (32) would exhaust.
  - **(T4.6 part 3 → T6) Measurements the simulation could not make.** Bootstrap time against the
    real-clock fetch limiter: 4500 chunks took 4.3–4.5 s from the first chunk, the limiter's
    1000/s (with 4 stripe peers at 250/s each); with 3 peers on 4 stripes a peer's 250/s set it
    (Radius). First-chunk latency of a catch-up: 5.0 s after a join (the first round), 0.5–5.1 s
    after a radius decrease. Re-ask waits: `want_claims{denied}` 313–512 in Churn, all resolved
    without a duplicate; their waits are not separately visible in-process. Trimmed stripe parts:
    not reached (no radius increase during a stripe). Q7 under expired-batch skew and unknown-batch
    junk: not in scope here (no batch expiry in the suite).
- **(T6.1 part 2) Production code.** None changed; no test seam added (`node.RangesyncOptions`
  and `reconciler.Options.LargeDiff` were enough). Four findings, each with the way to reproduce
  it in the suite, are in the Follow-ups: the verification scoring honest stripe peers under
  uploads (skipped sub-case), the reconciler's rate lagging a radius decrease (latent at the
  node's interval), the stripe path's moved-root race and the in-sync nested-root sessions.
- **(T6.1 part 2) Acceptance-criteria verdicts.** Every row of acceptance-criteria.md whose
  "Measured by" names T6.1, across parts 1 and 2. "Asserted" means the suite fails on it; part 1's
  numbers are its fix-round results (N = 5 and 9 twice, N = 17 once at 30/s), part 2's are this
  round's (Runs). B = Baseline (pullsync-only), R = rangesync-only, M = Mixed.

  | Row | Verdict | Numbers (against Baseline where it has one) |
  |---|---|---|
  | AC-C1 union + Index | **pass**, asserted in every scenario | every node's reserve = its root's union, Index = brute force (summary + 4 levels), root cache probed; part 2 adds: no chunk outside a node's root |
  | AC-C2 quiescence ≤ 120 s | **pass**, asserted (part 1) | last session 0.1–5.2 s after the last upload; part-2 scenarios reach equal digests with a 5-s quiet tail. "The last `pairwise_diff` are 0" cannot be observed: equal digests start no session |
  | AC-C3 stale stamp | **pass**, asserted | all nodes end with the newer stamp in 0.05–4.95 s; the older fetched once per node into the rejected cache (stored once before where a node held neither), then skipped in 3–6 later rounds (`want_claims{rejected}` 11–20); `rejected_cache_size` 1; one refetch per `RejectTTL` (long run). Pullsync also converges here |
  | AC-C4 edge peers | **pass**, asserted | 0 sessions between disjoint roots once ready; nested peers sync only the overlapping range (no reserve holds a chunk outside its root, Radius and nested Bootstrap). Notes: up to 6 start-up sessions across disjoint halves (harness index at radius 0 before the warm-up rebuild); in-sync nested peers keep running sessions (finding, idle cost) |
  | AC-V1 2 × RoundInterval | **pass**, asserted | part 1: max 5.0–5.5 s at N = 9 and 17 (bound 10 s); mixed radii: max 4.95–9.97 s against 2 × the round interval at the upload (≥ 10–30 s); latency run (50 ms, N = 9): max 5.9 s |
  | AC-V3 bootstrap ≤ pullsync | **pass** (proxy; literal: a tie at the window's resolution) | join to union R 9.3–9.5 s vs B 9.5–9.9 s (0.97–1.00); N = 9: 9.3 s vs 12.0 s (0.77); nested root 9.3–14.9 s. Literal (long run): R 16m02s, B 16m12s after the join (0.99), both at the same 10-minute window boundary, so the comparison has 10-minute resolution (Follow-ups) |
  | AC-V4 new half ≤ pullsync | **fail in-process** (proxy); literal: tie; **needs testnet** | decrease to new half complete R 8.8–17.8 s vs B 9.4–10.1 s (1.04–1.76 in runs with both): the round lag (0.5–5.1 s), 4 stripes on 3 peers, a reassignment on the moved-root race in 3 of 8 runs. Literal: R 15m58s, B 16m06s after the decrease (0.99, same boundary). At 2^22 the round lag vanishes; T6.3 |
  | AC-B1 metadata ≤ 50 % | **fail at N = 5 and 9, pass at N = 17** (part 1, reported) | R ÷ B 0.65–0.73, 0.57, 0.35 (0.65–0.69 at N = 5 this round); with 50 ms latency at N = 9: 0.60; mixed radii 373–560 B per upload (B 975 B, one run) |
  | AC-B2 bootstrap metadata | **pass**, asserted | 70.2 B per chunk (nested root 70.2–74.8 B) against 1.15 × 70 = 80.5 B, verification < 5 KB against 1 MB; B: 408 B per chunk at N = 5 (R ÷ B 0.17), 816 B at N = 9 (0.09) |
  | AC-B3 bootstrap chunks once | **pass**, asserted | 1.000 × the chunks (B 2.36–2.43 at N = 5, 2.91 at N = 9) |
  | AC-B4 radius decrease | **pass**, asserted | chunk bytes 1.000 × the new half (B 1.75–1.83); old half 0 B per peer (digests equal, no session) |
  | AC-D1 zero duplicates | **pass**, asserted | 0 repeats in rangesync-only, Mixed (R and D nodes), Bootstrap, Radius, Stale stamp; push races 0–2 per run within 1 s; with faults (Churn) 0 repeats ≤ 0 expired claims |
  | AC-D3 dual cross-protocol (informational) | reported (part 1) | 0 at N = 5, 9; 13 at N = 17 (metric counted 4; it undercounts, Follow-ups) |
  | AC-D5 pullsync duplicates (informational) | reported | part 1: 1.1–1.2 repeats per needed delivery (1.4 with 50 ms latency); Bootstrap B 6105–6443 repeats for 4500 chunks at N = 5 (1.4), 8615 at N = 9 (1.9); Radius B 3402–3765 for 4525 (0.8) |
  | AC-K2 symbols per difference | **not measurable as defined** | no difference label on `symbols_per_difference` (part 1); unfiltered medians ≤ 2–3, p95 ≤ 3–4.75, a tail > 5 in some runs (mixed radii mean 2.3–3.6) |
  | AC-K3 no corrupt decode | **pass**, asserted | 0 in every run |
  | AC-R7 CPU ≤ pullsync | **pass** (steady state and bootstrap) | steady R ÷ B 0.52–0.74 (0.65–0.76 at N = 5 this round, 0.57 with 50 ms latency at N = 9); bootstrap 0.53–0.60 at N = 5, 0.66 at N = 9; radius-decrease phase 0.74–1.24 (not an AC-R7 case) |
  | AC-N1 sync rate | **pass**, but for the window's wording | > 0 throughout a bootstrap (min 1.0) and from a new half's first chunk; 0 at every reconciler in steady state (asserted); after a decrease the reconciler reads 0 until its next round (0.5–5.1 s, Follow-ups); back at 0 ~15.8 min after the last catch-up chunk, at the same instant as the puller's: "within the 10-min window" holds for neither (`pkg/rate` reads 0 10–20 min after, Follow-ups) |
  | AC-N2 no early decrease | **pass**, asserted | a joiner with a 1-s reserve worker and a reserve below its threshold kept its radius; its sync rate never read 0 (guard, then the bootstrap) |
  | AC-N3 puller's sequencing | **pass**, asserted | one decrease; none while the rate was > 0 (≥ 10 minutes); long run: the second decrease 2.9 s (R) and 3.7 s (B) after the last positive rate read, within one 10-s worker tick |
  | AC-N5 sample equality (T6.1 part) | **pass**, asserted | the reserve sample of every bootstrap joiner (R, nested R, P) equals a holder's |
  | AC-S3 serves pullsync | **pass**, asserted (part 1) | R and D nodes served offers and chunks to P nodes; `go test ./pkg/pullsync/... ./pkg/puller/...` green |
  | AC-S6 audit | **not measurable** | Phase 5 not done |

  Summary: 17 rows pass (AC-N1 with its window wording to fix), 2 fail (AC-B1 at N = 5 and 9, for
  T6.4's re-basing; AC-V4 in-process, which needs the testnet at 2^22), 2 are reported only (AC-D3,
  AC-D5) and 2 are not measurable here (AC-K2 as defined, AC-S6). Rows T6.1 does not name but
  measured on the way: AC-R1 (index slab 21.0 MB per node), AC-R2 (rebuild 42.0 MB = 2 × slab),
  AC-V5 (in-sync nested peers keep running sessions, Follow-ups).
- **(T6.1 part 2) Runs.** No `-race` (no C compiler). All on this 4-core host, one at a time
  unless said.
  - Default `go test -tags=integration -count=1 ./pkg/node/` twice, the second on the final code:
    both passed, 8m16s and 8m15s.
  - Bootstrap at N = 5: 6 rangesync, 3 Baseline and 4 nested-root runs (development, a
    `-count=3` diagnostic with debug logs, the two default runs), one probe failure in a nested
    run before `checkCaches` retried overlapping sessions; N = 9 once
    (`RANGESYNC_ACCEPTANCE_SCENARIO_SIZES=9`).
  - Radius decrease: 8 rangesync and 4 Baseline runs (one each in the long run, under load), plus
    a first run with a 1-s reserve-worker interval that showed the double decrease (Follow-ups) and
    failed AC-N1 and AC-N3 as it should.
  - Stale stamp: 5 runs of each network, after the first two versions of the check failed (an
    address-index method on pullsync nodes; a rule that counted the unavoidable first fetch).
  - Churn: 3 runs at N = 5 and 1 at N = 9; the sub-case "bootstrap during uploads"
    (`RANGESYNC_ACCEPTANCE_REPRO=1`) once, failing on 2 honest peers scored; the first two versions
    of the scenario, with the joiner during the uploads in the main case, scored 3 the same way.
  - Part 1 with the window (temporary build) at N = 5 and 9, and without at N = 9, once each.
  - Latency: `RANGESYNC_ACCEPTANCE_SIZES=9 RANGESYNC_ACCEPTANCE_LATENCY=50ms`, once, passed.
  - Long: `RANGESYNC_ACCEPTANCE_LONG=1`, Bootstrap, Radius decrease and Stale stamp in three
    processes at once (RSS 1.8, 1.5 and 1.1 GB while idle), once; all passed, in 16.5, 16.6 and
    12 min.
  - `go test ./pkg/node/... ./pkg/pullsync/... ./pkg/puller/... ./pkg/storer/...` (default build)
    passes. Once, alongside the storer tests, `TestRangesyncToggle` (T6.2) failed with "rangesync:
    peer busy"; 30/30 isolated and the rerun of the whole set passed (Follow-ups).
  - `go vet ./pkg/node/...` with and without `-tags=integration`, `GOTOOLCHAIN=go1.26.1 make lint`,
    `GOTOOLCHAIN=go1.26.1 golangci-lint run --build-tags integration ./pkg/node/...` and
    `go build ./...` are clean. Only the four acceptance files were formatted (gofumpt, gci).
- **(T6.1 part 2 fix round 1) AC-B4 in both overlay orders (review M1).** With seed 200 the
  Radius decreaser's overlay is the lowest of the six, so it initiates every session (§8.2) and
  compares its old-half peers' digests over their root, which lies inside its own
  (`compareDigests` with `RangeSummary`, `scheduler.go`): equal, no session, 0 B. In the opposite
  order the old-half peers initiate, and they cannot compare (their root lies inside the
  decreaser's, whose summary covers more: `known` false), so they start a session with it in
  every round they run (the AC-V5 finding).
  - `TestAcceptanceRadius` runs a third network, "rangesync decreaser highest"
    (`radiusOverlay(true)`: the same seed, the decreaser's half made the upper one and the
    decreaser the highest node of it; its new half is the other half, 4475 chunks). Both
    rangesync networks check their order (`Fatalf` if the decreaser is not the lowest, or the
    highest) and assert AC-B4 alike: the new half's chunks once, chunk bytes ≤ 1.01 × the new
    half, and < 1 KB of reconcile bytes with each old-half peer from the release to the end of
    the tail. The table also counts the reconcile streams per old-half peer.
  - Measured (decreaser highest, 8 runs; lowest, 5 runs; see Runs): lowest 0 B and 0 sessions
    per old-half peer, as before; highest **150–200 B in 3–4 sessions per old-half peer**, each one
    fingerprint exchange (~50 B: the root Fingerprint of two equal sets). Chunk bytes 1.000 × the
    new half and 0 repeats in both orders; decrease to the new half complete 10.4–13.7 s
    (highest, the 5 runs with both networks), 11.6–13.9 s (lowest).
  - **Verdict per order: AC-B4 passes in both** on the criterion's bytes (< 1 KB per peer), so no
    known-fail path was needed and none was added. With the decreaser highest it is not "one
    fingerprint exchange per peer" but one per round of each old-half peer, and it does not stop
    with the decrease: ~50 B per old-half peer per round from then on (~100 B per minute at
    `MaxRoundInterval`). That recurring part is AC-V5's idle cost (the nested-root finding,
    "(T6.1 part 2 → T4/T6.3, Low) In-sync peers with nested roots keep running sessions"), not a
    cost of the decrease.
  - The cache probe (`checkCaches`) failed in the new order (3 of 6 nodes in one default run, 1
    node in 2 of 3 isolated runs): it read the node's `parts_sent`, and a session the node had
    under way with the decreaser when a probe attempt began (opened before the attempt, so the
    stream count did not see it) added its Fingerprint part. The check now reads what the node
    answered at the probe (`parts_received`), which counts the probe's own session only; root
    iterations keep the retry. Passed in every run since (6).
- **(T6.1 part 2 fix round 1) AC-B2's two parts asserted apart (review M2).** `checkBootstrap`
  asserted `bytes_metadata{dir="in"}` ≤ 1.15 × count × 70 B + 1 MB: 1.36 MB for 4500 chunks against
  0.32 MB measured, 4.3× room, where the verdict claimed each part against its own bound. It now
  asserts AC-B2's two parts separately, at the rangesync joiners:
  - the IDs, `bytes_metadata{dir="in"}` from the join to the union (the stripes' listings and the
    Status exchanges) ≤ 1.15 × count × 70 B (0.362 MB);
  - the verification, the same from the union to the end of the tail ≤ 1 MB. `VerifyStripes`
    runs once `transferStripes` returns (`stripes_transfer.go`), so in a static network it starts
    after the union; one that starts before the union's snapshot counts in the ID part, which only
    tightens that check.

  Measured: IDs 70.2 B per chunk (0.316 MB, 0.87 of the bound; nested root 70.3 B), verification
  **147–196 B** for 4 stripes (nested root 392 B): 4 verification sessions of about one
  fingerprint exchange each. The table shows both parts and their bounds.
- **(T6.1 part 2 fix round 1) The default run and `RANGESYNC_ACCEPTANCE_FULL` (review M5).** This
  supersedes the runtime numbers of "(T6.1 part 2) Default sizes and the 10-minute package
  timeout" and of the Follow-up "(T6.1 part 2) `make test-integration` and the 10-min timeout,
  updated".
  - Per network, on the default run before this round (package 480 s): part 1 Baseline 38 s,
    rangesync 55 s, Mixed 64 s; Bootstrap Baseline 21 s, rangesync 38 s, nested root 47 s; Radius
    Baseline 31 s, rangesync 83 s (of which the mixed-radii upload phase ~30 s); Stale stamp
    Baseline 7 s, rangesync 37 s; Churn 59 s. The new Radius network adds ~52 s.
  - Without `RANGESYNC_ACCEPTANCE_FULL` (or `RANGESYNC_ACCEPTANCE_LONG`, which implies it,
    `accFull`) the run skips: every Baseline network, part 1's included (they compare; of their
    own they assert only convergence and the pullsync nodes' radius sequencing, today's code);
    the nested-root Bootstrap (a variant of the rangesync Bootstrap, through session-root claims);
    and the mixed-radii upload phase of both Radius networks (AC-V1, AC-D1 and AC-K3 with nested
    roots). Without `FULL` the tables' ratios to the Baseline read "-".
  - Default `go test -tags=integration ./pkg/node/`: package 352 s (the probe failure above) and
    **364 s** (6m06s with the build), against 480–493 s; on the final code (a table-only change
    later) it passed again inside `make test-integration`. The full set,
    `RANGESYNC_ACCEPTANCE_FULL=1 go test -tags=integration -timeout 30m ./pkg/node/`: package
    585 s (9m47s), all passed; it needs the `-timeout`, 15 s under the default 10 min. The `-race`
    command of "(T6.1 part 2) Tests to run with -race on CI" should set
    `RANGESYNC_ACCEPTANCE_FULL=1`, so that it runs what part 2 recorded.
- **(T6.1 part 2 fix round 1) AC-V1 with mixed radii against its bound (review M5).** Correction:
  the check compared each upload with 2 × the smallest round interval of the six nodes at the
  upload; the record's "at least 20–30 s (at least 10 s in one run)" was the smallest such bound
  over a phase. That falls to 2 × `MinRoundInterval` (10 s) as soon as any node's interval has, in
  one of 8 runs, while the slowest upload took 9.94–9.97 s in 6 of 8 runs.
  - The bound is now per upload, as AC-V1 sets it: the upload is on every node of its root once
    the last of them has it, and each takes it in its own rounds at its own interval
    (`round_interval_seconds` is per node), so 2 × `RoundInterval` is set by the slowest of them:
    `accV1Bound` is 2 × the largest round interval at the upload among the nodes whose root holds
    it. That bound is wide here, ≥ 60 s in every run: the decreaser, whose root holds every
    upload, idled at `MaxRoundInterval` (30 s) before the phase.
  - So the check also asserts `accStatusBound`, 2 × `StatusMinInterval` + `MinRoundInterval` =
    15 s, the bound T4.6 part 1 fix round 1 derived for a lacker below every holder (Low 4): 1.5×
    the slowest upload measured, and tighter than the old effective bound in 7 of 8 runs. The
    count within 2 × `MinRoundInterval` and within the old bound stay in the table.
  - FULL run: decreaser lowest, max 9.94 s (at the decreaser; median there 4.69 s), decreaser
    highest, max 5.02 s; 150/150 within both bounds and within 10 s. A decreaser that initiates
    with the holders waits a Status interval and a round (10 s), so AC-V1 at the minimum interval
    stays a knife edge for it; that is T6.4's wording item ("(T4.6 part 1 fix round 1 → T6.4) AC-V1
    against the 15 s bound").
- **(T6.1 part 2 fix round 1) Corrections (review Lows 7 and 8, nits).**
  - AC-N1 and AC-N3 in the Radius decrease are asserted around the reconciler's known lag, so
    "asserted" overstated them. AC-N1's minimum is taken from the new half's first chunk
    (`w.span(firstAt, …)`), not from the decrease, and the rate reads 0 before it (0.5–5.1 s);
    the decreaser's reserve worker ticks every 10 s (`accRadiusWakeUp`), chosen to outlast that
    lag, so the default AC-N3 check (one decrease in the run) cannot catch the double decrease a
    1-s worker showed. AC-N3 to the letter (the second decrease only after the last positive rate
    read) ran once, in the long run. Bootstrap's AC-N1 and AC-N2 (from the join, with a 1-s worker)
    stand as asserted.
  - AC-V5 is not "measured on the way" with mixed radii: it fails there (idle sessions every round
    between in-sync nested-root peers), and the summary below lists it so.
  - "(T6.1 part 2 → T4/T6.4) Stripe plans load peers unevenly…": "R ÷ B 0.88–1.76 at 3 peers"
    reads 1.04–1.76, as in the Radius results (the 4 runs with both networks).
  - AC-B2's nested-root number: the verdict table's "70.2–74.8 B" has no source in the Bootstrap
    results (70.3 B); this round measured 70.3 B again. The corrected row below uses them.
  - AC-V4 this round: no stripe was reassigned in the 10 rangesync Radius networks whose tables were
    logged, and the decrease still took R ÷ B 1.20 (lowest) and 1.27 (highest) in the FULL run
    (B 10.4 s): the round lag and 4 stripes on 3 peers, as recorded.
- **(T6.1 part 2 fix round 1) Acceptance-criteria verdicts, corrected.** This supersedes the rows
  of "(T6.1 part 2) Acceptance-criteria verdicts" listed here, and its summary; the other rows
  stand.

  | Row | Verdict | Numbers |
  |---|---|---|
  | AC-V1 2 × RoundInterval | **pass**, asserted (mixed radii: with `FULL`) | part 1 as recorded (bound 10 s); mixed radii max 4.95–9.97 s (this round 5.02 and 9.94 s, decreaser highest 5.02 s) against 2 × the largest round interval at the upload of the root's nodes (≥ 60 s) and the 15-s Status bound, both asserted; the slowest within 2 × `MinRoundInterval` (10 s) by 30–60 ms in 7 of 10 runs |
  | AC-V4 new half ≤ pullsync | **fail in-process** (proxy); literal: tie; **needs testnet**, where the moved-root race (Medium, Follow-ups) makes it fail in the decreases the race hits | R 8.8–17.8 s vs B 9.4–10.1 s (1.04–1.76) in part 2; this round 10.4–13.9 s with no race, R ÷ B 1.20 and 1.27 (B 10.4 s) |
  | AC-B2 bootstrap metadata | **pass**, asserted (its two parts apart) | IDs 70.2 B per chunk (nested root 70.3 B) against 1.15 × 70 = 80.5 B; verification 147–196 B (nested root 392 B) against 1 MB; B: 408 B per chunk at N = 5 (R ÷ B 0.17), 816 B at N = 9 (0.09) |
  | AC-B4 radius decrease | **pass**, asserted in both overlay orders | decreaser lowest: chunk bytes 1.000 × the new half (B 1.75–1.88), old half 0 B per peer (no session); decreaser highest: 1.000, old half 150–200 B per peer (3–4 sessions of one fingerprint exchange); the latter recur once per round of each old-half peer after the decrease (AC-V5) |
  | AC-N1 sync rate | **pass**, but for the window's wording; in Radius only from the new half's first chunk | Bootstrap > 0 from the join (min 1.0, asserted); Radius > 0 from the first chunk (asserted), 0 for 0.5–5.1 s after the decrease (lag, Follow-ups); steady state 0 (asserted); back at 0 ~16 min after the last catch-up chunk, as the puller |
  | AC-N3 puller's sequencing | **pass to the letter in the long run only** (once); the default run asserts one decrease with a 10-s worker that outlasts the lag | long run: the second decrease 2.9 s (R) and 3.7 s (B) after the last positive rate read; a 1-s worker decreased twice, 1 s apart (lag, Follow-ups) |
  | AC-V5 idle, no sessions (not a T6.1 row) | **fail with mixed radii**; holds with one radius | in-sync peers with nested roots run a session per round: idle 8.0 per minute (nested-root joiner, 4 peers) and 3.4 (decreaser, 5 peers) in the long run; 3–4 per old-half peer in the ~25 s after a decrease with the decreaser highest; same-root peers 0 |

  Summary, corrected: of the rows T6.1 names, 17 pass (AC-N1 with its window wording to fix and,
  in Radius, from the first chunk; AC-N3 to the letter in the long run only), 2 fail (AC-B1 at
  N = 5 and 9, for T6.4's re-basing; AC-V4 in-process, which needs the testnet, where the moved-root
  race makes it fail in the decreases it hits), 2 are reported only (AC-D3, AC-D5) and 2 are not
  measurable here (AC-K2 as defined, AC-S6). Rows T6.1 does not name, measured on the way: AC-R1
  and AC-R2 pass (index slab 21.0 MB per node, rebuild 42.0 MB = 2 × slab); **AC-V5 fails** with
  mixed radii.
- **(T6.1 part 2 fix round 1) Production code.** None changed. Test and record changes only:
  `rangesync_acceptance_scenarios_test.go` (the Radius orientations, the AC-B2 split, the AC-V1
  bounds, `accFull`), `rangesync_acceptance_test.go` (part 1's Baseline behind `accFull`) and
  `rangesync_acceptance_harness_test.go` (the probe reads `parts_received`).
- **(T6.1 part 2 fix round 1) Runs.** No `-race` (no C compiler). All on this 4-core host, one at a
  time.
  - Default `go test -tags=integration -count=1 ./pkg/node/` before the changes (the breakdown
    above): 480 s, passed. After: 352 s (the probe failure in the new order), then 364 s, passed.
  - `TestAcceptanceRadius` alone with the new order: once with both networks (passed), 3 runs of
    the new order with a debug log of the probe (2 failed the probe, the diagnosis above), 3 runs
    after the fix (passed).
  - `RANGESYNC_ACCEPTANCE_FULL=1 go test -tags=integration -count=1 -timeout 30m ./pkg/node/`,
    once: passed, package 585 s.
  - `make test-integration` once, on the final code: 7m20s; `pkg/node` passed (393 s, alongside the
    other packages). The target failed on two packages outside this plan, unchanged on this branch
    (last touched by a74befda) and so failing on the base as well: `pkg/jsonhttp`
    `TestPanicRespond` (it fails the same way in the default build with this host's go1.27.0: it
    expects `json.UnsupportedTypeError`, the encoder returns `json.UnsupportedValueError`) and
    `pkg/resolver/client/ens` (`ens_integration_test.go:82`: undefined `testutil`, a build
    failure under the tag). Follow-ups.
  - `go test ./pkg/node/... ./pkg/pullsync/... ./pkg/puller/... ./pkg/storer/...` (default build)
    passes. `go vet ./pkg/node/...` with and without `-tags=integration`,
    `GOTOOLCHAIN=go1.26.1 make lint`, `GOTOOLCHAIN=go1.26.1 golangci-lint run --build-tags
    integration ./pkg/node/...` and `go build ./...` are clean. Only the three edited files were
    formatted (gofumpt, gci).
- **(Orchestrator, T6.1 part 2 landing, 2026-10-22) Part 2 landed after one fix round; T6.1 done.**
  Tests only (pkg/node, `-tags=integration`). Review: no High; five Mediums and three Lows. The
  test-side ones (AC-B4's overlay order, AC-B2's split assertion, the runtime margin and AC-V1's
  bound, the verdict wording) were fixed in fix round 1, which verified clean (no re-review: no
  High). The two production findings ranked Medium (the stripe worker racing the moved-root
  Status; stripe verification scoring honest deliverers while uploads land in their stripes) are
  NOT fixed here (T6.1 is tests only). They are recorded to be fixed or explicitly accepted before
  T6.3's testnet comparison runs, a decision for the user at G7. The fix round's verifier: default
  integration run 357–407 s; `RANGESYNC_ACCEPTANCE_FULL=1` 602 s (needs `-timeout`, Follow-ups).
  `make test-integration` fails outside the plan on the base too: `pkg/jsonhttp`
  `TestPanicRespond` under the host's go1.27.0 (passes under go1.26.1) and the `ens` integration
  test, which does not compile (`undefined: testutil`).
- **(T6.5 part 1) Stripe sessions wait for this node's root to reach their peer (fixes "(T6.1 part
  2 → T4, Low) Stripe sessions race the moved-root Status", ranked Medium by "(T6.1 part 2 fix
  round 1 → before T6.3)").** As built (`stripes_transfer.go`):
  - Before each session a stripe attempt opens (transfers, restarts, reassignments and
    verifications alike, whatever moved the root), `reachedRoot(peer)` must hold: the peer's
    `peerState.reached` (the root of the last Status of this node that reached it) equals
    `LocalStatus().Root`, the condition round sessions use (`scheduleLocked`, `recheckSession`).
    `handleStatus` stores a Status before its `FullClose`, so `reached` means the peer holds it.
  - A peer that lacks it is not asked: the attempt waits as for the peer's Busy
    (`errRootNotReached` in `stripeBusy`), as "(T4.5 part 2c fix round 2, Low 1) An open backoff is
    the peer's Busy for stripes" made the open backoff: off the stripe clock (unless a fetch of the
    attempt runs), without its slots, counted in `maxStripeBusyRetries` (30, about 30 s) with the
    peer's Busy answers and its open backoff. It asks again after `stripeRetryInterval`, or at once
    when a Status of this node reaches the peer (`peerState.rootWake`, closed by `probe`, as
    `st.idle` wakes the stripes waiting for a round session): with the 1-s poll alone, the first run
    of the harness after the fix took the new half's first chunk 1 s later (6.05 s against 5.05 s
    after the decrease) in 3 of 5 runs of one orientation. Past it the attempt fails without a session (`ran` false, so the peer may still verify
    the stripe) and `runStripe` reassigns the stripe, unscored.
  - Not checked: a peer the reconciler keeps no entry for (`TransferStripes` called directly), an
    Unsupported one, and a `LocalStatus` that fails (the session reports it).

  Decision: the wait is the peer's Busy, not a cause on this node. A local cause runs on the stripe
  clock and restarts with the same peer (up to `maxStripeRestarts` × `StripeTimeout`), so a peer
  that never takes this node's Status (it resets the Status stream, or stalls it to the timeout)
  would hold the stripe for hours. 30 s covers the gossip: a moved root is due at once, or a few
  seconds later under the sender-side rate (`PeerStatusInterval`, 2.5 s, per Status past
  `statusBurst` back to back), and a failed send is retried after 5 s, then 10 s. `planStripes`
  still plans on such a peer (it is in `ready`): a wait of seconds costs less than the imbalance a
  re-plan would cause, which is what made the race expensive (one peer serving 3 of 4 stripes).

  Not built: the finding's "keep the peer instead of reassigning a stripe whose session the peer
  refused for a stale root". The refusal is a stream reset, which the frozen wire does not tell
  apart from any other reset; the check closes the window before the session opens instead. What
  is left of the race: a root that moves again between the check and the session's
  `LocalStatus` (two radius changes within milliseconds), which ends as before (a reassignment).
  Kept: own jobs and claims (preemption, adoption, caps), the stripe inactivity clock (the wait is
  off it), the SyncRate holds (the transfer holds `catchUpFetches` throughout; the own-job grace,
  `radiusHold` and `claimIdleTimeout` all outlast 30 s), the fetch credit (no fetch is made). No
  wire change. Superseded: nothing written; T3.4's "newHalf is registered ... through the stripe
  worker as soon as a peer with a Status covers it" still holds, its stripe sessions now wait for
  the root as round sessions do.
- **(T6.5 part 1) Stripe verification judges the deliverer on the chunks stamped before its listing
  (fixes "(T6.1 part 2 → T4/T6.4, finding) Stripe verification scores honest deliverers when uploads
  land in their stripes", ranked Medium by "(T6.1 part 2 fix round 1 → before T6.3)").** As built:
  - `rangesync.FetchResult.StampTimes` (additive API, no wire change): the stamp timestamp of each
    chunk stored, in the order stored (`deliver`). The reconciler carries it in `fetchOutcome`.
  - `StripeResult.ListedAt`: when the session of the deliverer's last attempt opened, on the
    reconciler's clock (`transferStripe` sets it before each `ReconcileWith`; `runStripe` keeps the
    last attempt's). The deliverer is accountable for what it held then: the last attempt lists all
    it holds of the stripe that this node lacks, whatever earlier attempts delivered.
  - `verifyStripe` passes `ListedAt − verifyStampSlack` (10 min) to the verifier's attempt, which
    counts `VerifyResult.Newer`: the stored chunks whose stamps are at or after it and not more than
    `verifyStampSlack` ahead of this node's clock when stored. The deliverer is scored
    (`ErrStripeVerification`) when `Stored − Newer > max(verifyMinDiff, PeerStored/verifyDiffRatio)`
    (tolerance unchanged). A stored chunk whose stamp time the Service does not report counts (as
    before: the fakes without `FetchWithResult`). Unchanged: the verifier's own check
    (`Undelivered` above its tolerance), the `Tried` exclusion, `verification_diff` (it still
    observes `Stored`, all the verification found).

  Why stamps, not a tolerance scaled by the upload rate (the finding's two directions). The stamp
  of a chunk the verification stores is signed by its batch owner and bound to the ID the verifier
  listed (the ID holds the stamp's hash), so neither the deliverer nor the verifier can move it; the
  deliverer cannot move `ListedAt` earlier either (a restart only moves it later, which makes more of
  what it withheld count). A rate-scaled tolerance would need an upload rate from evidence the
  deliverer cannot influence, which leaves the peers' Status counts: debounced to 5 s, noisy over
  the seconds of the suite's transfers, lowered by evictions, and per root rather than per stripe;
  and it would give a withholding deliverer a free budget of *any* chunks, old ones included, as
  large as the expected uploads (at heavy load a large one). With stamps only recent chunks are
  excused. The slack covers an upload's spread through the neighborhood (up to (`takeoverRounds` +
  1) × `MaxRoundInterval` = 90 s for a peer that takes over), the push's latency and clock skew of
  minutes; the upper bound (a stamp more than the slack ahead of the clock is no evidence of a late
  upload) keeps a colluding uploader from shielding chunks by dating them in the future.

  What an attacker can still do:
  - a deliverer can leave out unscored the chunks stamped less than `verifyStampSlack` before its
    listing opened (at the reference load of 15 uploads/s per neighborhood ~9,000, ~2,250 per
    stripe of 4: 0.2 % of a 2^22/4 stripe), those that reach it while it lists, and, as before,
    max(4, 1 %) of any. The verification still fetches all of them from the verifier: only the
    deterrent is lost;
  - a colluding uploader gains nothing by stamping honest chunks anew: a second stamp of an address
    is another ID, and the original keeps its stamp; future-dated stamps count;
  - framing an honest deliverer still costs what it did: upload into its stripe during its transfer
    more chunks than the tolerance (addresses are cheap to mine into a stripe), now with stamps
    backdated by more than 10 min (free for the batch owner). The deliverer's score rises by one
    (deprioritized above `ScoreThreshold`, decays one per 10 min); no disconnect;
  - a colluding verifier still shields a deliverer (lists nothing), as before.

  False positives left: honest chunks stamped more than 10 min before the listing that reach the
  neighborhood after it (deferred uploads pushed late, a pusher backlog, re-uploads with their old
  stamps) and a node whose clock runs more than 10 min ahead; both fall under the 1 % tolerance in
  ordinary operation. In tests whose stamps are random (`postagetesting.MustNewStamp`, the sims'
  `randBytes`) or whose reconciler runs on a fake clock, `Newer` is arbitrary; no such test checks a
  deliverer's score.

  Superseded: T3.3's "Scoring" ("the tolerance absorbs uploads that arrived during the transfer")
  and "Verification blame basis" ("The deliverer is judged on the chunks the verifier delivered"),
  and T4.4's "The deliverer check (verifier-Stored above the tolerance) is unchanged": the
  deliverer is now judged on the verifier's stored chunks stamped before `ListedAt` −
  `verifyStampSlack` (and not more than it ahead of the clock). T3.3's "Tolerance basis"
  consequence ("a deliverer can withhold up to max(4, 1%) of its stripe per bootstrap unscored")
  grows by the recent chunks above.
- **(T6.5 part 1) Results and runs.** No `-race` (no C compiler); one test process at a time on
  this 4-core host.
  - Regression tests, each failing before its fix (run against the old code by reverting the
    production lines only): `TestRadiusStripeWaitsForMovedRoot` ("2 sessions opened with higher
    before it had the new root", then reassigned), and without the wake-up it times out (the fake
    clock does not move); `TestVerifyStripesUploadsDuringTransfer` (the deliverer scored 1 in the
    four cases whose chunks are newer); `TestTransferStripesListedAt`; `TestFetchStampTimes`.
  - `TestAcceptanceRadius`, 5 runs × 2 rangesync orientations (default, no Baseline), before and
    after:

    | | before (base code) | after (final code) |
    |---|---|---|
    | runs with a stripe reassigned | 2 of 10 (both decreaser lowest), 1 each | 0 of 10 |
    | decrease to first chunk | 3.2–5.1 s | 3.0–5.1 s |
    | decrease to the new half complete | 11.6–17.7 s (17.7 s in the 2 race runs, one peer 15 MB) | 11.4–14.2 s |
    | peer failures; disconnects | 0; 0 | 0; 0 |

    An intermediate version without the wake-up (the 1-s poll only), 5 runs: 0 of 10 reassigned,
    but the first chunk 6.05 s instead of 5.05 s in 3 of 5 runs of the decreaser-lowest
    orientation, which the wake-up removed.
  - `TestAcceptanceChurn/N=5/bootstrap_during_uploads`: on the base code with
    `RANGESYNC_ACCEPTANCE_REPRO=1`, 2 of 2 runs failed ("honest peers scored": 2 and 1 peer
    failures); after, unskipped, 3 of 3 runs passed with 0 peer failures, 0 disconnects (joiner's
    base 9.5–10.9 s after it joined, `stripes_total` done 4). The sibling case "leave rejoin and
    stripe peer" passed in the same 3 runs (reassigned 1 each, as it asserts; 0 peer failures).
  - AC-B2 (`-tags=sim`, `TestG6Bootstrap$|TestG6BootstrapTail`, 670 s): identical to T4.7 part 4's
    record, within bound: `TestG6Bootstrap` 1410 and 1409 KB of 2610 KB (16 stripes done, none
    reassigned), `TestG6BootstrapTail` 1232 / 1263 / 1326 KB of 2288 KB (1.10 / 1.13 / 1.18 ×),
    caught up after 5333.93–5335.13 s, no stripe timed out.
  - Default `go test -tags=integration -count=1 ./pkg/node/`: passed, package 507 s (wall 8m29s),
    up from 357–407 s at T6.1 part 2's landing: the unskipped sub-case adds 54–81 s (its setup
    varies, 16–54 s). Under the 10-min default timeout, with ~90 s of margin (Follow-ups).
  - `RANGESYNC_ACCEPTANCE_FULL=1 go test -tags=integration -count=1 -timeout 30m -run
    'TestAcceptanceRadius|TestAcceptanceChurn' ./pkg/node/`, once: passed, package 313 s. Radius:
    Baseline new half 9.3 s, rangesync 13.4 s and 12.5 s (decreaser highest), `stripes_total` done
    4 with none reassigned in both orientations, mixed-radii uploads max 9.97 s and 4.96 s (within
    the bounds), 0 peer failures; Churn: both sub-cases passed, 0 peer failures.
  - `go test ./pkg/rangesync/...` (default build), `go test ./pkg/rangesync/reconciler/
    -count=3 -cpu=1,2,4` and `go test ./pkg/pullsync/... ./pkg/puller/... ./pkg/storer/...` pass;
    `go build ./...` is clean; `go vet` on `./pkg/rangesync/...` (default and `-tags=sim`) and
    `-tags=integration ./pkg/node/` clean; `GOTOOLCHAIN=go1.26.1 make lint` and `golangci-lint run`
    with `--build-tags sim ./pkg/rangesync/...` and `--build-tags integration ./pkg/node/...`: 0
    issues. Only the edited files were formatted (gofumpt, gci). No `pkg/pullsync`, `pkg/puller`,
    `pkg/storer`, `go.mod` or proto change; no new parser of peer input (no `followup-fuzzing.md`
    line).

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
- **(T6.1 part 1 fix round 1 → T6.3, T6.4) The dual cohort's duplicate formula, corrected.** The
  formula at the end of "(T6.1 part 1 → T6.3, T6.4) `cross_protocol_duplicates_total` undercounts
  on dual nodes" is wrong. `bee_rangesync_fetch_delivered_total` counts only fetched chunks that
  were stored (`deliver` in `pkg/rangesync/fetch.go`: `Delivered.Inc()` after the `Put`). A
  delivery that finds the chunk in the reserve already goes only to
  `bee_rangesync_cross_protocol_duplicates_total`, and pullsync's `bee_pullsync_chunks_delivered`
  counts every chunk it puts, stored before or not. The dual cohort's exact number of duplicate
  deliveries is therefore `bee_pullsync_chunks_delivered` + `bee_rangesync_fetch_delivered_total`
  + `bee_rangesync_cross_protocol_duplicates_total` minus the unique chunks added. Deliveries whose
  stamp was rejected (`bee_rangesync_fetch_rejected_total`, or an older stamp than the reserve's)
  are in neither term. T6.3's AC-D3 uses this formula.
- **(T6.1 part 1 fix round 1 → part 2) Harness notes on the transport, consolidated.** These replace
  the last bullet of "(T6.1 part 1 → part 2) Harness notes …" (latency).
  - **Bootstrap needs a bounded stream window.** accNet gives every stream an unbounded buffer per
    direction, so a writer never waits for its reader. On libp2p a yamux stream starts with a
    256 KiB receive window (go-yamux `initialStreamWindow`; go-libp2p lets it grow up to 16 MiB).
    In steady state a page or a Want carries far less, but a bootstrap moves a whole reserve, and
    its throughput, and the two protocols' relative times (AC-V3), depend on that window. Bootstrap
    should run with a bounded per-stream window in accNet (256 KiB: the writer blocks until the
    reader drains).
  - **Latency 0 makes three kinds of numbers unrepresentative.** Every part-1 and fix-round run used
    `Latency` 0:
    - upload latency: pullsync's 0.25 s median has no round trip in it, and rangesync's numbers are
      set by its rounds alone;
    - push races: a fetch completes within tens of milliseconds of its claim's re-check, so races
      are rare (3 in 29 part-1 runs); with a round trip of 50–100 ms they become more frequent, and
      `accPushRaceWindow` (1 s) must stay well above the measured fetch latency, which the suite
      reports in every run;
    - pullsync duplicates: racing offers from the p holders arrive within one page of each other.

    Run at least one size with a one-way latency (`accOptions.Latency`, for example 50 ms) and
    re-check `accPushRaceWindow` against the fetch latency it reports.
- **(T6.1 part 1 fix round 1) Tests to run with -race on CI.** No change to the "(T6.1 part 1)" list:
  the new `TestAccDuplicateClassification` checks a pure function.
- **(T6.1 part 2 → T4/T6.4, finding) Stripe verification scores honest deliverers when uploads
  land in their stripes.** `verifyStripes` judges a deliverer on the chunks the verifier delivered
  in its stripe, with a tolerance of max(`verifyMinDiff` 4, `PeerStored`/100). Chunks uploaded
  into the stripe after the deliverer listed it count against it. With a bootstrap during the
  reference load (`TestAcceptanceChurn/N=5/bootstrap_during_uploads`,
  `RANGESYNC_ACCEPTANCE_REPRO=1`: 15 uploads/s, an empty node joins at 5 s) the verifications
  found 15–44 chunks per ~1125-chunk stripe and scored 2–3 honest holders
  (`ErrStripeVerification`, "stripe incomplete at verification"); without the uploads (the
  default Churn case, with a stripe peer leaving mid-transfer) none. That the difference is the
  uploads is inferred: it comes only with them, and 15–44 per stripe matches 15/s over 4 stripes
  for the 4–12 s between a stripe's listing and its verification. At the node's scale the
  tolerance holds while uploads into the root during the bootstrap stay below ~1 % of the
  reserve: a 2^22 bootstrap at 1000 chunks/s takes ~70 min, so the bound is ~10 uploads/s into
  the neighborhood. A fail raises the score (deprioritized above `ScoreThreshold` 3, decays one
  per 10 min), it does not disconnect. Possible directions (production change, not done here):
  count only the verifier's chunks whose stamps are older than the stripe's listing, or scale the
  tolerance with the transfer's duration and the root's upload rate. T6.3 should watch
  `bee_rangesync_peer_failures_total` on bootstrapping nodes.
- **(T6.1 part 2 → T4, latent) The reconciler's sync rate lags a radius decrease.** Between the
  reserve's `SetRadius` and the round that tracks the new root (`trackRootLocked`: the index
  rebuild, then a round at most `MinRoundInterval` after the last) the radius hold is not armed and
  `SyncRate` reads 0 (0.5–5.1 s in the suite). A reserve worker that ticks faster lowered the radius a
  second time 1.0 s after the first (`TestAcceptanceRadius` with `accRadiusWakeUp` = 1 s: radii
  2 > 1 > 0). Unreachable at the node's 15-minute `reserveWakeUpDuration`; the puller's rate rises
  with its first page (~0.1 s). A fix would arm the hold when the reserve's radius drops (the
  `ReserveObserver` already sees `RadiusChanged`), not at the next round.
- **(T6.1 part 2 → T4, Low) Stripe sessions race the moved-root Status.** After a radius
  decrease the round registers the new half at once, and its stripe sessions can open before the
  stripe peer has this node's Status with the new root. The peer then cannot build a scope
  (`NewScope` of a disjoint stale root) and resets the stream, unscored by design; the attempt has
  no progress and the stripe is reassigned. Round sessions wait for `reached` (scheduler.go); the
  stripe path does not check it. Seen in 3 of 8 Radius runs (1–2 reassignments, debug log
  "stripe reassigned … reconcile: read: … stream reset" right after "radius decreased"); one peer
  then serves 3 of the 4 stripes and the new half takes 13.6–17.8 s against 8.8–13.3 s without. A
  fix would make the stripe worker wait for `reached` as rounds do.
- **(T6.1 part 2 → T4/T6.3, Low) In-sync peers with nested roots keep running sessions.**
  `compareDigests` cannot compare a Status whose root contains this node's root (the peer's
  summary covers more than the session root), so the deeper node counts the digests as different
  and runs a session with the shallower peer every round it initiates, or at the takeover cadence
  otherwise. Idle long runs measured 8.0 sessions per minute for a radius-1 node with 4 radius-2
  peers (Bootstrap, nested root) and 3.4 per minute for the Radius decreaser with 5 peers, each
  finding nothing, while same-root peers ran none. AC-V5 ("no sessions" when idle) does not hold
  with mixed radii; T6.3 should report `sessions_total` per cohort with mixed radii. A direction:
  the shallower peer's Status (or a reply) carrying the summary of the deeper root.
- **(T6.1 part 2 → T4/T6.4) Stripe plans load peers unevenly when their number is not a power
  of 2.** `PlanStripes` makes 2^s ≥ k stripes and gives stripe i to peer i mod k: 3 covering
  peers get 4 stripes, one serving two at the holder's 250 chunks/s, so the new half takes half
  of it ÷ 250/s instead of a third. With 9–15 peers (16 stripes) the slowest peer has 2/16
  against an even 1/k (1.1–1.9×). Together with the round lag this made AC-V4 fail in-process
  (R ÷ B 0.88–1.76 at 3 peers). At 2^22 with ≥ 4 peers the node's 1000/s fetch limit binds
  first, so T6.3 decides whether it matters.
- **(T6.1 part 2 → T6.4) Criteria wording.**
  - AC-N1 "returns to 0 within the 10-min window": `pkg/rate` (and the reconciler's copy) sums
    the current and the interpolated previous 10-minute bucket, aligned to multiples of 10 minutes
    of Unix time, so a rate reads 0 from 10 to 20 minutes after its last chunk. Both protocols
    took ~16 minutes in the long runs. Suggested: "within 2 × the window", or "no later than the
    puller's".
  - AC-V3 and AC-V4 compare the times to `sync_rate == 0`: two nodes that finish in the same
    10-minute bucket read 0 at the same instant (the long runs: R 16m02s, P 16m12s, nested 16m08s
    after the join; Radius R 15m58s, P 16m06s after the decrease). Compare the time to the union
    (the last catch-up chunk stored), as the suite does, and keep `sync_rate` as the testnet
    signal.
- **(T6.1 part 2 → proposal §7.7) Pullsync does learn the newer stamp in the two-push case.**
  "Today A never learns about s2" did not reproduce: B's `Put` of the newer stamp gets a new bin
  ID, so A's live sync is offered it and replaces the older one. Both protocols converge there;
  what rangesync needs on top is the rejected cache, so that its rounds do not refetch an older
  stamp that a peer still lists (AC-C3).
- **(T6.1 part 2) `accPushRaceWindow` and stripe fetches.** A Want of 250 IDs at the holder's
  250 chunks/s takes a second, so the fetch latency in a striped transfer reaches 0.9–1.0 s
  (Churn). No scenario pushes during a striped transfer (Churn's sub-case that does is skipped);
  one that does needs the window above the stripe fetch latency, or races per fetch.
- **(T6.1 part 2) Tests to run with -race on CI.** Append to the "(T6.1 part 1)" list:
  `TestAcceptanceNetWindow`, `TestAcceptanceBootstrap`, `TestAcceptanceRadius`,
  `TestAcceptanceStaleStamp` and `TestAcceptanceChurn` (`pkg/node`, `-tags=integration`). They run
  late joins, leaves and rejoins (connections torn down under running sessions, fetches and stripe
  workers), the bounded window's waits, the stale peer's service and the reserve workers' radius
  decreases, none of it ever under `-race` here (no C compiler). Suggested:
  `go test -race -tags=integration -run 'TestAcceptance' -timeout 60m ./pkg/node/` (default
  sizes; part 2 adds ~5 min without `-race`).
- **(T6.1 part 2) `make test-integration` and the 10-min timeout, updated.** The default
  `go test -tags=integration ./pkg/node/` now runs part 1 at N = 5 and the part-2 scenarios at
  their default sizes: 8m15s on this host (package 493 s: part 1 at N = 5 156 s, Bootstrap 108 s, Radius decrease 118 s, Stale stamp 49 s, Churn 62 s), against 5.9–6.8 min before part 2. `RANGESYNC_ACCEPTANCE_SIZES=5,9` (part 1's former default) adds
  ~3.5 min and needs a `-timeout`.
- **(T6.1 part 2) A load-induced flake in `TestRangesyncToggle`** (T6.2, `pkg/node`, default
  build): "rangesync: peer busy" at `rangesync_toggle_test.go:475` once, in
  `go test ./pkg/node/... ./pkg/pullsync/... ./pkg/puller/... ./pkg/storer/...` with the storer
  tests running alongside; 30/30 isolated and the rerun of the set passed. The served check right
  after a start can meet the index still being built (Busy). Watch it on CI; if it recurs, retry
  the check on Busy.
- **(T6.1 part 2 fix round 1 → before T6.3) Two findings rank Medium: fix them, or accept them
  explicitly, before T6.3's testnet comparison runs.** This supersedes the Low rank of "(T6.1 part
  2 → T4, Low) Stripe sessions race the moved-root Status" and gives "(T6.1 part 2 → T4/T6.4,
  finding) Stripe verification scores honest deliverers when uploads land in their stripes" a
  rank. Both are production changes in `pkg/rangesync/reconciler`; this round changed no code.
  - **Stripe sessions race the moved-root Status (Medium).** When the race hits, the stripes of
    the peers that did not yet have the decreaser's new root are reassigned, and one peer serves
    3 of the 4 stripes at the holder's 250 chunks/s (`handleMaxChunksPerSecond`, `fetch.go`): 3/4
    of the new half ÷ 250/s, against the puller's 3 peers × 250/s with its 1.8× duplicates (the
    new half × 1.8 ÷ 750/s), ≈ 1.25× slower with 3 covering peers. Nothing in it shrinks with the
    reserve: the Status debounce and the round are fixed times, unlike the round lag that 2^22
    makes negligible. So AC-V4 fails on testnet in the decreases the race hits (3 of 8 here,
    ~3/8), and T6.3's radius change would measure the race, not the protocol. Fix direction
    (small): the stripe worker waits until the stripe peer's `reached` equals this node's current
    root before it opens a stripe session, as `scheduleLocked` does for round sessions, and keeps
    the peer instead of reassigning a stripe whose session the peer refused for a stale root.
  - **Stripe verification scores honest deliverers under uploads (Medium).** It does not depend on
    scale: the tolerance is 1 % of the stripe and the transfer takes the stripe ÷ the fetch rate,
    so the uploads that land in a stripe during its transfer stay below the tolerance only while
    the neighborhood takes fewer than ~1 % of the fetch rate, ~10 uploads/s (the finding's own
    derivation, at any reserve size). T6.3 runs at the reference load (15 uploads/s per
    neighborhood in the suite), so every bootstrap and striped catch-up there scores honest stripe
    peers: `bee_rangesync_peer_failures_total`, which T6.3 is to watch to tell withholding from
    honest service, fills with false positives, and honest peers' scores rise
    (`ErrStripeVerification`, deprioritized above `ScoreThreshold`). Fix directions (from the
    finding): count against the deliverer only the verifier's chunks whose stamps are older than
    the stripe's listing, or scale the tolerance with the transfer's duration × the root's upload
    rate.
- **(T6.1 part 2 fix round 1 → T6.3 or a later harness run) A large session decode on the real
  clock is not exercised (review Low 6).** `accLargeDiff` (128) stands for `LargeDiff` (65536) in
  the bootstrap only: `IsBootstrap` ignores `LargeDiff`, and the ratios differ (128 is ~1/35 of a
  4500-chunk reserve, 65536 is 1/64 of 2^22). Every catch-up between 128 and 65536 IDs, which the
  node decodes in a session (a node back after minutes away, a peer that missed a burst), is
  striped in the part-2 scenarios instead; Churn keeps its leaver below 128 (~75 missed) on
  purpose. So no part-2 scenario runs a session that decodes thousands of IDs on the real clock
  (`SessionSymbolBudget`, `More` requests, the root cache under a large difference, one session's
  fetch of thousands of IDs at 250 chunks/s); T4.6 part 3 did it on a simulated clock only. T6.3
  should time one (a node of a testnet cohort stopped for some minutes under the reference load),
  or a later harness run should: Churn with `accOptions.LargeDiff` left at 0 (the node's 65536)
  and a leaver away long enough to miss a few thousand uploads.
- **(T6.1 part 2 fix round 1) `make test-integration` needs a `-timeout`.** It runs
  `go test -tags=integration -v ./...` with `go test`'s default 10-min timeout per package binary.
  `pkg/node`'s default run is ~6 min now (fix round 1), but `RANGESYNC_ACCEPTANCE_FULL`,
  `RANGESYNC_ACCEPTANCE_SIZES=5,9,17`, `RANGESYNC_ACCEPTANCE_LONG` or `-race` take it past 10 min,
  and a slower runner may too. The Makefile is outside this plan's files: add
  `-timeout $(TEST_TIMEOUT)` (30m, as `test-race` uses) to `test-integration`. CI (`go.yml`) does
  not run the integration tag, so nothing runs this suite automatically yet (see "Tests to run
  with -race on CI"). The target also fails today on two packages outside this plan (Decisions
  log, "(T6.1 part 2 fix round 1) Runs"): `pkg/jsonhttp` `TestPanicRespond` under go1.27 and the
  `pkg/resolver/client/ens` integration test, which does not compile (undefined `testutil`); both
  need an owner before `make test-integration` can gate anything.
- **(T6.5 part 1) Tests to run with -race on CI.** They ran with `-count=3 -cpu=1,2,4` (no C
  compiler here). `pkg/rangesync/reconciler`: `TestRadiusStripeWaitsForMovedRoot`,
  `TestVerifyStripesUploadsDuringTransfer`, `TestTransferStripesListedAt` (new) and
  `TestRadiusDecreaseThenIncreaseCancels` (changed: it moves the fake clock so that the fourth
  root sent back to back goes out). `pkg/rangesync`: `TestFetchStampTimes`. `pkg/node`
  (`-tags=integration`): `TestAcceptanceChurn/N=5/bootstrap_during_uploads`, which now runs by
  default (append it to the "(T6.1 part 2)" list).
- **(T6.5 part 1 → T6.5 part 2 / T6.3) Tell uploads from withholding on the dashboards.**
  `verification_diff` still observes everything a verification stored, uploads since the listing
  included, so on a bootstrapping node under load it is not a withholding signal; only
  `peer_failures_total` (with part 2's reason label, `ErrStripeVerification`) is. A histogram of
  `VerifyResult.Newer`, or of `Stored − Newer`, would show both; not added here (part 2 owns the
  metrics). T6.3 should expect no `ErrStripeVerification` on honest bootstraps under the reference
  load; one is either withholding or one of the false-positive sources in the decisions log (late
  pushes of old stamps, a clock more than 10 min ahead).
- **(T6.5 part 1 → T6.4, proposal wording) §9 "Stripe withholding" and §7.4.** The verification
  now judges a deliverer on the chunks stamped more than `verifyStampSlack` before its listing; the
  proposal's mitigation row does not say what counts as withheld. T6.4's report could state it,
  with the residual window (decisions log).
- **(T6.5 part 1 → sims) Stamps on the simulated clock.** The sims stamp with random bytes
  (`simNeighborhood.newStamp`: random timestamps) and run the reconciler
  on a simulated clock, so a verification's `Newer` there is arbitrary. No sim checks a deliverer's
  score today; one that does (withholding at scale) must stamp on the simulated clock.
- **(T6.5 part 1) The default integration run is 507 s with the unskipped Churn sub-case.** Under
  `go test`'s 10-min default timeout per package with ~90 s to spare on this host (decisions log,
  "(T6.5 part 1) Results and runs"); a slower runner, or part 2's additions, can cross it. This adds
  weight to "(T6.1 part 2 fix round 1) `make test-integration` needs a `-timeout`"; alternatively the
  Churn sub-cases could share one harness setup (16–54 s each).
