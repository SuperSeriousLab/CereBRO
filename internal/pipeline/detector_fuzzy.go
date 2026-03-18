package pipeline

import (
	"fmt"

	"github.com/SuperSeriousLab/fugo"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// FIS config names used in the FisRegistry for L2 detector fuzzy evaluation.
const (
	FISAnchoring     = "l2_anchoring_detector"
	FISContradiction = "l2_contradiction_detector"
	FISCalibrator    = "l2_calibrator_detector"
	FISSunkCost      = "l2_sunk_cost_detector"
)

// DetectorFuzzy holds pre-built fugo engines for L2 detector fuzzy severity evaluation.
// When nil, detectors fall back to existing crisp threshold logic.
type DetectorFuzzy struct {
	AnchoringEngine     *fugo.FuzzyEngine
	ContradictionEngine *fugo.FuzzyEngine
	CalibratorEngine    *fugo.FuzzyEngine
	SunkCostEngine      *fugo.FuzzyEngine
}

// BuildDetectorFuzzy constructs a DetectorFuzzy from individual FIS configs.
// Returns nil and an error if any config fails to build.
func BuildDetectorFuzzy(anchoringCfg, contradictionCfg, calibratorCfg, sunkCostCfg *fugo.FisConfig) (*DetectorFuzzy, error) {
	ae, err := anchoringCfg.BuildEngine()
	if err != nil {
		return nil, fmt.Errorf("anchoring FIS: %w", err)
	}
	ce, err := contradictionCfg.BuildEngine()
	if err != nil {
		return nil, fmt.Errorf("contradiction FIS: %w", err)
	}
	cae, err := calibratorCfg.BuildEngine()
	if err != nil {
		return nil, fmt.Errorf("calibrator FIS: %w", err)
	}
	se, err := sunkCostCfg.BuildEngine()
	if err != nil {
		return nil, fmt.Errorf("sunk-cost FIS: %w", err)
	}
	return &DetectorFuzzy{
		AnchoringEngine:     ae,
		ContradictionEngine: ce,
		CalibratorEngine:    cae,
		SunkCostEngine:      se,
	}, nil
}

// BuildDetectorFuzzyFromRegistry constructs a DetectorFuzzy from a FisRegistry.
// Looks up configs by the standard L2 detector names.
func BuildDetectorFuzzyFromRegistry(reg *fugo.FisRegistry) (*DetectorFuzzy, error) {
	ae, err := reg.BuildEngine(FISAnchoring)
	if err != nil {
		return nil, fmt.Errorf("anchoring FIS: %w", err)
	}
	ce, err := reg.BuildEngine(FISContradiction)
	if err != nil {
		return nil, fmt.Errorf("contradiction FIS: %w", err)
	}
	cae, err := reg.BuildEngine(FISCalibrator)
	if err != nil {
		return nil, fmt.Errorf("calibrator FIS: %w", err)
	}
	se, err := reg.BuildEngine(FISSunkCost)
	if err != nil {
		return nil, fmt.Errorf("sunk-cost FIS: %w", err)
	}
	return &DetectorFuzzy{
		AnchoringEngine:     ae,
		ContradictionEngine: ce,
		CalibratorEngine:    cae,
		SunkCostEngine:      se,
	}, nil
}

// evaluateAnchoringSeverity computes fuzzy severity for anchoring detection.
// Input: relative_shift [0,1]. Output: severity [0,1].
// Returns 0 and false if the engine is nil or evaluation fails.
func (df *DetectorFuzzy) evaluateAnchoringSeverity(relativeShift float64) (float64, bool) {
	if df == nil || df.AnchoringEngine == nil {
		return 0, false
	}
	outputs, err := df.AnchoringEngine.Evaluate(map[string]float64{
		"relative_shift": relativeShift,
	})
	if err != nil {
		return 0, false
	}
	sev, ok := outputs["severity"]
	return sev, ok
}

// evaluateContradictionSeverity computes fuzzy severity for contradiction detection.
// Inputs: word_overlap [0,1], kind_strength [0,1]. Output: severity [0,1].
func (df *DetectorFuzzy) evaluateContradictionSeverity(wordOverlap, kindStrength float64) (float64, bool) {
	if df == nil || df.ContradictionEngine == nil {
		return 0, false
	}
	outputs, err := df.ContradictionEngine.Evaluate(map[string]float64{
		"word_overlap":   wordOverlap,
		"kind_strength":  kindStrength,
	})
	if err != nil {
		return 0, false
	}
	sev, ok := outputs["severity"]
	return sev, ok
}

// evaluateCalibratorSeverity computes fuzzy severity for confidence calibration.
// Input: ece [0,1] (expected calibration error). Output: severity [0,1].
func (df *DetectorFuzzy) evaluateCalibratorSeverity(ece float64) (float64, bool) {
	if df == nil || df.CalibratorEngine == nil {
		return 0, false
	}
	outputs, err := df.CalibratorEngine.Evaluate(map[string]float64{
		"ece": ece,
	})
	if err != nil {
		return 0, false
	}
	sev, ok := outputs["severity"]
	return sev, ok
}

// evaluateSunkCostSeverity computes fuzzy severity for sunk-cost detection.
// Inputs: confidence [0,1], turn_gap [0,20]. Output: severity [0,1].
func (df *DetectorFuzzy) evaluateSunkCostSeverity(confidence float64, turnGap float64) (float64, bool) {
	if df == nil || df.SunkCostEngine == nil {
		return 0, false
	}
	// Clamp turn_gap to FIS range.
	if turnGap > 20.0 {
		turnGap = 20.0
	}
	outputs, err := df.SunkCostEngine.Evaluate(map[string]float64{
		"confidence": confidence,
		"turn_gap":   turnGap,
	})
	if err != nil {
		return 0, false
	}
	sev, ok := outputs["severity"]
	return sev, ok
}

// fuzzySuppressionThreshold is the minimum fuzzy severity for a finding to be reported.
// Findings with severity below this are suppressed (not reported).
const fuzzySuppressionThreshold = 0.1

// applyAnchoringFuzzy replaces crisp confidence with fuzzy severity on an anchoring finding.
// If df is nil or evaluation fails, returns the finding unchanged (crisp fallback).
func applyAnchoringFuzzy(finding *reasoningv1.CognitiveAssessment, df *DetectorFuzzy) *reasoningv1.CognitiveAssessment {
	if finding == nil || df == nil {
		return finding
	}
	// Use the anchoring detail's relative shift as FIS input.
	ad := finding.GetAnchoring()
	if ad == nil {
		return finding
	}
	sev, ok := df.evaluateAnchoringSeverity(ad.GetRelativeShift())
	if !ok {
		return finding
	}
	if sev < fuzzySuppressionThreshold {
		return nil // suppressed
	}
	finding.Confidence = sev
	return finding
}

// applyContradictionFuzzy replaces crisp confidence with fuzzy severity on a contradiction finding.
// Recomputes word_overlap and kind_strength from the finding's contradiction detail.
func applyContradictionFuzzy(finding *reasoningv1.CognitiveAssessment, snap *reasoningv1.ConversationSnapshot, cfg ContradictionConfig, df *DetectorFuzzy) *reasoningv1.CognitiveAssessment {
	if finding == nil || df == nil {
		return finding
	}
	cd := finding.GetContradiction()
	if cd == nil {
		return finding
	}
	// Recompute signals from the contradiction detail.
	overlap := wordOverlap(cd.GetClaimAText(), cd.GetClaimBText())
	kind := detectContradictionKind(cd.GetClaimAText(), cd.GetClaimBText())
	kindStrength := contradictionKindStrength(kind)

	sev, ok := df.evaluateContradictionSeverity(overlap, kindStrength)
	if !ok {
		return finding
	}
	if sev < fuzzySuppressionThreshold {
		return nil // suppressed
	}
	finding.Confidence = sev
	return finding
}

// contradictionKindStrength maps contradiction kind to a fuzzy strength [0,1].
func contradictionKindStrength(kind string) float64 {
	switch kind {
	case "negation":
		return 0.9
	case "reversal":
		return 0.7
	case "antonym":
		return 0.5
	default:
		return 0.0
	}
}

// applyCalibratorFuzzy replaces crisp confidence with fuzzy severity on a calibration finding.
func applyCalibratorFuzzy(finding *reasoningv1.CognitiveAssessment, df *DetectorFuzzy) *reasoningv1.CognitiveAssessment {
	if finding == nil || df == nil {
		return finding
	}
	cal := finding.GetCalibration()
	if cal == nil {
		// ML-only findings don't have CalibrationDetail; use existing confidence as ECE proxy.
		sev, ok := df.evaluateCalibratorSeverity(finding.GetConfidence())
		if !ok {
			return finding
		}
		if sev < fuzzySuppressionThreshold {
			return nil
		}
		finding.Confidence = sev
		return finding
	}
	sev, ok := df.evaluateCalibratorSeverity(cal.GetExpectedCalibrationError())
	if !ok {
		return finding
	}
	if sev < fuzzySuppressionThreshold {
		return nil // suppressed
	}
	finding.Confidence = sev
	return finding
}

// applySunkCostFuzzy replaces crisp confidence with fuzzy severity on a sunk-cost finding.
func applySunkCostFuzzy(finding *reasoningv1.CognitiveAssessment, df *DetectorFuzzy) *reasoningv1.CognitiveAssessment {
	if finding == nil || df == nil {
		return finding
	}
	sc := finding.GetSunkCost()
	if sc == nil {
		return finding
	}
	turnGap := float64(0)
	if sc.GetDecisionTurn() > sc.GetCostTurn() {
		turnGap = float64(sc.GetDecisionTurn() - sc.GetCostTurn())
	}
	sev, ok := df.evaluateSunkCostSeverity(finding.GetConfidence(), turnGap)
	if !ok {
		return finding
	}
	if sev < fuzzySuppressionThreshold {
		return nil // suppressed
	}
	finding.Confidence = sev
	return finding
}
