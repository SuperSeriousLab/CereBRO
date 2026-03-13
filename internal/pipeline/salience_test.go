package pipeline

import (
	"math"
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

const floatTol = 1e-9

func TestFilterSalience_SingleHighSalience(t *testing.T) {
	a := &reasoningv1.CognitiveAssessment{
		FindingType:  reasoningv1.FindingType_CONTRADICTION,
		Severity:     reasoningv1.FindingSeverity_WARNING,
		DetectorName: "contradiction-tracker",
		Explanation:  "The user contradicted their earlier position on project timeline and budget allocation significantly.",
		RelevantTurns: []uint32{3, 7},
		Confidence:   0.8,
		Contradiction: &reasoningv1.ContradictionDetail{
			ClaimAText: "Project will take 3 months",
			ClaimBText: "Project will take 12 months",
		},
	}

	cfg := DefaultSalienceConfig()
	result := FilterSalience([]*reasoningv1.CognitiveAssessment{a}, cfg)

	if len(result.Scores) != 1 {
		t.Fatalf("expected 1 score, got %d", len(result.Scores))
	}

	sc := result.Scores[0]

	// Novelty: 1/1 = 1.0
	if math.Abs(sc.Novelty-1.0) > floatTol {
		t.Errorf("novelty: got %.4f, want 1.0", sc.Novelty)
	}

	// Actionability: +0.3 (contradiction evidence) +0.3 (turn refs) +0.2 (explanation > 50) +0.2 (confidence > 0.7) = 1.0
	if sc.Actionability < 0.8 {
		t.Errorf("actionability: got %.4f, want >= 0.8", sc.Actionability)
	}

	// Should be above threshold.
	if !sc.AboveThreshold {
		t.Error("expected above threshold")
	}

	if len(result.Salient) != 1 {
		t.Fatalf("expected 1 salient finding, got %d", len(result.Salient))
	}
}

func TestFilterSalience_DuplicateTypes(t *testing.T) {
	assessments := make([]*reasoningv1.CognitiveAssessment, 5)
	for i := range assessments {
		assessments[i] = &reasoningv1.CognitiveAssessment{
			FindingType:   reasoningv1.FindingType_ANCHORING_BIAS,
			Severity:      reasoningv1.FindingSeverity_CAUTION,
			DetectorName:  "anchoring-detector",
			Explanation:   "Short",
			RelevantTurns: []uint32{uint32(i)},
			Confidence:    0.5,
			Anchoring: &reasoningv1.AnchoringDetail{
				AnchorValue:   100.0,
				EstimateValue: 95.0,
			},
		}
	}

	cfg := DefaultSalienceConfig()
	result := FilterSalience(assessments, cfg)

	if len(result.Scores) != 5 {
		t.Fatalf("expected 5 scores, got %d", len(result.Scores))
	}

	for _, sc := range result.Scores {
		// Novelty: 1/5 = 0.2
		if math.Abs(sc.Novelty-0.2) > floatTol {
			t.Errorf("novelty: got %.4f, want 0.2", sc.Novelty)
		}
		// Actionability: +0.3 (anchoring evidence) +0.3 (turn refs) = 0.6
		if math.Abs(sc.Actionability-0.6) > floatTol {
			t.Errorf("actionability: got %.4f, want 0.6", sc.Actionability)
		}
		// Composite: 0.4*0.2 + 0.4*0.6 + 0.2*0.5 = 0.08 + 0.24 + 0.10 = 0.42
		expected := 0.4*0.2 + 0.4*0.6 + 0.2*0.5
		if math.Abs(sc.Score-expected) > floatTol {
			t.Errorf("score: got %.4f, want %.4f", sc.Score, expected)
		}
	}
}

func TestFilterSalience_CriticalNoExplanation(t *testing.T) {
	a := &reasoningv1.CognitiveAssessment{
		FindingType:  reasoningv1.FindingType_CONTRADICTION,
		Severity:     reasoningv1.FindingSeverity_CRITICAL,
		DetectorName: "contradiction-tracker",
		Explanation:  "Short.",
		Confidence:   0.5,
	}

	cfg := DefaultSalienceConfig()
	result := FilterSalience([]*reasoningv1.CognitiveAssessment{a}, cfg)

	if len(result.Scores) != 1 {
		t.Fatalf("expected 1 score, got %d", len(result.Scores))
	}

	sc := result.Scores[0]

	// Novelty: 1.0, Actionability: 0.0, Severity: 1.0
	// Composite: 0.4*1.0 + 0.4*0.0 + 0.2*1.0 = 0.6
	expected := 0.4*1.0 + 0.4*0.0 + 0.2*1.0
	if math.Abs(sc.Score-expected) > floatTol {
		t.Errorf("score: got %.4f, want %.4f", sc.Score, expected)
	}

	if !sc.AboveThreshold {
		t.Error("CRITICAL severity should push finding above threshold")
	}

	if len(result.Salient) != 1 {
		t.Fatalf("expected 1 salient finding, got %d", len(result.Salient))
	}
}

func TestFilterSalience_MaxFindings(t *testing.T) {
	assessments := make([]*reasoningv1.CognitiveAssessment, 12)
	for i := range assessments {
		assessments[i] = &reasoningv1.CognitiveAssessment{
			FindingType:   reasoningv1.FindingType(1 + int32(i%4)), // rotate types for decent novelty
			Severity:      reasoningv1.FindingSeverity_WARNING,
			DetectorName:  "test-detector",
			Explanation:   "This is a detailed explanation that exceeds fifty characters easily for testing purposes.",
			RelevantTurns: []uint32{uint32(i)},
			Confidence:    0.8,
			Contradiction: &reasoningv1.ContradictionDetail{
				ClaimAText: "claim A",
				ClaimBText: "claim B",
			},
		}
	}

	cfg := DefaultSalienceConfig()
	result := FilterSalience(assessments, cfg)

	if len(result.Scores) != 12 {
		t.Fatalf("expected 12 scores, got %d", len(result.Scores))
	}

	if len(result.Salient) != 10 {
		t.Errorf("expected MaxFindings=10 salient, got %d", len(result.Salient))
	}
}

func TestFilterSalience_BelowThreshold(t *testing.T) {
	// 5 identical ANCHORING_BIAS with no evidence, no turns, short explanation, low confidence.
	assessments := make([]*reasoningv1.CognitiveAssessment, 5)
	for i := range assessments {
		assessments[i] = &reasoningv1.CognitiveAssessment{
			FindingType:  reasoningv1.FindingType_ANCHORING_BIAS,
			Severity:     reasoningv1.FindingSeverity_INFO,
			DetectorName: "anchoring-detector",
			Explanation:  "Short.",
			Confidence:   0.3,
		}
	}

	cfg := DefaultSalienceConfig()
	result := FilterSalience(assessments, cfg)

	// Novelty: 1/5 = 0.2, Actionability: 0.0, Severity: 0.25
	// Composite: 0.4*0.2 + 0.4*0.0 + 0.2*0.25 = 0.08 + 0.0 + 0.05 = 0.13
	for _, sc := range result.Scores {
		expected := 0.4*0.2 + 0.4*0.0 + 0.2*0.25
		if math.Abs(sc.Score-expected) > floatTol {
			t.Errorf("score: got %.4f, want %.4f", sc.Score, expected)
		}
		if sc.AboveThreshold {
			t.Error("expected below threshold")
		}
	}

	if len(result.Salient) != 0 {
		t.Errorf("expected 0 salient findings, got %d", len(result.Salient))
	}
}

func TestFilterSalience_Empty(t *testing.T) {
	cfg := DefaultSalienceConfig()

	// nil input
	result := FilterSalience(nil, cfg)
	if result == nil {
		t.Fatal("expected non-nil result for nil input")
	}
	if len(result.Scores) != 0 {
		t.Errorf("expected 0 scores for nil input, got %d", len(result.Scores))
	}
	if len(result.Salient) != 0 {
		t.Errorf("expected 0 salient for nil input, got %d", len(result.Salient))
	}

	// empty input
	result = FilterSalience([]*reasoningv1.CognitiveAssessment{}, cfg)
	if result == nil {
		t.Fatal("expected non-nil result for empty input")
	}
	if len(result.Scores) != 0 {
		t.Errorf("expected 0 scores for empty input, got %d", len(result.Scores))
	}
	if len(result.Salient) != 0 {
		t.Errorf("expected 0 salient for empty input, got %d", len(result.Salient))
	}
}
