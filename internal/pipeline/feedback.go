// Feedback Evaluator — bounded predictive coding for low-confidence reports.
//
// When self-confidence is low, selects the weakest findings for re-evaluation.
// Detectors receive FeedbackContext containing what other detectors found on
// the first pass, allowing them to adjust confidence based on corroboration.
//
// The feedback loop is bounded: pass_number is always 2, and the evaluator
// only generates requests when pass_count == 1. There is no pass_number == 3.
//
// Phase 4 deliverable. Brain analogue: cortico-thalamic feedback loops and
// predictive coding (Rao & Ballard, 1999).
package pipeline

import (
	"sort"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// FeedbackConfig holds tunable parameters for the Feedback Evaluator.
type FeedbackConfig struct {
	FeedbackThreshold       float64 // self-confidence below this triggers feedback
	MaxReevalFindings       int     // max findings to re-evaluate per pass
	ConfidenceImprovementMin float64 // min delta to accept updated finding
}

// DefaultFeedbackConfig returns the default configuration.
func DefaultFeedbackConfig() FeedbackConfig {
	return FeedbackConfig{
		FeedbackThreshold:       0.6,
		MaxReevalFindings:       3,
		ConfidenceImprovementMin: 0.1,
	}
}

// FeedbackContext provides second-pass detectors with context from the first pass.
// Detectors check if this is non-nil to determine if they're on a feedback pass.
type FeedbackContext struct {
	PassNumber     int
	OriginalReport *reasoningv1.ReasoningReport
	PeerFindings   []*reasoningv1.CognitiveAssessment // what other detectors found
	RequestID      string
}

// FeedbackResult holds the output of the feedback evaluation.
type FeedbackResult struct {
	Applied          bool     // whether feedback was actually applied
	ReevalDetectors  []string // detector names that were re-evaluated
	ConfidenceDeltas []float64 // confidence changes per re-evaluated finding
}

// EvaluateFeedback checks if feedback is needed and runs the re-evaluation loop.
// Returns the (possibly updated) findings and a FeedbackResult.
func EvaluateFeedback(
	findings []*reasoningv1.CognitiveAssessment,
	selfConf *cerebrov1.SelfConfidenceReport,
	snap *reasoningv1.ConversationSnapshot,
	report *reasoningv1.ReasoningReport,
	cfg FeedbackConfig,
	detectors map[Detector]DetectorFunc,
) ([]*reasoningv1.CognitiveAssessment, *FeedbackResult) {
	// Step 1: Check if feedback is needed.
	if selfConf.GetOverallConfidence() >= cfg.FeedbackThreshold {
		return findings, &FeedbackResult{Applied: false}
	}

	// Step 2: Select findings for re-evaluation (lowest confidence first).
	type indexedFinding struct {
		index   int
		finding *reasoningv1.CognitiveAssessment
	}
	candidates := make([]indexedFinding, len(findings))
	for i, f := range findings {
		candidates[i] = indexedFinding{index: i, finding: f}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].finding.GetConfidence() < candidates[j].finding.GetConfidence()
	})

	limit := cfg.MaxReevalFindings
	if limit > len(candidates) {
		limit = len(candidates)
	}
	toReeval := candidates[:limit]

	if len(toReeval) == 0 {
		return findings, &FeedbackResult{Applied: false}
	}

	// Step 3: Build FeedbackContext.
	feedbackCtx := &FeedbackContext{
		PassNumber:     2,
		OriginalReport: report,
		PeerFindings:   findings,
		RequestID:      "feedback-pass-2",
	}

	// Step 4: Re-evaluate selected detectors.
	updated := make([]*reasoningv1.CognitiveAssessment, len(findings))
	copy(updated, findings)

	result := &FeedbackResult{Applied: false}

	for _, candidate := range toReeval {
		detName := Detector(candidate.finding.GetDetectorName())
		fn, ok := detectors[detName]
		if !ok {
			continue
		}

		// Re-run detector with feedback context.
		newAssessment := fn(snap)
		if newAssessment == nil {
			// Detector found nothing on re-evaluation. Only remove if the original
			// finding was significant (strictly > threshold). A finding at exactly
			// the threshold is too marginal to count its disappearance as evidence.
			// Note: line 138 uses >= for delta checks — different semantics:
			// disappearance = "was it significant?", delta = "is the change significant?".
			if candidate.finding.GetConfidence() > cfg.ConfidenceImprovementMin {
				// Finding disappeared — significant change. Set confidence to 0.
				updatedFinding := shallowCopyAssessment(candidate.finding)
				updatedFinding.Confidence = 0
				updated[candidate.index] = updatedFinding
				result.Applied = true
				result.ReevalDetectors = append(result.ReevalDetectors, string(detName))
				result.ConfidenceDeltas = append(result.ConfidenceDeltas, -candidate.finding.GetConfidence())
			}
			continue
		}

		// Apply feedback adjustments based on peer findings.
		adjustedAssessment := applyFeedbackAdjustment(newAssessment, feedbackCtx)

		delta := adjustedAssessment.GetConfidence() - candidate.finding.GetConfidence()
		absDelta := delta
		if absDelta < 0 {
			absDelta = -absDelta
		}

		if absDelta >= cfg.ConfidenceImprovementMin-1e-9 {
			updated[candidate.index] = adjustedAssessment
			result.Applied = true
		}
		result.ReevalDetectors = append(result.ReevalDetectors, string(detName))
		result.ConfidenceDeltas = append(result.ConfidenceDeltas, delta)
	}

	// Filter out zeroed-confidence findings (disappeared on re-eval).
	var filtered []*reasoningv1.CognitiveAssessment
	for _, f := range updated {
		if f.GetConfidence() > 0 {
			filtered = append(filtered, f)
		}
	}

	return filtered, result
}

// applyFeedbackAdjustment adjusts a finding's confidence based on peer findings.
// This implements the per-detector feedback behavior from the spec.
func applyFeedbackAdjustment(
	finding *reasoningv1.CognitiveAssessment,
	ctx *FeedbackContext,
) *reasoningv1.CognitiveAssessment {
	if ctx == nil || len(ctx.PeerFindings) == 0 {
		return finding
	}

	result := shallowCopyAssessment(finding)
	detName := finding.GetDetectorName()

	// Count peer findings that overlap our relevant turns.
	overlapping := findOverlappingPeers(finding, ctx.PeerFindings)

	switch detName {
	case "contradiction-tracker":
		// Scope-guard corroboration → +0.1; no corroboration → -0.1
		if hasPeerDetector(overlapping, "scope-guard") {
			result.Confidence += 0.1
		} else if len(overlapping) == 0 {
			result.Confidence -= 0.1
		}

	case "anchoring-detector", "anchoring-detector-context":
		// Confidence-calibrator corroboration → +0.1; none → -0.05
		if hasPeerDetector(overlapping, "confidence-calibrator") {
			result.Confidence += 0.1
		} else if len(overlapping) == 0 {
			result.Confidence -= 0.05
		}

	case "scope-guard":
		// Contradiction-tracker corroboration → +0.1; no change otherwise
		// (Forge-optimized threshold — don't perturb without evidence)
		if hasPeerDetector(overlapping, "contradiction-tracker") {
			result.Confidence += 0.1
		}

	case "confidence-calibrator":
		// Any peer on same turns → +0.1; solo → -0.05
		if len(overlapping) > 0 {
			result.Confidence += 0.1
		} else {
			result.Confidence -= 0.05
		}

	case "sunk-cost-detector":
		// Decision-ledger on overlapping turns → +0.15; no change otherwise
		if hasPeerDetector(overlapping, "decision-ledger") {
			result.Confidence += 0.15
		}

	// decision-ledger: no feedback adjustment (factual tracking)
	// claim-extractor: no feedback adjustment (extraction, not judgment)
	}

	// Clamp confidence to [0.0, 1.0].
	if result.Confidence > 1.0 {
		result.Confidence = 1.0
	}
	if result.Confidence < 0.0 {
		result.Confidence = 0.0
	}

	return result
}

func findOverlappingPeers(
	finding *reasoningv1.CognitiveAssessment,
	peers []*reasoningv1.CognitiveAssessment,
) []*reasoningv1.CognitiveAssessment {
	myTurns := turnSet(finding.GetRelevantTurns())
	myDet := finding.GetDetectorName()

	var overlapping []*reasoningv1.CognitiveAssessment
	for _, peer := range peers {
		if peer.GetDetectorName() == myDet {
			continue // Skip self.
		}
		for _, t := range peer.GetRelevantTurns() {
			if myTurns[t] || (t > 0 && myTurns[t-1]) || myTurns[t+1] { // ±1 turn window
				overlapping = append(overlapping, peer)
				break
			}
		}
	}
	return overlapping
}

func hasPeerDetector(peers []*reasoningv1.CognitiveAssessment, detName string) bool {
	for _, p := range peers {
		if p.GetDetectorName() == detName {
			return true
		}
	}
	return false
}

func turnSet(turns []uint32) map[uint32]bool {
	s := make(map[uint32]bool, len(turns))
	for _, t := range turns {
		s[t] = true
	}
	return s
}

func shallowCopyAssessment(a *reasoningv1.CognitiveAssessment) *reasoningv1.CognitiveAssessment {
	return &reasoningv1.CognitiveAssessment{
		FindingType:   a.GetFindingType(),
		Severity:      a.GetSeverity(),
		Explanation:   a.GetExplanation(),
		RelevantTurns: a.GetRelevantTurns(),
		Confidence:    a.GetConfidence(),
		DetectorName:  a.GetDetectorName(),
		Anchoring:     a.GetAnchoring(),
		SunkCost:      a.GetSunkCost(),
		Contradiction: a.GetContradiction(),
		Scope:         a.GetScope(),
		Calibration:   a.GetCalibration(),
	}
}
