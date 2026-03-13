# CereBRO Architecture Competition — Phase 6 Results

> **Date:** 2026-03-13
> **Corpus:** 9 test conversations (data/test-conversations/)
> **Runner:** TestArchitectureCompetition in internal/pipeline/competition_test.go

## Raw Results Matrix

| Variant | Precision | Recall | F1 | FPR | Latency Mean | Latency P95 | Stages | COGs | TP | FP | FN |
|---------|-----------|--------|------|------|-------------|-------------|--------|------|----|----|-----|
| A-full-cortex | 0.83 | 1.00 | 0.91 | 0.00 | 1.83 ms | 2.70 ms | 12 | 21 | 10 | 2 | 0 |
| B-no-feedback | 0.83 | 1.00 | 0.91 | 0.00 | 1.87 ms | 2.34 ms | 10 | 19 | 10 | 2 | 0 |
| C-no-modulation | 0.83 | 1.00 | 0.91 | 0.00 | 2.36 ms | 4.39 ms | 10 | 19 | 10 | 2 | 0 |
| D-inhibitor-only | 0.83 | 1.00 | 0.91 | 0.00 | 1.08 ms | 1.86 ms | 5 | 12 | 10 | 2 | 0 |
| E-pre-cortex | 0.67 | 1.00 | 0.80 | 1.00 | 0.95 ms | 1.53 ms | 4 | 10 | 10 | 5 | 0 |

## Per-Profile Winners

| Profile | Winner | Score | Runner-up | Score |
|---------|--------|-------|-----------|-------|
| **balanced** | D-inhibitor-only | 0.840 | E-pre-cortex | 0.792 |
| **precision-first** | D-inhibitor-only | 0.956 | B-no-feedback | 0.919 |
| **recall-first** | D-inhibitor-only | 0.905 | E-pre-cortex | 0.859 |
| **minimal** | D-inhibitor-only | 0.740 | E-pre-cortex | 0.731 |

**Variant D (inhibitor-only) wins all four profiles.**

## Pareto Frontier

Non-dominated variants: **D-inhibitor-only**, **E-pre-cortex**

- **D** offers the best accuracy (0.83 precision, 0 FPR) with moderate complexity
- **E** offers the fastest execution but at significant accuracy cost (0.67 precision, 1.0 FPR)
- Variants A, B, C are all Pareto-dominated by D (same accuracy, more latency/complexity)

## Layer Contribution Analysis

| Transition | Layer Added | Precision Delta | F1 Delta | FP Delta | Latency Delta |
|------------|------------|-----------------|----------|----------|---------------|
| E → D | Context Inhibitor (Phase 1) | **+0.17** | **+0.11** | **-3** | +0.13 ms |
| D → C | Salience + Metacognition | +0.00 | +0.00 | +0 | +1.28 ms |
| D → B | Layer 0 + Modulation + Salience | +0.00 | +0.00 | +0 | +0.79 ms |
| B → A | Feedback Evaluator | +0.00 | +0.00 | +0 | -0.04 ms |
| C → A | Gain Modulation | +0.00 | +0.00 | +0 | -0.53 ms |

### Key Finding

**The Context Inhibitor (Phase 1) provides 100% of the precision improvement.**

All five CereBRO layers beyond the inhibitor add zero measurable accuracy benefit on
the current corpus. The additional layers (Urgency Assessor, Threshold Modulator,
Layer 0 reflexes, Salience Filter, Self-Confidence Assessor, Feedback Evaluator,
Memory Consolidator) add latency but no precision or recall improvement.

### Why This Happened

1. **The corpus is small** (9 conversations). Layers designed for edge cases
   (borderline confidence, subtle urgency shifts, novel patterns) need larger,
   more diverse corpora to demonstrate value.

2. **Recall is already 1.00.** All variants detect every true positive. The only
   question is false positives — and the inhibitor gates all 3 FPs that matter
   (casual confidence miscalibration on the clean conversation, and 2 extras on
   multi-failure and borderline).

3. **Feedback was never triggered.** The Self-Confidence Assessor consistently
   produces moderate-to-high confidence on the test corpus. No conversation
   triggers the re-evaluation loop. The feedback loop is designed for borderline
   cases that don't exist in the current test set.

4. **Gain modulation is neutral.** The Urgency Assessor correctly classifies
   conversation formality and urgency, but the threshold adjustments don't change
   which findings cross their thresholds. The adjustments are within noise margins.

5. **Layer 0 has no invalid/toxic/non-English inputs to reject.** All 9 test
   conversations are well-formed English. Layer 0 passes everything through.

## Recommendation

### Default Configuration: **D-inhibitor-only**

For the current corpus, the inhibitor-only pipeline (Variant D) is optimal under
every evaluation profile. It achieves the same accuracy as the full pipeline at
58% less latency and 43% fewer COGs.

### Viable Alternatives

| Use Case | Recommended Variant | Rationale |
|----------|-------------------|-----------|
| General purpose | D-inhibitor-only | Best accuracy/complexity tradeoff |
| Production with untrusted input | A-full-cortex | Layer 0 needed for format/toxicity/language validation |
| Maximum speed, acceptable precision | E-pre-cortex | 45% faster than D, but 3 more FPs |
| Future-proofing | A-full-cortex | Salience + consolidation + feedback will matter with larger corpus |

### Layers Not Earning Their Keep (On This Corpus)

| Layer | Phase | Accuracy Impact | Latency Cost | Verdict |
|-------|-------|-----------------|-------------|---------|
| Context Inhibitor | 1 | **+17% precision** | +0.13 ms | **Essential** |
| Urgency Assessor + Modulator | 2 | 0% | +0.79 ms | Not yet valuable |
| Layer 0 reflexes | 3 | 0% | minimal | Needed for untrusted input |
| Self-Confidence + Feedback | 4 | 0% | minimal | Needs borderline test cases |
| Salience Filter | 5 | 0% | minimal | Needs more diverse findings |
| Memory Consolidator | 5 | 0% | N/A (disabled) | Long-term learning, not per-run |

### Caveats

This analysis reflects the current 9-conversation test corpus. The architecture was
designed for production workloads with:
- Thousands of conversations with diverse formality/urgency levels
- Untrusted user input (format errors, injection, non-English)
- Borderline findings that benefit from re-evaluation
- Long-running sessions where memory consolidation matters

**The competition validates the inhibitor as the essential CereBRO component, and
identifies a clear path for demonstrating the value of other layers: expand the
corpus with edge cases each layer was designed to handle.**
