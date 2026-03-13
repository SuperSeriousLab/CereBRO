# CereBRO — Implementation Build Order

> Six phases, each independently valuable and testable.
> Each phase builds on the previous but does not require all prior phases to be
> complete — graceful degradation means skipped phases reduce capability, not correctness.

**Cross-references:** [ARCHITECTURE.md](ARCHITECTURE.md) (layer specs),
[NEW_COGS.md](NEW_COGS.md) (COG implementations), [CONTRACTS.md](CONTRACTS.md) (proto messages)

**Current baseline:** F1=0.78, Precision=0.64, Recall=1.00, FP=5 (3 are casual
CONFIDENCE_MISCALIBRATION on "absolutely"/"definitely")

---

## Phase 1: Context Inhibitor — Precision Win

> **The single highest-value COG.** Expected to eliminate the 3 known false
> positives and reduce overall FP rate significantly.

### Deliverables

1. Proto: `cerebro.v1.InhibitionDecision`, `cerebro.v1.InhibitionAction` enum
2. COG: `context-inhibitor` (Go, PURE, ~500 LOC)
3. Integration: wire after detectors, before synthesis-aggregator in pipeline-v3
4. Composition: `cognitive-pipeline-v3` manifest with Context Inhibitor inserted
5. Tests: unit tests (5 gates × positive/negative) + integration test against corpus

### Proto Changes

- New file: `proto/cerebro/v1/cerebro.proto` (or extend reasoning.proto — TBD)
- Messages: `InhibitionDecision`, `InhibitionAction` enum
- No changes to existing messages — Context Inhibitor consumes `CognitiveAssessment`
  and produces `InhibitionDecision` + filtered `CognitiveAssessment` list

### Pipeline Integration

Current: `Router → [detectors] → Aggregator`
Phase 1: `Router → [detectors] → Context Inhibitor → Aggregator`

The Context Inhibitor sits between the detector fan-in and the Aggregator.
It receives all raw assessments and the ConversationSnapshot, applies 5 gates,
and passes only disinhibited findings to the Aggregator.

For Phase 1, the GainSignal input is stubbed with defaults:
`{urgency: 0.5, complexity: 0.5, formality: 0.5, mode: TONIC}`. Real GainSignal
comes in Phase 2.

### Exit Criteria

- [ ] 3 CONFIDENCE_MISCALIBRATION FPs on casual "absolutely"/"definitely" → INHIBITED
- [ ] No true positives lost (recall stays at 1.00)
- [ ] All existing test conversations produce same or better results
- [ ] Integration test: run full corpus, verify FP count drops from 5 to ≤2

### Expected Metrics

| Metric | Before | After | Change |
|--------|--------|-------|--------|
| Precision | 0.64 | 0.80+ | +0.16+ |
| Recall | 1.00 | 1.00 | unchanged |
| F1 | 0.78 | 0.89+ | +0.11+ |
| False positives | 5 | ≤2 | -3+ |

### Validation

Test conversations that validate this phase:
- Casual conversations with "absolutely"/"definitely" → FPs suppressed
- Technical discussions with "absolutely certain about 10M req" → TP preserved
- Clean conversations → still clean (no new FPs introduced)
- Pathological conversations → all findings preserved (CRITICAL auto-pass)

### Dependencies

None. This phase is self-contained.

### Estimated Effort

~2 days: 1 day for COG implementation + tests, 1 day for pipeline integration +
corpus validation.

---

## Phase 2: Urgency Assessor + Threshold Modulator — Context Sensitivity

> **Makes the pipeline context-aware.** High-stakes conversations get scrutinized
> more; casual conversations get lighter treatment.

### Deliverables

1. Proto: `cerebro.v1.GainSignal`, `cerebro.v1.GainMode` enum, `cerebro.v1.ThresholdAdjustments`
2. COG: `urgency-assessor` (Go, PURE, ~300 LOC)
3. COG: `threshold-modulator` (Go, PURE, ~200 LOC)
4. Integration: Urgency Assessor wired after Intake, GainSignal broadcast to
   Context Inhibitor and all detectors
5. Detector modifications: add `gain_signal` input port to each detector,
   use it to adjust thresholds via `effective_threshold = base * (1 + offset)`
6. Composition: update pipeline-v3 manifest

### Proto Changes

- Add: `GainSignal`, `GainMode`, `ThresholdAdjustments`
- Modify: each detector's manifest to declare `gain_signal` input port

### Exit Criteria

- [ ] High-urgency conversations (security, legal, financial) → lower thresholds
- [ ] Casual conversations → higher thresholds, fewer borderline findings
- [ ] GainSignal correctly classifies test conversations by urgency/formality
- [ ] No recall degradation on high-stakes test conversations
- [ ] Precision improvement on casual test conversations

### Expected Metrics

| Metric | Before (Phase 1) | After | Change |
|--------|-------------------|-------|--------|
| Precision | 0.80+ | 0.85+ | +0.05 |
| Recall | 1.00 | 1.00 | unchanged |
| F1 | 0.89+ | 0.91+ | +0.02 |

Smaller improvement than Phase 1 — the biggest wins came from inhibition.
Phase 2 refines sensitivity for edge cases.

### Validation

- Casual conversation corpus: fewer borderline findings
- High-stakes corpus entries: all findings preserved or increased
- Mixed corpus: overall metrics improve

### Dependencies

Phase 1 (Context Inhibitor uses GainSignal — currently stubbed). Phase 2 provides
the real GainSignal, replacing the stub.

### Estimated Effort

~3 days: 1 day each for Urgency Assessor and Threshold Modulator, 1 day for
detector modifications and integration.

---

## Phase 3: Layer 0 Reflexes — Fast Reject

> **Reduces load on expensive layers.** Malformed, toxic, or unsupported-language
> input is rejected before any cognitive processing.

### Deliverables

1. Proto: `cerebro.v1.ValidationResult`, `cerebro.v1.ToxicityResult`, `cerebro.v1.LanguageResult`
2. COG: `format-validator` (Go, PURE, ~150 LOC)
3. COG: `toxicity-gate` (Go, PURE, ~250 LOC)
4. COG: `language-detector` (Go, PURE, ~200 LOC)
5. Data: default blocklist at `data/blocklists/default.txt`
6. Data: language trigram profiles at `data/language-profiles/`
7. Integration: chain before existing Intake
8. Composition: update pipeline manifest

### Proto Changes

- Add: `ValidationResult`, `ToxicityResult`, `LanguageResult`

### Exit Criteria

- [ ] Invalid UTF-8 input → rejected with clear error
- [ ] Oversized input → rejected before parsing
- [ ] Toxic content → blocked with matched patterns listed
- [ ] Non-English input → rejected (or detected for future multi-language support)
- [ ] All valid test conversations pass through unchanged
- [ ] Layer 0 total latency < 10ms

### Expected Metrics

No change to F1/precision/recall on the existing corpus (all corpus entries are
valid English text). The improvement is operational:

| Metric | Before | After |
|--------|--------|-------|
| Malformed input handling | Parse error in Intake | Clean rejection at Layer 0 |
| Toxic input handling | Processed fully then reported | Blocked immediately |
| Latency for rejected input | 500ms+ (full pipeline) | <10ms |

### Validation

New test cases (not existing corpus):
- Malformed UTF-8 → rejected
- 10MB input → rejected
- Blocklisted content → blocked
- French/German text → detected and rejected (when only "en" supported)

### Dependencies

None (independent of Phases 1-2). Can be built in parallel.

### Estimated Effort

~2 days: 1 day for all three COGs (they're simple), 1 day for blocklist curation +
language profiles + integration.

---

## Phase 4: Self-Confidence Assessor + Feedback Evaluator — Metacognition

> **The system can now assess its own reliability.** Users know when to trust the
> report and when to investigate further.

### Deliverables

1. Proto: `cerebro.v1.SelfConfidenceReport`, `cerebro.v1.ConfidenceRecommendation`,
   `cerebro.v1.FeedbackRequest`, `cerebro.v1.FeedbackResponse`
2. COG: `self-confidence-assessor` (Go, PURE, ~300 LOC)
3. COG: `feedback-evaluator` (Go, PURE, ~350 LOC)
4. Detector modifications: each detector must handle FeedbackRequest (pass_number=2)
   and return FeedbackResponse with re-evaluated assessment
5. Integration: wire after Aggregator, before final report emission
6. Composition: update pipeline manifest with second-pass subgraph

### Proto Changes

- Add: `SelfConfidenceReport`, `ConfidenceRecommendation`, `FeedbackRequest`,
  `FeedbackResponse`

### Exit Criteria

- [ ] Self-confidence correlates with actual accuracy (measure on corpus)
- [ ] Low-confidence reports trigger re-evaluation
- [ ] Re-evaluation improves at least 50% of re-evaluated findings
- [ ] Feedback is bounded: exactly one re-evaluation pass (pass_number check)
- [ ] No infinite loops or circular wires in composition graph
- [ ] Pipeline with feedback still completes within 2x latency budget

### Expected Metrics

| Metric | Before (Phase 2) | After | Change |
|--------|-------------------|-------|--------|
| Precision | 0.85+ | 0.88+ | +0.03 |
| Recall | 1.00 | 1.00 | unchanged |
| F1 | 0.91+ | 0.93+ | +0.02 |

Additionally: reports include confidence score, enabling downstream filtering.

### Validation

- High-agreement findings (all detectors concur) → high self-confidence
- Mixed findings (detectors disagree) → low self-confidence → feedback triggered
- Feedback improves borderline findings on corpus entries with known ground truth

### Dependencies

Phases 1-2 (needs GainSignal for self-confidence input). Phase 3 is independent.

### Estimated Effort

~4 days: 1 day each for Self-Confidence and Feedback Evaluator, 1 day for
detector FeedbackRequest handling, 1 day for integration + wiring validation.

---

## Phase 5: Memory Consolidator + Salience Filter — Learning Loop

> **Closes the Lamarckian loop.** The system learns from runtime experience.

### Deliverables

1. Proto: `cerebro.v1.ConsolidationEntry`, `cerebro.v1.ConsolidationTrigger`,
   `cerebro.v1.SalienceScore`
2. COG: `memory-consolidator` (Go, PURE, ~400 LOC)
3. COG: `salience-filter` (Go, PURE, ~250 LOC)
4. Integration: Memory Consolidator runs asynchronously after report delivery.
   Salience Filter wired between Context Inhibitor and Aggregator.
5. Forge integration: consolidated entries are in NDJSON format compatible with
   existing corpus loader
6. Documentation: consolidation schema, corpus growth monitoring

### Proto Changes

- Add: `ConsolidationEntry`, `ConsolidationTrigger`, `SalienceScore`

### Exit Criteria

- [ ] Confirmed findings generate corpus entries
- [ ] Rejected findings generate negative corpus entries
- [ ] Novel patterns auto-consolidate
- [ ] Forge can evolve against consolidated corpus entries
- [ ] Corpus growth is bounded (cooldown, max entries per session)
- [ ] Salience filter reduces report noise without losing critical findings

### Expected Metrics

Metrics improve **over time**, not immediately:

| Metric | Immediate | After 1 Forge Cycle |
|--------|-----------|---------------------|
| Precision | unchanged | +0.02-0.05 (corpus-driven) |
| Recall | unchanged | unchanged (bounded params) |
| Corpus size | +entries per run | Grows toward domain coverage |

### Validation

- Run pipeline on 10 test conversations → verify consolidation entries created
- Run Forge against expanded corpus → verify fitness improves
- Verify no duplicate entries, no entries exceeding max per session
- Verify cooldown prevents flood on rapid requests

### Dependencies

Phases 1-2 (uses InhibitionDecisions and GainSignal for consolidation metadata).
Phase 4 (uses CerebroReport which includes SelfConfidenceReport).

### Estimated Effort

~3 days: 1 day each for Memory Consolidator and Salience Filter, 1 day for
Forge integration + corpus validation.

---

## Phase 6: Architecture Competition via AIP — Evolve Everything

> **The meta-phase.** Use AIP to compete alternative CereBRO configurations and
> find the optimal architecture empirically.

### Deliverables

1. Competition manifests: 3-5 alternative CereBRO compositions as textproto
2. AIP arena: define the architecture competition arena (CerebroReport scoring)
3. Trait profiles: balanced, precision-first, recall-first, minimal
4. Runner integration: run full CereBRO pipeline as a competition contestant
5. Results documentation: which architecture won, why, by how much

### Competition Architecture Variants

| Variant | Description |
|---------|------------|
| **full-cortex** | All 5 layers, all COGs, feedback enabled |
| **no-feedback** | Layers 0-3 + Aggregator, no Self-Confidence or Feedback |
| **no-layer0** | Skip format/toxicity/language (start at Intake) |
| **minimal** | Layer 2 + Context Inhibitor only (smallest useful config) |
| **aggressive** | Full cortex with low thresholds (maximize recall) |

### Exit Criteria

- [x] AIP successfully runs architecture competition with 5 compositions
- [x] Pareto frontier identifies winning configurations per profile
- [x] Results are reproducible across multiple runs
- [x] Winning architecture documented with rationale

### Results (2026-03-13)

Winner: **D-inhibitor-only** (all 4 profiles). Context Inhibitor provides 100%
of precision improvement. Pareto frontier: D + E. Full results in
`data/competitions/ARCHITECTURE_COMPETITION.md`.

### Expected Metrics

Architecture competition doesn't directly improve metrics — it identifies the
best configuration of already-built components. The winning architecture becomes
the default for future deployments.

### Dependencies

All prior phases (needs all COGs built to compete full configurations).

### Estimated Effort

~2 days: 1 day for competition setup (manifests, arena, runner), 1 day for
running competitions + analysis.

---

## Summary Timeline

```
Phase 1 ──────── Context Inhibitor ────────── (~2 days)
Phase 2 ──────── Urgency + Threshold ──────── (~3 days)
Phase 3 ──────── Layer 0 Reflexes ─────────── (~2 days)  ← can parallel with 1-2
Phase 4 ──────── Metacognition + Feedback ──── (~4 days)
Phase 5 ──────── Memory + Salience ─────────── (~3 days)
Phase 6 ──────── Architecture Competition ──── (~2 days)
                                         Total: ~16 days

Critical path: Phase 1 → 2 → 4 → 5 → 6 (~14 days)
Phase 3 is independent and can run in parallel with any other phase.
```

## Cumulative Expected Metrics

| After Phase | Precision | Recall | F1 | FP Count |
|-------------|-----------|--------|-----|----------|
| Baseline | 0.64 | 1.00 | 0.78 | 5 |
| Phase 1 | 0.80+ | 1.00 | 0.89+ | ≤2 |
| Phase 2 | 0.85+ | 1.00 | 0.91+ | ≤2 |
| Phase 3 | 0.85+ | 1.00 | 0.91+ | ≤2 (+ operational gains) |
| Phase 4 | 0.88+ | 1.00 | 0.93+ | ≤1 |
| Phase 5 | 0.90+ (over time) | 1.00 | 0.95+ | → 0 (with learning) |
| Phase 6 | optimal config | | | empirically determined |
