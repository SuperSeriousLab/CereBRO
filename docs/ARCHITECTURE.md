# CereBRO — Five-Layer Architecture Specification

> Engineering specification mapping brain structures to COG layers.
> Each layer has defined COGs, port contracts, latency budgets, and failure modes.

**Cross-references:** [PRINCIPLES.md](PRINCIPLES.md) (design rationale),
[NEW_COGS.md](NEW_COGS.md) (new COG specs), [CONTRACTS.md](CONTRACTS.md) (proto messages),
[BUILD_ORDER.md](BUILD_ORDER.md) (implementation sequence)

---

## Master Diagram

```
                        ┌─────────────────────────────────────┐
                        │           INPUT (raw bytes)          │
                        └──────────────┬──────────────────────┘
                                       │
                    ╔══════════════════╪══════════════════════╗
                    ║  LAYER 0: SENSORY  (<10ms)              ║
                    ║  ┌───────────────┐ ┌──────────┐ ┌─────┐║
                    ║  │FormatValidator│→│ToxicityGt│→│LangD│║
                    ║  └───────────────┘ └──────────┘ └─────┘║
                    ║  Brain: Brainstem sensory gating        ║
                    ╚══════════════════╪══════════════════════╝
                                       │ validated input
                    ╔══════════════════╪══════════════════════╗
                    ║  LAYER 1: THALAMIC RELAY  (<50ms)       ║
                    ║  ┌──────────┐  ┌────────┐  ┌─────────┐ ║
                    ║  │  Intake  │→ │ Router │  │ Urgency │ ║
                    ║  └──────────┘  └────┬───┘  └────┬────┘ ║
                    ║  Brain: Thalamus + RAS           │      ║
                    ╚══════════════════╪═══════════════╪══════╝
                              fan-out  │    GainSignal │
                    ╔══════════════════╪═══════════════╪══════╗
                    ║  LAYER 2: CORTICAL SPECIALISTS  (<500ms)║
                    ║  ┌──────┐ ┌──────┐ ┌──────┐ ┌────────┐ ║
                    ║  │Anchor│ │Contra│ │Scope │ │Confid. │ ║
                    ║  └──┬───┘ └──┬───┘ └──┬───┘ └───┬────┘ ║
                    ║  ┌──┴───┐ ┌──┴───┐    │         │      ║
                    ║  │Sunk$ │ │DecLdg│    │         │      ║
                    ║  └──┬───┘ └──┬───┘    │    ┌────┴────┐ ║
                    ║     │        │        │    │ClaimExt │ ║
                    ║  Brain: PFC, ACC, OFC, Parietal        ║
                    ╚══════════════════╪══════════════════════╝
                              fan-in   │ raw assessments
                    ╔══════════════════╪══════════════════════╗
                    ║  LAYER 3: INHIBITION & MODULATION       ║
                    ║            (<100ms)                      ║
                    ║  ┌──────────────┐ ┌────────┐ ┌────────┐║
                    ║  │CtxInhibitor  │ │Salience│ │ThrshMod│║
                    ║  └──────┬───────┘ └───┬────┘ └────────┘║
                    ║  Brain: Basal ganglia, Amygdala, LC     ║
                    ╚══════════════════╪══════════════════════╝
                              gated    │ findings
                    ╔══════════════════╪══════════════════════╗
                    ║  LAYER 4: INTEGRATION & META  (<200ms)  ║
                    ║  ┌──────────┐ ┌──────────┐ ┌─────────┐ ║
                    ║  │Aggregator│ │SelfConfid│ │Feedback │ ║
                    ║  └──────────┘ └──────────┘ └─────────┘ ║
                    ║  Brain: Prefrontal integration          ║
                    ╚══════════════════╪══════════════════════╝
                                       │ CerebroReport
                    ╔══════════════════╪══════════════════════╗
                    ║  LAYER 5: MEMORY & LEARNING  (async)    ║
                    ║  ┌──────────────┐  ┌──────────────────┐ ║
                    ║  │WorkingMemory │  │MemConsolidator   │ ║
                    ║  └──────────────┘  └────────┬─────────┘ ║
                    ║  Brain: Hippocampus          │→ Forge    ║
                    ╚═════════════════════════════════════════╝
```

---

## Layer 0: SENSORY (Brainstem Reflexes)

### Brain Structures

Models the brainstem's sensory gating — fast, reflexive rejection of harmful or
malformed input before it reaches higher processing.

- **Pain withdrawal reflex:** Spinal cord circuit, 10-15ms, no cortical involvement
  (Sherrington, 1906, *The Integrative Action of the Nervous System*)
- **Auditory startle:** Brainstem reticular formation, 5-10ms latency
  (Davis et al., 1982, *Neural Mechanisms of Startle Behavior*, Plenum Press)
- **Sensory gating (P50 suppression):** Brainstem/thalamic filtering of repeated
  stimuli (Freedman et al., 1987, "Neurobiological studies of sensory gating in
  schizophrenia," *Schizophrenia Bulletin*, 13:669-678)

**Mapping precision:** Direct computational mapping — brainstem reflexes are
pattern-matching circuits with fixed thresholds and no learning. Layer 0 COGs
are the same: fixed rules, no adaptation, instant response.

### COGs

| COG | Status | Function |
|-----|--------|----------|
| **format-validator** | NEW | UTF-8 validation, size limits, structure checks |
| **toxicity-gate** | NEW | Keyword/pattern toxicity screening (blocklist) |
| **language-detector** | NEW | Language identification, reject unsupported |

### Port Contracts

**Input:**
```
in: raw_input (bytes — raw message before any protobuf parsing)
```

**Output:**
```
out: validated_input (ValidationResult — see CONTRACTS.md)
     — contains: valid (bool), input_bytes (bytes), rejection_reason (string)
out: rejection (ValidationResult with valid=false)
```

Each Layer 0 COG is chained: FormatValidator → ToxicityGate → LanguageDetector.
If any rejects, the pipeline short-circuits and returns the rejection immediately.

### Latency Budget

**<10ms total for all three COGs.** No network calls, no I/O beyond reading the
input bytes. All checks are in-memory pattern matching.

### Failure Mode

If Layer 0 is removed: malformed input reaches expensive downstream layers,
potentially causing parsing errors or wasted computation. Toxic content passes
unfiltered. The pipeline still functions but wastes resources and may produce
findings on content that should have been rejected outright.

---

## Layer 1: THALAMIC RELAY

### Brain Structures

Models the thalamus and reticular activating system (RAS):

- **Thalamus:** Every sensory pathway (except olfaction) relays through the
  thalamus before reaching cortex. The thalamus doesn't just relay — it gates
  and prioritizes (Sherman & Guillery, 2002, "The role of the thalamus in the
  flow of information to the cortex," *Philosophical Transactions of the Royal
  Society B*, 357:1695-1708)
- **Reticular activating system:** Modulates arousal and attention. Projects
  diffusely to cortex, controlling the gain of cortical processing (Moruzzi &
  Magoun, 1949, "Brain stem reticular formation and activation of the EEG,"
  *Electroencephalography and Clinical Neurophysiology*, 1:455-473)
- **Thalamic reticular nucleus (TRN):** A shell of inhibitory neurons surrounding
  the thalamus. Selectively gates which thalamic relay neurons are active
  (Crick, 1984, "Function of the thalamic reticular complex: the searchlight
  hypothesis," *Proceedings of the National Academy of Sciences*, 81:4586-4590)

**Mapping precision:** The thalamic relay function is a direct mapping — Conversation
Intake and Cognitive Router literally relay and route enriched input to specialist
modules. The RAS arousal/gain function maps to the Urgency Assessor.

### COGs

| COG | Status | Function |
|-----|--------|----------|
| **conversation-intake** | EXISTS | Enriches ConversationSnapshot with metadata |
| **cognitive-router** | EXISTS | Classifies and fans out to relevant detectors |
| **urgency-assessor** | NEW | Emits GainSignal based on conversation context |

### Port Contracts

**conversation-intake:**
```
in:  conversation_snapshot (ConversationSnapshot)
out: enriched_snapshot (ConversationSnapshot — with TurnMetadata populated)
```

**cognitive-router:**
```
in:  conversation_snapshot (ConversationSnapshot)
out: anchoring_snapshot, sunk_cost_snapshot, contradiction_snapshot,
     scope_snapshot, calibration_snapshot, decision_snapshot
     (all ConversationSnapshot — routed copies)
```

**urgency-assessor (NEW):**
```
in:  conversation_snapshot (ConversationSnapshot)
out: gain_signal (GainSignal — see CONTRACTS.md)
```

The GainSignal contains:
- `urgency` (float, 0.0-1.0): How time-sensitive or high-stakes the conversation is
- `complexity` (float, 0.0-1.0): How structurally complex the argument is
- `formality` (float, 0.0-1.0): How formal the conversational register is
- `mode` (enum: PHASIC/TONIC): Whether to focus narrowly or scan broadly

See [CONTRACTS.md](CONTRACTS.md) for the full GainSignal specification.

**GainSignal delivery mechanism:** The brain uses volume transmission (diffuse
chemical release) for neuromodulation. The COG bus is point-to-point. CereBRO
bridges this with **N explicit wires** in the composition manifest — one wire from
`urgency-assessor.gain_signal` to each consumer:

```
# In composition manifest — one wire per consumer
wires { source_cog: "urgency-assessor"  source_port: "gain_signal"
        target_cog: "anchoring-detector" target_port: "gain_signal" }
wires { source_cog: "urgency-assessor"  source_port: "gain_signal"
        target_cog: "scope-guard"        target_port: "gain_signal" }
wires { source_cog: "urgency-assessor"  source_port: "gain_signal"
        target_cog: "contradiction-tracker" target_port: "gain_signal" }
# ... one wire per Layer 2 detector + context-inhibitor + threshold-modulator
```

This is verbose (9+ wires for gain alone) but correct — it uses existing
composition wiring with no new bus primitives. The Threshold Modulator receives
the same GainSignal and emits ThresholdAdjustments via the same point-to-point
pattern. A future optimization could add a `fan_out` port type to the composition
spec, but explicit wires are sufficient and auditable.

### Latency Budget

**<50ms total.** Intake and Router are proven fast (enrichment is mechanical
tokenization). Urgency Assessor must also be mechanical — keyword/pattern based,
no LLM calls.

### Failure Mode

If Layer 1 is removed: raw input reaches detectors without enrichment (missing
TurnMetadata, keywords, entity mentions). Detectors still function but with
degraded accuracy. Without the Router, all detectors receive all input regardless
of relevance — wasteful but not broken. Without the Urgency Assessor, all
conversations are treated with identical sensitivity — no context adaptation.

---

## Layer 2: CORTICAL SPECIALISTS

### Brain Structures

Maps to the specialized processing regions of the cerebral cortex. Each detector
corresponds to a cortical area with a specific computational function:

| COG | Brain Analogue | Justification |
|-----|---------------|---------------|
| **contradiction-tracker** | Anterior cingulate cortex (ACC) | ACC monitors for conflict between competing representations (Botvinick et al., 2001, "Conflict monitoring and cognitive control," *Psychological Review*, 108:624-652). Contradiction detection is literal conflict monitoring. **Direct mapping.** |
| **anchoring-detector** | Orbitofrontal cortex (OFC) | OFC represents value and detects when numerical estimates are biased by irrelevant reference points (Padoa-Schioppa & Assad, 2006, "Neurons in the orbitofrontal cortex encode economic value," *Nature*, 441:223-226). **Moderate mapping** — OFC does value encoding broadly; anchoring is a specific case. |
| **scope-guard** | Dorsolateral prefrontal cortex (DLPFC) | DLPFC maintains goals in working memory and monitors deviation (Curtis & D'Esposito, 2003, "Persistent activity in the prefrontal cortex during working memory," *Trends in Cognitive Sciences*, 7:415-423). Scope drift = goal deviation. **Direct mapping.** |
| **confidence-calibrator** | Posterior parietal cortex | Parietal cortex integrates evidence and tracks confidence (Kiani & Shadlen, 2009, "Representation of confidence associated with a decision by neurons in the parietal cortex," *Science*, 324:759-764). Calibration = confidence accuracy. **Direct mapping.** |
| **sunk-cost-detector** | Ventromedial prefrontal cortex (vmPFC) | vmPFC evaluates ongoing costs vs benefits and is implicated in sunk-cost behavior (Camerer & Weber, 1999, "The econometrics and behavioral economics of escalation of commitment," *Journal of Economic Behavior & Organization*, 39:59-82; Kable & Glimcher, 2007, "The neural correlates of subjective value during intertemporal choice," *Nature Neuroscience*, 10:1625-1633). **Moderate mapping.** |
| **decision-ledger** | Lateral prefrontal cortex | Tracks decision history, alternatives considered, and rationale. Maps to lateral PFC's role in maintaining structured representations of choices (Badre & D'Esposito, 2009, "Is the rostro-caudal axis of the frontal lobe hierarchical?" *Nature Reviews Neuroscience*, 10:659-669). **Loose mapping** — lateral PFC does many things. |
| **claim-extractor** | Wernicke's area / angular gyrus | Language comprehension and proposition extraction (Binder et al., 2009, "Where is the semantic system?" *Cerebral Cortex*, 19:2767-2796). This is the one HYBRID COG — uses LLM, like how language comprehension requires the full cortical language network. **Architectural analogy** more than computational mapping. |

### Gain Signal Integration

Each Layer 2 detector receives the GainSignal from the Urgency Assessor (Layer 1)
via a new input port: `gain_signal`. The gain modulates detection thresholds:

```
effective_threshold = base_threshold * (1.0 + gain_offset)
```

Where `gain_offset` is computed by the Threshold Modulator (Layer 3) from the
GainSignal. For high-urgency conversations, `gain_offset` is negative (lower
thresholds → more sensitive). For casual conversations, `gain_offset` is positive
(higher thresholds → fewer findings).

**Which threshold per detector:** Gain applies to the COG's **primary detection
threshold** only — the single config field that gates whether a finding is emitted.
Each detector identifies its primary threshold via a manifest annotation
(`gain_target: true` on the config field). The mapping:

| Detector | Primary Threshold (gain_target) | Other Fields (not modulated) |
|----------|-------------------------------|------------------------------|
| scope-guard | `drift_threshold` | sustained_turns, reference_window_size |
| anchoring-detector | `shift_threshold` | proximity_window |
| confidence-calibrator | `ece_threshold` | formality_weight |
| contradiction-tracker | `contradiction_threshold` | — |
| sunk-cost-detector | `cost_sensitivity` | — |
| decision-ledger | `decision_threshold` | — |

Secondary thresholds (sustained_turns, proximity_window, etc.) are structural
parameters, not sensitivity knobs — modulating them would change detector behavior
in ways that gain modulation shouldn't affect.

**Implementation note:** In Phase 1 (Context Inhibitor only), gain integration is
not yet built. Detectors run at their base thresholds. Gain integration is
added in Phase 2. This means the gain_signal port is defined but unused until
Phase 2 — this is acceptable per contracts-first design (ports exist before
code reads them).

### Port Contracts

All detectors share the same port pattern:
```
in:  conversation_snapshot (ConversationSnapshot)
in:  gain_signal (GainSignal) — optional, ignored if absent
out: assessment (CognitiveAssessment)
```

Claim Extractor additionally:
```
in:  conversation_snapshot (ConversationSnapshot)
out: argument_structure (ArgumentStructure)
```

### Latency Budget

**<500ms per detector.** Detectors run in parallel, so the Layer 2 latency is
the max of all activated detectors, not the sum. The Cognitive Router selectively
activates only relevant detectors, reducing average-case latency.

### Failure Mode

If Layer 2 is removed: no findings are generated. The pipeline produces an empty
report. This is the core analytical layer — without it, CereBRO is just input
validation and plumbing.

If a single detector fails: the pipeline produces a partial report (existing
behavior). The `detectors_timed_out` field in ReasoningReport tracks this.

---

## Layer 3: INHIBITION & MODULATION

### Brain Structures

| COG | Brain Structure | Reference |
|-----|----------------|-----------|
| **context-inhibitor** | Basal ganglia (striatum → GPi/GPe → thalamus) | Mink, 1996, "The basal ganglia: focused selection and inhibition of competing motor programs," *Progress in Neurobiology*, 50:381-425. Default state: tonic inhibition. Disinhibition requires evidence. See [PRINCIPLES.md](PRINCIPLES.md) §3. |
| **salience-filter** | Amygdala | The amygdala rapidly evaluates stimuli for biological relevance (salience) and modulates attention (LeDoux, 2000, "Emotion circuits in the brain," *Annual Review of Neuroscience*, 23:155-184; Phelps & LeDoux, 2005, "Contributions of the amygdala to emotion processing," *Trends in Cognitive Sciences*, 9:46-53). |
| **threshold-modulator** | Locus coeruleus (norepinephrine system) | Implements the gain modulation from the Urgency Assessor's signal. Maps to NE's multiplicative gain effect on cortical processing (Aston-Jones & Cohen, 2005). See [PRINCIPLES.md](PRINCIPLES.md) §4. |

**Mapping precision:** Context Inhibitor ↔ basal ganglia is the strongest mapping
in the entire architecture — both implement default-suppress with evidence-based
disinhibition. Salience Filter ↔ amygdala is moderate (amygdala is more complex
than novelty scoring). Threshold Modulator ↔ LC/NE is direct (both are
multiplicative gain modulators).

### COGs

| COG | Status | Function |
|-----|--------|----------|
| **context-inhibitor** | NEW | Default-suppress all findings; disinhibit only contextually relevant ones |
| **salience-filter** | NEW | Score findings by novelty and actionability; filter low-salience |
| **threshold-modulator** | NEW | Compute gain offsets from GainSignal; broadcast to Layer 2 |

### Context Inhibitor Algorithm

The Context Inhibitor is the most important new COG. Its algorithm:

1. **Receive** all raw CognitiveAssessments from Layer 2 + the ConversationSnapshot
2. **Default state:** all findings marked INHIBITED
3. **For each finding, evaluate disinhibition criteria:**

   a. **Corroboration score** (0.0-1.0): How many other detectors flagged
      overlapping turns? `score = overlapping_detectors / total_active_detectors`
      Threshold: >0.0 (at least one other detector must flag nearby turns)

   b. **Severity pass** (bool): CRITICAL findings always disinhibit. WARNING
      findings disinhibit if confidence > 0.7. CAUTION/INFO require corroboration.

   c. **Formality gate** (bool): If conversation formality < 0.3 (casual) AND
      finding is CONFIDENCE_MISCALIBRATION AND the trigger word is in the
      casual-hedging set {"absolutely", "definitely", "totally", "obviously",
      "literally"}, the finding remains INHIBITED regardless of other criteria.

   d. **Stakes gate** (bool): If GainSignal.urgency < 0.3 (low stakes) AND
      finding severity is INFO or CAUTION, the finding remains INHIBITED.

4. **Output:** InhibitionDecision per finding: DISINHIBITED or INHIBITED + reason

**Test case — the 3 false positives:**
- Input: casual conversation with "I'm absolutely sure we should go with option A"
- Formality score: <0.3 (short turns, no domain vocabulary, no hedging elsewhere)
- Finding: CONFIDENCE_MISCALIBRATION on "absolutely"
- Formality gate: trigger word "absolutely" is in casual-hedging set → INHIBITED
- Result: finding suppressed. Correct — casual certainty language is not miscalibration.

See [NEW_COGS.md](NEW_COGS.md) §5 for the full specification.

### Port Contracts

**context-inhibitor:**
```
in:  raw_assessments (repeated CognitiveAssessment)
in:  conversation_snapshot (ConversationSnapshot)
in:  gain_signal (GainSignal)
out: inhibition_decisions (repeated InhibitionDecision)
out: gated_assessments (repeated CognitiveAssessment — only DISINHIBITED ones)
```

**salience-filter:**
```
in:  gated_assessments (repeated CognitiveAssessment)
in:  conversation_snapshot (ConversationSnapshot)
out: salience_scores (repeated SalienceScore)
out: salient_assessments (repeated CognitiveAssessment — above salience threshold)
```

**threshold-modulator:**
```
in:  gain_signal (GainSignal)
out: threshold_adjustments (map<string, float> — detector_name → gain_offset)
```

### Latency Budget

**<100ms total** across three serial COGs:

| COG | Budget | Justification |
|-----|--------|---------------|
| Context Inhibitor | <50ms | 5 gates per finding, all in-memory boolean checks |
| Salience Filter | <30ms | Arithmetic scoring + sort of ≤20 findings |
| Threshold Modulator | <10ms | Single weighted sum from GainSignal |

**Execution order:** Context Inhibitor → Salience Filter (serial, since Salience
receives only the gated findings from the Inhibitor). Threshold Modulator runs
in parallel with both (it reads GainSignal, not findings). The critical path
is Inhibitor + Salience = <80ms, well within the <100ms budget.

All three are in-memory computations on small data (≤20 findings, ≤50 turns).

### Failure Mode

If Layer 3 is removed: all raw findings pass through to the Aggregator unfiltered.
The pipeline reverts to current behavior — functional but with higher false positive
rate. This is the **graceful degradation** property: Layer 3 is an improvement
layer, not a critical path. The pipeline worked before Layer 3 existed.

---

## Layer 4: INTEGRATION & META

### Brain Structures

| COG | Brain Structure | Reference |
|-----|----------------|-----------|
| **synthesis-aggregator** | Prefrontal integration | Miller & Cohen, 2001, "An integrative theory of prefrontal cortex function," *Annual Review of Neuroscience*, 24:167-202 — PFC integrates diverse inputs into a unified representation for action selection. |
| **self-confidence-assessor** | Metacognitive monitoring (anterior PFC) | Fleming et al., 2010, "Relating introspective accuracy to individual differences in brain structure," *Science*, 329:1541-1543 — anterior PFC gray matter volume correlates with metacognitive accuracy. |
| **feedback-evaluator** | Cortico-thalamic feedback | Rao & Ballard, 1999 (see [PRINCIPLES.md](PRINCIPLES.md) §5). One-pass predictive coding cycle. |

**Mapping precision:** Aggregator ↔ PFC integration is direct (both combine
diverse specialist outputs into a unified report). Self-Confidence ↔ metacognition
is direct (both are systems assessing their own reliability). Feedback Evaluator ↔
cortico-thalamic loops is a simplified analogy (we do one pass; the brain does
continuous feedback).

### COGs

| COG | Status | Function |
|-----|--------|----------|
| **synthesis-aggregator** | EXISTS | Combines assessments into ReasoningReport |
| **self-confidence-assessor** | NEW | Rates system confidence in the report |
| **feedback-evaluator** | NEW | Single re-evaluation pass for consistency |

### Self-Confidence Algorithm

The Self-Confidence Assessor computes a confidence score for the overall report:

```
confidence = w1 * agreement_score + w2 * margin_score + w3 * historical_score

agreement_score = 1.0 - (variance of finding confidences across detectors)
margin_score = mean(abs(finding.confidence - threshold) for each finding)
historical_score = accuracy_on_similar_patterns (from corpus lookup, if available)
```

Where:
- **Agreement:** If all detectors agree (all fired or none fired), confidence is
  high. Mixed signals reduce confidence.
- **Margin:** Findings far above threshold are more confident than borderline ones.
- **Historical:** If the corpus contains similar patterns with known outcomes,
  past accuracy informs current confidence.

Weights `w1, w2, w3` are evolvable via Forge.

### Feedback Protocol

The Feedback Evaluator implements bounded predictive coding:

1. **Trigger condition:** Self-confidence < `feedback_threshold` (default 0.6,
   evolvable). If self-confidence is high, skip feedback entirely.
2. **Generate FeedbackRequest:** Identifies which findings have the lowest
   confidence or highest inconsistency with the overall report.
3. **Re-evaluation:** Selected detectors receive a FeedbackRequest containing:
   - The original ConversationSnapshot
   - The first-pass synthesis (what other detectors found)
   - A specific question: "Given that [other findings], re-evaluate [this finding]"
4. **Second pass:** Re-evaluated assessments flow to the Aggregator for a
   second synthesis. The FeedbackResponse carries `pass_number = 2`.
5. **No further feedback.** The Aggregator only accepts pass_number ≤ 2.
   This is the accumulator pattern — bounded by protocol, not by convention.

### Port Contracts

**synthesis-aggregator:**
```
in:  assessment (CognitiveAssessment — fan-in from multiple detectors)
out: report (ReasoningReport)
```

**self-confidence-assessor:**
```
in:  report (ReasoningReport)
in:  gain_signal (GainSignal)
out: confidence_report (SelfConfidenceReport)
```

**feedback-evaluator:**
```
in:  report (ReasoningReport)
in:  confidence_report (SelfConfidenceReport)
out: feedback_request (FeedbackRequest) — sent to selected Layer 2 detectors
in:  feedback_response (FeedbackResponse) — from re-evaluated detectors
out: updated_report (ReasoningReport) — second-pass synthesis
```

### Latency Budget

**<200ms.** Aggregation is fast (existing, proven). Self-confidence computation
is arithmetic. The feedback loop, when triggered, adds the cost of re-running
selected detectors — but only those detectors with low-confidence findings,
not all detectors.

### Failure Mode

If Layer 4 is removed: no synthesis — individual detector findings arrive without
integration. Users see raw assessments instead of a prioritized report. The
pipeline produces useful output but without the meta-analysis that makes it
actionable.

If the Feedback Evaluator is removed specifically: the pipeline produces the
first-pass report only. This is the current behavior — functional, just without
the self-correction mechanism.

---

## Layer 5: MEMORY & LEARNING

### Brain Structures

| COG | Brain Structure | Reference |
|-----|----------------|-----------|
| **Working Memory (Conversation State Manager)** | Prefrontal working memory | Goldman-Rakic, 1995, "Cellular basis of working memory," *Neuron*, 14:477-485 — PFC neurons maintain task-relevant information across delays. |
| **memory-consolidator** | Hippocampus | Teyler & DiScenna, 1986 (indexing theory); McClelland et al., 1995 (complementary learning systems). See [PRINCIPLES.md](PRINCIPLES.md) §6. |

**Mapping precision:** Working memory ↔ PFC is direct (both maintain state across
processing). Memory Consolidator ↔ hippocampus is a strong structural analogy —
both create sparse indices rather than storing full experiences. The consolidation-
to-Forge connection is an architectural analogy (Forge is not neurologically similar
to sleep replay).

### COGs

| COG | Status | Function |
|-----|--------|----------|
| **conversation-intake** (state) | EXISTS | Maintains conversation state across turns |
| **memory-consolidator** | NEW | Creates sparse corpus entries from confirmed findings |

### Memory Consolidator Algorithm

The Memory Consolidator implements hippocampal indexing:

**What gets indexed (sparse index, not full conversation):**
- Finding types detected and their confidences
- Whether each finding was DISINHIBITED or INHIBITED (and why)
- Conversation metadata: turn count, formality score, urgency score, domain markers
- Detector agreement pattern (which detectors fired, which didn't)
- Outcome (if available): was the finding confirmed or rejected by user feedback?

**What does NOT get indexed:**
- Full conversation text (too large, privacy concerns)
- Raw turn content (only keywords and metadata)
- Intermediate processing state

**Consolidation triggers:**
1. **Explicit feedback:** User confirms or rejects a finding → immediate index entry
2. **High-confidence agreement:** All detectors agree with high confidence → auto-index
3. **Novel pattern:** Finding type/severity combination not seen before → auto-index
4. **Periodic sweep:** Every N pipeline runs, consolidate the highest-information entries

**Corpus entry format:**
```ndjson
{"conversation_id": "...", "finding_types": [...], "confidences": [...],
 "inhibited": [...], "formality": 0.7, "urgency": 0.3, "turn_count": 12,
 "outcome": "confirmed", "detector_pattern": "anchor+scope+contra"}
```

**Connection to Forge:**
Consolidated entries are appended to `data/corpus/*.ndjson`. The Forge evaluates
COG fitness against the full corpus (existing + new entries). This is the
Lamarckian loop: runtime experience → corpus → evolution → improved COGs.

### Port Contracts

**memory-consolidator:**
```
in:  cortex_report (CerebroReport — final pipeline output)
in:  inhibition_decisions (repeated InhibitionDecision — from Layer 3)
in:  conversation_metadata (ConversationSnapshot — for context features)
out: consolidation_entry (ConsolidationEntry — sparse index for corpus)
```

### Latency Budget

**Asynchronous.** The Memory Consolidator does not block the pipeline response.
It runs after the report is delivered, just as hippocampal consolidation occurs
after the experience.

### Failure Mode

If Layer 5 is removed: the pipeline has no memory. Each run is independent.
No learning occurs. The system still works — it just doesn't improve over time.
This is the current behavior. Layer 5 enables the Lamarckian advantage but is
not required for basic operation.

---

## Layer Interaction Summary

| From | To | Data | Purpose |
|------|----|------|---------|
| Layer 0 | Layer 1 | Validated input | Clean input for processing |
| Layer 1 | Layer 2 | Enriched snapshot + fan-out | Specialist activation |
| Layer 1 | Layer 3 | GainSignal | Context for modulation |
| Layer 2 | Layer 3 | Raw assessments | Findings for gating |
| Layer 3 | Layer 4 | Gated assessments | Filtered findings |
| Layer 4 | Layer 2 | FeedbackRequest | Re-evaluation (one pass) |
| Layer 4 | Layer 5 | CerebroReport | For consolidation |
| Layer 3 | Layer 5 | InhibitionDecisions | For learning what was suppressed |

## Graceful Degradation

Each layer can be removed without breaking the pipeline:

| Removed Layer | Impact | Equivalent To |
|--------------|--------|---------------|
| Layer 0 | Malformed/toxic input reaches detectors | Current behavior |
| Layer 1 (Urgency only) | No context adaptation | Current behavior |
| Layer 2 | No findings generated | Broken — this is the core |
| Layer 3 | Higher false positive rate | Current behavior (pre-CereBRO) |
| Layer 4 (Self-Conf + Feedback only) | No metacognition, no re-evaluation | Current behavior |
| Layer 5 | No learning, no improvement | Current behavior |

Only Layer 2 is absolutely required. Every other layer is an enhancement that
degrades gracefully. This is by design — it means CereBRO can be built incrementally
(see [BUILD_ORDER.md](BUILD_ORDER.md)).
