package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/SuperSeriousLab/fugo"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func buildTestArbitrator(t *testing.T) *CrossLayerArbitrator {
	t.Helper()
	var cfg fugo.FisConfig
	if err := json.Unmarshal([]byte(arbitrationFISJSON), &cfg); err != nil {
		t.Fatalf("parse arbitration FIS: %v", err)
	}
	arb, err := BuildCrossLayerArbitrator(&cfg)
	if err != nil {
		t.Fatalf("BuildCrossLayerArbitrator: %v", err)
	}
	return arb
}

// =============================================================================
// TestArbitration_CompoundPathology — arbitrator produces compound score
// =============================================================================

func TestArbitration_CompoundPathology(t *testing.T) {
	arb := buildTestArbitrator(t)

	// Scenario 1: Multiple high-severity findings, no inhibition → high pathology.
	criticalFindings := []*reasoningv1.CognitiveAssessment{
		makeAssessment("contradiction-tracker",
			reasoningv1.FindingType_CONTRADICTION,
			reasoningv1.FindingSeverity_CRITICAL,
			0.9, []uint32{1, 5}),
		makeAssessment("scope-guard",
			reasoningv1.FindingType_SCOPE_DRIFT,
			reasoningv1.FindingSeverity_WARNING,
			0.8, []uint32{3}),
		makeAssessment("anchoring-detector",
			reasoningv1.FindingType_ANCHORING_BIAS,
			reasoningv1.FindingSeverity_WARNING,
			0.7, []uint32{2}),
		makeAssessment("sunk-cost-detector",
			reasoningv1.FindingType_SUNK_COST_FALLACY,
			reasoningv1.FindingSeverity_CAUTION,
			0.6, []uint32{4}),
	}

	// No inhibition — all findings pass through.
	noInhResult := &InhibitorResult{
		Gated: criticalFindings,
	}

	highResult := arb.Arbitrate(criticalFindings, noInhResult)
	if highResult.CompoundPathology < 0.5 {
		t.Errorf("expected high compound_pathology > 0.5 for many severe findings, got %.3f",
			highResult.CompoundPathology)
	}
	if highResult.Action == ArbitrationDismiss {
		t.Errorf("expected monitor or investigate action, got dismiss (cp=%.3f)",
			highResult.CompoundPathology)
	}

	// Scenario 2: Single low-severity finding → low pathology.
	mildFindings := []*reasoningv1.CognitiveAssessment{
		makeAssessment("confidence-calibrator",
			reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION,
			reasoningv1.FindingSeverity_INFO,
			0.3, []uint32{1}),
	}
	mildInhResult := &InhibitorResult{
		Gated: mildFindings,
	}

	lowResult := arb.Arbitrate(mildFindings, mildInhResult)
	if lowResult.CompoundPathology > 0.5 {
		t.Errorf("expected low compound_pathology < 0.5 for single mild finding, got %.3f",
			lowResult.CompoundPathology)
	}

	// Verify gradient: high > low.
	if highResult.CompoundPathology <= lowResult.CompoundPathology {
		t.Errorf("high pathology (%.3f) should exceed low pathology (%.3f)",
			highResult.CompoundPathology, lowResult.CompoundPathology)
	}

	t.Logf("high scenario: cp=%.3f action=%s, low scenario: cp=%.3f action=%s",
		highResult.CompoundPathology, highResult.Action,
		lowResult.CompoundPathology, lowResult.Action)
}

func TestArbitration_InhibitionReducesPathology(t *testing.T) {
	arb := buildTestArbitrator(t)

	findings := []*reasoningv1.CognitiveAssessment{
		makeAssessment("contradiction-tracker",
			reasoningv1.FindingType_CONTRADICTION,
			reasoningv1.FindingSeverity_WARNING,
			0.7, []uint32{1}),
		makeAssessment("anchoring-detector",
			reasoningv1.FindingType_ANCHORING_BIAS,
			reasoningv1.FindingSeverity_WARNING,
			0.6, []uint32{2}),
		makeAssessment("scope-guard",
			reasoningv1.FindingType_SCOPE_DRIFT,
			reasoningv1.FindingSeverity_CAUTION,
			0.5, []uint32{3}),
	}

	// No inhibition.
	noInh := &InhibitorResult{Gated: findings}
	noInhResult := arb.Arbitrate(findings, noInh)

	// High inhibition — only one finding passes.
	highInh := &InhibitorResult{
		Gated: findings[:1],
		Decisions: []*cerebrov1.InhibitionDecision{
			{Action: cerebrov1.InhibitionAction_DISINHIBITED},
			{Action: cerebrov1.InhibitionAction_INHIBITED},
			{Action: cerebrov1.InhibitionAction_INHIBITED},
		},
	}
	highInhResult := arb.Arbitrate(findings, highInh)

	// High inhibition ratio should reduce compound pathology.
	if highInhResult.CompoundPathology >= noInhResult.CompoundPathology {
		t.Errorf("high inhibition (cp=%.3f) should produce lower or equal pathology than no inhibition (cp=%.3f)",
			highInhResult.CompoundPathology, noInhResult.CompoundPathology)
	}

	t.Logf("no inhibition: cp=%.3f, high inhibition: cp=%.3f",
		noInhResult.CompoundPathology, highInhResult.CompoundPathology)
}

func TestArbitration_EmptyFindings(t *testing.T) {
	arb := buildTestArbitrator(t)

	result := arb.Arbitrate(nil, nil)
	if result.CompoundPathology != 0.0 {
		t.Errorf("empty findings should produce cp=0.0, got %.3f", result.CompoundPathology)
	}
	if result.Action != ArbitrationDismiss {
		t.Errorf("empty findings should produce dismiss action, got %s", result.Action)
	}
}

func TestArbitration_ActionThresholds(t *testing.T) {
	// Verify the action classification thresholds.
	tests := []struct {
		cp     float64
		action ArbitrationAction
	}{
		{0.0, ArbitrationDismiss},
		{0.2, ArbitrationDismiss},
		{0.35, ArbitrationDismiss},
		{0.36, ArbitrationMonitor},
		{0.5, ArbitrationMonitor},
		{0.65, ArbitrationMonitor},
		{0.66, ArbitrationInvestigate},
		{0.9, ArbitrationInvestigate},
		{1.0, ArbitrationInvestigate},
	}
	for _, tt := range tests {
		got := classifyAction(tt.cp)
		if got != tt.action {
			t.Errorf("classifyAction(%.2f) = %q, want %q", tt.cp, got, tt.action)
		}
	}
}

// =============================================================================
// TestArbitration_NilFugo_Passthrough — nil = all findings pass through
// =============================================================================

func TestArbitration_NilFugo_Passthrough(t *testing.T) {
	var arb *CrossLayerArbitrator // nil

	findings := []*reasoningv1.CognitiveAssessment{
		makeAssessment("contradiction-tracker",
			reasoningv1.FindingType_CONTRADICTION,
			reasoningv1.FindingSeverity_CRITICAL,
			0.9, []uint32{1}),
	}

	result := arb.Arbitrate(findings, nil)
	if result.CompoundPathology != 0.0 {
		t.Errorf("nil arbitrator should produce cp=0.0, got %.3f", result.CompoundPathology)
	}
	if result.Action != ArbitrationDismiss {
		t.Errorf("nil arbitrator should produce dismiss action, got %s", result.Action)
	}
	if result.FindingCount != 1 {
		t.Errorf("nil arbitrator should report finding count=1, got %d", result.FindingCount)
	}
}

// =============================================================================
// Embedded FIS config for tests
// =============================================================================

const arbitrationFISJSON = `{
  "name": "cross_layer_arbitration",
  "defuzz_method": "centroid",
  "centroid_resolution": 200,
  "input_variables": [
    {
      "name": "max_severity",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "low", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.2, "d": 0.4 } },
        { "name": "moderate", "Triangular": { "a": 0.25, "b": 0.5, "c": 0.75 } },
        { "name": "high", "Trapezoidal": { "a": 0.6, "b": 0.8, "c": 1.0, "d": 1.0 } }
      ]
    },
    {
      "name": "finding_density",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "sparse", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.15, "d": 0.35 } },
        { "name": "moderate", "Triangular": { "a": 0.2, "b": 0.45, "c": 0.7 } },
        { "name": "dense", "Trapezoidal": { "a": 0.55, "b": 0.75, "c": 1.0, "d": 1.0 } }
      ]
    },
    {
      "name": "inhibition_ratio",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "low", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.2, "d": 0.45 } },
        { "name": "moderate", "Triangular": { "a": 0.3, "b": 0.5, "c": 0.7 } },
        { "name": "high", "Trapezoidal": { "a": 0.55, "b": 0.8, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "output_variables": [
    {
      "name": "compound_pathology",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "healthy", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.1, "d": 0.25 } },
        { "name": "mild", "Triangular": { "a": 0.15, "b": 0.3, "c": 0.45 } },
        { "name": "concerning", "Triangular": { "a": 0.35, "b": 0.5, "c": 0.7 } },
        { "name": "serious", "Triangular": { "a": 0.55, "b": 0.75, "c": 0.9 } },
        { "name": "critical", "Trapezoidal": { "a": 0.8, "b": 0.9, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "rules": [
    { "conditions": [{ "variable": "max_severity", "term": "high" }, { "variable": "finding_density", "term": "dense" }], "connector": "and", "consequent": { "variable": "compound_pathology", "term": "critical" } },
    { "conditions": [{ "variable": "max_severity", "term": "high" }, { "variable": "finding_density", "term": "moderate" }], "connector": "and", "consequent": { "variable": "compound_pathology", "term": "serious" } },
    { "conditions": [{ "variable": "max_severity", "term": "high" }, { "variable": "finding_density", "term": "sparse" }], "connector": "and", "consequent": { "variable": "compound_pathology", "term": "concerning" } },
    { "conditions": [{ "variable": "max_severity", "term": "moderate" }, { "variable": "finding_density", "term": "dense" }], "connector": "and", "consequent": { "variable": "compound_pathology", "term": "serious" } },
    { "conditions": [{ "variable": "max_severity", "term": "moderate" }, { "variable": "finding_density", "term": "moderate" }], "connector": "and", "consequent": { "variable": "compound_pathology", "term": "concerning" } },
    { "conditions": [{ "variable": "max_severity", "term": "moderate" }, { "variable": "finding_density", "term": "sparse" }], "connector": "and", "consequent": { "variable": "compound_pathology", "term": "mild" } },
    { "conditions": [{ "variable": "max_severity", "term": "low" }, { "variable": "finding_density", "term": "dense" }], "connector": "and", "consequent": { "variable": "compound_pathology", "term": "concerning" } },
    { "conditions": [{ "variable": "max_severity", "term": "low" }, { "variable": "finding_density", "term": "moderate" }], "connector": "and", "consequent": { "variable": "compound_pathology", "term": "mild" } },
    { "conditions": [{ "variable": "max_severity", "term": "low" }, { "variable": "finding_density", "term": "sparse" }], "connector": "and", "consequent": { "variable": "compound_pathology", "term": "healthy" } },
    { "conditions": [{ "variable": "inhibition_ratio", "term": "high" }, { "variable": "max_severity", "term": "moderate" }], "connector": "and", "consequent": { "variable": "compound_pathology", "term": "mild" } },
    { "conditions": [{ "variable": "inhibition_ratio", "term": "high" }, { "variable": "max_severity", "term": "low" }], "connector": "and", "consequent": { "variable": "compound_pathology", "term": "healthy" } },
    { "conditions": [{ "variable": "inhibition_ratio", "term": "low" }, { "variable": "finding_density", "term": "dense" }], "connector": "and", "consequent": { "variable": "compound_pathology", "term": "critical" } }
  ]
}`
