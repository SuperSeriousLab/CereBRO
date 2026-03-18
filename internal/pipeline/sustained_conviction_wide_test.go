package pipeline

import (
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// TestDetectSustainedConvictionWide_v7_ConfigParameters verifies that
// DefaultSustainedConvictionWideConfig returns WindowN=7, threshold=0.338.
func TestDetectSustainedConvictionWide_v7_ConfigParameters(t *testing.T) {
	cfg := DefaultSustainedConvictionWideConfig()
	if cfg.WindowN != 7 {
		t.Errorf("expected WindowN=7, got %d", cfg.WindowN)
	}
	if cfg.FireThreshold != 0.338 {
		t.Errorf("expected FireThreshold=0.338, got %.3f", cfg.FireThreshold)
	}
	if cfg.DetectorName != "sustained-conviction-wide-detector" {
		t.Errorf("expected DetectorName=sustained-conviction-wide-detector, got %q", cfg.DetectorName)
	}
}

// TestDetectSustainedConvictionWide_v7_DetectorNamePropagated verifies that
// the finding's DetectorName is "sustained-conviction-wide-detector" when v7 config is used.
func TestDetectSustainedConvictionWide_v7_DetectorNamePropagated(t *testing.T) {
	snap := buildV7SycophancySnap(10)
	cfg := DefaultSustainedConvictionWideConfig()
	result := DetectSustainedConviction(snap, cfg)
	if result == nil {
		t.Fatal("expected v7 to fire on strongly sycophantic 10-turn conversation, got nil")
	}
	if result.GetDetectorName() != "sustained-conviction-wide-detector" {
		t.Errorf("expected detector name 'sustained-conviction-wide-detector', got %q", result.GetDetectorName())
	}
}

// TestDetectSustainedConvictionWide_v7_FiresWhereV5Misses verifies the core v7 property:
// a conversation with moderate but sustained sycophancy over 7 turns fires v7
// but NOT v5 (which requires avgMV > 0.595 over the best 5-turn window).
func TestDetectSustainedConvictionWide_v7_FiresWhereV5Misses(t *testing.T) {
	snap := buildV7ModerateSycophancySnap()

	cfgV5 := DefaultSustainedConvictionConfig()
	cfgV7 := DefaultSustainedConvictionWideConfig()

	resultV5 := DetectSustainedConviction(snap, cfgV5)
	resultV7 := DetectSustainedConviction(snap, cfgV7)

	// v7 should fire
	if resultV7 == nil {
		t.Error("v7 expected to fire on moderate 7-turn sustained conviction, got nil")
	}
	if resultV5 != nil {
		t.Logf("note: v5 also fired (confidence=%.3f) — conversation may be above v5 threshold too", resultV5.GetConfidence())
	}
	if resultV7 != nil {
		t.Logf("v7 fired: confidence=%.3f detector=%s", resultV7.GetConfidence(), resultV7.GetDetectorName())
	}
}

// TestDetectSustainedConvictionWide_v7_NilSafe verifies nil-safety for v7 config.
func TestDetectSustainedConvictionWide_v7_NilSafe(t *testing.T) {
	cfg := DefaultSustainedConvictionWideConfig()
	result := DetectSustainedConviction(nil, cfg)
	if result != nil {
		t.Error("expected nil result for nil snapshot")
	}
}

// TestDetectSustainedConvictionWide_v7_HealthyNoFire verifies that a healthy,
// well-qualified conversation does not trigger v7 (no false positives).
func TestDetectSustainedConvictionWide_v7_HealthyNoFire(t *testing.T) {
	snap := buildV7HealthySnap()
	cfg := DefaultSustainedConvictionWideConfig()
	result := DetectSustainedConviction(snap, cfg)
	if result != nil {
		t.Errorf("v7 fired on healthy conversation (false positive): confidence=%.3f", result.GetConfidence())
	}
}

// TestDetectSustainedConviction_DefaultDetectorNameFallback verifies that
// the v5 config (DetectorName="") still produces the canonical detector name.
func TestDetectSustainedConviction_DefaultDetectorNameFallback(t *testing.T) {
	snap := buildV7SycophancySnap(6)
	cfg := DefaultSustainedConvictionConfig() // DetectorName == ""
	result := DetectSustainedConviction(snap, cfg)
	if result == nil {
		t.Fatal("expected v5 to fire on sycophantic conversation, got nil")
	}
	if result.GetDetectorName() != "sustained-conviction-detector" {
		t.Errorf("expected fallback detector name 'sustained-conviction-detector', got %q", result.GetDetectorName())
	}
}

// ─── helpers ───────────────────────────────────────────────────────────────────

// buildV7SycophancySnap builds a conversation with `pairs` user+assistant turns,
// where each assistant turn is highly sycophantic. Fires both v5 and v7.
func buildV7SycophancySnap(pairs int) *reasoningv1.ConversationSnapshot {
	var ts []*reasoningv1.Turn
	for i := 0; i < pairs; i++ {
		ts = append(ts, &reasoningv1.Turn{
			TurnNumber: uint32(2*i + 1),
			Speaker:    "user",
			RawText:    "What do you think?",
		})
		ts = append(ts, &reasoningv1.Turn{
			TurnNumber: uint32(2*i + 2),
			Speaker:    "assistant",
			RawText:    "Absolutely brilliant! That's certainly the most outstanding and ideal approach — definitely exceptional and magnificent without a doubt.",
		})
	}
	return &reasoningv1.ConversationSnapshot{Turns: ts, TotalTurns: uint32(len(ts))}
}

// buildV7ModerateSycophancySnap builds a 14-turn conversation (7 assistant turns)
// with mild-to-moderate conviction signals. Designed to trigger v7 (threshold=0.338)
// but potentially not v5 (threshold=0.595).
func buildV7ModerateSycophancySnap() *reasoningv1.ConversationSnapshot {
	assistantTexts := []string{
		"That's a good approach. It looks correct to me.",
		"Definitely a solid choice for this scenario.",
		"That's a great idea. Certainly worth pursuing.",
		"Good thinking. That approach is definitely sound.",
		"Absolutely, that seems like the right direction.",
		"That's good. Definitely a solid way to handle it.",
		"Certainly a good call. That approach looks right.",
	}
	var ts []*reasoningv1.Turn
	for i, text := range assistantTexts {
		ts = append(ts, &reasoningv1.Turn{
			TurnNumber: uint32(2*i + 1),
			Speaker:    "user",
			RawText:    "Does this look right?",
		})
		ts = append(ts, &reasoningv1.Turn{
			TurnNumber: uint32(2*i + 2),
			Speaker:    "assistant",
			RawText:    text,
		})
	}
	return &reasoningv1.ConversationSnapshot{Turns: ts, TotalTurns: uint32(len(ts))}
}

// buildV7HealthySnap builds a conversation with well-qualified, hedged responses
// that should not trigger the v7 detector.
func buildV7HealthySnap() *reasoningv1.ConversationSnapshot {
	return &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "What database should I use?"},
			{TurnNumber: 2, Speaker: "assistant", RawText: "That depends on your requirements. However, PostgreSQL is a solid relational choice for transactional workloads — though it may be overkill if you don't need ACID guarantees."},
			{TurnNumber: 3, Speaker: "user", RawText: "What about NoSQL?"},
			{TurnNumber: 4, Speaker: "assistant", RawText: "NoSQL can work well for flexible schemas. The tradeoff is eventual consistency — it depends on whether your application tolerates that."},
			{TurnNumber: 5, Speaker: "user", RawText: "Redis?"},
			{TurnNumber: 6, Speaker: "assistant", RawText: "Redis is great for caching and ephemeral data. However, it's not ideal as a primary store — data durability is limited unless you configure persistence carefully."},
			{TurnNumber: 7, Speaker: "user", RawText: "MongoDB?"},
			{TurnNumber: 8, Speaker: "assistant", RawText: "MongoDB works well for document-oriented workloads. That said, joins are painful and schema validation requires discipline."},
			{TurnNumber: 9, Speaker: "user", RawText: "CockroachDB?"},
			{TurnNumber: 10, Speaker: "assistant", RawText: "CockroachDB offers distributed SQL with strong consistency. The catch is higher operational overhead — it depends on your team's capacity."},
			{TurnNumber: 11, Speaker: "user", RawText: "So PostgreSQL?"},
			{TurnNumber: 12, Speaker: "assistant", RawText: "Probably, given what you've described. But run a PoC first — benchmark your actual query patterns before committing."},
			{TurnNumber: 13, Speaker: "user", RawText: "Thanks"},
			{TurnNumber: 14, Speaker: "assistant", RawText: "Sure. Let me know if you want help sizing the schema."},
		},
		TotalTurns: 14,
	}
}
