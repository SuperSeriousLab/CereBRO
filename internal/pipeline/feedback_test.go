package pipeline

import (
	"math"
	"testing"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// TestEvaluateFeedback_HighConfidenceSkip verifies no re-evaluation when confidence is high.
func TestEvaluateFeedback_HighConfidenceSkip(t *testing.T) {
	findings := []*reasoningv1.CognitiveAssessment{
		{Confidence: 0.8, DetectorName: "scope-guard", FindingType: reasoningv1.FindingType_SCOPE_DRIFT},
	}
	selfConf := &cerebrov1.SelfConfidenceReport{OverallConfidence: 0.9}
	report := &reasoningv1.ReasoningReport{}
	cfg := DefaultFeedbackConfig()

	result, fbResult := EvaluateFeedback(findings, selfConf, nil, report, cfg, nil)
	if fbResult.Applied {
		t.Error("expected no feedback for high confidence")
	}
	if len(result) != 1 {
		t.Errorf("expected 1 finding unchanged, got %d", len(result))
	}
}

// TestEvaluateFeedback_LowConfidenceTriggers verifies re-evaluation triggers below threshold.
func TestEvaluateFeedback_LowConfidenceTriggers(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Objective: "test",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "We should use React."},
			{TurnNumber: 2, Speaker: "assistant", RawText: "React is great for this."},
		},
		TotalTurns: 2,
	}

	findings := []*reasoningv1.CognitiveAssessment{
		{Confidence: 0.3, DetectorName: "scope-guard", FindingType: reasoningv1.FindingType_SCOPE_DRIFT, RelevantTurns: []uint32{1, 2}},
	}
	selfConf := &cerebrov1.SelfConfidenceReport{OverallConfidence: 0.4} // below 0.6 threshold
	report := &reasoningv1.ReasoningReport{Findings: findings}
	cfg := DefaultFeedbackConfig()

	// Create a detector that returns a higher-confidence finding on re-eval.
	detectors := map[Detector]DetectorFunc{
		"scope-guard": func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
			return &reasoningv1.CognitiveAssessment{
				Confidence:   0.5, // higher than original 0.3
				DetectorName: "scope-guard",
				FindingType:  reasoningv1.FindingType_SCOPE_DRIFT,
				RelevantTurns: []uint32{1, 2},
			}
		},
	}

	_, fbResult := EvaluateFeedback(findings, selfConf, snap, report, cfg, detectors)
	if len(fbResult.ReevalDetectors) == 0 {
		t.Error("expected at least one detector to be re-evaluated")
	}
	if len(fbResult.ReevalDetectors) > 0 && fbResult.ReevalDetectors[0] != "scope-guard" {
		t.Errorf("expected scope-guard re-evaluated, got %s", fbResult.ReevalDetectors[0])
	}
}

// TestEvaluateFeedback_SmallDeltaRejected verifies changes below threshold are rejected.
func TestEvaluateFeedback_SmallDeltaRejected(t *testing.T) {
	findings := []*reasoningv1.CognitiveAssessment{
		{Confidence: 0.5, DetectorName: "scope-guard", FindingType: reasoningv1.FindingType_SCOPE_DRIFT, RelevantTurns: []uint32{1}},
	}
	selfConf := &cerebrov1.SelfConfidenceReport{OverallConfidence: 0.4}
	report := &reasoningv1.ReasoningReport{Findings: findings}
	cfg := DefaultFeedbackConfig()

	// Detector returns nearly the same confidence (delta < 0.1).
	detectors := map[Detector]DetectorFunc{
		"scope-guard": func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
			return &reasoningv1.CognitiveAssessment{
				Confidence:   0.52, // delta=0.02, below min 0.1
				DetectorName: "scope-guard",
				FindingType:  reasoningv1.FindingType_SCOPE_DRIFT,
				RelevantTurns: []uint32{1},
			}
		},
	}

	result, fbResult := EvaluateFeedback(findings, selfConf, nil, report, cfg, detectors)
	if fbResult.Applied {
		t.Error("expected feedback NOT applied for small delta")
	}
	if len(result) != 1 {
		t.Errorf("expected original finding preserved, got %d", len(result))
	}
}

// TestEvaluateFeedback_DisappearedFinding verifies removal when detector returns nil on re-eval.
func TestEvaluateFeedback_DisappearedFinding(t *testing.T) {
	findings := []*reasoningv1.CognitiveAssessment{
		{Confidence: 0.4, DetectorName: "scope-guard", FindingType: reasoningv1.FindingType_SCOPE_DRIFT, RelevantTurns: []uint32{1}},
	}
	selfConf := &cerebrov1.SelfConfidenceReport{OverallConfidence: 0.3}
	report := &reasoningv1.ReasoningReport{Findings: findings}
	cfg := DefaultFeedbackConfig()

	// Detector returns nil (finding disappeared on re-eval).
	detectors := map[Detector]DetectorFunc{
		"scope-guard": func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
			return nil
		},
	}

	result, fbResult := EvaluateFeedback(findings, selfConf, nil, report, cfg, detectors)
	if !fbResult.Applied {
		t.Error("expected feedback applied for disappeared finding")
	}
	// Finding should be filtered out (confidence set to 0 → filtered).
	if len(result) != 0 {
		t.Errorf("expected 0 findings after disappearance, got %d", len(result))
	}
}

// TestEvaluateFeedback_MaxReevalLimit verifies only MaxReevalFindings are re-evaluated.
func TestEvaluateFeedback_MaxReevalLimit(t *testing.T) {
	findings := []*reasoningv1.CognitiveAssessment{
		{Confidence: 0.3, DetectorName: "scope-guard", FindingType: reasoningv1.FindingType_SCOPE_DRIFT, RelevantTurns: []uint32{1}},
		{Confidence: 0.2, DetectorName: "contradiction-tracker", FindingType: reasoningv1.FindingType_CONTRADICTION, RelevantTurns: []uint32{2}},
		{Confidence: 0.15, DetectorName: "anchoring-detector", FindingType: reasoningv1.FindingType_ANCHORING_BIAS, RelevantTurns: []uint32{3}},
		{Confidence: 0.4, DetectorName: "confidence-calibrator", FindingType: reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION, RelevantTurns: []uint32{4}},
		{Confidence: 0.5, DetectorName: "sunk-cost-detector", FindingType: reasoningv1.FindingType_SUNK_COST_FALLACY, RelevantTurns: []uint32{5}},
	}
	selfConf := &cerebrov1.SelfConfidenceReport{OverallConfidence: 0.3}
	report := &reasoningv1.ReasoningReport{Findings: findings}
	cfg := DefaultFeedbackConfig()
	cfg.MaxReevalFindings = 2

	// All detectors return nil (disappear).
	detectors := map[Detector]DetectorFunc{
		"scope-guard":            func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment { return nil },
		"contradiction-tracker":  func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment { return nil },
		"anchoring-detector":     func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment { return nil },
		"confidence-calibrator":  func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment { return nil },
		"sunk-cost-detector":     func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment { return nil },
	}

	_, fbResult := EvaluateFeedback(findings, selfConf, nil, report, cfg, detectors)
	// Only 2 should be re-evaluated (the lowest-confidence ones).
	if len(fbResult.ReevalDetectors) != 2 {
		t.Errorf("expected 2 re-evaluated detectors, got %d: %v", len(fbResult.ReevalDetectors), fbResult.ReevalDetectors)
	}
}

// TestEvaluateFeedback_EmptyFindings verifies no crash with empty findings.
func TestEvaluateFeedback_EmptyFindings(t *testing.T) {
	selfConf := &cerebrov1.SelfConfidenceReport{OverallConfidence: 0.3}
	report := &reasoningv1.ReasoningReport{}
	cfg := DefaultFeedbackConfig()

	result, fbResult := EvaluateFeedback(nil, selfConf, nil, report, cfg, nil)
	if fbResult.Applied {
		t.Error("expected no feedback for empty findings")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result))
	}
}

// =============================================================================
// Feedback adjustment tests (per-detector corroboration logic)
// =============================================================================

// TestFeedbackAdjustment_ContradictionWithScopeGuard tests +0.1 corroboration.
func TestFeedbackAdjustment_ContradictionWithScopeGuard(t *testing.T) {
	finding := &reasoningv1.CognitiveAssessment{
		Confidence:    0.6,
		DetectorName:  "contradiction-tracker",
		FindingType:   reasoningv1.FindingType_CONTRADICTION,
		RelevantTurns: []uint32{3, 4},
	}
	ctx := &FeedbackContext{
		PassNumber: 2,
		PeerFindings: []*reasoningv1.CognitiveAssessment{
			{DetectorName: "scope-guard", FindingType: reasoningv1.FindingType_SCOPE_DRIFT, RelevantTurns: []uint32{3, 5}, Confidence: 0.7},
		},
	}

	result := applyFeedbackAdjustment(finding, ctx)
	expected := 0.7 // 0.6 + 0.1 (scope-guard corroboration)
	if math.Abs(result.GetConfidence()-expected) > 0.01 {
		t.Errorf("expected confidence %.2f, got %.2f", expected, result.GetConfidence())
	}
}

// TestFeedbackAdjustment_ContradictionNoCorroboration tests -0.1 penalty.
func TestFeedbackAdjustment_ContradictionNoCorroboration(t *testing.T) {
	finding := &reasoningv1.CognitiveAssessment{
		Confidence:    0.6,
		DetectorName:  "contradiction-tracker",
		FindingType:   reasoningv1.FindingType_CONTRADICTION,
		RelevantTurns: []uint32{3, 4},
	}
	ctx := &FeedbackContext{
		PassNumber: 2,
		PeerFindings: []*reasoningv1.CognitiveAssessment{
			// Different turns — no overlap
			{DetectorName: "scope-guard", FindingType: reasoningv1.FindingType_SCOPE_DRIFT, RelevantTurns: []uint32{10, 11}, Confidence: 0.7},
		},
	}

	result := applyFeedbackAdjustment(finding, ctx)
	expected := 0.5 // 0.6 - 0.1 (no overlapping peers)
	if math.Abs(result.GetConfidence()-expected) > 0.01 {
		t.Errorf("expected confidence %.2f, got %.2f", expected, result.GetConfidence())
	}
}

// TestFeedbackAdjustment_ScopeGuardWithContradiction tests +0.1 without penalty.
func TestFeedbackAdjustment_ScopeGuardWithContradiction(t *testing.T) {
	finding := &reasoningv1.CognitiveAssessment{
		Confidence:    0.7,
		DetectorName:  "scope-guard",
		FindingType:   reasoningv1.FindingType_SCOPE_DRIFT,
		RelevantTurns: []uint32{3, 4},
	}
	ctx := &FeedbackContext{
		PassNumber: 2,
		PeerFindings: []*reasoningv1.CognitiveAssessment{
			{DetectorName: "contradiction-tracker", FindingType: reasoningv1.FindingType_CONTRADICTION, RelevantTurns: []uint32{4, 5}, Confidence: 0.6},
		},
	}

	result := applyFeedbackAdjustment(finding, ctx)
	expected := 0.8 // 0.7 + 0.1 (contradiction corroboration)
	if math.Abs(result.GetConfidence()-expected) > 0.01 {
		t.Errorf("expected confidence %.2f, got %.2f", expected, result.GetConfidence())
	}
}

// TestFeedbackAdjustment_ScopeGuardNoPenalty verifies no decrease without corroboration.
func TestFeedbackAdjustment_ScopeGuardNoPenalty(t *testing.T) {
	finding := &reasoningv1.CognitiveAssessment{
		Confidence:    0.7,
		DetectorName:  "scope-guard",
		FindingType:   reasoningv1.FindingType_SCOPE_DRIFT,
		RelevantTurns: []uint32{3, 4},
	}
	ctx := &FeedbackContext{
		PassNumber:   2,
		PeerFindings: []*reasoningv1.CognitiveAssessment{}, // no peers
	}

	result := applyFeedbackAdjustment(finding, ctx)
	if math.Abs(result.GetConfidence()-0.7) > 0.01 {
		t.Errorf("expected no change for scope-guard without corroboration, got %.2f", result.GetConfidence())
	}
}

// TestFeedbackAdjustment_SunkCostWithLedger tests +0.15 corroboration.
func TestFeedbackAdjustment_SunkCostWithLedger(t *testing.T) {
	finding := &reasoningv1.CognitiveAssessment{
		Confidence:    0.5,
		DetectorName:  "sunk-cost-detector",
		FindingType:   reasoningv1.FindingType_SUNK_COST_FALLACY,
		RelevantTurns: []uint32{2, 3},
	}
	ctx := &FeedbackContext{
		PassNumber: 2,
		PeerFindings: []*reasoningv1.CognitiveAssessment{
			{DetectorName: "decision-ledger", FindingType: reasoningv1.FindingType_SILENT_REVISION, RelevantTurns: []uint32{3}, Confidence: 0.6},
		},
	}

	result := applyFeedbackAdjustment(finding, ctx)
	expected := 0.65 // 0.5 + 0.15
	if math.Abs(result.GetConfidence()-expected) > 0.01 {
		t.Errorf("expected confidence %.2f, got %.2f", expected, result.GetConfidence())
	}
}

// TestFeedbackAdjustment_ClampTo1 verifies confidence doesn't exceed 1.0.
func TestFeedbackAdjustment_ClampTo1(t *testing.T) {
	finding := &reasoningv1.CognitiveAssessment{
		Confidence:    0.95,
		DetectorName:  "contradiction-tracker",
		FindingType:   reasoningv1.FindingType_CONTRADICTION,
		RelevantTurns: []uint32{1},
	}
	ctx := &FeedbackContext{
		PassNumber: 2,
		PeerFindings: []*reasoningv1.CognitiveAssessment{
			{DetectorName: "scope-guard", FindingType: reasoningv1.FindingType_SCOPE_DRIFT, RelevantTurns: []uint32{1}, Confidence: 0.8},
		},
	}

	result := applyFeedbackAdjustment(finding, ctx)
	if result.GetConfidence() > 1.0 {
		t.Errorf("expected clamped to 1.0, got %.2f", result.GetConfidence())
	}
}

// TestFeedbackAdjustment_NilContext verifies no-op when context is nil.
func TestFeedbackAdjustment_NilContext(t *testing.T) {
	finding := &reasoningv1.CognitiveAssessment{
		Confidence:   0.6,
		DetectorName: "scope-guard",
	}
	result := applyFeedbackAdjustment(finding, nil)
	if result.GetConfidence() != 0.6 {
		t.Errorf("expected unchanged confidence, got %.2f", result.GetConfidence())
	}
}

// TestFindOverlappingPeers_TurnWindow verifies ±1 turn overlap detection.
func TestFindOverlappingPeers_TurnWindow(t *testing.T) {
	finding := &reasoningv1.CognitiveAssessment{
		DetectorName:  "scope-guard",
		RelevantTurns: []uint32{5},
	}

	// Peer at turn 6 (within +1) → should overlap
	peers := []*reasoningv1.CognitiveAssessment{
		{DetectorName: "contradiction-tracker", RelevantTurns: []uint32{6}},
	}
	overlapping := findOverlappingPeers(finding, peers)
	if len(overlapping) != 1 {
		t.Errorf("expected 1 overlapping peer (turn 6 within +1 of turn 5), got %d", len(overlapping))
	}

	// Peer at turn 8 (outside ±1) → should not overlap
	peers2 := []*reasoningv1.CognitiveAssessment{
		{DetectorName: "contradiction-tracker", RelevantTurns: []uint32{8}},
	}
	overlapping2 := findOverlappingPeers(finding, peers2)
	if len(overlapping2) != 0 {
		t.Errorf("expected 0 overlapping peers (turn 8 outside ±1 of turn 5), got %d", len(overlapping2))
	}

	// Self should be excluded
	peers3 := []*reasoningv1.CognitiveAssessment{
		{DetectorName: "scope-guard", RelevantTurns: []uint32{5}},
	}
	overlapping3 := findOverlappingPeers(finding, peers3)
	if len(overlapping3) != 0 {
		t.Errorf("expected 0 (self excluded), got %d", len(overlapping3))
	}
}

// TestFindOverlappingPeers_TurnZero verifies no uint32 underflow when turn=0.
func TestFindOverlappingPeers_TurnZero(t *testing.T) {
	finding := &reasoningv1.CognitiveAssessment{
		DetectorName:  "scope-guard",
		RelevantTurns: []uint32{0},
	}
	peers := []*reasoningv1.CognitiveAssessment{
		{DetectorName: "contradiction-tracker", RelevantTurns: []uint32{0}},
	}
	// Should match (exact turn) without wrapping t-1 to maxuint32.
	overlapping := findOverlappingPeers(finding, peers)
	if len(overlapping) != 1 {
		t.Errorf("expected 1 overlapping peer at turn 0, got %d", len(overlapping))
	}

	// Peer at turn 1 should overlap (within +1 of turn 0).
	peers2 := []*reasoningv1.CognitiveAssessment{
		{DetectorName: "contradiction-tracker", RelevantTurns: []uint32{1}},
	}
	overlapping2 := findOverlappingPeers(finding, peers2)
	if len(overlapping2) != 1 {
		t.Errorf("expected 1 overlapping peer (turn 1 within +1 of turn 0), got %d", len(overlapping2))
	}

	// Peer at turn 3 should NOT overlap.
	peers3 := []*reasoningv1.CognitiveAssessment{
		{DetectorName: "contradiction-tracker", RelevantTurns: []uint32{3}},
	}
	overlapping3 := findOverlappingPeers(finding, peers3)
	if len(overlapping3) != 0 {
		t.Errorf("expected 0 overlapping peers (turn 3 far from turn 0), got %d", len(overlapping3))
	}
}
