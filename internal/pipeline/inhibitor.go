package pipeline

import (
	"fmt"
	"strings"

	"github.com/SuperSeriousLab/fugo"
	"google.golang.org/protobuf/proto"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/textutil"
)

// FIS config names used in the FisRegistry for L3 gates.
const (
	FISFormality = "l3_formality_gate"
	FISSeverity  = "l3_severity_gate"
	FISEvidence  = "l3_evidence_gate"
)

// FuzzyInhibitor holds pre-built fugo engines for fuzzy gate evaluation.
// When nil, the inhibitor falls back to crisp binary logic.
type FuzzyInhibitor struct {
	FormalityEngine *fugo.FuzzyEngine
	SeverityEngine  *fugo.FuzzyEngine
	EvidenceEngine  *fugo.FuzzyEngine
}

// BuildFuzzyInhibitor constructs a FuzzyInhibitor from individual FIS configs.
// Returns nil and an error if any config fails to build.
func BuildFuzzyInhibitor(formalityCfg, severityCfg, evidenceCfg *fugo.FisConfig) (*FuzzyInhibitor, error) {
	fe, err := formalityCfg.BuildEngine()
	if err != nil {
		return nil, fmt.Errorf("formality FIS: %w", err)
	}
	se, err := severityCfg.BuildEngine()
	if err != nil {
		return nil, fmt.Errorf("severity FIS: %w", err)
	}
	ee, err := evidenceCfg.BuildEngine()
	if err != nil {
		return nil, fmt.Errorf("evidence FIS: %w", err)
	}
	return &FuzzyInhibitor{
		FormalityEngine: fe,
		SeverityEngine:  se,
		EvidenceEngine:  ee,
	}, nil
}

// BuildFuzzyInhibitorFromRegistry constructs a FuzzyInhibitor from a FisRegistry.
// Looks up configs by the standard gate names (l3_formality_gate, etc.).
func BuildFuzzyInhibitorFromRegistry(reg *fugo.FisRegistry) (*FuzzyInhibitor, error) {
	fe, err := reg.BuildEngine(FISFormality)
	if err != nil {
		return nil, fmt.Errorf("formality FIS: %w", err)
	}
	se, err := reg.BuildEngine(FISSeverity)
	if err != nil {
		return nil, fmt.Errorf("severity FIS: %w", err)
	}
	ee, err := reg.BuildEngine(FISEvidence)
	if err != nil {
		return nil, fmt.Errorf("evidence FIS: %w", err)
	}
	return &FuzzyInhibitor{
		FormalityEngine: fe,
		SeverityEngine:  se,
		EvidenceEngine:  ee,
	}, nil
}

// InhibitorConfig holds the Context Inhibitor's tunable parameters.
type InhibitorConfig struct {
	CorroborationThreshold  float64
	ConfidenceThresholdWarn float64
	FormalityThreshold      float64
	StakesThreshold         float64
	CasualHedgeWords        []string
	ProximityWindowTurns    uint32
	Fuzzy                   *FuzzyInhibitor // optional; nil = crisp fallback
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

	if cfg.Fuzzy != nil {
		// Fuzzy path: evaluate FIS gates, apply multiplicative suppression.
		for _, a := range assessments {
			d, suppressed := evaluateFuzzyInhibition(a, assessments, snap,
				formality, urgency, activeDetectors, turnFindings, cfg)
			decisions = append(decisions, d)
			if d.GetAction() == cerebrov1.InhibitionAction_DISINHIBITED {
				gated = append(gated, suppressed)
			}
		}
	} else {
		// Crisp path: original 5-gate binary logic.
		for _, a := range assessments {
			d := evaluateDisinhibition(a, assessments, snap, formality, urgency,
				activeDetectors, turnFindings, cfg)
			decisions = append(decisions, d)
			if d.GetAction() == cerebrov1.InhibitionAction_DISINHIBITED {
				gated = append(gated, a)
			}
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
	// Exception: classical/philosophical text (archaic vocabulary) scores
	// below 0.85 formality despite being genuinely formal — bypass Gate 1
	// for such texts so that real miscalibration findings are not suppressed.
	// isClassicalTextFor uses both generic and pathology-specific marker
	// dictionaries so that domain vocabulary in classical economic or
	// epistemic texts is also recognised.
	if finding.GetFindingType() == reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION {
		if formality < cfg.FormalityThreshold && !isClassicalTextFor(snap, finding.GetFindingType()) {
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

// evaluateFuzzyInhibition runs fuzzy FIS evaluation on gates 1, 3, 4, 5.
// Gate 1 (casual hedge) and Gate 2 (CRITICAL auto-pass) remain crisp overrides.
// For gates 3–5, the FIS engines produce inhibition_strength in [0,1].
// The finding's confidence is multiplied by (1 - max_inhibition_strength).
// Returns the decision and a (possibly suppressed) copy of the assessment.
func evaluateFuzzyInhibition(
	finding *reasoningv1.CognitiveAssessment,
	allFindings []*reasoningv1.CognitiveAssessment,
	snap *reasoningv1.ConversationSnapshot,
	formality, urgency float64,
	activeDetectors map[string]bool,
	turnFindings map[uint32][]*reasoningv1.CognitiveAssessment,
	cfg InhibitorConfig,
) (*cerebrov1.InhibitionDecision, *reasoningv1.CognitiveAssessment) {
	fid := findingID(finding)
	fz := cfg.Fuzzy

	// Gate 1 (crisp): Casual hedging suppression — still binary override.
	// Exception: classical/philosophical text bypasses this gate (see evaluateDisinhibition).
	// isClassicalTextFor uses pathology-specific marker dictionaries so that
	// domain vocabulary in classical epistemic or economic texts is recognised.
	if finding.GetFindingType() == reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION {
		if formality < cfg.FormalityThreshold && !isClassicalTextFor(snap, finding.GetFindingType()) {
			triggerText := extractTriggerText(finding, snap)
			if containsCasualHedge(triggerText, cfg.CasualHedgeWords) {
				return makeDecision(fid, cerebrov1.InhibitionAction_INHIBITED,
					"casual_hedge_in_informal_context", 0, finding), finding
			}
		}
	}

	// Gate 2 (crisp): Severity auto-pass — CRITICAL always disinhibits.
	if finding.GetSeverity() == reasoningv1.FindingSeverity_CRITICAL {
		return makeDecision(fid, cerebrov1.InhibitionAction_DISINHIBITED,
			"severity_auto_pass", 0, finding), finding
	}

	// Fuzzy gates: evaluate each FIS and collect inhibition strengths.
	var maxInhibition float64
	var maxReason string

	// Formality gate: how much should informal context suppress findings?
	if fz.FormalityEngine != nil {
		outputs, err := fz.FormalityEngine.Evaluate(map[string]float64{
			"formality": formality,
		})
		if err == nil {
			if inh, ok := outputs["inhibition_strength"]; ok && inh > maxInhibition {
				maxInhibition = inh
				maxReason = "fuzzy_formality_gate"
			}
		}
	}

	// Severity gate: stakes-based suppression (replaces crisp gates 3).
	// Map proto severity ordinal to FIS range [0, 3].
	// INFO=1→0.75, CAUTION=2→1.5, WARNING=3→2.25, CRITICAL=4→3.0 (won't reach here).
	if fz.SeverityEngine != nil {
		sevOrd := float64(finding.GetSeverity()) * 0.75
		outputs, err := fz.SeverityEngine.Evaluate(map[string]float64{
			"severity": sevOrd,
			"urgency":  urgency,
		})
		if err == nil {
			if inh, ok := outputs["inhibition_strength"]; ok && inh > maxInhibition {
				maxInhibition = inh
				maxReason = "fuzzy_severity_gate"
			}
		}
	}

	// Evidence gate: confidence + corroboration (replaces crisp gates 4 and 5).
	corr := computeCorroboration(finding, activeDetectors, turnFindings, cfg.ProximityWindowTurns)
	if fz.EvidenceEngine != nil {
		outputs, err := fz.EvidenceEngine.Evaluate(map[string]float64{
			"confidence":    finding.GetConfidence(),
			"corroboration": corr,
		})
		if err == nil {
			if inh, ok := outputs["inhibition_strength"]; ok && inh > maxInhibition {
				maxInhibition = inh
				maxReason = "fuzzy_evidence_gate"
			}
		}
	}

	// Apply multiplicative suppression: confidence × (1 - inhibition_strength).
	// Inhibition > 0.8 → INHIBITED (full block with suppressed confidence).
	// Inhibition <= 0.8 → DISINHIBITED (passes with reduced confidence).
	suppressedConf := finding.GetConfidence() * (1.0 - maxInhibition)

	// Create suppressed copy of the assessment (don't mutate original).
	suppressed := cloneAssessment(finding)
	suppressed.Confidence = suppressedConf

	if maxInhibition > 0.8 {
		return makeDecision(fid, cerebrov1.InhibitionAction_INHIBITED,
			maxReason, corr, finding), suppressed
	}

	if maxReason == "" {
		maxReason = "all_gates_passed"
	}
	return makeDecision(fid, cerebrov1.InhibitionAction_DISINHIBITED,
		maxReason, corr, finding), suppressed
}

// cloneAssessment creates a deep copy of a CognitiveAssessment via protobuf Clone.
func cloneAssessment(a *reasoningv1.CognitiveAssessment) *reasoningv1.CognitiveAssessment {
	return proto.Clone(a).(*reasoningv1.CognitiveAssessment)
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

// isClassicalText returns true when the conversation contains enough archaic /
// classical-philosophy vocabulary to be treated as formal regardless of its
// raw formality score.  Classical texts (Plato, Aristotle, KJV-style prose)
// use archaic constructions that the formality scorer underweights, so they
// can score below the 0.85 FormalityThreshold while being genuinely formal.
// Gate 1 should not suppress CONFIDENCE_MISCALIBRATION findings in such texts.
//
// The heuristic: at least 2 distinct classicalFormalMarkers must appear across
// the conversation.  A single archaic word could be coincidental; two or more
// strongly indicates classical register.
//
// Deprecated: prefer isClassicalTextFor which accepts a pathology type and
// also checks domain-specific classical marker dictionaries.
func isClassicalText(snap *reasoningv1.ConversationSnapshot) bool {
	return isClassicalTextFor(snap, reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION)
}

// isClassicalTextFor returns true when the conversation reads as classical /
// formal philosophical or academic discourse when evaluated in the context of
// a specific pathology type.
//
// In addition to the generic classicalFormalMarkers it consults a
// pathology-specific dictionary so that domain vocabulary from classical
// economic, epistemic, or commitment-theory texts is also recognised:
//
//   - SUNK_COST_FALLACY    → classicalSunkCostMarkers
//   - CONFIDENCE_MISCALIBRATION → classicalConfidenceMarkers
//   - all others           → generic classicalFormalMarkers only
//
// The heuristic requires at least 2 distinct marker hits across all applicable
// dictionaries.  Matching two markers from different dictionaries counts.
func isClassicalTextFor(snap *reasoningv1.ConversationSnapshot, pathology reasoningv1.FindingType) bool {
	if snap == nil {
		return false
	}

	// Build the combined marker set for this pathology.
	markers := classicalFormalMarkers
	switch pathology {
	case reasoningv1.FindingType_SUNK_COST_FALLACY:
		markers = append(markers, classicalSunkCostMarkers...)
	case reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION:
		markers = append(markers, classicalConfidenceMarkers...)
	}

	seen := make(map[string]bool)
	for _, turn := range snap.GetTurns() {
		text := strings.ToLower(turn.GetRawText())
		for _, marker := range markers {
			if !seen[marker] && strings.Contains(text, marker) {
				seen[marker] = true
				if len(seen) >= 2 {
					return true
				}
			}
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
//
// The scorer handles three register types:
//   - Modern casual: contractions, internet slang, exclamations → low score
//   - Modern formal/academic: technical hedges, institutional phrasing → high score
//   - Classical/literary: archaic vocabulary, complex syntax, no contractions → high score
func ComputeFormality(snap *reasoningv1.ConversationSnapshot) float64 {
	if snap == nil {
		return 0.5
	}

	var formalCount, informalCount int

	for _, turn := range snap.GetTurns() {
		text := textutil.NormalizeQuotes(strings.ToLower(turn.GetRawText()))

		// Modern formal markers
		for _, marker := range formalMarkers {
			if strings.Contains(text, marker) {
				formalCount++
			}
		}

		// Classical / archaic formal markers
		for _, marker := range classicalFormalMarkers {
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
		if len(words) > 50 {
			formalCount++ // Very long sentences strongly suggest formality
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

		// Semicolons signal complex, multi-clause sentences (formal / literary register)
		if strings.Count(text, ";") >= 2 {
			formalCount++
		}

		// Rhetorical question patterns (classical argumentative style)
		for _, pat := range rhetoricalPatterns {
			if strings.Contains(text, pat) {
				formalCount++
				break
			}
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
	"moreover", "nevertheless", "notwithstanding", "nonetheless",
	"it follows that", "one must", "let us consider", "as i have said",
	"it is evident", "we must acknowledge", "i maintain",
	"in conclusion", "in summary", "to summarise", "to summarize",
}

// classicalFormalMarkers covers archaic and literary vocabulary that signals
// formal classical register. These appear in texts such as Plato (Jowett),
// Aristotle, King James Bible, and 18th–19th century philosophical prose.
var classicalFormalMarkers = []string{
	"whence", "hitherto", "wherefore", "thereof", "wherein",
	"hereby", "heretofore", "forthwith", "inasmuch", "therein",
	"therefrom", "thereupon", "hereafter", "hereunto", "heretofore",
	"perchance", "mayhaps", "methinks", "forsooth", "verily",
	"pray tell", "pray ", "nay ", " nay,",
	"thou ", "thee ", "thy ", "thine ", "dost ", "hath ", "wilt ",
	"for it is", "for he who", "for she who", "for they who",
	"is it not", "do you not", "have you not", "are you not",
	"ought to", "ought not", "would have it",
	"in like manner", "in the same manner", "by the same token",
	"on the contrary", "to the contrary",
	"it must be", "it cannot be", "it would seem", "it appears that",
	"as i have argued", "as we have seen", "as has been shown",
	"let us suppose", "let us assume", "let us grant",
	"one who is", "he who is", "she who is",
}

// classicalSunkCostMarkers covers vocabulary that appears in classical
// philosophical, economic, and academic texts when discussing prior commitment,
// resource allocation, irrecoverable costs, and loss aversion.  These phrases
// signal that sunk-cost reasoning is being analyzed in a formal register so
// that the inhibitor does not gate genuine SUNK_COST_FALLACY findings on the
// grounds of apparent informality.
//
// Examples drawn from Aristotle's Nicomachean Ethics, Adam Smith's Wealth of
// Nations, 19th-century utilitarian literature, and academic decision theory.
var classicalSunkCostMarkers = []string{
	// Resource commitment language
	"committed resources", "prior investment", "having already expended",
	"irrecoverable costs", "past expenditure", "resources already allocated",
	"previous commitment", "having invested", "resources already committed",
	"expenditure already incurred", "costs already borne", "already laid out",
	"previously expended", "having already laid out", "having already committed",
	// Loss / irreversibility framing
	"cannot now be recovered", "now past recovery", "beyond recovery",
	"what has been spent cannot", "the loss already sustained",
	"irrecoverable expenditure", "irreversible commitment",
	"already foregone", "having foregone", "the sacrifice already made",
	"what is already lost", "the investment already made",
	// Classical economic register
	"sunk capital", "the capital already sunk", "capital already expended",
	"the outlay already made", "money already expended", "already laid the foundation",
	"having laid the foundation", "the cost already incurred",
	// Philosophical commitment-persistence framing
	"bound by prior agreement", "obligated by prior commitment",
	"honour our prior commitment", "honour our earlier commitment",
	"fidelity to the original", "adherence to the original plan",
	"having pledged ourselves", "having given our word",
	"the promise already given", "the vow already taken",
}

// classicalConfidenceMarkers covers vocabulary that appears in classical
// philosophical, theological, and academic texts when making assertions of
// absolute certainty, epistemic dogmatism, or self-evident truth.  These
// phrases signal that confidence-miscalibration patterns are being analyzed
// in a formal register so that genuine CONFIDENCE_MISCALIBRATION findings are
// not suppressed by the formality gate.
//
// Examples drawn from Plato, Descartes, Kant, Leibniz, scholastic theology,
// and 18th–19th century rationalist prose.
var classicalConfidenceMarkers = []string{
	// Absolute certainty claims
	"without question", "it is self-evident", "cannot be doubted",
	"absolute certainty", "indubitable", "beyond all reasonable doubt",
	"it is certain that", "necessarily follows", "must be the case",
	"it is axiomatic", "admits of no doubt", "admits no doubt",
	"cannot possibly be doubted", "no reasonable doubt", "beyond doubt",
	"it is evident to all", "manifest to all", "plain to all",
	// Rationalist / scholastic register
	"it is demonstrable", "demonstrably true", "demonstrable by reason",
	"can be demonstrated", "admits of demonstration", "follows necessarily",
	"it follows necessarily", "reason compels us", "reason dictates",
	"it is self-contradictory to deny", "the contrary is inconceivable",
	"reason alone suffices", "pure reason establishes",
	"by the light of reason", "the light of natural reason",
	// Dogmatic assertion markers
	"there can be no question", "it is unquestionable", "unquestionably true",
	"it is unquestionably", "unquestionably the case", "no room for doubt",
	"leaves no room for doubt", "no question can arise",
	"it is incontrovertible", "incontrovertibly established",
	"incontestably true", "incontestably the case",
	// Classical epistemic certainty phrases
	"i know with certainty", "i am certain beyond", "i know for certain",
	"i am entirely certain", "wholly certain", "perfectly certain",
	"absolutely certain", "certain beyond question",
}

var informalMarkers = []string{
	"i guess", "kinda", "sorta", "gonna", "wanna", "gotta",
	"lol", "haha", "btw", "imo", "imho", "tbh",
	"yeah", "yep", "nah", "nope", "cool", "awesome",
	"hey ", "hi ", "yo ", "sup ",
}

// rhetoricalPatterns captures argumentative question forms common in classical
// philosophical dialogue and formal oratory.
var rhetoricalPatterns = []string{
	"is it not the case", "do you not think", "would you not say",
	"can you not see", "must we not", "should we not",
	"is it not true", "is it not so", "is this not",
}

var contractions = []string{
	"don't", "doesn't", "can't", "won't", "isn't", "aren't",
	"i'm", "you're", "we're", "they're", "it's", "that's",
	"i've", "you've", "we've", "they've", "i'd", "you'd",
	"couldn't", "wouldn't", "shouldn't", "let's",
}
