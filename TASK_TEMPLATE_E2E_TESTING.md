# Task Assignment: Comprehensive E2E Testing Until Convergence

> **Type:** Claude Code Task Template  
> **Trigger:** Run after development is complete at spec/phase/milestone level  
> **Goal:** Find all bugs before users do — achieve statistical confidence in system correctness  
> **Resources:** Compute unlimited. Ollama available at `http://10.70.70.14` (GLM 4.7 Flash, tokens free).

---

## 0. Epistemological Honesty

You cannot prove the absence of bugs (Rice's theorem). What you *can* do:

- Drive the **estimated remaining bug count** toward zero using capture-recapture statistics across independent testing methods
- Achieve **mutation testing score > 95%** (meaning >95% of injected faults are detected by the test suite)
- Reach **state coverage > 90%** of the defined state model
- Observe **finding rate decay** to zero across multiple independent discovery methods over sustained runs

When all four of these converge, you have the strongest statistical confidence practically available. That's the exit condition.

### The Capture-Recapture Framing — And Its Limits

The mathematical framing is the **Lincoln-Petersen capture-recapture estimator**: if independent method A finds bugs {a₁, a₂, a₃} and independent method B finds {a₂, a₄}, estimate total bugs ≈ (|A| × |B|) / |A ∩ B| = 6, implying ~2 undiscovered.

**Critical limitations you must internalize:**

1. **Independence assumption is violated.** The testing methods in this template are NOT statistically independent. Property-based testing and fuzzing both find input validation bugs. EDD and metamorphic testing both find state transition bugs. Correlated methods bias the estimator downward (underestimates total bugs). Treat the estimate as a lower bound, not ground truth.

2. **Small sample sizes blow up confidence intervals.** If method A finds 3 bugs and method B finds 2 with 1 overlap, the point estimate is 6 but the 95% CI is roughly [4, 30]. With typical bug counts in single digits, the math gives directional signal, not precision.

3. **Bug identity is a judgment call.** Two methods may surface the same root cause through different symptoms, or different root causes through similar symptoms. Whether bug-from-A and bug-from-B are "the same bug" requires human analysis. Flag overlaps for human review rather than auto-computing.

4. **The estimator cannot see bug classes outside all methods' reach.** If none of your methods can find a category of bug, capture-recapture reports zero remaining while that class is entirely undiscovered. The estimate is bounded by collective method capability, not actual bug population.

**What this means in practice:** Use capture-recapture as one signal among several, not as proof. Real exit confidence comes from convergence of ALL metrics together. No single metric is sufficient. Even all together they provide high confidence, not certainty.

---

## 1. Reconnaissance (Do This First)

Before writing a single test, understand what you're testing.

### Actions
1. Read every specification document in the project.
2. Read every source file. Build a model of: all entry points (API routes, CLI commands, event handlers, cron jobs); all persistent state (databases, files, caches, config); all external dependencies (APIs, LLMs, network services); all state machines (entity lifecycles, workflow phases); all invariants the code assumes but doesn't enforce.
3. Map the dependency graph — what calls what, what reads/writes where.
4. Identify the system's "state surface" — the Cartesian product of all state variables.
5. List every error handling path — especially the ones that swallow errors or log-and-continue.
6. Document all findings in `testing/RECONNAISSANCE.md`.

### Output
`testing/RECONNAISSANCE.md` containing: entry point catalog, state variable inventory, state machine diagrams (mermaid or text), dependency graph, identified risk areas, and candidate invariants.

---

## 2. Static Analysis Sweep

Zero-execution bug finding. Run every applicable linter and analyzer.

### Actions — adapt to the project's language(s)

**Go:** `go vet`, `staticcheck`, `golangci-lint` (enable all linters), `govulncheck`. **Python:** `ruff`, `mypy --strict`, `bandit` (security), `pylint`. **JS/TS:** `eslint` (strict config), `tsc --noEmit --strict`. **Rust:** `cargo clippy -- -W clippy::all -W clippy::pedantic`. **General:** `semgrep` (with auto rulesets), `trivy` (dependency vulnerabilities).

For each finding: if it's a real bug, fix it and log in `testing/FINDINGS.md`. If false positive, suppress with a comment explaining why. If style issue, ignore unless it masks a real problem.

**Tool availability:** Before running any tool, verify it's installed or installable. If unavailable for this ecosystem, skip and note the gap. Don't waste time fighting package installation.

### Output
All static analysis findings resolved or documented. Zero warnings in CI-mode runs.

---

## 3. Test Suite Hardening

Before generating new tests, make existing tests rigorous.

### 3a. Coverage Analysis

1. Run existing tests with coverage instrumentation.
2. Identify uncovered code paths — especially error handlers, edge case branches, cleanup/rollback logic, timeout paths, concurrent access paths.
3. Write tests for every uncovered path that handles real logic (skip trivial getters).
4. Target: line coverage > 90%, branch coverage > 85%. These are a floor, not a goal.

### 3b. Negative Testing

For every API endpoint / public function: test with null/nil/undefined, empty inputs, boundary values, type mismatches, and malformed inputs. For every state transition: attempt every INVALID transition and verify rejection; attempt transitions with stale/expired state. For every external dependency: test with error responses, garbage responses, timeouts, and valid-but-unexpected data.

### 3c. Concurrency Testing

1. Identify all shared mutable state.
2. For every shared resource, write tests that hammer it with parallel reads, parallel writes, mixed read-write contention, and read-modify-write race conditions.
3. Use race detectors (Go: `-race` flag, Python: ThreadSanitizer, etc.).
4. Run the full test suite 10x looking for flaky tests — every flake is a potential race condition.

---

## 4. Property-Based Testing

Generate random inputs, check invariants. **First independent discovery method.**

### Actions
1. For every function/endpoint with ≥2 parameters, define the input domain and properties that must ALWAYS hold: return type correctness, no panic/crash on valid input, idempotency where expected, reversibility of encode/decode and insert/delete pairs, monotonic ordering preservation, size relationships.
2. Use the appropriate tool: Go (`rapid` or `gopter`), Python (`hypothesis`), JS/TS (`fast-check`). **Verify the tool exists and is maintained before using it.** If nothing works, fall back to hand-rolled random generation — the technique matters more than the library.
3. Run with at least 10,000 examples per property.
4. For every failure: shrink to minimal case, log in `testing/FINDINGS.md` with `source: property-based`, fix the bug, keep shrunk example as regression test.

---

## 5. Fuzzing

Grammar-aware and coverage-guided fuzzing. **Second independent discovery method.**

### Actions
1. For every input-parsing function: write a fuzz target. Go: native fuzzing (`go test -fuzz`). Python: `atheris`. For API endpoints: use `Schemathesis` for OpenAPI-guided fuzzing if a schema exists.
2. Run each fuzz target for at least 10 minutes or until coverage plateaus.
3. For every crash/hang/unexpected error: minimize the input, log with `source: fuzzing`, fix, add as regression test.

### 5a. LLM-Augmented Fuzzing (use Ollama)

For every endpoint accepting natural language or semi-structured text:
1. Verify Ollama availability: `curl http://10.70.70.14:11434/api/tags` — confirm model name.
2. **If Ollama is unavailable:** Skip this sub-phase. Note the gap. Every phase using Ollama has a non-LLM fallback.
3. If available, generate adversarial inputs. Prompt patterns: "Generate 50 inputs a confused user might submit to [describe endpoint]"; "Generate 50 inputs designed to break [describe parser]"; "Generate 50 technically valid but semantically weird inputs for [context]."
4. Rate limit: max 10 concurrent requests, backoff on 429/503.
5. Feed each generated input through the system, check all invariants.

---

## 6. Metamorphic Testing

**Third independent discovery method.** Test relationships between outputs.

### Actions
1. Identify metamorphic relations: reformulation (same semantics, different wording → same result), permutation (reorder independent inputs → same final state), scaling (double input → proportional resource usage), identity (no-op → state unchanged), composition (A then B ≡ AB where applicable).
2. For each relation: generate input pairs. Use Ollama for natural language reformulations if available.
3. For every violation: log with `source: metamorphic`, determine if real bug or mis-specified relation.

---

## 7. Contract and Integration Testing

**Fourth independent discovery method** (if the system has external interfaces).

### Actions
1. For every consumed external API: test with valid, empty, malformed, slow, changed-schema, and unexpected-status-code responses.
2. For every exposed API: verify schema compliance, test all documented error codes, verify undocumented codes don't leak.
3. For every inter-component boundary: verify serialization round-trips, error propagation preserves context, timeout handling works.

---

## 8. Mutation Testing

Answers: "If there WERE a bug here, would any test catch it?" Run AFTER all test-writing phases, against the complete suite.

### Actions
1. Identify an available mutation testing tool. **Go:** check if `gremlins`, `ooze`, or `go-mutesting` are maintained — Go mutation tools have a history of abandonment. If nothing works, do manual mutation analysis of critical paths. **Python:** `mutmut`. **JS/TS:** `stryker`.
2. For every surviving mutant: analyze real gap vs. semantically equivalent. If real: write a killing test. If equivalent: document and exclude.
3. Target: mutation score > 95%. Equivalent mutant rates of 5-15% are typical; fighting the last few percent is waste.
4. Log mutation score as a convergence metric.

---

## 9. EDD — Evolutionary Session Testing

**Fifth independent discovery method.** The heavy hitter for stateful systems. See `EDD_METHODOLOGY.md` for full theory.

### Decision Gate: Is EDD Justified?

EDD requires building a custom evolutionary harness. Evaluate before committing:
- **Build EDD if:** system has ≥ 5 stateful endpoints, entity lifecycles with ≥ 3 phases, accumulated state across sessions, or non-deterministic components (LLMs).
- **Skip EDD if:** system is essentially stateless, CRUD with trivial state, or < 5 endpoints. Rely on the other four methods and note the gap.

**STOP: Present this decision to the human before proceeding.**

### Actions (if proceeding)

**9a. MODEL — Define the genome:**
- Catalog all user actions as gene types with payload generators and timing distributions.
- Define chaos events: dependency failures, resource exhaustion, process disruption, clock manipulation.
- Define ALL system invariants: data (structural correctness), behavioral (system does what it claims), performance (response bounds), cross-entity (relationships, counts, consistency).

**9b. SEED:** 30% random, 30% happy-path with mutation, 20% adversarial, 20% chaos-heavy.

**9c. EVOLVE:** Population 50, session length 20-100 genes, tournament selection k=5, crossover 0.7, mutation 0.3 (increase if diversity < 0.3), elitism top 2, 10% fresh randoms per generation. Fitness weights: invariant violations 5.0, state coverage 3.0, error diversity 2.0, performance anomalies 2.0, recovery failures 4.0, temporal degradation 1.5. **Full environment reset between every generation — abort if reset fails.**

**9d. DISCOVER (every 10 generations):** Cluster findings, replay 5x for reproduction confidence, classify severity.

**9e. HARDEN:** Fix critical/major findings, write regression tests, add invariant (ratchet), preserve discovering chromosomes.

**9f. CONVERGENCE (every 50 generations):** Zero new findings for 100 generations AND state coverage > 90% = converged. Stagnated with coverage < 90% = expand actions, mutation rate, fresh randoms.

**9g. Timeline Testing (after standard convergence):** Chain 5-10 sessions with time gaps. Evolve timeline structure. Check accumulation bugs, degradation, stale state. 50 timeline-generations minimum.

### The Fix-During-Test Problem

**CRITICAL:** When this template says "fix the bug," it means fix and re-run the phase that found it. Do NOT continue testing a system you're actively modifying — you're testing a moving target and the capture-recapture math assumes a fixed system.

Protocol: run all discovery phases first (recording but not fixing). Then batch-fix. Then re-run all phases on the fixed system. The convergence analysis uses only the final-run data.

**Exception:** If a bug blocks further testing (crash on startup, corrupted state preventing any session), fix it immediately, reset, and restart the current phase.

---

## 10. Performance and Stress Testing

1. **Baseline:** response time per endpoint with empty state.
2. **Loaded:** 100x and 1000x expected data volume.
3. **Stress:** find the breaking point.
4. **Soak:** 1 hour steady load — watch for memory leaks, connection leaks, file handle leaks, degradation.
5. For every cliff: log with `source: performance`, root cause, fix or document.

---

## 11. Security Sweep

1. **Input injection** on all text inputs: SQL injection, command injection, path traversal, XSS, template injection — as applicable.
2. **Auth/authz** (if applicable): every endpoint without auth, expired tokens, privilege escalation.
3. **Dependency audit:** `govulncheck` / `npm audit` / `pip-audit` / `cargo audit`.
4. **LLM-augmented payloads** (if Ollama available): creative injection attempts.

---

## 12. Convergence Analysis and Exit

### Actions

1. Collect all findings tagged by source method.
2. **Do NOT auto-compute capture-recapture if total bugs < 10 or pairwise overlap < 2.** The estimator is meaningless at small samples. Report raw discovery data and defer to human judgment.
3. If sample sizes are sufficient: compute Chapman estimator N̂ = ((|A|+1)(|B|+1) / (|A∩B|+1)) - 1. Report estimate AND confidence interval. Flag that independence is violated and estimate is a lower bound.
4. Fit defect discovery curve (Musa logarithmic model) to finding-rate-over-time. Extrapolate.

### Exit Criteria — ALL must be satisfied

- Mutation score > 95% (equivalent mutants documented)
- State coverage > 90% of defined state model
- EDD: zero new findings for 100 generations (or skipped with justification)
- All independent methods report zero new findings in their most recent full run
- All critical and major findings fixed and regression-tested
- Soak test: 1 hour at load with zero errors
- If capture-recapture computable: estimated remaining < 2 at 95% CI lower bound
- If NOT computable: finding rate has been zero across all methods for the final 20% of total testing effort

### If not met
Identify failing criterion. Determine real gap vs. measurement issue. Continue or adjust.

### Final Output
`testing/CONVERGENCE_REPORT.md`: methods used, coverage by method, bugs by source and severity, capture-recapture estimates with stated limitations (or explanation of inapplicability), defect discovery curve, residual risk assessment, items requiring human judgment.

---

## 13. Output Structure

```
testing/
├── RECONNAISSANCE.md          — System understanding and risk areas
├── FINDINGS.md                — All bugs, tagged by source/severity/status
├── CONVERGENCE_REPORT.md      — Final statistical analysis
├── CHECKPOINT.md              — Session resumption state
├── invariants/                — System invariants (grows via ratchet)
├── seeds/                     — EDD seed chromosomes
├── edd/                       — EDD harness code (if built)
├── fuzz/                      — Fuzz targets and corpora
├── property/                  — Property-based test files
├── mutation/                  — Mutation testing results
└── performance/               — Baselines and load test results
```

---

## 14. Ollama Integration

Ollama at `http://10.70.70.14` — use for adversarial input generation, metamorphic reformulation, invariant brainstorming, finding analysis, and test oracle augmentation (probabilistic only — flag for human review, never auto-close).

**First action:** `curl http://10.70.70.14:11434/api/tags` to verify availability and model name. If unavailable, proceed without — every phase has a non-LLM fallback. Rate limit: max 10 concurrent, backoff on errors.

---

## 15. Execution Phases and Checkpointing

### Phase Dependencies

**Sequential:** Reconnaissance → Static Analysis → Test Suite Hardening.

**Parallel** (for independence): Property-Based, Fuzzing, Metamorphic, Contract Testing.

**Sequential:** Mutation Testing → EDD → Performance → Security → Convergence.

### Checkpointing

After each phase, append to `testing/CHECKPOINT.md`: phase completed, bugs found (count/severity), time spent, key decisions, next phase. **On restart:** read checkpoint, resume from last completed phase. Do NOT re-run unless codebase changed.

### Time Estimates (Honest)

For a mid-sized system (~20 endpoints, moderate state):
- Phases 1-3: 4-8 hours (reconnaissance must be thorough)
- Phases 4-7 (parallel + mutation): 4-12 hours
- Phase 9 (EDD, if justified): 8-48 hours (harness build + evolution)
- Phases 10-12: 2-4 hours

---

## 16. Human Checkpoints

**STOP and report at these points:**

1. **After Reconnaissance:** "Here's my understanding and risk areas. Correct me."
2. **After first critical findings:** "Found [N] critical issues. Review before I continue?"
3. **Before EDD harness build:** "System state complexity is [X]. EDD is [justified/overkill]. Your call."
4. **If capture-recapture stays high:** "Estimate [N] remaining. Recommend architectural review of [areas]."
5. **Before final report:** "Results ready. [These items] need your judgment."

---

## 17. Adaptation Notes

When applying to a specific project: replace generic action types with actual entry points, replace generic invariants with actual correctness properties, adjust fitness weights, adjust chaos events, verify all tool names are current and installable, calibrate exit thresholds after first run, scope EDD to state complexity.

---

## 18. What This Template Does NOT Cover

1. **Subjective UX quality.** No automated method judges if an error message is confusing or a workflow is intuitive.
2. **Distributed systems consistency.** EDD's chaos engineering scratches the surface. Jepsen-style testing requires specialized tooling.
3. **Formal verification.** For safety-critical properties, model checking provides guarantees this cannot.
4. **Supply chain attacks.** Dependency auditing catches known CVEs, not sophisticated compromises.
5. **Bug classes outside all methods' reach.** Epistemic humility required in the final report.
