# Feedback Evaluator

CereBRO Phase 2, Layer 5 -- Feedback Loop.

## Algorithm

Checks the self-confidence report. If overall confidence falls below the
feedback threshold, selectively re-runs detectors on the weakest findings:

1. **Threshold check** -- If self-confidence >= threshold, pass findings through
   unchanged.
2. **Finding selection** -- Select up to `max_reeval_findings` lowest-confidence
   findings for re-evaluation.
3. **Re-evaluation** -- Re-run the corresponding detector on the conversation
   snapshot.
4. **Confidence delta** -- Accept re-evaluated findings only if confidence
   improves by at least `confidence_improvement_min`.

## Ports

| Port | Direction | Type |
|------|-----------|------|
| findings | IN | CognitiveAssessment |
| self_confidence | IN | SelfConfidenceReport |
| conversation_snapshot | IN | ConversationSnapshot |
| reasoning_report | IN | ReasoningReport |
| feedback_result | OUT | string (FeedbackResult is Go-only) |

## Config

| Field | Type | Default | Evolvable |
|-------|------|---------|-----------|
| feedback_threshold | DOUBLE | 0.6 | yes |
| max_reeval_findings | UINT32 | 3 | yes |
| confidence_improvement_min | DOUBLE | 0.1 | yes |
