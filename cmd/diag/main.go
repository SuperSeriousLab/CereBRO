package main

import (
	"encoding/json"
	"fmt"
	"os"
	"bufio"
	"strings"
	"unicode"

	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "shall": true, "can": true,
	"to": true, "of": true, "in": true, "for": true, "on": true,
	"with": true, "at": true, "by": true, "from": true, "as": true,
	"into": true, "about": true, "like": true, "through": true,
	"and": true, "but": true, "or": true, "nor": true, "not": true,
	"so": true, "yet": true, "both": true, "either": true, "neither": true,
	"very": true, "just": true, "also": true, "too": true, "much": true,
	"it": true, "its": true, "i": true, "me": true, "my": true,
	"you": true, "your": true, "he": true, "she": true, "we": true,
	"they": true, "them": true, "their": true, "this": true, "that": true,
	"these": true, "those": true, "what": true, "which": true, "who": true,
}

func stemWord(w string) string {
	if len(w) <= 3 { return w }
	type rule struct { suffix, replacement string }
	rules := []rule{
		{"fulness", "ful"}, {"ousness", "ous"}, {"iveness", "ive"},
		{"nesses", ""}, {"lessly", ""}, {"ically", "ic"},
		{"ments", ""}, {"ation", ""}, {"ition", ""}, {"alism", ""},
		{"alist", ""}, {"ality", ""}, {"ative", ""}, {"izing", ""}, {"ising", ""}, {"ating", ""},
		{"ness", ""}, {"less", ""}, {"ment", ""}, {"able", ""}, {"ible", ""},
		{"ious", ""}, {"tion", ""},
		{"ism", ""}, {"ist", ""}, {"ize", ""}, {"ise", ""}, {"ily", ""},
		{"ity", ""}, {"ion", ""}, {"ing", ""}, {"ous", ""}, {"ive", ""},
		{"ful", ""}, {"ies", "y"}, {"ice", ""},
		{"ly", ""}, {"er", ""}, {"ed", ""}, {"es", ""}, {"al", ""},
		{"y", ""}, {"s", ""},
	}
	for _, r := range rules {
		if strings.HasSuffix(w, r.suffix) {
			candidate := w[:len(w)-len(r.suffix)] + r.replacement
			if len(candidate) >= 3 { return candidate }
		}
	}
	return w
}

func extractKeywords(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	seen := make(map[string]bool)
	var keywords []string
	for _, w := range words {
		w = strings.TrimFunc(w, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if w == "" || len(w) < 3 || stopwords[w] { continue }
		s := stemWord(w)
		if !seen[s] { seen[s] = true; keywords = append(keywords, s) }
	}
	return keywords
}

func stemmedJaccard(anchorKWs map[string]bool, text string) float64 {
	turnKWs := extractKeywords(text)
	if len(turnKWs) == 0 || len(anchorKWs) == 0 { return 0.0 }
	intersection := 0
	for _, kw := range turnKWs {
		if anchorKWs[kw] { intersection++ }
	}
	union := len(anchorKWs) + len(turnKWs) - intersection
	if union == 0 { return 0.0 }
	return float64(intersection) / float64(union)
}

var hedgeWords = []string{
	"maybe", "perhaps", "possibly", "might", "could be", "i think",
	"i believe", "i suppose", "i guess", "i wonder", "not sure",
	"uncertain", "unclear", "probably", "likely",
}

func isStrongDeclarative(text string) bool {
	lower := strings.ToLower(text)
	trimmed := strings.TrimSpace(text)
	if strings.HasSuffix(trimmed, "?") { return false }
	for _, opener := range []string{"what ", "who ", "how ", "when ", "where ", "why ", "is it ", "are you ", "do you "} {
		if strings.HasPrefix(lower, opener) { return false }
	}
	if len(strings.Fields(text)) < 6 { return false }
	for _, hedge := range hedgeWords {
		if strings.Contains(lower, hedge) { return false }
	}
	for _, marker := range []string{" is ", " are ", " must ", " always ", " never ", " only ", " will ", " shall ", " does ", " do "} {
		if strings.Contains(lower, marker) { return true }
	}
	if strings.HasPrefix(lower, "the ") || strings.HasPrefix(lower, "justice") || strings.HasPrefix(lower, "truth") || strings.HasPrefix(lower, "virtue") {
		return true
	}
	return false
}

type CorpusEntry struct {
	EntryID  string          `json:"entry_id"`
	Input    json.RawMessage `json:"input"`
}

func main() {
	f, _ := os.Open("data/corpus/tier2-v1.ndjson")
	defer f.Close()
	cfg := pipeline.DefaultPipelineConfig()
	_ = cfg
	pj := protojson.UnmarshalOptions{DiscardUnknown: true}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	
	// Only check ca entries
	for scanner.Scan() {
		var e CorpusEntry
		json.Unmarshal(scanner.Bytes(), &e)
		if !strings.HasPrefix(e.EntryID, "tier2-ca") { continue }
		var snap reasoningv1.ConversationSnapshot
		pj.Unmarshal(e.Input, &snap)
		turns := snap.GetTurns()
		fmt.Printf("=== %s (%d turns) ===\n", e.EntryID, len(turns))
		maxScan := 3
		if maxScan > len(turns) { maxScan = len(turns) }
		for i := 0; i < maxScan; i++ {
			t := turns[i]
			strong := isStrongDeclarative(t.GetRawText())
			kws := extractKeywords(t.GetRawText())
			anchorKWs := make(map[string]bool)
			for _, k := range kws { anchorKWs[k] = true }
			if strong {
				orbitCount := 0
				total := 0
				for _, st := range turns {
					if st.GetTurnNumber() <= t.GetTurnNumber() { continue }
					overlap := stemmedJaccard(anchorKWs, st.GetRawText())
					if overlap >= 0.3 { orbitCount++ }
					total++
				}
				orbitRatio := 0.0
				if total > 0 { orbitRatio = float64(orbitCount)/float64(total) }
				fires := orbitRatio >= 0.6
				fmt.Printf("  Anchor t%d (orbit=%.3f %d/%d, FIRES=%v) kws=%v\n",
					t.GetTurnNumber(), orbitRatio, orbitCount, total, fires, kws)
			}
		}
		fmt.Println()
	}
}
