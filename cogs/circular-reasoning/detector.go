// Package main implements the Circular Reasoning COG.
//
// PURE deterministic — no LLM calls. Detects when an AI agent restates a
// claim as its own justification — the conclusion is used as a premise.
// "X is true because X is true" pattern — self-referential circular logic.
//
// Signal logic:
//   - Split each assistant turn into sentences (split on '.', '!', '?', ';')
//   - For each sentence pair (A, B), compute Jaccard word-overlap (stop words excluded)
//   - If similarity > SimilarityThreshold AND one sentence contains a causal
//     connector ("because", "since", "therefore", etc.) → circular pair
//   - circular_ratio = circular_turns / total_turns_with_content
//   - Fire when circular_turns >= MinCircularTurns AND circular_ratio > CircularRatioThreshold
//   - Confidence = 0.5 + 0.5 * (circular_ratio - threshold) / (1.0 - threshold)
//   - Severity: HIGH if circular_ratio > 0.6, MEDIUM if > threshold
//
// Finding type: UNSUPPORTED_CONCLUSION
// Tier: Tier2_Structural
// Layer: 2 (Cortical Specialists / Detectors)
//
// The implementation delegates to internal/pipeline.DetectCircularReasoning().
package main

import (
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Circular Reasoning detector's tunable parameters.
type Config = pipeline.CircularReasoningConfig

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return pipeline.DefaultCircularReasoningConfig()
}

// Run executes the Circular Reasoning detector on a ConversationSnapshot.
// Returns nil if there is insufficient data or the circular ratio is below threshold.
func Run(snap *reasoningv1.ConversationSnapshot, cfg Config) *reasoningv1.CognitiveAssessment {
	return pipeline.DetectCircularReasoning(snap, cfg)
}
