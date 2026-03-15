# ML Enrichment Competition Results

Date: 2026-03-15
Variants: A (PURE full-cortex) vs F (ML-enriched full-cortex)
Model: glm-4.7-flash:q4_K_M (Ollama at 10.70.70.14)
Corpus: 15 entries (2 per detector type + 3 clean)

## Overall Accuracy

| Variant | Precision | Recall | F1 | FPR | TP | FP | FN |
|---------|-----------|--------|-----|-----|----|----|----|
| A-full-cortex | 1.000 | 0.083 | 0.154 | 0.000 | 1 | 0 | 11 |
| F-ml-enriched | 0.125 | 0.083 | 0.100 | 1.000 | 1 | 7 | 11 |

## Latency

| Variant | Mean (ms) | P95 (ms) | P99 (ms) |
|---------|-----------|----------|----------|
| A-full-cortex (PURE) | 0.5 | 0.6 | 0.6 |
| F-ml-enriched | 309980.1 | 574752.3 | 634588.6 |

## Profile Winners

- **balanced**: A-full-cortex
- **precision-first**: A-full-cortex
- **recall-first**: A-full-cortex
- **minimal**: A-full-cortex

## Pareto Frontier

A-full-cortex

## Per-Detector Analysis

| Detector | PURE Prec | PURE Rec | PURE F1 | ML Prec | ML Rec | ML F1 | Delta F1 | ML Benefit |
|----------|-----------|----------|---------|---------|--------|-------|----------|------------|
| ANCHORING_BIAS | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | +0.000 | neutral |
| SUNK_COST_FALLACY | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | +0.000 | neutral |
| CONTRADICTION | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | +0.000 | neutral |
| SCOPE_DRIFT | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | +0.000 | neutral |
| CONFIDENCE_MISCALIBRATION | 1.000 | 0.500 | 0.667 | 0.111 | 0.500 | 0.182 | -0.485 | NO (-) |
| SILENT_REVISION | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | +0.000 | neutral |

### Detector-level Observations

- **ANCHORING_BIAS**: PURE F1=0.000 → ML F1=0.000 (no meaningful change). TP(P)=0 FP(P)=0 FN(P)=2 → TP(ML)=0 FP(ML)=0 FN(ML)=2
- **SUNK_COST_FALLACY**: PURE F1=0.000 → ML F1=0.000 (no meaningful change). TP(P)=0 FP(P)=0 FN(P)=2 → TP(ML)=0 FP(ML)=0 FN(ML)=2
- **CONTRADICTION**: PURE F1=0.000 → ML F1=0.000 (no meaningful change). TP(P)=0 FP(P)=0 FN(P)=2 → TP(ML)=0 FP(ML)=0 FN(ML)=2
- **SCOPE_DRIFT**: PURE F1=0.000 → ML F1=0.000 (no meaningful change). TP(P)=0 FP(P)=0 FN(P)=2 → TP(ML)=0 FP(ML)=0 FN(ML)=2
- **CONFIDENCE_MISCALIBRATION**: PURE F1=0.667 → ML F1=0.182 (degraded by -0.485 F1). TP(P)=1 FP(P)=0 FN(P)=1 → TP(ML)=1 FP(ML)=8 FN(ML)=1
- **SILENT_REVISION**: PURE F1=0.000 → ML F1=0.000 (no meaningful change). TP(P)=0 FP(P)=0 FN(P)=2 → TP(ML)=0 FP(ML)=0 FN(ML)=2

## Per-Entry Comparison (Extraction Quality)

| Entry | Expected | PURE Findings | ML Findings | ML Enrichments | Δ |
|-------|----------|---------------|-------------|----------------|---|
| cog-001 | ANCHORING_BIAS | (none) | CONFIDENCE_MISCALIBRATION | 4 | ML+ |
| cog-002 | ANCHORING_BIAS | (none) | CONFIDENCE_MISCALIBRATION | 4 | ML+ |
| cog-004 | SUNK_COST_FALLACY | (none) | (none) | 2 | = |
| cog-005 | SUNK_COST_FALLACY | (none) | (none) | 2 | = |
| cog-007 | CONTRADICTION | (none) | (none) | 4 | = |
| cog-008 | CONTRADICTION | (none) | CONFIDENCE_MISCALIBRATION | 5 | ML+ |
| cog-009 | SCOPE_DRIFT | (none) | CONFIDENCE_MISCALIBRATION | 4 | ML+ |
| cog-010 | SCOPE_DRIFT | (none) | (none) | 4 | = |
| cog-011 | CONFIDENCE_MISCALIBRATION | CONFIDENCE_MISCALIBRATION | CONFIDENCE_MISCALIBRATION | 2 | = |
| cog-012 | CONFIDENCE_MISCALIBRATION | (none) | (none) | 5 | = |
| cog-013 | SILENT_REVISION | (none) | (none) | 2 | = |
| cog-014 | SILENT_REVISION | (none) | CONFIDENCE_MISCALIBRATION | 4 | ML+ |
| cog-015 | CLEAN | (none) | CONFIDENCE_MISCALIBRATION | 5 | ML+ |
| cog-016 | CLEAN | (none) | CONFIDENCE_MISCALIBRATION | 3 | ML+ |
| cog-035 | CLEAN | (none) | CONFIDENCE_MISCALIBRATION | 4 | ML+ |

## LLM Reliability

- Model: glm-4.7-flash:q4_K_M (29.9B parameters, Q4_K_M quantization)
- JSON format mode enforced via Ollama `format: "json"`
- Temperature: 0.1 (near-deterministic)
- Timeout per turn: 120s (generous for 29.9B model)
- Fallback: All failures gracefully degrade to PURE (FallbackToPure=true)
- Latency includes all turns in each conversation

## Recommendation

Overall F1: PURE=0.154, ML=0.100 (Δ=-0.054)

Latency overhead: ML is 614227.4x slower (PURE mean=0.5ms, ML mean=309980.1ms)

**ML enrichment is NOT RECOMMENDED** on this corpus. PURE pipeline wins on both accuracy and latency.

ML enrichment is best treated as **opt-in infrastructure** for deployments where:
1. Latency budget is relaxed (>1s per conversation acceptable)
2. Confidence boosting and richer semantic context are valuable
3. Detectors that benefit most (Sunk-Cost, Anchoring-Context, Confidence-Calibrator) are in scope

The PURE pipeline (Variant A) remains the primary path: lower latency, no external dependencies, consistent accuracy.
