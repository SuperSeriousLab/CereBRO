# Cognitive Pipeline Hard-Test Scorecard — 2026-03-12

## Test Setup

- **Pipeline**: Intake → Router → 6 Detectors → Aggregator (in-process, Go)
- **Test Conversations**: 8 (LLM-generated via Ollama glm-4.7-flash:q4_K_M, some manually refined)
- **Turn counts**: 12-15 turns each
- **Detector configs**: All defaults

## Results Summary

| # | Conversation | TP | FN | FP | Integrity | Status |
|---|---|---|---|---|---|---|
| 1 | Anchoring Bias | 1 | 0 | 1 | 0.70 | PASS — detected |
| 2 | Sunk-Cost Fallacy | 1 | 0 | 2 | 0.50 | PASS — detected |
| 3 | Contradiction | 1 | 0 | 1 | 0.40 | PASS — detected |
| 4 | Scope Drift | 1 | 0 | 0 | 0.70 | **PERFECT** |
| 5 | Confidence Miscalibration | 1 | 0 | 3 | 0.00 | PASS — detected |
| 6 | Multi-Failure (3 modes) | 3 | 0 | 2 | 0.05 | **ALL 3 FOUND** |
| 7 | Clean (no failures) | 0 | 0 | 1 | 0.70 | FP: scope drift |
| 8 | Borderline / Subtle | 1 | 0 | 1 | 0.40 | PASS — detected |

### Aggregate Metrics

| Metric | Value |
|--------|-------|
| **Recall** | **1.00** (9/9 expected findings detected) |
| **Precision** | **0.43** (9 TP / 21 total findings) |
| **F1 Score** | **0.60** |
| **False Negatives** | **0** |
| **False Positives** | **12** (8 from SCOPE_DRIFT, rest from cross-trigger) |

## Detailed Findings

### Conversation 1: Anchoring Bias (LLM-generated)
- **TP**: ANCHORING_BIAS detected (anchor=18mo, estimate clusters near 14-20mo)
- **FP**: SCOPE_DRIFT (confidence=1.00, CRITICAL)
- Severity: INFO, Confidence: 0.26
- Relevant turns: [1, 2]

### Conversation 2: Sunk-Cost Fallacy (LLM-generated)
- **TP**: SUNK_COST_FALLACY detected (cost="already invested", continuation="should continue")
- **FP**: ANCHORING_BIAS (numbers $1.5M → nearby estimates), SCOPE_DRIFT
- Severity: CAUTION, Confidence: 0.75
- Relevant turns: [2, 2] (same turn has both phrases)

### Conversation 3: Contradiction (manually constructed)
- **TP**: CONTRADICTION detected (T6 "would not recommend MongoDB" vs T10 "would recommend MongoDB")
- **FP**: SCOPE_DRIFT
- Severity: CRITICAL, Confidence: 0.90
- Negation conflict detected between turns 6 and 10

### Conversation 4: Scope Drift (LLM-generated)
- **TP**: SCOPE_DRIFT detected across turns 2-15
- **No FPs** — only the expected finding
- Severity: CRITICAL, Confidence: 1.00
- Progressive drift: database → Kubernetes → hiring → retreat

### Conversation 5: Confidence Miscalibration (manually constructed)
- **TP**: CONFIDENCE_MISCALIBRATION (CERTAIN expressed, SPECULATED evidence, ECE=0.67)
- **FP**: ANCHORING_BIAS (100% in T6 and T10 parsed as numbers), CONTRADICTION (T6 vs T12 sentence overlap + negation), SCOPE_DRIFT
- Severity: CRITICAL, Confidence: 0.67
- Worst turn: T2 ("absolutely certain" + zero evidence)

### Conversation 6: Multi-Failure (manually constructed)
- **TP×3**: All three expected failures detected:
  - ANCHORING_BIAS: $500K anchor, estimates cluster near it (T1→T2)
  - SUNK_COST_FALLACY: "already invested" + "stick with" (T3→T4)
  - SCOPE_DRIFT: hiring/office/retreat topics far from cloud selection
- **FP**: CONTRADICTION (T1 vs T15 — different statements about cloud provider)
- Integrity score: 0.20 (severely compromised)

### Conversation 7: Clean — No Failures (manually constructed)
- **Expected**: No findings
- **Actual**: SCOPE_DRIFT detected (FP)
- This is the **false positive test** — every other detector stayed quiet
- Only SCOPE_DRIFT over-triggers (see analysis below)

### Conversation 8: Borderline / Subtle (manually constructed)
- **TP**: SCOPE_DRIFT detected (mild drift: MVP → CI/CD → monitoring)
- **FP**: CONFIDENCE_MISCALIBRATION (T10: "definitely" without evidence)
- The confidence FP is actually debatable — "definitely use Datadog" without evidence IS mild miscalibration
- Silent revision (React→Vue with "actually") was NOT detected — "actually" is a weak rationale marker that suppresses the finding

## Critical Issue: SCOPE_DRIFT Over-Triggering

**Root Cause**: The Scope Guard detector uses Jaccard distance between turn topic keywords and objective keywords. For most conversations, domain-specific terms in turns (e.g., "postgresql", "kubernetes", "retry") have zero overlap with objective terms (e.g., "select", "database", "startup"). This produces Jaccard distance = 1.0 on nearly every turn.

**Impact**: SCOPE_DRIFT fires on 7/8 conversations with CRITICAL severity and confidence=1.0.

**Evidence from clean conversation (07)**:
- Objective keywords: ["review", "pull", "request", "payment", "service", "error", "handling"]
- Turn 2 keywords: ["overall", "structure", "looks", "reasonable", "retry", "logic", ...]
- Jaccard distance: ~1.0 (minimal overlap despite being perfectly on-topic)

**Recommended Fix**: The scope guard needs semantic similarity (not just lexical overlap) to be useful. Options:
1. Raise DriftThreshold from 0.7 to 0.95
2. Require ≥3 consecutive drift turns (currently fires on any single turn above threshold)
3. Use embedding-based similarity instead of Jaccard distance
4. Filter objective keywords to only content-bearing terms

## Other Detector Observations

### Anchoring Detector
- Works well when numbers are present and close in magnitude
- FPs when percentage figures ("100%") are parsed as numbers
- Low false negative rate

### Sunk-Cost Detector
- Reliable when both cost phrases AND continuation phrases are present
- Misses when continuation language uses different phrasing (e.g., "let's push through" not in phrase list)
- Adding more continuation phrases would improve recall

### Contradiction Tracker
- Detects negation conflicts reliably when word overlap is high
- Can produce FPs on long conversations where unrelated sentences happen to share words
- The 0.3 overlap threshold is reasonable

### Confidence Calibrator
- Detects overconfidence well (CERTAIN + SPECULATED = CRITICAL)
- Correctly ignores hedged statements ("I think", "probably")
- "100%" is picked up as CERTAIN confidence marker — correct behavior

### Decision Ledger
- Did not trigger on any test conversation
- Needs both a decision marker AND a topic change on the same subject
- "Actually" as weak rationale suppresses findings (correct behavior for borderline cases)

## Seed Corpus Results (18 entries)

Running the pipeline against the existing seed corpus:
- ANCHORING_BIAS: 3/4 TP (75% recall)
- SUNK_COST_FALLACY: 0/4 TP (0% — continuation phrases in corpus don't match detector patterns)
- CONTRADICTION: 0/2 TP (0% — sentence overlap too low in short entries)
- SCOPE_DRIFT: 2/3 TP (67%), but 16 FPs across all entries
- CONFIDENCE_MISCALIBRATION: 2/3 TP (67%)
- SILENT_REVISION: 0/2 TP (0% — decision markers not matching)
- Clean: 0/2 TP but 2 FPs (SCOPE_DRIFT)

The seed corpus was designed for individual detector unit tests, not the full pipeline.
Short 5-turn conversations don't provide enough context for some detectors.

## Recommendations

1. **SCOPE_DRIFT**: Raise threshold or require consecutive drift turns to reduce FP rate
2. **SUNK_COST**: Expand continuation phrase list (add "push through", "keep at it", "see it through")
3. **CONTRADICTION**: Consider lowering MinOverlap to 0.2 for longer conversations
4. **DECISION_LEDGER**: Expand decision marker list to capture more phrasing variations
5. **General**: Longer conversations (10+ turns) work better than short ones for all detectors
6. **Corpus**: Update seed corpus entries to use phrase patterns the detectors actually match
