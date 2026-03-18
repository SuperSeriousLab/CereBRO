// Package main implements the Evidence Quality COG.
//
// PURE deterministic — no LLM calls. Detects when the majority of evidence
// cited is anecdotal rather than empirical — phrases like "I heard",
// "supposedly", "some say", "people think" instead of "studies show",
// "data shows", "research indicates". High-confidence claims built on
// anecdotal foundations.
//
// Signal logic (per assistant turn):
//   - Count anecdotal phrases (case-insensitive)
//   - Count empirical phrases (case-insensitive)
//   - Count high-confidence markers (case-insensitive)
//   - anecdotal_ratio = anecdotal / (anecdotal + empirical + 1.0)
//   - high_confidence_with_anecdote = high_conf_markers > 0 AND anecdotal > 0
//
// Aggregate across turns:
//   - avg_anecdotal_ratio across assistant turns
//   - high_conf_anecdote_turns = turns where both conditions hold
//
// Fire UNSUPPORTED_CONCLUSION when:
//   - avg_anecdotal_ratio > 0.70 AND anecdotal total >= 2
//   - OR high_conf_anecdote_turns >= 2
//   - Confidence = 0.5 + 0.5 * avg_anecdotal_ratio
//   - Severity: HIGH if avg_ratio > 0.85 or high_conf_anecdote_turns >= 3, MEDIUM otherwise
//
// Finding type: UNSUPPORTED_CONCLUSION
// Tier: Tier2_Structural
// Layer: 2 (Cortical Specialists / Detectors)
//
// The implementation delegates to internal/pipeline.DetectEvidenceQuality().
package main

import (
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Evidence Quality detector's tunable parameters.
type Config = pipeline.EvidenceQualityConfig

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return pipeline.DefaultEvidenceQualityConfig()
}

// Run executes the Evidence Quality detector on a ConversationSnapshot.
// Returns nil if there is insufficient data or the anecdotal ratio is below threshold.
func Run(snap *reasoningv1.ConversationSnapshot, cfg Config) *reasoningv1.CognitiveAssessment {
	return pipeline.DetectEvidenceQuality(snap, cfg)
}
