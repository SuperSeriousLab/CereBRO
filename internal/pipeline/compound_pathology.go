package pipeline

// Package pipeline — Tier 3 Compound Pathology Aggregator.
//
// CompoundPathologyAggregator is the first Tier 3 meta-COG. It reads signals
// from all active Tier 1 and Tier 2 COG findings and computes a compound
// pathology risk score using a Mamdani FIS (fugo).
//
// Inputs to the FIS:
//   - active_detector_count  — how many detectors fired (normalized 0-1, max 10)
//   - max_single_severity    — highest severity/confidence among fired detectors (0-1)
//   - avg_confidence         — mean confidence across all fired detectors (0-1)
//
// Output:
//   - compound_risk (0-1)
//
// When compound_risk > 0.6, a COMPOUND_PATHOLOGY finding is emitted with
// severity = compound_risk.
//
// Pipeline ordering: this COG is intended to run AFTER all other COGs have
// produced findings. Currently it is wired as a post-aggregation step in the
// pipeline (Stage 5.5) alongside the CrossLayerArbitrator.
//
// TODO(ordering): formalize a "Tier 3 slot" in the pipeline with guaranteed
// ordering — compound-pathology always runs last among assessment COGs.

import (
	"fmt"

	"github.com/SuperSeriousLab/fugo"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// FIS config name for the Tier 3 compound pathology aggregator.
const FISCompoundPathology = "tier3_compound_pathology"

// compoundPathologyThreshold is the compound_risk above which a
// COMPOUND_PATHOLOGY finding is emitted.
const compoundPathologyThreshold = 0.6

// maxDetectorCount is the maximum number of detectors assumed when normalizing
// active_detector_count to [0, 1].
const maxDetectorCount = 10.0

// CompoundPathologyConfig holds configuration for the Tier 3 aggregator.
type CompoundPathologyConfig struct {
	// EmitThreshold is the compound_risk score above which a COMPOUND_PATHOLOGY
	// finding is emitted. Default: 0.6.
	EmitThreshold float64
}

// DefaultCompoundPathologyConfig returns production defaults.
func DefaultCompoundPathologyConfig() CompoundPathologyConfig {
	return CompoundPathologyConfig{
		EmitThreshold: compoundPathologyThreshold,
	}
}

// CompoundPathologyResult holds the output of the Tier 3 aggregator.
type CompoundPathologyResult struct {
	// CompoundRisk is the Mamdani FIS output (0-1).
	CompoundRisk float64

	// ActiveDetectorCount is the raw count of detectors that fired.
	ActiveDetectorCount int

	// MaxSingleSeverity is the highest severity/confidence among fired detectors.
	MaxSingleSeverity float64

	// AvgConfidence is the mean confidence across all fired detectors.
	AvgConfidence float64

	// Finding is non-nil when compound_risk > threshold.
	Finding *reasoningv1.CognitiveAssessment
}

// CompoundPathologyAggregator is the Tier 3 meta-COG.
// It holds a pre-built fugo engine and is configured via CompoundPathologyConfig.
type CompoundPathologyAggregator struct {
	Engine *fugo.FuzzyEngine
	Config CompoundPathologyConfig
}

// BuildCompoundPathologyAggregator constructs an aggregator from a FIS config.
func BuildCompoundPathologyAggregator(cfg *fugo.FisConfig, aggCfg CompoundPathologyConfig) (*CompoundPathologyAggregator, error) {
	e, err := cfg.BuildEngine()
	if err != nil {
		return nil, fmt.Errorf("compound-pathology FIS: %w", err)
	}
	return &CompoundPathologyAggregator{Engine: e, Config: aggCfg}, nil
}

// BuildCompoundPathologyAggregatorFromRegistry constructs an aggregator from a FisRegistry.
func BuildCompoundPathologyAggregatorFromRegistry(reg *fugo.FisRegistry, aggCfg CompoundPathologyConfig) (*CompoundPathologyAggregator, error) {
	e, err := reg.BuildEngine(FISCompoundPathology)
	if err != nil {
		return nil, fmt.Errorf("compound-pathology FIS: %w", err)
	}
	return &CompoundPathologyAggregator{Engine: e, Config: aggCfg}, nil
}

// NewDefaultCompoundPathologyAggregator constructs an aggregator with the
// embedded default FIS config. This is the recommended entry point for most
// pipeline usages. Returns an error only if the embedded FIS is malformed
// (should not happen in production).
func NewDefaultCompoundPathologyAggregator() (*CompoundPathologyAggregator, error) {
	engine, err := buildDefaultCompoundPathologyEngine()
	if err != nil {
		return nil, fmt.Errorf("build default compound-pathology engine: %w", err)
	}
	return &CompoundPathologyAggregator{
		Engine: engine,
		Config: DefaultCompoundPathologyConfig(),
	}, nil
}

// Aggregate computes a compound pathology score from a set of findings.
//
// It is designed to run after all Tier 1/Tier 2 COGs have produced their
// assessments. The input findings slice should contain the full set of
// pre-inhibition raw findings for maximum signal coverage.
//
// When the aggregator is nil, returns a zero-risk result (passthrough).
func (a *CompoundPathologyAggregator) Aggregate(
	findings []*reasoningv1.CognitiveAssessment,
) *CompoundPathologyResult {
	if a == nil || a.Engine == nil {
		return &CompoundPathologyResult{}
	}

	if len(findings) == 0 {
		return &CompoundPathologyResult{}
	}

	activeCount, maxSev, avgConf := computeCompoundInputs(findings)

	// Normalize active_detector_count to [0, 1].
	normalizedCount := clamp(float64(activeCount)/maxDetectorCount, 0.0, 1.0)

	outputs, err := a.Engine.Evaluate(map[string]float64{
		"active_detector_count": normalizedCount,
		"max_single_severity":   maxSev,
		"avg_confidence":        avgConf,
	})
	if err != nil {
		// FIS evaluation error — return neutral result.
		return &CompoundPathologyResult{
			ActiveDetectorCount: activeCount,
			MaxSingleSeverity:   maxSev,
			AvgConfidence:       avgConf,
		}
	}

	risk, ok := outputs["compound_risk"]
	if !ok {
		return &CompoundPathologyResult{
			ActiveDetectorCount: activeCount,
			MaxSingleSeverity:   maxSev,
			AvgConfidence:       avgConf,
		}
	}

	result := &CompoundPathologyResult{
		CompoundRisk:        risk,
		ActiveDetectorCount: activeCount,
		MaxSingleSeverity:   maxSev,
		AvgConfidence:       avgConf,
	}

	threshold := a.Config.EmitThreshold
	if threshold == 0 {
		threshold = compoundPathologyThreshold
	}

	if risk > threshold {
		result.Finding = buildCompoundPathologyFinding(risk, activeCount, maxSev, avgConf)
	}

	return result
}

// computeCompoundInputs extracts the three FIS input signals from findings.
//
//   - activeCount: total number of findings (distinct detectors that fired)
//   - maxSev: highest effective severity among findings (0-1)
//   - avgConf: mean confidence across all findings (0-1)
func computeCompoundInputs(findings []*reasoningv1.CognitiveAssessment) (int, float64, float64) {
	activeCount := len(findings)
	var maxSev float64
	var confSum float64

	for _, f := range findings {
		conf := f.GetConfidence()
		// Boost confidence with proto severity ordinal if it is higher.
		// CRITICAL=1.0, WARNING=0.75, CAUTION=0.5, INFO=0.25.
		sevBoost := float64(f.GetSeverity()) * 0.25
		effective := conf
		if sevBoost > effective {
			effective = sevBoost
		}
		if effective > maxSev {
			maxSev = effective
		}
		confSum += conf
	}

	avgConf := 0.0
	if activeCount > 0 {
		avgConf = confSum / float64(activeCount)
	}

	return activeCount, clamp(maxSev, 0.0, 1.0), clamp(avgConf, 0.0, 1.0)
}

// buildCompoundPathologyFinding constructs a CognitiveAssessment for a
// COMPOUND_PATHOLOGY finding.
func buildCompoundPathologyFinding(
	risk float64,
	activeCount int,
	maxSev float64,
	avgConf float64,
) *reasoningv1.CognitiveAssessment {
	severity := reasoningv1.FindingSeverity_WARNING
	if risk > 0.85 {
		severity = reasoningv1.FindingSeverity_CRITICAL
	}

	return &reasoningv1.CognitiveAssessment{
		FindingType:  reasoningv1.FindingType_COMPOUND_PATHOLOGY,
		Severity:     severity,
		Confidence:   risk,
		DetectorName: "compound-pathology-aggregator",
		Explanation: fmt.Sprintf(
			"Compound pathology detected: %d detectors fired (max_severity=%.2f, avg_confidence=%.2f) → compound_risk=%.3f",
			activeCount, maxSev, avgConf, risk,
		),
	}
}

// buildDefaultCompoundPathologyEngine constructs a fugo.FuzzyEngine from the
// embedded Mamdani FIS definition.
//
// Rule set (7 rules):
//
//	R1: IF active_count IS high   AND max_severity IS high   → compound_risk IS critical
//	R2: IF active_count IS high   AND max_severity IS medium → compound_risk IS elevated
//	R3: IF active_count IS medium AND avg_confidence IS high  → compound_risk IS elevated
//	R4: IF active_count IS medium AND max_severity IS high   → compound_risk IS elevated
//	R5: IF active_count IS low    AND max_severity IS high   → compound_risk IS moderate
//	R6: IF active_count IS low    AND avg_confidence IS low   → compound_risk IS nominal
//	R7: IF active_count IS low                               → compound_risk IS nominal
func buildDefaultCompoundPathologyEngine() (*fugo.FuzzyEngine, error) {
	// Input: active_detector_count (normalized 0-1, where 1.0 = 10+ detectors)
	activeCount := fugo.FuzzyVariable{
		Name: "active_detector_count",
		Min:  0.0,
		Max:  1.0,
		Terms: []fugo.Term{
			{Name: "low", MF: fugo.Trapezoidal{A: 0.0, B: 0.0, C: 0.15, D: 0.35}},
			{Name: "medium", MF: fugo.Triangular{A: 0.2, B: 0.45, C: 0.7}},
			{Name: "high", MF: fugo.Trapezoidal{A: 0.55, B: 0.75, C: 1.0, D: 1.0}},
		},
	}

	// Input: max_single_severity (0-1)
	maxSeverity := fugo.FuzzyVariable{
		Name: "max_single_severity",
		Min:  0.0,
		Max:  1.0,
		Terms: []fugo.Term{
			{Name: "low", MF: fugo.Trapezoidal{A: 0.0, B: 0.0, C: 0.25, D: 0.45}},
			{Name: "medium", MF: fugo.Triangular{A: 0.3, B: 0.55, C: 0.75}},
			{Name: "high", MF: fugo.Trapezoidal{A: 0.6, B: 0.8, C: 1.0, D: 1.0}},
		},
	}

	// Input: avg_confidence (0-1)
	avgConf := fugo.FuzzyVariable{
		Name: "avg_confidence",
		Min:  0.0,
		Max:  1.0,
		Terms: []fugo.Term{
			{Name: "low", MF: fugo.Trapezoidal{A: 0.0, B: 0.0, C: 0.25, D: 0.45}},
			{Name: "medium", MF: fugo.Triangular{A: 0.3, B: 0.5, C: 0.7}},
			{Name: "high", MF: fugo.Trapezoidal{A: 0.55, B: 0.75, C: 1.0, D: 1.0}},
		},
	}

	// Output: compound_risk (0-1)
	compoundRisk := fugo.FuzzyVariable{
		Name: "compound_risk",
		Min:  0.0,
		Max:  1.0,
		Terms: []fugo.Term{
			{Name: "nominal", MF: fugo.Trapezoidal{A: 0.0, B: 0.0, C: 0.1, D: 0.3}},
			{Name: "moderate", MF: fugo.Triangular{A: 0.2, B: 0.4, C: 0.6}},
			{Name: "elevated", MF: fugo.Triangular{A: 0.5, B: 0.7, C: 0.85}},
			{Name: "critical", MF: fugo.Trapezoidal{A: 0.75, B: 0.9, C: 1.0, D: 1.0}},
		},
	}

	rules := []fugo.FuzzyRule{
		// R1: High detector count + high severity → critical risk
		{
			Conditions: []fugo.Condition{
				{Variable: "active_detector_count", Term: "high"},
				{Variable: "max_single_severity", Term: "high"},
			},
			Connector:  fugo.ConnectorAnd,
			Consequent: fugo.Consequent{Variable: "compound_risk", Term: "critical"},
			Weight:     1.0,
		},
		// R2: High detector count + medium severity → elevated risk
		{
			Conditions: []fugo.Condition{
				{Variable: "active_detector_count", Term: "high"},
				{Variable: "max_single_severity", Term: "medium"},
			},
			Connector:  fugo.ConnectorAnd,
			Consequent: fugo.Consequent{Variable: "compound_risk", Term: "elevated"},
			Weight:     1.0,
		},
		// R3: Medium detector count + high confidence → elevated risk
		{
			Conditions: []fugo.Condition{
				{Variable: "active_detector_count", Term: "medium"},
				{Variable: "avg_confidence", Term: "high"},
			},
			Connector:  fugo.ConnectorAnd,
			Consequent: fugo.Consequent{Variable: "compound_risk", Term: "elevated"},
			Weight:     1.0,
		},
		// R4: Medium detector count + high severity → elevated risk
		{
			Conditions: []fugo.Condition{
				{Variable: "active_detector_count", Term: "medium"},
				{Variable: "max_single_severity", Term: "high"},
			},
			Connector:  fugo.ConnectorAnd,
			Consequent: fugo.Consequent{Variable: "compound_risk", Term: "elevated"},
			Weight:     1.0,
		},
		// R5: Low detector count + high severity → moderate risk
		{
			Conditions: []fugo.Condition{
				{Variable: "active_detector_count", Term: "low"},
				{Variable: "max_single_severity", Term: "high"},
			},
			Connector:  fugo.ConnectorAnd,
			Consequent: fugo.Consequent{Variable: "compound_risk", Term: "moderate"},
			Weight:     1.0,
		},
		// R6: Low detector count + low confidence → nominal risk
		{
			Conditions: []fugo.Condition{
				{Variable: "active_detector_count", Term: "low"},
				{Variable: "avg_confidence", Term: "low"},
			},
			Connector:  fugo.ConnectorAnd,
			Consequent: fugo.Consequent{Variable: "compound_risk", Term: "nominal"},
			Weight:     1.0,
		},
		// R7: Low detector count alone → nominal risk (lower weight, catch-all)
		{
			Conditions: []fugo.Condition{
				{Variable: "active_detector_count", Term: "low"},
			},
			Connector:  fugo.ConnectorAnd,
			Consequent: fugo.Consequent{Variable: "compound_risk", Term: "nominal"},
			Weight:     0.7,
		},
	}

	engine := &fugo.FuzzyEngine{
		InputVars:          []fugo.FuzzyVariable{activeCount, maxSeverity, avgConf},
		OutputVars:         []fugo.FuzzyVariable{compoundRisk},
		Rules:              rules,
		DefuzzMethod:       fugo.DefuzzCentroid,
		CentroidResolution: 200,
	}

	return engine, nil
}
