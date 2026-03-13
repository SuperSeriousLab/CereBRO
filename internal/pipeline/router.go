package pipeline

import (
	"strings"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// Detector identifies which cognitive detector to activate.
type Detector string

const (
	DetectorAnchoring     Detector = "anchoring-detector"
	DetectorSunkCost      Detector = "sunk-cost-detector"
	DetectorContradiction Detector = "contradiction-tracker"
	DetectorScopeGuard    Detector = "scope-guard"
	DetectorCalibrator    Detector = "confidence-calibrator"
	DetectorLedger        Detector = "decision-ledger"
)

// RouterConfig holds activation thresholds.
type RouterConfig struct {
	ScopeGuardMinTurns      uint32
	AnchoringMinNumerics    uint32
	DecisionLedgerMinTurns  uint32
}

// DefaultRouterConfig returns standard router thresholds.
func DefaultRouterConfig() RouterConfig {
	return RouterConfig{
		ScopeGuardMinTurns:     3,
		AnchoringMinNumerics:   2,
		DecisionLedgerMinTurns: 2,
	}
}

// RoutingDecision contains which detectors were activated and why.
type RoutingDecision struct {
	Activated []Detector
	Reasons   []string
}

// costPhrasePatterns for sunk-cost activation check.
var costPhrasePatterns = []string{
	"already spent", "already invested", "invested so much", "come this far",
	"put so much into", "too much time", "too much money", "too much effort",
	"can't waste", "don't want to waste", "sunk cost", "we've already", "i've already",
}

// decisionPhrasePatterns for decision-ledger activation check.
var decisionPhrasePatterns = []string{
	"let's go with", "we'll use", "i'll choose", "decided to", "going with",
	"the plan is", "we should", "i recommend",
}

// Route classifies a conversation and determines which detectors to activate.
// Mirrors the Rust cognitive-router logic.
func Route(snap *reasoningv1.ConversationSnapshot, cfg RouterConfig) RoutingDecision {
	var activated []Detector
	var reasons []string

	turns := snap.GetTurns()
	nTurns := uint32(len(turns))

	// Anchoring: needs ≥ N numeric tokens across ≥ 2 turns
	numericTurns := make(map[uint32]bool)
	totalNumerics := uint32(0)
	for _, t := range turns {
		if meta := t.GetMetadata(); meta != nil {
			n := uint32(len(meta.GetNumericTokens()))
			if n > 0 {
				numericTurns[t.GetTurnNumber()] = true
				totalNumerics += n
			}
		}
	}
	if totalNumerics >= cfg.AnchoringMinNumerics && len(numericTurns) >= 2 {
		activated = append(activated, DetectorAnchoring)
		reasons = append(reasons, "numeric tokens found across multiple turns")
	}

	// Confidence Calibrator: any turns present
	if nTurns > 0 {
		activated = append(activated, DetectorCalibrator)
		reasons = append(reasons, "conversation has turns to analyze")
	}

	// Contradiction Tracker: ≥ 2 turns
	if nTurns >= 2 {
		activated = append(activated, DetectorContradiction)
		reasons = append(reasons, "multi-turn conversation enables cross-turn comparison")
	}

	// Decision Ledger: decision language + ≥ 2 turns
	if nTurns >= cfg.DecisionLedgerMinTurns {
		hasDecision := false
		for _, t := range turns {
			lower := strings.ToLower(t.GetRawText())
			for _, p := range decisionPhrasePatterns {
				if strings.Contains(lower, p) {
					hasDecision = true
					break
				}
			}
			if hasDecision {
				break
			}
		}
		if hasDecision {
			activated = append(activated, DetectorLedger)
			reasons = append(reasons, "decision language detected in conversation")
		}
	}

	// Scope Guard: objective present AND ≥ 3 turns
	if snap.GetObjective() != "" && nTurns >= cfg.ScopeGuardMinTurns {
		activated = append(activated, DetectorScopeGuard)
		reasons = append(reasons, "objective present with sufficient turns for drift detection")
	}

	// Sunk-Cost: cost language detected
	hasCost := false
	for _, t := range turns {
		lower := strings.ToLower(t.GetRawText())
		for _, p := range costPhrasePatterns {
			if strings.Contains(lower, p) {
				hasCost = true
				break
			}
		}
		if hasCost {
			break
		}
	}
	if hasCost {
		activated = append(activated, DetectorSunkCost)
		reasons = append(reasons, "cost/investment language detected")
	}

	return RoutingDecision{Activated: activated, Reasons: reasons}
}
