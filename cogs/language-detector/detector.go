package main

import (
	"encoding/json"
	"math"
	"os"
	"strings"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// Config holds the language detector's parameters.
type Config struct {
	SupportedLanguages []string
	MinConfidence      float64
	FallbackLanguage   string
	MinSampleChars     uint32
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		SupportedLanguages: []string{"en"},
		MinConfidence:      0.55,
		FallbackLanguage:   "en",
		MinSampleChars:     20,
	}
}

// LanguageProfile maps trigrams to their normalized frequencies.
type LanguageProfile map[string]float64

// LoadProfile reads a JSON language profile from disk.
func LoadProfile(path string) (LanguageProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var profile LanguageProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, err
	}
	return profile, nil
}

// LoadProfiles loads all language profiles from a directory.
// Returns a map of language_code -> profile.
func LoadProfiles(dir string) (map[string]LanguageProfile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	profiles := make(map[string]LanguageProfile)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		lang := strings.TrimSuffix(entry.Name(), ".json")
		profile, err := LoadProfile(dir + "/" + entry.Name())
		if err != nil {
			return nil, err
		}
		profiles[lang] = profile
	}
	return profiles, nil
}

// Detect identifies the language of the conversation text.
// Uses trigram frequency analysis (Cavnar & Trenkle 1994) with a composite
// scoring method combining cosine similarity and profile overlap.
func Detect(snap *reasoningv1.ConversationSnapshot, profiles map[string]LanguageProfile, cfg Config) *cerebrov1.LanguageResult {
	if snap == nil || len(profiles) == 0 {
		return &cerebrov1.LanguageResult{
			LanguageCode: cfg.FallbackLanguage,
			Confidence:   0.5,
			Supported:    isSupported(cfg.FallbackLanguage, cfg.SupportedLanguages),
		}
	}

	// Extract sample text (first 2000 chars from all turns combined).
	sample := extractSample(snap, 2000)

	// Too short for reliable detection — use fallback.
	if uint32(len(sample)) < cfg.MinSampleChars {
		return &cerebrov1.LanguageResult{
			LanguageCode: cfg.FallbackLanguage,
			Confidence:   0.5,
			Supported:    isSupported(cfg.FallbackLanguage, cfg.SupportedLanguages),
		}
	}

	// Count trigrams in sample.
	sampleProfile := countTrigrams(sample)

	// Score against each language profile.
	bestLang := ""
	bestScore := 0.0
	for lang, profile := range profiles {
		score := profileScore(sampleProfile, profile)
		if score > bestScore {
			bestScore = score
			bestLang = lang
		}
	}

	// Below confidence threshold — treat as unreliable detection.
	if bestScore < cfg.MinConfidence {
		return &cerebrov1.LanguageResult{
			LanguageCode: bestLang,
			Confidence:   bestScore,
			Supported:    false,
		}
	}

	return &cerebrov1.LanguageResult{
		LanguageCode: bestLang,
		Confidence:   bestScore,
		Supported:    isSupported(bestLang, cfg.SupportedLanguages),
	}
}

// profileScore computes a composite similarity between a sample and a reference profile.
// It combines cosine similarity with profile overlap (fraction of the sample's
// probability mass that falls on trigrams present in the reference). The composite
// gives robust discrimination while producing scores in the useful [0.7, 1.0]
// range for correct language matches.
func profileScore(sample, ref LanguageProfile) float64 {
	cos := cosineSimilarity(sample, ref)

	// Profile overlap: fraction of sample's probability mass on known trigrams.
	overlap := 0.0
	for k, va := range sample {
		if _, ok := ref[k]; ok {
			overlap += va
		}
	}

	// Composite: geometric mean biased toward overlap which has better discrimination.
	// Cosine is typically 0.1-0.5, overlap 0.2-0.8 for 300-trigram profiles.
	// We square-root the cosine to lift its range, then combine.
	liftedCos := math.Sqrt(cos)
	score := 0.4*liftedCos + 0.6*overlap
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// extractSample concatenates turn text up to maxChars.
func extractSample(snap *reasoningv1.ConversationSnapshot, maxChars int) string {
	var b strings.Builder
	for _, turn := range snap.GetTurns() {
		text := turn.GetRawText()
		remaining := maxChars - b.Len()
		if remaining <= 0 {
			break
		}
		if len(text) > remaining {
			text = text[:remaining]
		}
		b.WriteString(text)
		b.WriteByte(' ')
	}
	return strings.ToLower(strings.TrimSpace(b.String()))
}

// countTrigrams counts character trigram frequencies in text.
func countTrigrams(text string) LanguageProfile {
	profile := make(LanguageProfile)
	runes := []rune(text)
	if len(runes) < 3 {
		return profile
	}
	total := 0
	for i := 0; i <= len(runes)-3; i++ {
		trigram := string(runes[i : i+3])
		profile[trigram]++
		total++
	}
	// Normalize.
	if total > 0 {
		for k, v := range profile {
			profile[k] = v / float64(total)
		}
	}
	return profile
}

// cosineSimilarity computes the cosine similarity between two frequency profiles.
func cosineSimilarity(a, b LanguageProfile) float64 {
	var dot, magA, magB float64
	for k, va := range a {
		magA += va * va
		if vb, ok := b[k]; ok {
			dot += va * vb
		}
	}
	for _, vb := range b {
		magB += vb * vb
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}

func isSupported(lang string, supported []string) bool {
	for _, s := range supported {
		if s == lang {
			return true
		}
	}
	return false
}
