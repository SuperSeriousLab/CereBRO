// verify_findings.go — Stage 12: post-pipeline finding verification via SLR/Grok.
//
// After the pipeline produces findings, VerifyFindings calls SLR with a Grok
// model to double-check each finding against the conversation text.  The SLR
// response ID is then used to send outcome feedback back to SLR so it can
// train its routing model.
//
// This is fire-and-forget: the function launches a goroutine and returns
// immediately.  Errors are logged and never surface to the caller.
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

const (
	// verifyModel is the SLR model hint used for finding verification.
	// We want Grok (reasoning tier) for accurate cross-checking.
	verifyModel = "grok"

	// verifyTimeout is the per-finding SLR call budget.
	verifyTimeout = 20 * time.Second

	// feedbackTimeout is the budget for the SLR /v1/feedback POST.
	feedbackTimeout = 5 * time.Second
)

// slrChatRequest is the OpenAI-compatible request sent to SLR.
type slrChatRequest struct {
	Model    string        `json:"model"`
	Messages []slrChatMsg  `json:"messages"`
}

type slrChatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// slrChatResponse is the OpenAI-compatible response from SLR.
type slrChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// slrFeedbackRequest is the payload sent to SLR /v1/feedback.
type slrFeedbackRequest struct {
	RequestID    string  `json:"request_id"`
	Outcome      string  `json:"outcome"`       // "success" or "failure"
	QualityScore float64 `json:"quality_score"` // 0.0–1.0
}

// VerifyFindingsConfig holds the configuration for verify-findings.
type VerifyFindingsConfig struct {
	// SLREndpoint is the base URL of the SLR gateway, e.g. "http://192.168.14.69:8081".
	// When empty the stage is a no-op.
	SLREndpoint string
}

// VerifyFindings launches a goroutine that verifies each finding via SLR/Grok
// and then posts outcome feedback to SLR /v1/feedback.
// Returns immediately — never blocks the pipeline.
func VerifyFindings(
	findings []*reasoningv1.CognitiveAssessment,
	snap *reasoningv1.ConversationSnapshot,
	cfg VerifyFindingsConfig,
) {
	if cfg.SLREndpoint == "" || len(findings) == 0 {
		return
	}

	// Snapshot the text once to pass into the goroutine.
	convText := conversationText(snap)

	go func() {
		httpClient := &http.Client{}
		for _, finding := range findings {
			verifyFinding(finding, convText, cfg.SLREndpoint, httpClient)
		}
	}()
}

// verifyFinding calls SLR to verify a single finding and posts outcome feedback.
func verifyFinding(
	finding *reasoningv1.CognitiveAssessment,
	convText string,
	slrEndpoint string,
	httpClient *http.Client,
) {
	prompt := buildVerifyPrompt(finding, convText)

	// --- Step 1: call SLR to verify the finding ---
	requestID, confirmed, confidence, err := callSLRVerify(slrEndpoint, prompt, httpClient)
	if err != nil {
		log.Printf("[verify-findings] SLR call failed for %s: %v", finding.GetDetectorName(), err)
		return
	}

	// --- Step 2: map Grok's verdict to outcome + quality_score ---
	outcome := "success"
	qualityScore := confidence
	if !confirmed {
		outcome = "failure"
		qualityScore = 0.0
	}

	// --- Step 3: POST outcome feedback to SLR ---
	if err := postSLRFeedback(slrEndpoint, requestID, outcome, qualityScore, httpClient); err != nil {
		log.Printf("[verify-findings] feedback POST failed for %s (req=%s): %v",
			finding.GetDetectorName(), requestID, err)
	}
}

// callSLRVerify sends the verification prompt to SLR and parses the response.
// Returns: requestID, confirmed, confidence, error.
func callSLRVerify(endpoint, prompt string, client *http.Client) (string, bool, float64, error) {
	reqBody := slrChatRequest{
		Model: verifyModel,
		Messages: []slrChatMsg{
			{Role: "user", Content: prompt},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", false, 0, fmt.Errorf("marshal: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), verifyTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		endpoint+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", false, 0, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return "", false, 0, fmt.Errorf("HTTP: %w", err)
	}
	defer httpResp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		return "", false, 0, fmt.Errorf("read body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return "", false, 0, fmt.Errorf("SLR status %d: %s", httpResp.StatusCode, string(respBytes))
	}

	var slrResp slrChatResponse
	if err := json.Unmarshal(respBytes, &slrResp); err != nil {
		return "", false, 0, fmt.Errorf("parse response: %w", err)
	}

	if len(slrResp.Choices) == 0 {
		return "", false, 0, fmt.Errorf("empty choices from SLR")
	}

	content := slrResp.Choices[0].Message.Content
	confirmed, confidence := parseVerifyResponse(content)

	return slrResp.ID, confirmed, confidence, nil
}

// parseVerifyResponse interprets the LLM response to determine if the finding
// is confirmed and extracts a confidence score.
//
// The LLM is asked to reply with "CONFIRMED:<score>" or "REJECTED:<score>" where
// score is a float in 0.0–1.0.  Any other format is treated as REJECTED:0.0.
func parseVerifyResponse(content string) (confirmed bool, confidence float64) {
	upper := strings.ToUpper(strings.TrimSpace(content))

	if strings.HasPrefix(upper, "CONFIRMED") {
		confirmed = true
		confidence = extractScore(upper)
	} else if strings.HasPrefix(upper, "REJECTED") {
		confirmed = false
		confidence = extractScore(upper)
	} else {
		// Fallback: look for "yes" / "correct" keywords as soft confirmation.
		if strings.Contains(upper, "YES") || strings.Contains(upper, "CORRECT") ||
			strings.Contains(upper, "CONFIRMED") {
			confirmed = true
			confidence = 0.6
		} else {
			confirmed = false
			confidence = 0.0
		}
	}

	// Clamp.
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	return
}

// extractScore parses the numeric score from a string like "CONFIRMED:0.87".
func extractScore(s string) float64 {
	idx := strings.Index(s, ":")
	if idx < 0 || idx+1 >= len(s) {
		return 0.7 // default when score is absent but verdict is present
	}
	scoreStr := strings.TrimSpace(s[idx+1:])
	// Take only up to first whitespace/newline.
	if i := strings.IndexAny(scoreStr, " \t\n\r"); i >= 0 {
		scoreStr = scoreStr[:i]
	}
	var f float64
	if _, err := fmt.Sscanf(scoreStr, "%f", &f); err != nil {
		return 0.7
	}
	return f
}

// postSLRFeedback sends the outcome feedback to SLR /v1/feedback.
func postSLRFeedback(endpoint, requestID, outcome string, qualityScore float64, client *http.Client) error {
	body := slrFeedbackRequest{
		RequestID:    requestID,
		Outcome:      outcome,
		QualityScore: qualityScore,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), feedbackTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		endpoint+"/v1/feedback", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("SLR feedback returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// buildVerifyPrompt constructs the verification prompt for a single finding.
func buildVerifyPrompt(finding *reasoningv1.CognitiveAssessment, convText string) string {
	var sb strings.Builder

	sb.WriteString("You are verifying a cognitive-bias finding produced by an automated reasoning detector.\n\n")
	sb.WriteString("Finding details:\n")
	sb.WriteString(fmt.Sprintf("  Type:       %s\n", finding.GetFindingType().String()))
	sb.WriteString(fmt.Sprintf("  Detector:   %s\n", finding.GetDetectorName()))
	sb.WriteString(fmt.Sprintf("  Severity:   %s\n", finding.GetSeverity().String()))
	sb.WriteString(fmt.Sprintf("  Confidence: %.2f\n", finding.GetConfidence()))
	sb.WriteString(fmt.Sprintf("  Explanation: %s\n", finding.GetExplanation()))

	if len(finding.GetRelevantTurns()) > 0 {
		turns := make([]string, len(finding.GetRelevantTurns()))
		for i, t := range finding.GetRelevantTurns() {
			turns[i] = fmt.Sprintf("%d", t)
		}
		sb.WriteString(fmt.Sprintf("  Relevant turns: %s\n", strings.Join(turns, ", ")))
	}

	sb.WriteString("\nConversation transcript:\n")
	sb.WriteString(convText)

	sb.WriteString("\n\nDoes the conversation evidence actually support this finding?\n")
	sb.WriteString("Reply ONLY with one of these two formats:\n")
	sb.WriteString("  CONFIRMED:<confidence>   — e.g. CONFIRMED:0.92\n")
	sb.WriteString("  REJECTED:<confidence>    — e.g. REJECTED:0.85\n")
	sb.WriteString("Where <confidence> is your certainty in your verdict (0.0–1.0).\n")
	sb.WriteString("No other text.\n")

	return sb.String()
}

// conversationText builds a plain-text transcript of the conversation snapshot.
func conversationText(snap *reasoningv1.ConversationSnapshot) string {
	if snap == nil {
		return ""
	}
	var sb strings.Builder
	for _, t := range snap.GetTurns() {
		sb.WriteString(fmt.Sprintf("Turn %d [%s]: %s\n", t.GetTurnNumber(), t.GetSpeaker(), t.GetRawText()))
	}
	return sb.String()
}
