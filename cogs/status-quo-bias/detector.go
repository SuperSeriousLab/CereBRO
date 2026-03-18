// Package main implements the Status-Quo Bias COG.
//
// PURE deterministic — no LLM calls. Detects when the assistant systematically
// frames the status quo as the default-good and change as requiring extra
// justification — asymmetric argument burden.
//
// Signal logic (per assistant turn):
//   - Count status_quo phrases ("we've always", "time-tested", "tried and true", ...)
//   - Count change_burden phrases ("why change", "no reason to change", ...)
//   - Count change_positive phrases (counter-signal: "let's improve", "modernize", ...)
//   - bias_score = (status_quo + change_burden) / (status_quo + change_burden + change_positive + 1.0)
//
// Aggregate across turns with any matched phrases:
//   - avg_bias_score across scored assistant turns
//
// Fire STATUS_QUO_BIAS when:
//   - avg_bias_score > 0.65 AND (status_quo_total + change_burden_total) >= 3
//   - Confidence = 0.5 + 0.5 * (avg_bias_score - 0.65) / 0.35
//   - Severity: HIGH if avg > 0.85, MEDIUM otherwise
//
// Finding type: STATUS_QUO_BIAS
// Tier: Tier2_Structural
// Layer: 2 (Cortical Specialists / Detectors)
//
// The implementation delegates to internal/pipeline.DetectStatusQuoBias().
package main

import (
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Status-Quo Bias detector's tunable parameters.
type Config = pipeline.StatusQuoBiasConfig

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return pipeline.DefaultStatusQuoBiasConfig()
}

// Run executes the Status-Quo Bias detector on a ConversationSnapshot.
// Returns nil if there is insufficient data or bias score is below threshold.
func Run(snap *reasoningv1.ConversationSnapshot, cfg Config) *reasoningv1.CognitiveAssessment {
	return pipeline.DetectStatusQuoBias(snap, cfg)
}
