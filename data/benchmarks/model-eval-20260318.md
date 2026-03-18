# Model Evaluation: Corpus Generation — 2026-03-18

## Task
Generate 10-turn cognitive bias conversations in JSON format: `{turns: [...], topic: "..."}`.

## Results
| Model | Valid JSON | Schema OK | Avg Latency | Notes |
|-------|-----------|-----------|-------------|-------|
| gemma3:4b (format:json) | 1/10 | 0/10 | 120s (timeout) | 9/10 timeout at 120s |
| glm-4.7-flash:q4_K_M (format:json) | 0/10 | 0/10 | 26s | Returns empty with format constraint |
| glm-4.7-flash:q4_K_M (no format, fence-strip) | 8/10 | 8/10 | 73.6s | 2 timeouts at 90s |

## Recommendation
Use `glm-4.7-flash:q4_K_M` without `format: json`, with markdown fence stripping. 80% success rate.
Available models on Ollama (10.70.70.14): nemotron-3-nano:4b, qwen2.5:14b-instruct, qwen3:8b, qwen3:30b-a3b, glm-4.7-flash, gemma3:4b, qwen3-coder:30b.
Future: benchmark qwen2.5:14b-instruct for structured output (likely higher quality, slower).
