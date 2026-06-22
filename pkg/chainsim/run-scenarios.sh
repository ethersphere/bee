#!/usr/bin/env bash
# Run chainsim scenario tests and collect all artifacts into a timestamped directory.
#
# Usage:
#   ./pkg/chainsim/run-scenarios.sh                    # run all scenarios, then highload tests
#   ./pkg/chainsim/run-scenarios.sh TestScenario_Happy # run one scenario by name
#   ./pkg/chainsim/run-scenarios.sh TestHighload       # run highload tests only
#
# Scenario output: pkg/chainsim/scenario-results-<timestamp>/
# Highload output:  pkg/chainsim/highload-results-<timestamp>/
#   (STRESS_DURATION=15m, HIGHLOAD_DUMP=60s, HIGHLOAD_TIMEOUT=4h by default)
#
# Each scenario produces:
#   events.jsonl  — every structured log event (blocks, tx accept/reject/execute, retry broadcasts)
#   state.json    — SimChain snapshot at scenario end
#   stats.json    — cumulative SimChain counters
#   result.json   — scenario outcome (hash, receipt, error, duration)

set -euo pipefail

REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
cd "${REPO_ROOT}"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
OUTDIR="${REPO_ROOT}/pkg/chainsim/scenario-results-${TIMESTAMP}"
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
if 'txs' in r:
    txs = r['txs']
    ok = sum(1 for x in txs if x.get('has_receipt') and x.get('status') == 1)
    fail = sum(1 for x in txs if x.get('error'))
    print(f'{ok} ok, {fail} errors, {len(txs)} total, {r.get(\"blocks_total\",0)} blocks')
elif isinstance(r, list):
    ok = sum(1 for x in r if x.get('has_receipt') and x.get('status') == 1)
    fail = sum(1 for x in r if x.get('error'))
    print(f'{ok} ok, {fail} errors, {len(r)} total')
else:
    s = 'OK' if r.get('has_receipt') and r.get('status') == 1 else r.get('error', 'unknown')
    print(f'{s} ({r.get(\"duration_ms\", 0)}ms, {r.get(\"blocks_total\", 0)} blocks)')
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

# --- Highload tests ---
if [ "${RUN_FILTER}" = "TestScenario" ] || echo "${RUN_FILTER}" | grep -q 'Highload'; then
    HL_OUTDIR="${REPO_ROOT}/pkg/chainsim/highload-results-${TIMESTAMP}"
    mkdir -p "${HL_OUTDIR}"
    echo ""
    echo "=== highload tests ==="
    echo "  output: ${HL_OUTDIR}"

    HL_TIMEOUT="${HIGHLOAD_TIMEOUT:-4h}"
    HL_DURATION="${STRESS_DURATION:-15m}"
    HL_DUMP="${HIGHLOAD_DUMP:-60s}"
    echo "  duration per test: ${HL_DURATION}"
    echo "  dump interval:     ${HL_DUMP}"
    set +e
    STRESS_DURATION="${HL_DURATION}" HIGHLOAD_DUMP="${HL_DUMP}" \
    SCENARIO_OUTPUT_DIR="${HL_OUTDIR}" go test -tags=scenario -v -count=1 \
        -timeout="${HL_TIMEOUT}" -run 'TestHighload' \
        ./pkg/chainsim/... 2>&1 | tee "${HL_OUTDIR}/test-output.log"
    HL_EXIT=$?
    set -e

    echo "  highload exit: ${HL_EXIT}"
    echo "  artifacts: ${HL_OUTDIR}/"
    if [ ${HL_EXIT} -ne 0 ]; then
        TEST_EXIT=${HL_EXIT}
    fi
fi

exit ${TEST_EXIT}
