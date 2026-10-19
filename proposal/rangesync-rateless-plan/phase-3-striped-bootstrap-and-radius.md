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

## Follow-ups
_(sessions append here)_
- **(T3.1) Striper not wired into Service.** session.go already calls `cfg.Striper` (T1.8); `StripeCollector` implements it, but `Service.sessionConfig` (reconcile.go) never sets `Config.Striper` and `Reconcile` has no parameter for one. Wiring needs a rangesync API change outside session.go (T3.2).
- **(T6.0) Stripe and verification metrics (T3.2, T3.3).** §10 has none, but AC-B2 (verification
  ≤ 1 MB, "a few KB" in §7.4) and the T6.1 Churn scenario ("stripes reassigned") need them.
  Proposed: `stripes_total{result}` (done, reassigned, timeout) in T3.2 and `verification_diff`
  (differences found per verified stripe, histogram) in T3.3. Bytes spent in the verification
  round can come from `bytes_metadata` if T3.3 counts them separately (e.g. a `{phase}` label) or
  via a test hook.
