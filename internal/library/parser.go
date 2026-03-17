// Package library provides tools for parsing classical philosophical texts
// into ConversationSnapshot format for the CereBRO cognitive pipeline.
package library

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// CorpusEntry matches the NDJSON format used by data/corpus/*.ndjson.
type CorpusEntry struct {
	EntryID  string           `json:"entry_id"`
	Input    SnapshotJSON     `json:"input"`
	Expected []ExpectedResult `json:"expected"`
	Metadata *EntryMetadata   `json:"metadata,omitempty"`
}

// SnapshotJSON is the JSON representation of a ConversationSnapshot.
type SnapshotJSON struct {
	Turns      []TurnJSON `json:"turns"`
	Objective  string     `json:"objective"`
	TotalTurns int        `json:"total_turns"`
}

// TurnJSON is the JSON representation of a Turn.
type TurnJSON struct {
	TurnNumber int    `json:"turn_number"`
	Speaker    string `json:"speaker"`
	RawText    string `json:"raw_text"`
}

// ExpectedResult describes an expected finding.
type ExpectedResult struct {
	FindingType string `json:"finding_type"`
}

// EntryMetadata holds provenance information.
type EntryMetadata struct {
	Source string `json:"source,omitempty"`
	Domain string `json:"domain,omitempty"`
	Notes  string `json:"notes,omitempty"`
}

// DialogueSegment is a chunk of dialogue suitable for one corpus entry.
type DialogueSegment struct {
	ID        string
	Objective string
	Turns     []TurnJSON
	Expected  []string // finding type names
	Notes     string
}

// ToCorpusEntry converts a DialogueSegment to a CorpusEntry.
func (s *DialogueSegment) ToCorpusEntry() CorpusEntry {
	expected := make([]ExpectedResult, len(s.Expected))
	for i, e := range s.Expected {
		expected[i] = ExpectedResult{FindingType: e}
	}
	return CorpusEntry{
		EntryID: s.ID,
		Input: SnapshotJSON{
			Turns:      s.Turns,
			Objective:  s.Objective,
			TotalTurns: len(s.Turns),
		},
		Expected: expected,
		Metadata: &EntryMetadata{
			Source: "plato_republic_b1",
			Domain: "philosophy",
			Notes:  s.Notes,
		},
	}
}

// ToProtoSnapshot converts a SnapshotJSON to a protobuf ConversationSnapshot.
func (s *SnapshotJSON) ToProtoSnapshot() *reasoningv1.ConversationSnapshot {
	turns := make([]*reasoningv1.Turn, len(s.Turns))
	for i, t := range s.Turns {
		turns[i] = &reasoningv1.Turn{
			TurnNumber: uint32(t.TurnNumber),
			Speaker:    t.Speaker,
			RawText:    t.RawText,
		}
	}
	objective := s.Objective
	keywords := extractKeywords(objective)
	return &reasoningv1.ConversationSnapshot{
		Turns:             turns,
		Objective:         objective,
		ObjectiveKeywords: keywords,
		TotalTurns:        uint32(len(turns)),
	}
}

// extractKeywords does basic keyword extraction from objective text.
func extractKeywords(text string) []string {
	stopwords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "shall": true, "can": true, "to": true,
		"of": true, "in": true, "for": true, "on": true, "with": true,
		"at": true, "by": true, "from": true, "as": true, "into": true,
		"through": true, "during": true, "before": true, "after": true,
		"and": true, "but": true, "or": true, "nor": true, "not": true,
		"so": true, "yet": true, "both": true, "either": true, "neither": true,
		"it": true, "its": true, "this": true, "that": true, "these": true,
		"those": true, "what": true, "which": true, "who": true, "whom": true,
		"i": true, "you": true, "he": true, "she": true, "we": true, "they": true,
	}
	words := strings.Fields(strings.ToLower(text))
	var keywords []string
	seen := map[string]bool{}
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()-")
		if w == "" || stopwords[w] || seen[w] {
			continue
		}
		seen[w] = true
		keywords = append(keywords, w)
	}
	return keywords
}

// WriteNDJSON writes corpus entries as NDJSON to a file.
func WriteNDJSON(path string, entries []CorpusEntry) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return fmt.Errorf("encode entry %s: %w", e.EntryID, err)
		}
	}
	return nil
}

// WriteAnnotations writes expected findings in the same format as
// data/test-conversations/expected.json.
func WriteAnnotations(path string, annotations map[string][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(annotations)
}
