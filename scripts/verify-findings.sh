#!/bin/bash
set -euo pipefail

CEREBRO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DATE=${1:-$(date +%Y-%m-%d)}
FINDINGS_DIR="$CEREBRO_DIR/data/generation/candidates/$DATE"
VERIFIED_DIR="$CEREBRO_DIR/data/generation/verified/$DATE"
DISAGREED_DIR="$CEREBRO_DIR/data/generation/disagreements/$DATE"
UNCERTAIN_DIR="$CEREBRO_DIR/data/generation/uncertain/$DATE"
SLR_ENDPOINT="${SLR_ENDPOINT:-http://192.168.14.69:8081}"
PROMPT_TEMPLATE="$CEREBRO_DIR/data/generation/verify-prompt.txt"

mkdir -p "$VERIFIED_DIR" "$DISAGREED_DIR" "$UNCERTAIN_DIR"

if [[ ! -d "$FINDINGS_DIR" ]] || [[ -z "$(ls "$FINDINGS_DIR"/*.json 2>/dev/null)" ]]; then
    echo "No findings to verify in $FINDINGS_DIR"
    exit 0
fi

VERIFIED=0; DISAGREED=0; UNCERTAIN=0; ERRORS=0

for finding in "$FINDINGS_DIR"/*.json; do
    [[ -f "$finding" ]] || continue
    BASENAME=$(basename "$finding")

    # Skip already-processed findings
    [[ -f "$VERIFIED_DIR/$BASENAME" ]] && { ((VERIFIED++)) || true; continue; }
    [[ -f "$DISAGREED_DIR/$BASENAME" ]] && { ((DISAGREED++)) || true; continue; }
    [[ -f "$UNCERTAIN_DIR/$BASENAME" ]] && { ((UNCERTAIN++)) || true; continue; }

    CONV_TEXT=$(jq -r '.conversation_text // "N/A"' "$finding" 2>/dev/null)
    FINDING_TYPE=$(jq -r '.finding_type // "UNKNOWN"' "$finding" 2>/dev/null)
    CONFIDENCE=$(jq -r '.confidence // "N/A"' "$finding" 2>/dev/null)
    EXPLANATION=$(jq -r '.explanation // "N/A"' "$finding" 2>/dev/null)

    # Build prompt from template
    PROMPT=$(cat "$PROMPT_TEMPLATE")
    PROMPT="${PROMPT//\{\{CONVERSATION_TEXT\}\}/$CONV_TEXT}"
    PROMPT="${PROMPT//\{\{FINDING_TYPE\}\}/$FINDING_TYPE}"
    PROMPT="${PROMPT//\{\{CONFIDENCE\}\}/$CONFIDENCE}"
    PROMPT="${PROMPT//\{\{EXPLANATION\}\}/$EXPLANATION}"

    # Send to Grok via SLR (cx=0.95 forces quality tier)
    RESPONSE=$(curl -s --max-time 60 -X POST "$SLR_ENDPOINT/v1/chat/completions" \
      -H "Content-Type: application/json" \
      -d "$(jq -n --arg prompt "$PROMPT" '{
          "model": "auto:cx=0.95",
          "messages": [{"role": "user", "content": $prompt}],
          "temperature": 0.0,
          "max_tokens": 200
        }')" 2>/dev/null)

    CONTENT=$(echo "$RESPONSE" | jq -r '.choices[0].message.content // empty' 2>/dev/null)

    if [[ -z "$CONTENT" ]]; then
        echo "WARN: empty response for $BASENAME" >&2
        ((ERRORS++)) || true
        continue
    fi

    # Extract verdict from first line
    VERDICT=$(echo "$CONTENT" | head -1 | tr '[:lower:]' '[:upper:]' | grep -oE 'AGREE|DISAGREE|UNCERTAIN' | head -1)

    # Add verification metadata to the finding
    VERIFIED_FINDING=$(jq --arg verdict "${VERDICT:-UNCERTAIN}" --arg reason "$CONTENT" \
        '. + {grok_verdict: $verdict, grok_reason: $reason}' "$finding")

    case "$VERDICT" in
        AGREE)    echo "$VERIFIED_FINDING" > "$VERIFIED_DIR/$BASENAME"; ((VERIFIED++)) || true ;;
        DISAGREE) echo "$VERIFIED_FINDING" > "$DISAGREED_DIR/$BASENAME"; ((DISAGREED++)) || true ;;
        *)        echo "$VERIFIED_FINDING" > "$UNCERTAIN_DIR/$BASENAME"; ((UNCERTAIN++)) || true ;;
    esac

    sleep 1  # Rate limiting
done

echo "Verified: $VERIFIED"
echo "Disagreed: $DISAGREED"
echo "Uncertain: $UNCERTAIN"
echo "Errors: $ERRORS"
