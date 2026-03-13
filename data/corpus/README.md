# Corpus Data

Labeled test corpora for AIP evaluation pipelines. Each corpus file is in NDJSON
format (one JSON object per line), compatible with `internal/aip.LoadCorpus()`.

## Entry Format

```json
{
  "entry_id": "cog-001",
  "input": { ... },
  "expected": [{"finding_type": "ANCHORING_BIAS"}]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `entry_id` | string | Unique identifier (required, non-empty) |
| `input` | object | Arbitrary JSON passed to the Pinions interpreter as input |
| `expected` | array | Expected findings; each has a `finding_type` string matching proto `FindingType` enum names |

## cognitive-v1.ndjson

18 labeled conversation snapshots for cognitive bias detection. Each input is a
`ConversationSnapshot` with `turns`, `objective`, and `total_turns` fields.

### Finding type coverage

| Finding Type | Count | Entry IDs |
|---|---|---|
| ANCHORING_BIAS | 3 (+1 multi) | cog-001, cog-002, cog-003, cog-017 |
| SUNK_COST_FALLACY | 3 (+1 multi) | cog-004, cog-005, cog-006, cog-017 |
| CONTRADICTION | 2 | cog-007, cog-008 |
| SCOPE_DRIFT | 2 (+1 multi) | cog-009, cog-010, cog-018 |
| CONFIDENCE_MISCALIBRATION | 2 (+1 multi) | cog-011, cog-012, cog-018 |
| SILENT_REVISION | 2 | cog-013, cog-014 |
| (clean, no findings) | 2 | cog-015, cog-016 |
| Multiple findings | 2 | cog-017, cog-018 |

### ConversationSnapshot input schema

```json
{
  "turns": [
    {"turn_number": 1, "speaker": "alice", "raw_text": "..."}
  ],
  "objective": "What the conversation is trying to achieve",
  "total_turns": 5
}
```
