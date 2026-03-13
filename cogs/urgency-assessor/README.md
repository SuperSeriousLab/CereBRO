# Urgency Assessor

CereBRO Phase 2, Layer 2 — Gain Modulation.

Keyword-based urgency, complexity, and formality assessor. Scans recent turns
for urgency/stakes keywords, computes structural complexity from turn count and
speaker diversity, and reuses the formality heuristic from the Context Inhibitor.

Produces a `GainSignal` consumed by the Threshold Modulator.

## GainSignal Fields

- **Urgency** (0.0–1.0): baseline 0.15 + keyword hits
- **Complexity** (0.0–1.0): weighted from turn count, speakers, turn length
- **Formality** (0.0–1.0): formal vs informal marker ratio
- **Mode**: PHASIC (urgency > 0.6) or TONIC

## Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| phasic_urgency_threshold | 0.6 | Urgency above this triggers PHASIC mode |
| complexity_turn_threshold | 10 | Turn count normalization threshold |
| recent_turn_window | 5 | How many recent turns to scan for keywords |
