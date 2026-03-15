package pipeline

import (
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// ============================================================
// Helpers
// ============================================================

// makeTurn creates a Turn with the given number, speaker, and text.
func makeTurn(num uint32, speaker, text string) *reasoningv1.Turn {
	return &reasoningv1.Turn{
		TurnNumber: num,
		Speaker:    speaker,
		RawText:    text,
	}
}

// makeConceptualSnap builds a ConversationSnapshot from raw text turns (not enriched).
func makeConceptualSnap(turns []*reasoningv1.Turn) *reasoningv1.ConversationSnapshot {
	return &reasoningv1.ConversationSnapshot{
		Turns:      turns,
		TotalTurns: uint32(len(turns)),
	}
}

// ============================================================
// isStrongDeclarative unit tests
// ============================================================

func TestIsStrongDeclarative_DetectsDeclaratives(t *testing.T) {
	cases := []struct {
		text string
		want bool
		desc string
	}{
		{
			"Justice is the advantage of the stronger.",
			true,
			"classic declarative with copula 'is'",
		},
		{
			"AI will replace all jobs in the economy.",
			true,
			"declarative with 'will'",
		},
		{
			"The ruler must always serve the interests of the state.",
			true,
			"declarative with 'must'",
		},
		{
			"Maybe justice is the advantage of the stronger.",
			false,
			"hedged with 'maybe'",
		},
		{
			"Is justice the advantage of the stronger?",
			false,
			"question not declarative",
		},
		{
			"I think justice matters.",
			false,
			"hedged with 'I think'",
		},
		{
			"Hi.",
			false,
			"too short",
		},
		{
			"Perhaps we could consider a different approach here.",
			false,
			"hedged with 'perhaps'",
		},
	}
	for _, tc := range cases {
		got := isStrongDeclarative(tc.text)
		if got != tc.want {
			t.Errorf("%s: isStrongDeclarative(%q) = %v, want %v", tc.desc, tc.text, got, tc.want)
		}
	}
}

// ============================================================
// hasAcknowledgedCounter unit tests
// ============================================================

func TestHasAcknowledgedCounter_Detected(t *testing.T) {
	turns := []*reasoningv1.Turn{
		makeTurn(1, "thrasymachus", "Justice is the advantage of the stronger."),
		makeTurn(2, "socrates", "But what if the ruler makes a mistake about what benefits him?"),
		makeTurn(3, "thrasymachus", "You're right, I should clarify that point."),
	}
	if !hasAcknowledgedCounter(turns, 1) {
		t.Error("expected counter acknowledgement to be detected")
	}
}

func TestHasAcknowledgedCounter_NotDetected_NoAck(t *testing.T) {
	turns := []*reasoningv1.Turn{
		makeTurn(1, "thrasymachus", "Justice is the advantage of the stronger."),
		makeTurn(2, "socrates", "But what about the weak?"),
		makeTurn(3, "thrasymachus", "The strong rule, that is natural."),
	}
	if hasAcknowledgedCounter(turns, 1) {
		t.Error("expected no counter acknowledgement")
	}
}

func TestHasAcknowledgedCounter_Reassertion_Ignored(t *testing.T) {
	turns := []*reasoningv1.Turn{
		makeTurn(1, "thrasymachus", "Justice is the advantage of the stronger."),
		makeTurn(2, "thrasymachus", "Actually, but still my main point stands regardless."),
	}
	if hasAcknowledgedCounter(turns, 1) {
		t.Error("immediate reassertion should negate acknowledgement")
	}
}

// ============================================================
// DetectConceptualAnchoring — Test 1: Thrasymachus (classical) — DETECTED
// ============================================================

func TestDetectConceptualAnchoring_Thrasymachus_Detected(t *testing.T) {
	// Thrasymachus anchors on "justice is the advantage of the stronger" then
	// defends it throughout. Socrates echoes the anchor terms in his questions
	// (faithful to Republic Book 1 where Socrates always restates the terms
	// under examination). No acknowledgement of counter-examples.
	turns := []*reasoningv1.Turn{
		// Anchor turn: strong declarative about justice
		makeTurn(1, "thrasymachus", "Justice is the advantage of the stronger."),
		// Socrates echoes "justice", "advantage", "stronger" in his counter-questions
		makeTurn(2, "socrates", "Is justice then purely the advantage of the stronger party?"),
		makeTurn(3, "thrasymachus", "The stronger always defines what justice is for the weaker."),
		makeTurn(4, "socrates", "But if justice is the advantage of the stronger, what if the stronger errs?"),
		makeTurn(5, "thrasymachus", "Justice serves the advantage of the stronger ruler in all cases."),
		makeTurn(6, "socrates", "The physician serves the patient — does justice serve the stronger advantage only?"),
		makeTurn(7, "thrasymachus", "The stronger defines what is just — that is the nature of justice."),
		makeTurn(8, "socrates", "Then justice as advantage for the stronger benefits only the rulers?"),
		makeTurn(9, "thrasymachus", "Exactly — justice is what benefits the stronger party."),
		makeTurn(10, "socrates", "Justice as the stronger advantage — what of the unjust who gains more?"),
		makeTurn(11, "thrasymachus", "The advantage of the stronger is what justice is — nothing else."),
		makeTurn(12, "socrates", "So justice and the advantage of the stronger are identical?"),
		makeTurn(13, "thrasymachus", "Justice is the advantage of the stronger, Socrates, plain and simple."),
		makeTurn(14, "socrates", "Then the just person with no advantage is not stronger?"),
		makeTurn(15, "thrasymachus", "Justice means the stronger takes advantage — always."),
	}

	snap := makeConceptualSnap(turns)
	cfg := DefaultConceptualAnchoringConfig()
	cfg.MinTurns = 8
	// Lower OrbitThreshold slightly to match realistic Jaccard in a 15-turn
	// dialogue where anchor keywords ("just","advantage","strong") appear in
	// most but not all turns.
	cfg.OrbitThreshold = 0.5

	result := DetectConceptualAnchoring(snap, cfg)

	if result == nil {
		t.Fatal("expected CONCEPTUAL_ANCHORING finding for Thrasymachus dialogue, got nil")
	}
	if result.GetFindingType() != reasoningv1.FindingType_CONCEPTUAL_ANCHORING {
		t.Errorf("finding type: got %v, want CONCEPTUAL_ANCHORING", result.GetFindingType())
	}
	if result.GetDetectorName() != "conceptual-anchoring-detector" {
		t.Errorf("detector name: got %q, want %q", result.GetDetectorName(), "conceptual-anchoring-detector")
	}
	detail := result.GetConceptualAnchoring()
	if detail == nil {
		t.Fatal("ConceptualAnchoringDetail should be populated")
	}
	if detail.GetAnchorTurn() == 0 {
		t.Error("AnchorTurn should be set")
	}
	if detail.GetSemanticOrbitRatio() < 0.5 {
		t.Errorf("orbit ratio %.2f should be >= 0.5 for Thrasymachus dialogue", detail.GetSemanticOrbitRatio())
	}
	if detail.GetCounterClaimsAcknowledged() {
		t.Error("CounterClaimsAcknowledged should be false — no acknowledgements in dialogue")
	}
	if result.GetConfidence() <= 0 {
		t.Error("confidence should be positive")
	}
}

// ============================================================
// Test 2: Short conversation — NOT detected (< MinTurns)
// ============================================================

func TestDetectConceptualAnchoring_TooShort_NotDetected(t *testing.T) {
	turns := []*reasoningv1.Turn{
		makeTurn(1, "user", "Justice is the advantage of the stronger."),
		makeTurn(2, "assistant", "That's an interesting claim."),
		makeTurn(3, "user", "Justice is always what the strong dictate."),
	}

	snap := makeConceptualSnap(turns)
	cfg := DefaultConceptualAnchoringConfig()
	// Default MinTurns = 8, conversation only has 3 turns

	result := DetectConceptualAnchoring(snap, cfg)

	if result != nil {
		t.Errorf("expected nil for short conversation (< MinTurns), got finding: %v", result.GetExplanation())
	}
}

// ============================================================
// Test 3: Healthy debate with acknowledged counter-claim — NOT detected
// ============================================================

func TestDetectConceptualAnchoring_HealthyDebate_NotDetected(t *testing.T) {
	// Speaker asserts claim then later acknowledges a counter-argument.
	turns := []*reasoningv1.Turn{
		makeTurn(1, "alice", "AI will replace all jobs in the near future."),
		makeTurn(2, "bob", "What about creative work that requires human judgment?"),
		makeTurn(3, "alice", "AI will replace jobs in manufacturing and service sectors."),
		makeTurn(4, "bob", "But doctors, teachers, and artists seem hard to replace."),
		makeTurn(5, "alice", "AI will handle routine medical diagnosis efficiently."),
		makeTurn(6, "bob", "Interpersonal care requires human empathy though."),
		makeTurn(7, "alice", "That's a fair point — I should refine my claim."),
		makeTurn(8, "alice", "AI will replace routine tasks, but not all human roles."),
		makeTurn(9, "bob", "That seems more accurate, creative work will persist."),
		makeTurn(10, "alice", "Yes, the augmentation model is more likely than full replacement."),
	}

	snap := makeConceptualSnap(turns)
	cfg := DefaultConceptualAnchoringConfig()
	cfg.MinTurns = 8

	result := DetectConceptualAnchoring(snap, cfg)

	if result != nil {
		t.Errorf("expected nil for debate where counter-claims are acknowledged, got: %v", result.GetExplanation())
	}
}

// ============================================================
// Test 4: Modern fixation — DETECTED
// ============================================================

func TestDetectConceptualAnchoring_ModernFixation_Detected(t *testing.T) {
	// Modern conversation where someone fixates on one claim without engaging alternatives.
	// The anchor uses compact vocabulary that echoes throughout the debate, as happens
	// in a sustained single-topic argument. The assistant also echoes the core terms
	// when posing counter-questions, as is natural in debate.
	turns := []*reasoningv1.Turn{
		makeTurn(1, "user", "Markets always allocate resources better than government planning."),
		makeTurn(2, "assistant", "Do markets always allocate resources efficiently for public goods?"),
		makeTurn(3, "user", "Markets allocate resources better than government planning in all sectors."),
		makeTurn(4, "assistant", "Healthcare markets allocate resources differently than consumer markets."),
		makeTurn(5, "user", "Markets always outperform government planning for resource allocation."),
		makeTurn(6, "assistant", "Environmental markets fail to allocate resources for pollution correctly."),
		makeTurn(7, "user", "Markets are always the superior mechanism for allocating resources."),
		makeTurn(8, "assistant", "Do markets always allocate public safety resources better than government?"),
		makeTurn(9, "user", "Government planning always fails — markets allocate better."),
		makeTurn(10, "assistant", "Markets allocate luxury goods well but fail for essential public resources."),
		makeTurn(11, "user", "Markets always allocate resources more efficiently than planning."),
		makeTurn(12, "assistant", "Government planning allocates resources for defense better than markets could."),
	}

	snap := makeConceptualSnap(turns)
	cfg := DefaultConceptualAnchoringConfig()
	cfg.MinTurns = 8
	cfg.OrbitThreshold = 0.5

	result := DetectConceptualAnchoring(snap, cfg)

	if result == nil {
		t.Fatal("expected CONCEPTUAL_ANCHORING finding for modern market fixation, got nil")
	}
	if result.GetFindingType() != reasoningv1.FindingType_CONCEPTUAL_ANCHORING {
		t.Errorf("finding type: got %v, want CONCEPTUAL_ANCHORING", result.GetFindingType())
	}
	if result.GetConceptualAnchoring().GetCounterClaimsAcknowledged() {
		t.Error("CounterClaimsAcknowledged should be false — no acknowledgements")
	}
}

// ============================================================
// Test 5: Normal evolving debate — NOT detected
// ============================================================

func TestDetectConceptualAnchoring_EvolvingDebate_NotDetected(t *testing.T) {
	// Conversation where the topic and positions genuinely evolve across turns.
	turns := []*reasoningv1.Turn{
		makeTurn(1, "alice", "We should adopt a remote work policy for all employees."),
		makeTurn(2, "bob", "Some roles require physical presence in the office."),
		makeTurn(3, "alice", "Hybrid models could work for those requiring presence."),
		makeTurn(4, "bob", "What about team collaboration and spontaneous meetings?"),
		makeTurn(5, "alice", "Digital tools can support collaboration effectively."),
		makeTurn(6, "bob", "Junior employees benefit more from in-person mentoring though."),
		makeTurn(7, "alice", "We could design onboarding programs that blend both approaches."),
		makeTurn(8, "bob", "That sounds reasonable — a tiered approach by role type."),
		makeTurn(9, "alice", "Exactly, with flexibility built in based on individual needs."),
		makeTurn(10, "bob", "We should trial this with one department first to measure outcomes."),
	}

	snap := makeConceptualSnap(turns)
	cfg := DefaultConceptualAnchoringConfig()
	cfg.MinTurns = 8

	result := DetectConceptualAnchoring(snap, cfg)

	if result != nil {
		t.Errorf("expected nil for evolving debate where topic shifts, got: %v", result.GetExplanation())
	}
}

// ============================================================
// Test 6: No anchor found (only hedged claims in early turns)
// ============================================================

func TestDetectConceptualAnchoring_NoAnchor_NotDetected(t *testing.T) {
	// All early turns are hedged — no strong declarative anchor.
	turns := []*reasoningv1.Turn{
		makeTurn(1, "alice", "Maybe justice is somehow related to what benefits society."),
		makeTurn(2, "bob", "I think that could be one interpretation of justice."),
		makeTurn(3, "alice", "Perhaps fairness is the key component here."),
		makeTurn(4, "bob", "I'm not sure about that definition."),
		makeTurn(5, "alice", "It might depend on the cultural context."),
		makeTurn(6, "bob", "Possibly, different societies have different conceptions."),
		makeTurn(7, "alice", "I think we need more philosophical grounding."),
		makeTurn(8, "bob", "Perhaps we should read more primary sources first."),
		makeTurn(9, "alice", "Could be useful to start with Rawls or Aristotle."),
		makeTurn(10, "bob", "I think Rawls might be more accessible for our purposes."),
	}

	snap := makeConceptualSnap(turns)
	cfg := DefaultConceptualAnchoringConfig()
	cfg.MinTurns = 8

	result := DetectConceptualAnchoring(snap, cfg)

	if result != nil {
		t.Errorf("expected nil when no strong declarative anchor exists, got: %v", result.GetExplanation())
	}
}

// ============================================================
// Test 7: Classical domain context — detector RUNS (not skipped)
// ============================================================

func TestDetectConceptualAnchoring_ClassicalDomain_NotSkipped(t *testing.T) {
	// When classical DomainContext is set, the NUMERIC anchoring detector is
	// skipped via SkipAnchoring, but conceptual-anchoring-detector must still run.
	cfg := DefaultPipelineConfig()
	cfg.DomainContext = &DomainContext{PrimaryDomain: "philosophy", TextEra: "classical", Confidence: 0.85}
	cfg = applyDomainContext(cfg)

	detectors := buildDetectorMap(cfg)

	if _, ok := detectors[DetectorConceptualAnchoring]; !ok {
		t.Error("conceptual-anchoring-detector should be in the detector map even for classical domain")
	}
	if _, ok := detectors[DetectorAnchoring]; ok {
		t.Error("numeric anchoring-detector should be absent for classical domain (SkipAnchoring=true)")
	}
}

// ============================================================
// Test 8: Detector config fields
// ============================================================

func TestDefaultConceptualAnchoringConfig(t *testing.T) {
	cfg := DefaultConceptualAnchoringConfig()
	if cfg.AnchorThreshold != 0.3 {
		t.Errorf("AnchorThreshold: got %.2f, want 0.3", cfg.AnchorThreshold)
	}
	if cfg.OrbitThreshold != 0.6 {
		t.Errorf("OrbitThreshold: got %.2f, want 0.6", cfg.OrbitThreshold)
	}
	if cfg.MinTurns != 8 {
		t.Errorf("MinTurns: got %d, want 8", cfg.MinTurns)
	}
	if cfg.MaxAnchorTurns != 3 {
		t.Errorf("MaxAnchorTurns: got %d, want 3", cfg.MaxAnchorTurns)
	}
}

// ============================================================
// Test 9: Cephalus brief dialogue — NOT detected
// (Short dialogue, no sustained orbit)
// ============================================================

func TestDetectConceptualAnchoring_Cephalus_NotDetected(t *testing.T) {
	// Cephalus gives a brief account of justice without sustained defence.
	// He leaves the dialogue early — no orbit to detect.
	turns := []*reasoningv1.Turn{
		makeTurn(1, "cephalus", "Old age brings peace when the strong passions subside."),
		makeTurn(2, "socrates", "Do you find that wealth helps with this, Cephalus?"),
		makeTurn(3, "cephalus", "Yes, wealth helps one avoid certain injustices."),
		makeTurn(4, "socrates", "What do you say justice is, Cephalus?"),
		makeTurn(5, "cephalus", "Telling the truth and paying one's debts, I suppose."),
		makeTurn(6, "socrates", "But what if returning a sword harms the owner?"),
		makeTurn(7, "cephalus", "I see — perhaps that definition needs refinement."),
	}

	snap := makeConceptualSnap(turns)
	cfg := DefaultConceptualAnchoringConfig()
	// MinTurns = 8, conversation has only 7 — should return nil

	result := DetectConceptualAnchoring(snap, cfg)

	if result != nil {
		t.Errorf("expected nil for short Cephalus dialogue, got: %v", result.GetExplanation())
	}
}

// ============================================================
// Test 10: Router activates conceptual anchoring when declarative present
// ============================================================

func TestRouter_ConceptualAnchoringActivated(t *testing.T) {
	turns := []*reasoningv1.Turn{
		makeTurn(1, "user", "Justice is the advantage of the stronger."),
		makeTurn(2, "assistant", "That is a strong claim."),
		makeTurn(3, "user", "Justice is always what benefits the rulers."),
		makeTurn(4, "assistant", "What about the interests of citizens?"),
		makeTurn(5, "user", "Justice serves the stronger party exclusively."),
	}
	snap := makeConceptualSnap(turns)
	cfg := DefaultRouterConfig()

	routing := Route(snap, cfg)

	found := false
	for _, d := range routing.Activated {
		if d == DetectorConceptualAnchoring {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected conceptual-anchoring-detector to be activated for conversation with declarative in early turns")
	}
}

// ============================================================
// Test 11: Router does NOT activate for short conversations
// ============================================================

func TestRouter_ConceptualAnchoringNotActivated_TooShort(t *testing.T) {
	turns := []*reasoningv1.Turn{
		makeTurn(1, "user", "Justice is the advantage of the stronger."),
		makeTurn(2, "assistant", "That's interesting."),
		makeTurn(3, "user", "I believe that strongly."),
	}
	snap := makeConceptualSnap(turns)
	cfg := DefaultRouterConfig()

	routing := Route(snap, cfg)

	for _, d := range routing.Activated {
		if d == DetectorConceptualAnchoring {
			t.Error("conceptual-anchoring-detector should NOT activate for < 4 turn conversations")
			break
		}
	}
}
