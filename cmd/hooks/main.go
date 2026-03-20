// Package main implements a cerebro-hook binary for Claude Code.
//
// It reads hook JSON from stdin, runs CereBRO's fuzzy 5-layer pipeline on the
// accumulated session state, and outputs hook response JSON on stdout.
//
// Hook events: UserPromptSubmit, PostToolUse, Stop.
// Passive mode: always returns continue=true.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SuperSeriousLab/fugo"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

const (
	cerebroDir     = ".cerebro"
	sessionsDir    = "sessions"
	fisDir         = "config/fis"
	maxInteraction = 20 // sliding window
)

// HookInput is the envelope received from Claude Code hooks.
type HookInput struct {
	SessionID        string          `json:"session_id"`
	TranscriptPath   string          `json:"transcript_path"`
	CWD              string          `json:"cwd"`
	HookEventName    string          `json:"hook_event_name"`
	LastAssistantMsg string          `json:"last_assistant_message"`
	ToolName         string          `json:"tool_name"`
	ToolInput        json.RawMessage `json:"tool_input"`
	ToolResponse     string          `json:"tool_response"`
	Prompt           string          `json:"prompt"`
}

// HookOutput is the response written back to Claude Code.
type HookOutput struct {
	Continue           bool                `json:"continue"`
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

// HookSpecificOutput carries hook-type-specific fields.
type HookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// HookSession persists conversation state between hook invocations.
type HookSession struct {
	SessionID            string         `json:"session_id"`
	Interactions         []Interaction  `json:"interactions"`
	LastUpdated          time.Time      `json:"last_updated"`
	ReportedPathologies  map[string]int `json:"reported_pathologies,omitempty"` // finding_type → interaction count when last reported
	LastCWD              string         `json:"last_cwd,omitempty"`             // last observed working directory; used for context-change decay
}

// Interaction is a single turn in the conversation.
type Interaction struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

func main() {
	var input HookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		outputContinue()
		return
	}

	switch input.HookEventName {
	case "UserPromptSubmit":
		handleUserPromptSubmit(input)
	case "PostToolUse":
		handlePostToolUse(input)
	case "Stop":
		handleStop(input)
	default:
		outputContinue()
	}
}

func handleUserPromptSubmit(input HookInput) {
	// Load session, append user prompt.
	sess := loadSession(input.SessionID)

	// Decay compound pathology cooldowns if the working directory changed project.
	decayPathologiesOnContextChange(sess, input.CWD)

	prompt := input.Prompt
	if prompt == "" {
		// Check for cached alerts from previous Stop event.
		alerts := readAlertCache(input.SessionID)
		if alerts != "" {
			clearAlertCache(input.SessionID)
			emitContext(alerts)
			return
		}
		outputContinue()
		return
	}

	if len(prompt) > 2000 {
		prompt = prompt[:2000]
	}

	sess.Interactions = appendInteraction(sess.Interactions, Interaction{
		Role:      "user",
		Content:   prompt,
		Timestamp: time.Now(),
	})
	saveSession(input.SessionID, sess)

	// Check for cached alerts from previous Stop event.
	alerts := readAlertCache(input.SessionID)
	if alerts != "" {
		clearAlertCache(input.SessionID)
		emitContext(alerts)
		return
	}

	outputContinue()
}

func handlePostToolUse(input HookInput) {
	if input.ToolName == "" {
		outputContinue()
		return
	}

	sess := loadSession(input.SessionID)

	toolResp := input.ToolResponse
	if len(toolResp) > 1000 {
		toolResp = toolResp[:1000]
	}

	text := fmt.Sprintf("[tool:%s] %s", input.ToolName, toolResp)
	sess.Interactions = appendInteraction(sess.Interactions, Interaction{
		Role:      "assistant",
		Content:   text,
		Timestamp: time.Now(),
	})
	saveSession(input.SessionID, sess)

	outputContinue()
}

func handleStop(input HookInput) {
	sess := loadSession(input.SessionID)

	// Decay compound pathology cooldowns if the working directory changed project.
	decayPathologiesOnContextChange(sess, input.CWD)

	msg := input.LastAssistantMsg
	if msg == "" {
		outputContinue()
		return
	}

	if len(msg) > 2000 {
		msg = msg[:2000]
	}

	sess.Interactions = appendInteraction(sess.Interactions, Interaction{
		Role:      "assistant",
		Content:   msg,
		Timestamp: time.Now(),
	})
	saveSession(input.SessionID, sess)

	// Run the pipeline on the session.
	result := runPipeline(sess)
	if result == nil {
		outputContinue()
		return
	}

	summary := formatPipelineResultWithCooldown(result, sess)
	// Save session after cooldown map may have been updated.
	saveSession(input.SessionID, sess)
	if summary == "" {
		outputContinue()
		return
	}

	// Cache the alert for next UserPromptSubmit.
	writeAlertCache(input.SessionID, summary)
	outputContinue()
}

// runPipeline builds a ConversationSnapshot from the session and runs the
// CereBRO pipeline with all fuzzy components enabled.
func runPipeline(sess *HookSession) *pipeline.PipelineResult {
	if len(sess.Interactions) < 2 {
		return nil // need at least a user + assistant turn
	}

	snap := buildSnapshot(sess)
	cfg := buildPipelineConfig(sess)

	return pipeline.Run(snap, cfg)
}

// userOverridePhrases are patterns that indicate the user is explicitly changing direction.
// When a user turn matches these, the turn is tagged [USER_OVERRIDE] so the contradiction
// detector can skip it as a contradiction candidate.
var userOverridePhrases = []string{
	"actually", "no, don't", "instead", "change your", "i want you to",
	"forget that", "disregard", "new approach", "let's do", "lets do",
	"scratch that", "never mind", "start over", "do x instead",
}

// isUserOverrideTurn returns true if the turn text contains explicit direction-change signals.
func isUserOverrideTurn(role, text string) bool {
	if role != "user" {
		return false
	}
	lower := strings.ToLower(text)
	for _, phrase := range userOverridePhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// buildSnapshot converts session interactions into a proto ConversationSnapshot.
// User turns that contain direction-change signals are prefixed with [USER_OVERRIDE]
// so the contradiction detector can skip them as contradiction candidates.
func buildSnapshot(sess *HookSession) *reasoningv1.ConversationSnapshot {
	turns := make([]*reasoningv1.Turn, 0, len(sess.Interactions))
	for i, inter := range sess.Interactions {
		rawText := inter.Content
		if isUserOverrideTurn(inter.Role, inter.Content) {
			rawText = "[USER_OVERRIDE] " + rawText
		}
		turns = append(turns, &reasoningv1.Turn{
			TurnNumber: uint32(i + 1),
			Speaker:    inter.Role,
			RawText:    rawText,
		})
	}
	return &reasoningv1.ConversationSnapshot{
		Turns:      turns,
		TotalTurns: uint32(len(turns)),
		Objective:  "claude_code_session",
	}
}

// orchestrationProjectNames is the set of uppercase project identifiers whose
// presence in session text indicates a multi-project orchestration session.
var orchestrationProjectNames = []string{
	"AETHELRED", "CereBRO", "GEARS", "DORIANG", "FuzzyGuard",
	"SLR", "LAMS", "PTS", "OIL", "brutem", "Sophrim",
}

// isOrchestrationSession returns true when the session has > 8 interactions AND
// contains multi-project indicators (turns mentioning "project", "AETHELRED",
// "deploy", or multiple uppercase project names).
func isOrchestrationSession(sess *HookSession) bool {
	if len(sess.Interactions) <= 8 {
		return false
	}
	projectMentions := 0
	for _, inter := range sess.Interactions {
		text := inter.Content
		if strings.Contains(strings.ToLower(text), "project") ||
			strings.Contains(strings.ToLower(text), "deploy") {
			projectMentions++
		}
		for _, name := range orchestrationProjectNames {
			if strings.Contains(text, name) {
				projectMentions++
				break
			}
		}
	}
	return projectMentions >= 3
}

// projectDepthForContextChange is the number of path components (from root)
// that constitute a "project root" for context-change detection purposes.
// Depth 4 matches the workspace layout /home/<user>/<workspace>/<project>.
const projectDepthForContextChange = 4

// projectRootFromCWD returns a stable "project root" prefix by taking the
// first projectDepthForContextChange path components of cwd. This ensures
// that two paths within the same project subtree resolve to the same key,
// regardless of how deep inside the project they are.
//
// Examples (depth=4):
//
//	/home/user/eidos/GEARS/cmd/foo     → /home/user/eidos/GEARS
//	/home/user/eidos/GEARS/internal/x  → /home/user/eidos/GEARS
//	/home/user/eidos/GEARS             → /home/user/eidos/GEARS
//	/home/user/eidos/CereBRO           → /home/user/eidos/CereBRO
//	/tmp                               → /tmp  (fewer than depth components)
func projectRootFromCWD(cwd string) string {
	if cwd == "" {
		return ""
	}
	// Split path into components and reassemble up to depth.
	parts := strings.Split(filepath.Clean(cwd), string(filepath.Separator))
	// parts[0] is "" for absolute paths (before the leading "/")
	if len(parts) <= projectDepthForContextChange {
		return cwd
	}
	return string(filepath.Separator) + filepath.Join(parts[1:projectDepthForContextChange+1]...)
}

// decayPathologiesOnContextChange resets the ReportedPathologies map when the
// working directory indicates the user has moved to a different project. This
// prevents compound pathology scores from staying suppressed after a topic/project
// context change. A no-op when cwd is empty or unchanged.
func decayPathologiesOnContextChange(sess *HookSession, cwd string) {
	if cwd == "" {
		return
	}
	newRoot := projectRootFromCWD(cwd)
	oldRoot := projectRootFromCWD(sess.LastCWD)

	if sess.LastCWD == "" {
		// First time we see a CWD — just record it, no decay needed.
		sess.LastCWD = cwd
		return
	}

	if newRoot != oldRoot {
		// Project root changed → reset compound pathology cooldowns so detectors
		// can re-report in the new project context.
		sess.ReportedPathologies = nil
	}

	sess.LastCWD = cwd
}

// cerebroMode returns the operating mode from the CEREBRO_MODE env var.
// Valid values: "deterministic" (default), "enriched".
// deterministic: zero external HTTP calls; pure fuzzy pipeline.
// enriched: SLR + Sophrim endpoints active (external LLM calls).
func cerebroMode() string {
	mode := os.Getenv("CEREBRO_MODE")
	if mode == "enriched" {
		return "enriched"
	}
	return "deterministic"
}

// buildPipelineConfig constructs a pipeline config with fuzzy components.
// Loads FIS configs from the source tree or ~/.cerebro/config/fis/.
// When CEREBRO_MODE=deterministic (default), all external HTTP endpoints
// (SLR, Sophrim) are disabled — the pipeline runs fully offline.
func buildPipelineConfig(sess *HookSession) pipeline.PipelineConfig {
	cfg := pipeline.DefaultPipelineConfig()
	cfg.UseInhibitor = true
	cfg.UseNeuromodulation = true
	cfg.UseMetacognition = false // skip for speed
	cfg.UseSalience = false     // skip for speed
	cfg.UseLayer0 = false       // skip for hook (not needed for Claude Code)

	// CEREBRO_MODE controls whether external LLM services are used.
	// deterministic (default): SLREndpoint and SophrimEndpoint are empty —
	// no network calls, pure fuzzy Scope Guard, <1ms per pipeline run.
	// enriched: SLR semantic similarity + Sophrim domain hints active.
	if cerebroMode() == "enriched" {
		cfg.ScopeGuard.SLREndpoint = "http://192.168.14.69:8081"
		cfg.SophrimEndpoint = "http://192.168.14.65:8090"
	}
	// In deterministic mode, SLREndpoint and SophrimEndpoint remain ""
	// (set by DefaultPipelineConfig), so no external calls are made.

	// Detect multi-project orchestration sessions and set OrchestrationMode.
	// This raises the scope-drift trigger threshold by 20% to avoid false
	// positives in CTO sessions that legitimately span multiple projects.
	if sess != nil && isOrchestrationSession(sess) {
		cfg.ScopeGuard.OrchestrationMode = true
	}

	// Try to load FIS configs. If any fail, fall back to crisp.
	fisPath := findFISDir()
	if fisPath == "" {
		return cfg
	}

	// L1: Fuzzy Urgency
	if uc, err := fugo.LoadConfig(filepath.Join(fisPath, "l1_urgency.json")); err == nil {
		if fu, err := pipeline.BuildFuzzyUrgency(uc); err == nil {
			cfg.FuzzyUrgency = fu
		}
	}

	// L2: Detector Fuzzy (severity replacement)
	ancCfg, err1 := fugo.LoadConfig(filepath.Join(fisPath, "l2_anchoring_detector.json"))
	conCfg, err2 := fugo.LoadConfig(filepath.Join(fisPath, "l2_contradiction_detector.json"))
	calCfg, err3 := fugo.LoadConfig(filepath.Join(fisPath, "l2_calibrator_detector.json"))
	scCfg, err4 := fugo.LoadConfig(filepath.Join(fisPath, "l2_sunk_cost_detector.json"))
	if err1 == nil && err2 == nil && err3 == nil && err4 == nil {
		if df, err := pipeline.BuildDetectorFuzzy(ancCfg, conCfg, calCfg, scCfg); err == nil {
			cfg.DetectorFuzzy = df
		}
	}

	// L3: Fuzzy Inhibitor
	fmCfg, err1 := fugo.LoadConfig(filepath.Join(fisPath, "l3_formality_gate.json"))
	svCfg, err2 := fugo.LoadConfig(filepath.Join(fisPath, "l3_severity_gate.json"))
	evCfg, err3 := fugo.LoadConfig(filepath.Join(fisPath, "l3_evidence_gate.json"))
	if err1 == nil && err2 == nil && err3 == nil {
		if fi, err := pipeline.BuildFuzzyInhibitor(fmCfg, svCfg, evCfg); err == nil {
			cfg.Inhibitor.Fuzzy = fi
		}
	}

	// L4: Cross-Layer Arbitrator
	if arCfg, err := fugo.LoadConfig(filepath.Join(fisPath, "cross_layer_arbitration.json")); err == nil {
		if arb, err := pipeline.BuildCrossLayerArbitrator(arCfg); err == nil {
			cfg.Arbitrator = arb
		}
	}

	return cfg
}

// findFISDir looks for FIS config files in known locations.
func findFISDir() string {
	// 1. Source tree (for development).
	// Resolve relative to executable path or well-known locations.
	candidates := []string{
		// Relative to CWD (for dev builds)
		"config/fis",
		// Home directory
		filepath.Join(homeDir(), cerebroDir, fisDir),
		// Source tree absolute path
		"/home/js/eidos/CereBRO/config/fis",
	}

	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "l1_urgency.json")); err == nil {
			return c
		}
	}
	return ""
}

// formatPipelineResult produces a human-readable summary of pipeline findings.
func formatPipelineResult(result *pipeline.PipelineResult) string {
	if result == nil || result.Report == nil {
		return ""
	}

	report := result.Report
	if report.GetOverallIntegrityScore() >= 0.8 && len(report.GetFindings()) == 0 {
		return "" // clean — nothing to report
	}

	var parts []string

	// Report significant findings.
	for _, f := range report.GetFindings() {
		if f.GetConfidence() < 0.2 {
			continue // below reporting threshold
		}
		parts = append(parts, fmt.Sprintf("- %s (%s, conf=%.2f): %s",
			f.GetFindingType().String(),
			f.GetSeverity().String(),
			f.GetConfidence(),
			f.GetExplanation(),
		))
	}

	if len(parts) == 0 {
		return ""
	}

	// Include arbitration if available.
	arb := ""
	if result.Arbitration != nil && result.Arbitration.CompoundPathology > 0.2 {
		arb = fmt.Sprintf("\nCompound pathology: %.2f (%s)",
			result.Arbitration.CompoundPathology, result.Arbitration.Action)
	}

	return fmt.Sprintf(
		"CereBRO reasoning monitor (integrity=%.2f) detected:\n%s%s\n"+
			"Consider whether these patterns are affecting your reasoning quality.",
		report.GetOverallIntegrityScore(),
		strings.Join(parts, "\n"),
		arb,
	)
}

// pathologyCooldownTurns is the number of interaction turns that must pass
// before the same pathology type can be re-reported. Prevents COMPOUND_PATHOLOGY
// (and other findings) from firing on every single turn once triggered.
const pathologyCooldownTurns = 5

// formatPipelineResultWithCooldown is like formatPipelineResult but filters out
// findings whose type was already reported within the last pathologyCooldownTurns
// interactions. It updates sess.ReportedPathologies for any findings that ARE emitted.
func formatPipelineResultWithCooldown(result *pipeline.PipelineResult, sess *HookSession) string {
	if result == nil || result.Report == nil {
		return ""
	}

	report := result.Report
	currentCount := len(sess.Interactions)

	if sess.ReportedPathologies == nil {
		sess.ReportedPathologies = make(map[string]int)
	}

	var parts []string
	emittedTypes := make(map[string]bool)

	for _, f := range report.GetFindings() {
		if f.GetConfidence() < 0.2 {
			continue
		}
		findingType := f.GetFindingType().String()
		lastReportedAt, wasReported := sess.ReportedPathologies[findingType]
		if wasReported && (currentCount-lastReportedAt) <= pathologyCooldownTurns {
			continue // still in cooldown — suppress re-emission
		}
		parts = append(parts, fmt.Sprintf("- %s (%s, conf=%.2f): %s",
			findingType,
			f.GetSeverity().String(),
			f.GetConfidence(),
			f.GetExplanation(),
		))
		emittedTypes[findingType] = true
	}

	if len(parts) == 0 {
		return ""
	}

	// Update ReportedPathologies for emitted findings.
	for ft := range emittedTypes {
		sess.ReportedPathologies[ft] = currentCount
	}

	// Include arbitration if available.
	arb := ""
	if result.Arbitration != nil && result.Arbitration.CompoundPathology > 0.2 {
		arbType := "COMPOUND_PATHOLOGY"
		lastReportedAt, wasReported := sess.ReportedPathologies[arbType]
		if !wasReported || (currentCount-lastReportedAt) > pathologyCooldownTurns {
			arb = fmt.Sprintf("\nCompound pathology: %.2f (%s)",
				result.Arbitration.CompoundPathology, result.Arbitration.Action)
			sess.ReportedPathologies[arbType] = currentCount
		}
	}

	return fmt.Sprintf(
		"CereBRO reasoning monitor (integrity=%.2f) detected:\n%s%s\n"+
			"Consider whether these patterns are affecting your reasoning quality.",
		report.GetOverallIntegrityScore(),
		strings.Join(parts, "\n"),
		arb,
	)
}

func emitContext(alerts string) {
	json.NewEncoder(os.Stdout).Encode(HookOutput{ //nolint:errcheck
		Continue: true,
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:     "UserPromptSubmit",
			AdditionalContext: alerts,
		},
	})
}

func outputContinue() {
	json.NewEncoder(os.Stdout).Encode(HookOutput{Continue: true}) //nolint:errcheck
}

// --- Session persistence ---

func sessionPath(sessionID string) string {
	safe := sanitizeID(sessionID)
	dir := filepath.Join(homeDir(), cerebroDir, sessionsDir)
	_ = os.MkdirAll(dir, 0700)
	return filepath.Join(dir, safe+".json")
}

func loadSession(sessionID string) *HookSession {
	path := sessionPath(sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		return &HookSession{SessionID: sessionID, LastUpdated: time.Now()}
	}
	var sess HookSession
	if err := json.Unmarshal(data, &sess); err != nil {
		return &HookSession{SessionID: sessionID, LastUpdated: time.Now()}
	}
	return &sess
}

func saveSession(sessionID string, sess *HookSession) {
	sess.LastUpdated = time.Now()
	data, err := json.Marshal(sess)
	if err != nil {
		return
	}
	_ = os.WriteFile(sessionPath(sessionID), data, 0600)
}

func appendInteraction(interactions []Interaction, i Interaction) []Interaction {
	interactions = append(interactions, i)
	if len(interactions) > maxInteraction {
		interactions = interactions[len(interactions)-maxInteraction:]
	}
	return interactions
}

// --- Alert cache (cross-hook IPC) ---

func alertCachePath(sessionID string) string {
	safe := sanitizeID(sessionID)
	return filepath.Join(os.TempDir(), "cerebro-alerts-"+safe)
}

func writeAlertCache(sessionID, alerts string) {
	_ = os.WriteFile(alertCachePath(sessionID), []byte(alerts), 0600)
}

func readAlertCache(sessionID string) string {
	data, err := os.ReadFile(alertCachePath(sessionID))
	if err != nil {
		return ""
	}
	return string(data)
}

func clearAlertCache(sessionID string) {
	_ = os.Remove(alertCachePath(sessionID))
}

// --- Helpers ---

func sanitizeID(id string) string {
	safe := strings.ReplaceAll(id, "/", "_")
	safe = strings.ReplaceAll(safe, "..", "_")
	safe = strings.ReplaceAll(safe, "\\", "_")
	if len(safe) > 128 {
		safe = safe[:128]
	}
	return safe
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "/tmp"
}
