// Package main implements the Compound Pathology Aggregator COG.
//
// PURE deterministic — no LLM calls. First Tier 3 meta-COG.
// Reads signals from all active Tier 1 and Tier 2 COG findings and computes
// a compound pathology risk score using a Mamdani FIS (fugo).
//
// FIS inputs:
//   - active_detector_count  normalized count of detectors that fired (0-1, max 10)
//   - max_single_severity    highest severity/confidence among findings (0-1)
//   - avg_confidence         mean confidence across all findings (0-1)
//
// FIS output:
//   - compound_risk          aggregated pathology risk (0-1)
//
// When compound_risk > emit_threshold (default 0.6), emits a
// COMPOUND_PATHOLOGY CognitiveAssessment.
//
// Pipeline ordering: this COG runs AFTER all other COGs (Tier 1/2).
// Currently wired as Stage 5.6 in the pipeline.
// TODO(ordering): formalize a Tier 3 slot with guaranteed ordering.
//
// The implementation delegates to internal/pipeline.CompoundPathologyAggregator.
package main

import (
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Compound Pathology Aggregator's tunable parameters.
type Config = pipeline.CompoundPathologyConfig

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return pipeline.DefaultCompoundPathologyConfig()
}

// Result holds the aggregator output.
type Result = pipeline.CompoundPathologyResult

// Run executes the Compound Pathology Aggregator on a set of Tier 1/2 findings.
// Returns the compound risk score and an optional COMPOUND_PATHOLOGY finding.
func Run(
	findings []*reasoningv1.CognitiveAssessment,
	cfg Config,
) (*Result, error) {
	agg, err := pipeline.NewDefaultCompoundPathologyAggregator()
	if err != nil {
		return nil, err
	}
	agg.Config = cfg
	return agg.Aggregate(findings), nil
}

// RunWithDefaults executes the Compound Pathology Aggregator with default config.
func RunWithDefaults(findings []*reasoningv1.CognitiveAssessment) (*Result, error) {
	return Run(findings, DefaultConfig())
}
