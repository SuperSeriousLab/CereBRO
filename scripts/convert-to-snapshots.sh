#!/bin/bash
set -euo pipefail

INPUT_DIR="${1:?Usage: convert-to-snapshots.sh INPUT_DIR OUTPUT_DIR}"
OUTPUT_DIR="${2:?Usage: convert-to-snapshots.sh INPUT_DIR OUTPUT_DIR}"

mkdir -p "$OUTPUT_DIR"
CONVERTED=0
SKIPPED=0

for f in "$INPUT_DIR"/*.json; do
    [[ -f "$f" ]] || continue
    BASENAME=$(basename "$f" .json)
    OUT="$OUTPUT_DIR/${BASENAME}.json"

    # Try to parse as conversation JSON
    # The LLM output may have the JSON embedded in markdown code blocks
    CONTENT=$(cat "$f")

    # Strip markdown code fences if present
    CONTENT=$(echo "$CONTENT" | sed -n '/^```/,/^```/p' | sed '/^```/d' || echo "$CONTENT")
    # If no code fences, use raw content
    [[ -z "$CONTENT" ]] && CONTENT=$(cat "$f")

    # Extract turns array
    TURNS=$(echo "$CONTENT" | jq -c '.turns // empty' 2>/dev/null)
    if [[ -z "$TURNS" ]] || [[ "$TURNS" == "null" ]]; then
        echo "SKIP: $BASENAME (no turns array)" >&2
        ((SKIPPED++))
        continue
    fi

    TOPIC=$(echo "$CONTENT" | jq -r '.topic // "unknown"' 2>/dev/null)
    TURN_COUNT=$(echo "$TURNS" | jq 'length' 2>/dev/null)

    # Convert to ConversationSnapshot NDJSON format
    jq -n \
        --arg id "$BASENAME" \
        --arg topic "$TOPIC" \
        --argjson turns "$TURNS" \
        --argjson count "$TURN_COUNT" \
        '{
            entry_id: $id,
            input: {
                turns: [$turns[] | {
                    turn_number: (. as $t | ($turns | to_entries[] | select(.value == $t) | .key) + 1),
                    speaker: (.speaker // "unknown"),
                    raw_text: (.text // .content // "")
                }],
                objective: $topic,
                total_turns: $count
            },
            expected: []
        }' > "$OUT" 2>/dev/null

    if [[ $? -eq 0 ]]; then
        ((CONVERTED++))
    else
        echo "SKIP: $BASENAME (jq conversion failed)" >&2
        ((SKIPPED++))
    fi
done

echo "Converted: $CONVERTED, Skipped: $SKIPPED"
