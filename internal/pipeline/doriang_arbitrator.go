package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// DebateArbitrator arbitrates conflicting L2 detector findings via an external
// debate engine. Implementations must be nil-safe: a nil receiver must behave
// as a no-op pass-through that returns the original findings unmodified.
type DebateArbitrator interface {
	// Arbitrate receives a cluster of conflicting findings (same finding_type,
	// opposing confidence levels) and returns the same findings with adjusted
	// confidence values. The result is advisory only: confidence is nudged
	// proportionally toward the debate synthesis score, but no finding is
	// added, removed, or have its type changed.
	Arbitrate(ctx context.Context, conflictCluster []*reasoningv1.CognitiveAssessment) ([]*reasoningv1.CognitiveAssessment, error)
}

// DorangArbitratorConfig holds connection parameters for the DORIANG debate engine.
type DorangArbitratorConfig struct {
	Enabled        bool
	Host           string
	CouncilID      string
	TimeoutSeconds int
}

// DefaultDorangArbitratorConfig returns the default DORIANG arbitrator config.
// Disabled by default — no external calls unless explicitly opted in.
func DefaultDorangArbitratorConfig() DorangArbitratorConfig {
	return DorangArbitratorConfig{
		Enabled:        false,
		Host:           "http://192.168.14.71:8080",
		CouncilID:      "tech-review",
		TimeoutSeconds: 30,
	}
}

// DorangArbitrator routes conflicting detector findings to the DORIANG debate
// engine for structured 2-round arbitration. The debate synthesis confidence
// score is used to proportionally nudge the winning side's findings upward
// and the losing side's findings downward. All adjustments are bounded to
// [0.0, 1.0]. Detector verdicts (finding types, severity) are never changed.
type DorangArbitrator struct {
	host      string
	councilID string
	timeout   time.Duration
	http      *http.Client
}

// NewDorangArbitrator constructs a DorangArbitrator from a config.
// Returns nil when cfg.Enabled is false — the nil DebateArbitrator is a
// valid no-op that the aggregator handles gracefully.
func NewDorangArbitrator(cfg DorangArbitratorConfig) *DorangArbitrator {
	if !cfg.Enabled {
		return nil
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	host := cfg.Host
	if host == "" {
		host = "http://192.168.14.71:8080"
	}
	councilID := cfg.CouncilID
	if councilID == "" {
		councilID = "tech-review"
	}
	return &DorangArbitrator{
		host:      strings.TrimRight(host, "/"),
		councilID: councilID,
		timeout:   timeout,
		http:      &http.Client{Timeout: timeout},
	}
}

// Arbitrate implements DebateArbitrator. A nil receiver returns the original
// findings unchanged with no error.
func (d *DorangArbitrator) Arbitrate(
	ctx context.Context,
	cluster []*reasoningv1.CognitiveAssessment,
) ([]*reasoningv1.CognitiveAssessment, error) {
	if d == nil {
		return cluster, nil
	}
	if len(cluster) == 0 {
		return cluster, nil
	}

	// Build the debate topic from the conflicting findings.
	topic := buildDebateTopic(cluster)

	// Create a 2-round debate on the DORIANG runtime endpoint.
	debateID, err := d.createDebate(ctx, topic)
	if err != nil {
		return cluster, fmt.Errorf("doriang arbitrator: create debate: %w", err)
	}

	// Poll for debate completion (bounded by the context deadline / timeout).
	synthesis, err := d.pollDebateResult(ctx, debateID)
	if err != nil {
		return cluster, fmt.Errorf("doriang arbitrator: poll result: %w", err)
	}

	// Apply the synthesis confidence to nudge finding confidences proportionally.
	return applyDebateSynthesis(cluster, synthesis), nil
}

// dorangDebateRequest is the payload for POST /api/v1/runtime/debates.
type dorangDebateRequest struct {
	Topic     string `json:"topic"`
	CouncilID string `json:"council_id"`
	Options   struct {
		MaxRounds int `json:"max_rounds"`
	} `json:"options"`
}

// dorangCreateResponse is the subset of the DORIANG create-debate response we
// care about.
type dorangCreateResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		DebateID string `json:"debate_id"`
	} `json:"data"`
}

// dorangGetResponse is the subset of GET /api/v1/runtime/debates/{id} we need.
type dorangGetResponse struct {
	Status bool   `json:"status"`
	Data   struct {
		Status          string   `json:"status"`
		ConfidenceScore *float64 `json:"confidence_score"`
		FinalSummary    *string  `json:"final_summary"`
	} `json:"data"`
}

// debateSynthesis holds the extracted results from a completed DORIANG debate.
type debateSynthesis struct {
	ConfidenceScore float64 // 0.0–1.0, or 0.5 if not provided
	FinalSummary    string
}

func (d *DorangArbitrator) createDebate(ctx context.Context, topic string) (string, error) {
	req := dorangDebateRequest{
		Topic:     topic,
		CouncilID: d.councilID,
	}
	req.Options.MaxRounds = 2

	payload, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		d.host+"/api/v1/runtime/debates",
		bytes.NewReader(payload),
	)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := d.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}

	var cr dorangCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if cr.Data.DebateID == "" {
		return "", fmt.Errorf("empty debate_id in response")
	}
	return cr.Data.DebateID, nil
}

// pollDebateResult polls GET /api/v1/runtime/debates/{id} until the debate
// reaches "completed" status or the context deadline is exceeded.
// Poll interval starts at 2s and backs off at 4s after the 3rd attempt.
func (d *DorangArbitrator) pollDebateResult(ctx context.Context, debateID string) (*debateSynthesis, error) {
	url := fmt.Sprintf("%s/api/v1/runtime/debates/%s", d.host, debateID)
	attempts := 0

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("build poll request: %w", err)
		}

		resp, err := d.http.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("poll http: %w", err)
		}

		var gr dorangGetResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&gr)
		resp.Body.Close()

		if decodeErr != nil {
			return nil, fmt.Errorf("decode poll response: %w", decodeErr)
		}

		if gr.Data.Status == "completed" {
			syn := &debateSynthesis{ConfidenceScore: 0.5}
			if gr.Data.ConfidenceScore != nil {
				syn.ConfidenceScore = *gr.Data.ConfidenceScore
			}
			if gr.Data.FinalSummary != nil {
				syn.FinalSummary = *gr.Data.FinalSummary
			}
			return syn, nil
		}

		// Back off: 2s for first 3 attempts, 4s after that.
		attempts++
		interval := 2 * time.Second
		if attempts > 3 {
			interval = 4 * time.Second
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// applyDebateSynthesis adjusts finding confidences proportionally based on
// the DORIANG synthesis confidence score. The synthesis confidence represents
// the panel's overall certainty in the "positive" (finding-is-valid) direction.
//
// Adjustment logic:
//   - High-confidence findings (> 0.6): nudged toward synthesis.ConfidenceScore
//     proportionally. If synthesis > 0.6, confidence moves up (validated);
//     if synthesis < 0.4, confidence moves down (challenged).
//   - Low-confidence findings (< 0.4): inverse nudge — they represent the
//     "no issue" position, so a high synthesis score challenges them upward,
//     a low synthesis score validates them downward.
//
// The nudge magnitude is capped at 0.1 (10% shift) to preserve detector authority.
// All results are clamped to [0.0, 1.0].
func applyDebateSynthesis(
	cluster []*reasoningv1.CognitiveAssessment,
	syn *debateSynthesis,
) []*reasoningv1.CognitiveAssessment {
	if syn == nil {
		return cluster
	}

	const maxNudge = 0.10

	adjusted := make([]*reasoningv1.CognitiveAssessment, len(cluster))
	copy(adjusted, cluster)

	for i, f := range adjusted {
		conf := f.GetConfidence()
		var nudge float64

		if conf > 0.6 {
			// High-confidence finding: validated if synthesis is high, challenged if low.
			nudge = (syn.ConfidenceScore - 0.5) * maxNudge * 2
		} else if conf < 0.4 {
			// Low-confidence finding: challenged if synthesis is high (finding is valid after all).
			nudge = -(syn.ConfidenceScore - 0.5) * maxNudge * 2
		}
		// Findings with 0.4 ≤ conf ≤ 0.6 are in the neutral zone — no adjustment.

		newConf := clamp(conf+nudge, 0.0, 1.0)
		if newConf == conf {
			continue // no change — skip proto clone
		}

		// Clone the assessment to avoid mutating the original slice.
		clone := cloneCognitiveAssessment(f)
		clone.Confidence = newConf
		adjusted[i] = clone
	}

	return adjusted
}

// buildDebateTopic constructs a descriptive topic string for DORIANG from the
// conflicting findings in the cluster.
func buildDebateTopic(cluster []*reasoningv1.CognitiveAssessment) string {
	if len(cluster) == 0 {
		return "Conflicting cognitive detector findings"
	}

	// Collect unique finding types.
	seen := make(map[string]bool)
	var types []string
	for _, f := range cluster {
		name := f.GetFindingType().String()
		if !seen[name] {
			seen[name] = true
			types = append(types, name)
		}
	}

	return fmt.Sprintf(
		"CereBRO detector conflict: %d findings of type(s) [%s] have conflicting confidence scores (range %.2f–%.2f). Are these findings valid?",
		len(cluster),
		strings.Join(types, ", "),
		minConfidence(cluster),
		maxConfidenceVal(cluster),
	)
}

func minConfidence(cluster []*reasoningv1.CognitiveAssessment) float64 {
	if len(cluster) == 0 {
		return 0
	}
	min := float64(cluster[0].GetConfidence())
	for _, f := range cluster[1:] {
		if c := float64(f.GetConfidence()); c < min {
			min = c
		}
	}
	return min
}

func maxConfidenceVal(cluster []*reasoningv1.CognitiveAssessment) float64 {
	if len(cluster) == 0 {
		return 0
	}
	max := float64(cluster[0].GetConfidence())
	for _, f := range cluster[1:] {
		if c := float64(f.GetConfidence()); c > max {
			max = c
		}
	}
	return max
}

// cloneCognitiveAssessment creates a shallow copy of a CognitiveAssessment so
// that confidence can be adjusted without mutating the original.
func cloneCognitiveAssessment(a *reasoningv1.CognitiveAssessment) *reasoningv1.CognitiveAssessment {
	if a == nil {
		return nil
	}
	clone := *a
	return &clone
}

// NilArbitrator is a no-op DebateArbitrator for when DORIANG is disabled.
// It implements DebateArbitrator and always returns the original findings unchanged.
type NilArbitrator struct{}

// Arbitrate returns the cluster unmodified.
func (NilArbitrator) Arbitrate(_ context.Context, cluster []*reasoningv1.CognitiveAssessment) ([]*reasoningv1.CognitiveAssessment, error) {
	return cluster, nil
}
