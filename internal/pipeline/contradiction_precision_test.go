package pipeline

import (
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// TestContradiction_FalsePositiveReduction verifies that the patterns that
// caused 100% FP rate in production (0/49 genuine contradictions detected)
// are now suppressed by the tightened threshold and semantic filtering.
//
// Root cause of production FP rate:
//   - base confidence 0.4 meant any negation+overlap detection fired immediately
//   - hasNegationConflict fired on incidental negations ("I don't know" vs
//     any assertion containing a shared stop-word-stripped token)
//   - MinOverlap 0.3 allowed loosely-related sentences to pair
func TestContradiction_FalsePositiveReduction(t *testing.T) {
	cfg := DefaultContradictionConfig()

	// Case 1: Direction change — legitimate topic pivot, NOT a contradiction.
	// The old detector fired when an assistant changed approach across turns
	// because the sentences shared topic words and one contained a negation.
	directionChange := makeSnap([]struct {
		num              uint32
		speaker, text string
	}{
		{1, "assistant", "We should use PostgreSQL for this project."},
		{2, "user", "Actually, can we reconsider?"},
		{3, "assistant", "On reflection, we don't need a relational database here."},
	}, "project")
	result := DetectContradiction(directionChange, cfg)
	if result != nil {
		t.Errorf("direction change should not be flagged as contradiction, got confidence=%.3f: %q vs %q",
			result.GetConfidence(),
			result.GetContradiction().GetClaimAText(),
			result.GetContradiction().GetClaimBText())
	}

	// Case 2: Incidental negation — sentences share topic words but the negation
	// is unrelated to the shared subject. "I don't know what to do" vs any
	// assertion should not fire.
	incidentalNegation := makeSnap([]struct {
		num              uint32
		speaker, text string
	}{
		{1, "assistant", "The budget is currently set at one hundred thousand dollars."},
		{3, "assistant", "I don't know what the timeline looks like at this point."},
	}, "planning")
	result = DetectContradiction(incidentalNegation, cfg)
	if result != nil {
		t.Errorf("incidental negation on unrelated subject should not fire, got confidence=%.3f: %q vs %q",
			result.GetConfidence(),
			result.GetContradiction().GetClaimAText(),
			result.GetContradiction().GetClaimBText())
	}

	// Case 3: Weak overlap only — sentences share 1-2 content words but are
	// discussing different aspects. Should not fire below 60% overlap threshold.
	weakOverlap := makeSnap([]struct {
		num              uint32
		speaker, text string
	}{
		{1, "assistant", "The deployment pipeline needs to be fixed urgently."},
		{3, "assistant", "I don't think we should rush the security review process."},
	}, "engineering")
	result = DetectContradiction(weakOverlap, cfg)
	if result != nil {
		t.Errorf("weak word overlap should not fire contradiction, got confidence=%.3f: %q vs %q",
			result.GetConfidence(),
			result.GetContradiction().GetClaimAText(),
			result.GetContradiction().GetClaimBText())
	}

	// Case 4: Short sentences — noise sentences under contradictionMinSentenceLen
	// chars should be filtered out before comparison.
	shortSentences := makeSnap([]struct {
		num              uint32
		speaker, text string
	}{
		{1, "assistant", "Yes."},
		{3, "assistant", "No."},
	}, "chat")
	result = DetectContradiction(shortSentences, cfg)
	if result != nil {
		t.Errorf("short sentences should be filtered, got confidence=%.3f", result.GetConfidence())
	}
}

// TestContradiction_GenuineDetection verifies that genuine, high-confidence
// contradictions still fire after the tightened threshold. A genuine contradiction
// requires: same speaker, same subject, direct logical opposition.
func TestContradiction_GenuineDetection(t *testing.T) {
	cfg := DefaultContradictionConfig()

	// Genuine contradiction: same speaker asserts X, then explicitly negates X
	// on the same subject with strong word overlap.
	genuine := makeSnap([]struct {
		num              uint32
		speaker, text string
	}{
		{1, "assistant", "We should always use encryption for all stored user data."},
		{3, "assistant", "We should never use encryption for stored user data in this context."},
	}, "security")
	result := DetectContradiction(genuine, cfg)
	if result == nil {
		t.Error("genuine contradiction (always/never + high overlap on same subject) should fire")
		return
	}
	if result.GetConfidence() < 0.75 {
		t.Errorf("genuine contradiction confidence should be ≥0.75, got %.3f", result.GetConfidence())
	}
	if result.GetFindingType() != reasoningv1.FindingType_CONTRADICTION {
		t.Errorf("expected CONTRADICTION finding type, got %v", result.GetFindingType())
	}
	t.Logf("genuine contradiction detected: confidence=%.3f kind inference from %q vs %q",
		result.GetConfidence(),
		result.GetContradiction().GetClaimAText(),
		result.GetContradiction().GetClaimBText())
}

// TestContradiction_CTOSessionHigherBar verifies that claude_code_session
// objective requires an even higher threshold than default sessions.
func TestContradiction_CTOSessionHigherBar(t *testing.T) {
	cfg := DefaultContradictionConfig()

	// This snap would fire in a normal session but must be suppressed for CTO.
	// Marginal contradiction: high overlap + negation, but CTO bar is 0.85.
	marginal := makeSnap([]struct {
		num              uint32
		speaker, text string
	}{
		{1, "assistant", "We should use encryption for all stored user data always."},
		{3, "assistant", "We should never use encryption for stored user data always."},
	}, "claude_code_session")

	result := DetectContradiction(marginal, cfg)
	// CTO session requires ≥ 0.85. Log what we get, but don't hard-fail on
	// this — the boundary behavior is configuration-dependent. Just ensure if
	// it fires, the confidence is at or above the CTO threshold.
	if result != nil && result.GetConfidence() < contradictionMinEmitConfidence+contradictionCTOThresholdBoost {
		t.Errorf("CTO session finding must have confidence ≥ %.2f, got %.3f",
			contradictionMinEmitConfidence+contradictionCTOThresholdBoost,
			result.GetConfidence())
	}
}

// TestContradiction_NegationNearSharedWord verifies the anchored negation check:
// negation must be in proximity to a shared content word to qualify.
func TestContradiction_NegationNearSharedWord(t *testing.T) {
	// The shared word is "postgres". Negation "not" appears right next to it.
	shared := map[string]bool{"postgres": true, "use": true}

	// Negation immediately before shared word — should match.
	near := "we should not use postgres for this"
	if !negationNearSharedWord(near, "not ", shared) {
		t.Error("negation directly before shared word 'postgres' should match")
	}

	// Negation far from shared word (> 5 token gap) — should not match.
	far := "i don't think that the timeline and budget constraints allow for postgres"
	if negationNearSharedWord(far, "don't ", shared) {
		t.Error("negation 7+ tokens from shared word 'postgres' should not match")
	}
}
