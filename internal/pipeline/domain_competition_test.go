// Domain-Aware Architecture Competition — Task 4
//
// Runs all 5 variants (A–E) against two corpora:
//   - Modern:    9 test conversations (data/test-conversations/*.ndjson) — nil DomainContext
//   - Classical: 43 Republic entries  (data/library/corpus/classical-v1.ndjson)
//               — DomainContext{TextEra:"classical", PrimaryDomain:"philosophy", Confidence:0.85}
//
// Results are written to data/competitions/DOMAIN_AWARE_COMPETITION.md.
package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// ─── named types ─────────────────────────────────────────────────────────────

// corpusMetrics holds precision/recall/F1 and latency for one variant/corpus run.
type corpusMetrics struct {
	TP, FP, FN    int
	Precision     float64
	Recall        float64
	F1            float64
	LatencyMeanMs float64
	LatencyP95Ms  float64
}

// domainCompRow is one row of the domain-aware competition table.
type domainCompRow struct {
	Name      string
	Stages    int
	Modern    corpusMetrics
	Classical corpusMetrics
	Combined  float64 // F1 over union of TP/FP/FN
}

// ─── corpus loading ───────────────────────────────────────────────────────────

// loadNDJSONAllLines loads every line from an NDJSON file as a CompetitionEntry.
// Unlike loadCompetitionEntries (which reads one entry per file), this reads
// all entries from a single multi-entry NDJSON file.
func loadNDJSONAllLines(t *testing.T, path string) []CompetitionEntry {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var entries []CompetitionEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw corpusEntry
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			t.Fatalf("parse line in %s: %v", path, err)
		}
		snap, err := entryToSnapshot(&raw)
		if err != nil {
			t.Fatalf("convert snapshot in %s: %v", path, err)
		}
		var expected []string
		for _, exp := range raw.Expected {
			expected = append(expected, exp.FindingType)
		}
		entries = append(entries, CompetitionEntry{
			ID:       raw.EntryID,
			Snap:     snap,
			Expected: expected,
		})
	}
	return entries
}

// ─── measurement ─────────────────────────────────────────────────────────────

// measureWithDomainContext runs all entries through a variant with the given
// DomainContext injected into each config copy and returns corpusMetrics.
func measureWithDomainContext(
	variant ArchVariant,
	entries []CompetitionEntry,
	dc *DomainContext,
) corpusMetrics {
	var totalTP, totalFP, totalFN int
	var latencies []float64

	for _, entry := range entries {
		cfg := variant.Config
		cfg.DomainContext = dc

		start := time.Now()
		result := Run(entry.Snap, cfg)
		elapsed := time.Since(start)
		latencies = append(latencies, float64(elapsed.Microseconds())/1000.0) // ms

		// Post-inhibition findings take precedence when the inhibitor is enabled.
		actualTypes := make(map[string]bool)
		for _, f := range result.Findings {
			actualTypes[FindingTypeString(f.FindingType)] = true
		}
		if cfg.UseInhibitor && result.Report != nil {
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

	prec := 0.0
	if totalTP+totalFP > 0 {
		prec = float64(totalTP) / float64(totalTP+totalFP)
	}
	rec := 0.0
	if totalTP+totalFN > 0 {
		rec = float64(totalTP) / float64(totalTP+totalFN)
	}
	f1 := 0.0
	if prec+rec > 0 {
		f1 = 2 * prec * rec / (prec + rec)
	}

	sort.Float64s(latencies)
	return corpusMetrics{
		TP:            totalTP,
		FP:            totalFP,
		FN:            totalFN,
		Precision:     prec,
		Recall:        rec,
		F1:            f1,
		LatencyMeanMs: mean(latencies),
		LatencyP95Ms:  percentile(latencies, 0.95),
	}
}

// combinedF1 computes the F1 score over the union of two corpusMetrics sets.
func combinedF1(a, b corpusMetrics) float64 {
	tp := a.TP + b.TP
	fp := a.FP + b.FP
	fn := a.FN + b.FN
	prec := 0.0
	if tp+fp > 0 {
		prec = float64(tp) / float64(tp+fp)
	}
	rec := 0.0
	if tp+fn > 0 {
		rec = float64(tp) / float64(tp+fn)
	}
	if prec+rec == 0 {
		return 0
	}
	return 2 * prec * rec / (prec + rec)
}

// ─── competition test ─────────────────────────────────────────────────────────

// TestDomainAwareCompetition is the Task 4 domain-aware architecture competition.
//
//   - Modern:    9 test conversations, nil DomainContext
//   - Classical: 43 Republic entries,  DomainContext{TextEra:"classical", Confidence:0.85}
//   - Variants:  A-full-cortex, B-no-feedback, C-no-modulation, D-inhibitor-only, E-pre-cortex
func TestDomainAwareCompetition(t *testing.T) {
	// ── Paths ─────────────────────────────────────────────────────────────────
	modernDir := filepath.Join("..", "..", "data", "test-conversations")
	if _, err := os.Stat(modernDir); os.IsNotExist(err) {
		t.Skip("test-conversations directory not found")
	}
	classicalPath := filepath.Join("..", "..", "data", "library", "corpus", "classical-v1.ndjson")
	if _, err := os.Stat(classicalPath); os.IsNotExist(err) {
		t.Skip("classical corpus not found at " + classicalPath)
	}

	// ── Load corpora ──────────────────────────────────────────────────────────
	modernEntries := loadCompetitionEntries(t, modernDir) // one entry per ndjson file
	classicalEntries := loadNDJSONAllLines(t, classicalPath)
	t.Logf("Modern corpus:    %d entries", len(modernEntries))
	t.Logf("Classical corpus: %d entries", len(classicalEntries))

	// ── Domain contexts ───────────────────────────────────────────────────────
	modernDC := (*DomainContext)(nil)
	classicalDC := &DomainContext{
		TextEra:       "classical",
		PrimaryDomain: "philosophy",
		Confidence:    0.85,
	}

	// ── Load Layer 0 resources ────────────────────────────────────────────────
	variants := AllVariants()
	profiles, err := LoadLangProfiles(filepath.Join("..", "..", "data", "language-profiles"))
	if err != nil {
		t.Skipf("language profiles not found: %v", err)
	}
	blocklist, err := LoadBlocklist(filepath.Join("..", "..", "data", "blocklists", "default.txt"))
	if err != nil {
		t.Skipf("blocklist not found: %v", err)
	}
	for i := range variants {
		if variants[i].Config.UseLayer0 {
			variants[i].Config.Layer0.Toxicity.Blocklist = blocklist
			variants[i].Config.Layer0.Language.Profiles = profiles
		}
	}

	// ── Measure each variant ──────────────────────────────────────────────────
	var rows []domainCompRow
	for _, v := range variants {
		mod := measureWithDomainContext(v, modernEntries, modernDC)
		cls := measureWithDomainContext(v, classicalEntries, classicalDC)
		rows = append(rows, domainCompRow{
			Name:      v.Info.Name,
			Stages:    v.Info.StageCount,
			Modern:    mod,
			Classical: cls,
			Combined:  combinedF1(mod, cls),
		})
	}

	// ── Print table ───────────────────────────────────────────────────────────
	t.Log("\n========== DOMAIN-AWARE ARCHITECTURE COMPETITION ==========")
	t.Log("")
	t.Logf("%-22s %9s %10s %11s %12s %6s",
		"Variant", "Modern F1", "Classic F1", "Combined F1", "Latency(ms)", "Stages")
	t.Log(strings.Repeat("-", 75))
	for _, r := range rows {
		t.Logf("%-22s %9.3f %10.3f %11.3f %12.3f %6d",
			r.Name, r.Modern.F1, r.Classical.F1, r.Combined, r.Modern.LatencyMeanMs, r.Stages)
	}

	// ── Identify winners ──────────────────────────────────────────────────────
	modernWinner := rows[0]
	classicalWinner := rows[0]
	combinedWinner := rows[0]
	for _, r := range rows[1:] {
		if r.Modern.F1 > modernWinner.Modern.F1 {
			modernWinner = r
		}
		if r.Classical.F1 > classicalWinner.Classical.F1 {
			classicalWinner = r
		}
		if r.Combined > combinedWinner.Combined {
			combinedWinner = r
		}
	}

	t.Logf("\nModern winner:    %s (F1=%.3f)", modernWinner.Name, modernWinner.Modern.F1)
	t.Logf("Classical winner: %s (F1=%.3f)", classicalWinner.Name, classicalWinner.Classical.F1)
	t.Logf("Combined winner:  %s (F1=%.3f)", combinedWinner.Name, combinedWinner.Combined)

	// ── Key questions ─────────────────────────────────────────────────────────
	findRow := func(name string) domainCompRow {
		for _, r := range rows {
			if r.Name == name {
				return r
			}
		}
		return domainCompRow{Name: "NOT FOUND"}
	}
	varD := findRow("D-inhibitor-only")
	varA := findRow("A-full-cortex")

	t.Log("")
	t.Log("Key questions:")

	if varD.Modern.F1 >= modernWinner.Modern.F1-1e-9 {
		t.Logf("  Q1 Modern winner still D?  YES — D F1=%.3f", varD.Modern.F1)
	} else {
		t.Logf("  Q1 Modern winner still D?  NO  — D=%.3f, winner=%s (%.3f)",
			varD.Modern.F1, modernWinner.Name, modernWinner.Modern.F1)
	}

	if classicalWinner.Name != "D-inhibitor-only" {
		t.Logf("  Q2 Domain context changes classical winner?  YES → %s (F1=%.3f)",
			classicalWinner.Name, classicalWinner.Classical.F1)
	} else {
		t.Logf("  Q2 Domain context changes classical winner?  NO  → D-inhibitor-only (F1=%.3f)",
			varD.Classical.F1)
	}

	if varA.Classical.F1 > varD.Classical.F1 {
		t.Logf("  Q3 Full pipeline (A) > D on classical?  YES — A=%.3f, D=%.3f",
			varA.Classical.F1, varD.Classical.F1)
	} else {
		t.Logf("  Q3 Full pipeline (A) > D on classical?  NO  — A=%.3f, D=%.3f",
			varA.Classical.F1, varD.Classical.F1)
	}

	t.Logf("  Q4 Combined winner?  %s (F1=%.3f)", combinedWinner.Name, combinedWinner.Combined)

	// ── Write report ──────────────────────────────────────────────────────────
	outPath := filepath.Join("..", "..", "data", "competitions", "DOMAIN_AWARE_COMPETITION.md")
	if err := writeDomainReport(outPath, rows, modernWinner, classicalWinner, combinedWinner,
		len(modernEntries), len(classicalEntries), varD, varA); err != nil {
		t.Logf("WARNING: could not write report: %v", err)
	} else {
		t.Logf("Results written to %s", outPath)
	}

	// ── Sanity assertions ─────────────────────────────────────────────────────
	for _, r := range rows {
		if r.Modern.TP+r.Classical.TP == 0 {
			t.Errorf("%s: zero true positives across both corpora", r.Name)
		}
	}
}

// ─── report writer ────────────────────────────────────────────────────────────

func writeDomainReport(
	path string,
	rows []domainCompRow,
	modernWinner, classicalWinner, combinedWinner domainCompRow,
	nModern, nClassical int,
	varD, varA domainCompRow,
) error {
	var sb strings.Builder

	w := func(format string, args ...interface{}) {
		sb.WriteString(fmt.Sprintf(format, args...))
	}

	w("# Domain-Aware Architecture Competition — Task 4\n\n")
	w("**Date:** 2026-03-15  \n")
	w("**Modern corpus:**    %d conversations (`data/test-conversations/`)  \n", nModern)
	w("**Classical corpus:** %d Republic entries (`data/library/corpus/classical-v1.ndjson`)  \n\n", nClassical)

	w("## Setup\n\n")
	w("- **Modern entries:** `DomainContext = nil` — all pipeline defaults apply.\n")
	w("- **Classical entries:** `DomainContext{TextEra:\"classical\", PrimaryDomain:\"philosophy\", Confidence:0.85}`\n")
	w("  - `ScopeGuard.DriftThreshold` = 0.70 (default 0.79)\n")
	w("  - `ScopeGuard.SustainedTurns` = 3 (default 8)\n")
	w("  - `Calibrator.MinCertaintyWords` = 8 (default 5)\n")
	w("  - Anchoring detector: **skipped** (no numeric anchoring in classical text)\n")
	w("  - ConceptualAnchoring detector: **active** (propositional variant for classical text)\n\n")

	w("## Results\n\n")

	w("### Modern Corpus (nil DomainContext, %d entries)\n\n", nModern)
	w("| Variant | Precision | Recall | F1 | TP | FP | FN |\n")
	w("|---------|-----------|--------|----|----|----|----|\n")
	for _, r := range rows {
		w("| %-22s | %.3f | %.3f | **%.3f** | %d | %d | %d |\n",
			r.Name, r.Modern.Precision, r.Modern.Recall, r.Modern.F1,
			r.Modern.TP, r.Modern.FP, r.Modern.FN)
	}

	w("\n### Classical Corpus (DomainContext classical confidence=0.85, %d entries)\n\n", nClassical)
	w("| Variant | Precision | Recall | F1 | TP | FP | FN |\n")
	w("|---------|-----------|--------|----|----|----|----|\n")
	for _, r := range rows {
		w("| %-22s | %.3f | %.3f | **%.3f** | %d | %d | %d |\n",
			r.Name, r.Classical.Precision, r.Classical.Recall, r.Classical.F1,
			r.Classical.TP, r.Classical.FP, r.Classical.FN)
	}

	w("\n### Combined Summary\n\n")
	w("| Variant | Modern F1 | Classical F1 | Combined F1 | Latency(ms) | Stages |\n")
	w("|---------|-----------|-------------|-------------|-------------|--------|\n")
	for _, r := range rows {
		w("| %-22s | %.3f | %.3f | **%.3f** | %.3f | %d |\n",
			r.Name, r.Modern.F1, r.Classical.F1, r.Combined,
			r.Modern.LatencyMeanMs, r.Stages)
	}

	w("\n## Winners\n\n")
	w("| Category | Winner | F1 |\n")
	w("|----------|--------|----|\n")
	w("| Modern   | %s | %.3f |\n", modernWinner.Name, modernWinner.Modern.F1)
	w("| Classical | %s | %.3f |\n", classicalWinner.Name, classicalWinner.Classical.F1)
	w("| Combined | %s | %.3f |\n", combinedWinner.Name, combinedWinner.Combined)

	w("\n## Key Questions\n\n")

	// Q1
	w("### Q1: Does D-inhibitor-only still win on modern text?\n\n")
	if varD.Modern.F1 >= modernWinner.Modern.F1-1e-9 {
		w("**YES** — D-inhibitor-only F1=%.3f (tied for best or best on modern corpus).\n\n",
			varD.Modern.F1)
	} else {
		w("**NO** — D-inhibitor-only F1=%.3f; modern winner is **%s** (F1=%.3f).\n\n",
			varD.Modern.F1, modernWinner.Name, modernWinner.Modern.F1)
	}

	// Q2
	w("### Q2: Does domain context change the winner on classical text?\n\n")
	if classicalWinner.Name != "D-inhibitor-only" {
		w("**YES** — Classical winner is **%s** (F1=%.3f). D-inhibitor-only scores F1=%.3f on classical text.\n",
			classicalWinner.Name, classicalWinner.Classical.F1, varD.Classical.F1)
		w("\nDomain adjustments (lower DriftThreshold, lower SustainedTurns, higher MinCertaintyWords,\n")
		w("skip numeric anchoring, activate conceptual anchoring) shift the balance between variants.\n\n")
	} else {
		w("**NO** — D-inhibitor-only still wins on classical text (F1=%.3f).\n\n", varD.Classical.F1)
	}

	// Q3
	w("### Q3: Does the full pipeline (A) outperform D on classical text?\n\n")
	if varA.Classical.F1 > varD.Classical.F1 {
		w("**YES** — A-full-cortex F1=%.3f vs D-inhibitor-only F1=%.3f on classical corpus.\n",
			varA.Classical.F1, varD.Classical.F1)
		w("The extra layers (modulation, salience, metacognition) add value on classical text.\n\n")
	} else {
		w("**NO** — A-full-cortex F1=%.3f vs D-inhibitor-only F1=%.3f on classical corpus.\n",
			varA.Classical.F1, varD.Classical.F1)
		w("The extra layers do not improve over the minimal pipeline on this text.\n\n")
	}

	// Q4
	w("### Q4: Which variant wins on COMBINED (modern + classical)?\n\n")
	w("**%s** — combined F1=%.3f (TP/FP/FN aggregated over both corpora).\n\n",
		combinedWinner.Name, combinedWinner.Combined)

	w("## Notes\n\n")
	w("- Combined F1 computed over the union of TP/FP/FN from both corpora.\n")
	w("- Latency reported is modern corpus only (classical entries have a similar latency profile).\n")
	w("- Layer 0 resources (language profiles + blocklist) loaded for variants A, B, C which use Layer 0.\n")
	w("- Variant F (ML-enriched) excluded — requires Ollama and is tested separately.\n")
	w("- Previous competition (data/library/FULL_CORPUS_COMPETITION.md) ran without domain context.\n")
	w("  This competition isolates the effect of domain-aware configuration on each sub-corpus.\n")

	return os.WriteFile(path, []byte(sb.String()), 0o644)
}
