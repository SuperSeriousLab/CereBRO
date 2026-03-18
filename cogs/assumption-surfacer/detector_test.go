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

// TestRun_HealthyWithPremises verifies that a conversation with balanced claim and
// premise phrases does NOT fire.
func TestRun_HealthyWithPremises(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Should we rewrite the service?"},
		{2, "assistant", "It follows that we should rewrite it because the codebase is unmaintainable and since the test coverage is below 10%. Given that the team has complained for months, this means we should act. Therefore we should plan a rewrite based on the evidence and since the velocity has dropped by 40%."},
	})

	result := Run(snap, DefaultConfig())
	if result != nil {
		t.Errorf("expected nil for premise-rich conversation, got %v (confidence=%.3f, explanation=%s)",
			result.GetFindingType(), result.GetConfidence(), result.GetExplanation())
	}
}

// TestRun_UnsupportedConclusion_Fires verifies the detector fires on a high-ratio
// claim conversation with no stated premises.
func TestRun_UnsupportedConclusion_Fires(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "What architecture should we choose?"},
		{2, "assistant", "Clearly, microservices is the right choice. Obviously this is superior to monoliths. Therefore we should adopt it immediately. Consequently all teams must migrate. Certainly the benefits are undeniable. Hence the decision is obvious."},
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
	if result.GetDetectorName() != "assumption-surfacer-detector" {
		t.Errorf("expected detector name 'assumption-surfacer-detector', got %q", result.GetDetectorName())
	}
}

// TestRun_SeverityHigh verifies CRITICAL severity when assumption_ratio > 0.85.
func TestRun_SeverityHigh(t *testing.T) {
	// Many claim phrases, zero premise phrases — ratio approaches 1.0.
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Is this secure?"},
		{2, "assistant", "Clearly yes. Obviously it is. Therefore it's fine. Thus no action needed. Hence we're safe. Consequently we can proceed. Certainly this is undeniably the right call. This means we are done."},
	})

	result := Run(snap, DefaultConfig())
	if result == nil {
		t.Fatal("expected finding for high-ratio conversation, got nil")
	}
	if result.GetSeverity() != reasoningv1.FindingSeverity_CRITICAL {
		t.Errorf("expected CRITICAL severity for extreme assumption ratio, got %v", result.GetSeverity())
	}
}

// TestRun_BelowMinClaimPhrases verifies the detector does not fire when
// claim phrases < MinClaimPhrases even if ratio would be high.
func TestRun_BelowMinClaimPhrases(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Should we deploy?"},
		{2, "assistant", "Clearly, this is fine."},
	})

	cfg := DefaultConfig()
	cfg.MinClaimPhrases = 3 // require at least 3 claim phrases

	result := Run(snap, cfg)
	if result != nil {
		t.Errorf("expected nil when claim_phrases < MinClaimPhrases, got %v (confidence=%.3f)",
			result.GetFindingType(), result.GetConfidence())
	}
}

// TestRun_MultiTurnAccumulation verifies claim and premise counts accumulate
// across multiple assistant turns.
func TestRun_MultiTurnAccumulation(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Turn 1"},
		{2, "assistant", "Clearly we should proceed. Obviously this is correct."},
		{3, "user", "Turn 2"},
		{4, "assistant", "Therefore it must be done. Thus the outcome is guaranteed. Consequently we are correct."},
	})

	result := Run(snap, DefaultConfig())
	if result == nil {
		t.Fatal("expected UNSUPPORTED_CONCLUSION across multi-turn accumulation, got nil")
	}
	if result.GetFindingType() != reasoningv1.FindingType_UNSUPPORTED_CONCLUSION {
		t.Errorf("expected UNSUPPORTED_CONCLUSION, got %v", result.GetFindingType())
	}
}

// TestDefaultConfig verifies default config values.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ClaimRatioThreshold != 0.75 {
		t.Errorf("expected ClaimRatioThreshold=0.75, got %.2f", cfg.ClaimRatioThreshold)
	}
	if cfg.MinClaimPhrases != 2 {
		t.Errorf("expected MinClaimPhrases=2, got %d", cfg.MinClaimPhrases)
	}
}
