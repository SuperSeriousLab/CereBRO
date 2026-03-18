// Package main implements the Sustained Conviction Signal COG.
//
// PURE deterministic — no LLM calls. Detects sustained pathological
// conviction in the recent conversation window by computing the rolling
// average MembershipValue of the last 5 assistant-turn claims.
//
// Genesis rule: gen0_76 (SustainedConvictionSignal_v5)
// Tier: Tier1_Bias
// Layer: 2 (Cortical Specialists / Detectors)
//
// Fires when AvgMV(last 5 assistant claims) > 0.595.
// Covers: SYCOPHANCY, CATHEDRAL_COMPLEXITY, COUNTER_EVIDENCE_DEPLETION,
// CONFIDENCE_MISCALIBRATION.
package main

import (
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Sustained Conviction Detector's tunable parameters.
type Config = pipeline.SustainedConvictionConfig

// DefaultConfig returns the default configuration (gen0_76: window=5, threshold=0.595).
func DefaultConfig() Config {
	return pipeline.DefaultSustainedConvictionConfig()
}

// Run executes the Sustained Conviction Detector on a conversation snapshot.
// Returns a SYCOPHANCY finding when the rolling average MV of the last N
// assistant turns exceeds the FireThreshold. Returns nil if the threshold
// is not met or if insufficient turns are present.
func Run(snap *reasoningv1.ConversationSnapshot, cfg Config) *reasoningv1.CognitiveAssessment {
	return pipeline.DetectSustainedConviction(snap, cfg)
}
