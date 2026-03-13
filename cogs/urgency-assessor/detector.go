// Package main implements the Urgency Assessor COG.
//
// PURE deterministic — no LLM calls. Scans conversation text for urgency
// and stakes keywords, computes structural complexity and formality, and
// produces a GainSignal for downstream threshold modulation.
// Phase 2, Layer 2 (Gain Modulation).
package main

import (
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Urgency Assessor's tunable parameters.
type Config = pipeline.UrgencyConfig

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return pipeline.DefaultUrgencyConfig()
}

// GainSignal is the output type.
type GainSignal = pipeline.GainSignal

// Run executes the Urgency Assessor on a conversation snapshot.
func Run(snap *reasoningv1.ConversationSnapshot, cfg Config) *GainSignal {
	return pipeline.AssessUrgency(snap, cfg)
}
