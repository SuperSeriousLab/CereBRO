package pipeline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// captureFeedbackRequest runs a test Sophrim server, fires SendFeedback, and
// returns the decoded request body.  The caller provides signal and context.
func captureFeedbackRequest(t *testing.T, query string, factIDs []int64, signal, fbContext string) feedbackRequest {
	t.Helper()

	var mu sync.Mutex
	var received feedbackRequest
	done := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/feedback" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req feedbackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode error: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			close(done)
			return
		}
		mu.Lock()
		received = req
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer srv.Close()

	sender := NewFeedbackSender(srv.URL, 5*time.Second)
	// Call synchronously (not in goroutine) so we can wait reliably in tests.
	go sender.SendFeedback(context.Background(), query, factIDs, signal, fbContext)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for Sophrim feedback request")
	}

	mu.Lock()
	defer mu.Unlock()
	return received
}

// TestFeedbackSender_PositiveSignal verifies correct body for a positive signal.
func TestFeedbackSender_PositiveSignal(t *testing.T) {
	factIDs := []int64{1, 2, 3}
	req := captureFeedbackRequest(t, "what is anchoring bias?", factIDs, "positive", "findings=2,types=anchoring,contradiction")

	if req.Query != "what is anchoring bias?" {
		t.Errorf("expected query 'what is anchoring bias?', got %q", req.Query)
	}
	if len(req.FactIDs) != 3 {
		t.Errorf("expected 3 fact IDs, got %d", len(req.FactIDs))
	}
	if req.Signal != "positive" {
		t.Errorf("expected signal 'positive', got %q", req.Signal)
	}
	if req.Source != "cerebro" {
		t.Errorf("expected source 'cerebro', got %q", req.Source)
	}
	if req.Context != "findings=2,types=anchoring,contradiction" {
		t.Errorf("unexpected context: %q", req.Context)
	}
}

// TestFeedbackSender_NegativeSignal verifies correct body for a negative signal.
func TestFeedbackSender_NegativeSignal(t *testing.T) {
	factIDs := []int64{7, 42}
	req := captureFeedbackRequest(t, "sunk cost analysis", factIDs, "negative", "no_findings")

	if req.Signal != "negative" {
		t.Errorf("expected signal 'negative', got %q", req.Signal)
	}
	if req.Source != "cerebro" {
		t.Errorf("expected source 'cerebro', got %q", req.Source)
	}
	if req.Context != "no_findings" {
		t.Errorf("expected context 'no_findings', got %q", req.Context)
	}
}

// TestFeedbackSender_SourceIsAlwaysCerebro verifies source is hardcoded.
func TestFeedbackSender_SourceIsAlwaysCerebro(t *testing.T) {
	factIDs := []int64{99}
	req := captureFeedbackRequest(t, "test query", factIDs, "positive", "findings=1,types=scope_drift")

	if req.Source != "cerebro" {
		t.Errorf("source must always be 'cerebro', got %q", req.Source)
	}
}

// TestFeedbackSender_Unreachable verifies no panic when Sophrim is unreachable.
func TestFeedbackSender_Unreachable(t *testing.T) {
	// Use an address that will immediately fail (closed port).
	sender := NewFeedbackSender("http://127.0.0.1:1", 100*time.Millisecond)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Must not panic.
		sender.SendFeedback(context.Background(), "query", []int64{1}, "positive", "findings=1,types=anchoring")
	}()

	select {
	case <-done:
		// Good — returned without panic.
	case <-time.After(3 * time.Second):
		t.Fatal("SendFeedback blocked longer than expected on unreachable server")
	}
}

// TestFeedbackSender_ContextIncludesFindingCountAndTypes verifies the context
// string format produced by the pipeline hook helper functions.
func TestFeedbackSender_ContextIncludesFindingCountAndTypes(t *testing.T) {
	result := &PipelineResult{
		Findings: []*reasoningv1.CognitiveAssessment{
			{
				FindingType:  reasoningv1.FindingType_ANCHORING_BIAS,
				DetectorName: "anchoring-detector-context",
				Confidence:   0.8,
			},
			{
				FindingType:  reasoningv1.FindingType_CONTRADICTION,
				DetectorName: "contradiction-tracker",
				Confidence:   0.7,
			},
		},
	}

	types := findingTypes(result)
	// Both types should appear; order is insertion order of findings.
	if types == "" {
		t.Fatal("expected non-empty finding types string")
	}

	// The full context string as the pipeline wires it.
	fbContext := "findings=2,types=" + types
	if fbContext != "findings=2,types=anchoring,contradiction" {
		t.Errorf("unexpected context string: %q", fbContext)
	}
}

// TestFeedbackSender_DeduplicatesFindingTypes verifies duplicate types are collapsed.
func TestFeedbackSender_DeduplicatesFindingTypes(t *testing.T) {
	result := &PipelineResult{
		Findings: []*reasoningv1.CognitiveAssessment{
			{FindingType: reasoningv1.FindingType_SCOPE_DRIFT},
			{FindingType: reasoningv1.FindingType_SCOPE_DRIFT}, // duplicate
		},
	}
	types := findingTypes(result)
	if types != "scope_drift" {
		t.Errorf("expected deduplicated 'scope_drift', got %q", types)
	}
}

// TestFeedbackSender_EmptyFindings verifies findingTypes returns empty for no findings.
func TestFeedbackSender_EmptyFindings(t *testing.T) {
	result := &PipelineResult{Findings: nil}
	types := findingTypes(result)
	if types != "" {
		t.Errorf("expected empty string for nil findings, got %q", types)
	}
}
