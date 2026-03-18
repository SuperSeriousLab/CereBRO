# Evidence Asymmetry Detector

CereBRO Layer 2, Cortical Specialists — Evidence Grounding Analysis.

Combined implementation of genesis rules **gen4_78** (positive claims under-evidenced)
and **gen4_86** (negative claims over-evidenced) as a single ratio detector.

## Signal

Computes the evidence grounding ratio across all assistant turns:

```
evidence_asymmetry = avg_evidence_links(negative claims) / avg_evidence_links(positive claims)
```

When this ratio > 1.5, the agent is systematically better at grounding negative
claims than positive ones — a structural signature of CONFIDENCE_MISCALIBRATION.

## Threshold Zones

| Ratio | Zone | Action |
|-------|------|--------|
| < 1.0 | Healthy | No finding |
| 1.0 – 1.5 | Borderline | No finding (below threshold) |
| > 1.5 | Miscalibrated | `CONFIDENCE_MISCALIBRATION` finding |
| > 2.25 | Severe | `CONFIDENCE_MISCALIBRATION` CRITICAL |

## Method

1. Splits each assistant turn into sentence-level segments.
2. Classifies each segment as a positive or negative claim using keyword heuristics.
3. Counts evidence-link markers per segment.
4. Computes per-direction averages and the ratio.
5. Emits finding when ratio exceeds threshold.

## Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| miscalibration_threshold | 1.5 | Ratio above which finding fires |
| borderline_threshold | 1.0 | Lower boundary of borderline zone |
| min_assistant_turns | 2 | Minimum assistant turns before detection |
