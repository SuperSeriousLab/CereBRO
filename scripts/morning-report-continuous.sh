#!/bin/bash
# Generates morning report from continuous cold queue processing.
# Reads feeder logs, SLR cold queue metrics, and consolidation results.
set -uo pipefail

CEREBRO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DATE=$(date +%Y-%m-%d)
LOG_DIR="$CEREBRO_DIR/data/generation/logs/$DATE"
REPORT="$LOG_DIR/morning-report.md"
SLR_ENDPOINT="${SLR_ENDPOINT:-http://192.168.14.69:8081}"

mkdir -p "$LOG_DIR"

# Collect data
FEEDER_LOG="$LOG_DIR/feeder.log"
SUBMITTED=$(grep -c "Submitted" "$FEEDER_LOG" 2>/dev/null || echo 0)
TOTAL_SUBMITTED=$(grep "Submitted" "$FEEDER_LOG" 2>/dev/null | grep -oE '[0-9]+/' | tr -d '/' | paste -sd+ | bc 2>/dev/null || echo 0)

# SLR metrics
SLR_STATUS=$(curl -s --max-time 5 "$SLR_ENDPOINT/v1/queue/status" 2>/dev/null)
COLD_QUEUE_SIZE=$(echo "$SLR_STATUS" | jq -r '.cold_queue.size // "?"' 2>/dev/null)
IS_IDLE=$(echo "$SLR_STATUS" | jq -r '.idle // "?"' 2>/dev/null)

# SLR Prometheus cold metrics
SLR_METRICS=$(curl -s --max-time 5 "http://192.168.14.69:8080/metrics" 2>/dev/null)
COLD_PROCESSED=$(echo "$SLR_METRICS" | grep "slr_cold_processed_total" | grep -oE '[0-9]+' | tail -1 || echo "?")
COLD_PREEMPTED=$(echo "$SLR_METRICS" | grep "slr_cold_preempted_total" | grep -oE '[0-9]+' | tail -1 || echo "?")
CALLBACK_SUCCESS=$(echo "$SLR_METRICS" | grep "slr_cold_callback_success" | grep -oE '[0-9]+' | tail -1 || echo "?")
CALLBACK_FAIL=$(echo "$SLR_METRICS" | grep "slr_cold_callback_fail" | grep -oE '[0-9]+' | tail -1 || echo "?")

# Corpus growth
CORPUS_SIZE=$(wc -l < "$CEREBRO_DIR/data/corpus/consolidated.ndjson" 2>/dev/null || echo 0)

# Write report
cat > "$REPORT" << EOF
# CereBRO Continuous Loop — $DATE

## Cold Queue (24h window)
- Feeder runs: $SUBMITTED
- Conversations submitted: $TOTAL_SUBMITTED
- Cold queue depth: $COLD_QUEUE_SIZE
- Ollama idle: $IS_IDLE

## SLR Cold Processing
- Items processed: $COLD_PROCESSED
- Callbacks succeeded: $CALLBACK_SUCCESS
- Callbacks failed: $CALLBACK_FAIL
- Preempted by hot traffic: $COLD_PREEMPTED

## Corpus
- Consolidated entries: $CORPUS_SIZE

## Services
$(curl -sf --max-time 2 http://10.70.70.14:11434/api/tags > /dev/null 2>&1 && echo "- Ollama: UP" || echo "- Ollama: DOWN")
$(curl -sf --max-time 2 http://192.168.14.69:8080/health > /dev/null 2>&1 && echo "- SLR: UP" || echo "- SLR: DOWN")
$(curl -sf --max-time 2 http://192.168.14.65:8090/health > /dev/null 2>&1 && echo "- Sophrim: UP" || echo "- Sophrim: DOWN")
EOF

cat "$REPORT"
