# CereBRO — Three Timescales of Adaptation

> How CereBRO adapts at three timescales: phylogenetic (architecture), ontogenetic
> (parameters), and Lamarckian (acquired inheritance). Maps biological evolution
> to AIP/Forge mechanisms.

**Cross-references:** [ARCHITECTURE.md](ARCHITECTURE.md) (what gets evolved),
[BUILD_ORDER.md](BUILD_ORDER.md) (Phase 6: architecture competition),
[PRINCIPLES.md](PRINCIPLES.md) (§6 memory consolidation, §7 neurogenesis)

---

## Overview

| Timescale | Biological Analogue | CereBRO Mechanism | Changes What | Frequency |
|-----------|-------------------|-----------------|-------------|-----------|
| Phylogenetic | Species evolution | AIP composition competition | Architecture (which COGs, how wired) | Rare (quarterly) |
| Ontogenetic | Individual development | Forge PARAMETER/RULE evolution | Parameters within fixed architecture | Regular (weekly) |
| Lamarckian | (Impossible in biology) | Memory Consolidator → Forge loop | Parameters + corpus, inherited | Continuous |

---

## Timescale 1: Phylogenetic — Architecture Evolution

### Biological Basis

In biology, phylogenetic evolution operates across generations through natural
selection on heritable variation. Species-level architecture changes — new organs,
modified body plans, novel neural circuits — emerge over thousands to millions of
generations (Darwin, 1859, *On the Origin of Species*; Mayr, 1963, *Animal Species
and Evolution*, Harvard University Press).

The key properties:
- **Slow:** Architectural changes are expensive and risky
- **Population-based:** Multiple architectures compete simultaneously
- **Selection on fitness:** Only architectures that outperform alternatives survive
- **Variation + selection:** Random variation generates candidates; environment selects

### CereBRO Mechanism: AIP Composition Competition

AIP already supports competition between COG variants. Architecture evolution
extends this to compete **entire CereBRO compositions** — different layer
configurations, wiring patterns, and COG selections.

**What varies between compositions:**
- Which Layer 0 COGs are included (all three? just FormatValidator?)
- Which Layer 2 detectors are activated (full set? minimal set?)
- Layer 3 configuration (inhibitor thresholds, salience parameters)
- Feedback enabled or disabled (Layer 4 re-evaluation on or off)
- Wiring order within layers

**What is fixed across compositions:**
- The 5-layer structure itself (this is the "body plan")
- Port contracts (all COGs must use the same proto messages)
- Layer latency budgets

**AIP Competition Specification:**

```
Arena:
  in_type: "cog.reasoning.v1.ConversationSnapshot"
  out_type: "cerebro.v1.CerebroReport"

Traits:
  - name: "f1_score"           direction: MAXIMIZE  builtin: false
  - name: "precision"          direction: MAXIMIZE  builtin: false
  - name: "recall"             direction: MAXIMIZE  builtin: false
  - name: "latency_p95_ms"    direction: MINIMIZE  builtin: true
  - name: "memory_peak_mb"    direction: MINIMIZE  builtin: true
  - name: "cog_count"         direction: MINIMIZE  builtin: false

Profiles:
  - name: "balanced"
    weights: {f1: 0.4, precision: 0.2, recall: 0.2, latency: 0.1, memory: 0.05, cogs: 0.05}
  - name: "precision-first"
    weights: {f1: 0.2, precision: 0.5, recall: 0.1, latency: 0.1, memory: 0.05, cogs: 0.05}
  - name: "recall-first"
    weights: {f1: 0.2, precision: 0.1, recall: 0.5, latency: 0.1, memory: 0.05, cogs: 0.05}
  - name: "minimal"
    weights: {f1: 0.3, precision: 0.2, recall: 0.2, latency: 0.1, memory: 0.05, cogs: 0.15}
```

**Contestants** are composition manifests (textproto). Each specifies a complete
CereBRO wiring. AIP runs the corpus through each composition, measures traits,
scores via Pareto frontier, identifies per-profile winners.

**Expected frequency:** Quarterly or when new COGs are added. Architecture
competition is expensive (runs the full corpus through multiple complete pipelines)
and is only valuable when the available COG set changes.

---

## Timescale 2: Ontogenetic — Parameter Optimization

### Biological Basis

Ontogenetic development occurs within an individual's lifetime. The architecture
(body plan, neural circuit topology) is fixed at birth; parameters are tuned by
experience. Examples:

- **Synaptic plasticity:** Long-term potentiation (LTP) and depression (LTD)
  adjust connection strengths without changing which neurons are connected (Bliss
  & Collingridge, 1993, "A synaptic model of memory: long-term potentiation in
  the hippocampus," *Nature*, 361:31-39)
- **Perceptual learning:** Visual acuity improves with practice, not by growing
  new neurons but by tuning existing ones (Fahle, 2005, "Perceptual learning:
  specificity versus generalization," *Current Opinion in Neurobiology*, 15:154-160)
- **Critical periods:** Windows of heightened plasticity (Hensch, 2005, "Critical
  period plasticity in local cortical circuits," *Nature Reviews Neuroscience*,
  6:877-888)

### CereBRO Mechanism: Forge PARAMETER and RULE Evolution

This is what the Forge already does. Architecture is fixed; parameters change.

**PARAMETER level** (synaptic weight adjustment):
- Config field values: thresholds, weights, window sizes
- Bounded by manifest-declared min/max ranges
- Mutation: Gaussian perturbation within bounds
- Crossover: arithmetic blend of parent parameters
- Already proven: Scope Guard optimization found t=0.80, s=8, r=4

**RULE level** (dendritic remodeling):
- Pinions rule sets: which patterns match, what emissions fire
- Port contracts are fixed; internal logic changes
- Mutation: threshold perturb, comparison flip, statement delete/duplicate
- Crossover: on_block_swap, stmt_swap, condition_swap
- AST-aware operators validate via `pinions validate`

**What gets optimized per COG (ontogenetic):**

| COG | Evolvable Parameters |
|-----|---------------------|
| scope-guard | drift_threshold, sustained_turns, reference_window_size |
| anchoring-detector | shift_threshold, proximity_window, context_sensitivity |
| confidence-calibrator | ece_threshold, formality_weight |
| context-inhibitor | corroboration_threshold, formality_threshold, stakes_threshold |
| urgency-assessor | urgency_keywords weights, complexity_formula weights |
| threshold-modulator | gain_multiplier, mode_threshold |
| salience-filter | novelty_weight, actionability_weight, min_salience |
| self-confidence-assessor | agreement_weight, margin_weight, historical_weight |

**Expected frequency:** Weekly, or when the corpus changes significantly. Parameter
optimization is cheap (no code changes, just config values) and converges quickly
(the Forge reached convergence in 6 generations for Scope Guard).

---

## Timescale 3: Lamarckian — Acquired Inheritance

### Biological Context

In biology, the Lamarckian hypothesis (inheritance of acquired characteristics)
is false. Weismann's barrier (Weismann, 1893, *The Germ-Plasm: A Theory of
Heredity*) established that changes to somatic cells are not transmitted to germ
cells. A blacksmith's strong arms don't produce strong-armed offspring.

The **Baldwin Effect** (Baldwin, 1896, "A new factor in evolution," *The American
Naturalist*, 30:441-451) is the closest biology gets: organisms that can learn
within their lifetimes are more likely to survive, which creates selection pressure
favoring the capacity to learn. But the learning itself is not inherited — only
the capacity for it.

In biology, the information flow is strictly one-directional:
```
genome → development → phenotype → (selection) → next genome
                                    ↑
                     experience affects selection
                     but NOT the genome directly
```

### Why Software Is Different

In CereBRO, the Weismann barrier does not exist:

```
COG parameters → runtime → experience → corpus → Forge → improved parameters
      ↑                                                         │
      └─────────────── directly inherited ──────────────────────┘
```

Runtime experience **directly modifies** the "genome" (COG parameters) through the
Forge. Acquired traits are inherited. This is impossible in biology and gives
CereBRO a fundamental advantage.

### The Lamarckian Loop

```
┌─────────────────────────────────────────────────────────────────┐
│                     THE LAMARCKIAN LOOP                          │
│                                                                  │
│  1. RUNTIME                                                      │
│     Pipeline processes conversations                             │
│     Detectors produce findings                                   │
│     Context Inhibitor gates findings                             │
│     Users confirm or reject findings                             │
│                                                                  │
│  2. CONSOLIDATION (Memory Consolidator)                          │
│     Confirmed findings → positive corpus entries                 │
│     Rejected findings → negative corpus entries                  │
│     Inhibited findings → context annotations                     │
│     Sparse index: finding types, confidences, outcomes           │
│                                                                  │
│  3. CORPUS GROWTH                                                │
│     New entries appended to data/corpus/*.ndjson                 │
│     Corpus diversity tracked (coverage by finding type, domain)  │
│     Stale entries aged out (findings from outdated COG versions) │
│                                                                  │
│  4. FORGE EVOLUTION                                              │
│     Forge evaluates population against expanded corpus           │
│     Fitness = precision * recall * (1 - FP_rate)                 │
│     Tournament selection, crossover, mutation                    │
│     Convergence in ~5-10 generations (proven)                    │
│                                                                  │
│  5. DEPLOYMENT                                                   │
│     Winner parameters exported to COG manifest                   │
│     COG re-registered in GEARS with new version                  │
│     Pipeline picks up new parameters on next invocation          │
│                                                                  │
│  6. IMPROVED RUNTIME → back to step 1                            │
│     The improved COG processes new conversations                 │
│     Better findings → better corpus → better evolution           │
│     Virtuous cycle with diminishing returns (convergence)        │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### What Makes This Work

1. **The corpus is the germ line.** In biology, germ cells carry the inherited
   information. In CereBRO, the corpus carries it. The Memory Consolidator is
   the mechanism that writes to the germ line.

2. **The Forge is natural selection, compressed.** Instead of waiting for
   environmental pressure over generations, the Forge simulates hundreds of
   generations against the corpus in minutes.

3. **Deployment is reproduction.** When improved parameters are deployed, the
   new COG "inherits" the acquired improvements. There is no Weismann barrier
   preventing this.

4. **Convergence prevents runaway.** The Forge's convergence detection stops
   evolution when fitness plateaus. Combined with bounded parameter ranges
   (from manifests), this prevents parameter drift.

### Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Corpus poisoning (bad feedback → bad entries) | Consolidation requires minimum confidence threshold + user confirmation |
| Overfitting to corpus | Forge uses population-based evolution (diversity) + convergence detection |
| Catastrophic forgetting | Old corpus entries are retained; Forge evaluates against full corpus |
| Feedback loops (COG improves → changes findings → changes corpus → COG changes) | Forge evolves against the full historical corpus, not just recent entries. Bounded parameter ranges prevent runaway. |
| Distribution shift (production conversations differ from corpus) | Corpus diversity tracking; alerts when coverage gaps detected |
| Concept drift (what constitutes "good reasoning" changes — e.g., new LLM models behave differently, making old corpus entries encode stale expectations) | Corpus entries carry a `timestamp` and `model_version` tag. Forge can weight recent entries higher via age-decay. Periodic corpus review flags entries from deprecated model versions for re-evaluation or retirement. |

### Expected Frequency

Continuous. The Memory Consolidator writes entries after each pipeline run (when
triggered). The Forge can be scheduled to run nightly, weekly, or on-demand when
corpus growth exceeds a threshold.

---

## Timescale Interactions

The three timescales interact but don't interfere:

```
Phylogenetic (architecture)     ← changes COG selection + wiring
     │                             runs quarterly
     │
     ├── Ontogenetic (parameters)  ← changes config values within fixed architecture
     │        │                       runs weekly
     │        │
     │        └── Lamarckian (loop)  ← runtime experience drives parameter evolution
     │                                  runs continuously
     │
     └── Architecture competition uses the best parameters from ontogenetic +
         Lamarckian as its starting point
```

**Bottom-up influence:** Lamarckian learning improves parameters. Better parameters
mean better fitness scores in ontogenetic evolution. Better per-COG fitness means
more accurate architecture comparisons in phylogenetic competition.

**Top-down constraint:** Phylogenetic competition selects the architecture.
Ontogenetic evolution is scoped to that architecture. Lamarckian learning is
scoped to the parameters of that architecture's COGs.

This mirrors biology: evolution sets the body plan → development tunes the
organism → experience refines behavior within developmental constraints.
The difference: in CereBRO, experience feeds back into future development.

---

## Comparison to Biological Timescales

| Property | Biology | CereBRO |
|----------|---------|--------|
| Architecture change | Millions of years | Quarterly competition |
| Parameter tuning | Months to years (learning) | Hours (Forge) |
| Acquired inheritance | Impossible | Continuous (Lamarckian loop) |
| Selection mechanism | Environmental pressure | Corpus-based fitness |
| Variation mechanism | Random mutation | Guided operators (AST-aware) |
| Population size | Millions of individuals | 30-100 (Forge config) |
| Generations to converge | Thousands+ | 5-10 (proven) |

The compression ratio is extraordinary. What takes biology millions of years
takes CereBRO hours. This is the engineering advantage of digital systems:
perfect reproduction, instant evaluation, directed variation.
