package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/SuperSeriousLab/fugo"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// buildTestFuzzyInhibitor constructs a FuzzyInhibitor from embedded JSON configs.
func buildTestFuzzyInhibitor(t *testing.T) *FuzzyInhibitor {
	t.Helper()

	formalityCfg := parseFIS(t, formalityFISJSON)
	severityCfg := parseFIS(t, severityFISJSON)
	evidenceCfg := parseFIS(t, evidenceFISJSON)

	fi, err := BuildFuzzyInhibitor(formalityCfg, severityCfg, evidenceCfg)
	if err != nil {
		t.Fatalf("BuildFuzzyInhibitor: %v", err)
	}
	return fi
}

func parseFIS(t *testing.T, raw string) *fugo.FisConfig {
	t.Helper()
	var cfg fugo.FisConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("parseFIS: %v", err)
	}
	return &cfg
}

func fuzzyConfig(t *testing.T) InhibitorConfig {
	t.Helper()
	cfg := DefaultInhibitorConfig()
	cfg.Fuzzy = buildTestFuzzyInhibitor(t)
	return cfg
}

// =============================================================================
// Fuzzy gate tests
// =============================================================================

func TestInhibitor_FuzzyGate_SmoothSuppression(t *testing.T) {
	// Finding with medium formality gets partial suppression, not full block.
	// Medium formality = ~0.5 → formality gate should produce moderate inhibition
	// but not enough to fully block (threshold 0.8).
	assessment := makeAssessment("scope-guard",
		reasoningv1.FindingType_SCOPE_DRIFT,
		reasoningv1.FindingSeverity_WARNING,
		0.7, []uint32{2})

	// Create a mixed-formality conversation (neither very formal nor very informal).
	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "user", "I think we should look at the data more carefully."},
		{2, "assistant", "The specification suggests a different approach based on the analysis."},
	}, "discussion")

	// Add corroboration so evidence gate doesn't block.
	other := makeAssessment("contradiction-tracker",
		reasoningv1.FindingType_CONTRADICTION,
		reasoningv1.FindingSeverity_WARNING,
		0.8, []uint32{2})

	cfg := fuzzyConfig(t)
	cfg.StakesThreshold = 0.0

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment, other}, snap, cfg)

	var sgDecision *cerebrov1.InhibitionDecision
	var sgGated *reasoningv1.CognitiveAssessment
	for i, d := range result.Decisions {
		if d.GetDetectorName() == "scope-guard" {
			sgDecision = d
			// In fuzzy mode, gated findings have suppressed confidence.
			for _, g := range result.Gated {
				if g.GetDetectorName() == "scope-guard" {
					sgGated = g
					break
				}
			}
			_ = i
			break
		}
	}
	if sgDecision == nil {
		t.Fatal("no decision for scope-guard")
	}

	// With medium formality, the finding should pass (not fully blocked).
	if sgDecision.GetAction() != cerebrov1.InhibitionAction_DISINHIBITED {
		t.Errorf("medium formality finding should be DISINHIBITED, got %v (reason: %s)",
			sgDecision.GetAction(), sgDecision.GetReason())
	}

	// Confidence should be partially suppressed (less than original 0.7).
	if sgGated != nil && sgGated.GetConfidence() >= 0.7 {
		t.Errorf("fuzzy suppression should reduce confidence below 0.7, got %.3f",
			sgGated.GetConfidence())
	}
}

func TestInhibitor_FuzzyGate_HighSeverityPasses(t *testing.T) {
	// High severity findings pass with minimal inhibition.
	assessment := makeAssessment("contradiction-tracker",
		reasoningv1.FindingType_CONTRADICTION,
		reasoningv1.FindingSeverity_CRITICAL,
		0.9, []uint32{1})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "assistant", "yeah lol let's just go with it"},
	}, "casual chat")

	cfg := fuzzyConfig(t)

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment}, snap, cfg)

	if len(result.Decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(result.Decisions))
	}
	if result.Decisions[0].GetAction() != cerebrov1.InhibitionAction_DISINHIBITED {
		t.Errorf("CRITICAL should always disinhibit even in fuzzy mode, got %v",
			result.Decisions[0].GetAction())
	}
	if result.Decisions[0].GetReason() != "severity_auto_pass" {
		t.Errorf("expected reason severity_auto_pass, got %s",
			result.Decisions[0].GetReason())
	}
}

func TestInhibitor_FuzzyGate_LowEvidenceInhibited(t *testing.T) {
	// Thin evidence (low confidence + no corroboration) should get strong inhibition.
	assessment := makeAssessment("anchoring-detector",
		reasoningv1.FindingType_ANCHORING_BIAS,
		reasoningv1.FindingSeverity_WARNING,
		0.3, []uint32{1})
	// Second detector far away — no corroboration.
	other := makeAssessment("scope-guard",
		reasoningv1.FindingType_SCOPE_DRIFT,
		reasoningv1.FindingSeverity_WARNING,
		0.5, []uint32{20})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "assistant", "According to the specification, the estimate is 15 months."},
		{20, "assistant", "In accordance with the requirements, we should review the scope."},
	}, "analysis")

	cfg := fuzzyConfig(t)
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

	if anchDecision.GetAction() != cerebrov1.InhibitionAction_INHIBITED {
		t.Errorf("low evidence finding should be INHIBITED, got %v (reason: %s)",
			anchDecision.GetAction(), anchDecision.GetReason())
	}
}

func TestInhibitor_NilFugo_FallsBackToCrisp(t *testing.T) {
	// nil Fuzzy = current crisp behavior exactly.
	cfg := DefaultInhibitorConfig()
	if cfg.Fuzzy != nil {
		t.Fatal("DefaultInhibitorConfig should have nil Fuzzy")
	}

	// Use the same test case as Gate 2 crisp test.
	assessment := makeAssessment("contradiction-tracker",
		reasoningv1.FindingType_CONTRADICTION,
		reasoningv1.FindingSeverity_CRITICAL,
		0.9, []uint32{1, 5})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "user", "hey lol"},
		{5, "assistant", "nah"},
	}, "casual chat")

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment}, snap, cfg)

	if len(result.Decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(result.Decisions))
	}
	if result.Decisions[0].GetAction() != cerebrov1.InhibitionAction_DISINHIBITED {
		t.Errorf("crisp fallback: CRITICAL should always disinhibit, got %v",
			result.Decisions[0].GetAction())
	}
	if result.Decisions[0].GetReason() != "severity_auto_pass" {
		t.Errorf("expected reason severity_auto_pass, got %s",
			result.Decisions[0].GetReason())
	}

	// Also verify the gated findings haven't had their confidence modified.
	if len(result.Gated) != 1 {
		t.Fatalf("expected 1 gated finding, got %d", len(result.Gated))
	}
	if result.Gated[0].GetConfidence() != 0.9 {
		t.Errorf("crisp fallback should not modify confidence, got %.3f",
			result.Gated[0].GetConfidence())
	}
}

func TestInhibitor_FuzzyVsCrisp_NoRegression(t *testing.T) {
	// On the existing test corpus, fuzzy inhibition doesn't block findings
	// that crisp passes (no regression).
	// Use the standard integration test case: CRITICAL + low-conf WARNING.
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

	// Crisp baseline.
	crispCfg := DefaultInhibitorConfig()
	crispResult := Inhibit([]*reasoningv1.CognitiveAssessment{critical, lowConf}, snap, crispCfg)

	// Fuzzy path.
	fuzzyCfg := fuzzyConfig(t)
	fuzzyResult := Inhibit([]*reasoningv1.CognitiveAssessment{critical, lowConf}, snap, fuzzyCfg)

	// Everything crisp passes should also pass in fuzzy.
	crispGated := make(map[string]bool)
	for _, g := range crispResult.Gated {
		crispGated[g.GetDetectorName()] = true
	}

	for det := range crispGated {
		found := false
		for _, g := range fuzzyResult.Gated {
			if g.GetDetectorName() == det {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("regression: crisp passes %q but fuzzy blocks it", det)
		}
	}

	// CRITICAL must pass in both.
	if !crispGated["contradiction-tracker"] {
		t.Error("crisp should pass CRITICAL contradiction-tracker")
	}
	fuzzyPassesCritical := false
	for _, g := range fuzzyResult.Gated {
		if g.GetDetectorName() == "contradiction-tracker" {
			fuzzyPassesCritical = true
			break
		}
	}
	if !fuzzyPassesCritical {
		t.Error("fuzzy should pass CRITICAL contradiction-tracker")
	}
}

func TestInhibitor_FuzzyCasualHedge_StillWorks(t *testing.T) {
	// Gate 1 (casual hedge) should still work in fuzzy mode — it's a crisp override.
	assessment := makeAssessment("confidence-calibrator",
		reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION,
		reasoningv1.FindingSeverity_CRITICAL,
		0.67, []uint32{2})

	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "user", "hey how should we handle logging?"},
		{2, "assistant", "Absolutely, structured logging is the way to go!"},
	}, "casual chat")

	cfg := fuzzyConfig(t)
	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment}, snap, cfg)

	if result.Decisions[0].GetAction() != cerebrov1.InhibitionAction_INHIBITED {
		t.Errorf("casual hedge should still be INHIBITED in fuzzy mode, got %v (reason: %s)",
			result.Decisions[0].GetAction(), result.Decisions[0].GetReason())
	}
}

func TestInhibitor_FuzzyConfidenceSuppression(t *testing.T) {
	// Verify that fuzzy mode actually modifies confidence (soft suppression).
	// Use a scenario where a finding passes but with partial suppression.
	assessment := makeAssessment("scope-guard",
		reasoningv1.FindingType_SCOPE_DRIFT,
		reasoningv1.FindingSeverity_WARNING,
		0.8, []uint32{2})

	// Informal conversation → formality gate produces some inhibition.
	snap := makeSnap([]struct{ num uint32; speaker, text string }{
		{1, "user", "hey what do you think about this?"},
		{2, "assistant", "yeah I think we should go with that approach"},
	}, "chat")

	cfg := fuzzyConfig(t)
	cfg.StakesThreshold = 0.0

	result := Inhibit([]*reasoningv1.CognitiveAssessment{assessment}, snap, cfg)

	// Single detector → corroboration=1.0, so evidence gate is weak.
	// But formality is low → formality gate produces inhibition.
	// The finding may pass or be blocked depending on inhibition strength.
	// Key assertion: if it passes, confidence must be < original.
	for _, g := range result.Gated {
		if g.GetDetectorName() == "scope-guard" {
			if g.GetConfidence() >= 0.8 {
				t.Errorf("fuzzy suppression should reduce confidence below 0.8, got %.3f",
					g.GetConfidence())
			}
			return
		}
	}
	// If fully inhibited, that's also acceptable for very informal context.
	t.Logf("finding was fully inhibited (informal context), which is valid fuzzy behavior")
}

// =============================================================================
// Embedded FIS configs for tests (avoid file I/O in unit tests)
// =============================================================================

const formalityFISJSON = `{
  "name": "l3_formality_gate",
  "defuzz_method": "centroid",
  "centroid_resolution": 200,
  "input_variables": [
    {
      "name": "formality",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "informal", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.4, "d": 0.7 } },
        { "name": "moderate", "Triangular": { "a": 0.4, "b": 0.65, "c": 0.9 } },
        { "name": "formal", "Trapezoidal": { "a": 0.7, "b": 0.9, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "output_variables": [
    {
      "name": "inhibition_strength",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "none", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.05, "d": 0.15 } },
        { "name": "weak", "Triangular": { "a": 0.05, "b": 0.25, "c": 0.45 } },
        { "name": "moderate", "Triangular": { "a": 0.3, "b": 0.5, "c": 0.7 } },
        { "name": "strong", "Triangular": { "a": 0.55, "b": 0.75, "c": 0.95 } },
        { "name": "full", "Trapezoidal": { "a": 0.85, "b": 0.95, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "rules": [
    { "conditions": [{ "variable": "formality", "term": "informal" }], "consequent": { "variable": "inhibition_strength", "term": "strong" } },
    { "conditions": [{ "variable": "formality", "term": "moderate" }], "consequent": { "variable": "inhibition_strength", "term": "weak" } },
    { "conditions": [{ "variable": "formality", "term": "formal" }], "consequent": { "variable": "inhibition_strength", "term": "none" } }
  ]
}`

const severityFISJSON = `{
  "name": "l3_severity_gate",
  "defuzz_method": "centroid",
  "centroid_resolution": 200,
  "input_variables": [
    {
      "name": "severity",
      "min": 0.0,
      "max": 3.0,
      "terms": [
        { "name": "low", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.5, "d": 1.5 } },
        { "name": "medium", "Triangular": { "a": 0.5, "b": 1.5, "c": 2.5 } },
        { "name": "high", "Trapezoidal": { "a": 1.5, "b": 2.5, "c": 3.0, "d": 3.0 } }
      ]
    },
    {
      "name": "urgency",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "low", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.15, "d": 0.4 } },
        { "name": "moderate", "Triangular": { "a": 0.2, "b": 0.5, "c": 0.8 } },
        { "name": "high", "Trapezoidal": { "a": 0.6, "b": 0.85, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "output_variables": [
    {
      "name": "inhibition_strength",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "none", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.05, "d": 0.15 } },
        { "name": "weak", "Triangular": { "a": 0.05, "b": 0.25, "c": 0.45 } },
        { "name": "moderate", "Triangular": { "a": 0.3, "b": 0.5, "c": 0.7 } },
        { "name": "strong", "Triangular": { "a": 0.55, "b": 0.75, "c": 0.95 } },
        { "name": "full", "Trapezoidal": { "a": 0.85, "b": 0.95, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "rules": [
    { "conditions": [{ "variable": "severity", "term": "low" }, { "variable": "urgency", "term": "low" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "full" } },
    { "conditions": [{ "variable": "severity", "term": "low" }, { "variable": "urgency", "term": "moderate" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "strong" } },
    { "conditions": [{ "variable": "severity", "term": "low" }, { "variable": "urgency", "term": "high" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "moderate" } },
    { "conditions": [{ "variable": "severity", "term": "medium" }, { "variable": "urgency", "term": "low" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "moderate" } },
    { "conditions": [{ "variable": "severity", "term": "medium" }, { "variable": "urgency", "term": "moderate" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "weak" } },
    { "conditions": [{ "variable": "severity", "term": "medium" }, { "variable": "urgency", "term": "high" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "none" } },
    { "conditions": [{ "variable": "severity", "term": "high" }, { "variable": "urgency", "term": "low" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "weak" } },
    { "conditions": [{ "variable": "severity", "term": "high" }, { "variable": "urgency", "term": "moderate" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "none" } },
    { "conditions": [{ "variable": "severity", "term": "high" }, { "variable": "urgency", "term": "high" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "none" } }
  ]
}`

const evidenceFISJSON = `{
  "name": "l3_evidence_gate",
  "defuzz_method": "centroid",
  "centroid_resolution": 200,
  "input_variables": [
    {
      "name": "confidence",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "low", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.3, "d": 0.55 } },
        { "name": "medium", "Triangular": { "a": 0.35, "b": 0.55, "c": 0.75 } },
        { "name": "high", "Trapezoidal": { "a": 0.6, "b": 0.8, "c": 1.0, "d": 1.0 } }
      ]
    },
    {
      "name": "corroboration",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "none", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.05, "d": 0.2 } },
        { "name": "weak", "Triangular": { "a": 0.05, "b": 0.25, "c": 0.5 } },
        { "name": "strong", "Trapezoidal": { "a": 0.3, "b": 0.6, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "output_variables": [
    {
      "name": "inhibition_strength",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "none", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.05, "d": 0.15 } },
        { "name": "weak", "Triangular": { "a": 0.05, "b": 0.25, "c": 0.45 } },
        { "name": "moderate", "Triangular": { "a": 0.3, "b": 0.5, "c": 0.7 } },
        { "name": "strong", "Triangular": { "a": 0.55, "b": 0.75, "c": 0.95 } },
        { "name": "full", "Trapezoidal": { "a": 0.85, "b": 0.95, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "rules": [
    { "conditions": [{ "variable": "confidence", "term": "low" }, { "variable": "corroboration", "term": "none" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "full" } },
    { "conditions": [{ "variable": "confidence", "term": "low" }, { "variable": "corroboration", "term": "weak" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "strong" } },
    { "conditions": [{ "variable": "confidence", "term": "low" }, { "variable": "corroboration", "term": "strong" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "moderate" } },
    { "conditions": [{ "variable": "confidence", "term": "medium" }, { "variable": "corroboration", "term": "none" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "strong" } },
    { "conditions": [{ "variable": "confidence", "term": "medium" }, { "variable": "corroboration", "term": "weak" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "moderate" } },
    { "conditions": [{ "variable": "confidence", "term": "medium" }, { "variable": "corroboration", "term": "strong" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "weak" } },
    { "conditions": [{ "variable": "confidence", "term": "high" }, { "variable": "corroboration", "term": "none" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "moderate" } },
    { "conditions": [{ "variable": "confidence", "term": "high" }, { "variable": "corroboration", "term": "weak" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "weak" } },
    { "conditions": [{ "variable": "confidence", "term": "high" }, { "variable": "corroboration", "term": "strong" }], "connector": "and", "consequent": { "variable": "inhibition_strength", "term": "none" } }
  ]
}`
