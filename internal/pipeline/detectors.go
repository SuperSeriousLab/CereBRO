package pipeline

import (
	"math"
	"sort"
	"strings"

	"github.com/SuperSeriousLab/CereBRO/internal/textutil"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// ============================================================
// Anchoring Bias Detector
// ============================================================

type AnchoringConfig struct {
	ProximityThreshold float64
	MinNumericTokens   uint32
}

func DefaultAnchoringConfig() AnchoringConfig {
	return AnchoringConfig{ProximityThreshold: 0.15, MinNumericTokens: 2}
}

type numericEntry struct {
	value   float64
	turn    uint32
	context string
}

func DetectAnchoring(snap *reasoningv1.ConversationSnapshot, cfg AnchoringConfig) *reasoningv1.CognitiveAssessment {
	if snap == nil {
		return nil
	}
	entries := collectNumericEntries(snap)
	if uint32(len(entries)) < cfg.MinNumericTokens {
		return nil
	}

	anchor := entries[0]

	for i := 1; i < len(entries); i++ {
		estimate := entries[i]
		if estimate.turn == anchor.turn {
			continue
		}

		shift := anchoringRelativeShift(anchor.value, estimate.value)
		if shift < cfg.ProximityThreshold {
			confidence := 1.0 - (shift / cfg.ProximityThreshold)
			return &reasoningv1.CognitiveAssessment{
				FindingType:   reasoningv1.FindingType_ANCHORING_BIAS,
				Severity:      anchoringSeverityFromShift(shift, cfg.ProximityThreshold),
				Explanation:   "Numeric estimate appears anchored to an earlier value",
				RelevantTurns: []uint32{anchor.turn, estimate.turn},
				Confidence:    confidence,
				DetectorName:  "anchoring-detector",
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
	return nil
}

func collectNumericEntries(snap *reasoningv1.ConversationSnapshot) []numericEntry {
	var entries []numericEntry
	for _, turn := range snap.GetTurns() {
		meta := turn.GetMetadata()
		if meta == nil {
			continue
		}
		for _, nt := range meta.GetNumericTokens() {
			entries = append(entries, numericEntry{
				value:   nt.GetValue(),
				turn:    turn.GetTurnNumber(),
				context: nt.GetContextWindow(),
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].turn < entries[j].turn
	})
	return entries
}

func anchoringRelativeShift(anchor, estimate float64) float64 {
	denom := math.Max(math.Abs(anchor), 1.0)
	return math.Abs(estimate-anchor) / denom
}

func anchoringSeverityFromShift(shift, threshold float64) reasoningv1.FindingSeverity {
	ratio := shift / threshold
	if ratio < 0.3 {
		return reasoningv1.FindingSeverity_WARNING
	}
	if ratio < 0.7 {
		return reasoningv1.FindingSeverity_CAUTION
	}
	return reasoningv1.FindingSeverity_INFO
}

// ============================================================
// Sunk-Cost Fallacy Detector
// ============================================================

type SunkCostConfig struct {
	MinConfidence float64
}

func DefaultSunkCostConfig() SunkCostConfig {
	return SunkCostConfig{MinConfidence: 0.5}
}

var sunkCostPhrases = []string{
	"already spent", "already invested", "invested so much", "come this far",
	"put so much into", "too much time", "too much money", "too much effort",
	"can't waste", "don't want to waste", "sunk cost", "we've already", "i've already",
}

var continuationPhrases = []string{
	"should keep going", "should continue", "can't stop now", "shouldn't give up",
	"let's keep", "let's continue", "we must continue", "have to finish",
	"need to finish", "too late to change", "too late to stop",
	"might as well", "no point stopping", "stick with",
}

type phraseMatch struct {
	phrase string
	turn   uint32
}

func DetectSunkCost(snap *reasoningv1.ConversationSnapshot, cfg SunkCostConfig) *reasoningv1.CognitiveAssessment {
	if snap == nil {
		return nil
	}
	var costMatches, contMatches []phraseMatch

	for _, turn := range snap.GetTurns() {
		lower := textutil.NormalizeQuotes(strings.ToLower(turn.GetRawText()))

		for _, phrase := range sunkCostPhrases {
			if strings.Contains(lower, phrase) {
				costMatches = append(costMatches, phraseMatch{phrase: phrase, turn: turn.GetTurnNumber()})
				break
			}
		}
		for _, phrase := range continuationPhrases {
			if strings.Contains(lower, phrase) {
				contMatches = append(contMatches, phraseMatch{phrase: phrase, turn: turn.GetTurnNumber()})
				break
			}
		}
	}

	for _, cost := range costMatches {
		for _, cont := range contMatches {
			if cont.turn >= cost.turn {
				confidence := sunkCostConfidence(cost, cont)
				if confidence < cfg.MinConfidence {
					continue
				}
				return &reasoningv1.CognitiveAssessment{
					FindingType:   reasoningv1.FindingType_SUNK_COST_FALLACY,
					Severity:      sunkCostSeverity(confidence),
					Explanation:   "Past-investment language followed by continuation decision suggests sunk-cost reasoning",
					RelevantTurns: []uint32{cost.turn, cont.turn},
					Confidence:    confidence,
					DetectorName:  "sunk-cost-detector",
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
	return nil
}

func sunkCostConfidence(cost, cont phraseMatch) float64 {
	base := 0.5
	if cont.turn > cost.turn {
		base += 0.2
	}
	if cont.turn-cost.turn <= 2 {
		base += 0.15
	}
	strong := []string{"already spent", "already invested", "sunk cost"}
	for _, s := range strong {
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

func sunkCostSeverity(confidence float64) reasoningv1.FindingSeverity {
	if confidence >= 0.8 {
		return reasoningv1.FindingSeverity_WARNING
	}
	if confidence >= 0.6 {
		return reasoningv1.FindingSeverity_CAUTION
	}
	return reasoningv1.FindingSeverity_INFO
}

// ============================================================
// Contradiction Tracker
// ============================================================

type ContradictionConfig struct {
	MinOverlap float64
}

func DefaultContradictionConfig() ContradictionConfig {
	return ContradictionConfig{MinOverlap: 0.3}
}

var negationPrefixes = []string{
	"not ", "no ", "never ", "don't ", "doesn't ",
	"isn't ", "aren't ", "won't ", "can't ", "shouldn't ",
}

var reversalPhrases = []string{
	"actually", "i was wrong", "on second thought",
	"contrary to", "i take that back", "correction:",
}

var antonymPairs = [][2]string{
	{"increase", "decrease"}, {"always", "never"}, {"all", "none"},
	{"true", "false"}, {"agree", "disagree"}, {"support", "oppose"},
	{"accept", "reject"},
}

type sentenceRecord struct {
	text    string
	speaker string
	turn    uint32
}

func DetectContradiction(snap *reasoningv1.ConversationSnapshot, cfg ContradictionConfig) *reasoningv1.CognitiveAssessment {
	if snap == nil {
		return nil
	}

	var sentences []sentenceRecord
	for _, turn := range snap.GetTurns() {
		lower := textutil.NormalizeQuotes(strings.ToLower(turn.GetRawText()))
		for _, sent := range splitSentences(lower) {
			sent = strings.TrimSpace(sent)
			if len(sent) < 3 {
				continue
			}
			sentences = append(sentences, sentenceRecord{
				text:    sent,
				speaker: turn.GetSpeaker(),
				turn:    turn.GetTurnNumber(),
			})
		}
	}

	for i := 0; i < len(sentences); i++ {
		for j := i + 1; j < len(sentences); j++ {
			a := sentences[i]
			b := sentences[j]

			if a.speaker != b.speaker || a.turn == b.turn {
				continue
			}

			overlap := wordOverlap(a.text, b.text)
			if overlap < cfg.MinOverlap {
				continue
			}

			kind := detectContradictionKind(a.text, b.text)
			if kind == "" {
				continue
			}

			confidence := contradictionConfidence(overlap, kind)
			severity := contradictionSeverity(confidence)

			return &reasoningv1.CognitiveAssessment{
				FindingType:   reasoningv1.FindingType_CONTRADICTION,
				Severity:      severity,
				Explanation:   "Contradictory statements detected from the same speaker across turns",
				RelevantTurns: []uint32{a.turn, b.turn},
				Confidence:    confidence,
				DetectorName:  "contradiction-tracker",
				Contradiction: &reasoningv1.ContradictionDetail{
					ClaimAText: a.text,
					ClaimBText: b.text,
				},
			}
		}
	}
	return nil
}

func splitSentences(text string) []string {
	var result []string
	current := text
	for current != "" {
		idx := -1
		for _, sep := range []string{". ", "! ", "? "} {
			i := strings.Index(current, sep)
			if i >= 0 && (idx < 0 || i < idx) {
				idx = i
			}
		}
		if idx < 0 {
			result = append(result, strings.TrimSpace(current))
			break
		}
		sent := strings.TrimSpace(current[:idx+1])
		if len(sent) > 0 {
			result = append(result, sent)
		}
		current = current[idx+2:]
	}
	return result
}

func contWordSet(s string) map[string]bool {
	words := strings.Fields(s)
	set := make(map[string]bool, len(words))
	for _, w := range words {
		w = strings.Trim(w, ".,!?;:\"'()-")
		if len(w) > 0 {
			set[w] = true
		}
	}
	return set
}

func wordOverlap(a, b string) float64 {
	setA := contWordSet(a)
	setB := contWordSet(b)
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}

	stop := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "shall": true, "can": true, "to": true,
		"of": true, "in": true, "for": true, "on": true, "with": true,
		"at": true, "by": true, "from": true, "it": true, "this": true,
		"that": true, "i": true, "we": true, "they": true, "you": true,
		"he": true, "she": true, "and": true, "or": true, "but": true,
	}

	contentA := make(map[string]bool)
	for w := range setA {
		if !stop[w] {
			contentA[w] = true
		}
	}
	contentB := make(map[string]bool)
	for w := range setB {
		if !stop[w] {
			contentB[w] = true
		}
	}

	if len(contentA) == 0 || len(contentB) == 0 {
		return 0
	}

	intersection := 0
	for w := range contentA {
		if contentB[w] {
			intersection++
		}
	}

	smaller := len(contentA)
	if len(contentB) < smaller {
		smaller = len(contentB)
	}
	return float64(intersection) / float64(smaller)
}

func detectContradictionKind(a, b string) string {
	if hasNegationConflict(a, b) {
		return "negation"
	}
	for _, phrase := range reversalPhrases {
		if strings.Contains(b, phrase) {
			return "reversal"
		}
	}
	if hasAntonymConflict(a, b) {
		return "antonym"
	}
	return ""
}

func hasNegationConflict(a, b string) bool {
	for _, neg := range negationPrefixes {
		if strings.Contains(b, neg) && !strings.Contains(a, neg) {
			return true
		}
		if strings.Contains(a, neg) && !strings.Contains(b, neg) {
			return true
		}
	}
	return false
}

func hasAntonymConflict(a, b string) bool {
	wordsA := contWordSet(a)
	wordsB := contWordSet(b)
	for _, pair := range antonymPairs {
		if (wordsA[pair[0]] && wordsB[pair[1]]) || (wordsA[pair[1]] && wordsB[pair[0]]) {
			return true
		}
	}
	return false
}

func contradictionConfidence(overlap float64, kind string) float64 {
	base := 0.4
	base += overlap * 0.3
	switch kind {
	case "negation":
		base += 0.25
	case "reversal":
		base += 0.2
	case "antonym":
		base += 0.15
	}
	if base > 1.0 {
		base = 1.0
	}
	return base
}

func contradictionSeverity(confidence float64) reasoningv1.FindingSeverity {
	if confidence >= 0.8 {
		return reasoningv1.FindingSeverity_CRITICAL
	}
	if confidence >= 0.6 {
		return reasoningv1.FindingSeverity_WARNING
	}
	return reasoningv1.FindingSeverity_CAUTION
}

// ============================================================
// Scope Guard (Scope Drift Detector)
// ============================================================

type ScopeGuardConfig struct {
	DriftThreshold float64 // weighted Jaccard divergence above which a turn counts as drifting
	MinTurns       uint32  // minimum turns before drift detection activates
	ReferenceTurns uint32  // number of early turns to include in the reference set (default 3)
	WindowSize     uint32  // sliding window size for current-topic aggregation (default 3)
	SustainedTurns uint32  // consecutive drifting turns required before flagging (default 3)
}

func DefaultScopeGuardConfig() ScopeGuardConfig {
	return ScopeGuardConfig{
		DriftThreshold: 0.80,
		MinTurns:       3,
		ReferenceTurns: 4,
		WindowSize:     3,
		SustainedTurns: 8,
	}
}

func DetectScopeDrift(snap *reasoningv1.ConversationSnapshot, cfg ScopeGuardConfig) *reasoningv1.CognitiveAssessment {
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

	// Stage 1: Build reference frequency map from objective + first K turns.
	refFreq := make(map[string]float64)
	for _, kw := range objectiveKW {
		refFreq[kw] += 2.0 // objective keywords get double weight
	}
	refK := int(cfg.ReferenceTurns)
	if refK > len(turns) {
		refK = len(turns)
	}
	for i := 0; i < refK; i++ {
		meta := turns[i].GetMetadata()
		if meta == nil {
			continue
		}
		for _, kw := range meta.GetTopicKeywords() {
			refFreq[kw] += 1.0
		}
	}

	// Stage 2: Evaluate each turn after the reference window using a sliding window.
	winSize := int(cfg.WindowSize)
	if winSize < 1 {
		winSize = 1
	}

	// Collect all turn keywords for sliding window lookups.
	turnKWs := make([][]string, len(turns))
	for i, t := range turns {
		if m := t.GetMetadata(); m != nil {
			turnKWs[i] = m.GetTopicKeywords()
		}
	}

	var consecutiveDrift int
	var driftTurns []uint32
	var maxDrift float64
	var maxDriftTopics []string
	sustained := false

	startIdx := refK
	if startIdx < 1 {
		startIdx = 1
	}

	for i := startIdx; i < len(turns); i++ {
		// Build sliding window frequency map from last W turns.
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

		dist := weightedJaccardDivergence(refFreq, windowFreq)

		if dist > cfg.DriftThreshold {
			consecutiveDrift++
			if consecutiveDrift >= int(cfg.SustainedTurns) {
				sustained = true
			}
			if sustained {
				driftTurns = append(driftTurns, turns[i].GetTurnNumber())
				if dist > maxDrift {
					maxDrift = dist
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

	return &reasoningv1.CognitiveAssessment{
		FindingType:   reasoningv1.FindingType_SCOPE_DRIFT,
		Severity:      scopeSeverityFromDrift(maxDrift),
		Explanation:   "Conversation topics have drifted from the stated objective",
		RelevantTurns: driftTurns,
		Confidence:    maxDrift,
		DetectorName:  "scope-guard",
		Scope: &reasoningv1.ScopeDetail{
			DriftDistance:    maxDrift,
			CurrentTopics:   maxDriftTopics,
			ObjectiveTopics: objectiveKW,
		},
	}
}

// weightedJaccardDivergence computes 1 - (sum of min weights) / (sum of max weights)
// for two frequency maps. This is the weighted Jaccard distance.
func weightedJaccardDivergence(a, b map[string]float64) float64 {
	allKeys := make(map[string]bool)
	for k := range a {
		allKeys[k] = true
	}
	for k := range b {
		allKeys[k] = true
	}
	if len(allKeys) == 0 {
		return 0.0
	}

	var minSum, maxSum float64
	for k := range allKeys {
		va, vb := a[k], b[k]
		if va < vb {
			minSum += va
			maxSum += vb
		} else {
			minSum += vb
			maxSum += va
		}
	}
	if maxSum == 0 {
		return 0.0
	}
	return 1.0 - minSum/maxSum
}

func scopeSeverityFromDrift(drift float64) reasoningv1.FindingSeverity {
	if drift >= 0.95 {
		return reasoningv1.FindingSeverity_CRITICAL
	}
	if drift >= 0.85 {
		return reasoningv1.FindingSeverity_WARNING
	}
	if drift >= 0.75 {
		return reasoningv1.FindingSeverity_CAUTION
	}
	return reasoningv1.FindingSeverity_INFO
}

// ============================================================
// Confidence Calibrator
// ============================================================

type CalibratorConfig struct {
	MinMiscalibration float64
}

func DefaultCalibratorConfig() CalibratorConfig {
	return CalibratorConfig{MinMiscalibration: 0.5}
}

var confidenceKeywords = []struct {
	level    reasoningv1.ConfidenceLevel
	keywords []string
}{
	{
		level: reasoningv1.ConfidenceLevel_CERTAIN,
		keywords: []string{
			"definitely", "certainly", "i'm sure", "i\u2019m sure",
			"absolutely", "without a doubt", "100%",
		},
	},
	{
		level: reasoningv1.ConfidenceLevel_LIKELY,
		keywords: []string{
			"probably", "i think", "likely", "most likely", "i believe",
		},
	},
	{
		level: reasoningv1.ConfidenceLevel_POSSIBLE,
		keywords: []string{
			"maybe", "could be", "possibly", "might", "perhaps",
		},
	},
	{
		level: reasoningv1.ConfidenceLevel_SPECULATIVE,
		keywords: []string{
			"i wonder", "hypothetically", "what if", "suppose",
		},
	},
}

var evidenceMarkers = []string{
	"because", "since", "evidence shows", "data indicates",
	"according to", "studies show",
}

func DetectConfidenceMiscalibration(snap *reasoningv1.ConversationSnapshot, cfg CalibratorConfig) *reasoningv1.CognitiveAssessment {
	if snap == nil {
		return nil
	}

	var worstTurn uint32
	var worstECE float64
	var worstConf reasoningv1.ConfidenceLevel
	var worstWarrant reasoningv1.EpistemicStatus
	var worstSev reasoningv1.FindingSeverity
	found := false

	for _, turn := range snap.GetTurns() {
		text := calibratorNormalize(turn.GetRawText())
		if text == "" {
			continue
		}

		conf := detectConfidenceLevel(text)
		if conf == reasoningv1.ConfidenceLevel_CONFIDENCE_LEVEL_UNSPECIFIED ||
			conf == reasoningv1.ConfidenceLevel_UNKNOWN {
			continue
		}

		warrant := assessEvidenceLevel(text)
		ece := computeECE(conf, warrant)
		sev := classifyCalibrationSeverity(conf, warrant)

		if ece < cfg.MinMiscalibration {
			continue
		}

		if !found || ece > worstECE {
			worstTurn = turn.GetTurnNumber()
			worstECE = ece
			worstConf = conf
			worstWarrant = warrant
			worstSev = sev
			found = true
		}
	}

	if !found {
		return nil
	}

	return &reasoningv1.CognitiveAssessment{
		FindingType:   reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION,
		Severity:      worstSev,
		Explanation:   "Expressed confidence does not match evidence density",
		RelevantTurns: []uint32{worstTurn},
		Confidence:    worstECE,
		DetectorName:  "confidence-calibrator",
		Calibration: &reasoningv1.CalibrationDetail{
			ExpectedCalibrationError: worstECE,
			Expressed:                worstConf,
			ActualWarrant:            worstWarrant,
		},
	}
}

func calibratorNormalize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "\u2018", "'")
	s = strings.ReplaceAll(s, "\u2019", "'")
	s = strings.ReplaceAll(s, "\u201c", "\"")
	s = strings.ReplaceAll(s, "\u201d", "\"")
	return s
}

func detectConfidenceLevel(text string) reasoningv1.ConfidenceLevel {
	for _, group := range confidenceKeywords {
		for _, kw := range group.keywords {
			if strings.Contains(text, kw) {
				return group.level
			}
		}
	}
	return reasoningv1.ConfidenceLevel_CONFIDENCE_LEVEL_UNSPECIFIED
}

func assessEvidenceLevel(text string) reasoningv1.EpistemicStatus {
	count := 0
	for _, marker := range evidenceMarkers {
		if strings.Contains(text, marker) {
			count++
		}
	}
	for i := 0; i < len(text)-2; i++ {
		if text[i] == '[' && text[i+1] >= '0' && text[i+1] <= '9' {
			for j := i + 2; j < len(text); j++ {
				if text[j] == ']' {
					count++
					break
				}
				if text[j] < '0' || text[j] > '9' {
					break
				}
			}
		}
	}

	switch {
	case count >= 2:
		return reasoningv1.EpistemicStatus_EVIDENCED
	case count == 1:
		return reasoningv1.EpistemicStatus_INFERRED
	default:
		return reasoningv1.EpistemicStatus_SPECULATED
	}
}

func confidenceRank(c reasoningv1.ConfidenceLevel) float64 {
	switch c {
	case reasoningv1.ConfidenceLevel_CERTAIN:
		return 3.0
	case reasoningv1.ConfidenceLevel_LIKELY:
		return 2.0
	case reasoningv1.ConfidenceLevel_POSSIBLE:
		return 1.0
	default:
		return 0.0
	}
}

func evidenceRank(e reasoningv1.EpistemicStatus) float64 {
	switch e {
	case reasoningv1.EpistemicStatus_EVIDENCED:
		return 3.0
	case reasoningv1.EpistemicStatus_INFERRED:
		return 2.0
	case reasoningv1.EpistemicStatus_SPECULATED:
		return 1.0
	default:
		return 0.0
	}
}

func computeECE(conf reasoningv1.ConfidenceLevel, warrant reasoningv1.EpistemicStatus) float64 {
	const maxRank = 3.0
	return math.Abs(confidenceRank(conf)-evidenceRank(warrant)) / maxRank
}

func classifyCalibrationSeverity(conf reasoningv1.ConfidenceLevel, warrant reasoningv1.EpistemicStatus) reasoningv1.FindingSeverity {
	switch {
	case conf == reasoningv1.ConfidenceLevel_CERTAIN && warrant == reasoningv1.EpistemicStatus_SPECULATED:
		return reasoningv1.FindingSeverity_CRITICAL
	case conf == reasoningv1.ConfidenceLevel_CERTAIN && warrant == reasoningv1.EpistemicStatus_INFERRED:
		return reasoningv1.FindingSeverity_WARNING
	case conf == reasoningv1.ConfidenceLevel_LIKELY && warrant == reasoningv1.EpistemicStatus_SPECULATED:
		return reasoningv1.FindingSeverity_CAUTION
	default:
		return reasoningv1.FindingSeverity_INFO
	}
}

// ============================================================
// Decision Ledger (Silent Revision Detector)
// ============================================================

type LedgerConfig struct {
	TopicSimilarityThreshold float64
}

func DefaultLedgerConfig() LedgerConfig {
	return LedgerConfig{TopicSimilarityThreshold: 0.5}
}

var ledgerDecisionMarkers = []string{
	"let's go with", "we'll use", "i'll choose", "decided to", "going with",
	"the plan is", "we should", "i recommend",
}

var ledgerRationaleMarkers = []string{
	"because", "since", "the reason", "after reconsidering", "given that",
}

var ledgerWeakRationaleMarkers = []string{
	"just", "actually",
}

type ledgerDecision struct {
	marker     string
	topic      string
	turnNumber uint32
}

func DetectSilentRevision(snap *reasoningv1.ConversationSnapshot, cfg LedgerConfig) *reasoningv1.CognitiveAssessment {
	if snap == nil {
		return nil
	}

	var decisions []ledgerDecision

	for _, turn := range snap.GetTurns() {
		lower := textutil.NormalizeQuotes(strings.ToLower(turn.GetRawText()))

		for _, marker := range ledgerDecisionMarkers {
			idx := strings.Index(lower, marker)
			if idx < 0 {
				continue
			}
			topic := ledgerExtractTopic(lower, idx+len(marker))
			if topic == "" {
				continue
			}
			decisions = append(decisions, ledgerDecision{
				marker:     marker,
				topic:      topic,
				turnNumber: turn.GetTurnNumber(),
			})
			break
		}
	}

	if len(decisions) < 2 {
		return nil
	}

	for i := 0; i < len(decisions); i++ {
		for j := i + 1; j < len(decisions); j++ {
			earlier := decisions[i]
			later := decisions[j]

			if later.turnNumber <= earlier.turnNumber {
				continue
			}

			if ledgerTopicSimilarity(earlier.topic, later.topic) < cfg.TopicSimilarityThreshold {
				continue
			}

			if earlier.topic == later.topic {
				continue
			}

			laterTurnText := ledgerGetTurnText(snap, later.turnNumber)

			if ledgerHasStrongRationale(laterTurnText) {
				continue
			}

			severity := reasoningv1.FindingSeverity_WARNING
			confidence := 0.8

			if ledgerHasWeakRationale(laterTurnText) {
				severity = reasoningv1.FindingSeverity_CAUTION
				confidence = 0.6
			}

			return &reasoningv1.CognitiveAssessment{
				FindingType:   reasoningv1.FindingType_SILENT_REVISION,
				Severity:      severity,
				Explanation:   "Decision on similar topic changed without stated rationale — possible silent revision",
				RelevantTurns: []uint32{earlier.turnNumber, later.turnNumber},
				Confidence:    confidence,
				DetectorName:  "decision-ledger",
			}
		}
	}

	return nil
}

func ledgerExtractTopic(lower string, start int) string {
	rest := strings.TrimSpace(lower[start:])
	words := strings.Fields(rest)
	if len(words) == 0 {
		return ""
	}
	limit := len(words)
	if limit > 8 {
		limit = 8
	}
	return strings.Join(words[:limit], " ")
}

func ledgerTopicSimilarity(a, b string) float64 {
	wordsA := ledgerWordSet(a)
	wordsB := ledgerWordSet(b)

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}

	intersection := 0
	for w := range wordsA {
		if wordsB[w] {
			intersection++
		}
	}

	union := len(wordsA)
	for w := range wordsB {
		if !wordsA[w] {
			union++
		}
	}

	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func ledgerWordSet(s string) map[string]bool {
	stopWords := map[string]bool{
		"a": true, "an": true, "the": true, "to": true, "for": true,
		"of": true, "in": true, "on": true, "at": true, "and": true,
		"or": true, "is": true, "it": true, "we": true, "i": true,
		"that": true, "this": true, "with": true, "just": true,
		"actually": true,
	}
	set := make(map[string]bool)
	for _, w := range strings.Fields(s) {
		w = strings.TrimRight(w, ".,;:!?")
		if !stopWords[w] && len(w) > 1 {
			set[w] = true
		}
	}
	return set
}

func ledgerGetTurnText(snap *reasoningv1.ConversationSnapshot, turnNum uint32) string {
	for _, t := range snap.GetTurns() {
		if t.GetTurnNumber() == turnNum {
			return textutil.NormalizeQuotes(strings.ToLower(t.GetRawText()))
		}
	}
	return ""
}

func ledgerHasStrongRationale(text string) bool {
	lower := textutil.NormalizeQuotes(strings.ToLower(text))
	for _, marker := range ledgerRationaleMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func ledgerHasWeakRationale(text string) bool {
	lower := textutil.NormalizeQuotes(strings.ToLower(text))
	for _, marker := range ledgerWeakRationaleMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
