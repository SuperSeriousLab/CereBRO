# format-validator

**CereBRO Phase 3 — Layer 0 (Brainstem Reflexes)**

PURE deterministic COG. Validates input ConversationSnapshot structure,
encoding, and size limits before expensive cognitive processing.

## Checks

- Non-nil snapshot with at least one turn
- Turn count within configurable limit (default 500)
- Total text size within configurable limit (default 1MB)
- Valid UTF-8 in all text fields

## Config

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| max_input_bytes | uint32 | 1048576 | Max total text size |
| max_turns | uint32 | 500 | Max number of turns |
