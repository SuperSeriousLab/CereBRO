package adversarial

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// TestRunAdversarialEvolution runs the full genetic loop against the real Ollama
// instance and exports the top 5 survivors as NDJSON corpus entries.
//
// Skip with -short. Run with:
//
//	go test -run TestRunAdversarialEvolution -v -timeout 30m ./internal/adversarial/
func TestRunAdversarialEvolution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-Ollama evolution run under -short")
	}

	// ── Configuration ──────────────────────────────────────────────────────
	ollamaCfg := OllamaConfig{
		URL:     "http://10.70.70.14:11434",
		Model:   "glm-4.7-flash:q4_K_M",
		Timeout: 120 * time.Second, // model is slow (~60-90 s/call on this host)
	}

	// NOTE: Ollama on this host is ~60-90 s/call.
	// pop=5 × gen=3 = 15 evaluations ≈ 15–22 min — fits well under the 60-min
	// test timeout.  Increase when a faster model or host is available.
	evoCfg := EvolutionConfig{
		PopSize:     5,
		Generations: 3,
		EliteCount:  1,
		TournSize:   2,
		MutateRate:  0.3,
		Seed:        0, // time-seeded for diversity
	}

	pipelineCfg := pipeline.DefaultPipelineConfig()

	// Use a shared HTTP client with a generous transport timeout.
	client := &http.Client{Timeout: 60 * time.Second}

	// ── Sanity-check Ollama reachability before starting ───────────────────
	t.Log("[pre-flight] checking Ollama reachability …")
	pingReq, err := http.NewRequest(http.MethodGet, ollamaCfg.URL, nil)
	if err != nil {
		t.Fatalf("could not build Ollama ping request: %v", err)
	}
	pingResp, err := client.Do(pingReq)
	if err != nil {
		t.Fatalf("Ollama not reachable at %s: %v — is the Ollama server running?", ollamaCfg.URL, err)
	}
	pingResp.Body.Close()
	t.Logf("[pre-flight] Ollama reachable, status=%d", pingResp.StatusCode)

	// ── Run evolution ───────────────────────────────────────────────────────
	t.Logf("[evo] starting: pop=%d  gen=%d  model=%s",
		evoCfg.PopSize, evoCfg.Generations, ollamaCfg.Model)
	start := time.Now()

	result := RunEvolution(evoCfg, ollamaCfg, pipelineCfg, client)

	elapsed := time.Since(start)
	if result == nil {
		t.Fatal("RunEvolution returned nil")
	}

	t.Logf("[evo] finished in %s", elapsed.Round(time.Second))

	// ── Per-generation fitness report ───────────────────────────────────────
	t.Log("[evo] fitness progression:")
	for gen, best := range result.BestFitness {
		mean := 0.0
		if gen < len(result.MeanFitness) {
			mean = result.MeanFitness[gen]
		}
		t.Logf("  gen %d/%d  best=%.3f  mean=%.3f",
			gen+1, evoCfg.Generations, best, mean)
	}

	// ── Final population summary ────────────────────────────────────────────
	if len(result.FinalPopulation) == 0 {
		t.Fatal("final population is empty")
	}
	t.Logf("[evo] final population (%d individuals):", len(result.FinalPopulation))
	failureTypeCounts := make(map[string]int)
	for rank, ind := range result.FinalPopulation {
		var types []string
		for _, f := range ind.Template.FailureModes {
			types = append(types, f.Type)
			failureTypeCounts[f.Type]++
		}
		t.Logf("  rank %2d  fitness=%.3f  topic=%-12s  failures=[%s]",
			rank+1, ind.Fitness, ind.Template.Topic, strings.Join(types, ", "))
	}

	t.Log("[evo] failure type survival counts:")
	for ft, count := range failureTypeCounts {
		t.Logf("  %-30s  %d", ft, count)
	}

	// ── Export top 5 ───────────────────────────────────────────────────────
	const topN = 5
	top := TopN(result, topN)
	generateFn := defaultGenerateFunc(ollamaCfg, client)

	t.Logf("[export] generating corpus entries for top %d individuals …", len(top))
	entries := ExportCorpusEntries(top, generateFn, "adv-v1")
	t.Logf("[export] %d/%d entries generated (nil snaps skipped)", len(entries), len(top))

	if len(entries) == 0 {
		t.Error("[export] no corpus entries were generated — Ollama may be returning bad output")
		// Do not fatal: allow the test to report fitness data even with export failure.
		return
	}

	// ── Write NDJSON ────────────────────────────────────────────────────────
	outPath := filepath.Join("../../data/library/corpus", "adversarial-v1.ndjson")
	// Make path absolute relative to the test file location.
	absOut := filepath.Join("/home/js/eidos/CereBRO/data/library/corpus", "adversarial-v1.ndjson")

	if err := os.MkdirAll(filepath.Dir(absOut), 0o755); err != nil {
		t.Fatalf("could not create corpus output directory: %v", err)
	}

	f, err := os.Create(absOut)
	if err != nil {
		t.Fatalf("could not create output file %s: %v", absOut, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			t.Errorf("failed to encode entry %s: %v", entry.EntryID, err)
		}
	}

	t.Logf("[export] wrote %d entries to %s", len(entries), absOut)

	// ── Validate output ─────────────────────────────────────────────────────
	t.Log("[validate] reading back NDJSON …")
	rf, err := os.Open(absOut)
	if err != nil {
		t.Fatalf("could not re-open output file: %v", err)
	}
	defer rf.Close()

	var validEntries, gibberishEntries int
	scanner := bufio.NewScanner(rf)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MiB line buffer

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry CorpusEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("[validate] invalid JSON line: %v", err)
			continue
		}

		// Basic coherence checks.
		issues := validateEntry(entry)
		if len(issues) > 0 {
			t.Logf("[validate] entry %s has issues: %s", entry.EntryID, strings.Join(issues, "; "))
			gibberishEntries++
		} else {
			validEntries++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Errorf("[validate] scanner error: %v", err)
	}

	t.Logf("[validate] valid=%d  flagged=%d", validEntries, gibberishEntries)

	// ── Final assertions ────────────────────────────────────────────────────
	if len(entries) == 0 {
		t.Error("no corpus entries were exported")
	}

	bestFinal := result.FinalPopulation[0].Fitness
	_ = bestFinal // informational; don't assert a specific threshold

	// Log a short summary for the caller.
	t.Logf("[summary] generations=%d  best_final=%.3f  corpus_entries=%d  out=%s",
		evoCfg.Generations, bestFinal, len(entries), absOut)
	_ = outPath // suppress unused-variable warning
}

// validateEntry performs lightweight coherence checks on an exported corpus entry.
// Returns a slice of issue strings (empty = OK).
func validateEntry(entry CorpusEntry) []string {
	var issues []string

	if entry.EntryID == "" {
		issues = append(issues, "missing entry_id")
	}
	if entry.Input.TotalTurns == 0 {
		issues = append(issues, "total_turns=0")
	}
	if len(entry.Input.Turns) == 0 {
		issues = append(issues, "no turns")
	}
	if len(entry.Expected) == 0 {
		issues = append(issues, "no expected findings")
	}

	// Check for gibberish: turns should have non-empty text.
	emptyTurns := 0
	shortTurns := 0
	for _, turn := range entry.Input.Turns {
		text := strings.TrimSpace(turn.RawText)
		if text == "" {
			emptyTurns++
		} else if len(text) < 10 {
			shortTurns++
		}
	}
	if emptyTurns > 0 {
		issues = append(issues, fmt.Sprintf("%d empty turn(s)", emptyTurns))
	}
	if shortTurns > len(entry.Input.Turns)/2 {
		issues = append(issues, fmt.Sprintf("%d/%d turns suspiciously short (possible gibberish)", shortTurns, len(entry.Input.Turns)))
	}

	// Check expected finding types are non-empty.
	for i, ex := range entry.Expected {
		if ex.FindingType == "" {
			issues = append(issues, fmt.Sprintf("expected[%d] has empty finding_type", i))
		}
	}

	return issues
}
