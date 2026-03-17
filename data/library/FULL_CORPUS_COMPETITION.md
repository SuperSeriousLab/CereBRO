# CereBRO Full Corpus Architecture Competition

**Date:** 2026-03-14  
**Total entries:** 94 unique (deduplicated by entry_id)

## Corpus Composition

| Source | Unique entries contributed |
|--------|---------------------------|
| cognitive-v1    | 18 |
| expanded-v1     | 28 |
| generated-v1    | 0 |
| classical-v1    | 43 |
| adversarial-v1  | 5 |

## Finding Type Distribution (full corpus)

| Finding Type | Count |
|--------------|-------|
| ANCHORING_BIAS                      | 22 |
| CONTRADICTION                       | 30 |
| SCOPE_DRIFT                         | 24 |
| CONFIDENCE_MISCALIBRATION           | 17 |
| SUNK_COST_FALLACY                   | 25 |
| SILENT_REVISION                     | 5 |
| **Total** | **123** |

## Architecture Competition — Full Corpus (94 entries)

### Variant Trait Matrix

| Variant | Precision | Recall | F1 | FPR | Latency(ms) | P95(ms) | TP | FP | FN |
|---------|-----------|--------|----|-----|-------------|---------|----|----|-----|
| A-full-cortex          | 0.736 | 0.317 | 0.443 | 0.182 | 1.14 | 3.36 | 39 | 14 | 84 |
| B-no-feedback          | 0.736 | 0.317 | 0.443 | 0.182 | 0.71 | 2.39 | 39 | 14 | 84 |
| C-no-modulation        | 0.714 | 0.325 | 0.447 | 0.273 | 0.84 | 2.74 | 40 | 16 | 83 |
| D-inhibitor-only       | 0.714 | 0.325 | 0.447 | 0.273 | 0.86 | 2.75 | 40 | 16 | 83 |
| E-pre-cortex           | 0.592 | 0.366 | 0.452 | 0.545 | 0.58 | 2.05 | 45 | 31 | 78 |

### Profile Winners

| Profile | Winner |
|---------|--------|
| balanced             | E-pre-cortex |
| precision-first      | B-no-feedback |
| recall-first         | E-pre-cortex |
| minimal              | E-pre-cortex |

### Profile Score Matrix

(Normalized weighted scores — higher is better)

| Profile | A-full-cortex          | B-no-feedback          | C-no-modulation        | D-inhibitor-only       | E-pre-cortex           |
|---------|-----------------------|-----------------------|-----------------------|-----------------------|-----------------------|
| balanced | 0.6230                 | 0.7221                 | 0.6957                 | 0.7304                 | 0.7925                 |
| precision-first | 0.8160                 | 0.8434                 | 0.7963                 | 0.8163                 | 0.6797                 |
| recall-first | 0.7416                 | 0.7929                 | 0.7876                 | 0.8234                 | 0.8792                 |
| minimal | 0.4316                 | 0.5556                 | 0.5333                 | 0.6631                 | 0.7402                 |

### Pareto Frontier

Pareto-optimal variants: **B-no-feedback, C-no-modulation, D-inhibitor-only, E-pre-cortex**

## Comparison: Previous vs Full Corpus

Previous results from CLASSICAL_ANALYSIS.md (43-entry classical corpus):

| Variant | Classical P/R/F1 | Full Corpus P/R/F1 | F1 Change |
|---------|------------------|--------------------|-----------|
| A-full-cortex          | 0.600 / 0.208 / 0.309 | 0.736 / 0.317 / 0.443 | +0.134 |
| B-no-feedback          | 0.600 / 0.208 / 0.309 | 0.736 / 0.317 / 0.443 | +0.134 |
| C-no-modulation        | 0.577 / 0.208 / 0.306 | 0.714 / 0.325 / 0.447 | +0.141 |
| D-inhibitor-only       | 0.577 / 0.208 / 0.306 | 0.714 / 0.325 / 0.447 | +0.141 |
| E-pre-cortex           | 0.471 / 0.333 / 0.390 | 0.592 / 0.366 / 0.452 | +0.062 |

Previous profile winners (classical 43-entry corpus):

| Profile | Original (9 conv) | Classical (43) | Adversarial (5) | Full Corpus | Change |
|---------|-------------------|----------------|-----------------|-------------|--------|
| balanced             | D-inhibitor-only  | E-pre-cortex   | N/A             | E-pre-cortex | same |
| precision-first      | D-inhibitor-only  | B-no-feedback  | N/A             | B-no-feedback | same |
| recall-first         | D-inhibitor-only  | E-pre-cortex   | N/A             | E-pre-cortex | same |
| minimal              | D-inhibitor-only  | E-pre-cortex   | N/A             | E-pre-cortex | same |

## Key Questions

### Does D-inhibitor-only still win on the larger corpus?

**NO** — D-inhibitor-only wins 0/4 profiles on the full 94-entry corpus. The diversity of the combined corpus (conversational + classical + adversarial) favors other variants.

### Do modulation/feedback/salience layers earn their keep on harder, more diverse input?

F1 scores on full corpus:
- A-full-cortex (all layers): 0.443
- B-no-feedback (no metacognition): 0.443
- C-no-modulation (no urgency/threshold): 0.447
- D-inhibitor-only (minimal): 0.447

**No** — A-full-cortex F1 (0.443) does not meaningfully exceed D-inhibitor-only (0.447). The extra layers (modulation, feedback, salience) do not pay their way on this corpus.

**Feedback layer verdict**: B-no-feedback (0.443 F1) matches A-full-cortex (0.443) — the metacognition/feedback loop adds negligible value.

**Modulation layer verdict**: C-no-modulation (0.447 F1) is comparable to B-no-feedback (0.443) — urgency/threshold modulation adds marginal value.

**Profile wins summary**: A wins 0/4, B wins 1/4 profiles. The winner is E-pre-cortex.

### Overall Winner

| Variant | Profile wins (out of 4) |
|---------|------------------------|
| E-pre-cortex           | 3 |
| B-no-feedback          | 1 |

**Overall winner: E-pre-cortex** (3/4 profiles)

