---
name: release-test-plan
description: Builds a QA test plan for an upcoming Bee release. It takes every PR merged to master since the last release tag, sends sub-agents to analyze each PR's diff and description, and writes a per-PR rundown of what to test (beekeeper checks, API calls, gaps CI misses with proposed new tests, upgrade and compatibility risks) plus a release-level summary. Use when asked what to test for the next release, for a release test plan or regression checklist, or for a QA rundown of the changes since vX.Y.Z.
---

# Release test plan

Produce a test plan covering every PR merged to master since the last release
tag. You orchestrate. Sub-agents read the diffs, and you keep only their
one-line results, so your context stays small even when the range holds many
PRs.

Everything here is read-only. Do not comment on, label, or edit PRs or issues,
and do not commit. The only output is files under `dist/`, which is gitignored.

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
   then Medium, then Low): PR, title, risk, areas, beekeeper check types.
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

## Report

Tell the user the path to `TEST-PLAN.md`, the PR count per bucket, the High-risk
PRs with one line each, and any release-level items (protocol, migration, API,
or flag changes). Offer to publish the plan as an Artifact page if they want to
share it with the team.

## Reference

- [references/pr-analyst.md](references/pr-analyst.md): the per-PR sub-agent
  brief and output format.
- [references/test-areas.md](references/test-areas.md): a map from Bee packages
  to what to exercise and which beekeeper checks cover them.
