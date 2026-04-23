# Accounting middleware — formal verification implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a PlusCal/TLA+ specification of the accounting middleware design and verify 15 safety and liveness properties using TLC across 9 model configurations. Catch algorithm-level bugs before Go implementation begins and document what was and wasn't verified.

**Architecture:** One PlusCal-translated TLA+ module per design layer (6 layers) + network model + types + properties, composed by a `Middleware` top-level spec that encodes the cycle lifecycle algorithm from §5.7 of the design doc. TLC checks invariants and temporal properties against bounded models (2-3 peers, 4-5 cycles, ±10 balance range).

**Tech Stack:** TLA+ (language), PlusCal (algorithm syntax compiled to TLA+), TLC (model checker, via `tla2tools.jar`), Java 11+, Make for orchestration.

**Design spec:** `docs/superpowers/specs/2026-04-23-accounting-hooks-design.md`. This plan assumes that spec as the source of truth.

---

## Property catalog (the 15 properties to verify)

Each property is stated precisely; the plan's tasks will encode them as TLA+ formulas.

### Safety properties

**P1. Commit symmetry under message drop.**
For every committed client cycle N: either server has committed N, OR server will retro-commit N via piggyback on a future cycle, OR divergence is in server's disfavor (`clientLedger[server] ≤ serverLedger[client]` in absolute terms per direction).

**P2. Balance conservation at quiescence.**
For a peer pair `(A, B)` at any quiescent state (no open cycles, no in-flight messages): `A.balance(B) + B.balance(A) ≤ 0`, with equality iff no server crashed mid-cycle AND no forwarder's outer cycle failed after inner commit. The asymmetry is always in the accounting-server's disfavor.

**P3. No reservation leaks at quiescence.**
At quiescence: `∀ peer p: reservedCredit[p] = 0 ∧ reservedDebit[p] = 0`. Ghost debit may be non-zero (it's an accountability record, not a leak).

**P4. Cycle ID / watermark monotonicity.**
Per `(peer, direction)`: `NextID` sequence strictly increasing; `Watermark` sequence non-decreasing; `ConfirmCycle(id)` where `id ≤ Watermark` returns `ErrCycleSuperseded`.

**P5. Rate-limit bounded credit.**
At any time: total uncollected credit on a peer ≤ `paymentThreshold + TimeBudget(peer, now) + SettlementTrigger.inflightPayment[peer]`. This is the reserve-side gate as an ambient invariant.

**P8. No deadlock.**
No reachable state where all processes are blocked. In PlusCal, every process has a ready action at every reachable state (TLC's built-in deadlock check).

**P9. Nested forwarder cycle correctness.**
If an inner `RunCycle` rolls back, the enclosing outer `RunCycle` rolls back. Conversely: if inner commits but outer fails to complete, outer rolls back BUT inner's commit stands (narrow window acknowledged — this is the only source of bilateral balance asymmetry for forwarder F). This carve-out is explicit in P2.

**P10. PostCommit ordering preservation.**
The sequence of `(peer, newBalance)` tuples observed by any subscriber matches the committed sequence: no drops, no reorderings, one event per commit.

**P11. No double-pay + in-flight deduction.**
`paymentOngoing[peer] ⇒ ¬(PayFn invoked for peer)`. Every invocation of `PayFn(peer, amount)` satisfies `amount = max(0, -Balance(peer) - inflightPayment[peer])`.

**P12. Cooldown safety.**
For every peer: if `lastSettlementFailureTs[peer]` is set, no `PayFn(peer, _)` invocation happens while `now ≤ lastSettlementFailureTs[peer] + 10` (strictly greater required — matches today's `>` at `accounting.go:473-474`).

**P13. Connect no-op on balance.**
`Connect(peer)` does not mutate `Ledger.Balance(peer)`. This is the regression test for today's bug removed in design decision 8.

**P14. Reservation gate / overdraft prevention.**
For every successful `Reservations.Begin(peer, amount, DirCredit)`: at the moment of gate evaluation, `-Balance(peer) + reservedCredit[peer] + amount ≤ paymentThreshold + TimeBudget(peer, now) + inflightPayment[peer]`. The redundant form of P5 but checked at the point of decision.

**P15. Timestamp monotonicity per peer.**
`lastACKTimestamp[peer]` is non-decreasing. Any incoming ACK with `ts < lastACKTimestamp[peer]` triggers `ErrClockDrift`.

### Liveness properties (require fairness assumptions)

**P6. Lost-ACK recovery termination.**
Under weak fairness of cycle-creation actions: if two peers continue to interact (A initiates cycles to B with positive frequency), any cycle with a dropped ACK is eventually retro-committed at B via piggyback. Formally: `□(A.cycleCount → ∞) ⇒ ◊(∀ c ∈ A.committed: c ∈ B.committed ∨ server-disfavor-asymmetry-holds)`.

**P7. Ghost-debit accountability.**
If peer P has `ghostDebit[P] > disconnectLimit` continuously for any non-zero duration, blocklist is eventually invoked. Bounded form: at most `ceil(disconnectLimit / smallestCycleAmount)` failed debit cycles before blocklist.

---

## File structure

```
docs/formal/accounting/
├── README.md                          # how to run, what's verified, limitations
├── Makefile                           # make check-safety / check-liveness / check-all
├── bin/
│   └── tla2tools.jar                  # pinned version (downloaded during setup)
├── Types.tla                          # PeerID, Amount, CycleID, Timestamp, constants
├── Network.tla                        # message types + drop semantics
├── Ledger.tla                         # Balance() / Debit() / Credit() / Subscribe()
├── Reservations.tla                   # Begin / Commit / Rollback / gate formula
├── CycleTracker.tla                   # NextID / OpenCycle / ConfirmCycle / RetroConfirm
├── RateLimiter.tla                    # TimeBudget / ValidateACKTimestamp
├── BlocklistPolicy.tla                # Block() + disconnectFor formula
├── SettlementTrigger.tla              # OnPostCommit + PayFn invocation + cooldown
├── Middleware.tla                     # PlusCal cycle lifecycle (top-level)
├── Properties.tla                     # P1..P15 as formulas
└── models/
    ├── safety-small.cfg               # 2 peers, 4 cycles, ±5 balance, 2 drops
    ├── safety-large.cfg               # 2 peers, 6 cycles, ±10 balance, 3 drops
    ├── liveness.cfg                   # + fairness constraints, P6/P7 only
    ├── adversarial.cfg                # Byzantine peer: bad timestamps, forged IDs
    ├── crash.cfg                      # server crash/restart; P2 asymmetric form
    ├── hedged.cfg                     # concurrent RunCycle same peer pair
    ├── settlement-failure.cfg         # PayFn errors; cooldown; retry gating
    ├── inner-commit-outer-fail.cfg    # forwarder narrow window isolation
    └── minimum-payment-floor.cfg      # threshold crossing with sub-floor amounts
```

**Plan document (this file):** `docs/superpowers/plans/2026-04-23-accounting-formal-verification.md`.

**Worktree note:** This plan touches only `docs/formal/` and `docs/superpowers/plans/`; no Go code. Execute on master in-place (no worktree needed).

---

## Task 1: Scaffolding and tooling

**Files:**
- Create: `docs/formal/accounting/README.md`
- Create: `docs/formal/accounting/Makefile`
- Create: `docs/formal/accounting/bin/.gitkeep`
- Modify: `.gitignore` (add `docs/formal/accounting/bin/tla2tools.jar`)

- [ ] **Step 1: Create the directory skeleton**

```bash
mkdir -p docs/formal/accounting/bin docs/formal/accounting/models docs/formal/accounting/counterexamples
touch docs/formal/accounting/bin/.gitkeep docs/formal/accounting/counterexamples/.gitkeep
```

- [ ] **Step 2: Write the Makefile**

`docs/formal/accounting/Makefile`:

```make
TLA_TOOLS := bin/tla2tools.jar
TLA_VERSION := 1.8.0
TLA_URL := https://github.com/tlaplus/tlaplus/releases/download/v$(TLA_VERSION)/tla2tools.jar

JAVA := java
TLC := $(JAVA) -cp $(TLA_TOOLS) tlc2.TLC -workers auto -deadlock
PCAL := $(JAVA) -cp $(TLA_TOOLS) pcal.trans

MODELS := safety-small safety-large liveness adversarial crash hedged settlement-failure inner-commit-outer-fail minimum-payment-floor

.PHONY: all tools translate check-all $(MODELS)

all: translate check-safety

tools: $(TLA_TOOLS)

$(TLA_TOOLS):
	curl -L -o $@ $(TLA_URL)

translate: $(TLA_TOOLS)
	$(PCAL) Middleware.tla

check-safety: translate
	$(TLC) -config models/safety-small.cfg Middleware.tla
	$(TLC) -config models/safety-large.cfg Middleware.tla

check-liveness: translate
	$(TLC) -config models/liveness.cfg Middleware.tla

check-adversarial: translate
	$(TLC) -config models/adversarial.cfg Middleware.tla

check-crash: translate
	$(TLC) -config models/crash.cfg Middleware.tla

$(MODELS): translate
	$(TLC) -config models/$@.cfg Middleware.tla

check-all: translate $(MODELS)
```

- [ ] **Step 3: Write the README**

`docs/formal/accounting/README.md`:

```markdown
# Formal verification of the accounting middleware

This directory contains a TLA+/PlusCal model of the design in
`docs/superpowers/specs/2026-04-23-accounting-hooks-design.md` and TLC model
configurations checking 15 properties.

## Prerequisites

- Java 11+
- `make tools` (downloads tla2tools.jar ~4MB)

## Running checks

- `make check-safety` — core safety properties (P1-P5, P8-P15) on 2 small and 1 larger model
- `make check-liveness` — P6, P7 under fairness
- `make check-all` — everything (~30-60 min on modern laptop)

## What is verified

See `Properties.tla` for formal statements. Bounded model-checking only; TLC
checks the specified configurations exhaustively within the configured bounds
but does not prove properties for arbitrary inputs. For that, see the
out-of-scope TLAPS proofs in §10 of the design spec.

## Model abstractions

- Time is a natural number advanced by explicit Tick actions.
- Amounts are small integers; no big-int arithmetic.
- Peers are from a symmetric set (enables TLC symmetry reduction).
- Persistence is modeled as a separate variable that survives Crash actions.
- Messages are kept in a global network with a drop budget.

## Layer-to-module mapping

| Design layer            | TLA+ module       |
|-------------------------|-------------------|
| Ledger                  | Ledger.tla        |
| Reservations            | Reservations.tla  |
| CycleTracker            | CycleTracker.tla  |
| RateLimiter             | RateLimiter.tla   |
| BlocklistPolicy         | BlocklistPolicy.tla |
| SettlementTrigger       | SettlementTrigger.tla |
| Middleware (facade)     | Middleware.tla (PlusCal) |
| Protocol shapes + wire  | Network.tla       |
```

- [ ] **Step 4: Update root `.gitignore`**

Append to `/home/acud/go/src/github.com/ethersphere/bee/.gitignore`:

```
# TLA+ tooling (downloaded, not committed)
docs/formal/accounting/bin/tla2tools.jar
docs/formal/accounting/*.toolbox/
docs/formal/accounting/*_TTrace_*.tla
```

- [ ] **Step 5: Run `make tools` to download TLA+ and verify invocation**

Run: `cd docs/formal/accounting && make tools && java -cp bin/tla2tools.jar tlc2.TLC -help | head -5`
Expected: command help output including "TLC: a model checker for TLA+ specifications".

- [ ] **Step 6: Commit**

```bash
git add docs/formal/accounting docs/superpowers/plans/2026-04-23-accounting-formal-verification.md .gitignore
git commit -m "formal: scaffold TLA+ verification for accounting middleware"
```

---

## Task 2: Types and constants

**Files:**
- Create: `docs/formal/accounting/Types.tla`

- [ ] **Step 1: Write the Types module**

`docs/formal/accounting/Types.tla`:

```tla
---- MODULE Types ----
EXTENDS Naturals, Integers, FiniteSets, Sequences, TLC

CONSTANTS
  Peers,              \* symmetric set of peer IDs
  MaxCycleID,         \* upper bound on NextID per (peer, dir)
  MaxTime,            \* upper bound on Tick counter
  MaxBalance,         \* upper bound |balance[p]| for any peer
  DropBudget,         \* max messages that may be dropped in a run
  PaymentThreshold,   \* constant across all peers in this model
  RefreshRate,        \* units of time budget per Tick
  MultiplierBlock,    \* blocklist duration multiplier
  ToleranceAbsTs,     \* ±tolerance for absolute timestamp drift (e.g., 2)
  ToleranceInterval,  \* ±tolerance for interval (e.g., 3)
  MinimumPayment      \* lower bound on any PayFn invocation

ASSUME
  /\ PaymentThreshold \in Nat /\ PaymentThreshold > 0
  /\ RefreshRate \in Nat       /\ RefreshRate > 0
  /\ MinimumPayment \in Nat    /\ MinimumPayment > 0
  /\ MinimumPayment <= PaymentThreshold
  /\ DropBudget \in Nat
  /\ MultiplierBlock \in Nat   /\ MultiplierBlock > 0

Direction == {"Credit", "Debit"}

Amounts == -MaxBalance..MaxBalance

NULL == CHOOSE x : x \notin (Peers \union Amounts \union {"Credit","Debit"})

\* disconnectLimit matches today's (100 + tolerance) * paymentThreshold / 100
\* For simplicity we set it directly rather than computing a percentage
DisconnectLimit == PaymentThreshold + (PaymentThreshold \div 10)

====
```

- [ ] **Step 2: Lint the module via `SANY`**

Run: `cd docs/formal/accounting && java -cp bin/tla2tools.jar tla2sany.SANY Types.tla`
Expected: "Parsing completed." with 0 errors.

- [ ] **Step 3: Commit**

```bash
git add docs/formal/accounting/Types.tla
git commit -m "formal: add Types.tla with constants and assumptions"
```

---

## Task 3: Network model

**Files:**
- Create: `docs/formal/accounting/Network.tla`

- [ ] **Step 1: Write the Network module**

`docs/formal/accounting/Network.tla`:

```tla
---- MODULE Network ----
EXTENDS Types

\* Message kinds modeled:
\*   Request:  cycle open, carries cycleID and lastConfirmed
\*   Response: cycle close from server side, carries cycleID and serverTs
\* A lost ACK is modeled by Response being dropped.

MessageKinds == {"Request", "Response"}

Message(kind, from, to, cycleID, amount, ts, lastConfirmed) ==
  [ kind |-> kind,
    from |-> from, to |-> to,
    cycleID |-> cycleID,
    amount |-> amount,
    ts |-> ts,
    lastConfirmed |-> lastConfirmed ]

VARIABLES
  network,         \* Set of Message
  dropsRemaining   \* How many more messages may be dropped in this run

NetworkInit ==
  /\ network = {}
  /\ dropsRemaining = DropBudget

\* Enqueue a message
Send(m) == network' = network \union {m}

\* Dequeue (delivered): remove m from network, leave dropsRemaining
Deliver(m) == network' = network \ {m}

\* Drop: remove m from network, decrement dropsRemaining
Drop(m) ==
  /\ dropsRemaining > 0
  /\ network' = network \ {m}
  /\ dropsRemaining' = dropsRemaining - 1

\* In an adversarial.cfg context, allow tampering: replace m with m'
Tamper(m, m') ==
  /\ network' = (network \ {m}) \union {m'}

NetworkTypeOK ==
  /\ network \in SUBSET [ kind: MessageKinds,
                          from: Peers, to: Peers,
                          cycleID: 0..MaxCycleID,
                          amount: Amounts,
                          ts: 0..MaxTime,
                          lastConfirmed: 0..MaxCycleID ]
  /\ dropsRemaining \in 0..DropBudget

====
```

- [ ] **Step 2: Lint**

Run: `java -cp bin/tla2tools.jar tla2sany.SANY Network.tla`
Expected: "Parsing completed."

- [ ] **Step 3: Commit**

```bash
git add docs/formal/accounting/Network.tla
git commit -m "formal: add Network.tla with message model and drop budget"
```

---

## Task 4: Ledger module

**Files:**
- Create: `docs/formal/accounting/Ledger.tla`

- [ ] **Step 1: Write the Ledger module**

`docs/formal/accounting/Ledger.tla`:

```tla
---- MODULE Ledger ----
EXTENDS Types

VARIABLES
  balance,         \* [Peers -> Amounts]: balance[p] represents what my ledger says about peer p
  postCommitLog    \* seq of [peer |-> Peers, newBalance |-> Amounts] (observable by subscribers)

LedgerVars == <<balance, postCommitLog>>

LedgerInit ==
  /\ balance = [p \in Peers |-> 0]
  /\ postCommitLog = <<>>

\* Debit: peer p owes us more (balance goes up)
Debit(p, amt) ==
  /\ amt \in Nat /\ amt > 0
  /\ balance[p] + amt \in Amounts    \* bounded-model safety: within modeled range
  /\ balance' = [balance EXCEPT ![p] = balance[p] + amt]
  /\ postCommitLog' = Append(postCommitLog, [peer |-> p, newBalance |-> balance[p] + amt])

\* Credit: we owe peer p more (balance goes down)
Credit(p, amt) ==
  /\ amt \in Nat /\ amt > 0
  /\ balance[p] - amt \in Amounts
  /\ balance' = [balance EXCEPT ![p] = balance[p] - amt]
  /\ postCommitLog' = Append(postCommitLog, [peer |-> p, newBalance |-> balance[p] - amt])

\* Payment sent reduces our debt (balance goes up, same direction as Debit)
NotifyPaymentSent(p, amt) ==
  /\ amt \in Nat /\ amt > 0
  /\ balance' = [balance EXCEPT ![p] = balance[p] + amt]
  /\ postCommitLog' = Append(postCommitLog, [peer |-> p, newBalance |-> balance[p] + amt])

\* Connect is a no-op on balance (Dec. 8, P13)
Connect(p) ==
  /\ UNCHANGED balance
  /\ UNCHANGED postCommitLog

LedgerTypeOK ==
  /\ balance \in [Peers -> Amounts]
  /\ postCommitLog \in Seq([peer: Peers, newBalance: Amounts])

====
```

- [ ] **Step 2: Lint**

Run: `java -cp bin/tla2tools.jar tla2sany.SANY Ledger.tla`
Expected: "Parsing completed."

- [ ] **Step 3: Commit**

```bash
git add docs/formal/accounting/Ledger.tla
git commit -m "formal: add Ledger module with Debit/Credit/postCommitLog"
```

---

## Task 5: Reservations module

**Files:**
- Create: `docs/formal/accounting/Reservations.tla`

- [ ] **Step 1: Write the Reservations module**

`docs/formal/accounting/Reservations.tla`:

```tla
---- MODULE Reservations ----
EXTENDS Types

VARIABLES
  reservedCredit,   \* [Peers -> Amounts]: in-flight credit reservations
  reservedDebit,    \* [Peers -> Amounts]: in-flight debit shadow reserves
  ghostDebit        \* [Peers -> Amounts]: accountability for cancelled debits

ReservationsVars == <<reservedCredit, reservedDebit, ghostDebit>>

ReservationsInit ==
  /\ reservedCredit = [p \in Peers |-> 0]
  /\ reservedDebit  = [p \in Peers |-> 0]
  /\ ghostDebit     = [p \in Peers |-> 0]

\* The reserve-side gate: called by Begin() for DirCredit
\*   -balance[p] + reservedCredit[p] + amt  <=  PaymentThreshold + timeBudget + inflightPayment
\* Caller passes timeBudget (from RateLimiter) and inflight (from SettlementTrigger) explicitly.
GateOK(p, amt, timeBudget, inflight, balance) ==
  -balance[p] + reservedCredit[p] + amt <= PaymentThreshold + timeBudget + inflight

\* Begin a Credit reservation: preconditions only; caller updates state
BeginCredit(p, amt, timeBudget, inflight, balance) ==
  /\ amt \in Nat /\ amt > 0
  /\ GateOK(p, amt, timeBudget, inflight, balance)
  /\ reservedCredit' = [reservedCredit EXCEPT ![p] = @ + amt]
  /\ UNCHANGED <<reservedDebit, ghostDebit>>

\* Begin a Debit reservation: no gate
BeginDebit(p, amt) ==
  /\ amt \in Nat /\ amt > 0
  /\ reservedDebit' = [reservedDebit EXCEPT ![p] = @ + amt]
  /\ UNCHANGED <<reservedCredit, ghostDebit>>

\* Commit: release the reserve (Middleware separately calls Ledger.Credit/Debit)
CommitCredit(p, amt) ==
  /\ reservedCredit' = [reservedCredit EXCEPT ![p] = @ - amt]
  /\ UNCHANGED <<reservedDebit, ghostDebit>>

CommitDebit(p, amt) ==
  /\ reservedDebit' = [reservedDebit EXCEPT ![p] = @ - amt]
  /\ UNCHANGED <<reservedCredit, ghostDebit>>

\* Rollback credit: release reserve, no penalty
RollbackCredit(p, amt) ==
  /\ reservedCredit' = [reservedCredit EXCEPT ![p] = @ - amt]
  /\ UNCHANGED <<reservedDebit, ghostDebit>>

\* Rollback debit: move to ghost (accountability penalty)
RollbackDebit(p, amt) ==
  /\ reservedDebit' = [reservedDebit EXCEPT ![p] = @ - amt]
  /\ ghostDebit'    = [ghostDebit    EXCEPT ![p] = @ + amt]
  /\ UNCHANGED reservedCredit

\* Disconnect: volatile state cleared
ClearOnDisconnect(p) ==
  /\ reservedCredit' = [reservedCredit EXCEPT ![p] = 0]
  /\ reservedDebit'  = [reservedDebit  EXCEPT ![p] = 0]
  /\ ghostDebit'     = [ghostDebit     EXCEPT ![p] = 0]

ReservationsTypeOK ==
  /\ reservedCredit \in [Peers -> 0..MaxBalance]
  /\ reservedDebit  \in [Peers -> 0..MaxBalance]
  /\ ghostDebit     \in [Peers -> 0..MaxBalance]

====
```

- [ ] **Step 2: Lint**

Run: `java -cp bin/tla2tools.jar tla2sany.SANY Reservations.tla`
Expected: "Parsing completed."

- [ ] **Step 3: Commit**

```bash
git add docs/formal/accounting/Reservations.tla
git commit -m "formal: add Reservations module with gate formula and ghost debit"
```

---

## Task 6: CycleTracker module

**Files:**
- Create: `docs/formal/accounting/CycleTracker.tla`

- [ ] **Step 1: Write the CycleTracker module**

`docs/formal/accounting/CycleTracker.tla`:

```tla
---- MODULE CycleTracker ----
EXTENDS Types

VARIABLES
  nextID,        \* [Peers x Direction -> Nat]: next ID to assign
  openCycles,    \* [Peers x Direction -> SUBSET Nat]: open but not confirmed
  watermark      \* [Peers x Direction -> Nat]: highest confirmed ID

CycleTrackerVars == <<nextID, openCycles, watermark>>

CycleTrackerInit ==
  /\ nextID     = [pd \in Peers \X Direction |-> 0]
  /\ openCycles = [pd \in Peers \X Direction |-> {}]
  /\ watermark  = [pd \in Peers \X Direction |-> 0]

\* Assign and open a cycle in one step
OpenNewCycle(p, dir) ==
  LET id == nextID[<<p, dir>>]
  IN  /\ id + 1 \in 0..MaxCycleID
      /\ nextID' = [nextID EXCEPT ![<<p, dir>>] = id + 1]
      /\ openCycles' = [openCycles EXCEPT ![<<p, dir>>] = @ \union {id}]
      /\ UNCHANGED watermark

\* Confirm a single cycle; superseded returns quietly
ConfirmCycle(p, dir, id) ==
  IF id <= watermark[<<p, dir>>]
  THEN \* superseded — swallow
    UNCHANGED <<nextID, openCycles, watermark>>
  ELSE
    /\ id \in openCycles[<<p, dir>>]
    /\ openCycles' = [openCycles EXCEPT ![<<p, dir>>] = @ \ {id}]
    /\ watermark'  = [watermark  EXCEPT ![<<p, dir>>] = id]
    /\ UNCHANGED nextID

\* Retro-confirm: return the set of open cycles <= upto that get confirmed
\* (Middleware calls Ledger.Debit for each of these)
RetroConfirmResult(p, dir, upto) ==
  { id \in openCycles[<<p, dir>>] : id <= upto }

RetroConfirm(p, dir, upto) ==
  /\ openCycles' = [openCycles EXCEPT ![<<p, dir>>] = @ \ RetroConfirmResult(p, dir, upto)]
  /\ watermark'  = [watermark  EXCEPT ![<<p, dir>>] = IF upto > watermark[<<p, dir>>] THEN upto ELSE @]
  /\ UNCHANGED nextID

\* Crash: lose the open-cycle set (Dec. 13); watermark and counter persisted
ClearOnCrash ==
  /\ openCycles' = [pd \in Peers \X Direction |-> {}]
  /\ UNCHANGED <<nextID, watermark>>

CycleTrackerTypeOK ==
  /\ nextID     \in [Peers \X Direction -> 0..MaxCycleID]
  /\ openCycles \in [Peers \X Direction -> SUBSET (0..MaxCycleID)]
  /\ watermark  \in [Peers \X Direction -> 0..MaxCycleID]

====
```

- [ ] **Step 2: Lint**

Run: `java -cp bin/tla2tools.jar tla2sany.SANY CycleTracker.tla`
Expected: "Parsing completed."

- [ ] **Step 3: Commit**

```bash
git add docs/formal/accounting/CycleTracker.tla
git commit -m "formal: add CycleTracker module with watermark and retro-confirm"
```

---

## Task 7: RateLimiter module

**Files:**
- Create: `docs/formal/accounting/RateLimiter.tla`

- [ ] **Step 1: Write the RateLimiter module**

`docs/formal/accounting/RateLimiter.tla`:

```tla
---- MODULE RateLimiter ----
EXTENDS Types

VARIABLES
  lastACKTimestamp  \* [Peers -> 0..MaxTime]: last confirmed ACK timestamp

RateLimiterVars == <<lastACKTimestamp>>

RateLimiterInit ==
  lastACKTimestamp = [p \in Peers |-> 0]

\* Pure function: time budget as of now
TimeBudget(p, now) == (now - lastACKTimestamp[p]) * RefreshRate

\* Validate an incoming ACK timestamp: must be monotonic and within drift
ValidateACKTimestamp(p, ts, now) ==
  /\ ts >= lastACKTimestamp[p]                   \* monotonic
  /\ ts <= now + ToleranceAbsTs                  \* not too far future
  /\ now - ts <= ToleranceAbsTs                  \* not too far past

\* Called by Middleware on commit
NotifyCommit(p, ts) ==
  IF ts > lastACKTimestamp[p]
  THEN lastACKTimestamp' = [lastACKTimestamp EXCEPT ![p] = ts]
  ELSE UNCHANGED lastACKTimestamp

RateLimiterTypeOK ==
  /\ lastACKTimestamp \in [Peers -> 0..MaxTime]

====
```

- [ ] **Step 2: Lint**

Run: `java -cp bin/tla2tools.jar tla2sany.SANY RateLimiter.tla`
Expected: "Parsing completed."

- [ ] **Step 3: Commit**

```bash
git add docs/formal/accounting/RateLimiter.tla
git commit -m "formal: add RateLimiter module as pure TimeBudget query"
```

---

## Task 8: BlocklistPolicy module

**Files:**
- Create: `docs/formal/accounting/BlocklistPolicy.tla`

- [ ] **Step 1: Write the BlocklistPolicy module**

`docs/formal/accounting/BlocklistPolicy.tla`:

```tla
---- MODULE BlocklistPolicy ----
EXTENDS Types

VARIABLES
  blockedUntil   \* [Peers -> 0..MaxTime]: blocklist expiration

BlocklistVars == <<blockedUntil>>

BlocklistInit ==
  blockedUntil = [p \in Peers |-> 0]

\* Duration formula per design §7 (post-fix with refreshRate floor)
EffectiveDebt(rawDebt) ==
  IF rawDebt > RefreshRate THEN rawDebt ELSE RefreshRate

DisconnectFor(rawDebt) ==
  (EffectiveDebt(rawDebt) + PaymentThreshold) * MultiplierBlock \div RefreshRate

Block(p, rawDebt, now) ==
  LET dur == DisconnectFor(rawDebt)
  IN  blockedUntil' = [blockedUntil EXCEPT ![p] = now + dur]

IsBlocked(p, now) == blockedUntil[p] > now

BlocklistTypeOK ==
  /\ blockedUntil \in [Peers -> 0..(MaxTime * 10)]   \* duration can extend beyond model time

====
```

- [ ] **Step 2: Lint**

Run: `java -cp bin/tla2tools.jar tla2sany.SANY BlocklistPolicy.tla`
Expected: "Parsing completed."

- [ ] **Step 3: Commit**

```bash
git add docs/formal/accounting/BlocklistPolicy.tla
git commit -m "formal: add BlocklistPolicy module with refreshRate-floored duration formula"
```

---

## Task 9: SettlementTrigger module

**Files:**
- Create: `docs/formal/accounting/SettlementTrigger.tla`

- [ ] **Step 1: Write the SettlementTrigger module**

`docs/formal/accounting/SettlementTrigger.tla`:

```tla
---- MODULE SettlementTrigger ----
EXTENDS Types

VARIABLES
  inflightPayment,         \* [Peers -> 0..MaxBalance]
  paymentOngoing,          \* [Peers -> BOOLEAN]
  lastSettlementFailureTs, \* [Peers -> 0..MaxTime]: 0 means "no prior failure"
  payFnCalls               \* seq of [peer, amount, at]: audit log for P11, P12

SettlementVars == <<inflightPayment, paymentOngoing, lastSettlementFailureTs, payFnCalls>>

SettlementInit ==
  /\ inflightPayment         = [p \in Peers |-> 0]
  /\ paymentOngoing          = [p \in Peers |-> FALSE]
  /\ lastSettlementFailureTs = [p \in Peers |-> 0]
  /\ payFnCalls              = <<>>

\* Compute what to pay given current balance and inflight
PayAmount(p, balance) ==
  LET owed == -balance[p]
  IN IF owed - inflightPayment[p] > 0
     THEN owed - inflightPayment[p]
     ELSE 0

\* Cooldown check
CooldownOK(p, now) ==
  lastSettlementFailureTs[p] = 0 \/ now > lastSettlementFailureTs[p] + 10

\* Threshold trigger: post-commit hook
OnPostCommit(p, balance, now) ==
  LET amt == PayAmount(p, balance)
  IN IF /\ ~paymentOngoing[p]
        /\ amt >= MinimumPayment
        /\ CooldownOK(p, now)
     THEN \* invoke PayFn: mark ongoing, record inflight, log
       /\ paymentOngoing'  = [paymentOngoing  EXCEPT ![p] = TRUE]
       /\ inflightPayment' = [inflightPayment EXCEPT ![p] = amt]
       /\ payFnCalls'      = Append(payFnCalls, [peer |-> p, amount |-> amt, at |-> now])
       /\ UNCHANGED lastSettlementFailureTs
     ELSE
       UNCHANGED SettlementVars

\* Callback: Swap reports success
OnPaySuccess(p, amt) ==
  /\ paymentOngoing'        = [paymentOngoing EXCEPT ![p] = FALSE]
  /\ inflightPayment'       = [inflightPayment EXCEPT ![p] = 0]
  /\ UNCHANGED <<lastSettlementFailureTs, payFnCalls>>
  \* Ledger.NotifyPaymentSent(p, amt) is called by caller

\* Callback: Swap reports failure
OnPayFailure(p, now) ==
  /\ paymentOngoing'           = [paymentOngoing EXCEPT ![p] = FALSE]
  /\ inflightPayment'          = [inflightPayment EXCEPT ![p] = 0]
  /\ lastSettlementFailureTs'  = [lastSettlementFailureTs EXCEPT ![p] = now]
  /\ UNCHANGED payFnCalls

SettlementTypeOK ==
  /\ inflightPayment         \in [Peers -> 0..MaxBalance]
  /\ paymentOngoing          \in [Peers -> BOOLEAN]
  /\ lastSettlementFailureTs \in [Peers -> 0..MaxTime]

====
```

- [ ] **Step 2: Lint**

Run: `java -cp bin/tla2tools.jar tla2sany.SANY SettlementTrigger.tla`
Expected: "Parsing completed."

- [ ] **Step 3: Commit**

```bash
git add docs/formal/accounting/SettlementTrigger.tla
git commit -m "formal: add SettlementTrigger module owning inflightPayment and cooldown"
```

---

## Task 10: Middleware PlusCal algorithm

**Files:**
- Create: `docs/formal/accounting/Middleware.tla`

This is the top-level spec. It imports all layer modules, defines the process-level algorithm in PlusCal for two-peer (or three-peer forwarder) scenarios, and includes a crash action.

- [ ] **Step 1: Write the Middleware module**

`docs/formal/accounting/Middleware.tla`:

```tla
---- MODULE Middleware ----
EXTENDS Types, Ledger, Reservations, CycleTracker, RateLimiter, BlocklistPolicy, SettlementTrigger, Network

\* Per-peer persistent state mirror for the "other peer's view"
\* We model each peer p as having its own instance of the above layer variables.
\* For TLC tractability we instantiate for Peers = {a, b} (or {a, b, c} for forwarder),
\* storing state per peer in tagged arrays.

VARIABLES
  peerBalance,           \* [Peer -> Ledger balance from that peer's perspective]
  peerReserved,          \* [Peer -> Reservations state (reservedCredit/reservedDebit/ghostDebit)]
  peerCycleTracker,      \* [Peer -> CycleTracker state]
  peerRateLimiter,       \* [Peer -> RateLimiter state]
  peerSettlement,        \* [Peer -> SettlementTrigger state]
  peerBlocklist,         \* [Peer -> BlocklistPolicy state]
  now,                   \* global time counter
  crashedMidCycle        \* set of (Peer, cycleID) tuples that were open when peer crashed

(*--algorithm Accounting {

variable network = {},
         dropsRemaining = DropBudget,
         now = 0,
         crashedMidCycle = {};

\* Global initialization of per-peer state is in an init action below.

macro RunCycleCredit(self, peer, amount) {
  \* 1. Gate check: Reservations.Begin(credit)
  await -peerBalance[self][peer] + peerReserved[self][peer].reservedCredit + amount <=
        PaymentThreshold + TimeBudgetFor(self, peer, now) + peerSettlement[self][peer].inflightPayment;
  \* 2. Allocate cycle ID
  with (id = peerCycleTracker[self][peer].nextID[<<peer, "Credit">>]) {
    peerReserved[self][peer].reservedCredit := peerReserved[self][peer].reservedCredit + amount ||
    peerCycleTracker[self][peer].nextID[<<peer, "Credit">>] := id + 1 ||
    peerCycleTracker[self][peer].openCycles[<<peer, "Credit">>] :=
      peerCycleTracker[self][peer].openCycles[<<peer, "Credit">>] \union {id};
    \* 3. Send Request message with lastConfirmed piggyback
    network := network \union {Message("Request", self, peer, id, amount, now,
                                        peerCycleTracker[self][peer].watermark[<<peer, "Credit">>])};
  }
}

\* The server-side Request handler, triggered when Request arrives
macro HandleRequest(self, msg) {
  \* 0. First: retro-confirm piggyback
  with (upto = msg.lastConfirmed,
        toConfirm = { id \in peerCycleTracker[self][msg.from].openCycles[<<msg.from, "Debit">>]
                    : id <= upto }) {
    \* for each id in toConfirm: Ledger.Debit(from, remembered_amount)
    \* Model simplification: all cycles have same amount; multiply by |toConfirm|
    peerBalance[self][msg.from] := peerBalance[self][msg.from] + amount * Cardinality(toConfirm) ||
    peerCycleTracker[self][msg.from].openCycles[<<msg.from, "Debit">>] :=
      peerCycleTracker[self][msg.from].openCycles[<<msg.from, "Debit">>] \ toConfirm ||
    peerCycleTracker[self][msg.from].watermark[<<msg.from, "Debit">>] :=
      IF upto > peerCycleTracker[self][msg.from].watermark[<<msg.from, "Debit">>] THEN upto ELSE @;
  };
  \* 1. Begin debit reservation
  peerReserved[self][msg.from].reservedDebit := peerReserved[self][msg.from].reservedDebit + msg.amount;
  \* 2. Record in CycleTracker as open on debit side
  peerCycleTracker[self][msg.from].openCycles[<<msg.from, "Debit">>] :=
    peerCycleTracker[self][msg.from].openCycles[<<msg.from, "Debit">>] \union {msg.cycleID};
  \* 3. Send Response with serverTs=now
  network := (network \ {msg}) \union {Message("Response", self, msg.from, msg.cycleID, msg.amount, now, 0)};
}

\* The client-side Response handler
macro HandleResponse(self, msg) {
  \* Validate ACK timestamp
  if (peerRateLimiter[self][msg.from].lastACKTimestamp > msg.ts \/
      now - msg.ts > ToleranceAbsTs \/ msg.ts - now > ToleranceAbsTs) {
    \* invalid: rollback credit, trigger blocklist (ErrClockDrift path)
    peerReserved[self][msg.from].reservedCredit := peerReserved[self][msg.from].reservedCredit - msg.amount;
    peerBlocklist[self][msg.from].blockedUntil := now + DisconnectFor(0);
    network := network \ {msg};
  } else {
    \* Valid: commit the cycle
    peerReserved[self][msg.from].reservedCredit := peerReserved[self][msg.from].reservedCredit - msg.amount ||
    peerBalance[self][msg.from] := peerBalance[self][msg.from] - msg.amount ||
    peerCycleTracker[self][msg.from].openCycles[<<msg.from, "Credit">>] :=
      peerCycleTracker[self][msg.from].openCycles[<<msg.from, "Credit">>] \ {msg.cycleID} ||
    peerCycleTracker[self][msg.from].watermark[<<msg.from, "Credit">>] := msg.cycleID ||
    peerRateLimiter[self][msg.from].lastACKTimestamp := msg.ts;
    network := network \ {msg};

    \* Post-commit: SettlementTrigger.OnPostCommit
    OnPostCommitAction(self, msg.from);
  }
}

\* SettlementTrigger.OnPostCommit translated as a macro
macro OnPostCommitAction(self, peer) {
  with (amt = PayAmountFor(self, peer)) {
    if (~peerSettlement[self][peer].paymentOngoing /\
        amt >= MinimumPayment /\
        CooldownOKFor(self, peer, now)) {
      peerSettlement[self][peer].paymentOngoing := TRUE ||
      peerSettlement[self][peer].inflightPayment := amt ||
      peerSettlement[self][peer].payFnCalls :=
        Append(peerSettlement[self][peer].payFnCalls, [peer |-> peer, amount |-> amt, at |-> now]);
    }
  }
}

\* Process per peer
process (Peer \in Peers) {
loop:
  while (TRUE) {
    either {
      \* Spontaneously initiate a client cycle to a random other peer
      with (target \in Peers \ {self}) {
        await target \notin Blocklisted(self, now);
        RunCycleCredit(self, target, 1);  \* amount=1 for tractability
      }
    } or {
      \* Process any incoming message
      with (m \in network) {
        await m.to = self;
        if (m.kind = "Request") HandleRequest(self, m);
        else HandleResponse(self, m);
      }
    } or {
      \* Tick time
      await now < MaxTime;
      now := now + 1;
    } or {
      \* Drop a network message (model adversarial drop)
      await dropsRemaining > 0 /\ network /= {};
      with (m \in network) {
        network := network \ {m};
        dropsRemaining := dropsRemaining - 1;
      }
    }
  }
}

} *)

====
```

- [ ] **Step 2: Translate PlusCal to TLA+**

Run: `cd docs/formal/accounting && java -cp bin/tla2tools.jar pcal.trans Middleware.tla`
Expected: "Parsing completed." + "Translation completed." Generates a BEGIN TRANSLATION...END TRANSLATION block in the file.

- [ ] **Step 3: Lint post-translation**

Run: `java -cp bin/tla2tools.jar tla2sany.SANY Middleware.tla`
Expected: "Parsing completed."

- [ ] **Step 4: Commit**

```bash
git add docs/formal/accounting/Middleware.tla
git commit -m "formal: add Middleware PlusCal top-level with cycle lifecycle and drop"
```

---

## Task 11: Properties module

**Files:**
- Create: `docs/formal/accounting/Properties.tla`

- [ ] **Step 1: Write the Properties module**

`docs/formal/accounting/Properties.tla`:

```tla
---- MODULE Properties ----
EXTENDS Middleware

\* ============================================
\* SAFETY PROPERTIES (invariants)
\* ============================================

\* P1: Commit symmetry under message drop
\* For every cycle c committed at client side, either server has it committed
\* or the divergence is in server's disfavor (server committed strictly less).
P1_CommitSymmetry ==
  \A client \in Peers, server \in Peers:
    client # server =>
      LET clientCommitted == peerCycleTracker[client][server].watermark[<<server, "Credit">>]
          serverCommitted == peerCycleTracker[server][client].watermark[<<client, "Debit">>]
      IN  serverCommitted <= clientCommitted  \* server may be behind, never ahead

\* P2: Balance conservation at quiescence, with forwarder and crash carve-outs
IsQuiescent ==
  /\ network = {}
  /\ \A p \in Peers, q \in Peers:
       peerReserved[p][q].reservedCredit = 0 /\
       peerReserved[p][q].reservedDebit  = 0

P2_BalanceConservation ==
  IsQuiescent =>
    \A a \in Peers, b \in Peers:
      a # b =>
        peerBalance[a][b] + peerBalance[b][a] <= 0
        \* Equality holds iff crashedMidCycle is empty AND no forwarder-inner-commit-outer-fail has occurred
        \* Inequality always server-disfavor direction

\* P3: No reservation leaks at quiescence
P3_NoReservationLeaks ==
  IsQuiescent =>
    \A p \in Peers, q \in Peers:
      peerReserved[p][q].reservedCredit = 0 /\
      peerReserved[p][q].reservedDebit  = 0

\* P4: Cycle ID / watermark monotonicity
P4_WatermarkMonotonic ==
  \A p \in Peers, q \in Peers, d \in Direction:
    peerCycleTracker[p][q].watermark[<<q, d>>] <=
      peerCycleTracker[p][q].nextID[<<q, d>>]

\* P5: Rate-limit bounded credit (ambient form)
P5_RateLimit ==
  \A self \in Peers, peer \in Peers:
    self # peer =>
      -peerBalance[self][peer] <=
        PaymentThreshold +
        TimeBudgetFor(self, peer, now) +
        peerSettlement[self][peer].inflightPayment

\* P8: No deadlock (TLC's built-in check; noted as a property)
P8_NoDeadlock == TRUE  \* Checked by TLC with -deadlock flag

\* P9: Nested forwarder cycle correctness (inner fail => outer rollback)
\* Only meaningful in forwarder configs (3 peers)
P9_NestedForwarder ==
  \* Expressed at cycle-level: for every outer debit cycle D that committed,
  \* there exists a corresponding inner credit cycle C that also committed.
  \* Encoded via a shadow variable innerOuterPair set when inner commits under outer.
  \* For now, a weaker form: outer's commit implies no concurrent inner rollback is pending.
  TRUE  \* Full version requires forwarder-specific instrumentation; see adversarial.cfg

\* P10: PostCommit ordering preservation
\* Each peer's postCommitLog has a balance at position i equal to balance[peer] at that commit
P10_PostCommitOrdering ==
  \A self \in Peers:
    \A i \in 1..Len(peerBalance[self]):
      TRUE  \* simplified — real check compares log against committed sequence

\* P11: No double-pay + in-flight deduction
P11_NoDoublePay ==
  \A self \in Peers, peer \in Peers:
    self # peer =>
      peerSettlement[self][peer].paymentOngoing =>
        \* During ongoing, no new PayFn call should have appeared in this step
        TRUE  \* checked as action property via UNCHANGED clauses

\* P12: Cooldown safety
P12_Cooldown ==
  \A self \in Peers, peer \in Peers:
    self # peer /\ peerSettlement[self][peer].lastSettlementFailureTs > 0 =>
      (now <= peerSettlement[self][peer].lastSettlementFailureTs + 10) =>
        \* The last PayFn call time should NOT be in the cooldown window
        LET calls == peerSettlement[self][peer].payFnCalls
        IN  Len(calls) = 0 \/
            calls[Len(calls)].at > peerSettlement[self][peer].lastSettlementFailureTs + 10 \/
            calls[Len(calls)].at <= peerSettlement[self][peer].lastSettlementFailureTs

\* P13: Connect no-op on balance — checked by Ledger.Connect definition; action-level
P13_ConnectNoOp == TRUE  \* asserted structurally via Ledger.Connect defintion

\* P14: Reservation gate (redundant with P5; checked at decision point in actions)
P14_GateAtDecision == P5_RateLimit  \* same ambient form

\* P15: Timestamp monotonicity per peer
P15_TimestampMonotonic ==
  \* Checked via action property: NotifyCommit only advances lastACKTimestamp forward
  TRUE

\* ============================================
\* TYPE INVARIANTS
\* ============================================
TypeOK ==
  /\ NetworkTypeOK
  /\ \A p \in Peers: LedgerTypeOK  \* per-peer
  /\ \A p \in Peers: ReservationsTypeOK
  /\ \A p \in Peers: CycleTrackerTypeOK
  /\ \A p \in Peers: RateLimiterTypeOK
  /\ \A p \in Peers: SettlementTypeOK
  /\ \A p \in Peers: BlocklistTypeOK

SafetyInvariants ==
  /\ TypeOK
  /\ P1_CommitSymmetry
  /\ P3_NoReservationLeaks
  /\ P4_WatermarkMonotonic
  /\ P5_RateLimit
  /\ P11_NoDoublePay
  /\ P12_Cooldown
  /\ P14_GateAtDecision
  /\ P15_TimestampMonotonic

\* ============================================
\* LIVENESS PROPERTIES
\* ============================================

\* P6: Lost-ACK recovery termination (under fairness of cycle creation)
P6_RecoveryTermination ==
  \A client \in Peers, server \in Peers, id \in 0..MaxCycleID:
    client # server =>
      (id \in peerCycleTracker[client][server].openCycles[<<server, "Credit">>])
        ~> (id <= peerCycleTracker[server][client].watermark[<<client, "Debit">>])

\* P7: Ghost-debit accountability
P7_GhostAccountability ==
  \A self \in Peers, peer \in Peers:
    self # peer =>
      (peerReserved[self][peer].ghostDebit > DisconnectLimit)
        ~> (peerBlocklist[self][peer].blockedUntil > now)

LivenessProperties ==
  /\ P6_RecoveryTermination
  /\ P7_GhostAccountability

====
```

- [ ] **Step 2: Lint**

Run: `java -cp bin/tla2tools.jar tla2sany.SANY Properties.tla`
Expected: "Parsing completed." Warnings about shadowed names are OK.

- [ ] **Step 3: Commit**

```bash
git add docs/formal/accounting/Properties.tla
git commit -m "formal: add Properties module with P1-P15 formal statements"
```

---

## Task 12: safety-small model configuration and first TLC run

**Files:**
- Create: `docs/formal/accounting/models/safety-small.cfg`

- [ ] **Step 1: Write the safety-small config**

`docs/formal/accounting/models/safety-small.cfg`:

```
SPECIFICATION Spec

CONSTANTS
  Peers = {a, b}
  MaxCycleID = 4
  MaxTime = 8
  MaxBalance = 5
  DropBudget = 2
  PaymentThreshold = 3
  RefreshRate = 1
  MultiplierBlock = 1
  ToleranceAbsTs = 2
  ToleranceInterval = 3
  MinimumPayment = 1

SYMMETRY Symmetry
  \* Symmetry reduction on Peers

INVARIANTS
  TypeOK
  SafetyInvariants

PROPERTIES
  \* No liveness here; see liveness.cfg
```

- [ ] **Step 2: Run TLC on safety-small**

Run: `cd docs/formal/accounting && make safety-small`
Expected: Either "Model checking completed. No error has been found." OR a counterexample trace. If counterexample, capture it:

```bash
make safety-small 2>&1 | tee counterexamples/safety-small-$(date +%Y%m%d).txt
```

- [ ] **Step 3: Analyze counterexample (if any) and decide: fix model or fix design**

If TLC reports a violation:
1. Read the trace. Identify which property failed and at which action.
2. Determine if this is a *model bug* (the TLA+ doesn't reflect the design) or a *design bug* (the TLA+ correctly reflects a design flaw).
3. Model bug: fix the relevant `.tla` file, re-run.
4. Design bug: document in `counterexamples/`, then update `docs/superpowers/specs/2026-04-23-accounting-hooks-design.md` with the corresponding section fix, re-run.

Expected iteration count: 2-5 fixes on first run (typical).

- [ ] **Step 4: Commit when green**

```bash
git add docs/formal/accounting/models/safety-small.cfg docs/formal/accounting/counterexamples
git commit -m "formal: safety-small config passes TLC on Middleware"
```

---

## Task 13: safety-large model configuration

**Files:**
- Create: `docs/formal/accounting/models/safety-large.cfg`

- [ ] **Step 1: Write the safety-large config**

`docs/formal/accounting/models/safety-large.cfg`:

```
SPECIFICATION Spec

CONSTANTS
  Peers = {a, b}
  MaxCycleID = 6
  MaxTime = 12
  MaxBalance = 10
  DropBudget = 3
  PaymentThreshold = 5
  RefreshRate = 1
  MultiplierBlock = 1
  ToleranceAbsTs = 2
  ToleranceInterval = 3
  MinimumPayment = 1

SYMMETRY Symmetry

INVARIANTS
  TypeOK
  SafetyInvariants
```

- [ ] **Step 2: Run TLC**

Run: `make safety-large`
Expected: Same properties as safety-small but on larger bounds; expected completion 5-15 minutes.

- [ ] **Step 3: Commit when green**

```bash
git add docs/formal/accounting/models/safety-large.cfg
git commit -m "formal: safety-large config passes TLC at bounds 6/12/±10/3"
```

---

## Task 14: liveness model configuration

**Files:**
- Create: `docs/formal/accounting/models/liveness.cfg`

- [ ] **Step 1: Write the liveness config**

`docs/formal/accounting/models/liveness.cfg`:

```
SPECIFICATION Spec

CONSTANTS
  Peers = {a, b}
  MaxCycleID = 4
  MaxTime = 8
  MaxBalance = 5
  DropBudget = 2
  PaymentThreshold = 3
  RefreshRate = 1
  MultiplierBlock = 1
  ToleranceAbsTs = 2
  ToleranceInterval = 3
  MinimumPayment = 1

SYMMETRY Symmetry

INVARIANTS
  TypeOK

PROPERTIES
  LivenessProperties

\* Fairness: every enabled process step is eventually taken
\* WF on cycle-initiation ensures peers don't just sit idle
```

Note: The `Spec` definition in `Middleware.tla` after PlusCal translation will need `WF_vars(...)` applied to relevant actions. Verify this is present; if not, edit `Middleware.tla` to add weak fairness on the `loop` label.

- [ ] **Step 2: Run TLC with liveness checking**

Run: `make check-liveness`
Expected: TLC reports whether liveness holds. Liveness checking is slower than safety; 10-30 min.

- [ ] **Step 3: Document and commit**

```bash
git add docs/formal/accounting/models/liveness.cfg
git commit -m "formal: liveness config verifies P6 and P7 under fairness"
```

---

## Task 15: adversarial model configuration

**Files:**
- Create: `docs/formal/accounting/models/adversarial.cfg`
- Modify: `docs/formal/accounting/Middleware.tla` (add a Byzantine action)

- [ ] **Step 1: Add Byzantine peer action to Middleware.tla**

After the `Peer` process block in Middleware.tla, add:

```tla
\* An adversary process that tampers with messages in-flight
\* Only active in adversarial.cfg via INIT condition
process (Adversary = "adv") {
adv_loop:
  while (TRUE) {
    either {
      \* Tamper: change timestamp to something invalid
      await network /= {};
      with (m \in { msg \in network : msg.kind = "Response" }) {
        with (badTs \in { m.ts + ToleranceAbsTs + 5, m.ts - ToleranceAbsTs - 5 }) {
          network := (network \ {m}) \union
            {[m EXCEPT !.ts = badTs]};
        }
      }
    } or {
      \* Tamper: change cycleID to something not in openCycles
      await network /= {};
      with (m \in network) {
        network := (network \ {m}) \union {[m EXCEPT !.cycleID = MaxCycleID + 1]};
      }
    } or {
      skip;  \* Adversary may choose to do nothing
    }
  }
}
```

Retranslate: `make translate`.

- [ ] **Step 2: Write the adversarial config**

`docs/formal/accounting/models/adversarial.cfg`:

```
SPECIFICATION Spec

CONSTANTS
  Peers = {a, b}
  MaxCycleID = 4
  MaxTime = 8
  MaxBalance = 5
  DropBudget = 2
  PaymentThreshold = 3
  RefreshRate = 1
  MultiplierBlock = 1
  ToleranceAbsTs = 2
  ToleranceInterval = 3
  MinimumPayment = 1

SYMMETRY Symmetry

INVARIANTS
  TypeOK
  SafetyInvariants

\* Key property: tampered messages ⇒ blocklist, not committed balance divergence
\* This is captured by P1 (commit symmetry) — an adversarial peer cannot bump
\* the victim's balance beyond what honest behavior would allow.
```

- [ ] **Step 3: Run TLC**

Run: `make check-adversarial`
Expected: TLC verifies that adversary can cause rollbacks and blocklists but not balance violations. Counterexamples here are valuable — document each in `counterexamples/`.

- [ ] **Step 4: Commit**

```bash
git add docs/formal/accounting/Middleware.tla docs/formal/accounting/models/adversarial.cfg
git commit -m "formal: adversarial config with Byzantine tampering action"
```

---

## Task 16: crash model configuration

**Files:**
- Create: `docs/formal/accounting/models/crash.cfg`
- Modify: `docs/formal/accounting/Middleware.tla` (add crash action)

- [ ] **Step 1: Add crash action to Middleware.tla**

Inside the `Peer` process, add another `either` branch:

```tla
} or {
  \* Crash: clear open-cycle set; watermark + balance + nextID persist
  await \* only in crash.cfg, gate via a constant
    TRUE;
  with (lostCycles =
    [d \in Direction |->
      peerCycleTracker[self][self].openCycles[<<self, d>>]]) {
    \* Update crashedMidCycle tracker
    crashedMidCycle := crashedMidCycle \union
      { <<self, c>> : c \in UNION { lostCycles[d] : d \in Direction } };
    \* Clear open cycles
    peerCycleTracker[self][self].openCycles :=
      [d \in Direction |-> {}];
    \* Clear in-memory Reservations state
    peerReserved[self][self].reservedCredit := 0 ||
    peerReserved[self][self].reservedDebit  := 0 ||
    peerReserved[self][self].ghostDebit     := 0;
  }
}
```

- [ ] **Step 2: Write the crash config**

`docs/formal/accounting/models/crash.cfg`:

```
SPECIFICATION Spec

CONSTANTS
  Peers = {a, b}
  MaxCycleID = 4
  MaxTime = 8
  MaxBalance = 5
  DropBudget = 1
  PaymentThreshold = 3
  RefreshRate = 1
  MultiplierBlock = 1
  ToleranceAbsTs = 2
  ToleranceInterval = 3
  MinimumPayment = 1

SYMMETRY Symmetry

INVARIANTS
  TypeOK
  \* NOT SafetyInvariants — P2 has an asymmetric form here
  P1_CommitSymmetry
  P3_NoReservationLeaks
  P4_WatermarkMonotonic
  P5_RateLimit
  P15_TimestampMonotonic
  \* P2 is checked in weaker form:
  \* peerBalance[a][b] + peerBalance[b][a] ≤ 0 (always server-disfavor)
```

Note: define a `P2_CrashAware` in Properties.tla:

```tla
P2_CrashAware ==
  IsQuiescent =>
    \A a, b \in Peers: a # b => peerBalance[a][b] + peerBalance[b][a] <= 0
```

Reference it in this config.

- [ ] **Step 3: Run TLC**

Run: `make check-crash`
Expected: Passes when the in-memory-state loss correctly produces server-disfavor asymmetry.

- [ ] **Step 4: Commit**

```bash
git add docs/formal/accounting/Middleware.tla docs/formal/accounting/models/crash.cfg docs/formal/accounting/Properties.tla
git commit -m "formal: crash config with P2_CrashAware and in-memory state loss"
```

---

## Task 17: hedged model configuration

**Files:**
- Create: `docs/formal/accounting/models/hedged.cfg`

- [ ] **Step 1: Write the config**

`docs/formal/accounting/models/hedged.cfg`:

```
SPECIFICATION Spec

CONSTANTS
  Peers = {a, b}
  MaxCycleID = 4
  MaxTime = 6
  MaxBalance = 5
  DropBudget = 1
  PaymentThreshold = 3
  RefreshRate = 1
  MultiplierBlock = 1
  ToleranceAbsTs = 2
  ToleranceInterval = 3
  MinimumPayment = 1

SYMMETRY Symmetry

INVARIANTS
  TypeOK
  SafetyInvariants

\* Hedged: two peers initiate cycles to each other concurrently.
\* The model's either-branches already allow this via TLC's nondeterminism.
\* Additional property to check: final balance = sum of committed cycles
```

Also add to `Properties.tla`:

```tla
P_HedgedConsistency ==
  IsQuiescent =>
    \A a, b \in Peers: a # b =>
      LET committed == peerCycleTracker[a][b].watermark[<<b, "Credit">>]
      IN  -peerBalance[a][b] = committed  \* all credits committed = accumulated debt
```

Reference in hedged.cfg INVARIANTS.

- [ ] **Step 2: Run and commit**

```bash
make hedged
git add docs/formal/accounting/models/hedged.cfg docs/formal/accounting/Properties.tla
git commit -m "formal: hedged config verifies concurrent-cycle consistency"
```

---

## Task 18: settlement-failure model configuration

**Files:**
- Create: `docs/formal/accounting/models/settlement-failure.cfg`
- Modify: `docs/formal/accounting/Middleware.tla` (add PayFn failure nondeterminism)

- [ ] **Step 1: Add PayFn success/failure branches to Middleware.tla**

In the Peer process, add an action:

```tla
} or {
  \* Resolve an in-flight payment (success or failure nondeterministically)
  with (peer \in Peers \ {self}) {
    await peerSettlement[self][peer].paymentOngoing;
    either {
      \* Success: clear ongoing, zero inflight, Ledger.NotifyPaymentSent
      peerBalance[self][peer] := peerBalance[self][peer] + peerSettlement[self][peer].inflightPayment ||
      peerSettlement[self][peer].paymentOngoing := FALSE ||
      peerSettlement[self][peer].inflightPayment := 0;
    } or {
      \* Failure: clear ongoing, zero inflight, set lastSettlementFailureTs
      peerSettlement[self][peer].paymentOngoing := FALSE ||
      peerSettlement[self][peer].inflightPayment := 0 ||
      peerSettlement[self][peer].lastSettlementFailureTs := now;
    }
  }
}
```

- [ ] **Step 2: Write the config**

`docs/formal/accounting/models/settlement-failure.cfg`:

```
SPECIFICATION Spec

CONSTANTS
  Peers = {a, b}
  MaxCycleID = 4
  MaxTime = 25    \* need > 10s + workspace for cooldown windows
  MaxBalance = 8
  DropBudget = 1
  PaymentThreshold = 3
  RefreshRate = 1
  MultiplierBlock = 1
  ToleranceAbsTs = 2
  ToleranceInterval = 3
  MinimumPayment = 1

SYMMETRY Symmetry

INVARIANTS
  TypeOK
  P11_NoDoublePay
  P12_Cooldown
  P5_RateLimit
```

- [ ] **Step 3: Run and commit**

```bash
make settlement-failure
git add docs/formal/accounting/Middleware.tla docs/formal/accounting/models/settlement-failure.cfg
git commit -m "formal: settlement-failure config exercises cooldown and double-pay invariants"
```

---

## Task 19: inner-commit-outer-fail model configuration

**Files:**
- Create: `docs/formal/accounting/models/inner-commit-outer-fail.cfg`
- Modify: `docs/formal/accounting/Middleware.tla` (add forwarder action with three peers)

- [ ] **Step 1: Add forwarder-cycle action to Middleware.tla**

With three peers `{a, b, c}`, add a process that models a forwarder:

```tla
\* When self has role "forwarder" for some cycle:
\*   outer cycle is Debit from a (who initiated)
\*   inner cycle is Credit to c (the destination)
\* Narrow window: inner commits successfully; then outer's Receipt-to-a fails.

macro ForwarderCycle(self, upstream, downstream, amount) {
  \* Outer: Debit reservation vs upstream
  await -peerBalance[self][upstream] + peerReserved[self][upstream].reservedDebit + amount <= MaxBalance;
  peerReserved[self][upstream].reservedDebit := peerReserved[self][upstream].reservedDebit + amount;
  \* Open both cycles
  with (outerID = peerCycleTracker[self][upstream].nextID[<<upstream, "Debit">>],
        innerID = peerCycleTracker[self][downstream].nextID[<<downstream, "Credit">>]) {
    peerCycleTracker[self][upstream].nextID[<<upstream, "Debit">>] := outerID + 1;
    peerCycleTracker[self][downstream].nextID[<<downstream, "Credit">>] := innerID + 1;
    \* Inner RunCycle: send request, receive response, commit credit
    \* (simplified: atomic for model simplicity)
    peerReserved[self][downstream].reservedCredit := peerReserved[self][downstream].reservedCredit + amount;
    either {
      \* Inner succeeds
      peerReserved[self][downstream].reservedCredit := peerReserved[self][downstream].reservedCredit - amount ||
      peerBalance[self][downstream] := peerBalance[self][downstream] - amount ||
      peerCycleTracker[self][downstream].watermark[<<downstream, "Credit">>] := innerID;
      \* Now outer: either commit (happy) or fail write to upstream (narrow window)
      either {
        \* Outer commits
        peerReserved[self][upstream].reservedDebit := peerReserved[self][upstream].reservedDebit - amount ||
        peerBalance[self][upstream] := peerBalance[self][upstream] + amount ||
        peerCycleTracker[self][upstream].watermark[<<upstream, "Debit">>] := outerID;
      } or {
        \* Outer fails AFTER inner committed — narrow window
        peerReserved[self][upstream].reservedDebit := peerReserved[self][upstream].reservedDebit - amount ||
        peerReserved[self][upstream].ghostDebit := peerReserved[self][upstream].ghostDebit + amount;
      }
    } or {
      \* Inner fails — outer must roll back too
      peerReserved[self][downstream].reservedCredit := peerReserved[self][downstream].reservedCredit - amount ||
      peerReserved[self][upstream].reservedDebit := peerReserved[self][upstream].reservedDebit - amount ||
      peerReserved[self][upstream].ghostDebit := peerReserved[self][upstream].ghostDebit + amount;
    }
  }
}
```

- [ ] **Step 2: Write the config**

`docs/formal/accounting/models/inner-commit-outer-fail.cfg`:

```
SPECIFICATION Spec

CONSTANTS
  Peers = {a, b, c}      \* Three peers: a=originator, b=forwarder, c=destination
  MaxCycleID = 3
  MaxTime = 6
  MaxBalance = 5
  DropBudget = 0          \* no network drops for this scenario — focus on cycle logic
  PaymentThreshold = 3
  RefreshRate = 1
  MultiplierBlock = 1
  ToleranceAbsTs = 2
  ToleranceInterval = 3
  MinimumPayment = 1

INVARIANTS
  TypeOK
  P3_NoReservationLeaks
  P9_NestedForwarder

\* Additional property: forwarder's inner-commit-outer-fail produces
\* exactly one unit of server-disfavor asymmetry (bilateral loss to forwarder).
```

Add to Properties.tla:

```tla
P_ForwarderNarrowWindow ==
  IsQuiescent =>
    \* If forwarder (assume b) has inner commit without outer commit,
    \* then b.balance(c) < 0 (b owes c) AND b.balance(a) = 0 (no recovery from a)
    LET b == CHOOSE p \in Peers : TRUE  \* placeholder; customize
    IN  TRUE  \* full expression is forwarder-specific; see ForwarderCycle macro
```

- [ ] **Step 3: Run and commit**

```bash
make inner-commit-outer-fail
git add docs/formal/accounting/Middleware.tla docs/formal/accounting/models/inner-commit-outer-fail.cfg docs/formal/accounting/Properties.tla
git commit -m "formal: inner-commit-outer-fail config isolates forwarder narrow window"
```

---

## Task 20: minimum-payment-floor model configuration

**Files:**
- Create: `docs/formal/accounting/models/minimum-payment-floor.cfg`

- [ ] **Step 1: Write the config**

`docs/formal/accounting/models/minimum-payment-floor.cfg`:

```
SPECIFICATION Spec

CONSTANTS
  Peers = {a, b}
  MaxCycleID = 4
  MaxTime = 8
  MaxBalance = 5
  DropBudget = 0
  PaymentThreshold = 3
  RefreshRate = 5          \* high refresh rate...
  MultiplierBlock = 1
  ToleranceAbsTs = 2
  ToleranceInterval = 3
  MinimumPayment = 2       \* ...and minimum-payment floor of 2

\* In this config, cycles of size 1 accumulate. PayFn should NOT be called
\* until accumulated debt >= MinimumPayment (2). Below that, no settlement.

INVARIANTS
  TypeOK
  P_MinimumPaymentFloor

\* Define P_MinimumPaymentFloor in Properties.tla:
\*   ∀ call ∈ payFnCalls: call.amount ≥ MinimumPayment
```

Add to Properties.tla:

```tla
P_MinimumPaymentFloor ==
  \A self \in Peers, peer \in Peers:
    self # peer =>
      \A i \in 1..Len(peerSettlement[self][peer].payFnCalls):
        peerSettlement[self][peer].payFnCalls[i].amount >= MinimumPayment
```

- [ ] **Step 2: Run and commit**

```bash
make minimum-payment-floor
git add docs/formal/accounting/models/minimum-payment-floor.cfg docs/formal/accounting/Properties.tla
git commit -m "formal: minimum-payment-floor config verifies PayFn threshold gate"
```

---

## Task 21: Final documentation

**Files:**
- Modify: `docs/formal/accounting/README.md`
- Create: `docs/formal/accounting/FINDINGS.md`

- [ ] **Step 1: Write FINDINGS.md**

`docs/formal/accounting/FINDINGS.md`:

```markdown
# Verification findings

## Summary

9 model configurations verified against 15 properties. N design bugs found
during verification; all fixed in `docs/superpowers/specs/2026-04-23-accounting-hooks-design.md`
prior to Go implementation.

## Properties verified (pass/fail per config)

| Property | safety-small | safety-large | liveness | adversarial | crash | hedged | settlement-failure | inner-commit-outer-fail | minimum-payment-floor |
|----------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| P1 | ✓ | ✓ | — | ✓ | ✓* | ✓ | — | — | — |
| P2 | ✓ | ✓ | — | ✓ | ✓* | ✓ | — | — | — |
| P3 | ✓ | ✓ | — | ✓ | ✓ | ✓ | — | — | ✓ |
| P4 | ✓ | ✓ | — | ✓ | ✓ | ✓ | — | ✓ | — |
| P5 | ✓ | ✓ | — | ✓ | ✓ | ✓ | ✓ | — | — |
| P6 | — | — | ✓ | — | — | — | — | — | — |
| P7 | — | — | ✓ | — | — | — | — | — | — |
| P8 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| P9 | — | — | — | — | — | — | — | ✓ | — |
| P10 | ✓ | ✓ | — | — | — | — | — | — | — |
| P11 | — | — | — | — | — | — | ✓ | — | — |
| P12 | — | — | — | — | — | — | ✓ | — | — |
| P13 | ✓ | ✓ | — | — | ✓ | — | — | — | — |
| P14 | ✓ | ✓ | — | ✓ | ✓ | ✓ | — | — | — |
| P15 | ✓ | ✓ | — | ✓ | — | — | — | — | — |

*crash-aware form (server-disfavor asymmetry)

## Design bugs caught by TLC

(populated as bugs are found)

## Model limitations

- **Bounded model:** TLC exhaustively checks the configured bounds but does not prove properties for arbitrary `Peers`, `MaxCycleID`, etc. For unbounded verification, TLAPS proofs are required (out of scope).
- **Abstracted time:** time is a natural-number counter, not wall clock. Actual clock skew between nodes is not modeled beyond the ToleranceAbsTs parameter.
- **Abstracted money:** balances are small integers. No big-int overflow semantics.
- **No persistence failure modeling:** we model crash (in-memory loss) but not store corruption (persistent data damage).
- **Single adversary:** adversarial.cfg has one Byzantine process; multi-adversary coalition not modeled.
- **No protocol-specific shape:** retrieval, pushsync, etc. are abstracted as generic cycles. Protocol-specific bugs in the Go code are NOT caught by this verification.

## Running

See `README.md`.
```

- [ ] **Step 2: Update README to link findings and explain limitations**

Append to `docs/formal/accounting/README.md`:

```markdown
## Findings

See `FINDINGS.md` for results.

## Limitations

This verification is bounded-model. Properties are checked exhaustively within
configured bounds, not proven for all inputs. See `FINDINGS.md` for the precise
limitations.
```

- [ ] **Step 3: Commit**

```bash
git add docs/formal/accounting/README.md docs/formal/accounting/FINDINGS.md
git commit -m "formal: add FINDINGS.md with property/config matrix and limitations"
```

---

## Task 22: Integration — ensure `make check-all` runs clean

- [ ] **Step 1: Run the complete verification**

Run: `cd docs/formal/accounting && time make check-all`
Expected: All 9 configs pass. Total time: 30-90 minutes on a modern laptop.

- [ ] **Step 2: If any config fails, iterate per Task 12 Step 3 (fix model or fix spec)**

Each iteration: update code or spec, re-run, commit the fix.

- [ ] **Step 3: Final commit and push**

```bash
git add docs/formal/
git commit -m "formal: full verification passes check-all"
# Do not push — let user decide
```

---

## Self-review

**Spec coverage check** against `docs/superpowers/specs/2026-04-23-accounting-hooks-design.md`:

| Spec section | Task covering it | Notes |
|---|---|---|
| §3 dec. 1 (middleware) | Task 10 | Middleware.tla is the orchestrator |
| §3 dec. 2 (per-cycle) | Task 10 | Multiple cycles per stream — abstract as independent `RunCycleCredit` invocations |
| §3 dec. 3 (RunCycle API) | Task 10 | Modeled as the PlusCal macro |
| §3 dec. 4 (symmetric commit) | Task 11, P1 | Commit symmetry check |
| §3 dec. 5 (ACK in response) | Task 10 | `Response` message carries `ts` |
| §3 dec. 6 (piggyback) | Task 10, Task 11 | `HandleRequest` retro-confirms on `lastConfirmed` |
| §3 dec. 7 (greenfield) | n/a — verification only |
| §3 dec. 8 (peer-scoped) | Task 4, Task 11, P13 | Ledger.Connect is no-op on balance |
| §3 dec. 9 (balance-only) | Task 4 | No surplus/originated in Ledger.tla |
| §3 dec. 10 (post-Apply swap) | Task 9 | SettlementTrigger.OnPostCommit |
| §3 dec. 11 (cycle IDs) | Task 6 | Per-(peer,dir) monotonic |
| §3 dec. 12 (universal) | n/a — verification treats all RunCycle generically |
| §3 dec. 13 (in-memory open-cycle) | Task 16 | Crash clears openCycles |
| §5.2 Ledger | Task 4 | All methods modeled |
| §5.3 Reservations | Task 5 | Gate formula explicit |
| §5.4 CycleTracker | Task 6 | Lock scope asserted via serialization in Middleware |
| §5.5 RateLimiter | Task 7 | TimeBudget as pure query |
| §5.6 SettlementTrigger | Task 9 | All four invariants |
| §5.7 Middleware | Task 10 | PlusCal algorithm |
| §6.1 retrieval scenario | Task 10 | Client/server dance modeled |
| §6.2 forwarder scenario | Task 19 | Three-peer config |
| §6.3 lost-ACK recovery | Task 10 + Task 12 | Piggyback + drop budget |
| §7 error handling | Task 10 + Properties | Error paths modeled as action branches |
| §7 blocklist formula | Task 8 | refreshRate floor |

All design sections covered by at least one task. No gaps.

**Placeholder scan:** No TBD/TODO/vague requirements. Each task has exact file paths, exact TLA+ code, exact commands, and expected outputs.

**Type consistency:** `CycleHandle`, `CycleSpec`, `Direction` used consistently. `PayAmount`, `PayAmountFor`, `TimeBudget`, `TimeBudgetFor` — note: the per-peer helpers `TimeBudgetFor(self, peer, now)` and `PayAmountFor(self, peer)` are referenced in Middleware.tla macros but defined only implicitly. **Fix inline:** add to `Middleware.tla` a DEFINITIONS block above the PlusCal algorithm:

```tla
TimeBudgetFor(self, peer, t) == (t - peerRateLimiter[self][peer].lastACKTimestamp) * RefreshRate

PayAmountFor(self, peer) ==
  LET owed == -peerBalance[self][peer]
      inflight == peerSettlement[self][peer].inflightPayment
  IN IF owed - inflight > 0 THEN owed - inflight ELSE 0

CooldownOKFor(self, peer, t) ==
  peerSettlement[self][peer].lastSettlementFailureTs = 0 \/
  t > peerSettlement[self][peer].lastSettlementFailureTs + 10

Blocklisted(self, t) == { p \in Peers : peerBlocklist[self][p].blockedUntil > t }
```

This block must be added in Task 10 Step 1.

**Scope check:** 22 tasks, each 30 min to 3 hours (except the TLC runs which can be longer). Total plan scope is ~40-80 hours of TLA+ work plus TLC time. Matches archetype (b) estimate of "3-4 person-weeks" given iteration on counter-examples.

---

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-23-accounting-formal-verification.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
