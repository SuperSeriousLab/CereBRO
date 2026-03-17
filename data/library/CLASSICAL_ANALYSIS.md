# CereBRO Classical Analysis — Republic Book 1

**Date:** 2026-03-14  
**Corpus:** Plato's Republic Book 1 (Jowett translation)  
**Entries:** 43 (Cephalus: 4, Polemarchus: 13, Thrasymachus: 26)

## Pipeline Run Metrics

| Run | TP | FP | FN | Precision | Recall | F1 |
|-----|----|----|----|-----------|-----------|----|
| baseline             |  33 |  41 |  39 | 0.446 | 0.458 | 0.452 |
| inhibitor-only       |  25 |  29 |  47 | 0.463 | 0.347 | 0.397 |

## Per-Detector Breakdown (baseline config)

| Detector | TP | FP | FN | Precision | Recall | F1 | Notes |
|----------|----|----|----|-----------|-----------|----|-------|
| ANCHORING_BIAS               |   0 |   0 |  11 | 0.000 | 0.000 | 0.000 | Thrasymachus fixates on 'advantage of the stronger' |
| CONTRADICTION                |  16 |  12 |   6 | 0.571 | 0.727 | 0.640 | Polemarchus + Thrasymachus self-refutation chains |
| SCOPE_DRIFT                  |  10 |  22 |   3 | 0.312 | 0.769 | 0.444 | Thrasymachus late-dialogue scope shift |
| CONFIDENCE_MISCALIBRATION    |   4 |   7 |   9 | 0.364 | 0.308 | 0.333 | Thrasymachus overconfidence early in dialogue |
| SUNK_COST_FALLACY            |   3 |   0 |  10 | 1.000 | 0.231 | 0.375 | Polemarchus defends Simonides definition throughout |
| SILENT_REVISION              |   0 |   0 |   0 | 0.000 | 0.000 | 0.000 | Rare in Socratic dialogue format |

## Per-Section Breakdown (baseline config)

| Section | Entries | TP | FP | FN | Precision | Recall | F1 |
|---------|---------|----|----|----|-----------|-----------|----|
| cephalus       |       4 |   0 |   8 |   0 | 0.000 | 0.000 | 0.000 |
| polemarchus    |      13 |   7 |  17 |  13 | 0.292 | 0.350 | 0.318 |
| thrasymachus   |      26 |  26 |  16 |  26 | 0.619 | 0.500 | 0.553 |

## Context Inhibitor — Formality Analysis

The Context Inhibitor gates findings based on formality score.
Classical philosophical text is expected to score > 0.70 (formal).

- Average formality across 38 entries: **0.577**
- Entries below 0.70 threshold: [republic-b1-cep-001 (0.571) republic-b1-cep-004 (0.571) republic-b1-pol-001 (0.636) republic-b1-pol-002 (0.444) republic-b1-pol-003 (0.286) republic-b1-pol-005 (0.500) republic-b1-pol-006 (0.400) republic-b1-pol-007 (0.500) republic-b1-pol-008 (0.500) republic-b1-pol-009 (0.667) republic-b1-pol-010 (0.143) republic-b1-pol-012 (0.300) republic-b1-pol-013 (0.667) republic-b1-thr-003 (0.643) republic-b1-thr-004 (0.417) republic-b1-thr-007 (0.583) republic-b1-thr-008 (0.636) republic-b1-thr-010 (0.611) republic-b1-thr-011 (0.600) republic-b1-thr-012 (0.692) republic-b1-thr-014 (0.600) republic-b1-thr-015 (0.500) republic-b1-thr-017 (0.571) republic-b1-thr-020 (0.500) republic-b1-thr-021 (0.583) republic-b1-thr-022 (0.538) republic-b1-thr-023 (0.250) republic-b1-thr-024 (0.364)]

### Confidence Miscalibration Preservation

Expected CONFIDENCE_MISCALIBRATION in 13 entries, found in 1 (8%)

**WARNING**: Confidence miscalibration findings appear suppressed by the inhibitor.

## Architecture Competition (43-entry classical corpus)

### Profile Winners

| Profile | Winner |
|---------|--------|
| balanced             | E-pre-cortex |
| precision-first      | A-full-cortex |
| recall-first         | E-pre-cortex |
| minimal              | E-pre-cortex |

### Pareto Frontier

Pareto-optimal variants: **A-full-cortex, B-no-feedback, D-inhibitor-only, E-pre-cortex**

### Variant Trait Matrix

| Variant | Precision | Recall | F1 | FPR | Latency(ms) | P95(ms) | TP | FP | FN |
|---------|-----------|--------|----|-----|-------------|---------|----|----|-----|
| A-full-cortex          | 0.645 | 0.278 | 0.388 | 0.500 | 1.00 | 2.62 | 20 | 11 | 52 |
| B-no-feedback          | 0.645 | 0.278 | 0.388 | 0.500 | 1.97 | 5.98 | 20 | 11 | 52 |
| C-no-modulation        | 0.625 | 0.278 | 0.385 | 0.750 | 2.79 | 7.29 | 20 | 12 | 52 |
| D-inhibitor-only       | 0.625 | 0.278 | 0.385 | 0.750 | 1.83 | 4.54 | 20 | 12 | 52 |
| E-pre-cortex           | 0.548 | 0.319 | 0.404 | 1.000 | 1.55 | 3.91 | 23 | 19 | 49 |

### Profile Score Matrix

(Normalized weighted scores per profile)

| Profile | A-full-cortex          | B-no-feedback          | C-no-modulation        | D-inhibitor-only       | E-pre-cortex           |
|---------|-----------------------|-----------------------|-----------------------|-----------------------|-----------------------|
| balanced | 0.7777                 | 0.6924                 | 0.6225                 | 0.7493                 | 0.8002                 |
| precision-first | 0.8116                 | 0.8024                 | 0.7234                 | 0.7614                 | 0.6952                 |
| recall-first | 0.8027                 | 0.7808                 | 0.7445                 | 0.8164                 | 0.8815                 |
| minimal | 0.5560                 | 0.5338                 | 0.4697                 | 0.6718                 | 0.7361                 |

## Key Observations

1. **D-inhibitor-only did NOT dominate**: wins only 0/4 profiles — harder classical text may favor more complex variants.

2. **Full pipeline does not significantly outperform D-inhibitor-only** (F1: 0.388 vs 0.385). Simpler pipeline is preferred for classical text.

3. **Scope drift detection challenges**: Classical philosophical dialogue redefines scope by design — Socrates deliberately shifts scope to expose contradictions. High FP rate expected for SCOPE_DRIFT.

4. **SUNK_COST_FALLACY challenges**: Polemarchus's insistence on Simonides is a genuine sunk-cost pattern, but the detector is tuned for modern conversational markers (investment/cost vocabulary).

5. **Formality gate**: Classical text scores high on formality, so the inhibitor passes most findings rather than suppressing them. The 5-gate algorithm is not a bottleneck here.

## Per-Entry Detail (baseline, first 20 entries)

| Entry ID | Section | Expected | Found | TP | FP | FN |
|----------|---------|----------|-------|----|----|-----|
| republic-b1-cep-001       | cephalus     | (none)                                   | CONFIDENCE_MISCALIBRATION, CONTRADICTION, SCOPE_DRIFT |  0 |  3 |  0 |
| republic-b1-cep-002       | cephalus     | (none)                                   | CONFIDENCE_MISCALIBRATION, SCOPE_DRIFT |  0 |  2 |  0 |
| republic-b1-cep-003       | cephalus     | (none)                                   | CONTRADICTION, SCOPE_DRIFT          |  0 |  2 |  0 |
| republic-b1-cep-004       | cephalus     | (none)                                   | CONTRADICTION                       |  0 |  1 |  0 |
| republic-b1-pol-001       | polemarchus  | SUNK_COST_FALLACY                        | CONFIDENCE_MISCALIBRATION, CONTRADICTION, SCOPE_DRIFT, SUNK_COST_FALLACY |  1 |  3 |  0 |
| republic-b1-pol-002       | polemarchus  | SUNK_COST_FALLACY                        | SCOPE_DRIFT                         |  0 |  1 |  1 |
| republic-b1-pol-003       | polemarchus  | CONTRADICTION, SUNK_COST_FALLACY         | CONTRADICTION, SCOPE_DRIFT          |  1 |  1 |  1 |
| republic-b1-pol-004       | polemarchus  | CONTRADICTION, SUNK_COST_FALLACY         | CONTRADICTION, SCOPE_DRIFT          |  1 |  1 |  1 |
| republic-b1-pol-005       | polemarchus  | CONTRADICTION, SUNK_COST_FALLACY         | CONFIDENCE_MISCALIBRATION, CONTRADICTION, SCOPE_DRIFT |  1 |  2 |  1 |
| republic-b1-pol-006       | polemarchus  | CONTRADICTION, SUNK_COST_FALLACY         | (none)                              |  0 |  0 |  2 |
| republic-b1-pol-007       | polemarchus  | CONTRADICTION, SUNK_COST_FALLACY         | SCOPE_DRIFT                         |  0 |  1 |  2 |
| republic-b1-pol-008       | polemarchus  | CONTRADICTION, SUNK_COST_FALLACY         | CONFIDENCE_MISCALIBRATION, SCOPE_DRIFT, SUNK_COST_FALLACY |  1 |  2 |  1 |
| republic-b1-pol-009       | polemarchus  | CONTRADICTION, SUNK_COST_FALLACY         | CONTRADICTION, SCOPE_DRIFT          |  1 |  1 |  1 |
| republic-b1-pol-010       | polemarchus  | SUNK_COST_FALLACY                        | CONTRADICTION, SCOPE_DRIFT          |  0 |  2 |  1 |
| republic-b1-pol-011       | polemarchus  | SUNK_COST_FALLACY                        | SCOPE_DRIFT                         |  0 |  1 |  1 |
| republic-b1-pol-012       | polemarchus  | SUNK_COST_FALLACY                        | CONTRADICTION, SCOPE_DRIFT          |  0 |  2 |  1 |
| republic-b1-pol-013       | polemarchus  | SUNK_COST_FALLACY                        | SUNK_COST_FALLACY                   |  1 |  0 |  0 |
| republic-b1-thr-001       | thrasymachus | ANCHORING_BIAS, CONFIDENCE_MISCALIBRATION | CONTRADICTION                       |  0 |  1 |  2 |
| republic-b1-thr-002       | thrasymachus | ANCHORING_BIAS, CONFIDENCE_MISCALIBRATION | CONTRADICTION, SCOPE_DRIFT          |  0 |  2 |  2 |
| republic-b1-thr-003       | thrasymachus | ANCHORING_BIAS, CONFIDENCE_MISCALIBRATION | SCOPE_DRIFT                         |  0 |  1 |  2 |

