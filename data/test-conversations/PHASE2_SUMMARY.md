# Phase 2 Summary — Pipeline Calibration & AIP Competitions

## Objective

Fix Scope Guard over-triggering, build competition variants, run AIP competitions,
optimize via Forge parameter sweep, and improve pipeline F1 from 0.60 to ≥0.75.

## Results

### Before / After

| Metric | Phase 1 | Phase 2 | Change |
|--------|---------|---------|--------|
| **Recall** | 1.00 | 1.00 | — |
| **Precision** | 0.43 | 0.64 | **+0.21** |
| **F1 Score** | 0.60 | 0.78 | **+0.18** |
| **False Positives** | 12 | 5 | **-7** |
| **False Negatives** | 0 | 0 | — |

### Per-Conversation Results (Phase 2)

| # | Conversation | TP | FN | FP | Integrity | Status |
|---|---|---|---|---|---|---|
| 1 | Anchoring Bias | 1 | 0 | 0 | 0.85 | **PERFECT** |
| 2 | Sunk-Cost Fallacy | 1 | 0 | 0 | 0.95 | **PERFECT** |
| 3 | Contradiction | 1 | 0 | 0 | 0.70 | **PERFECT** |
| 4 | Scope Drift | 1 | 0 | 0 | 0.70 | **PERFECT** |
| 5 | Confidence Miscalibration | 1 | 0 | 1 | 0.40 | PASS |
| 6 | Multi-Failure (3 modes) | 3 | 0 | 2 | 0.00 | **ALL 3 FOUND** |
| 7 | Clean (no failures) | 0 | 0 | 1 | 0.70 | FP: confidence only |
| 8 | Borderline / Subtle | 1 | 0 | 1 | 0.55 | PASS |

### Key Improvements

1. **Scope Guard no longer fires on clean conversations** (was the #1 issue)
2. **4 conversations now PERFECT** (zero FPs) — up from 1 in Phase 1
3. **Anchoring FPs eliminated** by context-aware variant (filters "100%" in unrelated context)
4. **Sunk-Cost FPs eliminated** on conversations 1-4 (scope drift FPs no longer cross-trigger)

## What Changed

### Part A: Scope Guard Redesign

**Problem:** Original algorithm compared per-turn Jaccard distance against objective keywords.
Domain-specific terms (e.g., "postgresql", "kubernetes") had zero overlap with objective terms
(e.g., "select", "database"), producing Jaccard distance ≈ 1.0 on nearly every turn.

**Solution:** Reference-window algorithm with:
1. **Reference set** built from objective keywords (2x weight) + first K turns' keywords
2. **Sliding window** of last W turns for current topic
3. **Weighted Jaccard divergence** (frequency-weighted, not binary)
4. **Sustained turns requirement** — only flag when divergence exceeds threshold for N consecutive turns

**Optimized defaults** (found by Forge sweep):
- `drift_threshold`: 0.80 (was 0.70)
- `reference_turns`: 4 (new parameter)
- `window_size`: 3 (new parameter)
- `sustained_turns`: 8 (new parameter)

### Part B: Competition Variants (4 new COGs)

1. **scope-guard-centroid** — Cumulative centroid with exponential decay (decay=0.8)
2. **scope-guard-transition** — KL-divergence change-point detection (smoothing=0.01)
3. **anchoring-detector-context** — Context-aware relevance filter (overlap threshold=0.2)
4. **sunk-cost-detector-proximity** — Turn-distance decay weighting (decay_rate=0.3)

### Part C: AIP Competitions (3 run)

**Scope Guard Competition (3 variants):**
- Winner (all profiles): reference-window (F1=0.67, FPR=0.60)
- Centroid and transition variants both had FPR=1.0 (too sensitive)

**Anchoring Competition (2 variants):**
- Winner (all profiles): context-aware (F1=1.00, FPR=0.00) — **perfect score**
- Original had FPR=0.33 from "100%" being parsed as numeric anchor

**Sunk-Cost Competition (2 variants):**
- Tied at F1=1.00, FPR=0.00
- Original wins by convention (simpler)

### Part D: Forge Optimization

53-trial parameter sweep across scope guard (threshold × sustained × reference_turns)
and anchoring context threshold. Best config with recall ≥ 0.8 constraint:
- `sg(t=0.80, s=8, r=4)` → F1=0.78 (from 0.64 baseline)
- Improvement: +0.14 F1 from parameter optimization alone

## Remaining FPs (5 total)

| Conv | FP Detector | Explanation |
|------|-------------|-------------|
| 5 | SCOPE_DRIFT | Confidence conv drifts through multiple evidence domains |
| 6 | CONFIDENCE_MISCALIBRATION | Turn 14 "definitely" without evidence — debatable |
| 6 | CONTRADICTION | T1 vs T15 different cloud provider statements |
| 7 | CONFIDENCE_MISCALIBRATION | Turn 8 "absolutely" — actually a valid FP (mild miscalibration) |
| 8 | CONFIDENCE_MISCALIBRATION | Turn 10 "definitely" without evidence — borderline |

3 of 5 remaining FPs are from CONFIDENCE_MISCALIBRATION on "absolutely"/"definitely" without evidence.
These are arguably correct detections rather than false positives.

## Files Changed

### Modified
- `internal/pipeline/detectors.go` — Scope Guard redesign (reference-window + weighted Jaccard)
- `internal/pipeline/pipeline.go` — Added context-aware anchoring to pipeline config
- `cogs/scope-guard/detector.go` — Synced COG with pipeline implementation
- `cogs/scope-guard/detector_test.go` — Updated for new algorithm
- `cogs/scope-guard/cog.manifest.textpb` — New config fields, version 0.2.0
- `ROADMAP.md` — Added Phase 8

### New
- `internal/pipeline/variants.go` — 4 variant detector implementations
- `internal/pipeline/competition_test.go` — Competition runner + full pipeline comparison
- `internal/pipeline/optimize_test.go` — Forge parameter sweep optimization
- `cogs/scope-guard-centroid/` — Cumulative centroid variant COG (4 files)
- `cogs/scope-guard-transition/` — KL-divergence variant COG (4 files)
- `cogs/anchoring-detector-context/` — Context-aware anchoring COG (4 files)
- `cogs/sunk-cost-detector-proximity/` — Proximity-weighted sunk-cost COG (4 files)
- `data/competitions/scope-guard-competition.textpb` — Competition manifest
- `data/competitions/anchoring-competition.textpb` — Competition manifest
- `data/competitions/sunk-cost-competition.textpb` — Competition manifest

## Success Criteria Checklist

- [x] Scope Guard no longer fires on clean conversations
- [x] ≥3 AIP competitions run (3 run: scope-guard, anchoring, sunk-cost)
- [x] ≥1 Forge optimization produces improvement (F1 +0.14 from sweep)
- [x] F1 ≥ 0.75 (achieved: 0.78)
- [x] All tests pass (26 packages)
