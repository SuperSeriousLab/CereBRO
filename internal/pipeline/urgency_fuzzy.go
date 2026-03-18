package pipeline

import (
	"fmt"

	"github.com/SuperSeriousLab/fugo"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// FIS config name for L1 urgency assessment.
const FISUrgency = "l1_urgency"

// FuzzyUrgency holds a pre-built fugo engine for L1 fuzzy urgency assessment.
// When nil, the urgency assessor falls back to existing crisp logic.
type FuzzyUrgency struct {
	Engine *fugo.FuzzyEngine
}

// BuildFuzzyUrgency constructs a FuzzyUrgency from a FIS config.
func BuildFuzzyUrgency(cfg *fugo.FisConfig) (*FuzzyUrgency, error) {
	e, err := cfg.BuildEngine()
	if err != nil {
		return nil, fmt.Errorf("urgency FIS: %w", err)
	}
	return &FuzzyUrgency{Engine: e}, nil
}

// BuildFuzzyUrgencyFromRegistry constructs a FuzzyUrgency from a FisRegistry.
func BuildFuzzyUrgencyFromRegistry(reg *fugo.FisRegistry) (*FuzzyUrgency, error) {
	e, err := reg.BuildEngine(FISUrgency)
	if err != nil {
		return nil, fmt.Errorf("urgency FIS: %w", err)
	}
	return &FuzzyUrgency{Engine: e}, nil
}

// AssessUrgencyFuzzy produces a GainSignal using fuzzy inference when available.
// Falls back to crisp AssessUrgency (or AssessUrgencyML) when fu is nil.
func AssessUrgencyFuzzy(
	snap *reasoningv1.ConversationSnapshot,
	cfg UrgencyConfig,
	ml *cerebrov1.MLEnrichment,
	fu *FuzzyUrgency,
) *GainSignal {
	// Compute crisp values first — always needed as inputs or fallback.
	var crisp *GainSignal
	if ml != nil {
		crisp = AssessUrgencyML(snap, cfg, ml)
	} else {
		crisp = AssessUrgency(snap, cfg)
	}

	if fu == nil || fu.Engine == nil {
		return crisp // graceful fallback
	}

	// Evaluate fuzzy FIS with crisp urgency, complexity, formality as inputs.
	outputs, err := fu.Engine.Evaluate(map[string]float64{
		"urgency":    crisp.Urgency,
		"complexity": crisp.Complexity,
		"formality":  crisp.Formality,
	})
	if err != nil {
		return crisp // FIS evaluation error — fall back to crisp
	}

	gainSignal, ok := outputs["gain_signal"]
	if !ok {
		return crisp
	}

	// Determine mode from the fuzzy gain signal (threshold at 0.6, same as PhasicUrgencyThreshold default).
	mode := cerebrov1.GainMode_TONIC
	if gainSignal > cfg.PhasicUrgencyThreshold {
		mode = cerebrov1.GainMode_PHASIC
	}

	return &GainSignal{
		Urgency:    gainSignal, // fuzzy gain replaces raw urgency as the activation signal
		Complexity: crisp.Complexity,
		Formality:  crisp.Formality,
		Mode:       mode,
	}
}

// GainActivationLevel returns the activation tier for a fuzzy gain signal.
// Low (< 0.35): only critical detectors.
// Medium (0.35–0.65): most detectors.
// High (>= 0.65): all detectors.
type GainActivationLevel int

const (
	GainActivationLow    GainActivationLevel = iota // only critical detectors
	GainActivationMedium                            // most detectors
	GainActivationHigh                              // all detectors
)

// ClassifyGainActivation maps a gain signal value to an activation level.
func ClassifyGainActivation(gain float64) GainActivationLevel {
	if gain >= 0.65 {
		return GainActivationHigh
	}
	if gain >= 0.35 {
		return GainActivationMedium
	}
	return GainActivationLow
}

// CriticalDetectors are always activated regardless of gain signal level.
var CriticalDetectors = map[Detector]bool{
	DetectorScopeGuard:    true,
	DetectorContradiction: true,
}

// ShouldActivateDetector returns whether a detector should run at the given gain level.
// Critical detectors always run. Non-critical detectors are gated by gain level.
func ShouldActivateDetector(det Detector, level GainActivationLevel) bool {
	if CriticalDetectors[det] {
		return true // always active
	}
	switch level {
	case GainActivationHigh:
		return true
	case GainActivationMedium:
		// Medium: run all except inherited-position and conceptual-anchoring (expensive).
		return det != DetectorInheritedPosition && det != DetectorConceptualAnchoring
	case GainActivationLow:
		return false // only critical detectors
	}
	return true
}
