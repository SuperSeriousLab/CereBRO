# CereBRO — Cross-Domain Applications

> The CereBRO 5-layer architecture is domain-agnostic. Only Layer 2 specialists
> change between domains. Layers 0, 1, 3, 4, and 5 are reused wholesale.

**Cross-references:** [ARCHITECTURE.md](ARCHITECTURE.md) (layer definitions),
[PRINCIPLES.md](PRINCIPLES.md) (§2 parallel specialists, §3 inhibitory gating)

---

## The Reuse Thesis

CereBRO separates **architecture** (how processing is organized) from **domain
knowledge** (what is being analyzed). The 5-layer structure is an organizational
principle, not a cognitive-bias-specific design:

- **Layer 0 (SENSORY):** Input validation is universal — every domain needs format,
  safety, and language checks
- **Layer 1 (THALAMIC RELAY):** Intake enrichment and routing adapt to the domain
  but the pattern is identical — tokenize, classify, fan out
- **Layer 2 (CORTICAL SPECIALISTS):** **This is the only layer that changes per domain.** Different domains need different specialist detectors.
- **Layer 3 (INHIBITION & MODULATION):** Contextual gating is universal — every
  domain has false positive patterns that need suppression
- **Layer 4 (INTEGRATION & META):** Synthesis, self-confidence, and feedback are
  domain-independent
- **Layer 5 (MEMORY & LEARNING):** Consolidation and evolution are domain-independent

This means ~80% of the CereBRO architecture is reusable across any analytical domain.

---

## Domain 1: Code Review

### What Changes

**Layer 2 specialists become code analysis COGs:**

| Cognitive COG | Code Review COG | Finding Type |
|--------------|----------------|-------------|
| contradiction-tracker | **inconsistency-detector** | Contradictory comments vs code behavior |
| anchoring-detector | **magic-number-detector** | Hardcoded values without justification |
| scope-guard | **scope-creep-detector** | PR changes outside stated purpose |
| confidence-calibrator | **complexity-assessor** | Cyclomatic complexity, cognitive complexity |
| sunk-cost-detector | **tech-debt-tracker** | "We already have this pattern" justifications |
| decision-ledger | **design-decision-tracker** | Architecture decisions in PRs |
| claim-extractor | **intent-extractor** (HYBRID) | Extract stated intent from PR description |

### What Stays the Same

| Layer | Reused As-Is | Adaptation Needed |
|-------|-------------|-------------------|
| Layer 0 | format-validator (check file encoding), toxicity-gate (code comments) | Language-detector may need "programming language" detection |
| Layer 1 | conversation-intake → **diff-intake** (parse unified diff format) | Router classifies by file type / change type |
| Layer 3 | Context Inhibitor (suppress FPs in test files, generated code) | Casual-hedge list replaced with test-code markers |
| Layer 4 | Self-Confidence, Feedback Evaluator | Unchanged |
| Layer 5 | Memory Consolidator (learns which findings are confirmed/rejected) | Corpus format adapts to code review entries |

### Shared COGs

- **Context Inhibitor:** Works identically — suppresses low-salience findings in
  low-stakes contexts (e.g., formatting-only PRs, test files)
- **Salience Filter:** Unchanged — novelty and actionability apply to code findings too
- **Self-Confidence Assessor:** Unchanged — detector agreement still signals reliability
- **Memory Consolidator:** Unchanged — confirmed/rejected code review findings
  feed back into Forge optimization

### Example

A PR adds 500 lines to a critical payment service. CereBRO code review:
- Layer 0: validates diff format, checks for secrets in diff
- Layer 1: classifies as high-stakes (payment service), emits urgency=0.8
- Layer 2: scope-creep-detector finds changes to unrelated logging module;
  complexity-assessor flags 3 functions above threshold
- Layer 3: Context Inhibitor suppresses complexity warning on test file (low stakes);
  preserves scope-creep finding (high stakes)
- Layer 4: Self-confidence=0.7 (two detectors agree on scope issue)
- Layer 5: If reviewer confirms scope-creep, entry consolidated for future learning

---

## Domain 2: Financial Document Auditing

### What Changes

**Layer 2 specialists become numerical consistency COGs:**

| Cognitive COG | Financial COG | Finding Type |
|--------------|-------------|-------------|
| contradiction-tracker | **balance-verifier** | Assets ≠ liabilities + equity |
| anchoring-detector | **baseline-drift-detector** | Year-over-year figures anchored to wrong base |
| scope-guard | **completeness-checker** | Missing required disclosures |
| confidence-calibrator | **precision-validator** | Rounding errors, significant digit inconsistencies |
| sunk-cost-detector | **write-off-analyzer** | Delayed write-offs suggesting sunk-cost behavior |
| decision-ledger | **policy-change-tracker** | Accounting policy changes across periods |
| claim-extractor | **assertion-extractor** (HYBRID) | Extract financial assertions from narratives |

### What Stays the Same

| Layer | Reused As-Is | Adaptation Needed |
|-------|-------------|-------------------|
| Layer 0 | format-validator (PDF/XBRL validation), toxicity-gate (keep — financial docs can contain offensive content in emails, internal memos, narrative sections), language-detector | Language-detector useful for multi-language reports |
| Layer 1 | Intake → **document-intake** (parse financial statements) | Router classifies by statement type (income, balance sheet, cash flow) |
| Layer 3 | Context Inhibitor, Salience Filter, Threshold Modulator | Formality thresholds adjusted (financial docs are always formal) |
| Layer 4 | Self-Confidence, Feedback Evaluator | Unchanged |
| Layer 5 | Memory Consolidator | Corpus entries include financial period metadata |

### Shared COGs

The entire inhibition layer transfers. Financial document analysis has the same
false positive problem — a rounding difference of $1 in a $1B report is noise, not
a finding. The Context Inhibitor's stakes gate (urgency threshold) and severity
gate handle this naturally.

---

## Domain 3: Legal Contract Review

### What Changes

**Layer 2 specialists become clause verification COGs:**

| Cognitive COG | Legal COG | Finding Type |
|--------------|---------|-------------|
| contradiction-tracker | **clause-conflict-detector** | Contradictory obligations in same contract |
| anchoring-detector | **precedent-anchor-detector** | Terms anchored to outdated precedent |
| scope-guard | **scope-limitation-checker** | Obligations exceeding stated scope |
| confidence-calibrator | **ambiguity-detector** | Vague or ambiguous language in critical clauses |
| sunk-cost-detector | **renegotiation-detector** | "Existing agreement" appeals blocking better terms |
| decision-ledger | **amendment-tracker** | Track modifications across contract versions |
| claim-extractor | **obligation-extractor** (HYBRID) | Extract obligations, conditions, rights |

### What Stays the Same

| Layer | Reused As-Is | Adaptation Needed |
|-------|-------------|-------------------|
| Layer 0 | format-validator, language-detector | Toxicity-gate may detect problematic discriminatory language |
| Layer 1 | Intake → **contract-intake** (parse clauses, sections) | Router classifies by clause type (obligation, right, condition) |
| Layer 3 | Full inhibition layer | Formality always high (legal); urgency based on contract value |
| Layer 4 | Self-Confidence, Feedback Evaluator | Unchanged |
| Layer 5 | Memory Consolidator | Learns from lawyer confirm/reject patterns |

### Key Insight

Legal contracts are highly formal, so the formality gate in the Context Inhibitor
almost never triggers (formality ≈ 1.0 for legal text). This means more findings
pass through — which is correct behavior for legal review, where even minor
ambiguities matter.

---

## Domain 4: Educational Assessment

### What Changes

**Layer 2 REUSES cognitive COGs directly:**

| Cognitive COG | Educational Use | Notes |
|--------------|----------------|-------|
| contradiction-tracker | Detect contradictions in student reasoning | **Reused directly** |
| anchoring-detector | Detect anchoring in student estimates | **Reused directly** |
| scope-guard | Detect topic drift in student essays | **Reused directly** |
| confidence-calibrator | Assess student calibration ("I'm certain" vs evidence) | **Reused directly** |
| sunk-cost-detector | Detect sunk-cost reasoning in student decisions | **Reused directly** |
| decision-ledger | Track student decision quality | **Reused directly** |
| claim-extractor | Extract student claims for analysis | **Reused directly** |

This domain reuses ALL Layer 2 COGs unchanged — the cognitive pipeline was designed
to detect reasoning failures, and student reasoning is exactly the use case.

### What Stays the Same

**Everything.** Educational assessment is the closest domain to the original
cognitive augmentation use case. The only adaptation:

- Layer 1 Urgency Assessor: urgency based on assignment stakes (final exam vs
  homework), not on conversation context
- Layer 3 Context Inhibitor: formality threshold adjusted for student writing
  (more informal than professional communication)
- Layer 5 Memory Consolidator: consolidated entries include student level metadata
  (introductory vs advanced)

---

## Reuse Matrix

| Component | Cognitive | Code Review | Financial | Legal | Educational |
|-----------|-----------|-------------|-----------|-------|-------------|
| **Layer 0: format-validator** | reuse | reuse | adapt (PDF) | reuse | reuse |
| **Layer 0: toxicity-gate** | reuse | reuse | reuse | adapt | reuse |
| **Layer 0: language-detector** | reuse | adapt | reuse | reuse | reuse |
| **Layer 1: intake** | reuse | adapt (diff) | adapt (fin) | adapt (clause) | reuse |
| **Layer 1: router** | reuse | adapt | adapt | adapt | reuse |
| **Layer 1: urgency-assessor** | reuse | reuse | reuse | reuse | reuse |
| **Layer 2: detectors** | reuse | **replace** | **replace** | **replace** | **reuse** |
| **Layer 3: context-inhibitor** | reuse | reuse | reuse | reuse | reuse |
| **Layer 3: salience-filter** | reuse | reuse | reuse | reuse | reuse |
| **Layer 3: threshold-modulator** | reuse | reuse | reuse | reuse | reuse |
| **Layer 4: aggregator** | reuse | reuse | reuse | reuse | reuse |
| **Layer 4: self-confidence** | reuse | reuse | reuse | reuse | reuse |
| **Layer 4: feedback-evaluator** | reuse | reuse | reuse | reuse | reuse |
| **Layer 5: memory-consolidator** | reuse | reuse | reuse | reuse | reuse |

**Legend:** reuse = use as-is, adapt = minor config changes, replace = new domain-specific COGs, skip = not needed

### Quantified Reuse

- **Cognitive → Educational:** 100% reuse (14/14 COGs)
- **Cognitive → Code Review:** 71% reuse (10/14 reuse + adapt), 29% replace (4/14 Layer 2)
- **Cognitive → Financial:** 71% reuse, 29% replace
- **Cognitive → Legal:** 71% reuse, 29% replace

The 5-layer architecture achieves >70% component reuse across all analyzed domains.
The replaceable components (Layer 2 specialists) are the only domain-specific parts.

---

## Implications

1. **New domains require only Layer 2 specialists.** The infrastructure investment
   (Layers 0, 1, 3, 4, 5) pays off across every domain.

2. **Cross-domain COG sharing is possible.** A `contradiction-tracker` trained on
   legal documents may detect contradictions in financial documents too. The
   Memory Consolidator can capture this cross-domain learning.

3. **AIP architecture competition applies to all domains.** The same competition
   framework that optimizes CereBRO for cognitive augmentation can optimize it
   for code review, with the same traits (F1, precision, recall, latency).

4. **The Lamarckian loop accelerates new domains.** Deploy CereBRO in a new domain
   with baseline parameters → collect feedback → Forge optimizes → domain-specific
   performance improves without manual tuning.
