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
