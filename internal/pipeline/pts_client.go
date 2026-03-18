// PTSClient sends anomaly signals to PTS (Problem Tracking System) after pipeline runs.
//
// When CereBRO COGs detect anomalies — integrity score hits zero, individual
// finding confidence is below the low-confidence threshold, or the pipeline's
// self-confidence assessor recommends review — a cog/signal is POSTed to PTS.
//
// The sender is fire-and-forget: it runs in a goroutine and never blocks the
// pipeline. Errors are logged but never propagated.
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
)

const (
	// DefaultPTSEndpoint is the default PTS service URL.
	DefaultPTSEndpoint = "http://192.168.14.68:9746"

	// DefaultPTSTimeout is the HTTP timeout for PTS signal delivery.
	DefaultPTSTimeout = 5 * time.Second

	// LowConfidenceThreshold is the per-finding confidence level below which
	// a PTS signal is raised.
	LowConfidenceThreshold = 0.5

	// InjectConfidenceThreshold is the per-finding confidence level at or above
	// which a detection signal is injected into PTS via POST /inject.
	InjectConfidenceThreshold = 0.6

	// injectPTSTimeout is the HTTP timeout for PTS /inject calls.
	injectPTSTimeout = 3 * time.Second
)

// ptsSignalRequest is the payload for POST /cog/signal.
type ptsSignalRequest struct {
	Cog         string   `json:"cog"`
	Observation string   `json:"observation"`
	Artifacts   []string `json:"artifacts"`
}

// ptsInjectRequest is the payload for POST /inject.
type ptsInjectRequest struct {
	Text string `json:"text"`
}

// PTSClient sends cog signals to PTS.
type PTSClient struct {
	endpoint string
	timeout  time.Duration
	http     *http.Client
}

// NewPTSClient returns a PTSClient for the given endpoint (e.g. "http://192.168.14.68:9746").
func NewPTSClient(endpoint string, timeout time.Duration) *PTSClient {
	return &PTSClient{
		endpoint: endpoint,
		timeout:  timeout,
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

// Send posts a cog/signal to PTS.
// This method is intended to be called as a goroutine (fire-and-forget).
// Errors are logged but never propagated.
func (c *PTSClient) Send(ctx context.Context, cog, observation string, artifacts []string) {
	body := ptsSignalRequest{
		Cog:         cog,
		Observation: observation,
		Artifacts:   artifacts,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		log.Printf("[pts-client] marshal error: %v", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/cog/signal", bytes.NewReader(payload))
	if err != nil {
		log.Printf("[pts-client] request build error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("[pts-client] send error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[pts-client] unexpected status %d posting cog signal for %q", resp.StatusCode, cog)
	}
}

// collectPTSSignals inspects a completed PipelineResult and returns zero or more
// signals that should be sent to PTS.
//
// Trigger conditions:
//  1. OverallIntegrityScore == 0 — pipeline has aggregated enough severity to
//     floor the score; something meaningful is wrong.
//  2. Any individual finding has Confidence < LowConfidenceThreshold — the
//     detector fired but is uncertain; worth human review.
//  3. SelfConfidenceReport == LOW_CONFIDENCE_REVIEW_RECOMMENDED — the
//     metacognitive assessor flags the whole report as unreliable.
//  4. Layer 0 rejected the input — an unexpected or toxic input pattern.
func collectPTSSignals(result *PipelineResult) []ptsSignalRequest {
	var signals []ptsSignalRequest

	// Condition 4: Layer 0 rejection.
	if result.Rejected && result.Layer0 != nil {
		reason := "Layer 0 rejected input"
		if result.Layer0.Reason != "" {
			reason = fmt.Sprintf("Layer 0 rejected input: %s", result.Layer0.Reason)
		}
		signals = append(signals, ptsSignalRequest{
			Cog:         "layer0-reflex",
			Observation: reason,
			Artifacts:   []string{"layer0", "cerebro-pipeline"},
		})
		// Layer 0 stops the pipeline; no further conditions apply.
		return signals
	}

	if result.Report == nil {
		return signals
	}

	// Condition 1: Integrity score floored at zero.
	if result.Report.GetOverallIntegrityScore() == 0 {
		count := result.Report.GetCriticalCount()
		signals = append(signals, ptsSignalRequest{
			Cog: "cerebro-aggregator",
			Observation: fmt.Sprintf(
				"Pipeline integrity score reached 0 — %d critical finding(s) detected (conversation: %s)",
				count, result.Report.GetConversationId(),
			),
			Artifacts: []string{"cerebro-aggregator", "cerebro-pipeline"},
		})
	}

	// Condition 2: Individual findings with low confidence.
	for _, f := range result.Findings {
		if f.GetConfidence() < LowConfidenceThreshold {
			det := f.GetDetectorName()
			if det == "" {
				det = findingTypeName(f.GetFindingType())
			}
			signals = append(signals, ptsSignalRequest{
				Cog: det,
				Observation: fmt.Sprintf(
					"%s scored low confidence (%.2f) — possible false positive or ambiguous input (conversation: %s)",
					det, f.GetConfidence(), result.Report.GetConversationId(),
				),
				Artifacts: []string{det, "cerebro-pipeline", "layer2-detectors"},
			})
		}
	}

	// Condition 3: Metacognitive low-confidence recommendation.
	if result.SelfConf != nil &&
		result.SelfConf.GetRecommendation() == cerebrov1.ConfidenceRecommendation_LOW_CONFIDENCE_REVIEW_RECOMMENDED {
		signals = append(signals, ptsSignalRequest{
			Cog: "cerebro-self-confidence",
			Observation: fmt.Sprintf(
				"Self-confidence assessor flagged report for review (overall confidence: %.2f, pattern: %s, conversation: %s)",
				result.SelfConf.GetOverallConfidence(),
				result.SelfConf.GetFindingPattern(),
				result.Report.GetConversationId(),
			),
			Artifacts: []string{"cerebro-self-confidence", "cerebro-pipeline", "layer4-metacognition"},
		})
	}

	return signals
}

// maybeSendPTSSignals fires a goroutine for each signal that should be sent to PTS.
// It is a no-op when PTSEndpoint is empty. Never blocks.
func maybeSendPTSSignals(result *PipelineResult, endpoint string) {
	if endpoint == "" {
		return
	}
	signals := collectPTSSignals(result)
	if len(signals) == 0 {
		return
	}
	client := NewPTSClient(endpoint, DefaultPTSTimeout)
	for _, sig := range signals {
		s := sig // capture loop variable
		go client.Send(context.Background(), s.Cog, s.Observation, s.Artifacts)
	}
}

// SendInject POSTs a plain-text detection signal to PTS /inject.
// This method is intended to be called as a goroutine (fire-and-forget).
// Errors are logged with Warn level but never propagated.
func (c *PTSClient) SendInject(ctx context.Context, text string) {
	body := ptsInjectRequest{Text: text}

	payload, err := json.Marshal(body)
	if err != nil {
		log.Printf("[pts-client] inject marshal error: %v", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/inject", bytes.NewReader(payload))
	if err != nil {
		log.Printf("[pts-client] inject request build error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("[pts-client] inject send error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[pts-client] inject unexpected status %d", resp.StatusCode)
	}
}

// maybeInjectPTSFindings fires a goroutine for each finding with confidence >= InjectConfidenceThreshold,
// POSTing the detection text to PTS /inject. It is a no-op when endpoint is empty. Never blocks.
func maybeInjectPTSFindings(result *PipelineResult, endpoint string) {
	if endpoint == "" || result == nil || result.Rejected {
		return
	}
	convID := ""
	if result.Report != nil {
		convID = result.Report.GetConversationId()
	}
	client := &PTSClient{
		endpoint: endpoint,
		timeout:  injectPTSTimeout,
		http:     &http.Client{Timeout: injectPTSTimeout},
	}
	for _, f := range result.Findings {
		if f.GetConfidence() < InjectConfidenceThreshold {
			continue
		}
		cogName := f.GetDetectorName()
		if cogName == "" {
			cogName = findingTypeName(f.GetFindingType())
		}
		text := fmt.Sprintf(
			"CereBRO COG signal: %s detected %s (conf=%.2f) in conversation %s",
			cogName, findingTypeName(f.GetFindingType()), f.GetConfidence(), convID,
		)
		t := text // capture loop variable
		go client.SendInject(context.Background(), t)
	}
}

