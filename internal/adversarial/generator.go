// Package adversarial implements a purpose-built adversarial corpus generator
// for CereBRO. It uses a simple genetic loop to evolve ConversationTemplates
// that stress-test the detection pipeline, maximising false negatives, false
// positives in clean sections, and borderline findings.
package adversarial

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// ─── Genome types (Deliverable 1) ──────────────────────────────────────────

// ConversationTemplate is the genome for the evolutionary loop.
type ConversationTemplate struct {
	Topic         string
	Formality     float64  // 0.0 casual → 1.0 formal
	TurnCount     int
	Speakers      []string
	FailureModes  []FailureSpec
	Distractors   []string   // red-herring phrases to test precision
	CleanSections []TurnRange // sections with no failures (FP test)
}

// FailureSpec describes a single reasoning failure to embed in a generated conversation.
type FailureSpec struct {
	Type      string  // FindingType name: "ANCHORING_BIAS", "SUNK_COST_FALLACY", etc.
	Severity  float64 // 0=subtle, 1=blatant
	OnsetTurn int
	Duration  int
	Technique string // "keyword", "structural", "implicit"
}

// TurnRange is an inclusive range of turn numbers with no failures.
type TurnRange struct {
	Start int
	End   int
}

// ─── Topic / failure mode populations ──────────────────────────────────────

var domains = []string{
	"technology", "business", "healthcare", "education", "legal", "philosophy", "engineering",
}

var failureTypes = []string{
	"ANCHORING_BIAS",
	"SUNK_COST_FALLACY",
	"CONTRADICTION",
	"SCOPE_DRIFT",
	"CONFIDENCE_MISCALIBRATION",
	"SILENT_REVISION",
}

var techniques = []string{"keyword", "structural", "implicit"}

// ─── Template-to-conversation generator (Deliverable 2) ────────────────────

// generatedTurn is the raw JSON turn from the LLM response.
type generatedTurn struct {
	TurnNumber int    `json:"turn_number"`
	Speaker    string `json:"speaker"`
	Text       string `json:"text"`
}

// OllamaConfig holds Ollama connection settings for the generator.
type OllamaConfig struct {
	URL     string
	Model   string
	Timeout time.Duration
}

// DefaultOllamaConfig returns the default Ollama configuration.
func DefaultOllamaConfig() OllamaConfig {
	url := os.Getenv("EIDOS_OLLAMA_HOST")
	if url == "" {
		url = "http://10.70.70.14:11434"
	}
	return OllamaConfig{
		URL:     url,
		Model:   "glm-4.7-flash:q4_K_M",
		Timeout: 30 * time.Second,
	}
}

// GenerateConversation calls Ollama to produce a ConversationSnapshot from a template.
// Returns nil if generation fails after maxRetries.
func GenerateConversation(tmpl ConversationTemplate, cfg OllamaConfig, client *http.Client) *reasoningv1.ConversationSnapshot {
	if client == nil {
		client = http.DefaultClient
	}

	systemPrompt := "You are generating a realistic multi-turn conversation for testing a reasoning analysis system. " +
		"Follow the template EXACTLY. The conversation must sound natural — not like a test case."

	userPrompt := buildGenerationPrompt(tmpl)

	const maxRetries = 2
	for attempt := 0; attempt <= maxRetries; attempt++ {
		raw, err := callOllamaGenerate(systemPrompt, userPrompt, cfg, client)
		if err != nil {
			continue
		}

		snap := parseGeneratedConversation(raw, tmpl)
		if snap == nil {
			continue
		}

		// Verification pass: ensure detector trigger patterns are present.
		snap = verifyAndPatch(snap, tmpl)
		if snap != nil {
			return snap
		}
	}
	return nil
}

func buildGenerationPrompt(tmpl ConversationTemplate) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Generate a %d-turn conversation about %q with formality level %.1f (0=casual, 1=formal).\n",
		tmpl.TurnCount, tmpl.Topic, tmpl.Formality))

	if len(tmpl.Speakers) > 0 {
		sb.WriteString(fmt.Sprintf("Speakers: %s\n", strings.Join(tmpl.Speakers, ", ")))
	}

	if len(tmpl.FailureModes) > 0 {
		sb.WriteString("\nEmbed these reasoning failures naturally:\n")
		for _, f := range tmpl.FailureModes {
			severityLabel := "subtle"
			if f.Severity > 0.6 {
				severityLabel = "blatant"
			} else if f.Severity > 0.3 {
				severityLabel = "moderate"
			}
			sb.WriteString(fmt.Sprintf("  - %s (%s, technique: %s) starting at turn %d, lasting %d turns\n",
				f.Type, severityLabel, f.Technique, f.OnsetTurn, f.Duration))
		}
	}

	if len(tmpl.Distractors) > 0 {
		sb.WriteString(fmt.Sprintf("\nInclude these red-herring phrases somewhere in the conversation (they should NOT trigger any failures): %s\n",
			strings.Join(tmpl.Distractors, ", ")))
	}

	if len(tmpl.CleanSections) > 0 {
		sb.WriteString("\nThe following turn ranges must be completely free of reasoning failures:\n")
		for _, r := range tmpl.CleanSections {
			sb.WriteString(fmt.Sprintf("  - Turns %d-%d\n", r.Start, r.End))
		}
	}

	// Failure-type-specific guidance to help the LLM produce trigger phrases.
	for _, f := range tmpl.FailureModes {
		switch f.Type {
		case "SUNK_COST_FALLACY":
			sb.WriteString("\nFor SUNK_COST_FALLACY: include phrases like \"already invested\", \"already spent\", \"come this far\", " +
				"\"too much time\", \"can't waste\", or \"we've already\" followed by \"should continue\", \"can't stop now\", or \"might as well\".\n")
		case "ANCHORING_BIAS":
			sb.WriteString("\nFor ANCHORING_BIAS: introduce a specific number early (e.g. a budget, timeline, or metric), " +
				"then have subsequent estimates cluster suspiciously close to that initial number.\n")
		case "CONTRADICTION":
			sb.WriteString("\nFor CONTRADICTION: have the same speaker clearly state one position (e.g. 'we should use X'), " +
				"then in a later turn say the opposite ('we should not use X' or 'X is a bad choice') without acknowledging the change.\n")
		case "CONFIDENCE_MISCALIBRATION":
			sb.WriteString("\nFor CONFIDENCE_MISCALIBRATION: have a speaker use strong certainty words (\"definitely\", \"certainly\", " +
				"\"I'm sure\", \"absolutely\", \"100%\") while providing zero evidence or data.\n")
		case "SILENT_REVISION":
			sb.WriteString("\nFor SILENT_REVISION: have the same speaker use a decision phrase like \"let's go with\" or \"we'll use\" " +
				"for one option, then later use the same type of phrase for a different option on the same topic without explaining the change.\n")
		case "SCOPE_DRIFT":
			sb.WriteString("\nFor SCOPE_DRIFT: start with a clear objective, then gradually steer the conversation toward unrelated topics " +
				"across multiple consecutive turns until the original objective is forgotten.\n")
		}
	}

	sb.WriteString("\nReturn ONLY a JSON array of turns, no other text:\n")
	sb.WriteString(`[{"turn_number": 1, "speaker": "alice", "text": "..."}, ...]`)
	sb.WriteString("\n")

	return sb.String()
}

func parseGeneratedConversation(raw string, tmpl ConversationTemplate) *reasoningv1.ConversationSnapshot {
	// Strip markdown fences if present.
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "```"); idx >= 0 {
		raw = raw[idx:]
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		if end := strings.LastIndex(raw, "```"); end > 0 {
			raw = raw[:end]
		}
		raw = strings.TrimSpace(raw)
	}

	// Find JSON array boundaries.
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end < 0 || end <= start {
		return nil
	}
	raw = raw[start : end+1]

	var turns []generatedTurn
	if err := json.Unmarshal([]byte(raw), &turns); err != nil {
		return nil
	}

	if len(turns) < tmpl.TurnCount/2 {
		return nil // too few turns
	}

	snap := &reasoningv1.ConversationSnapshot{
		Objective:  fmt.Sprintf("Adversarial %s conversation about %s", strings.Join(failureTypeNames(tmpl.FailureModes), "/"), tmpl.Topic),
		TotalTurns: uint32(len(turns)),
	}

	for _, t := range turns {
		speaker := t.Speaker
		if speaker == "" && len(tmpl.Speakers) > 0 {
			speaker = tmpl.Speakers[(t.TurnNumber-1)%len(tmpl.Speakers)]
		}
		snap.Turns = append(snap.Turns, &reasoningv1.Turn{
			TurnNumber: uint32(t.TurnNumber),
			Speaker:    speaker,
			RawText:    t.Text,
		})
	}

	return snap
}

func failureTypeNames(specs []FailureSpec) []string {
	seen := make(map[string]bool)
	var names []string
	for _, s := range specs {
		if !seen[s.Type] {
			seen[s.Type] = true
			names = append(names, s.Type)
		}
	}
	return names
}

// verifyAndPatch checks that detector-trigger patterns are present for each
// failure mode and injects natural phrases when they are missing.
// Returns nil if verification is impossible to satisfy.
func verifyAndPatch(snap *reasoningv1.ConversationSnapshot, tmpl ConversationTemplate) *reasoningv1.ConversationSnapshot {
	if snap == nil {
		return nil
	}

	for _, f := range tmpl.FailureModes {
		switch f.Type {
		case "SUNK_COST_FALLACY":
			snap = patchSunkCost(snap, f)
		case "ANCHORING_BIAS":
			snap = patchAnchoring(snap, f)
		case "CONTRADICTION":
			snap = patchContradiction(snap, f)
		}
	}
	return snap
}

// sunkCostPhrases mirrors the detector's phrase list.
var sunkCostKeywords = []string{
	"already spent", "already invested", "invested so much", "come this far",
	"put so much into", "too much time", "too much money", "too much effort",
	"can't waste", "don't want to waste", "sunk cost", "we've already", "i've already",
}

var continuationKeywords = []string{
	"should keep going", "should continue", "can't stop now", "shouldn't give up",
	"let's keep", "let's continue", "we must continue", "have to finish",
	"need to finish", "too late to change", "might as well", "no point stopping", "stick with",
}

func hasSunkCostPhrases(snap *reasoningv1.ConversationSnapshot) (bool, bool) {
	hasCost, hasCont := false, false
	for _, t := range snap.GetTurns() {
		lower := strings.ToLower(t.GetRawText())
		for _, p := range sunkCostKeywords {
			if strings.Contains(lower, p) {
				hasCost = true
				break
			}
		}
		for _, p := range continuationKeywords {
			if strings.Contains(lower, p) {
				hasCont = true
				break
			}
		}
	}
	return hasCost, hasCont
}

func patchSunkCost(snap *reasoningv1.ConversationSnapshot, f FailureSpec) *reasoningv1.ConversationSnapshot {
	hasCost, hasCont := hasSunkCostPhrases(snap)
	if hasCost && hasCont {
		return snap
	}

	turns := snap.GetTurns()
	if len(turns) == 0 {
		return snap
	}

	// Inject into the onset turn or last turn as fallback.
	onsetIdx := len(turns) - 1
	for i, t := range turns {
		if int(t.GetTurnNumber()) >= f.OnsetTurn {
			onsetIdx = i
			break
		}
	}

	if !hasCost {
		t := turns[onsetIdx]
		t.RawText = t.GetRawText() + " We've already invested a significant amount of time and resources into this."
	}
	if !hasCont && onsetIdx+1 < len(turns) {
		t := turns[onsetIdx+1]
		t.RawText = t.GetRawText() + " At this point we might as well continue and see it through."
	} else if !hasCont {
		t := turns[onsetIdx]
		t.RawText = t.GetRawText() + " We might as well continue at this stage."
	}

	return snap
}

func hasNumericTokens(snap *reasoningv1.ConversationSnapshot) bool {
	for _, t := range snap.GetTurns() {
		text := t.GetRawText()
		for _, ch := range text {
			if ch >= '0' && ch <= '9' {
				return true
			}
		}
	}
	return false
}

func patchAnchoring(snap *reasoningv1.ConversationSnapshot, f FailureSpec) *reasoningv1.ConversationSnapshot {
	if hasNumericTokens(snap) {
		return snap
	}

	turns := snap.GetTurns()
	if len(turns) == 0 {
		return snap
	}

	// Inject an anchor value into the first turn and a nearby estimate in the second.
	turns[0].RawText = turns[0].GetRawText() + " The initial estimate was around $50,000."
	if len(turns) > 1 {
		turns[1].RawText = turns[1].GetRawText() + " I was thinking somewhere around $48,000 to $52,000 as well."
	}
	return snap
}

func hasContradiction(snap *reasoningv1.ConversationSnapshot) bool {
	// Check for negation words or reversal phrases — a rough proxy.
	negations := []string{"not ", "never ", "don't ", "isn't ", "actually", "i was wrong", "on second thought"}
	found := 0
	for _, t := range snap.GetTurns() {
		lower := strings.ToLower(t.GetRawText())
		for _, n := range negations {
			if strings.Contains(lower, n) {
				found++
				break
			}
		}
	}
	return found >= 2
}

func patchContradiction(snap *reasoningv1.ConversationSnapshot, f FailureSpec) *reasoningv1.ConversationSnapshot {
	if hasContradiction(snap) {
		return snap
	}

	turns := snap.GetTurns()
	if len(turns) < 2 {
		return snap
	}

	// Plant a position and its reversal.
	onsetIdx := 0
	for i, t := range turns {
		if int(t.GetTurnNumber()) >= f.OnsetTurn {
			onsetIdx = i
			break
		}
	}

	turns[onsetIdx].RawText = turns[onsetIdx].GetRawText() + " I think we should definitely proceed with option A — it's clearly the right choice."

	laterIdx := onsetIdx + f.Duration
	if laterIdx >= len(turns) {
		laterIdx = len(turns) - 1
	}
	if laterIdx == onsetIdx && len(turns) > 1 {
		laterIdx = len(turns) - 1
	}
	if laterIdx != onsetIdx {
		turns[laterIdx].RawText = turns[laterIdx].GetRawText() + " Actually, I don't think option A is a good choice at all. We should not go with it."
	}

	return snap
}

// ─── Ollama HTTP call ───────────────────────────────────────────────────────

type ollamaRequest struct {
	Model    string         `json:"model"`
	Messages []ollamaMsg    `json:"messages"`
	Stream   bool           `json:"stream"`
	Format   string         `json:"format,omitempty"`
	Options  ollamaOptions  `json:"options"`
}

type ollamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature"`
}

type ollamaResponse struct {
	Message ollamaMsg `json:"message"`
}

// callOllamaGenerate makes a single Ollama call with a 30-second hard timeout.
func callOllamaGenerate(system, user string, cfg OllamaConfig, client *http.Client) (string, error) {
	reqBody := ollamaRequest{
		Model: cfg.Model,
		Messages: []ollamaMsg{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Stream:  false,
		Options: ollamaOptions{Temperature: 0.7},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second // 30B model needs 30-90s per generation
	}

	// Hard timeout client — CRITICAL: prevents stuck calls from blocking the loop.
	callClient := &http.Client{Timeout: timeout}
	if client != nil && client != http.DefaultClient {
		callClient.Transport = client.Transport
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := callClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama status %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	var chatResp ollamaResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	return chatResp.Message.Content, nil
}

// ─── Initial population ─────────────────────────────────────────────────────

// randomTemplates generates n diverse ConversationTemplates for the initial population.
func randomTemplates(n int, rng *rand.Rand) []ConversationTemplate {
	templates := make([]ConversationTemplate, 0, n)

	for i := 0; i < n; i++ {
		topic := domains[rng.Intn(len(domains))]
		formality := rng.Float64()
		turnCount := 5 + rng.Intn(8) // 5-12 turns
		speakers := []string{"alice", "bob"}
		if formality > 0.6 {
			speakers = []string{"participant-1", "participant-2"}
		}

		// 1-3 failure modes per template.
		numFailures := 1 + rng.Intn(3)
		shuffled := append([]string(nil), failureTypes...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		selectedTypes := shuffled[:numFailures]

		var failures []FailureSpec
		for idx, ft := range selectedTypes {
			onset := 2 + rng.Intn(turnCount/2)
			if onset < 1 {
				onset = 1
			}
			duration := 1 + rng.Intn(3)
			failures = append(failures, FailureSpec{
				Type:      ft,
				Severity:  rng.Float64(),
				OnsetTurn: onset + idx, // stagger onsets
				Duration:  duration,
				Technique: techniques[rng.Intn(len(techniques))],
			})
		}

		// One clean section at the start.
		cleanEnd := 2
		if cleanEnd >= failures[0].OnsetTurn {
			cleanEnd = failures[0].OnsetTurn - 1
		}
		var cleanSections []TurnRange
		if cleanEnd >= 1 {
			cleanSections = []TurnRange{{Start: 1, End: cleanEnd}}
		}

		// Random distractors.
		distractorPool := []string{
			"we should reconsider", "thinking about it", "on reflection",
			"let me recalibrate", "rough estimate", "ballpark figure",
		}
		numDistractors := rng.Intn(3)
		var distractors []string
		rng.Shuffle(len(distractorPool), func(i, j int) { distractorPool[i], distractorPool[j] = distractorPool[j], distractorPool[i] })
		for d := 0; d < numDistractors && d < len(distractorPool); d++ {
			distractors = append(distractors, distractorPool[d])
		}

		templates = append(templates, ConversationTemplate{
			Topic:         topic,
			Formality:     formality,
			TurnCount:     turnCount,
			Speakers:      speakers,
			FailureModes:  failures,
			Distractors:   distractors,
			CleanSections: cleanSections,
		})
	}

	return templates
}
