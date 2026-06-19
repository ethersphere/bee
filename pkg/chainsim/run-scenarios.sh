#!/usr/bin/env bash
# Run chainsim scenario tests and collect all artifacts into a timestamped directory.
#
# Usage:
#   ./pkg/chainsim/run-scenarios.sh                    # run all scenarios
#   ./pkg/chainsim/run-scenarios.sh TestScenario_Happy # run one scenario by name
#
# Output goes to: pkg/chainsim/scenario-results-<timestamp>/
#
# Each scenario produces:
#   events.jsonl  — every structured log event (blocks, tx accept/reject/execute, retry broadcasts)
#   state.json    — SimChain snapshot at scenario end
#   stats.json    — cumulative SimChain counters
#   result.json   — scenario outcome (hash, receipt, error, duration)

set -euo pipefail

cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
OUTDIR="pkg/chainsim/scenario-results-${TIMESTAMP}"
mkdir -p "${OUTDIR}"

RUN_FILTER="${1:-TestScenario}"

echo "=== chainsim scenario runner ==="
echo "  output:  ${OUTDIR}"
echo "  filter:  ${RUN_FILTER}"
echo "  started: $(date -Iseconds)"
echo ""

export SCENARIO_OUTPUT_DIR="${OUTDIR}"

set +e
go test -tags=scenario -v -count=1 -timeout=300s \
    -run "${RUN_FILTER}" \
    ./pkg/chainsim/... 2>&1 | tee "${OUTDIR}/test-output.log"
TEST_EXIT=$?
set -e

echo ""
echo "=== collecting results ==="

# Build a summary JSON from all result.json files
SUMMARY="${OUTDIR}/summary.json"
echo "[" > "${SUMMARY}"
FIRST=true
for f in "${OUTDIR}"/*/result.json; do
    [ -f "$f" ] || continue
    if [ "$FIRST" = true ]; then
        FIRST=false
    else
        echo "," >> "${SUMMARY}"
    fi
    cat "$f" >> "${SUMMARY}"
done
echo "]" >> "${SUMMARY}"

# Count events per scenario
echo ""
echo "=== scenario results ==="
for d in "${OUTDIR}"/*/; do
    [ -d "$d" ] || continue
    scenario=$(basename "$d")
    events=$(wc -l < "${d}events.jsonl" 2>/dev/null || echo 0)
    if [ -f "${d}result.json" ]; then
        status=$(python3 -c "
import json, sys
r = json.load(open('${d}result.json'))
if isinstance(r, list):
    ok = sum(1 for x in r if x.get('has_receipt') and x.get('status') == 1)
    fail = sum(1 for x in r if x.get('error'))
    print(f'{ok} ok, {fail} errors, {len(r)} total')
else:
    s = 'OK' if r.get('has_receipt') and r.get('status') == 1 else r.get('error', 'unknown')
    print(f'{s} ({r.get(\"duration_ms\", 0)}ms)')
" 2>/dev/null || echo "?")
    else
        status="no result"
    fi
    printf "  %-40s %5s events  %s\n" "$scenario" "$events" "$status"
done

echo ""
echo "=== done ==="
echo "  exit code: ${TEST_EXIT}"
echo "  artifacts: ${OUTDIR}/"
echo ""
echo "To analyze: provide the entire ${OUTDIR}/ directory."
echo "Key files:"
echo "  ${OUTDIR}/summary.json          — all scenario outcomes"
echo "  ${OUTDIR}/test-output.log       — full go test output"
echo "  ${OUTDIR}/*/events.jsonl        — structured event logs"
echo "  ${OUTDIR}/*/state.json          — chain state snapshots"

exit ${TEST_EXIT}
