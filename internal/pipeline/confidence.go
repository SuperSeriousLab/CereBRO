// Self-Confidence Assessor — metacognitive assessment of pipeline report reliability.
//
// Three components combined with configurable weights:
//   1. Agreement score: cross-detector consistency (stdev of finding confidences)
//   2. Margin score: distance of findings from the borderline (0.5)
//   3. Historical score: accuracy on similar finding patterns (corpus lookup)
//
// Phase 4 deliverable. Brain analogue: anterior prefrontal cortex metacognitive
// monitoring (Fleming et al., 2010).
package pipeline

import (
	"math"
	"sort"
	"strings"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// SelfConfidenceConfig holds tunable parameters for the Self-Confidence Assessor.
type SelfConfidenceConfig struct {
	AgreementWeight            float64
	MarginWeight               float64
	HistoricalWeight           float64
	HighConfidenceThreshold    float64
	ModerateConfidenceThreshold float64
	PatternIndex               *PatternIndex // pre-loaded from corpus (shared with Memory Consolidator)
}

// DefaultSelfConfidenceConfig returns the default configuration.
func DefaultSelfConfidenceConfig() SelfConfidenceConfig {
	return SelfConfidenceConfig{
		AgreementWeight:            0.4,
		MarginWeight:               0.35,
		HistoricalWeight:           0.25,
		HighConfidenceThreshold:    0.8,
		ModerateConfidenceThreshold: 0.5,
	}
}

// AssessConfidence computes the self-confidence score for a pipeline report.
func AssessConfidence(
	report *reasoningv1.ReasoningReport,
	cfg SelfConfidenceConfig,
) *cerebrov1.SelfConfidenceReport {
	findings := report.GetFindings()

	// Agreement score
	agreement := computeAgreement(findings)

	// Margin score
	margin := computeMargin(findings)

	// Historical score
	pattern := ExtractFindingPattern(findings)
	historical := lookupHistoricalFromIndex(pattern, cfg.PatternIndex)

	// Composite
	overall := cfg.AgreementWeight*agreement +
		cfg.MarginWeight*margin +
		cfg.HistoricalWeight*historical

	if overall > 1.0 {
		overall = 1.0
	}
	if overall < 0.0 {
		overall = 0.0
	}

	// Recommendation
	var rec cerebrov1.ConfidenceRecommendation
	switch {
	case overall > cfg.HighConfidenceThreshold:
		rec = cerebrov1.ConfidenceRecommendation_HIGH_CONFIDENCE
	case overall > cfg.ModerateConfidenceThreshold:
		rec = cerebrov1.ConfidenceRecommendation_MODERATE_CONFIDENCE
	default:
		rec = cerebrov1.ConfidenceRecommendation_LOW_CONFIDENCE_REVIEW_RECOMMENDED
	}

	return &cerebrov1.SelfConfidenceReport{
		OverallConfidence: overall,
		AgreementScore:    agreement,
		MarginScore:       margin,
		HistoricalScore:   historical,
		FindingCount:      uint32(len(findings)),
		FindingPattern:    pattern,
		Recommendation:    rec,
	}
}

func computeAgreement(findings []*reasoningv1.CognitiveAssessment) float64 {
	if len(findings) == 0 {
		return 1.0 // Clean report — all detectors agree nothing's wrong.
	}
	if len(findings) == 1 {
		return 0.8 // Single finding — moderate agreement (no variance but no corroboration).
	}

	confidences := make([]float64, len(findings))
	for i, f := range findings {
		confidences[i] = f.GetConfidence()
	}

	sd := stdev(confidences)
	agreement := 1.0 - sd
	if agreement < 0.0 {
		agreement = 0.0
	}
	if agreement > 1.0 {
		agreement = 1.0
	}
	return agreement
}

func computeMargin(findings []*reasoningv1.CognitiveAssessment) float64 {
	if len(findings) == 0 {
		return 1.0 // No borderline findings.
	}

	var sum float64
	for _, f := range findings {
		sum += math.Abs(f.GetConfidence() - 0.5)
	}
	margin := (sum / float64(len(findings))) * 2.0
	if margin > 1.0 {
		margin = 1.0
	}
	return margin
}

// ExtractFindingPattern returns a sorted "+" delimited pattern of finding types.
func ExtractFindingPattern(findings []*reasoningv1.CognitiveAssessment) string {
	if len(findings) == 0 {
		return "CLEAN"
	}
	seen := make(map[string]bool)
	var types []string
	for _, f := range findings {
		name := f.GetFindingType().String()
		if !seen[name] {
			seen[name] = true
			types = append(types, name)
		}
	}
	sort.Strings(types)
	return strings.Join(types, "+")
}


func stdev(values []float64) float64 {
	if len(values) <= 1 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	var variance float64
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(values))
	return math.Sqrt(variance)
}
