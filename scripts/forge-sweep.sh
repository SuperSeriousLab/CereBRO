#!/bin/bash
# Simple parameter sweep for CereBRO's forge-eval
# Searches for optimal Scope Guard + Tier 2 detector parameters
# Output: ranked results sorted by F1

set -e
CORPUS="${1:-data/corpus/full-v2.ndjson}"
FORGE_EVAL="./forge-eval"
RESULTS_FILE="/tmp/forge-sweep-results.tsv"

if [ ! -f "$FORGE_EVAL" ]; then
  echo "Building forge-eval..."
  /home/js/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.7.linux-amd64/bin/go build -o "$FORGE_EVAL" ./cmd/forge-eval/
fi

echo -e "F1\tPrecision\tRecall\tTP\tFP\tFN\tParams" > "$RESULTS_FILE"

# Parameter grid
DRIFT_THRESHOLDS="0.60 0.65 0.70 0.75 0.79"
SUSTAINED_TURNS="2 3 4 5 6"
ANCHOR_THRESHOLDS="0.20 0.25 0.30 0.35"
ORBIT_THRESHOLDS="0.45 0.50 0.55 0.60"
MIN_CITATIONS="2 3 4"

COUNT=0
TOTAL=$((5 * 5 * 4 * 4 * 3))
echo "Running $TOTAL parameter combinations..."

for dt in $DRIFT_THRESHOLDS; do
  for st in $SUSTAINED_TURNS; do
    for at in $ANCHOR_THRESHOLDS; do
      for ot in $ORBIT_THRESHOLDS; do
        for mc in $MIN_CITATIONS; do
          COUNT=$((COUNT + 1))
          PARAMS="{\"drift_threshold\":\"$dt\",\"sustained_turns\":\"$st\",\"anchor_threshold\":\"$at\",\"orbit_threshold\":\"$ot\",\"min_citations\":\"$mc\"}"
          RESULT=$($FORGE_EVAL --corpus="$CORPUS" --params="$PARAMS" 2>/dev/null)
          F1=$(echo "$RESULT" | jq -r '.traits.f1')
          P=$(echo "$RESULT" | jq -r '.traits.precision')
          R=$(echo "$RESULT" | jq -r '.traits.recall')
          TP=$(echo "$RESULT" | jq -r '.traits.tp')
          FP=$(echo "$RESULT" | jq -r '.traits.fp')
          FN=$(echo "$RESULT" | jq -r '.traits.fn')
          echo -e "$F1\t$P\t$R\t$TP\t$FP\t$FN\tdt=$dt st=$st at=$at ot=$ot mc=$mc" >> "$RESULTS_FILE"
          if [ $((COUNT % 50)) -eq 0 ]; then
            echo "  [$COUNT/$TOTAL] F1=$F1 (dt=$dt st=$st at=$at ot=$ot mc=$mc)"
          fi
        done
      done
    done
  done
done

echo ""
echo "=== TOP 10 CONFIGURATIONS ==="
sort -t$'\t' -k1 -rn "$RESULTS_FILE" | head -11

echo ""
echo "=== BASELINE (default params) ==="
$FORGE_EVAL --corpus="$CORPUS" --params='{}' 2>/dev/null | jq .

echo ""
echo "Results saved to $RESULTS_FILE"
