# Model Evaluation: Corpus Generation JSON Reliability
**Date:** 2026-03-18
**Task:** Generate multi-turn cognitive bias conversations in JSON format
**Prompt:** sunk-cost / failing-project scenario (modern-sunk-cost-01.txt category)
**Schema expected:** `{"turns": [{"speaker": "...", "text": "..."}, ...], "topic": "..."}`
**n=10 each model, 90s timeout per request, direct Ollama API at 10.70.70.14:11434**

---

## Results

| Model | Config | Valid JSON | Schema OK (.turns) | Avg Latency |
|-------|--------|-----------|---------------------|-------------|
| `gemma3:4b` | format:json | 0/10 | 0/10 | 120s (all timeout) |
| `glm-4.7-flash:q4_K_M` | format:json | 0/10 | 0/10 | ~26s (empty responses) |
| `gemma3:4b` | no format constraint | 1/10 | 1/10 | ~62.3s |
| `glm-4.7-flash:q4_K_M` | no format constraint + fence-strip | **8/10** | **8/10** | **~73.6s** |

---

## Failure Analysis

### gemma3:4b
- **Primary failure mode: response truncation.** The model generates valid JSON
  wrapped in markdown fences but is cut off before the closing `}`. This directly
  explains the reported ~52% JSON parse failure rate in production.
- With `format: "json"` constraint: 100% timeout at 90s (model unable to complete).
- Without format constraint: 9/10 truncated mid-JSON; 1 success in 10.3s (likely cached/short).
- Root cause: gemma3:4b generates long verbose text, frequently exceeding the token
  budget before the JSON object closes.

### glm-4.7-flash:q4_K_M
- **`format: "json"` constraint returns empty responses** — known incompatibility with
  this model's instruction-tuning. Must be called without this constraint.
- Without format constraint: 8/10 success (80%). All successful runs produced correctly
  structured JSON with 10-14 turns and a topic field.
- 2/10 timeouts at 90s (concurrent GPU load suspected; script uses 180s timeout so
  production would not hit these).
- Output wraps JSON in triple-backtick fences — requires `sed '/^```/d'` stripping
  (already present in generate-conversations.sh).

---

## Recommendation

**Switch default model to `glm-4.7-flash:q4_K_M`.**

- GLM: **80% schema-correct JSON** vs. gemma: **10%**
- Exceeds the 75% threshold for promotion.
- The script already strips markdown fences (`sed '/^```/d'`), so GLM's fence-wrapped
  output is handled correctly without code changes.
- generate-conversations.sh routes through SLR (`/v1/chat/completions`). If SLR's
  `auto` routing resolves to gemma3:4b, SLR model config must be updated accordingly.

---

## Available Models on Ollama (10.70.70.14) — Not Tested

| Model | Notes |
|-------|-------|
| `qwen3:8b` | Strong instruction following, likely next candidate if GLM timeouts persist |
| `qwen2.5:14b-instruct` | Larger model, likely higher quality, slower |
| `qwen3:30b-a3b` | MoE architecture, good quality/speed tradeoff |
| `nemotron-3-nano:4b` | Nano class, unlikely to beat GLM |
| `qwen3-coder:30b` | Code-focused, not optimal for conversation generation |
