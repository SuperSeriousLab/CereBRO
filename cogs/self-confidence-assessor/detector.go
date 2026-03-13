// Package main implements the Self-Confidence Assessor COG.
//
// PURE deterministic — no LLM calls. Assesses system confidence in its own
// findings by computing agreement, margin, and historical scores into an
// overall self-confidence report.
// Phase 2, Layer 4 (Self-Assessment).
//
// The implementation delegates to internal/pipeline.AssessConfidence().
package main

import (
	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Self-Confidence Assessor's tunable parameters.
type Config = pipeline.SelfConfidenceConfig

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return pipeline.DefaultSelfConfidenceConfig()
}

// Run executes the Self-Confidence Assessor on a ReasoningReport.
func Run(report *reasoningv1.ReasoningReport, cfg Config) *cerebrov1.SelfConfidenceReport {
	return pipeline.AssessConfidence(report, cfg)
}
