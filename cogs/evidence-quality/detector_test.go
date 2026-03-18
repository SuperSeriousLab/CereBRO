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

// TestRun_EmpiricalConversation verifies that a conversation with strong
// empirical backing does NOT fire.
func TestRun_EmpiricalConversation(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Does exercise improve memory?"},
		{2, "assistant", "Research shows that aerobic exercise increases hippocampal volume. Studies show measurable improvements in spatial memory after 6 months of regular exercise."},
		{3, "user", "How significant is the effect?"},
		{4, "assistant", "Data shows effect sizes ranging from 0.3 to 0.6 standard deviations. Meta-analysis of peer-reviewed trials confirms the benefit is statistically significant."},
	})

	result := Run(snap, DefaultConfig())
	if result != nil {
		t.Errorf("expected nil for empirically-grounded conversation, got %v (explanation=%s)",
			result.GetFindingType(), result.GetExplanation())
	}
}

// TestRun_AnecdotalHighConfidence_Fires verifies the detector fires when
// multiple turns combine high-confidence markers with anecdotal phrases.
func TestRun_AnecdotalHighConfidence_Fires(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Is this supplement effective?"},
		{2, "assistant", "I heard it definitely works wonders. People say it is clearly superior to anything else available."},
		{3, "user", "Should I trust these claims?"},
		{4, "assistant", "Supposedly it is proven to be effective. Word is it certainly cures most ailments people think are untreatable."},
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
	if result.GetDetectorName() != "evidence-quality-detector" {
		t.Errorf("expected detector name 'evidence-quality-detector', got %q", result.GetDetectorName())
	}
}

// TestRun_HighAnecdotalRatio_Fires verifies firing on high anecdotal ratio alone.
func TestRun_HighAnecdotalRatio_Fires(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "What causes this problem?"},
		{2, "assistant", "I think it is probably related to stress. People say it might be genetic perhaps."},
		{3, "user", "Any solutions?"},
		{4, "assistant", "Some say you should probably just rest. In my experience it could be fixed by perhaps changing diet."},
	})

	result := Run(snap, DefaultConfig())
	if result == nil {
		t.Fatal("expected UNSUPPORTED_CONCLUSION finding for high anecdotal ratio, got nil")
	}
	if result.GetFindingType() != reasoningv1.FindingType_UNSUPPORTED_CONCLUSION {
		t.Errorf("expected UNSUPPORTED_CONCLUSION, got %v", result.GetFindingType())
	}
}

// TestRun_BelowThreshold_NoFire verifies detector does not fire when anecdotal
// count is too low.
func TestRun_BelowThreshold_NoFire(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "What do you think?"},
		{2, "assistant", "I think this might be a viable approach based on the data we have."},
		{3, "user", "Any other thoughts?"},
		{4, "assistant", "Research shows the technique is effective in most scenarios."},
	})

	// Only 1 anecdotal phrase total ("I think"), well below MinAnecdotalCount=2 for ratio path.
	// No high-conf+anecdote combo fires either.
	result := Run(snap, DefaultConfig())
	if result != nil {
		t.Errorf("expected nil for low anecdotal count, got %v", result.GetFindingType())
	}
}

// TestRun_SeverityCritical verifies CRITICAL severity when avg_ratio > 0.85.
func TestRun_SeverityCritical(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Tell me about this treatment."},
		{2, "assistant", "I heard it works. People say it probably helps. Some say it might be the best option. Word is it could be revolutionary. I think it is perhaps the future."},
		{3, "user", "And its safety?"},
		{4, "assistant", "Supposedly it is safe. People think it probably has no side effects. I believe it might be fine. Anecdotally it seems like it could be harmless."},
		{5, "user", "So it is reliable?"},
		{6, "assistant", "Some say it is supposedly proven. I heard it probably works for everyone. Rumor is it might even cure things perhaps."},
	})

	result := Run(snap, DefaultConfig())
	if result == nil {
		t.Fatal("expected finding for very high anecdotal ratio, got nil")
	}
	if result.GetSeverity() != reasoningv1.FindingSeverity_CRITICAL {
		t.Errorf("expected CRITICAL severity for avg_ratio > 0.85, got %v", result.GetSeverity())
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
		{1, "user", "I heard this works. Supposedly it is definitely proven."},
		{2, "user", "People say it clearly works. Word is it certainly fixes everything."},
	})

	result := Run(snap, DefaultConfig())
	if result != nil {
		t.Error("expected nil when there are no assistant turns")
	}
}

// TestDefaultConfig verifies default config values match spec.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AnecdotalRatioThreshold != 0.70 {
		t.Errorf("expected AnecdotalRatioThreshold=0.70, got %.2f", cfg.AnecdotalRatioThreshold)
	}
	if cfg.MinAnecdotalCount != 2 {
		t.Errorf("expected MinAnecdotalCount=2, got %d", cfg.MinAnecdotalCount)
	}
	if cfg.MinHighConfidenceAnecdoteTurns != 2 {
		t.Errorf("expected MinHighConfidenceAnecdoteTurns=2, got %d", cfg.MinHighConfidenceAnecdoteTurns)
	}
}
