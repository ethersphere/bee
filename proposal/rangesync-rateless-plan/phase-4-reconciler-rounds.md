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
- **(T4.2) The §8.1 machine is an explicit table in `reconciler/peerfsm.go`, with no I/O.**
  `transitions` maps state and event to the next state; an event a state does not list leaves the
  machine unchanged. `[*]` is `stateRemoved`. The reconciler fires the events under `r.mu`
  (`reconciler/peers.go`) and passes the time. Three kinds of edge are added to §8.1:
  - `Idle → Striped`: the stripe worker plans a range across every neighbor that covers it
    (§7.4, §7.5), so a stripe session starts from Idle.
  - `Fetching → Striped`: the fetch of a session's wants uses the fetch stream, not the session
    slot. The peer returns to Idle once both have ended. Round sessions still wait for the fetch,
    as the old `running` flag did.
  - A disconnect from every state.

  `Reconciling → Striped` is in the table but the wiring never takes it. A session hands its large
  ranges to the stripe worker and ends, and with one session per peer the peer cannot serve a
  stripe while its own session runs.
- **(T4.2) "Disjoint" means not eligible (`isEligible`).** That covers disjoint neighborhoods and a
  different Mapping. A round moves an exchanged peer from Probing to Idle, then to Removed when it
  is not eligible. It does this before any stripe is planned, so the stripes of a bootstrap find
  their peers Idle. A Removed peer is dropped from sessions, from stripe planning
  (`peerStatuses`) and from the start-up guard. It stays connected and keeps its Status gossip
  (T4.1: first contact, moved roots, heartbeat). It gets a new machine when it is eligible again
  (either root moved) or its Status is gone. `pkg/node` is unchanged.
- **(T4.2) `Unsupported` replaces the `unsupported` flag.** `ErrProtocolUnsupported` outside
  Probing means a reconnect the peer set did not show, so the peer gets a new machine, which then
  goes to Unsupported.
- **(T4.2) A peer serving a stripe is in Striped, and round and stripe sessions are exclusive.**
  This covers the T3.2 part 2 and T3.4 carry-overs. `transferStripe` (transfers and verifications)
  calls `acquireStripe`, which waits while the peer is Reconciling or Striped. The waiter is woken
  by a channel, and the wait counts towards the stripe timeout. Rounds start no session with a
  peer a stripe waits for, so the stripe is not starved. A peer that is unknown, Probing,
  Unsupported or Removed is not tracked, which keeps direct `TransferStripes` callers working as
  before; `peerLocks` stay for them. A round session no longer gets `ErrSessionLimit` from this
  node's own stripe session. A session the peer started, or the node-wide cap, still causes it
  (Busy).
- **(T4.2) Busy and errors back off per peer.** This is the T3.4 review item. Round sessions
  answered Busy (`isBusy`, including `ErrSessionLimit`) or failed count as one streak per peer.
  - The first in a row is retried by the next round, as before.
  - From the second, the wait is `MinRoundInterval`, doubling up to `maxSessionBackoff = 5 min`.
  - A completed session resets the streak.

  Busy stays unscored; the growing backoff is the deprioritization of peers that stay Busy.
  Stripe sessions keep their 1 s Busy retry within the stripe timeout and are not counted. The
  first step is zero because the T3.x tests run rounds on a frozen fake clock. In production,
  rounds are at least `MinRoundInterval` apart, except rounds that a topology change triggers.
- **(T4.2) Failed Status sends back off.** This is the T4.1 follow-up. The retry is
  `statusRetry(n)`: `StatusMinInterval` doubling with each failure in a row, up to
  `StatusMaxInterval`. A success resets it. A first contact or a moved root still goes out at
  once. A black-holed peer now holds up a gossip pass (≤ `statusTimeout`) at most once per 60 s
  instead of every 5 s.
- **(T4.2) Responder-side fetches are not a state.** `Wants` fetches what a peer that initiated
  listed; the peer's machine stays Idle.
- **(T4.2) An uncovered radius half needed no change (T3.5 carry-over).** `pending` has no time
  bound (only its `SyncRate` hold does), and every round retries `enqueueRadiusHalf`. So the half
  is striped once a neighbor that covers it appears, even past `radiusHold`. Admission now runs
  before `enqueueRadiusHalf`, so a neighbor admitted in a round counts in that round.
  `TestRadiusHalfStripedWhenCovered` pins this.
- **(T4.2 fix) The start-up guard waits for differences that are not resolved yet.**
  - The round's last critical section admits each peer from the same `LastStatus` it evaluates
    (`admitPeerLocked`). An exchanged, eligible peer is therefore always Idle there, or already in
    a session, fetch or stripe. A Status that arrives after `admitLocked` can no longer leave a
    peer Probing, or Removed but eligible, and so skipped without counting.
  - A differing peer this node initiates with that is backing off keeps the guard from arming.
  - A session that ends without completing, for any cause, disarms the guard until the next round.
    This also closes the case from before T4.2 where a Busy session released the guard.

  `StartupGuardTimeout` still bounds all of this.
- **(T4.2 fix) Causes on this node do not count towards the peer's backoff.** A session that failed
  for a local cause (`localCause`) fires `eventAborted`: Reconciling to Idle with no backoff, and
  the streak unchanged. Local causes are:
  - this node's session cap;
  - `fingerprint.ErrBusy`;
  - no session scope (this node's root moved);
  - `ErrNoStatus`;
  - `ErrMappingMismatch`.

  `pkg/rangesync` gains two additive sentinels to tell the session caps apart:
  `ErrNodeSessionLimit` and `ErrPeerSessionLimit`. Both wrap `ErrSessionLimit` and keep their
  messages. The backoff counts `ErrPeerBusy`, `ErrPeerSessionLimit`, and other errors of the
  exchange with the peer (§8.1 "error (backoff)"). A responder that refuses a session after this
  node's root moved answers Busy, so the reconciler cannot tell that refusal from a real Busy. It
  costs one step of the streak.
- **(T4.2 fix) A streak counts answers in a row.** It ends with a completed session, with an equal
  digest, or after a quiet period of `maxSessionBackoff` past the last backoff.
- **(T4.2 fix) Stripes are planned with admitted peers only.** `peerStatuses` leaves out Probing,
  Unsupported and Removed peers. A Probing peer that a round admits mid-stripe would race the
  round's session for the slot. Tests that call `TransferStripes` on a started harness now wait
  for admission (`waitAdmitted`).
- **(T4.2 fix) An Unsupported peer is followed again after a reconnect.** When a Status arrives
  that the peer had not sent at the Unsupported verdict (none then, or a different one), the peer
  gets a new machine and a first contact. Statuses are forgotten on a disconnect, so a reconnect
  that the peer set did not show looks like this. A Status present at the verdict proves nothing.
- **(T4.2 fix) `Reconciling → Striped` (`eventLargeDiff`) is unused for now.** It stays in the
  table so that the table matches §8.1. T4.5 may route a session's large range to the session's
  own peer if that pays.
- **(Orchestrator, pre-T4.3 check, 2026-10-19) Index rebuild time accepted at 26.5 s.** The
  T2.2/T2.6 follow-up ("rebuild ≤ 20 s at 4.2M, due before T4.3") had no task owner. The user
  accepted the measured 26.5 s at 4.2M (single-threaded, in-memory IDs, reference host) instead of
  a by-bucket parallel build. It is the sync-unavailable window on a first start (no live slab),
  under the 120 s start-up guard. The follow-up is closed; the storer-iterator rebuild time on a
  real node is still worth reading off T6.3's testnet runs.

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
- **(T4.2) Stripe registry trimming (T4.3).** A range that overlaps a registered one is still
  dropped, not trimmed (T3.2 part 2). For prefix ranges, an overlap is a containment. Trimming
  would split the larger range into the uncovered siblings, which multiplies jobs and the
  bootstrap and radius bookkeeping (`started`, `cancel`). That is stripe scheduling, not per-peer
  state.
- **(T4.2) Compare session-root summaries (T4.3).** `sameDigest` still treats different roots as
  different (T1.11). Comparing needs a `Service` method for the local summary of a range. The
  peer's Status carries only its own root's summary, so this works only when the session root is
  the peer's root. The "digest differs" decision belongs to the scheduler.
- **(T4.2) Escalate Status spam (T4.5).** The drops happen in `rangesync`'s `handleStatus`, which
  the reconciler never sees. Escalation needs a counter there that feeds `PeerScorer`. T4.1
  decided to escalate only if testnet shows persistent spam.
- **(T4.2) Stripe peer choice ignores the backoff (T4.3).** `PlanStripes`, `nextStripePeer` and
  `verifier` do not look at a peer's Busy streak. The scheduler could prefer peers that are not
  backing off.
- **(T4.2) First Busy retry on topology rounds (T4.3).** The first Busy has no extra wait, so a
  round that a topology change triggers can retry a Busy peer within milliseconds. T4.3's
  scheduler should rate-limit rounds; otherwise raise the first backoff step.
- **(T4.2) `peers{state}` gauge (T4.5).** No §10 metric covers it, but it would show testnet
  operators (T6) how many neighbors are Striped, backing off or dropped.
- **(T4.2) Tests to run with `-race` on CI.** These ran with `-count=10..40 -cpu=1,2,4` instead.
  - `pkg/rangesync/reconciler`: `TestPeerState*` (especially `TestPeerStateSessionSlotExclusive`
    and `TestPeerStateStripeWaitsForSession`), `TestPeerFSM*`, `TestStatusGossipBackoff`,
    `TestRadiusHalfStripedWhenCovered`.
  - Re-run the earlier lists with `-race`, because `round`, `session`, `probe` and `transferStripe`
    changed: `TestRadius*`, `TestTransferStripes*`, `TestVerifyStripes*`, `TestRoundBootstrap*`,
    `TestStartupGuard*`, `TestSyncRate*`, `TestStatusGossip*`, `TestRoundOwnsFirstContact`,
    `TestPeerJoinLeave`, `TestClose`.
- **(T4.2 fix) Local store errors still count towards the backoff (T4.5).** `ReconcileWith` wraps
  local store errors and stream errors alike, so a failing local store backs off every peer.
  Telling them apart needs an error class in `pkg/rangesync`.
- **(T4.2 fix) More tests to run with `-race` on CI.** These ran with `-count=10 -cpu=1,2,4`
  instead.
  - `pkg/rangesync/reconciler`: `TestStartupGuardBusyPeer`, `TestPeerStateStripeDuringFetch`,
    `TestPeerStateWaiterWoken`, `TestPeerStateLocalCauseNoBackoff`,
    `TestPeerStateUnsupportedReconnect`, `TestPeerStateReadmittedBackoff`, and
    `TestVerifyStripes*` again (`waitAdmitted`).
  - `pkg/rangesync`: `TestReconcileInitiatorSessionLimit`.
