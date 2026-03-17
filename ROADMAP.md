# CereBRO — Roadmap

> **Created:** 2026-03-17
> **Status:** Phases 1-6 complete. Nightly Loop automated. Lamarckian Loop proven.

## Vision

Biomimetic 5-layer cognitive pipeline that detects reasoning failures in
conversations. Domain-adaptive: modern analytical text (F1=0.91) and classical
philosophical text (F1=0.45). Self-improving via the Lamarckian Loop.

## Completed Phases

| Phase | Name | Key Outcome |
|-------|------|-------------|
| Cognitive 1 | Core Detectors | Anchoring, Sunk-Cost, Intake, Aggregator |
| Cognitive 2 | Full Tier 1 | 11 COGs, Router, Gateway |
| CORTEX 1 | Context Inhibitor | Precision 0.43→0.82 |
| CORTEX 2 | Urgency + Modulation | Threshold adaptation |
| CORTEX 3 | Layer 0 Reflexes | Format, toxicity, language gates |
| CORTEX 4 | Metacognition | Self-Confidence + Feedback (F1→0.91) |
| CORTEX 5 | Memory | Salience Filter + Consolidator |
| CORTEX 6 | Architecture Competition | D-inhibitor-only wins modern |

**Also complete (session work, not phased):**
- Domain-adaptive variant selection (RunAdaptive)
- 2 Tier 2 COGs: Conceptual Anchoring, Inherited-Position
- Lamarckian Loop: all 3 connections wired (A: feedback, B: domain hints, C: forge ingestion)
- Nightly Loop automation: 3 batches (generator, verifier, orchestrator, watchdog)
- 2 Forge cycles: F1 0.434→0.496 (+14.3%)
- Classical domain markers + formality fix + Porter-lite stemmer
- forge-eval subprocess evaluator (7 evolvable parameters)

## Current Metrics

| Metric | Modern (9 convos) | Full corpus (122) |
|--------|-------------------|-------------------|
| F1 | 0.91 | 0.496 |
| Precision | 0.83 | 0.545 |
| Recall | 1.00 | 0.456 |
| Variant | D-inhibitor-only | Adaptive (D modern + E classical) |

## Active Phases

### Phase 7: Production Deployment

Activate the nightly loop. Let the system run autonomously and collect
real data before building more detectors.

- [ ] Activate cron (`./scripts/setup-cron.sh` — user confirmation gate)
- [ ] Monitor first 7 nightly runs — review morning reports
- [ ] Verify consolidator produces corpus entries from generated conversations
- [ ] Run Lamarckian Cycle 3 on organically expanded corpus
- [ ] Multi-objective sweep: modern F1 ≥ 0.90 constraint + maximize classical F1
- [ ] Document production baseline (Cycle 3 results)

### Phase 8: Corpus Depth

The Forge found diminishing returns on parameters. Future F1 gains come
from corpus quality, not calibration.

- [ ] Grow corpus to 200+ entries via nightly consolidation
- [ ] Add borderline conversations (almost-triggers for Tier 2 detectors)
- [ ] Adversarial generator: evolve scope-drift-resistant conversations
- [ ] Classical library expansion: Meno, Gorgias dialogues
- [ ] Corpus diversity audit: 40% modern, 30% classical, 20% technical, 10% adversarial

### Phase 9: Tier 2 Completion (5 remaining COGs)

Build only what the corpus proves is needed. After Phase 8 corpus expansion,
measure which failure modes are uncovered. Build COGs for those.

- [ ] Assumption Surfacer — unstated premises
- [ ] Circular Reasoning Detector — premise depends on conclusion
- [ ] Evidence Quality Scorer — anecdotal vs systematic
- [ ] Status-Quo Bias Detector — default-option preference
- [ ] Entity Coherence Monitor — identity consistency

Each: ~250 LOC, PURE/deterministic, same DetectorFunc pattern. Spec in
`docs/TIER2_SPECS.md`. Build one, measure, then decide on next.

### Phase 10: Cross-Domain — Code Review

Second domain deployment. Reuse 80% of pipeline (Layers 0, 1, 3, 4, 5).
Swap Layer 2 detectors for code-analysis variants.

- [ ] Design code-review detector specs (scope-creep, inconsistency, intent-extractor)
- [ ] Build diff-intake COG (parse unified diff → ConversationSnapshot equivalent)
- [ ] Hand-craft 20 PR corpus entries with labeled findings
- [ ] Competition: which architecture variant wins on code review?
- [ ] Document: does domain-adaptive selection generalize to code domain?

### Phase 11: Architecture Evolution

Use AIP Forge to evolve not just parameters but pipeline composition.

- [ ] Vary: Layer 0 inclusion, Layer 2 detector set, Layer 3 thresholds, feedback on/off
- [ ] Competition: domain-adaptive variants per domain (modern, classical, technical, code)
- [ ] Pareto frontier: F1 vs latency vs stage count
- [ ] Graduate winners to GEARS manifests

## Deferred / Not Planned

- **ML Enricher revival** — PURE pipeline wins. ML archived. Revisit only if dedicated GPU available.
- **Real-time streaming** — Pipeline is batch (per-conversation). Streaming adds complexity without proportional value at current scale.
- **Multi-language** — Language Detector exists but all detectors are English-only. Defer until non-English corpus exists.
- **Containerization** — Prove the loop locally first. Deploy to Proxmox LXC only when nightly loop is stable for 30 days.

## Code Stats

```
Pipeline modules:   50 files
COG binaries:       10
Tests:              262
Corpus entries:     122 (full-v3)
Forge cycles:       2 complete
Nightly scripts:    9
```
