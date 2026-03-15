// ML Enrichment Competition Runner
//
// Compares Variant A (PURE full-cortex) vs Variant F (ML-enriched full-cortex)
// on a 15-entry representative corpus subset.
//
// Design: Makes exactly ONE ML pass over the corpus — all metrics are derived
// from the same set of run results, not separate measurement passes.
//
// Usage:
//
//	cd /home/js/eidos/CereBRO
//	go run data/competitions/ml_enrichment_runner/main.go
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// ----------------------------------------------------------------
// Corpus loading helpers
// ----------------------------------------------------------------

type corpusEntry struct {
	EntryID  string          `json:"entry_id"`
	Input    json.RawMessage `json:"input"`
	Expected []struct {
		FindingType string `json:"finding_type"`
	} `json:"expected"`
}

type snapshotJSON struct {
	Turns []struct {
		TurnNumber uint32 `json:"turn_number"`
		Speaker    string `json:"speaker"`
		RawText    string `json:"raw_text"`
	} `json:"turns"`
	Objective  string `json:"objective"`
	TotalTurns uint32 `json:"total_turns"`
}

func loadNDJSON(path string) ([]*corpusEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []*corpusEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e corpusEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parse line: %w", err)
		}
		entries = append(entries, &e)
	}
	return entries, nil
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

// ----------------------------------------------------------------
// Run result: findings + latency + ML enrichment count
// ----------------------------------------------------------------

type runResult struct {
	Found         map[string]bool
	LatencyMS     float64
	MLEnrichments int
}

// runOnce executes the pipeline once and captures all needed data.
func runOnce(snap *reasoningv1.ConversationSnapshot, cfg pipeline.PipelineConfig) runResult {
	start := time.Now()
	r := pipeline.Run(snap, cfg)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	found := make(map[string]bool)
	if cfg.UseInhibitor && r.Report != nil {
		for _, f := range r.Report.GetFindings() {
			found[pipeline.FindingTypeString(f.FindingType)] = true
		}
	} else {
		for _, f := range r.Findings {
			found[pipeline.FindingTypeString(f.FindingType)] = true
		}
	}

	return runResult{
		Found:         found,
		LatencyMS:     latency,
		MLEnrichments: len(r.MLEnrichments),
	}
}

// ----------------------------------------------------------------
// Metrics helpers
// ----------------------------------------------------------------

func pctile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi || hi >= len(sorted) {
		return sorted[lo]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func meanF(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range vals {
		s += v
	}
	return s / float64(len(vals))
}

type overallMetrics struct {
	TP, FP, FN      int
	Precision       float64
	Recall          float64
	F1              float64
	FPR             float64
	MeanLatencyMS   float64
	P95LatencyMS    float64
	P99LatencyMS    float64
}

type detectorMetrics struct {
	Name      string
	TP, FP, FN int
	Precision  float64
	Recall     float64
	F1         float64
}

// ----------------------------------------------------------------
// Compute metrics from pre-collected run results
// ----------------------------------------------------------------

func computeOverall(results []runResult, entries []pipeline.CompetitionEntry) overallMetrics {
	tp, fp, fn := 0, 0, 0
	cleanCount, cleanWithFindings := 0, 0
	var latencies []float64

	for i, res := range results {
		e := entries[i]
		expected := make(map[string]bool)
		for _, et := range e.Expected {
			expected[et] = true
		}
		for et := range expected {
			if res.Found[et] {
				tp++
			} else {
				fn++
			}
		}
		for ft := range res.Found {
			if !expected[ft] {
				fp++
			}
		}
		if len(e.Expected) == 0 {
			cleanCount++
			if len(res.Found) > 0 {
				cleanWithFindings++
			}
		}
		latencies = append(latencies, res.LatencyMS)
	}

	prec := 0.0
	if tp+fp > 0 {
		prec = float64(tp) / float64(tp+fp)
	}
	rec := 0.0
	if tp+fn > 0 {
		rec = float64(tp) / float64(tp+fn)
	}
	f1 := 0.0
	if prec+rec > 0 {
		f1 = 2 * prec * rec / (prec + rec)
	}
	fpr := 0.0
	if cleanCount > 0 {
		fpr = float64(cleanWithFindings) / float64(cleanCount)
	}
	sort.Float64s(latencies)

	return overallMetrics{
		TP: tp, FP: fp, FN: fn,
		Precision: prec, Recall: rec, F1: f1, FPR: fpr,
		MeanLatencyMS: meanF(latencies),
		P95LatencyMS:  pctile(latencies, 0.95),
		P99LatencyMS:  pctile(latencies, 0.99),
	}
}

func computePerDetector(results []runResult, entries []pipeline.CompetitionEntry, detectorTypes []string) []detectorMetrics {
	var out []detectorMetrics
	for _, dt := range detectorTypes {
		tp, fp, fn := 0, 0, 0
		for i, res := range results {
			e := entries[i]
			expectedHas := false
			for _, et := range e.Expected {
				if et == dt {
					expectedHas = true
					break
				}
			}
			detectedHas := res.Found[dt]
			if expectedHas && detectedHas {
				tp++
			} else if expectedHas && !detectedHas {
				fn++
			} else if !expectedHas && detectedHas {
				fp++
			}
		}
		prec := 0.0
		if tp+fp > 0 {
			prec = float64(tp) / float64(tp+fp)
		}
		rec := 0.0
		if tp+fn > 0 {
			rec = float64(tp) / float64(tp+fn)
		}
		f1 := 0.0
		if prec+rec > 0 {
			f1 = 2 * prec * rec / (prec + rec)
		}
		out = append(out, detectorMetrics{
			Name: dt, TP: tp, FP: fp, FN: fn,
			Precision: prec, Recall: rec, F1: f1,
		})
	}
	return out
}

// ----------------------------------------------------------------
// Main
// ----------------------------------------------------------------

func main() {
	repoRoot := "/home/js/eidos/CereBRO"
	corpusPath := filepath.Join(repoRoot, "data", "corpus", "full-v1.ndjson")
	profileDir := filepath.Join(repoRoot, "data", "language-profiles")
	blocklistPath := filepath.Join(repoRoot, "data", "blocklists", "default.txt")
	outPath := filepath.Join(repoRoot, "data", "competitions", "ML_ENRICHMENT_RESULTS.md")

	// ---- Load corpus ----
	allEntries, err := loadNDJSON(corpusPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR loading corpus: %v\n", err)
		os.Exit(1)
	}

	// Representative 15-entry subset: 2 per detector type + 3 clean
	detectorTypes := []string{
		"ANCHORING_BIAS",
		"SUNK_COST_FALLACY",
		"CONTRADICTION",
		"SCOPE_DRIFT",
		"CONFIDENCE_MISCALIBRATION",
		"SILENT_REVISION",
	}

	byType := make(map[string][]*corpusEntry)
	var cleanList []*corpusEntry
	for _, e := range allEntries {
		if len(e.Expected) == 0 {
			cleanList = append(cleanList, e)
		} else {
			ft := e.Expected[0].FindingType
			byType[ft] = append(byType[ft], e)
		}
	}

	var subset []*corpusEntry
	for _, dt := range detectorTypes {
		lst := byType[dt]
		count := 2
		if len(lst) < count {
			count = len(lst)
		}
		subset = append(subset, lst[:count]...)
	}
	for i := 0; i < 3 && i < len(cleanList); i++ {
		subset = append(subset, cleanList[i])
	}

	fmt.Printf("Corpus subset: %d entries\n", len(subset))

	// ---- Build CompetitionEntry slice ----
	var entries []pipeline.CompetitionEntry
	for _, e := range subset {
		snap, err := entryToSnapshot(e)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: skip %s: %v\n", e.EntryID, err)
			continue
		}
		var expected []string
		for _, ex := range e.Expected {
			expected = append(expected, ex.FindingType)
		}
		entries = append(entries, pipeline.CompetitionEntry{
			ID:       e.EntryID,
			Snap:     snap,
			Expected: expected,
		})
	}

	// ---- Build pipeline configs ----
	pureCfg := pipeline.FullCortexConfig()

	profiles, err := pipeline.LoadLangProfiles(profileDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: language profiles not found (%v)\n", err)
	} else {
		pureCfg.Layer0.Language.Profiles = profiles
	}
	blocklist, err := pipeline.LoadBlocklist(blocklistPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: blocklist not found (%v)\n", err)
	} else {
		pureCfg.Layer0.Toxicity.Blocklist = blocklist
	}

	mlCfg := pipeline.MLEnrichedConfig()
	mlCfg.Layer0.Language.Profiles = pureCfg.Layer0.Language.Profiles
	mlCfg.Layer0.Toxicity.Blocklist = pureCfg.Layer0.Toxicity.Blocklist
	mlCfg.MLEnricher.TimeoutPerTurn = 120 * time.Second

	// ---- Run PURE once per entry ----
	fmt.Printf("\nRunning Variant A (PURE full-cortex) on %d entries...\n", len(entries))
	pureResults := make([]runResult, len(entries))
	for i, e := range entries {
		fmt.Printf("  [%d/%d] %s (expected: %v)\n", i+1, len(entries), e.ID, e.Expected)
		pureResults[i] = runOnce(e.Snap, pureCfg)
		fmt.Printf("         found: %v  latency: %.1fms\n", mapKeys(pureResults[i].Found), pureResults[i].LatencyMS)
	}

	// ---- Run ML-enriched once per entry ----
	// Count total turns to set expectations
	totalTurns := 0
	for _, e := range entries {
		totalTurns += len(e.Snap.GetTurns())
	}
	fmt.Printf("\nRunning Variant F (ML-enriched) on %d entries (%d total turns = %d ML calls)...\n",
		len(entries), totalTurns, totalTurns)
	fmt.Println("Each ML call may take 30-120s for glm-4.7-flash:q4_K_M (29.9B). Total est: " +
		fmt.Sprintf("%.0f–%.0f min", float64(totalTurns)*30/60, float64(totalTurns)*120/60))
	fmt.Println()

	mlResults := make([]runResult, len(entries))
	for i, e := range entries {
		nTurns := len(e.Snap.GetTurns())
		fmt.Printf("  [%d/%d] %s (%d turns — starting ML calls)\n", i+1, len(entries), e.ID, nTurns)
		start := time.Now()
		mlResults[i] = runOnce(e.Snap, mlCfg)
		elapsed := time.Since(start)
		fmt.Printf("         found: %v  latency: %.1fms  ML enrichments: %d  elapsed: %s\n",
			mapKeys(mlResults[i].Found), mlResults[i].LatencyMS, mlResults[i].MLEnrichments, elapsed.Round(time.Second))
	}

	// ---- Compute all metrics from collected results ----
	pureOverall := computeOverall(pureResults, entries)
	mlOverall := computeOverall(mlResults, entries)
	purePerDet := computePerDetector(pureResults, entries, detectorTypes)
	mlPerDet := computePerDetector(mlResults, entries, detectorTypes)

	// ---- Build markdown ----
	var sb strings.Builder

	sb.WriteString("# ML Enrichment Competition Results\n\n")
	sb.WriteString(fmt.Sprintf("Date: %s\n", time.Now().Format("2006-01-02")))
	sb.WriteString("Variants: A (PURE full-cortex) vs F (ML-enriched full-cortex)\n")
	sb.WriteString("Model: glm-4.7-flash:q4_K_M (Ollama at 10.70.70.14:11434)\n")
	sb.WriteString(fmt.Sprintf("Corpus: %d entries (2 per detector type + 3 clean), %d total turns, %d ML calls\n\n", len(entries), totalTurns, totalTurns))

	// Summary table
	sb.WriteString("## Summary Table\n\n")
	sb.WriteString("| Variant | Precision | Recall | F1 | FPR | TP | FP | FN |\n")
	sb.WriteString("|---------|-----------|--------|-----|-----|----|----|----|\n")
	sb.WriteString(fmt.Sprintf("| A-full-cortex (PURE) | %.3f | %.3f | %.3f | %.3f | %d | %d | %d |\n",
		pureOverall.Precision, pureOverall.Recall, pureOverall.F1, pureOverall.FPR,
		pureOverall.TP, pureOverall.FP, pureOverall.FN,
	))
	sb.WriteString(fmt.Sprintf("| F-ml-enriched | %.3f | %.3f | %.3f | %.3f | %d | %d | %d |\n",
		mlOverall.Precision, mlOverall.Recall, mlOverall.F1, mlOverall.FPR,
		mlOverall.TP, mlOverall.FP, mlOverall.FN,
	))

	// Latency
	sb.WriteString("\n## Latency Comparison\n\n")
	sb.WriteString("| Variant | Mean (ms) | P95 (ms) | P99 (ms) |\n")
	sb.WriteString("|---------|-----------|----------|----------|\n")
	sb.WriteString(fmt.Sprintf("| A-full-cortex (PURE) | %.1f | %.1f | %.1f |\n",
		pureOverall.MeanLatencyMS, pureOverall.P95LatencyMS, pureOverall.P99LatencyMS))
	sb.WriteString(fmt.Sprintf("| F-ml-enriched | %.1f | %.1f | %.1f |\n",
		mlOverall.MeanLatencyMS, mlOverall.P95LatencyMS, mlOverall.P99LatencyMS))

	latRatio := mlOverall.MeanLatencyMS / pureOverall.MeanLatencyMS
	if pureOverall.MeanLatencyMS > 0 {
		sb.WriteString(fmt.Sprintf("\nML overhead: **%.0fx slower** than PURE (mean: %.1fms vs %.1fms)\n",
			latRatio, mlOverall.MeanLatencyMS, pureOverall.MeanLatencyMS))
	}

	// Per-detector analysis
	sb.WriteString("\n## Per-Detector Analysis (Precision / Recall / F1)\n\n")
	sb.WriteString("| Detector | PURE Prec | PURE Rec | PURE F1 | ML Prec | ML Rec | ML F1 | ΔF1 | ML Benefit |\n")
	sb.WriteString("|----------|-----------|----------|---------|---------|--------|-------|-----|------------|\n")
	for i, pd := range purePerDet {
		md := mlPerDet[i]
		delta := md.F1 - pd.F1
		benefit := "neutral"
		if delta > 0.05 {
			benefit = "YES — improved"
		} else if delta < -0.05 {
			benefit = "NO — degraded"
		}
		sb.WriteString(fmt.Sprintf("| %s | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %+.3f | %s |\n",
			pd.Name, pd.Precision, pd.Recall, pd.F1,
			md.Precision, md.Recall, md.F1, delta, benefit,
		))
	}

	// Per-detector narrative
	sb.WriteString("\n### Which Detectors Benefit Most from ML Enrichment?\n\n")
	for i, pd := range purePerDet {
		md := mlPerDet[i]
		delta := md.F1 - pd.F1
		direction := "no meaningful change (ΔF1 within ±0.05)"
		if delta > 0.05 {
			direction = fmt.Sprintf("IMPROVED by +%.3f F1", delta)
		} else if delta < -0.05 {
			direction = fmt.Sprintf("DEGRADED by %.3f F1", delta)
		}
		sb.WriteString(fmt.Sprintf("**%s**: PURE F1=%.3f → ML F1=%.3f (%s)\n",
			pd.Name, pd.F1, md.F1, direction))
		sb.WriteString(fmt.Sprintf("  PURE: TP=%d FP=%d FN=%d | ML: TP=%d FP=%d FN=%d\n\n",
			pd.TP, pd.FP, pd.FN, md.TP, md.FP, md.FN))
	}

	// Per-entry comparison
	sb.WriteString("## Extraction Quality — Per-Entry Comparison\n\n")
	sb.WriteString("| Entry | Expected | PURE Findings | ML Findings | ML Enrichments | Δ |\n")
	sb.WriteString("|-------|----------|---------------|-------------|----------------|---|\n")
	for i, e := range entries {
		pr := pureResults[i]
		mr := mlResults[i]
		expectedStr := strings.Join(e.Expected, ", ")
		if expectedStr == "" {
			expectedStr = "CLEAN"
		}
		pureStr := strings.Join(sortedKeys(pr.Found), ", ")
		if pureStr == "" {
			pureStr = "(none)"
		}
		mlStr := strings.Join(sortedKeys(mr.Found), ", ")
		if mlStr == "" {
			mlStr = "(none)"
		}
		delta := "="
		if len(mr.Found) > len(pr.Found) {
			delta = "ML+"
		} else if len(mr.Found) < len(pr.Found) {
			delta = "ML-"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %s |\n",
			e.ID, expectedStr, pureStr, mlStr, mr.MLEnrichments, delta,
		))
	}

	// LLM reliability
	sb.WriteString("\n## LLM Reliability\n\n")
	totalMLEnrichments := 0
	for _, r := range mlResults {
		totalMLEnrichments += r.MLEnrichments
	}
	sb.WriteString(fmt.Sprintf("- Total ML enrichment objects produced: %d / %d expected (fallback=0 on failure)\n",
		totalMLEnrichments, totalTurns))
	sb.WriteString("- Model: glm-4.7-flash:q4_K_M (29.9B parameters, Q4_K_M quantization)\n")
	sb.WriteString("- JSON format mode enforced via Ollama `format: \"json\"`\n")
	sb.WriteString("- Temperature: 0.1 (near-deterministic)\n")
	sb.WriteString("- Timeout per turn: 120s\n")
	sb.WriteString("- Fallback: failures degrade gracefully to PURE (FallbackToPure=true)\n")

	// Recommendation
	sb.WriteString("\n## Recommendation\n\n")
	f1Delta := mlOverall.F1 - pureOverall.F1
	sb.WriteString(fmt.Sprintf("Overall F1: PURE=%.3f → ML=%.3f (ΔF1=%+.3f)\n", pureOverall.F1, mlOverall.F1, f1Delta))
	if pureOverall.MeanLatencyMS > 0 {
		sb.WriteString(fmt.Sprintf("Latency: PURE mean=%.1fms → ML mean=%.1fms (%.0fx overhead)\n\n",
			pureOverall.MeanLatencyMS, mlOverall.MeanLatencyMS, latRatio))
	}

	if f1Delta > 0.02 {
		sb.WriteString("**Verdict: ML enrichment is RECOMMENDED** when latency budget allows (>1s per conversation). F1 improves meaningfully.\n\n")
	} else if f1Delta > -0.02 {
		sb.WriteString("**Verdict: ML enrichment is NEUTRAL** on overall accuracy. Its value is confined to specific detectors (see per-detector analysis). The latency cost is only justified for those detector types.\n\n")
	} else {
		sb.WriteString("**Verdict: ML enrichment is NOT RECOMMENDED** on this corpus. PURE pipeline wins on both accuracy and latency.\n\n")
	}

	sb.WriteString("### Deployment Guidance\n\n")
	sb.WriteString("ML enrichment is best treated as **opt-in infrastructure**:\n\n")
	sb.WriteString("- **Enable** when: latency budget >1s, conversation quality matters more than throughput, and the benefiting detectors are in scope\n")
	sb.WriteString("- **Disable** (default): real-time or high-throughput settings, Ollama unavailable, latency-sensitive deployments\n")
	sb.WriteString("- **PURE pipeline (Variant A)** remains the primary production path: deterministic, low-latency, no external dependencies\n")
	sb.WriteString("- Activate with `MLEnricherConfig.Enabled = true`; failures fall back to PURE automatically\n")

	// Write output
	if err := os.WriteFile(outPath, []byte(sb.String()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR writing results: %v\n", err)
		os.Exit(1)
	}

	// Console summary
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("COMPETITION COMPLETE")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("\n%-30s  Prec   Rec    F1    FPR   Mean(ms)\n", "Variant")
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("%-30s  %.3f  %.3f  %.3f  %.3f  %.1f\n", "A-full-cortex (PURE)",
		pureOverall.Precision, pureOverall.Recall, pureOverall.F1, pureOverall.FPR, pureOverall.MeanLatencyMS)
	fmt.Printf("%-30s  %.3f  %.3f  %.3f  %.3f  %.1f\n", "F-ml-enriched",
		mlOverall.Precision, mlOverall.Recall, mlOverall.F1, mlOverall.FPR, mlOverall.MeanLatencyMS)
	fmt.Printf("\nResults written to: %s\n", outPath)
}

func mapKeys(m map[string]bool) []string {
	return sortedKeys(m)
}

func sortedKeys(m map[string]bool) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
