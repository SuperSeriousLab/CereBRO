# CereBRO — New COG Specifications

> Full specifications for the 10 new COGs required by the CereBRO architecture.
> Each is specified to implementation-ready detail.

**Cross-references:** [ARCHITECTURE.md](ARCHITECTURE.md) (layer placement),
[CONTRACTS.md](CONTRACTS.md) (proto messages), [BUILD_ORDER.md](BUILD_ORDER.md) (implementation order)

---

## 1. format-validator (Layer 0)

**Description:** Validates that input is well-formed UTF-8 within size limits
before any protobuf parsing.

**Brain analogue:** Peripheral sensory receptor gating — malformed stimuli
(e.g., light outside visible spectrum) are rejected at the receptor level before
any neural processing occurs (Kandel et al., 2013, *Principles of Neural Science*,
5th ed., McGraw-Hill, Chapter 21: sensory transduction).

**Determinism:** PURE
**Language:** Go
**Estimated LOC:** ~150

### Port Contracts

```
in:  raw_input (bytes)
out: validated_input (ValidationResult) — valid=true, input passed through
out: rejection (ValidationResult) — valid=false, rejection_reason set
```

### Config Fields

```
max_input_bytes:    type=UINT32  default="1048576"  evolvable=false  # 1MB
max_turns:          type=UINT32  default="500"      evolvable=false
require_utf8:       type=BOOL    default="true"     evolvable=false
```

These are not evolvable — they are safety limits, not performance parameters.

### Algorithm

```pseudocode
function validate(input: bytes) -> ValidationResult:
    if len(input) > max_input_bytes:
        return reject("input exceeds size limit: {len} > {max}")

    if require_utf8 and not is_valid_utf8(input):
        return reject("input is not valid UTF-8")

    # Attempt protobuf parse of ConversationSnapshot
    snapshot = try_parse(input)
    if snapshot.error:
        return reject("invalid ConversationSnapshot: {error}")

    if len(snapshot.turns) > max_turns:
        return reject("turn count exceeds limit: {count} > {max}")

    if snapshot.turns is empty:
        return reject("conversation has no turns")

    return accept(input)
```

### Test Cases

1. **Positive:** Valid UTF-8 ConversationSnapshot with 5 turns → accepted
2. **Negative:** Input with invalid UTF-8 byte sequence (0xFF 0xFE mid-string) → rejected
3. **Edge:** Exactly max_input_bytes → accepted; max_input_bytes + 1 → rejected

### Reuses

Pattern from cognitive-gateway (input validation before processing).

---

## 2. toxicity-gate (Layer 0)

**Description:** Fast keyword and pattern-based toxicity screening using a
configurable blocklist. No LLM — pure string matching.

**Brain analogue:** Nociceptive withdrawal reflex — harmful stimuli trigger
immediate withdrawal before conscious processing (Sherrington, 1906, *The
Integrative Action of the Nervous System*; Purves et al., 2018, *Neuroscience*,
6th ed., Sinauer, Chapter 10).

**Determinism:** PURE
**Language:** Go
**Estimated LOC:** ~250

### Port Contracts

```
in:  validated_input (ValidationResult with valid=true)
out: passed_input (ToxicityResult) — toxic=false, input passed through
out: blocked (ToxicityResult) — toxic=true, matched_patterns listed
```

### Config Fields

```
blocklist_path:     type=STRING  default="data/blocklists/default.txt"  evolvable=false
case_sensitive:     type=BOOL    default="false"                         evolvable=false
min_match_length:   type=UINT32  default="3"                             evolvable=false
max_false_positive: type=DOUBLE  default="0.01"                          evolvable=true  min="0.001" max="0.05"
```

### Algorithm

```pseudocode
function screen(input: ValidationResult) -> ToxicityResult:
    text = extract_all_turn_text(input)
    text_lower = to_lowercase(text) if not case_sensitive

    matches = []
    for pattern in blocklist:
        if len(pattern) < min_match_length:
            continue
        if pattern found in text_lower:
            # Check word boundaries to avoid substring false positives
            if is_word_boundary_match(text_lower, pattern):
                matches.append(pattern)

    if len(matches) > 0:
        return ToxicityResult{toxic: true, matched_patterns: matches}

    return ToxicityResult{toxic: false}
```

### Test Cases

1. **Positive:** Input containing blocklisted slur → blocked, matched_patterns includes the term
2. **Negative:** Clean technical discussion about "killing a process" → passes (word boundary check prevents false match on "kill" substring when "kill" isn't on blocklist as exact term)
3. **Edge:** Input with blocklisted word embedded in a longer word (e.g., "assassinate" containing "ass") → passes due to word boundary matching

### Reuses

Pattern from scope-guard (keyword/pattern matching against a reference set).

---

## 3. language-detector (Layer 0)

**Description:** Identifies the primary language of the input text and rejects
unsupported languages.

**Brain analogue:** Thalamic sensory filtering — the thalamus routes sensory
input to the appropriate cortical processing area. Input that cannot be routed
(e.g., frequencies outside auditory range) is not processed (Sherman & Guillery,
2002, *Philosophical Transactions*, 357:1695-1708).

**Determinism:** PURE
**Language:** Go
**Estimated LOC:** ~200

### Port Contracts

```
in:  passed_input (ToxicityResult with toxic=false)
out: detected (LanguageResult) — language code, confidence, input passed through
out: unsupported (LanguageResult) — detected language not in supported set
```

### Config Fields

```
supported_languages: type=STRING  default="en"         evolvable=false  # comma-separated ISO 639-1
min_confidence:      type=DOUBLE  default="0.7"        evolvable=true   min="0.3" max="0.95"
fallback_language:   type=STRING  default="en"         evolvable=false
min_sample_chars:    type=UINT32  default="20"         evolvable=false
```

### Algorithm

```pseudocode
function detect(input: ToxicityResult) -> LanguageResult:
    text = extract_sample_text(input, max_chars=500)

    if len(text) < min_sample_chars:
        return LanguageResult{language: fallback_language, confidence: 0.5, supported: true}

    # Trigram frequency analysis (no external dependencies)
    trigram_counts = count_trigrams(text)
    scores = {}
    for lang in language_profiles:
        scores[lang] = cosine_similarity(trigram_counts, lang.profile)

    best_lang = max_by_score(scores)
    confidence = scores[best_lang]

    if confidence < min_confidence:
        return LanguageResult{language: "unknown", confidence: confidence, supported: false}

    supported = best_lang in supported_languages
    return LanguageResult{language: best_lang, confidence: confidence, supported: supported}
```

Language profiles are pre-computed trigram frequency distributions for each
supported language (standard technique — Cavnar & Trenkle, 1994, "N-gram-based
text categorization," *SDAIR-94*).

### Test Cases

1. **Positive:** English text → detected as "en" with confidence > 0.9
2. **Negative:** German text when only "en" is supported → rejected as unsupported
3. **Edge:** Very short input (< min_sample_chars) → defaults to fallback_language

### Reuses

Trigram counting pattern reusable from conversation-intake (keyword extraction).

---

## 4. urgency-assessor (Layer 1)

**Description:** Reads conversation context and emits a GainSignal that modulates
Layer 2 detector sensitivity.

**Brain analogue:** Locus coeruleus norepinephrine system — phasic NE bursts
enhance processing of salient stimuli; tonic NE promotes broad, unfocused
scanning (Aston-Jones & Cohen, 2005, "An integrative theory of locus coeruleus–
norepinephrine function," *Annual Review of Neuroscience*, 28:403-450; Berridge
& Waterhouse, 2003, *Brain Research Reviews*, 42:33-84).

**Determinism:** PURE
**Language:** Go
**Estimated LOC:** ~300

### Port Contracts

```
in:  conversation_snapshot (ConversationSnapshot)
out: gain_signal (GainSignal)
```

### Config Fields

```
urgency_keywords:        type=STRING  default="urgent,critical,deadline,asap,emergency,risk,liability,legal"  evolvable=false
stakes_keywords:         type=STRING  default="million,billion,contract,lawsuit,patient,safety,security"      evolvable=false
# Note: keyword lists are comma-separated STRING in COG config (not repeated
# STRING) because COG config fields are flat key-value pairs. The proto
# ConfigField type supports STRING with comma-separated convention. At runtime,
# the COG splits on comma. This matches the pattern used by existing COGs
# (scope-guard objective_keywords, anchoring-detector context_patterns).
complexity_turn_threshold: type=UINT32  default="10"    evolvable=true  min="3"   max="50"
formality_threshold:     type=DOUBLE  default="0.5"     evolvable=true  min="0.1" max="0.9"
```

### Algorithm

```pseudocode
function assess(snapshot: ConversationSnapshot) -> GainSignal:
    # Urgency: keyword density in recent turns
    recent_text = join(turn.raw_text for turn in snapshot.turns[-5:])
    urgency_hits = count_keyword_hits(recent_text, urgency_keywords)
    stakes_hits = count_keyword_hits(recent_text, stakes_keywords)
    urgency = clamp((urgency_hits + stakes_hits * 2) / 10.0, 0.0, 1.0)

    # Complexity: structural indicators
    total_turns = len(snapshot.turns)
    unique_speakers = count_distinct(turn.speaker for turn in snapshot.turns)
    avg_turn_length = mean(turn.metadata.word_count for turn in snapshot.turns)
    complexity = clamp(
        (total_turns / complexity_turn_threshold * 0.4) +
        (unique_speakers / 5.0 * 0.2) +
        (avg_turn_length / 200.0 * 0.4),
        0.0, 1.0
    )

    # Formality: presence of hedging, formal vocabulary, sentence structure
    formality = compute_formality(snapshot)  # see below

    # Mode: PHASIC if urgency is high and focused, TONIC otherwise
    mode = PHASIC if urgency > 0.6 else TONIC

    return GainSignal{urgency, complexity, formality, mode}

function compute_formality(snapshot) -> float:
    text = join_all_turns(snapshot)
    formal_markers = count_matches(text, formal_patterns)
    # formal_patterns: hedging phrases, passive voice, technical vocabulary,
    # long sentences, absence of contractions/slang
    informal_markers = count_matches(text, informal_patterns)
    # informal_patterns: contractions, slang, emoji, exclamation marks,
    # very short sentences, casual hedges ("I guess", "kinda", "lol")
    total = formal_markers + informal_markers
    if total == 0:
        return 0.5  # neutral
    return clamp(formal_markers / total, 0.0, 1.0)
```

### Test Cases

1. **Positive:** "We have a critical security vulnerability that needs to be patched before the deadline tomorrow" → urgency=0.8+, mode=PHASIC
2. **Negative:** "Hey, just wondering if you think we should maybe try a different color for the button" → urgency<0.2, formality<0.3, mode=TONIC
3. **Edge:** Long technical discussion (50 turns) with no urgency keywords → urgency=0.1, complexity=0.8, mode=TONIC (complex but not urgent)

### Reuses

Keyword matching from scope-guard. Formality computation is new but uses the
same tokenization patterns as conversation-intake.

---

## 5. context-inhibitor (Layer 3)

> **This is the most important new COG in CereBRO.** It implements basal ganglia
> inhibitory gating and is expected to deliver the largest single precision
> improvement.

**Description:** Default-suppresses all findings from Layer 2 detectors, then
selectively disinhibits only those that pass mechanical contextual relevance
criteria.

**Brain analogue:** Basal ganglia direct/indirect pathway — the globus pallidus
internus (GPi) tonically inhibits thalamocortical projections. The striatum must
accumulate sufficient evidence to inhibit the GPi (disinhibition), releasing
selected actions while maintaining suppression of competitors (Mink, 1996,
*Progress in Neurobiology*, 50:381-425; Redgrave et al., 1999, *Neuroscience*,
89:1009-1023; Hikosaka et al., 2000, *Physiological Reviews*, 80:953-978).

**Determinism:** PURE
**Language:** Go
**Estimated LOC:** ~500

**LOC note:** The 300 LOC soft limit applies to domain-specific detector COGs
(Layer 2). Infrastructure and gating COGs (Layers 0, 1, 3, 4, 5) follow the
general 500 LOC methodology limit. The Context Inhibitor's 5-gate algorithm
justifies the size — splitting it into smaller COGs would create artificial
lateral dependencies between gates that must execute sequentially on the same
finding set.

### Port Contracts

```
in:  raw_assessments (repeated CognitiveAssessment — all findings from Layer 2)
in:  conversation_snapshot (ConversationSnapshot — for context features)
in:  gain_signal (GainSignal — from Urgency Assessor)
out: inhibition_decisions (repeated InhibitionDecision — per-finding decision)
out: gated_assessments (repeated CognitiveAssessment — only disinhibited findings)
```

### Config Fields

```
corroboration_threshold:     type=DOUBLE  default="0.1"   evolvable=true  min="0.0"  max="0.5"
severity_auto_pass:          type=STRING  default="CRITICAL"              evolvable=false
confidence_threshold_warn:   type=DOUBLE  default="0.7"   evolvable=true  min="0.3"  max="0.95"
formality_threshold:         type=DOUBLE  default="0.3"   evolvable=true  min="0.1"  max="0.7"
stakes_threshold:            type=DOUBLE  default="0.3"   evolvable=true  min="0.1"  max="0.7"
casual_hedge_words:          type=STRING  default="absolutely,definitely,totally,obviously,literally,clearly,certainly"  evolvable=false
proximity_window_turns:      type=UINT32  default="2"     evolvable=true  min="1"    max="5"
```

### Algorithm (Full Detail)

```pseudocode
function inhibit(
    assessments: []CognitiveAssessment,
    snapshot: ConversationSnapshot,
    gain: GainSignal
) -> ([]InhibitionDecision, []CognitiveAssessment):

    decisions = []
    gated = []

    # Pre-compute context features
    active_detectors = distinct(a.detector_name for a in assessments)
    turn_findings = group_by_turns(assessments)  # map[turn_number] -> []Assessment

    for assessment in assessments:
        decision = evaluate_disinhibition(assessment, assessments, snapshot, gain,
                                           active_detectors, turn_findings)
        decisions.append(decision)
        if decision.action == DISINHIBITED:
            gated.append(assessment)

    return decisions, gated

function evaluate_disinhibition(
    finding: CognitiveAssessment,
    all_findings: []CognitiveAssessment,
    snapshot: ConversationSnapshot,
    gain: GainSignal,
    active_detectors: set[string],
    turn_findings: map[uint32][]CognitiveAssessment
) -> InhibitionDecision:

    # === GATE 1: Severity auto-pass ===
    # CRITICAL findings always disinhibit (like a pain signal that
    # bypasses basal ganglia gating entirely)
    if finding.severity >= severity_auto_pass:
        return InhibitionDecision{
            action: DISINHIBITED,
            reason: "severity_auto_pass",
            finding_id: finding_id(finding)
        }

    # === GATE 2: Casual hedging suppression ===
    # If the conversation is informal AND the finding is
    # CONFIDENCE_MISCALIBRATION AND the trigger word is casual,
    # suppress regardless of other gates.
    if finding.finding_type == CONFIDENCE_MISCALIBRATION:
        if gain.formality < formality_threshold:
            trigger_text = extract_trigger_text(finding, snapshot)
            if contains_casual_hedge(trigger_text, casual_hedge_words):
                return InhibitionDecision{
                    action: INHIBITED,
                    reason: "casual_hedge_in_informal_context",
                    finding_id: finding_id(finding)
                }

    # === GATE 3: Stakes gate ===
    # In low-stakes conversations, only WARNING+ findings pass
    if gain.urgency < stakes_threshold:
        if finding.severity <= CAUTION:
            return InhibitionDecision{
                action: INHIBITED,
                reason: "low_stakes_low_severity",
                finding_id: finding_id(finding)
            }

    # === GATE 4: Confidence gate ===
    # WARNING findings must have confidence above threshold
    if finding.severity == WARNING:
        if finding.confidence < confidence_threshold_warn:
            return InhibitionDecision{
                action: INHIBITED,
                reason: "warning_below_confidence_threshold",
                finding_id: finding_id(finding)
            }

    # === GATE 5: Corroboration gate ===
    # At least one other detector must have flagged overlapping turns
    # (within proximity_window_turns). This implements the "focused
    # selection" aspect — a finding needs converging evidence.
    corroboration = compute_corroboration(finding, all_findings,
                                           active_detectors,
                                           turn_findings,
                                           proximity_window_turns)
    if corroboration < corroboration_threshold:
        # Exception: if confidence is very high (>0.9), allow solo findings
        if finding.confidence <= 0.9:
            return InhibitionDecision{
                action: INHIBITED,
                reason: "insufficient_corroboration",
                finding_id: finding_id(finding)
            }

    # === All gates passed: DISINHIBIT ===
    return InhibitionDecision{
        action: DISINHIBITED,
        reason: "all_gates_passed",
        finding_id: finding_id(finding)
    }

function compute_corroboration(
    finding: CognitiveAssessment,
    all_findings: []CognitiveAssessment,
    active_detectors: set[string],
    turn_findings: map[uint32][]CognitiveAssessment,
    window: uint32
) -> float:
    """How many other detectors flagged turns near this finding's turns?"""
    my_turns = set(finding.relevant_turns)
    nearby_turns = expand_window(my_turns, window)
    corroborating_detectors = set()
    for turn in nearby_turns:
        for other in turn_findings.get(turn, []):
            if other.detector_name != finding.detector_name:
                corroborating_detectors.add(other.detector_name)
    other_detector_count = len(active_detectors) - 1
    if other_detector_count == 0:
        return 1.0  # only one detector active — can't require corroboration
    return len(corroborating_detectors) / other_detector_count

function contains_casual_hedge(text: string, hedge_words: []string) -> bool:
    """Check if trigger text contains a casual hedge word as a standalone word."""
    words = tokenize_lowercase(text)
    for word in words:
        if word in hedge_words:
            return true
    return false

function extract_trigger_text(finding: CognitiveAssessment, snapshot: ConversationSnapshot) -> string:
    """Get the text surrounding the finding's relevant turns."""
    texts = []
    for turn_num in finding.relevant_turns:
        turn = find_turn(snapshot, turn_num)
        if turn:
            texts.append(turn.raw_text)
    return join(texts, " ")
```

### Mechanical Definition of "Contextually Irrelevant"

The Context Inhibitor does **not** attempt to understand meaning. It defines
contextual irrelevance through five **mechanical** (structural, not semantic)
criteria:

1. **Casual hedge words in informal register:** If the conversation is informal
   (formality < 0.3) and the triggering text contains words from the casual_hedge
   set, the finding is contextually irrelevant. These words carry different weight
   in formal vs informal contexts — "absolutely" in a legal brief is a confidence
   claim; "absolutely" in a chat is an emphasis marker.

2. **Low stakes + low severity:** If urgency < 0.3 and severity ≤ CAUTION, the
   finding is not worth reporting even if technically correct. This is a pragmatic
   gate: the signal doesn't justify the noise.

3. **Insufficient corroboration:** If no other detector flags the same region of
   conversation, the finding is suspect. A real reasoning failure typically manifests
   in multiple ways (contradiction + scope drift, anchoring + overconfidence).

4. **Low confidence on non-critical findings:** WARNING findings below the
   confidence threshold are borderline by definition. In the absence of other
   supporting evidence, they are suppressed.

5. **Auto-pass for high severity:** CRITICAL findings always pass. This is the
   biological equivalent of nociceptive signals that bypass basal ganglia gating.

### Test Cases

1. **False positive suppression (the 3 FPs):**
   - Input: Casual conversation where user says "I'm absolutely sure we should go with React"
   - GainSignal: formality=0.2, urgency=0.1
   - Finding: CONFIDENCE_MISCALIBRATION on "absolutely", confidence=0.6
   - Gate 2: formality (0.2) < threshold (0.3) AND "absolutely" in casual_hedge_words → INHIBITED
   - Result: False positive correctly suppressed

2. **True positive preserved:**
   - Input: Technical architecture review where user says "I'm absolutely certain this will handle 10M requests" without evidence
   - GainSignal: formality=0.7, urgency=0.5
   - Finding: CONFIDENCE_MISCALIBRATION on "absolutely certain...10M requests", confidence=0.8
   - Gate 2: formality (0.7) > threshold (0.3) → gate does not fire
   - Gate 3: urgency (0.5) > stakes_threshold (0.3) → passes
   - Gate 4: confidence (0.8) > threshold (0.7) → passes
   - Gate 5: Scope Guard also flagged (scope expanding from "architecture" to "capacity") → corroboration > 0
   - Result: Finding correctly disinhibited

3. **Edge case — solo high-confidence finding:**
   - Input: Conversation where only anchoring-detector fires, but with confidence=0.95
   - Gate 5: corroboration=0.0 (no other detectors), BUT confidence > 0.9 → exception applies
   - Result: Finding disinhibited despite no corroboration (high-confidence solo exception)

4. **Edge case — CRITICAL always passes:**
   - Input: Blatant contradiction detected with severity=CRITICAL
   - Gate 1: severity ≥ CRITICAL → immediate DISINHIBITED
   - Result: No other gates evaluated

5. **Edge case — all detectors agree:**
   - Input: Pathological conversation triggering 5 detectors on overlapping turns
   - All findings have corroboration > 0.5 → all DISINHIBITED
   - Result: Full report (correct — this is a genuinely problematic conversation)

### Reuses

Turn-proximity logic from scope-guard (reference window expansion). Keyword
matching from urgency-assessor (formality patterns).

---

## 6. salience-filter (Layer 3)

**Description:** Scores findings by novelty and actionability, filtering out
low-salience items that pass the Context Inhibitor but aren't worth reporting.

**Brain analogue:** Amygdala — rapidly evaluates stimuli for biological salience
(threat, reward, novelty) and modulates attentional allocation (LeDoux, 2000,
*Annual Review of Neuroscience*, 23:155-184; Davis & Whalen, 2001, "The amygdala:
vigilance and emotion," *Molecular Psychiatry*, 6:13-34).

**Determinism:** PURE
**Language:** Go
**Estimated LOC:** ~250

### Port Contracts

```
in:  gated_assessments (repeated CognitiveAssessment — from Context Inhibitor)
in:  conversation_snapshot (ConversationSnapshot)
out: salience_scores (repeated SalienceScore — per-finding scores)
out: salient_assessments (repeated CognitiveAssessment — above threshold)
```

### Config Fields

```
novelty_weight:        type=DOUBLE  default="0.4"  evolvable=true  min="0.1"  max="0.8"
actionability_weight:  type=DOUBLE  default="0.4"  evolvable=true  min="0.1"  max="0.8"
severity_weight:       type=DOUBLE  default="0.2"  evolvable=true  min="0.0"  max="0.5"
min_salience:          type=DOUBLE  default="0.3"  evolvable=true  min="0.1"  max="0.7"
max_findings:          type=UINT32  default="10"   evolvable=false
```

### Algorithm

```pseudocode
function filter(assessments: []CognitiveAssessment, snapshot: ConversationSnapshot)
    -> ([]SalienceScore, []CognitiveAssessment):

    scored = []
    for a in assessments:
        novelty = compute_novelty(a, assessments)
        actionability = compute_actionability(a)
        severity_norm = normalize_severity(a.severity)  # INFO=0.25, CAUTION=0.5, WARNING=0.75, CRITICAL=1.0

        salience = (novelty_weight * novelty) +
                   (actionability_weight * actionability) +
                   (severity_weight * severity_norm)

        scored.append(SalienceScore{finding_id: id(a), score: salience,
                                     novelty: novelty, actionability: actionability})

    # Sort by salience descending, take top max_findings above threshold
    scored.sort_desc(by: score)
    salient = [s for s in scored if s.score >= min_salience][:max_findings]
    salient_assessments = [a for a in assessments if id(a) in salient_ids]

    return scored, salient_assessments

function compute_novelty(finding, all_findings) -> float:
    """How unique is this finding type in the current set?"""
    same_type_count = count(f for f in all_findings if f.finding_type == finding.finding_type)
    return 1.0 / same_type_count  # First of its type = 1.0, duplicates decrease

function compute_actionability(finding) -> float:
    """How actionable is this finding? Based on specificity of evidence."""
    score = 0.0
    if len(finding.relevant_claims) > 0:  score += 0.3  # has specific claims
    if len(finding.relevant_turns) > 0:   score += 0.3  # has turn references
    if len(finding.explanation) > 50:     score += 0.2  # has detailed explanation
    if finding.confidence > 0.7:          score += 0.2  # high confidence
    return score
```

### Test Cases

1. **Positive:** Single CONTRADICTION finding with detailed explanation and claim references → high salience (novelty=1.0, actionability=0.8)
2. **Negative:** Fifth ANCHORING_BIAS finding in same report → low novelty (0.2), filtered if below threshold
3. **Edge:** Exactly max_findings findings all above threshold → all pass; max_findings+1 → lowest salience dropped

### Reuses

Scoring pattern from synthesis-aggregator (weighted combination of metrics).

---

## 7. threshold-modulator (Layer 3)

**Description:** Translates the GainSignal from the Urgency Assessor into concrete
threshold adjustments for each Layer 2 detector.

**Brain analogue:** Norepinephrine gain modulation — NE acts as a multiplicative
gain factor on cortical neuron responses, increasing signal-to-noise ratio for
attended stimuli (Aston-Jones & Cohen, 2005; Servan-Schreiber et al., 1990,
"A network model of catecholamine effects: gain, signal-to-noise ratio, and
behavior," *Science*, 249:892-895).

**Determinism:** PURE
**Language:** Go
**Estimated LOC:** ~200

### Port Contracts

```
in:  gain_signal (GainSignal)
out: threshold_adjustments (ThresholdAdjustments — map of detector → gain_offset)
```

### Config Fields

```
max_gain_offset:       type=DOUBLE  default="0.3"   evolvable=true  min="0.05"  max="0.5"
urgency_weight:        type=DOUBLE  default="0.6"   evolvable=true  min="0.1"   max="0.9"
formality_weight:      type=DOUBLE  default="0.3"   evolvable=true  min="0.0"   max="0.5"
complexity_weight:     type=DOUBLE  default="0.1"   evolvable=true  min="0.0"   max="0.3"
```

### Algorithm

```pseudocode
function modulate(gain: GainSignal) -> ThresholdAdjustments:
    # Compute a single gain factor from the composite signal
    # Negative = lower thresholds (more sensitive)
    # Positive = higher thresholds (less sensitive)

    raw_gain = -(urgency_weight * gain.urgency) +
                (formality_weight * (1.0 - gain.formality)) +
                -(complexity_weight * gain.complexity)

    # Clamp to [-max_gain_offset, +max_gain_offset]
    gain_offset = clamp(raw_gain, -max_gain_offset, max_gain_offset)

    # Apply uniformly to all detectors (future: per-detector weights)
    adjustments = {}
    for detector in known_detectors:
        adjustments[detector] = gain_offset

    return ThresholdAdjustments{adjustments: adjustments}
```

High urgency → negative offset → lower thresholds → more sensitive.
Low formality (casual) → positive offset → higher thresholds → less sensitive.
High complexity → negative offset → more sensitive (complex arguments need scrutiny).

### Test Cases

1. **Positive:** urgency=0.9, formality=0.8, complexity=0.5 → negative gain_offset (more sensitive)
2. **Negative:** urgency=0.1, formality=0.2, complexity=0.1 → positive gain_offset (less sensitive)
3. **Edge:** All values at 0.5 → gain_offset near 0 (neutral modulation)

### Reuses

Parameter computation pattern from scope-guard (weighted combination, clamped bounds).

---

## 8. self-confidence-assessor (Layer 4)

**Description:** Computes a confidence score for the overall pipeline report based
on detector agreement, detection margins, and historical accuracy.

**Brain analogue:** Metacognitive monitoring in anterior prefrontal cortex —
neural circuits that assess the reliability of one's own cognitive processes
(Fleming et al., 2010, "Relating introspective accuracy to individual differences
in brain structure," *Science*, 329:1541-1543; Fleming & Dolan, 2012, "The neural
basis of metacognitive ability," *Philosophical Transactions of the Royal Society B*,
367:1338-1349).

**Determinism:** PURE
**Language:** Go
**Estimated LOC:** ~300

### Port Contracts

```
in:  report (ReasoningReport — from Aggregator)
in:  gain_signal (GainSignal)
out: confidence_report (SelfConfidenceReport)
```

### Config Fields

```
agreement_weight:    type=DOUBLE  default="0.4"  evolvable=true  min="0.1"  max="0.7"
margin_weight:       type=DOUBLE  default="0.35" evolvable=true  min="0.1"  max="0.7"
historical_weight:   type=DOUBLE  default="0.25" evolvable=true  min="0.0"  max="0.5"
corpus_path:         type=STRING  default="data/corpus/cognitive-v1.ndjson"  evolvable=false
```

### Algorithm

```pseudocode
function assess(report: ReasoningReport, gain: GainSignal) -> SelfConfidenceReport:
    findings = report.findings

    # === Agreement score ===
    # High agreement = either all detectors agree there's a problem, or none do.
    # Mixed signals reduce confidence.
    if len(findings) == 0:
        agreement = 1.0  # Clean report — all detectors agree (nothing found)
    else:
        confidences = [f.confidence for f in findings]
        agreement = 1.0 - stdev(confidences)  # Low variance = high agreement
        agreement = clamp(agreement, 0.0, 1.0)

    # === Margin score ===
    # How far are findings from their detection thresholds?
    # Findings well above threshold → high confidence. Borderline → low.
    if len(findings) == 0:
        margin = 1.0
    else:
        margins = [abs(f.confidence - 0.5) for f in findings]  # distance from borderline
        margin = mean(margins) * 2.0  # normalize: 0.5 distance → 1.0
        margin = clamp(margin, 0.0, 1.0)

    # === Historical score ===
    # How accurate has the pipeline been on similar finding patterns?
    pattern = extract_pattern(findings)  # e.g., "ANCHORING_BIAS+SCOPE_DRIFT"
    historical = lookup_historical_accuracy(pattern, corpus_path)
    # Returns accuracy (0-1) or 0.5 if pattern not found in corpus

    # === Composite ===
    confidence = (agreement_weight * agreement) +
                 (margin_weight * margin) +
                 (historical_weight * historical)

    return SelfConfidenceReport{
        overall_confidence: confidence,
        agreement_score: agreement,
        margin_score: margin,
        historical_score: historical,
        finding_count: len(findings),
        recommendation: recommend(confidence)
    }

function recommend(confidence: float) -> string:
    if confidence > 0.8:  return "HIGH_CONFIDENCE"
    if confidence > 0.5:  return "MODERATE_CONFIDENCE"
    return "LOW_CONFIDENCE_REVIEW_RECOMMENDED"
```

### Test Cases

1. **Positive:** Report with 3 findings, all confidence > 0.85, all same type → agreement=0.95, margin=0.7, overall high
2. **Negative:** Report with 5 findings, confidences [0.3, 0.9, 0.4, 0.8, 0.35] → agreement=0.6 (high variance), margin low, overall low
3. **Edge:** Empty report (no findings) → agreement=1.0, margin=1.0, historical depends on corpus, overall high

### Reuses

Statistical computation from synthesis-aggregator. Historical accuracy lookup
uses the shared in-memory pattern index described in Memory Consolidator §10
(loaded at startup, refreshed periodically — NOT per-request file reads).

---

## 9. feedback-evaluator (Layer 4)

**Description:** Implements a single re-evaluation pass using the accumulator
pattern. When self-confidence is low, selects low-confidence findings for
re-evaluation with additional context from the first-pass synthesis.

**Brain analogue:** Cortico-thalamic feedback loops and predictive coding — higher
cortical areas send predictions to lower areas, which return prediction errors
(Rao & Ballard, 1999, *Nature Neuroscience*, 2:79-87; Friston, 2010, *Nature
Reviews Neuroscience*, 11:127-138). See [PRINCIPLES.md](PRINCIPLES.md) §5.

**Determinism:** PURE
**Language:** Go
**Estimated LOC:** ~350

**Sync gateway exception:** The Feedback Evaluator has both out (FeedbackRequest)
and in (FeedbackResponse) ports, making it a blocking synchronous gateway — it
emits a request and waits for responses before producing its output. This is the
same documented-exception pattern used by the cognitive-gateway COG. Like the
gateway, the Feedback Evaluator is an infrastructure COG that bridges async
pipeline flow with a synchronous request-response cycle. The composition must
treat it as a sync node with a timeout (default: 2× Layer 2 budget = 1000ms).

### Port Contracts

```
in:  report (ReasoningReport — first-pass synthesis)
in:  confidence_report (SelfConfidenceReport — from Self-Confidence Assessor)
in:  conversation_snapshot (ConversationSnapshot)
out: feedback_request (FeedbackRequest — sent to selected Layer 2 detectors)
in:  feedback_response (FeedbackResponse — from re-evaluated detectors)
out: updated_report (ReasoningReport — second-pass synthesis, or original if skipped)
out: feedback_skipped (bool — true if self-confidence was sufficient)
```

### Config Fields

```
feedback_threshold:       type=DOUBLE  default="0.6"  evolvable=true  min="0.3"  max="0.9"
max_reeval_findings:      type=UINT32  default="3"    evolvable=true  min="1"    max="10"
confidence_improvement_min: type=DOUBLE default="0.1" evolvable=true  min="0.01" max="0.3"
```

### Algorithm

```pseudocode
function evaluate(
    report: ReasoningReport,
    confidence: SelfConfidenceReport,
    snapshot: ConversationSnapshot
) -> (FeedbackRequest | nil, ReasoningReport):

    # === Step 1: Check if feedback is needed ===
    if confidence.overall_confidence >= feedback_threshold:
        return nil, report  # First pass is confident enough

    # === Step 2: Select findings for re-evaluation ===
    # Pick the least-confident findings (most likely to change)
    candidates = sort_by_confidence_asc(report.findings)
    to_reeval = candidates[:max_reeval_findings]

    # === Step 3: Generate FeedbackRequest ===
    request = FeedbackRequest{
        pass_number: 2,
        original_report: report,
        reeval_finding_ids: [finding_id(f) for f in to_reeval],
        context: snapshot,
        # Include first-pass synthesis so detectors can see what others found
        synthesis_context: report.recommended_actions
    }

    return request, report  # Caller sends request to detectors

function integrate_feedback(
    original_report: ReasoningReport,
    responses: []FeedbackResponse
) -> ReasoningReport:

    updated_findings = copy(original_report.findings)
    for response in responses:
        if response.pass_number != 2:
            continue  # Only accept pass 2 (accumulator bound)
        # Replace original finding with re-evaluated version
        for i, f in updated_findings:
            if finding_id(f) == response.finding_id:
                if abs(response.updated.confidence - f.confidence) > confidence_improvement_min:
                    updated_findings[i] = response.updated
                break

    return ReasoningReport{
        findings: updated_findings,
        # Recalculate summary statistics
        ...recompute_stats(updated_findings)
    }
```

### Accumulator Pattern (Preventing Circular Wires)

The feedback loop is bounded by protocol:
1. `FeedbackRequest.pass_number` is always 2
2. Detectors check: if `pass_number > 1`, they re-evaluate and emit `FeedbackResponse`
3. The Feedback Evaluator only generates requests for `pass_number == 1` reports
4. There is no mechanism to generate a `pass_number == 3` request
5. The wiring graph remains acyclic — feedback is a second-stage subgraph, not a cycle

### Generic Detector Re-evaluation Protocol

All Layer 2 detectors must implement a `feedback_request` in-port. The generic
re-evaluation protocol:

1. Detector receives FeedbackRequest containing `original_report` (first-pass
   synthesis) and `reeval_finding_ids` (which of its findings to re-evaluate)
2. Detector re-runs its detection algorithm with **one additional input**: the
   `original_report.findings` from other detectors, exposed as a
   `peer_findings: []CognitiveAssessment` context field
3. Detectors may use peer findings to adjust confidence, NOT to change detection
   logic. The re-evaluation question is: "Given that these other findings exist,
   is your finding still valid and at what confidence?"
4. Detector emits FeedbackResponse with updated assessment

**Per-detector behavior:**

| Detector | Re-evaluation Logic |
|----------|-------------------|
| scope-guard | If contradiction-tracker also fired on overlapping turns, increase drift confidence (scope drift + contradiction = stronger signal) |
| anchoring-detector | If sunk-cost-detector fired on same turn, check if anchor value is the "sunk cost" — if so, increase confidence |
| confidence-calibrator | If no other detectors fired, reduce confidence (isolated calibration finding is more likely a false positive) |
| contradiction-tracker | If scope-guard fired, check whether "contradiction" is actually topic shift — if so, reduce confidence |
| sunk-cost-detector | If decision-ledger shows alternative was considered, reduce confidence (sunk cost less likely when alternatives are weighed) |
| decision-ledger | No re-evaluation — decision tracking is factual, not confidence-dependent |

Detectors that don't benefit from peer context return their original assessment
unchanged (confidence_delta = 0).

### Test Cases

1. **Positive:** Self-confidence=0.4, 2 findings with confidence 0.35 and 0.45 → both selected for re-evaluation, feedback generated
2. **Negative:** Self-confidence=0.85 → feedback skipped, original report returned unchanged
3. **Edge:** Re-evaluated finding confidence changes by 0.05 (< confidence_improvement_min) → original finding retained (change too small to matter)

### Reuses

Report manipulation from synthesis-aggregator. Accumulator pattern is new but
follows the same port contract discipline as all other COGs.

---

## 10. memory-consolidator (Layer 5)

**Description:** Creates sparse index entries from confirmed/rejected findings
and appends them to the Forge corpus, enabling the Lamarckian learning loop.

**Brain analogue:** Hippocampal indexing and sleep consolidation — the hippocampus
creates sparse pointers to cortical representations; consolidation transfers
these to long-term cortical storage (Teyler & DiScenna, 1986, *Behavioral
Neuroscience*, 100:147-154; Teyler & Rudy, 2007, *Hippocampus*, 17:1158-1169;
McClelland et al., 1995, *Psychological Review*, 102:419-457). See
[PRINCIPLES.md](PRINCIPLES.md) §6.

**Determinism:** PURE
**Language:** Go
**Estimated LOC:** ~400

### Port Contracts

```
in:  cerebro_report (CerebroReport — final pipeline output)
in:  inhibition_decisions (repeated InhibitionDecision — from Context Inhibitor)
in:  conversation_snapshot (ConversationSnapshot — for metadata extraction)
in:  user_feedback (optional FeedbackSignal — explicit confirm/reject from user)
out: consolidation_entry (ConsolidationEntry — sparse index for corpus)
```

### Config Fields

```
corpus_output_path:         type=STRING  default="data/corpus/consolidated.ndjson"  evolvable=false
min_confidence_for_auto:    type=DOUBLE  default="0.8"   evolvable=true  min="0.5"  max="0.95"
min_agreement_for_auto:     type=DOUBLE  default="0.7"   evolvable=true  min="0.3"  max="0.95"
consolidation_cooldown_sec: type=UINT32  default="60"    evolvable=false
max_entries_per_session:    type=UINT32  default="10"    evolvable=false
```

### Algorithm

```pseudocode
function consolidate(
    report: CerebroReport,
    decisions: []InhibitionDecision,
    snapshot: ConversationSnapshot,
    feedback: FeedbackSignal | nil
) -> ConsolidationEntry | nil:

    # === Trigger conditions (any one triggers consolidation) ===
    should_consolidate = false
    trigger_reason = ""

    # Trigger 1: Explicit user feedback
    if feedback != nil:
        should_consolidate = true
        trigger_reason = "user_feedback"

    # Trigger 2: High-confidence unanimous findings
    if not should_consolidate:
        findings = report.findings
        if len(findings) > 0:
            min_conf = min(f.confidence for f in findings)
            if min_conf >= min_confidence_for_auto:
                should_consolidate = true
                trigger_reason = "high_confidence_auto"

    # Trigger 3: Novel pattern (finding type combination not in corpus)
    # Uses in-memory pattern index — NOT per-request file reads. See note below.
    if not should_consolidate:
        pattern = finding_pattern(report)
        if not pattern_index.contains(pattern):
            should_consolidate = true
            trigger_reason = "novel_pattern"

    if not should_consolidate:
        return nil

    # === Build sparse index ===
    entry = ConsolidationEntry{
        conversation_id: snapshot_id(snapshot),
        timestamp: now(),
        trigger: trigger_reason,

        # Finding summary (NOT full findings — sparse index)
        finding_types: [f.finding_type for f in report.findings],
        finding_confidences: [f.confidence for f in report.findings],
        finding_severities: [f.severity for f in report.findings],

        # Inhibition summary
        inhibited_count: count(d for d in decisions if d.action == INHIBITED),
        disinhibited_count: count(d for d in decisions if d.action == DISINHIBITED),
        inhibition_reasons: distinct(d.reason for d in decisions if d.action == INHIBITED),

        # Conversation metadata (not content)
        turn_count: len(snapshot.turns),
        formality: compute_formality(snapshot),  # reuse urgency-assessor logic
        domain_markers: extract_domain_markers(snapshot),

        # Outcome
        outcome: feedback.outcome if feedback else "auto_confirmed",
        outcome_confidence: feedback.confidence if feedback else min_conf,

        # Detector agreement pattern
        detector_pattern: join(distinct(f.detector_name for f in report.findings), "+"),
    }

    # Append to corpus file
    append_ndjson(corpus_output_path, entry)
    return entry
```

### What Gets Indexed (Sparse Index)

Per hippocampal indexing theory, the Memory Consolidator stores **pointers and
metadata**, not the experience itself:

| Stored (index) | NOT Stored |
|----------------|-----------|
| Finding types and counts | Full conversation text |
| Confidence scores | Raw turn content |
| Severity levels | Intermediate processing state |
| Inhibition decisions | Detector internal state |
| Conversation metadata (length, formality) | User identity |
| Detector agreement pattern | Full CognitiveAssessment details |
| Outcome (confirmed/rejected) | Claim text |

### Test Cases

1. **Positive:** User confirms a CONTRADICTION finding → consolidation triggered with outcome="confirmed", entry written to corpus
2. **Negative:** Report with 3 findings all below confidence 0.5, no user feedback, pattern exists in corpus → no consolidation (no trigger fires)
3. **Edge:** Novel finding pattern (SCOPE_DRIFT + SUNK_COST_FALLACY never seen together) → auto-consolidation with trigger="novel_pattern"
4. **Edge:** Rapid-fire requests (within cooldown_sec) → second request skipped

### Corpus Access: Pattern Index

Both the Memory Consolidator (novel pattern check) and the Self-Confidence
Assessor (historical accuracy lookup) need corpus data during pipeline execution.
Per-request file reads are acceptable for small corpora (<100 entries) but become
a bottleneck as the corpus grows.

**Solution:** Both COGs use a **precomputed in-memory pattern index** loaded at
startup and refreshed when the corpus file changes (inotify/fswatch or periodic
reload, configurable interval, default 60s):

```pseudocode
type PatternIndex struct:
    patterns: set[string]                    # known finding type combinations
    accuracy_by_pattern: map[string]float    # pattern → historical accuracy
    last_modified: timestamp                 # corpus file mtime at last load

function load_index(corpus_path: string) -> PatternIndex:
    entries = parse_ndjson(corpus_path)
    index = PatternIndex{}
    for entry in entries:
        pattern = entry.detector_pattern
        index.patterns.add(pattern)
        # Accumulate accuracy: confirmed=1.0, rejected=0.0, auto=0.5
        index.accuracy_by_pattern[pattern].update(outcome_score(entry.outcome))
    index.last_modified = file_mtime(corpus_path)
    return index
```

The index is shared (read-only) between the two COGs. Memory overhead: ~1KB per
1000 corpus entries (patterns are short strings). Load time: <10ms for 10K entries.

### Reuses

NDJSON writing from forge (corpus loader). Formality computation from urgency-assessor.
Pattern matching from scope-guard. Pattern index is shared with self-confidence-assessor.
