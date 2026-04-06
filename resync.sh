#!/bin/bash

set -e

BEE_CMD="./bee start --config=config.yaml --skip-postage-snapshot=true"
ATTEMPTS=20
LOG_DIR="./restart-test-logs"
TRIGGER_REGEX='processEvents: dirty flag set, processing events.*"count"=[1-9]'
KILL_TIMEOUT=900  # 15 minutes

mkdir -p "$LOG_DIR"

echo "Starting dirty shutdown reproduction test"
echo "Attempts: $ATTEMPTS"
echo "Trigger regex: '$TRIGGER_REGEX'"
echo "Kill timeout: ${KILL_TIMEOUT}s"
echo "Logs directory: $LOG_DIR"
echo "==========================================="

for i in $(seq 1 "$ATTEMPTS"); do
    LOG_FILE="$LOG_DIR/attempt-${i}.log"
    echo ""
    echo "=== Attempt $i/$ATTEMPTS ==="

    > "$LOG_FILE"
    $BEE_CMD >> "$LOG_FILE" 2>&1 &
    PID=$!
    echo "Started bee with PID=$PID, logging to $LOG_FILE"

    FOUND=false
    SECONDS=0
    while [ "$SECONDS" -lt "$KILL_TIMEOUT" ]; do
        if ! kill -0 "$PID" 2>/dev/null; then
            echo "WARNING: PID=$PID exited on its own after ${SECONDS}s"
            break
        fi

        if grep -qE "$TRIGGER_REGEX" "$LOG_FILE" 2>/dev/null; then
            FOUND=true
            echo "TRIGGER detected after ${SECONDS}s — killing immediately with SIGKILL"
            kill -9 "$PID" 2>/dev/null || true
            break
        fi

        sleep 0.2

        if (( SECONDS % 30 == 0 && SECONDS > 0 )); then
            echo "  ... waiting for trigger (${SECONDS}s / ${KILL_TIMEOUT}s)"
        fi
    done

    if [ "$FOUND" = false ] && kill -0 "$PID" 2>/dev/null; then
        echo "TIMEOUT: trigger not seen in ${KILL_TIMEOUT}s, killing anyway"
        kill -9 "$PID" 2>/dev/null || true
    fi

    wait "$PID" 2>/dev/null || true

    if [ "$FOUND" = true ]; then
        echo "Attempt $i: killed during TransactionStart window (events > 0)"
    fi

    sleep 1
done

echo ""
echo "==========================================="
echo "Final run: checking for dirty shutdown..."
echo "==========================================="

FINAL_LOG="$LOG_DIR/final-check.log"
> "$FINAL_LOG"
$BEE_CMD >> "$FINAL_LOG" 2>&1 &
FINAL_PID=$!
echo "Started final bee with PID=$FINAL_PID"

SECONDS=0
while [ "$SECONDS" -lt 120 ]; do
    if grep -qE "dirty shutdown detected|batch service: Start called" "$FINAL_LOG" 2>/dev/null; then
        break
    fi
    sleep 1
done

sleep 5

echo ""
echo "=== Results ==="
echo ""

if grep -q "dirty shutdown detected" "$FINAL_LOG"; then
    echo "*** BUG REPRODUCED: dirty shutdown detected on final start ***"
    grep "dirty shutdown\|resync chain data\|batch store has been reset\|Start called" "$FINAL_LOG" | head -10
else
    echo "No dirty shutdown detected on final start"
    echo "Debug info from final log:"
    grep "Start called\|TransactionStart\|TransactionEnd\|dirty" "$FINAL_LOG" | head -10
fi

echo ""
echo "--- Scanning all attempt logs ---"
echo ""

DIRTY_COUNT=0
TRIGGER_COUNT=0
for f in "$LOG_DIR"/attempt-*.log; do
    fname=$(basename "$f")
    if grep -q "dirty shutdown detected" "$f"; then
        DIRTY_COUNT=$((DIRTY_COUNT + 1))
        echo "  DIRTY SHUTDOWN in $fname"
    fi
    if grep -qE "$TRIGGER_REGEX" "$f"; then
        TRIGGER_COUNT=$((TRIGGER_COUNT + 1))
    fi
done

echo ""
echo "Summary: $TRIGGER_COUNT/$ATTEMPTS attempts hit TransactionStart (events>0), $DIRTY_COUNT detected dirty shutdown on next start"

echo ""
echo "Cleaning up final bee process (PID=$FINAL_PID)..."
if kill -0 "$FINAL_PID" 2>/dev/null; then
    kill "$FINAL_PID"
    wait "$FINAL_PID" 2>/dev/null || true
fi

echo "Done. All logs saved in $LOG_DIR/"
