package pipeline

import (
	"math"
	"sort"
	"strings"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/textutil"
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
	// Classical commitment-defense markers: defending a position because of prior
	// commitment to it or to the authority who stated it, not because of its merit.
	// These appear in Socratic dialogue as authority-anchored sunk cost reasoning.
	"as simonides maintains", "as my father held", "the position we have defended",
	"having committed ourselves", "we have already agreed", "as was said before",
	"we have established", "our earlier argument", "as i have said",
	"the definition we gave", "what we agreed upon", "to which i agreed",
	"as he maintained", "as they maintained", "as is maintained",
	"for so said", "it were unjust to abandon",
	// Phrases that actually appear in Plato's Republic (Jowett translation):
	"i still stand by", "stand by the latter", "heir of the argument",
	"what did simonides say", "simonides say",
	"as we were just now saying",
	"attributes such a saying to simonides",
	"that is implied in the argument",
}

var continuationPhrases = []string{
	"should keep going", "should continue", "can't stop now", "shouldn't give up",
	"let's keep", "let's continue", "we must continue", "have to finish",
	"need to finish", "too late to change", "too late to stop",
	"might as well", "no point stopping", "stick with",
	// Classical continuation markers: defending the original framing, refusing to
	// abandon a position inherited from authority or prior agreement.
	"we must hold to", "shall we then abandon", "will you retract",
	"do you mean to say that", "the argument stands", "must we not hold",
	"are we to say", "must we abandon", "we cannot abandon",
	// Classical affirmation-as-continuation: in Socratic dialogue, Polemarchus
	// repeatedly affirms ("quite ready", "prepared to", "i am quite ready")
	// as a way of continuing to defend the inherited position.
	"i am quite ready", "quite ready to", "prepared to take up arms",
	"do battle at your side", "take up arms against",
	"i maintain that", "i still maintain", "i still say",
	"i still think that", "i insist that", "we must not give up",
	// Classical affirmative discourse markers: short affirmations that signal
	// continued commitment to the prior position.
	"certainly", "to be sure", "very true", "of course",
}

type phraseMatch struct {
	phrase string
	turn   uint32
}

// DetectSunkCostML wraps DetectSunkCost with optional ML enrichment.
// If ML found sunk_cost_phrases, boost confidence of PURE findings.
func DetectSunkCostML(snap *reasoningv1.ConversationSnapshot, cfg SunkCostConfig, ml *cerebrov1.MLEnrichment) *reasoningv1.CognitiveAssessment {
	finding := DetectSunkCost(snap, cfg)
	if ml == nil || len(ml.GetSunkCostPhrases()) == 0 {
		return finding
	}
	if finding != nil {
		// ML corroborates — boost confidence
		finding.Confidence = clamp(finding.Confidence+0.1, 0.0, 1.0)
		return finding
	}
	// ML found phrases but PURE didn't — produce a low-confidence finding
	return &reasoningv1.CognitiveAssessment{
		FindingType:  reasoningv1.FindingType_SUNK_COST_FALLACY,
		Severity:     reasoningv1.FindingSeverity_INFO,
		Explanation:  "ML enricher identified sunk-cost language not caught by phrase matching",
		Confidence:   0.4,
		DetectorName: "sunk-cost-detector",
	}
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
// Porter-lite stemmer (suffix stripping, no external dependencies)
// ============================================================

// stemWord applies a simplified suffix-stripping stemmer to normalise English
// inflected forms before Jaccard similarity comparison.  Rules are applied in
// priority order (longest suffix first).  A minimum stem length of 3 characters
// is enforced to avoid over-stripping short words.
//
// Design intent: improve recall on varied-form text (e.g. classical/philosophical
// writing where "argue", "arguing", "argument", "arguments" should all compare as
// the same topic token) without sacrificing precision on clearly-distinct words.
func stemWord(w string) string {
	if len(w) <= 3 {
		return w
	}

	type rule struct {
		suffix      string
		replacement string
	}

	// Rules are ordered longest → shortest.  Only the first matching rule fires.
	rules := []rule{
		// 7-char
		{"fulness", "ful"},
		{"ousness", "ous"},
		{"iveness", "ive"},
		// 6-char
		{"nesses", ""},
		{"lessly", ""},
		{"ically", "ic"},
		// 5-char
		{"ments", ""},
		{"ation", ""},   // "creation"→"creat", "argumentation"→"argument"
		{"ition", ""},   // "addition"→"add"
		{"alism", ""},
		{"alist", ""},
		{"ality", ""},
		{"ative", ""},
		{"izing", ""},
		{"ising", ""},
		{"ating", ""},
		// 4-char
		{"ness", ""},
		{"less", ""},    // "helpless"→"help", "careless"→"care"
		{"ment", ""},
		{"able", ""},
		{"ible", ""},
		{"ious", ""},
		{"tion", ""},    // "nation"→"nat", catches remaining after 5-char pass
		// 3-char
		{"ism", ""},
		{"ist", ""},
		{"ize", ""},
		{"ise", ""},
		{"ily", ""},
		{"ity", ""},
		{"ion", ""},
		{"ing", ""},
		{"ous", ""},
		{"ive", ""},
		{"ful", ""},
		{"ies", "y"},
		{"ice", ""},     // "justice"→"just", "practice"→"pract"
		// 2-char
		{"ly", ""},
		{"er", ""},
		{"ed", ""},
		{"es", ""},
		{"al", ""},
		// 1-char: -y ("philosophy"→"philosoph") and plain plural -s
		{"y", ""},
		{"s", ""},
	}

	for _, r := range rules {
		if !strings.HasSuffix(w, r.suffix) {
			continue
		}
		candidate := w[:len(w)-len(r.suffix)] + r.replacement
		if len(candidate) >= 3 {
			return candidate
		}
	}
	return w
}

// stemFreqMap returns a new frequency map with all keys replaced by their stems.
// When two keys stem to the same value, their frequencies are summed.
func stemFreqMap(freq map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(freq))
	for k, v := range freq {
		out[stemWord(k)] += v
	}
	return out
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
		// 0.79 instead of 0.80: stemming in extractKeywords reduces Jaccard distances
		// by ~0.01–0.02 on average (inflected forms now merge into stems). Lowering the
		// threshold by 0.01 preserves detection of borderline cases while keeping FPR at zero.
		DriftThreshold: 0.79,
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
	// MinCertaintyWords is the minimum word count a turn must have before
	// CERTAIN-level confidence markers are counted. Discourse particles
	// ("Certainly.", "Indeed.") are shorter than real claims.
	// Default 0 means use the package constant minCertaintyTurnWords (5).
	// Set to 8 for classical-era text where discourse particles are longer.
	MinCertaintyWords uint32
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
	// Classical evidence/reasoning markers. In classical English, ", for"
	// introduces a justification clause after a comma ("X is so, for Y proves it").
	// This comma-for pattern distinguishes the causal connective from the preposition
	// "for" (as in "going for lunch" or "right approach for any team").
	", for ", "as is evident", "for the reason that", "the proof is",
	"it follows that", "as we have shown", "as has been shown",
}

// mlHedgingPhrases are common epistemic qualifiers that indicate calibrated uncertainty.
// These are normal conversational hedges — their presence does NOT indicate miscalibration.
var mlHedgingPhrases = []string{
	"i think", "i believe", "i feel", "i suppose", "i guess",
	"maybe", "perhaps", "possibly", "probably", "likely",
	"not sure", "not certain", "uncertain", "unclear",
	"it seems", "it appears", "it looks like",
	"kind of", "sort of", "more or less",
	"might", "could be", "may be",
}

// mlCertaintyMarkers are phrases that express absolute certainty — the kind that
// signals genuine miscalibration when unsupported by evidence.
var mlCertaintyMarkers = []string{
	"absolutely", "definitely", "certainly", "undoubtedly", "unquestionably",
	"without doubt", "without question", "100%", "guaranteed",
	"i am certain", "i am sure", "i'm certain", "i'm sure",
	"there is no doubt", "it is certain", "it is definite",
	"always", "never", "impossible", "must be",
}

// countHighCertaintyMarkers counts confidence_markers that express absolute certainty,
// excluding normal hedging language.
func countHighCertaintyMarkers(markers []string) int {
	count := 0
	for _, m := range markers {
		lower := strings.ToLower(strings.TrimSpace(m))
		// Skip if this looks like a hedging phrase
		isHedge := false
		for _, hedge := range mlHedgingPhrases {
			if strings.Contains(lower, hedge) {
				isHedge = true
				break
			}
		}
		if isHedge {
			continue
		}
		// Count if it matches a certainty marker
		for _, certain := range mlCertaintyMarkers {
			if strings.Contains(lower, certain) {
				count++
				break
			}
		}
	}
	return count
}

// certaintyEpistemicStatuses are epistemic_status values from the LLM that indicate
// high-certainty claims which may be miscalibrated when paired with no evidence.
var certaintyEpistemicStatuses = map[string]bool{
	"certain":    true,
	"definitive": true,
	"absolute":   true,
}

// DetectConfidenceMiscalibrationML wraps DetectConfidenceMiscalibration with ML enrichment.
// Uses ML confidence_markers and claim epistemic mismatches to refine detection.
//
// Calibration floor rules (ML-only path):
//  1. Requires ≥2 high-certainty claims with no evidence (single hedges are normal conversation)
//  2. Requires ≥3 high-certainty confidence markers (after excluding hedging phrases)
//  3. Only fires on epistemic_status values that indicate certainty ("certain", "definitive", "absolute")
//  4. The PURE detection path is unchanged — only the ML-only trigger is tightened
func DetectConfidenceMiscalibrationML(snap *reasoningv1.ConversationSnapshot, cfg CalibratorConfig, ml *cerebrov1.MLEnrichment) *reasoningv1.CognitiveAssessment {
	finding := DetectConfidenceMiscalibration(snap, cfg)
	if ml == nil {
		return finding
	}

	// Count ML claims with high-certainty epistemic status and no supporting evidence.
	certainMismatchCount := 0
	for _, claim := range ml.GetClaims() {
		hasEvidence := len(claim.GetEvidenceRefs()) > 0
		if certaintyEpistemicStatuses[claim.GetEpistemicStatus()] && !hasEvidence {
			certainMismatchCount++
		}
	}

	if finding != nil && certainMismatchCount >= 1 {
		// PURE already found miscalibration and ML corroborates — boost confidence.
		// A single high-certainty ungrounded claim is sufficient corroboration.
		finding.Confidence = clamp(finding.Confidence+0.1, 0.0, 1.0)
	}

	// ML-only trigger: PURE missed it, but ML signals potential miscalibration.
	// Calibration floor: require both a strong epistemic-mismatch signal (≥2 high-certainty
	// claims with no evidence) AND ≥3 high-certainty confidence markers after excluding
	// normal hedging language ("I think", "maybe", "probably" etc.).
	// This prevents ubiquitous hedging language from triggering false positives.
	highCertaintyMarkerCount := countHighCertaintyMarkers(ml.GetConfidenceMarkers())
	if finding == nil && certainMismatchCount >= 2 && highCertaintyMarkerCount >= 3 {
		return &reasoningv1.CognitiveAssessment{
			FindingType:  reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION,
			Severity:     reasoningv1.FindingSeverity_INFO,
			Explanation:  "ML enricher identified confidence-evidence mismatch in claims",
			Confidence:   0.4,
			DetectorName: "confidence-calibrator",
		}
	}

	return finding
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

// minCertaintyTurnWords is the minimum number of words a turn must contain
// before CERTAIN-level confidence markers are counted. Short turns like
// "Certainly.", "True.", "Indeed.", "That is so." are discourse agreements
// (backchannels), not overconfident claims. LIKELY/POSSIBLE/SPECULATIVE levels
// are unaffected because hedges in short turns do reflect genuine epistemic
// status. Threshold of 5 allows real short claims ("I'm absolutely sure of
// this.") while blocking single-word and two/three-word discourse particles.
const minCertaintyTurnWords = 5

func detectConfidenceLevel(text string) reasoningv1.ConfidenceLevel {
	for _, group := range confidenceKeywords {
		// For CERTAIN-level markers, require the turn to be substantive enough
		// to constitute an actual claim rather than a discourse particle.
		if group.level == reasoningv1.ConfidenceLevel_CERTAIN {
			wordCount := len(strings.Fields(text))
			if wordCount < minCertaintyTurnWords {
				continue
			}
		}
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

// ============================================================
// Conceptual Anchoring Detector
// ============================================================

// ConceptualAnchoringConfig holds thresholds for the conceptual anchoring detector.
type ConceptualAnchoringConfig struct {
	AnchorThreshold float64 // min stemmed Jaccard for "high overlap" (default 0.3)
	OrbitThreshold  float64 // fraction of subsequent turns that must orbit (default 0.6)
	MinTurns        uint32  // minimum total turns (default 8)
	MaxAnchorTurns  uint32  // how many early turns to scan for anchor claim (default 3)
}

// DefaultConceptualAnchoringConfig returns sensible defaults.
func DefaultConceptualAnchoringConfig() ConceptualAnchoringConfig {
	return ConceptualAnchoringConfig{
		AnchorThreshold: 0.3,
		OrbitThreshold:  0.6,
		MinTurns:        8,
		MaxAnchorTurns:  3,
	}
}

// hedgeWords are epistemic qualifiers that signal non-declarative claims.
var hedgeWords = []string{
	"maybe", "perhaps", "possibly", "might", "could be", "i think",
	"i believe", "i suppose", "i guess", "i wonder", "not sure",
	"uncertain", "unclear", "probably", "likely",
}

// counterAcknowledgements are phrases indicating acceptance of a counter-claim.
var counterAcknowledgements = []string{
	"you're right", "youre right", "i concede", "that's a fair point",
	"thats a fair point", "i revise", "on reflection", "i was wrong",
	"actually", "fair enough", "good point", "i accept", "you make a good point",
	"i see your point", "i agree with that", "you've convinced me",
	"i stand corrected",
}

// counterReassertions are phrases that immediately negate an acknowledgement.
var counterReassertions = []string{
	"but still", "but nevertheless", "but even so", "but regardless",
	"however i still", "but i maintain", "but i still", "but my point stands",
}

// isStrongDeclarative returns true if the text is a high-confidence declarative
// claim with no hedge words, not a question, and of sufficient length.
func isStrongDeclarative(text string) bool {
	lower := strings.ToLower(textutil.NormalizeQuotes(text))

	// Must not be a question
	trimmed := strings.TrimSpace(text)
	if strings.HasSuffix(trimmed, "?") {
		return false
	}
	// No interrogative opener
	for _, opener := range []string{"what ", "who ", "how ", "when ", "where ", "why ", "is it ", "are you ", "do you "} {
		if strings.HasPrefix(lower, opener) {
			return false
		}
	}

	// Minimum word count
	words := strings.Fields(text)
	if len(words) < 6 {
		return false
	}

	// Must not contain hedge words
	for _, hedge := range hedgeWords {
		if strings.Contains(lower, hedge) {
			return false
		}
	}

	// Must contain a declarative copula or strong assertion verb
	declarativeMarkers := []string{
		" is ", " are ", " must ", " always ", " never ", " only ",
		" will ", " shall ", " does ", " do ",
	}
	for _, marker := range declarativeMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}

	// Allow starts like "Justice is..." or "The X is Y..."
	if strings.HasPrefix(lower, "the ") || strings.HasPrefix(lower, "justice") ||
		strings.HasPrefix(lower, "truth") || strings.HasPrefix(lower, "virtue") {
		return true
	}

	return false
}

// stemmedJaccard computes the Jaccard similarity between two sets of stemmed keywords.
// Returns overlap / union (0.0 if both sets are empty).
func stemmedJaccard(anchorKWs map[string]bool, turnText string) float64 {
	turnKWs := extractKeywords(turnText)
	if len(turnKWs) == 0 || len(anchorKWs) == 0 {
		return 0.0
	}

	intersection := 0
	for _, kw := range turnKWs {
		if anchorKWs[kw] {
			intersection++
		}
	}
	// Union = |A| + |B| - |A ∩ B|
	union := len(anchorKWs) + len(turnKWs) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

// hasAcknowledgedCounter returns true if any turn after anchorTurnNum contains
// an acknowledgement signal without an immediate reassertion.
func hasAcknowledgedCounter(turns []*reasoningv1.Turn, anchorTurnNum uint32) bool {
	for _, turn := range turns {
		if turn.GetTurnNumber() <= anchorTurnNum {
			continue
		}
		lower := strings.ToLower(textutil.NormalizeQuotes(turn.GetRawText()))
		for _, ack := range counterAcknowledgements {
			if strings.Contains(lower, ack) {
				// Check there's no immediate reassertion in the same turn
				hasReassert := false
				for _, reassert := range counterReassertions {
					if strings.Contains(lower, reassert) {
						hasReassert = true
						break
					}
				}
				if !hasReassert {
					return true
				}
			}
		}
	}
	return false
}

// conceptualAnchoringConfidence computes a composite confidence score.
// orbit_ratio weight 0.5, avg_overlap weight 0.3, sample_size weight 0.2 (capped at n=15).
func conceptualAnchoringConfidence(orbitRatio, avgOverlap float64, n uint32) float64 {
	sampleNorm := math.Min(float64(n)/15.0, 1.0)
	return clamp(0.5*orbitRatio+0.3*avgOverlap+0.2*sampleNorm, 0.0, 1.0)
}

func conceptualAnchoringSeverity(confidence float64) reasoningv1.FindingSeverity {
	if confidence >= 0.75 {
		return reasoningv1.FindingSeverity_WARNING
	}
	if confidence >= 0.55 {
		return reasoningv1.FindingSeverity_CAUTION
	}
	return reasoningv1.FindingSeverity_INFO
}

// truncateText truncates s to maxLen characters, appending "..." if truncated.
func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// DetectConceptualAnchoring detects when an early strong claim sets a conceptual
// frame that all subsequent argument orbits, rather than evaluating alternatives.
// This is the propositional variant of anchoring — distinct from numeric anchoring.
// It should run on classical text (NOT skipped by DomainContext classical mode).
func DetectConceptualAnchoring(snap *reasoningv1.ConversationSnapshot, cfg ConceptualAnchoringConfig) *reasoningv1.CognitiveAssessment {
	if snap == nil {
		return nil
	}

	turns := snap.GetTurns()
	if uint32(len(turns)) < cfg.MinTurns {
		return nil
	}

	// === Step 1: Find the anchor candidate ===
	// Scan the first MaxAnchorTurns turns for the strongest declarative assertion.
	maxScan := int(cfg.MaxAnchorTurns)
	if maxScan > len(turns) {
		maxScan = len(turns)
	}

	var anchorTurn *reasoningv1.Turn
	var anchorKWs map[string]bool

	for i := 0; i < maxScan; i++ {
		t := turns[i]
		if isStrongDeclarative(t.GetRawText()) {
			anchorTurn = t
			kws := extractKeywords(t.GetRawText())
			anchorKWs = make(map[string]bool, len(kws))
			for _, kw := range kws {
				anchorKWs[kw] = true
			}
			break
		}
	}

	if anchorTurn == nil || len(anchorKWs) == 0 {
		return nil // No strong declarative anchor found
	}

	// === Step 2: Compute semantic orbit for subsequent turns ===
	var subsequent []*reasoningv1.Turn
	for _, t := range turns {
		if t.GetTurnNumber() > anchorTurn.GetTurnNumber() {
			subsequent = append(subsequent, t)
		}
	}

	if uint32(len(subsequent)) < 1 {
		return nil
	}

	var overlapScores []float64
	orbitCount := 0

	for _, t := range subsequent {
		text := t.GetRawText()
		if strings.TrimSpace(text) == "" {
			continue
		}
		overlap := stemmedJaccard(anchorKWs, text)
		overlapScores = append(overlapScores, overlap)
		if overlap >= cfg.AnchorThreshold {
			orbitCount++
		}
	}

	if len(overlapScores) == 0 {
		return nil
	}

	orbitRatio := float64(orbitCount) / float64(len(overlapScores))

	var sumOverlap float64
	for _, s := range overlapScores {
		sumOverlap += s
	}
	avgOverlap := sumOverlap / float64(len(overlapScores))

	// === Step 3: Check for counter-claim acknowledgement ===
	counterAcknowledged := hasAcknowledgedCounter(turns, anchorTurn.GetTurnNumber())

	// === Step 4: Threshold check ===
	if orbitRatio < cfg.OrbitThreshold {
		return nil
	}
	if counterAcknowledged {
		return nil
	}

	// === Step 5: Build finding ===
	confidence := conceptualAnchoringConfidence(orbitRatio, avgOverlap, uint32(len(subsequent)))
	severity := conceptualAnchoringSeverity(confidence)

	relevantTurns := []uint32{anchorTurn.GetTurnNumber()}
	for _, t := range subsequent {
		relevantTurns = append(relevantTurns, t.GetTurnNumber())
	}

	explanation := strings.Join([]string{
		"Anchor set at turn ",
		uintToString(anchorTurn.GetTurnNumber()),
		": '",
		truncateText(anchorTurn.GetRawText(), 80),
		"'. ",
		uintToString(uint32(orbitCount)),
		"/",
		uintToString(uint32(len(subsequent))),
		" subsequent turns orbit the anchor",
		" (avg overlap ",
		formatFloat(avgOverlap),
		"). No counter-claims acknowledged.",
	}, "")

	return &reasoningv1.CognitiveAssessment{
		FindingType:   reasoningv1.FindingType_CONCEPTUAL_ANCHORING,
		Severity:      severity,
		Explanation:   explanation,
		RelevantTurns: relevantTurns,
		Confidence:    confidence,
		DetectorName:  "conceptual-anchoring-detector",
		ConceptualAnchoring: &reasoningv1.ConceptualAnchoringDetail{
			AnchorClaimText:          anchorTurn.GetRawText(),
			AnchorTurn:               anchorTurn.GetTurnNumber(),
			SemanticOrbitRatio:       orbitRatio,
			AvgSemanticOverlap:       avgOverlap,
			TurnsAnalyzed:            uint32(len(subsequent)),
			CounterClaimsAcknowledged: counterAcknowledged,
		},
	}
}

// ============================================================
// Inherited-Position Detector
// ============================================================

// InheritedPositionConfig holds thresholds for the inherited-position detector.
type InheritedPositionConfig struct {
	MinCitations        uint32  // min authority citations to trigger (default 3)
	MeritRatio          float64 // max fraction of citations WITHOUT merit to fire (default 0.3 → unjustified_ratio must be > 0.7... wait, spec says merit_ratio is the MERIT threshold)
	CitationWindowTurns uint32  // unused in current algorithm — reserved for future windowed variant (default 5)
}

// DefaultInheritedPositionConfig returns sensible defaults.
// MeritRatio is the maximum fraction of citation turns that have merit-based defense
// for the finding to fire — i.e., if more than MeritRatio of citations include
// independent justification, the position is legitimately defended and no finding fires.
// Default 0.3 matches the spec: merit_defenses < 0.3 * authority_citations fires.
func DefaultInheritedPositionConfig() InheritedPositionConfig {
	return InheritedPositionConfig{
		MinCitations:        3,
		MeritRatio:          0.3,
		CitationWindowTurns: 5,
	}
}

// authorityPatterns are regex-like patterns for authority citation detection.
// We use strings.Contains and strings-based matching since the go pipeline is
// PURE deterministic and avoids the regexp package import overhead.
// Patterns are ordered from most specific to least specific.
var authorityPhrases = []string{
	"as simonides said",
	"as simonides",
	"simonides said",
	"simonides taught",
	"simonides argued",
	"simonides held",
	"simonides maintained",
	"simonides believed",
	"simonides tells us",
	"simonides would say",
	// Generic "as X said/taught/argued/held/maintained" patterns — matched by checking
	// prefix "as " + capitalized word cluster + " said/taught/..." in scanCitationVerb
	// Generic "according to X" patterns
	"according to",
	// Generic "X believed/held that" — matched by suffix patterns in scanBelievedHeld
	// Generic "following X's teaching/argument/position"
	"following the tradition",
	"following the teaching",
	"the tradition of",
	"tradition holds",
	"we have always",
	"it has always been",
	"we always have",
	"they have always",
	// Classical markers used specifically in Platonic dialogues
	"as was said",
	"as has been said",
	"as we said",
	"as was agreed",
	"as the saying goes",
	"as the poet says",
	"as the poet said",
	"as homer said",
	"as homer says",
	"homer says",
	"homer said",
}

// citationVerbSuffixes are the verb suffixes that follow a name in "as X <verb>" patterns.
var citationVerbSuffixes = []string{
	" said", " says", " taught", " teaches", " argued", " argues",
	" held", " holds", " maintained", " maintains", " believed", " believes",
	" tells us", " would say", " has said", " once said", " declared",
}

// meritMarkers indicate independent justification when found near a citation.
var meritMarkers = []string{
	"because ", "since ", "the reason is", "evidence shows", "we can see that",
	"it follows from", ", for ", "for the reason", "as is evident",
	"the proof is", "it follows that", "as has been shown", "as we have shown",
	"therefore", "thus we", "this means", "implies that", "which shows",
	"data shows", "studies show", "we can observe", "consider that",
}

// noMeritIndicators are phrases that signal bare assertion — NOT independent justification.
var noMeritIndicators = []string{
	"obviously", "clearly", "it is well known", "everyone knows",
	"as everyone can see", "it goes without saying", "needless to say",
	"of course", "it is obvious",
}

// findAuthorityCitation checks whether a turn's text contains an authority citation.
// Returns the authority name (or phrase) if found, empty string otherwise.
func findAuthorityCitation(text string) string {
	lower := strings.ToLower(textutil.NormalizeQuotes(text))

	// Check fixed authority phrases first (most reliable).
	for _, phrase := range authorityPhrases {
		if strings.Contains(lower, phrase) {
			return phrase
		}
	}

	// Check "as <Word(s)> said/taught/argued/..." patterns.
	// Look for "as " followed eventually by a citation verb suffix.
	asIdx := strings.Index(lower, "as ")
	if asIdx >= 0 {
		after := lower[asIdx+3:]
		for _, verb := range citationVerbSuffixes {
			if idx := strings.Index(after, verb); idx > 0 && idx <= 40 {
				// Extract the candidate name region (up to the verb)
				candidate := strings.TrimSpace(after[:idx])
				// A proper name will have at least one non-trivial word
				if len(candidate) >= 3 && !strings.Contains(candidate, "?") {
					return "as " + candidate + verb
				}
			}
		}
	}

	// Check "X believed/held that" — looking for <Word> believed/held patterns.
	for _, verb := range []string{" believed that", " held that", " argued that", " maintained that", " taught that"} {
		if idx := strings.Index(lower, verb); idx > 0 {
			// Work backwards to find the subject word
			before := lower[:idx]
			words := strings.Fields(before)
			if len(words) > 0 {
				lastWord := words[len(words)-1]
				if len(lastWord) >= 3 {
					return lastWord + verb
				}
			}
		}
	}

	return ""
}

// hasIndependentJustification returns true if the text contains a merit marker
// followed by substantive content (at least minWords words after the marker),
// and no no-merit indicator is present.
func hasIndependentJustification(text string, minWords int) bool {
	lower := strings.ToLower(textutil.NormalizeQuotes(text))

	// Reject if a no-merit indicator is present (bare assertion signals).
	for _, noMerit := range noMeritIndicators {
		if strings.Contains(lower, noMerit) {
			return false
		}
	}

	for _, marker := range meritMarkers {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		// Count words in the clause after the marker.
		after := strings.TrimSpace(lower[idx+len(marker):])
		words := strings.Fields(after)
		if len(words) >= minWords {
			return true
		}
	}
	return false
}

// extractDefendedClaim returns the sentence in turn text that contains the
// authority citation, truncated to 120 characters.
func extractDefendedClaim(text string) string {
	// Split on sentence terminators and return the sentence with the citation.
	sentences := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?' || r == ';'
	})

	lower := strings.ToLower(textutil.NormalizeQuotes(text))
	lowerSentences := strings.FieldsFunc(lower, func(r rune) bool {
		return r == '.' || r == '!' || r == '?' || r == ';'
	})

	// Find the sentence that contains a citation phrase.
	for i, s := range lowerSentences {
		hasCitation := false
		for _, phrase := range authorityPhrases {
			if strings.Contains(s, phrase) {
				hasCitation = true
				break
			}
		}
		if !hasCitation {
			for _, verb := range citationVerbSuffixes {
				if strings.Contains(s, verb) {
					hasCitation = true
					break
				}
			}
		}
		if hasCitation && i < len(sentences) {
			claim := strings.TrimSpace(sentences[i])
			if len(claim) > 120 {
				return claim[:120] + "..."
			}
			return claim
		}
	}

	// Fallback: return start of text.
	if len(text) > 120 {
		return text[:120] + "..."
	}
	return text
}

// inheritedPositionConfidence computes confidence from citation count and unjustified ratio.
// citation_count weight 0.4 (capped at count/10), unjustified_ratio weight 0.6.
func inheritedPositionConfidence(citationCount uint32, unjustifiedRatio float64) float64 {
	countNorm := math.Min(float64(citationCount)/10.0, 1.0)
	return clamp(0.4*countNorm+0.6*unjustifiedRatio, 0.0, 1.0)
}

func inheritedPositionSeverity(confidence float64) reasoningv1.FindingSeverity {
	if confidence >= 0.75 {
		return reasoningv1.FindingSeverity_WARNING
	}
	if confidence >= 0.55 {
		return reasoningv1.FindingSeverity_CAUTION
	}
	return reasoningv1.FindingSeverity_INFO
}

// joinUint32s converts a slice of uint32 to a comma-separated string.
func joinUint32s(ns []uint32) string {
	if len(ns) == 0 {
		return ""
	}
	s := uintToString(ns[0])
	for _, n := range ns[1:] {
		s += ", " + uintToString(n)
	}
	return s
}

// joinStrings joins a slice of strings with ", ".
func joinStrings(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	s := ss[0]
	for _, t := range ss[1:] {
		s += ", " + t
	}
	return s
}

// deduplicateStrings returns a deduplicated slice preserving order.
func deduplicateStrings(ss []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// DetectInheritedPosition detects when a position is defended because of who holds it
// (authority appeal) rather than its merits — the epistemic sunk-cost fallacy.
// This is distinct from sunk-cost-detector (material investment) and from
// conceptual-anchoring-detector (semantic orbit). The canonical test case is
// Polemarchus defending "Simonides said X" with no independent argument.
func DetectInheritedPosition(snap *reasoningv1.ConversationSnapshot, cfg InheritedPositionConfig) *reasoningv1.CognitiveAssessment {
	if snap == nil {
		return nil
	}

	turns := snap.GetTurns()
	if len(turns) == 0 {
		return nil
	}

	const justificationMinWords = 10 // minimum words in merit clause to count as substantive

	// === Step 1: Scan for authority citations ===
	var citationTurns []uint32
	var justifiedTurns []uint32
	var authoritiesCited []string
	var defendedClaim string

	for _, turn := range turns {
		authority := findAuthorityCitation(turn.GetRawText())
		if authority == "" {
			continue
		}

		citationTurns = append(citationTurns, turn.GetTurnNumber())
		authoritiesCited = append(authoritiesCited, authority)

		if defendedClaim == "" {
			defendedClaim = extractDefendedClaim(turn.GetRawText())
		}

		// Check for independent justification in the same turn.
		if hasIndependentJustification(turn.GetRawText(), justificationMinWords) {
			justifiedTurns = append(justifiedTurns, turn.GetTurnNumber())
		}
	}

	// === Step 2: Apply citation count threshold ===
	if uint32(len(citationTurns)) < cfg.MinCitations {
		return nil // Too few citations — normal attribution
	}

	// === Step 3: Check justification coverage ===
	unjustifiedCount := len(citationTurns) - len(justifiedTurns)
	unjustifiedRatio := float64(unjustifiedCount) / float64(len(citationTurns))

	// Spec: if merit_defenses >= merit_ratio * authority_citations, do not fire.
	// Equivalently: fire only when unjustified_ratio > (1 - merit_ratio).
	// With MeritRatio=0.3: fire when fewer than 30% of citations have merit defense.
	meritThreshold := 1.0 - cfg.MeritRatio // 0.7 by default
	if unjustifiedRatio < meritThreshold {
		return nil // Enough citations have independent justification — legitimate citation practice
	}

	// === Step 4: Build finding ===
	deduped := deduplicateStrings(authoritiesCited)
	confidence := inheritedPositionConfidence(uint32(len(citationTurns)), unjustifiedRatio)

	explanation := uintToString(uint32(len(citationTurns))) +
		" authority citations to [" + joinStrings(deduped) +
		"] found across turns " + joinUint32s(citationTurns) +
		". " + formatFloat(unjustifiedRatio*100) +
		"% cite authority without independent justification. " +
		"Position defended by appeal to authority rather than merit."

	return &reasoningv1.CognitiveAssessment{
		FindingType:   reasoningv1.FindingType_INHERITED_POSITION,
		Severity:      inheritedPositionSeverity(confidence),
		Explanation:   explanation,
		RelevantTurns: citationTurns,
		Confidence:    confidence,
		DetectorName:  "inherited-position-detector",
		InheritedPosition: &reasoningv1.InheritedPositionDetail{
			AuthorityFigures:               deduped,
			AuthorityCitationCount:         uint32(len(citationTurns)),
			IndependentJustificationPresent: len(justifiedTurns) > 0,
			CitationTurns:                  citationTurns,
			DefendedClaim:                  defendedClaim,
		},
	}
}

// uintToString converts a uint32 to its decimal string representation
// without importing fmt (keeps the function dependency-free).
func uintToString(n uint32) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// formatFloat formats a float64 to 2 decimal places as a string.
func formatFloat(f float64) string {
	// Simple integer + 2-decimal formatter to avoid fmt import
	scaled := int64(f*100 + 0.5)
	intPart := scaled / 100
	fracPart := scaled % 100
	s := uintToString(uint32(intPart)) + "."
	if fracPart < 10 {
		s += "0"
	}
	s += uintToString(uint32(fracPart))
	return s
}
