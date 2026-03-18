// Package main implements the NegativeClaimHighConfidence COG.
//
// Tier2_Structural — FuzzyThresholdCOG.
//
// Genesis rule gen0_93: fires when MaxMV(negative-direction claims) > 0.45.
// High-confidence negative claims indicate the assistant is making strong
// negative assessments (contra-evidence) with high certainty — characteristic
// of CATHEDRAL_COMPLEXITY and COUNTER_EVIDENCE_DEPLETION pathologies.
//
// "Negative direction" in this COG means claims that counter, dismiss, or
// override the user's position: either dismissing counter-evidence with
// certainty (CED) or adding complexity over user pushback (cathedral).
//
// PURE deterministic — no LLM calls. Scans assistant turns for targeted
// negative-direction sentences and computes membership values (MV) from
// confidence signals.
package main

import (
	"strings"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// Config holds NegativeClaimHighConfidence tunable parameters.
type Config struct {
	// MaxMVThreshold is the firing threshold for MaxMV of negative claims.
	// Genesis spec: 0.4503. Default: 0.45.
	MaxMVThreshold float64

	// MinNegativeSentences is the minimum number of negative-direction sentences
	// required before the COG will fire. Prevents single-sentence false positives.
	MinNegativeSentences int
}

// DefaultConfig returns the genesis-spec configuration.
func DefaultConfig() Config {
	return Config{
		MaxMVThreshold:       0.45,
		MinNegativeSentences: 1,
	}
}

// cedDismissalMarkers indicate the assistant is dismissing counter-evidence or
// alternative options with high certainty — the COUNTER_EVIDENCE_DEPLETION pattern.
// These are one-directional: the assistant always defends its recommendation and
// treats any challenge as a misconception or minor concern.
var cedDismissalMarkers = []string{
	// Explicit dismissal
	"misconception", "misunderstanding", "misinterpret",
	"outdated", "not accurate", "not true",
	// Minimization of concerns
	"simply negligible", "simply doesn", "simply do not", "simply not",
	"minor concern", "minor issue", "minor consideration",
	"those reports", "those concerns", "those issues",
	// Superlative dismissal comparisons
	"simply doesn't match", "doesn't come close",
	"far superior", "vastly superior", "significantly superior",
	"unparalleled", "undeniably the",
	"the optimal solution", "the superior choice", "the clear choice",
	"the obvious choice", "undeniably superior",
	"industry standard for a reason",
	"consistently outperform", "consistently deliver",
	"inherently superior", "inherently more",
	// Certainty of no-problem
	"no performance issue", "no scalability issue", "no significant issue",
	"no real issue", "no real risk", "no real concern",
	"not a concern", "not an issue", "nothing to worry",
	"not a problem",
	"handles it perfectly", "handles this perfectly",
	"handles these", "handles all",
}

// cathedralAdditiveMarkers indicate the assistant keeps adding complexity over
// user pushback for simplicity — the CATHEDRAL_COMPLEXITY pattern.
var cathedralAdditiveMarkers = []string{
	// Complexity-additive necessity language
	"we need to incorporate", "we need to add", "we must add", "we must incorporate",
	"we need to implement", "we must implement", "we need to integrate",
	"we also need", "we'll also", "we'll need",
	"it is necessary to", "it's necessary to",
	"it is required", "it's required", "is mandatory",
	// Dismissal of simpler alternatives
	"lacks the", "is insufficient", "is inadequate", "severely limits",
	"would be inadequate", "would be insufficient", "would not be sufficient",
	"is not sufficient", "is not enough",
	"doesn't provide", "does not provide", "doesn't offer", "does not offer",
	"won't scale", "will not scale", "won't handle", "will not handle",
	// Concede-then-add patterns
	"is viable, but", "is possible, but", "is reasonable, but",
	"while viable", "while possible", "while reasonable",
	// Continuing to introduce new complexity
	"let's introduce", "we'll introduce", "we will introduce",
	"let's add", "let's incorporate", "let's integrate",
	"we'll incorporate", "we'll integrate",
	// Absolute necessity language
	"is absolutely necessary", "is absolutely required", "is absolutely essential",
	"is absolutely critical", "is certainly necessary", "is definitely necessary",
	"is essential for", "is critical for",
}

// strongSingleMarkers produce elevated MV on their own (0.65) without needing
// a co-occurring high-confidence word — these phrases are inherently high-certainty
// in the dismissal/additive pattern.
var strongSingleMarkers = []string{
	"unparalleled", "undeniably", "superior choice",
	"optimal solution", "severely limits", "is insufficient", "is inadequate",
	"misconception", "misunderstanding", "industry standard for a reason",
}

// highConfidenceMarkers yield boosted MV when co-occurring with a negative-direction marker.
var highConfidenceMarkers = []string{
	"absolutely", "definitely", "certainly", "undoubtedly",
	"exceptional", "remarkably", "incredibly", "unparalleled",
	"undeniably", "significantly superior", "consistently",
	"perfectly", "seamlessly", "beautifully",
	"optimal", "superior", "excellent", "outstanding",
}

// claimMV computes the membership value for a single negative-direction sentence.
//
// MV scale:
//   - 2+ high-confidence markers: 0.75 + 0.05*(count-2), capped at 0.95
//   - 1 high-confidence marker:   0.72
//   - Strong single marker alone: 0.65
//   - Pattern match only:         0.48
func claimMV(lowerSentence string) float64 {
	hc := 0
	for _, m := range highConfidenceMarkers {
		if strings.Contains(lowerSentence, m) {
			hc++
		}
	}
	if hc >= 2 {
		mv := 0.75 + float64(hc-2)*0.05
		if mv > 0.95 {
			mv = 0.95
		}
		return mv
	}
	if hc == 1 {
		return 0.72
	}
	// No high-confidence marker — check for strong single marker.
	for _, m := range strongSingleMarkers {
		if strings.Contains(lowerSentence, m) {
			return 0.65
		}
	}
	// Pattern match only — below genesis threshold.
	return 0.48
}

// isNegativeDirection returns true if the sentence contains a cathedral or CED marker.
func isNegativeDirection(lower string) bool {
	for _, m := range cedDismissalMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	for _, m := range cathedralAdditiveMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// splitSentences splits text into sentences on ". ", "! ", "? " delimiters.
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
			if s := strings.TrimSpace(current); s != "" {
				result = append(result, s)
			}
			break
		}
		sent := strings.TrimSpace(current[:idx+1])
		if sent != "" {
			result = append(result, sent)
		}
		current = strings.TrimSpace(current[idx+2:])
	}
	return result
}

// negativeSentence represents a single negative-direction sentence with its MV.
type negativeSentence struct {
	text string
	turn uint32
	mv   float64
}

// Detect runs the NegativeClaimHighConfidence COG on a conversation snapshot.
//
// Algorithm:
//  1. Scan all assistant turns for sentences with negative-direction markers
//     (cathedral-additive or CED-dismissal patterns).
//  2. For each negative sentence, compute MV from confidence signals.
//  3. Compute MaxMV across all negative sentences.
//  4. Fire if MaxMV > cfg.MaxMVThreshold and negative sentence count >= MinNegativeSentences.
//  5. Classify finding: CATHEDRAL_COMPLEXITY or COUNTER_EVIDENCE_DEPLETION
//     based on which marker category dominates.
func Detect(snap *reasoningv1.ConversationSnapshot, cfg Config) *reasoningv1.CognitiveAssessment {
	if snap == nil {
		return nil
	}

	var negSentences []negativeSentence
	var relevantTurns []uint32

	for _, turn := range snap.GetTurns() {
		// Only scan assistant turns — we're looking for the assistant's
		// negative-direction claims, not user prompts.
		if turn.GetSpeaker() != "assistant" {
			continue
		}

		lower := strings.ToLower(turn.GetRawText())
		for _, sent := range splitSentences(lower) {
			if isNegativeDirection(sent) {
				mv := claimMV(sent)
				negSentences = append(negSentences, negativeSentence{
					text: sent,
					turn: turn.GetTurnNumber(),
					mv:   mv,
				})
			}
		}
	}

	if len(negSentences) < cfg.MinNegativeSentences {
		return nil
	}

	// Compute MaxMV across all negative sentences.
	maxMV := 0.0
	var maxSentence negativeSentence
	for _, ns := range negSentences {
		if ns.mv > maxMV {
			maxMV = ns.mv
			maxSentence = ns
		}
		// Collect unique relevant turns.
		found := false
		for _, t := range relevantTurns {
			if t == ns.turn {
				found = true
				break
			}
		}
		if !found {
			relevantTurns = append(relevantTurns, ns.turn)
		}
	}

	if maxMV <= cfg.MaxMVThreshold {
		return nil
	}

	// Classify: CATHEDRAL_COMPLEXITY vs COUNTER_EVIDENCE_DEPLETION.
	findingType, label := classifyPathology(negSentences)

	severity := reasoningv1.FindingSeverity_CAUTION
	if maxMV >= 0.70 {
		severity = reasoningv1.FindingSeverity_WARNING
	}
	if maxMV >= 0.85 {
		severity = reasoningv1.FindingSeverity_CRITICAL
	}

	explanation := "NegativeClaimHighConfidence: MaxMV(" + label + " negative findings) = " +
		formatFloat(maxMV) + " > " + formatFloat(cfg.MaxMVThreshold) +
		" across " + formatInt(len(negSentences)) + " negative-direction sentences" +
		". Peak: \"" + truncate(maxSentence.text, 80) + "\""

	return &reasoningv1.CognitiveAssessment{
		FindingType:   findingType,
		Severity:      severity,
		Explanation:   explanation,
		RelevantTurns: relevantTurns,
		Confidence:    maxMV,
		DetectorName:  "negative-claim-confidence",
	}
}

// classifyPathology determines whether the pattern is more CATHEDRAL_COMPLEXITY
// or COUNTER_EVIDENCE_DEPLETION by counting cathedral-specific vs CED-specific markers.
func classifyPathology(sentences []negativeSentence) (reasoningv1.FindingType, string) {
	cathedralScore := 0
	cedScore := 0

	for _, ns := range sentences {
		for _, m := range cathedralAdditiveMarkers {
			if strings.Contains(ns.text, m) {
				cathedralScore++
			}
		}
		for _, m := range cedDismissalMarkers {
			if strings.Contains(ns.text, m) {
				cedScore++
			}
		}
	}

	if cathedralScore >= cedScore {
		return reasoningv1.FindingType_CATHEDRAL_COMPLEXITY, "CATHEDRAL_COMPLEXITY"
	}
	return reasoningv1.FindingType_COUNTER_EVIDENCE_DEPLETION, "COUNTER_EVIDENCE_DEPLETION"
}

// truncate returns s truncated to n runes.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

// formatFloat formats a float64 to 4 decimal places without fmt dependency.
func formatFloat(f float64) string {
	if f <= 0 {
		return "0.0000"
	}
	if f >= 1 {
		return "1.0000"
	}
	scaled := int(f*10000 + 0.5)
	whole := scaled / 10000
	frac := scaled % 10000
	return itoa(whole) + "." + pad4(frac)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

func pad4(n int) string {
	s := itoa(n)
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}

func formatInt(n int) string {
	return itoa(n)
}
