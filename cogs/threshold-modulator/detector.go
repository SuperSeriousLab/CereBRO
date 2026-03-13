// Package main implements the Threshold Modulator COG.
//
// PURE deterministic — no LLM calls. Translates a GainSignal into
// per-detector threshold adjustments. High urgency → lower thresholds
// (more sensitive). Low formality → higher thresholds (less sensitive).
// CORTEX Phase 2, Layer 2 (Gain Modulation).
package main

import (
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Threshold Modulator's tunable parameters.
type Config = pipeline.ModulatorConfig

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return pipeline.DefaultModulatorConfig()
}

// ThresholdAdjustments is the output type.
type ThresholdAdjustments = pipeline.ThresholdAdjustments

// Run executes the Threshold Modulator on a GainSignal.
func Run(gain *pipeline.GainSignal, cfg Config) *ThresholdAdjustments {
	return pipeline.Modulate(gain, cfg)
}
