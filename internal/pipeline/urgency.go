package pipeline

import (
	"strings"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/textutil"
)

// AssessUrgencyML produces a GainSignal using ML formality indicators when available.
// Falls back to PURE formality if ML is nil.
func AssessUrgencyML(snap *reasoningv1.ConversationSnapshot, cfg UrgencyConfig, ml *cerebrov1.MLEnrichment) *GainSignal {
	if snap == nil {
		return &GainSignal{Urgency: 0.5, Complexity: 0.5, Formality: 0.5, Mode: cerebrov1.GainMode_TONIC}
	}

	urgency := computeUrgency(snap, cfg)
	complexity := computeComplexity(snap, cfg)

	// Use ML formality if available, PURE as fallback
	formality := ComputeFormality(snap)
	if ml != nil && ml.GetFormality() != nil {
		mlFormality := ml.GetFormality().GetOverallScore()
		if mlFormality > 0 {
			// Blend ML and PURE formality (ML weighted higher)
			formality = 0.7*mlFormality + 0.3*formality
		}
	}

	mode := cerebrov1.GainMode_TONIC
	if urgency > cfg.PhasicUrgencyThreshold {
		mode = cerebrov1.GainMode_PHASIC
	}

	return &GainSignal{
		Urgency:    urgency,
		Complexity: complexity,
		Formality:  formality,
		Mode:       mode,
	}
}

// UrgencyConfig holds the Urgency Assessor's tunable parameters.
type UrgencyConfig struct {
	UrgencyKeywords          []string
	StakesKeywords           []string
	ComplexityTurnThreshold  uint32
	PhasicUrgencyThreshold   float64
	RecentTurnWindow         uint32 // how many recent turns to scan for urgency keywords
}

// DefaultUrgencyConfig returns the Phase 2 default configuration.
func DefaultUrgencyConfig() UrgencyConfig {
	return UrgencyConfig{
		UrgencyKeywords: []string{
			"urgent", "critical", "deadline", "asap", "emergency",
			"risk", "liability", "legal",
		},
		StakesKeywords: []string{
			"million", "billion", "contract", "lawsuit",
			"patient", "safety", "security",
		},
		ComplexityTurnThreshold: 10,
		PhasicUrgencyThreshold:  0.6,
		RecentTurnWindow:        5,
	}
}

// GainSignal is the Go representation of cerebro.v1.GainSignal.
type GainSignal struct {
	Urgency    float64
	Complexity float64
	Formality  float64
	Mode       cerebrov1.GainMode
}

// AssessUrgency reads a ConversationSnapshot and produces a GainSignal.
func AssessUrgency(snap *reasoningv1.ConversationSnapshot, cfg UrgencyConfig) *GainSignal {
	if snap == nil {
		return &GainSignal{Urgency: 0.5, Complexity: 0.5, Formality: 0.5, Mode: cerebrov1.GainMode_TONIC}
	}

	urgency := computeUrgency(snap, cfg)
	complexity := computeComplexity(snap, cfg)
	formality := ComputeFormality(snap) // reuse Phase 1 formality computation

	mode := cerebrov1.GainMode_TONIC
	if urgency > cfg.PhasicUrgencyThreshold {
		mode = cerebrov1.GainMode_PHASIC
	}

	return &GainSignal{
		Urgency:    urgency,
		Complexity: complexity,
		Formality:  formality,
		Mode:       mode,
	}
}

// computeUrgency scans recent turns for urgency and stakes keywords.
func computeUrgency(snap *reasoningv1.ConversationSnapshot, cfg UrgencyConfig) float64 {
	turns := snap.GetTurns()
	if len(turns) == 0 {
		return 0.0
	}

	// Take the last N turns.
	window := int(cfg.RecentTurnWindow)
	if window > len(turns) {
		window = len(turns)
	}
	recentTurns := turns[len(turns)-window:]

	// Build combined text from recent turns.
	var texts []string
	for _, t := range recentTurns {
		texts = append(texts, t.GetRawText())
	}
	combined := textutil.NormalizeQuotes(strings.ToLower(strings.Join(texts, " ")))
	words := tokenizeWords(combined)

	urgencySet := make(map[string]bool, len(cfg.UrgencyKeywords))
	for _, k := range cfg.UrgencyKeywords {
		urgencySet[k] = true
	}
	stakesSet := make(map[string]bool, len(cfg.StakesKeywords))
	for _, k := range cfg.StakesKeywords {
		stakesSet[k] = true
	}

	var urgencyHits, stakesHits int
	for _, w := range words {
		if urgencySet[w] {
			urgencyHits++
		}
		if stakesSet[w] {
			stakesHits++
		}
	}

	// Baseline urgency: every conversation has some importance.
	// Keywords add to the baseline. This prevents gate 3 from suppressing
	// findings in conversations that simply lack explicit urgency keywords.
	baseline := 0.15
	return clamp(baseline+(float64(urgencyHits)+float64(stakesHits)*2)/10.0, 0.0, 1.0)
}

// computeComplexity assesses structural complexity from turn count, speaker diversity, and turn length.
func computeComplexity(snap *reasoningv1.ConversationSnapshot, cfg UrgencyConfig) float64 {
	turns := snap.GetTurns()
	if len(turns) == 0 {
		return 0.0
	}

	totalTurns := float64(len(turns))
	threshold := float64(cfg.ComplexityTurnThreshold)
	if threshold == 0 {
		threshold = 10
	}

	// Unique speakers.
	speakers := make(map[string]bool)
	var totalWords int
	for _, t := range turns {
		speakers[t.GetSpeaker()] = true
		totalWords += len(strings.Fields(t.GetRawText()))
	}
	uniqueSpeakers := float64(len(speakers))
	avgTurnLength := float64(totalWords) / totalTurns

	return clamp(
		(totalTurns/threshold*0.4)+
			(uniqueSpeakers/5.0*0.2)+
			(avgTurnLength/200.0*0.4),
		0.0, 1.0)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
