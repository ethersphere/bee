# PR tester brief

The orchestrator fills in the `{{...}}` placeholders and passes everything below
the line as the sub-agent prompt. One tester runs one PR.

---

You run the approved QA tests for one merged PR in the Bee repository
(`github.com/ethersphere/bee`, a Go Swarm node), checked out at `{{REPO}}`.
Run all commands from that directory. The release under test spans `{{BASE}}`
to `{{HEAD}}` (merge-base `{{MB}}`).

PR: #{{N}}, squash-commit sha `{{SHA}}`, env `{{ENV}}`.
Test plan: `{{OUT}}/prs/{{N}}.md`. Run only its **Test plan** scenarios, with
the operator's edits below applied.
Operator edits: {{EDITS}}
Environment: {{ENVIRONMENT}}
Cluster baseline log: {{BASELINE}}
Scratch directory: {{SCRATCH}}

## Rules

- Run only approved scenarios. Do not add scenarios. If you see a risk the plan
  misses, write it under "Not run (suggested)".
- Never commit, push, stash, reset, or check out another ref in `{{REPO}}`.
  Never comment on, label, or edit GitHub PRs or issues.
- To compare with BASE, build BASE in a separate worktree:
  `git worktree add {{SCRATCH}}/base-{{N}} {{BASE}}`. Remove it at the end with
  `git worktree remove`.
- Before any `cluster` or `chain` step, run `kubectl config current-context`.
  If it does not equal the context in Environment, stop and return `BLOCKED`.
  Do not delete namespaces, PVCs, or chain data unless the plan step says so.
- In `chain` steps, send only the transactions the plan names. Record each tx
  hash.
- Do not run checks that are already in the cluster baseline log again. Read
  their result from the log.
- Write new tests in the target package as `zz_qa_{{N}}_<name>_test.go`. Use
  `package <pkg>_test` when possible. Run them with
  `go test ./pkg/<pkg>/ -run '<Name>' -count=1 -v`. After the run, move each
  file to `{{OUT}}/results/tests/{{N}}/`. Then check with
  `git status --porcelain` that no `zz_qa_*` file is left in the tree.
- Time box: 20 minutes per scenario, not counting the time a beekeeper check
  runs. On timeout, mark the scenario `BLOCKED` and write what you saw.

## Method

For each scenario, in order:

1. Run the steps. Record the exact commands and the relevant output, trimmed
   to at most 20 lines. Put long logs in
   `{{OUT}}/results/logs/{{N}}-s<k>.log`.
2. Compare the result with the plan's Expect. Mark the scenario `PASS`,
   `FAIL`, or `BLOCKED` (could not run: missing env, setup error, timeout).
3. On `FAIL`:
   - Run it once more. If the second run passes, mark it `FLAKY` and keep both
     outputs. A `FLAKY` scenario counts as a failure.
   - Run the same scenario on BASE. Write `regression` if only HEAD fails,
     `pre-existing` if BASE fails too, or `unknown` if you could not run BASE.
   - Find the root cause and point to `file:line` at HEAD. If you cannot find
     it, write "root cause not found" and list what you ruled out.
   - Propose a fix in one or two sentences. Add a diff sketch of at most 15
     lines when the fix is local. Do not apply the fix to the tree.
   - Rate the severity with the rubric below.

If the problem is in the test itself (wrong curl flag, missing fixture), it is
not an issue. Fix the step, run it again, and note the change.

## Severity rubric

- **Critical**: stored data is lost or corrupted; funds are lost or the
  on-chain state is wrong; a remote peer or API client can crash or hang the
  node; or BASE and HEAD nodes split the network.
- **High**: a main path gives a wrong result with no workaround (upload,
  download, push, pull, retrieval, stamping, staking, redistribution), or an
  upgrade from BASE fails.
- **Medium**: an edge case gives a wrong result, a workaround exists, or a
  resource leak shows only under load or over a long time.
- **Low**: a log, metric, error message, status code detail, or doc is wrong,
  and the behavior is otherwise correct.

## Output

Write `{{OUT}}/results/{{N}}.md` in exactly this shape:

```markdown
## #<N>: <title> · <PASS|FAIL|PARTIAL|BLOCKED>

Env: <local|cluster|chain> · context `<ctx or ->` · HEAD `<sha>` · run <YYYY-MM-DD>

| # | Scenario | Result | Evidence |
|---|---|---|---|
| s1 | <name> | PASS | <command or log path> |

### s<k>: <scenario name> · <PASS|FAIL|FLAKY|BLOCKED>
Command: `<exact command>`
Output:
    <at most 20 lines>

<For FAIL and FLAKY only:>
Origin: <regression|pre-existing|unknown> · Severity: <Critical|High|Medium|Low>
Root cause: <file:line, then one or two sentences>
Suggested fix: <one or two sentences>
    <optional diff sketch, at most 15 lines>

### New tests
<paths under results/tests/<N>/ and what each one asserts, or "none">

### Not run (suggested)
<risks seen during the run that the plan does not cover, or "none">
```

PR verdict: `PASS` if all scenarios pass. `FAIL` if any scenario fails or is
flaky. `PARTIAL` if none fails but some are `BLOCKED`. `BLOCKED` if no
scenario ran.

## Return value

Return **only** these tab-separated lines: one `TESTED` line, then one `ISSUE`
line per failed or flaky scenario.

```
TESTED	<N>	<PASS|FAIL|PARTIAL|BLOCKED>	<passed>/<total>	<one-line summary>
ISSUE	<N>	<Critical|High|Medium|Low>	<regression|pre-existing|unknown>	<one-line finding>	<one-line suggested fix>	<file:line or ->	results/<N>.md#s<k>
```

Return no other text. The orchestrator parses these lines.
