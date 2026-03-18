package main

import (
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func makeTestFinding(name string, ft reasoningv1.FindingType, sev reasoningv1.FindingSeverity, conf float64) *reasoningv1.CognitiveAssessment {
	return &reasoningv1.CognitiveAssessment{
		DetectorName: name,
		FindingType:  ft,
		Severity:     sev,
		Confidence:   conf,
	}
}

// TestRun_HighRisk verifies that many high-severity findings produce
// compound_risk > 0.6 and emit a COMPOUND_PATHOLOGY finding.
func TestRun_HighRisk(t *testing.T) {
	findings := []*reasoningv1.CognitiveAssessment{
		makeTestFinding("contradiction-tracker", reasoningv1.FindingType_CONTRADICTION, reasoningv1.FindingSeverity_CRITICAL, 0.9),
		makeTestFinding("scope-guard", reasoningv1.FindingType_SCOPE_DRIFT, reasoningv1.FindingSeverity_WARNING, 0.85),
		makeTestFinding("anchoring-detector", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_WARNING, 0.78),
		makeTestFinding("sunk-cost-detector", reasoningv1.FindingType_SUNK_COST_FALLACY, reasoningv1.FindingSeverity_WARNING, 0.72),
		makeTestFinding("calibrator", reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION, reasoningv1.FindingSeverity_CAUTION, 0.65),
	}

	result, err := RunWithDefaults(findings)
	if err != nil {
		t.Fatalf("RunWithDefaults: %v", err)
	}

	if result.CompoundRisk <= 0.6 {
		t.Errorf("expected compound_risk > 0.6 for high-risk scenario, got %.3f", result.CompoundRisk)
	}
	if result.Finding == nil {
		t.Errorf("expected COMPOUND_PATHOLOGY finding to be emitted (risk=%.3f)", result.CompoundRisk)
	}
	if result.Finding != nil && result.Finding.GetFindingType() != reasoningv1.FindingType_COMPOUND_PATHOLOGY {
		t.Errorf("expected COMPOUND_PATHOLOGY, got %v", result.Finding.GetFindingType())
	}

	t.Logf("high-risk: count=%d max_sev=%.2f avg_conf=%.2f compound_risk=%.3f",
		result.ActiveDetectorCount, result.MaxSingleSeverity, result.AvgConfidence, result.CompoundRisk)
}

// TestRun_LowRisk verifies that a single low-confidence finding produces
// compound_risk < 0.6 and emits no finding.
func TestRun_LowRisk(t *testing.T) {
	findings := []*reasoningv1.CognitiveAssessment{
		makeTestFinding("calibrator", reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION, reasoningv1.FindingSeverity_INFO, 0.3),
	}

	result, err := RunWithDefaults(findings)
	if err != nil {
		t.Fatalf("RunWithDefaults: %v", err)
	}

	if result.CompoundRisk >= 0.6 {
		t.Errorf("expected compound_risk < 0.6 for low-risk scenario, got %.3f", result.CompoundRisk)
	}
	if result.Finding != nil {
		t.Errorf("expected no COMPOUND_PATHOLOGY finding for low risk (compound_risk=%.3f)", result.CompoundRisk)
	}

	t.Logf("low-risk: count=%d max_sev=%.2f avg_conf=%.2f compound_risk=%.3f",
		result.ActiveDetectorCount, result.MaxSingleSeverity, result.AvgConfidence, result.CompoundRisk)
}

// TestRun_Empty verifies that nil input returns zero risk.
func TestRun_Empty(t *testing.T) {
	result, err := RunWithDefaults(nil)
	if err != nil {
		t.Fatalf("RunWithDefaults: %v", err)
	}

	if result.CompoundRisk != 0.0 {
		t.Errorf("expected compound_risk=0.0 for nil input, got %.3f", result.CompoundRisk)
	}
	if result.Finding != nil {
		t.Errorf("expected no finding for nil input")
	}
}

// TestRun_CustomThreshold verifies that a custom emit_threshold is honored.
func TestRun_CustomThreshold(t *testing.T) {
	findings := []*reasoningv1.CognitiveAssessment{
		makeTestFinding("contradiction-tracker", reasoningv1.FindingType_CONTRADICTION, reasoningv1.FindingSeverity_WARNING, 0.7),
		makeTestFinding("scope-guard", reasoningv1.FindingType_SCOPE_DRIFT, reasoningv1.FindingSeverity_WARNING, 0.65),
		makeTestFinding("anchoring-detector", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_CAUTION, 0.6),
	}

	// With threshold=0.9 (very high), finding should NOT be emitted even at moderate risk.
	highThresholdCfg := DefaultConfig()
	highThresholdCfg.EmitThreshold = 0.9

	result, err := Run(findings, highThresholdCfg)
	if err != nil {
		t.Fatalf("Run with high threshold: %v", err)
	}

	t.Logf("custom threshold: compound_risk=%.3f threshold=0.9 finding_emitted=%v",
		result.CompoundRisk, result.Finding != nil)

	// With default threshold (0.6), same findings may emit a finding.
	defaultResult, err := RunWithDefaults(findings)
	if err != nil {
		t.Fatalf("RunWithDefaults: %v", err)
	}

	if defaultResult.CompoundRisk != result.CompoundRisk {
		t.Errorf("same findings should produce same compound_risk regardless of threshold: got %.3f vs %.3f",
			defaultResult.CompoundRisk, result.CompoundRisk)
	}
	// If compound_risk is below 0.9 with defaults, the high-threshold run should also produce no finding.
	if result.CompoundRisk < 0.9 && result.Finding != nil {
		t.Errorf("finding emitted at risk=%.3f with threshold=0.9", result.CompoundRisk)
	}
}
