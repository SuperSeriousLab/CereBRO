// Package main implements the Entity Coherence COG.
//
// PURE deterministic — no LLM calls. Detects when the agent describes the same
// entity (person, system, concept) with contradictory properties across different
// assistant turns without acknowledging the change.
//
// Signal logic:
//
//   Step 1 — Entity extraction (heuristic):
//     - Named entities = capitalized multi-word token pairs (Title Case consecutive words)
//       that appear in >= 2 assistant turns.
//     - Proxy entities: "the system", "the model", "the solution", "this approach",
//       "the algorithm", "the service", "the api".
//     - Entity names are normalized to lowercase.
//
//   Step 2 — Descriptor extraction per turn:
//     For each entity mention, collect descriptors within a ±5-word window:
//     - Positive: "reliable", "stable", "fast", "accurate", "efficient", "robust",
//       "correct", "consistent", "good", "effective", "strong", "sound", "solid",
//       "trustworthy", "dependable"
//     - Negative: "unreliable", "unstable", "slow", "inaccurate", "inefficient",
//       "fragile", "incorrect", "inconsistent", "bad", "ineffective", "weak",
//       "unsound", "broken", "untrustworthy"
//
//   Step 3 — Contradiction detection:
//     For each entity seen in >= 2 turns:
//     - If entity has BOTH positive and negative descriptors across DIFFERENT turns
//       → contradiction pair.
//     - Skip turns that explicitly acknowledge a change ("but now", "however now",
//       "changed to", "used to be").
//
//   Fire CONTRADICTION when:
//     - contradiction_count >= 2
//     - Confidence = min(0.5 + 0.3 * contradiction_count, 0.95)
//     - Severity: HIGH (CRITICAL) if count >= 4, MEDIUM (WARNING) if count >= 2
//
// Finding type: CONTRADICTION
// Tier: Tier2_Structural
// Layer: 2 (Cortical Specialists / Detectors)
//
// The implementation delegates to internal/pipeline.DetectEntityCoherence().
package main

import (
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Entity Coherence detector's tunable parameters.
type Config = pipeline.EntityCoherenceConfig

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return pipeline.DefaultEntityCoherenceConfig()
}

// Run executes the Entity Coherence detector on a ConversationSnapshot.
// Returns nil if there is insufficient data or contradiction count is below threshold.
func Run(snap *reasoningv1.ConversationSnapshot, cfg Config) *reasoningv1.CognitiveAssessment {
	return pipeline.DetectEntityCoherence(snap, cfg)
}
