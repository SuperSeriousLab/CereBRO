// Tests for RunAdaptive — domain-adaptive variant selection.
//
// Covers:
//   1. Classical domain (confidence 0.8) → E-pre-cortex variant selected
//   2. Modern domain (confidence 0.9)    → D-inhibitor-only variant selected
//   3. Nil domain                        → D-inhibitor-only (safe default)
//   4. Classical but low confidence (0.3) → D-inhibitor-only
//   5. Comparison table: 3 modern + 3 classical through RunAdaptive,
//      verify adaptive matches standalone D on modern AND standalone E on classical
package pipeline

import (
	"path/filepath"
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// makeMinimalSnap returns a well-formed ConversationSnapshot suitable for
// variant-selection tests that don't require realistic finding patterns.
func makeMinimalSnap() *reasoningv1.ConversationSnapshot {
	return &reasoningv1.ConversationSnapshot{
		Objective:  "evaluate options",
		TotalTurns: 3,
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "I think option A is clearly correct."},
			{TurnNumber: 2, Speaker: "assistant", RawText: "Yes, let us proceed with option A."},
			{TurnNumber: 3, Speaker: "user", RawText: "Good. We always go with what has worked before."},
		},
		ObjectiveKeywords: []string{"option"},
	}
}

// stageCountForConfig counts the number of active pipeline stages in a config.
// This mirrors the StageCount values defined in the VariantInfo structs and lets
// tests confirm which variant was effectively chosen without requiring access to
// the config itself.
//
// D-inhibitor-only: 5 stages  (intake → router → detectors → inhibitor → aggregator)
// E-pre-cortex:     4 stages  (intake → router → detectors → aggregator)
func inferStageCount(cfg PipelineConfig) int {
	count := 3 // intake + router + detectors always present
	count++    // aggregator always present
	if cfg.UseLayer0 {
		count++
	}
	if cfg.UseInhibitor {
		count++
	}
	if cfg.UseNeuromodulation {
		count += 2 // urgency + modulator
	}
	if cfg.UseSalience {
		count++
	}
	if cfg.UseMetacognition {
		count += 2 // self-confidence + feedback
	}
	if cfg.MLEnricher.Enabled {
		count++
	}
	if cfg.Consolidator != nil {
		count++
	}
	return count
}

// ─── variant-selection tests ──────────────────────────────────────────────────

// TestRunAdaptive_classicalHighConfidence verifies that classical text with
// confidence > 0.6 routes to E-pre-cortex (4 stages, no inhibitor).
func TestRunAdaptive_classicalHighConfidence(t *testing.T) {
	snap := makeMinimalSnap()
	dc := &DomainContext{
		TextEra:       "classical",
		PrimaryDomain: "philosophy",
		Confidence:    0.8,
	}

	name := AdaptiveVariantName(dc)
	if name != "E-pre-cortex" {
		t.Errorf("classical confidence=0.8: want E-pre-cortex, got %s", name)
	}

	result, err := RunAdaptive(snap, dc, "")
	if err != nil {
		t.Fatalf("RunAdaptive error: %v", err)
	}
	if result == nil {
		t.Fatal("RunAdaptive returned nil result")
	}

	// E-pre-cortex has no inhibitor — Inhibition field must be nil.
	if result.Inhibition != nil {
		t.Error("classical → E-pre-cortex: Inhibition should be nil (no inhibitor stage)")
	}
}

// TestRunAdaptive_modernHighConfidence verifies that modern text routes to
// D-inhibitor-only (inhibitor stage present).
func TestRunAdaptive_modernHighConfidence(t *testing.T) {
	snap := makeMinimalSnap()
	dc := &DomainContext{
		TextEra:       "modern",
		PrimaryDomain: "technical",
		Confidence:    0.9,
	}

	name := AdaptiveVariantName(dc)
	if name != "D-inhibitor-only" {
		t.Errorf("modern confidence=0.9: want D-inhibitor-only, got %s", name)
	}

	result, err := RunAdaptive(snap, dc, "")
	if err != nil {
		t.Fatalf("RunAdaptive error: %v", err)
	}
	if result == nil {
		t.Fatal("RunAdaptive returned nil result")
	}

	// D-inhibitor-only always runs the inhibitor, so Inhibition must be non-nil.
	if result.Inhibition == nil {
		t.Error("modern → D-inhibitor-only: Inhibition should be non-nil (inhibitor stage active)")
	}
}

// TestRunAdaptive_nilDomain verifies nil domain falls back to D-inhibitor-only.
func TestRunAdaptive_nilDomain(t *testing.T) {
	snap := makeMinimalSnap()

	name := AdaptiveVariantName(nil)
	if name != "D-inhibitor-only" {
		t.Errorf("nil domain: want D-inhibitor-only, got %s", name)
	}

	result, err := RunAdaptive(snap, nil, "")
	if err != nil {
		t.Fatalf("RunAdaptive error: %v", err)
	}
	if result == nil {
		t.Fatal("RunAdaptive returned nil result")
	}

	// Inhibitor must be active (D variant).
	if result.Inhibition == nil {
		t.Error("nil domain → D-inhibitor-only: Inhibition should be non-nil")
	}
}

// TestRunAdaptive_classicalLowConfidence verifies that classical text with
// confidence ≤ 0.6 falls through to D-inhibitor-only.
func TestRunAdaptive_classicalLowConfidence(t *testing.T) {
	snap := makeMinimalSnap()
	dc := &DomainContext{
		TextEra:       "classical",
		PrimaryDomain: "philosophy",
		Confidence:    0.3,
	}

	name := AdaptiveVariantName(dc)
	if name != "D-inhibitor-only" {
		t.Errorf("classical confidence=0.3: want D-inhibitor-only, got %s", name)
	}

	result, err := RunAdaptive(snap, dc, "")
	if err != nil {
		t.Fatalf("RunAdaptive error: %v", err)
	}
	if result == nil {
		t.Fatal("RunAdaptive returned nil result")
	}

	if result.Inhibition == nil {
		t.Error("low-confidence classical → D-inhibitor-only: Inhibition should be non-nil")
	}
}

// TestRunAdaptive_nilSnap verifies a nil snapshot returns an error.
func TestRunAdaptive_nilSnap(t *testing.T) {
	_, err := RunAdaptive(nil, nil, "")
	if err == nil {
		t.Fatal("expected error for nil snap, got nil")
	}
}

// ─── comparison table ─────────────────────────────────────────────────────────

// TestRunAdaptive_ComparisonTable runs 3 modern + 3 classical conversations
// through RunAdaptive and verifies:
//   - Modern results match standalone D-inhibitor-only (same findings)
//   - Classical results match standalone E-pre-cortex (same findings)
//
// This test requires the test-conversations and classical corpus to be present;
// it is skipped if they are absent.
func TestRunAdaptive_ComparisonTable(t *testing.T) {
	modernDir := filepath.Join("..", "..", "data", "test-conversations")
	classicalPath := filepath.Join("..", "..", "data", "library", "corpus", "classical-v1.ndjson")

	modernEntries := loadCompetitionEntries(t, modernDir)
	if len(modernEntries) == 0 {
		t.Skip("no modern test conversations found")
	}
	// Use at most 3 modern entries.
	if len(modernEntries) > 3 {
		modernEntries = modernEntries[:3]
	}

	classicalEntries := loadNDJSONAllLines(t, classicalPath)
	if len(classicalEntries) == 0 {
		t.Skip("no classical corpus entries found")
	}
	// Use at most 3 classical entries.
	if len(classicalEntries) > 3 {
		classicalEntries = classicalEntries[:3]
	}

	classicalDC := &DomainContext{
		TextEra:       "classical",
		PrimaryDomain: "philosophy",
		Confidence:    0.85,
	}

	dCfg := InhibitorOnlyConfig()
	eCfg := PreCortexConfig()

	t.Log("\n========== RunAdaptive COMPARISON TABLE ==========")
	t.Log("")
	t.Logf("%-36s  %-14s  %-14s  %s", "Entry", "Adaptive", "Standalone", "Match?")
	t.Log(horizontalRule(70))

	allMatch := true

	// Modern: adaptive should select D-inhibitor-only; compare with standalone D.
	for _, entry := range modernEntries {
		adaptiveResult, err := RunAdaptive(entry.Snap, nil, "") // nil → modern default
		if err != nil {
			t.Fatalf("RunAdaptive modern error: %v", err)
		}
		standaloneResult := Run(entry.Snap, dCfg)

		adaptiveFindings := findingSet(adaptiveResult)
		standaloneFindings := findingSet(standaloneResult)

		match := setsEqual(adaptiveFindings, standaloneFindings)
		if !match {
			allMatch = false
		}
		t.Logf("%-36s  %-14s  %-14s  %v  [adaptive=%v standalone=%v]",
			entry.ID, "D-inhibitor-only", "D-inhibitor-only", yesNo(match),
			sortedKeys(adaptiveFindings), sortedKeys(standaloneFindings))
	}

	// Classical: adaptive should select E-pre-cortex; compare with standalone E + domain.
	eCfgWithDomain := eCfg
	eCfgWithDomain.DomainContext = classicalDC

	for _, entry := range classicalEntries {
		adaptiveResult, err := RunAdaptive(entry.Snap, classicalDC, "")
		if err != nil {
			t.Fatalf("RunAdaptive classical error: %v", err)
		}
		standaloneResult := Run(entry.Snap, eCfgWithDomain)

		adaptiveFindings := findingSet(adaptiveResult)
		standaloneFindings := findingSet(standaloneResult)

		match := setsEqual(adaptiveFindings, standaloneFindings)
		if !match {
			allMatch = false
		}
		t.Logf("%-36s  %-14s  %-14s  %v  [adaptive=%v standalone=%v]",
			entry.ID, "E-pre-cortex", "E-pre-cortex", yesNo(match),
			sortedKeys(adaptiveFindings), sortedKeys(standaloneFindings))
	}

	t.Log("")
	if allMatch {
		t.Log("All entries: adaptive matches standalone variant. PASS")
	} else {
		t.Error("Some entries: adaptive result diverged from expected standalone variant.")
	}
}

// ─── F1 regression: modern must stay >= 0.909 ─────────────────────────────────

// TestRunAdaptive_ModernF1Regression runs all 9 modern test conversations
// through RunAdaptive (nil domain → D-inhibitor-only) and asserts F1 >= 0.909.
func TestRunAdaptive_ModernF1Regression(t *testing.T) {
	modernDir := filepath.Join("..", "..", "data", "test-conversations")

	entries := loadCompetitionEntries(t, modernDir)
	if len(entries) == 0 {
		t.Skip("no modern test conversations found")
	}

	var totalTP, totalFP, totalFN int
	for _, entry := range entries {
		result, err := RunAdaptive(entry.Snap, nil, "")
		if err != nil {
			t.Fatalf("RunAdaptive error: %v", err)
		}

		actualTypes := findingSet(result)
		// Post-inhibition findings take precedence (inhibitor is active in D variant).
		if result.Inhibition != nil && result.Report != nil {
			actualTypes = make(map[string]bool)
			for _, f := range result.Report.GetFindings() {
				actualTypes[FindingTypeString(f.FindingType)] = true
			}
		}

		expectedTypes := make(map[string]bool)
		for _, et := range entry.Expected {
			expectedTypes[et] = true
		}

		for et := range expectedTypes {
			if actualTypes[et] {
				totalTP++
			} else {
				totalFN++
			}
		}
		for at := range actualTypes {
			if !expectedTypes[at] {
				totalFP++
			}
		}
	}

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

	t.Logf("RunAdaptive modern F1=%.3f (precision=%.3f recall=%.3f TP=%d FP=%d FN=%d)",
		f1, precision, recall, totalTP, totalFP, totalFN)

	if f1 < 0.909 {
		t.Errorf("REGRESSION: RunAdaptive modern F1=%.3f, must be >= 0.909", f1)
	}
}

// ─── small utilities ──────────────────────────────────────────────────────────

// findingSet returns a set of finding-type strings from a PipelineResult.
func findingSet(r *PipelineResult) map[string]bool {
	out := make(map[string]bool)
	if r == nil {
		return out
	}
	for _, f := range r.Findings {
		out[FindingTypeString(f.FindingType)] = true
	}
	return out
}

// setsEqual returns true when two string-bool maps have identical key sets.
func setsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// sortedKeys returns the keys of a map sorted for deterministic output.
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort — small sets only.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func yesNo(b bool) string {
	if b {
		return "YES"
	}
	return "NO "
}

func horizontalRule(n int) string {
	r := make([]byte, n)
	for i := range r {
		r[i] = '-'
	}
	return string(r)
}
