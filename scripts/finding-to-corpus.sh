#!/bin/bash
# Converts a verified finding JSON to corpus NDJSON entry
# Usage: finding-to-corpus.sh finding.json >> data/corpus/consolidated.ndjson
set -euo pipefail

FINDING="${1:?Usage: finding-to-corpus.sh FINDING.json}"

if [[ ! -f "$FINDING" ]]; then
    echo "ERROR: file not found: $FINDING" >&2
    exit 1
fi

# Read finding fields
ENTRY_ID=$(jq -r ".entry_id // \"consolidated-$(date +%s)\"" "$FINDING")
FINDING_TYPE=$(jq -r '.finding_type // "UNKNOWN"' "$FINDING")
CONV_TEXT=$(jq -r '.conversation_text // ""' "$FINDING")

# Build minimal ConversationSnapshot from conversation_text
# Split text into turns by newline (rough approximation)
TURNS=$(echo "$CONV_TEXT" | jq -R -s 'split("\n") | to_entries | map(select(.value | length > 0) | {turn_number: (.key + 1), speaker: (if .key % 2 == 0 then "A" else "B" end), raw_text: .value})')
TURN_COUNT=$(echo "$TURNS" | jq 'length')

# Output NDJSON entry
jq -nc \
    --arg id "$ENTRY_ID" \
    --arg ft "$FINDING_TYPE" \
    --argjson turns "$TURNS" \
    --argjson count "$TURN_COUNT" \
    '{
        entry_id: $id,
        input: {turns: $turns, objective: "consolidated from nightly loop", total_turns: $count},
        expected: [{finding_type: $ft}]
    }'
