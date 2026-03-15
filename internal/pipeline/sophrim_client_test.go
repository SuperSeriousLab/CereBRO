package pipeline

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func sophrimHintsResponse(primaryDomain, textEra string, confidence float64) string {
	resp := sophrimResponse{
		Augmented: true,
		DomainHints: &struct {
			PrimaryDomain  string   `json:"primary_domain"`
			SourceProjects []string `json:"source_projects"`
			TextEra        string   `json:"text_era"`
			Confidence     float64  `json:"confidence"`
		}{
			PrimaryDomain: primaryDomain,
			TextEra:       textEra,
			Confidence:    confidence,
		},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

func sophrimNoHintsResponse() string {
	resp := sophrimResponse{Augmented: false}
	b, _ := json.Marshal(resp)
	return string(b)
}

// ─── FetchDomainContext ────────────────────────────────────────────────────────

func TestSophrimClient_ClassicalHints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(sophrimHintsResponse("philosophy", "classical", 0.85)))
	}))
	defer server.Close()

	client := NewSophrimClient(server.URL, 2*time.Second)
	dc := client.FetchDomainContext(nil, "What is justice?") //nolint:staticcheck — nil ctx replaced by bkg in real call; test uses httptest

	if dc == nil {
		t.Fatal("expected DomainContext, got nil")
	}
	if dc.TextEra != "classical" {
		t.Errorf("TextEra: want classical, got %q", dc.TextEra)
	}
	if dc.Confidence != 0.85 {
		t.Errorf("Confidence: want 0.85, got %f", dc.Confidence)
	}
	if dc.PrimaryDomain != "philosophy" {
		t.Errorf("PrimaryDomain: want philosophy, got %q", dc.PrimaryDomain)
	}
}

func TestSophrimClient_TechnicalHints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(sophrimHintsResponse("technical", "modern", 0.90)))
	}))
	defer server.Close()

	client := NewSophrimClient(server.URL, 2*time.Second)
	dc := client.FetchDomainContext(nil, "Explain Go interfaces")

	if dc == nil {
		t.Fatal("expected DomainContext, got nil")
	}
	if dc.PrimaryDomain != "technical" {
		t.Errorf("PrimaryDomain: want technical, got %q", dc.PrimaryDomain)
	}
	if dc.TextEra != "modern" {
		t.Errorf("TextEra: want modern, got %q", dc.TextEra)
	}
}

func TestSophrimClient_Unreachable_ReturnsNil(t *testing.T) {
	// Point at a port that is almost certainly not listening.
	client := NewSophrimClient("http://127.0.0.1:19999", 200*time.Millisecond)
	dc := client.FetchDomainContext(nil, "some query")
	if dc != nil {
		t.Errorf("unreachable server: expected nil DomainContext, got %+v", dc)
	}
}

func TestSophrimClient_Timeout_ReturnsNil(t *testing.T) {
	// Server that never responds within the client timeout.
	block := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block // block forever
	}))
	defer func() {
		close(block)
		server.Close()
	}()

	client := NewSophrimClient(server.URL, 50*time.Millisecond)
	dc := client.FetchDomainContext(nil, "some query")
	if dc != nil {
		t.Errorf("timeout: expected nil DomainContext, got %+v", dc)
	}
}

func TestSophrimClient_NoHints_ReturnsNil(t *testing.T) {
	// Server returns augmented=false with no domain_hints.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(sophrimNoHintsResponse()))
	}))
	defer server.Close()

	client := NewSophrimClient(server.URL, 2*time.Second)
	dc := client.FetchDomainContext(nil, "some query")
	if dc != nil {
		t.Errorf("no hints: expected nil DomainContext, got %+v", dc)
	}
}

func TestSophrimClient_EmptySummary_ReturnsNil(t *testing.T) {
	// Should short-circuit before making any HTTP request.
	client := NewSophrimClient("http://127.0.0.1:19999", 200*time.Millisecond)
	dc := client.FetchDomainContext(nil, "")
	if dc != nil {
		t.Errorf("empty summary: expected nil DomainContext, got %+v", dc)
	}
}

func TestSophrimClient_NonOKStatus_ReturnsNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewSophrimClient(server.URL, 2*time.Second)
	dc := client.FetchDomainContext(nil, "some query")
	if dc != nil {
		t.Errorf("500 status: expected nil DomainContext, got %+v", dc)
	}
}

// ─── conversationSummary ───────────────────────────────────────────────────────

func TestConversationSummary_NilSnap(t *testing.T) {
	if s := conversationSummary(nil); s != "" {
		t.Errorf("nil snap: expected empty string, got %q", s)
	}
}

func TestConversationSummary_WithObjective(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Objective: "What is justice?",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "This text should not be used."},
		},
	}
	got := conversationSummary(snap)
	if got != "What is justice?" {
		t.Errorf("with objective: expected objective text, got %q", got)
	}
}

func TestConversationSummary_WithoutObjective_UsesTurns(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Hello"},
			{TurnNumber: 2, Speaker: "assistant", RawText: "Hi there"},
			{TurnNumber: 3, Speaker: "user", RawText: "How are you?"},
			{TurnNumber: 4, Speaker: "assistant", RawText: "This fourth turn should not appear"},
		},
	}
	got := conversationSummary(snap)
	if got == "" {
		t.Fatal("without objective: expected non-empty summary from turns")
	}
	// Should contain the first three turns' text.
	if got != "Hello Hi there How are you?" {
		t.Errorf("without objective: unexpected summary %q", got)
	}
	// Should NOT contain the fourth turn.
	if contains(got, "fourth turn") {
		t.Errorf("without objective: fourth turn should be excluded, got %q", got)
	}
}

func TestConversationSummary_Truncation(t *testing.T) {
	long := make([]byte, 600)
	for i := range long {
		long[i] = 'a'
	}
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: string(long)},
		},
	}
	got := conversationSummary(snap)
	if len(got) > 500 {
		t.Errorf("truncation: expected max 500 chars, got %d", len(got))
	}
}

func TestConversationSummary_EmptyObjectiveAndNoTurns(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{Objective: "  "}
	got := conversationSummary(snap)
	if got != "" {
		t.Errorf("whitespace objective + no turns: expected empty summary, got %q", got)
	}
}

// ─── Pipeline integration: SophrimEndpoint wiring ─────────────────────────────

func TestPipeline_SophrimEndpoint_PopulatesDomainContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(sophrimHintsResponse("philosophy", "classical", 0.85)))
	}))
	defer server.Close()

	snap := makeSnapWithNumerics()
	cfg := DefaultPipelineConfig()
	cfg.SophrimEndpoint = server.URL
	// DomainContext is nil — should be populated by Sophrim fetch.

	result := Run(snap, cfg)

	// With classical domain context, anchoring should be skipped.
	for _, f := range result.Findings {
		if f.GetDetectorName() == "anchoring-detector" {
			t.Error("anchoring-detector should be skipped for classical domain from Sophrim")
		}
	}
}

func TestPipeline_SophrimEndpoint_ExplicitDomainContextNotOverridden(t *testing.T) {
	// Sophrim returns classical, but caller already set a modern DomainContext.
	// The explicit value wins.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(sophrimHintsResponse("philosophy", "classical", 0.85)))
	}))
	defer server.Close()

	cfg := DefaultPipelineConfig()
	cfg.SophrimEndpoint = server.URL
	cfg.DomainContext = &DomainContext{PrimaryDomain: "technical", TextEra: "modern", Confidence: 0.95}

	// Since DomainContext is non-nil, Sophrim should NOT be called and the
	// anchoring detector should remain active (modern era does not skip anchoring).
	detectors := buildDetectorMap(applyDomainContext(cfg))
	if _, ok := detectors[DetectorAnchoring]; !ok {
		t.Error("explicit modern DomainContext: anchoring detector should remain active")
	}
}

func TestPipeline_SophrimEndpoint_Unreachable_NilContext(t *testing.T) {
	snap := makeSnapWithNumerics()
	cfg := DefaultPipelineConfig()
	cfg.SophrimEndpoint = "http://127.0.0.1:19999" // unreachable

	// Should not panic and should complete normally with nil DomainContext.
	result := Run(snap, cfg)
	if result == nil {
		t.Fatal("pipeline should return a result even when Sophrim is unreachable")
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
