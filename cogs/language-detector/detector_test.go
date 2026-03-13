package main

import (
	"os"
	"path/filepath"
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func loadTestProfiles(t *testing.T) map[string]LanguageProfile {
	t.Helper()
	// Navigate from cogs/language-detector/ to data/language-profiles/
	dir := filepath.Join("..", "..", "data", "language-profiles")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("language profiles not found at %s", dir)
	}
	profiles, err := LoadProfiles(dir)
	if err != nil {
		t.Fatalf("load profiles: %v", err)
	}
	if len(profiles) == 0 {
		t.Fatal("no profiles loaded")
	}
	return profiles
}

func TestDetect_EnglishParagraph(t *testing.T) {
	profiles := loadTestProfiles(t)
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "The quick brown fox jumps over the lazy dog. This is a common English sentence used for testing purposes. It contains all the letters of the alphabet and is widely recognized in typographic circles."},
		},
	}
	result := Detect(snap, profiles, DefaultConfig())
	if result.GetLanguageCode() != "en" {
		t.Errorf("expected en, got %s", result.GetLanguageCode())
	}
	if result.GetConfidence() < 0.7 {
		t.Errorf("expected confidence > 0.7, got %.2f", result.GetConfidence())
	}
	if !result.GetSupported() {
		t.Error("expected supported=true for en")
	}
}

func TestDetect_GermanParagraph(t *testing.T) {
	profiles := loadTestProfiles(t)
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Die Bundesrepublik Deutschland ist ein demokratischer und sozialer Bundesstaat. Alle Staatsgewalt geht vom Volke aus. Sie wird vom Volke in Wahlen und Abstimmungen ausgeübt."},
		},
	}
	cfg := DefaultConfig()
	cfg.SupportedLanguages = []string{"en"} // German not in supported set
	result := Detect(snap, profiles, cfg)
	if result.GetLanguageCode() != "de" {
		t.Errorf("expected de, got %s", result.GetLanguageCode())
	}
	if result.GetConfidence() < 0.7 {
		t.Errorf("expected confidence > 0.7, got %.2f", result.GetConfidence())
	}
	if result.GetSupported() {
		t.Error("expected supported=false for de (not in supported set)")
	}
}

func TestDetect_FrenchText(t *testing.T) {
	profiles := loadTestProfiles(t)
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "La France est un pays situé en Europe occidentale. Elle est connue pour sa culture, sa cuisine et son histoire riche. Paris est la capitale de la France."},
		},
	}
	cfg := DefaultConfig()
	cfg.SupportedLanguages = []string{"en"}
	result := Detect(snap, profiles, cfg)
	if result.GetLanguageCode() != "fr" {
		t.Errorf("expected fr, got %s", result.GetLanguageCode())
	}
	if result.GetSupported() {
		t.Error("expected supported=false for fr")
	}
}

func TestDetect_ShortInput(t *testing.T) {
	profiles := loadTestProfiles(t)
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Hi"},
		},
	}
	result := Detect(snap, profiles, DefaultConfig())
	if result.GetLanguageCode() != "en" {
		t.Errorf("expected fallback to en, got %s", result.GetLanguageCode())
	}
	if result.GetConfidence() != 0.5 {
		t.Errorf("expected confidence 0.5 for short input, got %.2f", result.GetConfidence())
	}
}

func TestDetect_NilSnapshot(t *testing.T) {
	profiles := loadTestProfiles(t)
	result := Detect(nil, profiles, DefaultConfig())
	if result.GetLanguageCode() != "en" {
		t.Errorf("expected fallback to en, got %s", result.GetLanguageCode())
	}
}

func TestCountTrigrams(t *testing.T) {
	profile := countTrigrams("the cat")
	if _, ok := profile["the"]; !ok {
		t.Error("expected 'the' trigram")
	}
	if _, ok := profile["he "]; !ok {
		t.Error("expected 'he ' trigram")
	}
	// Total should sum to approximately 1.0.
	var total float64
	for _, v := range profile {
		total += v
	}
	if total < 0.99 || total > 1.01 {
		t.Errorf("expected normalized frequencies summing to ~1.0, got %.3f", total)
	}
}

func TestCosineSimilarity_Identical(t *testing.T) {
	a := LanguageProfile{"the": 0.5, "and": 0.3, "for": 0.2}
	score := cosineSimilarity(a, a)
	if score < 0.99 {
		t.Errorf("expected ~1.0 for identical profiles, got %.3f", score)
	}
}

func TestCosineSimilarity_Disjoint(t *testing.T) {
	a := LanguageProfile{"the": 0.5, "and": 0.5}
	b := LanguageProfile{"der": 0.5, "und": 0.5}
	score := cosineSimilarity(a, b)
	if score != 0 {
		t.Errorf("expected 0 for disjoint profiles, got %.3f", score)
	}
}
