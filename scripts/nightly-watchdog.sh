#!/bin/bash
set -euo pipefail

DATE=$(date +%Y-%m-%d)
CEREBRO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
LOG_DIR="$CEREBRO_DIR/data/generation/logs/$DATE"
CONV_DIR="$CEREBRO_DIR/data/generation/output/$DATE"
ALERT_FILE="$LOG_DIR/watchdog-alert.md"
PROBLEMS=0

check() {
    local name="$1" condition="$2" message="$3"
    if ! eval "$condition"; then
        echo "PROBLEM: $name — $message" >> "$ALERT_FILE"
        ((PROBLEMS++)) || true
    fi
}

mkdir -p "$LOG_DIR"
rm -f "$ALERT_FILE"

check "Run exists" "[[ -d '$LOG_DIR' ]]" "No nightly directory. Cron may not have fired."
check "Report exists" "[[ -f '$LOG_DIR/morning-report.md' ]]" "No morning report. Loop may have crashed."
check "Conversations generated" "[[ \$(ls '$CONV_DIR'/*.json 2>/dev/null | wc -l) -gt 0 ]]" "Zero conversations."
check "No stale lock" "[[ ! -f /tmp/cerebro-nightly.lock ]] || ! kill -0 \$(cat /tmp/cerebro-nightly.lock 2>/dev/null) 2>/dev/null" "Lock file exists but process dead."
check "Ollama" "curl -sf --max-time 5 http://10.70.70.14:11434/api/tags > /dev/null 2>&1" "Ollama unreachable."
check "SLR" "curl -sf --max-time 5 http://192.168.14.69:8080/health > /dev/null 2>&1" "SLR unreachable."
check "Sophrim" "curl -sf --max-time 5 http://192.168.14.65:8090/health > /dev/null 2>&1" "Sophrim unreachable."

if [[ -f "$LOG_DIR/phases.log" ]]; then
    FAILURES=$(grep -c "FAILED\|ABORT" "$LOG_DIR/phases.log" 2>/dev/null; true)
    check "Phase failures" "[[ $FAILURES -lt 2 ]]" "$FAILURES phases failed."
fi

if [[ "$PROBLEMS" -gt 0 ]]; then
    echo "WATCHDOG: $PROBLEMS problems found" >> "$ALERT_FILE"
    cp "$ALERT_FILE" "$CEREBRO_DIR/data/generation/logs/LATEST_ALERT.md"
    echo "WATCHDOG: $PROBLEMS problems. See $ALERT_FILE"
    curl -s -X POST http://192.168.14.68:9746/inject \
        -H "Content-Type: application/json" \
        -d "{\"text\": \"CereBRO nightly: nightly-watchdog detected $PROBLEMS problem(s) — see $ALERT_FILE\"}" &
    exit 1
else
    echo "$(date +%H:%M:%S) all clear" >> "$CEREBRO_DIR/data/generation/logs/watchdog.log"
    rm -f "$CEREBRO_DIR/data/generation/logs/LATEST_ALERT.md"
    echo "WATCHDOG: all clear"
    exit 0
fi
