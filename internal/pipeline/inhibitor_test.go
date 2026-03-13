package pipeline

import (
	"testing"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func makeAssessment(detector string, findingType reasoningv1.FindingType, severity reasoningv1.FindingSeverity, confidence float64, turns []uint32) *reasoningv1.CognitiveAssessment {
	return &reasoningv1.CognitiveAssessment{
		DetectorName:  detector,
		FindingType:   findingType,
		Severity:      severity,
		Confidence:    confidence,
		RelevantTurns: turns,
		Explanation:   "test finding",
	}
}

func makeSnap(turns []struct{ num uint32; speaker, text string }, objective string) *reasoningv1.ConversationSnapshot {
	snap := &reasoningv1.ConversationSnapshot{
		Objective:  objective,
		TotalTurns: uint32(len(turns)),
	}
	for _, t := range turns {
		snap.Turns = append(snap.Turns, &reasoningv1.Turn{
			TurnNumber: t.num,
			Speaker:    t.speaker,
			RawText:    t.text,
		})
	}
	return snap
}

// =============================================================================
// Gate 1: Casual hedging suppression (runs before severity auto-pass)
// =============================================================================

func TestGate1_CasualAbsolutelyInInformalContext(t *testing.T) {
	assessment := makeAssessment("confidence-calibrator",
		reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION,
		reasoningv1.FindingSeverity_WARNING,
		0.67, []uint32{2})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "user", "hey what do you think?"},
		{2, "assistant", "I'm absolutely sure we should go with React, it's awesome!"},
	}, "tech chat")

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment}, snap, DefaultInhibitorConfig())

	if result.Decisions[0].GetAction() != cerebrov1.InhibitionAction_INHIBITED {
		t.Error("casual 'absolutely' in informal context should be INHIBITED")
	}
	if result.Decisions[0].GetReason() != "casual_hedge_in_informal_context" {
		t.Errorf("expected reason casual_hedge_in_informal_context, got %s", result.Decisions[0].GetReason())
	}
}

func TestGate1_CriticalMiscalibrationWithCasualHedge(t *testing.T) {
	// KEY TEST: CRITICAL severity CONFIDENCE_MISCALIBRATION with casual hedge
	// should be INHIBITED — Gate 1 (casual hedge) runs before Gate 2 (severity auto-pass).
	// This is the fix for the 3 known FPs in convs 07/08.
	assessment := makeAssessment("confidence-calibrator",
		reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION,
		reasoningv1.FindingSeverity_CRITICAL,
		0.67, []uint32{2})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "user", "hey how should we handle logging?"},
		{2, "assistant", "Absolutely, structured logging is the way to go!"},
	}, "casual chat")

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment}, snap, DefaultInhibitorConfig())

	if result.Decisions[0].GetAction() != cerebrov1.InhibitionAction_INHIBITED {
		t.Errorf("CRITICAL CONFIDENCE_MISCALIBRATION with casual hedge should be INHIBITED, got %v (reason: %s)",
			result.Decisions[0].GetAction(), result.Decisions[0].GetReason())
	}
	if result.Decisions[0].GetReason() != "casual_hedge_in_informal_context" {
		t.Errorf("expected reason casual_hedge_in_informal_context, got %s", result.Decisions[0].GetReason())
	}
}

func TestGate1_AbsolutelyInFormalContextNotInhibited(t *testing.T) {
	assessment := makeAssessment("confidence-calibrator",
		reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION,
		reasoningv1.FindingSeverity_WARNING,
		0.8, []uint32{2})

	// Multiple long, formal turns to push formality above 0.85
	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "user", "According to the specification, what is the recommended approach for this requirement pursuant to the compliance framework?"},
		{2, "assistant", "I am absolutely certain this architecture will handle 10 million requests per second based on the analysis of the load testing data and it should be noted that the requirement mandates this throughput."},
		{3, "user", "Furthermore, the assessment should include considerations regarding scalability constraints in accordance with the performance baseline established by the engineering team."},
		{4, "assistant", "Consequently, I would suggest that the data suggests we need additional capacity planning. With respect to the specification, the requirement demands formal verification of all throughput claims."},
	}, "system architecture review")

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment}, snap, DefaultInhibitorConfig())

	// Formal context — gate 1 should not fire.
	if result.Decisions[0].GetReason() == "casual_hedge_in_informal_context" {
		t.Error("formal context should NOT trigger casual hedge suppression")
	}
}

func TestGate1_NonConfidenceFindingWithAbsolutelyNotAffected(t *testing.T) {
	assessment := makeAssessment("contradiction-tracker",
		reasoningv1.FindingType_CONTRADICTION,
		reasoningv1.FindingSeverity_WARNING,
		0.7, []uint32{2})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "user", "hey"},
		{2, "assistant", "absolutely, let's go with that"},
	}, "chat")

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment}, snap, DefaultInhibitorConfig())

	// Gate 1 only applies to CONFIDENCE_MISCALIBRATION.
	if result.Decisions[0].GetReason() == "casual_hedge_in_informal_context" {
		t.Error("gate 1 should not apply to non-CONFIDENCE_MISCALIBRATION findings")
	}
}

// =============================================================================
// Gate 2: Severity auto-pass
// =============================================================================

func TestGate2_CriticalAlwaysDiinhibits(t *testing.T) {
	assessment := makeAssessment("contradiction-tracker",
		reasoningv1.FindingType_CONTRADICTION,
		reasoningv1.FindingSeverity_CRITICAL,
		0.9, []uint32{1, 5})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "user", "hey lol"},
		{5, "assistant", "nah"},
	}, "casual chat")

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment}, snap, DefaultInhibitorConfig())

	if len(result.Decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(result.Decisions))
	}
	if result.Decisions[0].GetAction() != cerebrov1.InhibitionAction_DISINHIBITED {
		t.Errorf("CRITICAL should always disinhibit, got %v", result.Decisions[0].GetAction())
	}
	if result.Decisions[0].GetReason() != "severity_auto_pass" {
		t.Errorf("expected reason severity_auto_pass, got %s", result.Decisions[0].GetReason())
	}
}

func TestGate2_WarningProceedsToOtherGates(t *testing.T) {
	assessment := makeAssessment("scope-guard",
		reasoningv1.FindingType_SCOPE_DRIFT,
		reasoningv1.FindingSeverity_WARNING,
		0.85, []uint32{3})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{3, "assistant", "Furthermore, the specification requires careful analysis of the data."},
	}, "technical review")

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment}, snap, DefaultInhibitorConfig())

	// WARNING should NOT auto-pass; it must proceed through other gates.
	if result.Decisions[0].GetReason() == "severity_auto_pass" {
		t.Error("WARNING should not get severity_auto_pass")
	}
}

// =============================================================================
// Gate 3: Stakes gate
// =============================================================================

func TestGate3_LowUrgencyLowSeverity(t *testing.T) {
	assessment := makeAssessment("anchoring-detector",
		reasoningv1.FindingType_ANCHORING_BIAS,
		reasoningv1.FindingSeverity_CAUTION,
		0.5, []uint32{1, 2})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "user", "hey what color should the button be?"},
		{2, "assistant", "I think blue would look nice, yeah definitely blue"},
	}, "button color")

	// Default urgency stub is 0.5 which is above 0.3 threshold.
	// Use a config with high stakes_threshold to trigger gate 3.
	cfg := DefaultInhibitorConfig()
	cfg.StakesThreshold = 0.6 // Urgency 0.5 < 0.6

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment}, snap, cfg)

	if result.Decisions[0].GetAction() != cerebrov1.InhibitionAction_INHIBITED {
		t.Error("low urgency + CAUTION should be INHIBITED")
	}
	if result.Decisions[0].GetReason() != "low_stakes_low_severity" {
		t.Errorf("expected reason low_stakes_low_severity, got %s", result.Decisions[0].GetReason())
	}
}

func TestGate3_LowUrgencyWarningSeverityProceeds(t *testing.T) {
	assessment := makeAssessment("scope-guard",
		reasoningv1.FindingType_SCOPE_DRIFT,
		reasoningv1.FindingSeverity_WARNING,
		0.8, []uint32{5})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{5, "assistant", "hey let's talk about something else"},
	}, "chat")

	cfg := DefaultInhibitorConfig()
	cfg.StakesThreshold = 0.6

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment}, snap, cfg)

	// WARNING should NOT be caught by gate 3 (gate 3 only catches CAUTION and below).
	if result.Decisions[0].GetReason() == "low_stakes_low_severity" {
		t.Error("WARNING severity should not be caught by stakes gate")
	}
}

// =============================================================================
// Gate 4: Confidence gate
// =============================================================================

func TestGate4_WarningBelowConfidenceThreshold(t *testing.T) {
	assessment := makeAssessment("scope-guard",
		reasoningv1.FindingType_SCOPE_DRIFT,
		reasoningv1.FindingSeverity_WARNING,
		0.5, []uint32{3})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{3, "assistant", "According to the specification, we should examine the data carefully."},
	}, "analysis")

	// Need to also have corroboration pass or the finding dies at gate 5.
	// Give it another finding on same turn for corroboration.
	other := makeAssessment("contradiction-tracker",
		reasoningv1.FindingType_CONTRADICTION,
		reasoningv1.FindingSeverity_WARNING,
		0.8, []uint32{3})

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment, other}, snap, DefaultInhibitorConfig())

	// Find the scope-guard decision.
	var sgDecision *cerebrov1.InhibitionDecision
	for _, d := range result.Decisions {
		if d.GetDetectorName() == "scope-guard" {
			sgDecision = d
			break
		}
	}
	if sgDecision == nil {
		t.Fatal("no decision for scope-guard")
	}

	if sgDecision.GetAction() != cerebrov1.InhibitionAction_INHIBITED {
		t.Error("WARNING with confidence 0.5 should be INHIBITED by gate 4")
	}
	if sgDecision.GetReason() != "warning_below_confidence_threshold" {
		t.Errorf("expected reason warning_below_confidence_threshold, got %s", sgDecision.GetReason())
	}
}

func TestGate4_WarningAboveConfidenceProceeds(t *testing.T) {
	assessment := makeAssessment("scope-guard",
		reasoningv1.FindingType_SCOPE_DRIFT,
		reasoningv1.FindingSeverity_WARNING,
		0.8, []uint32{3})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{3, "assistant", "The specification recommends a different approach based on the analysis."},
	}, "review")

	// Give corroboration.
	other := makeAssessment("contradiction-tracker",
		reasoningv1.FindingType_CONTRADICTION,
		reasoningv1.FindingSeverity_WARNING,
		0.8, []uint32{3})

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment, other}, snap, DefaultInhibitorConfig())

	var sgDecision *cerebrov1.InhibitionDecision
	for _, d := range result.Decisions {
		if d.GetDetectorName() == "scope-guard" {
			sgDecision = d
			break
		}
	}
	if sgDecision == nil {
		t.Fatal("no decision for scope-guard")
	}

	if sgDecision.GetReason() == "warning_below_confidence_threshold" {
		t.Error("WARNING with confidence 0.8 should not be caught by gate 4")
	}
}

func TestGate4_CautionNotAffected(t *testing.T) {
	assessment := makeAssessment("anchoring-detector",
		reasoningv1.FindingType_ANCHORING_BIAS,
		reasoningv1.FindingSeverity_CAUTION,
		0.5, []uint32{1})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "assistant", "Based on the specification, the recommended approach is the following."},
	}, "analysis")

	cfg := DefaultInhibitorConfig()
	cfg.StakesThreshold = 0.0 // Disable gate 3 so we can test gate 4

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment}, snap, cfg)

	// Gate 4 only applies to WARNING severity, not CAUTION.
	if result.Decisions[0].GetReason() == "warning_below_confidence_threshold" {
		t.Error("CAUTION should not be affected by gate 4")
	}
}

// =============================================================================
// Gate 5: Corroboration gate
// =============================================================================

func TestGate5_NoCorroboration(t *testing.T) {
	// Single finding, no other detectors fired on nearby turns.
	assessment := makeAssessment("anchoring-detector",
		reasoningv1.FindingType_ANCHORING_BIAS,
		reasoningv1.FindingSeverity_WARNING,
		0.75, []uint32{1}) // Above confidence threshold so Gate 4 doesn't catch it
	other := makeAssessment("scope-guard",
		reasoningv1.FindingType_SCOPE_DRIFT,
		reasoningv1.FindingSeverity_WARNING,
		0.8, []uint32{10}) // Far away turn

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "assistant", "Based on the analysis, the estimate is 15 months."},
		{10, "assistant", "Let us discuss the organizational structure instead."},
	}, "project planning")

	cfg := DefaultInhibitorConfig()
	cfg.StakesThreshold = 0.0 // Disable gate 3

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment, other}, snap, cfg)

	var anchDecision *cerebrov1.InhibitionDecision
	for _, d := range result.Decisions {
		if d.GetDetectorName() == "anchoring-detector" {
			anchDecision = d
			break
		}
	}
	if anchDecision == nil {
		t.Fatal("no decision for anchoring-detector")
	}

	if anchDecision.GetAction() != cerebrov1.InhibitionAction_INHIBITED {
		t.Error("finding with no corroboration should be INHIBITED")
	}
	if anchDecision.GetReason() != "insufficient_corroboration" {
		t.Errorf("expected reason insufficient_corroboration, got %s", anchDecision.GetReason())
	}
}

func TestGate5_WithCorroboration(t *testing.T) {
	// Two detectors flag the same turn.
	assessment := makeAssessment("anchoring-detector",
		reasoningv1.FindingType_ANCHORING_BIAS,
		reasoningv1.FindingSeverity_WARNING,
		0.8, []uint32{3})
	other := makeAssessment("scope-guard",
		reasoningv1.FindingType_SCOPE_DRIFT,
		reasoningv1.FindingSeverity_WARNING,
		0.8, []uint32{3})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{3, "assistant", "Based on the specification, the estimate matches the earlier anchor point and we have drifted from the original topic."},
	}, "project estimation")

	cfg := DefaultInhibitorConfig()
	cfg.StakesThreshold = 0.0

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment, other}, snap, cfg)

	var anchDecision *cerebrov1.InhibitionDecision
	for _, d := range result.Decisions {
		if d.GetDetectorName() == "anchoring-detector" {
			anchDecision = d
			break
		}
	}
	if anchDecision == nil {
		t.Fatal("no decision for anchoring-detector")
	}

	if anchDecision.GetAction() != cerebrov1.InhibitionAction_DISINHIBITED {
		t.Error("corroborated finding should be DISINHIBITED")
	}
}

func TestGate5_HighConfidenceSoloException(t *testing.T) {
	// High confidence (>0.9) solo finding should pass despite no corroboration.
	assessment := makeAssessment("anchoring-detector",
		reasoningv1.FindingType_ANCHORING_BIAS,
		reasoningv1.FindingSeverity_WARNING,
		0.95, []uint32{1})
	other := makeAssessment("scope-guard",
		reasoningv1.FindingType_SCOPE_DRIFT,
		reasoningv1.FindingSeverity_WARNING,
		0.8, []uint32{10})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "assistant", "Based on the specification, the analysis shows strong anchoring."},
		{10, "assistant", "In accordance with the requirements, the scope has shifted."},
	}, "analysis")

	cfg := DefaultInhibitorConfig()
	cfg.StakesThreshold = 0.0

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment, other}, snap, cfg)

	var anchDecision *cerebrov1.InhibitionDecision
	for _, d := range result.Decisions {
		if d.GetDetectorName() == "anchoring-detector" {
			anchDecision = d
			break
		}
	}
	if anchDecision == nil {
		t.Fatal("no decision for anchoring-detector")
	}

	if anchDecision.GetAction() != cerebrov1.InhibitionAction_DISINHIBITED {
		t.Errorf("high-confidence (0.95) solo finding should be DISINHIBITED, got %v reason=%s",
			anchDecision.GetAction(), anchDecision.GetReason())
	}
}

func TestGate5_OnlyOneDetectorActive(t *testing.T) {
	// Only one detector active — corroboration should be 1.0 (can't require from nonexistent).
	assessment := makeAssessment("scope-guard",
		reasoningv1.FindingType_SCOPE_DRIFT,
		reasoningv1.FindingSeverity_WARNING,
		0.8, []uint32{3})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{3, "assistant", "According to the specification, we should proceed with this approach."},
	}, "review")

	cfg := DefaultInhibitorConfig()
	cfg.StakesThreshold = 0.0

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment}, snap, cfg)

	if result.Decisions[0].GetAction() != cerebrov1.InhibitionAction_DISINHIBITED {
		t.Errorf("sole detector should not need corroboration, got %v reason=%s",
			result.Decisions[0].GetAction(), result.Decisions[0].GetReason())
	}
}

// =============================================================================
// Formality computation
// =============================================================================

func TestComputeFormality_Informal(t *testing.T) {
	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "user", "hey what do you think?"},
		{2, "assistant", "yeah I think it's awesome! let's go with that lol"},
	}, "chat")

	f := ComputeFormality(snap)
	if f >= 0.3 {
		t.Errorf("informal conversation should have formality < 0.3, got %.2f", f)
	}
}

func TestComputeFormality_Formal(t *testing.T) {
	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "user", "According to the specification, what is the recommended approach for this requirement in accordance with industry standards?"},
		{2, "assistant", "Based on the analysis of the data, it should be noted that the recommended approach requires careful consideration of multiple factors pursuant to the established framework."},
	}, "technical review")

	f := ComputeFormality(snap)
	if f <= 0.5 {
		t.Errorf("formal conversation should have formality > 0.5, got %.2f", f)
	}
}

// =============================================================================
// Integration: pipeline with inhibitor
// =============================================================================

func TestInhibitorIntegration_GatedFindingsOnly(t *testing.T) {
	// Create a scenario with one CRITICAL and one low-confidence WARNING.
	critical := makeAssessment("contradiction-tracker",
		reasoningv1.FindingType_CONTRADICTION,
		reasoningv1.FindingSeverity_CRITICAL,
		0.9, []uint32{1, 5})
	lowConf := makeAssessment("anchoring-detector",
		reasoningv1.FindingType_ANCHORING_BIAS,
		reasoningv1.FindingSeverity_WARNING,
		0.5, []uint32{8})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "assistant", "According to the specification, we should use PostgreSQL."},
		{5, "assistant", "Based on the analysis, we should not use PostgreSQL."},
		{8, "assistant", "The estimate is around 15 months."},
	}, "database selection")

	result := Inhibit([]*reasoningv1.CognitiveAssessment{critical, lowConf}, snap, DefaultInhibitorConfig())

	// CRITICAL should pass, low-confidence WARNING should be inhibited.
	if len(result.Gated) != 1 {
		t.Fatalf("expected 1 gated finding, got %d", len(result.Gated))
	}
	if result.Gated[0].GetFindingType() != reasoningv1.FindingType_CONTRADICTION {
		t.Error("only CRITICAL contradiction should pass through")
	}
}

func TestInhibitorEmptyFindings(t *testing.T) {
	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "user", "hello"},
	}, "greeting")

	result := Inhibit(nil, snap, DefaultInhibitorConfig())

	if len(result.Decisions) != 0 {
		t.Errorf("empty findings should produce 0 decisions, got %d", len(result.Decisions))
	}
	if len(result.Gated) != 0 {
		t.Errorf("empty findings should produce 0 gated, got %d", len(result.Gated))
	}
}
