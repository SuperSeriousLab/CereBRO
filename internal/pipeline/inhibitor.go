package pipeline

import (
	"fmt"
	"strings"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/textutil"
)

// InhibitorConfig holds the Context Inhibitor's tunable parameters.
type InhibitorConfig struct {
	CorroborationThreshold  float64
	ConfidenceThresholdWarn float64
	FormalityThreshold      float64
	StakesThreshold         float64
	CasualHedgeWords        []string
	ProximityWindowTurns    uint32
}

// DefaultInhibitorConfig returns the Phase 1 default configuration.
func DefaultInhibitorConfig() InhibitorConfig {
	return InhibitorConfig{
		CorroborationThreshold:  0.1,
		ConfidenceThresholdWarn: 0.55,
		FormalityThreshold:      0.85,
		StakesThreshold:         0.1,
		CasualHedgeWords: []string{
			"absolutely", "definitely", "totally", "obviously",
			"literally", "clearly", "certainly",
		},
		ProximityWindowTurns: 2,
	}
}

// InhibitorResult holds the output of the Context Inhibitor.
type InhibitorResult struct {
	Decisions  []*cerebrov1.InhibitionDecision
	Gated      []*reasoningv1.CognitiveAssessment
	Formality  float64
	Urgency    float64 // real value from GainSignal (Phase 2), or 0.5 stub (Phase 1 fallback)
}

// InhibitWithGain runs the 5-gate algorithm with a real GainSignal (Phase 2).
func InhibitWithGain(
	assessments []*reasoningv1.CognitiveAssessment,
	snap *reasoningv1.ConversationSnapshot,
	cfg InhibitorConfig,
	gain *GainSignal,
) *InhibitorResult {
	return inhibitInternal(assessments, snap, cfg, gain.Formality, gain.Urgency)
}

// Inhibit runs the 5-gate basal ganglia inhibition algorithm.
// Default state: all findings INHIBITED. Each must earn disinhibition.
// Phase 1 fallback: computes formality inline, stubs urgency=0.5.
func Inhibit(
	assessments []*reasoningv1.CognitiveAssessment,
	snap *reasoningv1.ConversationSnapshot,
	cfg InhibitorConfig,
) *InhibitorResult {
	formality := ComputeFormality(snap)
	urgency := 0.5 // Phase 1 stub
	return inhibitInternal(assessments, snap, cfg, formality, urgency)
}

func inhibitInternal(
	assessments []*reasoningv1.CognitiveAssessment,
	snap *reasoningv1.ConversationSnapshot,
	cfg InhibitorConfig,
	formality, urgency float64,
) *InhibitorResult {

	// Pre-compute context features.
	activeDetectors := make(map[string]bool)
	// turnFindings maps turn_number → list of assessments involving that turn.
	turnFindings := make(map[uint32][]*reasoningv1.CognitiveAssessment)
	for _, a := range assessments {
		activeDetectors[a.GetDetectorName()] = true
		for _, t := range a.GetRelevantTurns() {
			turnFindings[t] = append(turnFindings[t], a)
		}
	}

	var decisions []*cerebrov1.InhibitionDecision
	var gated []*reasoningv1.CognitiveAssessment

	for _, a := range assessments {
		d := evaluateDisinhibition(a, assessments, snap, formality, urgency,
			activeDetectors, turnFindings, cfg)
		decisions = append(decisions, d)
		if d.GetAction() == cerebrov1.InhibitionAction_DISINHIBITED {
			gated = append(gated, a)
		}
	}

	return &InhibitorResult{
		Decisions: decisions,
		Gated:     gated,
		Formality: formality,
		Urgency:   urgency,
	}
}

func evaluateDisinhibition(
	finding *reasoningv1.CognitiveAssessment,
	allFindings []*reasoningv1.CognitiveAssessment,
	snap *reasoningv1.ConversationSnapshot,
	formality, urgency float64,
	activeDetectors map[string]bool,
	turnFindings map[uint32][]*reasoningv1.CognitiveAssessment,
	cfg InhibitorConfig,
) *cerebrov1.InhibitionDecision {
	fid := findingID(finding)

	// Gate 1: Casual hedging suppression — runs first because it overrides
	// severity auto-pass for CONFIDENCE_MISCALIBRATION in informal contexts.
	// Without this ordering, CRITICAL-severity miscalibration findings on
	// casual "absolutely"/"definitely" would auto-pass Gate 2.
	if finding.GetFindingType() == reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION {
		if formality < cfg.FormalityThreshold {
			triggerText := extractTriggerText(finding, snap)
			if containsCasualHedge(triggerText, cfg.CasualHedgeWords) {
				return makeDecision(fid, cerebrov1.InhibitionAction_INHIBITED,
					"casual_hedge_in_informal_context", 0, finding)
			}
		}
	}

	// Gate 2: Severity auto-pass — CRITICAL always disinhibits.
	if finding.GetSeverity() == reasoningv1.FindingSeverity_CRITICAL {
		return makeDecision(fid, cerebrov1.InhibitionAction_DISINHIBITED,
			"severity_auto_pass", 0, finding)
	}

	// Gate 3: Stakes gate — low urgency + low severity → suppress.
	if urgency < cfg.StakesThreshold {
		if finding.GetSeverity() <= reasoningv1.FindingSeverity_CAUTION {
			return makeDecision(fid, cerebrov1.InhibitionAction_INHIBITED,
				"low_stakes_low_severity", 0, finding)
		}
	}

	// Gate 4: Confidence gate — WARNING needs confidence above threshold.
	if finding.GetSeverity() == reasoningv1.FindingSeverity_WARNING {
		if finding.GetConfidence() < cfg.ConfidenceThresholdWarn {
			return makeDecision(fid, cerebrov1.InhibitionAction_INHIBITED,
				"warning_below_confidence_threshold", 0, finding)
		}
	}

	// Gate 5: Corroboration gate — cross-detector agreement.
	corr := computeCorroboration(finding, activeDetectors, turnFindings, cfg.ProximityWindowTurns)
	if corr < cfg.CorroborationThreshold {
		// Exception: very high confidence solo findings pass.
		if finding.GetConfidence() <= 0.9 {
			return makeDecision(fid, cerebrov1.InhibitionAction_INHIBITED,
				"insufficient_corroboration", corr, finding)
		}
	}

	// All gates passed.
	return makeDecision(fid, cerebrov1.InhibitionAction_DISINHIBITED,
		"all_gates_passed", corr, finding)
}

func computeCorroboration(
	finding *reasoningv1.CognitiveAssessment,
	activeDetectors map[string]bool,
	turnFindings map[uint32][]*reasoningv1.CognitiveAssessment,
	window uint32,
) float64 {
	otherCount := len(activeDetectors) - 1
	if otherCount <= 0 {
		return 1.0 // Only one detector active — can't require corroboration.
	}

	myTurns := finding.GetRelevantTurns()
	nearbyTurns := expandWindow(myTurns, window)

	corroboratingDetectors := make(map[string]bool)
	for _, t := range nearbyTurns {
		for _, other := range turnFindings[t] {
			if other.GetDetectorName() != finding.GetDetectorName() {
				corroboratingDetectors[other.GetDetectorName()] = true
			}
		}
	}

	return float64(len(corroboratingDetectors)) / float64(otherCount)
}

func expandWindow(turns []uint32, window uint32) []uint32 {
	seen := make(map[uint32]bool)
	for _, t := range turns {
		low := t
		if t > window {
			low = t - window
		} else {
			low = 1
		}
		for i := low; i <= t+window; i++ {
			seen[i] = true
		}
	}
	result := make([]uint32, 0, len(seen))
	for t := range seen {
		result = append(result, t)
	}
	return result
}

func extractTriggerText(finding *reasoningv1.CognitiveAssessment, snap *reasoningv1.ConversationSnapshot) string {
	var texts []string
	for _, turnNum := range finding.GetRelevantTurns() {
		for _, turn := range snap.GetTurns() {
			if turn.GetTurnNumber() == turnNum {
				texts = append(texts, turn.GetRawText())
				break
			}
		}
	}
	return strings.Join(texts, " ")
}

func containsCasualHedge(text string, hedgeWords []string) bool {
	normalized := textutil.NormalizeQuotes(strings.ToLower(text))
	words := tokenizeWords(normalized)
	hedgeSet := make(map[string]bool, len(hedgeWords))
	for _, w := range hedgeWords {
		hedgeSet[w] = true
	}
	for _, w := range words {
		if hedgeSet[w] {
			return true
		}
	}
	return false
}

func tokenizeWords(text string) []string {
	var words []string
	for _, w := range strings.Fields(text) {
		w = strings.Trim(w, ".,;:!?\"'()-[]{}/<>")
		if w != "" {
			words = append(words, w)
		}
	}
	return words
}

func findingID(a *reasoningv1.CognitiveAssessment) string {
	return fmt.Sprintf("%s:%v", a.GetDetectorName(), a.GetRelevantTurns())
}

func makeDecision(fid string, action cerebrov1.InhibitionAction, reason string,
	corroboration float64, finding *reasoningv1.CognitiveAssessment) *cerebrov1.InhibitionDecision {
	return &cerebrov1.InhibitionDecision{
		FindingId:          fid,
		Action:             action,
		Reason:             reason,
		CorroborationScore: corroboration,
		DetectorName:       finding.GetDetectorName(),
		FindingType:        finding.GetFindingType(),
	}
}

// ComputeFormality estimates conversational formality from 0.0 (very informal)
// to 1.0 (very formal). Mechanical heuristic — no LLM.
func ComputeFormality(snap *reasoningv1.ConversationSnapshot) float64 {
	if snap == nil {
		return 0.5
	}

	var formalCount, informalCount int

	for _, turn := range snap.GetTurns() {
		text := textutil.NormalizeQuotes(strings.ToLower(turn.GetRawText()))

		// Formal markers
		for _, marker := range formalMarkers {
			if strings.Contains(text, marker) {
				formalCount++
			}
		}

		// Informal markers
		for _, marker := range informalMarkers {
			if strings.Contains(text, marker) {
				informalCount++
			}
		}

		// Structural signals
		words := strings.Fields(text)
		if len(words) > 25 {
			formalCount++ // Long sentences suggest formality
		}
		if len(words) < 8 && len(words) > 0 {
			informalCount++ // Very short turns suggest informality
		}

		// Contractions are informal
		for _, c := range contractions {
			if strings.Contains(text, c) {
				informalCount++
				break
			}
		}

		// Exclamation marks are informal
		if strings.Count(text, "!") > 0 {
			informalCount++
		}
	}

	total := formalCount + informalCount
	if total == 0 {
		return 0.5
	}
	return float64(formalCount) / float64(total)
}

var formalMarkers = []string{
	"according to", "furthermore", "therefore", "consequently",
	"it is recommended", "i would suggest", "in my assessment",
	"based on the analysis", "the data suggests", "it should be noted",
	"with respect to", "in accordance with", "pursuant to",
	"the specification", "the requirement", "as per",
}

var informalMarkers = []string{
	"i guess", "kinda", "sorta", "gonna", "wanna", "gotta",
	"lol", "haha", "btw", "imo", "imho", "tbh",
	"yeah", "yep", "nah", "nope", "cool", "awesome",
	"hey ", "hi ", "yo ", "sup ",
}

var contractions = []string{
	"don't", "doesn't", "can't", "won't", "isn't", "aren't",
	"i'm", "you're", "we're", "they're", "it's", "that's",
	"i've", "you've", "we've", "they've", "i'd", "you'd",
	"couldn't", "wouldn't", "shouldn't", "let's",
}
