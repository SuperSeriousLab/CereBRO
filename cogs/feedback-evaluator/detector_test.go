package main

import (
	"testing"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func TestRun_HighConfidence(t *testing.T) {
	findings := []*reasoningv1.CognitiveAssessment{
		{Confidence: 0.8, DetectorName: "scope-guard"},
	}
	selfConf := &cerebrov1.SelfConfidenceReport{OverallConfidence: 0.9}
	report := &reasoningv1.ReasoningReport{}
	cfg := DefaultConfig()
	result, fbResult := Run(findings, selfConf, nil, report, cfg, nil)
	if fbResult.Applied {
		t.Error("expected no feedback applied for high confidence")
	}
	if len(result) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result))
	}
}

func TestRun_EmptyFindings(t *testing.T) {
	selfConf := &cerebrov1.SelfConfidenceReport{OverallConfidence: 0.5}
	report := &reasoningv1.ReasoningReport{}
	cfg := DefaultConfig()
	result, fbResult := Run(nil, selfConf, nil, report, cfg, nil)
	if len(result) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result))
	}
	if fbResult.Applied {
		t.Error("expected no feedback applied for empty findings")
	}
}
