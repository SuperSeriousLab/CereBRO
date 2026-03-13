package pipeline

import (
	"math"
	"testing"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// =============================================================================
// Urgency Assessor tests
// =============================================================================

func TestUrgency_HighStakesConversation(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Objective: "security review",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "We found a critical security vulnerability in the payment system."},
			{TurnNumber: 2, Speaker: "assistant", RawText: "This is urgent. The risk to patient safety and security is high."},
			{TurnNumber: 3, Speaker: "user", RawText: "We need this fixed before the deadline tomorrow. The liability is enormous."},
		},
		TotalTurns: 3,
	}

	gain := AssessUrgency(snap, DefaultUrgencyConfig())

	if gain.Urgency < 0.6 {
		t.Errorf("expected urgency > 0.6 for high-stakes conversation, got %.2f", gain.Urgency)
	}
	if gain.Mode != cerebrov1.GainMode_PHASIC {
		t.Errorf("expected PHASIC mode for high urgency, got %v", gain.Mode)
	}
}

func TestUrgency_CasualConversation(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Objective: "button color",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Hey, what color should the button be?"},
			{TurnNumber: 2, Speaker: "assistant", RawText: "I think blue would look nice, yeah!"},
		},
		TotalTurns: 2,
	}

	gain := AssessUrgency(snap, DefaultUrgencyConfig())

	if gain.Urgency > 0.3 {
		t.Errorf("expected urgency < 0.3 for casual conversation (baseline + no keywords), got %.2f", gain.Urgency)
	}
	if gain.Formality > 0.4 {
		t.Errorf("expected formality < 0.4 for casual conversation, got %.2f", gain.Formality)
	}
	if gain.Mode != cerebrov1.GainMode_TONIC {
		t.Errorf("expected TONIC mode for low urgency, got %v", gain.Mode)
	}
}

func TestUrgency_LongTechnicalNoUrgency(t *testing.T) {
	// Build a 20-turn technical discussion with no urgency keywords.
	var turns []*reasoningv1.Turn
	for i := uint32(1); i <= 20; i++ {
		speaker := "user"
		if i%2 == 0 {
			speaker = "assistant"
		}
		turns = append(turns, &reasoningv1.Turn{
			TurnNumber: i,
			Speaker:    speaker,
			RawText:    "The implementation of the recursive descent parser requires careful consideration of the grammar rules and precedence levels for proper expression evaluation.",
		})
	}
	snap := &reasoningv1.ConversationSnapshot{
		Objective:  "parser design",
		Turns:      turns,
		TotalTurns: 20,
	}

	gain := AssessUrgency(snap, DefaultUrgencyConfig())

	if gain.Urgency > 0.3 {
		t.Errorf("expected urgency < 0.3 for non-urgent discussion (baseline only), got %.2f", gain.Urgency)
	}
	if gain.Complexity < 0.4 {
		t.Errorf("expected complexity > 0.4 for 20-turn discussion, got %.2f", gain.Complexity)
	}
	if gain.Mode != cerebrov1.GainMode_TONIC {
		t.Errorf("expected TONIC mode, got %v", gain.Mode)
	}
}

func TestUrgency_SingleTurn(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Objective: "quick question",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Hello"},
		},
		TotalTurns: 1,
	}

	gain := AssessUrgency(snap, DefaultUrgencyConfig())

	if gain.Complexity > 0.3 {
		t.Errorf("expected complexity < 0.3 for single turn, got %.2f", gain.Complexity)
	}
}

func TestUrgency_MixedFormalInformal(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Objective: "mixed chat",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "hey lol what do you think?"},
			{TurnNumber: 2, Speaker: "assistant", RawText: "According to the specification, the recommended approach is to use structured logging with correlation identifiers for distributed tracing."},
			{TurnNumber: 3, Speaker: "user", RawText: "cool thanks btw"},
		},
		TotalTurns: 3,
	}

	gain := AssessUrgency(snap, DefaultUrgencyConfig())

	// Formality should be in the middle range (not extreme).
	if gain.Formality < 0.2 || gain.Formality > 0.8 {
		t.Errorf("expected formality between 0.2-0.8 for mixed conversation, got %.2f", gain.Formality)
	}
}

func TestUrgency_NilSnapshot(t *testing.T) {
	gain := AssessUrgency(nil, DefaultUrgencyConfig())

	if gain.Urgency != 0.5 {
		t.Errorf("expected urgency 0.5 for nil snapshot, got %.2f", gain.Urgency)
	}
	if gain.Mode != cerebrov1.GainMode_TONIC {
		t.Errorf("expected TONIC mode for nil snapshot, got %v", gain.Mode)
	}
}

// =============================================================================
// Threshold Modulator tests
// =============================================================================

func TestModulate_HighUrgency(t *testing.T) {
	gain := &GainSignal{Urgency: 0.9, Formality: 0.8, Complexity: 0.5}
	adj := Modulate(gain, DefaultModulatorConfig())

	// High urgency → negative offset (more sensitive).
	for det, offset := range adj.Adjustments {
		if offset >= 0 {
			t.Errorf("expected negative offset for high urgency, got %.3f for %s", offset, det)
		}
	}
}

func TestModulate_LowUrgencyLowFormality(t *testing.T) {
	gain := &GainSignal{Urgency: 0.1, Formality: 0.2, Complexity: 0.1}
	adj := Modulate(gain, DefaultModulatorConfig())

	// Low urgency + low formality → positive offset (less sensitive).
	for det, offset := range adj.Adjustments {
		if offset <= 0 {
			t.Errorf("expected positive offset for low urgency + low formality, got %.3f for %s", offset, det)
		}
	}
}

func TestModulate_Neutral(t *testing.T) {
	gain := &GainSignal{Urgency: 0.5, Formality: 0.5, Complexity: 0.5}
	adj := Modulate(gain, DefaultModulatorConfig())

	// With default weights: -(0.6*0.5) + (0.3*0.5) + -(0.1*0.5) = -0.20
	// Slight negative bias (urgency_weight > formality_weight) is by design.
	for det, offset := range adj.Adjustments {
		if math.Abs(offset) > 0.25 {
			t.Errorf("expected moderate offset for neutral, got %.3f for %s", offset, det)
		}
	}
}

func TestModulate_BoundsClamp(t *testing.T) {
	// Extreme values should not exceed max_gain_offset.
	cfg := DefaultModulatorConfig()

	gain := &GainSignal{Urgency: 1.0, Formality: 1.0, Complexity: 1.0}
	adj := Modulate(gain, cfg)

	for det, offset := range adj.Adjustments {
		if offset < -cfg.MaxGainOffset || offset > cfg.MaxGainOffset {
			t.Errorf("offset %.3f exceeds bounds for %s (max %.2f)", offset, det, cfg.MaxGainOffset)
		}
	}

	gain2 := &GainSignal{Urgency: 0.0, Formality: 0.0, Complexity: 0.0}
	adj2 := Modulate(gain2, cfg)

	for det, offset := range adj2.Adjustments {
		if offset < -cfg.MaxGainOffset || offset > cfg.MaxGainOffset {
			t.Errorf("offset %.3f exceeds bounds for %s (max %.2f)", offset, det, cfg.MaxGainOffset)
		}
	}
}

func TestModulate_AllDetectorsPresent(t *testing.T) {
	gain := &GainSignal{Urgency: 0.5, Formality: 0.5, Complexity: 0.5}
	adj := Modulate(gain, DefaultModulatorConfig())

	for _, det := range KnownDetectors {
		if _, ok := adj.Adjustments[det]; !ok {
			t.Errorf("missing adjustment for detector %s", det)
		}
	}
}

func TestModulate_ScopeGuardExcluded(t *testing.T) {
	gain := &GainSignal{Urgency: 0.9, Formality: 0.1, Complexity: 0.9}
	adj := Modulate(gain, DefaultModulatorConfig())

	if _, ok := adj.Adjustments["scope-guard"]; ok {
		t.Error("scope-guard should be excluded from gain modulation (Forge-optimized threshold)")
	}
}

func TestApplyGainOffset(t *testing.T) {
	// Positive offset → higher threshold
	if got := ApplyGainOffset(0.80, 0.1); math.Abs(got-0.88) > 0.001 {
		t.Errorf("expected 0.88, got %.3f", got)
	}
	// Negative offset → lower threshold
	if got := ApplyGainOffset(0.80, -0.1); math.Abs(got-0.72) > 0.001 {
		t.Errorf("expected 0.72, got %.3f", got)
	}
	// Zero offset → unchanged
	if got := ApplyGainOffset(0.50, 0.0); math.Abs(got-0.50) > 0.001 {
		t.Errorf("expected 0.50, got %.3f", got)
	}
}
