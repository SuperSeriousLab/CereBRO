# CereBRO — Biomimetic Design Principles

> Seven organizational principles drawn from neuroscience, mapped to COG architecture.
> Each principle cites primary research and specifies how the biological mechanism
> translates to an engineering constraint.

**Cross-references:** [ARCHITECTURE.md](ARCHITECTURE.md) (layer specifications),
[NEW_COGS.md](NEW_COGS.md) (COG implementations), [CONTRACTS.md](CONTRACTS.md) (proto messages)

---

## Principle 1: Layered Processing with Increasing Abstraction

### Brain Basis

Sensory information flows through a hierarchy of processing stages, each adding
abstraction. The pathway: peripheral receptors → brainstem nuclei → thalamus →
primary sensory cortex → association cortex → prefrontal cortex. Each stage
transforms raw signal into increasingly abstract representations.

Latency data demonstrates the hierarchy:
- **Brainstem reflexes:** 10-15ms (e.g., auditory startle — Davis et al., 1982,
  "The mammalian startle response," *Neural Mechanisms of Startle Behavior*)
- **Thalamic relay:** 20-40ms (Jones, 2007, *The Thalamus*, 2nd ed., Cambridge
  University Press — thalamocortical transmission latencies)
- **Primary cortex (V1):** 40-60ms (Schmolesky et al., 1998, "Signal timing
  across the macaque visual system," *Journal of Neurophysiology*, 79:3272-3278)
- **Association cortex:** 100-200ms (Thorpe et al., 1996, "Speed of processing
  in the human visual system," *Nature*, 381:520-522 — ultra-rapid categorization)
- **Prefrontal integration:** 200-500ms (Miller & Cohen, 2001, "An integrative
  theory of prefrontal cortex function," *Annual Review of Neuroscience*, 24:167-202)

The key insight: early stages are fast but dumb. Later stages are slow but smart.
No stage tries to do everything.

### COG Mapping

| Brain Stage | CereBRO Layer | Latency Budget | Function |
|-------------|-------------|----------------|----------|
| Brainstem | Layer 0: SENSORY | <10ms | Format validation, toxicity gate |
| Thalamus | Layer 1: THALAMIC RELAY | <50ms | Intake, routing, urgency |
| Cortical specialists | Layer 2: CORTICAL | <500ms | Bias/fallacy detection |
| Basal ganglia | Layer 3: INHIBITION | <100ms | Suppress false positives |
| Prefrontal | Layer 4: INTEGRATION | <200ms | Synthesis, metacognition |
| Hippocampus | Layer 5: MEMORY | async | Consolidation, learning |

### Design Implications

- **Each layer has a strict latency budget.** A Layer 0 COG that takes 200ms is
  misclassified — it belongs in a later layer.
- **Early rejection is cheap.** If Layer 0 rejects input, Layers 1-5 never execute.
  This mirrors how brainstem gating prevents cortical processing of irrelevant stimuli.
- **Later layers may override earlier ones**, but not vice versa. Information flows
  forward through the hierarchy. Feedback (Principle 5) is a controlled exception.

---

## Principle 2: Parallel Specialist Modules

### Brain Basis

The cortex is organized into functionally specialized areas that process in parallel:
- **Broca's area** (left inferior frontal gyrus): speech production and syntax
  (Broca, 1861; modern confirmation via fMRI: Fadiga et al., 2009, "Broca's area
  in language, action, and music," *Annals of the New York Academy of Sciences*)
- **Wernicke's area** (posterior superior temporal gyrus): speech comprehension
  (Wernicke, 1874; Binder, 2015, "The Wernicke area," *Neurology*, 85:2170-2175)
- **Fusiform face area:** face recognition (Kanwisher et al., 1997, "The fusiform
  face area: a module in human extrastriate cortex specialized for face perception,"
  *Journal of Neuroscience*, 17:4302-4311)
- **V1-V5 visual hierarchy:** progressively complex feature extraction (Felleman
  & Van Essen, 1991, "Distributed hierarchical processing in the primate cerebral
  cortex," *Cerebral Cortex*, 1:1-47)

Why specialization evolved:
1. **Metabolic efficiency** — specialized circuits use less energy than general-purpose
   ones for the same task (Laughlin & Sejnowski, 2003, "Communication in neuronal
   networks," *Science*, 301:1870-1874)
2. **Fault isolation** — damage to Broca's area impairs speech production but not
   comprehension (and vice versa for Wernicke's)
3. **Independent optimization** — each area can be tuned by experience without
   destabilizing others (Kaas, 1987, "The organization of neocortex in mammals,"
   *Annual Review of Neuroscience*, 10:107-140)

### COG Mapping

This maps directly to **COG Law 2: No Lateral Knowledge** — COGs never import or
reference each other. Each detector is a specialist:

| Brain Specialist | COG Specialist | Shared Trait |
|-----------------|---------------|-------------|
| Anterior cingulate (conflict monitoring) | contradiction-tracker | Detects conflicting signals |
| Orbitofrontal cortex (value estimation) | anchoring-detector | Evaluates numerical anchoring |
| Dorsolateral PFC (planning/monitoring) | scope-guard | Tracks goal adherence |
| Ventromedial PFC (risk/reward) | sunk-cost-detector | Evaluates investment biases |
| Parietal cortex (calibration) | confidence-calibrator | Assesses confidence accuracy |

### Design Implications

- **COGs run in parallel**, not sequentially. The Cognitive Router fans out to all
  relevant detectors simultaneously, just as sensory input activates multiple cortical
  areas in parallel.
- **No COG reads another COG's internal state.** Each specialist receives the same
  input (ConversationSnapshot) and produces its own assessment independently.
- **AIP can optimize each COG independently** — Forge parameter sweeps on one detector
  don't affect others, mirroring how cortical plasticity in one area doesn't
  destabilize its neighbors.

---

## Principle 3: Inhibitory Gating (Basal Ganglia Circuit)

### Brain Basis

The basal ganglia implement a **default-inhibit** architecture. The globus pallidus
internus (GPi) tonically inhibits the thalamus, preventing action. To act, the
striatum must actively inhibit the GPi (disinhibition), releasing the thalamus.

The direct/indirect pathway model:
- **Direct pathway** (striatum → GPi): disinhibits selected actions
- **Indirect pathway** (striatum → GPe → STN → GPi): strengthens inhibition of
  competing actions

Key references:
- Mink, 1996, "The basal ganglia: focused selection and inhibition of competing
  motor programs," *Progress in Neurobiology*, 50:381-425 — established the
  focused selection model where basal ganglia select desired actions by
  disinhibiting them while suppressing competitors
- Redgrave et al., 1999, "The basal ganglia: a vertebrate solution to the selection
  problem?" *Neuroscience*, 89:1009-1023 — framed basal ganglia as a general
  selection mechanism, not just motor control
- Hikosaka et al., 2000, "Role of the basal ganglia in the control of purposive
  saccadic eye movements," *Physiological Reviews*, 80:953-978 — demonstrated
  the tonic inhibition / phasic disinhibition pattern in oculomotor circuits

**The critical insight:** The default state is suppression. Every potential action
is inhibited until sufficient evidence accumulates to disinhibit it. This is the
opposite of a naive pipeline where everything passes unless explicitly blocked.

### COG Mapping

The **Context Inhibitor** implements basal ganglia gating:
- Default state: **all findings are suppressed**
- Each finding must be actively **disinhibited** by meeting contextual criteria
- Findings that fail disinhibition are suppressed (not deleted — they remain in
  the raw assessment for audit)

This directly addresses the false positive problem. The current pipeline generates
findings and passes them all through. The Context Inhibitor inverts this: findings
are suppressed by default, and only those that survive contextual scrutiny reach
the final report.

**Mapping precision:** This is a **direct computational mapping**, not a metaphor.
The GPi's tonic inhibition → Context Inhibitor's default-suppress. The striatum's
evidence accumulation → Context Inhibitor's disinhibition criteria. The focused
selection → only contextually relevant findings pass.

### Design Implications

- **The Context Inhibitor sits after detectors, before aggregation** (Layer 3).
  Detectors generate freely; the inhibitor gates selectively.
- **Disinhibition criteria are mechanical**, not semantic. They check structural
  features of the conversation (formality markers, stakes indicators, domain
  keywords) rather than trying to "understand" context.
- **The 3 CONFIDENCE_MISCALIBRATION false positives** on casual "absolutely"/
  "definitely" are suppressed because: (a) surrounding text lacks stakes markers,
  (b) the turn is informal (short, no domain vocabulary), (c) no other detectors
  flagged the same region (low corroboration).

See [NEW_COGS.md](NEW_COGS.md) §5 for the full Context Inhibitor specification.

---

## Principle 4: Neuromodulatory Context

### Brain Basis

Four major neuromodulatory systems broadcast global signals that change how
local circuits process information:

**1. Norepinephrine (locus coeruleus → widespread cortex)**
- Controls arousal and signal-to-noise ratio
- Aston-Jones & Cohen, 2005, "An integrative theory of locus coeruleus–
  norepinephrine function: adaptive gain and optimal performance," *Annual Review
  of Neuroscience*, 28:403-450 — established the phasic/tonic mode theory:
  - **Phasic mode:** brief NE bursts enhance processing of task-relevant stimuli
    (high gain, focused attention)
  - **Tonic mode:** sustained NE elevation reduces selectivity (low gain,
    exploratory scanning)
- Berridge & Waterhouse, 2003, "The locus coeruleus–noradrenergic system:
  modulation of behavioral state and state-dependent cognitive processes,"
  *Brain Research Reviews*, 42:33-84

**2. Dopamine (VTA/SNc → prefrontal cortex, striatum)**
- Reward prediction error, learning rate modulation
- Schultz et al., 1997, "A neural substrate of prediction and reward," *Science*,
  275:1593-1599 — dopamine neurons encode prediction error, not reward itself
- Montague et al., 1996, "A framework for mesencephalic dopamine systems based on
  predictive Hebbian learning," *Journal of Neuroscience*, 16:1936-1947

**3. Serotonin (raphe nuclei → widespread)**
- Patience, inhibitory tone, confidence in predictions
- Doya, 2002, "Metalearning and neuromodulation," *Neural Networks*, 15:495-506 —
  proposed serotonin controls the time discount factor (patience vs impulsivity)
- Cools et al., 2008, "Serotonin and dopamine: unifying affective, activational,
  and decision functions," *Neuropsychopharmacology*, 33:1-12

**4. Acetylcholine (basal forebrain → cortex)**
- Attention, cortical plasticity, signal reliability
- Hasselmo, 2006, "The role of acetylcholine in learning and memory," *Current
  Opinion in Neurobiology*, 16:710-715
- Yu & Dayan, 2005, "Uncertainty, neuromodulation, and attention," *Neuron*,
  46:681-692 — acetylcholine signals expected uncertainty (unreliable priors)

### COG Mapping

Not all four neuromodulators need direct COG equivalents. The most valuable
mapping for CereBRO:

| Neuromodulator | COG Function | Priority |
|---------------|-------------|----------|
| Norepinephrine | **Urgency Assessor** — gain signal (phasic/tonic modes) | HIGH — build first |
| Serotonin | **Threshold Modulator** — adjusts detector sensitivity | HIGH — build second |
| Dopamine | Forge learning rate (already implicit in Forge config) | LOW — already exists |
| Acetylcholine | Self-Confidence Assessor (reliability signaling) | MEDIUM — Phase 4 |

The **Urgency Assessor** maps most directly to the locus coeruleus / norepinephrine
system. It reads conversation context and emits a **GainSignal** that modulates
all Layer 2 detectors:
- High urgency (phasic mode) → lower detection thresholds, more findings pass
- Low urgency (tonic mode) → higher thresholds, only strong signals pass

**Mapping precision:** The NE gain modulation is a **direct computational analogy** —
the math is literally the same (multiplicative gain on signal processing). The
dopamine/serotonin mappings are **architectural inspirations** — the computational
details differ but the functional role is preserved.

### Design Implications

- **The GainSignal is a broadcast**, not point-to-point. Every Layer 2 COG
  receives it, just as NE is released diffusely across cortex.
- **Gain is multiplicative**, not additive. It scales existing thresholds rather
  than adding a fixed offset. This preserves relative detector sensitivity.
- **Two COGs (Urgency Assessor + Threshold Modulator) together implement
  neuromodulation.** The Urgency Assessor reads context; the Threshold Modulator
  applies the gain to detector parameters.

---

## Principle 5: Recurrent Feedback

### Brain Basis

The brain is not a feedforward network. Cortico-thalamic feedback projections
outnumber feedforward projections by approximately 10:1 (Sherman & Guillery, 2002,
"The role of the thalamus in the flow of information to the cortex," *Philosophical
Transactions of the Royal Society B*, 357:1695-1708).

The **predictive coding** framework explains why:
- Rao & Ballard, 1999, "Predictive coding in the visual cortex: a functional
  interpretation of some extra-classical receptive-field effects," *Nature
  Neuroscience*, 2:79-87 — higher areas send predictions downward; lower areas
  send only the **prediction error** upward. This reduces redundant information
  flow.
- Friston, 2010, "The free-energy principle: a unified brain theory?" *Nature
  Reviews Neuroscience*, 11:127-138 — generalized predictive coding to the
  free energy principle: the brain minimizes surprise by continuously updating
  its generative model.
- Clark, 2013, "Whatever next? Predictive brains, situated agents, and the future
  of cognitive science," *Behavioral and Brain Sciences*, 36:181-204 — review
  establishing predictive processing as a unifying framework.

**Key constraint:** Feedback is bounded. The brain doesn't loop indefinitely — it
converges within a processing cycle (~100-300ms for a single perceptual inference).
Pathological feedback loops (e.g., epileptic seizures) are a failure mode, not a
feature.

### COG Mapping

The **Feedback Evaluator** implements bounded feedback:
1. Layer 4 receives the initial synthesis from the Aggregator
2. It generates a **prediction** about which findings are consistent/inconsistent
3. It sends a single FeedbackRequest back through the pipeline
4. Detectors that receive the request re-evaluate with the additional context
5. Updated assessments flow forward to a **second aggregation pass**
6. **No further feedback** — exactly one re-evaluation cycle

This differs from the **circular wire anti-pattern** in three critical ways:
- **Bounded iteration:** Exactly one feedback pass, enforced by the accumulator
  pattern (each message carries a `pass_number` field; Layer 4 only generates
  feedback for `pass_number == 1`)
- **Different input:** The feedback pass includes the first-pass synthesis as
  additional context — it's not re-running the same computation
- **Convergence guarantee:** The composition is acyclic in the wiring graph.
  Feedback is implemented as a second stage, not a cycle.

**Mapping precision:** The one-pass feedback is a **simplified computational analogy**.
Real cortical feedback is continuous and multi-pass. We deliberately constrain it
to one pass for determinism and latency guarantees. This is honest engineering
compromise, not biological fidelity.

### Design Implications

- **The Feedback Evaluator is Phase 4 of the build order** — it requires the
  Aggregator and detectors to be working first.
- **The accumulator pattern** (message carries its own pass count) prevents
  unbounded recursion at the protocol level, not just by convention.
- **Re-evaluation is optional** — if the first pass achieves high self-confidence
  (Principle 4, acetylcholine mapping), the Feedback Evaluator skips the
  second pass entirely.

---

## Principle 6: Memory Consolidation

### Brain Basis

The hippocampus does not store memories — it stores an **index** to distributed
cortical representations:

- Teyler & DiScenna, 1986, "The hippocampal memory indexing theory," *Behavioral
  Neuroscience*, 100:147-154 — proposed that the hippocampus stores a sparse
  pointer (index) to the cortical neurons that were active during an experience.
  Retrieval reactivates the cortical pattern via the index.
- Teyler & Rudy, 2007, "The hippocampal indexing theory and episodic memory:
  updating the index," *Hippocampus*, 17:1158-1169 — updated the theory with
  evidence for index updating during reconsolidation.

**Complementary Learning Systems** theory explains the two-speed architecture:
- McClelland et al., 1995, "Why there are complementary learning systems in the
  hippocampus and neocortex: insights from the successes and failures of
  connectionist models of learning and memory," *Psychological Review*, 102:419-457
  — the hippocampus learns quickly (one-shot) but forgets; the neocortex learns
  slowly (many exposures) but retains. Consolidation transfers knowledge from
  hippocampal fast storage to cortical slow storage, typically during sleep.
- O'Reilly & Norman, 2002, "Hippocampal and neocortical contributions to memory:
  advances in the complementary learning systems framework," *Trends in Cognitive
  Sciences*, 6:505-510

**Sleep consolidation:** During sleep, the hippocampus replays experiences to the
cortex, enabling gradual integration without catastrophic interference (Wilson &
McNaughton, 1994, "Reactivation of hippocampal ensemble memories during sleep,"
*Science*, 265:676-679).

### COG Mapping

The **Memory Consolidator** implements hippocampal indexing:
- **Runtime (hippocampal fast learning):** After each pipeline run, the Memory
  Consolidator creates a sparse index entry — not the full conversation, but:
  finding types detected, confidence scores, whether findings were confirmed or
  suppressed, conversation metadata (length, formality, domain markers)
- **Consolidation (sleep replay):** Periodically (not per-request), confirmed and
  rejected findings are formatted as NDJSON corpus entries and appended to the
  Forge corpus
- **Forge evolution (cortical slow learning):** The Forge runs against the expanded
  corpus, producing improved COG parameters. This is the "overnight optimization"
  equivalent of sleep consolidation.

The feedback loop: **runtime → sparse index → corpus entry → Forge evolution →
improved COGs → runtime**

**Mapping precision:** The sparse-index concept is a **direct computational mapping** —
we literally store pointers to patterns rather than full conversations. The
consolidation-as-Forge-evolution is an **architectural analogy** — Forge optimization
is not neurologically similar to sleep replay, but it serves the same functional
purpose (gradual integration of new experience into stable knowledge).

### Design Implications

- **The Memory Consolidator does NOT store full conversations.** It stores a sparse
  index: finding types, scores, context features. This is both biologically motivated
  and practically necessary (storage, privacy).
- **Consolidation is asynchronous.** It does not block the pipeline. The Forge runs
  offline, just as sleep consolidation doesn't interrupt waking cognition.
- **Catastrophic forgetting is prevented** by the Forge's population-based evolution.
  New corpus entries shift the fitness landscape gradually; they don't overwrite
  existing parameter sets.

---

## Principle 7: Neurogenesis and Pruning

### Brain Basis

The brain both creates new neurons and eliminates unused connections:

**Neurogenesis:**
- Gage, 2002, "Neurogenesis in the adult brain," *Journal of Neuroscience*,
  22:612-613 — confirmed adult neurogenesis in the hippocampal dentate gyrus.
  New neurons integrate into existing circuits and are thought to support
  pattern separation (distinguishing similar but distinct memories).
- Eriksson et al., 1998, "Neurogenesis in the adult human hippocampus," *Nature
  Medicine*, 4:1313-1317 — demonstrated adult neurogenesis in humans using BrdU
  labeling.

**Synaptic pruning:**
- Huttenlocher, 1979, "Synaptic density in human frontal cortex — developmental
  changes and effects of aging," *Brain Research*, 163:195-205 — documented the
  dramatic overproduction and subsequent pruning of synapses during development.
  Frontal cortex synaptic density peaks around age 1-2, then declines ~40% by
  adulthood.
- Huttenlocher & Dabholkar, 1997, "Regional differences in synaptogenesis in
  human cerebral cortex," *Journal of Comparative Neurology*, 387:167-178 —
  showed pruning timelines differ by cortical region.

The developmental pattern: **overproduce, then select.** The brain creates far more
neurons and synapses than it needs, then selectively eliminates those that aren't
useful. This is a generate-and-test strategy at the cellular level.

### COG Mapping

| Brain Process | CereBRO Mechanism |
|--------------|-----------------|
| Neurogenesis (new neurons) | AIP competition: generates new COG variants |
| Synaptic pruning (eliminate unused) | GEARS governance: deregisters unused COGs |
| Activity-dependent survival | Forge fitness: COGs survive based on corpus performance |
| Critical period plasticity | Early pipeline phases: more architectural freedom |

**The Lamarckian advantage:** In biology, acquired traits cannot be inherited
(Weismann barrier). A muscle built through exercise doesn't produce offspring
with larger muscles. But in software, **acquired improvements ARE inherited**:
- The Memory Consolidator captures runtime experience (acquired trait)
- The Forge evolves against that experience (selection)
- Improved COGs are deployed as the new default (inheritance)
- The next generation starts with the improvements already built in

This is impossible in biological evolution. It gives CereBRO a fundamental advantage:
the system can improve within a single deployment cycle, not just across generations.

**Mapping precision:** The overproduce-and-select pattern is a **direct computational
mapping** — AIP literally generates variant COGs and selects winners. The Lamarckian
inheritance is **unique to software** and has no biological equivalent. We note this
explicitly rather than pretending the analogy extends further than it does.

### Design Implications

- **AIP competitions are the neurogenesis mechanism.** Each competition generates
  multiple COG variants (Forge GENESIS level creates entirely new Pinions programs).
- **GEARS governance is the pruning mechanism.** COGs that consistently score below
  the Pareto frontier are candidates for deregistration. (Governance is currently
  deferred — see [ROADMAP](../../ROADMAP.md) — but the mechanism is designed.)
- **The Forge's three levels mirror three scales of biological plasticity:**
  - PARAMETER = synaptic weight adjustment (fast, local)
  - RULE = dendritic remodeling (medium, structural)
  - GENESIS = neurogenesis (slow, creates new units)
- **The Lamarckian loop is the unique value proposition of CereBRO.** No biological
  system can do this. It should be built (Phase 5) and measured.

---

## Summary: Principle-to-Layer Mapping

| Principle | Primary Layer | Key COG |
|-----------|-------------|---------|
| 1. Layered abstraction | All | Architecture itself |
| 2. Parallel specialists | Layer 2 | All detectors |
| 3. Inhibitory gating | Layer 3 | Context Inhibitor |
| 4. Neuromodulation | Layer 1 + 3 | Urgency Assessor, Threshold Modulator |
| 5. Recurrent feedback | Layer 4 | Feedback Evaluator |
| 6. Memory consolidation | Layer 5 | Memory Consolidator |
| 7. Neurogenesis/pruning | AIP + GEARS | Forge, governance |
