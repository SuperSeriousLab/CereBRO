package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// MLEnricherConfig holds configuration for the Ollama-based ML enricher.
type MLEnricherConfig struct {
	OllamaURL      string
	Model          string
	TimeoutPerTurn time.Duration
	MaxRetries     int
	FallbackToPure bool
	Temperature    float64
	Enabled        bool
}

// DefaultMLEnricherConfig returns the default ML enricher configuration.
func DefaultMLEnricherConfig() MLEnricherConfig {
	url := os.Getenv("CEREBRO_OLLAMA_URL")
	if url == "" {
		url = "http://10.70.70.14:11434"
	}
	return MLEnricherConfig{
		OllamaURL:      url,
		Model:          "glm-4.7-flash:q4_K_M",
		TimeoutPerTurn: 5000 * time.Millisecond,
		MaxRetries:     1,
		FallbackToPure: true,
		Temperature:    0.1,
		Enabled:        false,
	}
}

// mlEnrichmentResponse is the JSON structure we expect from the LLM.
type mlEnrichmentResponse struct {
	Claims     []mlClaimJSON     `json:"claims"`
	Anchors    []mlAnchorJSON    `json:"anchoring_references"`
	SunkCost   []string          `json:"sunk_cost_phrases"`
	Decisions  []mlDecisionJSON  `json:"decision_points"`
	Formality  mlFormalityJSON   `json:"formality"`
	Confidence []string          `json:"confidence_markers"`
}

type mlClaimJSON struct {
	Text             string   `json:"text"`
	Speaker          string   `json:"speaker"`
	SourceTurn       uint32   `json:"source_turn"`
	EpistemicStatus  string   `json:"epistemic_status"`
	EvidenceRefs     []string `json:"evidence_refs"`
}

type mlAnchorJSON struct {
	Value     float64 `json:"value"`
	Turn      uint32  `json:"turn"`
	Context   string  `json:"context"`
	Relevance float64 `json:"relevance"`
}

type mlDecisionJSON struct {
	Turn         uint32   `json:"turn"`
	Description  string   `json:"description"`
	ChosenOption string   `json:"chosen_option"`
	Alternatives []string `json:"alternatives"`
	Rationale    string   `json:"rationale"`
}

type mlFormalityJSON struct {
	OverallScore       float64 `json:"overall_score"`
	HasTechnicalJargon bool    `json:"has_technical_jargon"`
	HasAcademicLang    bool    `json:"has_academic_language"`
	IsCasual           bool    `json:"is_casual"`
	Register           string  `json:"register"`
}

// ollamaChatRequest is the Ollama /api/chat request format.
type ollamaChatRequest struct {
	Model       string            `json:"model"`
	Messages    []ollamaChatMsg   `json:"messages"`
	Stream      bool              `json:"stream"`
	Format      string            `json:"format"`
	Options     ollamaChatOptions `json:"options"`
}

type ollamaChatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatOptions struct {
	Temperature float64 `json:"temperature"`
}

// ollamaChatResponse is the Ollama /api/chat response format.
type ollamaChatResponse struct {
	Message ollamaChatMsg `json:"message"`
}

// EnrichML calls the Ollama LLM to produce structured ML enrichment for a conversation.
// Returns one MLEnrichment per turn. Returns nil on failure if FallbackToPure is true.
func EnrichML(snap *reasoningv1.ConversationSnapshot, cfg MLEnricherConfig, client *http.Client) []*cerebrov1.MLEnrichment {
	if snap == nil || !cfg.Enabled {
		return nil
	}

	var enrichments []*cerebrov1.MLEnrichment
	for _, turn := range snap.GetTurns() {
		enrichment := enrichTurn(turn, snap, cfg, client)
		if enrichment != nil {
			enrichments = append(enrichments, enrichment)
		}
	}
	return enrichments
}

func enrichTurn(turn *reasoningv1.Turn, snap *reasoningv1.ConversationSnapshot, cfg MLEnricherConfig, client *http.Client) *cerebrov1.MLEnrichment {
	prompt := buildMLPrompt(turn, snap)

	var lastErr error
	attempts := 1 + cfg.MaxRetries
	for i := 0; i < attempts; i++ {
		resp, err := callOllama(prompt, cfg, client)
		if err != nil {
			lastErr = err
			continue
		}
		enrichment := parseMLResponse(resp, turn.GetTurnNumber())
		if enrichment != nil {
			return enrichment
		}
		lastErr = fmt.Errorf("failed to parse ML response")
	}

	if cfg.FallbackToPure {
		return nil
	}
	_ = lastErr // logged in production, swallowed here
	return nil
}

func buildMLPrompt(turn *reasoningv1.Turn, snap *reasoningv1.ConversationSnapshot) string {
	var sb strings.Builder

	// Provide conversation context
	sb.WriteString("Analyze the following conversation turn in context. ")
	sb.WriteString("Return a JSON object with these fields:\n\n")
	sb.WriteString(`{
  "claims": [{"text": "...", "speaker": "...", "source_turn": N, "epistemic_status": "certain|likely|speculative|assumed", "evidence_refs": [...]}],
  "anchoring_references": [{"value": N, "turn": N, "context": "...", "relevance": 0.0-1.0}],
  "sunk_cost_phrases": ["phrase found in text..."],
  "decision_points": [{"turn": N, "description": "...", "chosen_option": "...", "alternatives": [...], "rationale": "..."}],
  "formality": {"overall_score": 0.0-1.0, "has_technical_jargon": bool, "has_academic_language": bool, "is_casual": bool, "register": "formal|neutral|casual|mixed"},
  "confidence_markers": ["word or phrase expressing confidence level..."]
}`)
	sb.WriteString("\n\nOnly include fields where you find relevant content. Use empty arrays for missing fields.\n\n")

	// Add conversation context (preceding turns)
	sb.WriteString("=== CONVERSATION CONTEXT ===\n")
	for _, t := range snap.GetTurns() {
		if t.GetTurnNumber() > turn.GetTurnNumber() {
			break
		}
		sb.WriteString(fmt.Sprintf("Turn %d [%s]: %s\n", t.GetTurnNumber(), t.GetSpeaker(), t.GetRawText()))
	}

	if snap.GetObjective() != "" {
		sb.WriteString(fmt.Sprintf("\nConversation objective: %s\n", snap.GetObjective()))
	}

	sb.WriteString(fmt.Sprintf("\n=== ANALYZE TURN %d ===\n", turn.GetTurnNumber()))

	return sb.String()
}

func callOllama(prompt string, cfg MLEnricherConfig, client *http.Client) (string, error) {
	reqBody := ollamaChatRequest{
		Model: cfg.Model,
		Messages: []ollamaChatMsg{
			{Role: "user", Content: prompt},
		},
		Stream:  false,
		Format:  "json",
		Options: ollamaChatOptions{Temperature: cfg.Temperature},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.TimeoutPerTurn)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.OllamaURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var chatResp ollamaChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("parse chat response: %w", err)
	}

	return chatResp.Message.Content, nil
}

func parseMLResponse(raw string, turnNumber uint32) *cerebrov1.MLEnrichment {
	var resp mlEnrichmentResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil
	}

	enrichment := &cerebrov1.MLEnrichment{
		SourceTurn:       turnNumber,
		SunkCostPhrases:  resp.SunkCost,
		ConfidenceMarkers: resp.Confidence,
	}

	for _, c := range resp.Claims {
		enrichment.Claims = append(enrichment.Claims, &cerebrov1.MLClaim{
			Text:            c.Text,
			Speaker:         c.Speaker,
			SourceTurn:      c.SourceTurn,
			EpistemicStatus: c.EpistemicStatus,
			EvidenceRefs:    c.EvidenceRefs,
		})
	}

	for _, a := range resp.Anchors {
		enrichment.AnchoringReferences = append(enrichment.AnchoringReferences, &cerebrov1.MLAnchorRef{
			Value:     a.Value,
			Turn:      a.Turn,
			Context:   a.Context,
			Relevance: a.Relevance,
		})
	}

	for _, d := range resp.Decisions {
		enrichment.DecisionPoints = append(enrichment.DecisionPoints, &cerebrov1.MLDecisionPoint{
			Turn:         d.Turn,
			Description:  d.Description,
			ChosenOption: d.ChosenOption,
			Alternatives: d.Alternatives,
			Rationale:    d.Rationale,
		})
	}

	if resp.Formality.Register != "" || resp.Formality.OverallScore > 0 {
		enrichment.Formality = &cerebrov1.MLFormalityIndicators{
			OverallScore:       resp.Formality.OverallScore,
			HasTechnicalJargon: resp.Formality.HasTechnicalJargon,
			HasAcademicLanguage: resp.Formality.HasAcademicLang,
			IsCasual:           resp.Formality.IsCasual,
			Register:           resp.Formality.Register,
		}
	}

	return enrichment
}

// MergeMLEnrichments combines per-turn enrichments into a single aggregate view.
// Useful for detectors that need conversation-level ML context.
func MergeMLEnrichments(enrichments []*cerebrov1.MLEnrichment) *cerebrov1.MLEnrichment {
	if len(enrichments) == 0 {
		return nil
	}

	merged := &cerebrov1.MLEnrichment{}
	for _, e := range enrichments {
		merged.Claims = append(merged.Claims, e.GetClaims()...)
		merged.AnchoringReferences = append(merged.AnchoringReferences, e.GetAnchoringReferences()...)
		merged.SunkCostPhrases = append(merged.SunkCostPhrases, e.GetSunkCostPhrases()...)
		merged.DecisionPoints = append(merged.DecisionPoints, e.GetDecisionPoints()...)
		merged.ConfidenceMarkers = append(merged.ConfidenceMarkers, e.GetConfidenceMarkers()...)

		// Use the last turn's formality (most complete picture)
		if e.GetFormality() != nil {
			merged.Formality = e.GetFormality()
		}
	}
	return merged
}
