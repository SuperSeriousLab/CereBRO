// Package main implements the Assumption Surfacer COG.
//
// PURE deterministic — no LLM calls. Detects when an AI agent makes
// high-confidence claims whose supporting premises were never stated in
// prior turns — arguing from unstated assumptions. This is a structural
// reasoning flaw where confidence is unjustified because the foundation
// is hidden.
//
// Signal logic:
//   - Count "claim phrases" (high-confidence assertions): "clearly", "obviously", etc.
//   - Count "premise phrases" (stated foundations): "because", "since", "given that", etc.
//   - assumption_ratio = claim_phrases / (claim_phrases + premise_phrases + 1.0)
//   - Fire when assumption_ratio > 0.75 AND claim_phrases >= 2
//   - Confidence = 0.5 + 0.5 * (assumption_ratio - 0.75) / 0.25
//   - Severity: HIGH if ratio > 0.85, MEDIUM if > 0.75
//
// Finding type: UNSUPPORTED_CONCLUSION
// Tier: Tier2_Structural
// Layer: 2 (Cortical Specialists / Detectors)
//
// The implementation delegates to internal/pipeline.DetectAssumptionSurfacer().
package main

import (
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Assumption Surfacer detector's tunable parameters.
type Config = pipeline.AssumptionSurfacerConfig

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return pipeline.DefaultAssumptionSurfacerConfig()
}

// Run executes the Assumption Surfacer detector on a ConversationSnapshot.
// Returns nil if there is insufficient data or the assumption ratio is below threshold.
func Run(snap *reasoningv1.ConversationSnapshot, cfg Config) *reasoningv1.CognitiveAssessment {
	return pipeline.DetectAssumptionSurfacer(snap, cfg)
}
