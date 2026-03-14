# Experience Driven Development (EDD)

> **Type:** Software Development Methodology  
> **Author:** John + Claude  
> **Date:** 2026-03-14  
> **Status:** v0.2 — Structurally-Guided Evolution  

---

## The Problem With How We Test

Testing methodologies answer progressively better questions:

| Methodology | Question | Discovers |
|-------------|----------|-----------|
| Unit testing | Does this function work? | Logic errors |
| Integration testing | Do these parts work together? | Interface errors |
| TDD | Does the code meet the spec? | Spec violations |
| BDD | Does the code behave as expected? | Behavior mismatches |
| Manual QA | Does a human notice anything wrong? | Obvious UX issues |
| Exploratory testing | What breaks when a human tries weird things? | Edge cases a script wouldn't try |

None of them answer: **What happens when a real person uses this system over time, under real conditions, with real psychology?**

A real user doesn't execute one action in isolation. They build up state over sessions. They develop habits. They get interrupted. They come back after a week and can't remember what they were doing. They try something that worked yesterday and it fails today because the system state changed. They get frustrated and start doing things fast and sloppy. They find workarounds that technically work but corrupt the data model. They use the system in ways the developer never imagined — not because they're creative, but because they misunderstood something on day one and built all their habits on that misunderstanding.

**The gap:** Testing validates correctness. But correctness is necessary and not sufficient. The thing that kills products is the accumulated experience of using them — the friction that builds up, the edge cases that only appear after 50 interactions, the state explosions that only happen when real usage patterns intersect with real failure conditions.

Manual exploratory testing tries to fill this gap but doesn't scale. A human can simulate maybe 30 minutes of use per test session. Real usage patterns emerge over weeks. The math doesn't work.

---

## The EDD Thesis

**Test durability, not just correctness.**

EDD tests whether a system remains correct, responsive, and internally consistent across the full space of possible user trajectories — sequences of actions, over time, under varying conditions, including failure. It does this by evolving simulated user sessions through natural selection, rewarding sessions that discover interesting system states.

**What EDD actually tests:** state-space robustness under evolved action sequences. Data integrity. Performance stability. Recovery after failure. Invariant preservation across thousands of usage paths.

**What EDD does NOT test:** subjective experience. Whether an error message is confusing. Whether the UI is intuitive. Whether the learning curve is acceptable. These require human judgment. EDD replaces the *mechanical* part of exploratory testing (trying lots of action sequences), not the *perceptual* part (evaluating quality of the result). Reduced manual testing, not zero.

The core mechanism: **evolutionary user simulation**. Instead of scripting test cases, you evolve them. Start with random user behavior, define fitness functions that reward "interesting" discoveries (crashes, data inconsistencies, performance cliffs, impossible states), and let natural selection find the edge cases that no human would think to test.

**Time dilation is the key advantage.** One hour of simulation can cover what would take weeks of real usage. Not because the simulation is faster than real-time (it can be), but because it runs thousands of sessions in parallel, each exploring a different trajectory through the system's state space.

### Prior Art and Lineage

| Technique | Origin | What EDD Takes From It |
|-----------|--------|----------------------|
| **Search-based software testing (SBST)** | McMinn 2004, EvoSuite | Evolutionary optimization of test inputs; fitness-guided exploration |
| **Model-based testing** | Erlang QuickCheck, state machine testing | State models, valid/invalid transition coverage, property checking |
| **Grammar-based fuzzing** | AFL, Peach Fuzzer, libFuzzer | Guided input mutation, coverage-tracked evolution |
| **Chaos engineering** | Netflix Simian Army, Chaos Monkey | Fault injection as a first-class testing activity |
| **Session-based test management** | Bach, SBTM | Structured exploratory testing with charter, time-box, and debrief |
| **Property-based testing** | QuickCheck, Hypothesis | Invariant-driven correctness checking, shrinking/minimization |
| **Static call graph analysis** | golang.org/x/tools, Soot, WALA | Structural map of reachable code paths *(v0.2)* |
| **Coverage-guided testing** | AFL, libFuzzer, go-fuzz | Runtime feedback steering exploration toward new paths *(v0.2)* |

**EDD's specific contribution** is the combination: evolving *sessions* (not individual inputs), with *temporal structure* (not single-shot), including *chaos events as genes* (not a separate layer), against *system invariants* (not expected outputs), with *the ratchet* (invariants only grow), guided by *structural intelligence* (not blind — v0.2).

### Non-Deterministic Systems

**Fitness noise:** The same chromosome produces different fitness scores. Mitigation: for critical evaluations, run 2-3 times, use median. For routine evolution, accept the noise.

**Flaky findings:** Replay 5 times. Reproduces ≥2/5 = confirmed. 1/5 = quarantined. 0/5 = discarded.

**Reproduction confidence:** Every finding gets a reproduction rate in its metadata. 5/5 > 2/5 in severity.

---

## Core Concepts

### 1. The User Genome

A simulated user session is a **chromosome** — an ordered sequence of **genes**, where each gene is one user action.

```
Gene {
    action:    enum    — what the user does (type, click, navigate, wait, etc.)
    payload:   any     — the content of the action (text typed, button clicked, etc.)
    timing:    duration — how long before executing this action
    condition: predicate — optional: only execute if system state matches
}

Chromosome = Gene[]  — a complete user session
Population = Chromosome[]  — many sessions running in parallel
```

The chromosome doesn't encode a test case. It encodes a *usage pattern*. A test case verifies a specific expectation. A usage pattern explores a trajectory without predetermined expectations.

### 2. Fitness Functions (What Makes a Session "Interesting")

EDD tests for **interestingness** — conditions that reveal something about the system not known before.

**Multi-objective fitness function:**

| Objective | Weight | Measures |
|-----------|--------|----------|
| **Invariant violations** | 5.0 | Data consistency rules broken |
| **Recovery failures** | 4.0 | System fails to recover after chaos |
| **State coverage** | 3.0 | Unique system states reached |
| **Code path coverage** *(v0.2)* | 3.0 | New functions reached in the structural map |
| **Target hits** *(v0.2)* | 2.5 | P1/P2 target map functions triggered |
| **Error diversity** | 2.0 | Unique error types encountered |
| **Performance anomalies** | 2.0 | Response times exceeding baseline by >2σ |
| **Temporal degradation** | 1.5 | Decline over session length |

The fitness function is the most important design decision. A bad one evolves sessions that are "interesting" in ways that don't matter. A good one finds production incidents before they happen.

**Adaptive weighting** *(v0.2)*: Early generations weight broad exploration (state coverage, code paths) higher. As coverage plateaus, shift weight toward invariant violations and frontier targets. This is a manual knob adjusted at stagnation checkpoints — not a continuous gradient. Automatic reweighting adds complexity without proven benefit.

### 3. Evolutionary Operators

**Selection:** Tournament, k=N. Balances exploration with exploitation.

**Crossover:** Single-point. Offspring explores path A leading into path B. Accept incoherent offspring (let selection kill them) in v0, or implement state-aware crossover if premature convergence is a problem.

**Mutation operators:**

| Operator | What It Does | What It Finds |
|----------|-------------|---------------|
| **Action swap** | Change action type | Incorrect handling, missing validation |
| **Payload mutation** | Modify action data | Parsing bugs, encoding, injection |
| **Timing shift** | Change delays | Race conditions, timeouts, queue overflow |
| **Action insertion** | Add random action | Unexpected mid-flow actions |
| **Action deletion** | Remove action | Skipped steps |
| **Chaos injection** | Insert disruption | Resilience, recovery, data integrity |
| **Persona shift** | Change input "personality" | Input diversity |
| **Sequence reversal** | Reverse subsequence | Order-dependent bugs |
| **Duplication** | Repeat same action | Idempotency, duplicate handling |
| **Targeted insertion** *(v0.2)* | Insert sequence aimed at unreached path | Deep code paths missed by organic evolution |

**On "adaptive mutation rates":** v0.1 used a uniform mutation rate across all genes. An appealing v0.2 improvement is *selective* mutation — lower rates for genes on known paths to frontier nodes, higher for genes in saturated regions. This is sound in principle but requires per-gene coverage attribution (execute the chromosome with and without each gene, compare coverage), which costs O(genes) additional executions per chromosome evaluated. For a 50-gene chromosome that's 50x overhead. **Recommendation for v0.2:** Don't implement per-gene adaptive rates. Instead, apply a simpler heuristic: when a chromosome reaches a new frontier node, mark the chromosome as "productive" and apply lower mutation to the whole chromosome in the next generation. This costs nothing extra and preserves productive paths without the attribution overhead.

### 4. System Invariants (The Oracle Problem)

**Data invariants** — structural correctness (valid phases, monotonic transitions, no orphans, consistent counts).

**Behavioral invariants** — the system does what it claims (every action produces expected record, context stays within budget).

**Experience invariants** — interaction quality holds (response time bounded, no empty responses to valid input, no data loss across restarts, no dead ends).

When a session violates an invariant, the chromosome is preserved for reproduction — its genes found something interesting.

### 5. Chaos Engineering Integration

Environmental disruptions as first-class genes. LLM timeout, garbage, refusal. DB latency, full disk. Process kill/restart. Clock skew. Concurrent access. Chaos genes mutate and crossover like user actions. Evolution discovers which chaos + action combinations produce violations.

### 6. The Experience Timeline

Sessions as timelines — sequences with gaps simulating days/weeks.

```
Timeline {
    sessions: [
        { actions: [...], duration: 10min },
        // gap: 4 hours
        { actions: [...], duration: 5min },
        // gap: 2 days
        { actions: [...], duration: 20min },
    ]
}
```

Catches: accumulated state, context loss, data growth, stale state, degradation at session 100 vs session 1.

### 7. Chromosome Shrinking *(New in v0.2)*

When a chromosome violates an invariant, the raw chromosome may be 50-100 genes. A human cannot analyze 100 actions to find the root cause. **Shrinking reduces a violation-producing chromosome to the minimal subsequence that still triggers the violation.**

This is the same principle as QuickCheck/Hypothesis shrinking for property-based testing, adapted to sequential gene execution.

**Algorithm:**

```
Shrink(chromosome, invariant_violated):
    minimal = chromosome
    for i in range(len(minimal)):
        candidate = minimal.without(gene[i])
        reset_system()
        result = execute(candidate)
        if result.violates(invariant_violated):
            minimal = candidate  // gene[i] wasn't necessary
            // restart loop with shorter chromosome
    
    // Second pass: try removing contiguous blocks (2, 4, 8 genes)
    for block_size in [2, 4, 8]:
        for start in range(0, len(minimal) - block_size):
            candidate = minimal.without(genes[start:start+block_size])
            reset_system()
            result = execute(candidate)
            if result.violates(invariant_violated):
                minimal = candidate
                break  // restart with shorter
    
    return minimal
```

**Cost:** Shrinking one chromosome costs O(N²) executions in the worst case (N = chromosome length). For a 50-gene chromosome against a fast system (~100ms per gene), that's ~25 minutes. For a slow system, it's hours. **Shrink only confirmed findings (reproduction rate ≥ 2/5), not every raw violation.** And shrink *after* the evolutionary run completes, not inline — shrinking is for human analysis, not for evolution.

**Why this matters:** A 50-gene finding that shrinks to 4 genes goes from "undecipherable" to "obviously a race condition between actions 2 and 3 when preceded by action 1 under chaos condition 4." The HARDEN phase's "30-60 minutes per critical finding" budget depends on this.

---

## Structurally-Guided Evolution (v0.2)

### The Problem With Blind Evolution

EDD v0.1 discovers code paths through behavioral exploration — random mutations lead to new states, fitness rewards novelty, selection breeds the explorers. This works, but has a fundamental limitation: **the GA doesn't know what it doesn't know.**

If a code path requires a specific precondition sequence — inject 3 problems of different categories, triage all to "incubate," wait past a timeout, then trigger a batch review — the probability of random mutation discovering that exact sequence is vanishingly small. The GA will churn through millions of generations without reaching that code. A human reading the source for 5 minutes would say "oh, you need to set up the incubation batch first."

v0.1 explores a dark cave with only a fitness sensor: you know when you find something interesting but don't know what you're missing. v0.2 adds a map: you know what rooms exist, which you haven't entered, and roughly how to reach them.

### The Structural Map

A machine-readable representation of the codebase's function call graph, annotated with priority and coverage status.

```
StructuralMap {
    nodes: map[FuncID]Node
    edges: []Edge{caller, callee}
    
    Node {
        id:        string
        name:      string      — human-readable
        file:      string
        line:      int
        tag:       enum        — PUBLIC_API | CRITICAL_PATH | ERROR_HANDLER |
                                  BUSINESS_LOGIC | STATE_MUTATION | INTERNAL_HELPER |
                                  BOILERPLATE
        priority:  enum        — P1 | P2 | P3 | SKIP
        visited:   bool        — has ANY session triggered this?
        visit_gen: int         — generation when first visited (-1 if never)
    }
}
```

**Construction tools by language:**

| Language | Tool | Fidelity | Blind Spots |
|----------|------|----------|-------------|
| Go | `golang.org/x/tools/go/callgraph` (RTA) | High | Interface dispatch, reflection, `go generate` |
| Python | `pyan3`, `pyreverse` | Medium | Dynamic dispatch, decorators, metaclasses, `getattr` |
| JS/TS | `madge` | Low (module-level) | Dynamic imports, `eval`, prototype chains |
| Rust | `cargo-call-stack` | High | `dyn` dispatch, unsafe blocks |
| Java | Soot, WALA | High | Reflection, dynamic proxies |

**Critical honesty about fidelity:** For statically-typed languages, the call graph is reasonably complete. For dynamic languages, it's an approximation with significant gaps. **Document which regions the map misses. Those are the highest-risk unknowns — code that neither the map nor the GA can see.**

If tooling fails: skip to manual entry-point mapping. EDD degrades to v0.1. Still functional, just slower to converge.

### Node Classification

| Tag | Priority | Rationale |
|-----|----------|-----------|
| `PUBLIC_API` | P1 | The attack surface — externally callable |
| `CRITICAL_PATH` | P1 | Between public API and persistent state — where corruption happens |
| `ERROR_HANDLER` | P1 | Catch/recover/fallback — almost never tested |
| `BUSINESS_LOGIC` | P2 | Domain computation/validation |
| `STATE_MUTATION` | P2 | Writes to DB/file/cache — side effects = risk |
| `INTERNAL_HELPER` | P3 | Utility, formatting, logging |
| `BOILERPLATE` | SKIP | Generated, trivial getters/setters |

Classification can be automated heuristically and refined by LLM analysis or human review.

### The Target Map (Pre-Hunt Analysis)

Before evolution begins, analyze the codebase for where bugs are *likely* to live. A prioritized hit list with reasoning.

**Sources of targeting intelligence:**

1. **Structural analysis:** Unvisited P1/P2 nodes with no test coverage.
2. **Complexity metrics:** High cyclomatic complexity, deep nesting. Complexity correlates with defect density (imperfectly, but the signal is real).
3. **Change recency:** `git log` — recently modified functions harbour fresh bugs.
4. **LLM code review:** Feed source to LLM: "Identify functions with subtle edge cases, unenforced invariants, or error handling gaps." Produces weakness analysis with reasoning.
5. **Dependency fan-in/fan-out:** Functions touching multiple state sources. Coordination bugs live at boundaries.

```
Target {
    function:       string
    reason:         string
    estimated_path: []string    — call chain leading here
    priority:       P1 | P2
    preconditions:  []string    — state required before execution
    source:         string      — static | complexity | llm | git | boundary
}
```

**LLM target generation — and its limits:** Expect 30-50% genuinely valuable, 20-30% plausible but uninteresting, 20-40% hallucinated (wrong function names, non-existent paths). **Validation is mandatory.** Cross-reference every LLM target against the structural map. Function not in call graph = discard. The LLM is an unreliable cartographer — valuable for spotting things humans miss, but its output needs surveying.

### LLM-Synthesized Seeds

When an LLM has the structural map AND target map, it can generate *strategic* seed chromosomes — sequences designed to reach specific functions.

**Prompt pattern:**

```
System: Generate test sequences for a software system. Produce a JSON 
array of actions that causes execution of the target function.

Context:
- Target: validatePaymentGateway(ctx, amount)
- Path: POST /payments → processStripe() → validateGatewayConfig() → TARGET
- Preconditions: Payment method registered. Amount > 0.
- Available actions: [gene type list with schemas]
- Example valid chromosome: [one complete, valid example]

Generate 3 chromosomes (JSON arrays of genes) that should trigger the target.
```

**Prompt quality matters more than quantity.** The difference between 20% and 60% validation pass rate is usually: (a) including a complete valid example chromosome, (b) including the exact JSON schema with types, not a prose description, (c) including the actual function signature with parameter types. Vague prompts → hallucinated responses.

**Three-gate validation:**

| Gate | Checks | Fail Action |
|------|--------|-------------|
| **Schema** | Valid action types? Payload well-formed? Timing in bounds? | Discard |
| **Structural** | Referenced endpoints exist in call graph? Sequence logically coherent? | Discard |
| **Smoke** | Execute once. Infrastructure error? | Discard. App error = pass. Hang = discard. |

Application errors (400, validation rejection, business rule violation) are NOT rejection criteria. The GA *wants* error-provoking seeds. Only infrastructure failures indicate a bad seed.

**Expected success rate:** 30-60% pass all gates. Retry failed targets 3x. All fail = mark "LLM-unreachable" — the GA must discover it organically.

### Stagnation Escape via Re-Targeting

v0.1's stagnation response: increase mutation rate, inject fresh randoms. Undirected.

v0.2 adds *directed* escape: detect plateau → analyze frontier → re-target with LLM (include context of what's been tried) → validate → inject → resume.

This breaks plateaus that random mutation would take thousands of generations to cross. Not guaranteed — the LLM's reasoning about execution paths is approximate — but when it works, it's transformative.

### Harness Self-Testing *(New in v0.2)*

The EDD harness is software. It can have bugs. A buggy invariant checker produces false positives. A buggy fitness function wastes evolution on irrelevant objectives. **Before trusting harness output, validate the harness itself:**

1. **Invariant checker validation:** Construct a known-bad system state (manually corrupt the DB). Verify the invariant checker catches it. Construct a known-good state. Verify no false positives.
2. **Fitness sanity check:** Execute a known chromosome (e.g., a happy-path seed). Verify the fitness score is positive and in expected range. Execute an empty chromosome. Verify score is near zero.
3. **Reset verification:** Run a chaos gene that corrupts state. Run ResetSystem(). Verify the system is actually clean — query the DB, check process state, verify health endpoint.
4. **Coverage feedback verification:** Execute a chromosome that calls a known function. Verify the coverage delta includes that function.

These checks take minutes and prevent hours of wasted evolution against a broken harness.

---

## The EDD Cycle (v0.2)

```
┌─────────────────────────────────────────────────────────────┐
│                     EDD CYCLE v0.2                           │
│                                                              │
│  0. MAP                                                      │
│     Build structural map (call graph + classification)       │
│     Generate target map (static + complexity + LLM)          │
│     Instrument binary for runtime coverage                   │
│     ↓                                                        │
│  1. MODEL                                                    │
│     Define user actions, system invariants,                  │
│     fitness functions (state + structural objectives),       │
│     chaos events                                             │
│     ↓                                                        │
│  2. VALIDATE HARNESS                                         │
│     Test invariant checker, fitness function, reset,         │
│     coverage feedback against known states                   │
│     ↓                                                        │
│  3. SEED                                                     │
│     Random + happy-path + adversarial + chaos                │
│     + LLM-synthesized targeted seeds (validated)             │
│     ↓                                                        │
│  4. EVOLVE                                                   │
│     Select, crossover, mutate                                │
│     Execute against instrumented system                      │
│     Evaluate fitness (state + structural + invariant)        │
│     Coverage feedback → frontier → weight adjustment         │
│     ↓                                                        │
│  5. DISCOVER                                                 │
│     Cluster, replay, shrink (minimal reproducer),            │
│     classify                                                 │
│     ↓                                                        │
│  6. HARDEN                                                   │
│     Fix. New invariant (ratchet). Preserve chromosomes.      │
│     ↓                                                        │
│  7. EXPAND / RE-TARGET                                       │
│     New feature? → New actions, invariants, targets          │
│     Stagnated? → LLM re-targeting of frontier nodes          │
│     → Return to EVOLVE                                       │
└─────────────────────────────────────────────────────────────┘
```

### The Ratchet Effect

Every finding → new invariant. Invariant set only grows. System gets harder to break. Evolution forced toward increasingly subtle issues. The ratchet only tightens.

**The ratchet is not free.** Converting finding → good invariant requires human analysis. Budget 30-60 minutes per critical finding — but only because shrinking reduces the chromosome to a minimal reproducer first. Without shrinking, budget 2-4 hours.

---

## Cost Model

### Compute

| Configuration | Genes/generation | Approx time (local, no LLM) |
|---------------|-----------------|------------------------------|
| 30 × 20 | 600 | ~30 seconds |
| 50 × 50 | 2,500 | ~2 minutes |
| 100 × 100 | 10,000 | ~8 minutes |

Plus: structural map construction (one-time, minutes), coverage query per generation (milliseconds), shrinking per confirmed finding (minutes to hours depending on system speed).

### LLM Cost (v0.2)

**With local LLM (Ollama on LAN):** Zero token cost. Constraint is throughput (~30s per call).

**With API LLM:** ~$0.01-0.05 per synthesis call depending on model. Budget: N targets × 3 attempts × $0.03 = ~$1-5 for initial seeding. Re-targeting adds ~$0.50 per escape attempt. Total LLM synthesis cost for a full run: $5-20.

| Activity | LLM calls | Frequency |
|----------|-----------|-----------|
| Target map | 1 per source file (batched) | Once |
| Seed synthesis | 3 per P1/P2 target | Once per target |
| Re-targeting | 3-5 per escape | Rare |

### Human Analysis

~2 hours/week for active development. Shrinking reduces per-finding analysis time. The structural map provides immediate context for where in the codebase the violation originates.

---

## Environment Isolation

Chaos events have side effects that survive the session. If generation N corrupts the DB and the reset doesn't fully clean up, generation N+1's findings are tainted.

### What Resets vs. What Persists

**Reset between every generation:**
- System-under-test state (DB, files, caches, config)
- Active chaos effects (disk limits, network injection, killed processes)
- System processes (kill and restart fresh)

**Persists across resets (cumulative over the entire run):**
- The `visited` bitset (which functions have been reached)
- The structural map and target map
- The finding log
- The seed population (elite chromosomes survive across generations)
- Fitness history and convergence metrics

### Reset Protocol

Define explicitly for each system. Example for daemon + SQLite:

```
ResetSystem():
1. SIGKILL daemon (if running), wait for exit
2. Delete database file
3. Remove disk limits, network injection
4. Restart dependencies (LLM server, etc.)
5. Start daemon fresh
6. Wait for health check
7. Verify: DB empty, stats report zeros, daemon responds
```

Abort generation if any step fails. Never run against tainted state.

---

## Convergence and Stagnation (v0.2)

| Signal | Indicates | Response |
|--------|-----------|----------|
| Findings = 0 for 20+ gens | May be robust | Expand actions, chaos |
| State coverage plateau | Mutation can't reach new states | Increase mutation, fresh randoms |
| **Code path plateau** *(v0.2)* | GA can't reach new functions | **LLM re-targeting** |
| Mean fitness decreasing | Ratchet working | Continue |
| Diversity < 0.3 | Premature convergence | Mutation ↑, elitism ↓ |
| **Frontier stuck on P1** *(v0.2)* | Critical code unreached | **Re-targeting + human review** |

### When to Stop

1. **Budget exhaustion.**
2. **Structural convergence** *(v0.2)*: Frontier = only P3/SKIP. All P1/P2 exercised. Zero findings for 3 consecutive full runs. Strongest automated signal.
3. **Manual override** with documented justification.

---

## Metrics (v0.2)

| Metric | What It Measures | Health Signal |
|--------|-----------------|---------------|
| **State coverage %** | Behavioral states reached | Increasing = good |
| **Code path coverage %** *(v0.2)* | Structural map functions reached | Target: >90% P1/P2 |
| **Frontier size** *(v0.2)* | Unreached P1/P2 remaining | Decreasing → 0 |
| **LLM seed success rate** *(v0.2)* | Passing three-gate validation | <30% = improve prompts |
| **Invariant violation rate** | Violations per gen | Decreasing = ratchet |
| **Mean fitness** | Average | Increasing = harder bugs |
| **Fitness diversity** | Variance | Low = convergence problem |
| **Unique findings per gen** | Discoveries | Decreasing non-zero = healthy |
| **Severity distribution** | Critical / major / minor | Shifting minor = maturing |
| **Shrink ratio** | Raw genes / minimal genes per finding | Lower = more focused findings |

---

## Implementation Architecture (v0.2)

```
edd/
├── structural/
│   ├── callgraph.go       — call graph construction + parsing
│   ├── classifier.go      — node priority classification
│   ├── frontier.go        — frontier engine
│   └── coverage.go        — runtime coverage query + delta
├── targeting/
│   ├── targets.go         — target map construction
│   ├── synthesizer.go     — LLM seed synthesis
│   └── validator.go       — three-gate validation
├── genome/
│   ├── gene.go
│   ├── chromosome.go
│   ├── timeline.go
│   ├── shrink.go          — chromosome minimization (v0.2)
│   └── population.go
├── evolution/
│   ├── selection.go
│   ├── crossover.go
│   ├── mutation.go
│   └── engine.go
├── fitness/
│   ├── invariants.go
│   ├── coverage.go        — state coverage
│   ├── structural.go      — code path coverage (v0.2)
│   ├── performance.go
│   └── scorer.go
├── execution/
│   ├── runner.go
│   ├── client.go
│   ├── observer.go
│   └── chaos.go
├── analysis/
│   ├── findings.go
│   ├── clusters.go
│   ├── reports.go
│   └── regression.go
├── validation/
│   └── harness_test.go    — harness self-tests (v0.2)
├── seeds/
│   ├── random.go
│   ├── patterns.go
│   ├── adversarial.go
│   └── targeted.go        — LLM-synthesized (v0.2)
└── cmd/
    └── edd/
        └── main.go
```

---

## Philosophical Notes

**EDD's worldview:** Software fails not because the code is wrong, but because the code doesn't anticipate how people actually use it over time. The gap between "works correctly" and "works durably" is the survivability gap.

**"Experience" is aspirational, not literal.** The simulation tests *system durability*, not *subjective experience*.

**The evolutionary metaphor is literal, not decorative.** EDD evolves usage patterns that survive the system's defenses. The patterns that survive are the ones that break things — and breaking things is the test's job.

**EDD treats bugs as prey, not as failures.** A found bug is a success. An unfound bug is a failure.

**The human stays in the loop** — at the analysis stage, not the execution stage.

**"Structurally-guided" does not mean "structurally-complete."** *(v0.2)* The call graph is an approximation of reality. Dynamic dispatch, reflection, runtime code generation, and framework magic all create paths static analysis cannot see. The map reduces the dark territory; it does not eliminate it. There are always unknown unknowns. Epistemic humility remains non-negotiable.
