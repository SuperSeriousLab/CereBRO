package pipeline

import (
	"strings"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// Detector identifies which cognitive detector to activate.
type Detector string

const (
	DetectorAnchoring             Detector = "anchoring-detector"
	DetectorSunkCost              Detector = "sunk-cost-detector"
	DetectorContradiction         Detector = "contradiction-tracker"
	DetectorScopeGuard            Detector = "scope-guard"
	DetectorCalibrator            Detector = "confidence-calibrator"
	DetectorLedger                Detector = "decision-ledger"
	DetectorConceptualAnchoring   Detector = "conceptual-anchoring-detector"
	DetectorInheritedPosition     Detector = "inherited-position-detector"
	DetectorEvidenceAsymmetry     Detector = "evidence-asymmetry-detector"
	DetectorSustainedConviction   Detector = "sustained-conviction-detector"
	DetectorUnderevidencedClaims  Detector = "underevidenced-claims"
	DetectorNegativeClaim         Detector = "negative-claim-confidence"
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
// Must stay in sync with sunkCostPhrases in detectors.go (router activates the detector;
// detector does the detailed matching). Subset is sufficient for routing.
var costPhrasePatterns = []string{
	"already spent", "already invested", "invested so much", "come this far",
	"put so much into", "too much time", "too much money", "too much effort",
	"can't waste", "don't want to waste", "sunk cost", "we've already", "i've already",
	// Classical commitment-defense markers (must mirror sunkCostPhrases subset):
	"as simonides", "simonides say", "heir of the argument", "attributes such a saying",
	"i still stand by", "stand by the latter", "as was said before",
	"we have already agreed", "our earlier argument", "having committed ourselves",
	"the position we have defended", "it were unjust to abandon",
	"that is implied in the argument", "as we were just now saying",
}

// inheritedPositionActivationPhrases are a subset of authorityPhrases used for
// fast routing activation. A match triggers the inherited-position-detector.
// Must not overlap heavily with costPhrasePatterns (sunk-cost detector routing).
var inheritedPositionActivationPhrases = []string{
	"as simonides", "simonides said", "simonides taught",
	"according to", "as homer", "homer said",
	"as aristotle", "as plato", "as socrates said",
	"as was said", "as the saying", "as the poet",
	"tradition holds", "we have always", "it has always been",
	"following the tradition", "the tradition of",
	// Generic "as X said/taught/argued" — router uses simple keyword presence
	" said that", " taught that", " argued that", " maintained that",
	" believed that", " held that",
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

	// Conceptual Anchoring: activate when conversation has >= 4 turns and any
	// declarative assertion appears in the early turns (first 3).
	// This detector runs on both modern and classical text.
	if nTurns >= 4 {
		maxScan := 3
		if int(nTurns) < maxScan {
			maxScan = int(nTurns)
		}
		hasDeclarative := false
		for i := 0; i < maxScan; i++ {
			if isStrongDeclarative(turns[i].GetRawText()) {
				hasDeclarative = true
				break
			}
		}
		if hasDeclarative {
			activated = append(activated, DetectorConceptualAnchoring)
			reasons = append(reasons, "declarative assertion in early turns — conceptual anchor candidate")
		}
	}

	// Inherited Position: activate when conversation has >= 4 turns and any
	// authority citation patterns are present. These patterns signal "X said/taught/..."
	// structures that the inherited-position-detector scrutinises for missing merit defense.
	if nTurns >= 4 {
		hasAuthorityCitation := false
		for _, t := range turns {
			lower := strings.ToLower(t.GetRawText())
			for _, p := range inheritedPositionActivationPhrases {
				if strings.Contains(lower, p) {
					hasAuthorityCitation = true
					break
				}
			}
			if hasAuthorityCitation {
				break
			}
		}
		if hasAuthorityCitation {
			activated = append(activated, DetectorInheritedPosition)
			reasons = append(reasons, "authority citation patterns detected — checking for inherited-position reasoning")
		}
	}

	// Evidence Asymmetry: activate when conversation has ≥ 4 turns and at least 2
	// assistant turns. Measures evidence grounding ratio of positive vs negative claims.
	if nTurns >= 4 {
		assistantTurns := uint32(0)
		for _, t := range turns {
			if strings.ToLower(t.GetSpeaker()) == "assistant" {
				assistantTurns++
			}
		}
		if assistantTurns >= 2 {
			activated = append(activated, DetectorEvidenceAsymmetry)
			reasons = append(reasons, "multi-turn assistant responses — checking evidence grounding asymmetry")
		}
	}

	// SustainedConviction: activate on any conversation with at least 1 assistant turn.
	// Tier1_Bias — checks rolling MV of recent claims for pathological conviction patterns.
	for _, t := range turns {
		if strings.ToLower(t.GetSpeaker()) == "assistant" {
			activated = append(activated, DetectorSustainedConviction)
			reasons = append(reasons, "assistant turns present — checking sustained conviction signal")
			break
		}
	}

	// UnderevidencedClaims: activate when conversation has ≥ 2 assistant turns.
	// Tier1_Bias — checks evidence-to-positive-claim ratio (gen10_89).
	{
		assistantCount := 0
		for _, t := range turns {
			if strings.ToLower(t.GetSpeaker()) == "assistant" {
				assistantCount++
			}
		}
		if assistantCount >= 2 {
			activated = append(activated, DetectorUnderevidencedClaims)
			reasons = append(reasons, "multiple assistant turns — checking evidence-to-positive-claim ratio")
		}
	}

	// NegativeClaim: activate when conversation has ≥ 1 assistant turn.
	// Tier2_Structural — MaxMV(negative-direction claims) > 0.45 (gen0_93).
	// Fires CATHEDRAL_COMPLEXITY or COUNTER_EVIDENCE_DEPLETION.
	for _, t := range turns {
		if strings.ToLower(t.GetSpeaker()) == "assistant" {
			activated = append(activated, DetectorNegativeClaim)
			reasons = append(reasons, "assistant turns present — checking negative-claim high-confidence signal")
			break
		}
	}

	return RoutingDecision{Activated: activated, Reasons: reasons}
}
