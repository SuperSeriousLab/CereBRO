package main

import (
	"testing"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func TestRun_CriticalDiinhibits(t *testing.T) {
	assessment := &reasoningv1.CognitiveAssessment{
		DetectorName:  "contradiction-tracker",
		FindingType:   reasoningv1.FindingType_CONTRADICTION,
		Severity:      reasoningv1.FindingSeverity_CRITICAL,
		Confidence:    0.9,
		RelevantTurns: []uint32{1, 5},
		Explanation:   "test",
	}
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "hey"},
			{TurnNumber: 5, Speaker: "assistant", RawText: "yo"},
		},
		TotalTurns: 5,
	}

	result := Run([]*reasoningv1.CognitiveAssessment{assessment}, snap, DefaultConfig())

	if len(result.Decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(result.Decisions))
	}
	if result.Decisions[0].GetAction() != cerebrov1.InhibitionAction_DISINHIBITED {
		t.Error("CRITICAL should disinhibit")
	}
	if len(result.Gated) != 1 {
		t.Errorf("expected 1 gated finding, got %d", len(result.Gated))
	}
}

func TestRun_CasualHedgeInhibits(t *testing.T) {
	assessment := &reasoningv1.CognitiveAssessment{
		DetectorName:  "confidence-calibrator",
		FindingType:   reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION,
		Severity:      reasoningv1.FindingSeverity_CRITICAL,
		Confidence:    0.67,
		RelevantTurns: []uint32{2},
		Explanation:   "test",
	}
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "hey what do you think?"},
			{TurnNumber: 2, Speaker: "assistant", RawText: "Absolutely, we should go with that approach!"},
		},
		TotalTurns: 2,
	}

	result := Run([]*reasoningv1.CognitiveAssessment{assessment}, snap, DefaultConfig())

	if len(result.Decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(result.Decisions))
	}
	if result.Decisions[0].GetAction() != cerebrov1.InhibitionAction_INHIBITED {
		t.Error("casual hedge CONFIDENCE_MISCALIBRATION should be INHIBITED")
	}
	if len(result.Gated) != 0 {
		t.Errorf("expected 0 gated findings, got %d", len(result.Gated))
	}
}

func TestRunWithGain_HighUrgency(t *testing.T) {
	assessment := &reasoningv1.CognitiveAssessment{
		DetectorName:  "contradiction-tracker",
		FindingType:   reasoningv1.FindingType_CONTRADICTION,
		Severity:      reasoningv1.FindingSeverity_CRITICAL,
		Confidence:    0.9,
		RelevantTurns: []uint32{1, 5},
		Explanation:   "test",
	}
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "hey"},
			{TurnNumber: 5, Speaker: "assistant", RawText: "yo"},
		},
		TotalTurns: 5,
	}

	gain := &GainSignal{
		Urgency:    0.9,
		Formality:  0.8,
		Complexity: 0.5,
		Mode:       cerebrov1.GainMode_PHASIC,
	}

	result := RunWithGain([]*reasoningv1.CognitiveAssessment{assessment}, snap, DefaultConfig(), gain)

	if len(result.Decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(result.Decisions))
	}
	if result.Decisions[0].GetAction() != cerebrov1.InhibitionAction_DISINHIBITED {
		t.Error("CRITICAL should disinhibit even with high urgency GainSignal")
	}
	if result.Urgency != 0.9 {
		t.Errorf("expected urgency 0.9 from GainSignal, got %.2f", result.Urgency)
	}
}

func TestRun_EmptyInput(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{TotalTurns: 0}
	result := Run(nil, snap, DefaultConfig())

	if len(result.Decisions) != 0 {
		t.Errorf("expected 0 decisions for empty input, got %d", len(result.Decisions))
	}
	if len(result.Gated) != 0 {
		t.Errorf("expected 0 gated for empty input, got %d", len(result.Gated))
	}
}
