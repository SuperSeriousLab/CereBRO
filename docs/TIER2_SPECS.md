# CereBRO — Tier 2 COG Specifications

> Specifications for Tier 2 specialist COGs addressing structural detection gaps
> identified in the Task 3 classical-text assessment.
>
> **Tier 2 definition:** Layer 2 (Cortical Specialist) COGs that detect reasoning
> failures not covered by the existing six core detectors. These fill structural
> gaps — cases where the _form_ of the failure is categorically different from
> what current detectors inspect, not merely a vocabulary variant.

**Cross-references:** [NEW_COGS.md](NEW_COGS.md) (existing COG specs),
[CONTRACTS.md](CONTRACTS.md) (proto messages), [ARCHITECTURE.md](ARCHITECTURE.md)
(layer placement)

**Structural gap context:** The Task 3 assessment against classical philosophical
text (Plato's _Republic_, Book I) exposed two categories of reasoning failure that
score 0 against all existing detectors because their surface form shares nothing
with what those detectors inspect. Both failures are structural, not lexical.

---

## 1. conceptual-anchoring-detector (Layer 2, Tier 2)

**Description:** Detects when an early strong claim sets the conceptual frame
for all subsequent argument, causing later reasoning to orbit the anchor rather
than evaluate alternatives independently.

**Brain analogue:** Prefrontal cortex framing bias — initial strong representations
bias subsequent evaluations by constraining the hypothesis space considered during
deliberation (Kahneman, 2011, _Thinking, Fast and Slow_, Farrar, Straus and Giroux,
Ch. 11; Epley & Gilovich, 2006, "The anchoring-and-adjustment heuristic,"
_Psychological Science_, 17:311-318). The existing `anchoring-detector` models the
_numeric_ variant of this mechanism (price/quantity anchors). This COG models the
_conceptual_ variant: an asserted proposition rather than a number becomes the
immovable reference frame.

**Determinism:** PURE (pattern matching on claim structure and keyword overlap —
no LLM required)
**Language:** Go
**Tier:** 2 (Layer 2 specialist)
**Estimated LOC:** ~280

### Structural gap addressed

The existing `anchoring-detector` (FindingType: `ANCHORING_BIAS`) triggers on
numeric anchors only — it looks for a baseline number followed by insufficiently
revised estimates. Classical conceptual anchoring has no numbers. Thrasymachus in
_Republic_ Book I anchors on "justice is the advantage of the stronger" as an
immovable propositional frame, then defends every subsequent exchange from within
that frame rather than engaging counter-claims on their merits. The detection
signal is _sustained semantic orbit_, not numeric proximity.

### Port Contracts

```
in:  conversation_snapshot (ConversationSnapshot)
out: assessment (CognitiveAssessment) — finding_type: CONCEPTUAL_ANCHORING
                                        detail: ConceptualAnchoringDetail (see proto below)
```

### Proto additions required

The existing `CognitiveAssessment` message uses `AnchoringDetail` (field 10)
for numeric anchors. Conceptual anchoring requires a distinct sub-message:

```protobuf
// Detail for CONCEPTUAL_ANCHORING findings.
// Populated in CognitiveAssessment.conceptual_anchoring (field 16).
message ConceptualAnchoringDetail {
  string anchor_claim_text = 1;       // the first strong assertion (the anchor)
  uint32 anchor_turn = 2;             // turn where anchor was set
  double semantic_orbit_ratio = 3;    // fraction of subsequent turns with overlap > threshold
  double avg_semantic_overlap = 4;    // mean overlap across all subsequent turns
  uint32 turns_analyzed = 5;          // total subsequent turns examined
  bool counter_claims_acknowledged = 6; // true if any counter-claim was accepted/refined
}
```

The `FindingType` enum requires a new value:

```protobuf
// In FindingType enum, after ANCHORING_BIAS = 10:
CONCEPTUAL_ANCHORING = 14;
```

### Config Fields

```
anchor_confidence_threshold: type=DOUBLE  default="0.7"  evolvable=true  min="0.4"  max="0.95"
  # Minimum expressed_confidence in Claim to qualify as the anchor candidate.
  # High-confidence declaratives only — hedged claims don't anchor.

semantic_overlap_threshold:  type=DOUBLE  default="0.7"  evolvable=true  min="0.4"  max="0.9"
  # Fraction of keywords shared between a subsequent turn and the anchor claim
  # required to count that turn as "orbiting" the anchor.

orbit_turn_ratio:             type=DOUBLE  default="0.6"  evolvable=true  min="0.3"  max="0.9"
  # Fraction of subsequent turns that must exhibit overlap >= semantic_overlap_threshold
  # to trigger a finding. Prevents firing on mere topic consistency.

min_subsequent_turns:         type=UINT32  default="4"   evolvable=true  min="2"    max="20"
  # Minimum number of subsequent turns required. Short conversations cannot
  # exhibit anchoring — there is no "orbit" to measure.

anchor_keywords:              type=STRING  default=""    evolvable=false
  # Optional comma-separated seed keywords known to indicate strong assertion
  # frames (e.g., "is,must,always,never,only"). Empty = use confidence signal only.
```

### Algorithm

```pseudocode
function detect(snapshot: ConversationSnapshot) -> CognitiveAssessment | nil:

    turns = snapshot.turns
    if len(turns) < min_subsequent_turns + 1:
        return nil  # Too short to exhibit anchoring

    # === Step 1: Find the anchor candidate ===
    # The anchor is the first high-confidence declarative claim.
    # We use mechanical signals only: sentence structure + keyword overlap.
    anchor_turn = nil
    anchor_keywords_set = {}

    for turn in turns:
        # High-confidence signal: declarative structure (no hedge words)
        # and expressed confidence markers ("is", "must", "always", "the X is Y")
        if is_strong_declarative(turn.raw_text, anchor_confidence_threshold):
            anchor_turn = turn
            anchor_keywords_set = extract_topic_keywords(turn)
            break

    if anchor_turn == nil:
        return nil  # No strong declarative found — no anchor to orbit

    # === Step 2: Compute semantic orbit for subsequent turns ===
    subsequent = turns[anchor_turn.turn_number:]
    if len(subsequent) < min_subsequent_turns:
        return nil

    orbit_count = 0
    overlap_scores = []

    for turn in subsequent:
        turn_keywords = set(turn.metadata.topic_keywords)
        if len(turn_keywords) == 0:
            continue
        overlap = len(turn_keywords ∩ anchor_keywords_set) / len(turn_keywords)
        overlap_scores.append(overlap)
        if overlap >= semantic_overlap_threshold:
            orbit_count += 1

    orbit_ratio = orbit_count / len(overlap_scores) if overlap_scores else 0.0
    avg_overlap = mean(overlap_scores) if overlap_scores else 0.0

    # === Step 3: Check for counter-claim acknowledgement ===
    # If at any point the speaker accepts a qualification or revises the anchor
    # claim, the "orbit" is broken — this is healthy discourse.
    counter_acknowledged = has_acknowledged_counter(turns, anchor_turn.turn_number)

    # === Step 4: Threshold check ===
    if orbit_ratio < orbit_turn_ratio:
        return nil  # Insufficient orbit — topic consistency, not anchoring

    if counter_acknowledged:
        return nil  # Counter-claims were engaged — no anchoring

    # === Step 5: Build finding ===
    confidence = compute_confidence(orbit_ratio, avg_overlap, len(subsequent))
    return CognitiveAssessment{
        finding_type: CONCEPTUAL_ANCHORING,
        severity: severity_from_confidence(confidence),
        explanation: format(
            "Anchor set at turn {}: '{}'. {}/{} subsequent turns orbit the anchor "
            "(avg overlap {:.2f}). No counter-claims acknowledged.",
            anchor_turn.turn_number,
            truncate(anchor_turn.raw_text, 80),
            orbit_count, len(subsequent),
            avg_overlap
        ),
        relevant_turns: [anchor_turn.turn_number] + [t.turn_number for t in subsequent],
        confidence: confidence,
        detector_name: "conceptual-anchoring-detector",
        conceptual_anchoring: ConceptualAnchoringDetail{
            anchor_claim_text: anchor_turn.raw_text,
            anchor_turn: anchor_turn.turn_number,
            semantic_orbit_ratio: orbit_ratio,
            avg_semantic_overlap: avg_overlap,
            turns_analyzed: len(subsequent),
            counter_claims_acknowledged: counter_acknowledged
        }
    }

function is_strong_declarative(text: string, threshold: float) -> bool:
    """
    Mechanical heuristic for high-confidence declarative assertion.
    Does NOT require LLM. Returns true if:
    - No hedge markers present ("maybe", "perhaps", "I think", "possibly",
      "might", "could be", "I'm not sure")
    - Declarative copula present: "X is Y", "X are Y", "X must", "X always",
      "X never", or strong assertion verb (subject + finite verb + object)
    - Sentence is not a question (no terminal "?", no interrogative opener)
    - Length > 5 words (substance check)
    """
    ...

function has_acknowledged_counter(turns: []Turn, anchor_turn_num: uint32) -> bool:
    """
    Returns true if any turn after the anchor contains an acknowledgement
    signal: "you're right", "I concede", "that's a fair point", "I revise",
    "actually", "on reflection", "I was wrong" — without immediately
    reasserting the original claim within the same turn.
    """
    ...

function compute_confidence(orbit_ratio: float, avg_overlap: float, n: uint32) -> float:
    """
    Composite confidence:
    - orbit_ratio weight 0.5  (primary signal — how many turns orbit)
    - avg_overlap weight 0.3  (depth of orbit)
    - sample_size weight 0.2  (more turns = more evidence, capped at n=15)
    Returns value in [0.0, 1.0].
    """
    sample_size_norm = min(n / 15.0, 1.0)
    return clamp(
        0.5 * orbit_ratio + 0.3 * avg_overlap + 0.2 * sample_size_norm,
        0.0, 1.0
    )
```

### Test Cases

1. **Detected — Thrasymachus (classical):**
   - Input: "Justice is the advantage of the stronger" (turn 3, no hedges) → 15
     turns of restatement and defense with no acknowledgement of Socrates'
     counter-examples
   - `anchor_keywords_set`: {justice, advantage, stronger}
   - `orbit_ratio`: ~0.87 (13/15 turns share ≥ 2 of 3 keywords)
   - `counter_acknowledged`: false
   - Result: CONCEPTUAL_ANCHORING, WARNING, confidence ~0.82

2. **Not detected — healthy debate (evolution of position):**
   - Input: Thesis stated at turn 2 → counter-argument at turn 5 → speaker
     at turn 6 says "That's a fair point — let me refine my position..."
     and subsequent turns show modified claims
   - `has_acknowledged_counter`: true
   - Result: nil (no finding — position evolved)

3. **Detected — modern equivalent:**
   - Input: "AI will replace all jobs" (turn 1, declarative, no hedges) →
     12 turns citing evidence for this claim, dismissing counter-examples
     ("but that's a special case", "that won't matter long-term")
   - `orbit_ratio`: ~0.75, `avg_overlap`: 0.71
   - `counter_acknowledged`: false (dismissals are not acknowledgements)
   - Result: CONCEPTUAL_ANCHORING, WARNING, confidence ~0.74

### Distinction from existing `anchoring-detector`

| Dimension | anchoring-detector (existing) | conceptual-anchoring-detector (new) |
|-----------|-------------------------------|--------------------------------------|
| Anchor type | Numeric value (price, quantity) | Propositional claim |
| Detection signal | Δ between anchor number and estimates | Semantic orbit ratio across turns |
| Input used | `numeric_tokens` in TurnMetadata | `topic_keywords` in TurnMetadata |
| Position-claim structure | "X said 10M → later said 8M" | "X asserted P → all later turns defend P" |
| False positive risk | Legitimate price revision | Topic consistency (same topic ≠ anchoring) |

The two detectors can co-fire (a numeric anchor can also be a conceptual frame),
but they inspect orthogonal signals and must not be merged.

### Reuses

Keyword overlap computation from `scope-guard` (fuzzy topic distance).
`is_strong_declarative` tokenization pattern from `conversation-intake`.

---

## 2. inherited-position-detector (Layer 2, Tier 2)

**Description:** Detects when a position is defended because of who holds it
or how long it has been held, rather than its merits — the authority-appeal
variant of sunk cost.

**Brain analogue:** Social conformity circuits — deference to authority and
in-group tradition overrides independent evaluation of evidence (Asch, 1956,
"Studies of independence and conformity," _Psychological Monographs_, 70:1-70;
Cialdini, 2001, _Influence: Science and Practice_, Allyn & Bacon, Ch. 6:
Authority). Unlike genuine sunk-cost (where the investment is material — money,
time, effort already spent), inherited-position sunk cost is _epistemic_: the
investment is in the authority figure's standing or the tradition's age, and
continuing to hold the position is how one honors that investment.

**Determinism:** PURE (pattern matching on citation structure and justification
presence — no LLM required)
**Language:** Go
**Tier:** 2 (Layer 2 specialist)
**Estimated LOC:** ~250

### Structural gap addressed

The existing `sunk-cost-detector` (FindingType: `SUNK_COST_FALLACY`) detects
investment-language patterns — phrases indicating past expenditure used to
justify continuation ("we've already spent", "too much invested to stop now").
Polemarchus in _Republic_ Book I exhibits a categorically different structure:
"Simonides said..." defended without independent justification. No investment
language appears. The detection signal is repeated authority citation without
accompanying independent evidence — the position is held because a respected
source holds it, not because evidence supports it.

### Port Contracts

```
in:  conversation_snapshot (ConversationSnapshot)
out: assessment (CognitiveAssessment) — finding_type: INHERITED_POSITION
                                        detail: InheritedPositionDetail (see proto below)
```

### Proto additions required

```protobuf
// Detail for INHERITED_POSITION findings.
// Populated in CognitiveAssessment.inherited_position (field 17).
message InheritedPositionDetail {
  repeated string authority_figures = 1;    // names/entities cited as authorities
  uint32 authority_citation_count = 2;      // total authority citations detected
  bool independent_justification_present = 3; // true if any counter-addressed on merit
  repeated uint32 citation_turns = 4;       // turns where authority was cited
  string defended_claim = 5;               // the position being defended by authority
}
```

The `FindingType` enum requires a new value:

```protobuf
// In FindingType enum, after INHERITED_POSITION:
INHERITED_POSITION = 15;
```

### Config Fields

```
authority_citation_patterns: type=STRING
  default="as X said,X taught,according to X,X showed,X argued,X believed,tradition holds,we have always,it has always been,X's view,following X"
  evolvable=false
  # Comma-separated patterns for authority citation. "X" is treated as a
  # wildcard for a proper noun or named entity. At match time, the COG
  # extracts the authority name from the matched context.

min_citation_count:          type=UINT32  default="3"   evolvable=true  min="2"    max="10"
  # Minimum number of authority citations to trigger a finding.
  # Single citations are normal scholarly practice.

independent_justification_keywords: type=STRING
  default="because,therefore,follows that,implies,evidence shows,data shows,studies show,we can observe,consider,the reason is,this means"
  evolvable=false
  # Keywords indicating independent justification. If a turn containing an
  # authority citation ALSO contains these keywords with substantive content,
  # that citation is treated as legitimate (authority + evidence).

justification_min_words:     type=UINT32  default="10"  evolvable=true  min="5"    max="30"
  # Minimum words in the independent justification clause to count as
  # substantive (prevents "because Simonides" from counting as justification).
```

### Algorithm

```pseudocode
function detect(snapshot: ConversationSnapshot) -> CognitiveAssessment | nil:

    # === Step 1: Scan for authority citations ===
    citation_turns = []     # turns with authority citations
    justified_turns = []    # citation turns that ALSO have independent justification
    authorities_cited = {}  # set of named authorities

    for turn in snapshot.turns:
        match = find_authority_citation(turn.raw_text, authority_citation_patterns)
        if match == nil:
            continue

        citation_turns.append(turn.turn_number)
        authorities_cited.add(match.authority_name)

        # Check for independent justification in the same turn
        if has_independent_justification(turn.raw_text,
                                          independent_justification_keywords,
                                          justification_min_words):
            justified_turns.append(turn.turn_number)

    # === Step 2: Apply citation count threshold ===
    if len(citation_turns) < min_citation_count:
        return nil  # Too few citations — normal attribution

    # === Step 3: Check justification coverage ===
    # If most citation turns have independent justification, this is
    # legitimate citation practice, not inherited-position reasoning.
    unjustified_ratio = (len(citation_turns) - len(justified_turns)) / len(citation_turns)

    if unjustified_ratio < 0.5:
        return nil  # More than half of citations are independently justified

    # === Step 4: Check if citations are defending a consistent claim ===
    # All citations should be defending the same core position.
    # Extract the defended claim from the first citation turn.
    defended_claim = extract_defended_claim(snapshot.turns, citation_turns[0])

    # === Step 5: Build finding ===
    confidence = compute_confidence(len(citation_turns), unjustified_ratio)
    return CognitiveAssessment{
        finding_type: INHERITED_POSITION,
        severity: severity_from_confidence(confidence),
        explanation: format(
            "{} authority citations to {} found across turns {}. "
            "{:.0f}% cite authority without independent justification. "
            "Position defended by appeal to authority rather than merit.",
            len(citation_turns),
            join(authorities_cited, ", "),
            join(citation_turns, ", "),
            unjustified_ratio * 100
        ),
        relevant_turns: citation_turns,
        confidence: confidence,
        detector_name: "inherited-position-detector",
        inherited_position: InheritedPositionDetail{
            authority_figures: list(authorities_cited),
            authority_citation_count: len(citation_turns),
            independent_justification_present: len(justified_turns) > 0,
            citation_turns: citation_turns,
            defended_claim: defended_claim
        }
    }

function find_authority_citation(text: string, patterns: []string) -> Match | nil:
    """
    Pattern-match for authority citation structures.
    "X" in patterns is a wildcard for a capitalized name or entity mention.
    Returns Match{authority_name, matched_pattern} or nil.
    Uses entity_mentions from TurnMetadata to identify candidate authority names.
    """
    ...

function has_independent_justification(text: string, keywords: []string, min_words: uint32) -> bool:
    """
    Returns true if the text contains a justification keyword AND the
    clause following that keyword is at least min_words long.
    Prevents "because Simonides" from counting (2 words < min_words).
    """
    ...

function extract_defended_claim(turns: []Turn, citation_turn_num: uint32) -> string:
    """
    Extracts the proposition being defended in the citation turn.
    Returns the sentence containing the citation as a proxy for the claim.
    Truncated to 120 characters for readability.
    """
    ...

function compute_confidence(citation_count: uint32, unjustified_ratio: float) -> float:
    """
    citation_count weight 0.4 (more citations = stronger signal)
    unjustified_ratio weight 0.6 (primary signal — how many lack justification)
    citation_count normalized: count / 10 (capped at 1.0 for count >= 10)
    """
    count_norm = min(citation_count / 10.0, 1.0)
    return clamp(0.4 * count_norm + 0.6 * unjustified_ratio, 0.0, 1.0)
```

### Test Cases

1. **Detected — Polemarchus (classical):**
   - Input: "Simonides said justice is giving what is owed" → defended 4 times
     by reference to Simonides ("but that's what Simonides meant", "Simonides
     was wise", "Simonides surely intended...") with no independent argument
   - `citation_turns`: [3, 7, 10, 14], `justified_turns`: []
   - `unjustified_ratio`: 1.0
   - Result: INHERITED_POSITION, WARNING, confidence ~0.80

2. **Not detected — legitimate citation (academic):**
   - Input: "Einstein showed E=mc² [followed by three-paragraph derivation
     explaining the mass-energy equivalence and its experimental confirmation]"
   - `citation_turns`: [2], `len < min_citation_count (3)` → nil
   - Even if threshold met, each citation is accompanied by derivation
     (> `justification_min_words`) → `unjustified_ratio` < 0.5 → nil
   - Result: nil (no finding)

3. **Detected — modern institutional deference:**
   - Input: "We've always done it this way" × 4 turns, no explanation of why
     the established practice is superior to the proposed alternative
   - `authority_citation_patterns` matches "we have always" (tradition citation)
   - `justified_turns`: [] (no substantive rationale in any turn)
   - Result: INHERITED_POSITION, CAUTION, confidence ~0.64

### Distinction from `assumption-surfacer`

The Assumption Surfacer (listed in the Tier 2 backlog, proto: `ArgumentStructure`
input) finds **unstated premises** — claims that a speaker relies on but never
articulates. The Inherited-Position detector finds **stated premises defended by
authority rather than merit** — the claim is stated explicitly ("Simonides said
X"), but the justification for holding it is social/traditional rather than
evidential.

| Dimension | assumption-surfacer | inherited-position-detector |
|-----------|---------------------|------------------------------|
| Premise state | Unstated / implicit | Stated explicitly |
| Detection target | Missing premise in argument structure | Absent independent justification |
| Input type | ArgumentStructure (claim graph) | ConversationSnapshot (raw text) |
| Primary signal | Gap in inference graph | Authority citation without evidence |
| Resolution | "State the assumption" | "Provide independent justification" |

The two can co-fire: a speaker might cite an authority AND rely on an unstated
premise about that authority's infallibility. They should not be merged — they
address different failure modes and suggest different remedies.

### Distinction from existing `sunk-cost-detector`

| Dimension | sunk-cost-detector (existing) | inherited-position-detector (new) |
|-----------|-------------------------------|-----------------------------------|
| Investment type | Material (money, time, effort) | Epistemic (authority standing, tradition) |
| Language signal | "already spent", "can't stop now", "too invested" | "X said", "tradition holds", "we always" |
| Continuation driver | Avoiding waste of past resources | Deferring to authority / preserving tradition |
| Structural gap in classical text | 0 matches on Polemarchus | Directly addresses Polemarchus |

### Reuses

Entity mention extraction from `conversation-intake` (proper nouns as authority
candidate names). Pattern matching from `scope-guard` (keyword list scanning).

---

## Proto Change Summary

Both COGs require additions to `proto/cog/reasoning/v1/reasoning.proto`.
**Contracts first** — implement these proto changes before building either COG.

### New FindingType values

```protobuf
// Add to FindingType enum (after existing entries, before "100+ reserved"):
CONCEPTUAL_ANCHORING = 14;
INHERITED_POSITION   = 15;
```

### New detail sub-messages

```protobuf
// Add to CognitiveAssessment message (fields 16, 17):
ConceptualAnchoringDetail conceptual_anchoring = 16;
InheritedPositionDetail   inherited_position   = 17;

// Add as top-level messages:
message ConceptualAnchoringDetail {
  string anchor_claim_text          = 1;
  uint32 anchor_turn                = 2;
  double semantic_orbit_ratio       = 3;
  double avg_semantic_overlap       = 4;
  uint32 turns_analyzed             = 5;
  bool   counter_claims_acknowledged = 6;
}

message InheritedPositionDetail {
  repeated string authority_figures            = 1;
  uint32          authority_citation_count     = 2;
  bool            independent_justification_present = 3;
  repeated uint32 citation_turns              = 4;
  string          defended_claim              = 5;
}
```

---

## Routing notes (Layer 1 router)

Both COGs operate on `ConversationSnapshot` directly and require no pre-processing
beyond what `conversation-intake` already provides (`topic_keywords` and
`entity_mentions` in `TurnMetadata`). The Layer 1 router should activate both
detectors when `total_turns >= 4` (the minimum for either to produce a finding).
No additional routing signals are needed.

Neither COG requires the Claim Extractor (the LLM-backed I-1 intake path). Both
are PURE deterministic COGs operating on mechanical metadata.
