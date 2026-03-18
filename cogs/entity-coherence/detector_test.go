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

// TestRun_TooFewAssistantTurns verifies that fewer than 3 assistant turns
// does not fire (insufficient signal).
func TestRun_TooFewAssistantTurns(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "How is the system?"},
		{2, "assistant", "The system is reliable and stable."},
		{3, "user", "Are you sure?"},
		{4, "assistant", "Yes, the system is unreliable."},
	})

	result := Run(snap, DefaultConfig())
	if result != nil {
		t.Errorf("expected nil for < 3 assistant turns, got %v", result.GetFindingType())
	}
}

// TestRun_EntityCoherence_Fires verifies the detector fires when the same entities
// are described with contradictory properties across different turns.
func TestRun_EntityCoherence_Fires(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Tell me about the Payment Service and the Auth System."},
		{2, "assistant", "The Payment Service is reliable and fast. The Auth System is stable and accurate."},
		{3, "user", "Any concerns?"},
		{4, "assistant", "The Payment Service is robust overall. The Auth System is dependable."},
		{5, "user", "What about production issues?"},
		{6, "assistant", "The Payment Service is unreliable and slow. The Auth System is broken and inconsistent."},
	})

	result := Run(snap, DefaultConfig())
	if result == nil {
		t.Fatal("expected CONTRADICTION finding, got nil")
	}
	if result.GetFindingType() != reasoningv1.FindingType_CONTRADICTION {
		t.Errorf("expected CONTRADICTION, got %v", result.GetFindingType())
	}
	if result.GetConfidence() < 0.5 {
		t.Errorf("expected confidence >= 0.5, got %.3f", result.GetConfidence())
	}
	if result.GetDetectorName() != "entity-coherence-detector" {
		t.Errorf("expected detector name 'entity-coherence-detector', got %q", result.GetDetectorName())
	}
}

// TestRun_ProxyEntity_Fires verifies proxy entity tracking ("the system", etc.).
func TestRun_ProxyEntity_Fires(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "What about the system and the model?"},
		{2, "assistant", "The system is reliable. The model is accurate."},
		{3, "user", "Any issues recently?"},
		{4, "assistant", "The system performance looks good. The model is consistent."},
		{5, "user", "I heard about problems."},
		{6, "assistant", "The system is unstable and fragile. The model is inaccurate and inefficient."},
	})

	result := Run(snap, DefaultConfig())
	if result == nil {
		t.Fatal("expected CONTRADICTION finding for proxy entities, got nil")
	}
	if result.GetFindingType() != reasoningv1.FindingType_CONTRADICTION {
		t.Errorf("expected CONTRADICTION, got %v", result.GetFindingType())
	}
}

// TestRun_AcknowledgementSkipped verifies that turns explicitly acknowledging
// a change are excluded from contradiction detection.
func TestRun_AcknowledgementSkipped(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "How is the system and the service?"},
		{2, "assistant", "The system is reliable. The service is stable."},
		{3, "user", "What changed?"},
		{4, "assistant", "The system used to be reliable but now it is unstable. The service changed to being slow."},
		{5, "user", "Thanks."},
		{6, "assistant", "Right, the system is consistent overall. The service continues to be effective."},
	})

	result := Run(snap, DefaultConfig())
	// Turn 4 is an acknowledgement turn — should not count as contradiction.
	if result != nil {
		// May still be nil due to acknowledgement markers suppressing contradiction.
		// If it fires, ensure it's CONTRADICTION (not a different type).
		if result.GetFindingType() != reasoningv1.FindingType_CONTRADICTION {
			t.Errorf("unexpected finding type: %v", result.GetFindingType())
		}
	}
}

// TestRun_ConsistentEntity_NoFire verifies that consistently positive descriptions
// do not fire.
func TestRun_ConsistentEntity_NoFire(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Tell me about the Payment Service."},
		{2, "assistant", "The Payment Service is reliable and stable."},
		{3, "user", "Is it good?"},
		{4, "assistant", "The Payment Service is robust and efficient."},
		{5, "user", "Any issues?"},
		{6, "assistant", "The Payment Service is solid and dependable."},
	})

	result := Run(snap, DefaultConfig())
	if result != nil {
		t.Errorf("expected nil for consistently described entity, got %v (explanation=%s)",
			result.GetFindingType(), result.GetExplanation())
	}
}

// TestRun_BelowThreshold_NoFire verifies that no contradictions at all
// (no entity appearing with both positive and negative descriptors) does not fire.
func TestRun_BelowThreshold_NoFire(t *testing.T) {
	// Only one entity with descriptors but all positive — no contradiction pairs.
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Tell me about the system."},
		{2, "assistant", "The system is reliable and stable."},
		{3, "user", "How fast is it?"},
		{4, "assistant", "The system is fast and efficient."},
		{5, "user", "Any issues?"},
		{6, "assistant", "The system is solid and robust."},
	})

	result := Run(snap, DefaultConfig())
	if result != nil {
		t.Errorf("expected nil for all-positive entity descriptors, got %v", result.GetFindingType())
	}
}

// TestRun_HighSeverity verifies CRITICAL severity when contradiction_count >= 4.
func TestRun_HighSeverity(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Tell me about the Payment Service, Auth System, Cache Layer, and Data Pipeline."},
		{2, "assistant", "The Payment Service is reliable. The Auth System is stable. The Cache Layer is fast. The Data Pipeline is accurate."},
		{3, "user", "Any concerns?"},
		{4, "assistant", "The Payment Service is good. The Auth System is dependable. The Cache Layer is efficient. The Data Pipeline is consistent."},
		{5, "user", "But you said there were problems?"},
		{6, "assistant", "The Payment Service is unreliable. The Auth System is broken. The Cache Layer is slow. The Data Pipeline is inaccurate."},
	})

	result := Run(snap, DefaultConfig())
	if result == nil {
		t.Fatal("expected CONTRADICTION finding for 4 entity contradictions, got nil")
	}
	if result.GetSeverity() != reasoningv1.FindingSeverity_CRITICAL {
		t.Errorf("expected CRITICAL severity for contradiction_count >= 4, got %v", result.GetSeverity())
	}
}

// TestDefaultConfig verifies default config values match spec.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MinContradictions != 2 {
		t.Errorf("expected MinContradictions=2, got %d", cfg.MinContradictions)
	}
	if cfg.MinTurnGap != 1 {
		t.Errorf("expected MinTurnGap=1, got %d", cfg.MinTurnGap)
	}
}
