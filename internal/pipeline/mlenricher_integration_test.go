//go:build integration

package pipeline

import (
	"net/http"
	"testing"
	"time"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func TestEnrichML_Integration_RealOllama(t *testing.T) {
	cfg := DefaultMLEnricherConfig()
	cfg.Enabled = true
	cfg.TimeoutPerTurn = 120 * time.Second // real LLM on local hardware needs generous timeout

	// Verify Ollama is reachable.
	resp, err := http.Get(cfg.OllamaURL + "/api/tags")
	if err != nil {
		t.Skipf("Ollama not reachable at %s: %v", cfg.OllamaURL, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("Ollama returned %d", resp.StatusCode)
	}

	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{
				TurnNumber: 1,
				Speaker:    "user",
				RawText:    "We've already invested $200,000 in this React migration. I'm absolutely sure we should continue with it rather than switching to Vue.",
			},
			{
				TurnNumber: 2,
				Speaker:    "assistant",
				RawText:    "I understand the investment concern. However, the $200,000 already spent is a sunk cost. The question should be: which framework will serve us better going forward?",
			},
			{
				TurnNumber: 3,
				Speaker:    "user",
				RawText:    "You're right, but we can't just waste all that work. Let's keep going with React. I definitely think it's the better choice.",
			},
		},
		Objective: "Evaluate frontend framework decision",
	}

	result := EnrichML(snap, cfg, http.DefaultClient)
	if len(result) == 0 {
		t.Fatal("expected at least one enrichment from real Ollama")
	}

	t.Logf("Got %d enrichments from Ollama", len(result))
	for _, e := range result {
		t.Logf("Turn %d: %d claims, %d anchors, %d sunk-cost phrases, formality=%v",
			e.GetSourceTurn(),
			len(e.GetClaims()),
			len(e.GetAnchoringReferences()),
			len(e.GetSunkCostPhrases()),
			e.GetFormality(),
		)
	}

	// Verify merged view
	merged := MergeMLEnrichments(result)
	if merged == nil {
		t.Fatal("expected non-nil merged enrichment")
	}
	t.Logf("Merged: %d claims, %d anchors, %d sunk-cost, %d confidence markers",
		len(merged.GetClaims()),
		len(merged.GetAnchoringReferences()),
		len(merged.GetSunkCostPhrases()),
		len(merged.GetConfidenceMarkers()),
	)
}

func TestPipeline_MLEnriched_Integration(t *testing.T) {
	cfg := MLEnrichedConfig()
	cfg.MLEnricher.TimeoutPerTurn = 120 * time.Second

	// Verify Ollama is reachable.
	resp, err := http.Get(cfg.MLEnricher.OllamaURL + "/api/tags")
	if err != nil {
		t.Skipf("Ollama not reachable: %v", err)
	}
	resp.Body.Close()

	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "We've already spent $200,000 on React. We should definitely keep going."},
			{TurnNumber: 2, Speaker: "user", RawText: "I'm absolutely sure this is the right choice. Let's continue."},
		},
		Objective: "Evaluate framework decision",
	}

	result := Run(snap, cfg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	t.Logf("Pipeline result: %d findings, %d ML enrichments, rejected=%v",
		len(result.Findings), len(result.MLEnrichments), result.Rejected)

	if len(result.MLEnrichments) == 0 {
		t.Error("expected ML enrichments in pipeline result")
	}
}
