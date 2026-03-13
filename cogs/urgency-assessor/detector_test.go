package main

import (
	"testing"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func TestRun_HighStakes(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Objective: "security review",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "We found a critical security vulnerability."},
			{TurnNumber: 2, Speaker: "assistant", RawText: "This is urgent. The risk to safety is high."},
		},
		TotalTurns: 2,
	}

	gain := Run(snap, DefaultConfig())

	if gain.Urgency < 0.6 {
		t.Errorf("expected urgency > 0.6 for high-stakes, got %.2f", gain.Urgency)
	}
	if gain.Mode != cerebrov1.GainMode_PHASIC {
		t.Errorf("expected PHASIC mode, got %v", gain.Mode)
	}
}

func TestRun_Casual(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Objective: "button color",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Hey, what color should the button be?"},
		},
		TotalTurns: 1,
	}

	gain := Run(snap, DefaultConfig())

	if gain.Urgency > 0.3 {
		t.Errorf("expected urgency < 0.3 for casual, got %.2f", gain.Urgency)
	}
	if gain.Mode != cerebrov1.GainMode_TONIC {
		t.Errorf("expected TONIC mode, got %v", gain.Mode)
	}
}

func TestRun_NilSnapshot(t *testing.T) {
	gain := Run(nil, DefaultConfig())

	if gain.Urgency != 0.5 {
		t.Errorf("expected 0.5 for nil snapshot, got %.2f", gain.Urgency)
	}
}
