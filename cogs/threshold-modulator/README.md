# Threshold Modulator

CereBRO Phase 2, Layer 2 — Gain Modulation.

Translates a GainSignal into per-detector threshold adjustments. The gain
formula balances urgency (increases sensitivity), formality (decreases
sensitivity when informal), and complexity (increases sensitivity).

## Gain Formula

```
raw_gain = -(urgency_weight * urgency) + (formality_weight * (1 - formality)) - (complexity_weight * complexity)
offset = clamp(raw_gain, -max_gain_offset, +max_gain_offset)
```

## Threshold Application

```
effective_threshold = base_threshold * (1.0 + offset)
```

Note: Scope Guard's DriftThreshold is excluded from gain modulation (Forge-optimized).

## Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| max_gain_offset | 0.15 | Maximum absolute offset |
| urgency_weight | 0.6 | Urgency contribution (negative = more sensitive) |
| formality_weight | 0.3 | Inverse formality contribution (positive = less sensitive) |
| complexity_weight | 0.1 | Complexity contribution (negative = more sensitive) |
