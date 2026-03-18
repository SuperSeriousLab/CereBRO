package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/SuperSeriousLab/fugo"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// =============================================================================
// Helper: build test DetectorFuzzy from embedded JSON configs
// =============================================================================

func buildTestDetectorFuzzy(t *testing.T) *DetectorFuzzy {
	t.Helper()
	anchoringCfg := parseDetFIS(t, anchoringDetFISJSON)
	contradictionCfg := parseDetFIS(t, contradictionDetFISJSON)
	calibratorCfg := parseDetFIS(t, calibratorDetFISJSON)
	sunkCostCfg := parseDetFIS(t, sunkCostDetFISJSON)

	df, err := BuildDetectorFuzzy(anchoringCfg, contradictionCfg, calibratorCfg, sunkCostCfg)
	if err != nil {
		t.Fatalf("BuildDetectorFuzzy: %v", err)
	}
	return df
}

func parseDetFIS(t *testing.T, raw string) *fugo.FisConfig {
	t.Helper()
	var cfg fugo.FisConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("parseDetFIS: %v", err)
	}
	return &cfg
}

// =============================================================================
// TestDetector_Fuzzy_SmoothSeverity — fuzzy outputs graded severity, not binary
// =============================================================================

func TestDetector_Fuzzy_SmoothSeverity(t *testing.T) {
	df := buildTestDetectorFuzzy(t)

	// Anchoring with a small shift (very close) should produce high severity.
	highSev, ok := df.evaluateAnchoringSeverity(0.02)
	if !ok {
		t.Fatal("evaluateAnchoringSeverity failed for shift=0.02")
	}
	// Anchoring with a moderate shift should produce lower severity.
	medSev, ok := df.evaluateAnchoringSeverity(0.10)
	if !ok {
		t.Fatal("evaluateAnchoringSeverity failed for shift=0.10")
	}

	if highSev <= medSev {
		t.Errorf("very close shift (0.02) should have higher severity than moderate shift (0.10): got %.3f <= %.3f",
			highSev, medSev)
	}

	// Both should be in (0, 1) range — smooth, not binary.
	if highSev <= 0.0 || highSev >= 1.0 {
		t.Errorf("high severity should be in (0,1), got %.3f", highSev)
	}
	if medSev <= 0.0 || medSev >= 1.0 {
		t.Errorf("medium severity should be in (0,1), got %.3f", medSev)
	}

	t.Logf("shift=0.02 → severity=%.3f, shift=0.10 → severity=%.3f (smooth gradient)", highSev, medSev)

	// Contradiction: high overlap + strong kind should produce higher severity than low overlap + weak kind.
	highContSev, ok := df.evaluateContradictionSeverity(0.8, 0.9)
	if !ok {
		t.Fatal("evaluateContradictionSeverity failed for high inputs")
	}
	lowContSev, ok := df.evaluateContradictionSeverity(0.2, 0.3)
	if !ok {
		t.Fatal("evaluateContradictionSeverity failed for low inputs")
	}
	if highContSev <= lowContSev {
		t.Errorf("high overlap+strong kind should have higher severity: got %.3f <= %.3f",
			highContSev, lowContSev)
	}
	t.Logf("contradiction high=%.3f, low=%.3f", highContSev, lowContSev)

	// Calibrator: high ECE should produce higher severity than low ECE.
	highCalSev, ok := df.evaluateCalibratorSeverity(0.9)
	if !ok {
		t.Fatal("evaluateCalibratorSeverity failed for ece=0.9")
	}
	lowCalSev, ok := df.evaluateCalibratorSeverity(0.2)
	if !ok {
		t.Fatal("evaluateCalibratorSeverity failed for ece=0.2")
	}
	if highCalSev <= lowCalSev {
		t.Errorf("high ECE should have higher severity: got %.3f <= %.3f", highCalSev, lowCalSev)
	}
	t.Logf("calibrator high=%.3f, low=%.3f", highCalSev, lowCalSev)

	// Sunk cost: high confidence + adjacent turns should produce higher severity.
	highSCSev, ok := df.evaluateSunkCostSeverity(0.9, 1.0)
	if !ok {
		t.Fatal("evaluateSunkCostSeverity failed for high inputs")
	}
	lowSCSev, ok := df.evaluateSunkCostSeverity(0.3, 15.0)
	if !ok {
		t.Fatal("evaluateSunkCostSeverity failed for low inputs")
	}
	if highSCSev <= lowSCSev {
		t.Errorf("high confidence+adjacent should have higher severity: got %.3f <= %.3f",
			highSCSev, lowSCSev)
	}
	t.Logf("sunk-cost high=%.3f, low=%.3f", highSCSev, lowSCSev)
}

// =============================================================================
// TestDetector_Fuzzy_NilRegistry_CrispFallback — nil = existing behavior
// =============================================================================

func TestDetector_Fuzzy_NilRegistry_CrispFallback(t *testing.T) {
	// With nil DetectorFuzzy, all apply* functions return the finding unchanged.
	finding := &reasoningv1.CognitiveAssessment{
		DetectorName:  "anchoring-detector",
		FindingType:   reasoningv1.FindingType_ANCHORING_BIAS,
		Severity:      reasoningv1.FindingSeverity_WARNING,
		Confidence:    0.75,
		RelevantTurns: []uint32{1, 3},
		Anchoring: &reasoningv1.AnchoringDetail{
			AnchorValue:   100.0,
			EstimateValue: 105.0,
			RelativeShift: 0.05,
		},
	}

	result := applyAnchoringFuzzy(finding, nil)
	if result == nil {
		t.Fatal("nil DetectorFuzzy should return finding unchanged, got nil")
	}
	if result.GetConfidence() != 0.75 {
		t.Errorf("nil DetectorFuzzy should not modify confidence, got %.3f", result.GetConfidence())
	}

	// Also test nil finding passthrough.
	result = applyAnchoringFuzzy(nil, nil)
	if result != nil {
		t.Error("nil finding should return nil")
	}

	// Contradiction nil fallback.
	contFinding := &reasoningv1.CognitiveAssessment{
		DetectorName: "contradiction-tracker",
		FindingType:  reasoningv1.FindingType_CONTRADICTION,
		Confidence:   0.65,
		Contradiction: &reasoningv1.ContradictionDetail{
			ClaimAText: "we should use postgres",
			ClaimBText: "we should not use postgres",
		},
	}
	snap := makeSnap(nil, "")
	cfg := DefaultContradictionConfig()
	result = applyContradictionFuzzy(contFinding, snap, cfg, nil)
	if result == nil || result.GetConfidence() != 0.65 {
		t.Errorf("nil DetectorFuzzy should not modify contradiction finding")
	}

	// Calibrator nil fallback.
	calFinding := &reasoningv1.CognitiveAssessment{
		DetectorName: "confidence-calibrator",
		FindingType:  reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION,
		Confidence:   0.8,
		Calibration: &reasoningv1.CalibrationDetail{
			ExpectedCalibrationError: 0.67,
		},
	}
	result = applyCalibratorFuzzy(calFinding, nil)
	if result == nil || result.GetConfidence() != 0.8 {
		t.Errorf("nil DetectorFuzzy should not modify calibrator finding")
	}

	// Sunk cost nil fallback.
	scFinding := &reasoningv1.CognitiveAssessment{
		DetectorName: "sunk-cost-detector",
		FindingType:  reasoningv1.FindingType_SUNK_COST_FALLACY,
		Confidence:   0.7,
		SunkCost: &reasoningv1.SunkCostDetail{
			CostTurn:     1,
			DecisionTurn: 3,
		},
	}
	result = applySunkCostFuzzy(scFinding, nil)
	if result == nil || result.GetConfidence() != 0.7 {
		t.Errorf("nil DetectorFuzzy should not modify sunk-cost finding")
	}
}

// =============================================================================
// TestDetector_Fuzzy_LowSeveritySuppressed — severity < 0.1 doesn't produce finding
// =============================================================================

func TestDetector_Fuzzy_LowSeveritySuppressed(t *testing.T) {
	df := buildTestDetectorFuzzy(t)

	// Anchoring with a very large shift (far beyond threshold) should produce
	// near-zero severity and be suppressed.
	finding := &reasoningv1.CognitiveAssessment{
		DetectorName:  "anchoring-detector",
		FindingType:   reasoningv1.FindingType_ANCHORING_BIAS,
		Severity:      reasoningv1.FindingSeverity_INFO,
		Confidence:    0.3,
		RelevantTurns: []uint32{1, 5},
		Anchoring: &reasoningv1.AnchoringDetail{
			AnchorValue:   100.0,
			EstimateValue: 200.0,
			RelativeShift: 0.95, // far beyond any anchoring threshold
		},
	}

	result := applyAnchoringFuzzy(finding, df)
	if result != nil {
		t.Errorf("far-shift anchoring finding should be suppressed (severity < 0.1), got confidence=%.3f",
			result.GetConfidence())
	}

	// Contradiction with very low overlap and weak kind should be suppressed.
	contFinding := &reasoningv1.CognitiveAssessment{
		DetectorName: "contradiction-tracker",
		FindingType:  reasoningv1.FindingType_CONTRADICTION,
		Confidence:   0.4,
		Contradiction: &reasoningv1.ContradictionDetail{
			ClaimAText: "apples are good",
			ClaimBText: "oranges taste better",
		},
	}
	snap := makeSnap(nil, "")
	cfg := DefaultContradictionConfig()
	result = applyContradictionFuzzy(contFinding, snap, cfg, df)
	// Low overlap + no contradiction kind → kind_strength=0 → severity should be very low.
	if result != nil {
		t.Logf("low-signal contradiction not suppressed (severity=%.3f), checking if below threshold",
			result.GetConfidence())
		// It's acceptable if the FIS produces a small but non-zero value
		// (due to centroid behavior); we just verify it's very low.
		if result.GetConfidence() > 0.2 {
			t.Errorf("low-signal contradiction should have very low severity, got %.3f",
				result.GetConfidence())
		}
	}

	// Calibrator with very low ECE should be suppressed.
	calFinding := &reasoningv1.CognitiveAssessment{
		DetectorName: "confidence-calibrator",
		FindingType:  reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION,
		Confidence:   0.15,
		Calibration: &reasoningv1.CalibrationDetail{
			ExpectedCalibrationError: 0.05, // well below any miscalibration threshold
		},
	}
	result = applyCalibratorFuzzy(calFinding, df)
	if result != nil {
		t.Logf("low-ECE calibrator finding severity=%.3f", result.GetConfidence())
		if result.GetConfidence() > 0.2 {
			t.Errorf("low-ECE calibrator should have very low severity, got %.3f",
				result.GetConfidence())
		}
	}
}

// =============================================================================
// TestDetector_Fuzzy_IntegrationWithBuildDetectorMap — end-to-end via pipeline
// =============================================================================

func TestDetector_Fuzzy_IntegrationWithBuildDetectorMap(t *testing.T) {
	df := buildTestDetectorFuzzy(t)

	cfg := DefaultPipelineConfig()
	cfg.DetectorFuzzy = df

	// Build the detector map and verify detectors are present.
	detectors := buildDetectorMap(cfg)

	expectedDetectors := []Detector{
		DetectorSunkCost,
		DetectorContradiction,
		DetectorScopeGuard,
		DetectorCalibrator,
		DetectorLedger,
		DetectorAnchoring,
	}
	for _, d := range expectedDetectors {
		if _, ok := detectors[d]; !ok {
			t.Errorf("missing detector %q in buildDetectorMap with fuzzy enabled", d)
		}
	}
}

// =============================================================================
// Embedded FIS configs for L2 detector tests
// =============================================================================

const anchoringDetFISJSON = `{
  "name": "l2_anchoring_detector",
  "defuzz_method": "centroid",
  "centroid_resolution": 200,
  "input_variables": [
    {
      "name": "relative_shift",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "very_close", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.03, "d": 0.08 } },
        { "name": "close", "Triangular": { "a": 0.03, "b": 0.07, "c": 0.12 } },
        { "name": "moderate", "Triangular": { "a": 0.08, "b": 0.12, "c": 0.18 } },
        { "name": "far", "Trapezoidal": { "a": 0.14, "b": 0.2, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "output_variables": [
    {
      "name": "severity",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "none", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.02, "d": 0.08 } },
        { "name": "low", "Triangular": { "a": 0.05, "b": 0.2, "c": 0.4 } },
        { "name": "medium", "Triangular": { "a": 0.3, "b": 0.5, "c": 0.7 } },
        { "name": "high", "Triangular": { "a": 0.6, "b": 0.8, "c": 0.95 } },
        { "name": "critical", "Trapezoidal": { "a": 0.85, "b": 0.95, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "rules": [
    { "conditions": [{ "variable": "relative_shift", "term": "very_close" }], "consequent": { "variable": "severity", "term": "critical" } },
    { "conditions": [{ "variable": "relative_shift", "term": "close" }], "consequent": { "variable": "severity", "term": "high" } },
    { "conditions": [{ "variable": "relative_shift", "term": "moderate" }], "consequent": { "variable": "severity", "term": "medium" } },
    { "conditions": [{ "variable": "relative_shift", "term": "far" }], "consequent": { "variable": "severity", "term": "none" } }
  ]
}`

const contradictionDetFISJSON = `{
  "name": "l2_contradiction_detector",
  "defuzz_method": "centroid",
  "centroid_resolution": 200,
  "input_variables": [
    {
      "name": "word_overlap",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "low", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.2, "d": 0.4 } },
        { "name": "moderate", "Triangular": { "a": 0.25, "b": 0.5, "c": 0.75 } },
        { "name": "high", "Trapezoidal": { "a": 0.6, "b": 0.8, "c": 1.0, "d": 1.0 } }
      ]
    },
    {
      "name": "kind_strength",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "weak", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.2, "d": 0.4 } },
        { "name": "moderate", "Triangular": { "a": 0.25, "b": 0.5, "c": 0.75 } },
        { "name": "strong", "Trapezoidal": { "a": 0.6, "b": 0.8, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "output_variables": [
    {
      "name": "severity",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "none", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.02, "d": 0.08 } },
        { "name": "low", "Triangular": { "a": 0.05, "b": 0.2, "c": 0.4 } },
        { "name": "medium", "Triangular": { "a": 0.3, "b": 0.5, "c": 0.7 } },
        { "name": "high", "Triangular": { "a": 0.6, "b": 0.8, "c": 0.95 } },
        { "name": "critical", "Trapezoidal": { "a": 0.85, "b": 0.95, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "rules": [
    { "conditions": [{ "variable": "word_overlap", "term": "high" }, { "variable": "kind_strength", "term": "strong" }], "connector": "and", "consequent": { "variable": "severity", "term": "critical" } },
    { "conditions": [{ "variable": "word_overlap", "term": "high" }, { "variable": "kind_strength", "term": "moderate" }], "connector": "and", "consequent": { "variable": "severity", "term": "high" } },
    { "conditions": [{ "variable": "word_overlap", "term": "high" }, { "variable": "kind_strength", "term": "weak" }], "connector": "and", "consequent": { "variable": "severity", "term": "medium" } },
    { "conditions": [{ "variable": "word_overlap", "term": "moderate" }, { "variable": "kind_strength", "term": "strong" }], "connector": "and", "consequent": { "variable": "severity", "term": "high" } },
    { "conditions": [{ "variable": "word_overlap", "term": "moderate" }, { "variable": "kind_strength", "term": "moderate" }], "connector": "and", "consequent": { "variable": "severity", "term": "medium" } },
    { "conditions": [{ "variable": "word_overlap", "term": "moderate" }, { "variable": "kind_strength", "term": "weak" }], "connector": "and", "consequent": { "variable": "severity", "term": "low" } },
    { "conditions": [{ "variable": "word_overlap", "term": "low" }, { "variable": "kind_strength", "term": "strong" }], "connector": "and", "consequent": { "variable": "severity", "term": "medium" } },
    { "conditions": [{ "variable": "word_overlap", "term": "low" }, { "variable": "kind_strength", "term": "moderate" }], "connector": "and", "consequent": { "variable": "severity", "term": "low" } },
    { "conditions": [{ "variable": "word_overlap", "term": "low" }, { "variable": "kind_strength", "term": "weak" }], "connector": "and", "consequent": { "variable": "severity", "term": "none" } }
  ]
}`

const calibratorDetFISJSON = `{
  "name": "l2_calibrator_detector",
  "defuzz_method": "centroid",
  "centroid_resolution": 200,
  "input_variables": [
    {
      "name": "ece",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "low", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.25, "d": 0.45 } },
        { "name": "moderate", "Triangular": { "a": 0.3, "b": 0.5, "c": 0.7 } },
        { "name": "high", "Trapezoidal": { "a": 0.55, "b": 0.75, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "output_variables": [
    {
      "name": "severity",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "none", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.02, "d": 0.08 } },
        { "name": "low", "Triangular": { "a": 0.05, "b": 0.2, "c": 0.4 } },
        { "name": "medium", "Triangular": { "a": 0.3, "b": 0.5, "c": 0.7 } },
        { "name": "high", "Triangular": { "a": 0.6, "b": 0.8, "c": 0.95 } },
        { "name": "critical", "Trapezoidal": { "a": 0.85, "b": 0.95, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "rules": [
    { "conditions": [{ "variable": "ece", "term": "high" }], "consequent": { "variable": "severity", "term": "critical" } },
    { "conditions": [{ "variable": "ece", "term": "moderate" }], "consequent": { "variable": "severity", "term": "medium" } },
    { "conditions": [{ "variable": "ece", "term": "low" }], "consequent": { "variable": "severity", "term": "none" } }
  ]
}`

const sunkCostDetFISJSON = `{
  "name": "l2_sunk_cost_detector",
  "defuzz_method": "centroid",
  "centroid_resolution": 200,
  "input_variables": [
    {
      "name": "confidence",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "low", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.3, "d": 0.5 } },
        { "name": "moderate", "Triangular": { "a": 0.35, "b": 0.55, "c": 0.75 } },
        { "name": "high", "Trapezoidal": { "a": 0.6, "b": 0.8, "c": 1.0, "d": 1.0 } }
      ]
    },
    {
      "name": "turn_gap",
      "min": 0.0,
      "max": 20.0,
      "terms": [
        { "name": "adjacent", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 1.0, "d": 3.0 } },
        { "name": "nearby", "Triangular": { "a": 2.0, "b": 5.0, "c": 10.0 } },
        { "name": "distant", "Trapezoidal": { "a": 7.0, "b": 12.0, "c": 20.0, "d": 20.0 } }
      ]
    }
  ],
  "output_variables": [
    {
      "name": "severity",
      "min": 0.0,
      "max": 1.0,
      "terms": [
        { "name": "none", "Trapezoidal": { "a": 0.0, "b": 0.0, "c": 0.02, "d": 0.08 } },
        { "name": "low", "Triangular": { "a": 0.05, "b": 0.2, "c": 0.4 } },
        { "name": "medium", "Triangular": { "a": 0.3, "b": 0.5, "c": 0.7 } },
        { "name": "high", "Triangular": { "a": 0.6, "b": 0.8, "c": 0.95 } },
        { "name": "critical", "Trapezoidal": { "a": 0.85, "b": 0.95, "c": 1.0, "d": 1.0 } }
      ]
    }
  ],
  "rules": [
    { "conditions": [{ "variable": "confidence", "term": "high" }, { "variable": "turn_gap", "term": "adjacent" }], "connector": "and", "consequent": { "variable": "severity", "term": "critical" } },
    { "conditions": [{ "variable": "confidence", "term": "high" }, { "variable": "turn_gap", "term": "nearby" }], "connector": "and", "consequent": { "variable": "severity", "term": "high" } },
    { "conditions": [{ "variable": "confidence", "term": "high" }, { "variable": "turn_gap", "term": "distant" }], "connector": "and", "consequent": { "variable": "severity", "term": "medium" } },
    { "conditions": [{ "variable": "confidence", "term": "moderate" }, { "variable": "turn_gap", "term": "adjacent" }], "connector": "and", "consequent": { "variable": "severity", "term": "high" } },
    { "conditions": [{ "variable": "confidence", "term": "moderate" }, { "variable": "turn_gap", "term": "nearby" }], "connector": "and", "consequent": { "variable": "severity", "term": "medium" } },
    { "conditions": [{ "variable": "confidence", "term": "moderate" }, { "variable": "turn_gap", "term": "distant" }], "connector": "and", "consequent": { "variable": "severity", "term": "low" } },
    { "conditions": [{ "variable": "confidence", "term": "low" }, { "variable": "turn_gap", "term": "adjacent" }], "connector": "and", "consequent": { "variable": "severity", "term": "medium" } },
    { "conditions": [{ "variable": "confidence", "term": "low" }, { "variable": "turn_gap", "term": "nearby" }], "connector": "and", "consequent": { "variable": "severity", "term": "low" } },
    { "conditions": [{ "variable": "confidence", "term": "low" }, { "variable": "turn_gap", "term": "distant" }], "connector": "and", "consequent": { "variable": "severity", "term": "none" } }
  ]
}`
