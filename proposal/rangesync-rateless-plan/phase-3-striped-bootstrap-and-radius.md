# Phase 3 — Striped bootstrap, radius changes, catch-up `SyncRate`

**Goal:** large differences (a new/empty node, a radius decrease) are transferred **once**,
striped across neighbors, then verified. A `rangesync`-only node bootstraps and handles radius
changes on its own, and reports a `SyncRate()` the reserve worker and redistribution can rely on.

**Proposal sections:** §6.3 (`LargeDiff` branch), §7.4, §7.5, §8.1 (`Striped` state), §8.4,
§9 (stripe withholding), §12 row 3.

**Depends on:** Phase 1 (not Phase 2 — striping uses fingerprints + item lists; the verification
round uses symbols when Phase 2 is present).

All code in `pkg/rangesync/reconciler/` (`stripes.go`). `pkg/puller` is not touched; when
`pullsync` also runs, it keeps doing its own historical sync and interval resets independently.

---

## T3.1 Bootstrap detection and stripe planner

**Depends on:** T1.8 (`Striper` hook), T1.9 (`Status`).

**Scope**

- **Detection:** own root count ≪ peers' `Status` counts for the same session root (a constant
  ratio, e.g. own < 50% of the median peer count — record the choice), or a session hits the
  `d̂ > LargeDiff` branch (§6.3 — implement the `Striper` hook stubbed in T1.8).
- **Planner:** pure function `PlanStripes(root Range, peers []PeerStatus) []Stripe` — split into
  `2^s ≥ k` prefix-aligned stripes, stripe `i` → peer `i mod k`; only peers whose session root
  covers the stripe are eligible (§7.6). Optional `redundancy = 2` (§9) behind a constant,
  default 1.
- **Sub-range variant** for T3.4: stripe only a given sub-range (the new sibling half).

**Tests:** k = 1, 3, 8, 17 peers; peers with narrower roots; no eligible peer for a stripe →
reported, not dropped.

**Size:** S.

---

## T3.2 Striped transfer with pipelined paging/fetch and reassignment

**Depends on:** T3.1, T1.9, T1.10.

**Scope:** per stripe, `FP(stripe, count 0)` → peer pages `Items(stripe, …, More)`; `Fetch`
starts as pages arrive (pipelined, bounded queue); all IDs go through the want set. Per-stripe
timeout; on failure/timeout/withholding, reassign the rest of the stripe to the next eligible
peer — re-running `FP(stripe)` makes a restart idempotent. Respect `rangesync`'s own global fetch
limiter (T1.10).

**Tests:** 4 in-memory peers + 1 empty node via `streamtest`: every chunk delivered exactly once;
kill one peer mid-stripe → its stripe completes via another; ID metadata transferred once (§7.4
table order of magnitude).

**Size:** L.

---

## T3.3 Verification round

**Depends on:** T3.2.

**Scope (§7.4 "SREP iteration 2"):** after all stripes complete, reconcile each stripe with a
**different** peer than the one that delivered it (a normal session on that range). Then leave
bootstrap mode. A large diff found in verification is reported as a failure of the deliverer
through the reconciler's single per-peer failure score (`PeerScorer`, T1.11) (§9).

**Tests:** a deliverer that withholds 5% of its stripe → verification fetches the rest from
another peer and the score increments.

**Size:** M.

---

## T3.4 Radius decrease / increase handling

**Depends on:** T3.2 (striping the new half), T1.6.

**Scope (§7.5):**
- Radius `r → r−1`: the Index rebuilds asynchronously at the new root (observer `RadiusChanged`,
  T1.6); sessions get `Busy` until it finishes. Per
  peer, compare the two children of the new root separately: the old half through a normal
  session, the new sibling half (counts far apart) through striping on that half only.
- Radius increase: the root narrows; nothing to transfer. Assert no fetch happens.
- While the Index rebuilds, peers answer `Busy` (T1.9) — back off and retry.

**Tests:** 3-node test, lower radius on one node → only the new half is transferred; the old half
costs one fingerprint exchange.

**Size:** M.

---

## T3.5 Catch-up `SyncRate` and reserve-worker interplay

**Depends on:** T3.2, T3.4, T1.11, T1.12.

**Why:** with `--puller-enable=false`, the reconciler's `SyncRate()` alone decides when the
reserve worker may lower the radius (`pkg/storer/reserve.go`, `reserveWorker`:
`count < threshold && SyncRate() == 0`) and whether the node counts as fully synced for
redistribution (`pkg/node/node.go`). If it reports 0 too early, the radius drops while chunks are
still arriving; if it never reaches 0, the radius never drops and the node never plays.

**Scope:** include striped transfers (bootstrap and radius-decrease halves) and verification
fetches in the catch-up rate, alongside sessions whose resolved difference (wants) is ≥ `CatchUpMinDiff`
(T1.11). Steady-state sessions never count. **Open question 9:** check `CatchUpMinDiff = 256`
against `pairwise_diff` from the simulations (T4.6) and record it.

**Tests:** storer + reconciler with a fake clock, `rangesync` only:
- bootstrap → `SyncRate() > 0` until stripes and verification finish, then 0 within the window;
- live uploads in steady state → `SyncRate()` stays 0;
- reserve below threshold → radius drops once, `SyncRate()` rises during the new-half transfer,
  and the next decrease waits for it — the same sequencing the puller gives today;
- combined syncer with both drivers: 0 only when both are 0.

**Size:** M.

---

## Decisions log
_(sessions append here — T3.1 bootstrap threshold and stripe redundancy default; T3.5
`CatchUpMinDiff`)_
- **(T3.1) Bootstrap threshold.** `IsBootstrap`: own count < `BootstrapRatio` (0.5) × median count of the peers whose Status root equals our root (upper median; peers with other roots are skipped; no peers or a zero median → not bootstrapping). No absolute floor. Limitation: a bootstrapping node whose root differs from all its neighbors' roots is never detected here and relies on the session's d̂ > LargeDiff trigger (Striper) alone.
- **(T3.1) Stripe redundancy default.** `StripeRedundancy = 1`; the verification round (T3.3) covers withholding. `PlanStripesRedundancy` takes 2 when needed.
- **(T3.1) Stripe depth and eligibility.** k counts peers whose root overlaps the striped range; s = ceil(log2 k), deepened to the deepest narrower peer root (cap 16 bits below the range) so each stripe is inside or outside every peer root at most 16 bits deeper; a peer root more than 16 bits below the range covers no stripe and is never eligible. `redundancy <= 0` is treated as 1; fewer eligible peers than the redundancy gives a shorter Peers list. Eligible = peer root contains the stripe; an ineligible peer's turn passes to the next eligible peer in rotation. Unassigned stripes come back with no Peers (`Unassigned`). The sub-range variant is `PlanStripes(subRange, peers)`.
- **(T3.2 part 1) Striper wiring API.** `rangesync.Service.ReconcileWith(ctx, peer, ReconcileOptions{Open, Striper, Wants})`; `Reconcile` is `ReconcileWith` with zero options. `Wants`, when set, receives each handled Message's wants on the session goroutine (blocking it holds the session back) and ReconcileWith returns none. `reconciler.Service` now lists `ReconcileWith` instead of `Reconcile`. `Reconciler.session` passes a `StripeCollector`; ranges it took over are planned with `PlanStripes` over the peers with a Status and transferred after the session's own fetch.
- **(T3.2 part 1) Transfer shape.** `Reconciler.TransferStripes(ctx, []Stripe) []StripeResult` (stripes_transfer.go): per stripe a session opened with `Open: [stripe]` from `Peers[0]` (count 0 on an empty node, so the peer pages Items); wants go to a per-stripe queue of `stripeQueue = 4` Message batches drained by `Fetch` on the same peer (want set + node-wide fetch limiter inside Fetch). Stripes of one peer run sequentially (one session per peer), peers in parallel. `ErrSessionLimit` is retried every `stripeRetryInterval = 1s` until the stripe timeout. Stored chunks count towards SyncRate (catch-up in flight for the whole transfer).
- **(T3.2 part 1) Stripe timeout and results.** `StripeTimeout = 15 min` (Options.StripeTimeout) bounds session + fetch of one stripe; timeout → `ErrStripeTimeout`, `stripes_total{result="timeout"}`. Completed → `done`. Other failures (session error, fetch error) return the error with no metric label in part 1; part 2 counts them as `reassigned`. A stripe without peers → `ErrNoStripePeer`, no session. Restart = calling TransferStripes again: the session lists only the IDs still missing (tested).
- **(T3.2 part 1) Catch-up span of a transfer.** `TransferStripes` holds `catchUpFetches` for its whole run, session phases included, so SyncRate reports at least `StartupGuardRate` throughout. Kept deliberately: a stripe exists only for a large difference (bootstrap or d̂ > LargeDiff), so the node is catching up from the first page; narrowing to fetch activity would let SyncRate drop to 0 ("synced") between pages and batches, which the T1.11 gate exists to prevent. T3.5 may refine it.
- **(T3.2 part 1) Backpressure.** A full want queue blocks the session goroutine in `Wants`, so the peer waits for the next Message. Safe today because the responder's only bound is its own 10-min `sessionTimeout`; a responder idle/read deadline shorter than one batch fetch would turn a slow fetch into a failed stripe (not a scored violation; restart lists only what is missing). Documented on `TransferStripes`; tested by `TestTransferStripesBackpressure`. Batches dropped on cancel are not counted in `Listed`.
- **(T3.2 part 2) Restart and reassignment.** `runStripe`: an attempt that failed but stored chunks (progress) restarts with the same peer (re-runs FP(stripe), lists only what is missing); this is the restart loop that lets a stripe outlive the 10-min `sessionTimeout` / per-attempt `StripeTimeout` (StripeTimeout now bounds one attempt). An attempt without progress, or a completed session with `Stored < Listed` (withholding, `ErrStripeWithheld`), is reassigned: next of `Stripe.Peers`, then the first untried peer with a Status whose root contains the stripe. At most `maxStripeReassign = 3` reassignments. `stripes_total{reassigned}` per reassignment; `done`/`timeout` once per stripe at the end. Sessions with one peer are serialized by a per-transfer peer lock (one lock held at a time, no deadlock). Limitation: IDs a peer lists but the stamp validator rejects look like withholding and cost up to 3 reassignments.
- **(T3.2 part 2) Stripe registry and worker.** Ranges from a session's Striper and the bootstrap root go through `enqueueStripes` into a registry (queued + in flight); a range overlapping (containing or contained in) a registered one is dropped and resurfaces in a later round. One `stripeWorker` goroutine (started in `Start`, ends on `Close` via r.ctx, in r.wg) transfers the queued ranges one at a time; the round's session goroutine no longer blocks on transfers.
- **(T3.2 part 2) Bootstrap trigger.** Each round calls `IsBootstrap(local, peerStatuses())`; when true it enqueues `local.Root` and starts no per-peer sessions that round (awaiting/guard bookkeeping unchanged). The bootstrap counts as a pending reconcile until its first wants arrive or the transfer ends, so the start-up guard keeps its "catch-up started" meaning.
- **(T3.2 part 2 review) Bounds.** Same-peer restarts with progress are capped at `maxStripeRestarts = 8` per peer (reset on reassignment); past it the stripe is reassigned as if no progress was made, so a trickling peer costs at most (8+1)×(3+1) attempts per stripe. The registry holds at most `maxStripeRegistry = 1024` ranges; past it a range is dropped (Debug log, `stripes_total{dropped}`) and resurfaces in a later round.
- **(T3.2 part 2 review) Bootstrap and the start-up guard (D9/D18).** The bootstrap round counts as a pending reconcile only when it registers the root; the guard then lifts on the first wants listed by any stripe session (the same "catch-up started" point as a normal session, whose wants are in hand when Reconcile returns) or when the transfer ends; the 120 s guard timeout still applies. Not "first stored chunk": a fetch-stalled bootstrap would then hold the guard to its timeout while SyncRate already reports ≥ StartupGuardRate through catchUpFetches. If the root overlaps a range already in a striped transfer, the round does not bootstrap and runs per-peer sessions (the guard waits for them). Per-peer sessions are skipped only while the bootstrap transfer is queued or in flight; after one that did not complete (any stripe error), the next round runs per-peer sessions before bootstrapping again.
- **(T3.2 part 2 review) Withholding false positives.** `Fetch` reports only the number stored, so a shortfall from stamp rejects or want-set claims held by another fetch cannot be told apart from withholding cheaply; it is counted as withholding and bounded by `maxStripeReassign` (tested: `TestTransferStripesPartialShortfall`). A restart lists only what is still missing, so the cost is re-listing that shortfall.
- **(T3.3) Verification round.** `Reconciler.VerifyStripes(ctx, []StripeResult) []VerifyResult` (stripes_verify.go). The stripe worker runs it on every transferred range (bootstrap root and large-diff ranges) right after `transferStripes`, before the registry entry and `bootstrapping` are cleared, so rounds keep skipping per-peer sessions until verification ends ("then leave bootstrap mode"). Each completed stripe is verified with a peer with a Status whose root covers it, other than its deliverer, lowest failure score first, by one `transferStripe` attempt (session `Open: [stripe]` + fetch from the verifier; ErrSessionLimit retried, StripeTimeout bound); no restart/reassignment. One session per verifier at a time, verifiers in parallel. Failed stripes (`Err != nil`) are not verified (`ErrStripeNotDone`): their deliverer is unclear and the next round's per-peer sessions cover them.
- **(T3.3) No other peer.** Skipped with a Debug log (`ErrNoVerifier`), no session, no score: verifying with the deliverer cannot reveal what it withheld.
- **(T3.3) Scoring.** `verification_diff` (histogram, buckets 0,1,4,...,65536) observes the verifier's listed IDs per completed verification. The deliverer gets one `Fail` (reason `ErrStripeVerification`) when diff > max(`verifyMinDiff = 4`, deliverer-stored/`verifyDiffRatio = 100`): the tolerance absorbs uploads that arrived during the transfer. A failed verification session scores nobody.
- **(T3.3) Guard and SyncRate.** Verification does not touch the start-up guard (it lifted at the transfer's first wants or end). `VerifyStripes` holds `catchUpFetches` for its run and its stored chunks go into the rate, so verification fetches count as catch-up (T3.5 builds on this); the gap between the transfer's and the verification's hold is a function return.
- **(T3.3) Verification bytes.** Not counted separately in the code: a `{phase}` label on `bytes_metadata` would need a `ReconcileOptions` field in pkg/rangesync (API change, outside the narrow scope). Choice: test hook / simulation measurement (T6.1 can count the verifier's reconcile-stream bytes, as `TestTransferStripesEmptyNode` does for metadata). See Follow-ups.

## Follow-ups
_(sessions append here)_
- **(T3.1) Striper not wired into Service.** session.go already calls `cfg.Striper` (T1.8); `StripeCollector` implements it, but `Service.sessionConfig` (reconcile.go) never sets `Config.Striper` and `Reconcile` has no parameter for one. Wiring needs a rangesync API change outside session.go (T3.2).
- **(T6.0) Stripe and verification metrics (T3.2, T3.3).** §10 has none, but AC-B2 (verification
  ≤ 1 MB, "a few KB" in §7.4) and the T6.1 Churn scenario ("stripes reassigned") need them.
  Proposed: `stripes_total{result}` (done, reassigned, timeout) in T3.2 and `verification_diff`
  (differences found per verified stripe, histogram) in T3.3. Bytes spent in the verification
  round can come from `bytes_metadata` if T3.3 counts them separately (e.g. a `{phase}` label) or
  via a test hook.
- **(T3.2 part 1) Bootstrap trigger not in the round.** An empty node's root session takes `DecisionRequestItems` (cL == 0), never the Striper, so bootstrap striping needs the round to call `IsBootstrap` + `PlanStripes(root)` + `TransferStripes` instead of per-peer sessions. Not wired (not in the T3.2 scope text); PART 2 or T3.3.
- **(T3.2 part 1) sessionTimeout vs. backpressure.** A stripe session is bounded by rangesync's fixed 10-min `sessionTimeout` on both sides; with the bounded queue a mainnet stripe (~500k IDs at 1000/k chunks/s) cannot drain in 10 min, so the session times out and must be restarted. PART 2's restart loop should re-run FP(stripe) while it makes progress, or the queue/timeout needs retuning.
- **(T3.2 part 1) Race tests for CI.** Run with `-race`: `TestTransferStripes*` (incl. Backpressure, Shutdown), `TestSessionStripesLargeRange` (pkg/rangesync/reconciler), `TestReconcileWithStripe` (pkg/rangesync). Locally only `-count=5 -cpu=1,2,4` (no C compiler).
- **(T3.2 part 1, review M1) Stripe registry and round blocking.** `Reconciler.session` runs `r.stripe` synchronously in the round's session goroutine (possibly for hours, keeping that peer's `running` set), and ranges handed over by two peers' sessions that overlap are both transferred: there is no stripe registry or dedup. PART 2 must add a registry (dedup of in-flight/planned ranges) and run transfers off the round's session goroutine.
- **(T3.2 part 2) Registry granularity.** Dedup is whole-range overlap; a range that only partially overlaps a registered one is dropped, not trimmed, and waits for a later round. Trimming, per-peer state for `Striped` (§8.1) and scheduling stripes against round sessions belong to T4.2/T4.3.
- **(T3.2 part 2) Race tests for CI.** `-race`: `TestTransferStripes*` (incl. PeerKilled, RestartOnProgress, Withheld), `TestEnqueueStripes*`, `TestRoundBootstrap*`, `TestTransferStripesTrickle`, `TestTransferStripesPartialShortfall`, `TestSessionStripesLargeRange`, `TestStartupGuard*` (pkg/rangesync/reconciler). Locally `-count=5 -cpu=1,2,4` only.
- **(T3.2 part 2 review) Withholding accounting.** Telling withholding apart from stamp rejects / foreign want-set claims needs `Fetch` to report requested-but-undelivered IDs separately (a rangesync API change).
- **(T3.3) Verification bytes label.** If AC-B2 needs a production metric, add `ReconcileOptions.Phase` (or similar) so rangesync labels `bytes_metadata{dir,phase}`; T3.3 left it to a test hook.
- **(T3.3) Race tests for CI.** `-race`: `TestVerifyStripes*`, `TestRoundBootstrapVerifies` (pkg/rangesync/reconciler). Locally `-count=3 -cpu=1,2,4` only.
