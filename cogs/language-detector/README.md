# language-detector

**CereBRO Phase 3 -- Layer 0 (Brainstem Reflexes)**

PURE deterministic COG. Detects conversation language using trigram
frequency analysis (Cavnar & Trenkle 1994).

## Algorithm

1. Extract sample text (first 500 chars from all turns)
2. Count character trigram frequencies
3. Compare against pre-computed language profiles via cosine similarity
4. Best match above min_confidence -> detected
5. Below min_confidence or too short -> fallback

## Config

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| supported_languages | string | "en" | Comma-separated supported codes |
| min_confidence | double | 0.7 | Min cosine similarity for detection |
| fallback_language | string | "en" | Fallback when detection fails |
| min_sample_chars | uint32 | 20 | Min chars for reliable detection |
