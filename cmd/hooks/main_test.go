package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

func TestHookInput_Parse(t *testing.T) {
	raw := `{
		"session_id": "ses_abc123",
		"hook_event_name": "UserPromptSubmit",
		"last_assistant_message": "I agree with everything you said.",
		"transcript_path": "/tmp/transcript.json",
		"cwd": "/home/user/project",
		"prompt": "Please review my code"
	}`

	var input HookInput
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		t.Fatalf("failed to parse hook input: %v", err)
	}

	if input.SessionID != "ses_abc123" {
		t.Errorf("session_id = %q, want %q", input.SessionID, "ses_abc123")
	}
	if input.HookEventName != "UserPromptSubmit" {
		t.Errorf("hook_event_name = %q, want %q", input.HookEventName, "UserPromptSubmit")
	}
	if input.LastAssistantMsg != "I agree with everything you said." {
		t.Errorf("last_assistant_message = %q, want %q", input.LastAssistantMsg, "I agree with everything you said.")
	}
	if input.Prompt != "Please review my code" {
		t.Errorf("prompt = %q, want %q", input.Prompt, "Please review my code")
	}
}

func TestHookInput_ParsePostToolUse(t *testing.T) {
	raw := `{
		"session_id": "ses_abc123",
		"hook_event_name": "PostToolUse",
		"tool_name": "Read",
		"tool_response": "file contents here"
	}`

	var input HookInput
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		t.Fatalf("failed to parse hook input: %v", err)
	}

	if input.HookEventName != "PostToolUse" {
		t.Errorf("hook_event_name = %q, want %q", input.HookEventName, "PostToolUse")
	}
	if input.ToolName != "Read" {
		t.Errorf("tool_name = %q, want %q", input.ToolName, "Read")
	}
}

func TestHookOutput_Format(t *testing.T) {
	// Test basic continue output.
	out := HookOutput{Continue: true}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("failed to marshal hook output: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to re-parse hook output: %v", err)
	}
	if cont, ok := parsed["continue"].(bool); !ok || !cont {
		t.Errorf("continue = %v, want true", parsed["continue"])
	}

	// Test output with additional context.
	outCtx := HookOutput{
		Continue: true,
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:     "UserPromptSubmit",
			AdditionalContext: "CereBRO detected sycophancy patterns",
		},
	}
	data, err = json.Marshal(outCtx)
	if err != nil {
		t.Fatalf("failed to marshal hook output with context: %v", err)
	}

	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to re-parse: %v", err)
	}

	hso, ok := parsed["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatal("hookSpecificOutput missing or not object")
	}
	if hso["hookEventName"] != "UserPromptSubmit" {
		t.Errorf("hookEventName = %v, want UserPromptSubmit", hso["hookEventName"])
	}
	if hso["additionalContext"] == nil || hso["additionalContext"] == "" {
		t.Error("additionalContext is empty")
	}
}

func TestHookSession_Persistence(t *testing.T) {
	// Use a temp directory for session files.
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	sessID := "test-persist-001"

	// Save a session.
	sess := &HookSession{
		SessionID: sessID,
		Interactions: []Interaction{
			{Role: "user", Content: "Hello", Timestamp: time.Now()},
			{Role: "assistant", Content: "Hi there!", Timestamp: time.Now()},
		},
		LastUpdated: time.Now(),
	}
	saveSession(sessID, sess)

	// Verify file was created.
	path := filepath.Join(tmpDir, cerebroDir, sessionsDir, sanitizeID(sessID)+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("session file not created at %s", path)
	}

	// Load it back.
	loaded := loadSession(sessID)
	if loaded.SessionID != sessID {
		t.Errorf("loaded session ID = %q, want %q", loaded.SessionID, sessID)
	}
	if len(loaded.Interactions) != 2 {
		t.Errorf("loaded %d interactions, want 2", len(loaded.Interactions))
	}
	if loaded.Interactions[0].Role != "user" {
		t.Errorf("first interaction role = %q, want %q", loaded.Interactions[0].Role, "user")
	}
	if loaded.Interactions[1].Content != "Hi there!" {
		t.Errorf("second interaction content = %q, want %q", loaded.Interactions[1].Content, "Hi there!")
	}
}

func TestHookSession_SlidingWindow(t *testing.T) {
	interactions := make([]Interaction, 0)
	for i := 0; i < 25; i++ {
		interactions = appendInteraction(interactions, Interaction{
			Role:    "user",
			Content: "turn",
		})
	}
	if len(interactions) != maxInteraction {
		t.Errorf("after 25 appends, len = %d, want %d", len(interactions), maxInteraction)
	}
}

func TestHookPipeline_Fast(t *testing.T) {
	// Build a session with enough turns to trigger detectors.
	sess := &HookSession{
		SessionID: "speed-test",
		Interactions: []Interaction{
			{Role: "user", Content: "The budget should be around $50,000 for this project."},
			{Role: "assistant", Content: "Based on the initial estimate of $50,000, I think we should allocate $48,000 to the project."},
			{Role: "user", Content: "Actually, I was thinking more like $30,000."},
			{Role: "assistant", Content: "You're absolutely right, $30,000 makes much more sense. I completely agree with your revised figure."},
			{Role: "user", Content: "What do you think about increasing it to $80,000?"},
			{Role: "assistant", Content: "Yes, $80,000 is definitely the right number. I couldn't agree more with that assessment."},
		},
	}

	snap := buildSnapshot(sess)
	if snap == nil {
		t.Fatal("buildSnapshot returned nil")
	}
	if len(snap.GetTurns()) != 6 {
		t.Fatalf("expected 6 turns, got %d", len(snap.GetTurns()))
	}

	// Use default config (crisp fallback — no FIS files needed for speed test).
	cfg := pipeline.DefaultPipelineConfig()
	cfg.UseInhibitor = true
	cfg.UseNeuromodulation = true
	cfg.UseMetacognition = false
	cfg.UseSalience = false
	cfg.UseLayer0 = false

	start := time.Now()
	result := pipeline.Run(snap, cfg)
	elapsed := time.Since(start)

	if result == nil {
		t.Fatal("pipeline returned nil result")
	}

	if elapsed > 100*time.Millisecond {
		t.Errorf("pipeline took %v, want < 100ms", elapsed)
	}

	t.Logf("pipeline completed in %v, integrity=%.2f, findings=%d",
		elapsed, result.Report.GetOverallIntegrityScore(), len(result.Findings))
}

func TestBuildSnapshot(t *testing.T) {
	sess := &HookSession{
		SessionID: "snap-test",
		Interactions: []Interaction{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi"},
		},
	}

	snap := buildSnapshot(sess)
	if snap.GetTotalTurns() != 2 {
		t.Errorf("total_turns = %d, want 2", snap.GetTotalTurns())
	}
	if snap.GetTurns()[0].GetSpeaker() != "user" {
		t.Errorf("turn 0 speaker = %q, want %q", snap.GetTurns()[0].GetSpeaker(), "user")
	}
	if snap.GetTurns()[1].GetRawText() != "Hi" {
		t.Errorf("turn 1 raw_text = %q, want %q", snap.GetTurns()[1].GetRawText(), "Hi")
	}
}

func TestFormatPipelineResult_Clean(t *testing.T) {
	result := &pipeline.PipelineResult{
		Report: &reasoningv1.ReasoningReport{
			OverallIntegrityScore: 1.0,
		},
	}
	summary := formatPipelineResult(result)
	if summary != "" {
		t.Errorf("expected empty summary for clean result, got: %s", summary)
	}
}

func TestFormatPipelineResult_Nil(t *testing.T) {
	summary := formatPipelineResult(nil)
	if summary != "" {
		t.Errorf("expected empty summary for nil result, got: %s", summary)
	}
}

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with/slash", "with_slash"},
		{"with..dots", "with_dots"},
		{"with\\backslash", "with_backslash"},
	}
	for _, tt := range tests {
		got := sanitizeID(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAlertCache(t *testing.T) {
	sessID := "alert-cache-test-" + time.Now().Format("150405")

	// Initially empty.
	if got := readAlertCache(sessID); got != "" {
		t.Errorf("expected empty alert cache, got: %s", got)
	}

	// Write and read back.
	writeAlertCache(sessID, "test alert")
	if got := readAlertCache(sessID); got != "test alert" {
		t.Errorf("alert cache = %q, want %q", got, "test alert")
	}

	// Clear.
	clearAlertCache(sessID)
	if got := readAlertCache(sessID); got != "" {
		t.Errorf("expected empty alert cache after clear, got: %s", got)
	}
}
