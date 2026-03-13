package pipeline

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// TestAssessConfidence_CleanReport verifies high confidence for no findings.
func TestAssessConfidence_CleanReport(t *testing.T) {
	report := &reasoningv1.ReasoningReport{}
	cfg := DefaultSelfConfidenceConfig()
	result := AssessConfidence(report, cfg)

	// Clean report: agreement=1.0, margin=1.0, historical=0.5 (no index)
	// overall = 0.4*1.0 + 0.35*1.0 + 0.25*0.5 = 0.875
	if result.GetRecommendation() != cerebrov1.ConfidenceRecommendation_HIGH_CONFIDENCE {
		t.Errorf("expected HIGH_CONFIDENCE for clean report, got %v", result.GetRecommendation())
	}
	if result.GetFindingPattern() != "CLEAN" {
		t.Errorf("expected CLEAN pattern, got %s", result.GetFindingPattern())
	}
	if result.GetFindingCount() != 0 {
		t.Errorf("expected 0 findings, got %d", result.GetFindingCount())
	}
}

// TestAssessConfidence_SingleFinding verifies moderate confidence for single finding.
func TestAssessConfidence_SingleFinding(t *testing.T) {
	report := &reasoningv1.ReasoningReport{
		Findings: []*reasoningv1.CognitiveAssessment{
			{Confidence: 0.8, FindingType: reasoningv1.FindingType_SCOPE_DRIFT, DetectorName: "scope-guard"},
		},
	}
	cfg := DefaultSelfConfidenceConfig()
	result := AssessConfidence(report, cfg)

	// Single finding: agreement=0.8, margin=(|0.8-0.5|)*2=0.6, historical=0.5
	// overall = 0.4*0.8 + 0.35*0.6 + 0.25*0.5 = 0.32+0.21+0.125 = 0.655
	if result.GetOverallConfidence() < 0.5 || result.GetOverallConfidence() > 0.8 {
		t.Errorf("expected moderate confidence, got %.3f", result.GetOverallConfidence())
	}
	if result.GetRecommendation() != cerebrov1.ConfidenceRecommendation_MODERATE_CONFIDENCE {
		t.Errorf("expected MODERATE_CONFIDENCE, got %v", result.GetRecommendation())
	}
	if result.GetFindingCount() != 1 {
		t.Errorf("expected 1 finding, got %d", result.GetFindingCount())
	}
}

// TestAssessConfidence_MultipleAgreeingFindings tests high agreement (similar confidences).
func TestAssessConfidence_MultipleAgreeingFindings(t *testing.T) {
	report := &reasoningv1.ReasoningReport{
		Findings: []*reasoningv1.CognitiveAssessment{
			{Confidence: 0.85, FindingType: reasoningv1.FindingType_SCOPE_DRIFT, DetectorName: "scope-guard"},
			{Confidence: 0.80, FindingType: reasoningv1.FindingType_CONTRADICTION, DetectorName: "contradiction-tracker"},
		},
	}
	cfg := DefaultSelfConfidenceConfig()
	result := AssessConfidence(report, cfg)

	// High agreement (close confidences), good margin (far from 0.5)
	if result.GetAgreementScore() < 0.9 {
		t.Errorf("expected high agreement for similar confidences, got %.3f", result.GetAgreementScore())
	}
	if result.GetMarginScore() < 0.5 {
		t.Errorf("expected good margin score, got %.3f", result.GetMarginScore())
	}
}

// TestAssessConfidence_DisagreeingFindings tests low agreement (divergent confidences).
func TestAssessConfidence_DisagreeingFindings(t *testing.T) {
	report := &reasoningv1.ReasoningReport{
		Findings: []*reasoningv1.CognitiveAssessment{
			{Confidence: 0.95, FindingType: reasoningv1.FindingType_SCOPE_DRIFT, DetectorName: "scope-guard"},
			{Confidence: 0.30, FindingType: reasoningv1.FindingType_ANCHORING_BIAS, DetectorName: "anchoring-detector"},
		},
	}
	cfg := DefaultSelfConfidenceConfig()
	result := AssessConfidence(report, cfg)

	// Low agreement (0.95 vs 0.30 = high stdev)
	if result.GetAgreementScore() > 0.75 {
		t.Errorf("expected low agreement for divergent confidences, got %.3f", result.GetAgreementScore())
	}
}

// TestAssessConfidence_BorderlineFindings tests low margin (confidences near 0.5).
func TestAssessConfidence_BorderlineFindings(t *testing.T) {
	report := &reasoningv1.ReasoningReport{
		Findings: []*reasoningv1.CognitiveAssessment{
			{Confidence: 0.51, FindingType: reasoningv1.FindingType_SCOPE_DRIFT, DetectorName: "scope-guard"},
			{Confidence: 0.49, FindingType: reasoningv1.FindingType_CONTRADICTION, DetectorName: "contradiction-tracker"},
		},
	}
	cfg := DefaultSelfConfidenceConfig()
	result := AssessConfidence(report, cfg)

	// Near-borderline: margin should be very low
	if result.GetMarginScore() > 0.1 {
		t.Errorf("expected low margin for borderline findings, got %.3f", result.GetMarginScore())
	}
}

// TestAssessConfidence_WithPatternIndex tests historical score lookup.
func TestAssessConfidence_WithPatternIndex(t *testing.T) {
	report := &reasoningv1.ReasoningReport{
		Findings: []*reasoningv1.CognitiveAssessment{
			{Confidence: 0.8, FindingType: reasoningv1.FindingType_SCOPE_DRIFT, DetectorName: "scope-guard"},
		},
	}
	cfg := DefaultSelfConfidenceConfig()
	idx := NewPatternIndex()
	idx.accuracy["SCOPE_DRIFT"] = 0.9
	idx.counts["SCOPE_DRIFT"] = 3
	cfg.PatternIndex = idx
	result := AssessConfidence(report, cfg)

	if math.Abs(result.GetHistoricalScore()-0.9) > 0.01 {
		t.Errorf("expected historical=0.9 with pattern match, got %.3f", result.GetHistoricalScore())
	}
}

// TestAssessConfidence_LowConfidenceRecommendation tests LOW recommendation.
func TestAssessConfidence_LowConfidenceRecommendation(t *testing.T) {
	report := &reasoningv1.ReasoningReport{
		Findings: []*reasoningv1.CognitiveAssessment{
			{Confidence: 0.50, FindingType: reasoningv1.FindingType_SCOPE_DRIFT, DetectorName: "scope-guard"},
			{Confidence: 0.50, FindingType: reasoningv1.FindingType_ANCHORING_BIAS, DetectorName: "anchoring-detector"},
			{Confidence: 0.50, FindingType: reasoningv1.FindingType_CONTRADICTION, DetectorName: "contradiction-tracker"},
		},
	}
	cfg := DefaultSelfConfidenceConfig()
	result := AssessConfidence(report, cfg)

	// All at 0.5: agreement=1.0 (no variance), margin=0.0 (all at borderline), historical=0.5
	// overall = 0.4*1.0 + 0.35*0.0 + 0.25*0.5 = 0.525 → but that's MODERATE
	// Need to push below 0.5. Override weights to penalize more.
	cfg.MarginWeight = 0.5
	cfg.AgreementWeight = 0.2
	cfg.HistoricalWeight = 0.3
	result = AssessConfidence(report, cfg)
	// overall = 0.2*1.0 + 0.5*0.0 + 0.3*0.5 = 0.35

	if result.GetRecommendation() != cerebrov1.ConfidenceRecommendation_LOW_CONFIDENCE_REVIEW_RECOMMENDED {
		t.Errorf("expected LOW_CONFIDENCE, got %v (overall=%.3f)", result.GetRecommendation(), result.GetOverallConfidence())
	}
}

// TestExtractFindingPattern verifies sorted pattern generation.
func TestExtractFindingPattern(t *testing.T) {
	findings := []*reasoningv1.CognitiveAssessment{
		{FindingType: reasoningv1.FindingType_SCOPE_DRIFT},
		{FindingType: reasoningv1.FindingType_ANCHORING_BIAS},
	}
	pattern := ExtractFindingPattern(findings)
	if pattern != "ANCHORING_BIAS+SCOPE_DRIFT" {
		t.Errorf("expected ANCHORING_BIAS+SCOPE_DRIFT, got %s", pattern)
	}

	// Deduplicated
	findings2 := []*reasoningv1.CognitiveAssessment{
		{FindingType: reasoningv1.FindingType_SCOPE_DRIFT},
		{FindingType: reasoningv1.FindingType_SCOPE_DRIFT},
	}
	pattern2 := ExtractFindingPattern(findings2)
	if pattern2 != "SCOPE_DRIFT" {
		t.Errorf("expected SCOPE_DRIFT (deduped), got %s", pattern2)
	}

	// Empty
	pattern3 := ExtractFindingPattern(nil)
	if pattern3 != "CLEAN" {
		t.Errorf("expected CLEAN for nil, got %s", pattern3)
	}
}

// TestLoadPatternIndex verifies corpus loading.
func TestLoadPatternIndex(t *testing.T) {
	// Non-existent file returns empty index, no error.
	idx, err := LoadPatternIndex("/nonexistent/path.ndjson")
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if len(idx.GetAccuracy()) != 0 {
		t.Errorf("expected empty index, got %d entries", len(idx.GetAccuracy()))
	}
}

// TestLoadPatternIndexFromCorpus verifies loading from the real corpus.
func TestLoadPatternIndexFromCorpus(t *testing.T) {
	corpusPath := filepath.Join("..", "..", "data", "corpus", "cognitive-v1.ndjson")
	if _, err := os.Stat(corpusPath); os.IsNotExist(err) {
		t.Skip("corpus not found")
	}

	idx, err := LoadPatternIndex(corpusPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	acc := idx.GetAccuracy()
	if len(acc) == 0 {
		t.Error("expected non-empty pattern index from real corpus")
	}
	t.Logf("loaded %d patterns from corpus", len(acc))
	for pattern, a := range acc {
		t.Logf("  %s: accuracy=%.2f", pattern, a)
	}
}

// TestStdev verifies standard deviation computation.
func TestStdev(t *testing.T) {
	// Empty and single value
	if stdev(nil) != 0 {
		t.Error("expected 0 for nil")
	}
	if stdev([]float64{5.0}) != 0 {
		t.Error("expected 0 for single value")
	}

	// Known values: [2, 4, 4, 4, 5, 5, 7, 9] → mean=5, variance=4, stdev=2
	sd := stdev([]float64{2, 4, 4, 4, 5, 5, 7, 9})
	if math.Abs(sd-2.0) > 0.01 {
		t.Errorf("expected stdev≈2.0, got %.3f", sd)
	}
}

// TestComputeAgreement verifies agreement scoring edge cases.
func TestComputeAgreement(t *testing.T) {
	// No findings = perfect agreement
	if computeAgreement(nil) != 1.0 {
		t.Error("expected 1.0 for no findings")
	}

	// Single finding = 0.8
	single := []*reasoningv1.CognitiveAssessment{{Confidence: 0.5}}
	if computeAgreement(single) != 0.8 {
		t.Errorf("expected 0.8 for single finding, got %.2f", computeAgreement(single))
	}

	// Two identical confidences = perfect agreement
	identical := []*reasoningv1.CognitiveAssessment{
		{Confidence: 0.7},
		{Confidence: 0.7},
	}
	if computeAgreement(identical) != 1.0 {
		t.Errorf("expected 1.0 for identical confidences, got %.2f", computeAgreement(identical))
	}
}

// TestComputeMargin verifies margin scoring.
func TestComputeMargin(t *testing.T) {
	// No findings = 1.0
	if computeMargin(nil) != 1.0 {
		t.Error("expected 1.0 for no findings")
	}

	// Finding at exactly 0.5 = 0 margin
	borderline := []*reasoningv1.CognitiveAssessment{{Confidence: 0.5}}
	if computeMargin(borderline) != 0.0 {
		t.Errorf("expected 0.0 margin for 0.5 confidence, got %.2f", computeMargin(borderline))
	}

	// Finding at 1.0 = max margin (1.0)
	confident := []*reasoningv1.CognitiveAssessment{{Confidence: 1.0}}
	if computeMargin(confident) != 1.0 {
		t.Errorf("expected 1.0 margin for 1.0 confidence, got %.2f", computeMargin(confident))
	}
}
