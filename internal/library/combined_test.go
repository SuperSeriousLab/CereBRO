// Package library — TestCombinedCorpus
//
// Assembles the full evaluation corpus from all available NDJSON sources,
// deduplicates by entry_id, writes data/corpus/full-v1.ndjson, then
// runs the architecture competition on all entries and produces
// data/library/FULL_CORPUS_COMPETITION.md.
//
// Run with: go test -run TestCombinedCorpus -v ./internal/library/
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// corpusSource describes a single NDJSON corpus file.
type corpusSource struct {
	label string
	path  string
}

// TestCombinedCorpus assembles all corpus sources, deduplicates, runs the
// 5-variant architecture competition, and writes the analysis report.
func TestCombinedCorpus(t *testing.T) {
	root := filepath.Join("..", "..")

	sources := []corpusSource{
		{"expanded-v1", filepath.Join(root, "data", "corpus", "expanded-v1.ndjson")},
		{"generated-v1", filepath.Join(root, "data", "corpus", "generated-v1.ndjson")},
		{"classical-v1", filepath.Join(root, "data", "library", "corpus", "classical-v1.ndjson")},
		{"adversarial-v1", filepath.Join(root, "data", "library", "corpus", "adversarial-v1.ndjson")},
	}

	// Also include cognitive-v1 if not already covered by expanded-v1.
	// expanded-v1 is the superset; cognitive-v1 is included for completeness
	// (deduplication handles overlaps).
	sources = append([]corpusSource{
		{"cognitive-v1", filepath.Join(root, "data", "corpus", "cognitive-v1.ndjson")},
	}, sources...)

	// ── Load and deduplicate ────────────────────────────────────────────────
	seen := make(map[string]bool)         // entry_id → loaded
	perSource := make(map[string]int)     // label → unique entries contributed
	var combined []CorpusEntry

	for _, src := range sources {
		entries, err := loadNDJSONIfExists(src.path)
		if err != nil {
			t.Fatalf("load %s: %v", src.label, err)
		}
		added := 0
		for _, e := range entries {
			if seen[e.EntryID] {
				continue
			}
			seen[e.EntryID] = true
			combined = append(combined, e)
			added++
		}
		perSource[src.label] = added
		t.Logf("Source %-15s — file entries: %d, unique new: %d", src.label, len(entries), added)
	}

	t.Logf("Combined corpus total: %d unique entries", len(combined))

	// ── Finding type distribution ───────────────────────────────────────────
	ftDist := make(map[string]int)
	for _, e := range combined {
		for _, exp := range e.Expected {
			ftDist[exp.FindingType]++
		}
	}
	t.Logf("Finding type distribution:")
	for ft, n := range ftDist {
		t.Logf("  %-35s %d", ft, n)
	}

	// ── Write full-v1.ndjson ────────────────────────────────────────────────
	outPath := filepath.Join(root, "data", "corpus", "full-v1.ndjson")
	if err := WriteNDJSON(outPath, combined); err != nil {
		t.Fatalf("write full corpus: %v", err)
	}
	t.Logf("Wrote %s", outPath)

	// Verify round-trip.
	reloaded, err := loadNDJSON(outPath)
	if err != nil {
		t.Fatalf("reload full corpus: %v", err)
	}
	if len(reloaded) != len(combined) {
		t.Errorf("round-trip mismatch: wrote %d, read back %d", len(combined), len(reloaded))
	}

	// ── Build competition entries ───────────────────────────────────────────
	var compEntries []pipeline.CompetitionEntry
	for _, e := range combined {
		snap := e.Input.ToProtoSnapshot()
		var expected []string
		for _, exp := range e.Expected {
			expected = append(expected, exp.FindingType)
		}
		compEntries = append(compEntries, pipeline.CompetitionEntry{
			ID:       e.EntryID,
			Snap:     snap,
			Expected: expected,
		})
	}

	// ── Run architecture competition ────────────────────────────────────────
	variants := pipeline.AllVariants()
	result := pipeline.RunCompetition(compEntries, variants)

	t.Logf("=== COMPETITION WINNERS (full corpus, %d entries) ===", len(combined))
	for _, prof := range []string{"balanced", "precision-first", "recall-first", "minimal"} {
		t.Logf("  %-20s → %s", prof, result.ProfileWinners[prof])
	}
	t.Logf("Pareto frontier: %v", result.Frontier)

	// ── Produce comparison report ───────────────────────────────────────────
	report := buildFullCorpusReport(perSource, sources, ftDist, combined, result)
	reportPath := filepath.Join(root, "data", "library", "FULL_CORPUS_COMPETITION.md")
	if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	t.Logf("Wrote report to %s", reportPath)
}

// loadNDJSONIfExists loads a file; returns empty slice (not error) if missing.
func loadNDJSONIfExists(path string) ([]CorpusEntry, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}
	return loadNDJSON(path)
}

// ── Report builder ────────────────────────────────────────────────────────────

func buildFullCorpusReport(
	perSource map[string]int,
	sources []corpusSource,
	ftDist map[string]int,
	combined []CorpusEntry,
	result *pipeline.CompetitionResult,
) string {
	var sb strings.Builder
	w := func(format string, args ...any) {
		fmt.Fprintf(&sb, format+"\n", args...)
	}

	w("# CereBRO Full Corpus Architecture Competition")
	w("")
	w("**Date:** 2026-03-14  ")
	w("**Total entries:** %d unique (deduplicated by entry_id)", len(combined))
	w("")

	// ── Corpus composition ────────────────────────────────────────────────
	w("## Corpus Composition")
	w("")
	w("| Source | Unique entries contributed |")
	w("|--------|---------------------------|")
	for _, src := range sources {
		w("| %-15s | %d |", src.label, perSource[src.label])
	}
	w("")

	// Finding type distribution.
	w("## Finding Type Distribution (full corpus)")
	w("")
	w("| Finding Type | Count |")
	w("|--------------|-------|")
	ftOrder := []string{
		"ANCHORING_BIAS", "CONTRADICTION", "SCOPE_DRIFT",
		"CONFIDENCE_MISCALIBRATION", "SUNK_COST_FALLACY", "SILENT_REVISION",
	}
	total := 0
	for _, ft := range ftOrder {
		n := ftDist[ft]
		total += n
		w("| %-35s | %d |", ft, n)
	}
	// Any unlisted types.
	for ft, n := range ftDist {
		found := false
		for _, known := range ftOrder {
			if ft == known {
				found = true
				break
			}
		}
		if !found {
			w("| %-35s | %d |", ft, n)
			total += n
		}
	}
	w("| **Total** | **%d** |", total)
	w("")

	// ── Competition results ───────────────────────────────────────────────
	w("## Architecture Competition — Full Corpus (%d entries)", len(combined))
	w("")
	w("### Variant Trait Matrix")
	w("")
	w("| Variant | Precision | Recall | F1 | FPR | Latency(ms) | P95(ms) | TP | FP | FN |")
	w("|---------|-----------|--------|----|-----|-------------|---------|----|----|-----|")
	for _, vr := range result.Variants {
		tr := vr.Traits
		w("| %-22s | %.3f | %.3f | %.3f | %.3f | %.2f | %.2f | %.0f | %.0f | %.0f |",
			vr.Info.Name,
			tr["precision"], tr["recall"], tr["f1"],
			tr["false_positive_rate"],
			tr["latency_mean_ms"], tr["latency_p95_ms"],
			tr["true_positives"], tr["false_positives"], tr["false_negatives"],
		)
	}
	w("")

	// Profile winners.
	w("### Profile Winners")
	w("")
	w("| Profile | Winner |")
	w("|---------|--------|")
	profileOrder := []string{"balanced", "precision-first", "recall-first", "minimal"}
	for _, prof := range profileOrder {
		w("| %-20s | %s |", prof, result.ProfileWinners[prof])
	}
	w("")

	// Profile score matrix.
	profileScores := pipeline.ScoreAllProfiles(result.Variants)
	variantNames := make([]string, len(result.Variants))
	for i, vr := range result.Variants {
		variantNames[i] = vr.Info.Name
	}
	w("### Profile Score Matrix")
	w("")
	w("(Normalized weighted scores — higher is better)")
	w("")
	header := "| Profile |"
	for _, n := range variantNames {
		header += fmt.Sprintf(" %-22s |", n)
	}
	w("%s", header)
	sep := "|---------|"
	for range variantNames {
		sep += "-----------------------|"
	}
	w("%s", sep)
	for _, prof := range profileOrder {
		scores := profileScores[prof]
		row := fmt.Sprintf("| %-7s |", prof)
		for _, n := range variantNames {
			row += fmt.Sprintf(" %-22.4f |", scores[n])
		}
		w("%s", row)
	}
	w("")

	// Pareto frontier.
	w("### Pareto Frontier")
	w("")
	w("Pareto-optimal variants: **%s**", strings.Join(result.Frontier, ", "))
	w("")

	// ── Comparison with previous results ─────────────────────────────────
	w("## Comparison: Previous vs Full Corpus")
	w("")
	w("Previous results from CLASSICAL_ANALYSIS.md (43-entry classical corpus):")
	w("")

	// Classical results as reference (from CLASSICAL_ANALYSIS.md).
	classicalRef := map[string][3]float64{
		"A-full-cortex":    {0.600, 0.208, 0.309},
		"B-no-feedback":    {0.600, 0.208, 0.309},
		"C-no-modulation":  {0.577, 0.208, 0.306},
		"D-inhibitor-only": {0.577, 0.208, 0.306},
		"E-pre-cortex":     {0.471, 0.333, 0.390},
	}

	w("| Variant | Classical P/R/F1 | Full Corpus P/R/F1 | F1 Change |")
	w("|---------|------------------|--------------------|-----------|")
	for _, vr := range result.Variants {
		tr := vr.Traits
		ref := classicalRef[vr.Info.Name]
		delta := tr["f1"] - ref[2]
		sign := "+"
		if delta < 0 {
			sign = ""
		}
		w("| %-22s | %.3f / %.3f / %.3f | %.3f / %.3f / %.3f | %s%.3f |",
			vr.Info.Name,
			ref[0], ref[1], ref[2],
			tr["precision"], tr["recall"], tr["f1"],
			sign, delta,
		)
	}
	w("")

	w("Previous profile winners (classical 43-entry corpus):")
	w("")
	classicalWinners := map[string]string{
		"balanced":        "E-pre-cortex",
		"precision-first": "B-no-feedback",
		"recall-first":    "E-pre-cortex",
		"minimal":         "E-pre-cortex",
	}
	w("| Profile | Original (9 conv) | Classical (43) | Adversarial (5) | Full Corpus | Change |")
	w("|---------|-------------------|----------------|-----------------|-------------|--------|")

	// Original 9-conv winners (from CLAUDE.md context: D-inhibitor-only won).
	originalWinners := map[string]string{
		"balanced":        "D-inhibitor-only",
		"precision-first": "D-inhibitor-only",
		"recall-first":    "D-inhibitor-only",
		"minimal":         "D-inhibitor-only",
	}

	for _, prof := range profileOrder {
		orig := originalWinners[prof]
		classical := classicalWinners[prof]
		full := result.ProfileWinners[prof]
		changed := "same"
		if full != classical {
			changed = "changed"
		}
		w("| %-20s | %-17s | %-14s | %-15s | %-11s | %s |",
			prof, orig, classical, "N/A", full, changed)
	}
	w("")

	// ── Key questions ─────────────────────────────────────────────────────
	w("## Key Questions")
	w("")

	// Q1: Does D-inhibitor-only still win on the larger corpus?
	inhibitorWins := 0
	for _, prof := range profileOrder {
		if strings.Contains(result.ProfileWinners[prof], "inhibitor-only") {
			inhibitorWins++
		}
	}
	w("### Does D-inhibitor-only still win on the larger corpus?")
	w("")
	if inhibitorWins >= 3 {
		w("**YES** — D-inhibitor-only wins %d/4 profiles on the full %d-entry corpus. The simple architecture generalizes.", inhibitorWins, len(combined))
	} else if inhibitorWins >= 1 {
		w("**PARTIALLY** — D-inhibitor-only wins %d/4 profiles on the full corpus (down from dominance on 9-conv). More diverse input exposes its limits.", inhibitorWins, len(combined))
	} else {
		w("**NO** — D-inhibitor-only wins 0/4 profiles on the full %d-entry corpus. The diversity of the combined corpus (conversational + classical + adversarial) favors other variants.", len(combined))
	}
	w("")

	// Q2: Do the modulation/feedback/salience layers earn their keep?
	w("### Do modulation/feedback/salience layers earn their keep on harder, more diverse input?")
	w("")

	// Find F1 scores for A, B, C, D, E.
	f1 := make(map[string]float64)
	for _, vr := range result.Variants {
		f1[vr.Info.Name] = vr.Traits["f1"]
	}

	aWins := 0
	for _, prof := range profileOrder {
		if result.ProfileWinners[prof] == "A-full-cortex" {
			aWins++
		}
	}
	bWins := 0
	for _, prof := range profileOrder {
		if result.ProfileWinners[prof] == "B-no-feedback" {
			bWins++
		}
	}

	fullF1 := f1["A-full-cortex"]
	noFeedF1 := f1["B-no-feedback"]
	noModF1 := f1["C-no-modulation"]
	inhibF1 := f1["D-inhibitor-only"]

	w("F1 scores on full corpus:")
	w("- A-full-cortex (all layers): %.3f", fullF1)
	w("- B-no-feedback (no metacognition): %.3f", noFeedF1)
	w("- C-no-modulation (no urgency/threshold): %.3f", noModF1)
	w("- D-inhibitor-only (minimal): %.3f", inhibF1)
	w("")

	if fullF1 > inhibF1+0.02 {
		w("**Yes** — the full pipeline's F1 (%.3f) meaningfully exceeds D-inhibitor-only (%.3f). The additional layers add value on the diverse corpus.", fullF1, inhibF1)
	} else if fullF1 > inhibF1+0.005 {
		w("**Marginally yes** — A-full-cortex F1 (%.3f) slightly exceeds D-inhibitor-only (%.3f). The added complexity has a small positive effect.", fullF1, inhibF1)
	} else {
		w("**No** — A-full-cortex F1 (%.3f) does not meaningfully exceed D-inhibitor-only (%.3f). The extra layers (modulation, feedback, salience) do not pay their way on this corpus.", fullF1, inhibF1)
	}
	w("")

	// Feedback layer specifically.
	if noFeedF1 >= fullF1-0.005 {
		w("**Feedback layer verdict**: B-no-feedback (%.3f F1) matches A-full-cortex (%.3f) — the metacognition/feedback loop adds negligible value.", noFeedF1, fullF1)
	} else {
		w("**Feedback layer verdict**: Removing feedback drops F1 from %.3f to %.3f — feedback contributes.", fullF1, noFeedF1)
	}
	w("")

	// Modulation layer specifically.
	if noModF1 >= noFeedF1-0.005 {
		w("**Modulation layer verdict**: C-no-modulation (%.3f F1) is comparable to B-no-feedback (%.3f) — urgency/threshold modulation adds marginal value.", noModF1, noFeedF1)
	} else {
		w("**Modulation layer verdict**: Removing modulation drops F1 from %.3f to %.3f — the urgency/threshold layer contributes.", noFeedF1, noModF1)
	}
	w("")

	// Profile win summary.
	w("**Profile wins summary**: A wins %d/4, B wins %d/4 profiles. The winner is %s.",
		aWins, bWins, result.ProfileWinners["balanced"])
	w("")

	// Sort profile winners for a clean list.
	winCounts := make(map[string]int)
	for _, prof := range profileOrder {
		winCounts[result.ProfileWinners[prof]]++
	}
	type nameCount struct {
		name  string
		count int
	}
	var ranked []nameCount
	for n, c := range winCounts {
		ranked = append(ranked, nameCount{n, c})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].count > ranked[j].count })

	w("### Overall Winner")
	w("")
	w("| Variant | Profile wins (out of 4) |")
	w("|---------|------------------------|")
	for _, nc := range ranked {
		w("| %-22s | %d |", nc.name, nc.count)
	}
	w("")
	if len(ranked) > 0 {
		w("**Overall winner: %s** (%d/4 profiles)", ranked[0].name, ranked[0].count)
	}
	w("")

	return sb.String()
}
