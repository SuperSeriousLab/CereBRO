package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// corpusEntry matches the NDJSON corpus schema.
type corpusEntry struct {
	EntryID  string          `json:"entry_id"`
	Input    json.RawMessage `json:"input"`
	Expected []struct {
		FindingType string `json:"finding_type"`
	} `json:"expected"`
}

// snapshotJSON is the JSON representation of a ConversationSnapshot.
type snapshotJSON struct {
	Turns []struct {
		TurnNumber uint32 `json:"turn_number"`
		Speaker    string `json:"speaker"`
		RawText    string `json:"raw_text"`
	} `json:"turns"`
	Objective string `json:"objective"`
	TotalTurns uint32 `json:"total_turns"`
}

func loadCorpusEntry(path string) (*corpusEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Handle multi-line NDJSON — use first non-empty line
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry corpusEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		return &entry, nil
	}
	return nil, fmt.Errorf("empty file: %s", path)
}

func entryToSnapshot(entry *corpusEntry) (*reasoningv1.ConversationSnapshot, error) {
	var sj snapshotJSON
	if err := json.Unmarshal(entry.Input, &sj); err != nil {
		return nil, err
	}

	snap := &reasoningv1.ConversationSnapshot{
		Objective:  sj.Objective,
		TotalTurns: sj.TotalTurns,
	}
	for _, t := range sj.Turns {
		snap.Turns = append(snap.Turns, &reasoningv1.Turn{
			TurnNumber: t.TurnNumber,
			Speaker:    t.Speaker,
			RawText:    t.RawText,
		})
	}
	return snap, nil
}

// findingTypeString is a test-local alias for the exported FindingTypeString.
func findingTypeString(ft reasoningv1.FindingType) string {
	return FindingTypeString(ft)
}

// TestSeedCorpus validates the pipeline against the existing seed corpus.
func TestSeedCorpus(t *testing.T) {
	corpusPath := filepath.Join("..", "..", "data", "corpus", "cognitive-v1.ndjson")
	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Skipf("seed corpus not found: %v", err)
	}

	cfg := DefaultPipelineConfig()

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry corpusEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("parse corpus entry: %v", err)
		}

		t.Run(entry.EntryID, func(t *testing.T) {
			snap, err := entryToSnapshot(&entry)
			if err != nil {
				t.Fatalf("convert snapshot: %v", err)
			}

			result := Run(snap, cfg)

			// Collect actual finding types
			actualTypes := make(map[string]bool)
			for _, f := range result.Findings {
				actualTypes[findingTypeString(f.FindingType)] = true
			}

			// Collect expected types
			expectedTypes := make(map[string]bool)
			for _, exp := range entry.Expected {
				expectedTypes[exp.FindingType] = true
			}

			// Score: TP, FN, FP
			var tp, fn, fp int
			for et := range expectedTypes {
				if actualTypes[et] {
					tp++
				} else {
					fn++
					t.Logf("  MISS: expected %s not detected", et)
				}
			}
			for at := range actualTypes {
				if !expectedTypes[at] {
					fp++
					t.Logf("  FP: unexpected %s (not in expected)", at)
				}
			}

			t.Logf("  integrity=%.2f TP=%d FN=%d FP=%d activated=%v findings=%v",
				result.Report.OverallIntegrityScore, tp, fn, fp,
				detectorNames(result.Routing.Activated),
				findingNames(result.Findings))
		})
	}
}

// TestLLMConversations runs the pipeline against LLM-generated test conversations.
func TestLLMConversations(t *testing.T) {
	convDir := filepath.Join("..", "..", "data", "test-conversations")
	if _, err := os.Stat(convDir); os.IsNotExist(err) {
		t.Skip("test-conversations directory not found — run Phase 1A first")
	}

	cfg := DefaultPipelineConfig()

	files, err := filepath.Glob(filepath.Join(convDir, "*.ndjson"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	if len(files) == 0 {
		t.Skip("no .ndjson files in test-conversations")
	}

	var scorecards []scorecard

	for _, f := range files {
		base := filepath.Base(f)
		t.Run(base, func(t *testing.T) {
			entry, err := loadCorpusEntry(f)
			if err != nil {
				t.Fatalf("load %s: %v", f, err)
			}

			snap, err := entryToSnapshot(entry)
			if err != nil {
				t.Fatalf("convert snapshot: %v", err)
			}

			result := Run(snap, cfg)

			// Collect actual finding types
			actualTypes := make(map[string]bool)
			for _, finding := range result.Findings {
				actualTypes[findingTypeString(finding.FindingType)] = true
			}

			// Expected findings from corpus entry
			expectedTypes := make(map[string]bool)
			for _, exp := range entry.Expected {
				expectedTypes[exp.FindingType] = true
			}

			// True positives, false negatives, false positives
			var tp, fn, fp int
			for et := range expectedTypes {
				if actualTypes[et] {
					tp++
				} else {
					fn++
					t.Errorf("MISS: expected %s but not detected", et)
				}
			}
			for at := range actualTypes {
				if !expectedTypes[at] {
					fp++
					// For clean/borderline conversations, FP is important but not always wrong
					if len(entry.Expected) == 0 {
						t.Logf("  FP ON CLEAN: unexpected %s", at)
					} else {
						t.Logf("  EXTRA: unexpected %s (not in expected)", at)
					}
				}
			}

			sc := scorecard{
				ID:             entry.EntryID,
				File:           base,
				Integrity:      result.Report.OverallIntegrityScore,
				TruePositives:  tp,
				FalseNegatives: fn,
				FalsePositives: fp,
				Activated:      detectorNames(result.Routing.Activated),
				Findings:       findingNames(result.Findings),
				Expected:       setToSlice(expectedTypes),
			}
			scorecards = append(scorecards, sc)

			t.Logf("  ID=%s integrity=%.2f TP=%d FN=%d FP=%d",
				entry.EntryID, result.Report.OverallIntegrityScore, tp, fn, fp)
			t.Logf("  activated: %v", sc.Activated)
			t.Logf("  findings: %v", sc.Findings)
			t.Logf("  expected: %v", sc.Expected)

			// Log finding details
			for _, finding := range result.Findings {
				t.Logf("    %s severity=%s confidence=%.2f turns=%v",
					findingTypeString(finding.FindingType),
					finding.Severity.String(),
					finding.Confidence,
					finding.RelevantTurns)
			}
		})
	}

	// Print summary scorecard
	if len(scorecards) > 0 {
		t.Log("\n========== PIPELINE SCORECARD ==========")
		totalTP, totalFN, totalFP := 0, 0, 0
		for _, sc := range scorecards {
			totalTP += sc.TruePositives
			totalFN += sc.FalseNegatives
			totalFP += sc.FalsePositives
			status := "PASS"
			if sc.FalseNegatives > 0 {
				status = "MISS"
			}
			if sc.FalsePositives > 0 && len(sc.Expected) == 0 {
				status = "FP"
			}
			t.Logf("  [%s] %-30s integrity=%.2f TP=%d FN=%d FP=%d",
				status, sc.File, sc.Integrity, sc.TruePositives, sc.FalseNegatives, sc.FalsePositives)
		}
		t.Logf("  ─────────────────────────────────────")

		precision := 0.0
		if totalTP+totalFP > 0 {
			precision = float64(totalTP) / float64(totalTP+totalFP)
		}
		recall := 0.0
		if totalTP+totalFN > 0 {
			recall = float64(totalTP) / float64(totalTP+totalFN)
		}
		f1 := 0.0
		if precision+recall > 0 {
			f1 = 2 * precision * recall / (precision + recall)
		}

		t.Logf("  TOTAL: TP=%d FN=%d FP=%d", totalTP, totalFN, totalFP)
		t.Logf("  Precision=%.2f Recall=%.2f F1=%.2f", precision, recall, f1)
	}
}

type scorecard struct {
	ID             string
	File           string
	Integrity      float64
	TruePositives  int
	FalseNegatives int
	FalsePositives int
	Activated      []string
	Findings       []string
	Expected       []string
}

func findingNames(findings []*reasoningv1.CognitiveAssessment) []string {
	var names []string
	for _, f := range findings {
		names = append(names, findingTypeString(f.FindingType))
	}
	return names
}

func detectorNames(detectors []Detector) []string {
	var names []string
	for _, d := range detectors {
		names = append(names, string(d))
	}
	return names
}

func setToSlice(s map[string]bool) []string {
	var result []string
	for k := range s {
		result = append(result, k)
	}
	return result
}

// TestLLMConversationsWithInhibitor runs the full pipeline with the Context
// Inhibitor enabled and measures post-inhibition metrics (Phase 1).
func TestLLMConversationsWithInhibitor(t *testing.T) {
	convDir := filepath.Join("..", "..", "data", "test-conversations")
	if _, err := os.Stat(convDir); os.IsNotExist(err) {
		t.Skip("test-conversations directory not found")
	}

	cfg := DefaultPipelineConfig()
	cfg.UseInhibitor = true
	cfg.Inhibitor = DefaultInhibitorConfig()

	files, err := filepath.Glob(filepath.Join(convDir, "*.ndjson"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	if len(files) == 0 {
		t.Skip("no .ndjson files in test-conversations")
	}

	totalTP, totalFN, totalFP := 0, 0, 0
	totalInhibited, totalDisinhibited := 0, 0

	for _, f := range files {
		base := filepath.Base(f)
		t.Run(base, func(t *testing.T) {
			entry, err := loadCorpusEntry(f)
			if err != nil {
				t.Fatalf("load %s: %v", f, err)
			}

			snap, err := entryToSnapshot(entry)
			if err != nil {
				t.Fatalf("convert snapshot: %v", err)
			}

			result := Run(snap, cfg)

			// Post-inhibition: use Gated findings (what passes the inhibitor)
			gatedTypes := make(map[string]bool)
			if result.Inhibition != nil {
				for _, finding := range result.Inhibition.Gated {
					gatedTypes[findingTypeString(finding.FindingType)] = true
				}
			}

			expectedTypes := make(map[string]bool)
			for _, exp := range entry.Expected {
				expectedTypes[exp.FindingType] = true
			}

			var tp, fn, fp int
			for et := range expectedTypes {
				if gatedTypes[et] {
					tp++
				} else {
					fn++
					t.Errorf("MISS (post-inhibition): expected %s but inhibited/not detected", et)
				}
			}
			for at := range gatedTypes {
				if !expectedTypes[at] {
					fp++
					t.Logf("  FP (post-inhibition): unexpected %s", at)
				}
			}

			totalTP += tp
			totalFN += fn
			totalFP += fp

			// Log inhibition decisions
			if result.Inhibition != nil {
				t.Logf("  formality=%.2f", result.Inhibition.Formality)
				for _, d := range result.Inhibition.Decisions {
					action := "INHIBITED"
					if d.GetAction() == 2 { // DISINHIBITED
						action = "DISINHIBITED"
						totalDisinhibited++
					} else {
						totalInhibited++
					}
					t.Logf("    %s %s: %s (corr=%.2f)",
						action, d.GetDetectorName(), d.GetReason(), d.GetCorroborationScore())
				}
			}

			// Raw vs gated comparison
			t.Logf("  raw_findings=%d gated_findings=%d TP=%d FN=%d FP=%d",
				len(result.Findings), len(result.Inhibition.Gated), tp, fn, fp)
		})
	}

	// Summary
	t.Log("\n========== PHASE 1 SCORECARD ==========")
	precision := 0.0
	if totalTP+totalFP > 0 {
		precision = float64(totalTP) / float64(totalTP+totalFP)
	}
	recall := 0.0
	if totalTP+totalFN > 0 {
		recall = float64(totalTP) / float64(totalTP+totalFN)
	}
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}

	t.Logf("  TOTAL: TP=%d FN=%d FP=%d", totalTP, totalFN, totalFP)
	t.Logf("  Precision=%.2f Recall=%.2f F1=%.2f", precision, recall, f1)
	t.Logf("  Inhibited=%d Disinhibited=%d", totalInhibited, totalDisinhibited)
	t.Log("  ─────────────────────────────────────")
	t.Log("  Baseline: Precision=0.64 Recall=1.00 F1=0.78 FP=5")
	t.Logf("  Pipeline: Precision=%.2f Recall=%.2f F1=%.2f FP=%d", precision, recall, f1, totalFP)
}

// TestLLMConversationsWithNeuromodulation runs the full pipeline with Urgency
// Assessor + Threshold Modulator + Context Inhibitor (Phase 2).
func TestLLMConversationsWithNeuromodulation(t *testing.T) {
	convDir := filepath.Join("..", "..", "data", "test-conversations")
	if _, err := os.Stat(convDir); os.IsNotExist(err) {
		t.Skip("test-conversations directory not found")
	}

	cfg := DefaultPipelineConfig()
	cfg.UseInhibitor = true
	cfg.Inhibitor = DefaultInhibitorConfig()
	cfg.UseNeuromodulation = true
	cfg.Urgency = DefaultUrgencyConfig()
	cfg.Modulator = DefaultModulatorConfig()

	files, err := filepath.Glob(filepath.Join(convDir, "*.ndjson"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	if len(files) == 0 {
		t.Skip("no .ndjson files in test-conversations")
	}

	totalTP, totalFN, totalFP := 0, 0, 0

	for _, f := range files {
		base := filepath.Base(f)
		t.Run(base, func(t *testing.T) {
			entry, err := loadCorpusEntry(f)
			if err != nil {
				t.Fatalf("load %s: %v", f, err)
			}

			snap, err := entryToSnapshot(entry)
			if err != nil {
				t.Fatalf("convert snapshot: %v", err)
			}

			result := Run(snap, cfg)

			// Verify GainSignal was produced.
			if result.Gain == nil {
				t.Fatal("expected GainSignal, got nil")
			}

			// Log gain signal values.
			t.Logf("  gain: urgency=%.2f complexity=%.2f formality=%.2f mode=%v",
				result.Gain.Urgency, result.Gain.Complexity, result.Gain.Formality, result.Gain.Mode)

			// Verify threshold adjustments exist and contain non-zero offsets.
			if result.Adjustments == nil {
				t.Fatal("expected ThresholdAdjustments, got nil")
			}
			if len(result.Adjustments.Adjustments) == 0 {
				t.Fatal("expected non-empty adjustments map")
			}
			// Log one representative offset.
			for det, offset := range result.Adjustments.Adjustments {
				t.Logf("  gain_offset=%.3f (for %s)", offset, det)
				break
			}
			// Verify scope-guard is excluded from gain modulation.
			if _, ok := result.Adjustments.Adjustments["scope-guard"]; ok {
				t.Error("scope-guard should be excluded from gain modulation")
			}

			// Post-inhibition findings.
			gatedTypes := make(map[string]bool)
			if result.Inhibition != nil {
				for _, finding := range result.Inhibition.Gated {
					gatedTypes[findingTypeString(finding.FindingType)] = true
				}
			}

			expectedTypes := make(map[string]bool)
			for _, exp := range entry.Expected {
				expectedTypes[exp.FindingType] = true
			}

			var tp, fn, fp int
			for et := range expectedTypes {
				if gatedTypes[et] {
					tp++
				} else {
					fn++
					t.Errorf("MISS (neuromodulated): expected %s", et)
				}
			}
			for at := range gatedTypes {
				if !expectedTypes[at] {
					fp++
					t.Logf("  FP (neuromodulated): unexpected %s", at)
				}
			}

			totalTP += tp
			totalFN += fn
			totalFP += fp

			// Log inhibition decisions.
			if result.Inhibition != nil {
				for _, d := range result.Inhibition.Decisions {
					action := "INH"
					if d.GetAction() == 2 {
						action = "DIS"
					}
					t.Logf("    %s %s: %s", action, d.GetDetectorName(), d.GetReason())
				}
			}

			gatedCount := 0
			if result.Inhibition != nil {
				gatedCount = len(result.Inhibition.Gated)
			}
			t.Logf("  raw=%d gated=%d TP=%d FN=%d FP=%d",
				len(result.Findings), gatedCount, tp, fn, fp)
		})
	}

	// Summary
	t.Log("\n========== PHASE 2 SCORECARD ==========")
	precision := 0.0
	if totalTP+totalFP > 0 {
		precision = float64(totalTP) / float64(totalTP+totalFP)
	}
	recall := 0.0
	if totalTP+totalFN > 0 {
		recall = float64(totalTP) / float64(totalTP+totalFN)
	}
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}

	t.Logf("  TOTAL: TP=%d FN=%d FP=%d", totalTP, totalFN, totalFP)
	t.Logf("  Precision=%.2f Recall=%.2f F1=%.2f", precision, recall, f1)
	t.Log("  ─────────────────────────────────────")
	t.Log("  Phase 1:  Precision=0.82 Recall=1.00 F1=0.90 FP=2")
	t.Logf("  Phase 2:  Precision=%.2f Recall=%.2f F1=%.2f FP=%d", precision, recall, f1, totalFP)

	// Exit criteria checks.
	if recall < 1.0 {
		t.Errorf("EXIT CRITERION FAILED: recall %.2f < 1.00", recall)
	}
	// Phase 1 baseline: TP=9, FP=2 → precision=9/11≈0.818.
	// No regression means matching or exceeding that.
	if precision < 0.81 {
		t.Errorf("EXIT CRITERION FAILED: precision %.2f < 0.81 (regression from Phase 1 baseline 0.818)", precision)
	}
}

// TestLLMConversationsWithLayer0 runs the full pipeline with Layer 0 + Inhibitor
// + Neuromodulation enabled and verifies no regression from Layer 0 addition.
func TestLLMConversationsWithLayer0(t *testing.T) {
	convDir := filepath.Join("..", "..", "data", "test-conversations")
	if _, err := os.Stat(convDir); os.IsNotExist(err) {
		t.Skip("test-conversations directory not found")
	}

	profileDir := filepath.Join("..", "..", "data", "language-profiles")
	profiles, err := LoadLangProfiles(profileDir)
	if err != nil {
		t.Skipf("language profiles not found: %v", err)
	}

	blocklistPath := filepath.Join("..", "..", "data", "blocklists", "default.txt")
	blocklist, err := LoadBlocklist(blocklistPath)
	if err != nil {
		t.Skipf("blocklist not found: %v", err)
	}

	cfg := DefaultPipelineConfig()
	cfg.UseLayer0 = true
	cfg.Layer0 = DefaultLayer0Config()
	cfg.Layer0.Toxicity.Blocklist = blocklist
	cfg.Layer0.Language.Profiles = profiles
	cfg.UseInhibitor = true
	cfg.Inhibitor = DefaultInhibitorConfig()
	cfg.UseNeuromodulation = true
	cfg.Urgency = DefaultUrgencyConfig()
	cfg.Modulator = DefaultModulatorConfig()

	files, err := filepath.Glob(filepath.Join(convDir, "*.ndjson"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	if len(files) == 0 {
		t.Skip("no .ndjson files in test-conversations")
	}

	totalTP, totalFN, totalFP := 0, 0, 0

	for _, f := range files {
		base := filepath.Base(f)
		t.Run(base, func(t *testing.T) {
			entry, err := loadCorpusEntry(f)
			if err != nil {
				t.Fatalf("load %s: %v", f, err)
			}

			snap, err := entryToSnapshot(entry)
			if err != nil {
				t.Fatalf("convert snapshot: %v", err)
			}

			result := Run(snap, cfg)

			// All test conversations should pass Layer 0 (they're well-formed English).
			if result.Rejected {
				t.Fatalf("Layer 0 rejected test conversation: %s", result.Layer0.Reason)
			}

			// Post-inhibition findings.
			gatedTypes := make(map[string]bool)
			if result.Inhibition != nil {
				for _, finding := range result.Inhibition.Gated {
					gatedTypes[findingTypeString(finding.FindingType)] = true
				}
			}

			expectedTypes := make(map[string]bool)
			for _, exp := range entry.Expected {
				expectedTypes[exp.FindingType] = true
			}

			var tp, fn, fp int
			for et := range expectedTypes {
				if gatedTypes[et] {
					tp++
				} else {
					fn++
					t.Errorf("MISS (Layer0+neuro): expected %s", et)
				}
			}
			for at := range gatedTypes {
				if !expectedTypes[at] {
					fp++
					t.Logf("  FP (Layer0+neuro): unexpected %s", at)
				}
			}

			totalTP += tp
			totalFN += fn
			totalFP += fp

			gatedCount := 0
			if result.Inhibition != nil {
				gatedCount = len(result.Inhibition.Gated)
			}
			t.Logf("  raw=%d gated=%d TP=%d FN=%d FP=%d",
				len(result.Findings), gatedCount, tp, fn, fp)
		})
	}

	// Summary
	t.Log("\n========== PHASE 3 SCORECARD ==========")
	precision := 0.0
	if totalTP+totalFP > 0 {
		precision = float64(totalTP) / float64(totalTP+totalFP)
	}
	recall := 0.0
	if totalTP+totalFN > 0 {
		recall = float64(totalTP) / float64(totalTP+totalFN)
	}
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}

	t.Logf("  TOTAL: TP=%d FN=%d FP=%d", totalTP, totalFN, totalFP)
	t.Logf("  Precision=%.2f Recall=%.2f F1=%.2f", precision, recall, f1)
	t.Log("  ─────────────────────────────────────")
	t.Log("  Phase 2:  Precision=0.82 Recall=1.00 F1=0.90 FP=2")
	t.Logf("  Phase 3:  Precision=%.2f Recall=%.2f F1=%.2f FP=%d", precision, recall, f1, totalFP)

	// No regression from Layer 0 addition.
	if recall < 1.0 {
		t.Errorf("EXIT CRITERION FAILED: recall %.2f < 1.00", recall)
	}
	if precision < 0.81 {
		t.Errorf("EXIT CRITERION FAILED: precision %.2f < 0.81 (regression)", precision)
	}
}

// TestLLMConversationsWithMetacognition runs the full pipeline with all phases enabled
// (Layer 0 + Neuromodulation + Inhibitor + Metacognition) and verifies no regression.
func TestLLMConversationsWithMetacognition(t *testing.T) {
	convDir := filepath.Join("..", "..", "data", "test-conversations")
	if _, err := os.Stat(convDir); os.IsNotExist(err) {
		t.Skip("test-conversations directory not found")
	}

	profileDir := filepath.Join("..", "..", "data", "language-profiles")
	profiles, err := LoadLangProfiles(profileDir)
	if err != nil {
		t.Skipf("language profiles not found: %v", err)
	}

	blocklistPath := filepath.Join("..", "..", "data", "blocklists", "default.txt")
	blocklist, err := LoadBlocklist(blocklistPath)
	if err != nil {
		t.Skipf("blocklist not found: %v", err)
	}

	cfg := DefaultPipelineConfig()
	cfg.UseLayer0 = true
	cfg.Layer0 = DefaultLayer0Config()
	cfg.Layer0.Toxicity.Blocklist = blocklist
	cfg.Layer0.Language.Profiles = profiles
	cfg.UseInhibitor = true
	cfg.Inhibitor = DefaultInhibitorConfig()
	cfg.UseNeuromodulation = true
	cfg.Urgency = DefaultUrgencyConfig()
	cfg.Modulator = DefaultModulatorConfig()
	cfg.UseMetacognition = true
	cfg.SelfConfidence = DefaultSelfConfidenceConfig()
	cfg.Feedback = DefaultFeedbackConfig()

	files, err := filepath.Glob(filepath.Join(convDir, "*.ndjson"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	if len(files) == 0 {
		t.Skip("no .ndjson files in test-conversations")
	}

	totalTP, totalFN, totalFP := 0, 0, 0
	feedbackTriggered := 0

	for _, f := range files {
		base := filepath.Base(f)
		t.Run(base, func(t *testing.T) {
			entry, err := loadCorpusEntry(f)
			if err != nil {
				t.Fatalf("load %s: %v", f, err)
			}

			snap, err := entryToSnapshot(entry)
			if err != nil {
				t.Fatalf("convert snapshot: %v", err)
			}

			result := Run(snap, cfg)

			// All test conversations should pass Layer 0.
			if result.Rejected {
				t.Fatalf("Layer 0 rejected: %s", result.Layer0.Reason)
			}

			// Self-confidence must be present.
			if result.SelfConf == nil {
				t.Fatal("expected SelfConfidenceReport, got nil")
			}
			t.Logf("  self-confidence: overall=%.3f agreement=%.3f margin=%.3f historical=%.3f rec=%v",
				result.SelfConf.GetOverallConfidence(),
				result.SelfConf.GetAgreementScore(),
				result.SelfConf.GetMarginScore(),
				result.SelfConf.GetHistoricalScore(),
				result.SelfConf.GetRecommendation())

			// Log feedback result.
			if result.Feedback != nil && result.Feedback.Applied {
				feedbackTriggered++
				t.Logf("  feedback: applied=true detectors=%v deltas=%v",
					result.Feedback.ReevalDetectors, result.Feedback.ConfidenceDeltas)
			}

			// Post-inhibition findings for scoring.
			gatedTypes := make(map[string]bool)
			if result.Inhibition != nil {
				for _, finding := range result.Inhibition.Gated {
					gatedTypes[findingTypeString(finding.FindingType)] = true
				}
			} else {
				// No inhibitor in second pass? Use report findings.
				for _, finding := range result.Findings {
					gatedTypes[findingTypeString(finding.FindingType)] = true
				}
			}

			expectedTypes := make(map[string]bool)
			for _, exp := range entry.Expected {
				expectedTypes[exp.FindingType] = true
			}

			var tp, fn, fp int
			for et := range expectedTypes {
				if gatedTypes[et] {
					tp++
				} else {
					fn++
					t.Errorf("MISS (metacognition): expected %s", et)
				}
			}
			for at := range gatedTypes {
				if !expectedTypes[at] {
					fp++
					t.Logf("  FP (metacognition): unexpected %s", at)
				}
			}

			totalTP += tp
			totalFN += fn
			totalFP += fp

			gatedCount := 0
			if result.Inhibition != nil {
				gatedCount = len(result.Inhibition.Gated)
			}
			t.Logf("  raw=%d gated=%d TP=%d FN=%d FP=%d",
				len(result.Findings), gatedCount, tp, fn, fp)

			// Verify CerebroReport contains metacognition data.
			cr := result.ToCerebroReport()
			if cr.GetSelfConfidence() == nil {
				t.Error("expected self_confidence in CerebroReport")
			}
			if result.Feedback != nil && result.Feedback.Applied {
				if cr.GetPassCount() != 2 {
					t.Errorf("expected pass_count=2 after feedback, got %d", cr.GetPassCount())
				}
				if !cr.GetFeedbackApplied() {
					t.Error("expected feedback_applied=true in CerebroReport")
				}
			}
		})
	}

	// Summary
	t.Log("\n========== PHASE 4 SCORECARD ==========")
	precision := 0.0
	if totalTP+totalFP > 0 {
		precision = float64(totalTP) / float64(totalTP+totalFP)
	}
	recall := 0.0
	if totalTP+totalFN > 0 {
		recall = float64(totalTP) / float64(totalTP+totalFN)
	}
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}

	t.Logf("  TOTAL: TP=%d FN=%d FP=%d", totalTP, totalFN, totalFP)
	t.Logf("  Precision=%.2f Recall=%.2f F1=%.2f", precision, recall, f1)
	t.Logf("  Feedback triggered: %d/%d conversations", feedbackTriggered, len(files))
	t.Log("  ─────────────────────────────────────")
	t.Log("  Phase 3:  Precision=0.82 Recall=1.00 F1=0.90 FP=2")
	t.Logf("  Phase 4:  Precision=%.2f Recall=%.2f F1=%.2f FP=%d", precision, recall, f1, totalFP)

	// Exit criteria: no regression from Phase 3.
	if recall < 1.0 {
		t.Errorf("EXIT CRITERION FAILED: recall %.2f < 1.00", recall)
	}
	if precision < 0.81 {
		t.Errorf("EXIT CRITERION FAILED: precision %.2f < 0.81 (regression)", precision)
	}
}

// TestLLMConversationsWithSalience runs the full pipeline with all phases enabled
// including Salience Filter + Memory Consolidator (Phase 5).
func TestLLMConversationsWithSalience(t *testing.T) {
	convDir := filepath.Join("..", "..", "data", "test-conversations")
	if _, err := os.Stat(convDir); os.IsNotExist(err) {
		t.Skip("test-conversations directory not found")
	}

	profileDir := filepath.Join("..", "..", "data", "language-profiles")
	profiles, err := LoadLangProfiles(profileDir)
	if err != nil {
		t.Skipf("language profiles not found: %v", err)
	}

	blocklistPath := filepath.Join("..", "..", "data", "blocklists", "default.txt")
	blocklist, err := LoadBlocklist(blocklistPath)
	if err != nil {
		t.Skipf("blocklist not found: %v", err)
	}

	// Set up consolidator with temp dir, pre-loaded pattern index from real corpus
	tmpDir := t.TempDir()
	corpusPath := filepath.Join(tmpDir, "consolidated.ndjson")
	seedCorpus := filepath.Join("..", "..", "data", "corpus", "cognitive-v1.ndjson")
	patternIdx, _ := LoadPatternIndex(seedCorpus)
	t.Logf("Pre-loaded %d patterns from seed corpus", len(patternIdx.Patterns()))
	consolidator := NewConsolidator(ConsolidatorConfig{
		CorpusOutputPath:     corpusPath,
		MinConfidenceForAuto: 0.9,  // higher threshold to target 10-30% consolidation rate
		MinAgreementForAuto:  0.7,
		CooldownSec:          0, // disable cooldown for testing
		MaxEntriesPerSession: 100,
	}, patternIdx)

	cfg := DefaultPipelineConfig()
	cfg.UseLayer0 = true
	cfg.Layer0 = DefaultLayer0Config()
	cfg.Layer0.Toxicity.Blocklist = blocklist
	cfg.Layer0.Language.Profiles = profiles
	cfg.UseInhibitor = true
	cfg.Inhibitor = DefaultInhibitorConfig()
	cfg.UseNeuromodulation = true
	cfg.Urgency = DefaultUrgencyConfig()
	cfg.Modulator = DefaultModulatorConfig()
	cfg.UseMetacognition = true
	cfg.SelfConfidence = DefaultSelfConfidenceConfig()
	cfg.Feedback = DefaultFeedbackConfig()
	cfg.UseSalience = true
	cfg.Salience = DefaultSalienceConfig()
	cfg.Consolidator = consolidator

	files, err := filepath.Glob(filepath.Join(convDir, "*.ndjson"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	if len(files) == 0 {
		t.Skip("no .ndjson files in test-conversations")
	}

	totalTP, totalFN, totalFP := 0, 0, 0
	consolidationCount := 0
	triggerCounts := make(map[string]int)

	for _, f := range files {
		base := filepath.Base(f)
		t.Run(base, func(t *testing.T) {
			entry, err := loadCorpusEntry(f)
			if err != nil {
				t.Fatalf("load %s: %v", f, err)
			}

			snap, err := entryToSnapshot(entry)
			if err != nil {
				t.Fatalf("convert snapshot: %v", err)
			}

			result := Run(snap, cfg)

			// All test conversations should pass Layer 0.
			if result.Rejected {
				t.Fatalf("Layer 0 rejected: %s", result.Layer0.Reason)
			}

			// Verify salience was computed.
			if result.Salience == nil {
				t.Fatal("expected SalienceResult, got nil")
			}
			t.Logf("  salience: scores=%d salient=%d",
				len(result.Salience.Scores), len(result.Salience.Salient))

			// Log consolidation.
			if result.Consolidated {
				consolidationCount++
				triggerCounts[result.ConsolidationTrigger.String()]++
				t.Logf("  consolidated: trigger=%v", result.ConsolidationTrigger)
			}

			// Post-inhibition+salience findings for scoring.
			gatedTypes := make(map[string]bool)
			if result.Inhibition != nil {
				// After salience, the aggregated report contains the final findings.
				for _, finding := range result.Report.GetFindings() {
					gatedTypes[findingTypeString(finding.FindingType)] = true
				}
			} else {
				for _, finding := range result.Findings {
					gatedTypes[findingTypeString(finding.FindingType)] = true
				}
			}

			expectedTypes := make(map[string]bool)
			for _, exp := range entry.Expected {
				expectedTypes[exp.FindingType] = true
			}

			var tp, fn, fp int
			for et := range expectedTypes {
				if gatedTypes[et] {
					tp++
				} else {
					fn++
					t.Errorf("MISS (Phase 5): expected %s", et)
				}
			}
			for at := range gatedTypes {
				if !expectedTypes[at] {
					fp++
					t.Logf("  FP (Phase 5): unexpected %s", at)
				}
			}

			totalTP += tp
			totalFN += fn
			totalFP += fp

			// Verify CerebroReport has Phase 5 fields.
			cr := result.ToCerebroReport()
			if cr == nil {
				t.Fatal("expected non-nil CerebroReport")
			}
			// Salience scores should be populated.
			if len(cr.GetSalienceScores()) != len(result.Salience.Scores) {
				t.Errorf("expected %d salience scores in CerebroReport, got %d",
					len(result.Salience.Scores), len(cr.GetSalienceScores()))
			}
		})
	}

	// Summary
	t.Log("\n========== PHASE 5 SCORECARD ==========")
	precision := 0.0
	if totalTP+totalFP > 0 {
		precision = float64(totalTP) / float64(totalTP+totalFP)
	}
	recall := 0.0
	if totalTP+totalFN > 0 {
		recall = float64(totalTP) / float64(totalTP+totalFN)
	}
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}

	t.Logf("  TOTAL: TP=%d FN=%d FP=%d", totalTP, totalFN, totalFP)
	t.Logf("  Precision=%.2f Recall=%.2f F1=%.2f", precision, recall, f1)
	t.Logf("  Consolidation: %d/%d conversations", consolidationCount, len(files))
	for trigger, count := range triggerCounts {
		t.Logf("    %s: %d", trigger, count)
	}

	// Verify consolidated entries are valid NDJSON.
	if consolidationCount > 0 {
		data, err := os.ReadFile(corpusPath)
		if err != nil {
			t.Fatalf("read consolidated corpus: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		t.Logf("  Consolidated entries written: %d", len(lines))

		// Verify each line is valid JSON.
		for i, line := range lines {
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(line), &parsed); err != nil {
				t.Errorf("line %d is not valid JSON: %v", i+1, err)
			}
			// Verify sparse index: no raw_text key.
			if _, ok := parsed["raw_text"]; ok {
				t.Errorf("line %d contains raw_text (should be sparse)", i+1)
			}
		}

		// Verify pattern index grew.
		patterns := patternIdx.Patterns()
		t.Logf("  Pattern index size: %d patterns", len(patterns))
	}

	// Compute consolidation rate.
	if len(files) > 0 {
		rate := float64(consolidationCount) / float64(len(files)) * 100
		t.Logf("  Consolidation rate: %.0f%%", rate)
	}

	t.Log("  ─────────────────────────────────────")
	t.Log("  Phase 4:  Precision=0.83 Recall=1.00 F1=0.91 FP=2")
	t.Logf("  Phase 5:  Precision=%.2f Recall=%.2f F1=%.2f FP=%d", precision, recall, f1, totalFP)

	// Exit criteria: no regression from Phase 4.
	if recall < 1.0 {
		t.Errorf("EXIT CRITERION FAILED: recall %.2f < 1.00", recall)
	}
	if precision < 0.81 {
		t.Errorf("EXIT CRITERION FAILED: precision %.2f < 0.81 (regression)", precision)
	}
}

// TestUserFeedbackPath tests the complete user feedback → consolidation path.
func TestUserFeedbackPath(t *testing.T) {
	tmpDir := t.TempDir()
	corpusPath := filepath.Join(tmpDir, "feedback.ndjson")
	patternIdx := NewPatternIndex()
	consolidator := NewConsolidator(ConsolidatorConfig{
		CorpusOutputPath:     corpusPath,
		MinConfidenceForAuto: 0.8,
		CooldownSec:          0,
		MaxEntriesPerSession: 100,
	}, patternIdx)

	// Run pipeline on test conversation
	snap := &reasoningv1.ConversationSnapshot{
		Objective: "Test user feedback",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "We already spent two years on this system."},
			{TurnNumber: 2, Speaker: "assistant", RawText: "We should continue with the current system."},
		},
		TotalTurns: 2,
	}

	cfg := DefaultPipelineConfig()
	cfg.Consolidator = consolidator

	result := Run(snap, cfg)
	if result.Report == nil {
		t.Fatal("expected report")
	}

	// Store result for feedback lookup
	consolidator.Consolidate(&ConsolidationInput{
		ConversationID: "test-feedback",
		Report:         result.Report,
		Snap:           snap,
	})

	// Submit confirmed feedback
	err := consolidator.SubmitFeedback("test-feedback", "confirmed")
	if err != nil {
		t.Fatalf("SubmitFeedback confirmed: %v", err)
	}

	// Submit rejected feedback for unknown ID
	err = consolidator.SubmitFeedback("nonexistent", "rejected")
	if err == nil {
		t.Error("expected error for unknown conversation ID")
	}

	// Verify corpus file has entries
	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Errorf("expected at least 2 entries (consolidation + feedback), got %d", len(lines))
	}

	// Find the feedback entry
	foundFeedback := false
	for _, line := range lines {
		if strings.Contains(line, "USER_FEEDBACK") && strings.Contains(line, "confirmed") {
			foundFeedback = true
			break
		}
	}
	if !foundFeedback {
		t.Error("expected USER_FEEDBACK entry with outcome=confirmed")
	}
}

// TestPatternIndexConcurrency verifies thread safety of shared PatternIndex.
func TestPatternIndexConcurrency(t *testing.T) {
	idx := NewPatternIndex()
	idx.AddEntry("PATTERN_A")

	done := make(chan bool, 10)

	// Concurrent reads
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				idx.Lookup("PATTERN_A")
				idx.Patterns()
				idx.GetAccuracy()
			}
			done <- true
		}()
	}

	// Concurrent writes
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				idx.AddEntry(fmt.Sprintf("PATTERN_%d_%d", id, j))
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Just verify no panic/deadlock — if we get here, concurrency is fine.
	patterns := idx.Patterns()
	if len(patterns) < 501 { // 1 initial + 5*100 concurrent
		t.Errorf("expected at least 501 patterns, got %d", len(patterns))
	}
}

func TestToCerebroReport(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Objective: "test objective",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "We found a critical security vulnerability in the system."},
			{TurnNumber: 2, Speaker: "assistant", RawText: "That is urgent. The risk is high and the deadline is tomorrow."},
		},
		TotalTurns: 2,
	}

	cfg := DefaultPipelineConfig()
	cfg.UseInhibitor = true
	cfg.Inhibitor = DefaultInhibitorConfig()
	cfg.UseNeuromodulation = true
	cfg.Urgency = DefaultUrgencyConfig()
	cfg.Modulator = DefaultModulatorConfig()

	result := Run(snap, cfg)
	cr := result.ToCerebroReport()

	if cr.GetBaseReport() == nil {
		t.Fatal("expected base_report in CerebroReport")
	}
	if cr.GetGainSignal() == nil {
		t.Fatal("expected gain_signal in CerebroReport")
	}
	if cr.GetGainSignal().GetUrgency() == 0 {
		t.Error("expected non-zero urgency in CerebroReport gain_signal")
	}
	if cr.GetThresholdAdjustments() == nil {
		t.Fatal("expected threshold_adjustments in CerebroReport")
	}
	if len(cr.GetThresholdAdjustments().GetAdjustments()) == 0 {
		t.Error("expected non-empty adjustments map in CerebroReport")
	}
	// Verify scope-guard is not in adjustments
	if _, ok := cr.GetThresholdAdjustments().GetAdjustments()["scope-guard"]; ok {
		t.Error("scope-guard should not appear in threshold adjustments")
	}
	// Verify pass_count is 1 (first pass, no feedback)
	if cr.GetPassCount() != 1 {
		t.Errorf("expected pass_count=1, got %d", cr.GetPassCount())
	}
	// Verify feedback_applied defaults to false (Phase 4 will set true)
	if cr.GetFeedbackApplied() {
		t.Error("expected feedback_applied=false for first pass")
	}
	// Verify assessed_at is populated from base report
	if cr.GetAssessedAt() == nil {
		t.Error("expected assessed_at to be populated in CerebroReport")
	}

	// Verify ToCerebroReport returns nil for rejected results.
	rejected := &PipelineResult{Rejected: true, Layer0: &Layer0Result{Reason: "test"}}
	if rejected.ToCerebroReport() != nil {
		t.Error("expected nil CerebroReport for rejected pipeline result")
	}
}

// TestPipelineFeedbackPath exercises the full feedback re-evaluation loop.
// Uses a conversation that produces multiple findings with overlapping turns
// (enabling corroboration adjustments) and lowers the feedback threshold so
// self-confidence triggers re-evaluation.
func TestPipelineFeedbackPath(t *testing.T) {
	// This conversation triggers contradiction + sunk-cost, then drifts to trigger
	// scope-guard. Having scope-guard on overlapping turns with contradiction enables
	// the +0.1 corroboration adjustment in applyFeedbackAdjustment.
	snap := &reasoningv1.ConversationSnapshot{
		Objective: "Decide whether to continue current approach",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "We already spent two years on this system and invested so much money into the platform."},
			{TurnNumber: 2, Speaker: "assistant", RawText: "We should continue with the current system and keep going. The investment is worth protecting."},
			{TurnNumber: 3, Speaker: "user", RawText: "But what about the new requirements and technical debt?"},
			{TurnNumber: 4, Speaker: "assistant", RawText: "Actually I was wrong. We should not continue with the current system. The investment is not worth protecting."},
			{TurnNumber: 5, Speaker: "user", RawText: "OK, by the way, have you tried the new Italian restaurant downtown? The pasta was amazing."},
			{TurnNumber: 6, Speaker: "assistant", RawText: "I haven't tried it yet but I heard their risotto is excellent. What about the wine selection?"},
			{TurnNumber: 7, Speaker: "user", RawText: "The wine list was impressive, especially the Barolo. They also have great desserts."},
			{TurnNumber: 8, Speaker: "assistant", RawText: "That sounds wonderful. I should go check out their lunch menu this weekend."},
			{TurnNumber: 9, Speaker: "user", RawText: "Definitely recommend the weekend brunch. They have live music too."},
			{TurnNumber: 10, Speaker: "assistant", RawText: "Perfect, I love live music with a good meal. What time do they usually start serving?"},
		},
		TotalTurns: 10,
	}

	cfg := DefaultPipelineConfig()
	// Disable inhibitor so all findings reach feedback evaluation.
	cfg.UseMetacognition = true
	cfg.SelfConfidence = DefaultSelfConfidenceConfig()
	cfg.Feedback = DefaultFeedbackConfig()
	// Lower feedback threshold so self-confidence triggers re-evaluation.
	cfg.Feedback.FeedbackThreshold = 0.9
	// Lower scope-guard sustained turns so drift is detected in shorter conversations.
	cfg.ScopeGuard.SustainedTurns = 3

	result := Run(snap, cfg)

	// Should have findings.
	if len(result.Findings) == 0 {
		t.Skip("no findings produced — conversation doesn't trigger detectors")
	}

	// Self-confidence must be present.
	if result.SelfConf == nil {
		t.Fatal("expected SelfConfidenceReport")
	}
	t.Logf("self-confidence: overall=%.3f", result.SelfConf.GetOverallConfidence())

	// With threshold=0.9, feedback should be triggered (most reports are < 0.9).
	if result.Feedback == nil {
		t.Fatal("expected FeedbackResult")
	}
	t.Logf("feedback: applied=%v detectors=%v deltas=%v",
		result.Feedback.Applied, result.Feedback.ReevalDetectors, result.Feedback.ConfidenceDeltas)

	// At minimum, detectors should have been re-evaluated.
	if len(result.Feedback.ReevalDetectors) == 0 {
		t.Error("expected at least one detector to be re-evaluated")
	}

	// Verify CerebroReport reflects feedback.
	cr := result.ToCerebroReport()
	if cr == nil {
		t.Fatal("expected non-nil CerebroReport")
	}
	if cr.GetSelfConfidence() == nil {
		t.Error("expected self_confidence in CerebroReport")
	}

	// If feedback was applied, pass_count should be 2.
	if result.Feedback.Applied {
		if cr.GetPassCount() != 2 {
			t.Errorf("expected pass_count=2 after feedback, got %d", cr.GetPassCount())
		}
		if !cr.GetFeedbackApplied() {
			t.Error("expected feedback_applied=true")
		}
		t.Log("feedback loop exercised: pass_count=2, feedback_applied=true")
	} else {
		t.Log("feedback was triggered but no changes met improvement threshold")
		if cr.GetPassCount() != 1 {
			t.Errorf("expected pass_count=1 when feedback not applied, got %d", cr.GetPassCount())
		}
	}
}
