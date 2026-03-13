package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func makeTestInput(convID string) *ConsolidationInput {
	return &ConsolidationInput{
		ConversationID: convID,
		Report: &reasoningv1.ReasoningReport{
			Findings: []*reasoningv1.CognitiveAssessment{
				{
					FindingType:   reasoningv1.FindingType_ANCHORING_BIAS,
					Severity:      reasoningv1.FindingSeverity_WARNING,
					Confidence:    0.85,
					DetectorName:  "anchoring-detector",
					Explanation:   "test finding",
					RelevantTurns: []uint32{1, 2},
				},
			},
		},
		Inhibition: &InhibitorResult{
			Decisions: []*cerebrov1.InhibitionDecision{
				{Action: cerebrov1.InhibitionAction_DISINHIBITED, Reason: "all_gates_passed"},
			},
		},
		SelfConf: &cerebrov1.SelfConfidenceReport{
			OverallConfidence: 0.85,
		},
		Snap: &reasoningv1.ConversationSnapshot{
			TotalTurns: 5,
		},
	}
}

func TestConsolidate_HighConfidence(t *testing.T) {
	dir := t.TempDir()
	corpusPath := filepath.Join(dir, "corpus.ndjson")

	idx := NewPatternIndex()
	// Pre-load the pattern so it's NOT novel.
	idx.AddEntry("ANCHORING_BIAS")

	cfg := ConsolidatorConfig{
		CorpusOutputPath:     corpusPath,
		MinConfidenceForAuto: 0.8,
		MinAgreementForAuto:  0.7,
		CooldownSec:          0,
		MaxEntriesPerSession: 10,
	}

	c := NewConsolidator(cfg, idx)
	input := makeTestInput("conv-1")

	result := c.Consolidate(input)
	if !result.Consolidated {
		t.Fatal("expected consolidation to occur")
	}
	if result.Trigger != cerebrov1.ConsolidationTrigger_CONSOLIDATION_HIGH_CONFIDENCE {
		t.Fatalf("expected HIGH_CONFIDENCE trigger, got %v", result.Trigger)
	}
	if result.Entry == nil {
		t.Fatal("expected entry to be non-nil")
	}
	if result.Entry.Trigger != "HIGH_CONFIDENCE" {
		t.Fatalf("expected trigger string HIGH_CONFIDENCE, got %q", result.Entry.Trigger)
	}

	// Verify file was written.
	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("read corpus file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("corpus file is empty")
	}

	// Verify pattern index was updated.
	acc := idx.GetAccuracy()
	if acc["ANCHORING_BIAS"] == 0 {
		t.Fatal("pattern index not updated")
	}
}

func TestConsolidate_NoTrigger(t *testing.T) {
	dir := t.TempDir()
	corpusPath := filepath.Join(dir, "corpus.ndjson")

	idx := NewPatternIndex()
	// Pre-load the pattern so it's NOT novel.
	idx.AddEntry("ANCHORING_BIAS")

	cfg := ConsolidatorConfig{
		CorpusOutputPath:     corpusPath,
		MinConfidenceForAuto: 0.8,
		MinAgreementForAuto:  0.7,
		CooldownSec:          0,
		MaxEntriesPerSession: 10,
	}

	c := NewConsolidator(cfg, idx)
	input := makeTestInput("conv-1")
	// Set confidence below threshold so HIGH_CONFIDENCE doesn't trigger.
	input.Report.Findings[0].Confidence = 0.4

	result := c.Consolidate(input)
	if result.Consolidated {
		t.Fatal("expected no consolidation")
	}

	// Verify no file written.
	if _, err := os.Stat(corpusPath); err == nil {
		t.Fatal("corpus file should not exist")
	}
}

func TestConsolidate_NovelPattern(t *testing.T) {
	dir := t.TempDir()
	corpusPath := filepath.Join(dir, "corpus.ndjson")

	idx := NewPatternIndex()
	// Do NOT pre-load — the pattern will be novel.

	cfg := ConsolidatorConfig{
		CorpusOutputPath:     corpusPath,
		MinConfidenceForAuto: 0.8,
		MinAgreementForAuto:  0.7,
		CooldownSec:          0,
		MaxEntriesPerSession: 10,
	}

	c := NewConsolidator(cfg, idx)
	input := makeTestInput("conv-1")
	// Use low confidence so HIGH_CONFIDENCE doesn't trigger first.
	input.Report.Findings[0].Confidence = 0.5
	input.Report.Findings[0].FindingType = reasoningv1.FindingType_SUNK_COST_FALLACY

	result := c.Consolidate(input)
	if !result.Consolidated {
		t.Fatal("expected consolidation to occur")
	}
	if result.Trigger != cerebrov1.ConsolidationTrigger_CONSOLIDATION_NOVEL_PATTERN {
		t.Fatalf("expected NOVEL_PATTERN trigger, got %v", result.Trigger)
	}
	if result.Entry.Trigger != "NOVEL_PATTERN" {
		t.Fatalf("expected trigger string NOVEL_PATTERN, got %q", result.Entry.Trigger)
	}
}

func TestConsolidate_Cooldown(t *testing.T) {
	dir := t.TempDir()
	corpusPath := filepath.Join(dir, "corpus.ndjson")

	idx := NewPatternIndex()
	idx.AddEntry("ANCHORING_BIAS")

	cfg := ConsolidatorConfig{
		CorpusOutputPath:     corpusPath,
		MinConfidenceForAuto: 0.8,
		MinAgreementForAuto:  0.7,
		CooldownSec:          1, // 1 second cooldown
		MaxEntriesPerSession: 10,
	}

	c := NewConsolidator(cfg, idx)
	input := makeTestInput("conv-1")

	// First consolidation should succeed.
	r1 := c.Consolidate(input)
	if !r1.Consolidated {
		t.Fatal("expected first consolidation to succeed")
	}

	// Second consolidation immediately should be blocked by cooldown.
	input2 := makeTestInput("conv-2")
	r2 := c.Consolidate(input2)
	if r2.Consolidated {
		t.Fatal("expected second consolidation to be blocked by cooldown")
	}
}

func TestConsolidate_MaxEntries(t *testing.T) {
	dir := t.TempDir()
	corpusPath := filepath.Join(dir, "corpus.ndjson")

	idx := NewPatternIndex()
	idx.AddEntry("ANCHORING_BIAS")

	cfg := ConsolidatorConfig{
		CorpusOutputPath:     corpusPath,
		MinConfidenceForAuto: 0.8,
		MinAgreementForAuto:  0.7,
		CooldownSec:          0, // no cooldown
		MaxEntriesPerSession: 2,
	}

	c := NewConsolidator(cfg, idx)

	// First two should succeed.
	for i := 0; i < 2; i++ {
		input := makeTestInput("conv-" + string(rune('a'+i)))
		r := c.Consolidate(input)
		if !r.Consolidated {
			t.Fatalf("expected consolidation %d to succeed", i+1)
		}
	}

	// Third should be blocked.
	input3 := makeTestInput("conv-c")
	r3 := c.Consolidate(input3)
	if r3.Consolidated {
		t.Fatal("expected third consolidation to be blocked by max entries")
	}
}

func TestConsolidate_NDJSONFormat(t *testing.T) {
	dir := t.TempDir()
	corpusPath := filepath.Join(dir, "corpus.ndjson")

	idx := NewPatternIndex()
	idx.AddEntry("ANCHORING_BIAS")

	cfg := ConsolidatorConfig{
		CorpusOutputPath:     corpusPath,
		MinConfidenceForAuto: 0.8,
		MinAgreementForAuto:  0.7,
		CooldownSec:          0,
		MaxEntriesPerSession: 10,
	}

	c := NewConsolidator(cfg, idx)

	// Write two entries.
	for i := 0; i < 2; i++ {
		input := makeTestInput("conv-ndjson")
		r := c.Consolidate(input)
		if !r.Consolidated {
			t.Fatalf("consolidation %d failed", i+1)
		}
	}

	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	for i, line := range lines {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("line %d is not valid JSON: %v", i+1, err)
		}
		// Verify required keys exist.
		for _, key := range []string{"entry_id", "timestamp", "trigger", "finding_types", "expected"} {
			if _, ok := obj[key]; !ok {
				t.Fatalf("line %d missing key %q", i+1, key)
			}
		}
	}
}

func TestConsolidate_SparseIndex(t *testing.T) {
	dir := t.TempDir()
	corpusPath := filepath.Join(dir, "corpus.ndjson")

	idx := NewPatternIndex()
	idx.AddEntry("ANCHORING_BIAS")

	cfg := ConsolidatorConfig{
		CorpusOutputPath:     corpusPath,
		MinConfidenceForAuto: 0.8,
		MinAgreementForAuto:  0.7,
		CooldownSec:          0,
		MaxEntriesPerSession: 10,
	}

	c := NewConsolidator(cfg, idx)
	input := makeTestInput("conv-sparse")
	r := c.Consolidate(input)
	if !r.Consolidated {
		t.Fatal("expected consolidation")
	}

	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}

	line := strings.TrimSpace(string(data))
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Sparse index entries must NOT contain raw text, claim text, or full assessment details.
	forbiddenKeys := []string{"raw_text", "claim_text", "assessment_details", "turns", "conversation_text"}
	for _, key := range forbiddenKeys {
		if _, ok := obj[key]; ok {
			t.Fatalf("entry should not contain key %q (sparse index)", key)
		}
	}

	// Verify expected sparse keys ARE present.
	expectedKeys := []string{
		"entry_id", "timestamp", "trigger", "finding_types",
		"finding_confidences", "finding_severities", "detector_pattern",
		"self_confidence", "expected",
	}
	for _, key := range expectedKeys {
		if _, ok := obj[key]; !ok {
			t.Fatalf("entry missing expected sparse key %q", key)
		}
	}
}

func TestSubmitFeedback_Confirmed(t *testing.T) {
	dir := t.TempDir()
	corpusPath := filepath.Join(dir, "corpus.ndjson")

	idx := NewPatternIndex()

	cfg := ConsolidatorConfig{
		CorpusOutputPath:     corpusPath,
		MinConfidenceForAuto: 0.8,
		MinAgreementForAuto:  0.7,
		CooldownSec:          0,
		MaxEntriesPerSession: 10,
	}

	c := NewConsolidator(cfg, idx)

	// Consolidate first to store the input.
	input := makeTestInput("conv-1")
	c.Consolidate(input)

	// Submit feedback.
	err := c.SubmitFeedback("conv-1", "confirmed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// Find the feedback entry (last line).
	lastLine := lines[len(lines)-1]
	var obj consolidationJSON
	if err := json.Unmarshal([]byte(lastLine), &obj); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if obj.Trigger != "USER_FEEDBACK" {
		t.Fatalf("expected trigger USER_FEEDBACK, got %q", obj.Trigger)
	}
	if obj.Outcome != "confirmed" {
		t.Fatalf("expected outcome confirmed, got %q", obj.Outcome)
	}
}

func TestSubmitFeedback_Rejected(t *testing.T) {
	dir := t.TempDir()
	corpusPath := filepath.Join(dir, "corpus.ndjson")

	idx := NewPatternIndex()

	cfg := ConsolidatorConfig{
		CorpusOutputPath:     corpusPath,
		MinConfidenceForAuto: 0.8,
		MinAgreementForAuto:  0.7,
		CooldownSec:          0,
		MaxEntriesPerSession: 10,
	}

	c := NewConsolidator(cfg, idx)

	// Consolidate first to store the input.
	input := makeTestInput("conv-1")
	c.Consolidate(input)

	// Submit feedback with "rejected".
	err := c.SubmitFeedback("conv-1", "rejected")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	lastLine := lines[len(lines)-1]
	var obj consolidationJSON
	if err := json.Unmarshal([]byte(lastLine), &obj); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if obj.Trigger != "USER_FEEDBACK" {
		t.Fatalf("expected trigger USER_FEEDBACK, got %q", obj.Trigger)
	}
	if obj.Outcome != "rejected" {
		t.Fatalf("expected outcome rejected, got %q", obj.Outcome)
	}
}

func TestSubmitFeedback_UnknownID(t *testing.T) {
	dir := t.TempDir()
	corpusPath := filepath.Join(dir, "corpus.ndjson")

	idx := NewPatternIndex()

	cfg := ConsolidatorConfig{
		CorpusOutputPath:     corpusPath,
		MinConfidenceForAuto: 0.8,
		MinAgreementForAuto:  0.7,
		CooldownSec:          0,
		MaxEntriesPerSession: 10,
	}

	c := NewConsolidator(cfg, idx)

	err := c.SubmitFeedback("unknown-conv", "confirmed")
	if err == nil {
		t.Fatal("expected error for unknown conversation ID")
	}
	if !strings.Contains(err.Error(), "unknown-conv") {
		t.Fatalf("error should mention conversation ID, got: %v", err)
	}
}

// Suppress unused import warnings.
var _ = time.Now
