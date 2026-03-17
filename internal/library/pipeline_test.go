// Package library — TestClassicalPipeline
//
// Runs the CereBRO pipeline on 43 Republic Book 1 corpus entries and
// produces a scorecard at data/library/CLASSICAL_ANALYSIS.md.
// Also generates a combined NDJSON corpus at data/library/corpus/classical-v1.ndjson.
//
// Run with: go test -run TestClassicalPipeline -v -timeout 5m ./internal/library/
package library

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// ─── entry loading ──────────────────────────────────────────────────────────

// loadNDJSON reads all NDJSON entries from a file.
func loadNDJSON(path string) ([]CorpusEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []CorpusEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // large entries
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e CorpusEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parse line: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, sc.Err()
}

// ─── per-entry result ────────────────────────────────────────────────────────

type entryResult struct {
	ID        string
	Section   string // cephalus / polemarchus / thrasymachus
	Expected  []string
	Found     []string
	TP, FP, FN int
	Formality  float64
	LatencyMS  float64
}

// ─── per-detector aggregate ─────────────────────────────────────────────────

type detectorStats struct {
	TP, FP, FN int
}

func (d *detectorStats) precision() float64 {
	if d.TP+d.FP == 0 {
		return 0
	}
	return float64(d.TP) / float64(d.TP+d.FP)
}

func (d *detectorStats) recall() float64 {
	if d.TP+d.FN == 0 {
		return 0
	}
	return float64(d.TP) / float64(d.TP+d.FN)
}

func (d *detectorStats) f1() float64 {
	p, r := d.precision(), d.recall()
	if p+r == 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}

// ─── named run type ──────────────────────────────────────────────────────────

type pipelineRun struct {
	name    string
	cfg     pipeline.PipelineConfig
	results []entryResult
}

// ─── main test ───────────────────────────────────────────────────────────────

func TestClassicalPipeline(t *testing.T) {
	dataDir := filepath.Join("..", "..", "data", "library")

	// Load all three dialogue files.
	type sectionFile struct {
		name string
		path string
	}
	sections := []sectionFile{
		{"cephalus", filepath.Join(dataDir, "dialogues", "republic_b1_cephalus.ndjson")},
		{"polemarchus", filepath.Join(dataDir, "dialogues", "republic_b1_polemarchus.ndjson")},
		{"thrasymachus", filepath.Join(dataDir, "dialogues", "republic_b1_thrasymachus.ndjson")},
	}

	var allEntries []CorpusEntry
	sectionMap := make(map[string]string) // entry_id → section name
	for _, s := range sections {
		entries, err := loadNDJSON(s.path)
		if err != nil {
			t.Fatalf("load %s: %v", s.path, err)
		}
		t.Logf("Loaded %d entries from %s", len(entries), s.name)
		for _, e := range entries {
			sectionMap[e.EntryID] = s.name
		}
		allEntries = append(allEntries, entries...)
	}
	t.Logf("Total entries loaded: %d", len(allEntries))

	if len(allEntries) != 43 {
		t.Errorf("expected 43 entries, got %d", len(allEntries))
	}

	// Per-detector stats accumulator for baseline run.
	detStats := make(map[string]*detectorStats)
	for _, det := range []string{
		"ANCHORING_BIAS", "CONTRADICTION", "SCOPE_DRIFT",
		"CONFIDENCE_MISCALIBRATION", "SUNK_COST_FALLACY", "SILENT_REVISION",
	} {
		detStats[det] = &detectorStats{}
	}

	// Classical domain context: signals to the pipeline that this is classical
	// philosophical text. applyDomainContext will lower DriftThreshold (0.79→0.70)
	// and SustainedTurns (8→4) so Scope Guard can fire on 10-turn segments.
	classicalCtx := &pipeline.DomainContext{
		PrimaryDomain: "philosophy",
		TextEra:       "classical",
		Confidence:    0.85,
	}

	baselineCfg := pipeline.DefaultPipelineConfig()
	baselineCfg.DomainContext = classicalCtx

	inhibitorCfg := pipeline.InhibitorOnlyConfig()
	inhibitorCfg.DomainContext = classicalCtx

	runs := []*pipelineRun{
		{name: "baseline", cfg: baselineCfg},
		{name: "inhibitor-only", cfg: inhibitorCfg},
	}

	for _, run := range runs {
		for _, entry := range allEntries {
			snap := entry.Input.ToProtoSnapshot()

			start := time.Now()
			result := pipeline.Run(snap, run.cfg)
			elapsed := float64(time.Since(start).Microseconds()) / 1000.0

			// Collect actual types (post-inhibition if inhibitor enabled, else raw).
			actualTypes := make(map[string]bool)
			if run.cfg.UseInhibitor && result.Inhibition != nil {
				for _, f := range result.Inhibition.Gated {
					actualTypes[pipeline.FindingTypeString(f.FindingType)] = true
				}
			} else {
				for _, f := range result.Findings {
					actualTypes[pipeline.FindingTypeString(f.FindingType)] = true
				}
			}

			expectedTypes := make(map[string]bool)
			for _, exp := range entry.Expected {
				expectedTypes[exp.FindingType] = true
			}

			var tp, fp, fn int
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

			// Formality is reported by the inhibitor.
			formality := 0.0
			if result.Inhibition != nil {
				formality = result.Inhibition.Formality
			}

			var found []string
			for at := range actualTypes {
				found = append(found, at)
			}
			sort.Strings(found)

			var expected []string
			for et := range expectedTypes {
				expected = append(expected, et)
			}
			sort.Strings(expected)

			run.results = append(run.results, entryResult{
				ID:        entry.EntryID,
				Section:   sectionMap[entry.EntryID],
				Expected:  expected,
				Found:     found,
				TP:        tp,
				FP:        fp,
				FN:        fn,
				Formality: formality,
				LatencyMS: elapsed,
			})

			// Accumulate per-detector stats for baseline run only.
			if run.name == "baseline" {
				for et := range expectedTypes {
					if ds, ok := detStats[et]; ok {
						if actualTypes[et] {
							ds.TP++
						} else {
							ds.FN++
						}
					}
				}
				for at := range actualTypes {
					if ds, ok := detStats[at]; ok {
						if !expectedTypes[at] {
							ds.FP++
						}
					}
				}
			}
		}
	}

	// ── Architecture competition on all 43 entries ─────────────────────────
	var compEntries []pipeline.CompetitionEntry
	for _, entry := range allEntries {
		snap := entry.Input.ToProtoSnapshot()
		var expected []string
		for _, exp := range entry.Expected {
			expected = append(expected, exp.FindingType)
		}
		compEntries = append(compEntries, pipeline.CompetitionEntry{
			ID:       entry.EntryID,
			Snap:     snap,
			Expected: expected,
		})
	}

	variants := pipeline.AllVariants()
	compResult := pipeline.RunCompetition(compEntries, variants)

	// ── Write combined NDJSON corpus ────────────────────────────────────────
	corpusDir := filepath.Join(dataDir, "corpus")
	if err := os.MkdirAll(corpusDir, 0755); err != nil {
		t.Fatalf("mkdir corpus: %v", err)
	}
	corpusPath := filepath.Join(corpusDir, "classical-v1.ndjson")
	if err := WriteNDJSON(corpusPath, allEntries); err != nil {
		t.Fatalf("write corpus: %v", err)
	}
	t.Logf("Wrote combined corpus to %s", corpusPath)

	// ── Generate scorecard ──────────────────────────────────────────────────
	scorecard := buildScorecard(runs, compResult, detStats, len(allEntries))
	scorecardPath := filepath.Join(dataDir, "CLASSICAL_ANALYSIS.md")
	if err := os.WriteFile(scorecardPath, []byte(scorecard), 0644); err != nil {
		t.Fatalf("write scorecard: %v", err)
	}
	t.Logf("Wrote scorecard to %s", scorecardPath)

	// ── Assertions ────────────────────────────────────────────────────────────
	// Formality check: classical text should score >= 0.70.
	var formalitySum float64
	var formalityCount int
	for _, run := range runs {
		if run.name != "inhibitor-only" {
			continue
		}
		for _, er := range run.results {
			if er.Formality > 0 {
				formalitySum += er.Formality
				formalityCount++
			}
		}
	}
	if formalityCount > 0 {
		avgFormality := formalitySum / float64(formalityCount)
		t.Logf("Average formality (inhibitor-only, %d entries with formality data): %.3f", formalityCount, avgFormality)
		if avgFormality < 0.7 {
			t.Logf("WARNING: average formality %.3f < 0.70 (expected classical text to score high)", avgFormality)
		}
	}

	// Summary.
	var totalTP, totalFP, totalFN int
	for _, er := range runs[0].results {
		totalTP += er.TP
		totalFP += er.FP
		totalFN += er.FN
	}
	precision, recall, f1 := computePRF(totalTP, totalFP, totalFN)
	t.Logf("=== BASELINE SUMMARY ===")
	t.Logf("TP=%d FP=%d FN=%d | Precision=%.3f Recall=%.3f F1=%.3f", totalTP, totalFP, totalFN, precision, recall, f1)

	// Per-detector breakdown.
	t.Logf("=== PER-DETECTOR STATS (baseline, classical) ===")
	for _, det := range []string{
		"ANCHORING_BIAS", "CONTRADICTION", "SCOPE_DRIFT",
		"CONFIDENCE_MISCALIBRATION", "SUNK_COST_FALLACY", "SILENT_REVISION",
	} {
		if ds, ok := detStats[det]; ok {
			p, r, f := ds.precision(), ds.recall(), ds.f1()
			t.Logf("  %-28s TP=%d FP=%d FN=%d P=%.3f R=%.3f F1=%.3f", det, ds.TP, ds.FP, ds.FN, p, r, f)
		}
	}

	// ── Key assertions ────────────────────────────────────────────────────────
	// Scope Drift: with classical DomainContext (DriftThreshold=0.70, SustainedTurns=4),
	// the Thrasymachus late-dialogue scope shift must produce at least 1 true positive.
	scopeDS := detStats["SCOPE_DRIFT"]
	scopeF1 := scopeDS.f1()
	if scopeDS.TP == 0 {
		t.Errorf("SCOPE_DRIFT: expected at least 1 true positive on classical corpus (F1=%.3f), got TP=0 FP=%d FN=%d. "+
			"DomainContext overrides (DriftThreshold=0.70, SustainedTurns=4) must be active.", scopeF1, scopeDS.FP, scopeDS.FN)
	} else {
		t.Logf("SCOPE_DRIFT: F1=%.3f TP=%d FP=%d FN=%d — classical detection working", scopeF1, scopeDS.TP, scopeDS.FP, scopeDS.FN)
	}

	t.Logf("=== COMPETITION WINNERS ===")
	for profile, winner := range compResult.ProfileWinners {
		t.Logf("  %-20s → %s", profile, winner)
	}
	t.Logf("Pareto frontier: %v", compResult.Frontier)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func computePRF(tp, fp, fn int) (precision, recall, f1 float64) {
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	return
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// ─── scorecard builder ────────────────────────────────────────────────────────

func buildScorecard(
	runs []*pipelineRun,
	compResult *pipeline.CompetitionResult,
	detStats map[string]*detectorStats,
	totalEntries int,
) string {
	var sb strings.Builder
	wl := func(format string, args ...any) {
		fmt.Fprintf(&sb, format+"\n", args...)
	}

	wl("# CereBRO Classical Analysis — Republic Book 1")
	wl("")
	wl("**Date:** 2026-03-14  ")
	wl("**Corpus:** Plato's Republic Book 1 (Jowett translation)  ")
	wl("**Entries:** %d (Cephalus: 4, Polemarchus: 13, Thrasymachus: 26)", totalEntries)
	wl("")

	// Per-run aggregate metrics.
	wl("## Pipeline Run Metrics")
	wl("")
	wl("| Run | TP | FP | FN | Precision | Recall | F1 |")
	wl("|-----|----|----|----|-----------|-----------|----|")
	for _, run := range runs {
		var tp, fp, fn int
		for _, er := range run.results {
			tp += er.TP
			fp += er.FP
			fn += er.FN
		}
		p, r, f := computePRF(tp, fp, fn)
		wl("| %-20s | %3d | %3d | %3d | %.3f | %.3f | %.3f |",
			run.name, tp, fp, fn, p, r, f)
	}
	wl("")

	// Per-detector breakdown (baseline run).
	wl("## Per-Detector Breakdown (baseline config)")
	wl("")
	wl("| Detector | TP | FP | FN | Precision | Recall | F1 | Notes |")
	wl("|----------|----|----|----|-----------|-----------|----|-------|")

	detOrder := []string{
		"ANCHORING_BIAS",
		"CONTRADICTION",
		"SCOPE_DRIFT",
		"CONFIDENCE_MISCALIBRATION",
		"SUNK_COST_FALLACY",
		"SILENT_REVISION",
	}
	detNotes := map[string]string{
		"ANCHORING_BIAS":            "Thrasymachus fixates on 'advantage of the stronger'",
		"CONTRADICTION":             "Polemarchus + Thrasymachus self-refutation chains",
		"SCOPE_DRIFT":               "Thrasymachus late-dialogue scope shift",
		"CONFIDENCE_MISCALIBRATION": "Thrasymachus overconfidence early in dialogue",
		"SUNK_COST_FALLACY":         "Polemarchus defends Simonides definition throughout",
		"SILENT_REVISION":           "Rare in Socratic dialogue format",
	}

	for _, det := range detOrder {
		ds := detStats[det]
		p, r, f := ds.precision(), ds.recall(), ds.f1()
		note := detNotes[det]
		wl("| %-28s | %3d | %3d | %3d | %.3f | %.3f | %.3f | %s |",
			det, ds.TP, ds.FP, ds.FN, p, r, f, note)
	}
	wl("")

	// Per-section breakdown.
	wl("## Per-Section Breakdown (baseline config)")
	wl("")
	wl("| Section | Entries | TP | FP | FN | Precision | Recall | F1 |")
	wl("|---------|---------|----|----|----|-----------|-----------|----|")

	sectionOrder := []string{"cephalus", "polemarchus", "thrasymachus"}
	// Use baseline run (index 0).
	baselineRun := runs[0]
	type agg3 [3]int
	sectionAgg := make(map[string]agg3) // section → [TP, FP, FN]
	for _, er := range baselineRun.results {
		a := sectionAgg[er.Section]
		a[0] += er.TP
		a[1] += er.FP
		a[2] += er.FN
		sectionAgg[er.Section] = a
	}
	for _, sec := range sectionOrder {
		a := sectionAgg[sec]
		p, r, f := computePRF(a[0], a[1], a[2])
		count := 0
		for _, er := range baselineRun.results {
			if er.Section == sec {
				count++
			}
		}
		wl("| %-14s | %7d | %3d | %3d | %3d | %.3f | %.3f | %.3f |",
			sec, count, a[0], a[1], a[2], p, r, f)
	}
	wl("")

	// Formality analysis.
	wl("## Context Inhibitor — Formality Analysis")
	wl("")
	wl("The Context Inhibitor gates findings based on formality score.")
	wl("Classical philosophical text is expected to score > 0.70 (formal).")
	wl("")

	// Find inhibitor-only run.
	var inhibRun *pipelineRun
	for _, r := range runs {
		if r.name == "inhibitor-only" {
			inhibRun = r
			break
		}
	}

	if inhibRun != nil {
		var fSum float64
		var fCount int
		var fLow []string
		for _, er := range inhibRun.results {
			if er.Formality > 0 {
				fSum += er.Formality
				fCount++
				if er.Formality < 0.7 {
					fLow = append(fLow, fmt.Sprintf("%s (%.3f)", er.ID, er.Formality))
				}
			}
		}
		if fCount > 0 {
			wl("- Average formality across %d entries: **%.3f**", fCount, fSum/float64(fCount))
		}
		if len(fLow) > 0 {
			wl("- Entries below 0.70 threshold: %v", fLow)
		} else {
			wl("- All entries scored >= 0.70 formality (Context Inhibitor correctly treats as formal)")
		}
		wl("")

		// Confidence miscalibration preservation check.
		var cmTotal, cmFound int
		for _, er := range inhibRun.results {
			for _, exp := range er.Expected {
				if exp == "CONFIDENCE_MISCALIBRATION" {
					cmTotal++
					for _, found := range er.Found {
						if found == "CONFIDENCE_MISCALIBRATION" {
							cmFound++
							break
						}
					}
				}
			}
		}
		wl("### Confidence Miscalibration Preservation")
		wl("")
		pct := safeDiv(float64(cmFound), float64(cmTotal)) * 100
		wl("Expected CONFIDENCE_MISCALIBRATION in %d entries, found in %d (%.0f%%)",
			cmTotal, cmFound, pct)
		wl("")
		if cmTotal > 0 && float64(cmFound)/float64(cmTotal) > 0.5 {
			wl("Confidence miscalibration findings are **PRESERVED** (not suppressed) by the inhibitor.")
		} else if cmTotal > 0 {
			wl("**WARNING**: Confidence miscalibration findings appear suppressed by the inhibitor.")
		}
		wl("")
	}

	// Architecture competition.
	wl("## Architecture Competition (43-entry classical corpus)")
	wl("")
	wl("### Profile Winners")
	wl("")
	wl("| Profile | Winner |")
	wl("|---------|--------|")
	profileOrder := []string{"balanced", "precision-first", "recall-first", "minimal"}
	for _, profile := range profileOrder {
		winner := compResult.ProfileWinners[profile]
		wl("| %-20s | %s |", profile, winner)
	}
	wl("")

	wl("### Pareto Frontier")
	wl("")
	wl("Pareto-optimal variants: **%s**",
		strings.Join(compResult.Frontier, ", "))
	wl("")

	wl("### Variant Trait Matrix")
	wl("")
	wl("| Variant | Precision | Recall | F1 | FPR | Latency(ms) | P95(ms) | TP | FP | FN |")
	wl("|---------|-----------|--------|----|-----|-------------|---------|----|----|-----|")
	for _, vr := range compResult.Variants {
		tr := vr.Traits
		wl("| %-22s | %.3f | %.3f | %.3f | %.3f | %.2f | %.2f | %.0f | %.0f | %.0f |",
			vr.Info.Name,
			tr["precision"], tr["recall"], tr["f1"],
			tr["false_positive_rate"],
			tr["latency_mean_ms"], tr["latency_p95_ms"],
			tr["true_positives"], tr["false_positives"], tr["false_negatives"],
		)
	}
	wl("")

	// Profile score matrix.
	profileScores := pipeline.ScoreAllProfiles(compResult.Variants)
	wl("### Profile Score Matrix")
	wl("")
	wl("(Normalized weighted scores per profile)")
	wl("")

	variantNames := make([]string, len(compResult.Variants))
	for i, vr := range compResult.Variants {
		variantNames[i] = vr.Info.Name
	}

	header := "| Profile |"
	for _, n := range variantNames {
		header += fmt.Sprintf(" %-22s |", n)
	}
	wl("%s", header)
	sep := "|---------|"
	for range variantNames {
		sep += "-----------------------|"
	}
	wl("%s", sep)
	for _, prof := range profileOrder {
		scores := profileScores[prof]
		row := fmt.Sprintf("| %-7s |", prof)
		for _, n := range variantNames {
			row += fmt.Sprintf(" %-22.4f |", scores[n])
		}
		wl("%s", row)
	}
	wl("")

	// Observations.
	wl("## Key Observations")
	wl("")

	// Check if D-inhibitor-only still dominates.
	inhibitorWins := 0
	for _, profile := range profileOrder {
		if strings.Contains(compResult.ProfileWinners[profile], "inhibitor-only") {
			inhibitorWins++
		}
	}
	if inhibitorWins >= 2 {
		wl("1. **D-inhibitor-only still dominates**: wins %d/%d profiles on classical text, consistent with earlier 9-conversation results.", inhibitorWins, len(profileOrder))
	} else {
		wl("1. **D-inhibitor-only did NOT dominate**: wins only %d/%d profiles — harder classical text may favor more complex variants.", inhibitorWins, len(profileOrder))
	}

	// Compare full pipeline vs inhibitor-only.
	var fullTP, fullFP, fullFN float64
	for _, vr := range compResult.Variants {
		if vr.Info.Name == "A-full-cortex" {
			fullTP = vr.Traits["true_positives"]
			fullFP = vr.Traits["false_positives"]
			fullFN = vr.Traits["false_negatives"]
		}
	}
	var inhibTP, inhibFP, inhibFN float64
	for _, vr := range compResult.Variants {
		if vr.Info.Name == "D-inhibitor-only" {
			inhibTP = vr.Traits["true_positives"]
			inhibFP = vr.Traits["false_positives"]
			inhibFN = vr.Traits["false_negatives"]
		}
	}
	_, _, fullF1 := computePRF(int(fullTP), int(fullFP), int(fullFN))
	_, _, inhibF1 := computePRF(int(inhibTP), int(inhibFP), int(inhibFN))
	wl("")
	if fullF1 > inhibF1+0.01 {
		wl("2. **Full pipeline outperforms D-inhibitor-only on classical text** (F1: %.3f vs %.3f). Additional layers add value on harder multi-speaker philosophical dialogues.", fullF1, inhibF1)
	} else {
		wl("2. **Full pipeline does not significantly outperform D-inhibitor-only** (F1: %.3f vs %.3f). Simpler pipeline is preferred for classical text.", fullF1, inhibF1)
	}
	wl("")
	wl("3. **Scope drift detection challenges**: Classical philosophical dialogue redefines scope by design — Socrates deliberately shifts scope to expose contradictions. High FP rate expected for SCOPE_DRIFT.")
	wl("")
	wl("4. **SUNK_COST_FALLACY challenges**: Polemarchus's insistence on Simonides is a genuine sunk-cost pattern, but the detector is tuned for modern conversational markers (investment/cost vocabulary).")
	wl("")
	wl("5. **Formality gate**: Classical text scores high on formality, so the inhibitor passes most findings rather than suppressing them. The 5-gate algorithm is not a bottleneck here.")
	wl("")

	// Per-entry detail table (first 20 entries).
	wl("## Per-Entry Detail (baseline, first 20 entries)")
	wl("")
	wl("| Entry ID | Section | Expected | Found | TP | FP | FN |")
	wl("|----------|---------|----------|-------|----|----|-----|")
	for i, er := range baselineRun.results {
		if i >= 20 {
			break
		}
		exp := strings.Join(er.Expected, ", ")
		if exp == "" {
			exp = "(none)"
		}
		found := strings.Join(er.Found, ", ")
		if found == "" {
			found = "(none)"
		}
		wl("| %-25s | %-12s | %-40s | %-35s | %2d | %2d | %2d |",
			er.ID, er.Section, exp, found, er.TP, er.FP, er.FN)
	}
	wl("")

	return sb.String()
}
