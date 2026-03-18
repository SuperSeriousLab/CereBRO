// Package main implements the Evidence Asymmetry COG.
//
// PURE deterministic — no LLM calls. Detects structural evidence grounding
// asymmetry between positive and negative claims in assistant turns.
//
// Genesis rules gen4_78 (positive claims under-evidenced) and gen4_86
// (negative claims over-evidenced) form a natural pair. This COG implements
// both as a single ratio detector:
//
//	evidence_asymmetry = avg_evidence_links(negative claims) / avg_evidence_links(positive claims)
//
// When ratio > 1.5 → CONFIDENCE_MISCALIBRATION finding.
// Threshold zones: < 1.0 healthy, 1.0-1.5 borderline, > 1.5 miscalibrated.
//
// Layer 2, Cortical Specialists — evidence grounding analysis.
// The implementation delegates to internal/pipeline.DetectEvidenceAsymmetry().
package main

import (
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Evidence Asymmetry detector's tunable parameters.
type Config = pipeline.EvidenceAsymmetryConfig

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return pipeline.DefaultEvidenceAsymmetryConfig()
}

// Run executes the Evidence Asymmetry detector on a ConversationSnapshot.
// Returns nil if there is insufficient data or the ratio is below threshold.
func Run(snap *reasoningv1.ConversationSnapshot, cfg Config) *reasoningv1.CognitiveAssessment {
	return pipeline.DetectEvidenceAsymmetry(snap, cfg)
}
