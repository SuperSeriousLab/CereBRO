package pipeline

// Variant detectors for AIP competition.
// Each variant implements the same interface as the original detector
// but uses a different algorithm or tuning strategy.

import (
	"math"
	"strings"
	"unicode"

	"github.com/SuperSeriousLab/CereBRO/internal/textutil"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// ============================================================
// Scope Guard — Cumulative Centroid Variant
// ============================================================

type ScopeGuardCentroidConfig struct {
	DriftThreshold float64
	MinTurns       uint32
	ReferenceTurns uint32
	SustainedTurns uint32
	DecayFactor    float64
}

func DefaultScopeGuardCentroidConfig() ScopeGuardCentroidConfig {
	return ScopeGuardCentroidConfig{
		DriftThreshold: 0.85,
		MinTurns:       3,
		ReferenceTurns: 3,
		SustainedTurns: 7,
		DecayFactor:    0.8,
	}
}

func DetectScopeDriftCentroid(snap *reasoningv1.ConversationSnapshot, cfg ScopeGuardCentroidConfig) *reasoningv1.CognitiveAssessment {
	if snap == nil {
		return nil
	}
	objectiveKW := snap.GetObjectiveKeywords()
	if len(objectiveKW) == 0 {
		return nil
	}
	turns := snap.GetTurns()
	if uint32(len(turns)) < cfg.MinTurns {
		return nil
	}

	// Build initial centroid from objective + first K turns.
	centroid := make(map[string]float64)
	for _, kw := range objectiveKW {
		centroid[kw] += 2.0
	}
	refK := int(cfg.ReferenceTurns)
	if refK > len(turns) {
		refK = len(turns)
	}
	for i := 0; i < refK; i++ {
		if m := turns[i].GetMetadata(); m != nil {
			for _, kw := range m.GetTopicKeywords() {
				centroid[kw] += 1.0
			}
		}
	}

	// Evaluate turns after reference window.
	var consecutiveDrift int
	var driftTurns []uint32
	var maxDrift float64
	var maxDriftTopics []string
	sustained := false

	for i := refK; i < len(turns); i++ {
		var turnKWs []string
		if m := turns[i].GetMetadata(); m != nil {
			turnKWs = m.GetTopicKeywords()
		}

		// Build turn frequency map.
		turnFreq := make(map[string]float64)
		for _, kw := range turnKWs {
			turnFreq[kw] += 1.0
		}

		dist := weightedJaccardDivergence(centroid, turnFreq)

		if dist > cfg.DriftThreshold {
			consecutiveDrift++
			if consecutiveDrift >= int(cfg.SustainedTurns) {
				sustained = true
			}
			if sustained {
				driftTurns = append(driftTurns, turns[i].GetTurnNumber())
				if dist > maxDrift {
					maxDrift = dist
					maxDriftTopics = turnKWs
				}
			}
		} else {
			consecutiveDrift = 0
			sustained = false
			// Update centroid with exponential decay.
			decay := cfg.DecayFactor
			for kw, w := range centroid {
				centroid[kw] = decay * w
				if turnFreq[kw] > 0 {
					centroid[kw] += (1 - decay) * turnFreq[kw]
				}
			}
			for kw, w := range turnFreq {
				if _, exists := centroid[kw]; !exists {
					centroid[kw] = (1 - decay) * w
				}
			}
		}
	}

	if len(driftTurns) == 0 {
		return nil
	}

	return &reasoningv1.CognitiveAssessment{
		FindingType:   reasoningv1.FindingType_SCOPE_DRIFT,
		Severity:      scopeSeverityFromDrift(maxDrift),
		Explanation:   "Conversation topics have drifted from the stated objective (centroid variant)",
		RelevantTurns: driftTurns,
		Confidence:    maxDrift,
		DetectorName:  "scope-guard-centroid",
		Scope: &reasoningv1.ScopeDetail{
			DriftDistance:    maxDrift,
			CurrentTopics:   maxDriftTopics,
			ObjectiveTopics: objectiveKW,
		},
	}
}

// ============================================================
// Scope Guard — KL-Divergence Transition Variant
// ============================================================

type ScopeGuardTransitionConfig struct {
	KLThreshold     float64
	MinTurns        uint32
	ReferenceTurns  uint32
	WindowSize      uint32
	SustainedTurns  uint32
	SmoothingFactor float64
}

func DefaultScopeGuardTransitionConfig() ScopeGuardTransitionConfig {
	return ScopeGuardTransitionConfig{
		KLThreshold:     2.0,
		MinTurns:        3,
		ReferenceTurns:  3,
		WindowSize:      3,
		SustainedTurns:  5,
		SmoothingFactor: 0.01,
	}
}

func DetectScopeDriftTransition(snap *reasoningv1.ConversationSnapshot, cfg ScopeGuardTransitionConfig) *reasoningv1.CognitiveAssessment {
	if snap == nil {
		return nil
	}
	objectiveKW := snap.GetObjectiveKeywords()
	if len(objectiveKW) == 0 {
		return nil
	}
	turns := snap.GetTurns()
	if uint32(len(turns)) < cfg.MinTurns {
		return nil
	}

	// Build baseline distribution from objective + first K turns.
	baselineFreq := make(map[string]float64)
	for _, kw := range objectiveKW {
		baselineFreq[kw] += 2.0
	}
	refK := int(cfg.ReferenceTurns)
	if refK > len(turns) {
		refK = len(turns)
	}
	for i := 0; i < refK; i++ {
		if m := turns[i].GetMetadata(); m != nil {
			for _, kw := range m.GetTopicKeywords() {
				baselineFreq[kw] += 1.0
			}
		}
	}

	// Collect turn keywords for sliding window.
	turnKWs := make([][]string, len(turns))
	for i, t := range turns {
		if m := t.GetMetadata(); m != nil {
			turnKWs[i] = m.GetTopicKeywords()
		}
	}

	winSize := int(cfg.WindowSize)
	var consecutiveDrift int
	var driftTurns []uint32
	var maxKL float64
	var maxDriftTopics []string
	sustained := false

	startIdx := refK
	if startIdx < 1 {
		startIdx = 1
	}

	for i := startIdx; i < len(turns); i++ {
		// Build sliding window frequency.
		windowFreq := make(map[string]float64)
		winStart := i - winSize
		if winStart < 0 {
			winStart = 0
		}
		for j := winStart; j <= i; j++ {
			for _, kw := range turnKWs[j] {
				windowFreq[kw] += 1.0
			}
		}

		kl := klDivergence(baselineFreq, windowFreq, cfg.SmoothingFactor)

		if kl > cfg.KLThreshold {
			consecutiveDrift++
			if consecutiveDrift >= int(cfg.SustainedTurns) {
				sustained = true
			}
			if sustained {
				driftTurns = append(driftTurns, turns[i].GetTurnNumber())
				if kl > maxKL {
					maxKL = kl
					maxDriftTopics = turnKWs[i]
				}
			}
		} else {
			consecutiveDrift = 0
			sustained = false
		}
	}

	if len(driftTurns) == 0 {
		return nil
	}

	// Map KL to 0-1 confidence.
	confidence := math.Min(1.0, maxKL/(2*cfg.KLThreshold))

	return &reasoningv1.CognitiveAssessment{
		FindingType:   reasoningv1.FindingType_SCOPE_DRIFT,
		Severity:      scopeSeverityFromDrift(confidence),
		Explanation:   "Conversation topics have drifted from the stated objective (KL-divergence variant)",
		RelevantTurns: driftTurns,
		Confidence:    confidence,
		DetectorName:  "scope-guard-transition",
		Scope: &reasoningv1.ScopeDetail{
			DriftDistance:    confidence,
			CurrentTopics:   maxDriftTopics,
			ObjectiveTopics: objectiveKW,
		},
	}
}

// klDivergence computes KL(p || q) with Laplace smoothing.
func klDivergence(p, q map[string]float64, smoothing float64) float64 {
	// Collect all keys.
	allKeys := make(map[string]bool)
	var pTotal, qTotal float64
	for k, v := range p {
		allKeys[k] = true
		pTotal += v
	}
	for k, v := range q {
		allKeys[k] = true
		qTotal += v
	}

	if pTotal == 0 || qTotal == 0 {
		return 0.0
	}

	n := float64(len(allKeys))
	pSmooth := pTotal + n*smoothing
	qSmooth := qTotal + n*smoothing

	var kl float64
	for k := range allKeys {
		pVal := (p[k] + smoothing) / pSmooth
		qVal := (q[k] + smoothing) / qSmooth
		if pVal > 0 && qVal > 0 {
			kl += pVal * math.Log(pVal/qVal)
		}
	}
	return kl
}

// ============================================================
// Anchoring Detector — Context-Aware Variant
// ============================================================

type AnchoringContextConfig struct {
	ProximityThreshold float64
	MinNumericTokens   uint32
	ContextThreshold   float64 // minimum keyword overlap for context relevance
}

func DefaultAnchoringContextConfig() AnchoringContextConfig {
	return AnchoringContextConfig{
		ProximityThreshold: 0.15,
		MinNumericTokens:   2,
		ContextThreshold:   0.2,
	}
}

func DetectAnchoringContext(snap *reasoningv1.ConversationSnapshot, cfg AnchoringContextConfig) *reasoningv1.CognitiveAssessment {
	if snap == nil {
		return nil
	}
	entries := collectNumericEntries(snap)
	if uint32(len(entries)) < cfg.MinNumericTokens {
		return nil
	}

	anchor := entries[0]
	anchorKW := extractContextKeywords(anchor.context)

	var best *reasoningv1.CognitiveAssessment
	var bestConf float64

	for i := 1; i < len(entries); i++ {
		estimate := entries[i]
		if estimate.turn == anchor.turn {
			continue
		}

		// Context relevance filter.
		estKW := extractContextKeywords(estimate.context)
		similarity := jaccardSimilarity(anchorKW, estKW)
		if similarity < cfg.ContextThreshold {
			continue
		}

		shift := anchoringRelativeShift(anchor.value, estimate.value)
		if shift < cfg.ProximityThreshold {
			confidence := (1.0 - shift/cfg.ProximityThreshold) * (0.5 + 0.5*similarity)
			if confidence > bestConf {
				bestConf = confidence
				best = &reasoningv1.CognitiveAssessment{
					FindingType:   reasoningv1.FindingType_ANCHORING_BIAS,
					Severity:      anchoringSeverityFromShift(shift, cfg.ProximityThreshold),
					Explanation:   "Numeric estimate appears anchored to an earlier value in related context",
					RelevantTurns: []uint32{anchor.turn, estimate.turn},
					Confidence:    confidence,
					DetectorName:  "anchoring-detector-context",
					Anchoring: &reasoningv1.AnchoringDetail{
						AnchorValue:   anchor.value,
						EstimateValue: estimate.value,
						RelativeShift: shift,
						AnchorTurn:    anchor.turn,
						EstimateTurn:  estimate.turn,
					},
				}
			}
		}
	}
	return best
}

func extractContextKeywords(text string) map[string]bool {
	words := strings.Fields(strings.ToLower(text))
	kw := make(map[string]bool)
	for _, w := range words {
		w = strings.TrimFunc(w, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if w == "" || len(w) < 3 || stopwords[w] {
			continue
		}
		kw[w] = true
	}
	return kw
}

func jaccardSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	union := make(map[string]bool)
	intersection := 0
	for k := range a {
		union[k] = true
	}
	for k := range b {
		union[k] = true
		if a[k] {
			intersection++
		}
	}
	if len(union) == 0 {
		return 1.0
	}
	return float64(intersection) / float64(len(union))
}

// ============================================================
// Sunk-Cost Detector — Proximity-Weighted Variant
// ============================================================

type SunkCostProximityConfig struct {
	MinConfidence float64
	DecayRate     float64 // controls how fast confidence drops with turn distance
}

func DefaultSunkCostProximityConfig() SunkCostProximityConfig {
	return SunkCostProximityConfig{
		MinConfidence: 0.5,
		DecayRate:     0.3,
	}
}

// Extended continuation phrases for the proximity variant.
var extendedContinuationPhrases = append(
	append([]string{}, continuationPhrases...),
	"push through",
	"keep at it",
	"see it through",
)

func DetectSunkCostProximity(snap *reasoningv1.ConversationSnapshot, cfg SunkCostProximityConfig) *reasoningv1.CognitiveAssessment {
	if snap == nil {
		return nil
	}

	var costMatches []phraseMatch
	var contMatches []phraseMatch

	for _, turn := range snap.GetTurns() {
		lower := textutil.NormalizeQuotes(strings.ToLower(turn.GetRawText()))

		for _, phrase := range sunkCostPhrases {
			if strings.Contains(lower, phrase) {
				costMatches = append(costMatches, phraseMatch{
					phrase: phrase,
					turn:   turn.GetTurnNumber(),
				})
				break
			}
		}

		for _, phrase := range extendedContinuationPhrases {
			if strings.Contains(lower, phrase) {
				contMatches = append(contMatches, phraseMatch{
					phrase: phrase,
					turn:   turn.GetTurnNumber(),
				})
				break
			}
		}
	}

	// Find highest-confidence pair with proximity weighting.
	var best *reasoningv1.CognitiveAssessment
	var bestConf float64

	for _, cost := range costMatches {
		for _, cont := range contMatches {
			if cont.turn < cost.turn {
				continue
			}

			turnGap := float64(cont.turn - cost.turn)
			base := sunkCostBaseConfidence(cost, cont)
			decay := 1.0 / (1.0 + cfg.DecayRate*turnGap)
			confidence := base * decay

			if confidence < cfg.MinConfidence {
				continue
			}
			if confidence > bestConf {
				bestConf = confidence
				best = &reasoningv1.CognitiveAssessment{
					FindingType:   reasoningv1.FindingType_SUNK_COST_FALLACY,
					Severity:      sunkCostSeverity(confidence),
					Explanation:   "Past-investment language followed by continuation decision (proximity-weighted)",
					RelevantTurns: []uint32{cost.turn, cont.turn},
					Confidence:    confidence,
					DetectorName:  "sunk-cost-detector-proximity",
					SunkCost: &reasoningv1.SunkCostDetail{
						CostReference:        cost.phrase,
						CostTurn:             cost.turn,
						ContinuationDecision: cont.phrase,
						DecisionTurn:         cont.turn,
					},
				}
			}
		}
	}
	return best
}

func sunkCostBaseConfidence(cost, cont phraseMatch) float64 {
	base := 0.5
	if cont.turn > cost.turn {
		base += 0.2
	}
	turnGap := cont.turn - cost.turn
	if turnGap <= 2 {
		base += 0.15
	}
	strongCost := []string{"already spent", "already invested", "sunk cost"}
	for _, s := range strongCost {
		if cost.phrase == s {
			base += 0.1
			break
		}
	}
	if base > 1.0 {
		base = 1.0
	}
	return base
}
