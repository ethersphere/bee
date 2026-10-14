# Phase 5 (optional for testnet, required before mainnet) — Salted audit

**Goal:** once per redistribution round, `rangesync` neighbors compare an anchor-salted digest of
their committed neighborhood. Equal plain digests with different audit digests means a crafted
fingerprint collision; the node finds out before the commit phase, while it can still repair.

**Optional for testnet, required before mainnet.** Acceptance testing (Phase 6) can run without
it, but without the audit a crafted fingerprint collision goes undetected, so no mainnet node may
enable `rangesync` until this phase has shipped. Do it before Phase 6 if testnet runs should also
cover collision detection.

**Proposal sections:** §7.9, §9 (fingerprint collisions), §6.2 (`Status.AuditRound`,
`Status.AuditDigest`), §10 (`audit_mismatch{kind}`), §12 row 5.

**Depends on:** Phase 1 (T5.3 also needs Phase 2). Non-goal (§3): the redistribution game is not
changed — the audit only *reads* round parameters.

---

## T5.1 Audit computation + optional round hook in storageincentives

**Depends on:** T1.2 (`Expand`), T1.5 (`ReserveIterateAddr`), T1.12 (node wiring).

**Scope**
- `pkg/rangesync/audit/`: `Compute(ctx, iterate, anchor, depth) (count, digest)` — scans
  `ReserveIterateAddr` over the committed neighborhood, sums `LtHash(Expand(anchor, id))`
  (T1.2), IDs only, never chunk data. ~4M SHAKE expansions, a few seconds.
- **The only change in `pkg/storageincentives`, additive and nil by default:** an optional
  observer, e.g. `type RoundObserver interface { RoundStarted(round uint64, anchor []byte, depth uint8) }`
  set through a setter or an option on `Agent`. Call it non-blocking once per round, **every
  round** — `handleSample` (`pkg/storageincentives/agent.go:406`) returns early when the
  neighborhood isn't selected, the node is frozen, unsynced, etc., so hook the `sample` phase
  event before those checks and fetch the anchor with `a.contract.ReserveSalt(ctx)` the way
  `makeSample` does; depth = `a.store.CommittedDepth()`. Errors are logged, never returned.
- **Round number (§7.9):** the anchor fetched when the claim phase of round *t* starts is the one
  sampled for round *t + 1* (`handleSample` records `round + 1`), so call
  `RoundStarted(t + 1, anchor, depth)`. Every node labels the audit the same way, and T5.2 compares
  only equal `AuditRound`s.
  When no observer is set (rangesync off), the agent behaves exactly as today.
- `rangesync` implements the observer; `pkg/node` wires it only when `rangesync` is enabled.
- Must finish before the commit phase; otherwise skip that round's audit (metric).

**Tests:** `agent_test.go`: with no observer, commit/reveal/claim outcomes unchanged; a fake
observer is called once per round with the right round number (*t + 1*), anchor and depth, including rounds where the
neighborhood is not selected; a slow observer does not delay commit. `audit`: order-independent,
anchor-dependent, one extra element changes the digest.

**Size:** S–M.

---

## T5.2 `AuditRound` / `AuditDigest` in `Status` + mismatch handling

**Depends on:** T5.1, T4.1.

**Scope:** fill `Status.AuditRound` and `AuditDigest`. For a peer's audit of the same round:
- audit differs **and** plain digests equal → `audit_mismatch{kind="collision_suspected"}`,
  Warning log, schedule a **salted session** (T5.3);
- audit differs and plain digests differ → normal session, `audit_mismatch{kind="real_diff"}`.
Compare over the **same** range (the session root, not each node's own root): if roots differ,
compute the audit for the session root or skip — record the choice.

**Tests:** a set pair with equal fingerprints but different contents (test-only weak fingerprint
via `export_test.go`) → collision path; real diff → normal path.

**Size:** M.

---

## T5.3 Salted session mode (collision-suspected path)

**Depends on:** T5.2, Phase 2.

**Scope (§7.9):** a session variant where every range fingerprint uses `Expand(anchor, id)`. The
caches can't serve salted summaries, so both sides compute them by scanning `ReserveIterateAddr`
per range — bounded by the session caps, and only allowed when both peers advertised the same
`AuditRound`. Symbol streams are unaffected (128-bit checksums, §9). Signal the mode with
`Message.AuditRound` in the first `reconcile` message; the field is already in the proto (T1.1),
so no protocol version bump is needed. Remove the Phase-1 "ignore non-zero `AuditRound`" rule.

**Tests:** the T5.2 collision fixture converges through the salted session; salted mode without a
matching `AuditRound` is refused.

**Size:** M–L.

---

## Decisions log
_(sessions append here)_

## Follow-ups
_(sessions append here)_
