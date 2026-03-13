# CereBRO

A biomimetic cognitive architecture that wires deterministic reasoning
components into a brain-inspired 5-layer pipeline.

Built on [GEARS](https://github.com/SuperSeriousLab/GEARS) — the registry
and compliance engine that stores, enforces, and wires the components
CereBRO composes.

## Current Performance

| Metric | Value |
|--------|-------|
| Precision | 0.83 |
| Recall | 1.00 |
| F1 | 0.91 |
| Pipeline | v7 (21 COGs, 30 wires, 5 layers) |

## Architecture

```
Layer 0: Sensory gating — format, toxicity, language (<10ms)
Layer 1: Thalamic relay — intake, routing, urgency (<50ms)
Layer 2: Cortical specialists — bias/fallacy detection (<500ms)
Layer 3: Inhibition & modulation — false positive suppression (<100ms)
Layer 4: Integration & meta — synthesis, confidence, feedback (<200ms)
Layer 5: Memory & learning — consolidation, async
```

## Build & Test

```bash
# Generate Go code from protos
bash scripts/protogen.sh

# Build
go build ./...

# Test
go test ./...

# Orientation
./scripts/cerebro-orient
```

## Documentation

| Document | Content |
|----------|---------|
| [Principles](docs/PRINCIPLES.md) | 7 biomimetic design principles |
| [Architecture](docs/ARCHITECTURE.md) | 5-layer specification |
| [Evolution](docs/EVOLUTION.md) | Three timescales of adaptation |
| [New COGs](docs/NEW_COGS.md) | 10 COG specifications |
| [Contracts](docs/CONTRACTS.md) | Proto message specifications |
| [Build Order](docs/BUILD_ORDER.md) | 6 implementation phases |
| [Cross-Domain](docs/CROSS_DOMAIN.md) | Code review, finance, legal, education |

## Architecture Competition Results

5 pipeline variants competed across 9 test conversations:

| Variant | Precision | Recall | F1 | Latency | Winner? |
|---------|-----------|--------|------|---------|---------|
| A-full-cortex | 0.83 | 1.00 | 0.91 | 1.83ms | |
| B-no-feedback | 0.83 | 1.00 | 0.91 | 1.87ms | |
| C-no-modulation | 0.83 | 1.00 | 0.91 | 2.36ms | |
| **D-inhibitor-only** | **0.83** | **1.00** | **0.91** | **1.08ms** | **All profiles** |
| E-pre-cortex | 0.67 | 1.00 | 0.80 | 0.95ms | |

Context Inhibitor provides 100% of precision improvement on current corpus.
Other layers add infrastructure for production workloads.

## License

See [GEARS](https://github.com/SuperSeriousLab/GEARS) for license terms.
