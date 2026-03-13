package pipeline

import (
	"path/filepath"
	"strings"
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func TestLayer0_ValidInput(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Hello, how are you?"},
		},
		TotalTurns: 1,
	}
	cfg := DefaultLayer0Config()
	result := RunLayer0(snap, cfg)
	if !result.Accepted {
		t.Errorf("expected accepted, got rejection: %s", result.Reason)
	}
}

func TestLayer0_NilSnapshot(t *testing.T) {
	cfg := DefaultLayer0Config()
	result := RunLayer0(nil, cfg)
	if result.Accepted {
		t.Error("expected rejection for nil snapshot")
	}
	if !strings.Contains(result.Reason, "format:") {
		t.Errorf("expected format rejection, got: %s", result.Reason)
	}
}

func TestLayer0_EmptyTurns(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{}
	cfg := DefaultLayer0Config()
	result := RunLayer0(snap, cfg)
	if result.Accepted {
		t.Error("expected rejection for empty turns")
	}
}

func TestLayer0_TooManyTurns(t *testing.T) {
	turns := make([]*reasoningv1.Turn, 600)
	for i := range turns {
		turns[i] = &reasoningv1.Turn{TurnNumber: uint32(i + 1), Speaker: "user", RawText: "hi"}
	}
	snap := &reasoningv1.ConversationSnapshot{Turns: turns, TotalTurns: 600}
	cfg := DefaultLayer0Config()
	result := RunLayer0(snap, cfg)
	if result.Accepted {
		t.Error("expected rejection for too many turns")
	}
	if !strings.Contains(result.Reason, "format:") {
		t.Errorf("expected format rejection, got: %s", result.Reason)
	}
}

func TestLayer0_ToxicContent(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "This is a death threat right here"},
		},
		TotalTurns: 1,
	}
	cfg := DefaultLayer0Config()
	cfg.Toxicity.Blocklist = []string{"death threat"}
	result := RunLayer0(snap, cfg)
	if result.Accepted {
		t.Error("expected rejection for toxic content")
	}
	if !strings.Contains(result.Reason, "toxicity:") {
		t.Errorf("expected toxicity rejection, got: %s", result.Reason)
	}
}

func TestLayer0_UnsupportedLanguage(t *testing.T) {
	profileDir := filepath.Join("..", "..", "data", "language-profiles")
	profiles, err := LoadLangProfiles(profileDir)
	if err != nil {
		t.Skipf("language profiles not found: %v", err)
	}

	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Die Bundesrepublik Deutschland ist ein demokratischer und sozialer Bundesstaat. Alle Staatsgewalt geht vom Volke aus. Sie wird vom Volke in Wahlen und Abstimmungen ausgeübt."},
		},
		TotalTurns: 1,
	}
	cfg := DefaultLayer0Config()
	cfg.Language.Profiles = profiles
	cfg.Language.SupportedLanguages = []string{"en"} // German not supported
	result := RunLayer0(snap, cfg)
	if result.Accepted {
		t.Error("expected rejection for unsupported language")
	}
	if !strings.Contains(result.Reason, "language:") {
		t.Errorf("expected language rejection, got: %s", result.Reason)
	}
}

func TestLayer0_WordBoundaryProtection(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "The assassination attempt was foiled"},
		},
		TotalTurns: 1,
	}
	cfg := DefaultLayer0Config()
	cfg.Toxicity.Blocklist = []string{"ass"}
	result := RunLayer0(snap, cfg)
	if !result.Accepted {
		t.Errorf("expected accepted (word boundary should prevent matching), got: %s", result.Reason)
	}
}

func TestLayer0_WordBoundaryUTF8(t *testing.T) {
	// "caféslur" — "é" is multi-byte UTF-8; "slur" is NOT at a word boundary.
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "The caféslur opened late"},
		},
		TotalTurns: 1,
	}
	cfg := DefaultLayer0Config()
	cfg.Toxicity.Blocklist = []string{"slur"}
	result := RunLayer0(snap, cfg)
	if !result.Accepted {
		t.Errorf("expected accepted (UTF-8 word boundary should prevent match), got: %s", result.Reason)
	}

	// "café slur" — space boundary, should match.
	snap2 := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "The café slur was awful"},
		},
		TotalTurns: 1,
	}
	result2 := RunLayer0(snap2, cfg)
	if result2.Accepted {
		t.Error("expected rejection for 'slur' at word boundary after space")
	}
}

func TestValidateFormat_InvalidUTF8(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Hello \xff world"},
		},
		TotalTurns: 1,
	}
	result := ValidateFormat(snap, DefaultFormatValidatorConfig())
	if result.GetValid() {
		t.Error("expected invalid for bad UTF-8")
	}
}

func TestScreenToxicity_MultipleMatches(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "This has a slur and a threat"},
		},
		TotalTurns: 1,
	}
	result := ScreenToxicity(snap, []string{"slur", "threat"}, false)
	if !result.GetToxic() {
		t.Error("expected toxic")
	}
	if len(result.GetMatchedPatterns()) != 2 {
		t.Errorf("expected 2 matches, got %d", len(result.GetMatchedPatterns()))
	}
}

func TestDetectLanguage_English(t *testing.T) {
	profileDir := filepath.Join("..", "..", "data", "language-profiles")
	profiles, err := LoadLangProfiles(profileDir)
	if err != nil {
		t.Skipf("language profiles not found: %v", err)
	}

	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "The quick brown fox jumps over the lazy dog. This is a common English sentence used for testing purposes. It contains all the letters of the alphabet and is widely recognized in typographic circles around the world."},
		},
		TotalTurns: 1,
	}
	cfg := DefaultLanguageDetectorConfig()
	cfg.Profiles = profiles
	result := DetectLanguage(snap, profiles, cfg)
	if result.GetLanguageCode() != "en" {
		t.Errorf("expected en, got %s", result.GetLanguageCode())
	}
	if result.GetConfidence() < 0.6 {
		t.Errorf("expected confidence > 0.6, got %.2f", result.GetConfidence())
	}
}

func TestDetectLanguage_ShortFallback(t *testing.T) {
	cfg := DefaultLanguageDetectorConfig()
	cfg.Profiles = map[string]LangProfile{"en": {"the": 0.05}}
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Hi"},
		},
	}
	result := DetectLanguage(snap, cfg.Profiles, cfg)
	if result.GetLanguageCode() != "en" {
		t.Errorf("expected fallback to en, got %s", result.GetLanguageCode())
	}
	if result.GetConfidence() != 0.5 {
		t.Errorf("expected confidence 0.5, got %.2f", result.GetConfidence())
	}
}

// TestPipelineWithLayer0 runs the full pipeline with Layer 0 enabled.
func TestPipelineWithLayer0(t *testing.T) {
	profileDir := filepath.Join("..", "..", "data", "language-profiles")
	profiles, err := LoadLangProfiles(profileDir)
	if err != nil {
		t.Skipf("language profiles not found: %v", err)
	}

	blocklistPath := filepath.Join("..", "..", "data", "blocklists", "default.txt")
	blocklist, err := LoadBlocklist(blocklistPath)
	if err != nil {
		t.Skipf("blocklist not found: %v", err)
	}

	snap := &reasoningv1.ConversationSnapshot{
		Objective: "Discuss project planning",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Let's discuss the project timeline and budget."},
			{TurnNumber: 2, Speaker: "assistant", RawText: "Sure, the estimated budget is $50,000 and the deadline is next month."},
		},
		TotalTurns: 2,
	}

	cfg := DefaultPipelineConfig()
	cfg.UseLayer0 = true
	cfg.Layer0 = DefaultLayer0Config()
	cfg.Layer0.Toxicity.Blocklist = blocklist
	cfg.Layer0.Language.Profiles = profiles
	cfg.UseInhibitor = true
	cfg.Inhibitor = DefaultInhibitorConfig()

	result := Run(snap, cfg)
	if result.Rejected {
		t.Fatalf("expected accepted, got rejected: %s", result.Layer0.Reason)
	}
	if result.Report == nil {
		t.Fatal("expected report, got nil")
	}
}

// TestPipelineLayer0Rejection verifies that Layer 0 short-circuits the pipeline.
func TestPipelineLayer0Rejection(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "This has a death threat in it"},
		},
		TotalTurns: 1,
	}

	cfg := DefaultPipelineConfig()
	cfg.UseLayer0 = true
	cfg.Layer0 = DefaultLayer0Config()
	cfg.Layer0.Toxicity.Blocklist = []string{"death threat"}

	result := Run(snap, cfg)
	if !result.Rejected {
		t.Fatal("expected rejection")
	}
	// Verify no downstream processing occurred.
	if result.Report != nil {
		t.Error("expected nil report when rejected")
	}
	if len(result.Findings) != 0 {
		t.Errorf("expected no findings when rejected, got %d", len(result.Findings))
	}
}

func BenchmarkLayer0(b *testing.B) {
	profileDir := filepath.Join("..", "..", "data", "language-profiles")
	profiles, err := LoadLangProfiles(profileDir)
	if err != nil {
		b.Skipf("language profiles not found: %v", err)
	}

	blocklistPath := filepath.Join("..", "..", "data", "blocklists", "default.txt")
	blocklist, err := LoadBlocklist(blocklistPath)
	if err != nil {
		b.Skipf("blocklist not found: %v", err)
	}

	snap := &reasoningv1.ConversationSnapshot{
		Objective: "Discuss project planning",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Let's discuss the project timeline and budget constraints for the upcoming quarter."},
			{TurnNumber: 2, Speaker: "assistant", RawText: "The estimated budget is $50,000 and we need to consider the team capacity."},
			{TurnNumber: 3, Speaker: "user", RawText: "What about the technical risks and the deployment schedule?"},
		},
		TotalTurns: 3,
	}

	cfg := DefaultLayer0Config()
	cfg.Toxicity.Blocklist = blocklist
	cfg.Language.Profiles = profiles

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RunLayer0(snap, cfg)
	}
}
