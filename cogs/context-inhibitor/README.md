# Context Inhibitor

CereBRO Phase 1, Layer 3 — Basal ganglia inhibitory gating.

## Algorithm

Default state: all findings INHIBITED. Each finding must earn disinhibition
through a 5-gate cascade:

1. **Casual hedge suppression** — CONFIDENCE_MISCALIBRATION findings with casual
   hedge words ("absolutely", "definitely", etc.) in informal contexts are
   suppressed regardless of severity.
2. **Severity auto-pass** — CRITICAL findings always disinhibit.
3. **Stakes gate** — Low urgency + low severity → suppress.
4. **Confidence gate** — WARNING findings need confidence above threshold.
5. **Corroboration gate** — Cross-detector agreement within proximity window.

## Impact

Baseline → CereBRO Phase 1:
- Precision: 0.64 → 0.82
- Recall: 1.00 → 1.00 (no TPs lost)
- F1: 0.78 → 0.90
- FP: 5 → 2

## Ports

| Port | Direction | Type |
|------|-----------|------|
| assessments | IN | CognitiveAssessment |
| conversation_snapshot | IN | ConversationSnapshot |
| gated_assessments | OUT | CognitiveAssessment |
| inhibition_log | OUT | InhibitionDecision |

## Config

| Field | Type | Default | Evolvable |
|-------|------|---------|-----------|
| corroboration_threshold | DOUBLE | 0.1 | yes |
| confidence_threshold_warn | DOUBLE | 0.55 | yes |
| formality_threshold | DOUBLE | 0.85 | yes |
| stakes_threshold | DOUBLE | 0.3 | yes |
| proximity_window_turns | UINT32 | 2 | yes |
