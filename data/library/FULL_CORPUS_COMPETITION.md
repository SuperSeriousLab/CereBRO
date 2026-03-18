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
| A-full-cortex          | 0.696 | 0.317 | 0.436 | 0.273 | 0.73 | 2.20 | 39 | 17 | 84 |
| B-no-feedback          | 0.696 | 0.317 | 0.436 | 0.273 | 0.71 | 1.90 | 39 | 17 | 84 |
| C-no-modulation        | 0.678 | 0.325 | 0.440 | 0.364 | 0.69 | 1.69 | 40 | 19 | 83 |
| D-inhibitor-only       | 0.678 | 0.325 | 0.440 | 0.364 | 0.68 | 1.89 | 40 | 19 | 83 |
| E-pre-cortex           | 0.584 | 0.366 | 0.450 | 0.545 | 0.53 | 1.53 | 45 | 32 | 78 |

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
| balanced | 0.6189                 | 0.6494                 | 0.6655                 | 0.6956                 | 0.7572                 |
| precision-first | 0.7803                 | 0.7898                 | 0.7512                 | 0.7727                 | 0.6827                 |
| recall-first | 0.7388                 | 0.7541                 | 0.7640                 | 0.8027                 | 0.8628                 |
| minimal | 0.4288                 | 0.4809                 | 0.4883                 | 0.6241                 | 0.7006                 |

### Pareto Frontier

Pareto-optimal variants: **B-no-feedback, C-no-modulation, D-inhibitor-only, E-pre-cortex**

## Comparison: Previous vs Full Corpus

Previous results from CLASSICAL_ANALYSIS.md (43-entry classical corpus):

| Variant | Classical P/R/F1 | Full Corpus P/R/F1 | F1 Change |
|---------|------------------|--------------------|-----------|
| A-full-cortex          | 0.600 / 0.208 / 0.309 | 0.696 / 0.317 / 0.436 | +0.127 |
| B-no-feedback          | 0.600 / 0.208 / 0.309 | 0.696 / 0.317 / 0.436 | +0.127 |
| C-no-modulation        | 0.577 / 0.208 / 0.306 | 0.678 / 0.325 / 0.440 | +0.134 |
| D-inhibitor-only       | 0.577 / 0.208 / 0.306 | 0.678 / 0.325 / 0.440 | +0.134 |
| E-pre-cortex           | 0.471 / 0.333 / 0.390 | 0.584 / 0.366 / 0.450 | +0.060 |

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
- A-full-cortex (all layers): 0.436
- B-no-feedback (no metacognition): 0.436
- C-no-modulation (no urgency/threshold): 0.440
- D-inhibitor-only (minimal): 0.440

**No** — A-full-cortex F1 (0.436) does not meaningfully exceed D-inhibitor-only (0.440). The extra layers (modulation, feedback, salience) do not pay their way on this corpus.

**Feedback layer verdict**: B-no-feedback (0.436 F1) matches A-full-cortex (0.436) — the metacognition/feedback loop adds negligible value.

**Modulation layer verdict**: C-no-modulation (0.440 F1) is comparable to B-no-feedback (0.436) — urgency/threshold modulation adds marginal value.

**Profile wins summary**: A wins 0/4, B wins 1/4 profiles. The winner is E-pre-cortex.

### Overall Winner

| Variant | Profile wins (out of 4) |
|---------|------------------------|
| E-pre-cortex           | 3 |
| B-no-feedback          | 1 |

**Overall winner: E-pre-cortex** (3/4 profiles)

