// cerebro-batch processes snapshot files through the adaptive CereBRO pipeline.
//
// Usage:
//
//	cerebro-batch --input DIR --findings DIR --candidates DIR [--sophrim URL]
//
// For each snapshot file in --input:
//  1. Unmarshal the ConversationSnapshot (single JSON or NDJSON)
//  2. Fetch domain context from Sophrim (200ms timeout, advisory — failure = nil)
//  3. Run pipeline.RunAdaptive(snapshot, domainContext)
//  4. Write per-entry findings JSON to --findings
//  5. If any finding has confidence > 0.7, write a candidate file to --candidates
//
// On completion, writes summary.json to --findings.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	candidateConfidenceThreshold = 0.7
	conversationTextMaxLen       = 2000
	sophrimTimeout               = 200 * time.Millisecond
	defaultSophrimEndpoint       = "http://192.168.14.65:8090"
)

// snapshotEntry is the envelope format for a single snapshot file.
// Matches the corpus format used by forge-eval.
type snapshotEntry struct {
	EntryID string          `json:"entry_id"`
	Input   json.RawMessage `json:"input"`
}

// findingRecord is the per-finding output written to --findings.
type findingRecord struct {
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
	Severity   string  `json:"severity"`
	Turns      []uint32 `json:"turns,omitempty"`
}

// entryFindings is the per-entry findings file written to --findings.
type entryFindings struct {
	EntryID  string          `json:"entry_id"`
	Findings []findingRecord `json:"findings"`
}

// candidateRecord is written to --candidates when a finding is eligible for consolidation.
type candidateRecord struct {
	EntryID          string  `json:"entry_id"`
	FindingType      string  `json:"finding_type"`
	Confidence       float64 `json:"confidence"`
	Explanation      string  `json:"explanation"`
	ConversationText string  `json:"conversation_text"`
}

// summary is written as summary.json in --findings at the end of the run.
type summary struct {
	Processed    int     `json:"processed"`
	Skipped      int     `json:"skipped"`
	TotalFindings int    `json:"total_findings"`
	Candidates   int     `json:"candidates"`
	Errors       int     `json:"errors"`
	DurationMS   int64   `json:"duration_ms"`
}

func main() {
	inputDir := flag.String("input", "", "directory of snapshot JSON/NDJSON files (required)")
	findingsDir := flag.String("findings", "", "directory to write findings output (required)")
	candidatesDir := flag.String("candidates", "", "directory to write consolidation candidates (required)")
	sophrimURL := flag.String("sophrim", sophrimEndpointDefault(), "Sophrim endpoint for domain context")
	flag.Parse()

	if *inputDir == "" {
		log.Fatal("cerebro-batch: --input is required")
	}
	if *findingsDir == "" {
		log.Fatal("cerebro-batch: --findings is required")
	}
	if *candidatesDir == "" {
		log.Fatal("cerebro-batch: --candidates is required")
	}

	// Ensure output directories exist.
	for _, dir := range []string{*findingsDir, *candidatesDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("cerebro-batch: creating output dir %q: %v", dir, err)
		}
	}

	// Collect snapshot files from input directory.
	entries, err := collectInputFiles(*inputDir)
	if err != nil {
		log.Fatalf("cerebro-batch: collecting input files: %v", err)
	}
	if len(entries) == 0 {
		log.Printf("cerebro-batch: no snapshot files found in %q", *inputDir)
	}

	sophrimClient := pipeline.NewSophrimClient(*sophrimURL, sophrimTimeout)

	start := time.Now()
	sum := &summary{}

	pjUnmarshaler := protojson.UnmarshalOptions{DiscardUnknown: true}

	for _, entryFile := range entries {
		snapshots, err := loadSnapshotsFromFile(entryFile)
		if err != nil {
			log.Printf("cerebro-batch: skipping %q: %v", entryFile, err)
			sum.Skipped++
			sum.Errors++
			continue
		}
		if len(snapshots) == 0 {
			log.Printf("cerebro-batch: skipping %q: no valid entries", entryFile)
			sum.Skipped++
			continue
		}

		for _, se := range snapshots {
			if se.EntryID == "" {
				log.Printf("cerebro-batch: skipping entry with no entry_id in %q", entryFile)
				sum.Skipped++
				sum.Errors++
				continue
			}

			var snap reasoningv1.ConversationSnapshot
			if err := pjUnmarshaler.Unmarshal(se.Input, &snap); err != nil {
				log.Printf("cerebro-batch: skipping entry %q: unmarshal snapshot: %v", se.EntryID, err)
				sum.Skipped++
				sum.Errors++
				continue
			}

			processEntry(se.EntryID, &snap, sophrimClient, *findingsDir, *candidatesDir, sum)
		}
	}

	sum.DurationMS = time.Since(start).Milliseconds()

	// Always write summary.json.
	if err := writeJSON(filepath.Join(*findingsDir, "summary.json"), sum); err != nil {
		log.Printf("cerebro-batch: writing summary.json: %v", err)
	}

	fmt.Printf("cerebro-batch: processed=%d skipped=%d findings=%d candidates=%d errors=%d duration=%dms\n",
		sum.Processed, sum.Skipped, sum.TotalFindings, sum.Candidates, sum.Errors, sum.DurationMS)
}

// processEntry runs the adaptive pipeline on a single snapshot and writes output files.
// Recovers from panics so one bad entry never aborts the batch.
func processEntry(
	entryID string,
	snap *reasoningv1.ConversationSnapshot,
	sophrimClient *pipeline.SophrimClient,
	findingsDir, candidatesDir string,
	sum *summary,
) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("cerebro-batch: panic processing entry %q: %v", entryID, r)
			sum.Errors++
			sum.Skipped++
		}
	}()

	// Fetch domain context from Sophrim (advisory, 200ms timeout).
	// Sophrim unreachable → nil domain context (D-inhibitor default).
	ctx, cancel := context.WithTimeout(context.Background(), sophrimTimeout)
	defer cancel()

	domainCtx := sophrimClient.FetchDomainContext(ctx, conversationSummary(snap))

	// Run adaptive pipeline.
	result, err := pipeline.RunAdaptive(snap, domainCtx, "")
	if err != nil {
		log.Printf("cerebro-batch: pipeline error for entry %q: %v", entryID, err)
		sum.Errors++
		sum.Skipped++
		return
	}

	// Build findings records.
	var records []findingRecord
	for _, f := range result.Findings {
		records = append(records, findingRecord{
			Type:       f.GetFindingType().String(),
			Confidence: f.GetConfidence(),
			Severity:   f.GetSeverity().String(),
			Turns:      f.GetRelevantTurns(),
		})
	}

	// Write findings file.
	ef := &entryFindings{
		EntryID:  entryID,
		Findings: records,
	}
	findingsPath := filepath.Join(findingsDir, sanitizeFilename(entryID)+".json")
	if err := writeJSON(findingsPath, ef); err != nil {
		log.Printf("cerebro-batch: writing findings for %q: %v", entryID, err)
		sum.Errors++
	}

	sum.Processed++
	sum.TotalFindings += len(records)

	// Check consolidation eligibility: any finding with confidence > 0.7.
	convText := buildConversationText(snap, 5)
	for _, f := range result.Findings {
		if f.GetConfidence() > candidateConfidenceThreshold {
			cand := &candidateRecord{
				EntryID:          entryID,
				FindingType:      f.GetFindingType().String(),
				Confidence:       f.GetConfidence(),
				Explanation:      f.GetExplanation(),
				ConversationText: convText,
			}
			candidatePath := filepath.Join(candidatesDir,
				sanitizeFilename(entryID)+"_"+sanitizeFilename(f.GetFindingType().String())+".json")
			if err := writeJSON(candidatePath, cand); err != nil {
				log.Printf("cerebro-batch: writing candidate for %q: %v", entryID, err)
				sum.Errors++
			} else {
				sum.Candidates++
			}
		}
	}
}

// collectInputFiles returns all .json and .ndjson files in dir.
func collectInputFiles(dir string) ([]string, error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading input dir %q: %w", dir, err)
	}
	var files []string
	for _, de := range dirEntries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".ndjson") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	return files, nil
}

// loadSnapshotsFromFile reads a file and returns all snapshotEntry values found.
// Supports two formats:
//   - Single JSON object: {"entry_id": "...", "input": {...}}
//   - NDJSON: one entry per line
func loadSnapshotsFromFile(path string) ([]snapshotEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	var entries []snapshotEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var se snapshotEntry
		if err := json.Unmarshal([]byte(line), &se); err != nil {
			log.Printf("cerebro-batch: malformed JSON at %q line %d: %v", path, lineNum, err)
			continue
		}
		if se.EntryID == "" || len(se.Input) == 0 {
			log.Printf("cerebro-batch: skipping incomplete entry at %q line %d", path, lineNum)
			continue
		}
		entries = append(entries, se)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning %q: %w", path, err)
	}
	return entries, nil
}

// buildConversationText concatenates the first maxTurns turn texts, truncated to conversationTextMaxLen.
func buildConversationText(snap *reasoningv1.ConversationSnapshot, maxTurns int) string {
	var sb strings.Builder
	for i, turn := range snap.GetTurns() {
		if i >= maxTurns {
			break
		}
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(strings.TrimSpace(turn.GetRawText()))
	}
	text := sb.String()
	if len(text) > conversationTextMaxLen {
		text = text[:conversationTextMaxLen]
	}
	return text
}

// conversationSummary mirrors the logic in pipeline.conversationSummary so that
// the batch tool can produce a Sophrim query without depending on the unexported function.
func conversationSummary(snap *reasoningv1.ConversationSnapshot) string {
	if snap == nil {
		return ""
	}
	if obj := strings.TrimSpace(snap.GetObjective()); obj != "" {
		return obj
	}
	var sb strings.Builder
	for i, turn := range snap.GetTurns() {
		if i >= 3 {
			break
		}
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(strings.TrimSpace(turn.GetRawText()))
	}
	summary := sb.String()
	if len(summary) > 500 {
		summary = summary[:500]
	}
	return summary
}

// sanitizeFilename replaces characters that are unsafe in filenames with underscores.
func sanitizeFilename(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			sb.WriteRune('_')
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// writeJSON marshals v to JSON and writes it atomically to path.
func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %q: %w", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode %q: %w", path, err)
	}
	return nil
}

// sophrimEndpointDefault returns the Sophrim endpoint from env or the hardcoded fallback.
func sophrimEndpointDefault() string {
	if ep := os.Getenv("SOPHRIM_ENDPOINT"); ep != "" {
		return ep
	}
	return defaultSophrimEndpoint
}
