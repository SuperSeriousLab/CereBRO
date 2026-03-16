#!/bin/bash
set -uo pipefail

COUNT=${1:-50}
OUTPUT_DIR=${2:-"data/generation/output/$(date +%Y-%m-%d)"}
PROMPT_DIR="data/generation/prompts"
SLR_ENDPOINT="${SLR_ENDPOINT:-http://192.168.14.69:8081}"

mkdir -p "$OUTPUT_DIR"

PROMPTS=($(ls "$PROMPT_DIR"/*.txt | shuf))
PROMPT_COUNT=${#PROMPTS[@]}

GENERATED=0
for i in $(seq 1 "$COUNT"); do
    PROMPT_FILE="${PROMPTS[$(( (i - 1) % PROMPT_COUNT ))]}"
    PROMPT_NAME=$(basename "$PROMPT_FILE" .txt)
    OUT_FILE="$OUTPUT_DIR/${PROMPT_NAME}-$(printf '%03d' $i).json"

    # Resume support
    [[ -f "$OUT_FILE" ]] && { GENERATED=$((GENERATED + 1)); continue; }

    SYSTEM_PROMPT=$(cat "$PROMPT_FILE")

    RESPONSE=$(curl -s --max-time 180 -X POST "$SLR_ENDPOINT/v1/chat/completions" \
      -H "Content-Type: application/json" \
      -d "$(jq -n \
        --arg sys "$SYSTEM_PROMPT" \
        --arg usr "Generate a realistic multi-turn conversation (8-15 turns) between two participants. Format as JSON: {\"turns\": [{\"speaker\": \"A\", \"text\": \"...\"}, ...], \"topic\": \"brief topic description\"}. Make it natural — include hesitations, tangents, and imperfect reasoning." \
        '{
          "model": "auto",
          "messages": [
            {"role": "system", "content": $sys},
            {"role": "user", "content": $usr}
          ],
          "temperature": 0.8,
          "max_tokens": 4096
        }')" 2>/dev/null)

    # Extract the content from OpenAI-compatible response
    CONTENT=$(echo "$RESPONSE" | jq -r '.choices[0].message.content // empty' 2>/dev/null)

    if [[ -n "$CONTENT" ]]; then
        # Strip markdown code fences if present
        CLEAN=$(echo "$CONTENT" | sed '/^```/d')
        # Validate JSON before writing — skip truncated responses
        if echo "$CLEAN" | jq '.' > /dev/null 2>&1; then
            echo "$CLEAN" > "$OUT_FILE"
            GENERATED=$((GENERATED + 1))
        else
            echo "WARN: invalid/truncated JSON for $PROMPT_NAME-$i — skipping" >&2
        fi
    else
        echo "WARN: empty response for $PROMPT_NAME-$i" >&2
    fi

    sleep 2
done

echo "Generated: $GENERATED/$COUNT in $OUTPUT_DIR"
