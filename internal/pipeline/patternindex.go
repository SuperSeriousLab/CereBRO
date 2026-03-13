// Shared Pattern Index — thread-safe in-memory index of finding patterns.
//
// Loaded at startup from NDJSON corpus files. Used by:
//   - Self-Confidence Assessor: historical accuracy lookup
//   - Memory Consolidator: novel pattern detection + index update
//
// Thread safety: RWMutex protects concurrent reads (confidence) and writes (consolidation).
//
// Phase 5 deliverable.
package pipeline

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync"
)

// PatternIndex holds precomputed corpus data for historical accuracy lookups.
// Thread-safe for concurrent reads and writes.
type PatternIndex struct {
	mu       sync.RWMutex
	accuracy map[string]float64 // pattern → historical accuracy [0.0, 1.0]
	counts   map[string]int     // pattern → occurrence count (for incremental updates)
}

// NewPatternIndex creates an empty PatternIndex.
func NewPatternIndex() *PatternIndex {
	return &PatternIndex{
		accuracy: make(map[string]float64),
		counts:   make(map[string]int),
	}
}

// LoadPatternIndex builds an in-memory pattern index from an NDJSON corpus file.
// Each entry must have "expected" (array of {finding_type}) to determine pattern.
// Historical accuracy = min(1.0, count / 3.0) — 3+ entries = fully calibrated.
func LoadPatternIndex(corpusPath string) (*PatternIndex, error) {
	idx := NewPatternIndex()

	data, err := os.ReadFile(corpusPath)
	if err != nil {
		return idx, nil // no corpus = empty index
	}

	type corpusEntry struct {
		Expected []struct {
			FindingType string `json:"finding_type"`
		} `json:"expected"`
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry corpusEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		pattern := extractPatternFromExpected(entry.Expected)
		idx.counts[pattern]++
	}

	// Accuracy = min(1.0, count / 3.0) — 3+ entries = fully calibrated.
	for pattern, count := range idx.counts {
		acc := float64(count) / 3.0
		if acc > 1.0 {
			acc = 1.0
		}
		idx.accuracy[pattern] = acc
	}

	return idx, nil
}

func extractPatternFromExpected(expected []struct{ FindingType string `json:"finding_type"` }) string {
	if len(expected) == 0 {
		return "CLEAN"
	}
	types := make([]string, len(expected))
	for i, e := range expected {
		types[i] = e.FindingType
	}
	sort.Strings(types)
	return strings.Join(types, "+")
}

// Lookup returns the historical accuracy for a pattern.
// Returns (accuracy, true) if found, (0.5, false) if not.
func (idx *PatternIndex) Lookup(pattern string) (float64, bool) {
	if idx == nil {
		return 0.5, false
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if acc, ok := idx.accuracy[pattern]; ok {
		return acc, true
	}
	return 0.5, false
}

// AddEntry updates the index with a new pattern observation.
// Increments the count and recalculates accuracy.
func (idx *PatternIndex) AddEntry(pattern string) {
	if idx == nil {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.counts[pattern]++
	acc := float64(idx.counts[pattern]) / 3.0
	if acc > 1.0 {
		acc = 1.0
	}
	idx.accuracy[pattern] = acc
}

// Patterns returns all known patterns (for testing).
func (idx *PatternIndex) Patterns() []string {
	if idx == nil {
		return nil
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	patterns := make([]string, 0, len(idx.accuracy))
	for p := range idx.accuracy {
		patterns = append(patterns, p)
	}
	return patterns
}

// GetAccuracy returns the accuracy map (read-only snapshot, for testing).
func (idx *PatternIndex) GetAccuracy() map[string]float64 {
	if idx == nil {
		return nil
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	result := make(map[string]float64, len(idx.accuracy))
	for k, v := range idx.accuracy {
		result[k] = v
	}
	return result
}

// lookupHistoricalFromIndex returns historical accuracy from a PatternIndex.
// Used by the Self-Confidence Assessor.
func lookupHistoricalFromIndex(pattern string, index *PatternIndex) float64 {
	if index == nil {
		return 0.5
	}
	acc, found := index.Lookup(pattern)
	if !found {
		return 0.5
	}
	return acc
}

