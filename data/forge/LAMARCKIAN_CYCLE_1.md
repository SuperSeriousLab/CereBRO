# Lamarckian Loop — Cycle 1 Results

> **Date:** 2026-03-17
> **Corpus:** full-v2.ndjson (112 entries: 18 cognitive + 46 expanded + 43 classical + 5 adversarial)
> **Method:** 1,200-combination grid sweep via forge-eval subprocess evaluator
> **Parameters evolved:** 5 (drift_threshold, sustained_turns, anchor_threshold, orbit_threshold, min_citations)

## Pipeline

- **Variant selection:** Adaptive (D-inhibitor-only for modern, E-pre-cortex for classical)
- **Corpus:** 112 entries across 4 domains
- **Evaluator:** `cmd/forge-eval/` standalone binary, subprocess interface
- **Sweep:** 5 drift_threshold × 5 sustained_turns × 4 anchor_threshold × 4 orbit_threshold × 3 min_citations = 1,200 combinations

## Results

### Baseline vs Best Evolved

| Metric | Baseline (defaults) | Best evolved | Delta |
|--------|-------------------|-------------|-------|
| **F1** | 0.434 | **0.477** | **+0.043 (+9.9%)** |
| Precision | 0.600 | 0.530 | -0.070 |
| Recall | 0.340 | 0.433 | **+0.093 (+27%)** |
| TP | 48 | 61 | +13 |
| FP | 32 | 54 | +22 |
| FN | 93 | 80 | -13 |

### Winning Configuration

```
drift_threshold  = 0.79  (unchanged from default)
sustained_turns  = 4     (was 8 — the primary lever)
anchor_threshold = 0.35  (was 0.30 — slightly looser)
orbit_threshold  = insensitive (0.45-0.60 all produce same result)
min_citations    = insensitive (2-4 all produce same result)
```

### Parameter Sensitivity Analysis

| Parameter | Sensitive? | Range tested | Impact |
|-----------|-----------|-------------|--------|
| sustained_turns | **HIGH** | 2-6 | st=2: highest recall (0.461), lowest precision (0.489). st=6: highest precision, lowest recall. st=4: best F1 balance. |
| drift_threshold | MODERATE | 0.60-0.79 | dt=0.79 (default) is already optimal for F1. Lower values increase FP without proportional TP gain. |
| anchor_threshold | LOW | 0.20-0.35 | at=0.35 marginally better. The Conceptual Anchoring detector fires rarely — its threshold matters less than Scope Guard's. |
| orbit_threshold | INSENSITIVE | 0.45-0.60 | No measurable impact across range. |
| min_citations | INSENSITIVE | 2-4 | No measurable impact. Inherited-Position fires infrequently. |

### Key Finding

**sustained_turns is the dominant parameter.** Reducing it from 8→4 catches 13 more true positives because:
1. Classical dialogues are 10 turns — with reference_turns=4, only 6 turns are evaluatable. st=8 was mathematically impossible (already known).
2. Even modern conversations benefit from st=4 — shorter drift episodes that were sub-threshold at st=8 now trigger.
3. The cost is 22 more false positives — borderline conversations where 4-turn drift patterns are noise, not signal.

**drift_threshold is already at optimum.** The previous grid search (53-trial) and the Porter-lite stemmer adjustment both converged on 0.79. This sweep confirms it holds on the expanded 112-entry corpus.

**Tier 2 detectors have flat landscapes.** Conceptual Anchoring and Inherited Position don't fire often enough to move F1. Their parameters matter on individual conversations but not on aggregate metrics. More corpus entries triggering these detectors are needed before their parameters can be meaningfully evolved.

## Deployment Decision

**DO NOT deploy st=4 globally.** The recall gain (+27%) comes with a precision loss (-12%). For the modern-only pipeline (D-inhibitor-only), precision is more valuable than recall (users see false positives; they don't see false negatives).

**RECOMMENDED:** Use sustained_turns via domain context:
- Modern text: st=8 (current, high precision)
- Classical text: st=3 (already set via domain context, confirmed correct)
- Mixed/unknown: st=4 (the sweep winner — balanced trade-off)

This is already partially implemented: `applyDomainContext` sets st=3 for classical. Adding a "mixed" domain context for unknown text with st=4 would capture the sweep's improvement without degrading modern precision.

## Verdict

**The Lamarckian Loop mechanism works.** Cycle 1 produced a measurable improvement (F1 +0.043) through systematic parameter exploration on an expanded corpus. The improvement is modest because:

1. **Scope Guard dominates.** Of 7 evolvable parameters, only 2 (sustained_turns, drift_threshold) materially affect F1. The other 5 have flat landscapes.
2. **Prior calibration was good.** The 53-trial grid search + stemmer adjustment already found near-optimal drift_threshold. The sweep confirms rather than improves it.
3. **Corpus needs growth.** 112 entries don't exercise Tier 2 detectors enough for their parameters to matter. Cycle 2 needs 200+ entries with deliberate anchoring and inherited-position patterns.

## What Cycle 2 Needs

1. **More corpus entries** — especially for Conceptual Anchoring and Inherited Position
2. **Consolidator-produced entries** — runtime confirmations from real pipeline usage
3. **Forge integration** — move from grid sweep to evolutionary search (tournament selection, crossover) for efficiency on larger parameter spaces
4. **Multi-objective optimization** — separate modern precision from classical recall as independent fitness dimensions (Pareto frontier)
