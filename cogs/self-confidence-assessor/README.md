# Self-Confidence Assessor

CereBRO Phase 2, Layer 4 -- Self-Assessment.

## Algorithm

Computes overall system confidence in its own findings using three weighted
components:

1. **Agreement score** -- Cross-detector agreement among findings.
2. **Margin score** -- How far individual confidence values sit from decision
   boundaries.
3. **Historical score** -- Consistency with prior assessment patterns.

The weighted sum produces an `OverallConfidence` value in [0, 1].

## Ports

| Port | Direction | Type |
|------|-----------|------|
| reasoning_report | IN | ReasoningReport |
| self_confidence | OUT | SelfConfidenceReport |

## Config

| Field | Type | Default | Evolvable |
|-------|------|---------|-----------|
| agreement_weight | DOUBLE | 0.4 | yes |
| margin_weight | DOUBLE | 0.35 | yes |
| historical_weight | DOUBLE | 0.25 | yes |
| high_confidence_threshold | DOUBLE | 0.8 | yes |
| moderate_confidence_threshold | DOUBLE | 0.5 | yes |
