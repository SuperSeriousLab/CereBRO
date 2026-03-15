// SophrimFeedbackSender sends retrieval quality signals to Sophrim after pipeline runs.
//
// This is Connection A of the Lamarckian Loop: when CereBRO confirms a finding,
// the Sophrim facts that were injected during grounding were relevant (positive).
// When no findings are produced, the grounding may have been insufficient (negative).
//
// The sender is fire-and-forget — errors are logged, never block the pipeline.
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// feedbackRequest is the payload sent to Sophrim's /feedback endpoint.
type feedbackRequest struct {
	Query   string  `json:"query"`
	FactIDs []int64 `json:"fact_ids"`
	Signal  string  `json:"signal"`
	Source  string  `json:"source"`
	Context string  `json:"context"`
}

// FeedbackSender sends retrieval quality signals to Sophrim after pipeline runs.
type FeedbackSender struct {
	endpoint string
	timeout  time.Duration
	http     *http.Client
}

// NewFeedbackSender returns a FeedbackSender configured for the given endpoint and timeout.
func NewFeedbackSender(endpoint string, timeout time.Duration) *FeedbackSender {
	return &FeedbackSender{
		endpoint: endpoint,
		timeout:  timeout,
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

// SendFeedback posts a retrieval quality signal to Sophrim /feedback.
// factIDs are the Sophrim fact IDs that were injected during grounding.
// signal is "positive" (finding confirmed — facts were relevant) or "negative".
// This method is intended to be called as a goroutine (fire-and-forget).
// Errors are logged but never propagated.
func (f *FeedbackSender) SendFeedback(ctx context.Context, query string, factIDs []int64, signal string, findingContext string) {
	body := feedbackRequest{
		Query:   query,
		FactIDs: factIDs,
		Signal:  signal,
		Source:  "cerebro",
		Context: findingContext,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		log.Printf("[sophrim-feedback] marshal error: %v", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.endpoint+"/feedback", bytes.NewReader(payload))
	if err != nil {
		log.Printf("[sophrim-feedback] request build error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.http.Do(req)
	if err != nil {
		log.Printf("[sophrim-feedback] send error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[sophrim-feedback] unexpected status %d", resp.StatusCode)
	}
}

// findingTypes returns a comma-separated list of unique finding type names from the result.
// Used to build the context string for the Sophrim feedback signal.
func findingTypes(result *PipelineResult) string {
	if result == nil || len(result.Findings) == 0 {
		return ""
	}
	seen := make(map[string]bool)
	var types []string
	for _, f := range result.Findings {
		name := findingTypeName(f.GetFindingType())
		if !seen[name] {
			seen[name] = true
			types = append(types, name)
		}
	}
	return strings.Join(types, ",")
}

// findingTypeName returns a short name for a FindingType enum value.
func findingTypeName(ft reasoningv1.FindingType) string {
	switch ft {
	case reasoningv1.FindingType_ANCHORING_BIAS:
		return "anchoring"
	case reasoningv1.FindingType_SUNK_COST_FALLACY:
		return "sunk_cost"
	case reasoningv1.FindingType_CONTRADICTION:
		return "contradiction"
	case reasoningv1.FindingType_SCOPE_DRIFT:
		return "scope_drift"
	case reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION:
		return "miscalibration"
	case reasoningv1.FindingType_SILENT_REVISION:
		return "silent_revision"
	case reasoningv1.FindingType_STATUS_QUO_BIAS:
		return "status_quo"
	default:
		return fmt.Sprintf("type_%d", int(ft))
	}
}
