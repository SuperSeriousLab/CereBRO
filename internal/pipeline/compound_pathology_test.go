package pipeline

import (
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// buildTestAggregator constructs a CompoundPathologyAggregator using the
// default embedded FIS config. Fails the test if construction fails.
func buildTestAggregator(t *testing.T) *CompoundPathologyAggregator {
	t.Helper()
	agg, err := NewDefaultCompoundPathologyAggregator()
	if err != nil {
		t.Fatalf("NewDefaultCompoundPathologyAggregator: %v", err)
	}
	return agg
}

// makeCompoundAssessment is a helper for building test assessments.
func makeCompoundAssessment(name string, ft reasoningv1.FindingType, sev reasoningv1.FindingSeverity, conf float64) *reasoningv1.CognitiveAssessment {
	return &reasoningv1.CognitiveAssessment{
		DetectorName: name,
		FindingType:  ft,
		Severity:     sev,
		Confidence:   conf,
	}
}

// =============================================================================
// TestCompoundPathology_HighRisk — many high-severity detectors → COMPOUND_PATHOLOGY
// =============================================================================

func TestCompoundPathology_HighRisk(t *testing.T) {
	agg := buildTestAggregator(t)

	// 5 high-confidence critical/warning findings → expect compound_risk > 0.6
	findings := []*reasoningv1.CognitiveAssessment{
		makeCompoundAssessment("contradiction-tracker", reasoningv1.FindingType_CONTRADICTION, reasoningv1.FindingSeverity_CRITICAL, 0.92),
		makeCompoundAssessment("scope-guard", reasoningv1.FindingType_SCOPE_DRIFT, reasoningv1.FindingSeverity_WARNING, 0.85),
		makeCompoundAssessment("anchoring-detector", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_WARNING, 0.78),
		makeCompoundAssessment("sunk-cost-detector", reasoningv1.FindingType_SUNK_COST_FALLACY, reasoningv1.FindingSeverity_WARNING, 0.72),
		makeCompoundAssessment("confidence-calibrator", reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION, reasoningv1.FindingSeverity_CAUTION, 0.65),
	}

	result := agg.Aggregate(findings)

	if result.CompoundRisk <= 0.6 {
		t.Errorf("expected compound_risk > 0.6 for high-risk scenario, got %.3f", result.CompoundRisk)
	}
	if result.Finding == nil {
		t.Errorf("expected COMPOUND_PATHOLOGY finding to be emitted (risk=%.3f)", result.CompoundRisk)
	}
	if result.Finding != nil && result.Finding.GetFindingType() != reasoningv1.FindingType_COMPOUND_PATHOLOGY {
		t.Errorf("expected FindingType_COMPOUND_PATHOLOGY, got %v", result.Finding.GetFindingType())
	}
	if result.Finding != nil && result.Finding.GetConfidence() != result.CompoundRisk {
		t.Errorf("finding confidence (%.3f) should equal compound_risk (%.3f)",
			result.Finding.GetConfidence(), result.CompoundRisk)
	}
	if result.ActiveDetectorCount != 5 {
		t.Errorf("expected ActiveDetectorCount=5, got %d", result.ActiveDetectorCount)
	}

	t.Logf("high-risk: count=%d max_sev=%.2f avg_conf=%.2f compound_risk=%.3f",
		result.ActiveDetectorCount, result.MaxSingleSeverity, result.AvgConfidence, result.CompoundRisk)
}

// =============================================================================
// TestCompoundPathology_LowRisk — single low-confidence finding → no COMPOUND_PATHOLOGY
// =============================================================================

func TestCompoundPathology_LowRisk(t *testing.T) {
	agg := buildTestAggregator(t)

	// Single low-confidence info finding → expect compound_risk < 0.6
	findings := []*reasoningv1.CognitiveAssessment{
		makeCompoundAssessment("confidence-calibrator",
			reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION,
			reasoningv1.FindingSeverity_INFO,
			0.3),
	}

	result := agg.Aggregate(findings)

	if result.CompoundRisk >= 0.6 {
		t.Errorf("expected compound_risk < 0.6 for low-risk scenario, got %.3f", result.CompoundRisk)
	}
	if result.Finding != nil {
		t.Errorf("expected no COMPOUND_PATHOLOGY finding for low risk (compound_risk=%.3f)", result.CompoundRisk)
	}

	t.Logf("low-risk: count=%d max_sev=%.2f avg_conf=%.2f compound_risk=%.3f",
		result.ActiveDetectorCount, result.MaxSingleSeverity, result.AvgConfidence, result.CompoundRisk)
}

// =============================================================================
// TestCompoundPathology_NoFindings — empty input → zero risk
// =============================================================================

func TestCompoundPathology_NoFindings(t *testing.T) {
	agg := buildTestAggregator(t)

	result := agg.Aggregate(nil)

	if result.CompoundRisk != 0.0 {
		t.Errorf("expected compound_risk=0.0 for empty findings, got %.3f", result.CompoundRisk)
	}
	if result.Finding != nil {
		t.Errorf("expected no finding for empty input")
	}
	if result.ActiveDetectorCount != 0 {
		t.Errorf("expected ActiveDetectorCount=0 for empty input, got %d", result.ActiveDetectorCount)
	}
}

// =============================================================================
// TestCompoundPathology_NilAggregator — nil aggregator returns passthrough
// =============================================================================

func TestCompoundPathology_NilAggregator(t *testing.T) {
	var agg *CompoundPathologyAggregator // nil

	findings := []*reasoningv1.CognitiveAssessment{
		makeCompoundAssessment("contradiction-tracker",
			reasoningv1.FindingType_CONTRADICTION,
			reasoningv1.FindingSeverity_CRITICAL,
			0.95),
	}

	result := agg.Aggregate(findings)

	if result.CompoundRisk != 0.0 {
		t.Errorf("nil aggregator should return zero risk, got %.3f", result.CompoundRisk)
	}
	if result.Finding != nil {
		t.Errorf("nil aggregator should return no finding")
	}
}

// =============================================================================
// TestCompoundPathology_Gradient — high risk > medium risk > low risk
// =============================================================================

func TestCompoundPathology_Gradient(t *testing.T) {
	agg := buildTestAggregator(t)

	// High risk: 5 high-confidence critical findings.
	highFindings := []*reasoningv1.CognitiveAssessment{
		makeCompoundAssessment("d1", reasoningv1.FindingType_CONTRADICTION, reasoningv1.FindingSeverity_CRITICAL, 0.9),
		makeCompoundAssessment("d2", reasoningv1.FindingType_SCOPE_DRIFT, reasoningv1.FindingSeverity_CRITICAL, 0.88),
		makeCompoundAssessment("d3", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_WARNING, 0.82),
		makeCompoundAssessment("d4", reasoningv1.FindingType_SUNK_COST_FALLACY, reasoningv1.FindingSeverity_WARNING, 0.80),
		makeCompoundAssessment("d5", reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION, reasoningv1.FindingSeverity_WARNING, 0.75),
	}

	// Medium risk: 2 medium-confidence warning findings.
	mediumFindings := []*reasoningv1.CognitiveAssessment{
		makeCompoundAssessment("d1", reasoningv1.FindingType_CONTRADICTION, reasoningv1.FindingSeverity_WARNING, 0.6),
		makeCompoundAssessment("d2", reasoningv1.FindingType_SCOPE_DRIFT, reasoningv1.FindingSeverity_CAUTION, 0.5),
	}

	// Low risk: 1 low-confidence info finding.
	lowFindings := []*reasoningv1.CognitiveAssessment{
		makeCompoundAssessment("d1", reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION, reasoningv1.FindingSeverity_INFO, 0.3),
	}

	highResult := agg.Aggregate(highFindings)
	mediumResult := agg.Aggregate(mediumFindings)
	lowResult := agg.Aggregate(lowFindings)

	t.Logf("gradient: high=%.3f medium=%.3f low=%.3f",
		highResult.CompoundRisk, mediumResult.CompoundRisk, lowResult.CompoundRisk)

	if highResult.CompoundRisk <= mediumResult.CompoundRisk {
		t.Errorf("high risk (%.3f) should exceed medium risk (%.3f)",
			highResult.CompoundRisk, mediumResult.CompoundRisk)
	}
	if mediumResult.CompoundRisk <= lowResult.CompoundRisk {
		t.Errorf("medium risk (%.3f) should exceed low risk (%.3f)",
			mediumResult.CompoundRisk, lowResult.CompoundRisk)
	}
}

// =============================================================================
// TestCompoundPathology_FindingSeverityMap — risk > 0.85 → CRITICAL
// =============================================================================

func TestCompoundPathology_FindingSeverityMap(t *testing.T) {
	agg := buildTestAggregator(t)

	// Maximal load: 10+ detectors all firing at max confidence.
	var manyFindings []*reasoningv1.CognitiveAssessment
	ftypes := []reasoningv1.FindingType{
		reasoningv1.FindingType_CONTRADICTION,
		reasoningv1.FindingType_SCOPE_DRIFT,
		reasoningv1.FindingType_ANCHORING_BIAS,
		reasoningv1.FindingType_SUNK_COST_FALLACY,
		reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION,
		reasoningv1.FindingType_SILENT_REVISION,
		reasoningv1.FindingType_CONCEPTUAL_ANCHORING,
		reasoningv1.FindingType_INHERITED_POSITION,
	}
	for i, ft := range ftypes {
		manyFindings = append(manyFindings, makeCompoundAssessment(
			"d"+string(rune('1'+i)),
			ft,
			reasoningv1.FindingSeverity_CRITICAL,
			0.95,
		))
	}

	result := agg.Aggregate(manyFindings)

	t.Logf("max-load: count=%d max_sev=%.2f avg_conf=%.2f compound_risk=%.3f",
		result.ActiveDetectorCount, result.MaxSingleSeverity, result.AvgConfidence, result.CompoundRisk)

	if result.CompoundRisk <= 0.6 {
		t.Errorf("expected compound_risk > 0.6 for max-load, got %.3f", result.CompoundRisk)
	}
	if result.Finding == nil {
		t.Errorf("expected COMPOUND_PATHOLOGY finding for max-load scenario")
		return
	}
	// At very high risk, severity should be CRITICAL.
	if result.CompoundRisk > 0.85 && result.Finding.GetSeverity() != reasoningv1.FindingSeverity_CRITICAL {
		t.Errorf("expected CRITICAL severity for risk=%.3f, got %v",
			result.CompoundRisk, result.Finding.GetSeverity())
	}
}

// =============================================================================
// TestComputeCompoundInputs — unit test for signal extraction helper
// =============================================================================

func TestComputeCompoundInputs(t *testing.T) {
	findings := []*reasoningv1.CognitiveAssessment{
		{Severity: reasoningv1.FindingSeverity_CRITICAL, Confidence: 0.9},
		{Severity: reasoningv1.FindingSeverity_WARNING, Confidence: 0.7},
		{Severity: reasoningv1.FindingSeverity_CAUTION, Confidence: 0.4},
	}

	count, maxSev, avgConf := computeCompoundInputs(findings)

	if count != 3 {
		t.Errorf("expected count=3, got %d", count)
	}
	// max_single_severity should be from CRITICAL finding:
	// conf=0.9 vs sevBoost=1.0 → effective=1.0
	if maxSev != 1.0 {
		t.Errorf("expected max_single_severity=1.0 (CRITICAL boost), got %.3f", maxSev)
	}
	// avg_confidence = (0.9 + 0.7 + 0.4) / 3 = 0.667
	expectedAvg := (0.9 + 0.7 + 0.4) / 3.0
	if diff := avgConf - expectedAvg; diff > 0.001 || diff < -0.001 {
		t.Errorf("expected avg_confidence=%.3f, got %.3f", expectedAvg, avgConf)
	}
}
