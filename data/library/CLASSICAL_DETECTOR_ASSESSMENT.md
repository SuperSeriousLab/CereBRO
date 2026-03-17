# CereBRO Classical Detector Assessment — Republic Book 1

**Date:** 2026-03-15
**Run command:** `go test -run TestClassicalPipeline -v -timeout 5m ./internal/library/`
**Corpus:** Plato's Republic Book 1 (Jowett translation), 43 entries
- Cephalus section: 4 entries (no expected findings)
- Polemarchus section: 13 entries (CONTRADICTION, SUNK_COST_FALLACY expected)
- Thrasymachus section: 26 entries (ANCHORING_BIAS, CONTRADICTION, SCOPE_DRIFT, CONFIDENCE_MISCALIBRATION expected)

---

## 1. Top-Line Pipeline Results

| Run | TP | FP | FN | Precision | Recall | F1 |
|-----|----|----|----|-----------|---------|----|
| baseline | 24 | 27 | 48 | 0.471 | 0.333 | 0.390 |
| inhibitor-only | 16 | 11 | 56 | 0.593 | 0.222 | 0.323 |

**Comparison with modern-conversation baseline (9-conversation corpus):**
- Modern corpus: F1=0.91 (D-inhibitor-only)
- Classical corpus: F1=0.390 (baseline), 0.323 (inhibitor-only)
- Delta: approximately -0.52 to -0.59 F1 drop on classical text

---

## 2. Per-Detector Results (baseline config)

| Detector | TP | FP | FN | Precision | Recall | F1 |
|----------|----|----|----|-----------|---------|----|
| CONTRADICTION | 16 | 12 | 6 | 0.571 | 0.727 | 0.640 |
| CONFIDENCE_MISCALIBRATION | 8 | 15 | 5 | 0.348 | 0.615 | 0.444 |
| ANCHORING_BIAS | 0 | 0 | 11 | 0.000 | 0.000 | 0.000 |
| SCOPE_DRIFT | 0 | 0 | 13 | 0.000 | 0.000 | 0.000 |
| SUNK_COST_FALLACY | 0 | 0 | 13 | 0.000 | 0.000 | 0.000 |
| SILENT_REVISION | 0 | 0 | 0 | n/a | n/a | n/a |

---

## 3. Detector-by-Detector Analysis

### 3.1 CONTRADICTION (contradiction-tracker)
**Result:** TP=16, FP=12, FN=6 | Precision=0.571 Recall=0.727 F1=0.640

**Assessment: PARTIALLY WORKING — best performer on classical text.**

The negation-prefix and antonym-pair approach partially captures Socratic
refutation, but produces significant FP noise. Several false positives stem from
the detector flagging Socrates's reductive reformulations ("Certainly not.")
against Polemarchus's earlier claims — lexically they overlap but the structure
is agreement-through-negation, not genuine self-contradiction.

False negatives (FN=6) appear where the contradiction spans too many turns for
the sliding-window approach to pair sentences (e.g., Polemarchus's
turn-1 "helping friends / harming enemies" claim vs. his turn-8 concession).

**Root cause of FP=12:** The word-overlap threshold (MinOverlap=0.3) is too low
for philosophical dialogue, where Socrates routinely echoes the interlocutor's
phrasing to build toward contradiction — triggering detections on agreement
exchanges, not genuine self-contradiction.

---

### 3.2 CONFIDENCE_MISCALIBRATION (confidence-calibrator)
**Result:** TP=8, FP=15, FN=5 | Precision=0.348 Recall=0.615 F1=0.444

**Assessment: PARTIALLY WORKING — correct in concept, tuned for wrong vocabulary.**

The PURE detector fires on CERTAIN-level confidence keywords. It finds Thrasymachus
because his rhetoric uses words like "certainly" (the Jowett translation uses these
naturally). However:

**Root cause of FP=15:**
- The `confidenceKeywords` list contains "certainly", "absolutely" — these appear in
  Jowett's translation as normal scholarly phrasing and rhetorical connectives, not
  as overconfidence markers ("Certainly not", "Certainly, he said").
- The Cephalus and early Polemarchus sections have zero expected
  CONFIDENCE_MISCALIBRATION, but the detector fires anyway because the Jowett
  prose uses "certainly" as a discourse particle, not a certainty claim.
- The evidence scorer (`assessEvidenceLevel`) only counts modern markers
  ("because", "since", "evidence shows", "data indicates") — classical argument
  uses "for" as the primary causal connective, which is stripped as a stop word.
  This means all classical claims are classified as SPECULATED, pushing ECE high.

**Root cause of FN=5:**
- Thrasymachus's most overconfident claims use classical rhetorical forms:
  "I say...", "Listen, then...", "Is it not so?" — none of which match the
  CERTAIN-level keywords ("definitely", "i'm sure", "100%").

---

### 3.3 ANCHORING_BIAS (anchoring-detector)
**Result:** TP=0, FP=0, FN=11 | F1=0.000

**Category: STRUCTURAL**

The anchoring-bias detector is fundamentally numeric. It looks for `NumericTokens`
in the conversation metadata and detects when a later numeric estimate stays close
to an earlier one. The entire detector logic (`collectNumericEntries`) depends on
`turn.GetMetadata().GetNumericTokens()`.

In Plato's Republic, the "anchoring" is conceptual — Thrasymachus repeatedly anchors
the discourse to his initial claim that "justice is the advantage of the stronger."
There are no numeric quantities in the dialogue. No numeric tokens exist in the
metadata. The detector produces zero output (no TP, no FP) because it cannot even
attempt detection.

**Why TP=0 and FP=0 both:** The detector returns `nil` immediately when
`len(entries) < cfg.MinNumericTokens` (threshold=2). Zero numeric tokens → no
attempt, no findings of any kind.

**Fix category:** This requires a new COG or a parallel conceptual-anchoring
detector that tracks repeated re-introduction of a framing claim across turns.
The numeric architecture is not salvageable for this use case — it is **STRUCTURAL**.

---

### 3.4 SCOPE_DRIFT (scope-guard)
**Result:** TP=0, FP=0, FN=13 | F1=0.000

**Category: STRUCTURAL + VOCABULARY**

The scope guard uses weighted Jaccard divergence between `objective_keywords` and
`topic_keywords` extracted per-turn. Two structural problems apply:

**Problem 1 — Keyword population:** The corpus entries were built from
`segmentDialogue()` in `republic_test.go`. The `objective` field is set to a human
phrase ("What is justice?"), but `input.metadata.topic_keywords` per turn depends
on `turn.GetMetadata().GetTopicKeywords()`. Inspecting a sample entry shows that
turns in the corpus have no `metadata` field populated — the `TurnJSON` struct only
carries `turn_number`, `speaker`, and `raw_text`. The `ToProtoSnapshot()` conversion
produces turns with nil metadata, so `GetTopicKeywords()` returns nil for every turn.

**Problem 2 — Even if keywords existed:** The `ScopeGuardConfig.SustainedTurns=8`
requires 8 consecutive drifting turns. The Thrasymachus dialogue does shift scope
late (from "justice is the advantage of the stronger" → rulership and salary
discussion), but segments are only 10 turns each. A threshold of 8 consecutive
drifting turns in a 10-turn window requires 80% of the segment to be drifting
before firing. With sparse metadata this cannot trigger.

**Stemmer impact on Scope Guard:** The Porter-lite stemmer was added to
`extractKeywords` and applied via `stemFreqMap`. However, since `topic_keywords`
are never populated in the classical corpus entries, the stemmer has no material to
operate on. The stemmer cannot help when the input keywords are absent.

**Fix category: STRUCTURAL** — requires either (a) populating `topic_keywords` at
corpus generation time using keyword extraction from raw_text, or (b) building a
keyword-free version of scope detection based on raw-text topic modeling.

---

### 3.5 SUNK_COST_FALLACY (sunk-cost-detector)
**Result:** TP=0, FP=0, FN=13 | F1=0.000

**Category: VOCABULARY**

This detector is entirely phrase-based. The phrases it seeks are:

```
sunkCostPhrases:    "already spent", "already invested", "invested so much",
                    "come this far", "put so much into", "too much time",
                    "too much money", "too much effort", "can't waste",
                    "don't want to waste", "sunk cost", "we've already",
                    "i've already"

continuationPhrases: "should keep going", "should continue", "can't stop now",
                     "shouldn't give up", "let's keep", "let's continue",
                     "we must continue", "have to finish", "need to finish",
                     "too late to change", "too late to stop", "might as well",
                     "no point stopping", "stick with"
```

Polemarchus's sunk-cost reasoning is expressed classically:
- "The words of Simonides are true and just..." (inherited authority, not
  "investment" vocabulary)
- "Then we must not say that justice is speaking the truth and paying our debts?"
  (accepting refutation of one formulation but defending the source)
- "But I must learn it, not from thee..." (defending the original framing)

None of these map to any phrase in either list. The underlying pattern — reluctance
to abandon a commitment because it was inherited from a respected authority — is
genuinely present, but the linguistic surface form is entirely different from
modern project-sunk-cost language.

The detector produces FP=0 because it requires a `costMatch` AND a
`continuationMatch` in order from the same conversation. Without matching either
list, it never fires.

**Fix category: VOCABULARY** — this is fixable by adding classical equivalents:
"for so said", "as Simonides maintains", "we must hold to", "it were unjust to
abandon", "the argument stands that", "to which I agreed", and similar patterns
of defending inherited/committed claims.

---

### 3.6 SILENT_REVISION (decision-ledger)
**Result:** TP=0, FP=0, FN=0 | n/a

**Category: NOT_PRESENT**

The decision-ledger detector looks for decision markers like "let's go with",
"we'll use", "i'll choose", "decided to", "going with", "the plan is",
"we should", "i recommend".

Socratic dialogue by design does not contain project-decision framing.
Participants do not make operational decisions — they negotiate definitions.
The zero result is correct: silent revision in the modern sense (changing a
technical recommendation without acknowledging it) is genuinely absent from
this corpus.

Zero FP confirms the detector appropriately stays quiet on alien text.

---

## 4. Formality Fix Assessment

**Expected:** classical text scores >0.70 formality (per CLAUDE.md context)
**Actual:** average formality = **0.577** across 38 entries with formality data

**The fix did not achieve its target.** Only 10 of 43 entries scored ≥0.70.

**Why formality is still low despite the fix:**

The classical formal markers added (`thou`, `thee`, `nay`, `wherefore`, etc.)
appear in some Thrasymachus turns but are sparse in the segmented corpus.
The majority of the Jowett translation uses standard 19th-century English
prose without archaic markers. The scoring model counts formal markers
against informal markers, and the structural signals (long sentences) help,
but many corpus entries have:

1. **Short turns** (len < 8 words) that trigger `informalCount++` — the
   Socratic dialogue format produces many short affirmative/negative responses
   ("Certainly.", "True.", "That is so.") which are scored as informal.

2. **Contractions:** The Jowett translation uses "don't" and "isn't" in places,
   triggering contraction detection. Also, Unicode curly-apostrophes in
   `don't` and `won't` appear in the source — these are handled by
   `NormalizeQuotes` but only after lowercasing. The contraction list
   uses ASCII apostrophes; if `NormalizeQuotes` isn't catching all variants,
   some contractions escape detection.

3. **Inhibitor consequence:** The low formality (0.577 average, many entries
   below 0.40) means Gate 1 in the inhibitor fires for CONFIDENCE_MISCALIBRATION
   findings when casual hedge words appear in the trigger text. This explains
   the inhibitor-only result showing only 1/13 expected CONFIDENCE_MISCALIBRATION
   entries surviving the inhibitor (8% preservation rate).

**Impact on findings suppression:**
- Baseline (no inhibitor): CONFIDENCE_MISCALIBRATION TP=8, FP=15
- Inhibitor-only: CONFIDENCE_MISCALIBRATION TP=1 (7 true positives suppressed)

The inhibitor is suppressing legitimate classical findings because it treats
the Jowett translation as informal based on short-turn structural signals.
The formality fix helped slightly (5-gate analysis passes more findings than
before) but the average score of 0.577 still classifies most entries as
semi-informal, triggering the casual-hedge gate on CONFIDENCE_MISCALIBRATION.

---

## 5. Stemmer Impact on Scope Guard

**Assessment: Cannot be measured — Scope Guard produces zero output.**

The Porter-lite stemmer is applied in `stemFreqMap()` to the reference frequency
map and potentially to turn keywords. However, as documented in Section 3.4,
the classical corpus entries have no `topic_keywords` populated in turn metadata.
The stemmer operates on keyword maps that are empty.

The stemmer would help Scope Guard if the corpus had keyword data. Specifically:
- "argue"/"arguing"/"argument"/"arguments" would stem to "argu" → unified token
- "just"/"justice"/"justly"/"unjust" would require the ice→"" rule to produce "just"
- "advantage"/"advantageous" → "advantag" → unified

Until `topic_keywords` are populated from `raw_text` during corpus generation,
the stemmer improvement to Scope Guard is **inert on this corpus**.

---

## 6. Architecture Competition (43-entry classical corpus)

| Variant | Precision | Recall | F1 | Notes |
|---------|-----------|--------|----|-------|
| A-full-cortex | 0.615 | 0.222 | 0.327 | |
| B-no-feedback | 0.615 | 0.222 | 0.327 | |
| C-no-modulation | 0.593 | 0.222 | 0.323 | |
| D-inhibitor-only | 0.593 | 0.222 | 0.323 | |
| E-pre-cortex | 0.471 | 0.333 | 0.390 | **Winner** |

**D-inhibitor-only does NOT win on classical text** (0/4 profiles).
E-pre-cortex (the baseline, no inhibitor) wins 3/4 profiles.

**Interpretation:** On modern conversations, the inhibitor raises precision by
suppressing false positives. On classical text, the inhibitor suppresses true
positives (FN increases from 48 to 56) because the formality scoring classifies
the text as semi-informal. The inhibitor's Gate 1 and Gate 5 both work against
classical text: Gate 1 fires on low-formality + "certainly" as a casual hedge;
Gate 5 fails because only 2 of 6 detectors fire (CONTRADICTION and
CONFIDENCE_MISCALIBRATION), giving low cross-detector corroboration.

---

## 7. Summary and Categorization

| Detector | Classical F1 | Category | Root Cause |
|----------|-------------|----------|------------|
| CONTRADICTION | 0.640 | PARTIALLY WORKING | Word-overlap threshold too low; FP=12 from discourse echoing |
| CONFIDENCE_MISCALIBRATION | 0.444 | VOCABULARY | Evidence markers miss "for" as causal connective; confidence keywords fire on discourse particles |
| ANCHORING_BIAS | 0.000 | STRUCTURAL | Entirely numeric architecture; conceptual anchoring is a different problem |
| SCOPE_DRIFT | 0.000 | STRUCTURAL | No topic_keywords in corpus metadata; SustainedTurns=8 incompatible with 10-turn segments |
| SUNK_COST_FALLACY | 0.000 | VOCABULARY | All phrases are modern project-management language; classical pattern uses inherited-authority vocabulary |
| SILENT_REVISION | n/a | NOT_PRESENT | Decision-making framing absent from Socratic dialogue by design |

### Fixable vs. Needs New COG

**Fixable with domain vocabulary additions:**
- SUNK_COST_FALLACY: add classical commitment-defense phrases
- CONFIDENCE_MISCALIBRATION: add "for" as an evidence marker; restrict certainty
  detection to speech-act markers ("I maintain", "I assert") not discourse particles

**Fixable with corpus/infrastructure changes:**
- SCOPE_DRIFT: populate `topic_keywords` from raw_text during corpus generation
- SCOPE_DRIFT: lower `SustainedTurns` from 8→3 for short-segment corpora

**Needs new COG:**
- ANCHORING_BIAS (conceptual): a conceptual-anchoring detector that tracks
  repeated re-introduction of a framing claim (not a numeric value)

**Correctly zero:**
- SILENT_REVISION: not a pattern in Socratic dialogue

---

## 8. Key Findings

1. The formality fix brought the average score to 0.577 but did not reach the 0.70
   target. The primary failure mode is short turns (common in Socratic dialogue)
   scoring as informal. The inhibitor penalises classical text through this path.

2. Three detectors produce zero output on classical text for fundamentally different
   reasons: ANCHORING_BIAS is structurally incompatible (numeric vs. conceptual),
   SCOPE_DRIFT lacks the metadata it needs (fixable), and SUNK_COST_FALLACY uses
   the wrong vocabulary register (fixable).

3. CONTRADICTION performs best (F1=0.640) because its approach — negation conflict +
   antonym pairs + same-speaker across turns — is language-register-independent.
   It does not depend on modern idioms.

4. The inhibitor (D-inhibitor-only) actively hurts performance on classical text.
   E-pre-cortex (raw detectors, no inhibitor) wins 3/4 competition profiles.
   The inhibitor is calibrated for modern conversational register and suppresses
   true positives in classical/literary input via the formality gate.

5. The stemmer improvement cannot be assessed on this corpus because the corpus
   has no `topic_keywords` metadata. It is inert but correct — it would help
   if the metadata were populated.
