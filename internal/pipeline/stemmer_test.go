package pipeline

import (
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// ----------------------------------------------------------------
// stemWord unit tests
// ----------------------------------------------------------------

func TestStemWord_CommonSuffixes(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// -ing stripping
		{"arguing", "argu"},
		{"running", "runn"},
		{"thinking", "think"},
		// -ness stripping
		{"darkness", "dark"},
		{"happiness", "happi"},
		// -ment stripping
		{"argument", "argu"},
		{"movement", "move"}, // "movement" strips "-ment" → "move" (correct root)
		// -ation stripping (5-char rule fires before -tion)
		{"nation", "nat"},
		// -tion stripping
		{"question", "ques"},
		// -er stripping
		{"runner", "runn"},
		{"thinker", "think"},
		// -ed stripping
		{"argued", "argu"},
		{"jumped", "jump"},
		// -ly stripping
		{"quickly", "quick"},
		{"slowly", "slow"},
		// Short words — must not be over-stripped (min len 3)
		{"big", "big"},
		{"run", "run"},
		// -ments plural stripping (via "ments" rule)
		{"arguments", "argu"},
		// -able stripping
		{"readable", "read"},
		// -less stripping
		{"helpless", "help"},
		// -ful stripping
		{"helpful", "help"},
		// -ice stripping (justice/just match)
		{"justice", "just"},
		// -y stripping (philosophy/philosopher match)
		{"philosophy", "philosoph"},
		{"philosopher", "philosoph"},
	}

	for _, tc := range cases {
		got := stemWord(tc.input)
		if got != tc.want {
			t.Errorf("stemWord(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestStemWord_MinimumLength(t *testing.T) {
	// Words that would under-shoot the 3-char minimum must be returned as-is.
	cases := []struct {
		input string
	}{
		{"add"},
		{"are"},
		{"ago"},
	}
	for _, tc := range cases {
		got := stemWord(tc.input)
		if len(got) < 3 {
			t.Errorf("stemWord(%q) = %q: result shorter than 3 chars", tc.input, got)
		}
	}
}

// ----------------------------------------------------------------
// extractKeywords stemming integration tests
// ----------------------------------------------------------------

func TestExtractKeywords_InflectedFormsShareStem(t *testing.T) {
	cases := []struct {
		desc      string
		textA     string
		textB     string
		wantMatch bool // whether the two texts share at least one keyword stem
	}{
		{
			// "arguing" and "argument" both stem to "argu" via -ing and -ment rules.
			desc:      "arguing vs argument — same stem via -ing / -ment stripping",
			textA:     "They were arguing about the policy proposal",
			textB:     "The core argument has not changed significantly",
			wantMatch: true,
		},
		{
			// "philosopher" and "philosophy" both stem to "philosoph".
			desc:      "philosopher vs philosophy — same stem via -er / -y stripping",
			textA:     "Every philosopher must study logic carefully",
			textB:     "Philosophy requires careful logical reasoning",
			wantMatch: true,
		},
		{
			// Completely unrelated words must NOT share a stem.
			desc:      "database vs pizza — no overlap expected",
			textA:     "PostgreSQL database selection criteria matter greatly",
			textB:     "Italian pizza topping choices include pepperoni mushroom",
			wantMatch: false,
		},
	}

	for _, tc := range cases {
		kwA := extractKeywords(tc.textA)
		kwB := extractKeywords(tc.textB)

		setA := make(map[string]bool, len(kwA))
		for _, k := range kwA {
			setA[k] = true
		}
		overlap := false
		for _, k := range kwB {
			if setA[k] {
				overlap = true
				break
			}
		}
		if overlap != tc.wantMatch {
			t.Errorf("%s: kwA=%v kwB=%v wantMatch=%v gotMatch=%v",
				tc.desc, kwA, kwB, tc.wantMatch, overlap)
		}
	}
}

// ----------------------------------------------------------------
// DetectScopeDrift stemming integration tests
// ----------------------------------------------------------------

// makeScopeSnap creates a minimal ConversationSnapshot suitable for scope drift
// testing. objective and turns are enriched so topic keywords are populated.
func makeScopeSnap(objective string, turnTexts []string) *reasoningv1.ConversationSnapshot {
	snap := &reasoningv1.ConversationSnapshot{
		Objective: objective,
	}
	for i, text := range turnTexts {
		snap.Turns = append(snap.Turns, &reasoningv1.Turn{
			TurnNumber: uint32(i + 1),
			Speaker:    "user",
			RawText:    text,
		})
	}
	return Enrich(snap)
}

// TestScopeDrift_InflectedFormsNoDrift verifies that a conversation that stays
// on topic is NOT flagged as drifting, even when it uses varied word forms
// (e.g. "justice" vs "just", "arguing" vs "argument").
func TestScopeDrift_InflectedFormsNoDrift(t *testing.T) {
	// 12 turns all discussing the same topic (philosophy of justice) using
	// varied inflections of the same root words.
	turns := []string{
		"We are discussing the philosophy of justice today",
		"Philosophers argue that justice requires fairness for all citizens",
		"The philosophical arguments about justice date back to ancient Greece",
		"Aristotle justified his views on justice through careful reasoning",
		"His justification relied on the concept of proportional distribution",
		"Many philosophers have argued similar positions on social justice",
		"The argumentation in Plato's Republic centers on justice and virtue",
		"Justice systems in democracies try to be just and impartial",
		"Philosophical reasoning about justice is fundamental to ethics",
		"Arguments for social justice often appeal to fairness principles",
		"Philosophers justify their ethical claims through reasoned argument",
		"The just society remains a central concern of moral philosophy",
	}

	snap := makeScopeSnap("Discuss the philosophy of justice", turns)
	cfg := DefaultScopeGuardConfig()
	result := DetectScopeDrift(snap, cfg)

	if result != nil {
		t.Errorf("expected no scope drift for on-topic philosophical discussion, got finding: %v", result.GetExplanation())
	}
}

// TestScopeDrift_ClearDriftDetected verifies that a conversation that clearly
// drifts away from its objective IS still flagged.
func TestScopeDrift_ClearDriftDetected(t *testing.T) {
	// Starts on philosophy, then drifts to completely unrelated topics.
	turns := []string{
		"Let us examine the philosophy of justice",
		"Plato argues that justice is a virtue of the soul",
		"Philosophers have reasoned about justice for millennia",
		"What is the best pizza topping for a weekend meal",
		"Pepperoni and mushrooms are popular pizza choices",
		"The best way to cook pizza is in a wood-fired oven",
		"Italian cuisine also includes pasta and risotto dishes",
		"Wine pairing with pasta dishes requires care and knowledge",
		"Red wines complement tomato-based pasta sauces well",
		"Cooking pasta requires salted boiling water and timing",
		"Homemade pizza dough needs yeast and proper fermentation",
		"Pizza ovens should reach high temperatures for crispy crust",
	}

	snap := makeScopeSnap("Examine the philosophy of justice", turns)
	cfg := DefaultScopeGuardConfig()
	// Use fewer sustained turns for a shorter conversation.
	cfg.SustainedTurns = 5
	result := DetectScopeDrift(snap, cfg)

	if result == nil {
		t.Error("expected scope drift detection for conversation that switched from philosophy to pizza, got nil")
	}
}

// TestScopeDrift_SustainedTurns8_CleanNotFlagged verifies that sustained_turns=8
// discrimination works: a conversation with only 7 consecutive drifting turns
// is NOT flagged.
func TestScopeDrift_SustainedTurns8_CleanNotFlagged(t *testing.T) {
	// 4 on-topic reference turns + 7 drifting turns = exactly one short of 8.
	turns := []string{
		// Reference turns (on topic: database selection)
		"Help me choose a database for my application",
		"PostgreSQL is a reliable relational database choice",
		"It handles SQL queries and ACID transactions well",
		"What hosting options are available for PostgreSQL",
		// 7 off-topic turns (not enough to trigger sustained=8)
		"Kubernetes orchestrates containerized applications at scale",
		"Container orchestration helps with deployment automation",
		"Hiring DevOps engineers requires specialized skill sets",
		"Remote work policies affect team collaboration dynamics",
		"Office space leasing involves long-term commercial contracts",
		"Ergonomic furniture improves developer productivity and health",
		"Standing desks reduce sedentary work-related health risks",
	}

	snap := makeScopeSnap("Choose a database for my application", turns)
	cfg := DefaultScopeGuardConfig()
	// sustained_turns=8 is already the default; explicitly set for clarity.
	cfg.SustainedTurns = 8
	result := DetectScopeDrift(snap, cfg)

	if result != nil {
		t.Errorf("expected no scope drift (only 7 consecutive drifting turns, need 8), got finding: %v",
			result.GetExplanation())
	}
}
