package pipeline

import (
	"context"
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// =============================================================================
// TestArbitratDorang_NilArbitrator — NilArbitrator returns findings unchanged
// =============================================================================

func TestArbitratDorang_NilArbitrator(t *testing.T) {
	var arb NilArbitrator

	findings := []*reasoningv1.CognitiveAssessment{
		makeAssessment("anchoring-detector",
			reasoningv1.FindingType_ANCHORING_BIAS,
			reasoningv1.FindingSeverity_WARNING,
			0.8, []uint32{1}),
		makeAssessment("anchoring-detector",
			reasoningv1.FindingType_ANCHORING_BIAS,
			reasoningv1.FindingSeverity_CAUTION,
			0.3, []uint32{2}),
	}

	result, err := arb.Arbitrate(context.Background(), findings)
	if err != nil {
		t.Fatalf("NilArbitrator.Arbitrate() returned error: %v", err)
	}
	if len(result) != len(findings) {
		t.Fatalf("NilArbitrator changed finding count: got %d, want %d", len(result), len(findings))
	}
	for i, f := range result {
		if f.GetConfidence() != findings[i].GetConfidence() {
			t.Errorf("NilArbitrator mutated finding[%d] confidence: got %.3f, want %.3f",
				i, f.GetConfidence(), findings[i].GetConfidence())
		}
	}
}

// =============================================================================
// TestArbitratDorang_DisabledConfig — NewDorangArbitrator returns nil when disabled
// =============================================================================

func TestArbitratDorang_DisabledConfig(t *testing.T) {
	cfg := DorangArbitratorConfig{
		Enabled:        false,
		Host:           "http://192.168.14.71:8080",
		CouncilID:      "tech-review",
		TimeoutSeconds: 30,
	}
	arb := NewDorangArbitrator(cfg)
	if arb != nil {
		t.Error("NewDorangArbitrator with Enabled=false should return nil")
	}
}

// =============================================================================
// TestArbitratDorang_DefaultConfig — DefaultDorangArbitratorConfig is disabled
// =============================================================================

func TestArbitratDorang_DefaultConfig(t *testing.T) {
	cfg := DefaultDorangArbitratorConfig()
	if cfg.Enabled {
		t.Error("DefaultDorangArbitratorConfig should have Enabled=false")
	}
	if cfg.Host == "" {
		t.Error("DefaultDorangArbitratorConfig should have a non-empty Host")
	}
	if cfg.TimeoutSeconds <= 0 {
		t.Errorf("DefaultDorangArbitratorConfig TimeoutSeconds should be > 0, got %d", cfg.TimeoutSeconds)
	}
}

// =============================================================================
// TestArbitratDorang_EnabledConfig — NewDorangArbitrator returns non-nil when enabled
// =============================================================================

func TestArbitratDorang_EnabledConfig(t *testing.T) {
	cfg := DorangArbitratorConfig{
		Enabled:        true,
		Host:           "http://192.168.14.71:8080",
		CouncilID:      "tech-review",
		TimeoutSeconds: 30,
	}
	arb := NewDorangArbitrator(cfg)
	if arb == nil {
		t.Error("NewDorangArbitrator with Enabled=true should return non-nil")
	}
}

// =============================================================================
// TestArbitratDorang_ConflictDetection — detectConflictClusters groups correctly
// =============================================================================

func TestArbitratDorang_ConflictDetection(t *testing.T) {
	tests := []struct {
		name         string
		findings     []*reasoningv1.CognitiveAssessment
		wantClusters int
	}{
		{
			name: "no conflict — same type same confidence range",
			findings: []*reasoningv1.CognitiveAssessment{
				makeAssessment("d1", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_WARNING, 0.7, nil),
				makeAssessment("d2", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_WARNING, 0.75, nil),
			},
			wantClusters: 0,
		},
		{
			name: "conflict — same type opposing confidence",
			findings: []*reasoningv1.CognitiveAssessment{
				makeAssessment("d1", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_WARNING, 0.8, nil),
				makeAssessment("d2", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_CAUTION, 0.3, nil),
			},
			wantClusters: 1,
		},
		{
			name: "conflict — semantically opposite types",
			findings: []*reasoningv1.CognitiveAssessment{
				makeAssessment("d1", reasoningv1.FindingType_SYCOPHANCY, reasoningv1.FindingSeverity_WARNING, 0.7, nil),
				makeAssessment("d2", reasoningv1.FindingType_CONTRADICTION, reasoningv1.FindingSeverity_WARNING, 0.7, nil),
			},
			wantClusters: 1,
		},
		{
			name: "no conflict — different unrelated types",
			findings: []*reasoningv1.CognitiveAssessment{
				makeAssessment("d1", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_WARNING, 0.7, nil),
				makeAssessment("d2", reasoningv1.FindingType_SCOPE_DRIFT, reasoningv1.FindingSeverity_WARNING, 0.7, nil),
			},
			wantClusters: 0,
		},
		{
			name: "single finding — no cluster possible",
			findings: []*reasoningv1.CognitiveAssessment{
				makeAssessment("d1", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_WARNING, 0.8, nil),
			},
			wantClusters: 0,
		},
		{
			name: "nil input",
			findings:     nil,
			wantClusters: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusters := detectConflictClusters(tt.findings)
			if len(clusters) != tt.wantClusters {
				t.Errorf("detectConflictClusters returned %d clusters, want %d",
					len(clusters), tt.wantClusters)
			}
		})
	}
}

// =============================================================================
// TestArbitratDorang_ApplyDebateSynthesis — confidence nudging is bounded
// =============================================================================

func TestArbitratDorang_ApplyDebateSynthesis(t *testing.T) {
	findings := []*reasoningv1.CognitiveAssessment{
		makeAssessment("d1", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_WARNING, 0.8, nil),
		makeAssessment("d2", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_CAUTION, 0.3, nil),
	}

	// High synthesis confidence: should nudge high-conf finding up, low-conf down.
	synHigh := &debateSynthesis{ConfidenceScore: 0.9}
	adjusted := applyDebateSynthesis(findings, synHigh)
	if len(adjusted) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(adjusted))
	}
	if adjusted[0].GetConfidence() <= findings[0].GetConfidence() {
		t.Errorf("high-conf finding should increase with high synthesis: %.3f → %.3f",
			findings[0].GetConfidence(), adjusted[0].GetConfidence())
	}
	if adjusted[1].GetConfidence() >= findings[1].GetConfidence() {
		t.Errorf("low-conf finding should decrease with high synthesis: %.3f → %.3f",
			findings[1].GetConfidence(), adjusted[1].GetConfidence())
	}

	// All confidence values must remain in [0.0, 1.0].
	for i, f := range adjusted {
		c := f.GetConfidence()
		if c < 0.0 || c > 1.0 {
			t.Errorf("finding[%d] confidence out of bounds: %.3f", i, c)
		}
	}
}

func TestArbitratDorang_ApplyDebateSynthesis_NilSynthesis(t *testing.T) {
	findings := []*reasoningv1.CognitiveAssessment{
		makeAssessment("d1", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_WARNING, 0.8, nil),
	}
	result := applyDebateSynthesis(findings, nil)
	if len(result) != len(findings) {
		t.Fatalf("nil synthesis should return original findings unchanged")
	}
}

// =============================================================================
// TestArbitratDorang_AggregateWithArbitration_NilArb — nil arb = plain aggregate
// =============================================================================

func TestArbitratDorang_AggregateWithArbitration_NilArb(t *testing.T) {
	findings := []*reasoningv1.CognitiveAssessment{
		makeAssessment("contradiction-tracker",
			reasoningv1.FindingType_CONTRADICTION,
			reasoningv1.FindingSeverity_WARNING,
			0.7, []uint32{1}),
	}

	// nil DebateArbitrator — must behave identically to plain Aggregate.
	reportWithNil := AggregateWithArbitration(context.Background(), findings, "conv-test", nil)
	reportDirect := Aggregate(findings, "conv-test")

	if len(reportWithNil.GetFindings()) != len(reportDirect.GetFindings()) {
		t.Errorf("nil arbitrator changed finding count: %d vs %d",
			len(reportWithNil.GetFindings()), len(reportDirect.GetFindings()))
	}
	if reportWithNil.GetOverallIntegrityScore() != reportDirect.GetOverallIntegrityScore() {
		t.Errorf("nil arbitrator changed integrity score: %.3f vs %.3f",
			reportWithNil.GetOverallIntegrityScore(), reportDirect.GetOverallIntegrityScore())
	}
}

// =============================================================================
// TestArbitratDorang_AggregateWithArbitration_NilArbitrator_Interface
// =============================================================================

func TestArbitratDorang_AggregateWithArbitration_NilArbitrator_Interface(t *testing.T) {
	// Test passing a typed nil as a DebateArbitrator interface value — must not panic.
	var arb DebateArbitrator // nil interface
	findings := []*reasoningv1.CognitiveAssessment{
		makeAssessment("scope-guard",
			reasoningv1.FindingType_SCOPE_DRIFT,
			reasoningv1.FindingSeverity_CAUTION,
			0.5, []uint32{2}),
	}

	// Should not panic.
	report := AggregateWithArbitration(context.Background(), findings, "conv-nil-iface", arb)
	if report == nil {
		t.Fatal("AggregateWithArbitration returned nil report")
	}
	if len(report.GetFindings()) != 1 {
		t.Errorf("expected 1 finding, got %d", len(report.GetFindings()))
	}
}

// =============================================================================
// TestArbitratDorang_AggregateWithArbitration_WithNilArbitrator_Struct
// Verifies DorangArbitrator nil receiver is safe (Arbitrate on nil *DorangArbitrator)
// =============================================================================

func TestArbitratDorang_AggregateWithArbitration_WithNilStruct(t *testing.T) {
	var arb *DorangArbitrator // nil pointer, but typed
	findings := []*reasoningv1.CognitiveAssessment{
		makeAssessment("d1", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_WARNING, 0.8, nil),
	}

	result, err := arb.Arbitrate(context.Background(), findings)
	if err != nil {
		t.Fatalf("nil DorangArbitrator.Arbitrate() should not error: %v", err)
	}
	if len(result) != len(findings) {
		t.Fatalf("nil DorangArbitrator should return original findings, got %d", len(result))
	}
}

// =============================================================================
// TestArbitratDorang_BuildDebateTopic — topic describes the conflict
// =============================================================================

func TestArbitratDorang_BuildDebateTopic(t *testing.T) {
	findings := []*reasoningv1.CognitiveAssessment{
		makeAssessment("d1", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_WARNING, 0.8, nil),
		makeAssessment("d2", reasoningv1.FindingType_ANCHORING_BIAS, reasoningv1.FindingSeverity_CAUTION, 0.3, nil),
	}

	topic := buildDebateTopic(findings)
	if topic == "" {
		t.Error("buildDebateTopic returned empty string")
	}
	if len(topic) < 20 {
		t.Errorf("buildDebateTopic returned suspiciously short topic: %q", topic)
	}
}

func TestArbitratDorang_BuildDebateTopic_Empty(t *testing.T) {
	topic := buildDebateTopic(nil)
	if topic == "" {
		t.Error("buildDebateTopic should return fallback for nil input")
	}
}
