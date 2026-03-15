// Package pipeline implements CereBRO's 5-layer biomimetic cognitive pipeline.
// It provides intake enrichment, 6 cognitive detectors, inhibition gating,
// neuromodulation, metacognitive self-confidence, and memory consolidation.
package pipeline

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"google.golang.org/protobuf/proto"
)

var numberRe = regexp.MustCompile(`[\$€£]?\d+(?:[.,]\d+)*%?`)

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

// Enrich takes a ConversationSnapshot and populates TurnMetadata for each turn.
// Returns a deep copy — the caller's original snapshot is never mutated.
// This ensures state isolation for Phase 5's Memory Consolidator, which may
// read the original snapshot asynchronously while the pipeline processes the
// next request.
func Enrich(snap *reasoningv1.ConversationSnapshot) *reasoningv1.ConversationSnapshot {
	if snap == nil {
		return snap
	}

	snap = proto.Clone(snap).(*reasoningv1.ConversationSnapshot)

	if snap.Objective != "" && len(snap.ObjectiveKeywords) == 0 {
		snap.ObjectiveKeywords = extractKeywords(snap.Objective)
	}

	snap.TotalTurns = uint32(len(snap.Turns))

	for _, turn := range snap.Turns {
		if turn.Metadata == nil {
			turn.Metadata = &reasoningv1.TurnMetadata{}
		}
		text := turn.GetRawText()

		turn.Metadata.TopicKeywords = extractKeywords(text)
		turn.Metadata.NumericTokens = extractNumericTokens(text)
		turn.Metadata.EntityMentions = extractEntities(text)
		turn.Metadata.WordCount = uint32(len(strings.Fields(text)))
	}

	return snap
}

func extractKeywords(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	seen := make(map[string]bool)
	var keywords []string
	for _, w := range words {
		w = strings.TrimFunc(w, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if w == "" || len(w) < 3 || stopwords[w] {
			continue
		}
		// Normalize to stem so inflected forms compare equal in Jaccard similarity.
		// "justice" → "justic", "arguing" → "argu", etc.
		s := stemWord(w)
		if !seen[s] {
			seen[s] = true
			keywords = append(keywords, s)
		}
	}
	return keywords
}

func extractNumericTokens(text string) []*reasoningv1.NumericToken {
	matches := numberRe.FindAllStringIndex(text, -1)
	var tokens []*reasoningv1.NumericToken

	for _, loc := range matches {
		raw := text[loc[0]:loc[1]]
		cleaned := strings.NewReplacer("$", "", "€", "", "£", "", ",", "", "%", "").Replace(raw)
		val, err := strconv.ParseFloat(cleaned, 64)
		if err != nil {
			continue
		}

		ctxStart := loc[0] - 50
		if ctxStart < 0 {
			ctxStart = 0
		}
		for ctxStart > 0 && !utf8.RuneStart(text[ctxStart]) {
			ctxStart--
		}
		ctxEnd := loc[1] + 50
		if ctxEnd > len(text) {
			ctxEnd = len(text)
		}
		for ctxEnd < len(text) && !utf8.RuneStart(text[ctxEnd]) {
			ctxEnd++
		}

		tokens = append(tokens, &reasoningv1.NumericToken{
			Value:         val,
			Position:      uint32(loc[0]),
			ContextWindow: text[ctxStart:ctxEnd],
		})
	}
	return tokens
}

func extractEntities(text string) []string {
	words := strings.Fields(text)
	var entities []string
	var current []string

	for _, w := range words {
		r, _ := utf8.DecodeRuneInString(w)
		if r != utf8.RuneError && unicode.IsUpper(r) {
			current = append(current, w)
		} else {
			if len(current) >= 2 {
				entity := strings.Join(current, " ")
				entity = strings.TrimRight(entity, ".,;:!?")
				entities = append(entities, entity)
			}
			current = nil
		}
	}
	if len(current) >= 2 {
		entities = append(entities, strings.Join(current, " "))
	}
	return entities
}
