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
