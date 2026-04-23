# Accounting as an encapsulating protocol middleware — design

**Status:** Design (brainstormed, not yet planned)
**Date:** 2026-04-23
**Scope:** `pkg/accounting`, `pkg/settlement/pseudosettle` (retired), all protocols that currently call into accounting (`pkg/pushsync`, `pkg/retrieval`, future protocols)

---

## 1. Motivation

Today, individual protocols (`pushsync`, `retrieval`) are aware of accounting: they directly call `PrepareCredit`, `PrepareDebit`, `Apply`, and `Cleanup` at specific points in their handlers, intertwined with wire events. This has four consequences that justify a rewrite:

1. **Protocol code carries accounting-shaped bugs.** The existing integration has latent issues (reservation leaks on settle failure inside `PrepareCredit`; non-atomic balance-plus-originated writes; swallowed `ErrOverRelease`; concurrent `debitAction.Apply` races) that manifest at the boundary between protocol logic and accounting logic, precisely because the boundary is implicit.
2. **Accounting couples to pricing.** Proximity-order pricing decisions are made by protocols and passed as scalars, but the call-site shape (which pricer method to use, when) is load-bearing in ways that require reading both the protocol and the accounting code to understand.
3. **The settlement layer contains a wire protocol (`pseudo-settle`) whose primary job is to reconcile discrepancies** that the asymmetric commit model creates. Remove the commit asymmetry and the reconciliation protocol becomes unnecessary.
4. **New protocols inherit this cost.** Any future chargeable protocol has to re-learn the Prepare/Apply/Cleanup dance, the error taxonomy, the ghost-balance invariant, the race windows. That's not sustainable.

This design makes accounting an **outside-in middleware** around protocols. Protocols declare chargeable cycles; the middleware handles reservation, commit, rollback, rate-limiting, and settlement invisibly.

## 2. Goals and non-goals

### Goals

- Protocols express *what is chargeable* (cost + direction + peer), not *when/how to reserve and commit*.
- Eliminate the `pseudo-settle` wire protocol by making commits symmetric on both sides by construction.
- Eliminate the known latent bugs in today's code at structural boundaries (leaks, races, atomicity gaps).
- Preserve every load-bearing invariant of today's accounting (see §5 of the synthesis that informed this doc).
- Each accounting concern (ledger math, reservations, cycle tracking, rate-limiting, settlement triggering) isolated to its own testable unit.

### Non-goals

- Backward compatibility with today's `accounting` package API. This is a greenfield rewrite; callers change.
- Changing the economic model (proximity-order pricing stays a protocol concern; the middleware doesn't see it).
- Replacing swap or monetary settlement. The middleware plugs into swap via the same `PayFn` callback shape as today.
- Auditability / event-sourced ledger. Considered and rejected as overkill for the problem.

## 3. Decisions (the axes the design fixes)

| # | Decision | Rationale |
|---|---|---|
| 1 | **Outside-in middleware** wrapping protocols | Protocols become accounting-unaware |
| 2 | **Per-cycle granularity**, multiple cycles per stream allowed | Doesn't over-constrain protocol design |
| 3 | **`RunCycle(ctx, spec, func() error)`** as the protocol↔middleware interface | Low magic; one line per chargeable operation; semantic errors via closure return |
| 4 | **Symmetric commit via cycle-ACK** with server-stamped timestamp | Eliminates the commit-asymmetry race window by construction |
| 5 | **ACK is carried in the existing response message** where one exists (retrieval Delivery, pushsync Receipt); synthesized trailer only for one-way protocols | Zero extra wire messages on the hot path for current protocols |
| 6 | **Lost-ACK recovered via `lastConfirmed` piggyback** on the next request | Replaces pseudo-settle's reconciliation function without a dedicated wire protocol |
| 7 | **Greenfield rewrite**; old `accounting` package and `pseudosettle` deleted | No coexistence burden; all protocols cut over at once |
| 8 | **Peer-identity-scoped balances**; survive disconnect and restart | Ledger represents economic relationship, not transport state; removes today's Connect-zeroes-persisted-balance footgun (`accounting.go:1458-1466`). Equivalence testing against today's `Connect` behavior deliberately diverges here — this is a bug fix, not a preserved invariant. |
| 9 | **Balance-only persistent state** per peer (drop `surplus`, drop `originated`) | **Semantic change from today:** overpayment flows through balance as negative debt (consumable credit for future debits), not into a debt-paying-only surplus bucket. This is not an equivalent re-encoding of today's surplus — it changes what over-credit *does* on the next cycle. Forwarders pay for all committed debt; network-wide-insurance of `originated` is gone. |
| 10 | **Post-`Apply` swap trigger**; off the reserve-to-commit critical path | Settlement is a consequence of committing, not a precondition for reserving |
| 11 | **Per-peer monotonic cycle IDs**, assigned by stream opener | Minimal state; ordering for free; makes `lastConfirmed` a single watermark |
| 12 | **Universal middleware coverage** | One code path for all p2p protocol registration; non-chargeable protocols declare zero cycles |
| 13 | **In-memory open-cycle set** (watermark persisted); server crash mid-cycle loses pending debits but can't overcharge | Safe-direction asymmetry; avoids a write per cycle-open |

## 4. Architecture

```
                   ┌───────────────────────────────────────┐
                   │   Protocol handlers (pushsync, …)     │
                   │                                       │
                   │   middleware.RunCycle(ctx, spec, fn)  │
                   └──────────────────┬────────────────────┘
                                      │
                              ┌───────▼─────────┐
                              │   Middleware    │   (thin facade)
                              └──┬─┬─┬─┬─┬──────┘
              ┌──────────────┬─┬─┴─┘ │ │ │
              │              │ │     │ │ │
              ▼              ▼ │     │ │ ▼
        ┌──────────┐  ┌─────────┴─┐  │ │ ┌───────────────┐
        │CycleTrkr │  │RateLimiter│  │ │ │ Settle Trigger│
        └────┬─────┘  └─────┬─────┘  │ │ └──────┬────────┘
             │              │        │ │        │
             │              │        ▼ ▼        │
             │              │   ┌────────────┐  │
             │              │   │Reservations│  │
             │              │   └─────┬──────┘  │
             │              │         │         │
             │              │         ▼         │
             │              │   ┌────────────┐  │
             └──────────────┴──▶│  Ledger    │◀─┘
                                └─────┬──────┘
                                      ▼
                               persistent store
                                      ▲
                                      │ (swap.Pay result)
                               ┌──────┴──────┐
                               │  Swap       │ (unchanged)
                               └─────────────┘
```

**Dependency direction is strictly downward.** Middleware depends on the five inner components; no component depends on the Middleware. Ledger is the only component with a persistent-store dependency.

**Data flow:** Middleware calls CycleTracker, RateLimiter, Reservations synchronously during `RunCycle`. Reservations → Ledger on Commit. SettlementTrigger observes Ledger via an in-line post-commit callback. Swap is invoked asynchronously from SettlementTrigger and calls back into Ledger on completion.

**Outside the package:** cost computation (protocol-supplied), peer-overlay resolution (p2p layer), blocklist enforcement (p2p layer, driven by a policy callback from Reservations).

## 5. Components

### 5.1 Shared types

```go
type Direction uint8
const (DirCredit Direction = iota; DirDebit)

type CycleSpec struct {
    Peer      swarm.Address
    Amount    *big.Int
    Direction Direction
}

type CycleHandle struct {
    ID           uint64  // monotonic per (self, peer, direction)
    ackTimestamp int64   // set by protocol fn before returning nil
    // unexported: references to Reservations txn
}
```

Sentinel errors (caller-inspectable):

```go
ErrOverdraft          // Reservations: reservedCredit + current debt would exceed allowance
ErrPeerBlocklisted    // Ledger: peer is under blocklist
ErrCycleSuperseded    // CycleTracker: ACK arrived for a stale cycle ID (internal — never returned to protocol)
ErrContext            // wraps ctx errors during lock acquisition
ErrClockDrift         // RateLimiter: ACK timestamp outside ±tolerance
ErrInvalidACK         // CycleTracker: ACK doesn't reference a known cycle
```

### 5.2 Ledger

```go
type Ledger interface {
    Balance(peer) (*big.Int, error)
    Debit(peer, amount) error              // balance += amount
    Credit(peer, amount) error             // balance -= amount
    NotifyPaymentSent(peer, amount) error  // balance += amount (swap cheque sent)
    Subscribe(PostCommitFn)                // SettlementTrigger hooks here
    EvictDormant(policy EvictPolicy) (int, error)
}
```

- State: one `*big.Int` per peer, persisted; hot-peer in-memory cache.
- Invariants: sign convention; atomic per-peer balance writes; `PostCommit(peer, newBalance)` event emitted after every mutation, under the peer lock.
- Dependencies: persistent store, peer-lock registry.

### 5.3 Reservations

```go
type Reservations interface {
    Begin(ctx, peer, amount, dir) (Txn, error)
}

type Txn interface {
    Commit() error    // moves reservation → Ledger mutation
    Rollback() error  // Credit: releases reserve; Debit: moves to ghostDebit
}
```

- State (per peer, in-memory only): `reservedCredit`, `reservedDebit`, `ghostDebit`, `lastACKTimestamp`. (In-flight monetary payment tracking lives in SettlementTrigger, not here — see §5.6.)
- Invariants:
  - Begin → Commit or Rollback exactly once (type-enforced, not convention).
  - **Reserve-side gate (core overdraft-prevention invariant):** on `Begin(peer, amount, DirCredit)`, require `Ledger.Balance(peer) negated + reservedCredit + amount ≤ paymentThreshold + RateLimiter.TimeBudget(peer, now) + SettlementTrigger.inflightPayment[peer]`. Violated ⇒ `ErrOverdraft`, no state change.
  - Debit Rollback accumulates to `ghostDebit`.
  - `ghostDebit > disconnectLimit` triggers blocklist callback.
- Dependencies: Ledger (for committed balance read and commit mutation), RateLimiter (for time-budget component of overdraft check on DirCredit), SettlementTrigger (for in-flight-payment component of same check), BlocklistPolicy (for ghost-overflow / disconnect-threshold).

The `settle()`-inside-`PrepareCredit` leak from today's design goes away by construction: Begin has no side-effects on swap; settlement is a post-commit observer.

### 5.4 CycleTracker

```go
type CycleTracker interface {
    NextID(peer, dir) uint64
    OpenCycle(peer, dir, id, amount)
    ConfirmCycle(peer, dir, id) error
    Watermark(peer, dir) uint64
    RetroConfirm(peer, dir, upto) []uint64
}
```

- State: per (peer, dir): counter, open-cycle set, last-confirmed watermark. Counter + watermark persisted; open-cycle set in-memory (lost on crash — see §3 decision 13).
- Invariants:
  - Cycle-ID monotonicity per (peer, dir); watermark only advances.
  - `ConfirmCycle(id)` fails with `ErrCycleSuperseded` if a higher watermark subsumes it; `RetroConfirm` is idempotent.
  - **`ConfirmCycle` and `RetroConfirm` are called under the peer lock** (the same lock held by `Reservations.Commit`), ensuring atomicity with the balance mutation. Without this, a late-arriving cycle-N ACK could race with a later cycle's watermark bump.
  - **`ErrCycleSuperseded` is never propagated out of `RunCycle`** — the Middleware swallows it (the superseded cycle was already retro-committed by the piggyback path).
- Dependencies: persistent store.

### 5.5 RateLimiter

```go
type RateLimiter interface {
    TimeBudget(peer, now) *big.Int                // extra allowance from elapsed time since lastACK
    ValidateACKTimestamp(peer, ts, now) error     // ±drift + monotonic
    NotifyCommit(peer, amount, dir, ackTs)
}
```

- State (per peer): `lastACKTimestamp`. Persisted for restart continuity.
- Invariants:
  - `TimeBudget(peer, now) = (now - lastACKTimestamp[peer]) × refreshRate`.
  - The reservation gate (owned by Reservations, see §5.3) combines `TimeBudget` with `paymentThreshold` and `SettlementTrigger.inflightPayment`; RateLimiter itself does not gate.
  - `ValidateACKTimestamp`: ACK timestamp must be monotonically non-decreasing per peer; clock drift ≤ tolerance (today's ±2s absolute, ±3s interval preserved).
- Dependencies: none (pure function of time + its own state).

### 5.6 SettlementTrigger

```go
type SettlementTrigger interface {
    OnPostCommit(peer, newBalance)
    SetPayFunc(PayFn)
}

type PayFn func(ctx, peer, amount) error
```

- State: `inflightPayment[peer]`, `paymentOngoing[peer]`, `lastSettlementFailureTs[peer]`. **`inflightPayment` is owned here, not by Reservations** (Reservations reads it via dependency).
- Invariants:
  - **No double-pay:** `paymentOngoing[peer] ⇒ PayFn is not invoked for peer`.
  - **In-flight deduction:** the amount passed to `PayFn(peer, amount)` equals `max(0, -Ledger.Balance(peer) - inflightPayment[peer])`, never more.
  - **Minimum-payment floor:** `PayFn(peer, amount)` is invoked only when `amount ≥ minimumPayment` (`minimumPayment = refreshRate / 5`, preserving today's `accounting.go:235`).
  - **Failure cooldown:** after a failed settlement, no new `PayFn` invocation for that peer until `now > lastSettlementFailureTs[peer] + 10s` (strict `>`, matching today's `failedSettlementInterval` at `accounting.go:473-474` — the effective wait is >10s, not exactly 10s).
  - **Semantic-change note (see §3 decision 9):** if a payment causes `Ledger.Balance(peer)` to cross below zero (over-credit), the excess remains in balance as spendable negative debt; subsequent debit cycles consume it before recording new positive debt. This is *not* today's surplus semantic.
- Dependencies: Ledger (read `Balance`, write via `NotifyPaymentSent`), Swap (injected `PayFn`).

### 5.7 Middleware

```go
type Middleware interface {
    RunCycle(ctx, spec, fn func(CycleHandle) error) error
    RegisterProtocol(spec ProtocolSpec)
    Connect(peer)      // idempotent; loads/initializes state; does NOT zero balance
    Disconnect(peer)   // idempotent; flushes volatile state; does NOT zero balance
}
```

The `RunCycle` orchestration:

```go
func (mw *Middleware) RunCycle(ctx, spec, fn) error {
    txn, err := mw.res.Begin(ctx, spec.Peer, spec.Amount, spec.Dir)
    if err != nil { return err }
    defer txn.Rollback()  // unconditional; no-op after Commit

    id := mw.cyc.NextID(spec.Peer, spec.Dir)
    mw.cyc.OpenCycle(spec.Peer, spec.Dir, id, spec.Amount)

    h := CycleHandle{ID: id, txn: txn}
    if err := fn(h); err != nil {
        return err  // defer rolls back
    }

    if err := txn.Commit(); err != nil {
        return err  // defer is idempotent no-op
    }
    mw.cyc.ConfirmCycle(spec.Peer, spec.Dir, id)
    mw.rl.NotifyCommit(spec.Peer, spec.Amount, spec.Dir, h.ackTimestamp)
    return nil
}
```

## 6. Data flow

### 6.1 Retrieval, client fetching chunk from peer P (credit path)

**Client:**
```
MW.RunCycle(ctx, {P, price, Credit}, func(h):
    R.Begin(P, price, Credit)    // RL.CheckAllowance; txn.reservedCredit += price
    C.NextID(P, Credit) = 42
    C.OpenCycle(P, Credit, 42, price)

    open stream
    write Request{cycleID:42, lastConfirmed:38}
    read Response{chunkData, cycleID:42, serverTs:T}
    if !validate(chunk): return ErrInvalid
    h.ackTimestamp = T
    return nil
)
// RunCycle resumes:
//   txn.Commit() → L.Credit(P, price) → ST.OnPostCommit(...) (may trigger swap async)
//   C.ConfirmCycle(P, Credit, 42)
//   RL.NotifyCommit(P, price, Credit, T)
```

**Server (P):**
```
on receiving Request{cycleID:42, lastConfirmed:38}:
    C.RetroConfirm(self, Credit, 38)   // piggyback cleanup

    MW.RunCycle(ctx, {client, price, Debit}, func(h):
        R.Begin(client, price, Debit)   // no overdraft check on debit
        h.ackTimestamp = now
        h.cycleID = 42                  // echoed from request

        write Response{chunkData, cycleID:42, serverTs:h.ackTimestamp}
        return nil
    )
    // txn.Commit() → L.Debit(client, price); if balance > disconnectLimit, blocklist
```

The server's `ackTimestamp` travels inside the Response; the client records the same `T`. Both sides' RateLimiters now share a canonical last-ACK timestamp. No dedicated ACK message for request-response protocols.

### 6.2 Pushsync forwarder F (receiving from A, pushing to B)

```
F's handler:
    MW.RunCycle(ctx, {A, price, Debit}, func(hA):         // outer: charge A
        read Delivery from A

        MW.RunCycle(ctx, {B, price, Credit}, func(hB):    // inner: pay B
            open stream to B
            send Delivery{cycleID:hB.ID, lastConfirmed:...}
            read Receipt from B {cycleID:hB.ID, serverTs}
            hB.ackTimestamp = serverTs
            return nil
        )
        // inner committed: B paid

        hA.ackTimestamp = now
        write Receipt to A {cycleID:hA.ID, serverTs:hA.ackTimestamp}
        return nil
    )
    // outer committed: A debited
```

Nested `RunCycle` calls. Each has its own cycle-ID namespace. Independent commit points. If inner fails, outer rolls back; A not debited. If inner succeeds but F's write to A fails, outer rolls back; F paid B but didn't collect from A — an economic loss to F in a narrow window (analogous to today's pushsync invariant #11, tests in `pushsync_test.go:609-700`).

### 6.3 Lost-ACK recovery via piggyback

Client commits cycle 42 locally (read Response before crash). Server crashes between writing Response and draining its TCP buffer — has no record of cycle 42 committed.

On client's next request (cycle 43): the request carries `lastConfirmed: 42`. Server handler:

1. `C.RetroConfirm(self, Credit, 42)` returns `[39, 40, 41, 42]` (open-but-not-confirmed cycles).
2. For each, retroactively runs the commit path: `L.Debit(client, price)`.
3. Both sides now agree.

**Crash-recovery caveat:** open-cycle set is in-memory (per decision 13). Server crash → open cycles lost → retro-confirm returns nothing for cycles open at crash time. Client's ledger has the debt; server's doesn't. Divergence is in server's disfavor — the safe direction (no false debt assignment). Accepted tradeoff.

## 7. Error handling

| Error | Produced by | Middleware response | Protocol sees |
|---|---|---|---|
| `ErrOverdraft` | RateLimiter via Reservations.Begin | Begin fails; no txn | returned from RunCycle; protocol retries different peer |
| `ErrContext` | Reservations.Begin (TryLock) | Begin fails; no state | returned from RunCycle |
| `ErrPeerBlocklisted` | Ledger / Reservations | Begin fails | peer unavailable until unblocked |
| `ErrClockDrift` | RateLimiter (inside fn, via `h.validateACK`) | fn returns → Rollback; blocklist callback fires | returned; peer blocklisted |
| `ErrInvalidACK` | CycleTracker | fn returns → Rollback | protocol-level error |
| `ErrCycleSuperseded` | CycleTracker.ConfirmCycle | swallowed by Middleware (idempotence) | never seen |
| Protocol-level error | fn | Rollback; bubble up | as-is |
| `ErrDisconnectThresholdExceeded` | Reservations.Commit via Ledger post-debit check | Commit succeeds; blocklist fires after | nil from Commit; p2p closes connection |

### Rollback semantics by direction

```go
func (t *Txn) Rollback() error {
    switch t.dir {
    case Credit:
        peer.reservedCredit -= t.amount
    case Debit:
        peer.reservedDebit -= t.amount
        peer.ghostDebit    += t.amount
        if peer.ghostDebit > peer.disconnectLimit {
            blocklistPolicy.Block(t.peer, ghostOverdrawDuration(peer))
        }
    }
}
```

Ghost debit is in-memory, reset on disconnect — same as today's `ghostBalance`. Prevents peer from spamming us with cycles they'll never let commit.

### Idempotent Commit / Rollback

Both are idempotent no-ops after the txn is finalized. Middleware uses unconditional `defer txn.Rollback()` without checking whether Commit ran. Double-calling either is a compile-time no-op on a consumed txn (use-once type).

### Blocklist policy — extracted

```go
type BlocklistPolicy interface {
    Block(peer, duration, reason string)
}
```

Injected into Reservations and Ledger. Duration formula (same as today's `blocklistUntil`):

```
disconnectFor(peer) = (effectiveDebt(peer) + paymentThreshold) × multiplier / refreshRate
rawDebt            = max(0, balance + reservedDebit + ghostDebit)
effectiveDebt      = max(rawDebt, refreshRate)    // floor ensures minimum penalty
```

The `refreshRate` floor is required to match today's behavior (`accounting.go:1407-1409`) — without it, a peer with near-zero effective debt would be blocklisted for a near-zero duration, defeating the accountability intent. Separating the policy from state-teardown means each can be tested independently.

### Edge cases preserved from today

- Context cancel during Begin: respected via TryLock; no txn created.
- Storage failure during Commit: Ledger returns error; Commit fails; defer Rollback releases reservation.
- Settlement fails mid-flight: SettlementTrigger handles via inflight counter, retry gate, 10s cooldown. Doesn't propagate to RunCycle.
- Concurrent RunCycle to same peer: each takes peer lock in Begin and releases, reacquires in Commit. Ledger writes are lock-protected. Today's concurrent-Debit race is eliminated by a single Ledger mutation path that holds the lock for the full balance RMW.
- `ErrOverRelease` swallow: eliminated by construction (use-once txn types make double-Release a compile error). Note: this is a Go type-system obligation, not a runtime invariant — formal verification at the algorithm level cannot check it; the TLA+ model should instead assert that each txn transitions exactly once from Begun to Finalized.

### Errors the protocol needs to distinguish

Just three classes:

- `ErrOverdraft`, `ErrPeerBlocklisted`, `ErrContext` → peer unavailable; try another.
- `ErrClockDrift` → peer misbehaved; already blocklisted.
- Anything else → protocol-level; bubble up.

Strictly simpler than today.

## 8. Testing

### Per-layer unit tests

- **Ledger:** sign convention, persistence roundtrip, atomicity under injected failure, concurrent Debit/Credit with `-race`, Subscribe callback invariants, dormant eviction policy.
- **Reservations:** Begin → Commit arithmetic, Begin → Rollback semantics per direction, idempotent finalization, ghost-debit overflow triggers blocklist, concurrent Begin correctness, `settle()`-leak regression (bug-fix assertion vs. today).
- **CycleTracker:** NextID monotonicity, persistence, watermark advancement, stale-ID rejection, RetroConfirm idempotence.
- **RateLimiter:** allowance math against injected clock, ACK timestamp validation, drift tolerance.
- **SettlementTrigger:** threshold-crossing → Pay; no double-pay while in-flight; minimum-payment floor; 10s cooldown on failure; deduplication of in-flight amount (the race flagged during brainstorm); overpayment landing → negative balance flows back.

### Integration tests

- Happy-path cycle: balances agree symmetrically on both simulated nodes.
- Lost-ACK recovery via piggyback: drop final message of cycle N; cycle N+1 includes `lastConfirmed: N`; server retro-commits; balances reconcile.
- Forwarder pattern: inner fail rolls back outer; outer succeeds commits both.
- Adversarial: manipulated timestamps trigger blocklist; debit-spam accumulates ghost debt; concurrent hedged retrieval with one winner.

### Equivalence tests (against today's observable behavior)

- Balance persistence roundtrip — store format interop or explicit migration tested.
- Blocklist duration formula — matches today's `blocklistUntil` for equivalent debt scenarios.
- Swap cheque production — same inputs produce same cheques as today's `settle()`.

### Property-based / fuzz

- Reservations: random Begin/Commit/Rollback sequences; conservation invariant.
- CycleTracker: random NextID/ConfirmCycle/RetroConfirm sequences; monotonicity invariant.
- Middleware: random concurrent RunCycle interleaving; Ledger balance matches expected-from-event-log.

### Explicitly out of scope

- Backward-compat with old `accounting` API (greenfield).
- Pseudo-settle wire-protocol tests (deleted).
- Protocol-level semantics (stay in protocol packages).

## 9. Migration notes

Greenfield rewrite, single cut-over. Both old `pkg/accounting` and `pkg/settlement/pseudosettle` are deleted in the same change as the new package lands. All protocols (`pushsync`, `retrieval`, plus any others currently touching accounting) are rewritten to use the new `Middleware.RunCycle` call shape.

No coexistence window; no feature flag. One release cycle carries the full migration.

## 10. Open questions (deferred)

- **Error model detail.** Error taxonomy was accepted as a "good starting point" during brainstorming. Expected to evolve as implementation surfaces real cases.
- **Dormant-peer eviction policy.** Exact thresholds (time since last cycle, zero-balance-only vs. small-balance) not specified; should be informed by testnet data.
- **Open-cycle persistence.** Decision 13 says in-memory. If crash-recovery data shows meaningful loss on server crashes, revisit and add persist-on-open.
- **Protocols-register-zero-cycles shape.** The exact form of `ProtocolSpec` for protocols with no chargeable cycles (hive, handshake, etc.) — placeholder, not designed.
- **Forwarding margin economics.** Today's retrieval has asymmetric PO-based pricing that rewards caching. That asymmetry is a protocol concern (middleware sees scalars), but whether the new design intentionally preserves it or moves toward uniform pricing is a product decision adjacent to this refactor.

## 11. References

- Current accounting: `pkg/accounting/accounting.go` (core), `pkg/accounting/metrics.go`
- Current pseudo-settle: `pkg/settlement/pseudosettle/`
- Protocol integration call-sites:
  - `pkg/pushsync/pushsync.go:175-326` (handler), `353-509` (pushToClosest), `526-565` (async push)
  - `pkg/retrieval/retrieval.go:253` (PrepareCredit), `374` (pricer), `424-510` (handler)
- Settlement interface: `pkg/settlement/interface.go:30-38`
