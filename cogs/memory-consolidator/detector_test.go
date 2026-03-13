package main

import (
	"testing"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

func TestRunConsolidator(t *testing.T) {
	idx := pipeline.NewPatternIndex()
	cfg := DefaultConfig()
	cfg.CorpusOutputPath = t.TempDir() + "/test-corpus.ndjson"

	consolidator := pipeline.NewConsolidator(cfg, idx)

	input := &pipeline.ConsolidationInput{
		ConversationID: "test-conv-1",
		Report: &reasoningv1.ReasoningReport{
			Findings: []*reasoningv1.CognitiveAssessment{
				{
					FindingType:  reasoningv1.FindingType_ANCHORING_BIAS,
					Severity:     reasoningv1.FindingSeverity_WARNING,
					Confidence:   0.85,
					DetectorName: "anchoring-detector",
				},
			},
		},
		SelfConf: &cerebrov1.SelfConfidenceReport{OverallConfidence: 0.8},
		Snap:     &reasoningv1.ConversationSnapshot{TotalTurns: 5},
	}

	result := RunConsolidator(input, consolidator)
	// Novel pattern should trigger consolidation.
	if !result.Consolidated {
		t.Error("expected consolidation to trigger")
	}
}

func TestSubmitFeedback(t *testing.T) {
	idx := pipeline.NewPatternIndex()
	cfg := DefaultConfig()
	cfg.CorpusOutputPath = t.TempDir() + "/test-feedback.ndjson"

	consolidator := pipeline.NewConsolidator(cfg, idx)

	// First, consolidate something to have a stored result.
	input := &pipeline.ConsolidationInput{
		ConversationID: "feedback-conv",
		Report: &reasoningv1.ReasoningReport{
			Findings: []*reasoningv1.CognitiveAssessment{
				{
					FindingType:  reasoningv1.FindingType_SUNK_COST_FALLACY,
					Confidence:   0.9,
					DetectorName: "sunk-cost-detector",
				},
			},
		},
		Snap: &reasoningv1.ConversationSnapshot{TotalTurns: 3},
	}
	consolidator.Consolidate(input)

	err := SubmitFeedback(consolidator, "feedback-conv", "confirmed")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Unknown conversation should error.
	err = SubmitFeedback(consolidator, "unknown", "confirmed")
	if err == nil {
		t.Error("expected error for unknown conversation")
	}
}
