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

	if out.ScopeGuard.SustainedTurns != 4 {
		t.Errorf("classical domain: expected SustainedTurns=4 (Forge Cycle 1 winner), got %d", out.ScopeGuard.SustainedTurns)
	}
}

func TestApplyDomainContext_classical_anchorThreshold(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{PrimaryDomain: "philosophy", TextEra: "classical", Confidence: 0.85}

	out := applyDomainContext(cfg)

	if out.ConceptualAnchoring.AnchorThreshold != 0.35 {
		t.Errorf("classical domain: expected AnchorThreshold=0.35 (Forge Cycle 1 winner), got %.2f", out.ConceptualAnchoring.AnchorThreshold)
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
	if cfg.ScopeGuard.SustainedTurns != 4 {
		t.Errorf("ScopeGuard.SustainedTurns: want 4, got %d", cfg.ScopeGuard.SustainedTurns)
	}
	if cfg.Calibrator.MinCertaintyWords != 8 {
		t.Errorf("Calibrator.MinCertaintyWords: want 8, got %d", cfg.Calibrator.MinCertaintyWords)
	}
}

// ─── isCodeReview ─────────────────────────────────────────────────────────────

func TestDomainContext_isCodeReview_nil(t *testing.T) {
	var dc *DomainContext
	if dc.isCodeReview() {
		t.Fatal("nil DomainContext should not be code-review")
	}
}

func TestDomainContext_isCodeReview_wrongDomain(t *testing.T) {
	dc := &DomainContext{PrimaryDomain: "technical", Confidence: 0.9}
	if dc.isCodeReview() {
		t.Fatal("domain 'technical' should not match code-review")
	}
}

func TestDomainContext_isCodeReview_lowConfidence(t *testing.T) {
	dc := &DomainContext{PrimaryDomain: "code-review", Confidence: 0.3}
	if dc.isCodeReview() {
		t.Fatal("low confidence code-review should not qualify")
	}
}

func TestDomainContext_isCodeReview_atThreshold(t *testing.T) {
	// Exactly at threshold (0.6) — must NOT qualify (condition is strictly >)
	dc := &DomainContext{PrimaryDomain: "code-review", Confidence: 0.6}
	if dc.isCodeReview() {
		t.Fatal("confidence exactly at threshold should not qualify")
	}
}

func TestDomainContext_isCodeReview_aboveThreshold(t *testing.T) {
	dc := &DomainContext{PrimaryDomain: "code-review", Confidence: 0.85}
	if !dc.isCodeReview() {
		t.Fatal("high-confidence code-review should be recognised")
	}
}

// ─── applyDomainContext — code review ─────────────────────────────────────────

func TestApplyDomainContext_codeReview_driftThresholdRaised(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{PrimaryDomain: "code-review", Confidence: 0.85}

	out := applyDomainContext(cfg)

	if out.ScopeGuard.DriftThreshold != 0.85 {
		t.Errorf("code-review domain: expected DriftThreshold=0.85, got %.2f", out.ScopeGuard.DriftThreshold)
	}
}

func TestApplyDomainContext_codeReview_sustainedTurnsLowered(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{PrimaryDomain: "code-review", Confidence: 0.85}

	out := applyDomainContext(cfg)

	if out.ScopeGuard.SustainedTurns != 3 {
		t.Errorf("code-review domain: expected SustainedTurns=3, got %d", out.ScopeGuard.SustainedTurns)
	}
}

func TestApplyDomainContext_codeReview_skipAnchoring(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{PrimaryDomain: "code-review", Confidence: 0.85}

	out := applyDomainContext(cfg)

	if !out.SkipAnchoring {
		t.Error("code-review domain: expected SkipAnchoring=true (numeric literals are not anchoring signals)")
	}
}

func TestApplyDomainContext_codeReview_minCertaintyWordsLowered(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{PrimaryDomain: "code-review", Confidence: 0.85}

	out := applyDomainContext(cfg)

	if out.Calibrator.MinCertaintyWords != 3 {
		t.Errorf("code-review domain: expected MinCertaintyWords=3, got %d", out.Calibrator.MinCertaintyWords)
	}
}

func TestApplyDomainContext_codeReview_noChangeOnLowConfidence(t *testing.T) {
	cfg := DefaultPipelineConfig()
	defaultDrift := DefaultScopeGuardConfig().DriftThreshold
	cfg.DomainContext = &DomainContext{PrimaryDomain: "code-review", Confidence: 0.3}

	out := applyDomainContext(cfg)

	if out.ScopeGuard.DriftThreshold != defaultDrift {
		t.Errorf("low-confidence code-review: DriftThreshold should be unchanged %.2f, got %.2f",
			defaultDrift, out.ScopeGuard.DriftThreshold)
	}
	if out.SkipAnchoring {
		t.Error("low-confidence code-review: SkipAnchoring should be false")
	}
}

// TestApplyDomainContext_codeReview_classicalTakesPrecedence verifies that when
// a context has both classical TextEra and code-review PrimaryDomain, classical
// adjustments apply (classical is checked first).
func TestApplyDomainContext_codeReview_classicalTakesPrecedence(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{
		PrimaryDomain: "code-review",
		TextEra:       "classical",
		Confidence:    0.85,
	}

	out := applyDomainContext(cfg)

	// Classical branch fires first: DriftThreshold=0.70, SustainedTurns=4,
	// MinCertaintyWords=8.
	if out.ScopeGuard.DriftThreshold != 0.70 {
		t.Errorf("classical takes precedence: expected DriftThreshold=0.70, got %.2f", out.ScopeGuard.DriftThreshold)
	}
	if out.Calibrator.MinCertaintyWords != 8 {
		t.Errorf("classical takes precedence: expected MinCertaintyWords=8, got %d", out.Calibrator.MinCertaintyWords)
	}
}

// ─── Pipeline integration — code review domain ────────────────────────────────

// TestPipeline_codeReviewDomain_anchoringSkipped verifies that when a code-review
// DomainContext is set, the anchoring detector does not appear in the detector map
// even though the conversation has numeric tokens.
func TestPipeline_codeReviewDomain_anchoringSkipped(t *testing.T) {
	snap := makeSnapWithNumerics()

	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{PrimaryDomain: "code-review", Confidence: 0.85}

	result := Run(snap, cfg)

	for _, f := range result.Findings {
		if f.GetDetectorName() == "anchoring-detector" {
			t.Errorf("code-review domain: anchoring-detector should be skipped, but finding was produced")
		}
	}
}

// TestPipeline_codeReviewDomain_noPanic verifies the pipeline runs to completion
// with a code-review DomainContext (smoke test).
func TestPipeline_codeReviewDomain_noPanic(t *testing.T) {
	snap := makeSnapWithNumerics()
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{PrimaryDomain: "code-review", Confidence: 0.85}

	result := Run(snap, cfg)

	if result == nil {
		t.Fatal("Run returned nil for code-review domain")
	}
}

// TestRunAdaptive_codeReview_usesInhibitorVariant verifies that code-review domain
// routes to D-inhibitor-only (same variant as modern, with adjusted thresholds).
func TestRunAdaptive_codeReview_usesInhibitorVariant(t *testing.T) {
	snap := makeSnapWithNumerics()
	dc := &DomainContext{PrimaryDomain: "code-review", Confidence: 0.85}

	name := AdaptiveVariantName(dc)
	if name != "D-inhibitor-only" {
		t.Errorf("code-review domain: want D-inhibitor-only, got %s", name)
	}

	result, err := RunAdaptive(snap, dc, "")
	if err != nil {
		t.Fatalf("RunAdaptive error: %v", err)
	}
	if result == nil {
		t.Fatal("RunAdaptive returned nil")
	}

	// D-inhibitor-only always runs the inhibitor.
	if result.Inhibition == nil {
		t.Error("code-review → D-inhibitor-only: Inhibition should be non-nil")
	}
}
