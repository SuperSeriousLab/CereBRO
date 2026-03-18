#!/bin/bash
# Feeds conversation generation requests to SLR's cold queue.
# Designed to run every 15 minutes via cron.
# Only submits if cold queue has room (< half capacity).
set -uo pipefail

CEREBRO_DIR="/home/js/eidos/CereBRO"
SLR_ENDPOINT="${SLR_ENDPOINT:-http://192.168.14.69:8081}"
PROMPT_DIR="$CEREBRO_DIR/data/generation/prompts"
BATCH_SIZE=5  # conversations per invocation
CALLBACK_URL="${CALLBACK_URL:-http://192.168.14.65:8090/cold-result}"  # Sophrim or local endpoint
DATE=$(date +%Y-%m-%d)
LOG_DIR="$CEREBRO_DIR/data/generation/logs/$DATE"
mkdir -p "$LOG_DIR"

# Check queue status
STATUS=$(curl -s --max-time 5 "$SLR_ENDPOINT/v1/queue/status" 2>/dev/null)
if [[ -z "$STATUS" ]]; then
    echo "$(date +%H:%M:%S) SLR unreachable — skipping" >> "$LOG_DIR/feeder.log"
    exit 0
fi

QUEUE_SIZE=$(echo "$STATUS" | jq -r '.cold_queue.size // 0')
QUEUE_CAP=$(echo "$STATUS" | jq -r '.cold_queue.capacity // 50')
HALF_CAP=$((QUEUE_CAP / 2))

if [[ "$QUEUE_SIZE" -ge "$HALF_CAP" ]]; then
    echo "$(date +%H:%M:%S) Cold queue $QUEUE_SIZE/$QUEUE_CAP (>50%) — skipping" >> "$LOG_DIR/feeder.log"
    exit 0
fi

# Select random prompts
PROMPTS=($(ls "$PROMPT_DIR"/*.txt 2>/dev/null | shuf | head -$BATCH_SIZE))
if [[ ${#PROMPTS[@]} -eq 0 ]]; then
    echo "$(date +%H:%M:%S) No prompts found — skipping" >> "$LOG_DIR/feeder.log"
    exit 0
fi

SUBMITTED=0
for PROMPT_FILE in "${PROMPTS[@]}"; do
    PROMPT_NAME=$(basename "$PROMPT_FILE" .txt)
    SYSTEM_PROMPT=$(cat "$PROMPT_FILE")
    JOB_ID="conv-$DATE-$PROMPT_NAME-$(date +%s)"

    RESPONSE=$(curl -s --max-time 10 -X POST "$SLR_ENDPOINT/v1/queue/backfill" \
      -H "Content-Type: application/json" \
      -d "$(jq -n \
        --arg sys "$SYSTEM_PROMPT" \
        --arg cb "$CALLBACK_URL" \
        --arg jid "$JOB_ID" \
        '{
          "request": {
            "model": "auto",
            "messages": [
              {"role": "system", "content": $sys},
              {"role": "user", "content": "Generate a realistic multi-turn conversation (8-15 turns) between two participants. Format as JSON: {\"turns\": [{\"speaker\": \"A\", \"text\": \"...\"}, ...], \"topic\": \"brief topic description\"}. Make it natural."}
            ],
            "temperature": 0.8,
            "max_tokens": 4096
          },
          "callback_url": $cb,
          "metadata": {"source": "cerebro-feeder", "job_id": $jid, "job_type": "conversation_generation"}
        }')" 2>/dev/null)

    STATUS_CODE=$(echo "$RESPONSE" | jq -r '.queue_position // "error"' 2>/dev/null)
    if [[ "$STATUS_CODE" != "error" ]]; then
        ((SUBMITTED++))
    fi
done

echo "$(date +%H:%M:%S) Submitted $SUBMITTED/$BATCH_SIZE to cold queue (was $QUEUE_SIZE/$QUEUE_CAP)" >> "$LOG_DIR/feeder.log"
