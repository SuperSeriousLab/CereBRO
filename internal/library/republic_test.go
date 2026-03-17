package library

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestParseRepublicBook1 reads the raw Gutenberg text and extracts
// the three dialogues into corpus-ready NDJSON.
//
// The Jowett translation is narrative: Socrates narrates in first person.
// Speech attribution uses patterns like "I said", "he replied", "said Thrasymachus".
//
// Run with: go test -run TestParseRepublicBook1 -v
func TestParseRepublicBook1(t *testing.T) {
	srcPath := "../../data/library/sources/plato/republic_book1.txt"
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		t.Skip("Republic text not downloaded yet")
	}

	lines, err := readLines(srcPath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	// Book 1 dialogue starts at "I went down yesterday to the Piraeus"
	// and ends before "BOOK II" in the actual text (around line 10341).
	dialogueStart := findLine(lines, "I went down yesterday to the Piraeus")
	if dialogueStart < 0 {
		t.Fatal("could not find dialogue start")
	}
	dialogueEnd := findLineFrom(lines, dialogueStart, "BOOK II")
	if dialogueEnd < 0 {
		dialogueEnd = len(lines)
	}
	t.Logf("Dialogue: lines %d-%d (%d lines)", dialogueStart, dialogueEnd, dialogueEnd-dialogueStart)

	book1 := lines[dialogueStart:dialogueEnd]

	// Split into paragraphs (blank-line separated).
	paragraphs := splitParagraphs(book1)
	t.Logf("Total paragraphs: %d", len(paragraphs))

	// The dialogue has three sections. We identify transition points:
	// 1. Cephalus section: from start until Polemarchus takes over the argument
	//    Key marker: "Here Cephalus retires" or "the possession of the argument to his heir"
	// 2. Polemarchus section: from inheriting the argument until Thrasymachus erupts
	//    Key marker: "Thrasymachus had many times made an attempt to get the argument"
	// 3. Thrasymachus section: from his eruption to end of Book 1

	cephalusEnd := findParagraph(paragraphs, "hand over the argument to Polemarchus")
	if cephalusEnd < 0 {
		cephalusEnd = findParagraph(paragraphs, "possession of the argument")
	}
	if cephalusEnd < 0 {
		cephalusEnd = findParagraph(paragraphs, "Cephalus retires")
	}
	thrasymachusStart := findParagraph(paragraphs, "Thrasymachus had many times")
	if thrasymachusStart < 0 {
		thrasymachusStart = findParagraph(paragraphs, "could no longer hold his peace")
	}
	if thrasymachusStart < 0 {
		thrasymachusStart = findParagraph(paragraphs, "Listen, then, he said")
	}

	t.Logf("Cephalus end paragraph: %d", cephalusEnd)
	t.Logf("Thrasymachus start paragraph: %d", thrasymachusStart)

	// Parse each section into turns using speaker attribution.
	cephalusTurns := parseTurns(paragraphs[:cephalusEnd+1], "cephalus")
	polTurns := parseTurns(paragraphs[cephalusEnd+1:thrasymachusStart], "polemarchus")
	thrTurns := parseTurns(paragraphs[thrasymachusStart:], "thrasymachus")

	t.Logf("Cephalus turns: %d", len(cephalusTurns))
	t.Logf("Polemarchus turns: %d", len(polTurns))
	t.Logf("Thrasymachus turns: %d", len(thrTurns))

	// Segment into corpus entries.
	cephalusEntries := segmentDialogue(cephalusTurns, "republic-b1-cep", "What is the meaning of justice?", nil, "Cephalus dialogue: justice as honesty and paying debts")
	polEntries := segmentDialogue(polTurns, "republic-b1-pol", "Is justice helping friends and harming enemies?",
		[]string{"CONTRADICTION", "SUNK_COST_FALLACY"}, "Polemarchus dialogue: inherited definition from Simonides")
	thrEntries := segmentDialogue(thrTurns, "republic-b1-thr", "What is justice?",
		[]string{"CONTRADICTION", "SCOPE_DRIFT", "ANCHORING_BIAS", "CONFIDENCE_MISCALIBRATION"},
		"Thrasymachus dialogue: justice as the advantage of the stronger")

	t.Logf("Corpus entries: cephalus=%d, polemarchus=%d, thrasymachus=%d, total=%d",
		len(cephalusEntries), len(polEntries), len(thrEntries),
		len(cephalusEntries)+len(polEntries)+len(thrEntries))

	// Write NDJSON files.
	dirs := []string{
		"../../data/library/dialogues",
		"../../data/library/annotations",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	writeOrFail(t, "../../data/library/dialogues/republic_b1_cephalus.ndjson", cephalusEntries)
	writeOrFail(t, "../../data/library/dialogues/republic_b1_polemarchus.ndjson", polEntries)
	writeOrFail(t, "../../data/library/dialogues/republic_b1_thrasymachus.ndjson", thrEntries)

	// Write annotations.
	writeAnnotations(t, "../../data/library/annotations/republic_b1_cephalus.json",
		cephalusEntries, nil)
	writeAnnotations(t, "../../data/library/annotations/republic_b1_polemarchus.json",
		polEntries, []string{"CONTRADICTION", "SUNK_COST_FALLACY"})
	writeAnnotations(t, "../../data/library/annotations/republic_b1_thrasymachus.json",
		thrEntries, []string{"CONTRADICTION", "SCOPE_DRIFT", "ANCHORING_BIAS", "CONFIDENCE_MISCALIBRATION"})
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

func findLine(lines []string, substr string) int {
	for i, l := range lines {
		if strings.Contains(l, substr) {
			return i
		}
	}
	return -1
}

func findLineFrom(lines []string, start int, substr string) int {
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), substr) {
			return i
		}
	}
	return -1
}

func splitParagraphs(lines []string) []string {
	var paragraphs []string
	var current strings.Builder
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			if current.Len() > 0 {
				paragraphs = append(paragraphs, strings.TrimSpace(current.String()))
				current.Reset()
			}
		} else {
			if current.Len() > 0 {
				current.WriteString(" ")
			}
			current.WriteString(trimmed)
		}
	}
	if current.Len() > 0 {
		paragraphs = append(paragraphs, strings.TrimSpace(current.String()))
	}
	return paragraphs
}

func findParagraph(paragraphs []string, substr string) int {
	for i, p := range paragraphs {
		if strings.Contains(p, substr) {
			return i
		}
	}
	return -1
}

// Speaker attribution patterns.
var (
	// "I said", "I replied", "I asked" = Socrates (first person narrator)
	reISaid = regexp.MustCompile(`(?i)\bI\s+(said|replied|asked|answered|continued|added|remarked|observed|responded)\b`)
	// "said Thrasymachus", "Thrasymachus said", "replied Polemarchus"
	reNamedSpeaker = regexp.MustCompile(`(?i)\b(said|replied|asked|answered|continued|added|remarked|observed|responded|exclaimed|interposed|rejoined)\s+(Socrates|Thrasymachus|Polemarchus|Cephalus|Glaucon|Adeimantus|Cleitophon)\b`)
	reNamedBefore  = regexp.MustCompile(`(?i)\b(Socrates|Thrasymachus|Polemarchus|Cephalus|Glaucon|Adeimantus|Cleitophon)\s+(said|replied|asked|answered|continued|added|remarked|observed|responded|exclaimed|interposed|rejoined)\b`)
	// "he said", "he replied" = current interlocutor
	reHeSaid = regexp.MustCompile(`(?i)\bhe\s+(said|replied|asked|answered|continued|added|remarked|observed|responded|exclaimed|rejoined)\b`)
)

type parsedTurn struct {
	speaker string
	text    string
}

// parseTurns assigns speakers to paragraphs based on attribution cues.
// defaultInterlocutor is the primary other speaker in this section.
func parseTurns(paragraphs []string, defaultInterlocutor string) []TurnJSON {
	var turns []TurnJSON
	lastSpeaker := "socrates" // Socrates narrates, so he starts
	turnNum := 1

	for _, p := range paragraphs {
		if len(p) < 5 {
			continue
		}

		speaker := identifySpeaker(p, defaultInterlocutor, lastSpeaker)

		// Strip attribution phrases from the text.
		text := cleanAttribution(p)
		if len(text) < 10 {
			continue
		}

		// Merge consecutive same-speaker turns.
		if len(turns) > 0 && turns[len(turns)-1].Speaker == speaker {
			turns[len(turns)-1].RawText += " " + text
			continue
		}

		turns = append(turns, TurnJSON{
			TurnNumber: turnNum,
			Speaker:    speaker,
			RawText:    text,
		})
		lastSpeaker = speaker
		turnNum++
	}

	// Renumber after merges.
	for i := range turns {
		turns[i].TurnNumber = i + 1
	}
	return turns
}

func identifySpeaker(p, defaultInterlocutor, lastSpeaker string) string {
	// Check for named speaker attribution.
	if m := reNamedSpeaker.FindStringSubmatch(p); len(m) > 2 {
		return strings.ToLower(m[2])
	}
	if m := reNamedBefore.FindStringSubmatch(p); len(m) > 1 {
		return strings.ToLower(m[1])
	}

	// Check for first-person attribution (Socrates narrating).
	if reISaid.MatchString(p) {
		return "socrates"
	}

	// Check for "he said" = default interlocutor.
	if reHeSaid.MatchString(p) {
		return defaultInterlocutor
	}

	// Short responses ("Yes.", "Certainly.", "True.") = alternate from last.
	trimmed := strings.TrimSpace(p)
	if len(trimmed) < 40 && !strings.Contains(trimmed, ",") {
		if lastSpeaker == "socrates" {
			return defaultInterlocutor
		}
		return "socrates"
	}

	// Default: alternate from last speaker.
	if lastSpeaker == "socrates" {
		return defaultInterlocutor
	}
	return "socrates"
}

var reAttrPhrase = regexp.MustCompile(`(?i)(,?\s*\b(I|he)\s+(said|replied|asked|answered|continued|added|remarked|observed|responded|exclaimed|rejoined)\s*[,.]?\s*)|(,?\s*\bsaid\s+(Socrates|Thrasymachus|Polemarchus|Cephalus|Glaucon|Adeimantus|Cleitophon)\s*[,.]?\s*)|(,?\s*\b(Socrates|Thrasymachus|Polemarchus|Cephalus|Glaucon|Adeimantus|Cleitophon)\s+(said|replied|asked|answered|continued|added|remarked|observed|responded|exclaimed|interposed|rejoined)\s*[,.]?\s*)`)

func cleanAttribution(p string) string {
	cleaned := reAttrPhrase.ReplaceAllString(p, " ")
	// Collapse whitespace.
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return strings.TrimSpace(cleaned)
}

// segmentDialogue splits a turn list into segments of ~8-12 turns each.
func segmentDialogue(turns []TurnJSON, idPrefix, objective string, expectedTypes []string, notes string) []CorpusEntry {
	if len(turns) == 0 {
		return nil
	}

	segSize := 10 // target turns per segment
	if len(turns) <= 15 {
		// Small dialogue: one segment.
		seg := DialogueSegment{
			ID:        idPrefix + "-001",
			Objective: objective,
			Turns:     renumber(turns),
			Expected:  expectedTypes,
			Notes:     notes,
		}
		return []CorpusEntry{seg.ToCorpusEntry()}
	}

	var entries []CorpusEntry
	segNum := 1
	for i := 0; i < len(turns); i += segSize {
		end := i + segSize
		if end > len(turns) {
			end = len(turns)
		}
		// Don't create tiny trailing segments.
		if len(turns)-end < 4 {
			end = len(turns)
		}

		seg := DialogueSegment{
			ID:        fmt.Sprintf("%s-%03d", idPrefix, segNum),
			Objective: objective,
			Turns:     renumber(turns[i:end]),
			Notes:     notes,
		}

		// Assign expected types based on position in dialogue.
		// Early segments: anchoring. Middle: contradiction. Late: scope drift.
		if expectedTypes != nil {
			frac := float64(i) / float64(len(turns))
			var assigned []string
			for _, et := range expectedTypes {
				switch et {
				case "ANCHORING_BIAS":
					if frac < 0.4 { // early in dialogue
						assigned = append(assigned, et)
					}
				case "CONTRADICTION":
					if frac >= 0.1 && frac <= 0.7 { // middle
						assigned = append(assigned, et)
					}
				case "SCOPE_DRIFT":
					if frac >= 0.5 { // late in dialogue
						assigned = append(assigned, et)
					}
				case "CONFIDENCE_MISCALIBRATION":
					if frac < 0.5 { // Thrasymachus is confident early
						assigned = append(assigned, et)
					}
				case "SUNK_COST_FALLACY":
					assigned = append(assigned, et) // throughout
				default:
					assigned = append(assigned, et)
				}
			}
			if len(assigned) == 0 {
				// No expected findings for this segment position.
				assigned = nil
			}
			seg.Expected = assigned
		}

		entries = append(entries, seg.ToCorpusEntry())
		segNum++
		if end == len(turns) {
			break
		}
	}
	return entries
}

func renumber(turns []TurnJSON) []TurnJSON {
	out := make([]TurnJSON, len(turns))
	for i, t := range turns {
		out[i] = t
		out[i].TurnNumber = i + 1
	}
	return out
}

func writeOrFail(t *testing.T, path string, entries []CorpusEntry) {
	t.Helper()
	if err := WriteNDJSON(path, entries); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("Wrote %d entries to %s", len(entries), path)
}

func writeAnnotations(t *testing.T, path string, entries []CorpusEntry, globalExpected []string) {
	t.Helper()
	annotations := make(map[string][]string)
	for _, e := range entries {
		var types []string
		for _, exp := range e.Expected {
			types = append(types, exp.FindingType)
		}
		if types == nil && globalExpected != nil {
			types = globalExpected
		}
		annotations[e.EntryID] = types
	}
	if err := WriteAnnotations(path, annotations); err != nil {
		t.Fatalf("write annotations %s: %v", path, err)
	}
	t.Logf("Wrote annotations to %s", path)
}
