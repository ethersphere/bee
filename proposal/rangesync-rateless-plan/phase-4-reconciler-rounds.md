# Phase 4 — Full reconciler: status gossip, rounds, de-duplication

**Goal:** grow the Phase-1 minimal reconciler into the full one of §8: status gossip,
neighborhood-wide reconciliation rounds, node-wide de-duplication and a rejected cache. A synced
neighborhood costs one ~100 B `Status` per change per neighbor, and each missing chunk is fetched
once by `rangesync`.

**Proposal sections:** §7.1, §7.8 (incl. cost model), §8.1, §8.2, §8.3, §9 (claim-and-stall,
status spam), §10, §12 row 4, §14 Q7–Q8.

**Depends on:** per task (below). Rounds (T4.3 onwards) only pay off with Phase 2's cheap
small-diff sessions, and T4.5's tests need Phase 3 complete (catch-up `SyncRate`, T3.5).

Code: `pkg/rangesync/reconciler/` and `pkg/rangesync/wantset/`. The `puller` is not touched; in
dual mode it keeps its own per-(peer, bin) workers, and the two run side by side.

---

## T4.1 Status gossip (debounce + heartbeat)

**Depends on:** T1.9, T1.11.

**Scope (§6.1, §7.1):** subscribe to the Index's `SubscribeRootChange()`; send `Status` to every
eligible neighbor at most every `StatusMinInterval = 5s` after a change, and at least every
`StatusMaxInterval = 60s`. Last received `Status` per peer in memory only. Per-peer inbound status
rate limit (§9).

**Tests:** fake clock: 1000 root changes → one `Status` per 5 s per peer; no change → one per
60 s; spammy peer is limited.

**Size:** S–M.

---

## T4.2 Per-peer state machine (probing, unsupported peers)

**Depends on:** T1.11, T3.2 (the `Striped` state drives striped transfers).

**Scope (§8.1):** `Probing → {Unsupported, Idle}`, `Idle ⇄ Reconciling → {Fetching, Striped}`,
`Fetching → Idle`, error/Busy → `Idle` with backoff, disconnect or disjoint roots → removed.
`Unsupported` = the peer doesn't speak `rangesync`; the reconciler ignores it (if `pullsync` is
enabled, the puller syncs with it as it always does — no hand-off between the two). Explicit
transition table, testable without I/O.

**Tests:** every transition in §8.1 as a table test; a `pullsync`-only peer ends in `Unsupported`;
roots becoming disjoint after a radius change drops the peer.

**Size:** M.

---

## T4.3 Round scheduler

**Depends on:** T4.1, T4.2, T2.4.

**Scope (§8.2):**
- **Initiator:** lower overlay starts; the other side waits `2 × RoundInterval + jitter` and starts
  if the mismatch persists.
- **Rounds:** at most one session per peer; all sessions in a round read the same index snapshot
  (deltas applied during the round reach the next one — confirm against T1.6's locking).
- **Adaptive `RoundInterval`:** toward 5 s while differences are found, toward 30 s while every
  digest matches.
- Node-wide `MaxConcurrentSessions = 16`.
- **Open question 8:** optional hook to trigger a round when a pushsync delivery to this node fails
  — behind a constant, default off; if implementing it would require touching `pkg/pushsync`, skip
  it and record that.

**Tests:** fake clock + fake sessions: initiator rule, takeover after silence, interval adapts both
ways, concurrency cap.

**Size:** M.

---

## T4.4 Want-set integration in rounds + rejected cache

**Depends on:** T4.3, T1.7.

**Scope (§7.8, §8.2):**
- Every decoded `onlyRemote` set is claimed; denied IDs are skipped for this peer. On expiry or
  failure, retry from another peer known (this round) to have it (`id → peers`).
- Claim-and-stall (§9) is reported through the reconciler's single per-peer failure score
  (`PeerScorer`, T1.11), not a separate score.
- **Rejected cache** in `pkg/rangesync/wantset`: IDs failing `validStamp` or returning
  `ErrOverwriteNewerChunk` skipped for `RejectTTL = 10 min`, bounded, metric
  `rejected_cache_size`. **Open question 7:** measure with T4.6 whether this is enough for
  expired-batch skew; record the finding.
- Metric `want_claims{result}`.

**Tests:** §7.8 literally (B1 and B2 both have x → fetched once; B1 stalls → fetched from B2 after
the deadline); stale-stamp chunk not refetched every round.

**Size:** M.

---

## T4.5 Replace the minimal reconciler; dual-mode coexistence

**Depends on:** T4.2–T4.4, T3.5.

**Scope:** remove the timer-driven Phase-1 loop from `pkg/rangesync/reconciler` in favour of the
state machine + scheduler; keep the `storer.Syncer` contract (T1.11, T3.5) unchanged. No change in
`pkg/node` beyond what T1.12 set up.

**Tests:**
- `rangesync`-only node with 8 neighbors: goroutines are O(neighbors), not O(bins × neighbors)
  (P6); no `pullsync` streams opened by this node.
- Dual-mode node: both protocols converge; `cross_protocol_duplicates_total` is reported; the
  combined `SyncRate` reaches 0 only when both have caught up.
- `go test ./pkg/pullsync/... ./pkg/puller/...` unchanged and green.

**Size:** M.

---

## T4.6 Neighborhood simulation / N-node integration test

**Depends on:** T4.5.

**Scope (§13 "Neighborhood simulation"):**
- Analytical model (SREPSim-style; a Go test or a small tool behind a build tag) of N nodes with
  push replication p ≈ 3: metadata bytes per new chunk, duplicate deliveries, time to
  convergence; compare with the §7.8 table (N = 9, 17), and with `pullsync` numbers.
- Integration test (`-tags=integration` if heavy) with N in-memory nodes over `streamtest`:
  random uploads to p nodes, convergence within a bounded number of rounds, zero duplicate
  `rangesync` deliveries.
- Feed `pairwise_diff` into the cache-size decision (open question 2), `RoundInterval` bounds and
  `CatchUpMinDiff` (open question 9).

**Size:** L.

---

## Decisions log
_(sessions append here — T4.3 pushsync trigger; T4.4 expired-batch finding)_
- **(T4.1) One Status send path with a per-peer schedule.** The round and a new gossip loop
  (`reconciler/gossip.go`, woken by `Service.SubscribeRootChange`, by rounds and by a timer) both
  pick due peers through `pickDueLocked`/`statusDue` under `r.mu`. Picking records the send before
  it runs, so the two paths never send the same change twice. The round picks in the critical
  section that adds new peers, so first contacts are always its own and its evaluation (start-up
  guard, sessions) sees the outcome. Per peer:
  - first contact, and a moved root (radius change): at once, to every `rangesync` peer. This
    keeps T3.4's send before the round's sessions, so it bypasses `StatusMinInterval`. Radius
    changes are rare.
  - a changed root summary: at most every `StatusMinInterval`, deduplicated against the last
    Status sent, so the Index signal that follows a radius-change round sends nothing.
  - heartbeat: `StatusMaxInterval` after the last send.
  - a failed send, or a "resend" (a round has no `LastStatus` of the peer, which may have lost
    ours on a quick reconnect): after `StatusMinInterval`.

  Rounds no longer send every peer a Status every `RoundInterval`: `round(full)` is now `round()`,
  and `trackRootLocked` no longer returns `changed`. Additive API: `reconciler.Service.SubscribeRootChange`
  (`*rangesync.Service` delegates to its Index), `reconciler.Options.After` (fake timers in tests),
  and the constants `reconciler.StatusMinInterval` and `reconciler.StatusMaxInterval`. `pkg/node`
  is unchanged.
- **(T4.1) The Index debounce stays at `StatusMinInterval`.** It only coalesces the gossip loop's
  wake-ups. The per-peer schedule bounds the sends on its own, so correctness does not depend on
  the Index's real-time debounce, and the tests use a fake clock.
- **(T4.1) Summary pushes and heartbeats go to eligible neighbors only** (§6.1, §7.1). These are
  peers whose `LastStatus` passes `isEligible`, plus peers whose Status is unknown. A peer with a
  disjoint root or another Mapping gets only the first contact and moved roots. Before T4.1, every
  connected `rangesync` peer got a Status every round. Eligibility is symmetric, and the side whose
  root moves sends to everyone, so a pair that becomes eligible finds out at once.
- **(T4.1) An inbound Status over the rate is dropped. It is not a violation and is not scored.**
  `PeerStatusBurst = 4`, then one per `PeerStatusInterval = StatusMinInterval / 2`. An honest
  sender sends one peer at most one Status per 5 s, apart from first contact and radius changes.
  Over the rate, `handleStatus` resets the stream before reading, records nothing, and counts
  `status_rate_limited_total`. Reasons not to disconnect or score:
  - `PeerScorer.Fail` implies a disconnect, and a reconnect race could trip an honest peer.
  - A Status does not start a session; sessions have their own limits.

  The reset makes the sender's `SendStatus` fail, so the sender retries after `StatusMinInterval`.
  The limiter is a `ratelimit.Limiter`, as for sessions: it is cleared on disconnect and never
  pruned otherwise.
- **(T4.1) The flaky `TestIndexSubscribeRootChange/debounced` is left as is.** T4.1 does not touch
  `pkg/rangesync/fingerprint` and does not rely on the debounce for correctness (see above).
- **(T4.1 fix) A dropped Status is now reported to the sender.** `SendStatus` and the new
  `SendStatusWith` return the `FullClose` error. libp2p's `FullClose` returns nil on EOF and an
  error when the peer reset the stream, so a Status the peer dropped over its rate counts as
  failed and is retried after `StatusMinInterval`. The earlier entry's "the sender retries" is true
  from here on. `streamtest`'s `FullClose` does not see resets, so `TestSendStatusPeerReset` wraps
  the streamer.
- **(T4.1 fix) The heartbeat goes to every `rangesync` peer, every `StatusMaxInterval`.** This
  replaces the earlier "heartbeats to eligible neighbors only"; summary-change pushes stay
  eligible-only. Without it, a Status an ineligible peer lost (quick reconnect) never healed:
  `exchanged` held the start-up guard until `StartupGuardTimeout`, and the other side resent every
  round. The alternative, dropping a peer from `exchanged` after N resends, hides the loss instead
  of healing it. The cost is what the old full rounds cost: one ~100 B Status per peer per 60 s.
- **(T4.1 fix) Statuses are sent exactly as picked, from ordered snapshots.** `*rangesync.Service`
  gains `SendStatusWith(ctx, peer, st)`; `SendStatus` is `LocalStatus` plus `SendStatusWith`. The
  narrow `reconciler.Service` replaces `SendStatus` with `SendStatusWith`; only the test fake
  implemented it. Rounds and gossip passes take `LocalStatus` and pick under a new `pickMu`, so no
  pick records an older snapshot than an earlier one. `pickMu` is taken before `r.mu` and never
  inside it, because `LocalStatus` can wait on Index locks and must not hold up `r.mu` (which
  `SyncRate` takes).
- **(T4.1 fix) Sender-side rate cap.** Per peer, a GCRA allows `statusBurst = PeerStatusBurst − 1
  = 3` Statuses back to back, then one per `PeerStatusInterval`. A first contact or a moved root
  waits only when three went out just before (for example, several radius changes within
  seconds). The one-token margin absorbs arrival jitter of up to `PeerStatusInterval`. So an honest
  sender never trips the receiver's limit, whatever the radius does.
- **(T4.1 fix) A failed send no longer clears `sent`.** `sent` now means "a Status of ours reached
  the peer since its entry was added" and is never cleared. A separate `failed` flag schedules the
  retry. So a failed heartbeat or push no longer skips the peer's sessions or disarms the guard. A
  send's outcome is recorded only in the `*peerState` it was picked with, so a peer that left and
  came back mid-send keeps its new entry's state.

## Follow-ups
_(sessions append here)_
- **(T6.0) `round_interval_seconds` gauge (T4.3).** The current adaptive `RoundInterval`. AC-V1
  (convergence within 2 × `RoundInterval`) and T6.3 ("confirm or adjust `RoundInterval` bounds")
  can't be checked on testnet without it.
- **(T6.0) `want_claims{result}` values (T4.4).** At least `granted`, `denied`, `expired`, and
  `late` (a delivery that arrives after its claim expired and the ID was claimed through another
  peer). AC-D1/AC-D2 bound intra-`rangesync` duplicates by expired claims and need `late` to tell
  them apart from pushsync races in `cross_protocol_duplicates_total`. If `late` needs the fetch
  path (T1.10) to know whether its claim is still held, extend the T1.7 want-set API additively.
- **(T4.1) Tests to run with `-race` on CI.** This host has no C compiler; these ran with
  `-count=10..20 -cpu=1,2,4` instead.
  - `pkg/rangesync/reconciler`: `TestStatusGossip*` (especially `TestStatusGossipLoop`),
    `TestRadius*`, `TestStartupGuard*`, `TestSyncRate*`, `TestPeerJoinLeave`, `TestClose`.
  - `pkg/rangesync`: `TestStatusRateLimit`, `TestStatus*`.
- **(T4.1) A received Status does not wake the reconciler (T4.3).** When a peer becomes eligible
  from its own side (its root moved), we send it our next Status only at the next gossip pass
  (≤ `StatusMaxInterval`). T4.3's scheduler reacts to digests and could add a status-received hook
  in `rangesync` (additive) that wakes the gossip loop and the round.
- **(T4.1) Slow or failing peers in a gossip pass (T4.2).** A pass sends synchronously (≤ 16 at
  once, `statusTimeout` 10 s each). A black-holed peer delays the pass's other sends by up to 10 s,
  and a failing peer is retried every `StatusMinInterval`. T4.2's per-peer backoff is the place to
  back off repeated Status failures.
- **(T4.1) Repeated Status spam is never escalated (T4.2).** Statuses over the rate are only
  dropped. If testnet shows peers persistently over the rate, escalate to `violation` with the
  failure score (for example, N drops within a window).
- **(T4.1) Moved-root race (T4.3).** If the gossip loop is sending a moved root while a round
  evaluates, the round can start a session before the peer has the new Status. The responder
  refuses (scope error or Busy), and the next round retries. This is benign, but T4.3's scheduler
  could wait for in-flight Status sends before it starts sessions.
- **(T4.1) `TestIndexSubscribeRootChange` data race (fingerprint).** Its parallel subtests
  ("immediate" and "debounced") share one `*rand.Rand`. Give each subtest its own when the flaky
  debounced wait gets fixed (phase-2 follow-up).
- **(T4.1 fix) More tests to run with `-race` on CI.** These ran with `-count=10 -cpu=1,2,4`
  instead.
  - `pkg/rangesync/reconciler`: `TestRoundOwnsFirstContact` and `TestStatusGossipSnapshotsInOrder`
    are probabilistic stress tests: with the first-contact race or the snapshot ordering
    reintroduced, they failed in 3 of 3 and in most runs. Also `TestStatusResultStaysWithItsEntry`,
    `TestFailedSendKeepsSessions`, `TestStatusGossipWithinPeerRate`.
  - `pkg/rangesync`: `TestSendStatusPeerReset`, `TestSendStatusWith`.
- **(T4.1 fix) The sender-side rate can also defer a moved root.** After three quick Statuses, the
  round that sees a radius change does not send at once, and its sessions meet the same benign
  refusal as in the moved-root race above. The same T4.3 remedy applies.
