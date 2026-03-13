// Package main implements the Feedback Evaluator COG.
//
// PURE deterministic — no LLM calls. Re-evaluates low-confidence findings
// when overall self-confidence is below threshold. Selectively re-runs
// detectors on weak findings to improve or confirm them.
// CORTEX Phase 2, Layer 5 (Feedback).
//
// The implementation delegates to internal/pipeline.EvaluateFeedback().
package main

import (
	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Feedback Evaluator's tunable parameters.
type Config = pipeline.FeedbackConfig

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return pipeline.DefaultFeedbackConfig()
}

// Result holds the feedback evaluation output.
type Result = pipeline.FeedbackResult

// Run executes the Feedback Evaluator.
func Run(
	findings []*reasoningv1.CognitiveAssessment,
	selfConf *cerebrov1.SelfConfidenceReport,
	snap *reasoningv1.ConversationSnapshot,
	report *reasoningv1.ReasoningReport,
	cfg Config,
	detectors map[pipeline.Detector]pipeline.DetectorFunc,
) ([]*reasoningv1.CognitiveAssessment, *Result) {
	return pipeline.EvaluateFeedback(findings, selfConf, snap, report, cfg, detectors)
}
