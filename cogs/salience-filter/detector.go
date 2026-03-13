// Package main implements the Salience Filter COG.
//
// PURE deterministic — no LLM calls. Scores findings by novelty and
// actionability, filtering low-salience items that pass the Context Inhibitor.
// Phase 5, Layer 3 (Salience).
//
// The implementation delegates to internal/pipeline.FilterSalience().
package main

import (
	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Salience Filter's tunable parameters.
type Config = pipeline.SalienceConfig

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return pipeline.DefaultSalienceConfig()
}

// RunSalienceFilter runs the salience filter on assessments.
func RunSalienceFilter(assessments []*reasoningv1.CognitiveAssessment, cfg Config) (*pipeline.SalienceResult, error) {
	result := pipeline.FilterSalience(assessments, cfg)
	return result, nil
}

// RunWithDefaults runs the salience filter with default config.
func RunWithDefaults(assessments []*reasoningv1.CognitiveAssessment) ([]*cerebrov1.SalienceScore, []*reasoningv1.CognitiveAssessment) {
	cfg := pipeline.DefaultSalienceConfig()
	result := pipeline.FilterSalience(assessments, cfg)
	return result.Scores, result.Salient
}
