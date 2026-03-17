---
name: cerebro
description: >
  CereBRO cognitive pipeline — build, test, run evaluations, and operate the
  biomimetic 5-layer reasoning pipeline. Use this skill whenever the user mentions
  cerebro, pipeline, forge-eval, nightly loop, forge sweep, conversation generation,
  COG competition, corpus, preflight, or asks to run/test/eval CereBRO. Also trigger
  on: "run the pipeline", "evaluate conversations", "forge sweep", "morning report",
  "nightly loop", "run preflight", "competition results", "build cerebro".
---

# CereBRO — Cognitive Pipeline Operations

Build, test, and run the CereBRO 5-layer biomimetic cognitive pipeline.
All operations run from `/home/js/eidos/CereBRO`.

## Project Context

| Key | Value |
|-----|-------|
| Path | `/home/js/eidos/CereBRO` |
| Language | Go |
| Pipeline layers | 0: Sensory (<10ms), 1: Thalamic (<50ms), 2: Cortical (<500ms), 3: Inhibition (<100ms), 4: Integration (<200ms), 5: Memory (async) |
| Current metrics | Precision: 0.83, Recall: 1.00, F1: 0.91, FP: 2 |
| SLR endpoint | `http://192.168.14.69:8081` |

## Session Start — Orient

```bash
cd /home/js/eidos/CereBRO
./scripts/cerebro-orient
```

## Build & Test

```bash
cd /home/js/eidos/CereBRO

# Build all binaries
go build ./...

# Run all tests
go test ./...

# Run pipeline tests only (fast, no corpus)
go test ./internal/pipeline/ -count=1 -short

# Run forge-eval (precision/recall against test conversations)
go run ./cmd/forge-eval/ -data data/test-conversations/ -expected data/test-conversations/expected.json
```

## Forge Evaluation (Competition)

Forge-eval tests the pipeline against labeled conversations and measures F1.

```bash
cd /home/js/eidos/CereBRO

# Quick eval (default corpus)
go run ./cmd/forge-eval/ -data data/test-conversations/

# Full eval with expected output comparison
go run ./cmd/forge-eval/ -data data/test-conversations/ -expected data/test-conversations/expected.json

# Forge sweep — test all architecture variants
./scripts/forge-sweep.sh
```

## Nightly Loop

The nightly loop generates conversations, runs detections, and updates the corpus.

```bash
cd /home/js/eidos/CereBRO

# Preflight check (verify all dependencies: SLR, disk, corpus)
./scripts/preflight-check.sh

# Full nightly loop (runs forge sweep, updates corpus)
./scripts/nightly-loop.sh

# Individual steps:
./scripts/generate-conversations.sh    # generate test conversations via SLR
./scripts/feed-cold-queue.sh           # feed unprocessed conversations to pipeline
./scripts/verify-findings.sh           # verify findings against expected
./scripts/finding-to-corpus.sh         # promote findings to corpus
```

## Morning Operations

```bash
cd /home/js/eidos/CereBRO

# Morning check (quick status: pipeline health, corpus size, recent findings)
./scripts/morning-check.sh

# Continuous morning report
./scripts/morning-report-continuous.sh
```

## Corpus Management

```bash
cd /home/js/eidos/CereBRO

# Convert findings to snapshot format
./scripts/convert-to-snapshots.sh

# Corpus files location
ls data/corpus/          # labeled NDJSON files
ls data/test-conversations/  # 9 test conversations + expected.json

# Corpus line counts
wc -l data/corpus/*.ndjson
```

## Batch Processing

```bash
cd /home/js/eidos/CereBRO

# Run batch on a directory of conversations (NDJSON)
go run ./cmd/cerebro-batch/ -input <dir> -output <out.ndjson>

# Diagnostic run
go run ./cmd/diag/ <input.ndjson>
```

## Proto Codegen

```bash
cd /home/js/eidos/CereBRO
./scripts/protogen.sh
```

Proto files:
- `proto/cerebro/v1/cerebro.proto` — pipeline types
- `proto/cog/reasoning/v1/reasoning.proto` — cognitive domain types

## Pipeline Architecture (quick ref)

```
Input
  └── Layer 0: Sensory (format, toxicity, language) <10ms
        └── Layer 1: Thalamic (intake, routing, urgency) <50ms
              └── Layer 2: Cortical (6 detectors + 4 variants) <500ms
                    └── Layer 3: Inhibition (5-gate inhibitor, salience) <100ms
                          └── Layer 4: Integration (aggregator, confidence, feedback) <200ms
                                └── Layer 5: Memory (async consolidation)
```

## Key Files

| File | Purpose |
|------|---------|
| `internal/pipeline/pipeline.go` | Pipeline entry point (`Run`) |
| `internal/pipeline/detectors.go` | 6 cognitive detectors |
| `internal/pipeline/inhibitor.go` | 5-gate context inhibitor |
| `internal/pipeline/competition.go` | Architecture competition runner |
| `internal/pipeline/mlenricher.go` | ML enricher (Ollama integration) |
| `data/test-conversations/expected.json` | Ground truth for eval |
| `compositions/*.textpb` | Pipeline composition manifests |

## Rules

- **Contracts first.** Proto before code changes.
- **No lateral COG imports.** COGs never import each other.
- **Go only** (plus Rust for future Rust COGs).
- Architecture winner stays until a challenger beats F1 0.91+.
