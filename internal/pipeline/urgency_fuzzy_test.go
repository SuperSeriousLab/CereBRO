package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/SuperSeriousLab/fugo"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func buildTestFuzzyUrgency(t *testing.T) *FuzzyUrgency {
	t.Helper()
	var cfg fugo.FisConfig
	if err := json.Unmarshal([]byte(urgencyFISJSON), &cfg); err != nil {
		t.Fatalf("parse urgency FIS: %v", err)
	}
	fu, err := BuildFuzzyUrgency(&cfg)
	if err != nil {
		t.Fatalf("BuildFuzzyUrgency: %v", err)
	}
	return fu
}

// =============================================================================
// TestUrgency_Fuzzy_GainSignal — urgency outputs graded gain signal
// =============================================================================

func TestUrgency_Fuzzy_GainSignal(t *testing.T) {
	fu := buildTestFuzzyUrgency(t)

	// High urgency + formal conversation → high gain signal.
	highSnap := &reasoningv1.ConversationSnapshot{
		Objective: "security review",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "We found a critical security vulnerability in the payment system."},
			{TurnNumber: 2, Speaker: "assistant", RawText: "This is urgent. The risk to patient safety and security is high. According to the specification, we must act immediately."},
			{TurnNumber: 3, Speaker: "user", RawText: "We need this fixed before the deadline tomorrow. The liability is enormous."},
		},
		TotalTurns: 3,
	}

	highGain := AssessUrgencyFuzzy(highSnap, DefaultUrgencyConfig(), nil, fu)
	if highGain.Urgency < 0.5 {
		t.Errorf("expected high gain signal > 0.5 for urgent+formal conversation, got %.3f", highGain.Urgency)
	}

	// Low urgency + informal conversation → low gain signal.
	lowSnap := &reasoningv1.ConversationSnapshot{
		Objective: "button color",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Hey, what color should the button be? lol"},
			{TurnNumber: 2, Speaker: "assistant", RawText: "I think blue would look nice, yeah!"},
		},
		TotalTurns: 2,
	}

	lowGain := AssessUrgencyFuzzy(lowSnap, DefaultUrgencyConfig(), nil, fu)
	if lowGain.Urgency > 0.5 {
		t.Errorf("expected low gain signal < 0.5 for casual conversation, got %.3f", lowGain.Urgency)
	}

	// Verify gradient: high > low.
	if highGain.Urgency <= lowGain.Urgency {
		t.Errorf("high urgency gain (%.3f) should exceed low urgency gain (%.3f)",
			highGain.Urgency, lowGain.Urgency)
	}

	// Verify outputs are in valid range.
	for _, g := range []*GainSignal{highGain, lowGain} {
		if g.Urgency < 0 || g.Urgency > 1 {
			t.Errorf("gain signal out of range: %.3f", g.Urgency)
		}
	}

	t.Logf("high gain=%.3f (mode=%v), low gain=%.3f (mode=%v)",
		highGain.Urgency, highGain.Mode, lowGain.Urgency, lowGain.Mode)
}

func TestUrgency_Fuzzy_PhasicMode(t *testing.T) {
	fu := buildTestFuzzyUrgency(t)

	// Very high urgency should trigger PHASIC mode.
	snap := &reasoningv1.ConversationSnapshot{
		Objective: "emergency",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "This is an emergency. The security risk is critical and urgent."},
			{TurnNumber: 2, Speaker: "assistant", RawText: "According to the specification, the liability and patient safety concerns require immediate action. This is critical."},
			{TurnNumber: 3, Speaker: "user", RawText: "The deadline is in one hour. We face enormous legal liability and risk."},
		},
		TotalTurns: 3,
	}

	gain := AssessUrgencyFuzzy(snap, DefaultUrgencyConfig(), nil, fu)
	if gain.Mode != cerebrov1.GainMode_PHASIC {
		t.Errorf("expected PHASIC mode for very high urgency fuzzy gain, got %v (gain=%.3f)",
			gain.Mode, gain.Urgency)
	}
}

// =============================================================================
// TestUrgency_NilFugo_CrispFallback — nil = existing behavior
// =============================================================================

func TestUrgency_NilFugo_CrispFallback(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Objective: "test",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "This is urgent and critical."},
		},
		TotalTurns: 1,
	}

	cfg := DefaultUrgencyConfig()

	// Crisp baseline.
	crisp := AssessUrgency(snap, cfg)

	// Fuzzy with nil engine — should produce identical results.
	fuzzy := AssessUrgencyFuzzy(snap, cfg, nil, nil)

	if crisp.Urgency != fuzzy.Urgency {
		t.Errorf("nil FuzzyUrgency should match crisp urgency: crisp=%.3f, fuzzy=%.3f",
			crisp.Urgency, fuzzy.Urgency)
	}
	if crisp.Complexity != fuzzy.Complexity {
		t.Errorf("nil FuzzyUrgency should match crisp complexity: crisp=%.3f, fuzzy=%.3f",
			crisp.Complexity, fuzzy.Complexity)
	}
	if crisp.Formality != fuzzy.Formality {
		t.Errorf("nil FuzzyUrgency should match crisp formality: crisp=%.3f, fuzzy=%.3f",
			crisp.Formality, fuzzy.Formality)
	}
	if crisp.Mode != fuzzy.Mode {
		t.Errorf("nil FuzzyUrgency should match crisp mode: crisp=%v, fuzzy=%v",
			crisp.Mode, fuzzy.Mode)
	}
}

func TestUrgency_NilFugo_NilSnap(t *testing.T) {
	gain := AssessUrgencyFuzzy(nil, DefaultUrgencyConfig(), nil, nil)
	if gain.Urgency != 0.5 {
		t.Errorf("nil snapshot should produce urgency=0.5, got %.3f", gain.Urgency)
	}
	if gain.Mode != cerebrov1.GainMode_TONIC {
		t.Errorf("nil snapshot should produce TONIC mode, got %v", gain.Mode)
	}
}

// =============================================================================
// Gain activation level tests
// =============================================================================

func TestGainActivation_Levels(t *testing.T) {
	tests := []struct {
		gain     float64
		expected GainActivationLevel
	}{
		{0.0, GainActivationLow},
		{0.2, GainActivationLow},
		{0.34, GainActivationLow},
		{0.35, GainActivationMedium},
		{0.5, GainActivationMedium},
		{0.64, GainActivationMedium},
		{0.65, GainActivationHigh},
		{0.8, GainActivationHigh},
		{1.0, GainActivationHigh},
	}
	for _, tt := range tests {
		got := ClassifyGainActivation(tt.gain)
		if got != tt.expected {
			t.Errorf("ClassifyGainActivation(%.2f) = %v, want %v", tt.gain, got, tt.expected)
		}
	}
}

func TestShouldActivateDetector_Critical(t *testing.T) {
	// Critical detectors activate at all levels.
	for _, det := range []Detector{DetectorScopeGuard, DetectorContradiction} {
		for _, level := range []GainActivationLevel{GainActivationLow, GainActivationMedium, GainActivationHigh} {
			if !ShouldActivateDetector(det, level) {
				t.Errorf("critical detector %q should activate at level %v", det, level)
			}
		}
	}
}

func TestShouldActivateDetector_NonCritical(t *testing.T) {
	// Non-critical detectors should NOT activate at low gain.
	nonCritical := []Detector{DetectorAnchoring, DetectorSunkCost, DetectorCalibrator, DetectorLedger}
	for _, det := range nonCritical {
		if ShouldActivateDetector(det, GainActivationLow) {
			t.Errorf("non-critical detector %q should NOT activate at low gain", det)
		}
		if !ShouldActivateDetector(det, GainActivationHigh) {
			t.Errorf("non-critical detector %q should activate at high gain", det)
		}
	}
}

// =============================================================================
// Embedded FIS config for tests
// =============================================================================

const urgencyFISJSON = `{
  "name": "l1_urgency",
  "defuzz_method": "centroid",
  "centroid_resolution": 200,
  "input_variables": [
    {
      "name": "urgency",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "low", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.15, "d": 0.35 } },
        { "name": "moderate", "Triangular": { "a": 0.2, "b": 0.45, "c": 0.7 } },
        { "name": "high", "Trapezoidal": { "a": 0.55, "b": 0.75, "c": 1.0, "d": 1.0 } }
      ]
    },
    {
      "name": "complexity",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "simple", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.2, "d": 0.4 } },
        { "name": "moderate", "Triangular": { "a": 0.25, "b": 0.5, "c": 0.75 } },
        { "name": "complex", "Trapezoidal": { "a": 0.6, "b": 0.8, "c": 1.0, "d": 1.0 } }
      ]
    },
    {
      "name": "formality",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "informal", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.3, "d": 0.55 } },
        { "name": "moderate", "Triangular": { "a": 0.35, "b": 0.55, "c": 0.75 } },
        { "name": "formal", "Trapezoidal": { "a": 0.6, "b": 0.8, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "output_variables": [
    {
      "name": "gain_signal",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "suppress", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.1, "d": 0.25 } },
        { "name": "low", "Triangular": { "a": 0.1, "b": 0.25, "c": 0.4 } },
        { "name": "moderate", "Triangular": { "a": 0.3, "b": 0.5, "c": 0.7 } },
        { "name": "high", "Triangular": { "a": 0.6, "b": 0.75, "c": 0.9 } },
        { "name": "full", "Trapezoidal": { "a": 0.8, "b": 0.9, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "rules": [
    { "conditions": [{ "variable": "urgency", "term": "high" }, { "variable": "formality", "term": "formal" }], "connector": "and", "consequent": { "variable": "gain_signal", "term": "full" } },
    { "conditions": [{ "variable": "urgency", "term": "high" }, { "variable": "formality", "term": "moderate" }], "connector": "and", "consequent": { "variable": "gain_signal", "term": "high" } },
    { "conditions": [{ "variable": "urgency", "term": "high" }, { "variable": "formality", "term": "informal" }], "connector": "and", "consequent": { "variable": "gain_signal", "term": "moderate" } },
    { "conditions": [{ "variable": "urgency", "term": "moderate" }, { "variable": "formality", "term": "formal" }], "connector": "and", "consequent": { "variable": "gain_signal", "term": "high" } },
    { "conditions": [{ "variable": "urgency", "term": "moderate" }, { "variable": "formality", "term": "moderate" }], "connector": "and", "consequent": { "variable": "gain_signal", "term": "moderate" } },
    { "conditions": [{ "variable": "urgency", "term": "moderate" }, { "variable": "formality", "term": "informal" }], "connector": "and", "consequent": { "variable": "gain_signal", "term": "low" } },
    { "conditions": [{ "variable": "urgency", "term": "low" }, { "variable": "formality", "term": "formal" }], "connector": "and", "consequent": { "variable": "gain_signal", "term": "moderate" } },
    { "conditions": [{ "variable": "urgency", "term": "low" }, { "variable": "formality", "term": "moderate" }], "connector": "and", "consequent": { "variable": "gain_signal", "term": "low" } },
    { "conditions": [{ "variable": "urgency", "term": "low" }, { "variable": "formality", "term": "informal" }], "connector": "and", "consequent": { "variable": "gain_signal", "term": "suppress" } },
    { "conditions": [{ "variable": "complexity", "term": "complex" }, { "variable": "urgency", "term": "moderate" }], "connector": "and", "consequent": { "variable": "gain_signal", "term": "high" } },
    { "conditions": [{ "variable": "complexity", "term": "complex" }, { "variable": "urgency", "term": "low" }], "connector": "and", "consequent": { "variable": "gain_signal", "term": "moderate" } }
  ]
}`
