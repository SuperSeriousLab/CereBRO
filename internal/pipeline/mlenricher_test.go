package pipeline

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// mockOllamaResponse returns a valid Ollama /api/chat response wrapping the given JSON content.
func mockOllamaResponse(content string) string {
	resp := ollamaChatResponse{
		Message: ollamaChatMsg{
			Role:    "assistant",
			Content: content,
		},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

func TestEnrichML_Disabled(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Hello"},
		},
	}
	cfg := DefaultMLEnricherConfig()
	cfg.Enabled = false

	result := EnrichML(snap, cfg, http.DefaultClient)
	if result != nil {
		t.Errorf("expected nil when disabled, got %v", result)
	}
}

func TestEnrichML_NilSnap(t *testing.T) {
	cfg := DefaultMLEnricherConfig()
	cfg.Enabled = true
	result := EnrichML(nil, cfg, http.DefaultClient)
	if result != nil {
		t.Errorf("expected nil for nil snap, got %v", result)
	}
}

func TestEnrichML_MockOllama(t *testing.T) {
	mlResp := `{
		"claims": [{"text": "We should use React", "speaker": "user", "source_turn": 1, "epistemic_status": "certain", "evidence_refs": []}],
		"anchoring_references": [{"value": 50000, "turn": 1, "context": "budget of $50,000", "relevance": 0.9}],
		"sunk_cost_phrases": ["already invested"],
		"decision_points": [{"turn": 1, "description": "framework choice", "chosen_option": "React", "alternatives": ["Vue", "Angular"], "rationale": ""}],
		"formality": {"overall_score": 0.6, "has_technical_jargon": true, "has_academic_language": false, "is_casual": false, "register": "neutral"},
		"confidence_markers": ["definitely", "I'm sure"]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockOllamaResponse(mlResp)))
	}))
	defer server.Close()

	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "We've already invested $50,000 in React, I'm sure we should definitely keep going with it."},
		},
		Objective: "Choose a frontend framework",
	}

	cfg := DefaultMLEnricherConfig()
	cfg.Enabled = true
	cfg.OllamaURL = server.URL

	result := EnrichML(snap, cfg, server.Client())
	if len(result) != 1 {
		t.Fatalf("expected 1 enrichment, got %d", len(result))
	}

	e := result[0]
	if e.GetSourceTurn() != 1 {
		t.Errorf("expected source_turn=1, got %d", e.GetSourceTurn())
	}
	if len(e.GetClaims()) != 1 {
		t.Errorf("expected 1 claim, got %d", len(e.GetClaims()))
	}
	if len(e.GetAnchoringReferences()) != 1 {
		t.Errorf("expected 1 anchor ref, got %d", len(e.GetAnchoringReferences()))
	}
	if len(e.GetSunkCostPhrases()) != 1 {
		t.Errorf("expected 1 sunk cost phrase, got %d", len(e.GetSunkCostPhrases()))
	}
	if e.GetFormality() == nil {
		t.Fatal("expected formality, got nil")
	}
	if e.GetFormality().GetRegister() != "neutral" {
		t.Errorf("expected register=neutral, got %s", e.GetFormality().GetRegister())
	}
	if len(e.GetConfidenceMarkers()) != 2 {
		t.Errorf("expected 2 confidence markers, got %d", len(e.GetConfidenceMarkers()))
	}
}

func TestEnrichML_FallbackOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Test"},
		},
	}

	cfg := DefaultMLEnricherConfig()
	cfg.Enabled = true
	cfg.OllamaURL = server.URL
	cfg.FallbackToPure = true

	result := EnrichML(snap, cfg, server.Client())
	// Should return empty slice (nil per-turn enrichments filtered out)
	if len(result) != 0 {
		t.Errorf("expected 0 enrichments on error with fallback, got %d", len(result))
	}
}

func TestEnrichML_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(mockOllamaResponse("not json at all")))
	}))
	defer server.Close()

	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Test"},
		},
	}

	cfg := DefaultMLEnricherConfig()
	cfg.Enabled = true
	cfg.OllamaURL = server.URL
	cfg.FallbackToPure = true

	result := EnrichML(snap, cfg, server.Client())
	if len(result) != 0 {
		t.Errorf("expected 0 enrichments on invalid JSON, got %d", len(result))
	}
}

func TestMergeMLEnrichments(t *testing.T) {
	enrichments := []*cerebrov1.MLEnrichment{
		{
			SourceTurn:      1,
			SunkCostPhrases: []string{"already spent"},
			Claims:          []*cerebrov1.MLClaim{{Text: "claim1"}},
		},
		{
			SourceTurn:        2,
			ConfidenceMarkers: []string{"definitely"},
			Claims:            []*cerebrov1.MLClaim{{Text: "claim2"}},
			Formality:         &cerebrov1.MLFormalityIndicators{OverallScore: 0.7, Register: "formal"},
		},
	}

	merged := MergeMLEnrichments(enrichments)
	if merged == nil {
		t.Fatal("expected non-nil merged enrichment")
	}
	if len(merged.GetClaims()) != 2 {
		t.Errorf("expected 2 claims, got %d", len(merged.GetClaims()))
	}
	if len(merged.GetSunkCostPhrases()) != 1 {
		t.Errorf("expected 1 sunk cost phrase, got %d", len(merged.GetSunkCostPhrases()))
	}
	if merged.GetFormality().GetRegister() != "formal" {
		t.Errorf("expected formality from last turn, got %s", merged.GetFormality().GetRegister())
	}
}

func TestMergeMLEnrichments_Nil(t *testing.T) {
	if MergeMLEnrichments(nil) != nil {
		t.Error("expected nil for nil input")
	}
	if MergeMLEnrichments([]*cerebrov1.MLEnrichment{}) != nil {
		t.Error("expected nil for empty input")
	}
}

// TestDetectorsIdentical_NilML verifies all ML-enhanced detectors produce identical output
// to their PURE counterparts when ml=nil.
func TestDetectorsIdentical_NilML(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "We already invested $50,000 in this project and I'm sure we should keep going."},
			{TurnNumber: 2, Speaker: "assistant", RawText: "That sounds like it might be a sunk cost consideration. The budget was $50,000."},
			{TurnNumber: 3, Speaker: "user", RawText: "Definitely, we can't stop now. Let's continue with the plan."},
		},
		Objective: "Evaluate project continuation",
	}
	snap = Enrich(snap)

	t.Run("SunkCost", func(t *testing.T) {
		cfg := DefaultSunkCostConfig()
		pure := DetectSunkCost(snap, cfg)
		ml := DetectSunkCostML(snap, cfg, nil)
		assertFindingsEqual(t, pure, ml)
	})

	t.Run("ConfidenceCalibrator", func(t *testing.T) {
		cfg := DefaultCalibratorConfig()
		pure := DetectConfidenceMiscalibration(snap, cfg)
		ml := DetectConfidenceMiscalibrationML(snap, cfg, nil)
		assertFindingsEqual(t, pure, ml)
	})

	t.Run("AnchoringContext", func(t *testing.T) {
		cfg := DefaultAnchoringContextConfig()
		pure := DetectAnchoringContext(snap, cfg)
		ml := DetectAnchoringContextML(snap, cfg, nil)
		assertFindingsEqual(t, pure, ml)
	})
}

func assertFindingsEqual(t *testing.T, a, b *reasoningv1.CognitiveAssessment) {
	t.Helper()
	if a == nil && b == nil {
		return
	}
	if a == nil || b == nil {
		t.Errorf("one is nil: a=%v, b=%v", a, b)
		return
	}
	if a.GetFindingType() != b.GetFindingType() {
		t.Errorf("finding type mismatch: %v vs %v", a.GetFindingType(), b.GetFindingType())
	}
	if a.GetSeverity() != b.GetSeverity() {
		t.Errorf("severity mismatch: %v vs %v", a.GetSeverity(), b.GetSeverity())
	}
	if math.Abs(a.GetConfidence()-b.GetConfidence()) > 1e-9 {
		t.Errorf("confidence mismatch: %f vs %f", a.GetConfidence(), b.GetConfidence())
	}
	if a.GetDetectorName() != b.GetDetectorName() {
		t.Errorf("detector name mismatch: %s vs %s", a.GetDetectorName(), b.GetDetectorName())
	}
}

// TestDetectorML_SunkCostBoost verifies ML enrichment boosts sunk-cost confidence.
func TestDetectorML_SunkCostBoost(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "We've already spent too much on this project."},
			{TurnNumber: 2, Speaker: "user", RawText: "We should keep going and finish it."},
		},
	}
	snap = Enrich(snap)

	cfg := DefaultSunkCostConfig()
	ml := &cerebrov1.MLEnrichment{
		SunkCostPhrases: []string{"already spent too much"},
	}

	pure := DetectSunkCost(snap, cfg)
	enhanced := DetectSunkCostML(snap, cfg, ml)

	if pure == nil {
		t.Fatal("expected PURE finding")
	}
	if enhanced == nil {
		t.Fatal("expected ML-enhanced finding")
	}
	if enhanced.GetConfidence() <= pure.GetConfidence() {
		t.Errorf("expected ML to boost confidence: pure=%f, enhanced=%f",
			pure.GetConfidence(), enhanced.GetConfidence())
	}
}

// TestDetectorML_SunkCostMLOnly verifies ML can produce findings PURE missed.
func TestDetectorML_SunkCostMLOnly(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "The resources committed to date are significant."},
		},
	}
	snap = Enrich(snap)

	cfg := DefaultSunkCostConfig()
	ml := &cerebrov1.MLEnrichment{
		SunkCostPhrases: []string{"resources committed to date"},
	}

	pure := DetectSunkCost(snap, cfg)
	if pure != nil {
		t.Skip("PURE unexpectedly found sunk-cost in this text")
	}

	enhanced := DetectSunkCostML(snap, cfg, ml)
	if enhanced == nil {
		t.Fatal("expected ML-only finding")
	}
	if enhanced.GetConfidence() != 0.4 {
		t.Errorf("expected confidence=0.4, got %f", enhanced.GetConfidence())
	}
	if enhanced.GetSeverity() != reasoningv1.FindingSeverity_INFO {
		t.Errorf("expected INFO severity, got %v", enhanced.GetSeverity())
	}
}

// TestDetectorML_CalibrationBoost verifies ML epistemic mismatch boosts calibration finding.
func TestDetectorML_CalibrationBoost(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "I'm absolutely sure this will work."},
		},
	}
	snap = Enrich(snap)

	cfg := DefaultCalibratorConfig()
	ml := &cerebrov1.MLEnrichment{
		Claims: []*cerebrov1.MLClaim{
			{Text: "this will work", EpistemicStatus: "certain", EvidenceRefs: nil},
		},
		ConfidenceMarkers: []string{"absolutely sure"},
	}

	pure := DetectConfidenceMiscalibration(snap, cfg)
	enhanced := DetectConfidenceMiscalibrationML(snap, cfg, ml)

	if pure == nil {
		t.Fatal("expected PURE finding")
	}
	if enhanced == nil {
		t.Fatal("expected ML-enhanced finding")
	}
	if enhanced.GetConfidence() <= pure.GetConfidence() {
		t.Errorf("expected ML to boost confidence: pure=%f, enhanced=%f",
			pure.GetConfidence(), enhanced.GetConfidence())
	}
}

// TestAssessUrgencyML verifies ML formality blending.
func TestAssessUrgencyML(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Hey, I guess we should look at this thing."},
		},
	}

	cfg := DefaultUrgencyConfig()

	// PURE formality
	pureGain := AssessUrgency(snap, cfg)

	// ML formality (formal register)
	ml := &cerebrov1.MLEnrichment{
		Formality: &cerebrov1.MLFormalityIndicators{
			OverallScore: 0.9,
			Register:     "formal",
		},
	}
	mlGain := AssessUrgencyML(snap, cfg, ml)

	// ML says formal (0.9), PURE says informal-ish. Blended should be higher than PURE.
	if mlGain.Formality <= pureGain.Formality {
		t.Errorf("expected ML to increase formality: pure=%f, ml=%f",
			pureGain.Formality, mlGain.Formality)
	}
}

// TestAssessUrgencyML_NilML verifies fallback to PURE.
func TestAssessUrgencyML_NilML(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Test message"},
		},
	}
	cfg := DefaultUrgencyConfig()

	pureGain := AssessUrgency(snap, cfg)
	mlGain := AssessUrgencyML(snap, cfg, nil)

	if math.Abs(pureGain.Formality-mlGain.Formality) > 1e-9 {
		t.Errorf("expected identical formality with nil ML: pure=%f, ml=%f",
			pureGain.Formality, mlGain.Formality)
	}
}

// TestPipeline_MLDisabled verifies pipeline results are unchanged when ML is disabled.
func TestPipeline_MLDisabled(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "We already spent $50,000 on this."},
			{TurnNumber: 2, Speaker: "user", RawText: "We should keep going."},
		},
		Objective: "Project evaluation",
	}

	cfg := DefaultPipelineConfig()
	cfg.UseInhibitor = true
	cfg.Inhibitor = DefaultInhibitorConfig()

	result := Run(snap, cfg)
	if result.MLEnrichments != nil {
		t.Error("expected nil ML enrichments when disabled")
	}
}
