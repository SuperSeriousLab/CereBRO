package main

import (
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func testBlocklist() []string {
	return []string{"hate speech", "slur", "threat"}
}

func TestScreen_BlocklistedStandaloneWord(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "This contains a slur in it"},
		},
	}
	result := Screen(snap, testBlocklist(), DefaultConfig())
	if !result.GetToxic() {
		t.Error("expected toxic for blocklisted standalone word")
	}
	if len(result.GetMatchedPatterns()) != 1 || result.GetMatchedPatterns()[0] != "slur" {
		t.Errorf("expected [slur], got %v", result.GetMatchedPatterns())
	}
}

func TestScreen_BlocklistedEmbeddedInWord(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "This is a slurry of data"},
		},
	}
	result := Screen(snap, testBlocklist(), DefaultConfig())
	if result.GetToxic() {
		t.Error("expected non-toxic when blocklisted term is embedded in longer word")
	}
}

func TestScreen_CleanTechnicalText(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Please kill the process and abort the operation"},
		},
	}
	result := Screen(snap, testBlocklist(), DefaultConfig())
	if result.GetToxic() {
		t.Error("expected non-toxic for clean technical text")
	}
}

func TestScreen_MultipleMatches(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "This has hate speech and a threat"},
		},
	}
	result := Screen(snap, testBlocklist(), DefaultConfig())
	if !result.GetToxic() {
		t.Error("expected toxic")
	}
	if len(result.GetMatchedPatterns()) != 2 {
		t.Errorf("expected 2 matched patterns, got %d", len(result.GetMatchedPatterns()))
	}
}

func TestScreen_EmptyBlocklist(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "anything goes"},
		},
	}
	result := Screen(snap, nil, DefaultConfig())
	if result.GetToxic() {
		t.Error("expected non-toxic with empty blocklist")
	}
}

func TestScreen_CaseInsensitive(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "This contains a SLUR here"},
		},
	}
	result := Screen(snap, testBlocklist(), DefaultConfig())
	if !result.GetToxic() {
		t.Error("expected toxic (case insensitive match)")
	}
}

func TestScreen_WordBoundaryPunctuation(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Is this a slur?"},
		},
	}
	result := Screen(snap, testBlocklist(), DefaultConfig())
	if !result.GetToxic() {
		t.Error("expected toxic (punctuation is a word boundary)")
	}
}

func TestScreen_NilSnapshot(t *testing.T) {
	result := Screen(nil, testBlocklist(), DefaultConfig())
	if result.GetToxic() {
		t.Error("expected non-toxic for nil snapshot")
	}
}

func TestMatchesWithWordBoundary(t *testing.T) {
	tests := []struct {
		text    string
		pattern string
		want    bool
	}{
		{"hello world", "hello", true},
		{"say hello!", "hello", true},
		{"helloworld", "hello", false},
		{"assassination", "ass", false},
		{"what an ass!", "ass", true},
		{"", "test", false},
		{"test", "test", true},
		{"testing", "test", false},
		{"a test b", "test", true},
		{"multi word phrase here", "word phrase", true},
		{"password", "word", false},
	}
	for _, tt := range tests {
		got := matchesWithWordBoundary(tt.text, tt.pattern)
		if got != tt.want {
			t.Errorf("matchesWithWordBoundary(%q, %q) = %v, want %v", tt.text, tt.pattern, got, tt.want)
		}
	}
}
