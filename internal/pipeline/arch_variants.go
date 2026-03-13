// Architecture variants for Phase 6 competition.
// Each factory returns a PipelineConfig with specific stages enabled/disabled.
// All share the same detector code — the difference is which pipeline stages run.
package pipeline

// VariantInfo describes an architecture variant for competition reporting.
type VariantInfo struct {
	Name        string
	Description string
	StageCount  int // number of active pipeline stages
	CogCount    int // number of COGs in the equivalent composition
}

// FullCortexConfig returns Variant A: all layers enabled (cognitive-pipeline-v7).
// Stages: 0 → 1 → 1.5 → 2 → 2.5 → 3 → 4 → 4.5 → 5 → 6 → 7 → 8
func FullCortexConfig() PipelineConfig {
	cfg := DefaultPipelineConfig()
	cfg.UseLayer0 = true
	cfg.Layer0 = DefaultLayer0Config()
	cfg.UseInhibitor = true
	cfg.Inhibitor = DefaultInhibitorConfig()
	cfg.UseNeuromodulation = true
	cfg.Urgency = DefaultUrgencyConfig()
	cfg.Modulator = DefaultModulatorConfig()
	cfg.UseSalience = true
	cfg.Salience = DefaultSalienceConfig()
	cfg.UseMetacognition = true
	cfg.SelfConfidence = DefaultSelfConfidenceConfig()
	cfg.Feedback = DefaultFeedbackConfig()
	// Consolidator intentionally nil for competition (avoid file I/O side effects).
	return cfg
}

// FullCortexInfo returns metadata for Variant A.
func FullCortexInfo() VariantInfo {
	return VariantInfo{
		Name:        "A-full-cortex",
		Description: "All 5 layers, all COGs, feedback enabled",
		StageCount:  12, // 0,1,1.5,2,2.5,3,4,4.5,5,6,7,8
		CogCount:    21,
	}
}

// NoFeedbackConfig returns Variant B: metacognition disabled.
// Tests whether the feedback loop (Phase 4) improves results.
// Stages: 0 → 1 → 1.5 → 2 → 2.5 → 3 → 4 → 4.5 → 5 → 8
func NoFeedbackConfig() PipelineConfig {
	cfg := DefaultPipelineConfig()
	cfg.UseLayer0 = true
	cfg.Layer0 = DefaultLayer0Config()
	cfg.UseInhibitor = true
	cfg.Inhibitor = DefaultInhibitorConfig()
	cfg.UseNeuromodulation = true
	cfg.Urgency = DefaultUrgencyConfig()
	cfg.Modulator = DefaultModulatorConfig()
	cfg.UseSalience = true
	cfg.Salience = DefaultSalienceConfig()
	cfg.UseMetacognition = false // no self-confidence or feedback
	return cfg
}

// NoFeedbackInfo returns metadata for Variant B.
func NoFeedbackInfo() VariantInfo {
	return VariantInfo{
		Name:        "B-no-feedback",
		Description: "Layers 0-3 + Aggregator, no metacognition",
		StageCount:  10,
		CogCount:    19,
	}
}

// NoModulationConfig returns Variant C: no urgency/threshold modulation.
// All detectors run at base thresholds. Tests Phase 2 value.
// Stages: 0 → 1 → 2 → 3 → 4 → 4.5 → 5 → 6 → 7 → 8
func NoModulationConfig() PipelineConfig {
	cfg := DefaultPipelineConfig()
	cfg.UseLayer0 = true
	cfg.Layer0 = DefaultLayer0Config()
	cfg.UseInhibitor = true
	cfg.Inhibitor = DefaultInhibitorConfig()
	cfg.UseNeuromodulation = false // no urgency assessor or threshold modulator
	cfg.UseSalience = true
	cfg.Salience = DefaultSalienceConfig()
	cfg.UseMetacognition = true
	cfg.SelfConfidence = DefaultSelfConfidenceConfig()
	cfg.Feedback = DefaultFeedbackConfig()
	return cfg
}

// NoModulationInfo returns metadata for Variant C.
func NoModulationInfo() VariantInfo {
	return VariantInfo{
		Name:        "C-no-modulation",
		Description: "No urgency/threshold modulation, base thresholds",
		StageCount:  10,
		CogCount:    19,
	}
}

// InhibitorOnlyConfig returns Variant D: minimum viable pipeline.
// Detectors + Context Inhibitor + Aggregator. No Layer 0, no modulation,
// no salience, no metacognition, no consolidation.
// Stages: 1 → 2 → 3 → 4 → 5
func InhibitorOnlyConfig() PipelineConfig {
	cfg := DefaultPipelineConfig()
	cfg.UseInhibitor = true
	cfg.Inhibitor = DefaultInhibitorConfig()
	// Everything else stays false/nil (defaults from DefaultPipelineConfig).
	return cfg
}

// InhibitorOnlyInfo returns metadata for Variant D.
func InhibitorOnlyInfo() VariantInfo {
	return VariantInfo{
		Name:        "D-inhibitor-only",
		Description: "Detectors + Inhibitor + Aggregator only",
		StageCount:  5,
		CogCount:    12,
	}
}

// PreCortexConfig returns Variant E: pre-pipeline baseline.
// No inhibition, no modulation, no metacognition, no consolidation.
// All detector findings go straight to the aggregator unfiltered.
// Stages: 1 → 2 → 3 → 5
func PreCortexConfig() PipelineConfig {
	return DefaultPipelineConfig()
}

// PreCortexInfo returns metadata for Variant E.
func PreCortexInfo() VariantInfo {
	return VariantInfo{
		Name:        "E-pre-cortex",
		Description: "Pre-pipeline baseline, detectors → aggregator unfiltered",
		StageCount:  4,
		CogCount:    10,
	}
}

// ArchVariant bundles a config with its metadata.
type ArchVariant struct {
	Config PipelineConfig
	Info   VariantInfo
}

// AllVariants returns all architecture variant configs and metadata in order A-E.
func AllVariants() []ArchVariant {
	return []ArchVariant{
		{FullCortexConfig(), FullCortexInfo()},
		{NoFeedbackConfig(), NoFeedbackInfo()},
		{NoModulationConfig(), NoModulationInfo()},
		{InhibitorOnlyConfig(), InhibitorOnlyInfo()},
		{PreCortexConfig(), PreCortexInfo()},
	}
}
