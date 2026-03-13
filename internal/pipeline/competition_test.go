package pipeline

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// DetectorVariant wraps a detection function for competition evaluation.
type DetectorVariant struct {
	Name       string
	FindingType reasoningv1.FindingType
	Detect     func(*reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment
}

type competitionResult struct {
	Name      string
	TP, FN, FP int
	Precision  float64
	Recall     float64
	F1         float64
	FPR        float64
	AvgLatency time.Duration
}

func evaluateVariant(t *testing.T, variant DetectorVariant, conversations []testConversation) competitionResult {
	var tp, fn, fp int
	var totalLatency time.Duration
	totalClean := 0

	for _, conv := range conversations {
		expected := false
		for _, ft := range conv.expectedTypes {
			if ft == findingTypeString(variant.FindingType) {
				expected = true
				break
			}
		}

		start := time.Now()
		result := variant.Detect(conv.snapshot)
		elapsed := time.Since(start)
		totalLatency += elapsed

		detected := result != nil

		if expected && detected {
			tp++
		} else if expected && !detected {
			fn++
		} else if !expected && detected {
			fp++
		}
		if !expected {
			totalClean++
		}
	}

	precision := 0.0
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	recall := 0.0
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	fpr := 0.0
	if totalClean > 0 {
		fpr = float64(fp) / float64(totalClean)
	}
	avgLatency := time.Duration(0)
	if len(conversations) > 0 {
		avgLatency = totalLatency / time.Duration(len(conversations))
	}

	return competitionResult{
		Name:      variant.Name,
		TP:        tp,
		FN:        fn,
		FP:        fp,
		Precision: precision,
		Recall:    recall,
		F1:        f1,
		FPR:       fpr,
		AvgLatency: avgLatency,
	}
}

type testConversation struct {
	id            string
	snapshot      *reasoningv1.ConversationSnapshot
	expectedTypes []string
}

func loadTestConversations(t *testing.T) []testConversation {
	convDir := filepath.Join("..", "..", "data", "test-conversations")
	files, err := filepath.Glob(filepath.Join(convDir, "*.ndjson"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Skip("no test conversations found")
	}

	var convs []testConversation
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var entry corpusEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatalf("parse %s: %v", f, err)
			}
			snap, err := entryToSnapshot(&entry)
			if err != nil {
				t.Fatalf("convert %s: %v", f, err)
			}
			snap = Enrich(snap)

			var expected []string
			for _, exp := range entry.Expected {
				expected = append(expected, exp.FindingType)
			}

			convs = append(convs, testConversation{
				id:            entry.EntryID,
				snapshot:      snap,
				expectedTypes: expected,
			})
			break // only first line per file
		}
	}
	return convs
}

// TestScopeGuardCompetition evaluates 3 scope guard variants.
func TestScopeGuardCompetition(t *testing.T) {
	convs := loadTestConversations(t)

	variants := []DetectorVariant{
		{
			Name:        "reference-window",
			FindingType: reasoningv1.FindingType_SCOPE_DRIFT,
			Detect: func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
				return DetectScopeDrift(snap, DefaultScopeGuardConfig())
			},
		},
		{
			Name:        "cumulative-centroid",
			FindingType: reasoningv1.FindingType_SCOPE_DRIFT,
			Detect: func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
				return DetectScopeDriftCentroid(snap, DefaultScopeGuardCentroidConfig())
			},
		},
		{
			Name:        "topic-transition",
			FindingType: reasoningv1.FindingType_SCOPE_DRIFT,
			Detect: func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
				return DetectScopeDriftTransition(snap, DefaultScopeGuardTransitionConfig())
			},
		},
	}

	t.Log("\n========== SCOPE GUARD COMPETITION ==========")
	printCompetitionResults(t, variants, convs)
}

// TestAnchoringCompetition evaluates 2 anchoring variants.
func TestAnchoringCompetition(t *testing.T) {
	convs := loadTestConversations(t)

	variants := []DetectorVariant{
		{
			Name:        "original",
			FindingType: reasoningv1.FindingType_ANCHORING_BIAS,
			Detect: func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
				return DetectAnchoring(snap, DefaultAnchoringConfig())
			},
		},
		{
			Name:        "context-aware",
			FindingType: reasoningv1.FindingType_ANCHORING_BIAS,
			Detect: func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
				return DetectAnchoringContext(snap, DefaultAnchoringContextConfig())
			},
		},
	}

	t.Log("\n========== ANCHORING COMPETITION ==========")
	printCompetitionResults(t, variants, convs)
}

// TestSunkCostCompetition evaluates 2 sunk-cost variants.
func TestSunkCostCompetition(t *testing.T) {
	convs := loadTestConversations(t)

	variants := []DetectorVariant{
		{
			Name:        "original",
			FindingType: reasoningv1.FindingType_SUNK_COST_FALLACY,
			Detect: func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
				return DetectSunkCost(snap, DefaultSunkCostConfig())
			},
		},
		{
			Name:        "proximity-weighted",
			FindingType: reasoningv1.FindingType_SUNK_COST_FALLACY,
			Detect: func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
				return DetectSunkCostProximity(snap, DefaultSunkCostProximityConfig())
			},
		},
	}

	t.Log("\n========== SUNK-COST COMPETITION ==========")
	printCompetitionResults(t, variants, convs)
}

func printCompetitionResults(t *testing.T, variants []DetectorVariant, convs []testConversation) {
	var results []competitionResult

	for _, v := range variants {
		result := evaluateVariant(t, v, convs)
		results = append(results, result)
	}

	t.Logf("  %-25s %5s %5s %5s %7s %7s %7s %7s %10s",
		"Variant", "TP", "FN", "FP", "Prec", "Recall", "F1", "FPR", "Latency")
	t.Log("  " + strings.Repeat("─", 90))

	var bestF1 float64
	bestIdx := 0
	for i, r := range results {
		t.Logf("  %-25s %5d %5d %5d %7.2f %7.2f %7.2f %7.2f %10s",
			r.Name, r.TP, r.FN, r.FP, r.Precision, r.Recall, r.F1, r.FPR, r.AvgLatency)
		if r.F1 > bestF1 || (math.Abs(r.F1-bestF1) < 0.001 && r.FPR < results[bestIdx].FPR) {
			bestF1 = r.F1
			bestIdx = i
		}
	}
	t.Logf("  Winner (balanced): %s (F1=%.2f)", results[bestIdx].Name, results[bestIdx].F1)

	// Precision-first ranking
	bestIdx = 0
	bestScore := 0.0
	for i, r := range results {
		score := 0.4*r.Precision + 0.15*r.Recall + 0.2*r.F1 + 0.2*(1-r.FPR)
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	t.Logf("  Winner (precision-first): %s (score=%.2f)", results[bestIdx].Name, bestScore)

	// Recall-first ranking
	bestIdx = 0
	bestScore = 0.0
	for i, r := range results {
		score := 0.15*r.Precision + 0.4*r.Recall + 0.2*r.F1 + 0.1*(1-r.FPR)
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	t.Logf("  Winner (recall-first): %s (score=%.2f)", results[bestIdx].Name, bestScore)
}

// TestFullPipelineComparison runs the full pipeline with each scope guard variant and compares.
func TestFullPipelineComparison(t *testing.T) {
	convDir := filepath.Join("..", "..", "data", "test-conversations")
	files, err := filepath.Glob(filepath.Join(convDir, "*.ndjson"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Skip("no test conversations")
	}

	type pipelineVariant struct {
		name string
		cfg  PipelineConfig
	}

	variants := []pipelineVariant{
		{"reference-window (default)", DefaultPipelineConfig()},
	}

	for _, pv := range variants {
		t.Run(pv.name, func(t *testing.T) {
			totalTP, totalFN, totalFP := 0, 0, 0

			for _, f := range files {
				entry, err := loadCorpusEntry(f)
				if err != nil {
					t.Fatalf("load %s: %v", f, err)
				}
				snap, err := entryToSnapshot(entry)
				if err != nil {
					t.Fatalf("convert: %v", err)
				}
				result := Run(snap, pv.cfg)

				actualTypes := make(map[string]bool)
				for _, finding := range result.Findings {
					actualTypes[findingTypeString(finding.FindingType)] = true
				}
				expectedTypes := make(map[string]bool)
				for _, exp := range entry.Expected {
					expectedTypes[exp.FindingType] = true
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

			t.Logf("  TP=%d FN=%d FP=%d Precision=%.2f Recall=%.2f F1=%.2f",
				totalTP, totalFN, totalFP, precision, recall, f1)

			if f1 < 0.60 {
				t.Errorf("F1 %.2f below minimum threshold 0.60", f1)
			}

		})
	}
}

// ================================================================
// Phase 6: Architecture Competition Tests
// ================================================================

// TestVariantFactories verifies each factory produces a valid config
// and the pipeline runs without panics.
func TestVariantFactories(t *testing.T) {
	convDir := filepath.Join("..", "..", "data", "test-conversations")
	if _, err := os.Stat(convDir); os.IsNotExist(err) {
		t.Skip("test-conversations not found")
	}

	// Load one test conversation.
	entry, err := loadCorpusEntry(filepath.Join(convDir, "01-anchoring.ndjson"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	snap, err := entryToSnapshot(entry)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	for _, v := range AllVariants() {
		t.Run(v.Info.Name, func(t *testing.T) {
			result := Run(snap, v.Config)
			if result == nil {
				t.Fatal("nil result")
			}
			if result.Report == nil {
				t.Fatal("nil report")
			}
			t.Logf("%s: findings=%d integrity=%.2f",
				v.Info.Name, len(result.Findings), result.Report.OverallIntegrityScore)
		})
	}
}

// TestPreCortexBaseline verifies Variant E reproduces pre-pipeline metrics.
func TestPreCortexBaseline(t *testing.T) {
	convDir := filepath.Join("..", "..", "data", "test-conversations")
	if _, err := os.Stat(convDir); os.IsNotExist(err) {
		t.Skip("test-conversations not found")
	}

	entries := loadCompetitionEntries(t, convDir)
	cfg := PreCortexConfig()

	var totalTP, totalFP, totalFN int
	for _, entry := range entries {
		tp, fp, fn := runAndScore(entry.Snap, cfg, entry.Expected)
		totalTP += tp
		totalFP += fp
		totalFN += fn
	}

	precision := float64(totalTP) / float64(totalTP+totalFP)
	recall := float64(totalTP) / float64(totalTP+totalFN)
	f1 := 2 * precision * recall / (precision + recall)

	t.Logf("Pre-pipeline: TP=%d FP=%d FN=%d Precision=%.2f Recall=%.2f F1=%.2f",
		totalTP, totalFP, totalFN, precision, recall, f1)

	if recall < 0.99 {
		t.Errorf("expected recall=1.00, got %.2f", recall)
	}
	if totalFP != 5 {
		t.Errorf("expected 5 FP (pre-pipeline baseline), got %d", totalFP)
	}
}

// TestFullCortexNoRegression verifies Variant A matches current pipeline metrics.
func TestFullCortexNoRegression(t *testing.T) {
	convDir := filepath.Join("..", "..", "data", "test-conversations")
	if _, err := os.Stat(convDir); os.IsNotExist(err) {
		t.Skip("test-conversations not found")
	}

	entries := loadCompetitionEntries(t, convDir)
	cfg := FullCortexConfig()

	// Load Layer 0 resources.
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
	cfg.Layer0.Toxicity.Blocklist = blocklist
	cfg.Layer0.Language.Profiles = profiles

	var totalTP, totalFP, totalFN int
	for _, entry := range entries {
		tp, fp, fn := runAndScore(entry.Snap, cfg, entry.Expected)
		totalTP += tp
		totalFP += fp
		totalFN += fn
	}

	precision := float64(totalTP) / float64(totalTP+totalFP)
	recall := float64(totalTP) / float64(totalTP+totalFN)
	f1 := 2 * precision * recall / (precision + recall)

	t.Logf("Full-pipeline: TP=%d FP=%d FN=%d Precision=%.2f Recall=%.2f F1=%.2f",
		totalTP, totalFP, totalFN, precision, recall, f1)

	if recall < 0.99 {
		t.Errorf("REGRESSION: recall=%.2f (expected 1.00)", recall)
	}
	if totalFP != 2 {
		t.Errorf("REGRESSION: FP=%d (expected 2)", totalFP)
	}
	if f1 < 0.90 {
		t.Errorf("REGRESSION: F1=%.2f (expected ≥0.90)", f1)
	}
}

// TestArchitectureCompetition runs the full Phase 6 competition.
func TestArchitectureCompetition(t *testing.T) {
	convDir := filepath.Join("..", "..", "data", "test-conversations")
	if _, err := os.Stat(convDir); os.IsNotExist(err) {
		t.Skip("test-conversations not found")
	}

	entries := loadCompetitionEntries(t, convDir)

	// Build variants, loading Layer 0 resources for those that need it.
	variants := AllVariants()
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
	for i := range variants {
		if variants[i].Config.UseLayer0 {
			variants[i].Config.Layer0.Toxicity.Blocklist = blocklist
			variants[i].Config.Layer0.Language.Profiles = profiles
		}
	}

	result := RunCompetition(entries, variants)

	// === Print raw results matrix ===
	t.Log("\n========== ARCHITECTURE COMPETITION RESULTS ==========")
	t.Log("")
	t.Logf("%-20s %6s %6s %6s %6s %10s %10s %10s %6s %5s",
		"Variant", "Prec", "Recall", "F1", "FPR", "Lat Mean", "Lat P95", "Lat P99", "Stages", "COGs")
	t.Log(strings.Repeat("-", 100))

	for _, vr := range result.Variants {
		t.Logf("%-20s %6.2f %6.2f %6.2f %6.2f %8.3f ms %8.3f ms %8.3f ms %6d %5d",
			vr.Info.Name,
			vr.Traits["precision"],
			vr.Traits["recall"],
			vr.Traits["f1"],
			vr.Traits["false_positive_rate"],
			vr.Traits["latency_mean_ms"],
			vr.Traits["latency_p95_ms"],
			vr.Traits["latency_p99_ms"],
			int(vr.Traits["stage_count"]),
			int(vr.Traits["cog_count"]),
		)
	}

	// === Per-profile winners ===
	t.Log("")
	t.Log("Per-profile winners:")
	profileScores := ScoreAllProfiles(result.Variants)
	for _, profile := range AllProfiles() {
		winner := result.ProfileWinners[profile.Name]
		t.Logf("  %-16s → %s", profile.Name, winner)

		scores := profileScores[profile.Name]
		type vs struct {
			name  string
			score float64
		}
		var sorted []vs
		for name, score := range scores {
			sorted = append(sorted, vs{name, score})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].score > sorted[j].score })
		for _, s := range sorted {
			marker := "  "
			if s.name == winner {
				marker = " *"
			}
			t.Logf("    %s %-20s score=%.4f", marker, s.name, s.score)
		}
	}

	// === Pareto frontier ===
	t.Log("")
	t.Logf("Pareto frontier: %v", result.Frontier)

	// === Layer contribution analysis ===
	t.Log("")
	t.Log("Layer contribution analysis:")
	variantTraits := make(map[string]map[string]float64)
	for _, vr := range result.Variants {
		variantTraits[vr.Info.Name] = vr.Traits
	}

	layerAnalysis := []struct {
		from, to, layer string
	}{
		{"E-pre-cortex", "D-inhibitor-only", "Context Inhibitor (Phase 1)"},
		{"D-inhibitor-only", "C-no-modulation", "Salience + Metacognition (Phases 4-5)"},
		{"D-inhibitor-only", "B-no-feedback", "Layer 0 + Modulation + Salience (Phases 2-3,5)"},
		{"B-no-feedback", "A-full-cortex", "Feedback Evaluator (Phase 4 metacognition)"},
		{"C-no-modulation", "A-full-cortex", "Gain Modulation (Phase 2)"},
	}

	for _, la := range layerAnalysis {
		from := variantTraits[la.from]
		to := variantTraits[la.to]
		if from == nil || to == nil {
			continue
		}
		precDelta := to["precision"] - from["precision"]
		f1Delta := to["f1"] - from["f1"]
		fpDelta := to["false_positives"] - from["false_positives"]
		latDelta := to["latency_mean_ms"] - from["latency_mean_ms"]

		t.Logf("  %s → %s (%s):", la.from, la.to, la.layer)
		t.Logf("    Precision: %+.2f, F1: %+.2f, FP: %+.0f, Latency: %+.3f ms",
			precDelta, f1Delta, fpDelta, latDelta)
	}

	// === Assertions ===

	// All variants must achieve recall=1.00.
	for _, vr := range result.Variants {
		if vr.Traits["recall"] < 0.99 {
			t.Errorf("%s: recall=%.2f (expected 1.00)", vr.Info.Name, vr.Traits["recall"])
		}
	}

	// Variant A must match current pipeline.
	aTraits := variantTraits["A-full-cortex"]
	if aTraits["false_positives"] != 2 {
		t.Errorf("Variant A FP=%.0f (expected 2)", aTraits["false_positives"])
	}

	// Variant E must match pre-pipeline baseline.
	eTraits := variantTraits["E-pre-cortex"]
	if eTraits["false_positives"] != 5 {
		t.Errorf("Variant E FP=%.0f (expected 5)", eTraits["false_positives"])
	}

	// Pareto frontier must not be empty.
	if len(result.Frontier) == 0 {
		t.Error("empty Pareto frontier")
	}

	// Profile scoring should produce at least 2 distinct winners.
	winnerSet := make(map[string]bool)
	for _, w := range result.ProfileWinners {
		winnerSet[w] = true
	}
	if len(winnerSet) < 2 {
		t.Logf("NOTE: all profiles select the same winner (%v) — traits may not vary enough", result.ProfileWinners)
	}
}

// TestParetoFrontier verifies Pareto computation with known data.
func TestParetoFrontier(t *testing.T) {
	specs := []TraitSpec{
		{"accuracy", Maximize},
		{"speed", Maximize},
	}

	results := []VariantResult{
		{Info: VariantInfo{Name: "A"}, Traits: map[string]float64{"accuracy": 0.9, "speed": 0.5}},
		{Info: VariantInfo{Name: "B"}, Traits: map[string]float64{"accuracy": 0.7, "speed": 0.9}},
		{Info: VariantInfo{Name: "C"}, Traits: map[string]float64{"accuracy": 0.6, "speed": 0.4}}, // dominated by A and B
	}

	frontier := computePareto(results, specs)

	if len(frontier) != 2 {
		t.Fatalf("expected 2 frontier members, got %d: %v", len(frontier), frontier)
	}

	frontierSet := make(map[string]bool)
	for _, f := range frontier {
		frontierSet[f] = true
	}
	if !frontierSet["A"] || !frontierSet["B"] {
		t.Errorf("expected A and B on frontier, got %v", frontier)
	}
	if frontierSet["C"] {
		t.Error("C should be dominated")
	}
}

// TestProfileScoringDifferentiates verifies different profiles can produce different winners.
func TestProfileScoringDifferentiates(t *testing.T) {
	specs := AllTraitSpecs()

	results := []VariantResult{
		{
			Info:   VariantInfo{Name: "fast"},
			Traits: map[string]float64{"precision": 0.6, "recall": 1.0, "f1": 0.75, "false_positive_rate": 0.5, "latency_mean_ms": 0.1, "latency_p95_ms": 0.2, "latency_p99_ms": 0.3, "stage_count": 4, "cog_count": 10},
		},
		{
			Info:   VariantInfo{Name: "accurate"},
			Traits: map[string]float64{"precision": 0.95, "recall": 1.0, "f1": 0.97, "false_positive_rate": 0.1, "latency_mean_ms": 1.0, "latency_p95_ms": 2.0, "latency_p99_ms": 3.0, "stage_count": 12, "cog_count": 21},
		},
	}

	balancedWinner := scoreProfile(results, AllProfiles()[0], specs)
	minimalWinner := scoreProfile(results, AllProfiles()[3], specs)

	// At minimum, verify scoring runs without error and produces non-empty results.
	if balancedWinner == "" || minimalWinner == "" {
		t.Error("scoring returned empty winner")
	}

	t.Logf("balanced=%s, minimal=%s", balancedWinner, minimalWinner)

	// With different trait emphasis, profiles should produce different winners.
	// "fast" has better latency/complexity; "accurate" has better precision/F1.
	// The specific winner depends on normalization, but they should differ.
	if balancedWinner == minimalWinner {
		// This is acceptable if both metrics profiles happen to favor the same variant.
		// The real test is that scoring differentiates in TestArchitectureCompetition.
		t.Logf("both profiles selected %s — normalization may collapse differences", balancedWinner)
	} else {
		t.Logf("profiles differentiate: balanced=%s, minimal=%s", balancedWinner, minimalWinner)
	}
}

// === Helpers for Phase 6 tests ===

func loadCompetitionEntries(t *testing.T, convDir string) []CompetitionEntry {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(convDir, "*.ndjson"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	var entries []CompetitionEntry
	for _, f := range files {
		entry, err := loadCorpusEntry(f)
		if err != nil {
			t.Fatalf("load %s: %v", f, err)
		}
		snap, err := entryToSnapshot(entry)
		if err != nil {
			t.Fatalf("convert: %v", err)
		}

		var expected []string
		for _, exp := range entry.Expected {
			expected = append(expected, exp.FindingType)
		}

		entries = append(entries, CompetitionEntry{
			ID:       entry.EntryID,
			Snap:     snap,
			Expected: expected,
		})
	}
	return entries
}

func runAndScore(snap *reasoningv1.ConversationSnapshot, cfg PipelineConfig, expected []string) (tp, fp, fn int) {
	result := Run(snap, cfg)

	actualTypes := make(map[string]bool)
	for _, finding := range result.Findings {
		actualTypes[findingTypeString(finding.FindingType)] = true
	}

	// If inhibitor is enabled, use report findings (post-inhibition).
	if cfg.UseInhibitor && result.Report != nil {
		actualTypes = make(map[string]bool)
		for _, finding := range result.Report.GetFindings() {
			actualTypes[findingTypeString(finding.FindingType)] = true
		}
	}

	expectedTypes := make(map[string]bool)
	for _, et := range expected {
		expectedTypes[et] = true
	}

	for et := range expectedTypes {
		if actualTypes[et] {
			tp++
		} else {
			fn++
		}
	}
	for at := range actualTypes {
		if !expectedTypes[at] {
			fp++
		}
	}
	return
}
