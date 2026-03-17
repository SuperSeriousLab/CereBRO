package pipeline

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// capturePTSServer starts a test HTTP server that records all received /cog/signal requests.
func capturePTSServer(t *testing.T) (url string, received func() []ptsSignalRequest, shutdown func()) {
	t.Helper()
	var mu sync.Mutex
	var reqs []ptsSignalRequest

	mux := http.NewServeMux()
	mux.HandleFunc("/cog/signal", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		var req ptsSignalRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "unmarshal error", http.StatusBadRequest)
			return
		}
		mu.Lock()
		reqs = append(reqs, req)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"accepted","problem_id":"test-id","phase":"detected"}`))
	})

	srv := httptest.NewServer(mux)
	return srv.URL,
		func() []ptsSignalRequest {
			mu.Lock()
			defer mu.Unlock()
			cp := make([]ptsSignalRequest, len(reqs))
			copy(cp, reqs)
			return cp
		},
		srv.Close
}

// waitForSignals polls received() until count signals arrive or timeout.
func waitForSignals(received func() []ptsSignalRequest, count int, timeout time.Duration) []ptsSignalRequest {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if sigs := received(); len(sigs) >= count {
			return sigs
		}
		time.Sleep(5 * time.Millisecond)
	}
	return received()
}

// ── collectPTSSignals unit tests ────────────────────────────────────────────

func TestCollectPTSSignals_CleanResult(t *testing.T) {
	result := &PipelineResult{
		Report: &reasoningv1.ReasoningReport{
			OverallIntegrityScore: 1.0,
			ConversationId:        "conv-clean",
		},
	}
	sigs := collectPTSSignals(result)
	if len(sigs) != 0 {
		t.Errorf("expected 0 signals for clean result, got %d", len(sigs))
	}
}

func TestCollectPTSSignals_ZeroIntegrityScore(t *testing.T) {
	result := &PipelineResult{
		Report: &reasoningv1.ReasoningReport{
			OverallIntegrityScore: 0,
			CriticalCount:         4,
			ConversationId:        "conv-zero",
		},
	}
	sigs := collectPTSSignals(result)
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signal for zero score, got %d", len(sigs))
	}
	if sigs[0].Cog != "cerebro-aggregator" {
		t.Errorf("unexpected cog name %q", sigs[0].Cog)
	}
	if len(sigs[0].Artifacts) == 0 {
		t.Error("expected non-empty artifacts")
	}
}

func TestCollectPTSSignals_LowConfidenceFinding(t *testing.T) {
	result := &PipelineResult{
		Report: &reasoningv1.ReasoningReport{
			OverallIntegrityScore: 0.9,
			ConversationId:        "conv-lowconf",
		},
		Findings: []*reasoningv1.CognitiveAssessment{
			{
				FindingType:  reasoningv1.FindingType_SCOPE_DRIFT,
				DetectorName: "scope-guard",
				Confidence:   0.3, // below 0.5 threshold
			},
		},
	}
	sigs := collectPTSSignals(result)
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signal for low-confidence finding, got %d", len(sigs))
	}
	if sigs[0].Cog != "scope-guard" {
		t.Errorf("unexpected cog name %q", sigs[0].Cog)
	}
}

func TestCollectPTSSignals_HighConfidenceFindingNoSignal(t *testing.T) {
	result := &PipelineResult{
		Report: &reasoningv1.ReasoningReport{
			OverallIntegrityScore: 0.7,
			ConversationId:        "conv-highconf",
		},
		Findings: []*reasoningv1.CognitiveAssessment{
			{
				FindingType:  reasoningv1.FindingType_CONTRADICTION,
				DetectorName: "contradiction-tracker",
				Confidence:   0.85, // above threshold — no signal
			},
		},
	}
	sigs := collectPTSSignals(result)
	if len(sigs) != 0 {
		t.Errorf("expected 0 signals for high-confidence finding, got %d", len(sigs))
	}
}

func TestCollectPTSSignals_MetacognitiveLowConfidence(t *testing.T) {
	result := &PipelineResult{
		Report: &reasoningv1.ReasoningReport{
			OverallIntegrityScore: 0.8,
			ConversationId:        "conv-metacog",
		},
		SelfConf: &cerebrov1.SelfConfidenceReport{
			OverallConfidence: 0.3,
			FindingPattern:    "SCOPE_DRIFT",
			Recommendation:    cerebrov1.ConfidenceRecommendation_LOW_CONFIDENCE_REVIEW_RECOMMENDED,
		},
	}
	sigs := collectPTSSignals(result)
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signal for metacognitive low confidence, got %d", len(sigs))
	}
	if sigs[0].Cog != "cerebro-self-confidence" {
		t.Errorf("unexpected cog name %q", sigs[0].Cog)
	}
}

func TestCollectPTSSignals_Layer0Rejection(t *testing.T) {
	result := &PipelineResult{
		Rejected: true,
		Layer0: &Layer0Result{
			Accepted: false,
			Reason:   "toxicity: matched [slur1]",
		},
	}
	sigs := collectPTSSignals(result)
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signal for Layer 0 rejection, got %d", len(sigs))
	}
	if sigs[0].Cog != "layer0-reflex" {
		t.Errorf("unexpected cog name %q", sigs[0].Cog)
	}
}

func TestCollectPTSSignals_MultipleConditions(t *testing.T) {
	// Zero score + low-confidence finding + metacognitive flag = 3 signals.
	result := &PipelineResult{
		Report: &reasoningv1.ReasoningReport{
			OverallIntegrityScore: 0,
			CriticalCount:         4,
			ConversationId:        "conv-multi",
		},
		Findings: []*reasoningv1.CognitiveAssessment{
			{
				FindingType:  reasoningv1.FindingType_ANCHORING_BIAS,
				DetectorName: "anchoring-detector",
				Confidence:   0.2,
			},
		},
		SelfConf: &cerebrov1.SelfConfidenceReport{
			OverallConfidence: 0.2,
			FindingPattern:    "ANCHORING_BIAS",
			Recommendation:    cerebrov1.ConfidenceRecommendation_LOW_CONFIDENCE_REVIEW_RECOMMENDED,
		},
	}
	sigs := collectPTSSignals(result)
	if len(sigs) != 3 {
		t.Errorf("expected 3 signals, got %d", len(sigs))
	}
}

// ── maybeSendPTSSignals integration tests (real HTTP) ───────────────────────

func TestMaybeSendPTSSignals_NoEndpoint(t *testing.T) {
	// With empty endpoint, no HTTP call is made. Should not panic.
	result := &PipelineResult{
		Report: &reasoningv1.ReasoningReport{
			OverallIntegrityScore: 0,
			ConversationId:        "conv-no-endpoint",
		},
	}
	maybeSendPTSSignals(result, "") // no-op
}

func TestMaybeSendPTSSignals_ZeroScore_PostsSignal(t *testing.T) {
	url, received, shutdown := capturePTSServer(t)
	defer shutdown()

	result := &PipelineResult{
		Report: &reasoningv1.ReasoningReport{
			OverallIntegrityScore: 0,
			CriticalCount:         2,
			ConversationId:        "conv-zero-fire",
		},
	}
	maybeSendPTSSignals(result, url)

	sigs := waitForSignals(received, 1, 2*time.Second)
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signal delivered to PTS, got %d", len(sigs))
	}
	if sigs[0].Cog != "cerebro-aggregator" {
		t.Errorf("unexpected cog %q", sigs[0].Cog)
	}
}

func TestMaybeSendPTSSignals_LowConfidenceFinding_PostsSignal(t *testing.T) {
	url, received, shutdown := capturePTSServer(t)
	defer shutdown()

	result := &PipelineResult{
		Report: &reasoningv1.ReasoningReport{
			OverallIntegrityScore: 0.9,
			ConversationId:        "conv-lowconf-fire",
		},
		Findings: []*reasoningv1.CognitiveAssessment{
			{
				FindingType:  reasoningv1.FindingType_SUNK_COST_FALLACY,
				DetectorName: "sunk-cost-detector",
				Confidence:   0.1,
			},
		},
	}
	maybeSendPTSSignals(result, url)

	sigs := waitForSignals(received, 1, 2*time.Second)
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(sigs))
	}
	if sigs[0].Cog != "sunk-cost-detector" {
		t.Errorf("unexpected cog %q", sigs[0].Cog)
	}
}

func TestMaybeSendPTSSignals_CleanResult_NoPost(t *testing.T) {
	url, received, shutdown := capturePTSServer(t)
	defer shutdown()

	result := &PipelineResult{
		Report: &reasoningv1.ReasoningReport{
			OverallIntegrityScore: 1.0,
			ConversationId:        "conv-clean-fire",
		},
	}
	maybeSendPTSSignals(result, url)

	// Give goroutines (if any) a chance to run.
	time.Sleep(50 * time.Millisecond)
	sigs := received()
	if len(sigs) != 0 {
		t.Errorf("expected 0 signals for clean result, got %d", len(sigs))
	}
}

// ── PTSClient.Send unit test ─────────────────────────────────────────────────

func TestPTSClientSend_DeliversSingleSignal(t *testing.T) {
	url, received, shutdown := capturePTSServer(t)
	defer shutdown()

	client := NewPTSClient(url, 2*time.Second)
	client.Send(t.Context(), "test-cog", "test observation", []string{"art1", "art2"})

	sigs := received()
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(sigs))
	}
	if sigs[0].Cog != "test-cog" {
		t.Errorf("unexpected cog %q", sigs[0].Cog)
	}
	if sigs[0].Observation != "test observation" {
		t.Errorf("unexpected observation %q", sigs[0].Observation)
	}
	if len(sigs[0].Artifacts) != 2 {
		t.Errorf("expected 2 artifacts, got %d", len(sigs[0].Artifacts))
	}
}

func TestPTSClientSend_ToleratesUnreachableEndpoint(t *testing.T) {
	// Should not panic or block — fire-and-forget.
	client := NewPTSClient("http://127.0.0.1:19999", 100*time.Millisecond)
	client.Send(t.Context(), "cog", "obs", nil) // synchronous call; errors are logged only
}
