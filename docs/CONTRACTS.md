# CereBRO — Proto Contract Specifications

> All new protobuf messages and enums required by the CereBRO architecture.
> These are documentation-form specifications — the actual .proto file will be
> written after review.

**Cross-references:** [ARCHITECTURE.md](ARCHITECTURE.md) (where each message flows),
[NEW_COGS.md](NEW_COGS.md) (which COGs produce/consume each message),
existing contracts in `proto/cog/reasoning/v1/reasoning.proto`

---

## Package

```
package: cerebro.v1
go_package: "github.com/SuperSeriousLab/GEARS/gen/go/cerebro/v1;cerebrov1"
```

Imports: `cog.reasoning.v1` (for CognitiveAssessment, ReasoningReport,
ConversationSnapshot, FindingType, FindingSeverity)

---

## New Enums

### GainMode

The operating mode of the neuromodulatory system.

```
enum GainMode {
  GAIN_MODE_UNSPECIFIED = 0;
  PHASIC = 1;   // Focused, high-gain processing (high urgency, narrow attention)
  TONIC = 2;    // Broad, low-gain scanning (low urgency, wide attention)
}
```

**Used by:** GainSignal (field 4)

### InhibitionAction

The gating decision for a single finding.

```
enum InhibitionAction {
  INHIBITION_ACTION_UNSPECIFIED = 0;
  INHIBITED = 1;      // Finding suppressed (default state)
  DISINHIBITED = 2;   // Finding passed through (evidence sufficient)
}
```

**Used by:** InhibitionDecision (field 2)

### ConsolidationTrigger

What caused a memory consolidation event.

```
enum ConsolidationTrigger {
  CONSOLIDATION_TRIGGER_UNSPECIFIED = 0;
  USER_FEEDBACK = 1;      // Explicit user confirm/reject
  HIGH_CONFIDENCE = 2;    // All findings above confidence threshold
  NOVEL_PATTERN = 3;      // Finding combination not seen before
  PERIODIC_SWEEP = 4;     // Scheduled consolidation
}
```

**Used by:** ConsolidationEntry (field 3)

### ConfidenceRecommendation

System's recommendation based on self-assessed confidence.

```
enum ConfidenceRecommendation {
  CONFIDENCE_RECOMMENDATION_UNSPECIFIED = 0;
  HIGH_CONFIDENCE = 1;                   // Report is reliable
  MODERATE_CONFIDENCE = 2;               // Report is likely correct
  LOW_CONFIDENCE_REVIEW_RECOMMENDED = 3; // Human review recommended
}
```

**Used by:** SelfConfidenceReport (field 7)

---

## Layer 0 Messages

### ValidationResult

Output of the format-validator COG.

```
message ValidationResult {
  bool valid = 1;                         // true if input passed validation
  bytes input_bytes = 2;                  // original input (passed through if valid)
  string rejection_reason = 3;           // empty if valid, descriptive if rejected
  uint32 input_size_bytes = 4;           // size of input for monitoring
}
```

**Produced by:** format-validator
**Consumed by:** toxicity-gate (if valid)

### ToxicityResult

Output of the toxicity-gate COG.

```
message ToxicityResult {
  bool toxic = 1;                         // true if toxicity detected
  bytes input_bytes = 2;                  // original input (passed through if clean)
  repeated string matched_patterns = 3;  // which blocklist entries matched
  double toxicity_score = 4;             // 0.0-1.0, density of toxic patterns
}
```

**Produced by:** toxicity-gate
**Consumed by:** language-detector (if not toxic)

### LanguageResult

Output of the language-detector COG.

```
message LanguageResult {
  string language_code = 1;              // ISO 639-1 (e.g., "en", "de", "fr")
  double confidence = 2;                 // 0.0-1.0, detection confidence
  bool supported = 3;                    // true if language is in supported set
  bytes input_bytes = 4;                 // original input (passed through if supported)
}
```

**Produced by:** language-detector
**Consumed by:** Layer 1 (conversation-intake)

---

## Layer 1 Messages

### GainSignal

Broadcast from the urgency-assessor to all Layer 2 and Layer 3 COGs.
Models the norepinephrine gain modulation signal.

```
message GainSignal {
  double urgency = 1;                    // 0.0-1.0, how time-sensitive/high-stakes
                                          //   0.0 = casual chat
                                          //   0.5 = standard professional discussion
                                          //   1.0 = emergency/critical decision
  double complexity = 2;                  // 0.0-1.0, structural complexity of argument
                                          //   0.0 = simple question/answer
                                          //   0.5 = multi-turn discussion
                                          //   1.0 = complex multi-party negotiation
  double formality = 3;                   // 0.0-1.0, conversational register
                                          //   0.0 = very informal (chat, slang)
                                          //   0.5 = neutral professional
                                          //   1.0 = highly formal (legal, academic)
  GainMode mode = 4;                     // PHASIC (focused) or TONIC (broad)
  string conversation_id = 5;            // for correlation
}
```

**Produced by:** urgency-assessor
**Consumed by:** All Layer 2 detectors (via gain_signal port), context-inhibitor,
threshold-modulator, self-confidence-assessor

---

## Layer 3 Messages

### InhibitionDecision

Per-finding gating decision from the context-inhibitor.

```
message InhibitionDecision {
  string finding_id = 1;                 // identifies which CognitiveAssessment
  InhibitionAction action = 2;           // INHIBITED or DISINHIBITED
  string reason = 3;                     // machine-readable reason code:
                                          //   "severity_auto_pass"
                                          //   "casual_hedge_in_informal_context"
                                          //   "low_stakes_low_severity"
                                          //   "warning_below_confidence_threshold"
                                          //   "insufficient_corroboration"
                                          //   "all_gates_passed"
  double corroboration_score = 4;        // 0.0-1.0, how much cross-detector support
  string detector_name = 5;              // which detector produced the finding
  FindingType finding_type = 6;          // for audit trail
}
```

**Produced by:** context-inhibitor
**Consumed by:** synthesis-aggregator (only DISINHIBITED findings), memory-consolidator
(full decisions for learning)

### SalienceScore

Per-finding salience rating from the salience-filter.

```
message SalienceScore {
  string finding_id = 1;                 // identifies which CognitiveAssessment
  double score = 2;                      // 0.0-1.0, composite salience
  double novelty = 3;                    // 0.0-1.0, how unique this finding type is
  double actionability = 4;              // 0.0-1.0, how actionable (has evidence, refs)
  bool above_threshold = 5;             // true if score >= min_salience config
}
```

**Produced by:** salience-filter
**Consumed by:** synthesis-aggregator (for ranking), cerebro-report (included in output)

### ThresholdAdjustments

Gain offset broadcast from threshold-modulator to Layer 2 detectors.

```
message ThresholdAdjustments {
  map<string, double> adjustments = 1;   // detector_name → gain_offset
                                          //   negative = more sensitive (lower threshold)
                                          //   positive = less sensitive (higher threshold)
                                          //   range: [-0.5, +0.5]
  GainMode source_mode = 2;              // which mode generated these adjustments
}
```

**Produced by:** threshold-modulator
**Consumed by:** All Layer 2 detectors (via threshold adjustment port)

---

## Layer 4 Messages

### SelfConfidenceReport

System's assessment of its own report reliability.

```
message SelfConfidenceReport {
  double overall_confidence = 1;         // 0.0-1.0, weighted composite
  double agreement_score = 2;            // 0.0-1.0, cross-detector agreement
  double margin_score = 3;               // 0.0-1.0, distance from detection thresholds
  double historical_score = 4;           // 0.0-1.0, accuracy on similar patterns
  uint32 finding_count = 5;             // how many findings in the report
  string finding_pattern = 6;           // e.g., "ANCHORING_BIAS+SCOPE_DRIFT"
  ConfidenceRecommendation recommendation = 7;
}
```

**Produced by:** self-confidence-assessor
**Consumed by:** feedback-evaluator (triggers re-evaluation if low), cerebro-report

### FeedbackRequest

Request from feedback-evaluator to Layer 2 detectors for re-evaluation.

```
message FeedbackRequest {
  uint32 pass_number = 1;               // always 2 (accumulator bound)
  repeated string reeval_finding_ids = 2; // which findings to re-evaluate
  ReasoningReport original_report = 3;   // first-pass synthesis for context
  ConversationSnapshot context = 4;      // original conversation
  repeated string synthesis_context = 5; // recommended_actions from first pass
  string request_id = 6;                // for correlation
}
```

**Produced by:** feedback-evaluator
**Consumed by:** Selected Layer 2 detectors (those that produced low-confidence findings)

### FeedbackResponse

Response from a re-evaluated detector.

```
message FeedbackResponse {
  uint32 pass_number = 1;               // always 2 (matches request)
  string finding_id = 2;                // which finding was re-evaluated
  CognitiveAssessment updated = 3;      // the re-evaluated assessment
  double confidence_delta = 4;          // change from original confidence
  string request_id = 5;               // correlation with FeedbackRequest
}
```

**Produced by:** Layer 2 detectors (on receiving FeedbackRequest)
**Consumed by:** feedback-evaluator (integrates into updated report)

---

## Layer 5 Messages

### ConsolidationEntry

Sparse index entry created by the memory-consolidator for corpus storage.

```
message ConsolidationEntry {
  string conversation_id = 1;           // conversation identifier
  google.protobuf.Timestamp timestamp = 2;
  ConsolidationTrigger trigger = 3;

  // Finding summary (sparse — not full assessments)
  repeated FindingType finding_types = 4;
  repeated double finding_confidences = 5;
  repeated FindingSeverity finding_severities = 6;

  // Inhibition summary
  uint32 inhibited_count = 7;
  uint32 disinhibited_count = 8;
  repeated string inhibition_reasons = 9;

  // Conversation metadata (not content)
  uint32 turn_count = 10;
  double formality = 11;                // 0.0-1.0
  double urgency = 12;                  // 0.0-1.0
  repeated string domain_markers = 13;  // extracted domain keywords

  // Outcome
  string outcome = 14;                  // "confirmed", "rejected", "auto_confirmed"
  double outcome_confidence = 15;       // 0.0-1.0

  // Detector pattern
  string detector_pattern = 16;         // e.g., "anchoring+scope+contra"
}
```

**Produced by:** memory-consolidator
**Consumed by:** Forge corpus (appended to NDJSON), self-confidence-assessor
(historical accuracy lookup)

---

## Top-Level Output

### CerebroReport

The final output of the CereBRO pipeline. Wraps the existing ReasoningReport and
adds CereBRO-specific metadata.

**Backward compatibility:** CerebroReport embeds a `ReasoningReport` as field 1.
Consumers that only understand ReasoningReport can extract it directly — they
don't need to know about CereBRO extensions. This is embed-and-extend, not
field-number duplication. A CerebroReport is NOT wire-compatible with a
ReasoningReport (they are different protobuf types). The pipeline emits
CerebroReport; the cognitive-gateway can extract and return the embedded
ReasoningReport to legacy consumers.

```
message CerebroReport {
  // Embedded base report — backward compatible extraction point
  cog.reasoning.v1.ReasoningReport base_report = 1;

  // CereBRO extensions
  SelfConfidenceReport self_confidence = 2;     // metacognitive assessment
  repeated InhibitionDecision inhibition_log = 3; // full inhibition audit trail
  repeated SalienceScore salience_scores = 4;    // per-finding salience
  GainSignal gain_signal = 5;                    // context that influenced processing
  bool feedback_applied = 6;                     // whether re-evaluation occurred
  uint32 pass_count = 7;                         // 1 (no feedback) or 2 (with feedback)

  // Layer 0 metadata
  string detected_language = 8;
  double language_confidence = 9;

  // Consolidation status
  bool consolidated = 10;                        // whether this run was indexed
  ConsolidationTrigger consolidation_trigger = 11;
}
```

**Produced by:** Layer 4 integration (replacing ReasoningReport at pipeline output)
**Consumed by:** cognitive-gateway (returned to caller), memory-consolidator

### Backward Compatibility

CerebroReport is a superset of ReasoningReport. Fields 1-15 are identical in
meaning and position. Consumers that read ReasoningReport can read CerebroReport
by ignoring fields 16+. This ensures the existing cognitive-gateway and any
external consumers continue to work during the transition.

---

## Message Flow Diagram

```
Layer 0: bytes → ValidationResult → ToxicityResult → LanguageResult
                                                          │
Layer 1: ConversationSnapshot ← parse ─────────────────────┘
         │
         ├→ GainSignal (broadcast)
         │
Layer 2: ConversationSnapshot → CognitiveAssessment (per detector)
         (+ GainSignal in)         │
                                    │
Layer 3: CognitiveAssessment[] ──→ InhibitionDecision[]
         + GainSignal              + gated CognitiveAssessment[]
         + ConversationSnapshot        │
                                       ├→ SalienceScore[]
                                       │
Layer 4: CognitiveAssessment[] ──→ ReasoningReport
                                    │→ SelfConfidenceReport
                                    │→ FeedbackRequest (if low confidence)
                                    │← FeedbackResponse
                                    │→ CerebroReport (final)
                                        │
Layer 5: CerebroReport ──────────→ ConsolidationEntry (→ corpus NDJSON)
```

---

## Proto File Location

When approved, the proto file will be:
```
proto/cerebro/v1/cerebro.proto
```

Generated Go code:
```
gen/go/cerebro/v1/cerebrov1/
```

This follows the existing pattern: `proto/{domain}/{version}/{file}.proto`.
