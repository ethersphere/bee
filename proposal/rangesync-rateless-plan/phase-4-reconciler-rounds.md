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

## T4.7 Hardening: G6 constants and pre-testnet blockers (added at G6, 2026-10-21)

**Depends on:** T4.6 (G6 decided). **Blocks:** T6.1 (AC-B2 is a T6.1 entry blocker) and any testnet
cohort that enables `rangesync`.

**Scope (each item lands as its own part, verified and reviewed separately):**
1. **AC-B2 bootstrap listing (first; T6.1 entry blocker).** A peer whose session finds an empty or
   near-empty node lists its whole reserve to it, and repeats every round (measured 1.4–3.8× over the
   AC-B2 bound; ≈ 294 MB per session at 2^22). Fix within constraints (a)–(d) of the T4.6 part 3
   round-2 AC-B2 entry, taking the round-2 review notes into account (only candidate A closes every
   case; B and C don't combine). Acceptance: the scaled case (test-settable `LargeDiff` or a Base large
   enough for stripes above `LargeDiff` and the child-range stripe path), including a
   responder-only-lacking-node case; AC-B2 within bound.
2. **G6 constants.** `GrowthFactor` 4 → 2, with the four tests that encode growth 4
   (TestProtocolConverges/4k, TestWorkedExamples, TestSREPSessionCost, TestSREPIdeal) and a follow-up
   listing the proposal text to update (L467, L824, L830, L894, L1175, L1191–1194); a per-node
   initiated-session cap replacing the constant `MaxInitiatedSessions` 8 (the constant 16 only fits
   N ≤ 17; keep room for inbound sessions and stripes); `LargeDiff` settable in tests. Caches,
   `BatchFactor` 1.6, `CatchUpMinDiff` 256 and `RoundInterval` 5–30 s stay. Re-measure AC-B1 at
   N = 9 / 17 with the harness after the change.
3. **Stripe-registry `SyncRate` pin.** The registry hold is unconditional, so a higher-overlay peer
   claiming a count difference above `LargeDiff` pins `SyncRate` for free. Hold for a stripe only while
   its job stores chunks, past a grace (as T4.5 part 2c did for fetches).
4. **Per-peer fetch credit that survives reconnects.** A peer's share of the fetch rate refills only
   with chunks it delivered; keyed so a reconnect doesn't reset it. Bounds every junk-lister path at
   once (residual today ≈ 70 junk IDs/s per connected Sybil, ≥ 100 with reconnect cycling).

**Tests:** each item's acceptance above; honest-churn tests for items 3–4 (no honest peer scored or
starved); `go test ./pkg/pullsync/... ./pkg/puller/... ./pkg/storer/...` if pkg/node or pkg/storer is
touched.

**Size:** L (four parts). Not in scope: the sustained-overwrite `SyncRate` pin (a T6.3 item and a fix
before mainnet), AC-B1 re-basing at N = 9 (T6.4).

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
- **(T4.3) Round scheduler in `reconciler/scheduler.go`.** It keeps the Phase-1 loop structure
  (round timer plus topology wakes, T4.5 replaces it) and adds:
  - *Takeover.* The lower overlay still initiates. The higher side starts its own session when
    the digests have differed for `2 × RoundInterval + jitter` (jitter: a per-peer fraction of one
    interval) with no session of the initiator with this node. The wait is measured against the
    current interval, so it shrinks with it while the mismatch persists. It restarts when the
    initiator completes a session or lists wants with this node (`Succeed`/`Wants`), and when a
    takeover gets `ErrPeerSessionLimit`: on this node that slot can only be held by the
    initiator's own session, so the takeover goes back to Idle with no backoff. An equal digest,
    a lost Status or ineligibility ends the wait. A takeover runs under the same rules as an
    initiated session (backoff, stripe waiters, bootstrap, radius opening).
  - *Adaptive interval.* After each round the interval halves (floor `MinRoundInterval = 5 s`)
    when any eligible, exchanged peer's digest differed, and doubles (cap `MaxRoundInterval =
    30 s`) otherwise, including rounds with no eligible peer. It starts at 5 s. Gauge
    `round_interval_seconds` (the T6.0 follow-up). `Options.RoundInterval` now fixes the interval
    (min = max) for tests; the exported constant `reconciler.RoundInterval` is gone (no user
    outside the package) in favor of `reconciler.MinRoundInterval`/`MaxRoundInterval`.
  - *Triggers.* A changed peer Status, a root change of this node (the initiator reacts to its
    own new chunks, not only its peers'), and a gossip send that delivered a root the peer lacked
    trigger a round. A trigger runs no sooner than the minimum interval after the last round, so
    triggers never exceed a peer's session rate (`PeerSessionInterval = MinRoundInterval / 2`).
    The round timer stays on the real clock as before; triggers follow `Options.Now`/`After`.
    Topology changes still run a round at once.
- **(T4.3) The node-wide cap is shared through `MaxInitiatedSessions = MaxConcurrentSessions / 2 =
  8` slots.** These cover every session this node opens: round sessions, takeovers, stripe
  transfers and verifications. rangesync's 16 tokens are shared by served and initiated sessions,
  so the other half stays for the sessions peers open with this node. With the lower-overlay rule,
  the lowest node of a neighborhood initiates with everyone, and without a reconciler cap it would
  take all 16 tokens and answer every peer Busy. A session waits for a slot, first come, first
  served, in its peer's Reconciling or Striped state, so a round never starts two sessions with one
  peer, and the start-up guard counts the waiting sessions as unresolved. A stripe takes its
  peer first and then a slot, so it never holds an idle slot while it waits for the peer. A stripe
  frees its slot before a Busy retry. A round no longer gets `ErrNodeSessionLimit` from its own
  sessions; served sessions can still cause it (a local cause, no backoff). This covers the
  T3.2 part 2 / T3.4 carry-over: T4.2 made a peer's round and stripe sessions exclusive, and this
  shares the node cap across peers.
- **(T4.3) Rounds do not read one index snapshot; T1.6's locking cannot give one.** `Commit` holds
  `ix.mu` in *read* mode across `run` and the apply, and writes the slab under `slabMu`.
  `Summary`, `RootSummary` and `Symbols` take `ix.mu` (R) and `slabMu` (R) per call. A round-wide
  `ix.mu.RLock` would therefore block only rebuilds, not deltas. A real snapshot would need a copy
  of the slab per round (8 MB of `LtHash` at B = 4096) or copy-on-write in the Index, which G1
  rules out ("`Commit` is unchanged"). What holds instead:
  - the round decides on one `LocalStatus` snapshot;
  - each symbol stream reads one fixed set (T2.4's stream snapshot), so churn never corrupts a
    decoder;
  - a delta applied during a round changes the root summary, which the Index signals. That
    gossips the new Status and, since T4.3, triggers the next round.

  So a delta reaches the current round's later sessions or the next round, and is never lost.
  Two sessions of one round can see different sets, so a chunk added mid-round may be fetched by
  one session and listed by the next. The want set (T4.4) de-duplicates the fetch. §8.2's "the
  index is under `RLock`" is not what T1.6 built; the proposal text should say "each symbol stream
  reads one snapshot" (follow-up below).
- **(T4.3) Moved-root race and sender-rate deferral (T4.1 follow-ups).** `peerState.reached` keeps
  the root of the last Status of ours that reached the peer. A round starts no session, initiated
  or taken over, with a peer whose `reached` is not this node's current root, and counts it as
  unresolved for the guard. A send in flight (gossip) or one the sender-side rate deferred no
  longer causes a refused session. The gossip pass that delivers the root triggers the round.
  Failed heartbeats and pushes still keep sessions (T4.1 fix): only the root matters for the
  session scope.
- **(T4.3) A received Status wakes the reconciler (T4.1 follow-up).** New additive API in
  `pkg/rangesync`: `Service.SubscribeStatus()` signals, coalesced (buffer of one), when a peer's
  recorded Status differs from its last one or is its first. Heartbeats that repeat a Status are
  not signaled. The reconciler takes it through an optional interface (`statusSubscriber`, with a
  compile-time check against `*rangesync.Service`), so the narrow `reconciler.Service` contract
  and `pkg/node` are unchanged. A signal wakes the gossip loop at once (a peer that became
  eligible from its side gets our changed summary without waiting for the heartbeat) and triggers
  a round.
- **(T4.3) Stripe peer choice avoids peers in backoff (T4.2 follow-up).** The stripe worker plans
  with the admitted peers whose `peerFSM.retryAt` has passed, and falls back to every admitted
  peer when that leaves a stripe without a peer. `nextStripePeer`'s fallback and `verifier` take
  such peers first. Stripe Busy answers still do not count towards the backoff.
- **(T4.3) Open question 8 (pushsync trigger): skipped, needs `pkg/pushsync`.** pushsync exposes no
  hook for a delivery to this node that failed. Its only per-chunk callbacks are `unwrap`,
  `gsocHandler` and `validStamp`, which `pkg/node` passes in. A stamp failure is rejected by
  rangesync too, so triggering on it gains nothing, and storer or forward failures are visible
  only inside pushsync's handler. Wrapping the storer in `pkg/node` would observe `Put` failures
  of every protocol, not pushsync deliveries. No constant was added. The status and root-change
  triggers above already start a round within `MinRoundInterval` of the Status change that a
  pushsync delivery to a neighbor causes.
- **(T4.3) Session memory budget of the symbol path (T2.1 follow-up).** New constants in
  `pkg/rangesync/constants.go`: `DecoderBytesPerID = 200`, `SessionDecoderMemory =
  SessionSymbolBudget × 88 B + 2 × SessionSymbolBudget × DecoderBytesPerID` (1.4 + 6.25 MiB) and
  `NodeDecoderMemory = MaxConcurrentSessions × SessionDecoderMemory` (about 122 MiB). A session
  receives at most `SessionSymbolBudget` symbols, and a decoder recovers at most two IDs per
  symbol (else `ErrCorrupt`). `TestSessionDecoderMemory` measures about 110 B per recovered ID
  beyond the symbols on amd64 and checks the bound. The godoc lists what the budget does not
  count and where each part is bounded:
  - stream prefix copies, at most cap(R) per open stream;
  - `MaxPendingReplies`;
  - wants, bounded by `LargeDiff` through the Striper;
  - one Message each way.

  `Local.Symbols` session cancellation was not cheap; see Follow-ups.
- **(T4.3 fix, M1) Stripes take at most `MaxStripeSessions = MaxInitiatedSessions / 2 = 4` slots,
  and their timeout starts once they have them.** A stripe session holds its slot for the whole
  `ReconcileWith`, which the fetch queue backpressures for up to `sessionTimeout` (10 min). Before
  this fix, eight stripes could hold every initiated slot for that long, round sessions waited in
  Reconciling, and a later wave of stripes lost its 15-min timeout while queuing. Now:
  - `transferStripe` takes a stripe slot, then a session slot, and only then starts the stripe
    timeout. Waiting for the peer (`acquireStripe`) still counts towards the timeout, as T4.2
    decided.
  - It holds both slots across its Busy retries and frees the session slot as soon as the session
    ends, before draining the fetch.
  - Round sessions take only a session slot, so at least four always remain for them.
  - Stripes are bounded by the global fetch limiter (1000 chunks/s) anyway, so four in parallel
    lose little throughput. The bootstrap of an empty node in an N = 17 neighborhood runs its 16
    stripes in four waves.
- **(T4.3 fix, M2) The interval adapts on comparable digests and on what sessions found.** The
  digest of a peer whose root differs from this node's covers another range. Two eligible peers
  with nested roots (radius off by one) always "differed", which kept the interval at 5 s. A
  round now counts as finding differences when either:
  - a peer with the *same* root has a differing digest, or
  - a session since the last round found differences: a completed initiated session with wants or
    ranges for the Striper, or wants a peer listed in a session this node served (`foundDiff`).

  A nested-root pair in sync therefore lets the interval grow to 30 s
  (`TestRoundIntervalNestedRoots`). Such a peer still gets a session every round, since the
  digests cannot tell sync. With the interval at its cap that is once per 30 s, plus rounds
  triggered by its Status changes. Comparing session-root summaries stays deferred (Follow-ups):
  it would only help the side with the wider root, because the peer's Status carries only its
  own root's summary.
- **(T4.3 fix, L1) A local `ErrPeerSessionLimit` never backs off, and a takeover answered Busy
  waits for the initiator again.** This supersedes the T4.2 fix entry's "the backoff counts
  `ErrPeerSessionLimit`". Round and stripe sessions of a followed peer exclude each other through
  its state machine, so a taken slot on this node means the peer runs its own session with this
  node, and is alive. In either role the session goes back to Idle with no backoff
  (`eventAborted`); a takeover also restarts its wait. A takeover answered `ErrPeerBusy` does the
  same: a peer that answers is not the unresponsive initiator the takeover covers. In the
  simultaneous open, both responders answer Busy:
  - the takeover side yields;
  - the initiator counts a first Busy, which the next round retries with no wait, so it keeps the
    initiator role.

  `TestSessionCollision` covers both roles and both orders. The T4.2 tests
  `TestPeerStateSessionBackoff` and `TestPeerStateLocalCauseNoBackoff` now use `ErrPeerBusy` for
  the peer answers that back off.
- **(T4.3 fix, L2) A session rechecks the round's decision once it has its slot.** It may have
  waited minutes. `recheckSession` requires, under `r.mu`:
  - the entry is the peer's current one and still Reconciling;
  - this node's root is unchanged since the round and has reached the peer;
  - the peer's Status is known and its digest still differs;
  - for a takeover, the initiator ran no session with this node since the decision.

  Otherwise the session frees its slot and ends with `errStaleSession`: no backoff, the guard
  disarmed until the next round, and a new `sessions_total{result="stale"}`. This closes the
  moved-root race for waiting sessions and keeps the sessions of stale entries from piling up on
  peer flaps.
- **(T4.3 fix, L3) Test determinism.** New `Options.Jitter` (the takeover jitter fraction,
  `rand.Float64` if nil); the takeover tests check the exact deadline with a fixed jitter. The
  round timer now runs on `Options.After` unless `Options.RoundInterval` fixes the interval. That
  option keeps the real clock so the frozen-clock tests that set it still get timed rounds. A
  started reconciler with the adaptive interval (`newScheduledHarness`) therefore gets no round
  the fake clock does not schedule.
- **(T4.3 fix round 2) A stripe frees both slots while it waits to retry Busy, its timeout runs
  only while it holds them, and its Busy retries are bounded.** This supersedes the
  "(T4.3 fix, M1)" entry's "holds both slots across its Busy retries". That rule let four peers
  that always answer Busy (Busy is free and unscored, and stripes do not back off) hold every
  stripe slot for 15 min per wave; honest peers at their session cap or rebuilding their index did
  the same. Now:
  - Before each 1 s retry wait, `transferStripe` frees its stripe slot and its session slot, then
    takes both again (`acquireStripeSlots`).
  - The stripe timeout is a pausable clock (`stripeClock`) that runs only while the attempt holds
    its slots: the peer wait, the session and its Busy answers. This is consistent with M1.
  - With the clock paused an always-Busy peer would be retried forever and block the stripe
    worker's job, so an attempt gives up after `maxStripeBusyRetries = 30` Busy retries (about
    30 s, longer than an index rebuild). runStripe then reassigns the stripe (at most
    `maxStripeReassign`).

  `TestStripeBusyDoesNotStall` (an honest stripe runs at once behind four always-Busy ones),
  `TestStripeBusyRetriesBounded` and `TestStripeTimeoutPausedWhileBusy` cover these.
  `TestStripeTimeoutAfterSlot` now holds every session slot with round sessions, so the stripe
  that waits is deterministic.
- **(T4.3 fix round 2) The takeover wait of a peer with another root uses `MaxRoundInterval`.**
  After the M2 fix, the lower node of a nested-root pair can reach 30 s while the higher node stays
  at 5 s for its other peers. With waits measured against its own interval, the higher node took
  over every 10–15 s while the initiator was alive. Digests of different roots cannot show
  whether the two are in sync, so for such a peer the wait is `2 × MaxRoundInterval + jitter`
  (60–90 s), independent of the local interval. Same-root peers see the same digests on both sides
  and keep the current interval. `TestRoundTakeoverNestedRoots` covers this.
- **(T4.3 fix round 2) Test gaps.** A stale session is now asserted to end with no backoff streak
  and to count in `sessions_total{stale}`, and a follow-up round runs its session at once
  (`TestSessionRecheckAfterSlot`). Wants listed in a served session keep the interval at the floor
  (`TestRoundIntervalServedWants`). New test exports: `SetStripeRetryInterval`, `SessionsTotal`,
  `PeerFailures`.
- **(T4.3 fix round 3) The stripe timeout bounds the fetch after the session again.** This
  supersedes the round-2 entry's "runs only while the attempt holds its slots" for the end of an
  attempt. The clock still starts once both slots are held and pauses while the attempt waits to
  retry the peer's Busy. It is no longer paused after the session: it runs on while the listed IDs
  are fetched, without the slots, as `StripeTimeout` documents. The round-2 code paused it
  there, so a peer that kept delivering slowly could hold the stripe worker and the Striped peer
  for up to `sessionTimeout` per queued batch instead of 15 min. Tested by
  `TestStripeFetchBoundedByTimeout`.
- **(T4.3 fix round 3) Busy causes on this node neither count towards `maxStripeBusyRetries` nor
  reassign the stripe.** `localBusy` covers this node's index rebuilding (`fingerprint.ErrBusy`)
  and its session slots being taken (`rangesync.ErrSessionLimit`: the node cap, or the peer's own
  session with this node). The attempt still frees its slots and waits `stripeRetryInterval` before
  each retry, but the stripe clock keeps running through those waits, so they end at the stripe
  timeout. runStripe restarts such an attempt with the same peer, counted in
  `maxStripeRestarts`, before it reassigns. The error then wraps both `ErrStripeTimeout` and the
  local cause. The stripe stays bounded: at most `(maxStripeRestarts + 1) × StripeTimeout` on one
  peer, then the usual reassignment. Before this, a local rebuild longer than about 2 min failed a
  concurrent radius-half stripe by reassigning it away from healthy peers. `TestStripeLocalBusy`
  covers it (more local Busy answers than capped attempts and restarts could take, then
  always-Busy-here with a short timeout), and `TestStripeBusyReassigns` covers the peer-Busy
  reassignment (Peer = B, Tried = 2, one reassignment, Busy peer unscored).
- **(T4.3 fix round 3) Stronger tests.** `TestStripeBusyDoesNotStall` now holds the round share of
  the session slots, so it fails unless a Busy stripe frees its session slot as well as its stripe
  slot. `TestStripeTimeoutPausedWhileBusy` also checks that held time adds up across Busy answers
  (100 ms per answer, so a 300 ms timeout expires), which catches a clock that restarts its
  budget on resume. Two "don't regress" tests had synchronization gaps that earlier T4.3 rounds
  exposed (1–2 failures in 120 stress runs, also at a9fa2066). Both are fixed in the tests only:
  - `TestPeerStateWaiterWoken` waits for the round session to be past its slot and recheck;
  - `TestRadiusIncreaseNoFetch` runs one more round once the peers hold the new root, since a
    gossip pass can have it in flight during the test's single round, and the follow-up round is
    deferred by the test's 1 h interval.
- **(T4.4) Rejected cache in the want set.** `wantset.Set` gains `Reject(id)`, `IsRejected(id)`
  and `RejectedLen()`, and `Claim` returns the new `Rejected` (checked after `Closed`, before
  `Held`). `Reject` ends the claim on the ID whichever peer holds it, as `Done` does: the ID
  includes the stamp, so the verdict holds for every peer. Rejecting again restarts the TTL.
  Expiry is passive, like the claims'. `MaxRejected = 10000` IDs, the oldest dropped first: ten
  seconds of the node-wide fetch rate; a larger rejected set takes longer than that to fetch, so
  the fetch limiter bounds its refetches. Measured about 270 B per ID, so about 2.7 MB at the cap
  (same as the want set). `New(claimDeadline, rejectTTL)`; its only production call site is
  `rangesync.New`. In `Fetch`:
  - a stamp the stamp validator rejects ends with `Reject` (was `Release`);
  - `ErrOverwriteNewerChunk` ends with `Reject` (was `Done`; this supersedes T1.7's "`Done` after
    `ErrOverwriteNewerChunk`");
  - rejected IDs are dropped before the fetch limiter wait, so they cost no tokens.

  `TestFetchInvalidStamp` now expects the rejected ID not to be claimable again (by design).
- **(T4.4) `FetchWithResult` (T3.2 part 2 carry-over).** Additive rangesync API; `Fetch` wraps it.
  `FetchResult` has:
  - `Stored`;
  - `Undelivered`: the IDs whose claims the fetch released. These were not delivered, delivered
    invalid, in a Want that failed or timed out, or failed to store on this node;
  - `Held`: IDs the want set denied because another fetch held the claim;
  - `Rejected`.

  IDs outside the radius, already in the reserve, refused as `PeerFull`/`Full`, or not reached
  after an error are in no part. The reconciler uses it through an optional interface
  (`resultFetcher`, compile-time checked against `*rangesync.Service`), as T4.3 did with
  `statusSubscriber`. So `reconciler.Service`, its fakes and `pkg/node` are unchanged. Without
  it, the old rule (every ID not stored is undelivered) applies.
- **(T4.4) Withholding is judged on undelivered IDs.** A stripe is withheld when
  `Undelivered > 0` (was `Stored < Listed`). A verifier is scored when `Undelivered` is above the
  tolerance (was `Diff − Stored`). New fields `StripeResult.Undelivered` and
  `VerifyResult.Undelivered`. These are no longer blamed:
  - foreign want-set claims and rejected stamps;
  - IDs stored meanwhile (pushsync or pullsync);
  - IDs outside the radius.

  The deliverer check (verifier-Stored above the tolerance) is unchanged.
  `TestStripeWithheldUndeliveredOnly` and `TestVerifyStripesUndeliveredOnly` fail under the old
  rules.
- **(T4.4) `id → peers` retries (§7.8) in `reconciler/wants.go`.** Only denied (`Held`) IDs are
  recorded, with the peers they were denied to. Only those need another source when the claim's
  fetch fails. A peer that listed an ID but whose fetch has not reached it yet claims it itself.
  - *Lifetime.* An ID is kept for `ClaimDeadline` after its first denial ("known this round":
    after that, the denied peer's next session claims the ID itself), at most `wantset.MaxClaims`
    IDs.
  - *Trigger.* `FetchResult.Undelivered`. This includes a stall: the fetch timeout
    (`ClaimDeadline / 2`) ends a stalled fetch before its claims would expire, so passive expiry
    needed no hook.
  - *Choice.* The first peer denied, still connected and not Unsupported, other than the failing
    one. It is popped, so a retry that fails too goes to the next source.
  - *Concurrency.* Retries run in `r.wg` goroutines, one retry fetch per peer at a time
    (`peerLocks`, which now forgets an unused peer's lock). So retries add at most one fetch
    stream per peer.
  - *SyncRate.* A catch-up fetch's retries are catch-up fetches (they hold `catchUpFetches`).
  - *Granularity.* Results come per `Fetch` call, not per Want. A denied ID claimed early in a
    long fetch is retried when that fetch returns, or, past the lifetime, left to the next round.

  Tests: `TestSREPFetchedOnce` and `TestSREPStallRetry` are §7.8 literally, with real services, the
  real want set and fetch timeout, and reconciler rounds. B1 stalls, so x comes from B2 after the
  timeout and y (only B1 has it) waits. The `TestWantSet*` unit tests cover the chain, a peer that
  left, the window and catch-up. Without the retry call, `TestSREPStallRetry` fails.
- **(T4.4) Claim-and-stall goes through the single `PeerScorer`; no new code.** rangesync's fetch
  timeout already reports a stalled holder to `Options.Scorer` (the reconciler) and disconnects
  it. `TestSREPStallRetry` asserts B1's score is 1. A peer that closes the fetch stream without
  some listed IDs is not scored: T1.10 treats eviction between session and fetch as honest. Its
  IDs are retried instead (see Follow-ups).
- **(T4.4) `want_claims{result}` (T6.0 carry-over).** Two kinds of count:
  - once per ID offered to the want set: `granted`, `denied` (`wantset.Held`), `peer_full`,
    `full`, `rejected` (the pre-claim check and `Claim`) and `closed`;
  - once per granted claim the fetch finds already ended, when a delivery arrives or when it
    releases the claim: `expired`. That is an expiry, or, after one, another fetch's `Done` or
    `Reject`. `late` counts those whose chunk was delivered anyway, so it is a subset of
    `expired`.

  Detection uses the additive `Set.HeldBy(id, peer)`. With the defaults a claim cannot expire
  during its fetch: `FetchTimeout = ClaimDeadline / 2` bounds each Want from the claim to the
  last delivery. So `expired` and `late` stay 0 unless the fetch timeout is raised, and
  intra-`rangesync` duplicates are at most `want_claims{expired}` (AC-D1). A closed service
  counts nothing. `TestFetchLateDelivery` and `TestFetchExpiredClaim` run with a 50 ms deadline.
- **(T4.4) Open question 7 (expired-batch skew): what the tests show.** The rejected cache stops
  per-round refetches. `TestRejectedNotRefetchedEachRound` (real services, reconciler rounds)
  covers a stale stamp (§7.7, `ErrOverwriteNewerChunk`) and an evicted-batch stamp
  (`validStamp` fails). Each is fetched once over four rounds; without the cache it is 4 Wants
  in 4 sessions. The ID is skipped through every peer and fetched once more after `RejectTTL`
  (`TestFetchRejectedNotRefetched`, `TestFetchRejectTTL`). What the cache cannot do:
  - *Prevent the first fetch.* Wants carry no batch ID (nil `BatchID`; G2 froze the wire). So a
    node that evicts an expired batch before its neighbors fetches every chunk of it that a
    session lists, once, and rejects it. That is up to skew × fetch rate deliveries per early
    node, for example 60 s × 1000/s = 60k × 4 KiB ≈ 240 MiB.
  - *Hold a large set.* At the full fetch rate the 10000 IDs fill in 10 s. While the skew lasts,
    later rounds refetch the oldest rejections, bounded by the fetch limiter.
  - *Resist a peer.* A peer can flush the cache by delivering invalid stamps for IDs it listed
    (`validStamp` runs before the CAC check, unpenalized; T1.10).
  - *Help a lagging node.* A node more than 10 blocks behind on chain events (bee's batch
    usability threshold) rejects a new batch's chunks for `RejectTTL`.

  The measurement under skew goes to T4.6 (Follow-ups).
- **(T4.4) Scope notes.** No `pkg/node`, `go.mod` or proto change. No new parser of peer input, so
  no `followup-fuzzing.md` line. The shared test fake gains `wantIDs` (per-peer want IDs).

- **(T4.4 fix round 1, High) The reserve is checked again after the claim.** This supersedes the
  "(T4.4) `want_claims{result}`" entry's claim that intra-`rangesync` duplicates are at most
  `want_claims{expired}`; that was false. `Fetch` checked the reserve, then waited for the fetch
  limiter (up to 250 ms at the default rate), then claimed. Meanwhile another fetch could claim
  the ID, store it and end its claim; the claim was then granted and the chunk delivered twice.
  That duplicate showed only in `cross_protocol_duplicates_total`, with no expired claim.

  Now `claim` checks `ReserveHasID` for each granted ID and ends the claim (`Done`) without
  requesting a chunk that is already stored. A claim ends with `Done` only after `Put`, so an ID
  that is claimed and not in the reserve is requested by no other fetch until the claim ends.
  An intra-`rangesync` duplicate therefore needs a claim that expired during its fetch.

  With the default timeouts that cannot happen: `FetchTimeout = ClaimDeadline / 2` bounds each
  Want from the claim to its last delivery. A stall is ended by that timeout and its stream reset,
  so no delivery comes late. So `want_claims{expired}`, `want_claims{late}` and intra-`rangesync`
  duplicates are 0 by construction, faults included. They become non-zero only if
  `FetchTimeout` is raised to `ClaimDeadline` or more.

  The cost is one more reserve lookup per granted ID (at most the fetch rate).
  `TestFetchClaimRechecksReserve` drains the limiter, stores x through another claim while the
  fetch waits, and checks that x is not requested again. It fails without the re-check.
- **(T4.4 fix round 1, Medium) Deferred and unresolved held IDs fail a stripe without blame.**
  This supersedes "(T4.4) Withholding is judged on undelivered IDs" where it let such a stripe
  end as done. Before this fix, IDs the want set refused (`PeerFull`, `Full`) and held IDs whose
  holder failed were counted nowhere. The stripe ended done, which cleared `bootstrapFailed`,
  marked the radius half done, and could get an honest deliverer blamed by the verification.
  One peer could also arrange it: hold the claims through our fetches from it, stall just under
  the timeout, then close (unscored, and never its own retry source). Now:
  - `FetchResult.Deferred` reports IDs refused as `Full` or `PeerFull`, and stamp checks that may
    pass (Low 1).
  - After its session's fetches, a stripe asks its peer again for the IDs still held or deferred,
    every `stripeRetryInterval`, up to `maxPendingPasses = ClaimDeadline / stripeRetryInterval`
    times (30). It does so without slots, on the stripe clock. An ID another fetch stored is
    skipped. One whose claim ended without the chunk is claimed for the stripe peer, which must
    then deliver it or be found withholding. This closes the evasion above.
  - IDs still pending fail the stripe with `ErrStripeDeferred` (`StripeResult.Deferred`). It is
    restarted with the same peer within `maxStripeRestarts`, then reassigned as usual. It is never
    scored and never verified: `VerifyStripes` verifies only done stripes, and a verification
    that ends deferred returns before any scoring.
  - Withholding takes precedence over deferral: undelivered IDs still reassign.
  - Rejected IDs count towards completion. They are deliberately not fetched (an invalid stamp, or
    an older one than this node holds), and the rounds fetch them after `RejectTTL` if they are
    still listed.

  Tests: `TestStripeDeferred` (held, peer full, full: 1 + `maxStripeRestarts` sessions, no score,
  not verified) and `TestStripeDeferredResolved` (a released foreign claim is fetched within the
  attempt; the peer's own claim, then withheld, is found withholding). Also reworked:
  `TestStripeWithheldUndeliveredOnly` (the held ID is now stored by its holder) and
  `TestVerifyStripesUndeliveredOnly` (a held ID makes the verification deferred and blames
  nobody).
- **(T4.4 fix round 1, Low 1) Only errors about the stamp itself reject an ID.** This supersedes
  the "(T4.4) Rejected cache" entry's "a stamp the stamp validator rejects ends with `Reject`".
  `stampRejected` covers what `postage.ValidStamp` returns for the stamp: `postage.ErrNotFound`
  (an unknown or expired batch), `ErrInvalidChunkAddress`, `ErrBucketMismatch`,
  `ErrInvalidIndex` and `ErrOwnerMismatch`. It also covers `crypto.ErrInvalidLength` and the
  missing-validator error. Any other error (a batch store failure) releases the claim and defers
  the ID; it says nothing about the peer.

  A signature that fails secp256k1 recovery is deterministic too, but has no sentinel without a
  new import. It is treated as one that may pass, so such a stamp is fetched again each round, as
  before T4.4.

  `TestFetchInvalidStamp` is back to its original assertions (its generic error is now a
  may-pass error). `TestFetchStampCheckFailure` covers each class. The help of
  `fetch_rejected_total` now reads "the stamp validator failed".
- **(T4.4 fix round 1, Low 2) Retry sources outlive the fetch that holds the claim.** This
  supersedes the *Lifetime* and *Granularity* bullets of "(T4.4) `id → peers` retries". A source
  is kept for `ClaimDeadline` after its last denial (a re-denial refreshes it). It is also kept
  while any fetch that started before that denial is running: `fetchFrom` registers each fetch
  for its duration. The claim's own fetch started before the denial, so its sources survive until
  its result arrives, however long it runs. That covers up to `LargeDiff` IDs at the fetch rate
  (about 65 s), where the old 30-second window from the first denial lost most retries of large
  catch-up fetches. Memory stays at most `maxSourceIDs`, the oldest dropped first.

  `TestWantSetRetryLongFetch` replaces `TestWantSetRetryWindow`, whose assertion (no retry after
  the claim deadline) is now intentionally reversed. `TestWantSetSourcesPruned` covers the drop
  once no older fetch runs.
- **(T4.4 fix round 1) `want_claims` coverage.** `TestFetchWantSetFull` checks `peer_full`, `full`
  (`Deferred`) and `closed` (no part, no Want).

- **(T4.4 fix round 2, Medium) Bad signatures are rejected, and only want-set refusals are asked
  for again.** This supersedes two round-1 entries. The Medium entry said `FetchResult.Deferred`
  also held stamp checks that may pass. The Low 1 entry said a signature that fails recovery is
  treated as one that may pass. That opened an unblamed stall: `postage.ValidStamp` returns the
  raw secp256k1 error for such a signature before its sentinel checks. So a stripe peer could list
  made-up IDs on a real batch and deliver junk; the IDs were deferred and asked for 30 times per
  attempt, each pass charging the fetch rate, then the stripe restarted 8 times, unscored and
  unverified. Now:
  - On the may-pass branch, `deliver` calls the exported `postage.RecoverBatchOwner(addr, stamp)`
    and rejects the ID when it fails. No `pkg/postage` change is needed (the round-1 follow-up is
    superseded below).
  - `FetchResult.Deferred` holds only `Full`/`PeerFull` refusals. A new `FetchResult.Unverified`
    holds IDs whose stamp check failed for a cause on this node (a batch store error). Their claims
    are released.
  - `resolvePending` asks again only for the IDs the want set held or refused. A peer cannot make
    the stripe ask by how it delivers.
  - Unverified IDs fail the stripe with `ErrStripeDeferred` (counted in `Deferred`): no blame, not
    verified, so a verifier cannot blame the deliverer for chunks this node failed to check. They
    are asked for once per restart. Only genuine local failures reach that path.

  Tests: `TestStripeBadSignatures` (real services: 8 made-up IDs with bad signatures plus 2 valid
  chunks; the stripe completes in one session with one Want and the bad IDs rejected; it fails
  without the recovery check), `TestStripeUnverified` (one fetch per attempt, none asking again),
  `TestFetchBadSignatureRejected`.
- **(T4.4 fix round 2, Low 1) The real bound on expired claims.** This supersedes round-1 High's
  "0 by construction, faults included". `FetchTimeout` bounds a Want's reads, but `deliver`'s
  `Put` ignores ctx. It takes the reserve's per-batch lock (`multx`), which `EvictBatchBin` holds
  for a whole batch eviction. So a claim can expire during its fetch when the Want plus one
  `deliver` outlasts `ClaimDeadline`, that is, when one `Put` blocks for more than
  `ClaimDeadline − FetchTimeout` (15 s by default). The next fetch's reserve re-check then
  misses, and the chunk is delivered twice. Now:
  - `deliver` checks the claim again after `Put`; a claim that expired meanwhile counts as
    `want_claims{expired}` (not `late`: the delivery arrived in time). So intra-`rangesync`
    duplicates are at most `want_claims{expired}` in every case, and are 0 unless a `Put` blocks
    that long.
  - `rangesync.New` clamps a `FetchTimeout` of `ClaimDeadline` or more to `ClaimDeadline / 2`.

  Tests: `TestFetchExpiredDuringPut` and `TestFetchTimeoutClamped`.
- **(T4.4 fix round 2, Low 2) `Peek` before the fetch limiter.** The want set gains
  `Peek(id, peer) Result`, which shares `Claim`'s checks and records nothing. `wantCandidates`
  drops the IDs `Peek` does not grant (rejected, held, full, peer full) before the limiter wait,
  replacing the separate rejected-cache check; `Claim` after the wait stays authoritative. So
  neither re-ask passes nor any other fetch spend fetch rate on IDs still held or refused. Each
  ID offered counts once in `want_claims`, at `Peek` or at `Claim`. Tests:
  `TestFetchPeekSkipsLimiter` (waits 1 s without it), `TestPeek`, and a `Peek` op in
  `TestModelReject`.
- **(T4.4 fix round 2, Low 3) Re-ask waits keep the peer.** For up to
  `maxPendingPasses × stripeRetryInterval` (30 s), the passes of `resolvePending` keep:
  - the stripe's per-peer lock, so other stripes of that peer wait;
  - the peer's Striped state, so rounds start no session with it.

  They hold no session slots, and the wait counts against the stripe timeout. Accepted for now,
  since it happens once per attempt and only while the want set refuses IDs. Follow-up for T4.5.
- **(T4.4 fix round 2) Test flakes were test bugs.**
  - `TestSREPStallRetry`'s reference time is now taken before A's reconciler starts. The fetch
    timeout starts in `fetch()` before `NewStream`, earlier than B1's handler signal.
  - `roundNode.start` (and so every `roundNode` test) delivers A's Status to each peer and waits
    until the peer holds it before starting the reconciler.
  - `newRadiusNet` waits until the peers hold the first round's Status.

  Cause, reproduced with a diagnostic copy under 8× CPU load (3 failures in 1500 runs): A sent
  both Statuses (old root, then moved root) and neither was dropped (no rate limit, no stream
  limit). But the first-contact handler stored after the moved-root handler. streamtest's
  `FullClose` returns before the remote handler runs, so two Status handlers of one sender can be
  in flight and store out of order, and the peer answers sessions Busy or keeps an old root.
  libp2p's `FullClose` waits for the remote, so production is not affected. `pkg/p2p/streamtest`
  is left unchanged (out of scope; Follow-ups).

- **(T4.5 part 1) What was left of the Phase-1 loop, and what replaced it.** After T4.1–T4.4 the
  T1.11 skeleton still showed in three places:
  - `round()` in `reconciler.go`, the Phase-1 loop body, grown in place to run the peer lifecycle,
    the Status picks, admission, catch-up planning and the session decisions in one function;
  - `peerState.fetching`, a per-peer flag next to the state machine's own Fetching state;
  - Phase-1 wording: `ScoreThreshold` ("Phase 4 may tighten this") and `sameDigest` ("Phase 4
    compares summaries of the session root").

  `round()` is now the scheduler's round in `scheduler.go`. It runs named steps, each in the file of
  its driver:
  1. `trackPeersLocked` (`peers.go`): the topology into the per-peer machines (new peers in Probing,
     departed peers through `eventDisconnected`, the Unsupported reconnect, the resend flag);
  2. `pickDueLocked` and `probe` (`gossip.go`): the Statuses that are due;
  3. `admitLocked`: Probing to Idle or Removed;
  4. `planCatchUp` (`stripes_transfer.go`): the bootstrap or radius-half transfer for the stripe
     worker;
  5. `scheduleLocked` (`scheduler.go`): the session decisions, the start-up guard and the adaptive
     interval.

  `session`, `recheckSession`, `initiates` and `sameDigest` moved to `scheduler.go`, and `probe` to
  `gossip.go`. `reconciler.go` keeps the types, the wiring, `SyncRate` and the scoring. This is code
  motion: the T4.4 suite passes unchanged, apart from the machine's table tests below. The package
  doc now names the two drivers (state machine, scheduler) and the goroutine model (P6).
- **(T4.5 part 1) The state machine tracks the round fetch.** `peerFSM.fetching` replaces
  `peerState.fetching`. The flag holds from `eventSessionDone` to `eventFetched`. The table gains
  `Striped --fetched--> Striped`, for a round fetch that ends during a stripe session. `stripe-done`
  goes to Fetching only while the round fetch runs, and to Idle otherwise. This is the machine's one
  guarded transition: the stripe's own wants are fetched while the peer is Striped. Callers now fire
  one event each, under the same lock as before, so no caller sees a state it did not see before.
  Test changes:
  - `beyond81` gains the new edge;
  - `PeerFSMIn(Striped)` has a round fetch pending, as when the machine got there through Fetching,
    so the literal §8.1 edge `Striped --> Fetching` still holds;
  - `TestPeerFSMRoundFetchDuringStripe` covers both orders and a stripe with no round fetch.
- **(T4.5 part 1) Kept, because they carry weight.**
  - *Topology changes still run a round at once.* Deferring them like the other triggers is the
    follow-up "(T4.3) First Busy retry on topology rounds", which the orchestrator scheduled for
    part 2. After the split it is the `changes` case of `loop`. The tests that drive rounds through
    topology changes with `RoundInterval = 1 h` need rework with it: `TestRoundOwnsFirstContact`,
    the radius tests (`radiusNet.round`) and the want-set harness (`wants_test.go`).
  - *`Options.RoundInterval`* (a fixed interval with a real-clock round timer). The node never
    sets it (`pkg/node` passes `Options{}`), but every `newHarness` test needs it: they run rounds
    every 10 ms in real time on a frozen fake clock (`settle()`). Its doc now says it is for tests.
  - *The adaptive round timer.* It is §8.2's `RoundInterval` (T4.3), not a Phase-1 timer.
  - *`eventLargeDiff`* (`Reconciling --> Striped`): the wiring does not use it. It stays so that
    the table matches §8.1 (T4.2).
  - *The admission pass before `planCatchUp`, and admission again in `scheduleLocked`.* Both are
    needed (T4.2 fix): stripes must find the peers Idle, and the evaluation admits from the Status
    it evaluates.
  - *`peerState.sent`, `served`, `verdict` and `takeover*`.* They are not machine state: they belong
    to the gossip, the guard, the Unsupported reconnect and the takeover.
  - *`orderCandidates`.* The verifier uses the unlocked form.
  - *The `ScoreThreshold` rule* ("run last"). Phase 4 does not tighten it: a violation already
    disconnects the peer, and Busy or failing peers back off. Only the doc changed.
- **(T4.5 part 1) Dead code.** `wantSources.len` had test callers only and moves into
  `export_test.go`. `golangci-lint run --tests=false --enable-only unused` finds nothing else in the
  reconciler.
- **(T4.5 part 1) Contracts unchanged.** These do not change:
  - `reconciler.Service`: `LocalStatus`, `SendStatusWith`, `LastStatus`, `SubscribeRootChange`,
    `ReconcileWith`, `Fetch`;
  - the optional `statusSubscriber` and `resultFetcher`;
  - the wiring: `New` → `SetService` → `Start`, `Options.Scorer` and `Options.Wants`;
  - the single `stripeWorker`;
  - the `SyncRate` guard: registered stripes, `radiusHold`, `StartupGuardTimeout` 120 s,
    `CatchUpMinDiff` 256;
  - `storer.Syncer`.

  No `pkg/node`, `go.mod` or proto change. There is no new parser of peer input, so no line in
  `followup-fuzzing.md`.
- **(T4.5 part 1) P6, measured.** `TestRangesyncOnlyGoroutines` gives the node under test a pprof
  goroutine label (`pprof.Do` around `Start`), which every goroutine its reconciler starts inherits.
  It counts the labeled goroutines that have a reconciler frame. That leaves out the peers' stream
  handlers, which streamtest starts from the node's goroutines, and it stays exact while other tests
  run in parallel.
  - Measured: with n sessions held at the peers, 3 + n (n = 4: 7; n = 8: 11). Once converged, 3:
    the round loop, the gossip loop and the stripe worker.
  - Asserted: n ≤ count ≤ 3 + 2n while held, and the count polled down to 3 after convergence. The
    node opens one session stream per neighbor and no stream of another protocol.

  For comparison, the puller runs 2 × (32 − r) workers per neighbor, a historical and a live one
  per bin: 512 for 8 neighbors at r = 0.
- **(T4.5 part 1) Dual-mode tests.** Both live in `pkg/rangesync/reconciler/coexistence_test.go`,
  where the `roundNode` harness (real services, reconcilers wired as in `pkg/node`) lives.
  `roundNodeOptions` gains `now`, `streamErr` and `streamer`.
  - `TestDualModeConverges` runs D (both protocols over one reserve) between R (rangesync only) and
    P (pullsync only):
    - P's rangesync streams fail as `IncompatibleStreamError`, as in libp2p, and D's machine for P
      ends Unsupported;
    - D pulls from P with pullsync and reconciles with R, and D and R end with the union;
    - x, held by both P and R, is pulled while D's rangesync fetch of x is at R, so D counts one
      `cross_protocol_duplicates_total`. The test reads it through `Service.Metrics()` and a
      Prometheus registry, as the node exposes it. R is not scored for it;
    - rangesync streams go to R only, pullsync streams to P only.
  - `TestDualModeSyncRate`: the node's `SyncRate` is the sum of the drivers (`pkg/node`'s combined
    syncer, which `TestCombinedSyncerBothDrivers` covers). The test sums them the same way. The
    reconciler side is real: its bootstrap keeps its rate at least at `StartupGuardRate` after the
    puller has caught up. After the bootstrap and the window its rate is 0, while pullsync keeps
    storing and R fetches those chunks in rangesync sessions (steady state, not catch-up). The sum
    stays above 0 while either driver catches up and reaches 0 once both have.
  - The puller is a stand-in (`pullerStandIn`: one pullsync page per call, its rate over the window
    on the test clock), because the real one cannot run here:
    - the mock reserve serves one scripted `SubscribeBin` response per call, and the puller calls
      it in a loop;
    - its 10-minute window runs on the real clock (`pkg/rate` takes no clock, and `pkg/puller` is
      frozen).

    P pulling from a dual node is not repeated: `TestCoexistence` (T1.13) covers it.
- **(T4.5 part 1) Part-2 follow-ups.** None became trivial or obsolete.
  - The first Busy retry on topology rounds is now one `case` in `loop`, plus the test rework above.
  - A `peers{state}` gauge is simpler now: the machine holds all per-peer sync state, including the
    round fetch.
  - The rest do not move: Status-spam escalation, local store errors in the backoff, registry
    trimming, `sameDigest` summaries, `Local.Symbols` ctx, the stripe bounds and timer, scoring
    undelivered IDs, the rejected-cache flush.

- **(T4.5 part 2a) Topology changes wait for the round floor; first contacts stay immediate.** This
  supersedes three entries: T4.3's "Topology changes still run a round at once", T4.2's "rounds are
  at least `MinRoundInterval` apart, except rounds that a topology change triggers", and part 1's
  kept item "Topology changes still run a round at once". It closes the last Phase-1 behavior.
  - A topology change is a trigger: its round runs no sooner than `MinRoundInterval` after the last
    one. So every round, and every session a round starts, is at least `MinRoundInterval` after the
    previous round, and a peer's first Busy (`sessionBackoff(1) = 0`) waits at least that long.
  - The round's first step is now `track` (`trackRootLocked`, `trackPeersLocked`, `pickDueLocked`,
    `probe`). A topology change runs `track` at once, then defers the round. T4.1's "round owns
    first contact" holds: `track` adds a peer and picks its first contact in one critical section
    under `pickMu`, and the gossip loop sees only peers that `track` added.
  - Start-up guard: a new entry, or a new machine for an Unsupported peer that reconnected, disarms
    the guard until a round has evaluated the peer.
  - Latency: the first Status to a new peer is unchanged (at once). Its first session waits up to
    `MinRoundInterval` (5 s) after the last round, and so can the guard's release (well under the
    120 s `StartupGuardTimeout`).

  Tests: `TestTopologyChangeFloor` and `TestStartupGuardNewPeer` (a new peer, and an Unsupported
  peer that reconnects). Reworked:
  - `TestRoundOwnsFirstContact` checks the first contact before it moves the fake clock an hour to
    the deferred round;
  - `radiusNet.round` calls `Round`;
  - the want harness's comments say its rounds run at Start only.

  `TestWantSetRetryLeftPeer` is unchanged, because `track` forgets a departed peer at once.
- **(T4.5 part 2a) A new machine has a new generation.** This is part-1 review Low 1.
  `peerState.gen` counts the machines an entry has had. `resetLocked` bumps it and replaces the
  three in-place resets:
  - `ErrProtocolUnsupported` outside Probing, in `probe`;
  - an Unsupported peer's new Status, in `trackPeersLocked`;
  - a Removed peer that is eligible again, in `admitPeerLocked`.

  The session outcome, its fetch's `eventFetched`, `recheckSession` and the stripe release fire
  only while `currentLocked` holds: the same entry and the same generation. Before this, an old
  fetch ended the new machine's Fetching early, an old Busy backed off the peer during the new
  session, an old session waiting for its slot passed its recheck on the new machine's Reconciling,
  and an old stripe's release ended a new stripe's Striped. The old machine's runs belong to the
  old connection. One of them may still overlap the new machine's first session or fetch (at most
  one per reset), as after a reconnect the peer set shows, which gets a new entry. Keeping the new
  machine idle until they end was not done, because that case does not do it either. Tests:
  `TestPeerStateResetIgnoresStaleRuns` (a stale fetch, a stale session, a stale session waiting for
  its slot, a stale stripe). Each check site fails one of them when reverted.
- **(T4.5 part 2a) Stripe restarts, the peer's session slot and the stripe clock.** These are the
  T4.3 round-3 review Lows 1 to 4. This supersedes three earlier statements:
  - T4.3 fix round 3's "`localBusy` covers ... its session slots being taken (the node cap, or the
    peer's own session with this node)";
  - T4.3 fix round 3's bound "at most `(maxStripeRestarts + 1) × StripeTimeout` on one peer";
  - T4.3 fix round 2's "the clock pauses while the attempt waits to retry the peer's Busy". That
    pause now needs no fetch of the attempt to be running.

  What changed:
  - *`ErrPeerSessionLimit` is the peer's Busy for stripes.* Only the peer's own session with this
    node holds that slot, and the responder keeps it through `busy()` and `FullClose`. It now
    counts towards `maxStripeBusyRetries`, off the clock, and the stripe is reassigned after about
    30 s, unscored. `localBusy` is `fingerprint.ErrBusy` and `ErrNodeSessionLimit`. Round sessions
    are unchanged (`eventAborted`, no backoff; T4.3 L1).
  - *One local-Busy restart budget per stripe.* Restarts after attempts held up only by this node
    draw on `maxStripeRestarts` per stripe, and reassigning does not reset that budget. Restarts
    after progress or deferral keep their per-peer budget.
  - *The bound, now in `runStripe`'s doc.* A stripe makes at most `(1 + maxStripeReassign) × (1 +
    maxStripeRestarts) = 36` attempts of `StripeTimeout` on the stripe clock (9 h). At most `1 +
    maxStripeRestarts + maxStripeReassign = 12` of them (3 h) are held up only by this node; this
    was 36 (about 9 h). Off the clock, per attempt: the wait for the peer's stripe lock and the
    slots, and the peer-Busy waits (at most 30 × (1 s + slot wait)).
  - *No pause while a fetch is pending.* The clock does not pause for the peer's Busy while a fetch
    of IDs listed by an earlier session of the attempt still runs (a count of queued batches).
  - *The injected clock.* `stripeClock`, the Busy retry wait and the re-ask wait use
    `Options.Now` and `Options.After`. `stripeClock` runs one goroutine per resumed stretch, which
    `pause` ends. A goroutine paused before it saw its expiry does not expire later. The attempt
    calls `expireIfDue` after each session answer and each wait, so it starts nothing past its
    timeout whatever the goroutine scheduling; on a fake clock that makes the timeout exact.

  Tests: `TestStripeLocalBusyBudget`, `TestStripePeerSessionLimitIsPeerBusy` and
  `TestStripeBusyPauseFetchPending`. These move to the fake clock (`newStripeHarness`; its
  `advancingAfter` makes each retry wait pass at once):
  - `TestStripeFetchBoundedByTimeout` now waits until the fetch has started and the session has
    ended, and only then moves the clock past the timeout;
  - `TestStripeLocalBusy` now checks both bounds;
  - `TestStripeTimeoutPausedWhileBusy` adds held time through a session hook;
  - `TestStripeTimeoutAfterSlot` was vacuous on a frozen fake clock;
  - `TestStripeBusyRetriesBounded` and `TestStripeBusyReassigns`.

  The want harness's `After` makes 1 ms waits pass at once. The package doc counts the stripe
  clock's goroutine.
- **(T4.5 part 2a) Re-ask waits release the peer.** This supersedes T4.4 fix round 2 Low 3
  ("Re-ask waits keep the peer"). `resolvePending` releases the stripe's per-peer lock and the
  peer's Striped state before each wait. It takes both back, lock first, for the next ask, and it
  holds no slot. The per-peer lock moved from `runStripe` and `verifyStripe` into
  `transferStripe`, which keeps what it holds on its peer in a `peerHold`. The stripe clock runs
  throughout, so the timeout bounds the waits, the waits to take the peer back, and the asks. The
  cost: another stripe of the same peer may now run its whole attempt during a wait, and the
  waiting stripe waits for it on its own clock (Follow-ups). Tests: `TestStripeReaskReleasesPeer`
  and `TestStripeReaskTimeoutBoundsPeerWait`.
- **(T4.5 part 2a) `Tried` holds only the peers that ran a session.** This supersedes T4.3 fix
  round 3's "`TestStripeBusyReassigns` covers ... Tried = 2". `StripeResult.Tried` now holds the
  peers whose attempt was answered other than Busy or listed IDs. `verifier` excludes only those,
  so a peer that only answered Busy may verify the stripe. A Busy peer is still never blamed: Busy
  is unscored, and a verification it answers Busy is incomplete. `runStripe`'s own tried set, which
  keeps a stripe from going back to a peer it was assigned to, is unchanged. Test:
  `TestVerifyStripesBusyOnlyPeer`.
- **(T4.5 part 2a) Contracts and scope.** These are unchanged:
  - `reconciler.Service` and its optional interfaces;
  - the wiring and the single `stripeWorker`;
  - the `SyncRate` guard semantics; the guard only waits for a new peer's round;
  - `storer.Syncer`;
  - the 8/4 slot split, the post-slot recheck, the takeover and the want-set claim protocol.

  There is no `pkg/node`, `go.mod` or proto change. There is no new parser of peer input, so no
  line in `followup-fuzzing.md`. New test helpers: `fakeService.sessionHook` and `busyErr`,
  `advancingAfter`, `newStripeHarness`, and the export `StripeSlotsTaken`.

- **(T4.5 part 2a fix round 1, Low 1) Verifications and bootstrap stripes keep the stripe lock
  across re-asks.** This supersedes the part 2a entry "Re-ask waits release the peer" for those
  two cases. `verifier` picks the first ready candidate for every stripe, so the verifications of
  a transfer queue on one peer's lock. One that freed the lock while it waited to ask again
  rejoined the back of the queue on its own clock and could time out. Verifications do not
  restart, so the deliverer then went unverified and unscored (§9 withholding). During a bootstrap
  rounds start no session, so the release bought them nothing. Now:
  - `peerLocks.keep` makes a stripe free only the peer's Striped state while it waits, and
    `holdPeer` takes back only what was freed;
  - `VerifyStripes`' lock set keeps its locks, and so does the stripe worker's transfer of a
    bootstrap job (`transferStripes` gains a bootstrap flag);
  - other transfers (radius halves, sessions' large ranges) still free the lock.

  Keeping the lock was chosen over spreading verifications across candidates: it also covers a
  neighborhood with a single candidate, and it keeps the lowest-score choice. Tests:
  `TestVerifyStripesShareVerifierReask`, `TestBootstrapStripeKeepsLockReask`.
- **(T4.5 part 2a fix round 1, Low 2) The clock checks cover every timed wait.** This corrects the
  part 2a entry's "starts nothing past its timeout whatever the goroutine scheduling". That held
  only for session answers and retry waits. The attempt now also calls `expireIfDue` after the
  wait for its peer before the first session, after taking its slots back in the Busy loop, and
  after taking its peer back for an ask. So no session and no ask starts past the timeout. A fetch
  already running ends when the clock's goroutine cancels it. Tests:
  `TestStripeClockCheckedAfterWaits` (peer, slots) and `TestStripeReaskClockCheckedAfterPeerWait`.
  They use `stripeWaits`, an `After` whose stripe expiries never fire and whose retry waits the
  test ends.
- **(T4.5 part 2a fix round 1, Low 3) Topology churn does not stretch the round gap.** This
  supersedes the part 2a entry's "A topology change runs `track` at once":
  - *A due round goes first.* The loop runs a due round (timer or deferred trigger), checked with
    a non-blocking select, before it handles the next trigger.
  - *Sends leave the loop.* A topology change runs `follow` on the loop: the root and the topology
    into the machines, and the picks, in the critical section that adds new peers. It then sends
    the picked Statuses in a goroutine of its own (`followTopology`, in `r.wg`). So a peer that
    never answers holds up no round. Sends stay bounded: at most one is in flight per peer
    (`sending`).
  - *A delivered send triggers a round,* as a gossip pass does: a round that ran while it was in
    flight skipped the peer's session.
  - *Rounds are unchanged.* `track`, the round's first step, is `follow` plus a synchronous
    `probe`, so a round still sees the outcome of the first contacts it sends (T4.1).
  - *Latency:* the first Status is still sent at once. If it is slower than the round floor, the
    first session waits for the round its delivery triggers.

  Tests: `TestRoundDueBeforeTopologyChange` (20 trials) and `TestTopologyChangeDoesNotHoldRounds`.
- **(T4.5 part 2a fix round 1, Low 4) Test gaps closed.**
  - `TestStripeReaskRetakesLock` fails when `holdPeer` skips the lock.
  - `TestRoundOwnsFirstContact` tests its race again. Peers join without a topology change, the
    test runs the round that finds them while the gossip loop runs passes back to back, and it
    checks the first contact right after that round. It fails when `track` sends nothing or does
    not pick.
  - `TestTopologyChangeFloor` and `TestStartupGuardNewPeer` wait until the first contact is
    recorded, since the send is no longer on the loop.

- **(T4.5 part 2b) Undelivered listed IDs count against the peer, in rangesync's fetch.** This
  supersedes T4.4's "Claim-and-stall goes through the single `PeerScorer`; no new code" where it
  says "A peer that closes the fetch stream without some listed IDs is not scored: T1.10 treats
  eviction between session and fetch as honest". A peer is asked only for IDs it listed (Items or
  symbol decodes). Each `FetchWithResult` call now keeps the peer's shortfall:
  - the IDs requested in Wants the peer closed (EOF) without delivering them, except those outside
    the peer's root by its last Status: its radius grew since it listed them;
  - its deliveries whose stamp is invalid on every node (`stampInvalid`): a signature that does not
    recover or has the wrong length, or an address, bucket or owner that does not match. An honest
    peer validated the stamp when it stored the chunk.

  A shortfall above `max(4, requested/100)` (`shortfallTolerance`, the same as the stripe
  verification's `verifyMinDiff` and `verifyDiffRatio`) is a violation. `initiatorViolation`
  reports it to `Options.Scorer` (the reconciler, the single `PeerScorer`) and disconnects the
  peer, as a stall does. It is judged on every return of the call, and also after each Want
  against the tolerance of all the IDs passed. The end judgment can only confirm that one, so a
  peer that listed junk costs one Want, not up to `LargeDiff` IDs of fetch rate. The undelivered
  IDs are still released, and retried from other sources. These do not count:
  - held, deferred and unverified IDs (want-set causes, and stamp checks that failed here);
  - rejections that depend on the chain state the two nodes have seen: `postage.ErrNotFound` (an
    unknown or expired batch, the Q7 skew) and `postage.ErrInvalidIndex` (a dilution raises the
    depth);
  - rejections that depend on this node's reserve (`ErrOverwriteNewerChunk`);
  - the IDs of a Want that failed (reset by the peer, a local error, the context);
  - anything when the call already reported a violation (one score per call), or while the Service
    closes.
- **(T4.5 part 2b) The tolerance against honest churn, and what it leaves.** Honest peers miss few
  listed IDs:
  - `unreserve` evicts bins below the radius, which lie outside the root, so it does not touch
    listed IDs;
  - a radius increase evicts only the overflow per pass, cheapest batches first, spread over a bin
    of about half the reserve, and the IDs it drops leave the peer's root, which its next Status
    shows;
  - what is left is chunks replaced by a newer stamp, or evicted with an expiring batch, in the
    second or so between listing and fetch (longer for catch-up fetches).

  What can still trip an honest peer: a batch that expires on the peer before it does here while
  more than 1% (and at least 5) of a fetch's IDs are in it, or a radius increase whose moved Status
  arrives after the fetch ends (it waits for the index rebuild). Such a peer is disconnected once
  and reconnects (no blocklist). Accepted for now; T6 should read it off testnet (Follow-ups). The
  root exclusion can be abused: a peer can send a narrower root before its fetch ends to excuse
  junk it listed. That is bounded by its Status rate, and the narrower root also scopes its next
  sessions with this node.
- **(T4.5 part 2b) Withholding and verification are not double-scored.**
  - A fetch violation fails the stripe attempt with that error (`fetchErr`), not as withheld.
    Withholding (`Undelivered > 0`) still reassigns a stripe unscored when the shortfall is within
    the tolerance.
  - A verification whose fetch was a violation ends with an error, so `verifyStripe` returns
    before its verifier rule. That rule (over the whole verification, a score without a
    disconnect) still catches shortfalls that stay under the tolerance of each fetch.
  - The deliverer's verification rule judges IDs the deliverer never listed, a different shortfall.
- **(T4.5 part 2b) Per-peer share of the rejected cache.** `wantset.Set.RejectFrom(id, peer)`
  (additive) records a rejection against the delivering peer; `Reject` stays unattributed. Each
  peer's share is `MaxRejectedPerPeer = MaxRejected / 4 = 2500`. A peer at its share drops its own
  oldest rejection first. The cache's own bound still drops the overall oldest when the shares
  together exceed `MaxRejected`, which takes at least four peers. So one peer can push out at most
  2500 rejections of other peers, and none once it holds its share. Rejecting an ID again moves it
  to the new peer's share. Chosen over:
  - evicting from the largest share first, which needs a max structure;
  - a per-peer rate, which depends on time and lets a patient peer flush the cache anyway.

  A share is O(1) per rejection: one more list element per ID, about 340 B per ID measured (was
  287 B), 3.4 MB at the cap. A quarter rather than the claims' tenth, because the honest
  expired-batch skew spreads the rejections of a batch's neighborhood share (thousands) over a few
  peers. `RejectedBy(peer)` (additive) reports a share. The fetch path rejects through the
  delivering peer.
- **(T4.5 part 2b) Requester-side fetch streams per peer.** `maxFetchStreamsOut =
  maxPeerFetchStreams − 1 = 3`, in rangesync, so it covers every source: round fetches, stripes,
  retries and the wants of served sessions. Each Want waits for a slot (`peerSlots`, refcounted per
  peer) before the fetch limiter and before its claims. So a waiting fetch holds no claims, and the
  bound from claim to last delivery (`FetchTimeout`) is unchanged. The cap is one below the
  responder's limit, for a handler that has not returned yet (as `maxPeerStatusStreams`). A side
  effect: a peer's claims are at most 3 × 250 = 750 < `MaxClaimsPerPeer`, so `PeerFull` is no
  longer reached in production; `Full` still is. The package doc no longer claims O(neighbors)
  goroutines for served sessions' fetches and retries, which queue up while the fetch rate lags;
  their streams are O(neighbors). One wants fetcher per peer would make the goroutines
  O(neighbors) too, but it changes the catch-up accounting behind `SyncRate` (Follow-ups).
- **(T4.5 part 2b) Persistent Status spam is a violation.** This supersedes T4.1's "An inbound
  Status over the rate is dropped. It is not a violation and is not scored." A dropped Status takes
  a token of a per-peer drop budget: `PeerStatusDropBurst = 2 × PeerStatusBurst = 8`, one regained
  per `PeerStatusDropInterval = 4 × PeerStatusInterval = 10 s` (additive constants, a
  `ratelimit.Limiter` cleared on disconnect). A drop past the budget is a violation (reset, scored,
  disconnected). A peer that sends more than a quarter over its rate (0.5/s) is escalated; below
  that, a drop costs a stream reset before anything is read. T4.1's concern no longer applies: the
  receiver's limits are cleared on disconnect, and the honest sender stays a Status below the rate.
  `TestStatusGossipNeverEscalated` runs the reconciler's real schedule for 20 minutes of the fake
  clock (summary changes every 100 ms, a reconnect every 5 minutes, bursts of six radius changes
  every 2 minutes, 20 s or 3 s) against a model of the receiver:
  - on time, nothing is dropped;
  - with arrivals delayed by up to the send timeout (10 s), at random or bunched (all sends of each
    10 s at once), at most 59 of 419 are dropped and none is escalated, with the limits cleared on
    a reconnect or kept across it.

  A first draft regained one drop per 60 s; the 20 s schedule with bunched delays escalated under
  it, hence 10 s.
- **(T4.5 part 2b) Local store errors are local causes (`rangesync.ErrLocal`).** The new additive
  sentinel wraps:
  - every error of a session's `Local` (fingerprint index, address index, reserve lookups). An
    `IterateIDs` callback's own error is returned as it is;
  - the `LocalStatus` error in `ReconcileWith`;
  - the fetch's reserve lookups and `Put`.

  The reconciler's `localCause` includes it: no backoff (`eventAborted`), no score. That also
  covers `fingerprint.ErrOutsideRoot` from a session (this node's root moved), which backed off
  before. Stripes are unchanged: a local error still ends the attempt and reassigns the stripe
  without a score (Follow-ups).
- **(T4.5 part 2b) The T4.4 round-2 Lows.**
  - `deliver` calls `claimExpired` on the early return too, when the chunk is in the reserve after
    the stamp check. So intra-`rangesync` duplicates are at most `want_claims{expired}` on that path
    too (`TestFetchExpiredDuringStampCheck`).
  - `TestFetchExpiredDuringPut` runs with a 500 ms claim deadline.
  - `wantset.Set.PeekPicked(id, peer, picked)` (additive) counts the Want's earlier candidates
    towards the caps. `wantCandidates` uses it, so near a cap only the room left is charged to the
    fetch limiter (`TestFetchPeekCountsPicked`, `TestPeekPicked`, an op in `TestModelReject`).
  - `TestFetchBadSignatureValidStamp` goes through the real `postage.ValidStamp` over a batch store,
    with chunks a stamper signed for the batch's owner.
- **(T4.5 part 2b) Tests changed by design.**
  - `TestFetchManyWants` puts the IDs its holder lacks outside the holder's root: otherwise its 258
    of 260 missing IDs are a violation.
  - `TestStripeBadSignatures` runs with 4 bad signatures, the tolerance (was 8);
    `TestStripeBadSignaturesScored` covers 8 (one score, a disconnect, the IDs not asked for again).
  - `TestPeerStateLocalCauseNoBackoff` adds two `ErrLocal` failures in a row.
- **(T4.5 part 2b) Contracts and scope.** These are unchanged: `reconciler.Service` and its
  optional interfaces, the wiring, the `SyncRate` guard, `storer.Syncer`, the want-set claim
  protocol and the T4.1–T4.5 rules not superseded above. New exported identifiers, all additive:
  `rangesync.ErrLocal`, `PeerStatusDropBurst`, `PeerStatusDropInterval`; `wantset.MaxRejectedPerPeer`,
  `Set.PeekPicked`, `Set.RejectFrom`, `Set.RejectedBy`. There is no `pkg/node`, `go.mod` or proto
  change, and no new parser of peer input. `followup-fuzzing.md` gains one line for the new
  invariants of two existing targets.

- **(T4.5 part 2b fix round 1, Medium 1a) A Status root must contain its sender, and root moves
  excuse little.** This supersedes the part 2b entry's "except those outside the peer's root by its
  last Status" and "The root exclusion can be abused ... bounded by its Status rate".
  - `handleStatus` treats a root that does not contain the sender's overlay as a violation. §7.6
    defines a root as `(B, rB)`, the node's own prefix at its storage radius, which is the
    Indexer's root, so no honest Status trips it. Any two roots of one peer are then nested.
  - A fetch requests no ID outside the peer's root by its last Status (looked up for each Want).
    Such IDs are `Undelivered`: retried from other peers, counted as withheld by a stripe, free of
    fetch rate, and never part of the shortfall.
  - A missing ID that left the root while the fetch ran is excused only when the move from the
    fetch's starting root is a radius increase of one or two bits (`plausibleStep`), and the first
    move of that peer within `rootExcuseInterval` (10 min). The same move again, the concurrent
    fetches of one radius increase, is excused too. The excuse is kept across reconnects.

  So a peer that lists junk away from its overlay and then narrows its root gets only the Wants
  already sent excused, once per 10 min.
- **(T4.5 part 2b fix round 1, Medium 1b) A reset counts.** A fetch stream that fails without a
  delivery, and was not ended by this node or by the fetch timeout, takes a token of a per-peer
  budget: `maxFetchResets = 4`, then one per `fetchResetInterval` (1 min). Past it, the failure is a
  violation. The budget is a `ratelimit.Limiter` cleared on disconnect. Honest causes (a store error
  on the responder, a shutdown) seldom exceed four, and a disconnect clears them. A peer that resets
  every Want gets one Want (at most 250 IDs) per minute unscored.
- **(T4.5 part 2b fix round 1, Medium 2) A first offense scores without a disconnect, and a miss
  counts when repeated.** This supersedes the part 2b entries "A shortfall above
  `max(4, requested/100)` ... is a violation" (scored and disconnected on the first offense) and "after
  each Want against the tolerance of all the IDs passed". The rules, in `shortfall.go`:
  - *Blatant.* When at least half of at least `blatantMinIDs = 64` requested IDs fall short (after
    excuses), the fetch is a violation at once. This is judged after each Want and at the end, so a
    peer that listed junk still costs one Want.
  - *Deferred verdict.* Otherwise a missing ID counts only when the same peer left it missing
    before, within `shortfallWindow = 10 min` (the reconciler's `ScoreDecayInterval`). The memory is
    per peer, at most `maxMissingPerPeer = MaxClaimsPerPeer` IDs, and is cleared on disconnect. An
    honest peer's index commits an eviction, a batch expiry or an in-place rewrite before its next
    listing, so it never lists such an ID again. Hot SOC and GSOC rewrites, correlated batch
    evictions and fetches that waited minutes in a queue therefore never count. Stamps invalid on
    every node count at once.
  - *Offense.* At the end of the fetch, a count above `max(4, n/100)` is an offense. The first
    within `shortfallWindow` is reported through `scorer.Fail` with a reason wrapping only the new
    additive `ErrFetchShortfall`, without a disconnect. The fetch's error wraps `ErrFetchShortfall`,
    so a stripe attempt fails with it and a verification does not score again. A repeat within the
    window is a violation (disconnect). `PeerScorer.Fail`'s doc now allows that reason; the
    reconciler only raises the score.

  What still trips an honest peer: a batch expiry or a radius step beyond the excused move that
  evicts more than half of a fetch of 64 or more IDs between the listing and the Want, which
  disconnects it once. An inconsistent responder store (its index lists chunks it lost) is scored at
  the second listing and disconnected at the third within 10 min, and again after each reconnect;
  such a store needs a repair. What a peer can still do unscored: fresh junk that stays below half
  of each fetch (it must fill the rest with chunks this node lacks), or below 64 IDs per fetch; and
  unknown-batch junk (T4.6).
- **(T4.5 part 2b fix round 1, Low 3) No judgment without a Status.** A disconnect deletes the
  peer's Status, and the judgment then returns nothing, so the IDs a root move excused are not
  counted as if there were no root. A peer that lists IDs has a Status (a session needs one), so
  only a disconnect during the fetch skips the judgment. A test fetch from a peer with no Status is
  not judged (`TestFetchManyWants` is back to its original setup).
- **(T4.5 part 2b fix round 1, Low 4) The requester opens at most two fetch streams per peer.** This
  supersedes part 2b's `maxFetchStreamsOut = maxPeerFetchStreams − 1 = 3`; it is now
  `maxPeerFetchStreams / 2`. A stream this node resets keeps its handler on the responder until the
  handler's next write fails, after its wait for the delivery rate, so the cap leaves room for two
  such handlers. Chosen over a responder that resets at limit + 1: that reset would charge this
  node's new reset budget (Medium 1b) to an honest responder and fail honest fetches. Two streams
  cost little throughput: the responder delivers at most 250 chunks/s per peer anyway. A peer's
  claims are then at most 500.
- **(T4.5 part 2b fix round 1) Tests and contracts.** `TestFetchShortfall*` are rewritten for the
  new rules: blatant, relisted (offenses and window), invalid stamps, honest churn (in-place
  rewrites, correlated batch eviction, a radius increase mid-fetch, causes on this node), excuse
  once, status gone, after a violation. New: `TestShortfallBook`, `TestFetchSkipsOutsidePeerRoot`,
  `TestFetchResetBudget`, `TestStatusRootContainsPeer`, `TestFetchStreamsLaggingHandlers`. The
  adversarial tests' raw peers sit inside their roots (`newRawPeerIn`). `TestStripeBadSignaturesScored`
  is now a first offense: scored once, not disconnected. The additive identifier is
  `rangesync.ErrFetchShortfall`. There is no `pkg/node`, `go.mod` or proto change.

- **(T4.5 part 2b fix round 2, Medium 1) A reset is charged whatever was delivered before it.** This
  supersedes fix round 1's "A fetch stream that fails without a delivery ... takes a token". A
  stream that fails with claimed IDs still undelivered takes a token of the peer's reset budget
  (`chargeReset`), so one delivery (even a chunk this node rejects without counting it, such as an
  unknown batch) before the reset no longer makes it free. These are still never charged: this
  node ending the fetch (its context, `Close`), the fetch timeout (a stall, a violation of its own),
  a stream whose claimed IDs all arrived, a peer whose Status is gone (it disconnected), and a peer
  already reported for a violation (Low 3). The IDs lost to the failure are not judged blatant: an
  honest network reset with many IDs in flight must not disconnect. They are not recorded as missing
  either, so a re-listing of them is not counted. The budget now runs on the Service's clock
  (`peerBudget`, `golang.org/x/time/rate` with explicit times; no `go.mod` change), so its refill is
  tested.
- **(T4.5 part 2b fix round 2, Medium 2) A root that swings back gets one move excused.** This
  supersedes fix round 1's "The same move again, the concurrent fetches of one radius increase, is
  excused too". The same move is excused again only for a fetch that started no later than the move
  was first excused (`shortfall.startedAt`). A later fetch has the new root as its starting root,
  unless the root swung back meanwhile, so `r → r+1 → r → r+1` gets the first increase excused, and
  then nothing more for the interval. A move excuses at most the Wants in flight when the peer's
  Status arrives: at most `maxFetchStreamsOut × MaxWantIDs = 500` IDs per peer per 10 min. Later
  Wants skip the IDs outside the new root.
- **(T4.5 part 2b fix round 2, Low 1) Small fresh junk stays unscored; not fixed.** Adding up the
  first misses per peer over `shortfallWindow` is not honest-safe. Hot SOC and GSOC rewrites are
  first misses that are never listed again, exactly like fresh junk, and a GSOC-heavy neighborhood
  can show most of each small fetch missing every round. The rate left is in the Follow-ups.
- **(T4.5 part 2b fix round 2, Low 2) Skips outside the verifier's root do not count against it.**
  `FetchResult.OutsideRoot` (additive) is how many of `Undelivered` were not requested because they
  lie outside the peer's root. `verifyStripe` judges the verifier on `Undelivered − outsideRoot`, and
  `VerifyResult.Undelivered` reports that. A stripe transfer still counts them as undelivered: the
  stripe peer cannot serve them, so the stripe is reassigned, not completed and then blamed on its
  deliverer.
- **(T4.5 part 2b fix round 2, Low 3) Stale entries and double scores.** A violation of a peer
  (`initiatorViolation`, `violation`) marks it for `violationGrace = 1 min`, cleared on disconnect.
  The shortfall judgment and `chargeReset` leave alone a marked peer and a peer whose Status is gone,
  so the other fetches that fail with the disconnect are neither charged nor scored again. After
  they record (the missing IDs, an offense, a reset), `forgetIfGone` clears the peer's entries if its
  Status disappeared meanwhile. The disconnect hook's clear may have run first, and this keeps it
  from leaving a stale entry. The disconnect hook also clears the mark and the reset budget.
- **(T4.5 part 2b fix round 2, Low 4 and 5) Comments and tests.** The comments say the requester's
  cap is half the peer's limit. New tests: `TestFetchResetAfterOneDelivery` (from the review's PoC),
  `TestFetchShortfallExcuseOscillation` (from the review's PoC), `TestFetchResetHonest` (this node's
  cancellations; a peer that disconnected, which leaves no charge behind; a peer already reported),
  `TestFetchShortfallAfterPeerViolation`, `TestVerifyStripesVerifierOutsideRoot`. Changed:
  `TestFetchResetBudget` (the refill on the fake clock, a Status), `TestShortfallBook` (the excuse
  for fetches that started before it, not after). The additive identifier is
  `rangesync.FetchResult.OutsideRoot`.

- **(T4.5 part 2b fix round 3, Medium 1) A fetch stream that fails to open is charged as a reset.**
  This supersedes fix round 2's (Medium 1) list of what is never charged, which left out the stream
  open: `fetch` returned the `NewStream` error before `stalled`. By then the Want has taken its fetch
  rate and holds its claims. A peer that reset the headers exchange, stalled it for libp2p's
  `HeadersRWTimeout` (10 s, below `FetchTimeout`, 15 s), or served no fetch stream, held up to 500
  claims and about 100 IDs/s of fetch rate per Sybil, with neither a stall violation nor a charge.
  A verifier escaped its own rule the same way, since `verifyStripe` does not judge a verification
  that ends with an error. A failed open now goes through `stalled`, like a failed read:
  - this node ending the fetch (its context, `Close`) is not charged;
  - the fetch timeout is a stall violation (the headers exchange outlasts it only when
    `HeadersRWTimeout` is set at or above `FetchTimeout`);
  - `p2p.ErrPeerNotFound` is not charged. libp2p returns it when the peer has no connection, or its
    connection is no longer up, which can be before the disconnect hook clears the peer's Status;
  - any other failure, while the peer has a Status, takes a token of the reset budget
    (`chargeReset`). Past the budget it is a violation.

  Honest transient failures cannot spend the budget. The requester opens at most
  `maxFetchStreamsOut = 2` fetch streams to a peer and holds the slot across the open, so one
  transient failure (a stream reset on the peer's side, a dial race) fails at most two opens or
  streams and takes at most two of the four tokens. The fetches queued behind them open after it.
  Two such events back to back cost nothing, and the budget refills one event per two minutes. A
  peer that fails every open (for example its inbound stream limit is full) is disconnected after
  the budget, as one that resets every stream already was; its connection is of no use for fetching
  then. A verifier's fetch is a `FetchWithResult` like any other, so a verifier whose open fails is
  charged. `verifyStripe` still does not judge it (`v.Err` is set), as with any reset: judging the
  released IDs would score honest transient failures. One cause is not excluded: this node's own
  resource manager refusing the stream (`network.ErrResourceLimitExceeded`, at 10,000 outbound
  streams). rangesync sees only `p2p.Streamer`, which has no sentinel for it, so such a node charges
  every peer it fetches from, and disconnects those with more than four failed opens in a minute.
  It heals itself (the disconnects free streams, scores decay); see Follow-ups.
- **(T4.5 part 2b fix round 3, Low 2) The root-move excuse covers more than 500 IDs; not capped.**
  This corrects fix round 2's (Medium 2) "A move excuses at most the Wants in flight when the peer's
  Status arrives: at most `maxFetchStreamsOut × MaxWantIDs = 500` IDs per peer per 10 min". The end
  judgment excuses every moved missing ID of a call that started before the move, and `sf.missing`
  accumulates over all of the call's Wants, so the Wants that ended before the move are excused too.
  Before the move, the judgment after each Want keeps the junk below half of the IDs requested. After
  it, the excused IDs no longer count, so the rest of the call may be junk up to half of the whole
  call. The worst case moves halfway through: 49% junk before the move (excused), then only junk,
  about 75% in all instead of 50%. For a `LargeDiff` call (65,536 IDs) that is about 16,000 IDs per
  `rootExcuseInterval` beyond the half that is unscored anyway, about 27 IDs/s per Sybil. Several
  calls running at the move each add theirs, within the fetch rate the peer gets. A cap is not
  honest-safe: a real radius step drops half of the peer's root, so an honest fetch can miss half of
  what it listed. No code change.
- **(T4.5 part 2b fix round 3, Low 3) A violation marks the peer only while it has a Status, and is
  reported once.** This extends fix round 2's (Low 3) violation marks. Both callers
  (`initiatorViolation`, `violation`) marked and scored unconditionally. So a second stalled Want
  whose deadline fell after the disconnect hook scored again, and its mark exempted the reconnected
  peer from the shortfall rules and the reset budget for up to `violationGrace`. Now
  `Service.markViolation`:
  - marks the peer only while it has a Status, then calls `forgetIfGone`;
  - reports whether the peer was marked already. The caller then skips `Fail`; it still resets the
    stream and disconnects the peer.

  A violation of a peer without a Status (for example a responder-side one from a peer that never
  sent a Status) is scored, each time. A second violation in the hook's latency window, after the
  hook cleared the mark, is scored once more (at most one per stream in flight). That is accepted.
  The recreated-Status race is accepted too: `handleStatus` can record a Status after the disconnect
  hook (T4.1). It hides the disconnect from `forgetIfGone`, so what the rules record in that window
  carries into the next connection until the peer's next disconnect or until it expires: a mark
  after `violationGrace` (1 min), missing IDs and an offense after `shortfallWindow` (10 min), reset
  charges at one per `fetchResetInterval`. Only a mark helps the peer (up to 1 min unjudged). It
  needs a Status recorded in the microseconds between the handler's read and its record, exactly
  when the hook runs, and then a second violation (a stall) before the streams of the closed
  connection fail. Each attempt costs a scored violation and a reconnect. Handling it with a connect
  hook would change the wiring.
- **(T4.5 part 2b fix round 3, Low 4) Tests and docs.** `judgeShortfall`, `chargeReset` and
  `markViolation` read the clock after `LastStatus`, so a test clock can land the disconnect between
  their look at the Status and their record. `TestFetchForgetIfGone` does that for a reset charge,
  missing IDs and a violation mark; each fails with a no-op `forgetIfGone`.
  `TestFetchShortfallAfterPeerViolation` pins `violationGrace` to one minute (a fetch one second
  before it is not judged). `followup-fuzzing.md`'s part 2b line states the current rules. New
  tests: `TestFetchStreamOpenCharged` (from the review's PoC: a reset or stalled headers exchange,
  no fetch handler), `TestFetchStreamOpenHonest` (a peer that is gone, this node's cancellations and
  `Close`, transient failures), `TestFetchForgetIfGone`, `TestViolationMarks`. There is no new
  exported identifier, and no `pkg/node`, `go.mod` or proto change.

- **(T4.5 part 2c, item 1) A fetch stream opens before the fetch rate and the claims, and a failed
  open is never charged.** This supersedes fix round 3's (Medium 1) "A fetch stream that fails to
  open is charged as a reset" as a whole, and narrows fix round 2's (Medium 1) reset charge: the
  reset budget applies only to a stream whose Want claimed IDs, so only when claims and fetch rate
  were spent. A Want now runs: stream slot (`maxFetchStreamsOut`) → open (`openFetchStream`, its
  own `FetchTimeout`) → fetch rate, with the stream open (bounded only by the fetch's context) →
  claims → Want and deliveries (`FetchTimeout` from the claims, as the claim deadline needs). What a
  failed open costs now:
  - nothing is charged and it is never a violation, whatever the cause: a reset or stalled headers
    exchange, a peer whose inbound streams are full, no fetch handler, the open timing out, the peer
    gone;
  - nothing was requested (no `Undelivered`), no claim is held and no fetch rate was taken, so the
    IDs stay free for other peers' fetches; the call ends with the error, and its other IDs are left
    to the next round, as for any fetch error;
  - the stream slot is held for the open, at most `FetchTimeout`. The cap still bounds streams: the
    slot is held from the open to the stream's end, and queued fetches hold none.

  The responder tolerates a stream that waits on the limiter: `handleFetch` reads the Want with the
  handler's context and no deadline, libp2p sets no deadline on an inbound stream after the headers
  exchange (`HeadersRWTimeout` bounds only that; `closeDeadline` only `FullClose`), and yamux has no
  per-stream idle timeout. Such a stream holds one of the responder's `maxPeerFetchStreams` handler
  slots, at most two per requester, as before. A stream left with nothing to claim after the wait (the
  IDs were stored or claimed meanwhile) is reset without a Want; the responder returns a plain error,
  not a violation. Honest-safety: `TestFetchInboundFullNotDisconnected` runs three neighbors of a
  peer whose inbound streams stay full for five minutes of the fake clock, bursts of six queued
  fetches each every 5 s: none scores or disconnects it, and each fetches from it once its streams
  free up. What a peer gains: it can fail or stall every fetch open unscored. That costs this node
  one of the peer's two stream slots per open and the peer's own fetches, not other peers' (nothing is
  claimed); its sessions still run (Follow-ups). A verifier whose opens fail escapes its own rule, and
  leaves its deliverer unjudged, as with any verification that ends with an error (Follow-ups).
- **(T4.5 part 2c, item 2) This node's own resource limit is a local cause (`ErrLocal`).** Verified
  in go-libp2p v0.49.0 (read-only): the resource manager returns `ErrStreamOrConnLimitExceeded`
  wrapping `fmt.Errorf("cannot reserve outbound stream: %w", network.ErrResourceLimitExceeded)`, and
  every hop up to rangesync wraps with `%w` (the scope's `wrapError`, swarm `Conn.NewStream`,
  `BasicHost.NewStream`, bee's `newStreamForPeerID` and `NewStream`). A refusal on the peer's side
  resets the new stream (`StreamResourceLimitExceeded`): this node sees a remote `*network.StreamError`,
  which unwraps to `network.ErrReset` (and its transport error), never to
  `ErrResourceLimitExceeded`. After the open, yamux ignores a failed memory reservation (it only
  stops growing the window), so the refusal shows only at the open. `openFetchStream` wraps such an
  error with `ErrLocal`: the reconciler's `localCause` (no backoff) and stripes (reassigned unscored,
  as for any fetch error). Charging is moot since item 1. The check imports
  `github.com/libp2p/go-libp2p/core/network` into `pkg/rangesync`, the first package outside
  `pkg/p2p` to do so (no `go.mod` change). Session and Status opens are not classified (Follow-ups).
- **(T4.5 part 2c, item 3) Docs and tests of part 2b.**
  - Correction: fix round 3's "the headers exchange outlasts it only when `HeadersRWTimeout` is set at
    or above `FetchTimeout`" was wrong. basichost's `IdentifyWait`, and the full multistream
    negotiation when the peer left the protocol out of identify, obey only the caller's context, so
    an open could stall for the whole `FetchTimeout` (15 s), which round 3 judged a stall violation
    at once. Since item 1 the open has a context of its own and a held open is neither a violation
    nor charged. The review's test (held open, `fetchTimeout` 100 ms → `ErrInvalidMessage`, one Fail,
    disconnected) is adapted to that: `TestFetchStreamOpenHeld` gets `context.DeadlineExceeded`, no
    Fail, no disconnect, no claim while it waits and no fetch rate.
  - Status gate: the stall violation of a Want (`no deliveries within FetchTimeout`, after the
    claims) is judged only while the peer has a Status, like the shortfall rules and the reset
    budget. A peer whose Status is gone at the deadline (the disconnect hook ran) is not scored
    (`TestFetchStalledHolderGone`). This narrows fix round 3's (Low 3) "A violation of a peer without
    a Status ... is scored, each time" for stalls; a responder-side violation still is.
  - The cross-path double Fail is covered: `TestViolationMarks` adds a requester violation then a
    responder violation of the same peer, and the reverse, one Fail each. Both already held.
- **(T4.5 part 2c, item 4) `peers{state}`.** `rangesync_peers{state}` is a collector that counts the
  reconciler's peers by the state of their machine under `r.mu` when it is collected, with every
  state reported (0 when empty): probing, unsupported, idle, reconciling, fetching, striped, and
  removed (dropped from sessions, still connected). It is registered with the reconciler's metrics.
  A peer backing off counts as idle; no separate label (Follow-ups).
- **(T4.5 part 2c, item 5) Session ranges are trimmed around registered stripe ranges.** This
  supersedes T3.2 part 2's "a range overlapping (containing or contained in) a registered one is
  dropped and resurfaces in a later round" for the ranges sessions hand to the Striper. Prefix ranges
  overlap only by containment:
  - a range inside a registered range is still not registered;
  - a session's range that contains registered ranges is trimmed (`trimStripe`): split into halves
    until each part is covered or holds no registered range, which gives the fewest prefix ranges
    over what is left. Each part is a job of its own, registered and removed by its own range, so the
    worker's bookkeeping is unchanged;
  - a bootstrap or radius job that contains registered ranges is still not registered: `started`,
    the radius half and its cancel are those of one job, and its round tries again.

  Each part counts against `maxStripeRegistry = 1024`. The walk stops at the room left; the parts past
  it are dropped and counted once in `stripes_total{dropped}`, and `enqueueStripes` reports false.
  The work is bounded by the room and the registry size, not by the depth of the registered ranges
  (a covered part is exactly a registered range, so at most 1024 of them), so a peer that hands deep
  ranges cannot make it expensive.
- **(T4.5 part 2c, item 6) Digests compare over the session root.** This supersedes T1.11's
  "different roots are compared as different" and extends the T4.3 fix (M2) rule. A Status
  summarizes its sender's root; when the peer's root lies inside this node's, that root is the session
  root, and the round compares the peer's summary with this node's summary of that range, read from
  the fingerprint index by the new additive `Service.RangeSummary` (no address-index scan, the index's
  own errors). The reconciler uses it through an optional interface (`rangeSummarizer`); the narrow
  `reconciler.Service` is unchanged. `compareDigests` reports whether the digests are comparable and
  equal:
  - comparable and equal: the peer's round ends as an equal digest (`eventEqualDigest`);
  - comparable and different: a round counts as finding differences (M2 now reads "a peer whose
    digest is comparable over the session root differs", was "a peer with the same root");
  - not comparable (this node's root lies inside the peer's, or the index cannot summarize the
    range): different, and the interval adapts on what sessions find, as M2 had it.

  `recheckSession` uses the same comparison. The takeover wait keeps its same-root rule, since only
  equal roots give both sides the same digests. Effect: when the wide side of an in-sync nested pair
  initiates, it no longer runs a session every round; the narrow side takes over after
  `2 × MaxRoundInterval + jitter`, so such a pair costs one session per 60 to 90 s instead of one per
  round. When the narrow side initiates nothing changes, and cannot without a wire change (G2): the
  peer's Status carries only its own root's summary. `TestRangeSummary` checks that a node's
  `RangeSummary` of a range equals the Status summary of a node whose root is that range. The round
  reads the index under `r.mu`; the index takes only its own read locks, briefly, and never blocks on
  the reconciler (its root-change signals are non-blocking).
- **(T4.5 part 2c, item 7) Cleanups.** `fetch_skipped_total{reason}` keeps its name: an ID in the
  reserve (before the claim and after it) is now `in_reserve`, and `held` is the want set's alone.
  `(*Session).pendingReplies` moves into `PendingReplies` in `export_test.go`.
- **(T4.5 part 2c) Tests changed by design.** `TestFetchStreamOpenCharged` becomes
  `TestFetchStreamOpenNotCharged` (adds a peer whose inbound streams are full).
  `TestFetchClaimRechecksReserve` sees a stream open and end without a Want, not no stream.
  `TestFetchStalledHolder` and `TestFetchWithResultFailed/stalled` send a Status first (the gate).
  `TestEnqueueStripesDedup` expects the super-range trimmed. `TestFetchStreamOpenHonest` keeps its
  cases; its budget wording went.
- **(T4.5 part 2c) Contracts and scope.** These are unchanged: the wiring, the narrow
  `reconciler.Service` and its optional interfaces (one more, optional), the `SyncRate` guard,
  `storer.Syncer`, `TestCombinedSyncerBothDrivers`, and the T4.x rules not superseded above. New
  exports, all additive: `rangesync.(*Service).RangeSummary`, the `rangesync_peers{state}` metric,
  the `in_reserve` reason. There is no `pkg/node`, `go.mod` or proto change, and no new parser of
  peer input; `followup-fuzzing.md` gains a line for the changed invariants of the T1.10 target.
  `Local.Symbols` cancellation stays a follow-up (not trivial), and so does the per-peer fetch credit.

- **(T4.5 part 2c fix round 1, High) A failed fetch open backs off the peer's opens; served
  sessions' fetches share one fetcher per peer; waiting fetches are bounded.** This supersedes the
  part 2c item 1 entry's "What a failed open costs now" where it says the open costs nothing but its
  stream slot: it is still never charged or scored, but it now backs off. The review's attack: a peer
  starts a served session every 2.5 s (`PeerSessionInterval`), each listing the same 256 or more junk
  IDs, and stalls every fetch open up to the fetch timeout. Nothing was claimed, so the queued calls
  never merged; nothing was scored; calls arrived at about 0.4/s while two slots drained about 0.2/s,
  and each queued call held a catch-up fetch, so `SyncRate` stayed at 1 and `isFullySynced` false
  (storage incentives off, no radius decrease). The fix:
  - *Open backoff* (`openGate`). An open that fails or times out backs off the peer's opens for
    `openBackoffBase = 5s`, doubling with each failed probe up to `maxOpenBackoff = 5min`. While it
    runs, the peer's fetch calls fail at once (`errOpenBackoff`), before they wait for a stream slot
    and again after, without a claim or a score. Once it ends, one open probes the peer at a time; an
    open that succeeds ends the backoff, and a disconnect clears it. Opens in flight when the first
    fails count with it, so one transient failure is one backoff of 5 s. An open this node ends, or one
    that finds the peer gone (`p2p.ErrPeerNotFound`), does not back off, and a failure recorded for a
    peer whose Status is gone meanwhile is cleared (the disconnect hook may have run first). The gate
    reads the clock only for a peer it backs off.
  - *One fetcher per peer for served sessions* (`fetchServed`). The wants that sessions this node
    served list while the peer's fetcher runs are queued, merged without duplicates and capped at
    `maxServedPending = MaxClaimsPerPeer` (the next sessions list the rest again), and fetched as one
    batch after it. A catch-up batch counts towards `SyncRate` while it runs, the queue does not, and
    the next batch is counted before the last one's count ends. This resolves "(T4.5 part 2b) One
    wants fetcher per peer" for served sessions; retries already run one at a time per peer.
  - *Waiters* are bounded per peer: past `maxFetchWaiters = 16` calls waiting for the peer's stream
    slots, a call fails at once (`errFetchQueueFull`). Honest fetches from one peer are fewer: a round
    fetch, one retry, the served fetcher, and the stripe sessions.

  Against the attack, the peer now has at most one catch-up fetch in flight, opened once per backoff,
  and the batches in between fail in microseconds. What remains is the probe: it may stall for the
  fetch timeout (15 s) while its batch counts, so the peer can hold `SyncRate` at 1 for 15 s per
  backoff, about 5% of the time once the backoff has reached its maximum (after about five minutes),
  more before that, and k such neighbors up to about k times that (Follow-ups). Honest-safety: a peer
  whose inbound streams stay full is still neither scored nor disconnected; its fetches resume at most
  `maxOpenBackoff` after its streams free up, and other peers serve the IDs meanwhile. A transient
  failure costs that peer's fetches 5 s: round and served fetches fail (their IDs come back with the
  next session), and stripes are reassigned unscored. This node's own resource refusal backs off the
  peer's opens too, since rangesync can no longer tell it apart (Low 1). That is not the reconciler's
  session backoff nor a score, and opening fewer streams is the right reaction to it, but it delays
  that peer's fetches by up to `maxOpenBackoff` after this node recovers (Follow-ups).
- **(T4.5 part 2c fix round 1, Low 1) No `ErrLocal` for this node's resource refusal.** This
  supersedes the part 2c item 2 entry. Its "the reconciler's `localCause` (no backoff) and stripes" was
  wrong: fetch errors never reach `localCause`, which judges session errors only, and stripes'
  `localBusy` ignores `ErrLocal`, so the wrap changed no behavior. The wrap and the
  `github.com/libp2p/go-libp2p/core/network` import are gone; no package outside `pkg/p2p` imports it.
  The read of the error chain stands (this node's refusal wraps `network.ErrResourceLimitExceeded`,
  the peer's is a reset) for a `pkg/p2p` sentinel (Follow-ups).
- **(T4.5 part 2c fix round 1, Low 2) Nested initiators are taken over at once during start-up.**
  Since item 6, the wide side of an in-sync nested pair runs no session, so on the narrow side the
  initiator's `awaiting` entry cleared only at the narrow side's own takeover, 60 to 90 s later, and
  `SyncRate` stayed at 1.0 that much longer after a restart (up to `StartupGuardTimeout`). Now, while
  the start-up guard holds, a peer that initiates, has not run a session with this node and whose
  digest is not comparable over the session root is taken over at once. This narrows, for that case
  only, the T4.3 fix round 2 rule that a peer with another root is taken over after
  `2 × MaxRoundInterval + jitter`. It costs one session per such peer at start-up, which the wide side
  ran before item 6. Once the peer has run a session with this node (`Succeed`, `served`) or the guard
  is released, the usual wait applies. If the wide side starts at the same time, the existing
  collision rules apply. Clearing `awaiting` instead was rejected: the narrow side cannot compare the
  digests, so it would release the guard without knowing it is in sync.
- **(T4.5 part 2c fix round 1, Low 3) Tests.** `TestFetchInboundFullNotDisconnected` now holds every
  open for the fetch timeout and asserts each neighbor's backoff on the fake clock (at most seven
  opens in five minutes, then a fetch once the streams free up). The High: `TestFetchStalledOpensBackOff`
  (the review's PoC: 80 arrivals, at most 12 calls in flight, at most two opens, no claim, no score;
  59 in flight and growing before the fix) and `TestWantsStalledOpens` (the reconciler over a real
  service: at most one catch-up fetch in flight, one open, the count ends with the sessions). Also new:
  `TestFetchStreamWaitersBounded`, `TestFetchBackoffSlotsBusy`, `TestOpenGate`, `TestWantsServedFetcher`,
  `TestStartupGuardNestedInitiator`. Changed by design: `TestFetchStreamOpenNotCharged` (each open is a
  probe after the last backoff; adds this node's resource limit, and `TestFetchStreamOpenLocalLimit`
  is gone), `TestFetchStreamOpenHeld` (the backoff between held opens), `TestFetchStreamOpenHonest`
  (a transient failure backs off for `OpenBackoffBase`; a peer that is gone and this node's
  cancellations do not; a disconnect during a failing open leaves no backoff),
  `TestSyncRateSteadyStateUploads` (distinct IDs, as served wants now merge duplicates).
- **(T4.5 part 2c fix round 1) Contracts and scope.** Unchanged as in part 2c. No exported identifier
  is added; the new errors are unexported. There is no `pkg/node`, `go.mod` or proto change.

- **(T4.5 part 2c fix round 2, High) A catch-up fetch in flight holds `SyncRate` up only while the
  stored-chunk rate is above 0 or within a node-wide catch-up grace.** This supersedes, for catch-up
  fetches in flight, T1.11's "`SyncRate` is at least `StartupGuardRate` (1.0) while a catch-up fetch
  is in flight" (phase 1, "T1.11 score/rate"), T3.2 part 1's "`TransferStripes` holds
  `catchUpFetches` for its whole run ... so SyncRate reports at least `StartupGuardRate` throughout"
  and T3.5's "on top of `catchUpFetches`" (phase 3), part 1's contract "the `SyncRate` guard" as far as
  it held catch-up fetches in flight, and fix round 1's "A catch-up batch counts towards `SyncRate`
  while it runs". The review's attack: a catch-up fetch held `SyncRate` at 1 until it ended, stored
  chunks or not. (a) A Sybil whose served session lists 256 or more junk IDs starts a probe and stalls
  it for the fetch timeout (15 s); the storage incentives agent samples `isFullySynced` once per
  phase at a time derived from the public round, so one Sybil covered about 60% of its 25 s poll and
  two about all of it. (b) A disconnect clears the open backoff and a fresh overlay costs about
  2^depth hashes, so a reconnecting Sybil held a probe about 75% of the time, which also defeats the
  reserve worker's 15-minute tick. (c) A held Want (the open succeeds, the IDs are claimed, the Want
  is held for 14.9 s, then reset) pins it the same way for one reset token. The rule now:
  - The chunks a catch-up stores count towards the rate as they are stored, not when the fetch
    returns: the new additive `rangesync.(*Service).FetchWithProgress` calls back per stored chunk,
    and the reconciler uses it through an optional interface (`progressFetcher`). A Service without
    it (the tests' fakes) counts at the end, as before. Stripe transfers and their verification
    count the same way.
  - A catch-up fetch in flight raises `SyncRate` to `StartupGuardRate` only while the rate is above 0
    (a catch-up stored a chunk within the window) or the catch-up grace holds. The grace
    (`catchUpGrace = 2 s`) is one budget for the node: it drains while any catch-up fetch is in flight
    and is refilled, to 2 s, only when a catch-up stores a chunk. It is never renewed per peer, per
    fetch, per overlay or with time.
  - Unchanged: a registered stripe range, a pending radius half (at most `radiusHold`) and the
    start-up guard (at most `StartupGuardTimeout`) each hold `SyncRate` at least at
    `StartupGuardRate`, as T3.5 and T1.11 have them; `CatchUpMinDiff`; the 10-minute window.
  - Rejected: a grace per fetch, since the attacker starts fetches at will (a served session per
    `PeerSessionInterval`, new overlays, reconnects that clear the backoff): a reconnecting Sybil
    would hold the rate for about grace / (grace + reconnect time). A grace refilled with time, since
    the attacker would save it for the sampling instant of each round.

  Numbers. Honest: a catch-up's first chunk needs an open, the fetch rate (a full burst at the start
  of a catch-up), the claims, the Want and one delivery, well under 2 s on a working link. From then
  on the rate is above 0 for 10 to 20 minutes after the last chunk stored, so a catch-up that keeps
  storing, however slowly, keeps `SyncRate` above 0 and the radius does not drop under it. A catch-up
  whose first chunk takes longer than the grace, with no catch-up chunk in the window, reads 0 until
  that chunk: accepted (the reserve worker ticks every 15 minutes, the agent samples once per phase),
  and the same if an attacker drained the grace first. Attacker: at most 2 s of `SyncRate` above 0
  per catch-up episode of this node (a refill needs a chunk this node lacked, accepted and stored),
  whatever the number of peers, overlays, reconnects, stalled opens or held Wants. Before, 15 s per
  probe per peer. Even timed perfectly, 2 s cover the agent's sample within its 25 s poll with about 8%
  chance, once per episode, and the end of an episode is not the attacker's to choose. Keeping the
  window itself above 0 needs chunks this node accepts (Follow-ups).

  Tests: `TestSyncRateStallingPeers` (fake clock; stalled opens and held Wants that claimed their IDs;
  after the first 2 s, 0 throughout 360 staggered stalls from eight overlays over 30 minutes, at most
  four in flight, the same overlays coming back as reconnects; a stored catch-up refills the grace
  once), `TestWantsStalledOpens` (real service, see Low 5), `TestSyncRateCatchUpStoring` (honest, real
  service: a catch-up that stored one chunk reads at least `StartupGuardRate` past the grace and above
  0 within the window, 0 with its fetch in flight after two windows without a chunk, above 0 once it
  stores the rest), `TestFetchWithProgress`. Changed by design: `TestSyncRateLargeCatchUp` (also 0
  past the grace while nothing is stored). Passing unchanged: `TestSyncRate*`, `TestStartupGuard*`,
  `TestDualModeSyncRate`, `TestWantSetCatchUpRetry` (its 1 s advance stays within the grace) and
  `pkg/node`'s `TestCombinedSyncerBothDrivers`.
- **(T4.5 part 2c fix round 2, Low 1) An open backoff is the peer's Busy for stripes.** This
  supersedes fix round 1's (High) "stripes are reassigned unscored" for a peer whose opens back off.
  An open backoff failed a stripe's fetch at once without progress, so a stripe tried each covering
  peer in turn within microseconds and could end; under this node's own stream refusal every peer
  backs off for up to `maxOpenBackoff`. Now:
  - `rangesync.ErrOpenBackoff` is exported (was `errOpenBackoff`) and
    `rangesync.(*Service).FetchOpensBackOff(peer)` reports the backoff (a probe in flight included);
    the reconciler reads it through an optional interface (`openBackoffReporter`).
  - A stripe attempt opens no session with a peer whose opens back off: that counts as the peer's
    Busy (off the stripe clock, `stripeRetryInterval` apart, at most `maxStripeBusyRetries`), after
    which the stripe is reassigned, as for Busy. An attempt whose session ran and whose fetch found
    the backoff is restarted with the same peer (within `maxStripeRestarts`), so its next attempt
    waits the same way.
  - `stripePeers` leaves peers whose opens back off out of `ready`, so `planStripes`, the
    reassignment fallback of `nextStripePeer` and `verifier` choose other covering peers first, and
    such a peer only when no other covers the range. A stripe's own planned peers are not re-checked
    when it starts; the Busy wait covers them.

  Nothing is scored. Tests: `TestTransferStripesOpenBackoff` (waited for; reassigned once the waits
  run out, no session with the peer; restarted after its fetch found the backoff),
  `TestStripePlanAvoidsOpenBackoff`.
- **(T4.5 part 2c fix round 2, Low 2) The served queue skips the IDs of the running batch and of the
  batch before.** This narrows fix round 1's "merged without duplicates". A session that completed
  while the peer's batch ran could list again an ID the batch was asking for; when the peer had
  evicted it meanwhile (honest churn), the next batch asked again, `relisted` counted it as junk
  listed again, and two such fetches within `shortfallWindow` disconnected an honest peer
  (`TestWantsServedChurn` before the fix: asked twice, scored 1). The queue's `seen` is now seeded
  with the running batch's IDs and, from the second batch on, with the batch before's too. An ID that
  a dropped listing needed for another cause (held by another claim, deferred) comes back with the
  next session, as any ID past `maxServedPending`. Tests: `TestWantsServedChurn` (real service),
  `TestWantsServedBatchesNotRequeued`.
- **(T4.5 part 2c fix round 2, Low 4) A probe that ends after `clear` leaves a newer backoff alone.**
  `openGate.begin` returns the backoff a probe belongs to, and `end` resets `probing`, or counts a
  failure, only on that backoff. Before, a probe whose backoff a disconnect cleared reset `probing` on
  the peer's newer backoff (two probes at once) and counted as its failure. `TestOpenGate` covers it.
- **(T4.5 part 2c fix round 2, Low 5) Tests.** `TestFetchDisconnectClearsOpenBackoff` fails once the
  disconnect hook no longer clears the open backoff (it backs off before the disconnect, so the
  `LastStatus` check in `openFetchStream` does not mask it). `TestWantsStalledOpens` now runs the
  reconciler and the service on one fake clock, reaches four probes and a reconnect, and checks that
  `SyncRate` reads 0 at each, that no open starts while the backoff runs and that at most one catch-up
  fetch is in flight; its old comment claimed the peer "cannot hold `SyncRate` above 0" without
  reaching a probe. For it, `rangesync.Options.Now` (additive) sets the service's clock, which the open
  backoff and the shortfall windows run on (nil: `time.Now`); the fetch timeout stays on the real
  clock.
- **(T4.5 part 2c fix round 2) Contracts and scope.** Unchanged: the wiring, the narrow
  `reconciler.Service` (two more optional interfaces), `storer.Syncer`, `TestCombinedSyncerBothDrivers`
  and the T4.x rules not superseded above. New exports, all additive:
  `rangesync.(*Service).FetchWithProgress`, `rangesync.(*Service).FetchOpensBackOff`,
  `rangesync.ErrOpenBackoff` (was unexported) and `rangesync.Options.Now`. There is no `pkg/node`,
  `go.mod` or proto change, no libp2p import in `pkg/rangesync`, and no new parser of peer input.

- **(T4.6 part 1) `pairwise_diff` is added; `pairwise_diff_half` stays as it is.** The new histogram
  `pairwise_diff` holds n_Δ (the elements only one side holds) per completed reconcile session, and
  both sides of a session observe it. `pairwise_diff_half` keeps its name, buckets and meaning (the
  wants of this side), because it is the quantity `CatchUpMinDiff` is compared with (T1.11, T3.5) and
  a dashboard may already read it. Only the unexported struct field and the `export_test.go` accessor
  are renamed (`PairwiseDiffHalf`). An aggregate over a neighborhood counts each session twice, which
  leaves its quantiles unchanged.
- **(T4.6 part 1) How one side knows n_Δ: `Session.Difference` (additive).** The rateless decoder
  gives both directions only on the side that decodes. In the steady state that is the initiator,
  and the responder of a root symbol stream sees only the listing of the IDs it lacks. So
  `Difference` counts, per resolved range:
  - for a symbol stream this side decoded, the IDs recovered in both directions (exact);
  - for every range resolved by items, this side's wants (exact) plus the local IDs the peer lacks,
    from the counts both sides already exchanged: |only local| = cL − (cR − wants), within [0, cL].
    cL is this side's count (its listing, Fingerprint or symbol stream). cR is the peer's count (its
    complete listing, or the count of its Fingerprint for a stream the peer decoded).

  Ranges taken over by the Striper, and ranges never answered (equal), count nothing. The value is
  exact when neither side changes during the session. `TestSessionConverges` (every case, both
  forms), `TestSessionExclusionCap` (answers past `MaxPendingReplies`), the corrupt, exhausted and
  stale-stamp symbol tests and `TestProtocolConverges` (both sides, all sizes) assert that both
  sides report the true n_Δ; a mutation of each of the three counting paths fails them. Under churn
  the value is approximate. A peer that lies about its counts moves it within the local count of a
  range. Nothing acts on it. In the simulations it is never below the session's wants.
- **(T4.6 part 1) `pairwise_diff` buckets: 0 and 2^k − 1 for k = 1 … 17.** A difference is an
  integer, so each bucket holds exactly one octave, and `le="255"` counts exactly the sessions below
  `CatchUpMinDiff` (Q9: the p99 is below 256 if and only if at least 99% of the samples are in
  `le="255"`). The top bound, 131071, is past 2 × `LargeDiff`. Octave resolution matches the
  power-of-two constants that the metric tunes (caches 4096/512/32, `MinBatch` 4, `CatchUpMinDiff`
  256). The simulation gets every sample exactly (`OnPairwiseDiff`), so part 3's p50 and p99 do not
  depend on the buckets. `TestPairwiseDiffBuckets` pins the layout.
- **(T4.6 part 1) The harness: `runNeighborhood(tb, simOptions) *simResult`** in
  `pkg/rangesync/neighborhood_sim_test.go` (package `rangesync_test`, so that it can use rangesync's
  `export_test.go`; it uses the reconciler's exported API only). Each of the N nodes has a real
  `rangesync.Service`, a real `reconciler.Reconciler` and a mock reserve, wired as in `pkg/node`
  (scorer, wants, `SetService`), in a full mesh at radius 0. Every node starts with the same `Base`
  chunks, so that no node bootstraps. Each upload is put into the reserves of p random nodes.
  - Options: `Nodes`, `Replication`, `Seed`, `Base`, `Batches`, `BatchSize`, `BatchInterval`,
    `Warmup`, `ChunkSize`, `Tail`, `Limit`, `Settle`, `MinRoundInterval`, `MaxRoundInterval`, and the
    `Reconciler`/`Service` hooks, which adjust each node's options.
  - Result, counted from the first upload:
    - per batch, the convergence time and the most rounds a node ran;
    - per node, the uploads, the chunks stored, the rangesync deliveries, the rounds,
      `bytes_metadata` and `bytes_chunks`, `want_claims` by result, and the exposed `pairwise_diff`
      count;
    - every `pairwise_diff` sample, with its time and node;
    - reconciler `sessions_total` by result;
    - the duplicate deliveries, the protocols of the opened streams, the fake-clock rate check, the
      convergence time, the last session after it, and the steps and wall time.

    Helpers: `diffsFrom` and `quantile`.
- **(T4.6 part 1) A fake clock that moves only while the neighborhood is quiet.** All reconcilers and
  services share one clock (`Options.Now`/`After`, `rangesync.Options.Now`). The harness counts the
  reconcilers' calls into their services and the stream handlers, from `NewStream` until the handler
  returns, as in flight. It moves the clock to the next pending timer only when nothing is in flight
  and nothing has started for `Settle` (15 ms of real time). So the work of one instant ends at that
  instant, and the times measure the protocol's waits, not the host. Three parts run on the real
  clock:
  - the Index's root-change debounce, which is not injectable: the reconciler's service view
    (`simService.SubscribeRootChange`) is driven instead by a debounce on the fake clock, from the
    reserve's put hook;
  - the per-peer Status and session limiters (`pkg/ratelimit` takes no clock): they are lifted by
    the `export_test.go` hook `DisablePeerRateLimits`, and the harness checks the same token buckets
    on the fake clock (`RateExceeded`, 0 in every run);
  - the fetch limiters and the fetch timeout: they are kept, and a fetch that waits on them is in
    flight, so it holds the clock.

  A clock moved too early, by a goroutine starved for longer than `Settle`, can only delay a
  convergence by one timer step. The overlays, chunks, targets and jitters are seeded. Which session
  claims a chunk first within one instant is up to the scheduler, so byte counts vary by about 10%
  between runs. The assertions are bounds and invariants.
- **(T4.6 part 1) Stream bytes go through an in-test pipe.** streamtest's record deadlocks when a
  writer runs more than 16 writes ahead of its reader (the T4.4 note and `TestFetchManyWants`), and a
  fetch of more than 16 chunks does that. streamtest still routes each stream
  (`WithPeerProtocols`), runs the peer's handler with the peer's address, and supplies the headers,
  the version and the `RecorderDisconnecter`. The bytes go through `simPipe`, an unbounded buffer
  with streamtest's closing rules: `Close` ends the writes, `FullClose` and `Reset` also end the
  reads, and neither waits for the remote. The two ends are paired by a header that a `Headler` of
  the wrapped protocol echoes. `pkg/p2p` is unchanged. Deliveries are counted from the kept bytes of
  the fetch streams. A run with 120 uploads per batch at N = 17 (5040 deliveries) completes in 2 s.
- **(T4.6 part 1) Round interval bounds as reconciler options (additive).** The task asks for the
  harness to take `RoundInterval` bounds. `Options.RoundInterval` fixes min = max and moves the round
  timer to the real clock, so it is unusable on the fake clock. The new
  `reconciler.Options.MinRoundInterval` and `MaxRoundInterval` override the bounds and keep the timer
  on `After`, as `StartupGuardTimeout` and `StripeTimeout` already override theirs. A maximum below
  the minimum is raised to it, and `RoundInterval` takes precedence. The node leaves them zero.
  `TestRoundIntervalBounds` covers the adaptation and the trigger floor, and
  `TestNeighborhoodRoundBounds` covers the harness plumbing (a mutation that drops the option fails
  it).
- **(T4.6 part 1) The convergence bound: 2 × `StatusMinInterval` + `MinRoundInterval` = 15 s, and at
  most 3 rounds of any node.** Within `StatusMinInterval` a holder's index signals the change (or a
  timed round sees it sooner). The signal sends the holder's Status (deferred to at most
  `StatusMinInterval` after its last change send to that peer) and triggers its round, no sooner
  than `MinRoundInterval` after its last round. That round opens sessions with every higher-overlay
  lacker. A lower-overlay lacker has the Status by 2 × `StatusMinInterval`, which triggers its round
  within `MinRoundInterval`. The want set lets each lacker fetch each upload once (§7.8: one round,
  no redundant fetch). The adaptive interval does not add, because the triggers do not wait for the
  round timer. Rounds are at least `MinRoundInterval` apart, so a node runs at most 3 in the window.
  §7.8's I = 1 round counts no gossip latency; the rest of the bound is the debounce and the Status
  schedule.

  Measured (4 batches of 8 uploads, 17 s apart, from 61 s, which is off the 5-s grid of quiet rounds
  and heartbeats; 20 runs each at `-cpu=1,4`):
  - N = 5: 5 s and 1 round per batch;
  - N = 9: 5 s and 1 round (a batch at 0–3 s when a timer coincided);
  - N = 17: 5–10 s and 1–2 rounds. The second round comes from sessions that went stale while they
    waited for one of the 8 slots, because every node holds some uploads and lacks others, so its
    root moves under them. Busy answers at the node session limit add to it.

  There were zero duplicate deliveries in every run.
- **(T4.6 part 1) Invariants the tests assert for free.**
  - Each node stores and receives exactly the uploads it lacked.
  - `want_claims`: `expired` and `late` are 0, and `granted` ≥ stored. A granted claim is released
    unrequested when the reserve check after the claim (T4.4 fix round 1) finds the chunk stored by
    another fetch.
  - Only `rangesync` streams are opened (no `pullsync`).
  - No Status or session exceeds a peer's rate on the fake clock.
  - No session fails (`sessions_total{error}` is 0).
  - `pairwise_diff` has 2 × `sessions_total{ok}` samples, all below `CatchUpMinDiff`, summing to at
    least twice the chunks fetched, and the histogram exposed through `Service.Metrics` has the same
    count.
  - The last session starts within `StatusMaxInterval` + 2 × `MaxRoundInterval` of convergence
    (AC-C2; measured 5 s).
- **(T4.6 part 1) Tests and wall times** (single run, this host):
  - `TestNeighborhoodConverges` (N = 5): 0.9 s;
  - `TestNeighborhoodSREP`: N = 9 0.9 s, N = 17 1.2 s, skipped with `-short`;
  - `TestNeighborhoodRoundBounds`: 0.6 s;
  - `TestPairwiseDiffBuckets`: < 0.01 s;
  - `TestRoundIntervalBounds`: 0.2 s.

  No integration tag is needed. Scope:
  - Production changes: `Session.Difference`, the `pairwise_diff` metric and the two reconciler
    options.
  - New hooks in `export_test.go`: `DisablePeerRateLimits`, `OnPairwiseDiff`,
    `PairwiseDiffBuckets`, and `PairwiseDiffHalf` (renamed).
  - There is no `pkg/node`, `pkg/p2p`, `go.mod` or proto change, no libp2p import in
    `pkg/rangesync`, no new parser of peer input, and no `*_fuzz_test.go`.
- **(T4.6 part 1) Correction: `TestNeighborhoodRoundBounds` reads the interval gauge, not round
  counts.** This corrects the "Round interval bounds as reconciler options" entry above. The first
  version counted each node's rounds over the run, and it failed in 2 of 30 loaded runs (10 or 11
  rounds against at least 14). On the quiet stretches a round runs in the loop goroutine with no call
  in flight. If that goroutine is starved for longer than `Settle` before it re-arms its timer, the
  harness moves to a later timer, and the round runs late. The test now checks each node's
  `round_interval_seconds` after the quiet tail (`simNodeResult.RoundInterval`: the overridden
  maximum, 8 s, against 30 s without the option), which does not depend on timing. A mutation that
  drops the option still fails it. The other assertions do not count rounds over idle stretches. A
  late round can only lengthen one convergence by one timer step, and 60 more loaded runs at
  `-cpu=1,2,4` stayed within the bound. Waiting until every fired timer channel is read would not
  help: about 80% of them are abandoned, because the gossip loop re-arms on every wake.

- **(T4.6 part 1 fix round 1, High) The serving side of a decoded stream records hint(R).** Before,
  only the decoding side wrote the hint (`session_symbols.go`, on `Done`) and only the deciding side
  read it (`decide`). In the steady state the lower overlay initiates and decodes the higher's root
  stream, so the side that decides never had a hint (the review measured 859 reads, 0 non-zero, at
  N = 17). `TestSessionSymbolsHints` hid it: both sessions shared one hints map. Now the serving side
  records, once the peer's listing of a stream it decoded ends (`Items` without `Reply` answering
  this side's stream), `servedHint` = min(n_Δ of the range, the symbols this side sent for it), where
  n_Δ = listed + (cL − (cR − listed)) as in `Difference` (cL: the count of this side's stream, cR:
  the count of the peer's Fingerprint). Nothing changes on the wire (G2), and the cache stays the
  4096-entry LRU keyed by (peer, range). Why the difference and not the symbols sent (the review's
  suggestion): after a miss the stream grew ×4, so its length is up to 4 × the batch it needed;
  starting the next stream there would keep that overshoot in every session and never shrink. The
  difference gives the next stream `FirstBatch(d)` as §6.3 intends.
  - The bound: a decoder recovers at most one ID per symbol it received, so an honest peer's n_Δ
    is at most the symbols sent, which are at most cap(R) and `SessionSymbolBudget`. A peer that
    lists more IDs or lies about its Fingerprint count raises the hint to the length of the stream it
    asked for, and this node's next first batch to at most `FirstBatch` of that (past cap(R) or
    `MaxBatchSymbols` the range is split instead). A Fingerprint whose count differs from this
    node's by as much already gets that batch (d̂ = |cL − cR|), so the hint adds no amplification.
    A peer can flush the shared LRU with one hint per decoded stream (Follow-ups); losing a hint
    only makes a first batch smaller.
  - Tests: `TestSessionSymbolsHints` (now one hints map per side: both sides record 20 after a cold
    session that needed a `More`, and the next first batch is `FirstBatch(20)`; it encoded the
    shared map, not the cold start), `TestSessionSymbolsServedHintBound` (a listing of 200 IDs after
    a 4-symbol stream records 4), `TestProtocolServedHint` (two services, the same initiator twice
    over a hidden difference of 20: the responder's first batch is `MinBatch`, then
    `FirstBatch(20)`). Dropping the record fails the first and the last, dropping the bound the
    second. `TestWorkedExamples`, `TestSession*`, `TestSessionSymbols*` and `TestProtocolConverges`
    pass unchanged: the worked examples run without hints, and `TestProtocolConverges`'s second
    session finds equal sets.
  - Measured with the harness (5 seeds each, 4 batches; mean and range; "before" is this round's
    harness with the serving-side record disabled):

    | N, uploads per batch | symbols per difference | `bytes_metadata{out}` per upload (of pullsync's N(N−1) × 102 B) |
    |---|---|---|
    | 9, 8 | 3.58 (3.37–3.97) → 3.73 (3.34–4.19) | 7.7 KB (105%) → 7.8 KB (106%) |
    | 9, 120 | 3.32 (3.19–3.47) → 2.84 (2.61–2.98) | 6.0 KB (82%) → 5.3 KB (72%) |
    | 17, 8 | 3.69 (3.45–3.91) → 3.68 (3.41–4.01) | 22.0 KB (79%) → 22.4 KB (81%) |
    | 17, 120 | 3.27 (3.04–3.37) → 3.43 (3.26–3.59) | 15.5 KB (56%) → 16.3 KB (59%) |

    `More` requests per upload (3 seeds, 120 per batch) fall from 0.63 to 0.20 (N = 9) and from 1.87
    to 0.76 (N = 17). So the hint removes about two thirds of the round trips but does not bring
    the symbols near §7.8's 1.35–1.72, and AC-B1 stays above 50%. The review's cause is only part of
    it: with 120 uploads per batch the count difference already gave many streams a fair estimate,
    and what remains is `FirstBatch` = 1.6 d + 2, which sits at the median of the symbols a
    difference d needs, so about half the streams miss and the `More` grows them ×4 (up to 6.4 d).
    With an exact estimate that costs about 0.5 × 1.6 + 0.5 × 6.4 ≈ 4 symbols per difference, and
    a random cold start growing ×4 about 3.2 (the ×4 geometric overshoot). Light loads are
    `MinBatch`-bound (d̂ ≤ 1, one to six differences per session). The constants are G6's
    (Follow-ups: the `GrowthFactor` and `BatchFactor` runs).
- **(T4.6 part 1 fix round 1, Medium 1) `pairwise_diff` is exact on the decoding side only.** This
  supersedes, in "(T4.6 part 1) `pairwise_diff` is added", the claim that an aggregate over a
  neighborhood "leaves its quantiles unchanged", and, in "(T4.6 part 1) How one side knows n_Δ", the
  reading that both sides report the true n_Δ beyond the churn-free tests that assert it.
  - The decoding side counts the IDs it recovered: the size of the difference between the two
    snapshots the streams came from, exact under churn too.
  - The serving side derived its value from its wants, which `HasID` filters: an ID it gained from
    another peer since it served the stream dropped the value by 2 (once from the wants, once from
    |only local|). It now counts the decoder's listing (`inStream.decodedDifference`), which the
    review measured fixes N = 5. What remains is the decoder's change between its Fingerprint (cR)
    and the snapshot it decoded against: an element it gained meanwhile that this side holds
    counts once more. The serving side cannot see that count without a wire change, and taking the
    decoder's snapshot at its Fingerprint would copy each cached prefix (up to 4096 symbols, ~350 KB)
    for every range it opens, most of them equal; so it stays approximate.
  - Measured (Σ over pairs of |initiator − responder| ÷ Σ initiator, from the first upload, 5
    seeds): 21% (15–27%) at N = 9 and 26% (13–39%) at N = 17 with 8 uploads per batch, 1.1% and 9.3%
    with 120. The responder is always the larger. The medians of all samples and of the exact ones
    were equal in every run; the p99s differ (N = 17, 120 per batch: 78–97 against 68–80). Q9 is
    unaffected (p99 8–10 at the default load, below 100 at 120 per batch).
  - Part 3 reads the decoding side: `SessionSample.Exact` (only decoded streams, or no difference
    at all) and `simResult.exactDiffsFrom`, which counts each session once (a session that found no
    difference is exact on both sides and counts its initiator's sample; a side that decoded a
    stream sends its peer a listing, which makes the peer's sample inexact). `TestSessionSymbolsDifferenceListed`
    gains an ID the decoder lists on the serving side before it handles the listing: both sides
    report 20 (the serving side reported 18 before; restoring the wants fails it).
- **(T4.6 part 1 fix round 1) Per-session samples: `OnSession` replaces `OnPairwiseDiff`.** The
  harness needs each side's role, exactness, peer and symbol counts, which the histogram hook could
  not carry. `Service` gets an unexported test seam, `sessionDone func(peer, *Session, initiator)`,
  called after the session's metrics in `ReconcileWith` and `handleReconcile`; it is nil in a node.
  `export_test.go`'s `OnSession(func(SessionSample))` sets it; `SessionSample` holds `Peer`,
  `Initiator`, `Difference`, `Exact`, `Recovered`, `SymbolsReceived` and `SymbolsSent`.
  `OnPairwiseDiff` is removed. The harness's samples (`simSample.Session`) carry it, and
  `simResult.symbolsPerDifference` is Σ symbols received ÷ Σ recovered (the overshoot and streams
  that did not decode included).
- **(T4.6 part 1 fix round 1, Medium 2) The harness models a one-way latency on the fake clock.**
  This supersedes the in-flight rule of "(T4.6 part 1) A fake clock that moves only while the
  neighborhood is quiet" and extends "(T4.6 part 1) The harness" (a `Delay` option, `Spans`,
  `StreamBytes`, `RoundGap`).
  - `simOptions.Delay` (default 0): every byte, close and reset a stream end writes reaches the other
    end `Delay` later on the clock (`simPipe` schedules the arrival), and a stream open costs one
    round trip (bee's headers exchange; the handler starts and waits for the first bytes). A session
    of k round trips then takes 2k × `Delay` after its open (an equal one 1, a decoded one without a
    `More` 2, each `More` one more), a Status send 2 × `Delay`, and a fetch the open plus its Want
    and Delivery legs.
  - Quiet now means: every call and handler in flight is parked (a goroutine reading a stream it has
    nothing to read on, or sleeping on the clock for an open) and nothing started for `Settle`. An
    event that can let a reader go on (an arrival, a close of its end) counts it as working again
    before it wakes it. Readers on an empty pipe park at `Delay` 0 too: their remote is in flight or
    parked itself. pkg/rangesync closes or resets every stream before a call or handler returns,
    which wakes a read it left behind, so no reader stays parked for a call that ended.
  - A fetch waits for one of the peer's `MaxFetchStreamsOut` slots in the harness before it counts
    as in flight (`simNode.fetchSlot`): a third fetch to a peer would otherwise wait inside the
    service while the fetch that holds the slot is parked, and hold the clock. The service's
    `maxFetchWaiters` path is not reached in the simulation.
  - `simResult.Spans` records every call of a reconciler into its service (kind, node, peer, start,
    duration on the clock, error), so part 3 can time sessions (Q3: round trips against overshoot),
    fetches (first chunk: open, Want and first Delivery, 4 × `Delay` with no limit wait) and the gap
    between a Status and the session it triggers. `TestNeighborhoodDelay` (N = 5, 100 ms) checks that
    every completed call takes a multiple of `Delay`, sessions whole round trips and at least 4 ×
    `Delay`, Status sends at least 2 × `Delay`, and that the uploads converge within
    `convergenceBound` plus the legs of a Status, a session with three `More`s and a fetch (measured:
    6.1–6.3 s). `TestSimStream` covers the closing rules, the delay and the parking. Mutations that
    send at once, drop the open's round trip or stop parking fail them.
  - Not on the injected clock, so part 3 cannot time them: the node-wide fetch limiter and the
    holder's delivery limiter (`pkg/ratelimit` takes no clock; a fetch that waits on one is in flight
    and its wait takes no simulated time), the fetch timeout (`context.WithTimeout`), the session
    and Status timeouts, and the want set's claim deadline and reject TTL (`wantset.New` takes no
    clock). A parked read lets the clock run while they count real time, so a stalled holder cannot
    be timed, and a fetch that spans `FetchTimeout` of real time (15 s; each clock step costs at
    least `Settle`) fails. Moving them is not cheap: the limiter needs a clock-aware replacement on
    the fetch path, the timeout an injected timer and a cause check where `fetch` tests
    `DeadlineExceeded`, and the want set an exported clock option; and the three only help together,
    with stall scenarios (Follow-ups). There is no bandwidth limit (a 4 KB Delivery takes the same
    leg as a Want), no jitter, and no protocol negotiation round trip.
  - Measured at 50 ms (1 seed each): N = 9 converges in 5.65 s per batch (wall 2.6–2.9 s), N = 17 in
    1.95–10.75 s (8 per batch) and 5.9–6.15 s (120 per batch), wall 3.9–4.1 s.
- **(T4.6 part 1 fix round 1, Low 6) A reset reaches the remote as a reset.** This supersedes "Reset
  also end[s] the reads" in "(T4.6 part 1) Stream bytes go through an in-test pipe". `simEnd.Reset`
  was a `FullClose`, so the remote read `io.EOF`: a responder that had sent its empty Message took an
  initiator's reset for the normal end. Now this end's and the remote's reads and writes fail with
  `errSimReset` (libp2p's `network.ErrReset`, which pkg/rangesync does not import), and the remote
  drops what it has not read. `FullClose` keeps its rules. `TestSimStream/reset` fails with the old
  `Reset`. The default runs are unchanged (no session or fetch fails in them).
- **(T4.6 part 1 fix round 1, Lows 2, 3 and 5) Round bounds and `Settle`.**
  - Low 3: `convergenceRounds` is `convergenceBound / MinRoundInterval + 1` = 4: the window from an
    upload to its convergence is closed, so rounds at 0, 5, 10 and 15 s all count. This supersedes
    "at most 3 rounds of any node" in "(T4.6 part 1) The convergence bound".
  - Low 2: `TestNeighborhoodRoundBounds` used a 2 s minimum, below what production allows:
    `PeerSessionInterval` (`MinRoundInterval / 2`) and the reconciler's `sessionBackoffBase`
    (`MinRoundInterval`) stay tied to 5 s, so a node with shorter rounds would trip its peers'
    session limits, which the simulation lifts. It now runs 7–8 s over 2 batches and also checks
    that no node runs two rounds closer than 7 s (`simNodeResult.RoundGap`; a late round only widens
    a gap) and `RateExceeded` = 0. Dropping the minimum override now fails it (rounds 5 s apart);
    before, only the maximum was checked. This extends "(T4.6 part 1) Correction:
    `TestNeighborhoodRoundBounds` reads the interval gauge".
  - Low 4: the 15 s bound is looser than AC-V1's 2 × `RoundInterval` (10 s at the minimum). The
    derivation reaches it: a lacker below every holder gets the Status by 2 × `StatusMinInterval`
    when the holders sent it a change just before, and its round may then wait `MinRoundInterval`.
    The measured 10 s at N = 17 is within both. T6.4 should reconcile AC-V1's wording (Follow-ups).
  - Low 5: `Settle` defaults to 15 ms, 150 ms under the race detector (`racedetection.On`).
- **(T4.6 part 1 fix round 1) Scope, tests and wall times.** Production changes: `servedHint` and
  `decodedDifference` in `session.go` (comments in `hints.go`, `session_symbols.go`), the
  `sessionDone` seam (`rangesync.go`, `reconcile.go`). Test hooks: `OnSession` and `SessionSample`
  (new), `OnPairwiseDiff` (removed). There is no `pkg/node`, `pkg/p2p`, `go.mod` or proto change, no
  libp2p import in `pkg/rangesync`, and no `*_fuzz_test.go`; the serving-side hint is derived from
  peer input, so `followup-fuzzing.md` gets a line. Single runs on this host:
  `TestNeighborhoodConverges` 0.8 s, `TestNeighborhoodSREP` 0.9 s and 1.2 s,
  `TestNeighborhoodRoundBounds` 0.8 s, `TestNeighborhoodDelay` 2.6 s, `TestSimStream`,
  `TestSessionSymbolsDifferenceListed`, `TestSessionSymbolsHints` and
  `TestSessionSymbolsServedHintBound` < 0.05 s, `TestProtocolServedHint` 0.07 s.

- **(T4.6 part 1 fix round 2, Medium) The harness's `FullClose` waits for the remote's close, as
  bee's does.** This supersedes, in "(T4.6 part 1 fix round 1, Medium 2) The harness models a
  one-way latency", the timings "a session of k round trips then takes 2k × `Delay` after its open
  (an equal one 1, a decoded one without a `More` 2, each `More` one more), a Status send 2 ×
  `Delay`", and "None of them waits for the remote" in `simEnd`'s comment. pkg/p2p/libp2p's
  `FullClose` closes its writes and then reads until the remote's EOF (`closeDeadline` 30 s; bytes
  instead are `errExpectedEof`, both reset the stream). Every rangesync path ends with it: the Status
  sender (it reports a send as done only once the peer closed), the round's session (the round calls
  `ReconcileWith` without a `Wants` callback, so its wants come back only after the close) and the
  holder's fetch handler. The harness returned at once. Now `simEnd.FullClose` closes its writes,
  waits parked for the remote's end of the writes (`simPipe.awaitEOF`) and resets the stream on
  bytes (`errSimExpectedEOF`), a reset, or `simCloseDeadline` = 30 s on the clock
  (`errSimCloseTimeout`; the clock's timers can now be stopped, so a wait that ends leaves no step
  behind). With `Delay` D a Status send takes 4D, an equal session 6D, a decoded one without a
  `More` 8D (the open, three round trips, the close's round trip) and 2D more per `More`, and a
  fetch that stored a chunk 4D with its first Delivery at 4D (the holder closes first, so the
  requester's close returns at once). `TestNeighborhoodDelay`'s floors are now Status ≥ 4D,
  sessions ≥ 6D, a fetch that stored ≥ 4D with its first Delivery at exactly 4D, and one that only
  opened ≥ 2D (reset without a Want); reverting the wait fails them (Status 2D, sessions 4D).
  `TestSimStream` covers the wait, bytes instead of the close, and the deadline.
  - Effect: at 100 ms and N = 5, the sessions per run fell from 80 to 40 and the zero-difference
    ones from 40 to 0 (the cause was not traced; a session's wants and its node's root change now
    come a round trip later). The round-1 remark that zero-difference sessions rise with `Delay`
    was measured with the immediate close (Follow-ups). The `Delay` 0 runs are unchanged in
    simulated time (the remote closes in the same instant).
  - Round trips per decoded session at the steady load (below), counting the open and the close:
    4.5 with the hint, 5.4–5.8 without; a `More` is a quarter of a no-`More` session, not a third.
- **(T4.6 part 1 fix round 2, Low 3) The harness's tests check what part 3 reads.**
  - `assertNeighborhoodWithin` now checks the samples: an exact one's difference equals what its
    decoders recovered, no side recovers more IDs than symbols it received, `exactDiffsFrom` has one
    sample per completed session, and `symbolsPerDifference` is at least 1. `TestProtocolConverges`
    checks that the exact samples hold the true n_Δ: one on the decoding side for every size resolved
    by symbol streams (both sides at 0), none for 100% (an empty side, item lists), none or one for
    4k (item exchanges at the leaves unless every level-1 stream decodes, which depends on the
    random IDs). Mutations that mark every sample exact, count both sides of an equal session, or
    drop the first-arrival time fail them.
  - `simSpan` now separates `Wait` (a fetch's wait for one of the peer's slots, which happens
    before the service runs it) from `Duration`, and records `Streams` (opened, from a `simCall` in
    the call's context, which pkg/rangesync passes on to `NewStream`), `Stored` and `FirstDelivery`
    (the first Delivery's arrival on the fetch streams, from the service's start).
  - First-chunk latency: `FirstDelivery` is the open, the Want and one Delivery leg, 4D. The holder
    writes all Deliveries of a Want in one instant and the pipe has no bandwidth, so they all arrive
    with the first, and the holder's delivery limiter and the requester's fetch limiter wait on the
    real clock (no simulated time). Part 3 can read the latency to the first chunk from
    `FirstDelivery`, but needs a bandwidth model (per connection, since streams share it) to time the
    spread of a large Want or a catch-up.
- **(T4.6 part 1 fix round 2, Low 2) AC-B1 in the harness: a modeled baseline, and a steady mode.**
  The harness divides `bytes_metadata` by N(N − 1) × 102 B per upload, the pullsync cost §7.8
  models (every node offered each upload by each of its N − 1 peers), not a measured pullsync.
  `logNeighborhood` now says "modeled" and logs the bytes received next to those sent: AC-B1 counts
  received bytes, and in the closed mesh every byte one node sends another receives, so the two sums
  are equal once the run is quiet (they were in every run). `steadyOptions(nodes, seed)` is the
  steady mode the review used, 24 batches of 20 uploads 1.3 s apart (~15 per second), so that
  sessions see a stream rather than separate bursts; `TestNeighborhoodSteady` runs it at N = 9 with
  50 ms (invariants and bounds as `TestNeighborhoodDelay`; 79% of the modeled pullsync, 3.1 s). Part
  3 should measure AC-B1 in this mode. §7.8's prediction counts only the symbols; the decoder's
  `Items` listing of the IDs only it holds (~70 B each, in about half of the p(N − p) pairs) adds
  about 9 points at N = 9 and 5 at N = 17 (the review's figures), as do the `Want` IDs AC-B1 already
  mentions.
- **(T4.6 part 1 fix round 2, Low 1) `simOptions.NoHints`.** G6 needs the hint on and off at a
  steady rate. `export_test.go`'s `DisableHints` sets the service's hint cache to nil, and
  `sessionConfig` (production, `reconcile.go`) gives a session no `Hints` then; a node never has a
  nil cache. `TestProtocolServedHint` has a "no hints" case (both first batches `MinBatch`).
- **(T4.6 part 1 fix round 2) Scope, tests and wall times.** Production: the nil check in
  `sessionConfig`. Test hooks: `DisableHints` (new). There is no `pkg/node`, `pkg/p2p`, `go.mod` or
  proto change, no libp2p import and no new parser of peer input. Single runs:
  `TestNeighborhoodDelay` 2.1 s, `TestNeighborhoodSteady` 3.1 s (skipped with `-short`), the others
  as in round 1.
- **(T4.6 part 1 fix round 3, Medium) The harness closes its streams before it stops the
  reconcilers.** This extends "(T4.6 part 1 fix round 2, Medium) The harness's `FullClose` waits for
  the remote's close". Since that wait, a run that ended while a session or a Status send was in its
  close round trip never tore down. `simNeighborhood.close` (the test's cleanup) called
  `Reconciler.Close` first, which waits for the reconciler's calls (`wg.Wait`). The call waited,
  parked in `FullClose`, for the remote's close or `simCloseDeadline`, both due on a clock that no
  longer moved; the streams were closed only after that. The test died at the go test timeout. The
  review hit it with `Delay` 100 ms at N = 5, seed 5: with a `Limit` that ends the run unconverged
  (5.6–5.95 s, 8 of 200 runs) and with `Tail` 64.0–64.15 s (4 of 21). The default `Tail` (60 s)
  escaped only because it equals `StatusMaxInterval` (the review).
  - Now `close` first closes every stream (both pipes: reads get `io.EOF`, writes
    `ErrStreamClosed`) and refuses new ones. `NewStream` returns `errSimClosed`, checked under the
    lock that adds a stream to the list, so no stream opens unclosed. Then it stops the reconcilers
    and the services. A `FullClose` returns as on the remote's close (`awaitEOF` already took a
    stream the harness closed as done), and a read returns `io.EOF` after the bytes that arrived.
  - Why not keep moving the clock until the reconcilers closed (the review's other option): the run
    is over and measured. Moving the clock after it runs more protocol work (calls in flight finish,
    30 s deadlines fire) and needs every wait to be on the fake clock, so a wait on the real clock
    would spin it. Closing the streams ends every wait at once without the clock; the old `close`
    did the same, only last.
  - `TestNeighborhoodTeardown` (new): a first run (N = 5, seed 5, 100 ms) finds the earliest Status
    sent after the convergence (3.8 s after it). A second run ends 3 legs into that send (`Tail`
    4.1 s), in its `FullClose`, and tears down under a 30 s watchdog. When the watchdog fires, the
    test fails and moves the clock past the close deadlines, so that the cleanup does not hang. The
    test also checks that a Status send ended in the teardown within its `FullClose` (2 to 4 legs
    in), so it fails, instead of passing vacuously, if the protocol's timing moves that send.
  - Revert check: with the old order the test fails at the watchdog (34 s). A `Tail` scan from 3.9 s
    to 4.2 s hung for 4.0–4.15 s only. The 15 `Limit` values 5.6–5.95 s (25 ms apart) all fail
    unconverged, as intended, and tear down (7 s); with the old order, three of them were still in
    teardown at a 90 s go test timeout.
- **(T4.6 part 1 fix round 3, Lows 2 and 3) Test fixes.** `TestSimStream`'s `parked` and `fullClose`
  helpers take the subtest's `t`: a `t.Fatalf` on the parent `t` from a parallel subtest panics the
  test binary instead of failing the subtest. `TestNeighborhoodConverges` runs with hint(R) and
  without it (`simOptions.NoHints`; subtests "hints" and "no hints") under the same assertions:
  convergence within the bound, no duplicate delivery, the sample checks. Both converge each batch
  in 5 s with no duplicate.
- **(T4.6 part 1 fix round 3, Low 4) The G6 table's small differences are within noise.** A record
  of the review's figures for "(T4.6 part 1 fix round 2 → part 3, G6) The symbol overshoot: the
  growth step, with and without the hint". At N = 9 steady, as a share of the modeled pullsync:
  - seed 9 run three times: 76.9, 77.7, 79.7%;
  - seeds 1–4: 74.2–78.5%;
  - `Delay` 0/20/100/200 ms: 80.9/82.6/72.9/70.5%.

  The hint's advantage over no hint ranges from −1 to +6 points across `Delay`. `GrowthFactor` 2's
  drop of about 18 points holds. The hint and `BatchFactor` rows (differences of 5 points or less)
  need a seed spread and a `Delay` sweep before G6 relies on them (Follow-ups). The baseline is
  §7.8's pullsync column (N − 1 offers × 100 B per node; the harness counts 102 B per offer).
  AC-B1's measured pullsync can only exceed it, so the percentages are conservative: they bound the
  measured share from above.
- **(T4.6 part 1 fix round 3) Scope, tests and wall times.** Test-only: `neighborhood_sim_test.go`
  (the teardown order, `errSimClosed`) and `neighborhood_test.go`. There is no production,
  `pkg/node`, `pkg/p2p`, `go.mod` or proto change. Single runs: `TestNeighborhoodTeardown` 4.1 s
  (two runs), `TestNeighborhoodConverges` 0.75 s (two parallel subtests), the others as in round 2.
- **(T4.6 part 2) The analytical model: `srep_model_test.go` (package `rangesync_test`, test-only).**
  An SREPSim-style model of a neighborhood: it follows the protocol's decisions and message sizes,
  not bytes on a stream, so a run of the steady mode takes 10–100 ms and G6 can sweep it. What is
  Monte Carlo and what is closed form:
  - *The decoder, Monte Carlo over the real code* (`srepDecodeTable`). For a difference of d IDs,
    random IDs are encoded with the FloatMapper and fed to `fingerprint.Decoder` one symbol at a
    time; the threshold K is the symbol count at `Done`. A batch of n symbols decodes if and only if
    n ≥ K (peeling more symbols never recovers fewer IDs), so the K samples are the decode-success
    distribution of every batch size. 128 draws per d in the default run, 1000 with `-tags=sim`.
    T2.6's benchmark reports only quantiles at six sizes, so the table is drawn here; it matches
    T2.6 (p50 16 at d = 10, 68 at d = 45) and §13's band (`TestSREPDecodeTable`).
  - *A session, closed form given K* (`srepParams.stream`, `sessionCost`, `srepWire`). The first
    batch max(`MinBatch`, ⌈`BatchFactor` · d̂⌉ + 2) with d̂ = max(|cA − cB|, hint(R), 1), then `More`s
    up to `GrowthFactor` × the symbols received, within cap(R) and `SessionSymbolBudget`. Sizes are
    the delimited encodings of the wire encoders, which `bytes_metadata` counts
    (`TestSREPWireSizes`), with each symbol's count varint at its expected length from the binomial
    count (`TestSREPSymbolBytes`, against `AppendSymbolData`: 81 B per symbol at the harness's
    counts, 82 B for a large reserve). Averaging over K gives a stream's expected cost and its `More`
    distribution.
  - *The neighborhood, discrete events* (`srepSim`, `srepParams.run`). Per node: the Index's
    debounce, Status gossip on the per-peer schedule, rounds (triggers deferred to
    `MinRoundInterval`, the adaptive timer, the round's Statuses sent before its sessions), the
    initiator rule and the takeover, `MaxInitiatedSessions` slots with the recheck (stale), one
    session at a time per pair, the session's legs as the harness has them (the open, the
    Fingerprint, the Symbols, a round trip per `More`, the Items, the empty answer, the close),
    hint(R) recorded by both sides, the claims and the fetch's legs. Each stream's threshold is drawn
    from the table for the session's true n_Δ.
  - `srepParams.ideal` is §7.8's closed form with the real decoder: synchronous rounds of
    `MinRoundInterval`, each pair once on the round's snapshot, d binomial with q = 2p(N − p) /
    (N(N − 1)), the count estimate |2X − d|, and hint(R) the previous round's d. Its oracle (a stream
    that stops at K, §4.3.7's cost) is a floor that no batch constant can beat. The pullsync baselines
    are `srepPullsyncACB1`, `srepPullsync78` and `srepPullsyncWire` (below).
  - Parameters (`srepParams`): N, p, the schedule (the harness's batches, or Poisson at `Rate`),
    `Delay`, `BatchFactorTenths`, `GrowthFactor`, `MinBatch`, `NoHints`, `NoClaims`, the gossip and
    round intervals, the slots, and `Base` and `Root` (counts and sizes). `srepParamsOf(simOptions)`
    takes the harness's options.
  - Not modeled: the node session cap and the peer session limit (Busy and `session_limit`, a
    handful per run at N = 17), a takeover colliding with the initiator's own session, the
    served-wants fetcher's queue, splits and item exchanges (counted as `Outside`: 0 in every run,
    production sizes included), Items pages past the first, the rate limits and the Status sender's
    GCRA, and, as in the harness, the fetch limiters and bandwidth.
- **(T4.6 part 2) At Delay 0 a message leg takes a microsecond.** The harness runs the calls of one
  instant concurrently in real time, one goroutine hand-off per leg, so they progress in step: every
  session of a round reads the indexes before any of them ends and fetches. The model's first
  version ran the legs of one instant in a random order, which let a fetch land before a concurrent
  session's read. n_Δ per upload then fell below p(N − p) (14.8 against the harness's 18.3 at N = 9,
  44.9 against 51.7 at N = 17), and the zero-difference share rose to 20–24% (harness 5–7%). With
  `srepInstant` = 1 µs per leg the step order holds (18.0 against 18.3 at N = 9), and the times move
  by microseconds. Events of one instant still run in a random order.
- **(T4.6 part 2) Validation: the model against the harness, 32 rows, all within tolerance.**
  `TestSREPModelVsHarness` (`-tags=sim`) runs the harness's steady mode (`steadyOptions`: 480
  uploads, ~15 per second) for N = 9 and 17 at Delay 0, 50 and 200 ms with hint(R) and at 50 ms
  without, 3 seeds each and one run after another, and the model with the same options, 10 seeds.
  The model's mean must lie within 10% of the harness's for the share of the modeled pullsync, the
  symbols per difference and n_Δ per upload, within 15% for the median n_Δ per session, and within
  1 s for the mean and the longest batch convergence. It ran with the current constants and, in
  scratch worktrees with the constant changed (and `FirstBatch` for `BatchFactor`), with each G6
  candidate. All 4 × 8 rows are within tolerance. Share of the modeled pullsync (%), hint(R) on,
  Delay 0 / 50 / 200 ms:

  | constants | N = 9 model | N = 9 harness | N = 17 model | N = 17 harness |
  |---|---|---|---|---|
  | now | 78 / 78 / 74 | 79 / 76 / 75 | 61 / 63 / 65 | 64 / 61 / 65 |
  | `GrowthFactor` 2 | 58 / 59 / 59 | 60 / 59 / 60 | 48 / 50 / 50 | 48 / 49 / 52 |
  | `BatchFactor` 2.0 | 67 / 69 / 72 | 67 / 72 / 73 | 62 / 63 / 64 | 63 / 62 / 63 |
  | both | 59 / 61 / 62 | 62 / 63 / 65 | 52 / 54 / 54 | 52 / 52 / 56 |

  Without hint(R) at 50 ms (model / harness): now 81 / 82 and 59 / 60, `GrowthFactor` 2 60 / 60 and
  45 / 45, `BatchFactor` 2.0 81 / 83 and 59 / 60, both 60 / 60 and 45 / 45. The other figures agree
  as closely, for example now at 50 ms (model / harness, N = 9 and N = 17): symbols per difference
  3.10 / 3.01 and 3.56 / 3.38; `More`s per upload 0.25 / 0.24 and 0.90 / 0.88 (without the hint
  0.82 / 0.83 and 2.39 / 2.46); Status 46 and 175 B and Want 421 and 985 B per upload (equal);
  sessions per upload 0.48 / 0.47 and 1.81 / 1.75; batch convergence 3.40 / 3.40 s and
  3.71 / 3.75 s, the longest 5.80 / 5.83 s and 6.22 / 6.35 s; no duplicate. Discrepancies, none
  past its tolerance:
  - N = 17 at Delay 0: n_Δ per upload 48.3–48.4 against the harness's 51.4–52.8 (−6 to −9%), its
    median 25 against 28. The harness's goroutines are not fully in step: some fetches land before a
    concurrent session's read and expose the chunk again. The model is a lower bound of n_Δ at
    Delay 0; at 50 ms it matches (49.5 / 49.7).
  - N = 17 symbols per difference: with hint(R) the model is 0.05–0.27 higher (2–10%; both at
    Delay 0 2.90 / 2.64), which partly offsets the n_Δ above in bytes. Without the hint the two agree
    within 0.03, so the gap is in the hints; those of the second wave of sessions (below) are the
    likely place. Not traced further.
  - The zero-difference share (not checked): the model's mean is 0–3 points above the harness's,
    with overlapping spreads (N = 9 at 50 ms 7.0% (1.4–12.2) against 3.7% (0–8.1)). An equal session
    costs ~45 B, under 0.3% of the bytes.
  - Harness repeatability: N = 17 at Delay 0 with seeds 1–3 measured 61.7%, 65.9% and 63.6% in
    three runs, as round 3 warned.
- **(T4.6 part 2 → G6) The comparison table: model, harness, §7.8 and pullsync.** At the steady
  load (median `pairwise_diff` 40 at N = 9 and 27 at N = 17, AC-B1's reference load), Delay 50 ms,
  hint(R) on. Metadata per upload as a share of AC-B1's pullsync baseline, N(N − 1) × 102 B; model
  10 seeds, harness 3 seeds; "ideal" is §7.8's closed form with the real decoder (`srepParams.ideal`,
  n_Δ = p(N − p)):

  | | N = 9 | N = 17 |
  |---|---|---|
  | pullsync, AC-B1 (102 B per offer; the base of every share) | 7.34 KB | 27.7 KB |
  | pullsync, §7.8 (100 B per offer) | 7.2 KB | 27.2 KB |
  | pullsync on the wire (104 B per Offer entry + pages, 15.4 uploads/s) | 8.0 KB (1.09×) | 30.2 KB (1.09×) |
  | §7.8's rangesync (symbols only) | 2.0–2.5 KB (27–34%) | 4.6–5.9 KB (17–21%) |
  | ideal with the oracle (1.52–1.60 symbols per difference) | 46% | 30% |
  | now: ideal / model / harness | 78% / 79% (5.9 KB) / 76% | 52% / 63% (17.6 KB) / 61% |
  | `GrowthFactor` 2 | 58% / 60% / 59% | 38% / 49% / 49% |
  | `BatchFactor` 2.0 | 63% / 69% / 72% | 45% / 63% / 62% |
  | both | 59% / 61% / 63% | 39% / 53% / 52% |

  The model across Delay 0–200 ms (means of 10 seeds), N = 9 and N = 17: now 74–79% and 62–65%,
  `GrowthFactor` 2 59–60% and 48–50%, `BatchFactor` 2.0 66–73% and 62–64%, both 59–62% and 52–54%;
  without the hint 81–82% and 58–61%, 60% and 43–45%, 80–81% and 57–63%, 60% and 43–45%. The ideal is
  within 0–10 points of the model at N = 9 and 10–19 points below it at N = 17, where n_Δ per upload
  exceeds p(N − p) (below). What else the model says, current constants at 50 ms, N = 9 / N = 17:
  - Bytes per upload: symbols 4.8 / 14.4 KB, the decoder's Items listing 644 / 1945 B, Want 421 /
    985 B, Status 46 / 175 B, opening Fingerprints 20 / 80 B, `More`s 2 / 8 B.
  - Symbols per difference 3.1–3.2 / 3.5–3.6 (§7.8: 1.35–1.72); per stream p10 1.6, p50 2.2 / 3.1,
    p90 6.0. The distribution is bimodal: a stream decodes in its first batch or takes a ×4 `More`.
  - Round trips per decoded session (open and close counted) 4.5–4.6: 57–59% of the streams need no
    `More` and 31% one; `More`s per upload 0.25 / 0.89 (0.82 / 2.4 without the hint).
  - n_Δ per upload 18.2 / 49.5 against p(N − p) = 18 / 42. The excess at N = 17 comes from the
    initiated-session slots: with a slot per peer (16) it falls to 44.0 and the share to 53% (now)
    and 40% (`GrowthFactor` 2) (Follow-ups).
  - Duplicates: none with claims, in every model and harness run. Without claims (`NoClaims`) the
    model counts 11.9 duplicate deliveries per upload at N = 9 and 20.3 at N = 17 (the synchronous
    bound (N − p)(p − 1) is 12 and 28).
  - Time to convergence of a batch (the harness's schedule): mean 3.4 / 3.7 s, longest 5.8 / 6.2 s
    at 50 ms; at Delay 0 a mean of 2.55 s and a longest of 5.0 s for both N; at 200 ms 5.1 and 8.2 s
    (N = 9), 6.1 and 9.9 s (N = 17). With Poisson uploads
    at 15.4/s an upload converges in 3.0–3.1 s on average. pullsync's fill latency is ~250 ms
    (`pageTimeout`) plus the Offer, Want and Delivery legs.
  - Upload rate (Poisson, 50 ms; 2, 5, 15.4 and 50 uploads/s): now 98 / 90 / 79 / 64% at N = 9
    and 76 / 69 / 62 / 55% at N = 17; `GrowthFactor` 2 78 / 69 / 60 / 53% and 65 / 56 / 49 / 44%.
    The median n_Δ per session is 5 / 12 / 38 / 125 and 3 / 8 / 25 / 82, so the reference load
    (≥ 20) starts near 8 uploads/s at N = 9 and 12–13 at N = 17.
  - p = 2 and 4 give 67% and 86% at N = 9, 56% and 67% at N = 17 with the current constants.
    `MinBatch` 2 or 8 moves the share by at most a point at the reference load. Production sizes (a
    2^20 reserve, so cap(R) = `RootSymbols` and 82 B symbols, and a root of radius 10) are within a
    point of the harness's sizes, and no stream reaches cap(R).
- **(T4.6 part 2) AC-B1 per configuration: no candidate passes at N = 9.** AC-B1
  (acceptance-criteria.md from T6.0, on master, not on this branch): rangesync's metadata per new
  chunk at most 50% of pullsync's (`bee_pullsync_chunks_offered` × 102 B) at the reference load.
  Model and harness agree:
  - now: 74–82% at N = 9 and 58–66% at N = 17; fails both.
  - `GrowthFactor` 2: 58–60% at N = 9, fails; 47.5–52% at N = 17, on the line (passes at Delay 0
    and 50 ms, fails at 200 ms).
  - `BatchFactor` 2.0: 66–73% and 62–64%; fails both, with no gain at N = 17.
  - both: 59–65% and 52–56%; fails both.
  - `GrowthFactor` 2 without the hint reaches 43–45% at N = 17, with 4 `More`s per upload (3.4×
    the round trips).
  - The floor, the oracle (1.5–1.6 symbols per difference in one batch), is 46% at N = 9 and 30%
    at N = 17. At N = 9 the Items listing, the Want IDs and the Status already cost 1.13 KB per
    upload, 15% of pullsync, so the symbols must stay below ~1.7 per difference, which only the
    oracle reaches (Follow-ups).
- **(T4.6 part 2) Correction: with an exact estimate a first batch decodes 73–91% of the time, not
  half.** This supersedes, in "(T4.6 part 1 fix round 1, High) The serving side of a decoded stream
  records hint(R)", "what remains is `FirstBatch` = 1.6 d + 2, which sits at the median of the
  symbols a difference d needs, so about half the streams miss" and its "about 0.5 × 1.6 + 0.5 × 6.4
  ≈ 4 symbols per difference". With d̂ = d the first batch lies above the median threshold (d = 45:
  74 symbols against T2.6's p50 68 and p90 79): it decodes in 73% (d = 20), 85% (45) and 91% (80) of
  the streams, at 3.09, 2.38 and 2.04 symbols per difference (`TestSREPSessionCost`). The steady state
  costs more because its estimates miss. The count estimate |cA − cB| is about √d (counts alone:
  3.3–3.6 symbols per difference), and hint(R) is the previous round's difference, a Poisson-noisy
  estimate of this one (ideal with the hint: 3.15 at N = 9, 3.45 at N = 17). A miss costs a
  ×`GrowthFactor` `More`: with 2, the counts alone cost 2.1–2.3 and an exact estimate 1.8–2.2.
  `BatchFactor` 2.0 decodes 95–100% of exact first batches for d ≥ 20, but against noisy hints it
  spends the extra symbols in every stream, which is why it gains little at N = 17.
- **(T4.6 part 2) The pullsync baseline is AC-B1's N(N − 1) × 102 B.** Every share above uses it, as
  the harness does, not §7.8's 100 B. On the wire an Offer entry is 104 B (the pb.Chunk and its
  framing), and each page adds its Get, Offer header and Want (~20 B). A live page holds the chunks
  of a bin that arrive less than `pageTimeout` (250 ms) apart: e^(−μ · 0.25 s) pages per chunk at a
  bin rate μ, and a node's bins get λ/2, λ/4, …. So `srepPullsyncWire` gives 1.04–1.19× the baseline
  from 50 down to 2 uploads/s, 1.09× at the steady load. AC-B1's measured baseline (T6.1) is
  `bee_pullsync_chunks_offered` × 102 B, and the offers can only exceed N(N − 1) per upload (re-offers
  after a reconnect), so the shares here bound the measured ones from above; against pullsync's
  bytes on the wire they would be 4–16% lower (8% at the steady load).
- **(T4.6 part 2) Scope, tests and wall times.** Test-only, in `pkg/rangesync`: `srep_model_test.go`
  (the model), `srep_model_check_test.go` (default build) and `srep_model_sim_test.go` (`-tags=sim`),
  and in `neighborhood_sim_test.go` the harness reports `more_requests_total` per node
  (`simNodeResult.MoreRequests`, from the existing `export_test.go` accessor). There is no
  production, `export_test.go`, `pkg/node`, `pkg/p2p`, `go.mod` or proto change, no new parser of
  peer input and no `*_fuzz_test.go`. Tests and single-run wall times:
  - default: `TestSREPModel` 2.0 s and `TestSREPIdeal` 2.0 s (in parallel), `TestSREPSessionCost`
    and `TestSREPDecodeTable` 0.2 s, `TestSREPWireSizes` and `TestSREPSymbolBytes` 0.04 s,
    `TestSREPFirstBatch`, `TestSREPStream` and `TestSREPPullsync` < 0.01 s; 2.1 s together;
  - `-tags=sim`: `TestSREPModelVsHarness` 100–145 s per constant set (24 harness runs, one after
    another; 105 s with the current constants), `TestSREPModelG6` 62 s.
  - `-count=5 -cpu=1,2,4`: the `TestSREP*` tests 35 s, the neighborhood tests (with the harness
    change) 67 s, all passing.
- **(T4.6 part 2 fix round 1, Medium 1) `FirstBatch` reads `BatchFactor` (production, additive).**
  This supersedes the Follow-up "(T4.6 part 2) `FirstBatch` does not read `BatchFactor`" and, in
  "(T4.6 part 2) Validation: the model against the harness, 32 rows, all within tolerance", "(and
  `FirstBatch` for `BatchFactor`)". `BatchFactor` is now `BatchFactorNum / BatchFactorDen`, two new
  constants (16 and 10); `BatchFactor` stays an untyped constant (1.6). `FirstBatch(d)` is
  `firstBatch(d, MinBatch, BatchFactorNum, BatchFactorDen)` = max(minBatch, ⌈num · d / den⌉ + 2),
  with d clamped at MaxUint64 / (2 num). At 16/10 these are the old expressions,
  (16d + 9) / 10 + 2 and MaxUint64 / 32. A fraction needs no tenths: 7/4 is 1.75, which tenths read
  as 1.8.
  - Tests: `TestFirstBatchPinned` pins the old formula for every d < 2^20 and around the clamp.
    It passed before and after the change. `TestFirstBatchFactor` checks the ceiling and other
    fractions through `FirstBatchOf` (export_test.go).
  - The model: `srepParams.firstBatch` calls `FirstBatchOf`. Its `BatchFactor` is a fraction
    (`srepFactor`, which replaces `BatchFactorTenths`) that defaults to the constants. "now" in
    `srepConfigs` and in `TestSREPSessionCost` is the compiled constants. `srepCompiled` reads the
    constants instead of inverting `FirstBatch(10)`. The harness runs `FirstBatch`.
  - Mutation check: with only `BatchFactorNum` changed to 20, `FirstBatch(3)` is 8 (was 7) and
    `FirstBatch(10)` is 22 (was 18). `TestFirstBatch`, `TestFirstBatchPinned` and
    `TestWorkedExamples` then fail, and `TestSREPFirstBatch` passes, so the model follows. With
    7/4 the model's factor prints 1.75, and both give 20 at d = 10.
  - `TestFirstBatch`, `TestCapacity`, `TestWorkedExamples` and `TestSessionSymbols*` pass
    unchanged.
- **(T4.6 part 2 fix round 1, Medium 2 and Low 2) The validation: a 3-point share bound, the
  components on their own, differences, 6 seeds and records.** This supersedes, in "(T4.6 part 2)
  Validation: the model against the harness, 32 rows, all within tolerance", the tolerances (10%
  for the share, the symbols and n_Δ), the 3 harness and 10 model seeds, its table, its
  discrepancy list and "all within tolerance". In "(T4.6 part 2 → G6) The comparison table", the
  harness column is now 6 seeds (below). `TestSREPModelVsHarness` now works as follows:
  - Seeds: the harness runs 6 seeds per row (`srepHarnessSeeds`), each run in its own subtest, and
    the model runs 20. `runNeighborhood` tears a neighborhood down with `tb.Cleanup`, so runs in one
    test function stayed alive until it ended. The old test kept a row's 3 runs alive together, and
    a 48-run collector reached 5 GB and was killed.
  - A difference of means passes within a tolerance plus twice its standard error, taken from the
    measured spreads of both sides. This covers the harness's nondeterminism: one seed's share
    moves by up to 3 points between runs. Bound and deviation are logged. The tolerances: the share
    3 points absolute; n_Δ per upload and the symbols per difference 6% each, so errors cannot
    cancel in the share beyond that; the median n_Δ 15%; convergence 1 s; and no duplicate delivery
    on either side (Low 2: both are 0 in every run). The largest component gaps at a Delay are 5.3%
    and 6.0% (below). At 5%, the live N = 17, 200 ms row would fail about one run in twenty.
  - Delay 0 is outside the model's domain for n_Δ, the symbols and the differences between
    configurations. At N = 17 the harness's n_Δ is 7–11% above the model's: its goroutines of one
    instant are not in step, so a session can read after a fetch of the same instant. The model's
    symbols per difference are 2–11% above the harness's, so the share's agreement there (within
    3.6 points) is partly cancellation. Delay 0 rows are checked for the share only. They do not
    back G6's calls.
  - Differences, model against harness, within 2 points (plus twice their standard error). Live,
    paired by seed: hint off − on at 50 ms, and Delay 200 − 50 ms. Against `srepHarnessRef`: each
    candidate's absolute figures, its difference from the compiled constants, and its hint
    difference. `srepHarnessRef` holds the harness's records of 4 configurations × 8 rows, measured
    with 6 seeds in scratch builds with the constants changed. The test logs the records of the
    compiled constants. A record check fails when the live harness drifts from its record. In the
    live run every row matched within 1.4 points.
  - The table: share of AC-B1's pullsync (%), model mean ± sd of 20 seeds / harness mean ± sd of 6
    seeds (range):

    | constants | N | Delay 0 | 50 ms | 200 ms | 50 ms, no hint |
    |---|---|---|---|---|---|
    | now | 9 | 78.0 ± 1.7 / 78.1 ± 2.6 (74.2–81.9) | 78.3 ± 2.1 / 76.9 ± 1.7 (74.1–78.3) | 73.8 ± 3.0 / 72.0 ± 2.4 (69.4–75.9) | 81.0 ± 2.4 / 80.4 ± 1.8 (77.8–82.0) |
    | now | 17 | 61.2 ± 1.0 / 64.8 ± 1.4 (62.7–66.8) | 62.9 ± 1.4 / 62.1 ± 2.0 (59.6–65.0) | 64.6 ± 1.8 / 66.2 ± 4.0 (60.7–72.6) | 59.1 ± 1.1 / 59.2 ± 1.2 (57.2–60.9) |
    | `GrowthFactor` 2 | 9 | 58.4 ± 0.8 / 60.7 ± 1.5 (59.1–63.2) | 59.7 ± 0.8 / 59.5 ± 0.5 (58.8–60.3) | 59.2 ± 0.9 / 59.7 ± 0.9 (58.6–61.2) | 60.1 ± 0.9 / 59.6 ± 1.0 (58.5–60.8) |
    | `GrowthFactor` 2 | 17 | 47.8 ± 0.7 / 50.5 ± 2.9 (47.1–53.1) | 49.4 ± 0.8 / 48.6 ± 1.1 (47.5–50.2) | 50.2 ± 0.9 / 52.1 ± 0.8 (50.8–53.1) | 44.3 ± 0.7 / 44.3 ± 0.9 (43.3–45.8) |
    | `BatchFactor` 2.0 | 9 | 66.4 ± 1.7 / 69.1 ± 2.0 (65.4–71.0) | 69.5 ± 1.9 / 71.1 ± 2.0 (69.2–74.6) | 70.3 ± 2.3 / 70.6 ± 1.8 (67.7–72.8) | 80.8 ± 1.4 / 80.8 ± 2.5 (77.2–84.7) |
    | `BatchFactor` 2.0 | 17 | 61.6 ± 1.3 / 62.1 ± 1.9 (58.7–64.7) | 63.2 ± 1.3 / 61.2 ± 1.8 (58.2–63.0) | 63.8 ± 1.5 / 63.8 ± 1.6 (62.0–66.5) | 58.9 ± 1.1 / 59.2 ± 1.2 (57.3–60.4) |
    | both | 9 | 58.6 ± 0.5 / 61.4 ± 1.3 (59.5–62.5) | 61.1 ± 1.0 / 62.2 ± 0.9 (61.4–63.4) | 62.2 ± 0.9 / 64.0 ± 0.8 (63.0–65.2) | 60.5 ± 0.6 / 59.9 ± 0.9 (59.0–60.9) |
    | both | 17 | 51.9 ± 0.8 / 51.2 ± 2.5 (47.5–54.4) | 53.5 ± 0.9 / 52.3 ± 0.8 (51.6–53.7) | 54.3 ± 0.9 / 55.7 ± 1.1 (53.7–57.0) | 44.3 ± 0.7 / 44.9 ± 0.9 (43.7–46.3) |

  - Component gaps, model − harness, n_Δ / symbols per difference (%):
    - at 50 ms: within ±1.2 / −3.1 to +4.1 in every configuration;
    - at 200 ms: N = 9 within ±0.5 / −3.6 to +3.5; N = 17 −4.1 to −5.3 / +1.2 to +6.0;
    - at Delay 0, N = 17: −7.4 to −10.7 / +2.2 to +10.7.

    So the model's n_Δ at N = 17 is a lower bound at 200 ms too. The harness refused no session in
    these runs (`sessions_total` has no `busy` or `session_limit`), so the session caps the model
    omits are not the cause. Not traced further. The gap shows in the Delay 200 − 50 ms
    difference at N = 17: the harness's share rises by 2.6–4.1 points, the model's by 0.7–1.8.
    With hint(R), the model's symbols at N = 17 sit 2–6% above the harness's (`BatchFactor` 2.0 the
    most), and without it within 2%. This is the second wave's hints, still untraced.
  - The differences G6's calls rest on (share points, model / harness, at 50 and 200 ms):
    - `GrowthFactor` 2 − now: −18.6 / −17.4 and −14.6 / −12.3 at N = 9; −13.5 / −13.5 and
      −14.4 / −14.1 at N = 17.
    - `BatchFactor` 2.0 − now: −8.8 / −5.8 and −3.4 / −1.4 at N = 9 (the model overstates the gain
      by 2–3 points); +0.3 / −0.9 and −0.8 / −2.5 at N = 17 (no gain beyond the noise: the
      harness's standard errors are 1.1 and 1.8).
    - both − now: −9.4 / −9.8 and −10.3 / −10.6 at N = 17.
    - hint off − on at 50 ms, N = 9 and N = 17: now +2.7 / +3.6 and −3.7 / −3.0; `GrowthFactor` 2
      +0.4 / +0.1 and −5.1 / −4.3; `BatchFactor` 2.0 +11.4 / +9.7 and −4.2 / −2.1; both
      −0.6 / −2.4 and −9.2 / −7.4.

    All are within their bounds.
  - What the model can decide: effects above about 3 points. These are `GrowthFactor` 2 everywhere,
    `BatchFactor` 2.0's gain at N = 9 and its absence at N = 17, and the hint at N = 17 (and with
    `BatchFactor` 2.0 at N = 9). It cannot decide whether `GrowthFactor` 2 with the hint passes
    AC-B1 at N = 17. At 50 ms the model gives 49.4% and the harness 48.6 ± 1.1% (47.5–50.2); at
    200 ms 50.2% and 52.1 ± 0.8%. Both are within the model's 3 points of 50%, so the call needs
    the harness or testnet (T6). The slot-per-peer figures in "(T4.6 part 2 → G6) The comparison
    table" (44.0 n_Δ per upload, 53% and 40%) are the model's alone and unvalidated (Follow-ups,
    fix round 1).
- **(T4.6 part 2 fix round 1, Medium 3) Correction: without the hint, `GrowthFactor` 2 costs 3.3×
  the `More`s and 1.35× the round trips, not "3.4× the round trips".** This supersedes, in "(T4.6
  part 2) AC-B1 per configuration: no candidate passes at N = 9", the bullet "`GrowthFactor` 2
  without the hint reaches 43–45% at N = 17, with 4 `More`s per upload (3.4× the round trips)".
  Figures at N = 17, 50 ms (harness 6 seeds, model 20):
  - `More`s per upload: 4.03 without the hint against 1.22 with it (3.3×).
  - Round trips per completed session (open and close counted): 6.33 against 4.68 (1.35×; model
    6.25 against 4.63). That is 1.65 round trips, 165 ms, per session at 50 ms.
  - Batch convergence: mean 3.99 s against 3.76 s; the longest is unchanged (6.49 s both).
  - Share: 44.3 ± 0.9% (43.3–45.8) in the harness and 44.3% in the model. Only the setups with
    `GrowthFactor` 2 and no hint clear AC-B1 at N = 17 with margin (both constants without the hint:
    44.9 ± 0.9%).

  So the hint costs bytes (4.3 points in the harness) and saves 1.65 of 6.33 round trips per
  session (26%) and 0.23 s of mean convergence. G6 should weigh those, not 3.4× the round trips.
- **(T4.6 part 2 fix round 1, Low 3) The n_Δ invariant of `TestSREPModel` is structural.** The old
  check n_Δ per upload ≥ p(N − p) held only for its seed: 18.2 against 18 at N = 9, while over 20
  seeds at 200 ms the model goes down to 17.7. Now:
  - `srepCheckDiffs`: the differences of all sessions are at least the chunks fetches delivered
    (stored or duplicate). Each delivered chunk was a difference of the session that wanted it,
    on one side only, so this is N − p per upload with claims. It holds for every seed, with and
    without claims. A mutation that records a quarter of each difference fails it.
  - n_Δ per upload ≥ 0.95 p(N − p) stays, documented as a regression check of the step order of
    one instant, not as a bound. The model gives 17.7–18.4 at N = 9 over 20 seeds at Delay 0–200
    ms (sd ≤ 0.1), and the random leg order it guards against gave 14.8.
- **(T4.6 part 2 fix round 1, Low 4) AC-B1 against pullsync's bytes on the wire.** This extends
  "(T4.6 part 2) AC-B1 per configuration: no candidate passes at N = 9" and closes the round-3
  review's Low 5 nit. AC-B1's baseline is 102 B per offer. On the wire an Offer entry is 104 B,
  plus each page's Get, Offer header and Want: 1.09× the baseline at the steady load. Against
  pullsync's wire bytes, every share above overstates rangesync's cost by about 8% (divide by 1.09).
  On that basis (harness, Delay 0–200 ms, hint on, N = 9 and N = 17):
  - now: 66–72% and 57–61%;
  - `GrowthFactor` 2: 55–56% and 45–48%, which passes at N = 17 on this basis;
  - `BatchFactor` 2.0: 63–65% and 56–59%;
  - both: 56–59% and 47–51%;
  - `GrowthFactor` 2 without the hint: 41% at N = 17.

  AC-B1 as written (T6.0) measures `bee_pullsync_chunks_offered` × 102 B, so the verdicts above
  stand. Re-basing it is G6's or T6.0's call (Follow-ups, "(T4.6 part 2 → G6) What can bring AC-B1
  within reach").
- **(T4.6 part 2 fix round 1) Scope, tests and wall times.**
  - Production (Medium 1 only, in `pkg/rangesync`): `constants.go` (`BatchFactorNum`,
    `BatchFactorDen`, `BatchFactor` derived from them) and `session_symbols.go` (`firstBatch`).
    Behavior is identical at the current constants.
  - Test-only: `export_test.go` (`FirstBatchOf`), `session_symbols_test.go`, and the three
    `srep_model*` files.
  - There is no `pkg/node`, `pkg/p2p`, `pkg/pullsync`, `go.mod` or proto change, no new parser of
    peer input and no `*_fuzz_test.go`.
  - Tests and wall times: single runs on this host.
    - default: `TestSREPModel` 1.7–1.9 s and `TestSREPIdeal` 1.8 s (in parallel),
      `TestFirstBatchPinned` 0.04 s, `TestFirstBatchFactor` < 0.01 s; `TestSREP*` and
      `TestFirstBatch*` at `-count=5`: 9 s.
    - `-tags=sim`: `TestSREPModelVsHarness` 184 s (48 harness runs, one after another, and the
      model's 640 runs), `TestSREPModelG6` 57 s. The records took about 3 minutes per scratch build.
    - `go test ./pkg/rangesync/...`, and `pkg/rangesync` at `-count=2 -cpu=1,2,4` (61 s): all pass.
      `TestCombinedSyncerBothDrivers` passes at `-count=3`.
    - `make build`, `make lint`, `golangci-lint --build-tags sim` and `go vet` (default and
      `-tags=sim`) are clean.
- **(T4.6 part 3) G6 summary: the measured inputs and a recommendation per question.** No production
  constant changed; the user decides at G6. How it was measured:
  - The harness (`runNeighborhood`) in its steady mode (`steadyOptions`: 24 batches of 20 uploads
    1.3 s apart, ~15.4 uploads per second, p = 3), N = 9 and 17, 6 seeds per row, Delay 0, 50, 100
    and 200 ms with hint(R) and 50 and 200 ms without (`TestG6Constants`). Every candidate ran in a
    scratch build with only its constant changed: `GrowthFactor` 2 and 3, `BatchFactorNum` 20 (2.0),
    both, and `reconciler.MaxInitiatedSessions` 16 alone and with `GrowthFactor` 2. Load, round
    interval, catch-up, bootstrap and fault runs are `TestG6Load`, `TestG6RoundInterval`,
    `TestG6Catchup`, `TestG6Bootstrap` and `TestG6FetchOpenFails` (all `-tags=sim`, next entries).
    The model (`TestSREPModelG6Part3`) covers the grids and loads past the harness's reach.
  - A share is metadata per upload over AC-B1's baseline, N(N − 1) × 102 B. Delay 0 rows are outside
    the validated domain (part 2 fix round 1: the goroutines of one instant are not in step) and
    back no verdict. One seed's share moves by up to 3 points between runs, so a difference counts
    only when the 6-seed spreads separate.

  **Q2, cache sizes (`RootSymbols` 4096, `Level1Symbols` 512, `BucketSymbols` 32).** Steady state
  (`TestG6Load`, Delay 50 ms, 4 seeds; a quantile is the mean over the seeds, a max the largest):

  | N | uploads/s | n_Δ per session p50 / p99 / max (exact side) | n_Δ p99, both sides | wants per side p50 / p99 / max | first batch of the largest n_Δ | streams exhausted |
  |---|---|---|---|---|---|---|
  | 9 | 15.4 | 40 / 54 / 58 | 61 | 20 / 32 / 35 | 95 | 0 |
  | 9 | 50 | 131 / 168 / 184 | 190 | 65 / 96 / 104 | 297 | 0 |
  | 9 | 100 | 260 / 327 / 343 | 368 | 129 / 184 / 203 | 551 | 0 |
  | 17 | 15.4 | 27 / 64 / 82 | 98 | 11 / 38 / 82 | 134 | 0 |
  | 17 | 50 | 86 / 190 / 259 | 306 | 37 / 84 / 169 | 417 | 0 |
  | 17 | 100 | 176 / 365 / 484 | 601 | 76 / 170 / 400 | 777 | 0 |

  With a reserve past `ScanLimit` (Base 5000, so cap(R) = `RootSymbols`) at 50 uploads per second
  the figures are the same (max n_Δ 174 and 214) and no stream exhausted. Catch-up (`TestG6Catchup`:
  N = 5, one node rejoins lacking Lag chunks of a 70000-chunk reserve, so the root and level-1
  streams end at their caches; Delay 50 ms, 2 seeds; every one of the 4 peers runs its own session
  over the same difference, and the want set de-duplicates only the fetches):

  | Lag | caught up | path | split children | exhausted | symbols per missing chunk, per session | symbols of the next sessions |
  |---|---|---|---|---|---|---|
  | 300 | 0.85 s | one root stream | 0 | 0 | 1.61 | 1988 (≈ the catch-up's 1928) |
  | 1000 | 1.08 s | one root stream | 0 | 0 | 1.60 | 6450 (6408) |
  | 2500 | 1.85 s | one root stream | 0 | 0 | 1.60 | 16052 (16008) |
  | 4000 | 2.62 s | root split, 16 level-1 streams | 64 | 0 | 1.63 | 186 |
  | 6000 | 3.97 s | level-1 ranges split again (to the harness's leaves, the scan path) | 1088 | 68 | 2.73 | 68 |
  | 12000 | 6.90 s | the same, 3 sessions per peer | 1280 | 350 | 1.93 | 116 |

  At Lag 6000 every session stops at `SessionSymbolBudget` (16384 symbols, exactly 4 × 16384 in all)
  and its remaining ranges answer exhausted and fall back to item lists: the session budget, not a
  cache, bounds a catch-up at ~6000 per session. Its longest session took 9 round trips (4 below Lag
  4000).

  Recommendation: **keep 4096 / 512 / 32**, and confirm T2.6's "keep `BucketSymbols` 32".
  - The steady state never leaves the root stream's cached prefix: at 100 uploads per second the
    largest difference needs a first batch of 777 symbols (19% of `RootSymbols`; the root splits
    only past d̂ = 2558), and no stream exhausted a cache in any of the ~460 steady-state runs that
    count it (`TestG6Constants` asserts it, `TestG6Load` reports it).
  - The caches matter for catch-ups only. One root stream resolves up to ~2500 missing chunks at
    1.60 symbols each per session; up to ~5000 the root splits once into level-1 streams (1.63);
    past that the level-1 ranges split again, onto bucket caches in production (32 symbols, ~18 by
    the estimate) or the scan path in the harness (8 leaf bits). A split costs a round trip and 16
    Fingerprints per split range, ~1% of the bytes. At 15 uploads per second an absent node misses
    ~15 chunks per second, so ~3 minutes away reach the first split and ~6 the second.
  - Memory (§5.3, of ~21 MB): root 0.36 MB, level-1 0.7 MB, bucket Fenwick 11.5 MB. Doubling
    `Level1Symbols` (+0.7 MB) would move the second split to ~10,000 missing; doubling `RootSymbols`
    (+0.36 MB) the first to ~5100. Neither shows in the steady state; T6.3 can show how often
    catch-ups of that size happen. `BucketSymbols` 16 saves 5.5 MB but descends at d ≈ 8–11 per
    bucket run (T2.6); bucket-cache streams appear only below a second split and for nested session
    roots (below).
  - Not measured (every harness node is at radius 0): nested session roots. A peer one bit deeper
    has a session root one bit below this node's root, which this node serves from the bucket cache
    (`cacheCapacity`: 32 symbols at a depth ≤ `LeafBits`) when it holds more than `ScanLimit` there,
    so such a session's stream exhausts past ~20 differences and splits (by inspection; Follow-ups).
  - Found: a catch-up's hint(R) sizes the next session's first batch. After a catch-up that decoded
    at the root (Lag ≤ 2500), each pair's next first batch is `FirstBatch(Lag)` for a difference of
    1–4 (the table's last column): the catch-up's symbol bytes once more (Follow-ups).

  **Q3, request-driven vs streamed batches, and `GrowthFactor`.** Harness, hint(R) on, 6 seeds;
  share at Delay 50 / 100 / 200 ms (sd ≤ 2.7 points, ≤ 1.4 for `GrowthFactor` 2), the rest at 50 ms:

  | constants | N | share % | Mores per upload | round trips per session | convergence mean, longest |
  |---|---|---|---|---|---|
  | `GrowthFactor` 4 (now) | 9 | 77.5 / 76.4 / 72.3 | 0.25 | 4.51 | 3.39 s, 5.90 s |
  | `GrowthFactor` 3 | 9 | 68.0 / 69.9 / 67.1 | 0.28 | 4.57 | 3.41 s, 5.90 s |
  | `GrowthFactor` 2 | 9 | 59.8 / 60.6 / 59.9 | 0.36 | 4.79 | 3.43 s, 6.10 s |
  | `GrowthFactor` 2, no hint | 9 | 59.7 / – / 59.2 | 1.40 | 7.02 | 3.68 s, 6.10 s |
  | `GrowthFactor` 4 (now) | 17 | 61.7 / 61.6 / 65.6 | 0.90 | 4.48 | 3.73 s, 6.45 s |
  | `GrowthFactor` 3 | 17 | 55.1 / 55.0 / 59.9 | 0.98 | 4.55 | 3.74 s, 6.55 s |
  | `GrowthFactor` 2 | 17 | 48.3 / 49.1 / 52.1 | 1.22 | 4.68 | 3.76 s, 6.55 s |
  | `GrowthFactor` 2, no hint | 17 | 44.7 / – / 46.2 | 4.10 | 6.35 | 3.98 s, 6.55 s |

  Streamed batches (the model from the same sessions' decoder thresholds K: a responder that streams
  on until a stop arrives overshoots K by what the line carries in a round trip, and saves the
  `More`s; 10 seeds, Delay 50 ms, hint on):

  | N | constants | `More`s per decoded stream | symbols per stream, request-driven / oracle K | streamed, 100 KB/s × 100 ms | 1 MB/s × 100 ms | 1 MB/s × 400 ms |
  |---|---|---|---|---|---|---|
  | 9 | `GrowthFactor` 4 | 0.57 | 127 / 62 | 184, +2.1 KB per upload | 1282, +43 KB | 4940, +178 KB |
  | 9 | `GrowthFactor` 2 | 0.80 | 88 / 62 | 184, +3.5 KB | 1282, +44 KB | 4940, +179 KB |
  | 17 | `GrowthFactor` 4 | 0.53 | 104 / 46 | 168, +8.9 KB | 1266, +162 KB | 4924, +672 KB |
  | 17 | `GrowthFactor` 2 | 0.72 | 76 / 46 | 168, +12.8 KB | 1265, +166 KB | 4924, +676 KB |

  Recommendation: **`GrowthFactor` 2** (this revises T2.6's "keep 4"), **keep the request-driven
  `More`** (T2.6 confirmed) and **keep hint(R)**.
  - `GrowthFactor` 2 is 12–18 points below now at N = 9 and 12.5–13.5 at N = 17, at every Delay; the
    spreads separate. Its cost: +0.28 and +0.20 round trips per session (28 and 20 ms at 50 ms one
    way), +0.03–0.04 s of mean convergence and +0.1–0.2 s of the longest. `GrowthFactor` 3 takes the
    same round trips for half the gain, so there is no reason to stop there.
  - Streamed batches lose on any working link: request-driven streams with `GrowthFactor` 2
    overshoot the oracle by 26–30 symbols, a streamed one by rate × RTT; they break even near 2 KB
    per round trip (about 20 KB/s at 100 ms). At 100 KB/s streaming adds 3.5 KB (N = 9) and 12.8 KB
    (N = 17) per upload, 80–95% on top of all the metadata `GrowthFactor` 2 sends (4.4 and 13.4 KB),
    to save 0.7–0.8 round trips per decoded stream. The wire (G2) has no stop message either.
  - hint(R) with `GrowthFactor` 2 costs nothing at N = 9 (59.8 against 59.7%) and 3.6–5.9 points at
    N = 17 (0.1 with 16 initiated slots, below), and saves 70% of the `More`s and 1.7 of 6.35 round
    trips per session (0.22 s of mean convergence at N = 17).

  **`BatchFactor` 1.6 → 2.0 (T2.6).** Harness, hint on, 6 seeds, share at Delay 50 / 100 / 200 ms:

  | constants | N = 9 | N = 17 |
  |---|---|---|
  | `BatchFactor` 1.6, `GrowthFactor` 4 (now) | 77.5 / 76.4 / 72.3 | 61.7 / 61.6 / 65.6 |
  | `BatchFactor` 2.0, `GrowthFactor` 4 | 70.9 / 72.2 / 70.5 | 62.0 / 62.2 / 64.5 |
  | `BatchFactor` 1.6, `GrowthFactor` 2 | 59.8 / 60.6 / 59.9 | 48.3 / 49.1 / 52.1 |
  | `BatchFactor` 2.0, `GrowthFactor` 2 | 62.2 / 64.7 / 64.4 | 52.3 / 53.1 / 56.4 |

  Without the hint `BatchFactor` 2.0 equals 1.6 within the noise (81.0 against 80.5% at N = 9, 59.2
  against 58.9% at N = 17). The model's grid with `GrowthFactor` 2 (10 seeds, 50 ms; `BatchFactor`
  1.2 / 1.4 / 1.6 / 2.0): N = 9 61.2 / 60.6 / 59.4 / 60.8%, N = 17 46.0 / 47.7 / 49.1 / 53.5%, at
  1.72 / 1.45 / 1.23 / 0.95 `More`s per upload. Recommendation: **keep `BatchFactor` 1.6** (this
  revises T2.6's 2.0, now measured with `FirstBatch` reading the constant). 2.0 helps only with
  `GrowthFactor` 4 at N = 9 (1.8–6.6 points) and not at N = 17; on top of `GrowthFactor` 2 it costs
  2.4–4.5 points at N = 9 and 4.0–4.3 at N = 17: with a cheap miss, the larger first batch it spends
  on every stream costs more than the misses it saves. The model's 1–3-point gain of 1.2–1.4 at N =
  17 is within its 3-point resolution and costs 0.2–0.5 `More`s per upload; not recommended without
  a harness build.

  **Q9, `CatchUpMinDiff` = 256.** The reconciler compares it with one side's wants
  (pairwise_diff_half: `session` for an initiator, `Wants` for a responder), not with n_Δ. Harness
  (`TestG6Load`, Delay 50 ms, 4 seeds) and model (Poisson uploads, 4 seeds pooled):

  | N | uploads/s | wants per side p50 / p99 / max, harness | sides ≥ 256, harness | wants max, model (share of sides ≥ 256) | pairwise_diff p99, both sides |
  |---|---|---|---|---|---|
  | 9 | 15.4 | 20 / 32 / 35 | 0 | 36 | 61 |
  | 9 | 50 | 65 / 96 / 104 | 0 | 90 | 190 |
  | 9 | 100 | 129 / 184 / 203 | 0 | 174 | 368 |
  | 9 | 150 | – | – | 241 (0) | – |
  | 9 | 200 | – | – | 328 (38%) | – |
  | 17 | 15.4 | 11 / 38 / 82 | 0 | 64 | 98 |
  | 17 | 50 | 37 / 84 / 169 | 0 | 221 | 306 |
  | 17 | 100 | 76 / 170 / 400 | 7 in one run | 411 (0.56%) | 601 |
  | 17 | 150 | – | – | 574 (2.1%) | – |
  | 17 | 200 | – | – | 805 (3.5%) | – |

  - What a steady-state side past 256 does: its fetches count towards `SyncRate`, and the chunks
    they store keep the 10-minute window above 0 for 10–20 minutes, so the reserve worker cannot
    lower the radius and `isFullySynced` is false (the node skips redistribution). T4.5 part 2c's
    grace does not shield this: the grace bounds catch-up fetches that store nothing, and these
    store chunks. A node runs ~12 × (N − 1) session sides a minute (~2000 per window at N = 17), so
    the bound is the max, not the p99: the first sides cross 256 between 50 and 100 uploads per
    second at N = 17 (max 169 at 50, 400 at 100) and near 180 per second at N = 9.
  - At the reference load the p99 is 32–38 and the max 35–82: 256 holds with a 3–7× margin.
  - The neighborhood's `pairwise_diff` aggregate (both sides, n_Δ) passes 256 at its p99 already at
    ~40 uploads per second at N = 17 (306 at 50), while no side's wants do: Q9's check "the share in
    `le="255"`" must read `pairwise_diff_half` (Follow-ups).
  - Recommendation: **raise `CatchUpMinDiff` to 1024**, unless the expected neighborhood upload rate
    stays well below 50 per second, where 256 holds. 1024 keeps the max below it up to ~200 uploads
    per second at N = 17 (model max 805). Its cost: a catch-up of fewer than 1024 missing chunks no
    longer holds `SyncRate`; measured, such a catch-up ends within ~1.1 s (Lag 1000: 1.08 s), long
    before the reserve worker's 15-minute tick or the agent's once-per-phase sample, and bootstraps
    and radius halves count through their stripes anyway. T6.3 confirms against testnet's real rate
    (README: Q9 "confirmed in T6.3").

  **`RoundInterval` bounds (adaptive 5–30 s).** Harness, Delay 50 ms, 3 seeds; steady: ~15 uploads
  per second; sparse: 6 batches of 8 uploads 47 s apart, so that the interval grows back between
  them, and a 10-minute quiet tail:

  | N | load | min, max | convergence mean, longest | share % | symbols per difference | rounds per node-minute | idle B per node-minute |
  |---|---|---|---|---|---|---|---|
  | 9 | steady | 5 s, 30 s | 3.39 s, 5.90 s | 76.0 | 3.00 | 5.5 | – |
  | 9 | steady | 7.5 s, 30 s | 4.54 s, 8.00 s | 82.8 | 3.37 | 4.6 | – |
  | 9 | steady | 10 s, 30 s | 5.81 s, 10.70 s | 88.2 | 3.64 | 3.8 | – |
  | 9 | steady | 5 s, 15 s / 60 s | 3.38–3.39 s, 5.90 s | 76.8 / 76.8 | – | 6.3 / 5.1 | – |
  | 9 | steady | 2.5 s, 30 s | 1.96 s, 5.20 s | 85.6 | – | 8.2 | – |
  | 9 | sparse | 5 s, 30 s | 6.14 s, 10.75 s | 112.5 | – | 3.2 | 355 |
  | 9 | sparse | 5 s, 15 s / 60 s | 6.52 / 6.33 s, 10.85 / 10.75 s | 113.5 / 112.9 | – | 4.8 / 2.5 | 353 / 353 |
  | 9 | sparse | 7.5 s / 10 s, 30 s | 7.31 / 5.95 s, 13.15 / 11.55 s | 113.3 / 112.3 | – | 3.1 / 2.9 | 352 |
  | 17 | steady | 5 s, 30 s | 3.76 s, 6.55 s | 62.0 | 3.40 | 5.5 | – |
  | 17 | steady | 7.5 s, 30 s | 4.91 s, 7.90 s | 65.0 | 3.69 | 4.6 | – |
  | 17 | steady | 10 s, 30 s | 6.22 s, 10.70 s | 64.7 | 3.67 | 3.8 | – |
  | 17 | steady | 5 s, 15 s / 60 s | 3.75 s, 6.55 / 6.30 s | 60.9 / 61.6 | – | 6.3 / 5.1 | – |
  | 17 | steady | 2.5 s, 30 s | 2.27 s, 5.65 s | 70.4 | – | 8.2 | – |
  | 17 | sparse | 5 s, 30 s | 7.24 s, 10.95 s | 85.4 | – | 3.4 | 712 |
  | 17 | sparse | 5 s, 15 s / 60 s | 8.94 / 7.44 s, 10.95 / 10.75 s | 87.0 / 89.5 | – | 5.0 / 2.7 | 709 / 712 |
  | 17 | sparse | 7.5 s / 10 s, 30 s | 10.76 / 8.82 s, 13.35 / 15.65 s | 88.4 / 87.9 | – | 3.1 / 2.9 | 704 / 705 |

  With `GrowthFactor` 2 (steady, min 5 / 7.5 / 10 s): N = 9 59.1 / 62.5 / 62.4%, N = 17 48.6 / 50.6
  / 47.9%, the same convergence. No Status or session went over a peer's rate in any run, 2.5 s
  included (one session per peer per 2.5 s is exactly `PeerSessionInterval`, with no margin).
  - Recommendation: **keep 5–30 s.** The 5 s minimum is the cheapest and the fastest the production
    limits allow. Longer minimums slow convergence by 1.2–2.5 s on average (AC-V1's 2 × min becomes
    15–20 s) and do not save bytes: the differences per session grow less than the sessions fall
    (n_Δ p50 41 → 57 at N = 9 for twice the interval), and the symbols per difference rise (3.00 →
    3.64). (The model, validated at 5 s only, predicts savings at 10–15 s; the harness does not show
    them.) 2.5 s cuts the mean convergence by 1.4–1.5 s for 8–10 more points and runs at
    `PeerSessionInterval` with no margin, while `sessionBackoffBase` stays 5 s.
  - The maximum costs no bytes either way: rounds that find nothing send nothing, and the idle cost
    is the Status heartbeat (352–355 B per node-minute at N = 9, 704–717 at N = 17, whatever the
    maximum). Nor does it move convergence, since the triggers do not wait for the round timer (the
    sparse longest 10.75–10.95 s at 15, 30 and 60 s). 60 s halves the idle rounds (one `LocalStatus`
    each) but doubles a takeover's wait for an unresponsive initiator (2 × interval + jitter, up to
    3 minutes); 15 s doubles the rounds for nothing. Keep 30 s.
  - AC-V1 (2 × `RoundInterval` = 10 s at the minimum): the steady longest is 5.9–6.6 s at 50 ms, but
    the sparse longest 10.75–10.95 s passes 10 s (part 1's 15 s derivation; T6.4 Follow-up), as does
    the steady longest at Delay 200 ms and N = 17 (10.4 s now, 8.8 s with 16 initiated slots).

  **AC-B1: share of pullsync per configuration.** Harness, 6 seeds; the range is over Delay 50, 100
  and 200 ms (validated), with Delay 0 in brackets; pass means ≤ 50% at every validated Delay:

  | constants | hint | N = 9 | N = 17 | verdict N = 9 | verdict N = 17 |
  |---|---|---|---|---|---|
  | now (`GrowthFactor` 4, `BatchFactor` 1.6, 8 slots) | on | 72.3–77.5 [77.8] | 61.6–65.6 [65.0] | fail | fail |
  | now | off | 79.8–80.5 | 58.9–66.3 | fail | fail |
  | `GrowthFactor` 3 | on | 67.1–69.9 [67.4] | 55.0–59.9 [57.9] | fail | fail |
  | `GrowthFactor` 3 | off | 71.1–72.0 | 52.9–56.7 | fail | fail |
  | `GrowthFactor` 2 | on | 59.8–60.6 [59.8] | 48.3–52.1 [49.0] | fail (+10) | fail at 200 ms (+2.1); passes at 50 and 100 ms (−1.7, −0.9; sd 0.7) |
  | `GrowthFactor` 2 | off | 59.2–59.7 | 44.7–46.2 | fail | pass (−3.8 to −5.3) |
  | `BatchFactor` 2.0 | on | 70.5–72.2 [68.0] | 62.0–64.5 [63.1] | fail | fail |
  | `BatchFactor` 2.0 | off | 77.6–81.0 | 59.2–67.0 | fail | fail |
  | both | on | 62.2–64.7 [62.1] | 52.3–56.4 [53.5] | fail | fail |
  | both | off | 59.2–59.5 | 44.7–47.0 | fail | pass (−3.0 to −5.3) |
  | 16 initiated slots | on | = now | 50.9–51.6 [62.3] | – | fail (+0.9 to +1.6) |
  | 16 initiated slots | off | = now | 52.5–53.1 | – | fail |
  | 16 initiated slots, `GrowthFactor` 2 | on | = `GrowthFactor` 2 | 39.5–40.3 [48.8] | fail | pass (−9.7 to −10.5) |
  | 16 initiated slots, `GrowthFactor` 2 | off | = `GrowthFactor` 2 | 39.4 | fail | pass |

  At N = 9 no node has more than 8 higher-overlay peers, so 16 slots change nothing there. Caveats:
  - The base is AC-B1's modeled N(N − 1) × 102 B. Pullsync on the wire is 1.09× that at this load
    (part 2), so against wire bytes every share is ~8% lower: `GrowthFactor` 2 then 54.9–55.6% at N
    = 9 (still fails) and 44.3–47.9% at N = 17 (passes at every Delay). A measured
    `bee_pullsync_chunks_offered` can only exceed N(N − 1) per upload, so these shares bound T6.1's
    from above.
  - 16 initiated slots are now validated in the harness, not only the model: at N = 17, 10–15 points
    less at every validated Delay, no Busy or session-limit answer, mean convergence 0.3 s faster
    (3.45 against 3.73 s at 50 ms), longest at 200 ms 8.8 against 10.4 s. But a constant 16 =
    `MaxConcurrentSessions` fits only N ≤ 17 with no stripe or takeover holding slots (part 2 fix
    round 1, Low 1): the production form is a per-node cap (the node's eligible higher-overlay
    peers, bounded by the tokens its served sessions leave), a code change after G6 (Follow-ups).
  - What would bring N = 9 to 50%: no constant. The best, `GrowthFactor` 2, is at 59–61% (55% on the
    wire basis), and the floor with an oracle stream is 46% (part 2), so 50% leaves ~4 points for
    the symbols' overshoot: ≤ ~1.7 symbols per difference, against 2.2 with `GrowthFactor` 2. A
    protocol change would have to cut a fixed cost: the decoder's Items listing (~9 points at N = 9;
    truncated IDs, which the frozen wire (G2) does not carry) or a closer first-batch estimate (none
    tried here). Otherwise AC-B1 needs re-basing at N = 9 (T6.0's call; acceptance-criteria.md is
    not edited here): a per-N threshold, for example ≤ 50% at N ≥ 17 and ≤ 65% at N = 9, which
    `GrowthFactor` 2 meets with ~5 points; the wire basis alone is not enough (55%). Its prediction
    ("28–35% at N = 9, 17–22% at N = 17") should be replaced by these measurements (part 2
    Follow-up).
  - Recommendation for AC-B1: `GrowthFactor` 2 with hint(R) and `BatchFactor` 1.6. N = 17 then sits
    on the line at the modeled base and passes on the wire basis; with a per-node initiated-session
    cap it passes with ~10 points. N = 9 passes only after re-basing.
- **(T4.6 part 3) The follow-ups addressed to T4.6: measured, or moved with a reason.** Harness runs
  at the steady load and Delay 50 ms unless named; the scenarios are the next entry's.
  - Initiated-session share 8 of 16 (T4.3; part 2 fix round 1, Medium 2): measured with a scratch
    build of `MaxInitiatedSessions` 16 (G6 summary, AC-B1). With 8 the N = 17 batches still converge
    within the bound (mean 3.73 s, longest 6.45 s; 10.4 s at 200 ms), but the second wave exposes
    differences again: n_Δ p99 62 against 39 with 16, and 10–15 points of AC-B1. Neither setting
    drew a Busy or session-limit answer.
  - Stripe/round split (T4.3 fix): measured in an N = 17 bootstrap (`TestG6Bootstrap`, node 0 empty,
    20000 chunks, 2 seeds): all 16 stripes done in 2.55–3.15 s of message legs, none reassigned or
    timed out, so the waves of `MaxStripeSessions` = 4 do not dominate. The transfer time against
    the fetch limiter cannot be timed here (the limiter runs on the real clock and was lifted): at
    the default 1000 chunks per second the transfer alone takes 20 s for these 20000 chunks and 70
    minutes for 2^22, far past any wave effect. Moved to T6.1 (Bootstrap).
  - Busy-heavy stripes (T4.3 fix round 2, T4.5 part 2a fix round 1): not observed in the same
    bootstrap: 2 and 6 sessions answered Busy, 0 and 1 over the peer's session limit, no stripe
    reassigned. Found instead: every lower-overlay peer's round session with the empty node lists
    all its IDs to it. Node 0 received 4.2–5.7 MB (rank 4: 4 lower peers) and 12.5–14.0 MB (rank 14)
    of metadata in two runs, against AC-B2's 2.6 MB (1.15 × 20000 × 70 B + 1 MB), and denied 9,495
    and 51,255 claims in the first, the IDs its stripes already held (Follow-ups).
  - Deferred stripes under a busy want set (T4.4 fix round 1): not reached. The bootstrap's stripes
    (1250 IDs each) drew no `want_claims{peer_full}` or `{full}`: the harness's fetches end within
    one instant, so claims never pile up towards `MaxClaims` or `MaxClaimsPerPeer`. Moved to T6.1
    (Bootstrap, with the real fetch limiter).
  - A peer that fails every fetch open keeps its sessions (T4.5 part 2c): measured
    (`TestG6FetchOpenFails`, N = 9, node 0 refuses every fetch stream, 3 seeds): convergence 3.41 s
    against 3.39 s (longest 5.90 s both), share 75.6 against 76.4%, 0.10 failed fetch calls per
    upload, no duplicate. Its IDs come from the other holders at no visible cost, so no backoff of
    its sessions is needed.
  - Zero-difference sessions (part 1 → part 3): at the reference load 3.0–3.1% of the sessions at 50
    ms, 0–1% at 100–200 ms (4–5% at Delay 0), ~45 B each: under 0.3% of the bytes. No change to the
    root-change trigger.
  - Metadata per upload broken down (part 1 → part 3): part 2's records stand (model and harness
    equal at 50 ms): Status 46 and 175 B, Want 421 and 985 B per upload at N = 9 and 17; the rest is
    the reconcile streams, mostly symbols, which the G6 summary's constants move.
  - Busy, collisions and stale sessions at N = 17 (part 1 → part 3): no session refused (Busy or
    session limit) in any steady run of part 3; stale sessions 0–0.085 per upload, the most at 200
    ms without the hint.
  - First-chunk latency of catch-up fetches (T4.5 part 2c fix round 2 and its review Low 1): the
    harness gives 4 × Delay by construction (the open, the Want and one Delivery leg; no bandwidth,
    and the fetch and delivery limiters on the real clock): 200 ms at 50 ms one way, a tenth of
    `catchUpGrace`. The real latency, with a loaded limiter and bandwidth, needs testnet: moved to
    T6.3.
  - Q7 under expired-batch skew (T4.4) and unknown-batch junk (T4.5 part 2b and fix round 1): the
    harness has no batch store, its stamp check accepts everything and the mock reserve's
    `EvictBatch` is a no-op, so neither skewed expiry nor the rejections it causes exist in it.
    Moved to T6.3 (around real batch expiries), or to a harness extension with a per-node expiry in
    the stamp check and eviction in the mock reserve.
  - Retry granularity (T4.4): the retries that wait for a long fetch (up to ~65 s) wait on the fetch
    limiter and timeout, both on the real clock here. Moved to T6.
  - Re-ask waits and the stripe lock (T4.5 part 2a, narrowed to radius halves and sessions' large
    ranges) and trimmed stripe parts (T4.5 part 2c): both need radius changes and registered deep
    ranges; every harness node stays at radius 0. Moved to T6.1 (Radius decrease, Bootstrap).
  - A failed verification leaves the deliverer unjudged (T4.5 part 2c): an adversarial gap, not a
    measurement; it stays open for hardening.
  - The model's n_Δ gaps at N = 17 (part 2 fix round 1): not traced. Part 3's calls rest on the
    harness; the model only filled grids it was validated on (Delay ≥ 50 ms, `MinRoundInterval` 5
    s). Its round-interval trend at 10–15 s contradicts the harness (G6 summary) and is not used.
- **(T4.6 part 3) What the harness gained (test-only, `neighborhood_sim_test.go`).**
  - `simSample.Wants`: the wants each session side handed its reconciler (pairwise_diff_half's
    value, what `CatchUpMinDiff` is compared with): an initiator's from `ReconcileWith`'s return, a
    responder's from the `Options.Wants` it passed (0 when it passed none), matched to the side's
    sample by node and peer (one session per peer and side at a time).
  - `simOptions.Lag` and `LagNode`: the node lacks the last Lag base chunks, which are valid
    content-addressed chunks then (only with a lag, so the other runs keep their chunks and seeds).
    The counters are taken from the start, `simResult.CaughtUp` is when it held them all, and
    `CatchUpCounters` the counters before the first upload. `Lag = Base` is a bootstrap.
  - `simOptions.FetchOpenFails`: nodes to which every fetch stream open fails (`errSimOpenRefused`).
  - `simResult.Counters` (labeled counters: `streams_exhausted_total{level}`, `parts_sent{type}`,
    `fetch_skipped_total{reason}`, `session_busy_total`, `stripes_total{result}`) and `TailBytes`
    (the streams opened after the last convergence).
  - Fix: the `Limit` check fired after the convergence when the `Tail` reached past it (a sparse run
    with a 10-minute tail failed "not converged" with every upload held). It now checks only an
    unconverged run.
  - `neighborhood_g6_test.go` (default build): `g6FiguresOf`, what G6 reads off a run, and
    `TestNeighborhoodG6Figures` (a short steady run and a short catch-up, with the invariants: a
    side never wants more than its difference, the wants cover the chunks stored, the catch-up's
    counters lie within the run's). `neighborhood_g6_sim_test.go` and `srep_model_g6_sim_test.go`
    (`-tags=sim`): the sweeps. `srep_model_test.go`: the model records the wants of both sides and
    each stream's decoder threshold.
- **(T4.6 part 3) The part-1 round-3 review's Lows (items 1–3).** Item 1: `simNeighborhood.close`
  takes an atomic `closing` flag first, so a second call returns at once, also while the first still
  waits; the cleanup after `TestNeighborhoodTeardown`'s watchdog no longer hangs behind it. Item 2:
  `TestNeighborhoodTeardown` tries the earliest three Status sends after the convergence in turn
  and, if none lands in its `FullClose`, says that the harness is near-deterministic (each run's
  timeline may differ from the first's) or the protocol moved the sends. Item 3: `TestSimClose`
  covers the close on its own: a parked read returns `io.EOF`, a parked `FullClose` returns nil (as
  on the remote's close), a stream opened after it gets `errSimClosed`, and a second close returns
  while the first is blocked. Reverting the flag fails its "second close" check (it waits for the
  first). Items 4 (`-race` timing) and 5 (the 102 B nit, answered in part 2 fix round 1, Low 4)
  needed no change.
- **(T4.6 part 3) Scope, tests and wall times.**
  - Test-only, in `pkg/rangesync`: `neighborhood_sim_test.go` (the harness additions, the `Limit`
    fix, the close flag), `neighborhood_test.go` (`TestNeighborhoodTeardown`, `TestSimClose`),
    `neighborhood_g6_test.go` (new), `neighborhood_g6_sim_test.go` and `srep_model_g6_sim_test.go`
    (new, `-tags=sim`) and `srep_model_test.go` (wants and thresholds). No production change: no
    measurement needed a metric (the wants come from the harness's wrappers, the counters from the
    existing metrics), and the candidates ran in scratch copies of the worktree with one constant
    edited, never committed. There is no `pkg/node`, `pkg/p2p`, `pkg/pullsync`, `pkg/puller`,
    `pkg/pushsync`, `pkg/postage`, `go.mod` or proto change, no new parser of peer input and no
    `*_fuzz_test.go`.
  - Default build, single runs: `TestNeighborhoodG6Figures` 2.2 s (two parallel subtests),
    `TestSimClose` < 0.01 s, `TestNeighborhoodTeardown` 4.4 s. `go test ./pkg/rangesync/...` passes
    (`pkg/rangesync` 11 s); the neighborhood tests, `TestSim*`, `TestSREPModel` and `TestSREPIdeal`
    at `-count=3 -cpu=1,2,4` pass in 70 s; `TestCombinedSyncerBothDrivers` passes at `-count=3`.
    Reverting the close flag fails `TestSimClose` ("a second close waits for the first").
  - `-tags=sim`, one run each on the committed code, one after another (wall, 79 minutes in all):
    `TestG6Constants` 304 s (72 harness runs), `TestG6Load` 266 s, `TestG6RoundInterval` 504 s,
    `TestG6Catchup` 378 s, `TestG6Bootstrap` 45 s, `TestG6FetchOpenFails` 22 s,
    `TestSREPModelG6Part3` 992 s (its rate sweep 609 s and round-interval sweep 329 s: decoder
    tables for differences of several hundred), `TestSREPModelVsHarness` 250 s (its record check
    passes, so the harness changes leave the runs without a lag unchanged) and `TestSREPModelG6` 121
    s. Each candidate build ran `TestG6Constants` in 250–310 s (N = 17 rows only: 160–165 s). The
    tables above come from the first runs; the second matched them within the seed spreads, the
    largest moves being the wants max at 50 uploads per second at N = 17 (169, then 199) and the
    bootstrap's metadata (above).
  - `make build`, `GOTOOLCHAIN=go1.26.1 make lint`, `golangci-lint run --build-tags sim` and `go
    vet` (default and `-tags=sim`) are clean; gofumpt and gci ran on the changed files only.
- **(T4.6 part 3 fix round 1) G6 summary (revised after review).** This supersedes, in "(T4.6 part
  3) G6 summary: the measured inputs and a recommendation per question": the Q9 recommendation
  ("raise `CatchUpMinDiff` to 1024") with the model's 150 and 200 uploads per second rows as its
  evidence; the cost figures of `GrowthFactor` 2 ("+0.1–0.2 s of the longest") and the implication
  that it is a change of the constant alone; the Q2 evidence "no stream exhausted a cache in any of
  the ~460 steady-state runs" and the unqualified "keep"; and the `BatchFactor` sentence "not
  recommended without a harness build" (built now). It extends the summary's AC-B1 caveats. In
  "(T4.6 part 3) The follow-ups addressed to T4.6" it supersedes the bootstrap bullet as a measure
  of AC-B2: its Base 20000 is below `LargeDiff` (AC-B2 below). The rest of the summary stands. No
  production code or constant changed; the candidates ran in scratch builds with only the named
  constants changed, as before. The decision sheet at the end collects the recommendations.

  **Q9, `CatchUpMinDiff`: keep 256.**
  - The quantity stands: one side's wants (`session` in scheduler.go, `len(wants) >=
    CatchUpMinDiff`, and `Wants` in reconciler.go), `pairwise_diff_half`'s value.
  - The crossings at N = 17 come from the 8 initiated slots, not from the load alone. The review ran
    `TestG6Load` in the 16-slot build (N = 17, Delay 50 ms, 4 seeds); against the committed code:

    | N = 17 | uploads/s | wants p99, max | sides ≥ 256 | n_Δ max (exact side) | share % |
    |---|---|---|---|---|---|
    | 8 slots (now) | 50 | 84, 169 (199 in a second run) | 0 | 259 | 56.4 |
    | 8 slots (now) | 100 | 170, 400 | 7 in one run | 484 | 53.0 |
    | 16 slots | 50 | 79, 98 | 0 | 142 | 44.8 |
    | 16 slots | 100 | 155, 185 | 0 | 267 | 40.1 |

    With 8 slots a node's 16 initiated pairs run in two waves; the second wave's pairs wait for a
    slot and meet larger differences, so the max doubles. At N = 9 no node initiates more than 8
    pairs: the max is 104 at 50 uploads per second and 203 at 100 (the review's re-run: 103, 203).
  - The loads are not mainnet rates. 15.4 per second is AC-B1's reference regime (median n_Δ ≥ 20),
    not a measured rate. At a stable radius a neighborhood's reserve is full (2^22 chunks) and
    admits as many chunks as leave it, so its upload rate is at most 2^22 / the mean chunk lifetime:
    48.5 per second if every batch lived one day, 6.9 at a week, 1.6 at 30 days; ≲ 1–3 per second at
    realistic lifetimes. 50–100 per second happens only as a burst into a reserve that is not full,
    while the radius is not stable either. The model's 150 and 200 per second rows lie outside its
    validated range (part 2 validated it at the steady ~15 per second only; at 50 and 100 per second
    its N = 17 max, 221 and 411, already differs from the harness's 169–199 and 400) and carry no
    weight.
  - Recommendation: **keep 256**, confirmed in T6.3 against testnet's `pairwise_diff_half`, read as
    the max per 10-minute window (Follow-ups, "(T4.6 part 3 → T6.0) Q9's check reads
    `pairwise_diff_half`"). Up to the one-day bound (≤ 48 uploads per second) the max is 104 at
    N = 9 and 169–199 at N = 17 with 8 slots, 1.3–2.5× below 256; at the reference load it is
    35–82, 3–7× below. If steady-state sides cross 256 on testnet, do not raise it to a fixed 1024:
    a fixed threshold only moves the crossing with the load, and catch-ups of 256–1023 missing
    chunks would stop holding `SyncRate`. Make the rule load-relative instead, for example: a side
    is a catch-up when its wants exceed max(256, a multiple of the wants the node's recent inflow
    predicts for the time since the pair's last session). A design for T4.x or T6, not measured
    here.
  - The per-node initiated-session cap (Follow-ups, "(T4.6 part 3 → G6, T4.x) A per-node
    initiated-session cap") decides the margin: with a cap of 16 the N = 17 max at 100 uploads per
    second is 185, below 256 with no change to `CatchUpMinDiff`; without a cap, N ≥ 17 crosses near
    100 per second, a burst rate only. Either way 256 holds at stable-radius rates.

  **AC-B2: a bootstrapping node's peers list their reserves to it.** A T6.1 entry blocker and
  pre-testnet blocker (Follow-ups); no G6 constant changes it.
  - Cause, by the code. `Decide` (session.go ~L184–196) returns `DecisionSendItems` when the peer's
    count in a range is 0 and `DecisionRequestItems` when this node's is 0, before the stripe
    branch. The stripe branch (d̂ > `LargeDiff`) is the initiator's only, so it is taken on a child
    range (the responder decides the root), and it is direction-blind: `StripeCollector` takes any
    range, and the reconciler transfers it as its own catch-up whichever side lacks (and the
    registered range holds its `SyncRate`: the blocker "stripe registry SyncRate pin"). Only the
    bootstrapping node knows that it bootstraps (`planCatchUp`, `IsBootstrap`). It opens no round
    session meanwhile (`scheduleLocked`), but every peer with a lower overlay keeps initiating with
    it each round, since their digests differ until the transfer ends, and a peer with a higher
    overlay takes over once the node ran no session with it for `takeoverRounds` intervals. Such a
    session lists what the node lacks: at count 0 the node's answer to the root is
    `DecisionRequestItems`, a listing of the peer's whole reserve; past that, the descent lists or
    decodes the rest. (With a root difference past 16 × `LargeDiff`, ~1.05M, the peer instead
    stripes its own root from its peers, and the stripes it assigns to the node list to it.)
  - Measured: `TestG6BootstrapPeers` (new, `-tags=sim`): N = 5, node 0 empty, Base 70000 and 100000
    (above `LargeDiff`, as the review asked), Delay 50 ms, the fetch limit lifted as in
    `TestG6Bootstrap`; the harness's new `Flows` give the bytes per peer and direction. One run per
    seed; rank is node 0's lower-overlay peers:

    | Base (AC-B2 bound) | seed | rank | node 0 metadata in, MB | its stripes, MB | each lower-overlay peer: MB (sessions; wants in all, the largest session's) | each higher-overlay peer: MB (sessions) |
    |---|---|---|---|---|---|---|
    | 70000 (6.6 MB) | 2 | 1 | 9.47 (1.43×) | 4.91 | 4.56 (5; 63,731, 45,500) | 0 (1) |
    | 70000 | 3 | 1 | 9.47 (1.43×) | 4.91 | 4.56 (4; 63,761, 45,500) | 0 (1) |
    | 70000 | 1 | 3 | 18.94 (2.85×) | 4.91 | 4.68 (4–5; ~65,300, 48,500) | 0 (1) |
    | 70000 | 4 | 4 | 25.06 (3.78×) | 4.91 | 5.04 (4–5; ~70,550, 50,250) | – |
    | 100000 (9.05 MB) | 1 | 2 | 30.22 (3.34×) | 7.01 | 10.25 (5–6; ~140,500, 76,500) | 1.36 (1; 18,160 and 17,928) |
    | 100000 | 2 | 2 | 30.05 (3.32×) | 7.01 | 10.17–10.21 (5; ~141,200, 76,750) | 1.31–1.35 (1; 18,195) |

    The stripes moved each ID once (70.1 B per ID). On top, each lower-overlay peer listed 0.93–1.03
    reserves at Base 70000 (caught up in 14.8–15.3 s of the clock) and 1.45–1.46 at Base 100000
    (19.6–20.1 s): its sessions repeat every round and each lists what node 0 still lacks (the wants
    sum to 1.35–1.84× the largest session's). At Base 100000 the transfer outlasted the takeover
    wait, and each higher-overlay peer's takeover listed another 0.19 reserves. At Base 70000 the
    higher-overlay peers' one session came after the transfer (under 1 KB). AC-B2 fails at every
    rank measured (1–4), by 1.4–3.8×. Part 3's Base 20000 figures (N = 17: 4.2–5.7 and 12.5–14.0 MB
    against 2.6 MB) are the same effect.
  - In production, by the same path (not measured): a session that meets the node at count 0 lists
    the peer's whole reserve, 2^22 × 70 B ≈ 294 MB, against AC-B2's ~340 MB for the whole bootstrap,
    of which the stripes take ~294 MB; one such session fails AC-B2. And the measured (1 + rank)
    reserves are a lower bound: the cost grows with the transfer's duration (above), and a 2^22
    transfer takes ~70 minutes at the default `FetchRate` (1000 chunks per second), against the
    harness's 15–20 s of clock. Each lower-overlay peer's sessions then run back to back, each
    listing what the node still lacks, and the higher-overlay peers take over between the node's
    stripe sessions with them: the node's inbound bandwidth bounds the metadata, not the reserve.
  - Not tunable: recorded with the root cause and the fix directions as a T6.1 entry blocker and
    pre-testnet blocker (Follow-ups), next to the stripe-registry SyncRate pin and the per-peer
    fetch credit.

  **`GrowthFactor` 2: the recommendation stands; it is not a change of the constant alone.**
  - Four tests in `pkg/rangesync` assume `GrowthFactor` 4 and must change with it (the review's GF2
    build, `go test ./pkg/rangesync/...`; the other packages pass): `TestProtocolConverges/4k` (40
    Messages against a bound of 29: integration_test.go ~L235–240 adds 2 × (5 + 4) Messages, the
    Mores from `MinBatch` to `RootSymbols` and to `Level1Symbols` at ×4; at ×2 they are 10 and 7);
    `TestWorkedExamples` (example 3: 9–11 Messages against the proposal's 7; §7.2: 5 Messages of
    813 B, a More to 8 instead of 16, not a listed outcome; example 2: 5 Messages of 11.3 KB and 7
    of 22.3 KB, not listed); `TestSREPSessionCost` (its "now" case reads the compiled constant: 1.89
    symbols per difference at d = 45, outside its [2, 2.8]); `TestSREPIdeal` (it asserts
    `GrowthFactor` 2 below the compiled constant; equal in a GF2 build).
  - The wire is compatible: `handleMore` checks only sent < UpTo ≤ cap (session_symbols.go
    ~L285–289), and the side that serves a stream follows the peer's Mores whatever their step, so
    GF2 and GF4 peers interoperate, each requester growing by its own constant. A rolling upgrade
    needs no version.
  - An unhinted two-sided difference with equal counts takes about twice the Mores.
    `TestG6EqualCounts` (new, `-tags=sim`): one session, A and B share ScanLimit + 1000 IDs (a
    cached root) and hold k IDs each that the other lacks, so d̂ = 1; 5 seeds, means; GF4 / GF2:

    | n_Δ (k per side) | Mores | Messages with parts | KB | root stream exhausted |
    |---|---|---|---|---|
    | 26 (13) | 2.0 / 3.8 | 7.0 / 10.6 | 6.2 / 5.7 | 0 / 0 of 5 |
    | 100 (50) | 3.0 / 6.0 | 9.0 / 15.0 | 24.4 / 24.5 | 0 / 0 |
    | 400 (200) | 4.0 / 8.0 | 11.0 / 19.0 | 97.1 / 97.2 | 0 / 0 |
    | 1000 (500) | 5.0 / 9.0 | 13.0 / 21.0 | 367 / 201 | 0 / 0 |
    | 2000 (1000) | 5.0 / 10.0 | 13.0 / 23.0 | 402 / 402 | 0 / 0 |
    | 3000 (1500) | 21.4 / 39.8 | 17.0 / 29.4 | 750 / 648 | 2 / 2 |

    Each More is a round trip: at n_Δ 2000 GF2 takes 5 more, 0.5 s at 50 ms one way and 2.0 s at
    200 ms, and never more bytes. (At 3000 the root exhausts in 2 of 5 sessions, and the level-1
    streams' Mores add in.) `TestG6Catchup` is one-sided (the counts differ by Lag, so d̂ is exact)
    and could not show this. Equal counts over a large difference without a hint take a restart (the
    hint LRU is empty) or a partition whose churn balanced the counts: rare, and latency only.
  - The Delay cost, corrected (`TestG6Constants`, hint on, 6 seeds, GF4 → GF2): at 200 ms +0.22 and
    +0.16 round trips per session (4.43 → 4.65 at N = 9, 4.47 → 4.63 at N = 17), +0.08 and +0.22 s
    of mean convergence, and +0.8 s of the longest (8.2 → 9.0 s and 10.4 → 11.2 s; the review's
    re-run of GF2 matched, 9.0 and 11.2 s); at 100 ms +0.21 and +0.18 round trips, the longest +0.4
    and +0.2 s; at 50 ms +0.28 and +0.20, the longest +0.2 and +0.1 s. The summary's "+0.1–0.2 s of
    the longest" held at 50 ms only.
  - Sparse load, closest to mainnet's rate (`TestG6RoundInterval`'s 6 batches of 8 uploads 47 s
    apart, 5–30 s, Delay 50 ms, 3 seeds; the review's GF2 build against two runs of now): N = 9
    112.5–113.9% → 93.3%, N = 17 85.4–86.7% → 77.2%; symbols per difference 3.79 → 2.78 and 3.80 →
    3.11, for +0.21 and +0.50 Mores per upload; the same convergence (longest 10.75 and 10.95 s).
    The gain holds where mainnet's load is: −19 to −21 points at N = 9 (−18 steady), −8 to −10 at
    N = 17 (−13 steady). Both stay above 50%, AC-B1b's informational regime.
  - Recommendation unchanged: **`GrowthFactor` 2**, landing together with the four tests, and
    accepting about twice the Mores for an unhinted large difference (latency, never bytes).

  **`BatchFactor` 1.2 and 1.4 with `GrowthFactor` 2 (built now).** Scratch builds with both
  constants changed, `TestG6Constants`, hint on, 6 seeds; share at Delay 50 / 100 / 200 ms, the rest
  at 50 ms unless named; 1.2's N = 17 rows at 50 and 200 ms ran twice (the second run in brackets):

  | `BatchFactor` (GF 2) | N = 9 | N = 17 | N = 17 at 200 ms: sd, range | Mores per upload, N = 9 / 17 | round trips per session, N = 9 / 17 | longest at 200 ms, N = 9 / 17 |
  |---|---|---|---|---|---|---|
  | 1.2 | 60.0 / 59.7 / 59.5 | 45.2 [45.9] / 46.4 / 48.8 [49.1] | 1.4 [0.9], 47.2–50.8 [47.3–50.0] | 0.53 / 1.70 | 5.11 / 4.96 | 9.0 / 11.6 [11.2] s |
  | 1.4 | 59.6 / 59.5 / 59.0 | 46.9 / 47.4 / 50.3 | 1.2, 47.9–51.3 | 0.44 / 1.42 | 4.91 / 4.80 | 9.0 / 11.6 s |
  | 1.6 (now) | 59.8 / 60.6 / 59.9 | 48.3 / 49.1 / 52.1 | 1.4, 51.0–54.1 | 0.36 / 1.22 | 4.79 / 4.68 | 9.0 / 11.2 s |
  | 2.0 (summary) | 62.2 / 64.7 / 64.4 | 52.3 / 53.1 / 56.4 | – | – | – | – |

  - At N = 17 a lower `BatchFactor` gains 2.7–3.3 points (1.2) and 1.4–1.8 (1.4) at every Delay, and
    the spreads separate (1.2: 4–7 standard errors of the difference at 50 and 200 ms; 1.4: 2.4–4).
    The model predicted 1–3 points (46.0 / 47.7 / 49.1% at 50 ms); the harness confirms it. At N = 9
    there is no gain (59.0–60.0% against 59.8–60.6%), as the model had it.
  - Does it bring N = 17 under 50% without the per-node cap? 1.2 does on the mean at every Delay,
    but at 200 ms by 0.9–1.2 points with an sd of 0.9–1.4 (its worst seeds at 50.0% and 50.8%); 1.4
    does not (50.3% at 200 ms). On the wire basis all three pass (1.2: 44.8–45.1%, 1.4: 46.2%, 1.6:
    47.9%).
  - The cost of 1.2: +0.32 and +0.28 round trips per session (N = 9 and 17), +0.17 and +0.48 Mores
    per upload, +0.0–0.4 s of the longest convergence at 200 ms at N = 17 and +0.2–0.3 s of its mean
    (6.39 → 6.56–6.69 s).
  - Recommendation: **keep 1.6** (the summary's "keep" vs 2.0 stands: same sign in all 6 cells). 1.2
    buys ~3 points at N = 17, none at N = 9, for ~0.3 round trips per session, and its N = 17 pass
    at 200 ms is within one sd of the line; the per-node initiated-session cap gives ~10 points
    (39.5–40.3%) with faster convergence. Only if G6 wants AC-B1 at N = 17 on the modeled base
    without that cap is 1.2 the constant to take, with its round trips.

  **AC-B1 caveats (extends the summary's).**
  - The 16-slot build also doubled `MaxStripeSessions` (scheduler.go ~L88: `MaxInitiatedSessions /
    2`, 4 → 8). The steady runs start no stripe, so it did not act there; a production cap must keep
    `MaxStripeSessions` apart (Follow-ups, the per-node cap).
  - The harness's uploads arrive in synchronized batches (20 at once every 1.3 s), and its nodes
    start together, so their rounds share one 5 s grid with no phase offset (the harness's only
    jitter is the takeover wait's, as in production). Real rounds and uploads are spread; whether
    that lowers or raises the share is not known. T6.1 and T6.3 measure the real one.

  **Q2, caches: keep for equal radii (revises the evidence).**
  - "No stream exhausted a cache in any of the ~460 steady-state runs" says nothing about the
    caches: at Base 256 every node holds fewer than `ScanLimit` (4096) IDs, so every root stream ran
    on the scan path, unlimited. Only the 4 Base-5000 runs (`TestG6Load`, 2 seeds at N = 9 and 17,
    50 uploads per second) served the root from `RootSymbols`, as did `TestG6Catchup`'s (Base
    70000), whose catch-ups ran past it.
  - The recommendation rests on the bound, not on that count: the largest first batch at 100 uploads
    per second is 777 symbols (19% of `RootSymbols`), and the root splits only past d̂ = 2558,
    whatever the reserve.
  - That holds for equal radii only. A session with a peer one bit deeper runs on the peer's root,
    one bit below this node's, which `cacheCapacity` (session_symbols.go ~L44–57) serves from
    `BucketSymbols` (32: depth 1 ≤ `LeafBits`) once this node holds more than `ScanLimit` there, so
    such a session exhausts past n_Δ ≈ 18 and splits. "Keep 4096 / 512 / 32" is for equal radii;
    mixed radii stay with the Follow-up "(T4.6 part 3 → T6, a radius-aware harness) Nested session
    roots use the bucket cache".

  **`RoundInterval` bounds and hint(R) (records).**
  - The review re-ran `TestG6RoundInterval` on the committed code: every row within its seed spread
    (for example the sparse 5–30 s rows at 113.9% and 86.7% against 112.5% and 85.4%, the steady
    ones at 78.0% and 60.9% against 76.0% and 62.0%; the same convergence). "Keep 5–30 s" stands.
  - The stale catch-up hint is bounded: a root hint is a difference that decoded at the root, and a
    first batch above `RootSymbols` splits instead, so the next session's first batch is at most
    `FirstBatch(2558)` = 4095 symbols (~336 KB at 82 B each), once per pair (Follow-ups).

  **G6 decision sheet.** For the user at G6; confidence is in the measurement, T6.3 is what testnet
  must confirm.

  | decision | recommended | key number | confidence | T6.3 (or T6.x) confirms |
  |---|---|---|---|---|
  | Q2 caches (`RootSymbols`, `Level1Symbols`, `BucketSymbols`) | keep 4096 / 512 / 32 | steady first batch ≤ 777 symbols (19% of the root) at 100 uploads/s; a catch-up splits the root past ~2500 missing, the level-1 ranges past ~5000 | high for equal radii; mixed radii unmeasured (a nested session root gets 32 symbols) | catch-up sizes and frequency; `streams_exhausted_total{level}` where radii differ (radius-aware harness or testnet) |
  | `GrowthFactor` | **2** (from 4), landing with its 4 tests | −12 to −18 points of share at the steady load, −8 to −21 at the sparse one, never more bytes; costs +0.16–0.28 round trips per session, +0.8 s of the longest at 200 ms, ~2× the Mores (+5 round trips) on an unhinted equal-count difference of 1000–2000 | high: the spreads separate at every Delay, both N; wire-compatible with GF4 peers | share and convergence at real latency; mixed GF2/GF4 peers during the upgrade |
  | request-driven `More`; hint(R) | keep both | streaming loses on any link above ~20 KB/s at 100 ms (+3.5 and +12.8 KB per upload at 100 KB/s); the hint saves 70% of the Mores for 0–6 points | high | Mores per upload; the stale catch-up hint (≤ 4095 symbols per pair, once) |
  | `BatchFactor` | keep 1.6 | 2.0 costs 2.4–4.5 points with GF2; 1.2 gains 2.7–3.3 at N = 17 only, for +0.3 round trips per session | high against 2.0 (same sign in 6 cells); 1.2's gain is real but marginal, worth it only without the per-node cap | nothing specific |
  | `CatchUpMinDiff` | keep 256 | wants max 35–82 at the reference load, 104 (N = 9) and 169–199 (N = 17, 8 slots) at 50 uploads/s; stable-radius rate ≤ 48/s (1-day lifetimes), ≲ 1–3/s realistic; crossings only at 100/s, N = 17, 8 slots (185 with 16) | medium: the rate bound is an argument, not a measurement | `pairwise_diff_half` max per 10-minute window; on crossings a load-relative rule, not a fixed 1024 |
  | `RoundInterval` bounds | keep 5–30 s | longer minimums: +1.2–2.5 s of mean convergence and no bytes saved; the maximum costs only heartbeats (352–355 B per node-minute at N = 9, 704–717 at 17) | high (re-run by the review) | T6.4: AC-V1's 2 × 5 s is passed at the sparse load (10.75–10.95 s); the bound needs 15 s |
  | initiated-session cap | per node: the eligible higher-overlay peers, within the tokens served sessions leave, `MaxStripeSessions` kept apart (not a constant 16); a code change after G6 | N = 17: −10 to −15 points of share, the wants max halved at 100/s (400 → 185), 0.3 s faster mean convergence, no Busy | medium: harness at N ≤ 17; the 16-slot build also doubled `MaxStripeSessions` | Busy and session-limit answers and the share at N ≥ 17 |
  | AC-B1 per N | N = 17: GF2 passes at 50 and 100 ms and misses by 2.1 points at 200 ms on the modeled base (passes on the wire basis, by ~10 with the per-node cap, narrowly with `BatchFactor` 1.2); N = 9 fails with every constant (best 59.0–60.6%, 54–56% on the wire) | re-base at N = 9 (T6.0's call), for example ≤ 65% at N = 9 and ≤ 50% at N ≥ 17 | medium: synchronized harness load, no round phases | T6.1 baseline against `rangesync` only; T6.3 |

  Pre-testnet blockers, all before any testnet cohort enables `rangesync` (Follow-ups): the stripe
  registry SyncRate pin; the per-peer fetch credit; and the bootstrapping node's peers listing their
  reserves to it (AC-B2), also a T6.1 entry blocker. Code changes after G6 that are not blockers:
  `GrowthFactor` 2 with its tests, the per-node initiated-session cap, the stale catch-up hint, and
  caches for mixed radii (T6).
- **(T4.6 part 3 fix round 1) Scope, tests and wall times.**
  - Test-only, in `pkg/rangesync`: `neighborhood_sim_test.go` (`simResult.Flows`, the stream bytes
    by writer, reader and opener; `simPair.to`, the handler's node), `neighborhood_g6_test.go`
    (`TestNeighborhoodG6Figures` checks that the flows split `StreamBytes` exactly, between two
    nodes) and `neighborhood_g6_sim_test.go` (`TestG6BootstrapPeers` and `TestG6EqualCounts`,
    `-tags=sim`). No production change and no constant committed: the candidates (`GrowthFactor` 2;
    `GrowthFactor` 2 with `BatchFactorNum` 12 and 14) ran in scratch copies of the worktree. There
    is no `pkg/node`, `pkg/p2p`, `pkg/pullsync`, `pkg/puller`, `go.mod` or proto change, no new
    parser of peer input and no `*_fuzz_test.go`.
  - `-tags=sim`, one run at a time: `TestG6BootstrapPeers` 405 s at Base 70000 (4 runs of 95–108 s:
    the transfer waits on the holders' delivery limiter, on the real clock) and 302 s at Base
    100000, seeds 1 and 2 (144–157 s each; the test process peaked near 0.7 GB); `TestG6EqualCounts`
    0.9 s (1.1 s in the GF2 build); `TestG6Constants`' hint-on rows at 50, 100 and 200 ms 258 s in
    the GF2 + `BatchFactor` 1.2 build (its N = 17 rows at 50 and 200 ms once more, 95 s) and 152 s
    in the 1.4 build. The review's own logs supplied the 16-slot `TestG6Load`, the GF2 sparse and
    constants runs, the GF2 unit failures and the `TestG6RoundInterval` re-run.
  - Default build: `go test ./pkg/rangesync/...` passes (`pkg/rangesync` 11 s); the neighborhood and
    `TestSim*` tests pass at `-count=3 -cpu=1,2,4` (42 s). `make build`, `GOTOOLCHAIN=go1.26.1 make
    lint`, `golangci-lint run --build-tags sim ./pkg/rangesync/...` and `go vet` (default and
    `-tags=sim`) are clean; gofumpt and gci ran on the changed files only.
- **(T4.6 part 3 fix round 2) G6 summary (revised, round 2).** This supersedes, in "(T4.6 part 3 fix
  round 1) G6 summary (revised after review)": in its Q9 part, the rate bound of the bullet "The
  loads are not mainnet rates" (from "At a stable radius" to "while the radius is not stable
  either"; its sentences on the 15.4 per second regime and on the model's 150 and 200 per second
  rows stand), "Up to the one-day bound (≤ 48 uploads per second)", "a burst rate only" and "Either
  way 256 holds at stable-radius rates"; in its AC-B2 part, the bullets "In production, by the same
  path (not measured)" and "Not tunable: recorded with the root cause and the fix directions"; in
  its `GrowthFactor` part, "rare, and latency only"; in its `BatchFactor` part, "a lower
  `BatchFactor` gains 2.7–3.3 points (1.2)"; the label "its stripes, MB" of its AC-B2 table
  (Records, below); and its "G6 decision sheet" with the blocker paragraph after it, which the "G6
  decision sheet (final)" below replaces. In the Follow-ups, the corrected AC-B2 entry supersedes
  the round-1 fix directions. The measurements stand; nothing was re-measured. No production code or
  constant changed; `TestG6BootstrapPeers` changed its comment and one table label only.

  **AC-B2: the fix directions are withdrawn; a fix must meet four constraints.**
  - Stripe sessions are count-0 listings past `LargeDiff`. Node 0 opens FP(sr, 0)
    (stripes_transfer.go ~L505, `Open: sr` at its count 0), and the holder, as responder, lists all
    of sr (`Decide`: `Remote.Count == 0` gives `DecisionSendItems`, session.go ~L187). At 2^22 with
    5–32 peers `PlanStripes` cuts 8–32 stripes of 2^17–2^19 IDs, 2–8 × `LargeDiff` (2^20–2^21 with
    2–4 peers). Round 1's direction 3 ("no session lists more than `LargeDiff` IDs of a range") and
    direction 4 (a responder-side cap) refuse exactly these sessions, and the bootstrap stalls.
  - Direction 4 acts on the wrong side. In a round session node 0 is the responder: at count 0 it
    answers the root with an empty `Items{Reply}` (`DecisionRequestItems`, session.go ~L746), and
    the initiator does the listing in its answer; in the descent the initiator lists each child at
    node 0's count 0 (`DecisionSendItems`). A responder-side cap misses the measured leak and hits
    the stripes.
  - Direction 2 compares counts across roots, which `IsBootstrap` deliberately avoids (stripes.go
    ~L48–55, equal roots only): an edge peer 1–2 bits deeper (§7.6) holds ½–¼ the count, falls below
    `BootstrapRatio` (0.5) and would be deferred indefinitely, syncing only through takeovers.
    Direction 1 rests on `IsBootstrap`, which misses a bootstrapping node whose root differs from
    every neighbor's (its doc says so). "1 or 2 closes the bootstrap case" overclaimed.
  - The harness did not run the production path; round 1's "by the same path" was wrong. Its stripes
    are 17,500–25,000 IDs (Base 70000–100000 over 4 peers) and its level-1 ranges at most ~6,300,
    all below `LargeDiff`. At 2^22 the stripes exceed it, and while node 0's root difference exceeds
    16 × `LargeDiff` (~1.05M, the first ~75% of a 2^22 transfer) the level-1 ranges do too: once
    node 0 holds anything it splits the root (as responder it cannot stripe), and the peer, as
    initiator, hands each child past `LargeDiff` to its `StripeCollector`, direction-blind. The peer
    then registers a range it already holds (its own `SyncRate` held: the stripe-registry blocker),
    and the stripes it plans on node 0 list to node 0 again (node 0 answers each at count 0 with
    `Items{Reply}`). What stands: a session that meets node 0 at count 0 lists the peer's reserve,
    2^22 × 70 B ≈ 294 MB against AC-B2's ~340 MB, and the cost grows with the transfer's duration
    (~70 minutes at 2^22 and the default `FetchRate`). How the child-range path adds up at 2^22 is
    not measured.
  - Constraints any fix must meet:
    - (a) stripe sessions keep working: a count-0 listing past `LargeDiff` that the bootstrapping
      node requests (it opens the session at count 0) is served in full;
    - (b) a round session does not make a non-bootstrapping peer list its whole reserve, or most of
      it, to a count-0 or near-empty peer: the measured leak, which the *initiator* lists (its
      answer to `Items{Reply}`, and `DecisionSendItems` on the children);
    - (c) edge peers and nested roots (§7.6) and stale-stamp divergence (§7.7) are neither deferred
      nor starved;
    - (d) detection does not rest on the equal-root `IsBootstrap` alone: a bootstrapping node whose
      root differs from every neighbor's is covered too.
  - Candidate approaches (T4.x chooses; none measured), with the side each acts on:
    - A. *The lacking side declines* (acts on the side with the smaller count, in either role; in
      the leak, node 0 as responder). `Decide` takes the large-difference test before the count-0
      branches, by direction: the side that holds fewer by more than the threshold neither asks for
      items nor splits; as initiator it stripes (as now), as responder it hands the range to its own
      transfer and answers nothing for it (the empty answer `DecisionEqual` gives; the peer's
      session then ends, which the frozen wire allows). The holder's count-0 `DecisionSendItems`
      stays, so (a) holds. It meets (b) at the root and in the descent, (d) (it compares two counts
      of one range of one session root) and (c) (same-range counts; §7.7 differences are far below
      the threshold). Only the lacking side changes, so it protects a new node against old peers
      during a rolling upgrade. Trade-offs: responders get a Striper (initiators only today,
      session.go ~L82–84), so it must land with the stripe-registry fix (a peer claiming a far
      larger count would make responders register too) and must not register what the bootstrap plan
      already holds; it does not stop the holder's listing in a round session that the lacking node
      opens itself at count 0 (the same answer a stripe session gets), so an undetected
      bootstrapping node (d) must not open round sessions far below the peer's count in the session
      root (a session-root check next to `IsBootstrap`); and below the threshold the descent still
      lists what node 0 lacks, up to `LargeDiff` × 70 B ≈ 4.6 MB per session through the transfer's
      tail (~65 s of fetches at the default `FetchRate`), which over a few lower-overlay peers and
      rounds can pass AC-B2's ~45 MB headroom (340 − 294 MB): the threshold for a node with a
      transfer running may need to sit below `LargeDiff` (an estimate, not measured).
    - B. *The initiator takes the large-difference test first, by direction* (acts on the initiator:
      the peer in a round session, or a higher-overlay peer's takeover). On a child Fingerprint the
      initiator's `Decide` tests the large difference before the count-0 branches: where it holds
      more by more than `LargeDiff` it neither lists nor stripes (it leaves the range to the lacking
      side); where it holds fewer it stripes, as now. It removes the child-range path and the
      holder's own registration (the stripe-registry case of a peer claiming a smaller count).
      Trade-offs: it does not reach the root's count-0 answer, which runs no `Decide` on the
      initiator (`handleItems` answers the `Items{Reply}`), so it needs C or A for the root; it
      changes the peer, so old peers keep listing during a rolling upgrade; and the lacking side
      must then take the range itself (its stripes, or A).
    - C. *The initiator's listing is capped and becomes a stripe hand-over* (acts on the initiator,
      with a hand-over on the responder). Past `LargeDiff` of difference the initiator answers an
      `Items{Reply}` with at most a cap of IDs and ends its answer early, which the frozen wire
      allows (the responder checks only that the answer does not end before its own last Reply page,
      session.go ~L813, and a count-0 Reply is one empty page); the responder, whose listing came
      out far shorter than the counts' gap, hands the range to its Striper. The stripe sessions are
      untouched, since there the holder answers a Fingerprint as responder (a). Trade-offs: a cap's
      worth of IDs per session until the hand-over (per round when the responder does not hand over:
      a drip rather than a stall, so (c) holds); the responder-Striper and registry dependencies of
      A; and the peer changes, so old peers keep listing.
    - D. *The bootstrapping node answers Busy* (round 1's direction 1; acts on node 0 as responder).
      It is legal on the frozen wire (Busy as the responder's first Message), covers lower-overlay
      initiators and takeovers alike, and Busy is not scored. Trade-offs: keyed on the reconciler's
      bootstrap state it fails (d); keyed instead on the opening Fingerprint's count against its own
      in the session root it meets (d), but it refuses whole sessions, the small differences of new
      uploads and of §7.7 included, for the transfer's duration (~70 minutes at 2^22), leaving them
      to the verification round; and the peer's Busy backoff must not delay its sessions past that.
    - E. *An explicit bootstrap signal in `Status` or the Fingerprint* (acts on both sides). The
      cleanest detection, but the wire is frozen (G2): it needs a field G2 forbids or a new protocol
      version, so it is out of scope unless G2 is reopened. Round 1's direction 2 (defer by Status
      count) is its implicit form, and it fails (c) across roots and (d) within equal roots.
  - Acceptance needs a scaled case, since the harness's geometry cannot show the production path: a
    test-settable `LargeDiff` (a const today, constants.go ~L35; an `Options` field or an
    `export_test.go` hook) near Base/64, which keeps 2^22's ratio of the root difference to
    `LargeDiff` (about 1,100 at Base 70000: stripes past it, and the child-range path for the first
    ~75% of the transfer; a scaled `LargeDiff` also stripes catch-ups that would decode at 2^22's
    scale, so the test reads the bootstrap only); or a harness Base past 4 × `LargeDiff` for stripes
    past it (≥ ~262,000 at N = 5) and past 16 × `LargeDiff` (~1.05M) for the child-range path,
    beyond this host's memory (Base 100000 peaked near 0.7 GB). It must check (a) that the stripes
    complete, (b) node 0's metadata within AC-B2 scaled to Base at ranks 1–4 with takeovers firing,
    and (c) and (d) with a peer one bit deeper and a bootstrapping node whose root no peer shares
    (the radius-aware harness of T6, or a session-level test). Then `TestG6BootstrapPeers`,
    `TestG6Bootstrap` and T6.1's Bootstrap case.
  - The scope is unchanged: a T6.1 entry blocker and pre-testnet blocker (Follow-ups, the corrected
    entry).

  **Q9, `CatchUpMinDiff`: keep 256, at medium to low confidence.**
  - Round 1's rate bound misapplied Little's law twice. (i) The inflow is the reserve over the mean
    residence, and the residence is not the batch's TTL: uploads arrive throughout a batch's life,
    so a chunk stays for the batch's remaining life, about half of it on average with uniform
    uploads. At a one-day TTL the bound is ~97 per second (~14 at a week, ~3 at 30 days), and ~97 is
    where N = 17 with 8 slots already crosses 256 (7 sides at 100 per second). (ii) Overwrites add
    IDs without growing the reserve. On a stamp-index collision with a newer timestamp the reserve
    replaces the older chunk (pkg/storer/internal/reserve/reserve.go ~L189–195, and `removeChunk`
    for another address), and the newcomer is a new ID. Postage charges per slot and time, not per
    write, so a mutable batch's owner can rewrite its slots at any rate for the cost of the uploads:
    a sustained rate is bounded by no TTL, and "only as a burst" is false.
  - The crossing load is about 50–100 IDs per second per neighborhood at N = 17 with 8 slots (wants
    max 169–199 at 50, 400 at 100). It is lower at N > 17, where a node's initiated pairs run in
    more waves (untested), and higher with the per-node initiated-session cap (16 slots: 185 at 100
    per second).
  - A crossing pins `SyncRate`. A side at 256 or more is a catch-up (scheduler.go ~L521) and its
    fetches count towards `SyncRate`; a sustained stream past the crossing load keeps it above 0,
    which stops the reserve worker's radius decrease (pkg/storer/reserve.go ~L176) and the storage
    incentives' `isFullySynced` (pkg/node/node.go ~L1378) on the neighborhood's nodes while it
    lasts. `pkg/puller` adds only historical intervals to its rate (puller.go ~L392–397), so live
    load never pins it: here rangesync would do worse. The pin lifts within the rate window once the
    stream stops. T6.3's ordinary testnet load is far below the crossing and won't show it.
  - Recommendation: **keep 256**, at medium to low confidence. It holds at the loads measured and at
    the fresh-upload rates of realistic lifetimes, but not against a sustained overwrite stream, and
    a fixed raise (1024) only moves the crossing. Follow-ups: a load-relative or overwrite-aware
    catch-up rule before mainnet (T4.x or T6), and a T6.3 item that drives a sustained overwrite
    stream on purpose. Not a pre-testnet blocker (the final sheet's blocker list says why).

  **`GrowthFactor` 2: the recommendation stands, with three additions.**
  - The proposal's text changes with the constant (pullsync-rangesync-rateless-proposal.md): §6.3's
    normative "`More{R, UpTo: min(4 × received, cap(R))}`" (L824) and "growth factor 4" (L830), and
    the worked examples: §7.2's "`More(root, UpTo: 4 × 7 = 28)`" (L894), example 2's "one More(root,
    UpTo 272)" (L1175, 68 × 4) and example 3's "More(root, UpTo 16)" and "More(root, UpTo 64)"
    (L1191, L1194; GF2 grows 4 → 8 → 16 → 32 → 64). No other code depends on 4: the constant's one
    use is the requester's `upTo` (session_symbols.go ~L403), and session.go ~L568 names it in a
    comment.
  - An unmeasured CPU cost: on the scan path (no cache, `cacheCapacity` 0) each More reruns
    `Local.Symbols(r, 0, end)` from symbol 0 (`symStream.window`, session_symbols.go ~L125), reading
    the range's elements again. GF2 about doubles a stream's Mores, so it about doubles an honest
    scan stream's reads (at most `MaxScansPerMessage`, 64, scan batches per Message). Recorded; not
    a reason to keep 4.
  - Unhinted sessions are rare only for honest causes (round 1: a restart, a balanced partition). A
    peer can flush the shared hint LRU (Follow-ups, "(T4.6 part 1 fix round 1 → TF/hardening) One
    peer can flush the hint LRU" and "(T4.6 part 1 fix round 2) The hint LRU: stale entries and a
    cheaper flush"), after which the pairs' sessions run unhinted and each large equal-count
    difference takes about twice GF4's Mores: latency, never bytes, until that follow-up lands.

  **Records.**
  - `BatchFactor` 1.2's gain at N = 17 is 2.4–3.3 points (its second run at 50 ms: 45.9 against
    48.3), not 2.7–3.3.
  - The per-node initiated-session cap gives −9 to −12 points with GF2 (16 slots and GF2:
    39.5–40.3%, against GF2's 48.3–52.1%), not the sheet's −10 to −15.
  - The AC-B2 table's "its stripes, MB" is the bytes node 0 read on every reconcile stream it
    opened: its stripes, its verification and its post-bootstrap sessions. `TestG6BootstrapPeers`
    now labels it "node 0's sessions (stripes and later)". Its comment said takeovers do not fire;
    they did at Base 100000 (the higher-overlay peers' 1.31–1.36 MB), and the comment now says so.

  **G6 decision sheet (final).** This supersedes the "G6 decision sheet" of "(T4.6 part 3 fix round
  1) G6 summary (revised after review)" and, where they differ, the per-question recommendations of
  "(T4.6 part 3) G6 summary: the measured inputs and a recommendation per question". For the user at
  G6; confidence is in the measurement, and the last column is what testnet must confirm.

  | decision | value | key number | confidence | T6.3 (or T6.x) confirms |
  |---|---|---|---|---|
  | Q2 caches (`RootSymbols`, `Level1Symbols`, `BucketSymbols`) | keep 4096 / 512 / 32 | steady first batch ≤ 777 symbols (19% of the root) at 100 uploads/s; a catch-up splits the root past ~2500 missing | high for equal radii; mixed radii unmeasured (a nested session root gets 32 symbols) | `streams_exhausted_total{level}` where radii differ; catch-up sizes |
  | `GrowthFactor` | **2** (from 4), with its 4 tests and the proposal's text (L824, L830, L894, L1175, L1191–1194) | −12 to −18 points of share at the steady load, −8 to −21 at the sparse; +0.16–0.28 round trips per session, +0.8 s of the longest at 200 ms; ~2× the Mores on an unhinted equal-count difference (+5 round trips at n_Δ 1000–2000), more often while a peer can flush the hint LRU; ~2× an honest scan stream's reads (unmeasured) | high; wire-compatible with GF4 peers | share and convergence at real latency; mixed GF2/GF4 peers; scan CPU |
  | request-driven `More`; hint(R) | keep both | streaming loses on any link above ~20 KB/s at 100 ms; the hint saves 70% of the Mores | high | Mores per upload; the stale catch-up hint (≤ 4095 symbols per pair, once) |
  | `BatchFactor` | keep 1.6 | 2.0 costs 2.4–4.5 points with GF2; 1.2 gains 2.4–3.3 at N = 17 only, for +0.3 round trips per session | high against 2.0; 1.2's gain is real but marginal, worth it only without the per-node cap | nothing specific |
  | `CatchUpMinDiff` | keep 256 | wants max 35–82 at the reference load, 104 (N = 9) and 169–199 (N = 17, 8 slots) at 50/s; crossing at ~50–100 IDs/s per neighborhood (N = 17, 8 slots), lower at N > 17 (untested), higher with the per-node cap (185 at 100/s); fresh uploads ≤ ~97/s at one-day TTLs, overwrites bounded by no TTL | medium to low: loads measured, rates argued; a sustained overwrite stream pins `SyncRate` | `pairwise_diff_half` max per 10-minute window; a deliberate sustained overwrite stream (Follow-ups) |
  | `LargeDiff` | keep 65536; make it test-settable | at 2^22 with 5–32 peers, stripes of 2^17–2^19 IDs (2–8×); the child-range stripe path past 16× (~1.05M, the first ~75% of a 2^22 bootstrap); the AC-B2 fix interacts with it (below it the descent lists up to ~4.6 MB per session) | medium: not exercised at scale (harness stripes 17,500–25,000 IDs) | the scaled test first; T6.1's Bootstrap case (AC-B2), stripe sizes and counts |
  | `RoundInterval` bounds | keep 5–30 s | longer minimums: +1.2–2.5 s of mean convergence and no bytes saved; the maximum costs only heartbeats (352–355 B per node-minute at N = 9, 704–717 at 17) | high (re-run by the review) | T6.4: the sparse load's longest convergence (10.75–10.95 s) exceeds AC-V1's 2 × 5 s; the bound needs 15 s |
  | initiated-session cap | per node: the eligible higher-overlay peers, within the tokens served sessions leave, `MaxStripeSessions` kept apart (not a constant 16); a code change after G6 | N = 17 with GF2: −9 to −12 points of share; wants max at 100/s 400 → 185; mean convergence 0.3 s faster; no Busy | medium: harness at N ≤ 17; the 16-slot build also doubled `MaxStripeSessions` | Busy and session-limit answers; the share at N ≥ 17 |
  | AC-B1 per N | re-base at N = 9 (T6.0's call), for example ≤ 65% at N = 9 and ≤ 50% at N ≥ 17 | N = 17: GF2 passes at 50 and 100 ms and misses by 2.1 points at 200 ms on the modeled base (passes on the wire basis, by ~10 with the per-node cap, narrowly with `BatchFactor` 1.2); N = 9 fails with every constant (best 59.0–60.6%, 54–56% on the wire) | medium: synchronized harness load, no round phases | T6.1 baseline against `rangesync` only; T6.3 |

  Pre-testnet blockers, all before any testnet cohort enables `rangesync` (Follow-ups, "(T4.6 part 3
  fix round 2) Pre-testnet blockers (final)"):
  1. The bootstrapping node's peers list their reserves to it (AC-B2), also a T6.1 entry blocker: a
     fix that meets constraints (a)–(d) above, accepted by the scaled test.
  2. The stripe-registry `SyncRate` pin; before or with the AC-B2 fix if that fix gives responders a
     Striper (A, C).
  3. The per-peer fetch credit, with a credit that survives a peer's reconnects (the initial credit
     of a new or reconnected peer is the item to settle first).

  Not a blocker: the sustained-overwrite `SyncRate` pin. Unlike the stripe-registry pin, which one
  claimed count buys, it needs a sustained upload stream of ~50–100 IDs per second aimed at one
  neighborhood; it stores nothing wrong and lifts within the rate window once the stream stops; and
  testnet cohorts will not produce it by accident. It is a T6.3 item (Follow-ups, "(T4.6 part 3 fix
  round 2 → T6.3) A sustained overwrite stream"), and the catch-up rule's fix lands before mainnet
  enablement. Code changes after G6 that are not blockers: `GrowthFactor` 2 with its tests and the
  proposal's text, the per-node initiated-session cap, the stale catch-up hint, a test-settable
  `LargeDiff` (with the AC-B2 fix), and caches for mixed radii (T6).
- **(T4.6 part 3 fix round 2) Scope, tests and wall times.**
  - Documentation: the entry above and the Follow-ups below. The only code change is the comment,
    one table label and the log line of `TestG6BootstrapPeers` in `neighborhood_g6_sim_test.go`
    (`-tags=sim`). No production change, no constant, no new measurement; no `pkg/node`, `pkg/p2p`,
    `go.mod`, proto or `*_fuzz_test.go` change.
  - `make build`, `GOTOOLCHAIN=go1.26.1 make lint`, `golangci-lint run --build-tags sim
    ./pkg/rangesync/...` and `go vet` (default and `-tags=sim`) are clean; `go test
    ./pkg/rangesync/...` passes (`pkg/rangesync` 10 s); with `-tags=sim`,
    `TestNeighborhoodG6Figures` and `TestG6EqualCounts` pass (3 s). gofumpt ran on the changed test
    file only. No test's behavior changed, so nothing is new for `-race`.
- **(Orchestrator, G6, 2026-10-21) G6 decided by the user.**
  - Constants: the "G6 decision sheet (final)" as recommended — `GrowthFactor` 4 → 2 (+ the four test
    updates and the proposal text), a per-node initiated-session cap, `LargeDiff` test-settable; caches
    4096/512/32, `BatchFactor` 1.6, `CatchUpMinDiff` 256, `RoundInterval` 5–30 s, request-driven `More`
    and hint(R) unchanged. Applied in T4.7, before T6.1.
  - T4.6 part 3 landed after two unclean fix rounds with the round-2 review notes appended to the AC-B2
    blocker (user's choice); T4.6 ticked.
  - Next: T4.7 hardening (AC-B2 first, then G6 constants, stripe-registry pin, per-peer fetch credit),
    then Phase 6. P8: Phase 5 after Phase 6, before mainnet. P9: the earlier proposal committed as
    history on the docs branch.

- **(T4.7 part 1) AC-B2: the lacking side declines (candidate A), as built.**
  - The rule. `Decide` (session.go) tests the large difference second, after `DecisionEqual` and
    before the count-0 branches, by direction: `DecisionStripe` when the range is splittable and
    `cR − cL >` the threshold, in either role. The threshold is `LargeDiff`, or
    `Config.LargeDiff` (tests), or 0 where `DecisionInput.Transferring` (below). The lacking side
    offers the range to its `Striper`; taken over, the session emits nothing for it (a responder
    whose every range is taken sends the empty Message, which ends the initiator's session with
    the range unanswered, equal at its end: legal on the frozen wire); declined, or with no
    `Striper`, the range is decided by the rest of the rule (`decideSmall`), the pre-T4.7 rule
    without its stripe branch. The holder never stripes: its old initiator-only, direction-blind
    branch is gone, so no peer registers a range it holds (constraint (a): its count-0
    `DecisionSendItems` stays and serves the stripe sessions, which have no `Striper`).
    `DecisionInput.Initiator` is no longer read by `Decide` (kept, documented).
  - Responders get a `Striper`: `rangesync.Options.Striper` (served sessions; `ReconcileWith`
    keeps its per-session one), wired in pkg/node by `rangesyncOptions` (node.go ~L1208,
    `TestRangesyncOptions`). The reconciler implements it (`Reconciler.Stripe`): it takes every
    offered range over and registers it in the same stripe registry as the initiator path
    (`registerStripeLocked`: non-overlap, cap 1024, trimming), except while the bootstrap plan
    holds it, that is while a bootstrap is queued or in flight (`bootstrapping`, and the registry
    covers it anyway) and while `IsBootstrap` holds before the next round registers the root (a
    registered range inside the root would keep `planCatchUp` from registering the bootstrap).
  - Constraint (d): `LacksSessionRoot` (stripes.go, next to `IsBootstrap`) compares the two counts
    of a peer's session root: this node's (its Status, or `RangeSummary` when the peer's root lies
    inside its own) and the peer's (its Status, or its count spread evenly over its root when
    this node's root lies inside the peer's: shifted right by the depth difference). A round that
    finds this node more than the threshold behind registers the session root (a session job; one
    already registered is not registered again) and opens no round session with that peer while
    the check holds; a radius decrease's old-half opening skips the check. It covers the case A does not: a node that opens a round session at
    a count far below the peer's gets the stripe session's answer, a listing.
  - Decision (orchestrator point 4): **in a range a registered transfer covers, the threshold is
    0.** `rangesync.TransferStriper` (a `Striper` with `Transferring(r)`) sets
    `DecisionInput.Transferring`; the reconciler (registry containment) and the `StripeCollector`
    of its round sessions implement it, and `lacksSessionRootLocked` uses 0 for a covered session
    root. Measured (`TestG6BootstrapTail`, below): with `LargeDiff` alone the tail of a transfer
    of the default fetch rate's duration, where node 0's difference is below `LargeDiff`, still
    made each lower-overlay peer list what node 0 lacked every round (536–1703 wanted IDs, 86–116
    in the largest session, 0.03–0.11× of the lacked IDs at ranks 1–4 of N = 5); at 2^22 that is
    2.1–6.8 × `LargeDiff` IDs, ~10–31 MB at N = 5, and it grows with the rank (each lower-overlay
    peer lists independently, the takeovers add), so at N = 17 it would pass AC-B2's ~46 MB
    headroom on its own (an estimate). With the rule the tail lists nothing (0 wanted IDs at every
    rank).
  - Deviations. (1) `LargeDiff` test-settable lands here, not in part 2 (orchestrator decision 5):
    `rangesync.Options.LargeDiff` and `reconciler.Options.LargeDiff` (both zero → 65536; the node
    sets neither), `Config.LargeDiff`, `DecisionInput.LargeDiff`; part 2 only confirms it. (2)
    The zero threshold inside a running transfer is beyond candidate A's text (point 4). (3) §6.3's
    normative order changes (Follow-ups lists the text).
  - Small differences wait (review note 5). While a node declines a range, the new uploads and
    stale-stamp (§7.7) differences in it wait too: in a bootstrap for the whole transfer (~70
    minutes at 2^22: A alone defers them while the node's difference passes `LargeDiff`, the zero
    threshold over the tail as well), then for the verification round and the next rounds; the stripe sessions themselves reconcile their ranges (an upload that
    lands before a stripe's session opens is transferred with it). A transfer that ends lifts the
    rule. A peer claiming a far larger count can make a node decline (Follow-ups, the registry
    pin).
  - The stripe-registry `SyncRate` pin (part 3, after this part, user's order). Part 1 widens the
    gap temporarily: a peer claiming a far larger count now also makes a responder register
    (`Reconciler.Stripe`), and a peer's Status alone makes the rounds register its session root
    (`LacksSessionRoot`); both register the same jobs as the initiator path, so part 3's fix (hold
    for a stripe only while its job stores chunks, past a grace) covers them and closes the gap
    before Phase 6 (Follow-ups).
  - Scaled acceptance (review note 6). `LargeDiff` = Base/64 keeps 2^22's ratio (64) of the root
    difference to `LargeDiff`: stripes of Base/4 IDs (16 × `LargeDiff`) past it, level-1 ranges
    (4 ×) past it while node 0 lacks more than a quarter of its root. The bound is AC-B2 scaled,
    1.15 × Lag × 70 B + 1 MB, as `TestG6BootstrapPeers` reads it; "÷" is node 0's
    `bytes_metadata{dir="in"}` over Lag × 70 B, and the fixed per-session costs (Fingerprints of
    the declined sessions, Status, verification) do not scale with Base, so they weigh far more
    here than at 2^22 (~0.1–0.6 MB there).
    - `TestG6BootstrapScaled` (`-tags=sim`; N = 5, node 0 empty, Delay 50 ms, seeds 1–4; "lifted":
      the fetch limit lifted as in `TestG6BootstrapPeers`, "2 min": a node-wide fetch rate on the
      clock of Base/120 per second): every run passes `check` (stripes done, none timed out; node
      0 within the bound; no peer stripe session; no duplicate):

      | Base (bound) | fetch | ranks | node 0 metadata in, MB | ÷ | peers' sessions with node 0 (takeovers) | wanted IDs | caught up s |
      |---|---|---|---|---|---|---|---|
      | 70000 (6.64 MB) | lifted | 3, 1, 1, 4 | 4.908–4.909 | 1.002 | 8–21 (0–3) | 2–6 | 15.8 |
      | 70000 | 2 min | 3, 1, 1, 4 | 4.909–4.913 | 1.002–1.003 | 47–99 (0–23) | 0 | 120.7–122.0 |
      | 100000 (9.05 MB) | lifted | 2, 2, 4, 1 | 7.011–7.012 | 1.002 | 9–24 (0–3) | 1–9 | 21.8–22.0 |
      | 100000 | 2 min | 2, 2, 4, 1 | 7.012–7.015 | 1.002 | 48–100 (0–23) | 0 | 120.7–121.9 |

      Against G6 (`TestG6BootstrapPeers`, unscaled): 1.43–3.78× at Base 70000, 3.32–3.34× at
      100000. Takeovers fire at both Bases in both modes at every rank below 4 (rank 4 has no
      higher-overlay peer): 1–3 with the fetch limit lifted (the transfer takes 15.8–22 s of the
      clock), 8–23 with the 2-minute transfer. The wanted IDs of the lifted runs are the 4 uploads,
      pushed after the transfer and listed by more than one peer. 1508 s for the 16 runs (72–80 s
      at Base 70000, 104–113 s at 100000), peak RSS 746 MB.
    - Default build, `neighborhood_bootstrap_test.go` (Base 8000, `LargeDiff` 125, the 2-minute
      transfer; ~10–19 s each, run in parallel): `TestNeighborhoodBootstrapScaled` (rank 4:
      1.01×, 100 peer sessions, 0 wanted); `TestNeighborhoodBootstrapResponderOnly` (node 0 at the
      highest overlay, responder of every pair, lacks a level-1 range of 500 IDs, 4 × `LargeDiff`,
      holding 15/16 of the reserve, so not bootstrapping: it declines, transfers the root striped
      and converges; 67–73 KB against 1.04 MB, the stripes' Fingerprints weighing against 35 KB of
      IDs); `TestNeighborhoodBootstrapEdgePeer` (constraint (c): an edge peer at radius 1 among the
      peers of an empty node: its round sessions run, its uploads converge, it registers nothing;
      1.02–1.23×, the spread from the plan's peer order, Follow-ups);
      `TestNeighborhoodBootstrapForeignRoot` (constraint (d), node 0 at the lowest overlay so its
      own rounds decide: its root contains the peers' at radius 1, 1.00×; its root inside theirs,
      the estimate path, 1.00–1.08×).
    - The tests catch the old behavior: without the responder `Striper` the responder-only node
      read 4.03× and ran no stripe, and the Base 16000 bootstrap with a 2-minute transfer 53.55×
      (rank 1, 45 takeovers); without the session-root check the foreign-root cases read
      5.14–5.36×.
    - `TestG6BootstrapTail` (`-tags=sim`; Base 16000, `LargeDiff` 250, a fetch rate on the clock of
      3 per second, so the transfer takes ~89 minutes and its tail ~83 s, as 2^22's ~70 minutes and
      ~65 s at 1000 per second; `StripeTimeout` lifted to 3 h, see the stripe-restart finding in
      Follow-ups; seeds 1, 2 and node 0 at the highest overlay):

      | rank | ÷ before the zero threshold (wanted IDs, largest) | ÷ with it (wanted) | peers' bytes with it |
      |---|---|---|---|
      | 1 | 1.16 (536, 86) | 1.10 (0) | 93 KB, 2120 declined sessions (1053 takeovers) |
      | 2 | 1.26 (1703, 116) | 1.13 (0) | 125 KB, 2837 (703) |
      | 4 | 1.27 (1345, 86) | 1.18 (0) | 188 KB, 4270 (0) |

      What remains above 1.0 is the Fingerprint of each declined session (~44 B), 0.08–0.17× here
      and ~0.6 MB at 2^22 with 16 initiators. 151–155 s per run.
  - G6 re-runs (unchanged tests, production `LargeDiff` 65536; `-tags=sim`, 796 s, peak RSS 586
    MB): `TestG6BootstrapPeers` (N = 5, node 0 empty, fetch limit lifted) reads 4.908–4.909 MB at
    Base 70000, ranks 1, 1, 3, 4: 0.74 of the 6.64 MB bound, 1.002× of the lacked IDs (G6:
    9.47–25.06 MB, 1.43–3.78× the bound); and 7.011–7.012 MB at Base 100000, ranks 1, 2, 2, 4: 0.77
    of the 9.05 MB bound, 1.002× (G6: 30.05–30.22 MB, 3.32–3.34× the bound); the peers' sessions
    bring 0–1 KB and 0–3 wanted IDs each. With the production threshold the root difference
    passes `LargeDiff` only for the first 4,464 (Base 70000) or 34,464 (100000) chunks; the zero
    threshold inside the bootstrap transfer declines the rest, so both Bases now read the stripes
    alone. `TestG6Bootstrap` (N = 17, Base
    20000, below `LargeDiff` throughout): 1.412 and 1.409 MB at ranks 4 and 14 (bound 2.61 MB;
    1.009× and 1.006× of the lacked IDs; G6: 4.2–5.7 and 12.5–14.0 MB), all 16 stripes done, none
    reassigned or timed out, no duplicate.
  - Code: session.go (`Decide`, `decideSmall`, `TransferStriper`, `Config.LargeDiff`,
    `DecisionInput.LargeDiff`/`Transferring`, the decide fallback), rangesync.go
    (`Options.Striper`, `Options.LargeDiff`), reconcile.go (`sessionConfig`), constants.go
    (comments), reconciler: stripes.go (`LacksSessionRoot`, `StripeCollector.Transferring`),
    stripes_transfer.go (`Reconciler.Stripe`, `Transferring`), scheduler.go (the session-root check
    in `scheduleLocked`, `lacksSessionRootLocked`), reconciler.go (`Options.LargeDiff`); pkg/node:
    syncer.go (`rangesyncOptions`), node.go (one call). Harness (neighborhood_sim_test.go):
    `LargeDiff`, `Radius`, `Overlay`, `ChunkRoot`, `LagRoot`, `FetchClockRate`, `simSpan.Stripe`,
    per-root convergence, the responder `Striper`; the default runs draw the same random numbers.
    No wire, proto, pkg/pullsync, pkg/puller, go.mod or `*_fuzz_test.go` change; no new parser of
    peer input (one line in followup-fuzzing.md for the session target).
  - Checks: `make build`, `go vet` (default and `-tags=sim`) on pkg/rangesync/... and pkg/node/...,
    `GOTOOLCHAIN=go1.26.1 golangci-lint run ./pkg/rangesync/... ./pkg/node/...` and with
    `--build-tags sim` are clean; gofumpt ran on the changed files only (gci finds nothing in
    them; node.go's import order is untouched, as before). `go test ./pkg/rangesync/...` passes
    (`pkg/rangesync` ~28 s, of it the five new harness runs ~15 s in parallel), `go test
    ./pkg/node/...` (with `TestCombinedSyncerBothDrivers`) and `go test ./pkg/pullsync/...
    ./pkg/puller/... ./pkg/storer/...` pass; with the defaults (`--rangesync-enable=false`) the
    node builds no rangesync service, so `rangesyncOptions` is never called (`TestSyncWiring`).
    No `-race` on this host: the new and changed tests ran with `-count=3 -cpu=1,2,4` (the
    harness tests `-count=1 -cpu=1,2,4`); the CI list is in Follow-ups. The `-tags=sim` runs
    above are one at a time.
- **(T4.7 part 1 fix round 1) Review findings: a peer's claim lowers no threshold; the stripe clock
  is an inactivity timeout; the bootstrap tests assert their claims.** Commits 3992a351 (code and
  unit tests), f325a8a7 (harness tests) and 002c6460 (own transfers past the registry bound). The
  review's findings, in its order:
  - *Medium 1: a peer's claim and this node's own transfers.* A registered range is now one of two
    kinds (`stripeJob.own`). Own: the bootstrap of the root (`planCatchUp`: `IsBootstrap`, against
    the median of the peers' counts) and a radius decrease's new half (`enqueueRadiusHalf`). A
    claim: anything a peer's count put there, a range a session's Striper took over (served:
    `Reconciler.ServedStriper`; opened: the round session's `StripeCollector` through
    `enqueueStripes`) or a session root the rounds' check found far behind the peer's Status
    (`LacksSessionRoot`); `stripeJob.peer` is that peer. Four rules follow:
    - only own jobs set the threshold to 0 (`Transferring`, its served, collector and
      `lacksSessionRootLocked` users alike), so a liar's claim stops neither the round sessions
      with the other peers nor their small differences (pushsync misses, §7.7) in the claimed range;
      the claimed range moves through the claim's stripes and the rounds both, as before T4.7;
    - claims are capped: a session registers at most `maxSessionStripes` = 1 range, the others it
      takes over are dropped (taken over all the same, offered again by a later session), and a
      peer holds at most `maxPeerStripes` = 2 claims in the registry, trimmed parts counted one by
      one (dropped past it, `stripes_total{dropped}`). One opening of 256 ranges registers one;
    - an own job is not refused for a claim in its way: registering it drops the claims that
      overlap it, ends the claim in flight whatever its range (the one stripe worker would run it
      first, as long as its peers keep it going: `stripeJob.cancel`, set by the worker), and queues
      it before every claim (`addStripeLocked`); nor is it dropped at `maxStripeRegistry`, which
      now bounds the claims (the own jobs, at most a bootstrap and a radius half, come on top).
      Only another own job in its way refuses it, as before (`TestRoundBootstrapOverlap`,
      `TestRadiusHalfOverlapRetried`; `HoldStripe` now holds an own job);
    - to know the peer, `rangesync.Options.Striper` becomes a per-session factory
      (`func(peer swarm.Address) Striper`, called once per served session; pkg/node wires
      `r.ServedStriper`); the reconciler no longer is a Striper itself.

    This supersedes, in "(T4.7 part 1) AC-B2: the lacking side declines (candidate A), as built":
    "in a range a registered transfer covers, the threshold is 0" and "`lacksSessionRootLocked`
    uses 0 for a covered session root" (own transfers only, and over their union, below), "it takes
    every offered range over and registers it ... except while the bootstrap plan holds it" (capped
    claims), and, in `registerStripeLocked`'s rules, "a bootstrap or radius job that contains
    registered ranges is not registered" (own ranges only). Tests: `TestClaimKeepsRounds` (real
    services: a claim of node 0's whole root, registered and in flight for the whole test, and node
    0 still syncs the 3 chunks an honest peer holds beyond it through round sessions, as initiator
    and as responder; with claims counted in `Transferring` both subtests time out),
    `TestClaimCaps`, `TestOwnJobPreemptsClaims` (bootstrap: claims inside the root dropped, the one
    in flight ended, the threshold 0; radius: the overlapping claim dropped, the one in flight
    ended, the other claim queued behind the half; a registry full of claims: the half registered
    on top, queued first), `TestTransferring` (rewritten: own transfers
    and their union cover, a claim covers nothing, a round runs over a claimed root),
    `TestServedStripeRegisters` (through `ServedStriper`), `TestReconcileServedStriper` (one
    Striper per served session, for its peer), `TestRangesyncOptions`. The own bootstrap's tail
    still reads threshold 0: no peer lists node 0 anything it lacked (`checkDeclined`, Low 1
    below, in every bootstrap run, `TestG6BootstrapTail` included).

    The cost falls on the two cases whose transfer is a claim: the responder-only lacking node and
    a bootstrapping node whose root no neighbor shares (constraint (d)). Their tails, below
    `LargeDiff`, are reconciled by the sessions again, as candidate A's text expected (up to
    `LargeDiff` × 70 B ≈ 4.6 MB per session at 2^22): `TestNeighborhoodBootstrapResponderOnly`
    reads 67 KB (1.91 × of 35 KB lacked, as in part 1), `TestNeighborhoodBootstrapForeignRoot`
    1.00× and 1.08× (as in part 1); at 2^22 not measured (Follow-ups).
  - *The StripeTimeout relisting (Follow-ups, the PRE-T6.1 finding).* `stripeClock` is an
    inactivity timeout: it expires once it has run `StripeTimeout` since the attempt's fetches
    last stored a chunk (`progress`: `FetchWithProgress`'s stored callback through `fetchFrom`'s
    new `progress` argument, or the fetch's return with chunks stored without a
    `progressFetcher`), or since the attempt started. Its cap is what is left of the peer's budget,
    `stripePeerBudget` = 1 + `maxStripeRestarts` = 9 `StripeTimeout`s on the clock for all the
    attempts of one stripe with one peer (`runStripe` sums `StripeResult.clock`); an attempt that
    ran out of it (`capped`) is reassigned, not restarted, so a peer that drips a real chunk now
    and then holds a stripe no longer than the 9 attempts of 15 minutes did (2.25 h per peer, 9 h
    per stripe). This narrows the orchestrator's "a hard cap of (1 + `maxStripeRestarts`) ×
    `StripeTimeout` per attempt": with a cap per attempt only, a peer that drips and stalls just
    before the cap would be restarted 8 times, up to ~20 h; with the budget per peer each attempt's
    cap is that product less what the peer's earlier attempts ran. A verification is one attempt
    with the whole budget as its cap. Kept: the clock
    pauses for the peer's Busy with no fetch running and runs through this node's Busy, on the
    injected clock, and `expireIfDue` runs after every timed wait; new, it also runs before each
    listed batch is fetched, and a batch is no longer fetched once the attempt's context ended (so
    the timeout is exact on a fake clock: the fetch goroutine starts no fetch past it). This
    supersedes T4.3 fix round 3's "the stripe timeout bounds the fetch after the session again" in
    its bound (15 minutes of the attempt became 15 minutes of no chunk stored, capped by the
    budget), and T4.5 part 2a's "*The bound* ... 36 attempts of `StripeTimeout` on the stripe
    clock (9 h)" (now: at most 36 attempts, and at most `stripePeerBudget` × `StripeTimeout` on the
    clock per peer, 9 h per stripe; 12 attempts held up only by this node, each ending at 15
    minutes of inactivity, 3 h) and its "The attempt calls `expireIfDue` after each session answer
    and each wait" (and before each fetch). A restart still lists the remainder again (it opens at
    the node's count), but only after a peer stored nothing for `StripeTimeout` or lost its whole
    budget. Tests: `TestStripeInactivityTimeout` (fake clock, a stripe of peers A then B: A
    delivering one chunk per fetch at 0.9 timeout apart finishes at 5.4 timeouts in one session;
    A dripping without end is reassigned to B after 9 timeouts, one session with A; A stalling is
    reassigned after one timeout; with `progress` a no-op "slow" goes to B and "drip" runs 5
    sessions with A) and `TestNeighborhoodBootstrapStripeTimeout` (Low 1 below).
  - *Low 1: the tests assert their headlines.* The harness records the distinct IDs a node wanted
    from the sessions it served (`simNodeResult.ServedWants`) and how many of them are lagging base
    chunks (`ServedLagging`). `bootstrapFigures.checkDeclined`: the peers of a node that
    bootstraps by `IsBootstrap` list it no ID it lacked, and no more distinct IDs than the uploads.
    Distinct, not the per-session sum `wants`: an upload pushed late is listed by each peer whose
    session finds node 0 without it (part 1's lifted runs read 2–9 for 4 uploads). `atRank(k)`
    places node 0 at rank k (its first byte 0x80, k peers below, the others above).
    `TestG6BootstrapScaled` runs ranks 1–4 at both Bases (seed = rank), checks `check` and
    `checkDeclined` in every run, and asserts takeovers in the 2-minute runs at ranks 1–3 at both
    Bases; `TestG6BootstrapTail` runs the default `StripeTimeout` (seeds 1, 2 and highest; one
    3 h row kept for comparison) and checks `check`, `checkDeclined` and zero wants in every run;
    `TestNeighborhoodBootstrapScaled` (default build) runs at rank 2 and asserts takeovers (rank 2,
    16 takeovers, 1.01×, no ID listed); new `TestNeighborhoodBootstrapStripeTimeout` (default
    build, rank 1, the 2-minute transfer, `StripeTimeout` 30 s, so each stripe outlasts it about 4
    times): `check`, `checkDeclined`, at most two completed stripe sessions per stripe (transfer and
    verification); 564 KB (1.01×) of the 1644 KB bound, 4 stripes done, none timed out. With the old
    clock (`progress` a no-op): 3446 KB (6.15×, over the bound), 250 duplicate deliveries, 26
    completed stripe sessions for 4 stripes, caught up after 191 s instead of 122 s.
  - *Low 2: `Transferring` over the union of the own transfers.* A range is covered when the own
    ranges cover it together (`trimStripe` with no room left: no uncovered part), not only one
    containing it; claims never count. In practice the own transfers are one bootstrap root or one
    radius half, and both at once only after a radius decrease during a bootstrap, so the union
    matters there. Not closed: a range only partly inside the own transfers (the new root over a
    radius half in its tail) keeps the session's threshold. Declining it would decline its other
    part too, and splitting it does not help: the frozen wire makes the splitting side send all
    `R.Children(FanoutBits)` in one Message (§6.4, `Handle`'s rules), and the holder decides on
    them, so a child inside the half that node 0 still lacks entirely is listed. At 2^22 the
    root's difference passes below `LargeDiff` only once node 0 holds over 97% of the half, when
    no child is empty, so what is listed is what the symbols or items of a small difference cost
    (up to `LargeDiff` IDs per session); not measured.
  - *Low 3:* `recheckSession` reruns the session-root check (`lacksLocked`) for a session over the
    session root (no Open): a session that waited for a slot no longer opens once this node is far
    behind the peer, or once its own transfer of the root was registered and the peer holds more.
    The round registers the claim, as before. `TestSessionRecheckAfterSlot` adds both cases (each
    fails without the check).
  - *Low 4:* the `Striper` and `Config.Striper` docs (session.go) say "more than the threshold
    (LargeDiff, or 0 where Transferring)". The proposal-text follow-up's "§6.3 L834 (constants)" is
    L830 (Follow-ups).
  - Checks: `go build ./...`; `go vet` on pkg/rangesync/... and pkg/node/..., and with `-tags=sim`
    on pkg/rangesync/...; `GOTOOLCHAIN=go1.26.1 make lint`, and `golangci-lint run --build-tags sim
    ./pkg/rangesync/...`: 0 issues; gofumpt and gci on the changed files only (no change). `go
    test ./pkg/rangesync/... ./pkg/node/...` pass (`pkg/rangesync` ~29 s);
    `go test ./pkg/pullsync/... ./pkg/puller/... ./pkg/storer/...` and pkg/node
    (`TestCombinedSyncerBothDrivers`, `TestSyncWiring`) pass (pkg/node changed one line of
    `rangesyncOptions`, wiring only; the defaults build no rangesync service). No `-race`: the
    reconciler package ran `-count=3 -cpu=1,2,4` nine times (one failure of
    `TestSyncRateBootstrap`, not reproduced in 90 runs of it alone, nor in six base-code runs of
    the package; Follow-ups), the changed pkg/rangesync tests `-count=2 -cpu=1,2,4`. No wire, proto,
    pkg/pullsync, pkg/puller, go.mod or `*_fuzz_test.go` change; one line in followup-fuzzing.md.
  - Scaled acceptance re-run (`-tags=sim`, one at a time, 2189 s, peak RSS 710 MB; every run
    passes `check` and `checkDeclined`; at f325a8a7, before 002c6460, which changes only an own
    job's registration in a full registry):
    - `TestG6BootstrapScaled` (N = 5, node 0 empty, `LargeDiff` Base/64, Delay 50 ms, ranks 1–4 by
      `atRank`): node 0 reads 4.908–4.913 MB at Base 70000 (bound 6.64 MB) and 7.010–7.014 MB at
      100000 (bound 9.05 MB), 1.001–1.003× of the lacked IDs at every rank in both modes, as part 1
      (1.002–1.003×). Takeovers, ranks 1 / 2 / 3 / 4: "2 min" 24 / 15 / 7 / 0 at Base 70000 and
      23 / 15 / 8 / 0 at 100000; "lifted" 3 / 2 / 1 / 0 at both. The peers' sessions listed node 0
      no ID it lacked; their wants sum to 0 in the "2 min" runs and 1–7 in the "lifted" ones, 1–3
      distinct IDs, all uploads (4 per run). No stripe timed out, no duplicate. 1561 s.
    - `TestG6BootstrapTail` (Base 16000, `LargeDiff` 250, 3 per second on the clock, the transfer
      ~89 minutes, stripes ~6 × `StripeTimeout`), rank (seed), the default `StripeTimeout`: 1 (1)
      1232 KB of 2288 KB, 1.100×; 2 (2) 1263 KB, 1.128×; 4 (1, highest) 1326 KB, 1.184×; caught up
      after 5334 s (part 1 with the default timeout: 9.34–9.45×, 38–40 stripe sessions for 4
      stripes, ~129 minutes), 9–12 stripe sessions (4 transfers, 4 verifications, Busy retries),
      none timed out, 0 wants, no duplicate. The 3 h row reads the same as the default one (seed 1:
      1232 KB, 1.100×), and as part 1's lifted rows (1.10, 1.13, 1.18×): the inactivity timeout
      costs nothing over lifting the timeout. 628 s.
  - The rest of the `-tags=sim` suite (the verifier's gap), after the fix, one at a time (2895 s,
    then 32 s; peak RSS 1.36 GB, sampled): every test passes, and no figure moved outside its seed
    spread (the steady and catch-up runs register no stripe, so the fix changes nothing there).
    `TestG6Constants` (308 s): shares, hint on, at 50 / 100 / 200 ms 77.2 / 76.1 / 71.9% at N = 9
    and 61.0 / 62.3 / 64.9% at N = 17 (G6: 77.5 / 76.4 / 72.3 and 61.7 / 61.6 / 65.6, sd 2–3
    points), convergence 3.40 s / 5.90 s and 3.73 s / 6.45 s (G6 3.39 / 5.90 and 3.73 / 6.45).
    `TestG6Load` (235 s): n_Δ p50 / p99 / max within the seeds' noise of the G6 Q2 table (N = 9,
    15.4/s: 40 / 55 / 58 against 40 / 54 / 58; N = 17, 100/s: 174 / 360 / 488 against 176 / 365 /
    484), no stream exhausted; sides ≥ `CatchUpMinDiff` at N = 17, 100/s: 15 (G6: 7 in one run;
    max wants 393 against 400). `TestG6RoundInterval` (441 s): the sparse runs' longest 10.75 and
    10.95 s (G6: 10.75–10.95). `TestG6Catchup` (306 s): caught up 0.85 / 1.08 / 1.80 / 2.62 / 4.03
    / 6.75 s at Lag 300–12000 (G6 0.85 / 1.08 / 1.85 / 2.62 / 3.97 / 6.90), symbols the same,
    exhausted 68 and 349 (68, 350). `TestG6Bootstrap` (32 s): 1411 and 1410 KB at ranks 4 and 14
    (part 1: 1412, 1409), 16 stripes done, none reassigned or timed out. `TestG6BootstrapPeers`
    (760 s): 4908–4909 KB and 7011–7012 KB (part 1 the same). `TestG6EqualCounts` (1 s): the G6
    GF4 column exactly. `TestG6FetchOpenFails` (19 s): 3.41 s, 75.7% (G6 3.41 s, 75.6%).
    `TestSREPModelVsHarness` (190 s): its record check passes (every live − recorded within its
    bound); `TestSREPModelG6` (64 s), `TestSREPModelG6Part3` (536 s). Then `go test -tags=sim
    ./pkg/rangesync/... -skip 'TestG6|TestSREPModelVsHarness|TestSREPModelG6'`: the other 1316
    tests and subtests (`TestNeighborhood*` included) pass.
- **(T4.7 part 1 fix round 2) A registry full of claims does not hold a radius half back;
  `TestSyncRateRadiusHalfRegistryFull` rewritten.** The verifier's failure: the test timed out
  "waiting for decrease seen" (21 / 18 / 13 of 30 runs alone at `-cpu` 1 / 2 / 4, none on
  rangesync/phase-4; bisected to 002c6460).
  - *Cause (the verifier's reading, confirmed).* Since 002c6460 an own job is registered on top of a
    full registry, and since 3992a351 `HoldStripe` holds own jobs. The test's 1024 held ranges lie in
    the old half, so they neither overlap nor contain the new half: the round after the decrease
    registers it (1025 ranges; read with the fetches gated) and, with nothing gating its fetches,
    transfers and verifies it within about 10 ms. In 43 of 90 runs here `RadiusPending` was first
    polled after the half had ended; the other runs passed without the half ever waiting (it ended
    within the 100 ms `settle`, before the "blocked half registered" check). So in no run did the
    test test a half that a full registry held back. Fix round 1 did not see it: its nine package
    stress runs ran before 002c6460, and after it only `TestOwnJobPreemptsClaims`,
    `TestEnqueueStripes*` and `TestClaimCaps` were run again.
  - *Decision: the code is right; the test changes.* Since fix round 1 everything in the registry
    besides the own jobs is a peer's claim. If a registry full of claims could drop the half, peers
    whose claims fill it would keep the half unregistered for `radiusHold` (10 minutes). After that
    `SyncRate` reads 0, and the reserve worker can lower the radius again before the half is
    transferred. That is the dependence on peers' claims that fix round 1 removed. The registry stays
    bounded by this node's own state. A claim is registered only while fewer than
    `maxStripeRegistry` ranges are registered, own ones counted. Own jobs come from two places
    only, each guarded by a flag that is set when the job is registered and cleared, under the same
    lock, when the worker removes it: `planCatchUp` (`r.bootstrapping`) and `enqueueRadiusHalf`
    (`r.radius.striping`). An own job is never trimmed into parts. So the registry holds at most
    `maxStripeRegistry` + 2 = 1026 ranges. The only code change is to two comments in radius.go:
    `radiusHold`'s list of reasons no longer has "full registry", and its overlap, like the refusal
    comment in `enqueueRadiusHalf`, is with another own transfer.
  - *The test.* `TestSyncRateRadiusHalfRegistryFull` fills the registry with `MaxStripeRegistry`
    claims inside the old half (new export_test helper `HoldClaims`: claims of no peer, registered,
    not queued), gates the fetches and lowers the root. The half is then registered on top (1025
    ranges), a further claim is refused and the registry stays at 1025, and the half's stripe
    sessions open. With the claims released and the clock past the window (nothing stored, the
    catch-up grace spent), `SyncRate` is ≥ `StartupGuardRate` from the half's transfer alone. Once
    the gate opens the half is done (a stripe done, nothing registered, the half no longer pending),
    and past the window `SyncRate` reads 0. The test keeps the old one's intent: the rate is held
    while the half runs, and the half is transferred. The waiting itself stays pinned by
    `TestSyncRateRadiusHalfUncovered` and `TestSyncRateRadiusHalfOverlap`. Without 002c6460's rule
    (own `room = 1` reverted) the test times out "waiting for half registered", as
    `TestOwnJobPreemptsClaims/full registry` fails. 300 of 300 runs alone (`-count=100 -cpu=1,2,4`)
    pass.
  - *Superseded:* in "(T3.5 review) Radius decrease before registration" (phase 3), "full registry"
    among the reasons a pending half is not registered: since 002c6460 a registry of claims never
    refuses it, and, with fix round 1's preemption, "overlap with a registered range" means an own
    range. Also its "Tested per no-op path
    (`TestSyncRateRadiusHalf{Uncovered,Overlap,RegistryFull}`)": RegistryFull now tests the
    registered-on-top path, and the no-op paths are Uncovered and Overlap.
    This narrows "(T3.2 part 2 review) Bounds", "The registry holds at most `maxStripeRegistry =
    1024` ranges; past it a range is dropped", to the claims, own ones counted. Up to two own
    transfers go on top (1026), and none is dropped. `radiusHold`, `maxRadiusAttempts` and the
    SyncRate rule of T3.5 are unchanged.
  - *The fix-round-1 `TestSyncRateBootstrap` failure* was that test's own (`syncrate_test.go:45`,
    "sync rate 0 during the bootstrap transfer"), in a package run before 002c6460, not this one.
    Its follow-up stands.
  - Checks: `go build ./pkg/rangesync/...`; `go vet` on the reconciler package, also with
    `-tags=sim`; `GOTOOLCHAIN=go1.26.1 make lint`, and `golangci-lint run --build-tags sim
    ./pkg/rangesync/...`: 0 issues; gofumpt and gci on the three changed files only (no change).
    Stress: `go test ./pkg/rangesync/reconciler/ -count=10 -cpu=1,2,4`, run twice, had no failure
    (138 s each), `TestSyncRateBootstrap` included (60 runs); `go test ./pkg/rangesync/ -count=3
    -cpu=1,2,4` had no failure (270 s). No `-race` (no C compiler; Follow-ups). No pkg/pullsync,
    pkg/puller, go.mod, proto or `*_fuzz_test.go` change.
- **(Orchestrator, T4.7 part 1 landing, 2026-10-21) Part 1 landed after two fix rounds.** Review
  round 1: no High; Medium 1 (a lying peer's claim set the threshold to 0 over its range) and four
  Lows, plus the StripeTimeout relisting (AC-B2), all fixed in fix round 1. Fix round 1's verifier
  failed `TestSyncRateRadiusHalfRegistryFull` (the test's premise, not the code; fixed in fix round
  2). The re-review of both fix rounds found no High or Medium; its Lows are in the Follow-ups
  ("(T4.7 part 1 re-review; recorded by the orchestrator at landing)"), the first of them handed to
  part 3. README is ticked only after part 4.
- **(T4.7 part 2) G6 constants, as built.** Commits 7a059f96 (`GrowthFactor` 2), 1f907eeb (the
  per-node initiated-session cap), 9884fdf7 (AC-B1 re-measured: the sim-tagged harness and the
  model's records) and the decisions-and-follow-ups commit.
  - `GrowthFactor` 4 → 2 (constants.go). Its one use is the decoding side's `More`
    (session_symbols.go ~L403), so GF2 and GF4 peers interoperate: no wire or proto change. The
    four tests that encoded 4: `TestProtocolConverges` derives its bound's Mores from
    `GrowthFactor` (`mores`: 10 to `RootSymbols` and 7 to `Level1Symbols`, 2 × 17 Messages; the 4k
    case takes 40 of its 45); `TestWorkedExamples` takes GF2's outcomes for examples 2, 3 and §7.2,
    with the reason in its case list (example 2: 3 Messages and 68 symbols for 6 of 10 draws, 5
    and 136 for 3, 7 and 272 for 1; example 3: 11 Messages and 64 symbols for 4 of 5, 9 and 32 for
    1; §7.2: 3 and 4 for 7 of 10, 5 and 8 for 3); `TestSREPSessionCost`'s "now" band at d = 45 is
    [1.6, 2.2] symbols per difference (1.89 measured), and GF4 keeps a row of its own ([2, 2.8]);
    `TestSREPIdeal` compares the constants with GF4 explicitly (2.15 against 3.11 symbols per
    difference at N = 9, 2.29 against 3.45 at N = 17). With GF4 compiled the updated tests fail
    (checked before the constant changed). The proposal's text is a Follow-up.
  - **The per-node initiated-session cap, the rule** (scheduler.go, `InitiatedSessionCap`). Each
    round counts its eligible peers (exchanged Statuses, `isEligible`, whatever their digest or
    backoff) by overlay, `higher` (this node initiates) and `lower` (they initiate), and sets the
    cap before it starts its sessions:

        cap = max(MinInitiatedSessions, min(higher + MaxStripeSessions, MaxConcurrentSessions − lower))   if higher + lower ≤ MaxConcurrentSessions
        cap = MinInitiatedSessions                                                                     otherwise

    `MinInitiatedSessions` = 8 (`MaxConcurrentSessions` / 2, the constant before G6) is the floor
    and the cap until the first round; `MaxStripeSessions` = 4 is now a constant of its own (not
    derived from the cap). Round sessions, takeovers, stripe transfers and verifications all take
    a slot of it, stripes a stripe slot first, as before. Why each part:
    - Up to N = 17 a node's sessions, one per peer and shared by served and initiated sessions,
      fit its 16 tokens, so no session is refused for them whatever the cap; the cap covers every
      higher-overlay peer (one wave: N = 17 rank k gets 16 − k, every rank of N = 9 at least its 8 −
      k) and, where the tokens left by the lower-overlay peers allow, 4 stripe sessions besides. A
      stripe or a takeover holding a slot can push one round session to a second wave at the
      ranks whose cap is exactly their higher-overlay peers (N = 17, ranks 0–8); it never takes a
      lower-overlay peer's token (one session per peer).
    - Past N = 17 the cap is the floor, which leaves half of the tokens to the sessions the node
      serves (inbound sessions from lower-overlay peers keep room). Measured first instead (the
      decision sheet's wording taken literally): the room above with the reserve capped at 8,
      `max(8, min(higher + 4, 16 − min(lower, 8)))`, which gives the low ranks up to 16 slots past
      N = 17 too. It turned into the Busy storm L4679–4687 warned of (`TestG6InitiatedCap`,
      steady load, 50 ms, 3 seeds, against the constant 8 in the same run): N = 25 Busy 0.363
      against 0.190 per upload, convergence mean 5.54 against 4.17 s, longest 10.95 against 9.05
      s, 10 pairs without a session against 1, share 41.7 against 42.7%; N = 33 Busy 0.690
      against 0.518 and session-limit 0.047 against 0.015 per upload, convergence 5.13 against
      4.24 s, share 38.2 against 36.4%. The higher overlays are token-bound there (a rank above
      16 has more lower-overlay peers than tokens) and more sessions at once from the lowest
      overlays only add Busy answers. So past the tokens the cap stays at the floor: the
      behavior before G6, measured bounded (below).
    - Deviation: a step at N = 17/18, not a slope; N = 18–24 runs the pre-G6 cap (not measured
      separately, Follow-ups).
  - **The mechanism.** `slotPool` (slots.go): a FIFO semaphore whose size `resize` changes; a
    smaller size takes no slot from a holder and lets no waiter in until the slots in use are below
    it; a waiter whose context ends takes none. It replaces the `slots` channel; `acquireSlot`,
    `acquireStripeSlots` and their callers are unchanged. `MaxInitiatedSessions` is gone.
    `reconciler.Options.InitiatedSessions` (and `simOptions.InitiatedSessions`, the model's
    `srepParams.InitiatedSessions`) fixes a constant cap for simulations and slot-waiting tests;
    the node leaves it zero. The model (srep_model_test.go) gives each node the cap of its rank.
  - **`LargeDiff` settable (scope item 3): confirmed, no new work.** Part 1 made it settable
    (`reconciler.Options.LargeDiff`, `rangesync.Options.LargeDiff` → `Config.LargeDiff` /
    `DecisionInput.LargeDiff`); the harness sets both from `simOptions.LargeDiff`
    (neighborhood_sim_test.go ~L665 and ~L683), which `TestG6BootstrapScaled`,
    `TestNeighborhoodBootstrapScaled` and the other scaled bootstrap tests use.
  - Unchanged: caches 4096/512/32, `BatchFactor` 1.6, `CatchUpMinDiff` 256, `RoundInterval` 5–30 s,
    request-driven `More`, hint(R); `MaxConcurrentSessions` 16, so `NodeDecoderMemory` too.
- **(T4.7 part 2) AC-B1 re-measured with the landed constants.** `TestG6Constants` (`-tags=sim`,
  steady load, 6 seeds, the same modeled base N(N − 1) × 102 B as L2693–2738; "8" is the control,
  the landed build with a constant cap of 8, i.e. G6's `GrowthFactor` 2 row; at N = 9 the two caps
  run the same sessions, since no node there has more than 8 higher-overlay peers and the cap is
  at least 8). Range over Delay 50, 100 and 200 ms, Delay 0 in brackets; pass means ≤ 50% at every
  validated Delay:

  | constants | hint | N = 9 | N = 17 | verdict N = 9 | verdict N = 17 |
  |---|---|---|---|---|---|
  | landed (`GrowthFactor` 2, per-node cap, `BatchFactor` 1.6) | on | 59.7–60.5 [59.8] | 39.5–40.3 [47.3] | fail (+9.7 to +10.5) | pass (−9.7 to −10.5) |
  | landed | off | 59.3–59.7 | 39.2–39.5 | fail | pass (−10.5 to −10.8) |
  | control: `GrowthFactor` 2, cap 8 | on | = landed | 48.6–51.7 [50.0] | – | fail at 200 ms (+1.7) |
  | control | off | = landed | 44.3–46.5 | – | pass |

  - Against G6 (L2693–2738): the control reproduces G6's `GrowthFactor` 2 rows (48.3–52.1 [49.0],
    off 44.7–46.2), and the landed rows its scratch "16 initiated slots, `GrowthFactor` 2" rows
    (39.5–40.3 [48.8], off 39.4): the per-node cap gives what the 16-slot build gave, with
    `MaxStripeSessions` back at 4. The expectation held: N = 9 fails in every configuration, N =
    17 passes with GF2 only with the cap.
  - Wire basis (÷ 1.09, part 2): N = 9 54.4–55.5% (still fails), N = 17 36.0–37.0%.
  - Convergence, landed, hint on (mean; longest) at 50 / 100 / 200 ms: N = 9 3.43 / 4.00 / 5.28 s
    (6.10 / 7.00 / 9.00); N = 17 3.47 / 4.16 / 5.60 s (6.05 / 7.50 / 10.00), against 3.76 / 4.59
    / 6.55 s (6.55 / 8.30 / 11.20) with cap 8. Hint off, 50 / 200 ms: N = 9 3.68 / 6.20 s (6.10 /
    9.00), N = 17 3.69 / 6.34 s (6.25 / 10.00). AC-V1 (10 s): the N = 17 longest at 200 ms is now
    10.00 s, at the bound (11.2 s with cap 8).
  - Busy and session-limit answers: none at N = 9 or 17, either cap (opening side and serving
    side); stale rechecks ≤ 0.002 per upload at a Delay (0.058 at Delay 0, N = 17).
  - What the cap removes at N = 17 (50 / 100 / 200 ms, hint on): the second wave's differences,
    n_Δ p99 39–42 against 59–71 and wants max 37–40 against 80–97; symbols per difference
    2.32–2.37 against 2.53–2.58; Mores 1.19–1.28 per upload, round trips 4.70–4.73 per session.
  - Verdict: N = 17 passes AC-B1 with ~10 points to spare at every validated Delay, hint on or
    off; N = 9 fails by ~10 points (55% on the wire), as G6 expected: re-basing at N = 9 is T6.0's
    call (L2732–2735 unchanged).
- **(T4.7 part 2) Past N = 17: bounded Busy, no starved node.** `TestG6InitiatedCap` (`-tags=sim`,
  steady load, Delay 50 ms, 3 seeds; per upload; "served min" the fewest completed sessions per
  lower-overlay peer that a node served, the smallest of the seeds; "unpaired" the pairs with no
  completed session, the most, which converge through the others):

  | N | cap | share % | convergence mean, longest s | sessions | Busy, session limit, Busy given | served min, unpaired |
  |---|---|---|---|---|---|---|
  | 17 | landed | 39.3 | 3.51, 6.25 | 1.74 | 0, 0, 0 | 6.0, 0 |
  | 17 | 8 | 48.4 | 3.78, 6.65 | 1.73 | 0, 0, 0 | 6.0, 0 |
  | 25 | landed | 42.6 | 4.01, 9.25 | 3.60 | 0.174, 0.003, 0.174 | 4.2, 1 |
  | 25 | 8 | 42.9 | 4.25, 10.55 | 3.63 | 0.176, 0.003, 0.176 | 4.1, 3 |
  | 33 | landed | 36.9 | 4.27, 8.55 | 6.29 | 0.517, 0.006, 0.517 | 3.6, 2 |
  | 33 | 8 | 36.5 | 4.48, 9.50 | 6.30 | 0.528, 0.040, 0.528 | 3.8, 3 |

  Past 17 the landed cap is the constant 8 (the rows differ by the seeds' noise): ~5% (N = 25) and
  ~8% (N = 33) of the sessions answered Busy, all by a node's session tokens or a simultaneous
  open, each followed by the T4.2 backoff, and every node serves ≥ 3.6 sessions per lower-overlay
  peer. The default build has `TestNeighborhoodPastTokens` (N = 25, one seed): converges within
  `TestNeighborhoodSteady`'s bound, served min ≥ 1, Busy and session-limit answers ≤ a tenth of
  the completed sessions (80 of 1740 measured).
- **(T4.7 part 2) The model and its harness records.** The model now runs the per-node cap of
  each rank by default; its configurations carry their cap (`srepConfig.slots`, records
  `srepRef.slots`): G6's records are the constant 8 (`slots` 8, unchanged), the compiled
  constants' new records the per-node cap (`slots` 0, `TestSREPModelVsHarness`). At N = 17 and a
  Delay the model matches the harness with the cap (share 39.6 against 39.3% at 50 ms, 39.2
  against 39.9% at 200 ms). Decision: at Delay 0 the compiled constants' share and records are
  logged, not checked. With up to 16 sessions of one instant the harness's Delay-0 runs are
  not repeatable (N = 17, the same 6 seeds: 47.3 ± 3.6% in `TestG6Constants`, then 52.8 ± 2.9%
  and 48.8 ± 3.2% in two `TestSREPModelVsHarness` runs, n_Δ 18–27% above the model's, whose share
  is 38%); Delay 0 was already outside the validated domain (T4.6 part 2 fix round 1) and backs no
  verdict. At a Delay the live harness matches the new records within 0.3 points.
- **(T4.7 part 2) Scope, tests and wall times.**
  - Code: pkg/rangesync/constants.go (`GrowthFactor`), pkg/rangesync/reconciler (scheduler.go,
    reconciler.go, the new slots.go, a comment in stripes_transfer.go) and tests. No pkg/node,
    pkg/pullsync, pkg/puller, go.mod, proto or `*_fuzz_test.go` change; README not ticked.
  - `make build`, `go vet` (pkg/rangesync default and `-tags=sim`, and pkg/node),
    `GOTOOLCHAIN=go1.26.1 golangci-lint run ./pkg/rangesync/...` with and without `--build-tags
    sim` and `GOTOOLCHAIN=go1.26.1 make lint` are clean; `go test ./pkg/rangesync/...` passes
    (`pkg/rangesync` 30 s, reconciler 5 s). gofumpt and gci ran on the changed files only (gci
    flags coexistence_test.go's import order, as at HEAD: left alone).
  - `-tags=sim`, one at a time (4 cores, nothing else running): `TestG6Constants` 403 s (108
    harness runs), `TestG6InitiatedCap` 124 s (twice, one per rule), `TestSREPModelVsHarness` 460 s
    then 183 s (passes), N = 25 runs ~640 MB RSS (sampled).
  - No `-race` here: the new and changed tests ran with `-count=20 -cpu=1,2,4`, the slot pool's
    with `-count=50`, the whole reconciler package with `-count=3 -cpu=1,2,4`, the harness and
    session tests (`TestNeighborhood*` of the default build, `TestProtocolConverges`,
    `TestWorkedExamples`, `TestSREP*`) with `-count=2 -cpu=1,2,4`; the CI list is in Follow-ups.
- **(T4.7 part 2 fix round 1) The per-node cap slides past N = 17: no step.** Review finding
  (Medium): the landed rule stepped from up to 16 slots at N = 17 to 8 at N = 18 (one peer more
  halved a low rank's cap, and a node whose eligible peers sat at 16/17 flipped 16 ↔ 8), and N =
  18–24 ran the cap-8 control unmeasured. Adopted the reviewer's continuous rule
  (`InitiatedSessionCap`, scheduler.go):

      excess = max(0, higher + lower − MaxConcurrentSessions)
      cap    = max(MinInitiatedSessions, min(higher + MaxStripeSessions, MaxConcurrentSessions − lower − excess))

  It is the landed rule while higher + lower ≤ 16 (N ≤ 17) and 8 at every rank from higher +
  lower ≥ 24 (N ≥ 25); between, each peer past the tokens takes a slot off the room: N = 18
  ranks 0–7 get 15 … 8 (the step gave 8), N = 21 ranks 0–4 12 … 8, N = 24 rank 0 9. One peer more
  moves the cap by at most 1 (a higher-overlay peer) or 2 (a lower-overlay one).
  `TestG6InitiatedCap` (`-tags=sim`, steady load, 6 seeds; now N = 17, 18, 21, 25 at Delay 50 and
  200 ms and N = 33 at 50 ms, the per-node cap against the constant 8). "step" is the landed
  rule, measured with the code of 62615390 (the restructured test, before the rule changed);
  "continuous" and "8" are two runs each at N = 18 and 21 (the second after the first, the same
  seeds), one elsewhere. Share of AC-B1's modeled base (N(N − 1) × 102 B per upload), mean ± sd;
  convergence mean and longest; Busy and session-limit answers per upload (opening side; the
  serving side's Busy given matched Busy in every row); served min the fewest sessions per
  lower-overlay peer a node served, unpaired the most pairs with no completed session:

  | N | Delay | cap | share % | convergence mean, longest s | Busy, session limit /upload | served min, unpaired |
  |---|---|---|---|---|---|---|
  | 17 | 50 ms | per node (either rule) | 39.5 ± 0.5 | 3.48, 6.25 | 0, 0 | 6.0, 0 |
  | 17 | 50 ms | 8 | 48.4 ± 0.9 | 3.76, 6.50 | 0, 0 | 6.0, 0 |
  | 17 | 200 ms | per node (either rule) | 40.0 ± 1.0 | 5.52, 9.20 | 0, 0 | 6.0, 0 |
  | 17 | 200 ms | 8 | 52.4 ± 1.1 | 6.49, 11.20 | 0, 0 | 5.0, 0 |
  | 18 | 50 ms | continuous | 45.8 ± 2.1; 46.1 ± 2.4 | 3.69, 6.45; 3.71, 6.55 | 0.024, 0.001; 0.022, 0.001 | 5.6, 0; 5.6, 0 |
  | 18 | 50 ms | step | 49.3 ± 1.7 | 3.86, 6.45 | 0.008, 0 | 5.7, 0 |
  | 18 | 50 ms | 8 | 50.0 ± 1.1; 49.6 ± 1.0 | 3.86, 6.55; 3.87, 6.75 | 0.005, 0; 0.009, 0 | 5.8, 0; 5.8, 0 |
  | 18 | 200 ms | continuous | 46.4 ± 1.1; 46.4 ± 0.7 | 5.45, 9.20; 5.15, 8.40 | 0.062, 0; 0.052, 0 | 6.1, 1; 6.0, 0 |
  | 18 | 200 ms | step | 52.3 ± 0.6 | 5.91, 9.10 | 0.020, 0 | 6.1, 1 |
  | 18 | 200 ms | 8 | 52.7 ± 0.8; 52.9 ± 1.2 | 5.91, 9.10; 5.91, 9.10 | 0.023, 0; 0.012, 0 | 6.2, 0; 6.0, 0 |
  | 21 | 50 ms | continuous | 46.0 ± 1.2; 47.2 ± 1.1 | 4.26, 10.55; 4.05, 10.55 | 0.107, 0.001; 0.086, 0 | 4.5, 2; 5.0, 0 |
  | 21 | 50 ms | step | 46.3 ± 1.2 | 3.93, 9.35 | 0.080, 0 | 4.6, 0 |
  | 21 | 50 ms | 8 | 46.0 ± 1.2; 47.0 ± 1.7 | 4.15, 9.45; 4.01, 10.55 | 0.085, 0.001; 0.074, 0 | 4.7, 2; 4.8, 0 |
  | 21 | 200 ms | continuous | 48.5 ± 0.9; 48.2 ± 1.7 | 6.20, 9.60; 6.14, 9.40 | 0.178, 0.001; 0.189, 0.001 | 4.9, 1; 5.0, 4 |
  | 21 | 200 ms | step | 48.8 ± 1.5 | 6.23, 10.00 | 0.167, 0.002 | 4.8, 2 |
  | 21 | 200 ms | 8 | 48.9 ± 2.1; 49.2 ± 2.0 | 6.12, 10.00; 6.05, 9.10 | 0.148, 0; 0.140, 0.001 | 4.9, 3; 4.8, 1 |
  | 25 | 50 ms | continuous | 42.8 ± 1.5 | 4.35, 10.75 | 0.192, 0.002 | 3.6, 5 |
  | 25 | 50 ms | 8 | 42.3 ± 1.1 | 4.32, 10.65 | 0.179, 0.002 | 3.9, 3 |
  | 25 | 200 ms | continuous | 44.4 ± 1.3 | 6.71, 9.20 | 0.400, 0.004 | 3.8, 5 |
  | 25 | 200 ms | 8 | 44.3 ± 1.1 | 6.54, 9.60 | 0.375, 0.006 | 3.9, 6 |
  | 33 | 50 ms | continuous | 36.1 ± 0.8 | 4.61, 10.95 | 0.548, 0.045 | 3.7, 12 |
  | 33 | 50 ms | 8 | 36.5 ± 0.7 | 4.26, 8.80 | 0.501, 0.023 | 3.9, 5 |

  - The step past 17 is the constant 8, as built: its rows match the cap-8 rows within their
    spread (N = 18: 49.3 / 52.3 against 49.6–50.0 / 52.7–52.9%). At N = 25 and 33 both caps are 8
    at every rank once every peer is eligible, so those pairs of rows measure the run-to-run noise:
    Busy within 0.05 per upload, longest convergence 8.80 against 10.95 s, unpaired 5 against 12
    (of 528 pairs) at N = 33.
  - N = 18: the continuous rule takes 3.2–6.5 points off the share (45.8–46.4% against
    49.3–52.9%; a seed's sd is 0.6–2.4, the standard error of a 6-seed mean ≤ 1) and 0.15–0.76 s
    off the mean convergence. Busy rises from 0.005–0.023 to 0.022–0.062 per upload (from 0.3–1.1%
    to 1.2–2.9% of the completed sessions), about a sixth of N = 25's at the same Delay (0.18–0.40,
    5–10%). Served min 5.6–6.1, unpaired ≤ 1.
  - N = 21 (ranks 0–4 above 8): no gain and no loss beyond the noise: share 46.0–48.5 against
    46.0–49.2%, mean convergence within 0.35 s, Busy 0.086–0.107 against 0.074–0.085 per upload
    at 50 ms and 0.178–0.189 against 0.140–0.167 at 200 ms (3.3–6.4% of the sessions, against
    2.9–5.7%), below N = 25's rate. Served min ≥ 4.5, unpaired ≤ 4 (of 210).
  - Decision: the continuous rule. It keeps Busy bounded (1.2–6.4% of the sessions at N = 18–21,
    where the step gave 0.3–5.7%: below N = 25's rate at the same Delay, which T4.7 part 2
    accepted), starves no node, and removes the cliff: a peer that comes or goes moves the cap by at
    most 2. Where it differs from the step it is not worse beyond the noise but for N = 21's Busy
    at 200 ms (+0.01–0.05 per upload), and at N = 18 it is better by 3–6 standard errors. The
    longest convergence at N ≥ 21 reaches 10–11 s in some runs with either cap (the AC-V1 bound is
    10 s): a property of the neighborhood size, not of the rule.
  - **The AC-B1 picture changes at N = 18–24.** With the landed step N = 18 fails AC-B1 at 200 ms
    (52.3–52.9%, +2.3 to +2.9) and sits at the bound at 50 ms (49.3–50.0%): the part-2 follow-up's
    "N ≥ 17 passes" held only at the measured N = 17, 25 and 33. With the continuous rule every
    measured point passes (N = 18: 45.8–46.4%; N = 21: 46.0–48.5%; N = 25: 42.8–44.4%; N = 33:
    36.1%), but between 18 and 24 by 1.5–4.2 points, not the ~10 of N = 17 (Follow-ups).
  - Tests: `TestInitiatedSessionCap` takes the ranks of N = 18, 21 and 24 (no `floor(18)`) and
    checks over higher, lower ≤ 40 that past the tokens the cap is at most the room less one slot
    per peer past them, 8 from 8 peers past them, and that one peer more moves it by at most 1
    (higher) or 2 (lower); `TestRoundSessionCapPerNode` has N = 18 rank 0 at 15 sessions at once and
    N = 21 rank 2 at 10; `TestRoundSessionCap` takes 24 peers (8 past the tokens: the floor; 19
    give 13 now). `TestRoundSessionCapFollowsPeers` (17 higher and 8 lower peers) and the N = 25
    cases keep their cap (8 under both rules). The model (srep_model_test.go) runs N = 9 and 17
    only, where the rules agree: its records stand.
- **(T4.7 part 2 fix round 1) The `initiated_session_cap` gauge (review Low 1).** Each node
  judges "past the tokens" by its own eligible peers, which may not be its targets' (mixed radii:
  a deeper node sees ≤ 16 peers and takes up to 16 slots for targets that see more and are short
  of tokens). The harness runs one radius, so this is not measured here; T6.3 is to (Follow-ups).
  Added `bee_rangesync_initiated_session_cap` (reconciler metrics.go, appended last, as
  `round_interval_seconds`: a GaugeFunc read when collected), the slot pool's current size: the
  per-node cap the last round set (`MinInitiatedSessions` until the first), or the fixed cap of
  `Options.InitiatedSessions`. `TestRoundSessionCapFollowsPeers` checks that it follows the cap
  (16, then 8). No harness change.
- **(T4.7 part 2 fix round 1) Delay-0 checks of `TestSREPModelVsHarness` (review Low 2).** Part 2
  turned the Delay-0 share check (model against harness) and the live-against-record checks into
  logs for every row of the compiled constants, though only N = 17 had become non-repeatable. Now
  a row is checked at Delay 0 when no node opens more than `MinInitiatedSessions` sessions at one
  instant (`srepConfig.repeatable`, from `srepParams.wave`: at each rank the cap or the
  higher-overlay peers, whichever is fewer), as with the constant cap of 8, whose Delay-0 records
  repeated:
  - asserted again at Delay 0 (as before part 2): the compiled constants at N = 9 (wave 8: the
    share against the model within 3 points; the live harness against its record, share within
    1.5 points, n_Δ and symbols within 3%); the cap-8 configurations' share against their records
    (asserted throughout, now through the same `repeatable`, always true for them);
  - logged only: the compiled constants at N = 17, Delay 0 (wave 16), against the model and their
    record, with a sanity bound instead: the harness's mean share at most 60%
    (`srepDelay0ShareBound`; 47.3–52.8% measured);
  - unchanged: n_Δ, the symbols and the median n_Δ per session are checked at a Delay only, for
    every configuration (as before part 2).
  - Run (`TestSREPModelVsHarness`, 238 s, passes): N = 9 at Delay 0 checked, share −0.56 (±3.96)
    against the model, the live harness against its record share −0.28 (±2.79), n_Δ +1.08%
    (±4.23), symbols −1.48% (±5.79); N = 17 at Delay 0 logged, harness 49.1 ± 2.0% (≤ 60%), and
    its n_Δ −8.58% (±7.98) against the record, which would have failed: the reason it stays a log.
- **(T4.7 part 2 fix round 1) Scope, tests and wall times.**
  - Code: pkg/rangesync/reconciler (scheduler.go `InitiatedSessionCap`, metrics.go and
    reconciler.go for the gauge) and tests (scheduler_test.go, export_test.go,
    neighborhood_g6_sim_test.go `TestG6InitiatedCap`'s rows, srep_model_sim_test.go). No pkg/node,
    pkg/pullsync, pkg/puller, go.mod, proto or `*_fuzz_test.go` change; README not ticked.
  - `go build ./...`, `go vet` on pkg/rangesync/... (default and `-tags=sim`),
    `GOTOOLCHAIN=go1.26.1 make lint` and `GOTOOLCHAIN=go1.26.1 golangci-lint run --build-tags sim
    ./pkg/rangesync/...`: 0 issues. gofumpt and gci on the changed files only.
    `go test ./pkg/rangesync/...` passes (`pkg/rangesync` 35 s, reconciler 6 s).
  - `-tags=sim`, one at a time (4 cores, nothing else running): `TestG6InitiatedCap` 708 s (108
    runs), its N = 18 and 21 rows again 317 s, the step's N = 18 and 21 rows 134 s on the landed
    code; `TestSREPModelVsHarness` 238 s.
  - No `-race` here: the changed tests (`TestInitiatedSessionCap`, `TestRoundSessionCap*`,
    `TestMetrics`, `TestRoundIntervalAdapts`, `TestPeersGauge`, `TestSlotPool*`) ran with
    `-count=20 -cpu=1,2,4`, the whole reconciler package with `-count=3 -cpu=1,2,4`,
    `TestNeighborhoodPastTokens` with `-count=2 -cpu=1,2,4` and pkg/rangesync with `-count=1
    -cpu=1,2,4`: no failure. The CI list is in Follow-ups.
- **(Orchestrator, T4.7 part 2 landing, 2026-10-21) Part 2 landed after one fix round.** Review: no
  High; the Medium (the 16 → 8 cap step at N = 17/18) and both Lows were fixed in fix round 1, which
  verified clean (no re-review: no High). README's "Shared constants" now shows growth factor 2 and
  the per-node initiated-session cap. AC-B1 after part 2: N = 9 fails (59–61% modeled, 54–56% on the
  wire; re-basing stays with T6.0/T6.4), N = 17 passes by ~10 points, N = 18–24 pass by only 1.5–4.2.
- **(T4.7 part 3) The stripe-registry `SyncRate` pin, as built: a registered stripe job holds
  `SyncRate` only while it stores chunks, past a grace.** Commits 98a95800 (reconciler: code and
  tests) and 68685ce1 (pkg/rangesync: `TestFetchWithProgressNotStored`). The PRE-TESTNET BLOCKER
  of T4.5 part 2c (round 2), widened by part 1 ("(T4.7 part 1 → part 3) The registry `SyncRate`
  pin, widened" and "… corrected"). `SyncRate` no longer holds at `StartupGuardRate` because the registry is not
  empty; it holds for a registered job as follows (reconciler.go `SyncRate`, rate.go):
  - *A peer's claim* (the served sessions' Striper, a round session's `StripeCollector`, the
    rounds' session-root check) holds nothing of its own. Its transfer and its verification are
    catch-up fetches in flight, as before, so they hold `SyncRate` at `StartupGuardRate` only while
    the rate is above 0 (a catch-up chunk stored within the window) or the node-wide catch-up grace
    holds (`catchUpGrace` 2 s, refilled only by stored chunks: the T4.5 part 2c rule for catch-up
    fetches, applied to stripes); a queued claim holds nothing. Why the node-wide budget: peers
    register claims at will, again as soon as one ends (≤ `maxPeerStripes` per peer, any number of
    peers, overlays and reconnects), so a grace per claim would be renewed by each registration,
    the per-fetch grace that T4.5 part 2c rejected.
  - *This node's own transfers* (its bootstrap, a radius decrease's new half), queued,
    transferring or being verified, hold `SyncRate` at `StartupGuardRate` while the own-job grace
    holds: `ownJobGrace` = `radiusHold` = `SyncRateWindow` (10 minutes), one budget for the node
    (`Reconciler.ownGrace`, a `graceBudget` now sized per use) that drains while an own job is
    registered (`ownJobsChangedLocked`, at every registration, removal, takeover and on Close) and
    is refilled to full only when an own job stores a chunk (`jobStored`), never with time or per
    job. Past the grace an own job holds only through its chunks, as any catch-up does. Why a
    grace of its own, and that long: an honest own transfer's first chunk can take far longer
    than the catch-up grace (the stripe and session slots, the wait for the peer's round session
    to end, up to `maxStripeBusyRetries` Busy retries of 1 s per attempt, this node's index
    rebuild, re-asks of held IDs), and any peer can drain the catch-up grace; registering a
    radius half must not shorten the hold it had while pending (`radiusHold`, T3.5 review); 10
    minutes stay below the reserve worker's 15-minute wake-up. Why one budget for the node, not
    one per job: a peer cannot start an own job at will, but a Sybil majority of the peers that
    share this node's root can make it bootstrap again and again (`IsBootstrap` through the median,
    re-review Low 4, T6.3); with a grace per job each bootstrap cycle (the Sybils' stripes time out
    after `StripeTimeout`, ≥ 15 minutes, then the bootstrap is registered again) would read above 0
    for 10 of every ≥ 15 minutes; with one budget it reads above 0 for one grace in all until an
    own job stores a chunk. Why not the T3.5 hold: a stalling peer among an own job's stripe peers,
    its reassignments (up to 4 peers in a row) and its verifier held a real own job, and with it
    `SyncRate`, for up to 4 × 15 + 15 minutes after the honest stripes ended (2.25 h per peer if
    it drips), and a Sybil-made bootstrap held it all the time.
  - *Verification phases* count as transfers: their fetches report through the same per-job hook,
    so a chunk a verification stores refills the own-job grace (and restarts a claim's idle clock,
    below), and one that stores nothing drains the grace (an honest verification after a transfer
    that stored lasts seconds, within the window of the transfer's chunks).
  - *Stored chunks count as they are stored*: the stripe worker passes a per-job `stored` hook
    (`jobStored`) through `transferStripes`, `runStripe`, `transferStripe` (with the stripe clock's
    progress), `resolvePending` and the new internal `verifyStripes` (`VerifyStripes` and
    `TransferStripes` keep their signatures), and `fetchFrom` calls it per chunk through
    `FetchWithProgress` (`progressFetcher`), or once at the fetch's end without one.
  - *Unchanged*: the start-up guard (T1.11, T4.2, T4.5 part 2a rules; `StartupGuardTimeout`); a
    pending half that is not registered holds `SyncRate` for at most `radiusHold` from the
    decrease or its last failed transfer (T3.5 review). It is not pinnable: only this node's
    radius decrease and its failed transfers re-arm it, at most `maxRadiusAttempts` per decrease,
    and the reserve worker lowers the radius only with the reserve below its threshold and
    `SyncRate` at 0, which no peer controls. `catchUpGrace`, `CatchUpMinDiff`, the 10-minute
    window, `pkg/node`'s combined syncer.
  - Numbers. A lying claimant: at most `catchUpGrace` (2 s) of `SyncRate` above 0 per catch-up
    episode of this node, the T4.5 part 2c bound, whatever its claims; before, `StartupGuardRate`
    for as long as a claim was registered, that is all the time. A peer that drips: above 0 only
    while its chunks are in the window (10–20 minutes per chunk this node accepts and stores), as
    an honest slow peer's would; before, all the time. An own job that stalls: at most the window
    of its last chunk (≥ `ownJobGrace`) after it; before, until the job ended (≈ 75 minutes for
    a stall, ≈ 11 h for a drip). A Sybil-made bootstrap: one `ownJobGrace` until an own job stores;
    before, all the time (its threshold-0 effect stays, T6.3). Honest: an own job stays held
    through pauses of up to 10 minutes before its first chunk and of the window (≥ 10 minutes)
    after one; an honest claim before its first chunk reads 0 once the catch-up grace is spent
    (the T4.5 part 2c honest window, now also for claims: seconds on a working link; at start-up
    the start-up guard covers a session-root lack, which keeps the guard unarmed for up to
    `StartupGuardTimeout`), and above 0 from its first chunk through the window.
- **(T4.7 part 3) A peer's claim that stores nothing leaves the stripe worker
  (`claimIdleTimeout`).** Part 1's hand-over (2) and re-review Low 2. The worker runs a claim on
  an idle clock of its own (`stripeJob.idle`, a `stripeClock` with no cap, on the injected clock):
  once the claim's job has stored no chunk for `claimIdleTimeout` = 2 minutes since it started or
  last stored one, `endIdleClaim` ends its context (cause `errClaimIdle`), counts
  `stripes_total{idle}` and logs it (Debug); the worker then removes it, and its peers' later
  sessions or rounds register it again, behind the claims registered meanwhile. The clock spans
  the whole job, every stripe's attempts, restarts and reassignments and the verification round;
  only the job's stored chunks restart it, and it runs through Busy waits. Own jobs are never
  ended so (one that took a claim over disarms its clock). No peer is scored: an ended claim is
  nobody's fault. Sizing: an honest claim's first chunk needs at most one attempt's Busy waits
  (30 × 1 s), its re-asks of IDs the want set held (`maxPendingPasses` × 1 s = 30 s) and a fetch
  timeout (`ClaimDeadline` / 2 = 15 s) together, 75 s; its verification, after a transfer that
  stored, sessions that find their stripes equal: seconds. A liar's claim holds the worker for
  at most `claimIdleTimeout` after its job's last stored chunk, however many lying peers its
  stripe is reassigned to (up to 4 in a row) and whoever verifies it; before, its stripe's
  inactivity timeout per lying peer (4 × 15 minutes) and the verification's (15 minutes), ≈ 75
  minutes, and with a drip up to 4 × 2.25 h + 2.25 h. So an honest claim (a responder-only lack, a
  constraint-(d) session root) waits behind a stalling liar for at most `claimIdleTimeout` per
  liar claim queued ahead of it (FIFO among claims, at most `maxPeerStripes` per lying peer). Not
  bounded: a peer that drips a chunk this node lacks and accepts at least every 2 minutes holds
  the worker as an honest slow peer would, up to the stripe budgets (Follow-ups); and an honest
  claim whose plan gives a stripe to a stalling peer now ends `claimIdleTimeout` after its honest
  stripes' last chunk instead of reassigning that stripe after `StripeTimeout`, so that part is
  transferred by a later registration, planned afresh (Follow-ups). The idle clock adds one
  goroutine while a claim runs (doc.go).
- **(T4.7 part 3) An own job no longer ends the claim in flight of its range (re-review Low 1,
  AC-B2 headroom).** Both fix directions, since neither alone covers every trigger:
  - *The session-root check registers no claim while `IsBootstrap` holds* (scheduleLocked, as
    `servedStriper.Stripe` already did): `planCatchUp` now also returns whether this node is
    bootstrapping (false while a radius half is pending). This is the failed-bootstrap trigger:
    the round after a bootstrap that did not complete opens no session with a peer this node lacks
    far behind and registers no claim of the root, and the next round registers the bootstrap
    again. `planCatchUp`'s doc ("one round runs the per-peer sessions before the next try") now
    says so: the sessions of that round run with the peers within `LargeDiff` only.
  - *An own job whose range is the claim in flight's takes the claim over* (`adoptLocked`, from
    `planCatchUp` and `enqueueRadiusHalf`) instead of ending it (`preemptLocked`) and listing the
    range again: the claim becomes the own job (`own`, `bootstrap` or `radius`; a half's context,
    which a radius increase cancels, ends it too through `context.AfterFunc`), keeps its plan and
    transfer options (a bootstrap that took a claim over does not keep its peers' locks across
    asks, `peerLocks.keep`), is not counted as a pending reconcile (its transfer is under way) and
    lowers the threshold to 0 from then on (`Transferring`). Another own job in its way refuses
    it as it refuses a registration. The start-up trigger as described (no peer admitted, so
    `IsBootstrap` is false and a served session registers the root) is mostly harmless: with no
    admitted peer the claim's plan has no stripe peer and it ends at once (`ErrNoStripePeer`),
    listing nothing. What remains are the cases where a stripe peer is admitted before
    `IsBootstrap` turns true (an edge peer admitted before the peers that share the root, a median
    not yet past the ratio, the race between a round's `admitLocked` and its `planCatchUp`), all of
    them a running claim of the root that the bootstrap now takes over. A claim in flight of
    another range (inside the own job's, or containing it) is still ended (Follow-ups).
- **(T4.7 part 3) Re-review Low 3: a session past the peer's claim cap stops at once.**
  `enqueueStripes` checks the peer's claims before each range and, at `maxPeerStripes`, drops the
  rest of the session's ranges at once, counted once (`stripes_total{dropped}`) and logged once
  with their number (`dropPeerFullLocked`); a served session (`servedStriper.full`) does the same
  from the first range it cannot register. So `stripes_total{dropped}` counts one per session
  that met the cap, not one per range. The help text of `stripes_total` lists `dropped` and the
  new `idle`.
- **(T4.7 part 3) Item 5: the T4.5 part 2c test gaps.** Closed: `TestFetchWithProgressNotStored`
  (pkg/rangesync: an ID in the reserve, skipped before the Want; a delivery another protocol
  stored meanwhile; a stale stamp, `ErrOverwriteNewerChunk`: none of them reported, the other
  chunk once) and `TestSyncRateCatchUpAfterGraceDrained` (reconciler, real Service: after a
  stalled catch-up drained the grace, an honest catch-up reads 0 until its first chunk and at
  least `StartupGuardRate` from it on, its fetch still in flight). Not done (optional): the
  one-chunk-then-reset pin (L4685–4688); part 4's per-peer fetch credit is the fix that bounds it.
- **(T4.7 part 3) Superseded.**
  - T3.5 (phase 3, "(T3.5) Catch-up span of striping"): "`SyncRate` reports at least
    `StartupGuardRate` while any range is in the stripe registry (queued, transferring, between
    transfer and verification, verifying)": a claim never does; an own job within the own-job
    grace, past it only through its chunks.
  - T3.5 review ("Radius decrease before registration"): "A registered half counts through the
    registry with no bound (its transfer has its own StripeTimeout/reassign bounds)": a registered
    half counts within the own-job grace. Its `radiusHold`, `maxRadiusAttempts` and the pending
    half's hold stand.
  - T4.5 part 2c fix round 2, "Unchanged: a registered stripe range, a pending radius half (at most
    `radiusHold`) and the start-up guard … each hold `SyncRate` at least at `StartupGuardRate`, as
    T3.5 and T1.11 have them": the registered stripe range no longer does (above).
  - "(T3.2 part 2 review) Bootstrap and the start-up guard (D9/D18)" (phase 3): "after one that did
    not complete (any stripe error), the next round runs per-peer sessions before bootstrapping
    again": with the peers within `LargeDiff` only, and no claim of the root while `IsBootstrap`
    holds (`planCatchUp`'s doc said it too); and "The bootstrap round counts as a pending reconcile
    only when it registers the root": not when it takes a claim of the root over.
  - T4.7 part 1 fix round 1, Medium 1: an own job "ends the claim in flight whatever its range":
    except the claim in flight of its own range, which it takes over.
  - The hand-overs "(T4.7 part 1 → part 3) The registry `SyncRate` pin, widened" and "… corrected"
    (Follow-ups) are closed by the two entries above; so is the re-review's Low 2 (the worker
    hold, sized with the verification phase and Sybil reassignment) and the PRE-TESTNET BLOCKER
    "stripe registry SyncRate pin". The per-peer fetch credit (part 4) is the other blocker.
- **(T4.7 part 3) Tests, scope and checks.**
  - New (reconciler): `TestSyncRateClaimPin` (status and opening paths: a higher-overlay peer
    claims a count more than `LargeDiff` above and stalls its stripe; `SyncRate` 0 past the
    catch-up grace with the claim registered and in flight, the claim leaves the worker after
    `ClaimIdleTimeout` and a claim queued behind it runs, registered again and still 0, no peer
    scored), `TestSyncRateClaimDrip` (one stored chunk per 25 minutes from the liar's claim,
    registered again each time: above 0 after each chunk, 0 from two windows after it, the claim
    leaves the worker each time), `TestSyncRateHonestChurn` (a claim storing every 90 s, a
    bootstrap and a radius half storing every 3 minutes, each after Busy answers, the own ones
    after a 5-minute stall before their first chunk: `SyncRate` above 0 (the reserve worker does
    not lower the radius) throughout and through the verification, nothing ended, no score, 0
    past the window), `TestSyncRateOwnJobGrace` (a stalled bootstrap: above 0 within the grace, 0
    past it, still 0 when it fails and is registered again, above 0 once it stores),
    `TestClaimStoringNotEnded` (real services: an honest claim whose one fetch stores a chunk
    every 30 s for 6 minutes is not ended: each chunk restarts the idle clock as it is stored),
    `TestRoundBootstrapFailedPastLargeDiff` (Low 1's failed-bootstrap trigger, the gap of
    `TestRoundBootstrapFailedResumesSessions`), `TestRoundBootstrapTakesOverStartupClaim` (Low 1's
    start-up trigger, an edge peer's claim of the empty root taken over once a same-root peer is
    admitted: one stripe session with the edge peer, nothing listed twice),
    `TestOwnJobTakesOverClaim` (bootstrap and radius half; a radius increase ends a half taken
    over), `TestClaimCapDropsOnce` (Low 3), `TestSyncRateCatchUpAfterGraceDrained`; pkg/rangesync:
    `TestFetchWithProgressNotStored`. Each new test of a changed rule fails on the old code
    (checked: the pin and drip tests on the old `SyncRate`, the failed-bootstrap test without the
    `IsBootstrap` skip, the takeover tests without `adoptLocked`, `TestClaimStoringNotEnded` with
    the chunks counted at the fetch's end).
  - Changed by design (an own job that stores nothing no longer holds `SyncRate` past the own-job
    grace): `TestSyncRateBootstrap` (within the grace above 0, past it 0, then as before),
    `TestSyncRateRadiusDecreaseSequencing` and `TestSyncRateRadiusHalfRegistryFull` (their stalled
    halves checked within the grace instead of two windows on), `TestDualModeSyncRate` (the puller
    pulls first and its window passes during the bootstrap's grace). `TestSyncRateRegisteredStripe`
    passes unchanged (a registered own job within the grace).
  - Test harness: `pinService` (syncrate_pin_test.go: per-peer stripe stalls, listings, gated
    fetches that report each chunk as `FetchWithProgress` does), `newPinHarness`; `roundNodeOptions.after`;
    export_test: `OwnJobGrace`, `ClaimIdleTimeout`, `RunningStripe`, `Bootstrapping`,
    `FailBootstrap`; `HoldStripe`/`ReleaseStripe` keep the own-job grace's bookkeeping. On a fake
    clock a `stripeClock` that a progress moved re-arms from the clock it reads then, so its expiry
    can come up to one clock step late; `TestSyncRateClaimDrip` steps the clock until the claim ends.
    `streamtest` deadlocks when its writer runs more than 16 deliveries ahead of a lagging reader
    (known), so the real-service tests fetch at most 12 chunks.
  - Code: reconciler.go (`SyncRate`, `ownGrace`, `ownJobsLocked`, `ownJobsChangedLocked`),
    rate.go (`ownJobGrace`, `graceBudget.size`, `newGraceBudget`), stripes_transfer.go
    (`claimIdleTimeout`, `stripeIdle`, `errClaimIdle`, `stored` plumbing, `stripeJob.idle`,
    `dropPeerFullLocked`, `servedStriper.full`, `adoptLocked`, `planCatchUp`, the worker,
    `jobStored`, `endIdleClaim`), stripes_verify.go (`verifyStripes`), scheduler.go (round,
    `scheduleLocked`), radius.go (`enqueueRadiusHalf`, docs), metrics.go (help text), doc.go
    (goroutines). No pkg/node, pkg/pullsync, pkg/puller, pkg/storer, go.mod, proto or
    `*_fuzz_test.go` change; no new parser of peer input (followup-fuzzing.md untouched); README not
    ticked.
  - Checks: `go build ./...`; `go vet` on pkg/rangesync/... (default and `-tags=sim`);
    `GOTOOLCHAIN=go1.26.1 make lint` and `GOTOOLCHAIN=go1.26.1 golangci-lint run --build-tags sim
    ./pkg/rangesync/...`: 0 issues; gofumpt and gci on the changed files only (coexistence_test.go's
    import order is gci-flagged on rangesync/phase-4 already and untouched). `go test
    ./pkg/rangesync/...` passes (pkg/rangesync 32 s, reconciler 6 s); pkg/node's
    `TestCombinedSyncerBothDrivers`, `TestSyncWiring`, `TestRangesyncOptions` pass. No `-race`
    here: the reconciler package ran `-count=3 -cpu=1,2,4` (48 s), `TestSyncRate*`,
    `TestStartupGuard*`, `TestRadius*`, `TestRoundBootstrap*`, `TestStripe*`,
    `TestTransferStripes*`, `TestVerifyStripes*`, `TestClaim*`, `TestOwnJob*`,
    `TestEnqueueStripes*` and `TestDualModeSyncRate` `-count=20 -cpu=1,2,4` (163 s), and
    `TestNeighborhoodBootstrap*` and `TestFetch*` (pkg/rangesync) `-count=5 -cpu=1,2,4` (379 s): no
    failure. The CI list is in Follow-ups.
  - AC-B2 re-check (`-tags=sim`, `go test -run TestG6Bootstrap`, one run, 2975 s; no RSS
    figure, no `/usr/bin/time` on this host): every run passes its `check` and
    `checkDeclined`, no stripe timed out, no duplicate, and every figure is part 1 fix round 1's
    within the seeds' spread. The own transfers of these runs store throughout, so the own-job
    grace never ran out, and no claim was ended. `TestG6BootstrapScaled` (N = 5, node 0 empty,
    `LargeDiff` Base/64, ranks 1–4): node 0 reads 4908–4913 KB at Base 70000 (bound 6635 KB) and
    7010–7014 KB at 100000 (bound 9050 KB), 1.001–1.003× of the lacked IDs (part 1 fix round 1:
    4.908–4.913 and 7.010–7.014 MB); takeovers ranks 1 / 2 / 3 / 4, "2 min": 24 / 16 / 7 / 0 at Base
    70000 and 23 / 15 / 7 / 0 at 100000 (24 / 15 / 7 / 0, 23 / 15 / 8 / 0), "lifted" 3 / 2 / 1 / 0;
    the peers listed node 0 no ID it lacked (wants 0 in "2 min", 1–7 uploads in "lifted"); 1557 s.
    `TestG6BootstrapTail` (Base 16000, `LargeDiff` 250, 3 per second on the clock): 1232 KB
    (1.100×), 1263 KB (1.128×), 1326 KB (1.184×) of 2288 KB at ranks 1, 2, 4, caught up after
    5334–5335 s, 0 wants, 9–13 stripe sessions, the 3 h row as the default one (identical to part
    1 fix round 1); 620 s. `TestG6Bootstrap` (N = 17, Base 20000): 1410 and 1408 KB at ranks 4 and
    14 of 2610 KB (1411, 1410), 16 stripes done, none reassigned or timed out; 35 s.
    `TestG6BootstrapPeers` (production `LargeDiff`): 4908–4909 KB at Base 70000 and 7011 KB at
    100000 (4908–4909, 7011–7012); 758 s.

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
- **(T4.3) Tests to run with `-race` on CI.** These ran with `-count=10..20 -cpu=1,2,4` instead.
  - `pkg/rangesync/reconciler`: `TestRound*` (new: `TestRoundInitiatorRule`,
    `TestRoundTakeover*`, `TestRoundIntervalAdapts`, `TestRoundSessionCap*`,
    `TestRoundWaitsForMovedRoot`; also `TestRoundOwnsFirstContact` and `TestRoundBootstrap*`),
    `TestStatusChangeTriggersRound`, `TestRootChangeTriggersRound`, `TestStatusChangeWakesGossip`,
    `TestStripePlanAvoidsBackoff`.
  - Re-run with `-race`, because `round`, `session`, `probe`, `loop`, `gossip` and
    `transferStripe` changed: `TestStatusGossip*`, `TestPeerState*`, `TestStartupGuard*`,
    `TestSyncRate*`, `TestRadius*`, `TestTransferStripes*`, `TestVerifyStripes*`, `TestPeerJoinLeave`,
    `TestClose`.
  - `pkg/rangesync`: `TestSubscribeStatus`, `TestStatus*`.
- **(T4.3) First Busy retry on topology rounds (T4.5).** Still open. Triggers are now deferred to
  the minimum interval, but topology changes still run a round at once, which can retry a peer's
  first Busy within milliseconds. Deferring topology rounds like triggers breaks the tests that
  drive rounds through topology changes with `RoundInterval = 1 h`: `TestRoundOwnsFirstContact`
  and the radius tests. T4.5 replaces the loop and should rework those tests with it.
- **(T4.3) Stripe registry trimming (T4.5).** Not done. A partially overlapping range is still
  dropped and shows up again in a later round. Trimming changes the job bookkeeping (`started`,
  bootstrap and radius jobs, `cancel`). T4.6's simulation should first show whether the dropped
  ranges cost rounds.
- **(T4.3) Compare session-root summaries in `sameDigest` (T4.5).** Not done. It needs an
  additive `Service` method for the local summary of a range, which changes the narrow
  `reconciler.Service` and its fakes. It helps only when the session root is the peer's root,
  since the peer's Status carries only its root's summary. "Different roots ⇒ different" costs one
  Fingerprint exchange per round with such a peer, not a wrong outcome.
- **(T4.3) `Local.Symbols` cancellation (T4.5 or later).** Not cheap. The scan path takes no
  context, so a slow scan blocks `Handle` for up to `maxScanIDs` IDs. Cancellation needs a context
  in the `Local` interface (T1.8 contract), in `local.scanIDs`, and in every session-test fake.
  The scan is bounded (`ScanLimit`, `MaxScansPerMessage`), so it is a latency issue, not a
  memory one.
- **(T4.3) Correct §8.2 "all sessions in a round read the same index snapshot".** T1.6's locking
  gives per-stream snapshots, not per-round ones (see Decisions log). Edit the proposal, or let
  T6.0's acceptance text say "each symbol stream reads one snapshot".
- **(T4.3) Initiated-session share (T4.6).** `MaxInitiatedSessions = 8` leaves half of the 16
  tokens for served sessions. In a neighborhood of more than 9 differing peers, the lowest node
  runs its sessions in several waves within a round. T4.6's N = 17 simulation should check the
  convergence time against `RoundInterval`.
- **(T4.3 fix) T4.6: check the stripe/round split.** In addition to the initiated-session share
  above, stripes now take at most `MaxStripeSessions = 4` of the 8 initiated slots. An empty node
  in an N = 17 neighborhood bootstraps its 16 stripes in four waves, each within its own stripe
  timeout. T4.6 should measure the bootstrap time against the fetch limiter and the session
  timeout, and adjust the split if the waves dominate.
- **(T4.3 fix) Tests to run with `-race` on CI.** These ran with `-count=10 -cpu=1,2,4` instead.
  - `pkg/rangesync/reconciler`: `TestStripeSessionCap`, `TestStripeTimeoutAfterSlot`,
    `TestRoundIntervalNestedRoots`, `TestSessionCollision`, `TestSessionRecheckAfterSlot`,
    `TestStatusChangeFloor`, `TestRoundSessionCap` (second round while waiting),
    `TestStripePlanAvoidsBackoff` (fallback and verifier).
  - Re-run with `-race`, because `transferStripe`, `session` and `loop` changed again:
    `TestTransferStripes*`, `TestVerifyStripes*`, `TestRadius*`, `TestPeerState*`, `TestRound*`,
    `TestStatusChange*`, `TestRootChangeTriggersRound`.
- **(T4.3 fix round 2) Tests to run with `-race` on CI.** These ran with `-count=10 -cpu=1,2,4`
  instead.
  - `pkg/rangesync/reconciler`: `TestStripeBusyDoesNotStall`, `TestStripeBusyRetriesBounded`,
    `TestStripeTimeoutPausedWhileBusy`, `TestStripeTimeoutAfterSlot`,
    `TestRoundTakeoverNestedRoots`, `TestRoundIntervalServedWants`, `TestSessionRecheckAfterSlot`.
  - Re-run with `-race`, because `transferStripe` changed again: `TestTransferStripes*`,
    `TestVerifyStripes*`, `TestRadius*`, `TestRoundBootstrap*`, `TestStripeSessionCap`.
- **(T4.3 fix round 2) T4.6: Busy-heavy stripes.** An always-Busy peer now costs a stripe about
  30 s per attempt before it is reassigned. T4.6 should check whether `maxStripeBusyRetries`
  delays bootstraps when many neighbors sit at their session cap.
- **(T4.3 fix round 3) A Busy-only peer counts as tried for verification (T4.5).** A stripe peer
  that only answered Busy (past `maxStripeBusyRetries`) lands in `StripeResult.Tried`, so
  `verifier` skips it. In small neighborhoods that means more `ErrNoVerifier`; the Busy peer is
  never blamed. T4.5 could record only the peers that delivered (or ran a session) as tried for
  verification.
- **(T4.3 fix round 3) Tests to run with `-race` on CI.** These ran with `-count=10 -cpu=1,2,4`
  instead.
  - `pkg/rangesync/reconciler`: `TestStripeFetchBoundedByTimeout`, `TestStripeLocalBusy`,
    `TestStripeBusyReassigns`, `TestStripeBusyDoesNotStall`, `TestStripeTimeoutPausedWhileBusy`,
    `TestPeerStateWaiterWoken` and `TestRadiusIncreaseNoFetch` (both ran 60 × 3 after their fix).
  - Re-run with `-race`, because `transferStripe` and `runStripe` changed: `TestTransferStripes*`,
    `TestVerifyStripes*`, `TestRadius*`, `TestRoundBootstrap*`.
- **(T4.3 round-3 review, Lows; recorded by the orchestrator at landing) Stripe bounds and timer (T4.5).**
  1. `restarts` resets on each reassign (stripes_transfer.go ~L236/245). For a node-wide local cause
     (`ErrNodeSessionLimit`, `fingerprint.ErrBusy`) the 3 reassigns gain nothing and multiply the wait
     by 4 (worst case ≈ 15 min × 9 attempts × 4 peers ≈ 9 h; the doc states only the 135 min per-peer
     figure). `ErrPeerSessionLimit` is under the peer's control: the responder holds the slot through
     `busy()`→`FullClose` (reconcile.go ~L292), so a peer that delivers nothing can pin a stripe for
     ~135 min (round 2: ~30 s). Fix: one local-Busy restart budget per stripe, not reset on reassign
     (or count `ErrPeerSessionLimit` as peer Busy), and document the total bound.
  2. The peer-Busy pause (~L400) can overlap a fetch still draining from an earlier session in the
     same attempt (only after a mid-session local `ErrBusy`), adding up to 30 × (1 s + slot wait) of
     fetch time the clock does not bound. Fix: don't pause while a fetch is pending.
  3. Tests: `TestStripeFetchBoundedByTimeout` is vacuous if the 100 ms timeout fires before the
     session ends (gate on the fetch actually starting); `TestStripeLocalBusy` checks only the lower
     time bound.
  4. `stripeClock` and the Busy retry timer (~L404/481) use the real clock, not the injected one.
- **(T4.4 → T4.6) Open question 7 under skew.** Measure on an N-node simulation with a batch
  that expires a few blocks apart across nodes:
  - the wasted deliveries of an early-evicting node;
  - the `rejected_cache_size` peak against `MaxRejected`;
  - refetches after the cache overflows.

  Then decide whether Q7's alternative is needed. Either exclude batches within N blocks of
  expiry from fingerprints and symbols (the Index would need batch expiry), or reject per batch
  (wants would need batch IDs, frozen by G2). Also check a node that lags more than 10 blocks
  behind batch creation (its new chunks are rejected for 10 min). Record the finding against Q7.
- **(T4.4 → T6.0) `want_claims` semantics for AC-D1/AC-D2/AC-D4.** `denied` is `wantset.Held`.
  `expired` counts every granted claim found ended before its fetch ended it; `late` is the subset
  delivered anyway. Both are expected to stay 0 with the default fetch timeout. The acceptance
  text should bound duplicates by `want_claims{result="expired"}`, not by `expired + late`.
- **(T4.4 → T4.5) Undelivered IDs are not scored (§9 "bogus wants count against the peer").** A
  peer that lists IDs and closes its fetch stream without them costs a retry or a round, never a
  score. If testnet shows it, score it above a tolerance, as `verifyStripe` does.
- **(T4.4 → T4.5) Rejected-cache flush.** A peer can fill the FIFO cache with its own IDs by
  delivering invalid stamps. `validStamp` runs before the CAC/SOC check and its failure is
  unpenalized (T1.10). Options: validate CAC/SOC before the stamp, or cap rejections per peer.
- **(T4.4 → T4.5) Concurrent fetch streams per peer (pre-existing).** Several sources can run
  fetches to one peer at once:
  - a round fetch;
  - responder-side `Wants` fetches, one goroutine per served session, unbounded;
  - a stripe fetch;
  - now at most one retry.

  Only `MaxClaimsPerPeer` bounds their streams, and only when Wants are full (1000 / 250 = 4). The
  holder disconnects over `maxPeerFetchStreams = 4`. A requester-side per-peer stream bound would
  remove the risk.
- **(T4.4 → T4.6) Retry granularity.** Results come per `Fetch` call. T4.6 should check whether
  retries that wait for a long fetch (up to `LargeDiff` / 1000/s ≈ 65 s) delay convergence; a
  per-Want result callback would retry sooner.
- **(T4.4) `fetch_skipped_total{reason="held"}` is ambiguous (pre-existing).** It counts both "in
  the reserve" and the want set's `Held`. `want_claims{result="denied"}` now covers the latter;
  T4.5 could rename the reserve reason.
- **(T4.4) Proposal §5.3 memory table.** Add the rejected cache: about 2.7 MB at `MaxRejected`
  (measured about 270 B per ID), next to the want set's 2.7 MB (T1.7 follow-up).
- **(T4.4) Tests to run with `-race` on CI.** These ran with `-count=10..20 -cpu=1,2,4` instead.
  - `pkg/rangesync/wantset`: `TestConcurrentReject`, and `TestConcurrent*` again.
  - `pkg/rangesync`: `TestFetchWithResult*`, `TestFetchRejected*`, `TestFetchRejectTTL`,
    `TestFetchLateDelivery`, `TestFetchExpiredClaim`, and `TestFetch*` again (the fetch path
    changed).
  - `pkg/rangesync/reconciler`: `TestWantSet*`, `TestSREP*`, `TestRejectedNotRefetchedEachRound`,
    `TestStripeWithheldUndeliveredOnly`, `TestVerifyStripesUndeliveredOnly`. Re-run, because the
    fetch path and `peerLocks` changed: `TestTransferStripes*`, `TestVerifyStripes*`,
    `TestStripe*`, `TestRadius*`, `TestRound*`, `TestPeerState*`.
- **(T4.4 fix round 1 → T6.4) AC-D1/AC-D2/AC-D3 wording.** With the reserve re-check after the
  claim and the default timeouts, intra-`rangesync` duplicates and `want_claims{expired,late}` are
  0 by construction, faults included (see Decisions log). This supersedes the "(T4.4 → T6.0)
  `want_claims` semantics" bullet's "bound duplicates by `want_claims{result="expired"}`": that
  bound still holds, but trivially (0 ≤ 0). In acceptance-criteria.md:
  - AC-D1's "with faults: duplicates ≤ expired claims" can read "zero, faults included, unless
    `FetchTimeout` ≥ `ClaimDeadline`".
  - AC-D2: in a `rangesync`-only cohort, `cross_protocol_duplicates_total` counts only other
    writers (pushsync) racing a fetch. `want_claims{result="late"}` is expected to be 0, and a
    non-zero value flags a misconfigured timeout. AC-D2's "upper bound on AC-D1" note and the
    summary row ("can't tell intra-`rangesync` duplicates apart") can be tightened.
  - AC-D3's "the `ReserveHasID` re-check before `Want`": there are now two, one before the limiter
    wait and one after the claim.
- **(T4.4 fix round 1 → T4.6) Deferred stripes under a busy want set.** A stripe with deferred IDs
  makes up to `maxPendingPasses` extra `Fetch` calls per attempt (30 s), then restarts. T4.6
  should check that a bootstrap near `MaxClaims` or `MaxClaimsPerPeer` does not burn its restarts
  and reassignments on `ErrStripeDeferred`. If it does, wait longer between passes or exempt
  deferral from the restart count.
- **(T4.4 fix round 1) Signature-recovery stamp errors are not rejected.** secp256k1 recovery
  errors have no sentinel, so such stamps are refetched each round. A `postage` sentinel wrapping
  `RecoverBatchOwner` errors would let `stampRejected` cover them; that needs a `pkg/postage`
  change, outside this plan's allowed scope.
- **(T4.4 fix round 1) Tests to run with `-race` on CI.** These ran with `-count=10 -cpu=1,2,4`
  instead.
  - `pkg/rangesync`: `TestFetchClaimRechecksReserve`, `TestFetchWantSetFull`,
    `TestFetchStampCheckFailure`, and `TestFetch*` again (`claim` and `deliver` changed).
  - `pkg/rangesync/reconciler`: `TestStripeDeferred`, `TestStripeDeferredResolved`,
    `TestWantSetRetryLongFetch`, `TestWantSetSourcesPruned`, `TestStripeWithheldUndeliveredOnly`
    and `TestVerifyStripesUndeliveredOnly`. Re-run, because `transferStripe`, `runStripe` and
    `wantSources` changed again: `TestWantSet*`, `TestSREP*`, `TestTransferStripes*`,
    `TestVerifyStripes*`, `TestStripe*`, `TestRadius*`, `TestRound*`.
- **(T4.4 fix round 2 → T6.4) AC-D1/AC-D2 wording, corrected.** This supersedes the round-1 T6.4
  bullet's "0 by construction, faults included". Intra-`rangesync` duplicates are at most
  `want_claims{expired}`, which now also counts claims that expired while `Put` blocked. Both are 0
  unless a `Put` waits for the reserve's batch lock (a batch eviction) for more than
  `ClaimDeadline − FetchTimeout`; `FetchTimeout` is clamped below `ClaimDeadline`. AC-D1's "with
  faults" clause should name that bound, and AC-D2 should read a non-zero
  `want_claims{expired}` as long `Put` waits, not as a misconfigured timeout.
- **(T4.4 fix round 2) Signature-recovery errors are rejected, without a `pkg/postage` change.**
  This supersedes the round-1 follow-up "Signature-recovery stamp errors are not rejected".
  `deliver` calls the exported `postage.RecoverBatchOwner` itself.
- **(T4.4 fix round 2 → T4.5) Release the stripe peer while re-asks wait.** The `resolvePending`
  waits (up to 30 s per attempt) keep the stripe's per-peer lock and the peer in Striped (Decisions
  log, Low 3). If testnet shows rounds with such peers delayed, drop the peer lock and state
  during the waits.
- **(T4.4 fix round 2 → test infrastructure) streamtest's `FullClose` should wait for the remote
  handler.** libp2p's waits, so a Status (or any message) is handled before the sender's next
  stream. streamtest's returns at once, so handlers of one sender can run out of order. The
  rangesync tests work around it by waiting for `LastStatus`. Changing `pkg/p2p/streamtest` is
  outside this plan's scope.
- **(T4.4 fix round 2) Tests to run with `-race` on CI.** These ran with `-count=10 -cpu=1,2,4`
  instead, and `TestSREPStallRetry` and `TestRadiusIncreaseNoFetch` with `-count=60 -cpu=1,2,4`,
  with and without CPU load.
  - `pkg/rangesync/wantset`: `TestPeek`, `TestModelReject`.
  - `pkg/rangesync`: `TestFetchBadSignatureRejected`, `TestFetchPeekSkipsLimiter`,
    `TestFetchExpiredDuringPut`, `TestFetchTimeoutClamped`, and `TestFetch*` again
    (`wantCandidates` and `deliver` changed).
  - `pkg/rangesync/reconciler`: `TestStripeBadSignatures`, `TestStripeUnverified`. Re-run, because
    the test harnesses and `resolvePending` changed: `TestSREP*`, `TestRadius*`,
    `TestRejectedNotRefetchedEachRound`, `TestStripeDeferred*`, `TestStripe*`,
    `TestTransferStripes*`, `TestVerifyStripes*`, `TestWantSet*`.
- **(T4.4 round-2 review, Lows; recorded by the orchestrator at landing) Fetch accounting and tests (T4.5).**
  1. fetch.go ~L445-449: the "reserve already has the chunk" early return skips the post-`Put`
     expiry check. If the claim expires while `validStamp` waits on the batch store (≥ 15 s stall)
     and another fetch stores the chunk, the duplicate shows only in `cross_protocol_duplicates`,
     contradicting "≤ `want_claims{expired}` in every case". Fix: call `claimExpired` there too, or
     soften the wording.
  2. `TestFetchExpiredDuringPut` (fetch_result_test.go): the 50 ms claim deadline must cover claim →
     delivery; on a loaded CI `-race` run the delivery can be counted `late` and the test fails.
     Raise it to ≥ 500 ms.
  3. fetch.go ~L187: `Peek` doesn't count candidates already picked for the same Want, so near
     `MaxClaimsPerPeer`/`MaxClaims` all 250 IDs peek as granted and the limiter charges 250 tokens;
     "no fetch rate on refused IDs" is approximate. Doc wording.
  4. The bad-signature tests use a fake validator (`recoverStamp`), not `postage.ValidStamp` with a
     batch store; add one test through the real validator. A peer still pays nothing for rejected
     junk while this node spends a fetch token per listed junk ID (covered by the T4.5 follow-ups on
     scoring peers that don't deliver listed IDs).
- **(T4.5 part 1) Tests to run with `-race` on CI.** The whole reconciler package ran with
  `-count=5 -cpu=1,2,4`, and the new tests with `-count=30..40 -cpu=1,2,4`, also under CPU load.
  - `pkg/rangesync/reconciler`, new: `TestRangesyncOnlyGoroutines`, `TestDualModeConverges`,
    `TestDualModeSyncRate`, `TestPeerFSMRoundFetchDuringStripe`.
  - Re-run, because `round`, `session`, the stripe release and the machine changed:
    `TestPeerState*` (especially `TestPeerStateStripeDuringFetch`), `TestPeerFSM*`, `TestRound*`,
    `TestRoundOwnsFirstContact`, `TestStatusGossip*`, `TestStartupGuard*`, `TestSyncRate*`,
    `TestRadius*`, `TestTransferStripes*`, `TestVerifyStripes*`, `TestStripe*`, `TestWantSet*`,
    `TestSREP*`, `TestPeerJoinLeave`, `TestClose`.
- **(T4.5 part 1 → part 2) Defer topology rounds.** This is the "(T4.3) First Busy retry on topology
  rounds" item. Make the `changes` case of `loop` a trigger, and rework the tests that drive rounds
  through topology changes with `RoundInterval = 1 h`: `TestRoundOwnsFirstContact`,
  `radiusNet.round` and the `wants_test.go` harness. The start-up guard should stay disarmed for a
  peer that connects before the next round evaluates it.
- **(T4.5 part 1) Dual mode with the real puller.** It needs a reserve whose `SubscribeBin` follows
  its contents (the real storer, or a better mock) and a puller rate window on an injectable clock.
  `pkg/puller` is frozen, so T6's testnet runs with both flags on are where the combined `SyncRate`
  with the real puller gets checked.
- **(T4.5 part 1) `(*Session).pendingReplies` has test callers only** (`pkg/rangesync/session.go`,
  T2.x). `golangci-lint --tests=false --enable-only unused` flags it. Move it into
  `export_test.go`. It is outside the reconciler, so it was left alone.
- **(T4.5 part 1 review, Lows; recorded by the orchestrator at landing) For T4.5 part 2.**
  1. In-place FSM reset (`gossip.go` ~L222, `peers.go` ~L67): `st.fsm = newPeerFSM()` also clears
     `fetching`, which the old `peerState.fetching` survived. Fetching (F1 running) → hidden
     reconnect → Unsupported → re-admit → a stripe from Idle ends Idle, so a round session and a
     second fetch can start while F1 runs, and F1's `eventFetched` can end the new Fetching early.
     Fix: a generation number on `peerState` checked by session/release callbacks, or replace `st`
     on reset.
  2. `TestRangesyncOnlyGoroutines` cannot see inbound-triggered goroutines: the fetch goroutine
     `Wants` starts (`reconciler.go` ~L408) runs on the inbound handler goroutine (remote's pprof
     label under streamtest), and `AfterFunc` callbacks (`stripeClock`) escape the count. `doc.go`'s
     "O(neighbors)" doesn't hold for served-session fetches (one per served session; see the T4.4
     "concurrent fetch streams per peer" follow-up).
  3. "No pullsync streams opened" is vacuous in that harness (no puller); the real guarantee is in
     the pkg/node wiring tests.
  4. Dual-mode `SyncRate` with a stand-in puller is accepted (T6 follow-up already recorded).
  5. The remaining Phase-1 behavior is immediate topology rounds (T4.3 deferred it to T4.5); part 2
     must close it.
- **(T4.5 part 2a) Resolved: defer topology rounds.** This closes "(T4.2) First Busy retry on
  topology rounds (T4.3)", "(T4.3) First Busy retry on topology rounds (T4.5)", "(T4.5 part 1 →
  part 2) Defer topology rounds" and item 5 of the part-1 review Lows. See the Decisions log.
- **(T4.5 part 2a) Resolved: FSM in-place reset.** This closes item 1 of the part-1 review Lows
  (`peerState.gen`).
- **(T4.5 part 2a) Resolved: stripe bounds and timer.** This closes items 1 to 4 of "(T4.3 round-3
  review, Lows ...) Stripe bounds and timer (T4.5)".
- **(T4.5 part 2a) Resolved: release the stripe peer while re-asks wait.** This closes "(T4.4 fix
  round 2 → T4.5) Release the stripe peer while re-asks wait".
- **(T4.5 part 2a) Resolved: a Busy-only peer may verify.** This closes "(T4.3 fix round 3) A
  Busy-only peer counts as tried for verification (T4.5)".
- **(T4.5 part 2a → T4.6) Re-ask waits and the stripe lock.** While a stripe waits to ask again, a
  stripe reassigned to the same peer may take the lock and run its whole attempt, and the waiting
  stripe then waits for it on its own clock. T4.6 should check that bootstraps near
  `MaxClaimsPerPeer` do not time out stripes waiting for each other. If they do, hand the lock back
  to the waiting stripe first, or skip the lock for re-asks (they are fetches, not sessions).
- **(T4.5 part 2a) Tests to run with `-race` on CI.** These ran with `-count=20 -cpu=1,2,4`
  instead, and the reconciler package with `-count=5 -cpu=1,2,4`.
  - `pkg/rangesync/reconciler`, new: `TestPeerStateResetIgnoresStaleRuns`,
    `TestTopologyChangeFloor`, `TestStartupGuardNewPeer`, `TestStripeLocalBusyBudget`,
    `TestStripePeerSessionLimitIsPeerBusy`, `TestStripeBusyPauseFetchPending`,
    `TestStripeReaskReleasesPeer`, `TestStripeReaskTimeoutBoundsPeerWait`,
    `TestVerifyStripesBusyOnlyPeer`.
  - Reworked: `TestRoundOwnsFirstContact`, `TestStripeFetchBoundedByTimeout`,
    `TestStripeLocalBusy`, `TestStripeTimeoutPausedWhileBusy`, `TestStripeTimeoutAfterSlot`,
    `TestStripeBusyRetriesBounded`, `TestStripeBusyReassigns`, `TestRadius*` (`radiusNet.round`).
  - Re-run, because `loop`, `session`, the machine resets, `transferStripe`, `resolvePending` and
    `runStripe` changed: `TestPeerState*`, `TestRound*`, `TestStatusGossip*`, `TestStartupGuard*`,
    `TestStripe*`, `TestTransferStripes*`, `TestVerifyStripes*`, `TestWantSet*`, `TestSREP*`,
    `TestRejectedNotRefetchedEachRound`, `TestPeerJoinLeave`, `TestClose`, and the T4.5 part 1
    tests.
- **(T4.5 part 2a fix round 1 → T4.6) Busy-heavy stripes, extended.** This extends "(T4.3 fix
  round 2) T4.6: Busy-heavy stripes". Since part 2a, `ErrPeerSessionLimit` is the peer's Busy for
  stripes, so a stripe is reassigned after about 30 s of it. An honest lower peer sends every ID
  to an empty node (`session.go`, count 0), so its own session with this node holds the per-peer
  slot for long. Stripes assigned to such peers can then be pushed away and exhaust
  `maxStripeReassign`, which fails the bootstrap. T4.6 should measure this in an N = 17
  bootstrap. If it shows, count peer-session-limit Busy separately with a longer bound, or prefer
  peers that are not in a session with this node when planning stripes.
- **(T4.5 part 2a fix round 1) Re-ask waits and the stripe lock, narrowed.** This narrows "(T4.5
  part 2a → T4.6) Re-ask waits and the stripe lock". Verifications and bootstrap stripes now keep
  the lock, so the open case is the other transfers: radius halves and sessions' large ranges.
  Nothing is left open for verifications.
- **(T4.5 part 2a fix round 1) Tests to run with `-race` on CI.** These ran with
  `-count=20 -cpu=1,2,4` instead, and the reconciler package with `-count=5 -cpu=1,2,4`.
  - `pkg/rangesync/reconciler`, new: `TestStripeClockCheckedAfterWaits`,
    `TestStripeReaskClockCheckedAfterPeerWait`, `TestRoundDueBeforeTopologyChange`,
    `TestTopologyChangeDoesNotHoldRounds`, `TestStripeReaskRetakesLock`,
    `TestVerifyStripesShareVerifierReask`, `TestBootstrapStripeKeepsLockReask`.
  - Changed: `TestRoundOwnsFirstContact`, `TestTopologyChangeFloor`, `TestStartupGuardNewPeer`,
    `TestStripeReaskReleasesPeer`, `TestStripeReaskTimeoutBoundsPeerWait`.
  - Re-run, because `loop`, `follow`/`track`, `transferStripe` and `resolvePending` changed:
    `TestRound*`, `TestStatusGossip*`, `TestStartupGuard*`, `TestPeerState*`, `TestStripe*`,
    `TestTransferStripes*`, `TestVerifyStripes*`, `TestRoundBootstrap*`, `TestRadius*`,
    `TestWantSet*`, `TestSREP*`, and the T4.5 part 1 tests.
- **(T4.5 part 2b) Resolved: undelivered listed IDs are scored.** This closes "(T4.4 → T4.5)
  Undelivered IDs are not scored (§9 'bogus wants count against the peer')" and the last sentence of
  item 4 of the T4.4 round-2 Lows. See the Decisions log.
- **(T4.5 part 2b) Resolved: rejected-cache flush.** This closes "(T4.4 → T4.5) Rejected-cache
  flush" (`RejectFrom`, a per-peer share).
- **(T4.5 part 2b) Resolved: concurrent fetch streams per peer.** This closes "(T4.4 → T4.5)
  Concurrent fetch streams per peer (pre-existing)" (`maxFetchStreamsOut`) and item 2 of the T4.5
  part-1 review Lows (the `doc.go` wording; the goroutine count is the follow-up below).
- **(T4.5 part 2b) Resolved: Status spam is escalated.** This closes "(T4.1) Repeated Status spam is
  never escalated (T4.2)" and "(T4.2) Escalate Status spam (T4.5)".
- **(T4.5 part 2b) Resolved: local store errors no longer back off.** This closes "(T4.2 fix) Local
  store errors still count towards the backoff (T4.5)" (`rangesync.ErrLocal`).
- **(T4.5 part 2b) Resolved: fetch accounting and tests.** This closes items 1 to 4 of "(T4.4
  round-2 review, Lows; recorded by the orchestrator at landing) Fetch accounting and tests (T4.5)".
- **(T4.5 part 2b → T6) Fetch shortfall violations on testnet.** Honest peers should never be
  disconnected for a shortfall. Read the debug log reason "requested ids not delivered" (counted in
  `invalid_message_total`) around batch expiries and radius increases. If honest peers show up,
  raise the tolerance for large fetches, or defer the judgment until the peer's next Status. A
  dedicated counter would make this readable from metrics.
- **(T4.5 part 2b → T4.6) Junk with unknown-batch stamps is not scored.** A peer can list IDs of a
  made-up batch and deliver chunks with such stamps. They are rejected (`postage.ErrNotFound`) but
  not counted, because the expired-batch skew (Q7) looks the same; its share of the rejected cache
  and the fetch rate bound the cost. If T4.6's Q7 measurement shows how many unknown-batch
  rejections an honest skew causes, such rejections can count with a larger tolerance of their
  own.
- **(T4.5 part 2b) One wants fetcher per peer.** Served sessions' fetches and retry batches each
  take a goroutine, which queue up per peer while the fetch rate lags; their streams are capped. A
  single fetcher per peer with a bounded queue would make the goroutines O(neighbors), but `Wants`
  counts a catch-up fetch per served session (`catchUpFetches`), which the `SyncRate` guard reads,
  so the merge needs that accounting reworked.
- **(T4.5 part 2b) Stripes and `ErrLocal`.** A local store error still ends a stripe attempt and
  reassigns the stripe, unscored. It could restart on the per-stripe local budget, as `localBusy`
  does, once a restart waits before it retries.
- **(T4.5 part 2b) Tests to run with `-race` on CI.** These ran with `-count=20 -cpu=1,2,4` instead,
  and `pkg/rangesync` and the reconciler package with `-count=5 -cpu=1,2,4`.
  - `pkg/rangesync`, new: `TestFetchExpiredDuringStampCheck`, `TestFetchPeekCountsPicked`,
    `TestFetchBadSignatureValidStamp`, `TestLocalStoreErrors`, `TestReconcileLocalStoreError`,
    `TestFetchLocalStoreError`, `TestFetchRejectedShare`, `TestFetchStreamsPerPeer`,
    `TestStatusSpamEscalated`, `TestFetchShortfall*`. Changed: `TestFetchExpiredDuringPut`,
    `TestFetchManyWants`. Re-run, because `FetchWithResult`, `deliver` and `handleStatus` changed:
    `TestFetch*`, `TestStatus*`, `TestReconcile*`.
  - `pkg/rangesync/wantset`, new: `TestPeekPicked`, `TestRejectFrom_PeerShare`,
    `TestReject_DefaultShare`, `TestModelRejectShare`. Changed: `TestModelReject`. Re-run:
    `TestConcurrentReject`, `TestReject*`.
  - `pkg/rangesync/reconciler`, new: `TestStatusGossipNeverEscalated`,
    `TestStripeBadSignaturesScored`. Changed: `TestStripeBadSignatures`,
    `TestPeerStateLocalCauseNoBackoff`. Re-run, because every fetch now waits for a stream slot and
    may end a violation: `TestSREP*`, `TestWantSet*`, `TestRejectedNotRefetchedEachRound`,
    `TestStripe*`, `TestTransferStripes*`, `TestVerifyStripes*`, `TestRadius*`, the T4.5 part 1
    tests (`TestDualMode*`, `TestRangesyncOnlyGoroutines`).
- **(T4.5 part 2b fix round 1) Resolved: the review's Medium 1, Medium 2, Low 3 and Low 4.** See the
  Decisions log. They amend the part 2b resolution of "(T4.4 → T4.5) Undelivered IDs are not
  scored" and "(T4.4 → T4.5) Concurrent fetch streams per peer".
- **(T4.5 part 2b fix round 1 → T4.6) Unknown-batch junk fills the rejected shares (review Low 5).**
  This extends "(T4.5 part 2b → T4.6) Junk with unknown-batch stamps is not scored". Four or more
  Sybils that deliver unscored unknown-batch junk fill every share of the rejected cache, so honest
  rejections last about 10 s instead of `RejectTTL`. The fetch rate bounds it, so it is accepted
  until Q7's measurement.
- **(T4.5 part 2b fix round 1 → T6) Shortfall offenses on testnet.** Read the first offenses (the
  reconciler's debug log, reason `ErrFetchShortfall`) and the violations ("requested ids not
  delivered" in `invalid_message_total`). Honest peers should show none; a blatant one around a
  batch expiry or a radius change would call for a wider excuse.
- **(T4.5 part 2b fix round 1) Fresh junk below half of a fetch is never counted.** The deferred
  verdict needs an ID listed again. If testnet shows peers that list a steady share of fresh junk,
  count the misses per peer over a longer window as well.
- **(T4.5 part 2b fix round 1) Tests to run with `-race` on CI.** These ran with
  `-count=20 -cpu=1,2,4` instead, and the three packages with `-count=5 -cpu=1,2,4`.
  - `pkg/rangesync`, new: `TestShortfallBook`, `TestFetchShortfallBlatant`,
    `TestFetchShortfallRelisted`, `TestFetchShortfallInvalidStamps`, `TestFetchShortfallExcuseOnce`,
    `TestFetchShortfallStatusGone`, `TestFetchSkipsOutsidePeerRoot`, `TestFetchResetBudget`,
    `TestStatusRootContainsPeer`, `TestFetchStreamsLaggingHandlers`. Changed:
    `TestFetchShortfallHonestChurn`, `TestFetchShortfallAfterViolation`, `TestFetchManyWants`,
    `TestReconcileAdversarial`, `TestReconcileOpeningOutsideRoot`. Re-run: `TestFetch*`, `TestStatus*`.
  - `pkg/rangesync/reconciler`, changed: `TestStripeBadSignaturesScored`. Re-run, because every fetch
    now looks up the peer's root and may end with `ErrFetchShortfall`: `TestStripe*`, `TestSREP*`,
    `TestWantSet*`, `TestVerifyStripes*`, `TestRadius*`, `TestDualMode*`.
- **(T4.5 part 2b fix round 2) Resolved: the round-2 review's Medium 1, Medium 2, Low 2, Low 3, Low 4
  and Low 5.** See the Decisions log.
- **(T4.5 part 2b fix round 2) Junk a peer can still have fetched unscored, per Sybil.** This
  replaces fix round 1's "Fresh junk below half of a fetch is never counted".
  - Fresh junk under `blatantMinIDs` (64) IDs per fetch is never listed again, so it is never
    counted, and is too small to be blatant. At a peer's served-session rate (one session per
    `PeerSessionInterval`, 2.5 s, one at a time), that is up to 63 IDs per 2.5 s, about 25 IDs/s,
    some 2.5% of `DefaultFetchRate`. It is the largest share left (review Low 1, not fixed; see the
    Decisions log).
  - Resets: 4 Wants, then one Want (250 IDs) per minute, about 4 IDs/s.
  - Root moves: at most 500 IDs per 10 min, under 1 ID/s.
  - Fresh junk under half of each larger fetch: the peer must fill the rest with chunks this node
    lacks.
  - Unknown-batch junk: T4.6.

  Testnet cannot observe a deliberate attacker of this kind: its pattern looks like honest SOC and
  GSOC rewrites. A fix would need a signal that tells a rewrite from junk, for example the
  responder delivering the current stamp of a rewritten address, which the frozen wire (G2) does
  not carry.
- **(T4.5 part 2b fix round 2) Tests to run with `-race` on CI.** These ran with
  `-count=20 -cpu=1,2,4` instead, and the three packages with `-count=5 -cpu=1,2,4`.
  - `pkg/rangesync`, new: `TestFetchResetAfterOneDelivery`, `TestFetchResetHonest`,
    `TestFetchShortfallExcuseOscillation`, `TestFetchShortfallAfterPeerViolation`. Changed:
    `TestFetchResetBudget`, `TestShortfallBook`. Re-run: `TestFetchShortfall*`, `TestFetchStreams*`,
    `TestFetchSkipsOutsidePeerRoot`.
  - `pkg/rangesync/reconciler`, new: `TestVerifyStripesVerifierOutsideRoot`. Re-run:
    `TestVerifyStripes*`, `TestStripeBadSignatures*`.
- **(T4.5 part 2b fix round 3) Resolved: the round-3 review's Medium 1, Low 2 (the figure), Low 3
  and Low 4.** See the Decisions log.
- **(T4.5 part 2b fix round 3) Junk a peer can still have fetched unscored, per Sybil.** This
  replaces fix round 2's entry of the same name.
  - Fresh junk under `blatantMinIDs` (64) IDs per fetch: up to 63 IDs per served session (one per
    `PeerSessionInterval`, 2.5 s), about 25 IDs/s, plus up to 63 per session of the rounds this node
    starts (one per peer per round, rounds at least `MinRoundInterval`, 5 s, apart), about 12.6 IDs/s,
    which round 2 left out. About 38 IDs/s in all, some 3.8% of `DefaultFetchRate`. It is the largest
    share left (round 2, Low 1, not fixed).
  - Root moves: about 16,000 IDs per 10 min for a `LargeDiff` call, about 27 IDs/s (round 2 said
    500 IDs per 10 min; see the Decisions log, round 3 Low 2).
  - Resets, now including stream opens that fail: 4 Wants, then one Want (250 IDs) per minute, about
    4 IDs/s. Before round 3 a peer that failed every open had about 100 IDs/s.
  - Fresh junk under half of each larger fetch: the peer must fill the rest with chunks this node
    lacks.
  - Unknown-batch junk: T4.6.
  - A violation mark carried over by the recreated-Status race: up to 1 min unjudged per disconnect,
    not reachable on demand (Decisions log, round 3 Low 3).

  The bounded rows add up to about 70 IDs/s per Sybil, some 7% of `DefaultFetchRate`. Round 2's note
  on testnet still applies.
- **(T4.5 part 2b fix round 3 → p2p) This node's own stream limit charges its peers.** When this
  node's libp2p resource manager refuses an outbound stream (`network.ErrResourceLimitExceeded`, at
  10,000 outbound streams), every fetch stream open fails, and each is charged to the peer as a reset
  (Decisions log, round 3 Medium 1). A `pkg/p2p` sentinel for a refusal on this node's side would let
  `fetch` skip the charge. Out of scope here (no `pkg/p2p` change).
- **(T4.5 part 2b fix round 3; orchestrator request) Per-peer fetch credit (before testnet).** A
  peer's share of the fetch rate would refill only with chunks it delivered. A peer that lists junk
  then runs out of fetch rate whatever path it uses (small fresh junk, root moves, resets and failed
  opens, unknown-batch junk), so this would bound every junk-lister path at once and could replace
  the shortfall heuristics (blatant, relisted, the root-move excuse, the reset budget). Not
  implemented. To settle first: the initial credit of a new or reconnected peer (Sybils reconnect),
  catch-up fetches (a bootstrap fetches from few peers at full rate), and how it composes with the
  node-wide fetch limiter and the `SyncRate` guard.
- **(T4.5 part 2b fix round 3) Tests to run with `-race` on CI.** These ran with
  `-count=20 -cpu=1,2,4` instead, and the three packages with `-count=5 -cpu=1,2,4`.
  - `pkg/rangesync`, new: `TestFetchStreamOpenCharged`, `TestFetchStreamOpenHonest`,
    `TestFetchForgetIfGone`, `TestViolationMarks`. Changed: `TestFetchShortfallAfterPeerViolation`.
    Re-run, because `fetch`, `initiatorViolation`, `violation`, `judgeShortfall` and `chargeReset`
    changed: `TestFetch*`, `TestStatus*`, `TestReconcile*`.
  - `pkg/rangesync/reconciler`, re-run, because a fetch whose stream fails to open may now end a
    violation: `TestStripe*`, `TestVerifyStripes*`, `TestSREP*`, `TestWantSet*`, `TestRadius*`.
- **(T4.5 part 2b round-3 review, Lows; recorded by the orchestrator at landing) For T4.5 part 2c.**
  1. **Sustained open failures disconnect an honest peer.** A failed open releases the stream slot at
     once, so queued Wants retry within milliseconds and any lasting condition (the peer's 5000
     system inbound streams full — no per-peer cap, so one third-party connection can fill them)
     fails every queued open: 5 opens → violation, from every honest neighbor, again after every
     reconnect. Fix: open the stream before `fetchLimiter.Wait` and `claim` (fetch.go ~L202-218), so
     a failed open holds no claims and no fetch rate and needs no charge (`handleFetch` has no
     deadline for reading the Want). This also removes Low 2's false positive.
  2. **This node's own 10k outbound stream limit is charged to the peer** (already a follow-up). Hard
     to reach (≤ 2 streams per peer; remote inducement needs many Sybils). The rcmgr error appears to
     wrap `network.ErrResourceLimitExceeded` through bee's `%w` chain while a remote reset unwraps
     only to `ErrReset`, so it can be told apart without a pkg/p2p change; verify.
  3. **Docs and test gap.** "The headers exchange outlasts FetchTimeout only when HeadersRWTimeout ≥
     FetchTimeout" is wrong: basichost's `IdentifyWait` and the full multistream negotiation (a peer
     omitting fetch from identify) obey only the fetch ctx, so a 15 s open stall is reachable and is
     an immediate violation; a headers stall is always charged. Add a test (held open, fetchTimeout
     100 ms → `ErrInvalidMessage`, one Fail, disconnected). That path has no Status gate (tiny window).
     The cross-path double-Fail case (requester mark, then responder violation, same peer) is
     untested.
- **(T4.5 part 2b round-3 review) The per-peer fetch credit must survive reconnects.** A disconnect
  clears reset tokens, missing-ID memory, offenses and the session limiter, and reconnects are cheap
  (10/s per IP), so a self-disconnecting Sybil gets ≥ 1 unscored failed Want (250 IDs) per cycle
  (≈ 100+ IDs/s), above the ≈ 70 IDs/s residual that assumes a connected Sybil. The credit (follow-up
  above) should be a hard gate before testnet and keyed so it survives reconnects, like the root excuse.
- **(T4.5 part 2c) Resolved: the round-3 review's Lows for part 2c.** This closes items 1 to 3 of
  "(T4.5 part 2b round-3 review, Lows; recorded by the orchestrator at landing) For T4.5 part 2c"
  (the open before the fetch rate and the claims; this node's own limit as `ErrLocal`; the docs, the
  held-open test, the Status gate, the cross-path test). See the Decisions log.
- **(T4.5 part 2c) Resolved without a `pkg/p2p` change: this node's own stream limit.** This closes
  "(T4.5 part 2b fix round 3 → p2p) This node's own stream limit charges its peers". No failed open
  is charged any more, and the refusal is told apart through `network.ErrResourceLimitExceeded`.
- **(T4.5 part 2c) Resolved: `peers{state}` gauge.** This closes "(T4.2) `peers{state}` gauge
  (T4.5)" (`rangesync_peers{state}`).
- **(T4.5 part 2c) Resolved for session ranges: stripe registry trimming.** This closes "(T4.2)
  Stripe registry trimming (T4.3)", "(T4.3) Stripe registry trimming (T4.5)" and "(T3.2 part 2)
  Registry granularity". Bootstrap and radius jobs are still not trimmed; their rounds retry them.
- **(T4.5 part 2c) Resolved for the wider root: session-root summaries.** This closes "(T4.2) Compare
  session-root summaries (T4.3)" and "(T4.3) Compare session-root summaries in `sameDigest` (T4.5)".
  The narrower side still compares as different; only a wire change could fix that (G2 freezes it).
- **(T4.5 part 2c) Resolved: cleanups.** This closes "(T4.5 part 1) `(*Session).pendingReplies` has
  test callers only" and "(T4.4) `fetch_skipped_total{reason="held"}` is ambiguous (pre-existing)".
- **(T4.5 part 2c → after phase 4) `Local.Symbols` cancellation.** Still not trivial, as "(T4.3)
  `Local.Symbols` cancellation (T4.5 or later)" found: it needs a context in the `Local` interface
  (the T1.8 contract), in `local.scanIDs` and in every session-test fake. The scan stays bounded
  (`ScanLimit`, `MaxScansPerMessage`).
- **(T4.5 part 2c) Session and Status opens refused by this node's resource manager.** Only the fetch
  open classifies `network.ErrResourceLimitExceeded` as `ErrLocal`. A refused reconcile stream backs
  off the peer's sessions (`eventError`), and a refused Status stream its gossip (`statusRetry`).
  One `errors.Is` at each open would make them local causes too.
- **(T4.5 part 2c → T4.6) A peer that fails every fetch open keeps its sessions.** Failed opens are
  not charged, so such a peer is neither scored nor backed off: each round may run a session with it
  whose fetch then fails. The IDs it lists are not claimed, so other peers serve them, and its
  sessions are bounded by the rounds. If T4.6 or testnet shows the cost, a backoff of the peer's
  sessions after failed opens (not a score) would bound it.
- **(T4.5 part 2c → T4.6) A failed verification leaves the deliverer unjudged.** A verifier whose
  opens fail (or whose stream resets) ends the verification with an error, and `verifyStripe` judges
  neither it nor the deliverer. A colluding verifier can so shield a withholding deliverer from its
  score; what the deliverer withheld comes back in later rounds. Retrying the verification with
  another verifier would close it.
- **(T4.5 part 2c) `peers{state}` has no backoff breakdown.** A peer backing off counts as idle. If
  T6 needs it, a separate gauge of peers past their retry time would show it.
- **(T4.5 part 2c → T4.6) Trimmed stripe parts are jobs of their own.** Each part is planned across
  its covering peers and verified on its own, so trimming around deep registered ranges multiplies
  stripe sessions. T4.6 should check that small parts cost less than the rounds they save.
- **(T4.5 part 2c) Tests to run with `-race` on CI.** These ran with `-count=20 -cpu=1,2,4` instead,
  and the three packages with `-count=5 -cpu=1,2,4`.
  - `pkg/rangesync`, new: `TestFetchStreamOpenNotCharged`, `TestFetchStreamOpenLocalLimit`,
    `TestFetchStreamOpenHeld`, `TestFetchInboundFullNotDisconnected`, `TestFetchLimiterWaitAfterOpen`,
    `TestFetchStalledHolderGone`, `TestRangeSummary`, `TestViolationMarks` (two new cases). Changed:
    `TestFetchClaimRechecksReserve`, `TestFetchStalledHolder`, `TestFetchWithResultFailed`,
    `TestFetchSkipsHeld`, `TestFetchSkipsClaimed`. Re-run, because `want` and `fetch` changed:
    `TestFetch*`, `TestFetchStreamOpenHonest`, `TestFetchForgetIfGone`, `TestFetchResetBudget`.
  - `pkg/rangesync/reconciler`, new: `TestPeersGauge`, `TestEnqueueStripesTrimmed`,
    `TestEnqueueStripesTrimmedTransferred`, `TestEnqueueStripesTrimmedBound`,
    `TestRoundSessionRootDigest`. Changed: `TestEnqueueStripesDedup`. Re-run, because the round's
    digest comparison and the stripe registry changed: `TestRound*`, `TestSessionRecheck*`,
    `TestRadius*`, `TestStripe*`, `TestTransferStripes*`, `TestVerifyStripes*`, `TestSREP*`,
    `TestWantSet*`, `TestPeerState*`.
- **(T4.5 part 2c fix round 1) Resolved: the review's High and Lows 1 to 3.** See the Decisions log.
  This also resolves "(T4.5 part 2b) One wants fetcher per peer" for the fetches of served sessions.
- **(T4.5 part 2c fix round 1) A probe can still hold `SyncRate` up part of the time.** A peer that
  stalls every fetch open holds its coalesced catch-up batch in flight for the fetch timeout once per
  open backoff: about 5% of the time at `maxOpenBackoff`, more in its first minutes, and k such
  neighbors add up. Counting a fetch as catch-up only once its Want went out (a hook from rangesync),
  or a shorter open timeout than `FetchTimeout`, would close it.
- **(T4.5 part 2c fix round 1 → p2p) A sentinel for this node's own resource refusal.** Low 1 dropped
  the `ErrLocal` classification, which needed `github.com/libp2p/go-libp2p/core/network`. A `pkg/p2p`
  sentinel would let a refused fetch open skip the per-peer open backoff (or back off node-wide), and
  serves "(T4.5 part 2c) Session and Status opens refused by this node's resource manager" too.
- **(T4.5 part 2c fix round 1) Tests to run with `-race` on CI.** These ran with `-count=20 -cpu=1,2,4`
  instead, and the three packages with `-count=5 -cpu=1,2,4`.
  - `pkg/rangesync`, new: `TestFetchStalledOpensBackOff`, `TestFetchStreamWaitersBounded`,
    `TestFetchBackoffSlotsBusy`, `TestOpenGate`. Changed: `TestFetchInboundFullNotDisconnected`,
    `TestFetchStreamOpenNotCharged`, `TestFetchStreamOpenHeld`, `TestFetchStreamOpenHonest`. Re-run,
    because `want` and the open changed: `TestFetch*`.
  - `pkg/rangesync/reconciler`, new: `TestWantsServedFetcher`, `TestWantsStalledOpens`,
    `TestStartupGuardNestedInitiator`. Changed: `TestSyncRateSteadyStateUploads`. Re-run, because
    `Wants` and the round's takeover changed: `TestStartupGuard*`, `TestSyncRate*`, `TestRound*`,
    `TestWantSet*`, `TestSREP*`, `TestDualMode*`, `TestResponderWantsFetched`.
- **(T4.5 part 2c fix round 2) Resolved: the review's High and Lows 1, 2, 4 and 5.** See the
  Decisions log. This resolves "(T4.5 part 2c fix round 1) A probe can still hold `SyncRate` up part
  of the time": a probe, a held Want or any catch-up fetch that stores nothing now holds it for at
  most `catchUpGrace` (2 s) per catch-up episode, node-wide.
- **(T4.5 part 2c fix round 2 → T4.6) The stripe registry hold is attacker-pinnable (by inspection,
  not measured).** A registered stripe range holds `SyncRate` at least at `StartupGuardRate` until its
  job (transfer and verification) ends (T3.5, unchanged here). A peer this node initiates sessions
  with (it picks a higher overlay) can report, for a range, a count above this node's by more than
  `LargeDiff`, which hands the range to the Striper; a stripe of the job planned on that peer, or on
  another Sybil covering it, can be held without progress for up to `sessionTimeout` (10 min) per
  attempt, through `maxStripeReassign` reassignments, and the next round's session can register the
  range again. A radius decrease's half registered for a transfer is held the same way; unregistered,
  it is bounded by `radiusHold` per decrease, and decreases are this node's own. The start-up guard is
  bounded by `StartupGuardTimeout` per start. A fix would bound the registry hold like the catch-up
  grace (hold only while a registered job stores chunks, past a grace), or demand that a large count
  difference be confirmed by a second peer before striping.
- **(T4.5 part 2c fix round 2) The rate window can be kept above 0 by delivering accepted chunks.** A
  catch-up that stores one chunk per window keeps `SyncRate` above 0. A session counts as catch-up
  from `CatchUpMinDiff` wants, and a fetch of 64 or more requested IDs with half of them missing is a
  blatant violation, so an attacker must deliver about half of each such session's wants: about 128
  chunks this node lacks, in its radius, with valid stamps, per window (fresh uploads, since this
  node holds what its neighbors hold). Tolerated: the node then really stores new data. If T6 shows it
  matters, counting only chunks that a second peer also lists would close it.
- **(T4.5 part 2c fix round 2 → T4.6) Measure the first-chunk latency of catch-up fetches.** A
  catch-up whose first chunk takes longer than `catchUpGrace`, with no catch-up chunk in the window,
  reads 0 until it arrives. If T4.6 or testnet shows first chunks near 2 s, raise the grace; it bounds
  an attacker the same way, once per episode.
- **(T4.5 part 2c fix round 2) Correction to "(T4.5 part 2c) Tests to run with `-race` on CI".**
  `TestFetchStreamOpenLocalLimit` is gone since fix round 1 (its case moved into
  `TestFetchStreamOpenNotCharged`); leave it out of that list.
- **(T4.5 part 2c fix round 2) Tests to run with `-race` on CI.** These ran with `-count=20
  -cpu=1,2,4` instead, and the three packages with `-count=5 -cpu=1,2,4`.
  - `pkg/rangesync`, new: `TestFetchWithProgress`, `TestFetchDisconnectClearsOpenBackoff`. Changed:
    `TestOpenGate`. Re-run, because the fetch path and the open gate changed: `TestFetch*`.
  - `pkg/rangesync/reconciler`, new: `TestSyncRateStallingPeers`, `TestSyncRateCatchUpStoring`,
    `TestWantsServedChurn`, `TestWantsServedBatchesNotRequeued`, `TestTransferStripesOpenBackoff`,
    `TestStripePlanAvoidsOpenBackoff`. Changed: `TestWantsStalledOpens`, `TestSyncRateLargeCatchUp`.
    Re-run, because `SyncRate`, the catch-up count, the served queue and the stripe attempts changed:
    `TestSyncRate*`, `TestStartupGuard*`, `TestWantSet*`, `TestWants*`, `TestSREP*`, `TestDualMode*`,
    `TestTransferStripes*`, `TestStripe*`, `TestVerifyStripes*`, `TestRoundBootstrap*`, `TestRadius*`,
    `TestResponderWantsFetched`.
  - `pkg/node`: `TestCombinedSyncerBothDrivers`.
- **(T4.5 part 2c round-2 review; recorded by the orchestrator at landing) PRE-TESTNET BLOCKER: stripe
  registry SyncRate pin.** The round-2 SyncRate fix bounds fetch-path pinning to ≤ 2 s per catch-up
  episode, but the stripe-registry hold (reconciler.go ~L579) is unconditional: a higher-overlay peer
  reports a count differing by > `LargeDiff` (session.go ~L193, its own claim), a stripe is
  registered, and SyncRate stays ≥ guard while the stripe stalls — storage incentives off and the
  radius pinned, for free (`pkg/puller` can't be pinned this way). Fix direction: hold for a stripe
  only while its job stores chunks, past a grace (as for fetches). Must be fixed before any testnet
  cohort enables rangesync (with the per-peer fetch credit follow-up). This supersedes the
  "stripe registry hold attacker-pinnable" follow-up's T4.6 placement.
- **(T4.5 part 2c round-2 review, Lows; recorded by the orchestrator at landing)**
  1. Honest window: without the grace, SyncRate reads 0 from the session decision to the first stored
     chunk; an attacker who drained the node-wide grace (or loads the fetch limiter / holds claims)
     stretches it. Bounded: only non-striped catch-ups (< 65,536 missing, ≤ 1.6% of a 2^22 reserve);
     reserve-worker harm needs the count within 65k of the 50% threshold and self-corrects; `pkg/puller`
     has the same window. Accepted; T4.6 should measure first-chunk latency.
  2. The "~128 accepted chunks per window" follow-up is wrong: one accepted chunk then a reset per
     10 min (no missing IDs recorded; the reset budget covers it) keeps the rate > 0 — about one
     stamped chunk per window per victim, unscored (matches `pkg/puller`'s one fresh chunk per
     reconnect). Optional fix: `rate.Add(-n)` for a Want ending with ≥ half its claims undelivered.
  3. Tests: `TestFetchWithProgress` lacks the `ErrOverwriteNewerChunk` and already-in-reserve
     branches; no test pins one-chunk-then-reset; no test of an honest catch-up after the grace was
     drained. followup-fuzzing.md still says `errOpenBackoff` (now `ErrOpenBackoff`).
- **(T4.6 part 1 → part 3) Zero-difference sessions.** At the default load, about a third or more of
  the `pairwise_diff` samples are 0: 39 of 116 (N = 5), 159 of 436 (N = 9) and 648 of 1508 (N = 17).
  All of them fall one `StatusMinInterval` after a batch converged. A node's own fetches change its
  root, and the debounced signal triggers its round. That round meets its peers' Statuses from before
  they fetched, because their pushes arrive in the same instant. The race is real, and network
  latency widens it. Each such session costs a Fingerprint and `MinBatch` symbols per pair. Part 3
  (G6) should measure the share at the reference load (median `pairwise_diff` ≥ 20) and decide
  whether a root change caused only by this node's rangesync fetches should trigger a round, or
  whether such a round should wait for the peers' Statuses.
- **(T4.6 part 1 → part 3) Metadata per upload is well above §7.8.** At the default load (8 uploads
  per batch, median `pairwise_diff` 1–4; the low-load regime of AC-B1b), `bytes_metadata{out}` per
  upload is about 2.4 KB (N = 5), 7.4 KB (N = 9) and 19.5 KB (N = 17), against §7.8's 2.0–2.5 KB and
  4.6–5.9 KB. With 120 uploads per batch (median 54 and 36) it is 5.9 KB and 16.3 KB. The figures
  include the Status gossip (each changed node to its N − 1 peers) and the zero-difference sessions
  above. Part 3 should break them down and compare at the reference load, averaged over seeds (byte
  counts vary by about 10% between runs).
- **(T4.6 part 1 → part 3) Busy, collisions and stale sessions at N = 17.** In the logged runs:
  `sessions_total{busy}` 4–5, `session_limit` 1–4 and `stale` 130–150 per run (32 uploads). They make
  some batches take a second round. This is input to "(T4.3) Initiated-session share (T4.6)" and
  "(T4.3 fix) T4.6: check the stripe/round split".
- **(T4.6 part 1 → part 3) What the harness cannot vary yet.**
  - Compile-time constants: the cache sizes (Q2), `MinBatch`, `CatchUpMinDiff`, `StatusMinInterval`
    and `StatusMaxInterval`, and the request-driven vs streamed choice (Q3). Sweeping them needs a
    production change (an option, or constants made variables). What `Options` exposes can be swept
    through the `Reconciler` and `Service` hooks: the round bounds, `LeafBits`, `FetchRate`,
    `FetchTimeout`, `StripeTimeout`, `StartupGuardTimeout` and `Jitter`.
  - Scenarios: radius changes, bootstraps and stripes, batch expiry skew (Q7), churning or Busy-heavy
    peers. Every node starts at radius 0 with the same base set. Part 3 adds these for the earlier
    T4.6 measurement follow-ups: Q7 under skew, retry granularity, deferred stripes, Busy-heavy
    stripes, re-ask waits, trimmed stripe parts, first-chunk latency of catch-ups, the stripe
    registry, and the initiated-session share and stripe split.
- **(T4.6 part 1) Proposal §10 and T6.0 wording.** §10's `pairwise_diff` ("histogram of n_Δ per
  session") is now observed by both sides of a session, and `pairwise_diff_half` remains next to it.
  AC-C2 and AC-K5 read `pairwise_diff`. A neighborhood-wide aggregate counts each session twice, so
  its quantiles are right but its sample count is twice the sessions. For Q9, the check is the share
  of samples in `le="255"`.
- **(T4.6 part 1) Tests to run with `-race` on CI.** These ran with `-count=10 -cpu=1,2,4` instead.
  - `pkg/rangesync`:
    - new: `TestNeighborhoodConverges`, `TestNeighborhoodSREP`, `TestNeighborhoodRoundBounds`,
      `TestPairwiseDiffBuckets`;
    - changed: `TestProtocolConverges`, `TestSessionConverges`, `TestSessionExclusionCap`,
      `TestSessionSymbolsStaleStamp`, `TestSessionSymbolsExhausted`, `TestSessionSymbolsCorrupt`.
  - `pkg/rangesync/reconciler`: `TestRoundIntervalBounds`.
  - Under `-race`, raise `simOptions.Settle` if a convergence bound fails: a clock moved too early
    only lengthens a convergence.
- **(T4.6 part 1 fix round 1 → part 3, G6) The symbol overshoot is `GrowthFactor`'s, not the hint's.**
  With the serving-side hint (Decisions), N = 17 at 120 uploads per batch still costs 3.3–3.6
  symbols per difference and 55–61% of pullsync's metadata (AC-B1 ≤ 50%), and N = 9 2.6–3.0 and
  68–75%. Runs with the hint and other constants (120 per batch, 2–3 seeds each; symbols per
  difference, share of pullsync's metadata, `More`s per upload):

  | constants | N = 9 | N = 17 |
  |---|---|---|
  | as now (`BatchFactor` 1.6, `GrowthFactor` 4) | 2.9–3.0, 75–78%, 0.20 | 3.1–3.5, 53–59%, 0.76 |
  | `GrowthFactor` 2 | 2.0–2.1, 55–59%, 0.32 | 2.2–2.3, 40–43%, 1.07 |
  | `GrowthFactor` 2, no hint | 2.1–2.2, 55–60%, 1.0 | 2.1–2.2, 40–42%, 3.2 |
  | `BatchFactor` 2.0 | 2.4–2.5, 63–68% | 3.0–3.2, 54–58% |
  | both | 2.1, 58–59% | 2.4–2.5, 44% |

  At 8 uploads per batch, `GrowthFactor` 2 gives N = 9 2.7–3.3 and 88–96%, N = 17 3.0–3.3 and 71–72%
  (`MinBatch` per session dominates). So `GrowthFactor` 2 meets AC-B1 at N = 17 but not at N = 9,
  where n_Δ per upload is also above p(N − p); the hint keeps its round trips near today's. T2.6 kept
  4 on the worked examples (round trips against overshoot); part 3 should decide it under
  `simOptions.Delay`, which now times the round trips (Q3), and check N = 9 against §7.8's p(N − p)
  pairs.
- **(T4.6 part 1 fix round 1) Correction: metadata per upload.** This supersedes the last sentence of
  "(T4.6 part 1 → part 3) Metadata per upload is well above §7.8": Status gossip and zero-difference
  sessions are not what puts it above §7.8. With 120 uploads per batch, Status is 53 B (N = 9) and
  199 B (N = 17) per upload, about 1%, and reconcile 4.8–5.6 KB and 14.3–15.2 KB: the excess is
  symbols, 2.8–3.4 per difference against 1.35–1.72 (above), on n_Δ per upload somewhat above p(N −
  p) (the review: 48–56 against 42 at N = 17). With 8 per batch, Status is 0.8 KB and 3.1 KB per
  upload (10–14%), zero-difference sessions 2–4% (next entry), and the rest is mostly `MinBatch`
  streams for one to six differences.
- **(T4.6 part 1 fix round 1) Correction: zero-difference sessions.** This supersedes "Each such
  session costs a Fingerprint and `MinBatch` symbols per pair" in "(T4.6 part 1 → part 3)
  Zero-difference sessions". Most are equal-digest sessions: an opening Fingerprint and an empty
  Message, about 45 B. Of the sessions whose initiator reported 0, 13 of 101 (N = 9) and 37 of 231
  (N = 17) carried a `MinBatch` stream that decoded to no difference (a side changed since its
  Fingerprint), with 8 uploads per batch; none of 70 and 235 with 120. Together 2–4% of the metadata
  at 8 per batch. The rest of that entry (the race, part 3's decision) stands. With `Delay` 50–100 ms
  the share of such sessions rises (half of the sessions at N = 5 and 9); part 3 should measure it
  there.
- **(T4.6 part 1 fix round 1) Correction: §10 and T6.0 wording of `pairwise_diff`.** This supersedes
  "its quantiles are right" in "(T4.6 part 1) Proposal §10 and T6.0 wording". A neighborhood-wide
  aggregate mixes in the serving side's samples, which are approximate while the neighborhood
  changes (Decisions, Medium 1): the medians matched in every run, the p99 did not (up to 97 against
  80 at N = 17 with 120 per batch). AC-C2, AC-K5 and Q9 (the share in `le="255"`) are unaffected at
  these loads; a dashboard that needs the tail should read the decoding side, which a metric label
  could expose (a T6.0 decision, not a wire change).
- **(T4.6 part 1 fix round 1 → T6.4) AC-V1 against the 15 s bound.** AC-V1 says 2 × `RoundInterval`
  (10 s at the minimum); the simulation's bound is 2 × `StatusMinInterval` + `MinRoundInterval` =
  15 s, which the derivation reaches (Decisions, Low 4), with 4 rounds of a node in the window.
  T6.4 should reconcile AC-V1's wording with it (or count the Status latency as §8.2 does).
- **(T4.6 part 1 fix round 1 → part 3) What the harness cannot vary or time.** This extends
  "(T4.6 part 1 → part 3) What the harness cannot vary yet".
  - A `MinRoundInterval` below 5 s: `PeerSessionInterval` and `sessionBackoffBase` stay tied to
    `rangesync.MinRoundInterval`, so production would rate-limit such a node; a sweep needs those
    to follow the option (a production change) or `RateExceeded` = 0 to hold.
  - With `Delay`: the fetch and delivery limiters take no simulated time; the fetch timeout, the
    session and Status timeouts and the want set's claim deadline and reject TTL run on the real
    clock, so stalls, claim expiry and Busy-heavy waits that end in a timeout cannot be timed (a
    stalled holder fails its fetch after 15 s of real time). Putting them on `rangesync.Options`'
    clock (an injected timer for the timeout, a clock-aware fetch limiter, a clock option for the
    want set) belongs with part 3's churn and Busy scenarios. No bandwidth, jitter or protocol
    negotiation round trip is modeled.
- **(T4.6 part 1 fix round 1 → TF/hardening) One peer can flush the hint LRU.** Each decoded stream
  records one (peer, range) hint on either side, and the 4096 entries are shared by all peers. A
  peer that opens a session over many disjoint ranges and decodes or lets this node decode each
  stream can evict every other peer's hints (performance only: their streams start from the count
  estimate again). A per-peer share of the LRU would bound it; not needed for honest peers, which
  record one or a few root and level-1 hints per session.
- **(T4.6 part 1 fix round 1) Tests to run with `-race` on CI.** This extends "(T4.6 part 1) Tests to
  run with `-race` on CI"; they ran with `-count=10 -cpu=1,2,4` instead.
  - new: `TestNeighborhoodDelay`, `TestSimStream`, `TestSessionSymbolsDifferenceListed`,
    `TestSessionSymbolsServedHintBound`, `TestProtocolServedHint`;
  - changed: `TestSessionSymbolsHints`, `TestNeighborhoodRoundBounds`, and through the harness
    (parking, fetch slots, resets) `TestNeighborhoodConverges` and `TestNeighborhoodSREP`.
  - `Settle` is now 150 ms under the race detector; raise it further if a convergence bound fails.
- **(T4.6 part 1 fix round 2 → part 3, G6) The symbol overshoot: the growth step, with and without
  the hint.** This supersedes the title and the table of "(T4.6 part 1 fix round 1 → part 3, G6)
  The symbol overshoot is `GrowthFactor`'s, not the hint's": the title overstated it (the hint
  shifts bytes by a few points either way), and the table was measured in bursts with a `FullClose`
  that did not wait. At the steady load (`steadyOptions`, 50 ms, 3 seeds; symbols per difference,
  share of the modeled pullsync, round trips per decoded session counting the open and the close):

  | constants | N = 9 | N = 17 |
  |---|---|---|
  | now (`BatchFactor` 1.6, `GrowthFactor` 4), hint | 3.05, 77%, 4.6 | 3.37, 61%, 4.5 |
  | now, no hint | 3.27, 82%, 5.8 | 3.28, 59%, 5.4 |
  | `GrowthFactor` 2, hint | 2.18, 59%, 4.8 | 2.50, 48%, 4.7 |
  | `GrowthFactor` 2, no hint | 2.20, 60%, 7.2 | 2.29, 44%, 6.4 |
  | `BatchFactor` 2.0, hint | 2.78, 71%, 4.4 | 3.35, 61%, 4.4 |
  | `BatchFactor` 2.0, no hint | 3.34, 83%, 5.8 | 3.31, 59%, 5.4 |
  | both, hint | 2.34, 62%, 4.6 | 2.80, 53%, 4.6 |

  The review measured the same at a steady ~15 uploads per second: 62% of pullsync with the hint
  and 60% without, and 1067 against 2363 `More`s. Bytes are about even and the hint cuts the `More`
  round trips by 55–70% (0.86 against 2.45 per upload at N = 17 here), so it gates nothing. Only `GrowthFactor` 2 brings N = 17 under 50%; N = 9
  stays near 60% in every row (its n_Δ per upload is above p(N − p), and the decoder's listing adds
  ~9 points, Decisions). G6 should sweep these with the hint on and off (`simOptions.NoHints`) at a
  steady rate and decide the round trips against the bytes with `Delay` set.
- **(T4.6 part 1 fix round 2) The hint LRU: stale entries and a cheaper flush.** This extends
  "(T4.6 part 1 fix round 1 → TF/hardening) One peer can flush the hint LRU". (1) Entries have no
  age: a hint from a large difference long ago sizes the next first batch of that (peer, range)
  until a newer decode replaces it; only LRU eviction ages them. (2) The serving-side record makes a
  flush cheaper: the peer opens with up to `MaxParts` (1024) disjoint ranges, this node serves a
  stream for each (scan-path ones up to `MaxScansPerMessage`, 64, per Message), and each one the
  peer "decodes" with any listing records a hint here, without this node decoding anything. Both
  cost only performance (first batches from the count estimate, or one oversized batch); an age
  limit and a per-peer share would bound them.
- **(T4.6 part 1 fix round 2) Correction: zero-difference sessions under `Delay`.** This supersedes
  "With `Delay` 50–100 ms the share of such sessions rises (half of the sessions at N = 5 and 9)"
  in "(T4.6 part 1 fix round 1) Correction: zero-difference sessions": that was measured with the
  harness's immediate `FullClose`. With the wait (Decisions, round 2) N = 5 at 100 ms has none and
  the steady N = 9 run 24 of 456 samples.
- **(T4.6 part 1 fix round 2 → part 3) What `Spans` can time.** Sessions in round trips (open, one
  per message pair, the close), the gap between a Status and the session it triggers, a fetch's slot
  wait (`Wait`) and its first Delivery (`FirstDelivery`, 4D). Not the spread of a Want's Deliveries
  (no bandwidth model; they arrive together), nor anything on the real clock (round 1's list).
- **(T4.6 part 1 fix round 2) Tests to run with `-race` on CI.** This extends the round-1 list;
  they ran with `-count=10 -cpu=1,2,4` instead.
  - new: `TestNeighborhoodSteady`;
  - changed: `TestNeighborhoodDelay`, `TestSimStream`, `TestProtocolServedHint`,
    `TestProtocolConverges`, and through the harness (`FullClose`, sample checks)
    `TestNeighborhoodConverges` and `TestNeighborhoodSREP`.
- **(T4.6 part 1 fix round 3 → part 3, G6) Report spreads, not single runs.** This extends "(T4.6
  part 1 fix round 2 → part 3, G6) The symbol overshoot: the growth step, with and without the
  hint". At the steady N = 9 load, rerunning one seed moves the share by about 3 points, seeds by
  about 4 and `Delay` by about 12 (Decisions, round 3, Low 4). Part 3 must report every row as a
  spread: at least 3 seeds and a `Delay` sweep (0–200 ms). G6 may rely on the hint and `BatchFactor`
  rows only if their spreads separate. `GrowthFactor` 2's drop is already larger than the spread.
- **(T4.6 part 1 fix round 3) Tests to run with `-race` on CI.** This extends the round-2 list; they
  ran with `-count=10 -cpu=1,2,4` instead.
  - new: `TestNeighborhoodTeardown`;
  - changed: `TestNeighborhoodConverges` (the "no hints" subtest), `TestSimStream` (the helpers'
    `t`), and through the harness's teardown order every neighborhood test.
  - `TestNeighborhoodTeardown`'s watchdog is 30 s, for a teardown that takes milliseconds.
- **(T4.6 part 1 round-3 review, Lows; recorded by the orchestrator at landing) For T4.6 part 3.**
  1. If `TestNeighborhoodTeardown`'s watchdog `t.Fatal` fires, the cleanup's second `close()` blocks in
     `r.Close`'s `closeOnce.Do` (neighborhood_sim_test.go ~L559) behind the stuck first call, so the
     test still hangs until the go test timeout and the `Errorf` is lost without `-v`. Make a second
     `close()` return at once (atomic "closing" flag) or panic with a goroutine dump.
  2. The teardown check relies on run 2 repeating run 1's timeline exactly; the harness is not quite
     repeatable (TestNeighborhoodDelay varied 118–119 steps / 2489–2490 B over 32 loaded runs).
     Teardown passed 66/66; on divergence the message "protocol timing moved" misleads — mention
     nondeterminism or retry with the next candidate. Part 3 must treat the harness as
     near-deterministic and report spreads.
  3. No test covers the `errSimClosed` path or the harness closing a parked `FullClose`/read.
  4. Estimated ~40 s for `TestNeighborhoodTeardown` under `-race` (150 ms Settle); fine under the 30 m
     CI timeout, optionally skip under `-short`.
  5. Nit: the "bound from above" AC-B1 claim needs measured pullsync ≥ 102 B per offer (what the
     harness counts), not 100 B.
- **(T4.6 part 2 → proposal §7.8) Figures the model corrects (for the proposal text; not edited
  here).**
  - "~1.35–1.72 coded symbols" per difference is the oracle's cost (the model: 1.72 at d = 4, 1.61 at
    20, 1.52 at 45, 1.47 at 80, so §4.3.7 holds). With request-driven batches and the current
    constants a steady-state stream costs 3.0–3.6 (`GrowthFactor` 2: 2.2–2.6), bimodal per stream
    (p50 2.2–3.1, p90 6.0).
  - "only (has, lacks) pairs carry it: p·(N − p)" holds at N = 9 (18.2 per upload against 18). N = 17
    carries 48–55 (+15–31%): the lowest overlays open their sessions in two waves of
    `MaxInitiatedSessions` = 8, and the second wave meets peers that fetched from the first.
  - "≈ 110–140 B" and the totals "N = 9 ~2.0–2.5 KB, N = 17 ~4.6–5.9 KB": 5.6–5.9 KB and 16.8–17.7 KB
    with the constants (`GrowthFactor` 2: 4.4 and 13.9 KB). The table counts only symbols. It omits
    the decoder's Items listing (70 B per ID only the decoder holds, ~35 B per difference: 0.64 and
    1.9 KB per upload), the Want IDs ((N − p) × 70 B: 0.42 and 0.99 KB) and the Status gossip (one
    per neighbor per `StatusMinInterval` under load: 46 and 175 B per upload at 15 uploads/s). Even
    with the oracle's symbols the totals are 3.4 and 8.3 KB, 46% and 30% of pullsync, not 28–35%
    and 17–22%.
  - "Today ~7.2 KB / ~27 KB": 7.34 and 27.7 KB with AC-B1's 102 B per offer; on the wire 8.0 and
    30.2 KB at 15 uploads/s.
  - "Fill latency: one round (`RoundInterval`, adaptive 5–30 s)": under load the round triggers keep
    every node on one 5 s grid, so an upload converges within about one `MinRoundInterval` plus ~12
    legs (mean 3.4–3.7 s, longest 5.8–6.3 s at 50 ms). The adaptive interval matters only on a quiet
    network (bound 15 s, part 1).
  - "A per-round fixed overhead of `MinBatch` symbols (~330 B)" holds (4 × 81–82 B and the
    framing). "The per-difference cost doesn't depend on guessing the difference size" holds only for
    the oracle: the guess sets the overshoot (Decisions, "(T4.6 part 2) Correction: with an exact
    estimate a first batch decodes 73–91% of the time").
- **(T4.6 part 2 → T6.0) AC-B1's prediction.** acceptance-criteria.md's "28–35% at N = 9, 17–22% at
  N = 17 (symbols only), ~33–40% and ~21–26% with Want IDs" repeats §7.8. The model's floor with the
  listing, the Want IDs and the Status is 46% and 30% (the oracle), and the current constants give
  74–82% and 58–66%. Re-base the prediction on the model or on part 3's measurement.
- **(T4.6 part 2 → G6) What can bring AC-B1 within reach.**
  - N = 17: `GrowthFactor` 2 is on the line (47.5–52%). More initiated slots remove the second wave:
    with a slot per peer the model gives 53% (now) and 40% (`GrowthFactor` 2), and the convergence
    does not change (3.4 against 3.7 s). The model caps neither the served sessions
    (`MaxConcurrentSessions`) nor answers Busy, so this is a bound, not a design. This extends "(T4.3)
    Initiated-session share (T4.6)": the waves do not slow convergence, but they expose differences
    again. Part 3 should measure it with the harness (`MaxInitiatedSessions` is a constant: an option
    or a scratch edit).
  - N = 9: no constant reaches 50%. The floor is 46%, and it needs at most ~1.7 symbols per
    difference. Options: a better first-batch estimate (the model tests variants cheaply, e.g. a
    margin on hint(R), or hint(R) plus the count difference), a cheaper decoder listing, or AC-B1
    re-based at N = 9 (against pullsync's wire bytes, 1.09×, or a higher threshold).
  - The hint trades bytes for round trips: with `GrowthFactor` 2 it costs 4–5 points at N = 17 (49%
    against 44–45%) and saves 70% of the `More`s (1.2 against 4.0 per upload). G6 should weigh the
    two with `Delay` set (Q3).
- **(T4.6 part 2) `FirstBatch` does not read `BatchFactor`.** `FirstBatch` computes ⌈1.6 d⌉ as
  (16d + 9) / 10, and nothing reads the constant `BatchFactor`, so changing it alone changes neither
  the protocol nor the harness (part 1's `BatchFactor` runs needed `FirstBatch` edited too).
  `TestSREPFirstBatch` fails if the two disagree. Whoever implements G6's decision should derive
  `FirstBatch` from the constant (a production change, not made here).
- **(T4.6 part 2 → part 3) The model as part 3's predictor.** Part 3 can sweep with `TestSREPModelG6`
  and measure with the harness only the rows G6 needs (`TestSREPModelVsHarness` in a scratch copy
  with the constant changed; the model follows the compiled constants through `srepCompiled`). The
  model leaves to the harness: Busy and the session limits, churn and stalls, splits, bootstraps and
  stripes, radius changes, expired-batch skew (Q7), and everything on the real clock. Part 3 should
  also re-check the two gaps left at N = 17: the harness's n_Δ 7–9% above the model's at Delay 0, and
  the model's symbols per difference 3–10% above the harness's.
- **(T4.6 part 2) Tests to run with `-race` on CI.** The model runs in the test goroutine, apart from
  the decode table's mutex, so `-race` matters only for the harness change (`MoreRequests`, read with
  the other counters). New: `TestSREPFirstBatch`, `TestSREPWireSizes`, `TestSREPSymbolBytes`,
  `TestSREPStream`, `TestSREPDecodeTable`, `TestSREPSessionCost`, `TestSREPPullsync`,
  `TestSREPIdeal`, `TestSREPModel`; changed through the harness: every neighborhood test. They ran
  with `-count=5 -cpu=1,2,4` instead.
- **(T4.6 part 2 fix round 1 → part 3, G6; Medium 2) The 16-slot gain is unvalidated.** This
  qualifies the N = 17 bullet of "(T4.6 part 2 → G6) What can bring AC-B1 within reach". The 53%
  (now) and 40% (`GrowthFactor` 2) with a slot per peer come from the model alone, and they run
  through the second wave's hints. That is where the model's symbols per difference with hint(R)
  sit 2–6% above the harness's at N = 17, a gap still untraced (Decisions, fix round 1, Medium 2),
  and where the model has no session cap. Part 3 must measure it with the harness (a scratch edit
  of `MaxInitiatedSessions`) before G6 relies on it.
- **(T4.6 part 2 fix round 1 → G6, T4.x; Low 1) 16 initiated slots fit only at N = 17.** At
  N = 17, node k (by overlay order) opens 16 − k sessions and serves k. With a slot per peer it
  needs exactly 16 tokens, which is `MaxConcurrentSessions`; served and initiated sessions share
  those tokens. `MaxInitiatedSessions = MaxConcurrentSessions / 2` keeps half for inbound
  sessions (scheduler.go). A constant 16 breaks that reasoning in two cases: at N > 17 (a node
  with more than 16 eligible neighbors), and whenever stripe transfers, verifications or takeovers
  hold slots. The excess then turns into Busy answers. If G6 wants the second wave gone, the cap
  must be per node or adaptive, not a constant: for example the node's eligible higher-overlay
  neighbors, bounded by the tokens its served sessions leave.
- **(T4.6 part 2 fix round 1 → part 3) The model's n_Δ gaps at N = 17.** This supersedes the
  last sentence of "(T4.6 part 2 → part 3) The model as part 3's predictor" ("re-check the two gaps
  left at N = 17"). The gaps with 6 seeds: the harness's n_Δ is 0–1% above the model's at 50 ms,
  4–5% at 200 ms and 7–11% at Delay 0. No session was refused in these runs, so the session caps
  the model omits are not the cause. The model's symbols per difference with hint(R) are 2–6%
  above the harness's (`BatchFactor` 2.0 the most). The model underestimates the share's rise from
  50 to 200 ms at N = 17 by about 2 points. Trace both before part 3 relies on the model at
  200 ms.
- **(T4.6 part 2 fix round 1) Re-record `srepHarnessRef` when the protocol changes.** The records
  are the harness's figures with each of G6's candidates (6 seeds, scratch builds with the
  constants changed). `TestSREPModelVsHarness` fails its record check when the live harness drifts
  from the record of the compiled constants, and logs the new records. The candidates' records
  must be re-measured in scratch builds, about 3 minutes each.
- **(T4.6 part 2 fix round 1) Tests to run with `-race` on CI.** This extends "(T4.6 part 2) Tests
  to run with `-race` on CI". They ran with `-count=2..5 -cpu=1,2,4` instead.
  - New: `TestFirstBatchPinned`, `TestFirstBatchFactor`.
  - Changed: `TestSREPFirstBatch`, `TestSREPSessionCost`, `TestSREPModel`.
  - Changed through `FirstBatch`: `TestFirstBatch`, `TestWorkedExamples`, `TestSessionSymbols*`,
    and the neighborhood tests.
  - `TestSREPModelVsHarness` (`-tags=sim`) runs 48 harness runs one after another, about 3 minutes
    on this host. Under `-race`, expect 10× that.
- **(T4.6 part 3 → G6, T4.x) hint(R) records catch-up differences.** A catch-up that decodes at the
  root records hint(R) = the catch-up's size on both sides, and the pair's next session, with a
  difference of 1–4, starts with `FirstBatch` of it: after Lag 300, 1000 and 2500, the next sessions
  took 1988, 6450 and 16052 symbols, as many as the catch-ups (`TestG6Catchup`). Past ~2500 the root
  splits and records no root hint, so only level-1 hints go stale. This extends "(T4.6 part 1 fix
  round 2) The hint LRU: stale entries and a cheaper flush". A fix, a code change after G6: record
  no hint for a difference of `CatchUpMinDiff` or more (steady-state sessions stay far below it), or
  clamp the hint against the count estimate.
- **(T4.6 part 3 → T4.x, T6.0 AC-B2) The round sessions of a bootstrapping node's peers list every
  ID.** In an N = 17 bootstrap of 20000 chunks (`TestG6Bootstrap`) the stripes moved the reserve
  once, but every peer with a lower overlay also ran its round session with the empty node, which
  resolves by listing all its IDs. Node 0 received 4.2–5.7 MB of metadata with 4 lower peers and
  12.5–14.0 MB with 14 (two runs), against AC-B2's 2.6 MB, and denied 9,495 and 51,255 claims of IDs
  its stripes held (first run). AC-B2 fails by a factor that grows with the node's rank. A fix: a
  peer defers its sessions with a peer whose Status count is below `BootstrapRatio` of its own (that
  peer stripes), or the bootstrapping node answers sessions Busy until its bootstrap transfer ends.
  Needed before T6.1's Bootstrap case can pass AC-B2.
- **(T4.6 part 3 → G6, T4.x) A per-node initiated-session cap.** If G6 takes the 16-slot gain at N =
  17 (10–15 points of AC-B1, faster convergence, no Busy), the cap must not be the constant 16 (part
  2 fix round 1, Low 1): for example the node's eligible higher-overlay peers, bounded by the tokens
  its served sessions leave, with `MaxStripeSessions` kept apart.
- **(T4.6 part 3 → T6, a radius-aware harness) Nested session roots use the bucket cache (by
  inspection).** A session with a peer one bit deeper runs on the peer's root, one bit below this
  node's. `cacheCapacity` gives this node's stream there `BucketSymbols` (32) once it holds more
  than `ScanLimit` in it, so a difference past ~20 exhausts and splits. Every harness node is at
  radius 0, so it was not measured. If mixed radii are common in a neighborhood, the root's half
  ranges may need a cache of their own (2 × 4096 symbols, ~0.7 MB).
- **(T4.6 part 3 → T6.0) Q9's check reads `pairwise_diff_half`.** This supersedes, in "(T4.6 part 1)
  Proposal §10 and T6.0 wording", "For Q9, the check is the share of samples in `le="255"`" of
  `pairwise_diff`. `CatchUpMinDiff` is compared with one side's wants, `pairwise_diff_half`'s value;
  `pairwise_diff`'s neighborhood aggregate (n_Δ, both sides) passes 255 at its p99 from ~40 uploads
  per second at N = 17 while no side's wants do. And the bound is the max per window, not the p99
  (G6 summary, Q9).
- **(T4.6 part 3 → T6.4) AC-V1 at a quiet network.** This extends "(T4.6 part 1 fix round 1 → T6.4)
  AC-V1 against the 15 s bound": the sparse runs' longest convergence is 10.75–10.95 s, past 2 ×
  `MinRoundInterval`, and so is the steady longest at Delay 200 ms and N = 17 (10.4 s).
- **(T4.6 part 3 → T6) Measurements the harness cannot make.** Q7 under expired-batch skew and the
  unknown-batch junk it lets through (no batch store, a no-op `EvictBatch`), retry granularity and
  the bootstrap time against the fetch limiter (real clock), deferred stripes near `MaxClaims`
  (fetches end within an instant), re-ask waits and trimmed stripe parts (radius changes), and the
  first-chunk latency of catch-ups (no bandwidth or limiter on the clock). See the Decisions log
  entry "(T4.6 part 3) The follow-ups addressed to T4.6".
- **(T4.6 part 3) `SessionSymbolBudget` bounds a catch-up session.** A session of a 6000-chunk
  catch-up stops at 16384 symbols and resolves its last ranges by item lists (68 exhausted streams
  per run), and a 12000-chunk one takes three sessions per peer (6.9 s). Informational: the budget
  bounds a session's decoder memory (T4.3), and bootstraps past `LargeDiff` stripe instead.
- **(T4.6 part 3) Resolved.** "(T4.3) Initiated-session share (T4.6)", "(T4.3 fix) T4.6: check the
  stripe/round split", "(T4.3 fix round 2) T4.6: Busy-heavy stripes" with its part 2a extension,
  "(T4.5 part 2c → T4.6) A peer that fails every fetch open keeps its sessions", "(T4.6 part 1 →
  part 3) Zero-difference sessions", "Metadata per upload is well above §7.8" and "Busy, collisions
  and stale sessions at N = 17", "(T4.6 part 2 fix round 1 → part 3, G6; Medium 2) The 16-slot gain
  is unvalidated", "(T4.6 part 1 fix round 3 → part 3, G6) Report spreads, not single runs", "(T4.6
  part 2 → G6) What can bring AC-B1 within reach", and items 1–3 of "(T4.6 part 1 round-3 review,
  Lows ...) For T4.6 part 3". The moved ones are in the entry above; "(T4.5 part 2c → T4.6) A failed
  verification leaves the deliverer unjudged" stays open.
- **(T4.6 part 3) Tests to run with `-race` on CI.** They ran with `-count=3 -cpu=1,2,4` instead.
  New: `TestSimClose`, `TestNeighborhoodG6Figures`. Changed: `TestNeighborhoodTeardown`, and through
  the harness (wants, lag, counters, the `Limit` check) every neighborhood test.
- **(T4.6 part 3 fix round 1 → T4.x, T6.1) PRE-TESTNET BLOCKER and T6.1 entry blocker: a
  bootstrapping node's peers list their reserves to it (AC-B2).** This supersedes the fix list and
  the last sentence ("Needed before T6.1's Bootstrap case can pass AC-B2") of "(T4.6 part 3 → T4.x,
  T6.0 AC-B2) The round sessions of a bootstrapping node's peers list every ID". Measured
  (Decisions, "(T4.6 part 3 fix round 1) G6 summary (revised after review)", AC-B2): at N = 5 and
  Base 70000–100000 the empty node reads (1 + rank) reserves of IDs or more, 1.4–3.8× AC-B2's bound,
  and the higher-overlay peers' takeovers add once the transfer outlasts their wait. At 2^22 one
  session that meets the node at count 0 costs ~294 MB against the bound's ~340 MB, and the sessions
  repeat for the ~70-minute transfer. Root cause: (1) `Decide` returns the count-0 item decisions
  before the stripe branch (session.go ~L187–192); (2) the stripe branch is the initiator's only and
  direction-blind (`StripeCollector`); (3) only the bootstrapping node knows (`planCatchUp`), while
  its lower-overlay peers keep initiating every round and the higher-overlay ones take over. Fix
  directions, to choose in T4.x:
  1. The bootstrapping node answers Busy to the sessions it does not open while its bootstrap
     transfer runs (the handler has the Busy answer; the reconciler's state would reach it through
     an option). It covers initiators and takeovers alike; Busy is not scored, and the peer backs
     off. The verification round after the transfer covers what changed meanwhile.
  2. A peer defers its sessions with a peer whose Status count is below `BootstrapRatio` of its own
     (that peer stripes). No wire change; it relies on a fresh Status.
  3. `Decide` checks the large-difference branch before the count-0 item decisions, on both sides,
     so that no session lists more than `LargeDiff` IDs of a range. Only together with a
     direction-aware stripe branch (stripe a range only where this side holds fewer): otherwise the
     holder stripes its own root and its registry holds its `SyncRate` (the stripe-registry
     blocker). And a responder past `LargeDiff` can answer only with the session-level Busy on the
     frozen wire (G2).
  4. The responder treats a peer at count 0 as bootstrapping and lists no more than `LargeDiff` IDs
     to it: the same wire limit as 3.
  1 or 2 closes the bootstrap case; 3 also bounds a large catch-up that is not a bootstrap.
  Acceptance: `TestG6BootstrapPeers` and `TestG6Bootstrap` within AC-B2's bound at every rank, then
  T6.1's Bootstrap case.
- **(T4.6 part 3 fix round 1) Pre-testnet blockers.** Three, all to be fixed before any testnet
  cohort enables `rangesync`: "(T4.5 part 2c round-2 review; recorded by the orchestrator at
  landing) PRE-TESTNET BLOCKER: stripe registry SyncRate pin", "(T4.5 part 2b fix round 3;
  orchestrator request) Per-peer fetch credit (before testnet)", and the AC-B2 entry above (also a
  T6.1 entry blocker). The first and the third meet in the stripe branch, which `Decide` takes on
  |cL − cR| alone: fix direction 3 above must make it direction-aware, which also removes from the
  first the case of a peer that claims a far smaller count.
- **(T4.6 part 3 fix round 1 → G6 change) `GrowthFactor` 2 lands with four tests.** If G6 takes
  it: `TestProtocolConverges` (derive the bound's Mores from `GrowthFactor`: 2 × (10 + 7) Messages
  at ×2), `TestWorkedExamples` (examples 2 and 3 and §7.2 take GF2's outcomes, with the reason in
  the case list), `TestSREPSessionCost` (the "now" band at d = 45, ~1.9 symbols per difference) and
  `TestSREPIdeal` (compare against `GrowthFactor` 4 explicitly, not the compiled constant). The wire
  needs nothing: GF2 and GF4 peers interoperate.
- **(T4.6 part 3 fix round 1 → T6.3, Q9) `CatchUpMinDiff` crossings.** This extends "(T4.6 part 3 →
  T6.0) Q9's check reads `pairwise_diff_half`": if steady-state sides reach 256 on testnet, the
  remedy is a load-relative rule, not a fixed 1024 (Decisions, Q9); a per-node initiated-session
  cap, if it lands first, halves the N = 17 maxima.
- **(T4.6 part 3 fix round 1) The stale catch-up hint is bounded.** This extends "(T4.6 part 3 → G6,
  T4.x) hint(R) records catch-up differences": its cost is at most `FirstBatch(2558)` = 4095 symbols
  (~336 KB) per pair, once; a larger root hint makes the first batch exceed `RootSymbols`, and the
  root splits instead. The fix stays as written, at low priority.
- **(T4.6 part 3 fix round 1) Tests to run with `-race` on CI.** They ran with `-count=3 -cpu=1,2,4`
  instead. Changed: `TestNeighborhoodG6Figures` (the flows check), and through `simResult.Flows`
  every neighborhood test. New, `-tags=sim` only: `TestG6BootstrapPeers`, `TestG6EqualCounts`.
- **(T4.6 part 3 fix round 2 → T4.x, T6.1) PRE-TESTNET BLOCKER and T6.1 entry blocker: a
  bootstrapping node's peers list their reserves to it (AC-B2), corrected.** This supersedes, in
  "(T4.6 part 3 fix round 1 → T4.x, T6.1) PRE-TESTNET BLOCKER and T6.1 entry blocker: a
  bootstrapping node's peers list their reserves to it (AC-B2)", the fix directions 1–4, the
  sentence "1 or 2 closes the bootstrap case; 3 also bounds a large catch-up that is not a
  bootstrap" and its acceptance; and, in "(T4.6 part 3 fix round 1) Pre-testnet blockers", in its
  last sentence, "fix direction 3 above must make it direction-aware" (the meeting point stands: a
  direction-aware stripe branch, candidate B, also removes from the registry pin the case of a peer
  that claims a far smaller count). Its measurement and root cause stand, with (2) made precise
  (Decisions, "(T4.6 part 3 fix round 2) G6 summary (revised, round 2)", AC-B2): in a round session
  the peer is the initiator and does the listing (its answer to node 0's count-0 `Items{Reply}`,
  session.go ~L746, and `DecisionSendItems` on the children at node 0's count 0), and while node 0's
  root difference exceeds 16 × `LargeDiff` the peer stripes level-1 ranges it already holds. The
  directions are withdrawn: 3 and 4 refuse the stripe sessions (count-0 listings of 2^17–2^19 IDs at
  2^22) and 4 caps the wrong side; 2 defers edge peers across roots; 1 depends on the equal-root
  `IsBootstrap`.
  - Constraints: (a) stripe sessions, the count-0 listings past `LargeDiff` that the bootstrapping
    node requests, still work; (b) a round session does not make a non-bootstrapping peer list its
    whole reserve to a count-0 or near-empty peer (the initiator's listing); (c) edge peers and
    nested roots (§7.6) and stale-stamp divergence (§7.7) are neither deferred nor starved; (d)
    detection does not rest on the equal-root `IsBootstrap` alone.
  - Candidates, with their trade-offs in the Decisions entry: A, the lacking side declines (node 0,
    in either role; the only one that protects a new node against old peers); B, the initiator takes
    the large-difference test first, by direction (the peer; it does not reach the root's count-0
    answer); C, the initiator caps its listing and the responder takes the rest as a stripe
    hand-over (both sides); D, the bootstrapping node answers Busy (node 0 as responder; (d) only if
    keyed on the session root's counts, and it refuses small differences meanwhile); E, an explicit
    bootstrap signal (the wire is frozen, G2). A and C give responders a Striper and must land with
    the stripe-registry fix; with A, the threshold for a node with a transfer running may need to
    sit below `LargeDiff` (the tail's descent lists up to ~4.6 MB per session).
  - Acceptance: a scaled case first, which runs stripes past `LargeDiff` and the child-range stripe
    path: a test-settable `LargeDiff` (near Base/64) or a harness Base past ~1.05M. It checks (a)
    that the stripes complete, (b) node 0's metadata within AC-B2 scaled to Base at ranks 1–4 with
    takeovers firing, and (c) and (d) with a peer one bit deeper and a bootstrapping node whose root
    no peer shares. Then `TestG6BootstrapPeers`, `TestG6Bootstrap` and T6.1's Bootstrap case.
- **(T4.6 part 3 fix round 2) Pre-testnet blockers (final).** This supersedes "(T4.6 part 3 fix
  round 1) Pre-testnet blockers". Three, all before any testnet cohort enables `rangesync`: (1) the
  AC-B2 entry above (also a T6.1 entry blocker); (2) "(T4.5 part 2c round-2 review; recorded by the
  orchestrator at landing) PRE-TESTNET BLOCKER: stripe registry SyncRate pin", before or with (1) if
  (1) gives responders a Striper; (3) "(T4.5 part 2b fix round 3; orchestrator request) Per-peer
  fetch credit (before testnet)", with a credit that survives a peer's reconnects. The
  sustained-overwrite `SyncRate` pin is not a pre-testnet blocker: it is a T6.3 item and a fix
  before mainnet (the two entries below; the reasons are in the Decisions entry's final sheet).
- **(T4.6 part 3 fix round 2 → T4.x or T6, before mainnet) The catch-up rule under sustained load.**
  This extends "(T4.6 part 3 fix round 1 → T6.3, Q9) `CatchUpMinDiff` crossings", whose trigger ("if
  steady-state sides reach 256 on testnet") testnet's ordinary load will not meet. Fresh uploads at
  a stable radius are bounded by ~2 × the reserve over the batch TTL (~97 per second at one-day
  TTLs), overwrites by no TTL (the reserve replaces the older chunk on a stamp-index collision, and
  postage charges per slot and time). Past ~50–100 IDs per second per neighborhood (N = 17, 8 slots;
  lower at N > 17, untested) steady-state sides cross 256, count as catch-ups and keep `SyncRate`
  above 0: no radius decrease and no `isFullySynced`, so no redistribution for the neighborhood
  while it lasts (`pkg/puller` counts only historical intervals, so live load does not pin it). A
  fix: a load-relative rule (a side is a catch-up when its wants exceed max(256, a multiple of what
  the node's recent inflow predicts for the time since the pair's last session)), or an
  overwrite-aware one (the inflow counts reserve replacements). Not designed or measured here.
- **(T4.6 part 3 fix round 2 → T6.3) A sustained overwrite stream.** A T6.3 item: one mutable batch
  rewrites its slots into one neighborhood at stepped rates (for example 10, 25, 50, 100 and 200 IDs
  per second, each held for more than one 10-minute window), with the cohort's N and the per-node
  cap as landed. Read `pairwise_diff_half`'s max per window, `sync_rate`, the reserve worker's
  radius and the storage incentives' sync check. It gives the crossing load on real rounds and
  decides how urgent the entry above is; T6.3's ordinary load does not reach it.
- **(T4.6 part 3 fix round 2 → G6 change) `GrowthFactor` 2: the proposal's text and the scan cost.**
  This extends "(T4.6 part 3 fix round 1 → G6 change) `GrowthFactor` 2 lands with four tests". With
  the constant, update pullsync-rangesync-rateless-proposal.md: L824 ("4 × received"), L830 ("growth
  factor 4"), L894 ("4 × 7 = 28"), L1175 ("UpTo 272") and L1191–1194 (example 3's "UpTo 16" and
  "UpTo 64", and its Message count, as `TestWorkedExamples` takes GF2's outcomes). No other code
  depends on 4 (its one use is session_symbols.go ~L403). Recorded, not measured: each More on the
  scan path reruns `Local.Symbols(r, 0, end)` (session_symbols.go ~L125), so GF2 about doubles an
  honest scan stream's reads.
- **(T4.6 part 3 round-2 review; recorded by the orchestrator at landing, G6 2026-10-21) Notes for
  the AC-B2 fix task (T4.7).** These refine the round-2 AC-B2 candidates; the G6 numbers and constants
  are unaffected.
  1. **Candidate C covers the leak only in the first session.** C caps only the answer to an
     `Items{Reply}`, which exists only while node 0's root count is exactly 0. From the next round node 0
     holds IDs and splits the root; the initiator then lists every count-0 child through
     `DecisionSendItems` (not a Reply answer; ~18 MB per empty child at 2^22, and stripes fill in prefix
     order, so most children are empty early) and still stripes non-empty children past `LargeDiff`
     regardless of direction. C needs B for the descent.
  2. **B and C don't combine:** B sends no listing, so C's hand-over trigger ("listing far shorter than
     the gap") never fires; a lacking node that is responder to all its peers (the highest overlay, with
     a non-bootstrap deficit past `LargeDiff` in a child) then starves, breaking constraint (c). **Only A
     closes every case.** The scaled acceptance test should add a responder-only-lacking-node case.
  3. The round-2 text "hands each child past `LargeDiff` to its StripeCollector" is wrong for count-0
     children: the count-0 check comes first in `Decide`, so they go to `DecisionSendItems`. The "~75%"
     figure assumes uniform filling.
  4. A and C need more than stated: a responder Striper needs a new `rangesync.Options` field plus
     wiring in `pkg/node` (node.go ~L1208); A and B reorder §6.3's normative rule and C truncates its
     normative answer (old responders read it as complete) — list the proposal text to change.
  5. A defers small differences while it declines (new uploads and §7.7 differences wait too); state it.
  6. Acceptance text: tie "with takeovers firing" to a Base (they fired only at Base 100000); check that
     no peer registers a range it already holds; "ratio … about 1,100" is the scaled `LargeDiff`.
  7. Q9: ~97/s is an estimate assuming uniform uploads, not a bound; reserve.go also overwrites
     immutable batches. The overwrite pin hits the whole neighborhood (the registry pin one node) and
     costs the attacker only pushsync bandwidth — keep it a T6.3 item, but say so.
  8. The GrowthFactor text list misses proposal L467 (§4.3.7, "More(R, up to 4 × b)").
  9. The decision sheet should state that GrowthFactor 2 and the per-node cap land before T6.1
     (EXECUTION.md §6, G6 row) — done in T4.7.
- **(T4.7 part 1 → the proposal) The decision rule's text after candidate A.** The code now differs
  from the proposal's normative text in these places (pullsync-rangesync-rateless-proposal.md; not
  edited here):
  - §6.3 L797–831: the flowchart (L807–822) tests `cR == 0` / `cL == 0` before `d̂ > LargeDiff`
    and stripes "initiator side" only (L813–814). Now: after EQ comes the large-difference test by
    direction, `cR − cL > LargeDiff` on the side that holds fewer, in either role (0 instead of
    `LargeDiff` in a range the node's own striped transfer covers); that side hands R to its
    Reconciler and emits nothing for R (a responder whose every range is taken answers the empty
    Message, which ends the initiator's session with R unanswered, i.e. equal); the holder never
    stripes, and its count-0 listing (`cR == 0` → Items) stays, which serves the stripe sessions.
    A range the Reconciler declines is decided by the rest of the rule. L799 ("Both sides apply
    the same rule") stays true; the "BIG" node's label and its position change, and the text needs
    a sentence on the empty answer and on why the holder keeps listing.
  - §6.3 L834 (constants): `LargeDiff = 65536` stays; add that tests scale it (not a wire value).
  - §7.4 L931 ("own count << peers' Status counts: bootstrap mode"): add the per-peer session-root
    check (a node more than `LargeDiff` behind a peer in their session root stripes that root
    instead of opening a round session with it; the peer's count is spread over its root when the
    node's root lies inside it) and the node's answers while bootstrapping (it declines, as
    responder, every range it lacks inside its transfer).
  - §8.2 L1226 ("Initiator"): the lower overlay initiates unless it is far behind the peer in their
    session root (above); the higher overlay's takeover then meets a responder that declines.
  - §10's metric table has no counter of declined ranges; nothing measures them on testnet except
    `sessions_total{ok}` with zero wants (T6.0 may want one).
- **(T4.7 part 1 → T6.1, AC-B2) PRE-T6.1 FINDING: a stripe that outlasts `StripeTimeout` lists
  again what it had listed.** Measured in the scaled bootstrap at the default fetch rate's
  duration (`TestG6BootstrapTail`, Base 16000, a fetch rate on the clock of 3 per second, the
  transfer ~89 minutes, stripes of 4000 IDs at ~0.75 per second): with the default 15-minute
  `StripeTimeout` node 0 read 9.34–9.45× the IDs it lacked (38–40 stripe sessions for 4 stripes,
  caught up after ~129 minutes instead of ~89), against 1.10–1.18× with the timeout lifted to 3
  h. Each timeout restarts the stripe with the same peer (`maxStripeRestarts`), and the restart's
  session lists the stripe's missing IDs again,
  including those the timed-out attempt had listed and not yet fetched. The harness overstates it
  (a whole stripe fits in one Message there, so every restart relists the whole remainder); in
  production the listing is paced by the stripe queue (`stripeQueue` 4 batches, one Message of
  IDs each, ~14,900 IDs, plus the batch the session's goroutine holds and the one being fetched:
  ~60–90k IDs listed ahead), so each restart relists ~4–6 MB. Estimated, not measured: at 2^22
  with 5–16 peers the stripes take 17–35 minutes at 250 chunks per second each (1000 per second
  over `MaxStripeSessions` 4), so 1–2 restarts per stripe, 16 in all, ~65–100 MB: past AC-B2's
  ~46 MB of headroom (340 − 294 MB) on its own. Not part 1's leak (no peer lists anything the node
  did not ask for), but an AC-B2 blocker for T6.1 all the same. Directions (none designed): a
  timeout that runs only while the attempt stores nothing (as part 3 proposes for the registry
  hold), or one scaled by the stripe's size at the node's fetch rate, or a restart that reopens
  only past the last ID the attempt stored.
- **(T4.7 part 1 → part 3) The registry `SyncRate` pin, widened.** Part 1 adds two ways to register
  a range, both through `registerStripeLocked` like the initiator path: the sessions this node
  serves (`Reconciler.Stripe`, when a peer's Fingerprint counts more than `LargeDiff` above this
  node's) and the rounds' session-root check (when a peer's Status count is more than `LargeDiff`
  above this node's count of their session root). Both are a peer's claim, so a peer claiming a
  far larger count now also makes a responder register, and a peer's Status alone makes the
  rounds register its session root again each time the transfer ends (each job ends quickly when
  the peer delivers nothing, and the honest peers' stripe sessions find little to do). Part 3's
  fix (hold `SyncRate` for a stripe only while its job stores chunks, past a grace) covers both,
  since they register the same jobs. While a liar's range is registered, the node also declines
  every lack inside it in its round sessions (`Transferring`), so its small differences there move
  only through the stripe sessions until the job ends; part 4's fetch credit does not change that.
- **(T4.7 part 1, low) The session-root check's estimate across nestings.** When this node's root
  lies inside the peer's, `LacksSessionRoot` spreads the peer's count evenly over its root. At
  2^22 the binomial spread of a half (σ ≈ 1000) is far below `LargeDiff`; at the scaled sizes of
  the tests it is not (σ ≈ 45 against `LargeDiff` 125 at Base 8000), and a root whose halves are
  filled unevenly (a peer that has just widened its radius) can trigger a striped transfer of the
  session root where a session would do. A false trigger costs a stripe job (sessions opened with
  the stripes instead of the root), not a deferral. Revisit with T6's radius-aware runs.
- **(T4.7 part 1, low) `PlanStripes` follows the map order of the peers.** `peerStatuses` iterates
  `r.peers`, so the stripe a peer gets, and whether an edge peer one bit deeper gets one at all
  (an ineligible peer's turn passes on), changes from run to run; a peer may then serve two stripes
  one after the other. `TestNeighborhoodBootstrapEdgePeer` read 1.02× or 1.23× of the lacked IDs
  with one seed: in the second, node 0 already held a few uploads of the later stripe, so the
  holder answered its opening (at count > 0) with a symbol stream instead of a listing, which the
  scaled geometry allows (every range of at most `ScanLimit` IDs has an unlimited stream). Not a
  correctness issue; sorting the peers would make plans reproducible.
- **(T4.7 part 1) Tests to run with `-race` on CI.** They ran with `-count=3 -cpu=1,2,4` instead.
  New: `TestDecideThreshold`, `TestSessionLackingSideDeclines`, `TestReconcileServedStriper`
  (pkg/rangesync); `TestServedStripeRegisters`, `TestRoundLackingSessionRoot`, `TestTransferring`,
  `TestLacksSessionRoot` (reconciler); `TestRangesyncOptions` (pkg/node); and the harness tests
  `TestNeighborhoodBootstrapScaled`, `TestNeighborhoodBootstrapResponderOnly`,
  `TestNeighborhoodBootstrapEdgePeer`, `TestNeighborhoodBootstrapForeignRoot` (served sessions now
  call into the reconciler, `Reconciler.Stripe` and `Transferring` take its lock on the handlers'
  goroutines). Changed: `TestDecide`, `TestDecideSymbols`, `TestSessionStriper`; through the
  harness (the responder Striper, radii, roots, the clock fetch rate) every neighborhood test; and
  re-run `TestRound*`, `TestRoundBootstrap*`, `TestStartupGuard*`, `TestSyncRate*`,
  `TestTransferStripes*`, `TestStripe*`, `TestCombinedSyncerBothDrivers`.
- **(T4.7 part 1 fix round 1 → T6.1, AC-B2) Fixed: a stripe that outlasts `StripeTimeout` no
  longer lists again what it had listed.** This supersedes the status "PRE-T6.1 FINDING" of "(T4.7
  part 1 → T6.1, AC-B2) PRE-T6.1 FINDING: a stripe that outlasts `StripeTimeout` lists again what
  it had listed", and its directions: `StripeTimeout` is an inactivity timeout, reset when the
  attempt's fetches store chunks, with a per-peer budget of 9 timeouts (Decisions, "(T4.7 part 1
  fix round 1) Review findings"). Measured (`TestG6BootstrapTail`, the default `StripeTimeout`, the
  transfer ~89 minutes, each stripe ~6 timeouts long): 1.10–1.18× of the lacked IDs at ranks 1, 2
  and 4 (bound 2288 KB; 9.34–9.45× before), no stripe timed out or restarted, caught up after the
  transfer's 89 minutes instead of ~129; and `TestNeighborhoodBootstrapStripeTimeout` (default
  build, a 30 s timeout, 2-minute stripes): 1.01× (6.15× and 250 duplicate deliveries with the
  old clock). A restart still relists the remainder, now
  only after a peer stored nothing for 15 minutes or spent its budget (a dripping or stalling
  peer, not an honest slow one). Left for T6.1: the real clock's fetch limiter (the harness's
  fetch rate on the clock is an upper bound of it).
- **(T4.7 part 1 fix round 1 → part 3) The registry `SyncRate` pin, corrected.** This corrects
  "(T4.7 part 1 → part 3) The registry `SyncRate` pin, widened": "each job ends quickly when the
  peer delivers nothing" holds for the honest peers' stripes of a claim's job, which find little
  to do, not for the liar's own stripe, which ends only when its attempt has stored nothing for
  `StripeTimeout` (15 minutes, then it is reassigned to an honest peer), or, if the liar delivers
  a real chunk now and then, after its budget of 9 timeouts (2.25 h). And "While a liar's range is
  registered, the node also declines every lack inside it in its round sessions (`Transferring`)"
  no longer holds: a claim lowers no threshold (fix round 1), and the claims are capped
  (`maxPeerStripes` 2 per peer, `maxSessionStripes` 1 per session). What is left for part 3: (1)
  the pin itself, now by at most `maxPeerStripes` claims per peer at a time, re-registered as soon
  as one ends; (2) a liar's claim in flight occupies the one stripe worker for as long as its own
  stripe lasts (above), so the other claims (an honest responder-only lack, a constraint-(d)
  session root) wait behind at most `maxPeerStripes` claims per lying peer; this node's own
  transfers do not wait (they end the claim in flight). Part 3's "hold for a stripe only while its
  job stores chunks, past a grace" is where (2) fits too: end a claim whose job stores nothing past
  the grace. Part 4's fetch credit does not change either.
- **(T4.7 part 1 fix round 1 → T6.1, AC-B2) The tails of the two claimed transfers.** A transfer
  that a peer's count registered (a claim) keeps the sessions' threshold at `LargeDiff`, so below
  it its tail is reconciled by the round sessions again: the responder-only lacking node (served
  claims) and a bootstrapping node whose root no neighbor shares (constraint (d), the session-root
  claim). Up to `LargeDiff` IDs (~4.6 MB at 2^22) per session through the tail, as candidate A's
  text estimated and as part 1 measured for the `IsBootstrap` tail before its zero threshold
  (536–1703 wanted IDs at Base 16000, ~10–31 MB at 2^22 and N = 5, past AC-B2's ~46 MB headroom at
  N = 17, estimated). The scaled default-build cases read as in part 1 (67 KB; 1.00× and 1.08×);
  the 2^22 geometry of these two cases is not measured. A direction that keeps a liar out: make
  the session-root transfer an own job when the median of the peers comparable over that session
  root says this node is far behind (`IsBootstrap`'s rule per session root), not one peer's
  Status.
- **(T4.7 part 1 fix round 1 → the proposal) Corrections to "(T4.7 part 1 → the proposal) The
  decision rule's text after candidate A".** "§6.3 L834 (constants)" is §6.3 L830 (L834 is §6.4).
  "0 instead of `LargeDiff` in a range the node's own striped transfer covers" means its own
  transfers only (its bootstrap and a radius decrease's half, over their union), not a range a
  peer's count registered; §7.4 L931's addition should say that the session-root check's transfer
  is the peer's claim (capped per peer, preempted by the node's own transfers, no zero threshold).
  §9's "stripe timeout and reassignment" (L1272) could say that the timeout runs while the stripe
  stores nothing, within a per-peer budget.
- **(T4.7 part 1 fix round 1, low) `TestSyncRateBootstrap` can race its own clock.** Its loop
  moves the clock 20 minutes per pass (`pastWindow`), past `StripeTimeout`, while the fetches are
  gated: the stripes time out, are reassigned, fail on the second pass, and the bootstrap is
  registered again a round later; a pass whose `SyncRate` read falls in that gap reads 0. Seen once
  in nine `-count=3 -cpu=1,2,4` runs of the package, not in 90 runs of the test alone nor in six
  runs of the package on the base code; the mechanism predates fix round 1 (the old timeout
  expired the gated attempts the same way). A `StripeTimeout` above the loop's 60 minutes in that
  test would remove it.
- **(T4.7 part 1 fix round 1) Tests to run with `-race` on CI.** They ran with `-count=3
  -cpu=1,2,4` instead (the harness tests `-count=2 -cpu=1,2,4`). New: `TestClaimCaps`,
  `TestOwnJobPreemptsClaims`, `TestClaimKeepsRounds`, `TestStripeInactivityTimeout`
  (reconciler; the stripe clock is now shared by the attempt's goroutine, its fetch goroutine and
  the fetch callbacks, under its own mutex), `TestNeighborhoodBootstrapStripeTimeout`. Changed:
  `TestTransferring`, `TestServedStripeRegisters`, `TestSessionRecheckAfterSlot`,
  `TestReconcileServedStriper`, `TestRangesyncOptions`, `TestNeighborhoodBootstrapScaled`; and
  re-run every `TestStripe*`, `TestTransferStripes*`, `TestEnqueueStripes*`,
  `TestRoundBootstrap*`, `TestRadius*`, `TestSyncRate*`, `TestVerifyStripes*` (the registry's
  entries and the stripe worker's claim context changed).
- **(T4.7 part 1 fix round 2) Tests to run with `-race` on CI.** They were run with `-count=100
  -cpu=1,2,4` alone and in the package stress instead (no C compiler here). Changed:
  `TestSyncRateRadiusHalfRegistryFull`. It was rewritten: the stripe worker, the rounds and the
  test's own registry edits (`HoldClaims`, `EnqueueStripes`, `ReleaseStripe`) all reach the
  registry under `r.mu` while the half's transfer runs.
- **(T4.7 part 1 re-review; recorded by the orchestrator at landing) Lows.**
  1. **→ T4.7 part 3 (AC-B2 headroom).** An own job cancels a running claim of the same range, and
     the IDs the claim listed are listed again. The session-root check (scheduler.go ~L427)
     registers a root claim with no `IsBootstrap` guard, and `preemptLocked` (stripes_transfer.go
     ~L1229–1236) cancels it in flight. Two triggers: a failed bootstrap while `IsBootstrap` still
     holds (the next round's `planCatchUp` clears `bootstrapFailed` and returns false, the
     session-root check registers the root as a claim and the worker starts it, and a round later
     the own bootstrap preempts it); and start-up, when no peer is admitted yet, `peerStatuses` is
     empty, `IsBootstrap` is false and a served session registers a root claim that the first
     round's bootstrap cancels. At 2^22 that is up to ~4–6 MB per running stripe, at most 4
     stripes, per cycle, against AC-B2's ~45 MB headroom. Fix direction: skip the session-root
     registration while `IsBootstrap` holds (as `servedStriper.Stripe` does, stripes_transfer.go
     ~L1056), and/or let the bootstrap take over a running claim of the same root instead of
     canceling it; cover both triggers with a test past `LargeDiff`. `planCatchUp`'s doc ("one round
     runs the per-peer sessions before the next try", ~L1289) is stale past `LargeDiff`, and
     `TestRoundBootstrapFailedResumesSessions` covers only a difference of 5.
  2. **Records.** The verification round's bound moved from one `StripeTimeout` (T3.3, phase-3
     L136) to the inactivity timeout within 9 × `StripeTimeout` (stripes_verify.go ~L144); T3.3's
     rule is superseded there too (not exploitable: one session is capped at 10 minutes and a 15 s
     fetch timeout applies). The residual "a liar's claim holds the stripe worker for as long as its
     own stripe lasts" omits the claim's verification phase and Sybil reassignment (up to 4 lying
     peers in a row); part 3 should size it with both.
  3. **Nit → part 3.** Once a peer is at its claim cap, `enqueueStripes` still tries every remaining
     range; each bumps `stripes_total{dropped}` and logs, inflating the metric by hundreds per
     session.
  4. **Pre-existing, widened by part 1 → T6.3 / before mainnet.** A Sybil majority of the peers that
     share the node's root can trigger `IsBootstrap` through the median. That was a real own job
     before; since part 1 it also sets the served sessions' threshold to 0 over the root, so the
     node declines every lack in the sessions it serves while that job runs.
- **(T4.7 part 2 → the proposal) `GrowthFactor` 2: the text to update.** pkg/rangesync's
  `GrowthFactor` is 2 since T4.7 part 2; the proposal still has 4 in seven places (not edited here).
  The line numbers are pullsync-rangesync-rateless-proposal.md's on rangesync/phase-4; each new text
  is what `TestWorkedExamples` now takes (worked_examples_test.go, its case list):
  - L467 (§4.3.7, the decoding flowchart): `REQ["More(R, up to 4 × b)"]` → `REQ["More(R, up to
    2 × b)"]`.
  - L824 (§6.3, the `Symbols{R, …}` row): "emit `More{R, UpTo: min(4 × received, cap(R))}`" →
    "emit `More{R, UpTo: min(2 × received, cap(R))}`" (the code also caps it at the session's
    remaining `SessionSymbolBudget`, T2.5).
  - L830 (§6.3, Constants): "batch factor 1.6, growth factor 4" → "batch factor 1.6, growth factor
    2".
  - L894 (§7.2): "A sends `More(root, UpTo: 4 × 7 = 28)`" → "A sends `More(root, UpTo: 2 × 7 =
    14)`".
  - L1175 (§7.10, example 2's note): "If 68 symbols were not enough, one More(root, UpTo 272) adds
    a round trip." → "If 68 symbols were not enough, one More(root, UpTo 136) adds a round trip
    (a second, UpTo 272, for a few draws)." (measured: 6 of 10 draws decode at 68, 3 at 136, 1 at
    272).
  - L1191–1195 (§7.10, example 3's sequence): the two More/Symbols round trips become four:
    `More(root, UpTo 8)`, `Symbols(root, 4…8)`, `More(root, UpTo 16)`, `Symbols(root, 8…16)`,
    `More(root, UpTo 32)`, `Symbols(root, 16…32)`, `More(root, UpTo 64)`, `Symbols(root, 32…64)`
    (the peel lines between them as now); L1199's note: "2 extra round trips" → "4 extra round
    trips" (11 Messages with parts, the same 64 × 82 B + 12 IDs; 9 Messages and 32 symbols when 32
    decode the 25 differences: 1 of 5 draws). The "about 2 KB of it overshoot" stays (64 symbols
    against ~40).
- **(T4.7 part 2 → the proposal) The per-node initiated-session cap in §8.2.** §8.2 (L1224–1231)
  has no rule for how many sessions a node opens at once; add after "Rounds": "**Cap.** A node
  opens at most a per-node number of sessions at once (round sessions, takeovers, stripe transfers
  and verifications together; at most 4 of them stripe sessions). With H eligible peers of a
  higher overlay and L of a lower one: while H + L ≤ 16 (`MaxConcurrentSessions`, the session
  tokens it shares with the sessions it serves; neighborhoods of up to 17 nodes) it is `max(8,
  min(H + 4, 16 − L))`, so that a round reaches every higher-overlay peer in one wave; past that it
  is 8, which leaves half of the tokens to the sessions it serves." (reconciler.InitiatedSessionCap,
  T4.7 part 2 Decisions.)
- **(T4.7 part 2 → T6.3) The per-node cap on testnet.** The rule steps at 17 nodes: up to 16
  eligible peers a node's cap covers every higher-overlay peer (up to 16 slots), from 17 it is 8
  (the pre-G6 constant). Measured at N = 9, 17, 25 and 33 (harness); N = 18–24 runs the pre-G6
  cap and was not measured on its own, and a slope between the two (fewer slots per peer past the
  tokens) was not tried. T6.3 should read, per neighborhood size, `sessions_total{busy}`,
  `sessions_total{session_limit}` and `session_busy_total` against the completed sessions (~5% at
  N = 25, ~8% at N = 33 in the harness), and AC-B1 at N ≥ 17. There is no gauge of the cap; a
  node's cap follows from its eligible peers by `InitiatedSessionCap` (add one, e.g.
  `initiated_session_cap`, if T6.0 wants it on the dashboard).
- **(T4.7 part 2 → T6.0, AC-B1) Re-measured with the landed constants.** N = 17 passes with ~10
  points at every validated Delay (39.2–40.3%), N = 9 fails by ~10 (59.3–60.5%, 54–56% on the
  wire): the re-basing at N = 9 stays T6.0's call (Decisions, "(T4.7 part 2) AC-B1 re-measured").
  AC-B1's prediction ("28–35% at N = 9, 17–22% at N = 17") should be replaced by these figures.
- **(T4.7 part 2) Tests to run with `-race` on CI.** No C compiler here: they ran with
  `-count=20 -cpu=1,2,4` (the slot pool's `-count=50`, the whole reconciler package `-count=3`,
  the harness tests `-count=2`). New:
  `TestSlotPool`, `TestSlotPoolCancelRace`, `TestInitiatedSessionCap`, `TestRoundSessionCapPerNode`,
  `TestRoundSessionCapFollowsPeers`, `TestRoundSessionCapStripesTakeovers` (reconciler);
  `TestNeighborhoodPastTokens` (rangesync). Changed: `TestRoundSessionCap`,
  `TestRoundSessionCapSharedWithStripes`, `TestStripeTimeoutAfterSlot`,
  `TestSessionRecheckAfterSlot`, `TestStripeBusyDoesNotStall`, `TestPeerStateResetIgnoresStaleRuns`,
  `TestRangesyncOnlyGoroutines` (reconciler); `TestProtocolConverges`, `TestWorkedExamples`
  (rangesync). Every session the reconciler opens now takes its slot from the new semaphore, so
  re-run the whole reconciler package and the neighborhood tests (`TestNeighborhood*`).
- **(T4.7 part 2 fix round 1 → the proposal) §8.2's cap: the continuous rule.** Supersedes the
  text of "(T4.7 part 2 → the proposal) The per-node initiated-session cap in §8.2": add after
  "Rounds": "**Cap.** A node opens at most a per-node number of sessions at once (round sessions,
  takeovers, stripe transfers and verifications together; at most 4 of them stripe sessions).
  With H eligible peers of a higher overlay and L of a lower one, and E = max(0, H + L − 16) of
  them past the 16 session tokens (`MaxConcurrentSessions`) it shares with the sessions it serves,
  it is `max(8, min(H + 4, 16 − L − E))`: up to 17 nodes a round reaches every higher-overlay peer
  in one wave; past that each peer past the tokens takes a slot off, down to 8 at every rank from
  25 nodes on, which leaves half of the tokens to the sessions it serves."
  (reconciler.InitiatedSessionCap; Decisions, "(T4.7 part 2 fix round 1) The per-node cap slides".)
- **(T4.7 part 2 fix round 1 → T6.3) Mixed radii: a Busy breakdown by the opener's cap.**
  Supersedes the last sentence of "(T4.7 part 2 → T6.3) The per-node cap on testnet" (the gauge
  now exists) and its "steps at 17 nodes" (it slides to 8 by 25). Each node judges "past the
  tokens" by its own eligible peers, which may not match its targets': with mixed radii a deeper
  node sees ≤ 16 peers and takes up to 16 slots, opening sessions to shallower targets that see
  more than 16 and are short of tokens. The harness runs one radius, so this is unmeasured. On a
  testnet with mixed radii, break `bee_rangesync_sessions_total{result=busy}` and
  `{result=session_limit}` (opening side) and `bee_rangesync_session_busy_total` (serving side)
  down by the opener's `bee_rangesync_initiated_session_cap` and both sides' depth, against the
  completed sessions (harness, one radius: 1–3% of the sessions at N = 18, 3–6% at N = 21, 5–10%
  at N = 25). If the deeper nodes' sessions draw most Busy answers, size the cap by the targets'
  peer counts (a Status field) or count a shallower target's peers; if Busy at N ≈ 20–24 runs
  above N = 25's, a steeper slope (two slots per peer past the tokens, the floor from N = 21) is
  the next candidate (not measured).
- **(T4.7 part 2 fix round 1 → T6.0, AC-B1) N = 18–24 pass by 1.5–4 points, not ~10.** With the
  continuous cap every measured N ≥ 17 passes AC-B1 at Delay 50 and 200 ms (N = 17 39.5–40.0%, 18
  45.8–46.4%, 21 46.0–48.5%, 25 42.8–44.4%, 33 36.1% at 50 ms), with the least margin at N = 21,
  200 ms (48.2–48.5%). With the step as landed in part 2, N = 18 failed at 200 ms (52.3–52.9%).
  This refines "(T4.7 part 2 → T6.0, AC-B1) Re-measured with the landed constants": AC-B1's
  figures should cite N = 18–21 too, and T6.0 should measure AC-B1 on testnet at N ≈ 20.
- **(T4.7 part 2 fix round 1) Tests to run with `-race` on CI.** No C compiler here: they ran with
  `-count=20 -cpu=1,2,4` (the whole reconciler package `-count=3`, `TestNeighborhoodPastTokens`
  `-count=2`, pkg/rangesync `-count=1`). Changed: `TestInitiatedSessionCap`, `TestRoundSessionCap`,
  `TestRoundSessionCapPerNode`, `TestRoundSessionCapFollowsPeers` (the gauge) (reconciler). The
  new gauge is read under the slot pool's lock from the metrics collector while rounds resize it,
  so also run `TestPeersGauge` and `TestMetrics` and the whole reconciler package.
- **(T4.7 part 3 → T6.3) A claimant that drips holds the stripe worker.** A peer whose claim's
  job stores a chunk this node lacks and accepts at least every `claimIdleTimeout` (2 minutes:
  about 720 stamped chunks a day per victim, uploads of its own) keeps its claim on the one stripe
  worker as an honest slow peer would, up to its stripes' budgets (`stripePeerBudget` ×
  `StripeTimeout` per peer, up to 4 peers, and its verification's: ≈ 11 h per claim); the other
  claims (a responder-only lack, a constraint-(d) session root) wait behind it, this node's own
  transfers do not. It holds `SyncRate` only through its chunks' window, as any catch-up does.
  Directions: a cap on a claim's worker time of at least an honest claim's transfer at the fetch
  rate (≈ 70 minutes for a whole root at 2^22), or turns between the claimants (a turn that ends
  lists again what its stripes had listed and not fetched, at most `stripeQueue` batches per
  stripe); part 4's fetch credit makes each dripped chunk a real one but does not shorten the
  hold. Not measured.
- **(T4.7 part 3 → T6.3) An honest claim with a stalling stripe peer is planned again.** A claim's
  job ends `claimIdleTimeout` after its honest stripes' last chunk, before a stalling peer's stripe
  is reassigned (`StripeTimeout`, 15 minutes), so that part of the range waits for the claimant's
  next session or round to register the claim again; the new plan follows the map order of the
  peers ("(T4.7 part 1, low) `PlanStripes` follows the map order of the peers"), so with s stalling
  peers among k that cover the range it takes about k / (k − s) registrations, each listing only
  what is still missing. Directions: a claim's stripes on an inactivity timeout below
  `claimIdleTimeout`, so that they are reassigned within the job, or a re-registered claim planned
  away from the peers its last job found stalling. Not measured with the harness.
- **(T4.7 part 3, re-review Low 1, the rest) An own job still ends a claim in flight of another
  range.** `adoptLocked` takes over only a claim of the own job's range. A bootstrap of the root
  still ends a running claim inside it (a served claim of a child range registered before
  `IsBootstrap` turned true), and a radius decrease's new half ends a running claim that contains
  it: between the root change and the round that registers the half, a session this node serves
  for a peer with the new root offers the new root itself (the peer counts about twice this
  node's), which the served Striper registers as a claim. What the ended claim listed and had not
  fetched is listed again (the re-review's ≈ 4–6 MB per running stripe at 2^22, at most 4
  stripes). Directions: the served Striper registers no claim that overlaps a pending half (as it
  registers none while bootstrapping), or an own job takes over a containing claim. Not measured.
- **(T4.7 part 3 → T6.3) The Sybil-made bootstrap, after part 3.** The re-review's Low 4 (a Sybil
  majority of the peers that share this node's root triggers `IsBootstrap` through the median)
  now holds `SyncRate` above 0 for one `ownJobGrace` in all until an own transfer stores a chunk
  (Decisions, "(T4.7 part 3) The stripe-registry `SyncRate` pin, as built"); its threshold-0
  effect on the sessions this node serves is unchanged and stays with T6.3.
- **(T4.7 part 3 → T6.1/T6.3) Claims' first-chunk window.** A claim reads `SyncRate` 0 from the
  end of the catch-up grace to its first stored chunk, as a catch-up fetch does (T4.5 part 2c
  round-2 Lows, 1): the responder-only lack and a constraint-(d) session root after start-up (at
  start-up the start-up guard holds while a session-root lack keeps it unarmed). Measure the
  claims' first-chunk latency on testnet with the stripes' (T4.6's first-chunk latency item).
- **(T4.7 part 3 → the proposal) §8.4: the holds.** §8.4 (L1247–1262) defines `SyncRate` and the
  start-up guard; what else holds it is not in the proposal. After "Steady-state rounds, which carry
  live traffic, do not count; …" add: "While catch-up work runs and has stored nothing yet,
  `SyncRate` is held at a small non-zero rate for a bounded time only: a catch-up fetch (a large
  session's or a stripe's) within one node-wide grace of 2 s, and this node's own striped
  transfers (its bootstrap and a radius decrease's new half) within one node-wide grace of 10
  minutes, each refilled only by a chunk that work stores; a radius decrease's new half that
  waits for its transfer for at most 10 minutes. A range a peer's count made this node stripe
  holds nothing of its own, so no peer can keep `SyncRate` above 0 by what it claims."
  (Decisions, "(T4.7 part 3) The stripe-registry `SyncRate` pin, as built".)
- **(T4.7 part 3) The one-chunk-then-reset pin (T4.5 part 2c round-2 Lows, 2) was not done** (it
  was optional); part 4's per-peer fetch credit is the fix that bounds it.
- **(T4.7 part 3) Tests to run with `-race` on CI.** No C compiler here: they ran with `-count=20
  -cpu=1,2,4` (the reconciler package `-count=3`, the pkg/rangesync ones `-count=5`). New:
  `TestSyncRateClaimPin`, `TestSyncRateClaimDrip`, `TestSyncRateHonestChurn`,
  `TestSyncRateOwnJobGrace`, `TestClaimStoringNotEnded`, `TestRoundBootstrapFailedPastLargeDiff`,
  `TestRoundBootstrapTakesOverStartupClaim`, `TestOwnJobTakesOverClaim`, `TestClaimCapDropsOnce`,
  `TestSyncRateCatchUpAfterGraceDrained` (reconciler); `TestFetchWithProgressNotStored`
  (rangesync). Changed: `TestSyncRateBootstrap`, `TestSyncRateRadiusDecreaseSequencing`,
  `TestSyncRateRadiusHalfRegistryFull`, `TestDualModeSyncRate` (reconciler). The claim's idle
  clock runs a goroutine and calls back under the reconciler's lock, every stored chunk of a
  stripe job takes that lock on its fetch goroutine (`jobStored`), and a round may turn the job in
  flight into an own job (`adoptLocked`) while the worker runs it, so re-run the whole reconciler
  package, and `TestNeighborhoodBootstrap*` (rangesync).
