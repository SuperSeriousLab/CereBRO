# Task: Artisan Audit + ML Enrichment COG

> Saved task spec for CereBRO. Two parts: Part A (craft audit), Part B (ML enrichment via Ollama).
> Run from the CereBRO repo at /home/jsevcu/CereBRO

---

Read CLAUDE.md, README.md, and run ./scripts/cerebro-orient.
Then read:
- docs/ARCHITECTURE.md (Layer 1 and Layer 2 sections)
- docs/PRINCIPLES.md (§2 parallel specialists, §4 neuromodulation)
- docs/CROSS_DOMAIN.md (reuse thesis)
- internal/pipeline/pipeline.go (all stages, DetectorFunc interface)
- internal/pipeline/intake.go (current PURE enrichment)
- internal/pipeline/detectors.go (what detectors consume)

## Mission — Two Parts

**Part A:** Artisan audit — now that CereBRO is its own repo, review the
migrated code with fresh eyes. Fix craft issues that accumulated across
16 phases of rapid building. Polish, not rewrite.

**Part B:** Build the ML Enrichment COG — CereBRO's first HYBRID COG using
the local Ollama LLM for richer structured extraction.

Part A first. The codebase should be pristine before adding new capability.

---

## PART A — Artisan Improvement Audit

This is a craft pass. Read every .go file in internal/pipeline/ as if you're
seeing it for the first time. You're looking for things that work but could
be better — not bugs, not features, but quality of craft.

### A1. Naming audit

Read every exported function, type, and constant. Ask:
- Does the name say what it does without reading the body?
- Are similar things named consistently?
- Are there names that made sense in GEARS context but are confusing in CereBRO context?
- Are acronyms consistent? (FP vs FalsePositive, TP vs TruePositive)

Fix names that are unclear. Don't rename things that are clear enough.

### A2. Function signature consistency

Review all DetectorFunc implementations:
- Do they all follow the same parameter pattern?
- Are config structs consistent?
- Could any benefit from extracting a shared helper?

Review pipeline stages:
- Consistent signature pattern?
- Clear data flow from types?
- Unused parameters?

### A3. Error handling craft

For each error path:
- Specific enough to diagnose?
- Includes context (stage, detector, turn)?
- Wrapped with %w?
- Any silently swallowed errors?

### A4. Comment quality

Classify each comment: Useful (keep), Stale (update/remove), Obvious (remove), Missing (add).
Pay special attention to: inhibitor gates, formality computation, feedback loop, calibrated thresholds.

### A5. Test quality

- Descriptive test names?
- Table-driven tests where appropriate?
- Edge cases tested?
- Tautological assertions?
- Commented-out tests?

### A6. Dead code and leftovers

Remove dead code. Resolve TODOs. Convert FIXMEs to fixes or documented decisions.

### A7. Package documentation

Write doc.go if missing. Update if references GEARS or CORTEX.

### A8. Data file audit

- Corpus entries parse correctly?
- expected.json matches current behavior?
- No stale cortex references in data files?

### A9. Script audit

Run cerebro-orient and protogen.sh. Fix anything wrong.

### A10. Go module hygiene

go mod tidy, go vet, staticcheck (if available).

### Part A commit: "cerebro: artisan audit — naming, errors, comments, tests, hygiene"

---

## PART B — ML Enrichment COG

### Context

Current pipeline enriches mechanically (keywords, numeric tokens, capitalization).
ML enricher uses local Ollama LLM for richer structured extraction.
ML enriches INPUT; detectors stay deterministic.

### Resources

- Ollama at http://10.70.70.14:11434
- Model: glm-4.7-flash:q4_K_M
- API: POST http://10.70.70.14:11434/api/chat
- Verify reachable before starting Part B

### Deliverable 1: ML Enricher (mlenricher.go)

Proto (cerebro.proto): MLEnrichment, MLClaim, MLAnchorRef, MLDecisionPoint, MLFormalityIndicators

Config:
- OllamaURL (default "http://10.70.70.14:11434", env CEREBRO_OLLAMA_URL)
- Model (default "glm-4.7-flash:q4_K_M")
- TimeoutPerTurn (default 5000ms)
- MaxRetries (default 1)
- FallbackToPure (default true)
- Temperature (default 0.1)
- Enabled (default false)

Call once per turn. Parse JSON response. Fallback on failure. Never crash.

### Deliverable 2: Detector enrichment consumption

- Sunk-Cost: check MLEnrichment.sunk_cost_phrases
- Anchoring: check MLEnrichment.anchoring_references relevance
- Confidence Calibrator: check ML confidence_markers + claim epistemic mismatches
- Urgency Assessor: use ML formality indicators

All guarded with `if ml != nil`. PURE first, ML adds on top. Never remove PURE findings.

### Deliverable 3: Pipeline variant with ML enrichment

Stage 1.3: ML Enricher (if enabled). MLEnrichedConfig() factory function.

### Deliverable 4: Competition — PURE vs ML

15-20 corpus entry subset. Measure accuracy + latency + ML reliability.
3 profiles: accuracy-first, speed-first, balanced.

### Deliverable 5: Results documentation

data/competitions/ML_ENRICHMENT_RESULTS.md with comparison table, per-detector analysis,
LLM reliability, latency analysis, architectural recommendation.

### Tests

Unit tests with mock Ollama (httptest). Integration tests with real Ollama (//go:build integration).
All detectors produce identical output when ml=nil.

### Exit Criteria

Part A: naming, signatures, errors, comments, tests, dead code, docs, data, scripts, hygiene.
Part B: proto, mlenricher.go, detector enhancements, variant F, unit tests, integration tests, competition, results doc.

### What NOT To Do

- Don't fine-tune Ollama model
- Don't replace PURE detectors with ML
- Don't depend on Ollama availability
- Don't run full corpus through ML in CI
- Don't call real Ollama in unit tests
- Don't add ML to architecture competition variants

### Commit Strategy

1. "cerebro: artisan audit — naming, errors, comments, tests, hygiene" (Part A)
2. "cerebro: ml enricher — ollama integration + detector enhancements" (Part B code)
3. "cerebro: ml competition results" (Part B results + documentation)
