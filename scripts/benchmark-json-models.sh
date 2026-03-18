#!/bin/bash
# benchmark-json-models.sh — Head-to-head JSON generation quality test.
# Tests gemma3:4b vs glm-4.7-flash:q4_K_M (and optionally qwen3:8b)
# for structured JSON output success rate across 6 corpus categories.
#
# Usage: ./scripts/benchmark-json-models.sh
set -uo pipefail

OLLAMA_ENDPOINT="${OLLAMA_ENDPOINT:-http://10.70.70.14:11434}"
ATTEMPTS_PER_CAT=5
DELAY=2

MODELS=(
    "gemma3:4b"
    "glm-4.7-flash:q4_K_M"
    "qwen3:8b"
)

declare -a CATEGORIES=(
    "sycophancy"
    "cathedral"
    "anchoring"
    "counter_evidence_depletion"
    "confidence_miscalibration"
    "healthy"
)

declare -A PROMPTS

PROMPTS[sycophancy]='Generate a realistic multi-turn conversation (6-10 turns) between a user and an AI assistant where the assistant demonstrates sycophancy. The topic should be technical (software architecture, system design, code review, or data analysis). The assistant should excessively agree with the user even when the user makes questionable claims, validate ideas without critical evaluation, avoid pointing out problems or risks, use flattering language, and when the user changes direction, immediately agree with the new direction too. The sycophancy should emerge gradually. Format as JSON: {"turns": [{"speaker": "user", "text": "..."}, {"speaker": "assistant", "text": "..."}, ...], "topic": "brief topic description"}. Make it natural, not cartoonish. Respond with valid JSON only, no markdown.'

PROMPTS[cathedral]='Generate a realistic multi-turn conversation (6-10 turns) between a user and an AI assistant where the assistant demonstrates cathedral complexity building. The topic should be technical (software architecture, system design, or infrastructure planning). The assistant should start with a simple solution but progressively add layers of abstraction, introduce unnecessary frameworks and patterns, make each response more complex than the last, use impressive-sounding but vaguely justified architectural terms, and ignore simpler alternatives. Format as JSON: {"turns": [{"speaker": "user", "text": "..."}, {"speaker": "assistant", "text": "..."}, ...], "topic": "brief topic description"}. Make it realistic. Respond with valid JSON only, no markdown.'

PROMPTS[anchoring]='Generate a realistic multi-turn conversation (6-10 turns) between a user and an AI assistant where the assistant demonstrates anchoring bias. The topic should be technical (estimation, capacity planning, performance benchmarking, or cost analysis). The assistant should latch onto the first number or metric mentioned, make all subsequent estimates orbit around that initial anchor, adjust only slightly when presented with contradicting evidence, and dismiss information that suggests the anchor is wrong. Format as JSON: {"turns": [{"speaker": "user", "text": "..."}, {"speaker": "assistant", "text": "..."}, ...], "topic": "brief topic description"}. Make it natural with realistic technical details. Respond with valid JSON only, no markdown.'

PROMPTS[counter_evidence_depletion]='Generate a realistic multi-turn conversation (6-10 turns) between a user and an AI assistant where the assistant fails to seek or present counter-evidence. The topic should be a technical decision (choosing a technology, evaluating a design, or analyzing a problem). The assistant should present only supporting evidence for a position, never proactively mention downsides or alternatives, minimize risks when asked directly, and build a case that sounds thorough but is entirely one-sided. Format as JSON: {"turns": [{"speaker": "user", "text": "..."}, {"speaker": "assistant", "text": "..."}, ...], "topic": "brief topic description"}. Respond with valid JSON only, no markdown.'

PROMPTS[confidence_miscalibration]='Generate a realistic multi-turn conversation (6-10 turns) between a user and an AI assistant where the assistant demonstrates confidence miscalibration. The topic should be technical (debugging, performance analysis, system diagnosis, or architecture evaluation). The assistant should express high certainty with weak or incomplete evidence, make definitive claims based on limited information, fail to acknowledge uncertainty when genuinely ambiguous, and present speculation as established fact. Format as JSON: {"turns": [{"speaker": "user", "text": "..."}, {"speaker": "assistant", "text": "..."}, ...], "topic": "brief topic description"}. Respond with valid JSON only, no markdown.'

PROMPTS[healthy]='Generate a realistic multi-turn conversation (6-10 turns) between a user and an AI assistant that demonstrates balanced, healthy reasoning. The topic should be technical (software architecture, system design, code review, or data analysis). The assistant should provide honest balanced feedback including both pros and cons, challenge questionable assumptions respectfully, acknowledge uncertainty when appropriate, present counter-evidence and alternative perspectives, adjust position when presented with new evidence, and express calibrated confidence. This is a HEALTHY conversation with no reasoning pathologies. Format as JSON: {"turns": [{"speaker": "user", "text": "..."}, {"speaker": "assistant", "text": "..."}, ...], "topic": "brief topic description"}. Respond with valid JSON only, no markdown.'

# Result tracking: model__category -> "attempts valid"
declare -A RESULTS

extract_and_validate_json() {
    local content="$1"
    # Strip markdown fences
    local clean
    clean=$(echo "$content" | sed '/^```/d')
    # Extract first JSON object
    local json_block
    json_block=$(echo "$clean" | python3 -c "
import sys, re
text = sys.stdin.read()
depth = 0
start = -1
for i, c in enumerate(text):
    if c == '{':
        if depth == 0:
            start = i
        depth += 1
    elif c == '}':
        depth -= 1
        if depth == 0 and start >= 0:
            print(text[start:i+1])
            break
" 2>/dev/null)

    if [[ -z "$json_block" ]]; then
        echo "NO_JSON_BLOCK"
        return 1
    fi

    if ! echo "$json_block" | jq '.' > /dev/null 2>&1; then
        echo "INVALID_JSON"
        return 1
    fi

    local turns
    turns=$(echo "$json_block" | jq -r '.turns // empty' 2>/dev/null)
    if [[ -z "$turns" || "$turns" == "null" ]]; then
        echo "NO_TURNS"
        return 1
    fi

    local turn_count
    turn_count=$(echo "$json_block" | jq '.turns | length' 2>/dev/null)
    if [[ -z "$turn_count" ]] || [[ "$turn_count" -lt 6 ]]; then
        echo "TOO_FEW_TURNS($turn_count)"
        return 1
    fi

    echo "OK($turn_count turns)"
    return 0
}

echo "=== CereBRO JSON Generation Benchmark ==="
echo "Endpoint: $OLLAMA_ENDPOINT"
echo "Attempts per category: $ATTEMPTS_PER_CAT"
echo "Models: ${MODELS[*]}"
echo ""

for model in "${MODELS[@]}"; do
    echo "--- Testing model: $model ---"
    for cat in "${CATEGORIES[@]}"; do
        key="${model}__${cat}"
        attempts=0
        valid=0
        prompt="${PROMPTS[$cat]}"

        for i in $(seq 1 "$ATTEMPTS_PER_CAT"); do
            attempts=$((attempts + 1))
            RESPONSE=$(curl -s --max-time 180 -X POST "$OLLAMA_ENDPOINT/api/generate" \
              -H "Content-Type: application/json" \
              -d "$(jq -n \
                --arg m "$model" \
                --arg p "$prompt" \
                '{model: $m, prompt: $p, stream: false, options: {temperature: 0.9, num_predict: 8192}}')" 2>/dev/null)

            CONTENT=$(echo "$RESPONSE" | jq -r '.response // empty' 2>/dev/null)

            if [[ -z "$CONTENT" ]]; then
                echo "  [$cat] attempt $i: EMPTY_RESPONSE"
                sleep "$DELAY"
                continue
            fi

            result=$(extract_and_validate_json "$CONTENT")
            status=$?
            if [[ $status -eq 0 ]]; then
                valid=$((valid + 1))
                echo "  [$cat] attempt $i: VALID $result"
            else
                echo "  [$cat] attempt $i: FAIL ($result)"
            fi
            sleep "$DELAY"
        done

        RESULTS["$key"]="$attempts $valid"
        echo "  [$cat] => $valid/$attempts valid"
    done
    echo ""
done

# Summary table
echo ""
echo "======================================================"
echo "=== RESULTS SUMMARY ==="
echo "======================================================"
printf "%-32s %-30s %8s %8s %12s\n" "Model" "Category" "Attempts" "Valid" "SuccessRate"
echo "----------------------------------------------------------------------"

declare -A MODEL_TOTALS_ATTEMPTS
declare -A MODEL_TOTALS_VALID

for model in "${MODELS[@]}"; do
    MODEL_TOTALS_ATTEMPTS["$model"]=0
    MODEL_TOTALS_VALID["$model"]=0
    for cat in "${CATEGORIES[@]}"; do
        key="${model}__${cat}"
        read -r att val <<< "${RESULTS[$key]:-0 0}"
        rate=0
        if [[ "$att" -gt 0 ]]; then
            rate=$(echo "scale=1; $val * 100 / $att" | bc)
        fi
        printf "%-32s %-30s %8d %8d %11s%%\n" "$model" "$cat" "$att" "$val" "$rate"
        MODEL_TOTALS_ATTEMPTS["$model"]=$(( MODEL_TOTALS_ATTEMPTS["$model"] + att ))
        MODEL_TOTALS_VALID["$model"]=$(( MODEL_TOTALS_VALID["$model"] + val ))
    done
    echo ""
done

echo "======================================================"
echo "=== MODEL TOTALS ==="
echo "======================================================"
printf "%-32s %8s %8s %12s\n" "Model" "Attempts" "Valid" "SuccessRate"
echo "----------------------------------------------------------------------"

WINNER=""
WINNER_RATE=0

for model in "${MODELS[@]}"; do
    att="${MODEL_TOTALS_ATTEMPTS[$model]}"
    val="${MODEL_TOTALS_VALID[$model]}"
    rate=0
    if [[ "$att" -gt 0 ]]; then
        rate=$(echo "scale=1; $val * 100 / $att" | bc)
    fi
    printf "%-32s %8d %8d %11s%%\n" "$model" "$att" "$val" "$rate"
    # Track winner (integer comparison using truncated rate)
    rate_int=$(echo "$rate" | cut -d. -f1)
    if [[ "$rate_int" -gt "$WINNER_RATE" ]]; then
        WINNER_RATE=$rate_int
        WINNER="$model"
    fi
done

echo ""
echo "Winner: $WINNER (${WINNER_RATE}% success rate)"

# GLM check
GLM_ATT="${MODEL_TOTALS_ATTEMPTS[glm-4.7-flash:q4_K_M]:-0}"
GLM_VAL="${MODEL_TOTALS_VALID[glm-4.7-flash:q4_K_M]:-0}"
GLM_RATE=0
if [[ "$GLM_ATT" -gt 0 ]]; then
    GLM_RATE=$(echo "scale=1; $GLM_VAL * 100 / $GLM_ATT" | bc)
fi
echo "GLM-4.7-flash total: $GLM_VAL/$GLM_ATT ($GLM_RATE%)"

GLM_RATE_INT=$(echo "$GLM_RATE" | cut -d. -f1)
if [[ "$GLM_RATE_INT" -gt 75 ]]; then
    echo ""
    echo "DECISION: GLM-4.7-flash exceeds 75% threshold. Recommend switching default model."
else
    echo ""
    echo "DECISION: GLM-4.7-flash does NOT exceed 75% threshold ($GLM_RATE%). No script change."
fi
