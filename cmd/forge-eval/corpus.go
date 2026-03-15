// corpus.go — corpus loading for forge-eval.
// Each line of the NDJSON corpus file is a CorpusEntry containing an entry_id,
// a ConversationSnapshot (as raw JSON), and a list of expected findings.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
)

// CorpusEntry is one labelled example in the NDJSON corpus.
type CorpusEntry struct {
	EntryID  string            `json:"entry_id"`
	Input    json.RawMessage   `json:"input"`
	Expected []ExpectedFinding `json:"expected"`
}

// ExpectedFinding is a single expected finding in a corpus entry.
type ExpectedFinding struct {
	FindingType string `json:"finding_type"`
}

// LoadCorpus reads an NDJSON file and returns all valid corpus entries.
// Malformed lines are logged to stderr and skipped — they do not cause a
// hard failure so a single bad entry never aborts the entire evaluation run.
func LoadCorpus(path string) ([]*CorpusEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open corpus %q: %w", path, err)
	}
	defer f.Close()
	return readCorpus(f)
}

func readCorpus(r io.Reader) ([]*CorpusEntry, error) {
	var entries []*CorpusEntry
	scanner := bufio.NewScanner(r)
	// Corpus lines can be long (embedded conversations).
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e CorpusEntry
		if err := json.Unmarshal(line, &e); err != nil {
			log.Printf("forge-eval: skipping malformed line %d: %v", lineNum, err)
			continue
		}
		if e.EntryID == "" {
			log.Printf("forge-eval: skipping line %d: missing entry_id", lineNum)
			continue
		}
		if len(e.Input) == 0 {
			log.Printf("forge-eval: skipping entry %q: missing input", e.EntryID)
			continue
		}
		entries = append(entries, &e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning corpus: %w", err)
	}
	return entries, nil
}
