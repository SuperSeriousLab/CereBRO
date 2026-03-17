package adversarial

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// ─── Unit tests ─────────────────────────────────────────────────────────────

// TestMutation verifies that mutate changes at least one field with rate=1.0.
func TestMutation(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	original := ConversationTemplate{
		Topic:     "technology",
		Formality: 0.5,
		TurnCount: 8,
		Speakers:  []string{"alice", "bob"},
		FailureModes: []FailureSpec{
			{Type: "ANCHORING_BIAS", Severity: 0.5, OnsetTurn: 3, Duration: 2, Technique: "keyword"},
			{Type: "SUNK_COST_FALLACY", Severity: 0.7, OnsetTurn: 5, Duration: 2, Technique: "structural"},
		},
	}

	// Rate 1.0 means every mutation operator fires.
	mutated := mutate(original, 1.0, rng)

	// At rate=1 with two failure modes, something should have changed.
	// Either severity, onset, technique, topic, formality, or a mode was added/removed.
	changed := mutated.Topic != original.Topic ||
		mutated.Formality != original.Formality ||
		len(mutated.FailureModes) != len(original.FailureModes)

	if !changed {
		// Check inner fields.
		for i, f := range mutated.FailureModes {
			if i >= len(original.FailureModes) {
				break
			}
			orig := original.FailureModes[i]
			if f.Severity != orig.Severity || f.OnsetTurn != orig.OnsetTurn || f.Technique != orig.Technique {
				changed = true
				break
			}
		}
	}

	if !changed {
		t.Error("mutate(rate=1.0) produced no observable change")
	}
}

// TestCrossover verifies that crossover produces a valid child from two parents.
func TestCrossover(t *testing.T) {
	rng := rand.New(rand.NewSource(7))

	parentA := ConversationTemplate{
		Topic:     "technology",
		Formality: 0.8,
		TurnCount: 6,
		Speakers:  []string{"alice", "bob"},
		FailureModes: []FailureSpec{
			{Type: "ANCHORING_BIAS", Severity: 0.5, OnsetTurn: 2, Duration: 2, Technique: "keyword"},
		},
	}
	parentB := ConversationTemplate{
		Topic:     "healthcare",
		Formality: 0.3,
		TurnCount: 8,
		Speakers:  []string{"doc", "patient"},
		FailureModes: []FailureSpec{
			{Type: "SUNK_COST_FALLACY", Severity: 0.9, OnsetTurn: 4, Duration: 3, Technique: "structural"},
			{Type: "CONTRADICTION", Severity: 0.4, OnsetTurn: 6, Duration: 1, Technique: "implicit"},
		},
	}

	child := crossover(parentA, parentB, rng)

	if len(child.FailureModes) == 0 {
		t.Error("crossover produced child with no failure modes")
	}
	if child.TurnCount != parentA.TurnCount {
		t.Errorf("crossover changed TurnCount: got %d, want %d", child.TurnCount, parentA.TurnCount)
	}

	// Topic should come from one of the parents.
	if child.Topic != parentA.Topic && child.Topic != parentB.Topic {
		t.Errorf("crossover produced unknown topic %q", child.Topic)
	}
}

// TestFitnessZeroOnNil verifies that nil conversions score 0.
func TestFitnessZeroOnNil(t *testing.T) {
	generateFn := func(tmpl ConversationTemplate) *reasoningv1.ConversationSnapshot {
		return nil // simulate generation failure
	}
	runFn := func(snap *reasoningv1.ConversationSnapshot) *pipeline.PipelineResult {
		t.Error("runFn should not be called when generate returns nil")
		return nil
	}

	tmpl := ConversationTemplate{
		Topic:        "technology",
		FailureModes: []FailureSpec{{Type: "ANCHORING_BIAS", Severity: 0.5}},
	}

	score := adversarialFitness(tmpl, generateFn, runFn)
	if score != 0.0 {
		t.Errorf("expected fitness=0.0 on nil snapshot, got %.3f", score)
	}
}

// TestFitnessWithMockPipeline verifies scoring logic with known pipeline results.
func TestFitnessWithMockPipeline(t *testing.T) {
	// Template expects ANCHORING_BIAS.
	tmpl := ConversationTemplate{
		Topic:     "business",
		Formality: 0.5,
		TurnCount: 5,
		Speakers:  []string{"alice", "bob"},
		FailureModes: []FailureSpec{
			{Type: "ANCHORING_BIAS", Severity: 0.7, OnsetTurn: 2, Duration: 2, Technique: "keyword"},
		},
		CleanSections: []TurnRange{{Start: 1, End: 1}},
	}

	mockSnap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "alice", RawText: "hello"},
			{TurnNumber: 2, Speaker: "bob", RawText: "world"},
		},
	}

	generateFn := func(t ConversationTemplate) *reasoningv1.ConversationSnapshot {
		return mockSnap
	}

	t.Run("missed_expected_finding_scores_positively", func(t *testing.T) {
		// Pipeline detects NOTHING → false negative on ANCHORING_BIAS → score > 0
		runFn := func(snap *reasoningv1.ConversationSnapshot) *pipeline.PipelineResult {
			return &pipeline.PipelineResult{Findings: nil}
		}
		score := adversarialFitness(tmpl, generateFn, runFn)
		if score <= 0.0 {
			t.Errorf("expected score > 0 for missed finding, got %.3f", score)
		}
	})

	t.Run("detected_expected_finding_scores_zero_fn_component", func(t *testing.T) {
		// Pipeline correctly detects ANCHORING_BIAS → no false negative reward
		runFn := func(snap *reasoningv1.ConversationSnapshot) *pipeline.PipelineResult {
			return &pipeline.PipelineResult{
				Findings: []*reasoningv1.CognitiveAssessment{
					{
						FindingType:   reasoningv1.FindingType_ANCHORING_BIAS,
						Confidence:    0.8,
						RelevantTurns: []uint32{2},
					},
				},
			}
		}
		// No FN, no borderline, no FP-in-clean → score/maxScore = 0/0.3 = 0.0
		// But borderline check: conf=0.8 is NOT borderline so no borderline reward either.
		score := adversarialFitness(tmpl, generateFn, runFn)
		// score should be 0 (no adversarial interest when correctly detected)
		if score != 0.0 {
			t.Errorf("expected score=0.0 when finding correctly detected at high confidence, got %.3f", score)
		}
	})

	t.Run("borderline_finding_scores_positive", func(t *testing.T) {
		// Pipeline detects ANCHORING_BIAS but with borderline confidence.
		runFn := func(snap *reasoningv1.ConversationSnapshot) *pipeline.PipelineResult {
			return &pipeline.PipelineResult{
				Findings: []*reasoningv1.CognitiveAssessment{
					{
						FindingType:   reasoningv1.FindingType_ANCHORING_BIAS,
						Confidence:    0.5, // borderline
						RelevantTurns: []uint32{2},
					},
				},
			}
		}
		score := adversarialFitness(tmpl, generateFn, runFn)
		// Borderline reward applies → score > 0
		if score <= 0.0 {
			t.Errorf("expected score > 0 for borderline confidence, got %.3f", score)
		}
	})

	t.Run("fp_in_clean_section_scores_positive", func(t *testing.T) {
		// Pipeline raises SUNK_COST in turn 1 which is in the clean section.
		runFn := func(snap *reasoningv1.ConversationSnapshot) *pipeline.PipelineResult {
			return &pipeline.PipelineResult{
				Findings: []*reasoningv1.CognitiveAssessment{
					{
						FindingType:   reasoningv1.FindingType_SUNK_COST_FALLACY,
						Confidence:    0.7,
						RelevantTurns: []uint32{1}, // in clean section [1,1]
					},
				},
			}
		}
		score := adversarialFitness(tmpl, generateFn, runFn)
		// FN reward + FP-in-clean reward should both apply.
		if score <= 0.0 {
			t.Errorf("expected score > 0 for FP in clean section, got %.3f", score)
		}
	})
}

// TestRandomTemplatesCount verifies randomTemplates returns n templates.
func TestRandomTemplatesCount(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	templates := randomTemplates(5, rng)
	if len(templates) != 5 {
		t.Errorf("randomTemplates(5) returned %d templates", len(templates))
	}
	for i, tmpl := range templates {
		if len(tmpl.FailureModes) == 0 {
			t.Errorf("template[%d] has no failure modes", i)
		}
		if tmpl.TurnCount < 5 {
			t.Errorf("template[%d] has too few turns: %d", i, tmpl.TurnCount)
		}
		if tmpl.Topic == "" {
			t.Errorf("template[%d] has empty topic", i)
		}
	}
}

// TestTournamentSelect verifies tournament selection returns a valid individual.
func TestTournamentSelect(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	pop := []Individual{
		{Template: ConversationTemplate{Topic: "a"}, Fitness: 0.1},
		{Template: ConversationTemplate{Topic: "b"}, Fitness: 0.5},
		{Template: ConversationTemplate{Topic: "c"}, Fitness: 0.9},
	}

	// With tournament size = all 3, the best (0.9) should often win.
	wins := 0
	for i := 0; i < 100; i++ {
		sel := tournamentSelect(pop, 3, rng)
		if sel.Fitness == 0.9 {
			wins++
		}
	}
	if wins < 60 {
		t.Errorf("best individual won only %d/100 tournaments with size 3", wins)
	}
}

// TestVerifyAndPatchSunkCost verifies that sunk-cost phrases are injected when missing.
func TestVerifyAndPatchSunkCost(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "alice", RawText: "Let's think about this project carefully."},
			{TurnNumber: 2, Speaker: "bob", RawText: "I agree, what are our options?"},
			{TurnNumber: 3, Speaker: "alice", RawText: "We could go left or right."},
		},
	}

	tmpl := ConversationTemplate{
		FailureModes: []FailureSpec{
			{Type: "SUNK_COST_FALLACY", OnsetTurn: 1, Duration: 2},
		},
	}

	patched := patchSunkCost(snap, tmpl.FailureModes[0])
	hasCost, hasCont := hasSunkCostPhrases(patched)
	if !hasCost {
		t.Error("patchSunkCost did not inject a cost phrase")
	}
	if !hasCont {
		t.Error("patchSunkCost did not inject a continuation phrase")
	}
}

// TestVerifyAndPatchAnchoring verifies that a numeric token is injected when missing.
func TestVerifyAndPatchAnchoring(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "alice", RawText: "We should plan this out carefully."},
			{TurnNumber: 2, Speaker: "bob", RawText: "Agreed, sounds good."},
		},
	}
	f := FailureSpec{Type: "ANCHORING_BIAS", OnsetTurn: 1}
	patched := patchAnchoring(snap, f)
	if !hasNumericTokens(patched) {
		t.Error("patchAnchoring did not inject numeric tokens")
	}
}

// ─── Integration smoke test ─────────────────────────────────────────────────

// TestOneGenerationSmoke runs a single generation with population=3 using a
// mock Ollama server. No real Ollama calls are made.
func TestOneGenerationSmoke(t *testing.T) {
	// Build a mock Ollama server that returns a minimal valid conversation.
	mockConversation := []generatedTurn{
		{TurnNumber: 1, Speaker: "alice", Text: "The initial budget was $100,000 for this project. We've already invested six months of work into it."},
		{TurnNumber: 2, Speaker: "bob", Text: "That sounds about right, I'd estimate around $95,000 to $105,000. We might as well continue at this point."},
		{TurnNumber: 3, Speaker: "alice", Text: "Let's go with $98,000 as our working number. We've already spent so much, can't stop now."},
		{TurnNumber: 4, Speaker: "bob", Text: "Agreed. We should continue and see this through."},
		{TurnNumber: 5, Speaker: "alice", Text: "Good. Budget confirmed at around $100,000."},
	}

	mockBody, _ := json.Marshal(mockConversation)
	mockResponse := map[string]interface{}{
		"message": map[string]string{
			"role":    "assistant",
			"content": string(mockBody),
		},
	}
	mockResponseBody, _ := json.Marshal(mockResponse)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(mockResponseBody)
	}))
	defer server.Close()

	ollamaCfg := OllamaConfig{
		URL:     server.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	}

	evoCfg := EvolutionConfig{
		PopSize:     3,
		Generations: 1,
		EliteCount:  1,
		TournSize:   2,
		MutateRate:  0.3,
		Seed:        42,
	}

	pipelineCfg := pipeline.DefaultPipelineConfig()

	result := RunEvolution(evoCfg, ollamaCfg, pipelineCfg, server.Client())

	if result == nil {
		t.Fatal("RunEvolution returned nil")
	}
	if len(result.FinalPopulation) != evoCfg.PopSize {
		t.Errorf("expected %d individuals, got %d", evoCfg.PopSize, len(result.FinalPopulation))
	}
	if len(result.BestFitness) != evoCfg.Generations {
		t.Errorf("expected %d best-fitness entries, got %d", evoCfg.Generations, len(result.BestFitness))
	}

	t.Logf("Smoke test best fitness: %.3f", result.BestFitness[0])
	t.Logf("Smoke test mean fitness: %.3f", result.MeanFitness[0])
}

// ─── Full evolution test (gated by -short) ──────────────────────────────────

// TestFullEvolution runs the full evolutionary loop against a mock Ollama server.
// This test is skipped under -short; use -run TestFullEvolution -timeout 30m for real runs.
func TestFullEvolution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping full evolution under -short")
	}

	mockConversation := []generatedTurn{
		{TurnNumber: 1, Speaker: "alice", Text: "The project estimate is $200,000. We've already invested three months into this approach."},
		{TurnNumber: 2, Speaker: "bob", Text: "I was thinking around $190,000 to $210,000. At this point we might as well continue."},
		{TurnNumber: 3, Speaker: "alice", Text: "Let's target $195,000. We can't stop now after all the investment."},
		{TurnNumber: 4, Speaker: "bob", Text: "I think we should not go with this vendor at all. Actually I've always preferred option B."},
		{TurnNumber: 5, Speaker: "alice", Text: "I'm 100% certain this will work perfectly. Let's go with it. We should continue with this."},
		{TurnNumber: 6, Speaker: "bob", Text: "Agreed. Let's go with option A for the implementation framework."},
		{TurnNumber: 7, Speaker: "alice", Text: "Actually let's go with option B for the framework. No reason needed."},
		{TurnNumber: 8, Speaker: "bob", Text: "We've already spent too much time here. Need to finish this."},
	}

	mockBody, _ := json.Marshal(mockConversation)
	mockResponse := map[string]interface{}{
		"message": map[string]string{
			"role":    "assistant",
			"content": string(mockBody),
		},
	}
	mockResponseBody, _ := json.Marshal(mockResponse)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(mockResponseBody)
	}))
	defer server.Close()

	ollamaCfg := OllamaConfig{
		URL:     server.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	}

	evoCfg := DefaultEvolutionConfig()
	evoCfg.Seed = 12345

	pipelineCfg := pipeline.DefaultPipelineConfig()

	result := RunEvolution(evoCfg, ollamaCfg, pipelineCfg, server.Client())

	if result == nil {
		t.Fatal("RunEvolution returned nil")
	}
	if len(result.FinalPopulation) == 0 {
		t.Fatal("empty final population")
	}

	t.Logf("Full evolution complete: %d generations, best final fitness=%.3f",
		evoCfg.Generations, result.FinalPopulation[0].Fitness)

	// Export top-5 entries.
	top5 := TopN(result, 5)
	generateFn := defaultGenerateFunc(ollamaCfg, server.Client())
	entries := ExportCorpusEntries(top5, generateFn, "adv")
	t.Logf("Exported %d corpus entries", len(entries))
}
