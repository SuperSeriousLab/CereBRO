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

// TestRun_StatusQuoBias_Fires verifies the detector fires when the assistant
// repeatedly frames existing state as default-good and change as burdensome.
func TestRun_StatusQuoBias_Fires(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Should we migrate to the new framework?"},
		{2, "assistant", "We've always used this approach and it works fine. The current approach is proven and time-tested."},
		{3, "user", "But the new one has better performance."},
		{4, "assistant", "Why change what is working? No reason to change the established practice. Better safe than sorry."},
	})

	result := Run(snap, DefaultConfig())
	if result == nil {
		t.Fatal("expected STATUS_QUO_BIAS finding, got nil")
	}
	if result.GetFindingType() != reasoningv1.FindingType_STATUS_QUO_BIAS {
		t.Errorf("expected STATUS_QUO_BIAS, got %v", result.GetFindingType())
	}
	if result.GetConfidence() < 0.5 {
		t.Errorf("expected confidence >= 0.5, got %.3f", result.GetConfidence())
	}
	if result.GetDetectorName() != "status-quo-bias-detector" {
		t.Errorf("expected detector name 'status-quo-bias-detector', got %q", result.GetDetectorName())
	}
}

// TestRun_ChangePositive_NoFire verifies that a conversation with strong
// change-positive framing does NOT fire.
func TestRun_ChangePositive_NoFire(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Should we keep the old system?"},
		{2, "assistant", "We should update to the new solution. Time to change our approach — let's improve the architecture."},
		{3, "user", "Any concerns?"},
		{4, "assistant", "The migration path is clear. We can modernize incrementally and refactor the legacy components."},
	})

	result := Run(snap, DefaultConfig())
	if result != nil {
		t.Errorf("expected nil for change-positive conversation, got %v (explanation=%s)",
			result.GetFindingType(), result.GetExplanation())
	}
}

// TestRun_BelowMatchThreshold_NoFire verifies detector does not fire when
// total bias phrase count is below MinMatchCount.
func TestRun_BelowMatchThreshold_NoFire(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "What do you think about changing our process?"},
		{2, "assistant", "The current approach has some merit, but we should evaluate alternatives carefully."},
	})

	result := Run(snap, DefaultConfig())
	if result != nil {
		t.Errorf("expected nil below match threshold, got %v", result.GetFindingType())
	}
}

// TestRun_SeverityCritical verifies CRITICAL severity when avg_bias_score > 0.85.
func TestRun_SeverityCritical(t *testing.T) {
	// Each assistant turn needs (sq+cb)/(sq+cb+1) > 0.85, i.e. sq+cb >= 6 per turn.
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "We need to modernize our stack."},
		{2, "assistant", "We've always done it this way. The current approach is tried and true. Maintain the status quo. Established practice is proven approach. Time-tested. Better safe than sorry."},
		{3, "user", "Performance is suffering."},
		{4, "assistant", "Why change? No reason to change. What's wrong with the established practice? Better safe than sorry. We've always done it. Stick with the current approach."},
		{5, "user", "Our competitors have moved on."},
		{6, "assistant", "Traditionally we've done this. Stick with the proven approach. Leave it as is. Why change for the sake of change? Do we really need to change? Is it worth changing?"},
	})

	result := Run(snap, DefaultConfig())
	if result == nil {
		t.Fatal("expected finding for very high bias score, got nil")
	}
	if result.GetSeverity() != reasoningv1.FindingSeverity_CRITICAL {
		t.Errorf("expected CRITICAL severity for avg_bias_score > 0.85, got %v", result.GetSeverity())
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
		{1, "user", "We've always done it this way. Why change? Better safe than sorry."},
		{2, "user", "The current approach is time-tested. No reason to change the established practice."},
	})

	result := Run(snap, DefaultConfig())
	if result != nil {
		t.Error("expected nil when there are no assistant turns")
	}
}

// TestDefaultConfig verifies default config values match spec.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.BiasScoreThreshold != 0.65 {
		t.Errorf("expected BiasScoreThreshold=0.65, got %.2f", cfg.BiasScoreThreshold)
	}
	if cfg.MinMatchCount != 3 {
		t.Errorf("expected MinMatchCount=3, got %d", cfg.MinMatchCount)
	}
}
