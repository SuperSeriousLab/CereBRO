package pipeline

import (
	"context"
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

// AggregateWithArbitration combines findings into a ReasoningReport, optionally
// routing conflicting detector findings to a DebateArbitrator before aggregation.
//
// Conflict detection: two findings conflict when they cover the same FindingType
// but one has confidence > 0.6 and the other < 0.4. If the arbitrator is nil,
// times out (ctx deadline exceeded), or returns an error, aggregation proceeds
// with the original findings — no regression.
//
// The arbitration result is advisory only: it nudges finding confidences
// proportionally but never removes findings or changes their types.
func AggregateWithArbitration(
	ctx context.Context,
	findings []*reasoningv1.CognitiveAssessment,
	conversationID string,
	arb DebateArbitrator,
) *reasoningv1.ReasoningReport {
	if arb != nil && len(findings) >= 2 {
		findings = applyArbitrationToConflicts(ctx, findings, arb)
	}
	return Aggregate(findings, conversationID)
}

// applyArbitrationToConflicts detects conflict clusters and routes each to the
// arbitrator. Non-conflicting findings pass through unchanged.
func applyArbitrationToConflicts(
	ctx context.Context,
	findings []*reasoningv1.CognitiveAssessment,
	arb DebateArbitrator,
) []*reasoningv1.CognitiveAssessment {
	clusters := detectConflictClusters(findings)
	if len(clusters) == 0 {
		return findings
	}

	// Build a set of finding indices that are part of a conflict cluster.
	conflictIdx := make(map[int]bool)
	for _, cluster := range clusters {
		for _, idx := range cluster.indices {
			conflictIdx[idx] = true
		}
	}

	// Start with the non-conflicting findings unchanged.
	result := make([]*reasoningv1.CognitiveAssessment, 0, len(findings))
	for i, f := range findings {
		if !conflictIdx[i] {
			result = append(result, f)
		}
	}

	// Arbitrate each cluster and append the adjusted findings.
	for _, cluster := range clusters {
		clusterFindings := make([]*reasoningv1.CognitiveAssessment, len(cluster.indices))
		for j, idx := range cluster.indices {
			clusterFindings[j] = findings[idx]
		}

		adjusted, err := arb.Arbitrate(ctx, clusterFindings)
		if err != nil {
			// On error (timeout, network, etc.): fall back to original findings.
			result = append(result, clusterFindings...)
		} else {
			result = append(result, adjusted...)
		}
	}

	return result
}

// conflictCluster groups findings that conflict with each other.
type conflictCluster struct {
	indices []int // indices into the original findings slice
}

// detectConflictClusters groups findings into conflict clusters.
//
// Two findings conflict when:
//   - They have the same FindingType AND one has confidence > 0.6 while the
//     other has confidence < 0.4 (opposing strength on the same type), OR
//   - They have semantically opposite FindingTypes (SYCOPHANCY vs CONTRADICTION,
//     or any pairing where one type directly opposes another).
//
// A cluster contains all findings involved in at least one conflict relationship.
func detectConflictClusters(findings []*reasoningv1.CognitiveAssessment) []conflictCluster {
	n := len(findings)
	if n < 2 {
		return nil
	}

	// Union-Find to group conflicting indices.
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}

	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int) {
		pa, pb := find(a), find(b)
		if pa != pb {
			parent[pa] = pb
		}
	}

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if findingsConflict(findings[i], findings[j]) {
				union(i, j)
			}
		}
	}

	// Collect groups that contain 2+ members (actual conflicts).
	groups := make(map[int][]int)
	for i := 0; i < n; i++ {
		root := find(i)
		groups[root] = append(groups[root], i)
	}

	var clusters []conflictCluster
	for _, indices := range groups {
		if len(indices) >= 2 {
			clusters = append(clusters, conflictCluster{indices: indices})
		}
	}
	return clusters
}

// findingsConflict returns true when two findings are in direct conflict.
//
// Conflict criteria:
//  1. Same FindingType with opposing confidence (one > 0.6, other < 0.4).
//  2. Semantically opposite FindingTypes (e.g. SYCOPHANCY vs CONTRADICTION
//     as a proxy — both indicate conflicting social/logical pressure directions).
func findingsConflict(a, b *reasoningv1.CognitiveAssessment) bool {
	ca := float64(a.GetConfidence())
	cb := float64(b.GetConfidence())

	// Criterion 1: same type, opposing confidence.
	if a.GetFindingType() == b.GetFindingType() {
		highLow := (ca > 0.6 && cb < 0.4)
		lowHigh := (ca < 0.4 && cb > 0.6)
		if highLow || lowHigh {
			return true
		}
	}

	// Criterion 2: semantically opposite types.
	if areOppositeTypes(a.GetFindingType(), b.GetFindingType()) {
		return true
	}

	return false
}

// oppositeTypePairs lists pairs of FindingTypes considered semantically opposite.
// Both orderings are covered by areOppositeTypes().
var oppositeTypePairs = [][2]reasoningv1.FindingType{
	// Conformity pressure vs. logical error — opposing cognitive forces.
	{reasoningv1.FindingType_SYCOPHANCY, reasoningv1.FindingType_CONTRADICTION},
	// Overconfident stance vs. under-evidence calibration error.
	{reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION, reasoningv1.FindingType_UNSUPPORTED_CONCLUSION},
	// Scope locked down vs. scope actively drifting.
	{reasoningv1.FindingType_STATUS_QUO_BIAS, reasoningv1.FindingType_SCOPE_DRIFT},
}

func areOppositeTypes(a, b reasoningv1.FindingType) bool {
	for _, pair := range oppositeTypePairs {
		if (a == pair[0] && b == pair[1]) || (a == pair[1] && b == pair[0]) {
			return true
		}
	}
	return false
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
