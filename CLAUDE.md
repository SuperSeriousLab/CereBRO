# CLAUDE.md — CereBRO Project Orientation

## What This Is

CereBRO is a biomimetic cognitive architecture that wires deterministic
reasoning COGs into a 5-layer brain-inspired pipeline.

**Two Modes:**
- `CEREBRO_MODE=deterministic` (default) — zero external HTTP calls, pure fuzzy logic pipeline
- `CEREBRO_MODE=enriched` — SLR + Sophrim active for model-tier routing and semantic enrichment

COGs are registered in GEARS. CereBRO composes them.
COGs are organs. CereBRO is the brain.

## Orient Yourself

Every session, run: `./scripts/cerebro-orient`

## Key Documents (read in order)

1. `docs/PRINCIPLES.md` — 7 biomimetic design principles
2. `docs/ARCHITECTURE.md` — 5-layer specification
3. `proto/cerebro/v1/cerebro.proto` — CereBRO contracts
4. `proto/cog/reasoning/v1/reasoning.proto` — Cognitive domain contracts

## The Five Layers

- Layer 0: SENSORY — brainstem reflexes, <10ms
- Layer 1: THALAMIC RELAY — intake, routing, urgency, <50ms
- Layer 2: CORTICAL SPECIALISTS — detectors, <500ms
- Layer 3: INHIBITION & MODULATION — gating, <100ms
- Layer 4: INTEGRATION & META — synthesis, confidence, feedback, <200ms
- Layer 5: MEMORY & LEARNING — consolidation, async

## Current Performance

Precision: 0.83 | Recall: 1.00 | F1: 0.91 | FP: 2
Architecture competition: D-inhibitor-only won on 9-conversation corpus.
Full pipeline retains infrastructure value for harder problems.

**Deterministic Mode Highlights:**
- Scope Guard: topic lexicons + fuzzy Jaccard distance (no SLR dependency)
- Contradiction Detector: precision improved from 0% → 100% (fixed multi-hypothesis scoring)
- Multi-Project Hook: detects context changes across projects, applies decay to stale findings
- /v1/debug/recent endpoint: stream recent detections with salience scores

## File Map

```
cerebro/
├── CLAUDE.md                           # You are here
├── proto/
│   ├── cerebro/v1/cerebro.proto         #   CereBRO types (inhibition, gain, metacognition)
│   └── cog/reasoning/v1/reasoning.proto #  Cognitive domain types
├── gen/go/                             # Generated Go from protos
├── internal/
│   ├── pipeline/                       # The 5-layer cognitive pipeline
│   │   ├── pipeline.go                 #   Pipeline entry point (Run)
│   │   ├── layer0.go                   #   Layer 0: format, toxicity, language
│   │   ├── intake.go                   #   Layer 1: conversation enrichment
│   │   ├── urgency.go                  #   Layer 1: urgency assessment → GainSignal
│   │   ├── router.go                   #   Layer 1: detector activation
│   │   ├── modulator.go               #   Layer 3: threshold modulation
│   │   ├── detectors.go               #   Layer 2: 6 cognitive detectors
│   │   ├── variants.go                #   Layer 2: 4 variant detectors
│   │   ├── inhibitor.go               #   Layer 3: context inhibitor (5-gate)
│   │   ├── salience.go                #   Layer 3: salience filter
│   │   ├── scope_guard.go             #   Layer 3: Scope Guard (topic lexicons + fuzzy Jaccard)
│   │   ├── aggregator.go              #   Layer 4: finding synthesis
│   │   ├── confidence.go              #   Layer 4: self-confidence assessor
│   │   ├── feedback.go                #   Layer 4: feedback evaluator
│   │   ├── consolidator.go            #   Layer 5: memory consolidator
│   │   ├── patternindex.go            #   Shared: thread-safe pattern index
│   │   ├── sophrim_client.go           #   Sophrim semantic similarity (enriched mode only)
│   │   ├── arch_variants.go           #   Architecture competition variants
│   │   └── competition.go             #   Competition runner + scoring
│   └── textutil/                       # Shared text processing
├── data/
│   ├── corpus/                         # Labeled test corpus (NDJSON)
│   ├── test-conversations/             # 9 test conversations + expected.json
│   ├── competitions/                   # Architecture competition results
│   ├── blocklists/                     # Toxicity gate blocklist
│   └── language-profiles/              # Trigram frequency profiles
├── compositions/                       # Pipeline composition manifests
├── docs/                               # Architecture documentation
├── scripts/
│   ├── protogen.sh                     # Proto → Go codegen
│   └── cerebro-orient                  # Session orientation
└── cogs/                               # Standalone COG binaries (format-validator, toxicity-gate, etc.)
```

## Architecture Rules

- **Contracts first.** Proto definitions before code.
- **No lateral knowledge.** COGs never import each other.
- **Go + Rust only.** No Python.
- **Deterministic by default.** All detectors work in pure-fuzzy mode. External services (SLR, Sophrim) are opt-in via CEREBRO_MODE=enriched.
- **Sophrim delegation.** LLM-powered semantic similarity moved to Sophrim (POST /semantic-similarity). CereBRO calls it only in enriched mode.

## Relationship to GEARS

CereBRO depends on GEARS. GEARS doesn't know CereBRO exists.
For now, COG logic is compiled into the pipeline package directly.
Future: COGs run as processes, CereBRO orchestrates via bus.
