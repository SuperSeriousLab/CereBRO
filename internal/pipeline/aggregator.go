package pipeline

import (
	"fmt"
	"sort"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var severityRank = map[reasoningv1.FindingSeverity]int{
	reasoningv1.FindingSeverity_CRITICAL: 4,
	reasoningv1.FindingSeverity_WARNING:  3,
	reasoningv1.FindingSeverity_CAUTION:  2,
	reasoningv1.FindingSeverity_INFO:     1,
}

// Aggregate combines multiple CognitiveAssessments into a ReasoningReport.
func Aggregate(assessments []*reasoningv1.CognitiveAssessment, conversationID string) *reasoningv1.ReasoningReport {
	if len(assessments) == 0 {
		return &reasoningv1.ReasoningReport{
			OverallIntegrityScore: 1.0,
			ConversationId:       conversationID,
			AssessedAt:           timestamppb.Now(),
		}
	}

	sorted := make([]*reasoningv1.CognitiveAssessment, len(assessments))
	copy(sorted, assessments)
	sort.Slice(sorted, func(i, j int) bool {
		ri := severityRank[sorted[i].GetSeverity()]
		rj := severityRank[sorted[j].GetSeverity()]
		if ri != rj {
			return ri > rj
		}
		return sorted[i].GetConfidence() > sorted[j].GetConfidence()
	})

	var critical, warning, caution uint32
	detectors := make(map[string]bool)

	for _, a := range sorted {
		switch a.GetSeverity() {
		case reasoningv1.FindingSeverity_CRITICAL:
			critical++
		case reasoningv1.FindingSeverity_WARNING:
			warning++
		case reasoningv1.FindingSeverity_CAUTION:
			caution++
		}
		if name := a.GetDetectorName(); name != "" {
			detectors[name] = true
		}
	}

	score := 1.0 - float64(critical)*0.3 - float64(warning)*0.15 - float64(caution)*0.05
	if score < 0 {
		score = 0
	}

	actions := generateActions(sorted)

	var detectorNames []string
	for name := range detectors {
		detectorNames = append(detectorNames, name)
	}
	sort.Strings(detectorNames)

	return &reasoningv1.ReasoningReport{
		Findings:              sorted,
		OverallIntegrityScore: score,
		CriticalCount:         critical,
		WarningCount:          warning,
		CautionCount:          caution,
		RecommendedActions:    actions,
		ConversationId:        conversationID,
		AssessedAt:            timestamppb.Now(),
		DetectorsActivated:    detectorNames,
	}
}

func generateActions(findings []*reasoningv1.CognitiveAssessment) []string {
	var actions []string
	seen := make(map[reasoningv1.FindingType]bool)

	for _, f := range findings {
		ft := f.GetFindingType()
		if seen[ft] {
			continue
		}
		seen[ft] = true

		switch ft {
		case reasoningv1.FindingType_ANCHORING_BIAS:
			actions = append(actions, "Consider generating estimates independently before comparing to initial values")
		case reasoningv1.FindingType_SUNK_COST_FALLACY:
			actions = append(actions, "Evaluate the decision based on future value, not past investment")
		case reasoningv1.FindingType_CONTRADICTION:
			actions = append(actions, "Resolve contradictory claims before proceeding")
		case reasoningv1.FindingType_SCOPE_DRIFT:
			actions = append(actions, "Re-anchor discussion to the original objective")
		case reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION:
			actions = append(actions, "Review confidence levels against available evidence")
		case reasoningv1.FindingType_SILENT_REVISION:
			actions = append(actions, "Explicitly state rationale when revising earlier decisions")
		case reasoningv1.FindingType_STATUS_QUO_BIAS:
			actions = append(actions, "Evaluate alternatives on merit rather than defaulting to the current state")
		default:
			actions = append(actions, fmt.Sprintf("Review finding: %s", ft.String()))
		}
	}
	return actions
}
