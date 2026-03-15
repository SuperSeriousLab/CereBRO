package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// sophrimRequest is the payload sent to Sophrim's /augment endpoint.
type sophrimRequest struct {
	Query string `json:"query"`
}

// sophrimResponse is the JSON structure returned by Sophrim's /augment endpoint.
type sophrimResponse struct {
	Augmented   bool `json:"augmented"`
	DomainHints *struct {
		PrimaryDomain  string   `json:"primary_domain"`
		SourceProjects []string `json:"source_projects"`
		TextEra        string   `json:"text_era"`
		Confidence     float64  `json:"confidence"`
	} `json:"domain_hints"`
}

// SophrimClient fetches domain hints from Sophrim for a conversation.
type SophrimClient struct {
	endpoint string
	timeout  time.Duration
	http     *http.Client
}

// NewSophrimClient returns a SophrimClient configured for the given endpoint and timeout.
func NewSophrimClient(endpoint string, timeout time.Duration) *SophrimClient {
	return &SophrimClient{
		endpoint: endpoint,
		timeout:  timeout,
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

// FetchDomainContext calls Sophrim /augment with the conversation summary and
// returns a DomainContext derived from the domain_hints in the response.
// Returns nil on any error (unreachable, timeout, no hints) — advisory, never blocking.
// A nil ctx is treated as context.Background().
func (c *SophrimClient) FetchDomainContext(ctx context.Context, summary string) *DomainContext {
	if ctx == nil {
		ctx = context.Background()
	}
	if summary == "" {
		return nil
	}

	payload, err := json.Marshal(sophrimRequest{Query: summary})
	if err != nil {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/augment", bytes.NewReader(payload))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}

	var sr sophrimResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil
	}

	if !sr.Augmented || sr.DomainHints == nil {
		return nil
	}

	return &DomainContext{
		PrimaryDomain: sr.DomainHints.PrimaryDomain,
		TextEra:       sr.DomainHints.TextEra,
		Confidence:    sr.DomainHints.Confidence,
	}
}

// conversationSummary extracts a text summary from a ConversationSnapshot suitable
// for passing to Sophrim's /augment endpoint.
//
// Strategy:
//  1. If the snapshot has a non-empty Objective, use it directly.
//  2. Otherwise, concatenate the RawText of the first 3 turns, truncated to 500 chars.
func conversationSummary(snap *reasoningv1.ConversationSnapshot) string {
	if snap == nil {
		return ""
	}

	if obj := strings.TrimSpace(snap.GetObjective()); obj != "" {
		return obj
	}

	var sb strings.Builder
	for i, turn := range snap.GetTurns() {
		if i >= 3 {
			break
		}
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(strings.TrimSpace(turn.GetRawText()))
	}

	summary := sb.String()
	if len(summary) > 500 {
		summary = summary[:500]
	}
	return summary
}
