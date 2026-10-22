---
name: release-test-plan
description: Builds and runs a QA test plan for an upcoming Bee release. It takes every PR merged to master since the last release tag, sends sub-agents to analyze each PR, shows the operator a short per-PR plan in the terminal for approval, then runs the approved tests with one sub-agent per PR and reports results, issues by severity, and suggested fixes. Use when asked what to test for the next release, for a release test plan or regression checklist, to run release QA, or for a QA rundown of the changes since vX.Y.Z.
---

# Release test plan

Build a test plan for every PR merged to master since the last release tag,
get the operator's approval, run the approved tests, and report the results.
You orchestrate. Sub-agents read the diffs and run the tests. You keep only
their one-line results, so your context stays small even when the range holds
many PRs.

The flow has three phases. Phase 2 is a hard gate:

1. **Plan** (steps 1–4). Read-only. The output is files under `dist/`.
2. **Approval** (step 5). Show the plan inline in the terminal. Stop. Run
   nothing until the operator approves.
3. **Run and report** (steps 6–7). One sub-agent per approved PR runs that
   PR's tests. You report results, issues, severity, and suggested fixes.

In every phase: do not comment on, label, or edit PRs or issues. Do not
commit, push, stash, or switch branches in the main working tree. All output
goes under `dist/`, which is gitignored.

## Writing style (ASD-STE100 principles)

Write every plan and result file (`prs/*.md`, `synthesis.md`, `TEST-PLAN.md`,
`approval.md`, `results/*.md`) and every user-facing report in **ASD-STE100 principles** (Simplified Technical English).

**Why.** QA text is an instruction radio, not a blog. Short, same-word,
one-idea sentences cut misreads when someone else runs the steps days later.

Rules (principles, not full dictionary lock):

- One idea per sentence. Prefer short sentences.
- Use the active voice. Put the actor first: "The node rejects the stamp", not
  "The stamp is rejected by the node".
- Use the same term for the same thing every time. Do not switch between
  synonyms (for example pick `chunk` or `data unit`, not both).
- Prefer concrete verbs and nouns. Avoid filler ("basically", "simply",
  "it should be noted that") and vague words ("appropriate", "properly",
  "correctly") unless you state the measurable check.
- Keep Bee domain terms as-is (`overlay`, `postage`, `SOC`, `reserve`). Do not
  invent softer paraphrases for them.
- Steps must be commands a tester can run. Expect must be an observable result
  (status code, body field, metric, log line), not a soft wish.

Do not claim full ASD-STE100 dictionary compliance. Swarm terms sit outside that
word list; follow the grammar and clarity rules above.

## Inputs

- **Base**: a tag. The default is the highest final `v*` tag; release
  candidates are skipped. If the user names a tag or an rc, use that instead.
- **Head**: the default is `origin/master`. The user can name another ref.

## Step 1: collect and triage

```bash
git fetch origin master --tags --quiet
.claude/skills/release-test-plan/scripts/collect-prs.sh [BASE] [HEAD]
```

Release tags are cut on `release-*` branches and are usually **not** ancestors
of master. The script therefore uses `merge-base(BASE, HEAD)..HEAD` and marks
commits that were already cherry-picked into BASE. Read the `#` header lines:
if `rc_tags_near_range` lists an rc newer than BASE, tell the user, because they
may want that rc as the base instead.

Create the output directory
`dist/test-plans/<BASE>..<head-short-sha>/` with a `prs/` subdirectory, and
save the TSV there as `prs.tsv`.

Triage each row by `status` and `kind`:

| Row | Action |
|---|---|
| `in-base` | No agent. List it under "Already shipped in BASE". |
| `reverted:by#N` whose revert is also in range | No agent for either PR. List the pair under "Net-zero (reverted)". If a later PR re-lands the feature (check subjects; for example "gsoc grained api" after a gsoc revert), mention that link in the re-landing PR's agent brief. |
| `revert:of#N` whose target is **not** in range | Analyze it as a normal PR, because it removes behavior that shipped in BASE. |
| `kind` = `ci`, or `kind` = `deps` touching only `go.mod`/`go.sum` | No agent and no test plan. List it under "Not tested (deps and CI)" with its subject, for example "bump google.golang.org/grpc 1.83.1 to 1.83.2". |
| `kind` = `deps`, touching other files | List the bump under "Not tested (deps and CI)". Analyze the code changes outside `go.mod`/`go.sum` as a normal PR, and say in `{{NOTES}}` that the version bump itself is out of scope. |
| `kind` = `docs` or `test-only` | No agent. Run `git show --stat <sha>` to confirm. List it under "No runtime testing needed" with a one-line reason. If the stat shows non-test code after all, analyze it. |
| `no-pr` (direct push) | Analyze it by sha, like a PR. |
| everything else | Analyze it. |

## Step 2: fan out sub-agents

Read [references/pr-analyst.md](references/pr-analyst.md) once. It is the brief
each sub-agent gets. Fill in its placeholders (`{{REPO}}` with the absolute repo
path, `{{BASE}}`, `{{HEAD}}`, `{{MB}}`, `{{OUT}}` with the absolute output
directory, `{{PRS}}`, and `{{NOTES}}`, or "none") and pass everything below its
line as the prompt.

- Use `subagent_type: general-purpose`. Each agent needs Bash (for `git` and
  `gh`), Read, and Write.
- One agent per PR. PRs with no more than 40 changed lines of non-test code
  (estimate from `lines`, after subtracting test files in `git show --stat`)
  can be batched up to 4 per agent. List each PR in that agent's brief.
- Launch in waves of up to 8 Agent calls **in a single message** so they run
  concurrently. Start the next wave as notifications arrive. Do not poll.
- Each agent writes `prs/<PR>.md` and returns only `RESULT` lines (the format is
  in the brief). Keep those lines; you do not need to read the files yet.
- If an agent errors or returns no `RESULT` line, relaunch it once. If it fails
  again, record the PR as "not analyzed" in the summary. Never silently drop a
  PR.

## Step 3: release-wide checks

Run these yourself; they are cheap, use git only, and catch changes that cross
PRs. `$MB` is the merge-base printed in the header.

```bash
git diff --stat $MB..HEAD -- openapi/                       # API surface changes
git diff $MB..HEAD -- openapi/Swarm.yaml | grep -n 'version:'  # info.version bumped?
git diff $MB..HEAD -- cmd/bee/cmd/cmd.go packaging/          # new or changed flags and defaults
git diff --stat $MB..HEAD -- '*.proto'                       # wire changes, so test mixed versions
git diff --stat $MB..HEAD -- pkg/storer/migration pkg/statestore pkg/shed  # migrations, so test upgrades
```

Each one that returns output becomes a release-level item in the plan, for
example: "Protocol X changed. Run a mixed cluster with BASE and HEAD nodes and
check that push and retrieval work in both directions."

Then launch **one synthesis agent**. Give it the output directory, the list of
`RESULT` lines, and the release-wide findings. It reads `prs/*.md` and writes
`synthesis.md` with three parts:

1. **Interactions.** PRs that touch the same subsystem and should be tested
   together in one scenario. Example: several topology and hive PRs together
   call for one connectivity and convergence run, not three separate ones.
2. **Consolidated beekeeper run.** The deduplicated set of checks, the cluster
   shape (full and light node counts), and whether a mixed-version cluster is
   needed.
3. **Upgrade path.** Steps to upgrade a node with an existing data directory
   from BASE to HEAD, if any migration, statestore, or config default changed.

## Step 4: assemble and verify

Write `TEST-PLAN.md` in the output directory:

1. Header: base, head sha, merge-base, date, PR count by triage bucket.
2. **Summary table**, built from the `RESULT` lines and sorted by risk (High,
   then Medium, then Low): PR, title, risk, env, areas, beekeeper check types,
   plan summary.
3. The release-level items from step 3.
4. The contents of `synthesis.md`.
5. One section per analyzed PR: concatenate `prs/*.md` in merge order (the TSV
   order).
6. Appendices: "Not tested (deps and CI)", "Already shipped in BASE",
   "Net-zero (reverted)", "No runtime testing needed", "Not analyzed".

Before you report, verify:

- Every PR in `prs.tsv` appears exactly once somewhere in `TEST-PLAN.md`.
  Check the PR numbers against the TSV mechanically, with grep.
- For each High-risk PR, open its section and spot-check one concrete claim
  (an endpoint, flag, or function) against the code. If it is wrong, fix the
  section and say so in your report.

## Step 5: approval gate

Do not run any test before the operator approves. Print the plan **inline in
the terminal**, built only from the `RESULT` lines. Do not paste
`TEST-PLAN.md`; give its path.

    Test plan v2.8.2..c6a3eba0 · 14 PRs to run (3 High, 6 Medium, 5 Low)
    Full plan: dist/test-plans/v2.8.2..c6a3eba0/TEST-PLAN.md
    Environment: kube context <ctx> · namespace <ns> · beekeeper cluster <name>

    Release-level
      - <proto> changed: mixed BASE/HEAD cluster, push and retrieve both ways.

    High
      #<N> <title> [chain]
        <What you run.> <What proves a pass.>
    Medium
      ...
    Low
      ...
    Not run: 9 (deps/CI 4, docs/test-only 3, in BASE 1, reverted 1)

Each PR gets at most two sentences: what you run, and what proves a pass. Tag
each PR with its env: `local` (go test or one local node), `cluster`
(beekeeper or several nodes), or `chain` (sends transactions).

Then ask with AskUserQuestion:

- **Approve all**: run every listed PR.
- **Approve local only**: run `local` PRs. Skip `cluster` and `chain` PRs.
- **Edit**: the operator writes changes as free text, for example
  "drop #5583; #5571 skip the restart; add a light node to #5630".

On Edit, update the affected `prs/<N>.md` sections. Print only the changed PRs
again, and ask again. Write the final approved set and every operator edit to
`approval.md`. Approval covers only the scenarios in that file.

If the environment line is not confirmed and any `cluster` or `chain` PR is
approved, ask for the kube context before step 6. The default kube context can
be a shared cluster.

## Step 6: run the approved tests

Read [references/pr-tester.md](references/pr-tester.md) once. Fill in its
placeholders and launch **one sub-agent per approved PR** (`subagent_type:
general-purpose`). One agent runs every scenario of its PR. Never split a PR
into one agent per test case. Never put two PRs in one tester, because a
failure in one PR can hide a failure in the other.

Schedule by env:

1. If the synthesis has a consolidated beekeeper run and any `cluster` PR is
   approved, start that run yourself first with Bash `run_in_background`.
   Save its log to `results/cluster-baseline.log`. Testers read this log and
   do not run those checks again.
2. `local` PRs: launch in waves of up to 8 Agent calls in a single message.
   They can run while the baseline runs.
3. `cluster` and `chain` PRs: launch one at a time, after the baseline ends.
   They share one cluster and one chain, so parallel runs corrupt each other's
   state (balances, reserve, radius).

Each tester writes `results/<N>.md` and returns only `TESTED` and `ISSUE`
lines. Keep those lines. If an agent errors or returns no `TESTED` line,
relaunch it once. If it fails again, record the PR as "not run" with the
reason. Never silently drop a PR.

When all testers are done, run `git status --porcelain` and check that no
`zz_qa_*` file is left in the tree. If one is left, move it to
`results/tests/<N>/`.

## Step 7: verify and report

Before you report:

- Check that every approved PR has exactly one `TESTED` line or a "not run"
  entry.
- For each Critical or High issue, read its evidence in `results/<N>.md`. If
  the repro takes less than 5 minutes, run it once yourself. Downgrade or drop
  an issue you cannot reproduce, and say so in the report.
- Merge issues from different PRs that have the same root cause.

Write `results/REPORT.md` with the rundown, the issue list, and links to each
`results/<N>.md`. Then print this in the terminal:

    Results v2.8.2..c6a3eba0 · 14 run: 10 pass, 2 fail, 1 partial, 1 blocked
    Verdict: NO-GO (1 Critical regression)

    Rundown
      PR     Verdict   Passed  Note
      #<N>   FAIL      3/5     <one line>
      #<N>   PASS      4/4
      ...

    Issues (most severe first)
      [Critical] #<N> regression · <finding>
        Fix: <suggested fix> (<file:line>)
        Evidence: results/<N>.md#s2
      [Medium] #<N> pre-existing · <finding>
        Fix: ...
    Not run: #<N> (blocked: <reason>), ...

Verdict rule: **NO-GO** if any Critical issue exists, or any High issue is a
regression in this range. Otherwise **GO**, with the open issues listed.
Pre-existing issues never block a release alone. List them separately.

Offer to publish `REPORT.md` as an Artifact page and to draft issue text for
each Critical and High issue. Do not file issues yourself.

## Reference

- [references/pr-analyst.md](references/pr-analyst.md): the per-PR sub-agent
  brief and output format.
- [references/test-areas.md](references/test-areas.md): a map from Bee packages
  to what to exercise and which beekeeper checks cover them.
- [references/pr-tester.md](references/pr-tester.md): the per-PR test runner
  brief, the severity rubric, and the result format.
