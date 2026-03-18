package main

import (
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// newSnap builds a ConversationSnapshot from turn data.
func newSnap(turns []struct {
	num     uint32
	speaker string
	text    string
}) *reasoningv1.ConversationSnapshot {
	var ts []*reasoningv1.Turn
	for _, t := range turns {
		ts = append(ts, &reasoningv1.Turn{
			TurnNumber: t.num,
			Speaker:    t.speaker,
			RawText:    t.text,
		})
	}
	return &reasoningv1.ConversationSnapshot{
		Turns:      ts,
		TotalTurns: uint32(len(ts)),
	}
}

// TestRun_NilSnapshot verifies nil safety.
func TestRun_NilSnapshot(t *testing.T) {
	result := Run(nil, DefaultConfig())
	if result != nil {
		t.Error("expected nil for nil snapshot")
	}
}

// TestRun_EmptySnapshot verifies an empty conversation does not fire.
func TestRun_EmptySnapshot(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{TotalTurns: 0}
	result := Run(snap, DefaultConfig())
	if result != nil {
		t.Error("expected nil for empty snapshot")
	}
}

// TestRun_HealthyConversation verifies that a conversation with distinct
// reasoning per sentence does NOT fire.
func TestRun_HealthyConversation(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Why should we migrate to microservices?"},
		{2, "assistant", "The monolith is unwieldy. Deployment takes four hours because every service must redeploy together."},
		{3, "user", "What about cost?"},
		{4, "assistant", "Costs increase initially. However team velocity improves because services release independently."},
	})

	result := Run(snap, DefaultConfig())
	if result != nil {
		t.Errorf("expected nil for non-circular conversation, got %v (explanation=%s)",
			result.GetFindingType(), result.GetExplanation())
	}
}

// TestRun_CircularReasoning_Fires verifies the detector fires when a conclusion
// is restated as its own justification within multiple turns.
// Each turn contains a sentence pair where sentence B restates sentence A's
// key words and adds a causal connector — X therefore X pattern.
func TestRun_CircularReasoning_Fires(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Is this approach correct?"},
		// Turn 2: "approach correct" in both sentences; second sentence has "because".
		{2, "assistant", "The approach is correct. The approach is correct because approach is always correct."},
		{3, "user", "Why is that valid?"},
		// Turn 4: "solution valid" in both sentences; second sentence has "therefore".
		{4, "assistant", "The solution is valid. The solution is valid therefore solution remains valid."},
	})

	result := Run(snap, DefaultConfig())
	if result == nil {
		t.Fatal("expected UNSUPPORTED_CONCLUSION finding, got nil")
	}
	if result.GetFindingType() != reasoningv1.FindingType_UNSUPPORTED_CONCLUSION {
		t.Errorf("expected UNSUPPORTED_CONCLUSION, got %v", result.GetFindingType())
	}
	if result.GetConfidence() < 0.5 {
		t.Errorf("expected confidence >= 0.5, got %.3f", result.GetConfidence())
	}
	if result.GetDetectorName() != "circular-reasoning-detector" {
		t.Errorf("expected detector name 'circular-reasoning-detector', got %q", result.GetDetectorName())
	}
}

// TestRun_BelowMinCircularTurns verifies detector does not fire when only one
// turn is circular but MinCircularTurns requires two.
func TestRun_BelowMinCircularTurns(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Should we proceed?"},
		// Turn 2: circular (X because X).
		{2, "assistant", "We should proceed. We should proceed because we should proceed."},
		{3, "user", "Are there risks?"},
		// Turn 4: not circular — genuinely different content.
		{4, "assistant", "The risks are manageable. We evaluated each factor and mitigation strategy carefully."},
		{5, "user", "Good point."},
		// Turn 6: not circular.
		{6, "assistant", "Thank you. The timeline looks achievable based on capacity analysis."},
	})

	// circular_turns=1, total_content_turns=3, ratio=0.33 < 0.35 AND circular_turns < MinCircularTurns=2.
	result := Run(snap, DefaultConfig())
	if result != nil {
		t.Errorf("expected nil when circular_turns < MinCircularTurns, got %v", result.GetFindingType())
	}
}

// TestRun_SeverityHigh verifies CRITICAL severity when circular_ratio > 0.6.
func TestRun_SeverityHigh(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Is this secure?"},
		// Turn 2: circular.
		{2, "assistant", "The system is secure. The system is secure because system is secure."},
		{3, "user", "Why is it safe?"},
		// Turn 4: circular.
		{4, "assistant", "The system is safe. The system is safe because system remains safe."},
		{5, "user", "Any vulnerabilities?"},
		// Turn 6: circular.
		{6, "assistant", "There are no vulnerabilities. There are no vulnerabilities because vulnerabilities do not exist."},
	})

	// circular_turns=3, total_content_turns=3, ratio=1.0 > 0.6 → CRITICAL.
	result := Run(snap, DefaultConfig())
	if result == nil {
		t.Fatal("expected finding for high-circular-ratio conversation, got nil")
	}
	if result.GetSeverity() != reasoningv1.FindingSeverity_CRITICAL {
		t.Errorf("expected CRITICAL severity for circular_ratio > 0.6, got %v", result.GetSeverity())
	}
}

// TestRun_CustomSimilarityThreshold verifies that raising the similarity threshold
// suppresses borderline circular pairs.
func TestRun_CustomSimilarityThreshold(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Is this approach correct?"},
		// Partial overlap: "approach" shared; Jaccard ~ 0.2 (below 0.95 but above 0.55).
		{2, "assistant", "The approach is promising. The approach seems correct because results were positive."},
		{3, "user", "Why is it well designed?"},
		{4, "assistant", "It is well designed because good design decisions were taken during planning."},
	})

	// With a very high threshold (0.95), these partial overlaps should not qualify.
	cfg := DefaultConfig()
	cfg.SimilarityThreshold = 0.95

	result := Run(snap, cfg)
	if result != nil {
		t.Errorf("expected nil for high similarity threshold, got %v (confidence=%.3f)",
			result.GetFindingType(), result.GetConfidence())
	}
}

// TestRun_OnlyUserTurns verifies that a conversation with only user turns
// does not fire (no assistant content to analyse).
func TestRun_OnlyUserTurns(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "This is correct. This is correct because this is correct."},
		{2, "user", "The plan works. The plan works because the plan works."},
	})

	result := Run(snap, DefaultConfig())
	if result != nil {
		t.Error("expected nil when there are no assistant turns")
	}
}

// TestDefaultConfig verifies default config values match spec.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.SimilarityThreshold != 0.55 {
		t.Errorf("expected SimilarityThreshold=0.55, got %.2f", cfg.SimilarityThreshold)
	}
	if cfg.CircularRatioThreshold != 0.35 {
		t.Errorf("expected CircularRatioThreshold=0.35, got %.2f", cfg.CircularRatioThreshold)
	}
	if cfg.MinCircularTurns != 2 {
		t.Errorf("expected MinCircularTurns=2, got %d", cfg.MinCircularTurns)
	}
}
