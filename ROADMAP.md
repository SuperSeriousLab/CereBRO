# CereBRO — Roadmap

> **Created:** 2026-03-17
> **Status:** Phases 1-6 complete. Phase 7 in progress (production deployment, cerebro-hook live). Lamarckian Loop proven.

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

Status: **In Progress** (P7.1-P7.4 delegated, P7.4 complete)

- [x] 7.1 Provision LXC container on Proxmox
- [x] 7.2 Build and deploy CereBRO gRPC service with fugo fuzzy components
- [ ] 7.3 Nightly cron pipeline execution (persist findings)
- [x] 7.4 Replace FuzzyGuard hooks with cerebro-hook binary (536us pipeline, 11 tests)
- [ ] 7.5 Stability gate — 7 consecutive nightly runs without failure
- [ ] 7.6 Deploy skill and service registry update

**Exit:** CereBRO running as production service, real-time Claude Code monitoring via cerebro-hook, nightly pipeline stable for 7 days.

### Phase 8: Corpus Expansion

- [ ] 8.1 Generate 300+ new conversations via SLR (diverse topics, all 4 novel pathology types)
- [ ] 8.2 Import to shared corpus format (CereBRO + FuzzyGuard + DORIANG compatible)
- [ ] 8.3 Re-run Genesis on expanded corpus — discover new patterns
- [ ] 8.4 EDD gate: validate expanded corpus quality (no duplicates, balanced distribution)

**Exit:** 500+ corpus entries with coverage across all known pathology types. Genesis discovers additional patterns.

### Phase 9: Genesis COG Graduates

4 novel patterns discovered by FuzzyGuard Genesis on the shared corpus. Two ready to implement, two deferred.

- [ ] 9.1 CounterEvidenceDepletionCOG (Tier 2, stateless) — reasoning persists without counter-evidence. 29 sessions, 3 Genesis rules with fitness >90, precision 1.0. Inputs: NegEvidence ratio, MaxMV of recent claims.
- [ ] 9.2 DirectionalLockCOG (Tier 2, stateless) — certainty disconnected from evidence quality. 20 sessions. Inputs: DirectionEntropy, confidence trends. Warning at entropy 0.90, detected at 0.60.
- [ ] 9.3 Forge-evolve fugo FIS configs for new COGs on expanded corpus
- [ ] 9.4 Re-validate F1 with new COGs + fuzzy pipeline (target: F1 > 0.91)
- [ ] 9.5 EDD gate: fuzz new COGs, boundary tests, no regression on existing detectors

**Deferred COGs:**
- SilentRevisionCOG (Tier 3, stateful) — needs GEARS contract extension for per-claim evidence snapshots. 7 corpus sessions.
- InheritedPositionCOG (Tier 1, stateless) — thin corpus coverage (3 sessions). Defer until corpus has 20+ examples.

**Exit:** 2 new COGs deployed in fuzzy pipeline. F1 maintained or improved. No regression.

### Phase 10: Cross-Domain — Code Review Deployment

- [ ] 10.1 Adapt pipeline for code review conversations (PR reviews, architecture discussions)
- [ ] 10.2 Generate code-review-specific corpus (50+ conversations)
- [ ] 10.3 Calibrate detector thresholds for code review domain (Forge-evolve FIS configs)
- [ ] 10.4 Deploy code review mode alongside conversation mode

**Exit:** CereBRO handles both conversational and code review reasoning detection.

### Phase 11: Forge Evolution + Online Learning

This is where fugo's P3.2 (Forge integration) and P2 (bandit learning) pay off.

- [ ] 11.1 Forge-evolve ALL fugo FIS configs on 500+ corpus (offline optimization)
- [ ] 11.2 Compare Forge-evolved vs hand-tuned FIS on held-out test set
- [ ] 11.3 Activate fugo bandit online learning in production — FIS rule weights tune themselves
- [ ] 11.4 Measure F1 improvement over 30 days of online learning
- [ ] 11.5 EDD gate: verify no drift, no degradation, bandit convergence

**Exit:** CereBRO self-improving in production. F1 trend positive over 30 days. Bandit converged.

## Deferred / Not Planned

- **ML Enricher revival** — PURE pipeline wins. ML archived. Revisit only if dedicated GPU available.
- **Real-time streaming** — Pipeline is batch (per-conversation). Streaming adds complexity without proportional value at current scale.
- **Multi-language** — Language Detector exists but all detectors are English-only. Defer until non-English corpus exists.

## Code Stats

```
Pipeline modules:   50 files
COG binaries:       10
Tests:              273 (incl. 11 cerebro-hook)
Corpus entries:     197 (full-v3 + Genesis imports)
Forge cycles:       2 complete
Nightly scripts:    9
Production:         LXC deployed, cerebro-hook live (536us)
```
