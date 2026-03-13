package main

import (
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func TestRun(t *testing.T) {
	report := &reasoningv1.ReasoningReport{
		Findings: []*reasoningv1.CognitiveAssessment{
			{Confidence: 0.8, DetectorName: "scope-guard", FindingType: reasoningv1.FindingType_SCOPE_DRIFT},
		},
	}
	cfg := DefaultConfig()
	result := Run(report, cfg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.OverallConfidence <= 0 || result.OverallConfidence > 1.0 {
		t.Errorf("confidence out of range: %.2f", result.OverallConfidence)
	}
}

func TestRun_EmptyReport(t *testing.T) {
	report := &reasoningv1.ReasoningReport{}
	cfg := DefaultConfig()
	result := Run(report, cfg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
