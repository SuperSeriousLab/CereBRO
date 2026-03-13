// Package main implements the Context Inhibitor COG.
//
// PURE deterministic — no LLM calls. Implements basal ganglia inhibitory
// gating: default-suppresses all findings, selectively disinhibits through
// 5 gates. CORTEX Phase 1, Layer 3 (Inhibition).
//
// Gate ordering:
//  1. Casual hedge suppression (CONFIDENCE_MISCALIBRATION + informal + hedge word)
//  2. Severity auto-pass (CRITICAL always disinhibits)
//  3. Stakes gate (low urgency + low severity → suppress)
//  4. Confidence gate (WARNING needs confidence above threshold)
//  5. Corroboration gate (cross-detector agreement)
//
// The implementation delegates to internal/pipeline.Inhibit().
package main

import (
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Context Inhibitor's tunable parameters.
type Config = pipeline.InhibitorConfig

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return pipeline.DefaultInhibitorConfig()
}

// Result holds the inhibition output.
type Result = pipeline.InhibitorResult

// GainSignal is the neuromodulatory context from the Urgency Assessor.
type GainSignal = pipeline.GainSignal

// Run executes the Context Inhibitor on a set of assessments (Phase 1 mode).
func Run(
	assessments []*reasoningv1.CognitiveAssessment,
	snap *reasoningv1.ConversationSnapshot,
	cfg Config,
) *Result {
	return pipeline.Inhibit(assessments, snap, cfg)
}

// RunWithGain executes the Context Inhibitor with a real GainSignal (Phase 2 mode).
func RunWithGain(
	assessments []*reasoningv1.CognitiveAssessment,
	snap *reasoningv1.ConversationSnapshot,
	cfg Config,
	gain *GainSignal,
) *Result {
	return pipeline.InhibitWithGain(assessments, snap, cfg, gain)
}
