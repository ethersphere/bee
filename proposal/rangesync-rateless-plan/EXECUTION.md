# Execution plan: `rangesync` (rateless)

How to run the tasks in [README.md](README.md) and the phase files. The phase files say
**what** to build. This file says **in what order, on which branches, how many at once, and
where a person has to decide before work continues**.

There are **no PRs**. Work lands on a stack of local phase branches (§3).

## 1. Shape of the work

- 33 tasks. T2.7 is optional. T2.8 (cross-architecture vectors) is deferred until before mainnet.
  Phase 5 is optional for testnet and required for mainnet.
- Rough effort in session units (S = 1, M = 2, L = 3): Phase 1 ≈ 24, Phase 2 ≈ 16, Phase 3 ≈ 10,
  Phase 4 ≈ 12, Phase 5 ≈ 6, Phase 6 ≈ 9 (plus testnet wall-clock). Total ≈ 80 units.
- **Critical path.** Derived from each task's "Depends on" line, not from the wave table:

  ```
  T1.0 → T1.4 → T1.5 → T1.9 → T3.1 → T3.2 → T4.2 → T4.3 → T4.4 → T4.5 → T4.6 → T6.1 → T6.4
  ```

  ≈ 26 units even with unlimited parallelism. Two things follow from it:
  - **The storer work (T1.0 → T1.4 → T1.5) gates everything.** T1.0 starts on day one.
  - **Striping (Phase 3) is on the critical path, not the rateless work (Phase 2).** T4.2 needs
    T3.2. The Phase 2 chain (T2.0 → T2.1 → T2.2 → T2.3 → T2.4) has about 2 units of slack before
    T4.3 needs it. A slip in T2.1/T2.2 eats that slack first; a slip in T3.2 moves the end date
    directly.

## 2. Pre-flight (before the first task session)

| # | Item | Status |
|---|---|---|
| P1 | Fix the duplicated header in README "Parallelism map". | Done |
| P2 | T1.6's tests used the storer mock, which arrives in T1.5. T1.6 now uses a local fake; the mock-based check moved to T1.9. | Done |
| P3 | Implementers no longer tick README; the orchestrator does when landing. README no longer refers to PRs. | Done |
| P4 | Cross-architecture vectors deferred: T2.1 checks amd64 only; the arm64 run is T2.8 (D22); testnet cohorts are amd64-only until then (T6.3). | Done |
| P5 | Set a git identity in the bee repo. Nothing is configured (no `~/.gitconfig`, no repo `user.*`), so sessions can't commit. The last commit used `acud <12988138+acud@users.noreply.github.com>`. Worktrees share the repo config. | **Open** |
| P6 | Install `protoc` on PATH (`make protobuftools` then installs `protoc-gen-gogofaster` v1.3.1). Check that `make format` bootstraps `gofumpt`/`gci`; neither is on PATH. `golangci-lint` is already in `~/go/bin`. | **Open** (needed by T1.1) |
| P7 | Commit the plan fixes on `docs/rangesync-rateless-proposal`, then create the phase branches (§3). | Done |
| P8 | Decide whether Phase 5 runs before Phase 6 (D11). The schedule fits it in without lengthening the critical path. | **Open** |
| P9 | `proposal/pullsync-rangesync-proposal.md` (the earlier, non-rateless proposal) is untracked. Commit it as history or leave it out. | **Open** |

## 3. Branches

### The stack

```
docs/rangesync-rateless-proposal   (master + the plan)
  └─ rangesync/phase-1
       └─ rangesync/phase-2
            └─ rangesync/phase-3
                 └─ rangesync/phase-4
                      └─ rangesync/phase-5
                           └─ rangesync/phase-6
```

- Every phase branch is created up front, each from the one below, so every lane has a place to
  land from step 0.
- **Invariant:** each phase branch always contains the whole branch below it. Phase 3 doesn't need
  Phase 2's code, and Phase 5 mostly needs Phase 1. They still sit in this order, so every phase
  sees everything that came before it.
- If Phase 5 is skipped for now, `rangesync/phase-5` stays empty (the same commit as
  `phase-4`), and `phase-6` still stacks on it. Doing Phase 5 later means restacking `phase-6`
  on top of it, which is the normal operation below.
- **Phase branches and the docs branch are never checked out**; only task branches are, each in
  its own worktree. Keep the main checkout detached. `rebase --update-refs` won't move a branch
  that's checked out somewhere.

### Task branches

Several lanes work on the same phase at once (for example T1.3 and T1.7 in step 1), so each
implementer still gets a short-lived branch and worktree:

```bash
git worktree add ../bee-wt/<ID> -b rangesync/<ID>-<slug> rangesync/phase-<N>
```

The session commits on its task branch: one or a few commits, each prefixed with the task ID
(`rangesync: T1.4 opt-in chunkAddr index and reserve observer`).

### Landing a task

The orchestrator runs these from the main checkout:

```bash
# 1. bring the task up to date with its phase branch
git -C ../bee-wt/<ID> rebase rangesync/phase-<N>
# 2. fast-forward the phase branch (refuses anything that isn't a fast-forward)
git fetch . rangesync/<ID>-<slug>:rangesync/phase-<N>
# 3. restack every phase above it in one go
git rebase --update-refs rangesync/phase-<N> rangesync/phase-6
git switch --detach   # the rebase leaves phase-6 checked out; detach again
# 4. tick the box in README.md on phase-<N> (a separate small commit), then clean up
git worktree remove ../bee-wt/<ID> && git branch -d rangesync/<ID>-<slug>
```

Tested with git 2.47 on a scratch repo: step 2 rejects a non-fast-forward, and step 3 moves every
intermediate phase branch. After step 3, running sessions on higher phases rebase their task
branch onto their (moved) phase branch before landing, which step 1 already does.

Restacks are cheap only while lower phases change things higher phases don't touch. Most lanes
work in separate packages, so conflicts are rare. When step 3 does conflict, resolve it in the
higher phase's commit, not by changing the lower phase.

### Staying current with master

Rebase the whole stack onto `origin/master` at the end of each phase (and sooner if
`pkg/storer`, `pkg/node` or `pkg/storageincentives` move upstream):

```bash
git fetch origin
git rebase --update-refs origin/master rangesync/phase-6
git switch --detach
```

This also moves `docs/rangesync-rateless-proposal`, since its plan commits aren't on master.

The line numbers the tasks cite (`node.go:1184`, `:1281`, `:1289`, `:1327`, `agent.go:406`) are
correct as of `9edc5ac3` and will drift. Sessions should treat them as hints.

## 4. How each task runs

### Roles

| Role | Who | Does | Never does |
|---|---|---|---|
| Orchestrator | One Claude session started from the **bee repo root**, so it and its subagents load bee's `CLAUDE.md` → `AGENTS.md` (a session started elsewhere passes that repo's instructions to every subagent) | Creates worktrees, spawns subagents, routes findings back to implementers, lands and restacks, ticks README, consolidates Decisions logs, writes gate summaries, amends the plan after a gate | Write or fix task code; run the task's checks itself |
| Implementer | A subagent per task (an L task can use several in a row) | Contract (where required), tests first, implementation, commits on the task branch, fixes findings | Land, restack, tick README |
| Verifier | A **fresh** subagent per verification pass | Runs the mechanical checks below and reports pass/fail with output excerpts | Edit code; judge design |
| Reviewer | A **fresh** subagent per review pass | Reads the diff against the task text and the cited proposal sections; reports findings ranked by severity | Edit code |
| You | — | Decide at gates (§6). Landing is automatic: a task that passes verification and review lands without asking | — |

The implementer is never its own verifier or reviewer. The verifier's work is running commands and
comparing results with the task text, so it can run on a smaller model (e.g. Sonnet) if cost
matters.

### Where the spec comes from

1. **The proposal is the design.** It defines the wire format (§6.2), the storer API (§5.4), the
   per-range decision rule (§6.3) and validation (§6.4).
2. **The phase-file task is the scope and acceptance:** which files to touch and which tests must
   exist.
3. **Contracts between tasks running in parallel** are defined only loosely in the proposal (for
   example the `Index` API that T1.8 codes against while T1.6 writes it). Tasks that publish an
   interface go **contract first**: the implementer's first commit contains only the exported
   types, interfaces and doc comments (it must build and lint), plus the list of tests it will
   write in the commit message. That commit is reviewed and landed on its own, so other lanes can
   code against it before the implementation lands.

   Contract-first tasks: T1.1 (proto and shared types; also G2), T1.4 (`ReserveObserver`, after
   G1), T1.5 (`ReserveAddrStore`), T1.6 (`Index`), T1.8 (`Session` and the local-summary
   interface), T2.1 (`Mapper`, `Decoder`), T1.11 (reconciler, `PeerScorer`). Every other task goes
   straight to tests and implementation.

### The loop

1. **Prepare (orchestrator).** Create the worktree and task branch from the phase branch (§3).
   Not the Agent tool's own worktree option: it branches from the session's repo, which must be
   bee for that to work, and it doesn't know about phase branches.
2. **Contract (contract-first tasks only).** The implementer commits the contract. A reviewer
   checks it against the proposal and against the tasks that will consume it. The orchestrator
   lands that commit and restacks.
3. **Implement.** The implementer gets the README session prompt plus: the task ID, the worktree
   path, the gate decisions so far, and the contracts it consumes. Tests first, then code, then the
   AGENTS.md pre-commit checklist, then commits. It reports a summary, decisions, follow-ups and
   any deviation from the task text.
4. **Verify and review, in parallel.**
   - **Verifier** checks, in the task's worktree, against `rangesync/phase-<N>`:
     1. `make format` leaves no diff; `make build`, `make lint`, `make vet` pass.
     2. `go test` (with `-race` where the task involves concurrency) passes for every touched
        package; `make fuzz-regression` for every fuzz target touched.
     3. Tasks touching `pkg/storer` or `pkg/node`:
        `go test ./pkg/pullsync/... ./pkg/puller/... ./pkg/storer/...` passes (ground rule).
     4. `git diff --stat rangesync/phase-<N>...` touches only what the task names; nothing in
        `pkg/pullsync` or `pkg/puller`; no `go.mod` change.
     5. Every test the task's **Tests** paragraph asks for exists and runs (it maps each item to a
        test function and lists any that are missing).
     6. Every new decoder of peer input has a `*_fuzz_test.go`.
     7. The phase file's Decisions log / Follow-ups have the entries the task asks for.
   - **Reviewer** reads the task, the proposal sections it cites, the ground rules and the diff;
     reports correctness bugs, deviations from the proposal, and missing edge cases, by severity.
5. **Fix.** The orchestrator sends verifier failures and reviewer findings to the **same**
   implementer (`SendMessage`, so it keeps its context). After the fix, a fresh verifier runs
   again; a fresh reviewer runs again if any finding was high severity. After two fix rounds
   without a clean pass, the orchestrator stops and brings the task to you.
6. **Land (orchestrator), automatically.** No approval is needed once verification and review
   pass: §3 "Landing a task", then tick README and copy the task's decisions into the phase file
   if the implementer didn't. The orchestrator stops only at gates (§6) and at a task that stays
   unclean after two fix rounds.

**Spikes (T1.0, T2.0, T6.0).** The implementer investigates and writes the result (and, for
T1.0, the regression test). The verifier checks the test runs. Instead of a code review, the
orchestrator summarises the findings for you at the gate; you decide; the orchestrator amends the
phase files before dependent tasks start.

**Concurrency.** Up to three implementers at once (one per lane, §5). Verifiers and reviewers are
short and run alongside them, but this host has 4 cores: only **one** verifier at a time may run
the heavy suites (`pkg/storer` with `-race`, the coexistence suite, integration tags, the 4.2M-item
fill benchmark). The orchestrator queues those.

**State lives on disk**, not in the orchestrator's context: the branch stack, the README
checklist, the phase files' Decisions logs, and the worktrees in flight. A new orchestrator
session can pick up from there.

## 5. Schedule: three lanes

Lanes keep conflict-prone areas in one session line. Lane A is the critical path. Lane B covers
the pure `fingerprint` package. Lane C covers protocol/reconciler glue. A step ends when its
tasks have landed; steps are not equal in length. Each task lands on its own phase's branch,
whichever lane runs it.

| Step | Lane A: spine (critical path) | Lane B: fingerprint / rateless | Lane C: protocol / reconciler | Gate after |
|---|---|---|---|---|
| 0 | **T1.0** snapshot spike | **T1.1** skeleton, proto, constants | T2.0 CertainSync spike · T6.0 criteria (docs) | G1, G2, G3 |
| 1 | **T1.4** storer index + observer | T1.3 Fenwick → T1.2 LtHash | T1.7 want set → T1.8 session logic | — |
| 2 | **T1.5** fill, iteration, API, mock | T1.6 Index | T1.8 (cont.) | G4 |
| 3 | **T1.9** status / reconcile | T2.1 rateless library | T1.10 fetch | — |
| 4 | **T3.1** stripe planner → **T3.2** striped transfer | T2.1 (cont.) → T2.2 symbol caches | T1.11 minimal reconciler | — |
| 5 | **T3.2** (cont.) | T2.2 (cont.) → T2.3 | T1.12 node wiring → T1.13 | **G5** Phase 1 done |
| 6 | **T4.2** state machine | T2.4 symbol session | T4.1 gossip → T3.4 radius → T3.3 verification | — |
| 7 | **T4.3** scheduler | T2.5 limits · T2.6 benchmarks | T3.5 catch-up `SyncRate` · T6.2 toggle tests | — |
| 8 | **T4.4** want set in rounds | T5.1 → T5.2 (if Phase 5) | T2.7 integer mapper (optional) | — |
| 9 | **T4.5** replace minimal reconciler | T5.3 (if Phase 5) | T6.3 runbook + dashboards | — |
| 10 | **T4.6** simulation | — | — | **G6** tuning |
| 11 | **T6.1** acceptance suite | T6.2 rerun | T6.3 testnet comparison runs (human, wall-clock) | G7 |
| 12 | **T6.4** exit report | — | — | — |

T2.8 isn't scheduled; it runs once an arm64 environment exists, any time after T2.1.

All dependencies were checked against each task's "Depends on" line. The non-obvious ones:
T1.9 waits for T1.5, T1.6 **and** T1.8 (all end at step 2). T3.2 waits for T1.10. T4.2 waits for
T3.2 and T1.11. T4.3 waits for T4.1, T4.2 and T2.4 (all step 6). T4.5 waits for T3.5.

Lane B's slack: at steps 8–10 Lane B is free if Phase 5 is skipped. Use it for T2.7 or to split
an L task off Lane A (e.g. the analytical half of T4.6).

## 6. Gates (a person decides before dependent work starts)

| Gate | After | Decision | If negative |
|---|---|---|---|
| G1 | T1.0 | Does the IndexStore iterator give snapshot isolation? What is the final `ReserveObserver.Commit` shape? | Amend T1.4 and T1.6 in the phase file **before** step 1 starts on Lane A. Lane B continues with T1.2/T1.3, which are unaffected. |
| G2 | T1.1 | Review the wire schema once: `Symbols`, `More`, `Status.Mapping`, `Message.AuditRound`. | Fix it in T1.1. Phases 2 and 5 assume it is frozen. |
| G3 | T2.0 | Rateless IBLT or CertainSync? | If CertainSync, rewrite T2.1–T2.4 before T2.1 starts. Lane B takes T1.7/T1.8 from Lane C meanwhile. |
| G4 | T1.5 | Blocking or background fill (measured: < 60 s for 4.2M items?). | A background fill adds `Busy` handling to T1.9 and changes T6.2's first case. Note that this 4-core host may not represent a production node. |
| G5 | T1.13 | Phase 1 done: a node with `--puller-enable=false --rangesync-enable=true` syncs; coexistence green; defaults byte-identical. | Rebase the stack onto `origin/master` (§3). Confirm the Phase 5 decision (P8). |
| G6 | T4.6 | Cache sizes (Q2), request-driven vs streamed batches (Q3), `CatchUpMinDiff` (Q9), `RoundInterval` bounds, from `pairwise_diff`. | Update constants before T6.1 so acceptance runs use the final values. |
| G7 | T6.1 / T6.3 | Testnet cohorts cover whole neighborhoods (D12) and run on amd64 only (D22); Phase 5 done if collision detection is in scope. | — |

## 7. Conflict hotspots and landing order

| Area | Tasks | Rule |
|---|---|---|
| `pkg/storer` | T1.0, T1.4, T1.5 (T3.5 tests only) | Already sequential on Lane A. |
| `pkg/rangesync/rangesync.go` (`Service`, `New`) | T1.1, T1.9, T1.10, T2.3 | T1.9 and T1.10 run at the same step: land T1.9 first; T1.10 rebases in landing step 1. |
| `pkg/rangesync/session.go` | T1.8, T2.4, T2.5, T3.1 (Striper hook), T5.3 | T3.1 implements the hook in `reconciler/stripes.go` and only touches `session.go` to wire it. |
| `pkg/rangesync/reconciler/` | T1.11, T3.1–T3.5, T4.1–T4.6 | Busiest package, spread over phase-3 and phase-4. At step 6, land Lane C's T4.1 before Lane A's T4.2 if both are ready. A phase-3 change here restacks into phase-4's work, so keep reconciler edits in phase-3 tasks narrow. |
| `constants.go`, `metrics.go` | most tasks | Append-only edits; resolve restack conflicts mechanically. |
| `pkg/node`, `cmd/bee` | T1.12, T5.1 (wiring), T6.2 | Sequential by schedule. |
| README checklist, phase-file Decisions logs | every task | The checklist is ticked only at landing. Decisions logs are append-only. |

## 8. Risks

- **Restack churn.** Phases are interleaved (step 3 already has Phase 1 and Phase 2 work in
  flight), so lower phases keep changing under higher ones. Most of it is in separate packages and
  restacks cleanly. The reconciler package (phases 3 and 4 at steps 6–7) is the exception; land
  those tasks one at a time and restack immediately.
- **Storer changes on the critical path.** T1.4/T1.5 are the only invasive changes. Review T1.0's
  result and T1.4's diff carefully before landing; a later fix there restacks through all six
  branches.
- **L tasks overrun one session** (T2.1, T2.2, T2.4, T3.2, T4.6, T6.1). Plan to land them in
  parts. T2.1 splits naturally into mapper + checksum + encoder / decoder / wire codec; T3.2 into
  paging + fetch pipeline / reassignment.
- **Upstream drift** in `pkg/storer` and `pkg/node` over the life of the stack. Rebase onto
  `origin/master` at each phase boundary.
- **Float mapper on mixed architectures.** With T2.8 deferred, nothing yet shows that
  `FloatMapper` gives identical results on arm64. Keep testnet runs amd64-only until T2.8 runs.
- **T6.3 needs testnet infrastructure and wall-clock time.** Plan the cohorts (whole
  neighborhoods, amd64) during step 9 so they're ready when T6.1 lands.
