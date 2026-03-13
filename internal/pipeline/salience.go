package pipeline

import (
	"fmt"
	"sort"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// SalienceConfig controls the Salience Filter scoring weights and thresholds.
type SalienceConfig struct {
	NoveltyWeight       float64
	ActionabilityWeight float64
	SeverityWeight      float64
	MinSalience         float64
	MaxFindings         int
}

// DefaultSalienceConfig returns production defaults for salience filtering.
func DefaultSalienceConfig() SalienceConfig {
	return SalienceConfig{
		NoveltyWeight:       0.4,
		ActionabilityWeight: 0.4,
		SeverityWeight:      0.2,
		MinSalience:         0.3,
		MaxFindings:         10,
	}
}

// SalienceResult holds per-finding scores and the filtered salient assessments.
type SalienceResult struct {
	Scores  []*cerebrov1.SalienceScore
	Salient []*reasoningv1.CognitiveAssessment
}

// FilterSalience scores findings by novelty, actionability, and severity,
// then filters to the top salient items above the configured threshold.
func FilterSalience(assessments []*reasoningv1.CognitiveAssessment, cfg SalienceConfig) *SalienceResult {
	if len(assessments) == 0 {
		return &SalienceResult{}
	}

	// Count findings per type for novelty calculation.
	typeCounts := make(map[reasoningv1.FindingType]int)
	for _, a := range assessments {
		typeCounts[a.GetFindingType()]++
	}

	type scored struct {
		score      *cerebrov1.SalienceScore
		assessment *reasoningv1.CognitiveAssessment
		salience   float64
	}

	all := make([]scored, 0, len(assessments))

	for _, a := range assessments {
		// 1. Novelty: inverse of same-type count.
		sameTypeCount := typeCounts[a.GetFindingType()]
		novelty := 1.0 / float64(sameTypeCount)

		// 2. Actionability: evidence-based scoring.
		actionability := 0.0
		if hasSpecificEvidence(a) {
			actionability += 0.3
		}
		if len(a.GetRelevantTurns()) > 0 {
			actionability += 0.3
		}
		if len(a.GetExplanation()) > 50 {
			actionability += 0.2
		}
		if a.GetConfidence() > 0.7 {
			actionability += 0.2
		}

		// 3. Severity normalized.
		severityNorm := normalizeSeverity(a.GetSeverity())

		// 4. Composite salience.
		salience := cfg.NoveltyWeight*novelty +
			cfg.ActionabilityWeight*actionability +
			cfg.SeverityWeight*severityNorm

		findingID := fmt.Sprintf("%s:%v", a.GetDetectorName(), a.GetRelevantTurns())

		s := &cerebrov1.SalienceScore{
			FindingId:      findingID,
			Score:          salience,
			Novelty:        novelty,
			Actionability:  actionability,
			AboveThreshold: salience >= cfg.MinSalience,
		}

		all = append(all, scored{score: s, assessment: a, salience: salience})
	}

	// Sort by salience descending.
	sort.Slice(all, func(i, j int) bool {
		return all[i].salience > all[j].salience
	})

	result := &SalienceResult{
		Scores: make([]*cerebrov1.SalienceScore, len(all)),
	}

	for i, s := range all {
		result.Scores[i] = s.score
	}

	// Keep top MaxFindings that are above threshold.
	for _, s := range all {
		if s.salience < cfg.MinSalience {
			continue
		}
		if len(result.Salient) >= cfg.MaxFindings {
			break
		}
		result.Salient = append(result.Salient, s.assessment)
	}

	return result
}

// hasSpecificEvidence returns true if the assessment contains structured
// evidence in its detail sub-messages (anchoring, contradiction, or sunk cost).
func hasSpecificEvidence(a *reasoningv1.CognitiveAssessment) bool {
	if anch := a.GetAnchoring(); anch != nil {
		// AnchoringDetail with any populated claim data.
		if anch.GetAnchorValue() != 0 || anch.GetEstimateValue() != 0 {
			return true
		}
	}
	if ctr := a.GetContradiction(); ctr != nil {
		if ctr.GetClaimAText() != "" || ctr.GetClaimBText() != "" {
			return true
		}
	}
	if sc := a.GetSunkCost(); sc != nil {
		if sc.GetCostReference() != "" {
			return true
		}
	}
	return false
}

// normalizeSeverity maps FindingSeverity enum values to 0.0-1.0.
func normalizeSeverity(s reasoningv1.FindingSeverity) float64 {
	switch s {
	case reasoningv1.FindingSeverity_INFO: // 1
		return 0.25
	case reasoningv1.FindingSeverity_CAUTION: // 2
		return 0.5
	case reasoningv1.FindingSeverity_WARNING: // 3
		return 0.75
	case reasoningv1.FindingSeverity_CRITICAL: // 4
		return 1.0
	default: // UNSPECIFIED = 0
		return 0.25
	}
}
