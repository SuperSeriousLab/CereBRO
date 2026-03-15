package pipeline

import (
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// ============================================================
// Helper — makeIPSnap builds a ConversationSnapshot from raw turns.
// Note: makeTurn is defined in conceptual_anchoring_test.go.
// makeSnap is already declared in inhibitor_test.go with a different signature.
// ============================================================

func makeIPSnap(turns []*reasoningv1.Turn) *reasoningv1.ConversationSnapshot {
	return &reasoningv1.ConversationSnapshot{
		Turns:      turns,
		TotalTurns: uint32(len(turns)),
	}
}

// ============================================================
// Test 1: Polemarchus dialogue — DETECTED
// Polemarchus defends "Simonides said X" with no independent argument.
// ============================================================

func TestDetectInheritedPosition_Polemarchus_Detected(t *testing.T) {
	turns := []*reasoningv1.Turn{
		makeTurn(1, "socrates", "What do you say justice is, Polemarchus?"),
		makeTurn(2, "polemarchus", "As Simonides said, justice is giving every man what is owed to him."),
		makeTurn(3, "socrates", "But what does Simonides mean by what is owed?"),
		makeTurn(4, "polemarchus", "Simonides was a wise man and he clearly meant to give friends what is good."),
		makeTurn(5, "socrates", "And what of enemies — what do we owe them?"),
		makeTurn(6, "polemarchus", "Simonides surely meant that enemies should receive harm, for that is their due."),
		makeTurn(7, "socrates", "Is it just for the just man to harm enemies?"),
		makeTurn(8, "polemarchus", "As Simonides argued, the just man helps friends and harms enemies."),
		makeTurn(9, "socrates", "But harming anyone makes them worse — can that be just?"),
		makeTurn(10, "polemarchus", "That is what Simonides maintained, and he was wiser than we are."),
	}

	snap := makeIPSnap(turns)
	cfg := DefaultInheritedPositionConfig()
	// Default MinCitations=3, should fire easily with 4+ Simonides citations

	result := DetectInheritedPosition(snap, cfg)

	if result == nil {
		t.Fatal("expected INHERITED_POSITION finding for Polemarchus dialogue, got nil")
	}
	if result.GetFindingType() != reasoningv1.FindingType_INHERITED_POSITION {
		t.Errorf("finding type: got %v, want INHERITED_POSITION", result.GetFindingType())
	}
	if result.GetDetectorName() != "inherited-position-detector" {
		t.Errorf("detector name: got %q, want %q", result.GetDetectorName(), "inherited-position-detector")
	}
	detail := result.GetInheritedPosition()
	if detail == nil {
		t.Fatal("InheritedPositionDetail should be populated")
	}
	if detail.GetAuthorityCitationCount() < 3 {
		t.Errorf("authority_citation_count: got %d, want >= 3", detail.GetAuthorityCitationCount())
	}
	if detail.GetIndependentJustificationPresent() {
		t.Error("independent_justification_present should be false — Polemarchus never argues on merit")
	}
	if len(detail.GetCitationTurns()) < 3 {
		t.Errorf("citation_turns: got %d entries, want >= 3", len(detail.GetCitationTurns()))
	}
	if detail.GetDefendedClaim() == "" {
		t.Error("defended_claim should be populated")
	}
	if result.GetConfidence() <= 0 {
		t.Error("confidence should be positive")
	}
}

// ============================================================
// Test 2: Thrasymachus dialogue — NOT detected
// Thrasymachus argues on his own authority — no "X said..." citations.
// ============================================================

func TestDetectInheritedPosition_Thrasymachus_NotDetected(t *testing.T) {
	turns := []*reasoningv1.Turn{
		makeTurn(1, "thrasymachus", "Justice is the advantage of the stronger. I tell you that plainly."),
		makeTurn(2, "socrates", "Can you explain what you mean by that?"),
		makeTurn(3, "thrasymachus", "The stronger party sets the rules, and those rules are called justice."),
		makeTurn(4, "socrates", "But what if the ruler makes a mistake about what benefits him?"),
		makeTurn(5, "thrasymachus", "A ruler who makes mistakes is not a ruler in the true sense. Rulers rule correctly by definition."),
		makeTurn(6, "socrates", "So the physician's art serves the patient, not the physician?"),
		makeTurn(7, "thrasymachus", "Every art and science serves the interest of the one who practices it, not of the subject."),
		makeTurn(8, "socrates", "Does the shepherd's art serve the shepherd or the sheep?"),
		makeTurn(9, "thrasymachus", "The shepherd serves himself — he fattens the flock for profit, not for the flock's sake."),
		makeTurn(10, "socrates", "Then justice in your view serves only the stronger party exclusively?"),
	}

	snap := makeIPSnap(turns)
	cfg := DefaultInheritedPositionConfig()

	result := DetectInheritedPosition(snap, cfg)

	if result != nil {
		t.Errorf("expected nil for Thrasymachus (argues own authority, no citations), got: %v", result.GetExplanation())
	}
}

// ============================================================
// Test 3: Modern proper citations with reasoning — NOT detected
// Einstein cited, followed by substantive derivation (> justificationMinWords).
// ============================================================

func TestDetectInheritedPosition_ProperCitations_NotDetected(t *testing.T) {
	turns := []*reasoningv1.Turn{
		makeTurn(1, "alice", "As Einstein argued, the photoelectric effect demonstrates that light comes in quanta."),
		makeTurn(2, "alice", "The reason is that classical wave theory predicts a continuous energy transfer, but experiment shows discrete electron ejections only above a threshold frequency, which implies quantised energy packets."),
		makeTurn(3, "bob", "How does this connect to the broader quantum theory?"),
		makeTurn(4, "alice", "As Planck held that energy is emitted in discrete units, because the black-body radiation curves fit E=nhf exactly and no continuous model matches the data across all frequencies."),
		makeTurn(5, "bob", "So both lines of evidence converge?"),
		makeTurn(6, "alice", "According to Bohr, the atomic model requires quantised electron orbits, since the emission spectrum shows discrete spectral lines whose frequencies match the orbital energy differences precisely."),
		makeTurn(7, "bob", "That is compelling evidence from three independent phenomena."),
	}

	snap := makeIPSnap(turns)
	cfg := DefaultInheritedPositionConfig()

	result := DetectInheritedPosition(snap, cfg)

	// Each citation is followed by substantive justification (> 10 words with merit markers).
	// Even if citation count >= 3, unjustified_ratio should be < meritThreshold.
	if result != nil {
		t.Errorf("expected nil for proper academic citations with independent reasoning, got: %v", result.GetExplanation())
	}
}

// ============================================================
// Test 4: Fewer than MinCitations — NOT detected
// ============================================================

func TestDetectInheritedPosition_BelowMinCitations_NotDetected(t *testing.T) {
	turns := []*reasoningv1.Turn{
		makeTurn(1, "user", "As Aristotle said, virtue is a mean between extremes."),
		makeTurn(2, "assistant", "Can you say more about what that means in practice?"),
		makeTurn(3, "user", "It means that courage is between cowardice and recklessness."),
		makeTurn(4, "assistant", "What about justice — where does that fall?"),
		makeTurn(5, "user", "Justice is giving each their due, according to circumstance."),
		makeTurn(6, "assistant", "That seems reasonable. What is the role of habit?"),
	}

	snap := makeIPSnap(turns)
	cfg := DefaultInheritedPositionConfig()
	// Only 1 authority citation — below MinCitations=3

	result := DetectInheritedPosition(snap, cfg)

	if result != nil {
		t.Errorf("expected nil for < MinCitations, got: %v", result.GetExplanation())
	}
}

// ============================================================
// Test 5: Below MinCitations in a short conversation — NOT detected
// Two authority citations is below the default threshold of 3.
// ============================================================

func TestDetectInheritedPosition_ShortConversation_NotDetected(t *testing.T) {
	turns := []*reasoningv1.Turn{
		makeTurn(1, "user", "As Simonides said, justice is giving what is owed."),
		makeTurn(2, "assistant", "Interesting — who was Simonides?"),
		makeTurn(3, "user", "He was a famous Greek poet who wrote about ethics."),
		makeTurn(4, "assistant", "What else did he say about justice?"),
	}

	snap := makeIPSnap(turns)
	cfg := DefaultInheritedPositionConfig()
	// Only 1 citation — below MinCitations=3

	result := DetectInheritedPosition(snap, cfg)

	if result != nil {
		t.Errorf("expected nil for conversation with only 1 citation (< MinCitations=3), got: %v", result.GetExplanation())
	}
}

// ============================================================
// Test 6: Institutional tradition deference — DETECTED
// "We have always done it this way" repeated without justification.
// ============================================================

func TestDetectInheritedPosition_InstitutionalDeference_Detected(t *testing.T) {
	turns := []*reasoningv1.Turn{
		makeTurn(1, "manager", "We have always done the annual review in December."),
		makeTurn(2, "employee", "But could we move it to July to align with fiscal planning?"),
		makeTurn(3, "manager", "We have always done it in December — it is our tradition."),
		makeTurn(4, "employee", "What are the reasons for December specifically?"),
		makeTurn(5, "manager", "It has always been December, and we should not change that."),
		makeTurn(6, "employee", "Many companies find mid-year reviews more effective."),
		makeTurn(7, "manager", "We have always used December and that is how it is done here."),
		makeTurn(8, "employee", "Can we at least run a pilot with one team?"),
		makeTurn(9, "manager", "No — we have always done December and it has always been this way."),
		makeTurn(10, "employee", "I understand the tradition, but is there evidence it works better?"),
	}

	snap := makeIPSnap(turns)
	cfg := DefaultInheritedPositionConfig()

	result := DetectInheritedPosition(snap, cfg)

	if result == nil {
		t.Fatal("expected INHERITED_POSITION finding for institutional deference, got nil")
	}
	if result.GetFindingType() != reasoningv1.FindingType_INHERITED_POSITION {
		t.Errorf("finding type: got %v, want INHERITED_POSITION", result.GetFindingType())
	}
	detail := result.GetInheritedPosition()
	if detail == nil {
		t.Fatal("InheritedPositionDetail should be populated")
	}
	if detail.GetAuthorityCitationCount() < 3 {
		t.Errorf("authority_citation_count: got %d, want >= 3", detail.GetAuthorityCitationCount())
	}
}

// ============================================================
// Test 7: Nil snapshot — not panics, returns nil
// ============================================================

func TestDetectInheritedPosition_NilSnap_ReturnsNil(t *testing.T) {
	cfg := DefaultInheritedPositionConfig()
	result := DetectInheritedPosition(nil, cfg)
	if result != nil {
		t.Errorf("expected nil for nil snapshot, got: %v", result)
	}
}

// ============================================================
// Test 8: Default config values are correct
// ============================================================

func TestDefaultInheritedPositionConfig(t *testing.T) {
	cfg := DefaultInheritedPositionConfig()
	if cfg.MinCitations != 3 {
		t.Errorf("MinCitations: got %d, want 3", cfg.MinCitations)
	}
	if cfg.MeritRatio != 0.3 {
		t.Errorf("MeritRatio: got %.2f, want 0.3", cfg.MeritRatio)
	}
	if cfg.CitationWindowTurns != 5 {
		t.Errorf("CitationWindowTurns: got %d, want 5", cfg.CitationWindowTurns)
	}
}

// ============================================================
// Test 9: Router activates inherited-position when authority patterns present
// ============================================================

func TestRouter_InheritedPositionActivated(t *testing.T) {
	turns := []*reasoningv1.Turn{
		makeTurn(1, "user", "As Simonides said, justice is giving what is owed."),
		makeTurn(2, "assistant", "What did Simonides mean by that?"),
		makeTurn(3, "user", "According to Simonides, giving friends good and enemies harm."),
		makeTurn(4, "assistant", "Is harming enemies really just?"),
		makeTurn(5, "user", "Simonides maintained that is indeed what justice requires."),
	}
	snap := makeIPSnap(turns)
	cfg := DefaultRouterConfig()

	routing := Route(snap, cfg)

	found := false
	for _, d := range routing.Activated {
		if d == DetectorInheritedPosition {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected inherited-position-detector to be activated for conversation with authority citations")
	}
}

// ============================================================
// Test 10: Router does NOT activate inherited-position for < 4 turns
// ============================================================

func TestRouter_InheritedPositionNotActivated_TooShort(t *testing.T) {
	turns := []*reasoningv1.Turn{
		makeTurn(1, "user", "As Simonides said, justice is giving what is owed."),
		makeTurn(2, "assistant", "Interesting claim."),
		makeTurn(3, "user", "Simonides was wise about these matters."),
	}
	snap := makeIPSnap(turns)
	cfg := DefaultRouterConfig()

	routing := Route(snap, cfg)

	for _, d := range routing.Activated {
		if d == DetectorInheritedPosition {
			t.Error("inherited-position-detector should NOT activate for < 4 turn conversations")
			break
		}
	}
}

// ============================================================
// Test 11: Detector is registered in buildDetectorMap
// ============================================================

func TestBuildDetectorMap_InheritedPositionRegistered(t *testing.T) {
	cfg := DefaultPipelineConfig()
	detectors := buildDetectorMap(cfg)

	if _, ok := detectors[DetectorInheritedPosition]; !ok {
		t.Error("inherited-position-detector should be registered in the detector map")
	}
}

// ============================================================
// Test 12: hasIndependentJustification distinguishes merit from bare assertion
// ============================================================

func TestHasIndependentJustification_MeritPresent(t *testing.T) {
	text := "As Simonides said, justice is giving what is owed, because the evidence from his other writings shows he meant proportional return based on what each person has contributed to society."
	if !hasIndependentJustification(text, 10) {
		t.Error("expected merit justification detected — 'because' followed by substantive clause")
	}
}

func TestHasIndependentJustification_BareAssertion(t *testing.T) {
	text := "As Simonides said, justice is giving what is owed. Obviously that is correct."
	if hasIndependentJustification(text, 10) {
		t.Error("expected no merit justification — 'obviously' is a no-merit indicator")
	}
}

func TestHasIndependentJustification_TooShortClause(t *testing.T) {
	text := "As Simonides argued, because he was wise."
	if hasIndependentJustification(text, 10) {
		t.Error("expected no merit justification — clause after 'because' is too short")
	}
}

// ============================================================
// Test 13: findAuthorityCitation matches expected patterns
// ============================================================

func TestFindAuthorityCitation_MatchesPhrases(t *testing.T) {
	cases := []struct {
		text    string
		wantHit bool
		desc    string
	}{
		{"As Simonides said, justice is giving what is owed.", true, "as Simonides said"},
		{"According to Aristotle, virtue is a mean.", true, "according to"},
		{"Homer said that the gods reward the brave.", true, "homer said"},
		{"Tradition holds that we do this in winter.", true, "tradition holds"},
		{"We have always done it this way.", true, "we have always"},
		{"I think justice is fairness.", false, "no authority citation"},
		{"The weather is nice today.", false, "no citation at all"},
		{"Einstein believed that the universe is deterministic.", true, "believed that pattern"},
	}
	for _, tc := range cases {
		got := findAuthorityCitation(tc.text)
		if tc.wantHit && got == "" {
			t.Errorf("%s: expected citation match for %q, got empty", tc.desc, tc.text)
		}
		if !tc.wantHit && got != "" {
			t.Errorf("%s: expected no citation for %q, got %q", tc.desc, tc.text, got)
		}
	}
}
