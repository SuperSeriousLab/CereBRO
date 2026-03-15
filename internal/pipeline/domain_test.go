package pipeline

import (
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// makeSnapWithNumerics builds a minimal ConversationSnapshot that has numeric
// tokens on two different turns so the anchoring detector would fire if active.
func makeSnapWithNumerics() *reasoningv1.ConversationSnapshot {
	return &reasoningv1.ConversationSnapshot{
		Objective:  "evaluate estimate",
		TotalTurns: 3,
		Turns: []*reasoningv1.Turn{
			{
				TurnNumber: 1,
				Speaker:    "user",
				RawText:    "I think the cost is around 100 dollars.",
				Metadata: &reasoningv1.TurnMetadata{
					NumericTokens: []*reasoningv1.NumericToken{
						{Value: 100, ContextWindow: "cost is around 100 dollars"},
					},
				},
			},
			{
				TurnNumber: 2,
				Speaker:    "assistant",
				RawText:    "Yes, approximately 105 dollars.",
				Metadata: &reasoningv1.TurnMetadata{
					NumericTokens: []*reasoningv1.NumericToken{
						{Value: 105, ContextWindow: "approximately 105 dollars"},
					},
				},
			},
			{
				TurnNumber: 3,
				Speaker:    "user",
				RawText:    "We should go with 102.",
				Metadata: &reasoningv1.TurnMetadata{
					NumericTokens: []*reasoningv1.NumericToken{
						{Value: 102, ContextWindow: "go with 102"},
					},
				},
			},
		},
		ObjectiveKeywords: []string{"cost", "estimate"},
	}
}

// ─── DomainContext.isClassical ────────────────────────────────────────────────

func TestDomainContext_isClassical_nil(t *testing.T) {
	var dc *DomainContext
	if dc.isClassical() {
		t.Fatal("nil DomainContext should not be classical")
	}
}

func TestDomainContext_isClassical_modern(t *testing.T) {
	dc := &DomainContext{PrimaryDomain: "technical", TextEra: "modern", Confidence: 0.9}
	if dc.isClassical() {
		t.Fatal("modern era should not be classical")
	}
}

func TestDomainContext_isClassical_lowConfidence(t *testing.T) {
	dc := &DomainContext{PrimaryDomain: "philosophy", TextEra: "classical", Confidence: 0.3}
	if dc.isClassical() {
		t.Fatal("low confidence classical should not be treated as classical")
	}
}

func TestDomainContext_isClassical_atThreshold(t *testing.T) {
	// Exactly at threshold (0.6) — must NOT qualify (condition is strictly >)
	dc := &DomainContext{PrimaryDomain: "philosophy", TextEra: "classical", Confidence: 0.6}
	if dc.isClassical() {
		t.Fatal("confidence exactly at threshold should not qualify")
	}
}

func TestDomainContext_isClassical_aboveThreshold(t *testing.T) {
	dc := &DomainContext{PrimaryDomain: "philosophy", TextEra: "classical", Confidence: 0.85}
	if !dc.isClassical() {
		t.Fatal("high-confidence classical should be recognised")
	}
}

// ─── applyDomainContext ───────────────────────────────────────────────────────

func TestApplyDomainContext_nil_noChange(t *testing.T) {
	cfg := DefaultPipelineConfig()
	defaultRef := cfg.ScopeGuard.ReferenceTurns
	defaultCW := cfg.Calibrator.MinCertaintyWords

	out := applyDomainContext(cfg)

	if out.ScopeGuard.ReferenceTurns != defaultRef {
		t.Errorf("nil domain: ReferenceTurns changed from %d to %d", defaultRef, out.ScopeGuard.ReferenceTurns)
	}
	if out.Calibrator.MinCertaintyWords != defaultCW {
		t.Errorf("nil domain: MinCertaintyWords changed from %d to %d", defaultCW, out.Calibrator.MinCertaintyWords)
	}
	if out.SkipAnchoring {
		t.Error("nil domain: SkipAnchoring should be false")
	}
}

func TestApplyDomainContext_modern_noChange(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{TextEra: "modern", Confidence: 0.95}
	defaultRef := DefaultScopeGuardConfig().ReferenceTurns

	out := applyDomainContext(cfg)

	if out.ScopeGuard.ReferenceTurns != defaultRef {
		t.Errorf("modern domain: ReferenceTurns should be default %d, got %d", defaultRef, out.ScopeGuard.ReferenceTurns)
	}
	if out.SkipAnchoring {
		t.Error("modern domain: SkipAnchoring should be false")
	}
}

func TestApplyDomainContext_lowConfidence_noChange(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{TextEra: "classical", Confidence: 0.3}
	defaultRef := DefaultScopeGuardConfig().ReferenceTurns

	out := applyDomainContext(cfg)

	if out.ScopeGuard.ReferenceTurns != defaultRef {
		t.Errorf("low-confidence classical: ReferenceTurns should be default %d, got %d", defaultRef, out.ScopeGuard.ReferenceTurns)
	}
	if out.SkipAnchoring {
		t.Error("low-confidence classical: SkipAnchoring should be false")
	}
}

func TestApplyDomainContext_classical_scopeGuardDriftThreshold(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{PrimaryDomain: "philosophy", TextEra: "classical", Confidence: 0.85}

	out := applyDomainContext(cfg)

	if out.ScopeGuard.DriftThreshold != 0.70 {
		t.Errorf("classical domain: expected DriftThreshold=0.70, got %.2f", out.ScopeGuard.DriftThreshold)
	}
}

func TestApplyDomainContext_classical_scopeGuardSustainedTurns(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{PrimaryDomain: "philosophy", TextEra: "classical", Confidence: 0.85}

	out := applyDomainContext(cfg)

	if out.ScopeGuard.SustainedTurns != 3 {
		t.Errorf("classical domain: expected SustainedTurns=3, got %d", out.ScopeGuard.SustainedTurns)
	}
}

func TestApplyDomainContext_classical_calibratorMinCertaintyWords8(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{PrimaryDomain: "philosophy", TextEra: "classical", Confidence: 0.85}

	out := applyDomainContext(cfg)

	if out.Calibrator.MinCertaintyWords != 8 {
		t.Errorf("classical domain: expected MinCertaintyWords=8, got %d", out.Calibrator.MinCertaintyWords)
	}
}

func TestApplyDomainContext_classical_skipAnchoring(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{PrimaryDomain: "philosophy", TextEra: "classical", Confidence: 0.85}

	out := applyDomainContext(cfg)

	if !out.SkipAnchoring {
		t.Error("classical domain: expected SkipAnchoring=true")
	}
}

// ─── Pipeline integration ─────────────────────────────────────────────────────

// TestPipeline_classicalDomain_anchoringSkipped verifies that when a classical
// DomainContext is set, the anchoring detector does not appear in findings even
// when the conversation has numeric tokens that would normally trigger it.
func TestPipeline_classicalDomain_anchoringSkipped(t *testing.T) {
	snap := makeSnapWithNumerics()

	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{PrimaryDomain: "philosophy", TextEra: "classical", Confidence: 0.85}

	result := Run(snap, cfg)

	for _, f := range result.Findings {
		if f.GetDetectorName() == "anchoring-detector" {
			t.Errorf("classical domain: anchoring-detector should be skipped, but finding was produced")
		}
	}
}

// TestPipeline_nilDomain_anchoringRuns verifies zero regression: when DomainContext
// is nil, the anchoring detector is included in the detector map and can fire.
func TestPipeline_nilDomain_anchoringRuns(t *testing.T) {
	cfg := DefaultPipelineConfig()
	// DomainContext is nil — defaults must be preserved.

	detectorMap := buildDetectorMap(cfg)
	if _, ok := detectorMap[DetectorAnchoring]; !ok {
		t.Error("nil domain context: anchoring detector should be in the detector map")
	}
}

// TestPipeline_modernDomain_anchoringRuns verifies zero regression for modern era.
func TestPipeline_modernDomain_anchoringRuns(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{TextEra: "modern", Confidence: 0.95}
	cfg = applyDomainContext(cfg)

	detectorMap := buildDetectorMap(cfg)
	if _, ok := detectorMap[DetectorAnchoring]; !ok {
		t.Error("modern domain context: anchoring detector should be in the detector map")
	}
}

// TestPipeline_classicalDomain_scopeGuardConfigApplied verifies the config field
// is set before buildDetectorMap is called. We inspect via applyDomainContext directly.
func TestPipeline_classicalDomain_configFields(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{TextEra: "classical", Confidence: 0.75}
	cfg = applyDomainContext(cfg)

	if cfg.ScopeGuard.DriftThreshold != 0.70 {
		t.Errorf("ScopeGuard.DriftThreshold: want 0.70, got %.2f", cfg.ScopeGuard.DriftThreshold)
	}
	if cfg.ScopeGuard.SustainedTurns != 3 {
		t.Errorf("ScopeGuard.SustainedTurns: want 3, got %d", cfg.ScopeGuard.SustainedTurns)
	}
	if cfg.Calibrator.MinCertaintyWords != 8 {
		t.Errorf("Calibrator.MinCertaintyWords: want 8, got %d", cfg.Calibrator.MinCertaintyWords)
	}
}
