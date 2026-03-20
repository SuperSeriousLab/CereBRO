#!/bin/bash
# CereBRO Nightly Loop Orchestrator
#
# 8-phase automated pipeline:
#   1. Lock         — prevent overlapping runs
#   2. Preflight    — verify all services are up
#   3. Generate     — produce synthetic conversations via SLR
#   4. Convert      — convert raw JSON to ConversationSnapshot format
#   5. Process      — run cerebro-batch through the cognitive pipeline
#   6. Verify       — Grok cross-verification of high-confidence findings
#   7. Consolidate  — append verified findings to corpus
#   8. Forge sweep  — parameter sweep if corpus grew
#      + Sophrim ingest if forge ran
#   9. Morning report — write readable markdown summary
#
# Usage: nightly-loop.sh [COUNT]
#   COUNT: number of conversations to generate (default: 50)
#
# Environment:
#   SLR_ENDPOINT      — override SLR base URL (default: http://192.168.14.69:8081)
#   SOPHRIM_ENDPOINT  — override Sophrim URL  (default: http://192.168.14.65:8090)
#
# Does NOT activate cron — call this script from cron externally.

set -uo pipefail

# Load API keys not available in cron env
[[ -f /home/js/.config/consult/keys ]] && source /home/js/.config/consult/keys

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
COUNT="${1:-50}"
DATE=$(date +%Y-%m-%d)
CEREBRO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$CEREBRO_DIR/data/generation/logs/$DATE"
LOCK_FILE="/tmp/cerebro-nightly.lock"
GO_BIN="/home/js/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.7.linux-amd64/bin/go"

# Per-run directories
CONV_DIR="$CEREBRO_DIR/data/generation/output/$DATE"
SNAP_DIR="$CEREBRO_DIR/data/generation/snapshots/$DATE"
FIND_DIR="$CEREBRO_DIR/data/generation/findings/$DATE"
CAND_DIR="$CEREBRO_DIR/data/generation/candidates/$DATE"
CORPUS_FILE="$CEREBRO_DIR/data/corpus/consolidated.ndjson"

PHASES_LOG="$LOG_DIR/phases.log"
REPORT_FILE="$LOG_DIR/morning-report.md"

# Phase counters (populated during run)
PHASE_ERRORS=0
CONVS_GENERATED=0
SNAPS_CONVERTED=0
FINDINGS_TOTAL=0
CANDIDATES_TOTAL=0
VERIFIED_COUNT=0
CONSOLIDATED_COUNT=0
CORPUS_BEFORE=0
CORPUS_AFTER=0
FORGE_RAN=0
SOPHRIM_INGESTED=0
ABORT_REASON=""

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
log_phase() {
    local phase="$1"
    local msg="$2"
    local ts
    ts=$(date '+%Y-%m-%dT%H:%M:%S')
    echo "[$ts] [$phase] $msg" | tee -a "$PHASES_LOG"
}

log_error() {
    local phase="$1"
    local msg="$2"
    log_phase "$phase" "ERROR: $msg"
    ((PHASE_ERRORS++)) || true
}

pts_notify() {
  local msg="$1"
  curl -s -X POST http://192.168.14.68:9746/inject \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg text "$msg" '{text: $text}')" > /dev/null 2>&1 &
}

# Write the morning report — always called, even on failure.
write_morning_report() {
    local end_time
    end_time=$(date '+%Y-%m-%dT%H:%M:%S')
    local status="SUCCESS"
    [[ -n "$ABORT_REASON" ]] && status="ABORTED ($ABORT_REASON)"
    [[ "$PHASE_ERRORS" -gt 0 && -z "$ABORT_REASON" ]] && status="PARTIAL ($PHASE_ERRORS phase errors)"

    mkdir -p "$LOG_DIR"
    cat > "$REPORT_FILE" <<EOF
# CereBRO Nightly Loop — $DATE

**Status:** $status
**Finished:** $end_time

## Pipeline Summary

| Phase | Result |
|-------|--------|
| Conversations generated | $CONVS_GENERATED / $COUNT |
| Snapshots converted     | $SNAPS_CONVERTED |
| Pipeline findings       | $FINDINGS_TOTAL |
| Consolidation candidates| $CANDIDATES_TOTAL |
| Verified (AGREE)        | $VERIFIED_COUNT |
| Consolidated to corpus  | $CONSOLIDATED_COUNT |
| Corpus size before      | $CORPUS_BEFORE entries |
| Corpus size after       | $CORPUS_AFTER entries |
| Forge sweep ran         | $([ "$FORGE_RAN" -eq 1 ] && echo "yes" || echo "no") |
| Sophrim ingested        | $([ "$SOPHRIM_INGESTED" -eq 1 ] && echo "yes" || echo "no") |
| Phase errors            | $PHASE_ERRORS |

## Logs

- Phases: \`$PHASES_LOG\`
- Full log: \`$LOG_DIR/nightly.log\`

EOF
    log_phase "REPORT" "Morning report written to $REPORT_FILE"
}

# Cleanup lock on exit.
cleanup() {
    write_morning_report
    [[ -f "$LOCK_FILE" ]] && rm -f "$LOCK_FILE"
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Phase 1: Lock — prevent overlapping runs
# ---------------------------------------------------------------------------
mkdir -p "$LOG_DIR"
exec >> "$LOG_DIR/nightly.log" 2>&1

log_phase "LOCK" "Starting nightly loop (COUNT=$COUNT, DATE=$DATE)"

if [[ -f "$LOCK_FILE" ]]; then
    EXISTING_PID=$(cat "$LOCK_FILE" 2>/dev/null || echo "")
    if [[ -n "$EXISTING_PID" ]] && kill -0 "$EXISTING_PID" 2>/dev/null; then
        ABORT_REASON="lock held by PID $EXISTING_PID"
        log_phase "LOCK" "ABORT: $ABORT_REASON"
        exit 1
    else
        log_phase "LOCK" "Stale lock file (PID $EXISTING_PID gone) — removing"
        rm -f "$LOCK_FILE"
    fi
fi
echo "$$" > "$LOCK_FILE"
log_phase "LOCK" "Lock acquired (PID $$)"

# ---------------------------------------------------------------------------
# Phase 2: Preflight — verify all services
# ---------------------------------------------------------------------------
log_phase "PREFLIGHT" "Checking services..."
if ! "$CEREBRO_DIR/scripts/preflight-check.sh" 2>&1 | tee -a "$PHASES_LOG"; then
    ABORT_REASON="preflight failed"
    log_phase "PREFLIGHT" "ABORT: one or more services unavailable"
    pts_notify "CereBRO nightly: preflight-check failed — one or more required services unreachable (SLR/Ollama/Sophrim)"
    exit 1
fi
log_phase "PREFLIGHT" "All services healthy"

# ---------------------------------------------------------------------------
# Phase 3: Generate conversations (skipped if cold queue already populated dir)
# ---------------------------------------------------------------------------
mkdir -p "$CONV_DIR"
EXISTING_CONVS=$(ls "$CONV_DIR"/*.json 2>/dev/null | wc -l)

if [[ "$EXISTING_CONVS" -gt 0 ]]; then
    # Cold queue feeder has already deposited conversations — skip generation.
    CONVS_GENERATED=$EXISTING_CONVS
    log_phase "GENERATE" "Skipping generation — found $CONVS_GENERATED existing conversations from cold queue"
else
    # Fallback: no cold queue output yet — generate directly (backward compat).
    log_phase "GENERATE" "No cold queue output found — generating $COUNT conversations directly..."
    if "$CEREBRO_DIR/scripts/generate-conversations.sh" "$COUNT" "$CONV_DIR" 2>&1 | tee -a "$PHASES_LOG"; then
        CONVS_GENERATED=$(ls "$CONV_DIR"/*.json 2>/dev/null | wc -l)
        log_phase "GENERATE" "Generated: $CONVS_GENERATED files in $CONV_DIR"
    else
        log_error "GENERATE" "generate-conversations.sh exited non-zero"
        CONVS_GENERATED=$(ls "$CONV_DIR"/*.json 2>/dev/null | wc -l)
    fi
fi

if [[ "$CONVS_GENERATED" -eq 0 ]]; then
    ABORT_REASON="zero conversations available"
    log_phase "GENERATE" "ABORT: nothing to process"
    pts_notify "CereBRO nightly: Phase 3 found 0 conversations. Cold queue feeder and direct generation both produced nothing."
    exit 1
fi

# ---------------------------------------------------------------------------
# Phase 4: Convert + Process
# ---------------------------------------------------------------------------
log_phase "CONVERT" "Converting $CONVS_GENERATED conversations to snapshots..."
mkdir -p "$SNAP_DIR"

if "$CEREBRO_DIR/scripts/convert-to-snapshots.sh" "$CONV_DIR" "$SNAP_DIR" 2>&1 | tee -a "$PHASES_LOG"; then
    SNAPS_CONVERTED=$(ls "$SNAP_DIR"/*.json 2>/dev/null | wc -l)
    log_phase "CONVERT" "Converted: $SNAPS_CONVERTED snapshots"
else
    log_error "CONVERT" "convert-to-snapshots.sh exited non-zero"
    SNAPS_CONVERTED=$(ls "$SNAP_DIR"/*.json 2>/dev/null | wc -l)
fi

if [[ "$SNAPS_CONVERTED" -eq 0 ]]; then
    log_error "CONVERT" "No snapshots produced — skipping pipeline"
else
    # Build cerebro-batch if missing or stale
    BATCH_BIN="$CEREBRO_DIR/bin/cerebro-batch"
    if [[ ! -f "$BATCH_BIN" ]]; then
        log_phase "BUILD" "Building cerebro-batch..."
        if (cd "$CEREBRO_DIR" && "$GO_BIN" build -o "$BATCH_BIN" ./cmd/cerebro-batch/) 2>&1 | tee -a "$PHASES_LOG"; then
            log_phase "BUILD" "cerebro-batch built OK"
        else
            log_error "BUILD" "cerebro-batch build failed — skipping pipeline"
            pts_notify "CereBRO nightly: cerebro-batch build failed — pipeline skipped for $DATE"
            SNAPS_CONVERTED=0
        fi
    fi
fi

if [[ "$SNAPS_CONVERTED" -gt 0 ]]; then
    log_phase "PROCESS" "Running cerebro-batch on $SNAPS_CONVERTED snapshots..."
    mkdir -p "$FIND_DIR" "$CAND_DIR"
    SOPHRIM_URL="${SOPHRIM_ENDPOINT:-http://192.168.14.65:8090}"
    if "$CEREBRO_DIR/bin/cerebro-batch" \
        --input "$SNAP_DIR" \
        --findings "$FIND_DIR" \
        --candidates "$CAND_DIR" \
        --sophrim "$SOPHRIM_URL" 2>&1 | tee -a "$PHASES_LOG"; then
        FINDINGS_TOTAL=$(ls "$FIND_DIR"/*.json 2>/dev/null | grep -v summary.json | wc -l)
        CANDIDATES_TOTAL=$(ls "$CAND_DIR"/*.json 2>/dev/null | wc -l)
        log_phase "PROCESS" "Findings: $FINDINGS_TOTAL, Candidates: $CANDIDATES_TOTAL"
    else
        log_error "PROCESS" "cerebro-batch exited non-zero"
        FINDINGS_TOTAL=$(ls "$FIND_DIR"/*.json 2>/dev/null | grep -v summary.json | wc -l)
        CANDIDATES_TOTAL=$(ls "$CAND_DIR"/*.json 2>/dev/null | wc -l)
        pts_notify "CereBRO nightly: Phase 5 batch processing failed (cerebro-batch exited non-zero). Findings: $FINDINGS_TOTAL, Candidates: $CANDIDATES_TOTAL."
    fi
fi

# ---------------------------------------------------------------------------
# Phase 5: Verify findings with Grok via SLR
# ---------------------------------------------------------------------------
if [[ "$CANDIDATES_TOTAL" -gt 0 ]]; then
    log_phase "VERIFY" "Verifying $CANDIDATES_TOTAL candidates..."
    if "$CEREBRO_DIR/scripts/verify-findings.sh" "$DATE" 2>&1 | tee -a "$PHASES_LOG"; then
        VERIFIED_DIR="$CEREBRO_DIR/data/generation/verified/$DATE"
        VERIFIED_COUNT=$(ls "$VERIFIED_DIR"/*.json 2>/dev/null | wc -l)
        log_phase "VERIFY" "Verified (AGREE): $VERIFIED_COUNT"
    else
        log_error "VERIFY" "verify-findings.sh exited non-zero"
        VERIFIED_DIR="$CEREBRO_DIR/data/generation/verified/$DATE"
        VERIFIED_COUNT=$(ls "$VERIFIED_DIR"/*.json 2>/dev/null | wc -l)
        pts_notify "CereBRO nightly: Phase 6 Grok verification failed (verify-findings.sh exited non-zero). Verified so far: $VERIFIED_COUNT."
    fi
else
    log_phase "VERIFY" "No candidates to verify — skipping"
fi

# ---------------------------------------------------------------------------
# Phase 6: Consolidate verified findings into corpus
# ---------------------------------------------------------------------------
VERIFIED_DIR="$CEREBRO_DIR/data/generation/verified/$DATE"
CORPUS_BEFORE=$(wc -l < "$CORPUS_FILE" 2>/dev/null || echo 0)

if [[ "$VERIFIED_COUNT" -gt 0 ]]; then
    log_phase "CONSOLIDATE" "Consolidating $VERIFIED_COUNT verified findings..."
    mkdir -p "$(dirname "$CORPUS_FILE")"
    for finding in "$VERIFIED_DIR"/*.json; do
        [[ -f "$finding" ]] || continue
        if "$CEREBRO_DIR/scripts/finding-to-corpus.sh" "$finding" >> "$CORPUS_FILE" 2>>"$PHASES_LOG"; then
            ((CONSOLIDATED_COUNT++)) || true
        else
            log_error "CONSOLIDATE" "finding-to-corpus.sh failed on $finding"
        fi
    done
    CORPUS_AFTER=$(wc -l < "$CORPUS_FILE" 2>/dev/null || echo 0)
    log_phase "CONSOLIDATE" "Consolidated: $CONSOLIDATED_COUNT. Corpus: $CORPUS_BEFORE → $CORPUS_AFTER entries"
else
    CORPUS_AFTER=$CORPUS_BEFORE
    log_phase "CONSOLIDATE" "No verified findings to consolidate — skipping"
fi

# ---------------------------------------------------------------------------
# Phase 7: Forge sweep — run if corpus grew
# ---------------------------------------------------------------------------
CORPUS_GROWTH=$(( CORPUS_AFTER - CORPUS_BEFORE ))
if [[ "$CORPUS_GROWTH" -gt 0 ]]; then
    log_phase "FORGE" "Corpus grew by $CORPUS_GROWTH entries — running forge sweep..."
    if (cd "$CEREBRO_DIR" && "$CEREBRO_DIR/scripts/forge-sweep.sh" "$CORPUS_FILE") 2>&1 | tee -a "$PHASES_LOG"; then
        FORGE_RAN=1
        log_phase "FORGE" "Forge sweep complete"
    else
        log_error "FORGE" "forge-sweep.sh exited non-zero"
        FORGE_RAN=0
        pts_notify "CereBRO nightly: Phase 8 forge sweep regression detected (forge-sweep.sh exited non-zero). Corpus growth: $CORPUS_GROWTH entries."
    fi
else
    log_phase "FORGE" "Corpus unchanged — skipping forge sweep"
fi

# ---------------------------------------------------------------------------
# Phase 8: Sophrim ingest — if forge ran (Connection C)
# ---------------------------------------------------------------------------
if [[ "$FORGE_RAN" -eq 1 ]]; then
    SOPHRIM_URL="${SOPHRIM_ENDPOINT:-http://192.168.14.65:8090}"
    log_phase "SOPHRIM" "Ingesting updated corpus into Sophrim at $SOPHRIM_URL..."
    INGEST_PAYLOAD=$(jq -nc \
        --arg corpus "$CORPUS_FILE" \
        --arg date "$DATE" \
        '{"corpus_path": $corpus, "source": "nightly-loop", "date": $date}')
    HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
        --max-time 30 \
        -X POST "$SOPHRIM_URL/ingest" \
        -H "Content-Type: application/json" \
        -d "$INGEST_PAYLOAD" 2>>"$PHASES_LOG" || echo "000")
    if [[ "$HTTP_STATUS" =~ ^2 ]]; then
        SOPHRIM_INGESTED=1
        log_phase "SOPHRIM" "Ingest accepted (HTTP $HTTP_STATUS)"
    else
        log_error "SOPHRIM" "Ingest returned HTTP $HTTP_STATUS — Sophrim may not support /ingest yet"
    fi
fi

log_phase "DONE" "Nightly loop complete. Errors: $PHASE_ERRORS"
