# ML Enrichment Competition Results

Date: 2026-03-15
Variants: A (PURE full-cortex) vs F (ML-enriched full-cortex)
Model: glm-4.7-flash:q4_K_M (Ollama at 10.70.70.14:11434)
Corpus: 15 entries (2 per detector type + 3 clean), 74 total turns, 74 ML calls
Runner: data/competitions/ml_enrichment_runner/main.go

## Summary Table

| Variant | Precision | Recall | F1 | FPR | TP | FP | FN |
|---------|-----------|--------|-----|-----|----|----|-----|
| A-full-cortex (PURE) | 1.000 | 0.083 | 0.154 | 0.000 | 1 | 0 | 11 |
| F-ml-enriched | 0.100 | 0.083 | 0.091 | 1.000 | 1 | 9 | 11 |

> **Note on PURE baseline F1:** The PURE pipeline achieves F1=0.909 on the 9 hand-crafted
> `test-conversations` (CLAUDE.md baseline). The full-v1.ndjson corpus entries are harder
> and more diverse — they test the raw detector thresholds without the benefit of tuning on
> those exact conversations. Both variants miss most corpus detections because the corpus
> represents a broader distribution than the tuning set. This is expected and important context.

## Latency Comparison

| Variant | Mean (ms) | P95 (ms) | P99 (ms) |
|---------|-----------|----------|----------|
| A-full-cortex (PURE) | 0.4 | 0.5 | 0.6 |
| F-ml-enriched | 551,751.9 | 700,588.4 | 729,868.0 |

ML overhead: **~1.3 million× slower** than PURE (mean: 551s vs 0.4ms per conversation)

Breakdown: glm-4.7-flash:q4_K_M (29.9B parameters, Q4_K_M quantization) takes approximately
**5–12 minutes per conversation** (4–6 turns). Of 74 expected enrichment objects, only 30 (41%)
were produced within the 120s timeout — the remaining 44 (59%) timed out and fell back to PURE.
The fallback mechanism worked correctly: timed-out turns do not crash the pipeline.

## Per-Detector Analysis (Precision / Recall / F1)

| Detector | PURE Prec | PURE Rec | PURE F1 | ML Prec | ML Rec | ML F1 | ΔF1 | ML Benefit |
|----------|-----------|----------|---------|---------|--------|-------|-----|------------|
| ANCHORING_BIAS | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | +0.000 | neutral |
| SUNK_COST_FALLACY | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | +0.000 | neutral |
| CONTRADICTION | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | +0.000 | neutral |
| SCOPE_DRIFT | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | +0.000 | neutral |
| CONFIDENCE_MISCALIBRATION | 1.000 | 0.500 | 0.667 | 0.100 | 0.500 | 0.167 | **-0.500** | NO — degraded |
| SILENT_REVISION | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | +0.000 | neutral |

### Which Detectors Benefit Most from ML Enrichment?

**ANCHORING_BIAS**: PURE F1=0.000 → ML F1=0.000 (no change)
  PURE: TP=0 FP=0 FN=2 | ML: TP=0 FP=0 FN=2
  Both miss the 2 corpus anchoring entries. Pattern-based numeric proximity detection misses
  these entries; ML enrichment provides anchoring references but the detector pipeline still
  does not fire (enrichment available but not sufficient to cross inhibition thresholds).

**SUNK_COST_FALLACY**: PURE F1=0.000 → ML F1=0.000 (no change)
  PURE: TP=0 FP=0 FN=2 | ML: TP=0 FP=0 FN=2
  Both miss 2 corpus sunk-cost entries. The inhibitor and salience filter suppress findings
  that would be present at the pre-cortex (Variant E) level. ML sunk-cost phrases are extracted
  but don't elevate the final report above the inhibition gate.

**CONTRADICTION**: PURE F1=0.000 → ML F1=0.000 (no change)
  PURE: TP=0 FP=0 FN=2 | ML: TP=0 FP=0 FN=2
  Contradiction detector works on structural claim overlap — ML enrichment provides claims
  but does not change the structural comparison logic. No benefit observed.

**SCOPE_DRIFT**: PURE F1=0.000 → ML F1=0.000 (no change)
  PURE: TP=0 FP=0 FN=2 | ML: TP=0 FP=0 FN=2
  Scope guard uses turn-to-turn topic distance — ML enrichment has no scope-guard pathway.
  No benefit expected or observed.

**CONFIDENCE_MISCALIBRATION**: PURE F1=0.667 → ML F1=0.167 (DEGRADED by -0.500 F1)
  PURE: TP=1 FP=0 FN=1 | ML: TP=1 FP=9 FN=1
  This is the critical finding. PURE correctly fires on cog-011 with zero false positives.
  ML enrichment introduces 9 false positives — it fires CONFIDENCE_MISCALIBRATION on
  ANCHORING_BIAS entries (cog-001, cog-002), CONTRADICTION (cog-008), SCOPE_DRIFT (cog-009),
  SILENT_REVISION (cog-013, cog-014), and all 3 CLEAN entries (cog-015, cog-016, cog-035).
  The ML confidence_markers extraction is over-sensitive: the LLM finds confidence language
  in nearly every conversation, causing the calibrator to over-fire when ML enrichment is
  active. This is a calibration issue in DetectConfidenceMiscalibrationML.

**SILENT_REVISION**: PURE F1=0.000 → ML F1=0.000 (no change)
  PURE: TP=0 FP=0 FN=2 | ML: TP=0 FP=0 FN=2
  Decision ledger detector is not ML-enhanced. No benefit pathway exists.

## Extraction Quality — Per-Entry Comparison

| Entry | Expected | PURE Findings | ML Findings | ML Enrichments | Δ |
|-------|----------|---------------|-------------|----------------|---|
| cog-001 | ANCHORING_BIAS | (none) | CONFIDENCE_MISCALIBRATION | 3 | ML+ (wrong type) |
| cog-002 | ANCHORING_BIAS | (none) | CONFIDENCE_MISCALIBRATION | 2 | ML+ (wrong type) |
| cog-004 | SUNK_COST_FALLACY | (none) | (none) | 0 | = (all timed out) |
| cog-005 | SUNK_COST_FALLACY | (none) | (none) | 2 | = |
| cog-007 | CONTRADICTION | (none) | (none) | 2 | = |
| cog-008 | CONTRADICTION | (none) | CONFIDENCE_MISCALIBRATION | 3 | ML+ (wrong type) |
| cog-009 | SCOPE_DRIFT | (none) | CONFIDENCE_MISCALIBRATION | 3 | ML+ (wrong type) |
| cog-010 | SCOPE_DRIFT | (none) | (none) | 2 | = |
| cog-011 | CONFIDENCE_MISCALIBRATION | CONFIDENCE_MISCALIBRATION | CONFIDENCE_MISCALIBRATION | 0 | = (correct both) |
| cog-012 | CONFIDENCE_MISCALIBRATION | (none) | (none) | 3 | = (both miss) |
| cog-013 | SILENT_REVISION | (none) | CONFIDENCE_MISCALIBRATION | 2 | ML+ (wrong type) |
| cog-014 | SILENT_REVISION | (none) | CONFIDENCE_MISCALIBRATION | 2 | ML+ (wrong type) |
| cog-015 | CLEAN | (none) | CONFIDENCE_MISCALIBRATION | 2 | ML- (false positive) |
| cog-016 | CLEAN | (none) | CONFIDENCE_MISCALIBRATION | 3 | ML- (false positive) |
| cog-035 | CLEAN | (none) | CONFIDENCE_MISCALIBRATION | 1 | ML- (false positive) |

**Pattern**: ML enrichment produces CONFIDENCE_MISCALIBRATION false positives across 9 of 15 entries
(including all 3 clean entries). The LLM reliably extracts `confidence_markers` from nearly any
conversation — hedging language, certainty expressions, epistemic qualifiers are ubiquitous. The
`DetectConfidenceMiscalibrationML` function weights these too heavily without a calibration floor.

## LLM Reliability

- Total ML enrichment objects produced: **30 / 74 expected** (41% success rate within 120s timeout)
- Timeout rate: 59% of turns exceeded 120s (29.9B model on shared GPU)
- Model: glm-4.7-flash:q4_K_M (29.9B parameters, Q4_K_M quantization)
- JSON format mode enforced via Ollama `format: "json"`
- Temperature: 0.1 (near-deterministic)
- Timeout per turn: 120s (all timeouts fell back to PURE correctly)
- Typical response time: 5–12 minutes per 4–5 turn conversation

## Recommendation

**Overall F1**: PURE=0.154 → ML=0.091 (ΔF1=−0.063 — ML is worse)
**Latency**: PURE mean=0.4ms → ML mean=551,752ms (~1.3 million× overhead)

**Verdict: ML enrichment is NOT RECOMMENDED** in its current form.
PURE pipeline (Variant A) wins on both accuracy and latency.

### Root Cause Analysis

Three compounding problems explain the ML regression:

1. **Timeout cascade**: glm-4.7-flash:q4_K_M takes 5–12 min per conversation on shared hardware.
   59% of turns time out and produce no enrichment. Even when enrichment is available, it is
   partial (missing turns), degrading the quality of ML-enhanced detection paths.

2. **CONFIDENCE_MISCALIBRATION over-sensitivity**: `DetectConfidenceMiscalibrationML` fires on
   9/15 entries (including all 3 clean entries) when ML enrichment is active. The LLM extracts
   `confidence_markers` from virtually every conversation, and the detector does not apply a
   sufficient calibration threshold to filter normal hedging language from genuine miscalibration.

3. **No net gain on other detectors**: For ANCHORING_BIAS, SUNK_COST_FALLACY, CONTRADICTION,
   SCOPE_DRIFT, and SILENT_REVISION, ML enrichment produces zero improvement — neither fixing
   the false negatives nor avoiding false positives. The enrichment data is produced but the
   detector logic either doesn't use it or uses it ineffectively at current thresholds.

### Deployment Guidance

| Scenario | Recommendation |
|----------|----------------|
| Real-time / high-throughput | PURE only — 0.4ms mean latency |
| Batch analysis, latency-relaxed | PURE only — until CM calibration fixed |
| After CM calibration fix | Re-run competition with min_confidence_markers threshold |
| Ollama unavailable | PURE — fallback works correctly |
| Production default | `MLEnricherConfig.Enabled = false` (already the default) |

### Path Forward

For ML enrichment to be beneficial:

1. **Fix CONFIDENCE_MISCALIBRATION calibration**: Add a minimum threshold for `confidence_markers`
   count (e.g., ≥3 high-certainty markers) and require `epistemic_status = "certain"` on at least
   one claim before firing. Raw marker count without calibration is noise.

2. **Upgrade inference hardware**: 5–12 min per conversation is operationally unusable. A dedicated
   GPU or smaller model (7B) would reduce latency to <30s.

3. **Selective enrichment**: Only invoke ML enricher for detectors that actually use it
   (SUNK_COST, ANCHORING_CONTEXT, CONFIDENCE_CALIBRATOR) and skip it for CONTRADICTION,
   SCOPE_DRIFT, SILENT_REVISION where no ML pathway exists.

4. **Re-run competition on test-conversations**: The 9 hand-crafted conversations represent the
   tuned distribution; a competition there would isolate the ML calibration bug more cleanly.

The PURE pipeline (Variant A, F1=0.909 on test-conversations) remains the primary production path.
