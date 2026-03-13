package main

import (
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func TestRunSalienceFilter(t *testing.T) {
	assessments := []*reasoningv1.CognitiveAssessment{
		{
			FindingType:   reasoningv1.FindingType_CONTRADICTION,
			Severity:      reasoningv1.FindingSeverity_WARNING,
			Confidence:    0.85,
			DetectorName:  "contradiction-tracker",
			Explanation:   "Found contradicting statements about the system architecture in turns 2 and 4",
			RelevantTurns: []uint32{2, 4},
		},
	}

	result, err := RunSalienceFilter(assessments, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Scores) != 1 {
		t.Fatalf("expected 1 score, got %d", len(result.Scores))
	}
	if result.Scores[0].GetScore() < 0.3 {
		t.Errorf("expected score above threshold, got %.2f", result.Scores[0].GetScore())
	}
	if len(result.Salient) != 1 {
		t.Errorf("expected 1 salient assessment, got %d", len(result.Salient))
	}
}

func TestRunWithDefaults(t *testing.T) {
	assessments := []*reasoningv1.CognitiveAssessment{
		{
			FindingType:   reasoningv1.FindingType_ANCHORING_BIAS,
			Severity:      reasoningv1.FindingSeverity_WARNING,
			Confidence:    0.8,
			DetectorName:  "anchoring-detector",
			Explanation:   "Anchoring on initial estimate",
			RelevantTurns: []uint32{1},
		},
	}

	scores, salient := RunWithDefaults(assessments)
	if len(scores) != 1 {
		t.Fatalf("expected 1 score, got %d", len(scores))
	}
	if len(salient) != 1 {
		t.Errorf("expected 1 salient, got %d", len(salient))
	}
}

func TestRunSalienceFilter_Empty(t *testing.T) {
	result, err := RunSalienceFilter(nil, DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Scores) != 0 {
		t.Errorf("expected 0 scores for nil input, got %d", len(result.Scores))
	}
}
