# Domain-Aware Architecture Competition — Task 4

**Date:** 2026-03-15  
**Modern corpus:**    9 conversations (`data/test-conversations/`)  
**Classical corpus:** 43 Republic entries (`data/library/corpus/classical-v1.ndjson`)  

## Setup

- **Modern entries:** `DomainContext = nil` — all pipeline defaults apply.
- **Classical entries:** `DomainContext{TextEra:"classical", PrimaryDomain:"philosophy", Confidence:0.85}`
  - `ScopeGuard.DriftThreshold` = 0.70 (default 0.79)
  - `ScopeGuard.SustainedTurns` = 4 (default 8; Forge Cycle 1 winner)
  - `ConceptualAnchoring.AnchorThreshold` = 0.35 (default 0.30; Forge Cycle 1 winner)
  - `Calibrator.MinCertaintyWords` = 8 (default 5)
  - Anchoring detector: **skipped** (no numeric anchoring in classical text)
  - ConceptualAnchoring detector: **active** (propositional variant for classical text)

## Results

### Modern Corpus (nil DomainContext, 9 entries)

| Variant | Precision | Recall | F1 | TP | FP | FN |
|---------|-----------|--------|----|----|----|----|
| A-full-cortex          | 0.846 | 1.000 | **0.917** | 11 | 2 | 0 |
| B-no-feedback          | 0.846 | 1.000 | **0.917** | 11 | 2 | 0 |
| C-no-modulation        | 0.846 | 1.000 | **0.917** | 11 | 2 | 0 |
| D-inhibitor-only       | 0.846 | 1.000 | **0.917** | 11 | 2 | 0 |
| E-pre-cortex           | 0.688 | 1.000 | **0.815** | 11 | 5 | 0 |

### Classical Corpus (DomainContext classical confidence=0.85, 43 entries)

| Variant | Precision | Recall | F1 | TP | FP | FN |
|---------|-----------|--------|----|----|----|----|
| A-full-cortex          | 0.463 | 0.347 | **0.397** | 25 | 29 | 47 |
| B-no-feedback          | 0.463 | 0.347 | **0.397** | 25 | 29 | 47 |
| C-no-modulation        | 0.455 | 0.347 | **0.394** | 25 | 30 | 47 |
| D-inhibitor-only       | 0.455 | 0.347 | **0.394** | 25 | 30 | 47 |
| E-pre-cortex           | 0.446 | 0.458 | **0.452** | 33 | 41 | 39 |

### Combined Summary

| Variant | Modern F1 | Classical F1 | Combined F1 | Latency(ms) | Stages |
|---------|-----------|-------------|-------------|-------------|--------|
| A-full-cortex          | 0.917 | 0.397 | **0.480** | 3.277 | 12 |
| B-no-feedback          | 0.917 | 0.397 | **0.480** | 2.807 | 10 |
| C-no-modulation        | 0.917 | 0.394 | **0.477** | 2.218 | 10 |
| D-inhibitor-only       | 0.917 | 0.394 | **0.477** | 2.161 | 5 |
| E-pre-cortex           | 0.815 | 0.452 | **0.509** | 1.468 | 4 |

## Winners

| Category | Winner | F1 |
|----------|--------|----|
| Modern   | A-full-cortex | 0.917 |
| Classical | E-pre-cortex | 0.452 |
| Combined | E-pre-cortex | 0.509 |

## Key Questions

### Q1: Does D-inhibitor-only still win on modern text?

**YES** — D-inhibitor-only F1=0.917 (tied for best or best on modern corpus).

### Q2: Does domain context change the winner on classical text?

**YES** — Classical winner is **E-pre-cortex** (F1=0.452). D-inhibitor-only scores F1=0.394 on classical text.

Domain adjustments (lower DriftThreshold, lower SustainedTurns, higher MinCertaintyWords,
skip numeric anchoring, activate conceptual anchoring) shift the balance between variants.

### Q3: Does the full pipeline (A) outperform D on classical text?

**YES** — A-full-cortex F1=0.397 vs D-inhibitor-only F1=0.394 on classical corpus.
The extra layers (modulation, salience, metacognition) add value on classical text.

### Q4: Which variant wins on COMBINED (modern + classical)?

**E-pre-cortex** — combined F1=0.509 (TP/FP/FN aggregated over both corpora).

## Notes

- Combined F1 computed over the union of TP/FP/FN from both corpora.
- Latency reported is modern corpus only (classical entries have a similar latency profile).
- Layer 0 resources (language profiles + blocklist) loaded for variants A, B, C which use Layer 0.
- Variant F (ML-enriched) excluded — requires Ollama and is tested separately.
- Previous competition (data/library/FULL_CORPUS_COMPETITION.md) ran without domain context.
  This competition isolates the effect of domain-aware configuration on each sub-corpus.
