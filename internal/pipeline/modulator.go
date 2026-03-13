package pipeline

// ModulatorConfig holds the Threshold Modulator's tunable parameters.
type ModulatorConfig struct {
	MaxGainOffset    float64
	UrgencyWeight    float64
	FormalityWeight  float64
	ComplexityWeight float64
}

// DefaultModulatorConfig returns the Phase 2 default configuration.
func DefaultModulatorConfig() ModulatorConfig {
	return ModulatorConfig{
		MaxGainOffset:    0.15,
		UrgencyWeight:    0.6,
		FormalityWeight:  0.3,
		ComplexityWeight: 0.1,
	}
}

// ThresholdAdjustments maps detector names to gain offsets.
type ThresholdAdjustments struct {
	Adjustments map[string]float64 // detector_name → gain_offset [-max, +max]
}

// KnownDetectors lists the detector names that receive gain offsets.
// Note: scope-guard is excluded — its DriftThreshold is Forge-optimized (0.80)
// and must not be adjusted by gain modulation (causes both FPs and FNs).
var KnownDetectors = []string{
	"anchoring-detector",
	"anchoring-detector-context",
	"sunk-cost-detector",
	"contradiction-tracker",
	"confidence-calibrator",
	"decision-ledger",
}

// Modulate translates a GainSignal into concrete threshold adjustments.
//
// High urgency → negative offset → lower thresholds → more sensitive.
// Low formality → positive offset → higher thresholds → less sensitive.
// High complexity → negative offset → more sensitive.
func Modulate(gain *GainSignal, cfg ModulatorConfig) *ThresholdAdjustments {
	rawGain := -(cfg.UrgencyWeight * gain.Urgency) +
		(cfg.FormalityWeight * (1.0 - gain.Formality)) +
		-(cfg.ComplexityWeight * gain.Complexity)

	offset := clamp(rawGain, -cfg.MaxGainOffset, cfg.MaxGainOffset)

	adjustments := make(map[string]float64, len(KnownDetectors))
	for _, det := range KnownDetectors {
		adjustments[det] = offset
	}

	return &ThresholdAdjustments{Adjustments: adjustments}
}

// ApplyGainOffset computes the effective threshold given a base value and gain offset.
// effective = base * (1.0 + offset)
// Caller must ensure offset is clamped (Modulate does this via MaxGainOffset).
// With MaxGainOffset=0.15, effective range is [base*0.85, base*1.15].
func ApplyGainOffset(base, offset float64) float64 {
	return base * (1.0 + offset)
}
