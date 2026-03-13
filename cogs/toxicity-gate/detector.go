package main

import (
	"bufio"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// Config holds the toxicity gate's parameters.
type Config struct {
	BlocklistPath string
	CaseSensitive bool
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		BlocklistPath: "data/blocklists/default.txt",
		CaseSensitive: false,
	}
}

// LoadBlocklist reads a blocklist file (one term per line, # comments).
func LoadBlocklist(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var terms []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		terms = append(terms, line)
	}
	return terms, scanner.Err()
}

// Screen checks all turn text against the blocklist.
func Screen(snap *reasoningv1.ConversationSnapshot, blocklist []string, cfg Config) *cerebrov1.ToxicityResult {
	if snap == nil {
		return &cerebrov1.ToxicityResult{Toxic: false}
	}

	if len(blocklist) == 0 {
		return &cerebrov1.ToxicityResult{Toxic: false}
	}

	// Build normalized blocklist.
	normalizedBlocklist := make([]string, len(blocklist))
	for i, term := range blocklist {
		if cfg.CaseSensitive {
			normalizedBlocklist[i] = term
		} else {
			normalizedBlocklist[i] = strings.ToLower(term)
		}
	}

	var matchedPatterns []string

	for _, turn := range snap.GetTurns() {
		text := turn.GetRawText()
		if !cfg.CaseSensitive {
			text = strings.ToLower(text)
		}

		for _, pattern := range normalizedBlocklist {
			if matchesWithWordBoundary(text, pattern) {
				// Avoid duplicate pattern entries.
				if !containsString(matchedPatterns, pattern) {
					matchedPatterns = append(matchedPatterns, pattern)
				}
			}
		}
	}

	if len(matchedPatterns) == 0 {
		return &cerebrov1.ToxicityResult{Toxic: false}
	}

	score := float64(len(matchedPatterns)) / float64(len(normalizedBlocklist))
	if score > 1.0 {
		score = 1.0
	}

	return &cerebrov1.ToxicityResult{
		Toxic:           true,
		MatchedPatterns: matchedPatterns,
		ToxicityScore:   score,
	}
}

// matchesWithWordBoundary checks if pattern appears in text at a word boundary.
// A word boundary means the character before and after the match must be
// non-alphanumeric (space, punctuation, start/end of string).
func matchesWithWordBoundary(text, pattern string) bool {
	start := 0
	for {
		idx := strings.Index(text[start:], pattern)
		if idx < 0 {
			return false
		}
		matchStart := start + idx
		matchEnd := matchStart + len(pattern)

		// Check left boundary (decode full UTF-8 rune, not byte cast).
		leftOK := matchStart == 0 || !isWordRuneBefore(text, matchStart)
		// Check right boundary.
		rightOK := matchEnd >= len(text) || !isWordRuneAt(text, matchEnd)

		if leftOK && rightOK {
			return true
		}

		start = matchStart + 1
		if start >= len(text) {
			return false
		}
	}
}

// isWordRuneBefore decodes the UTF-8 rune ending at byte position pos and
// returns whether it is a letter or digit.
func isWordRuneBefore(text string, pos int) bool {
	r, _ := utf8.DecodeLastRuneInString(text[:pos])
	return r != utf8.RuneError && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

// isWordRuneAt decodes the UTF-8 rune starting at byte position pos and
// returns whether it is a letter or digit.
func isWordRuneAt(text string, pos int) bool {
	r, _ := utf8.DecodeRuneInString(text[pos:])
	return r != utf8.RuneError && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
