package adversarial

import (
	"net/http"
	"strings"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// ─── Pipeline runner ────────────────────────────────────────────────────────

// PipelineRunFunc is the function signature for running the pipeline.
// Allows test injection of mock runners.
type PipelineRunFunc func(snap *reasoningv1.ConversationSnapshot) *pipeline.PipelineResult

// GenerateFunc is the function signature for generating conversations.
// Allows test injection of mock generators.
type GenerateFunc func(tmpl ConversationTemplate) *reasoningv1.ConversationSnapshot

// pipelineRunner holds the generate + run functions used during fitness evaluation.
type pipelineRunner struct {
	generateFn PipelineRunFunc // NOT used directly here — see fitness func
	runFn      PipelineRunFunc
}

// defaultGenerateFunc wraps GenerateConversation for use in the runner.
func defaultGenerateFunc(ollamaCfg OllamaConfig, client *http.Client) GenerateFunc {
	return func(tmpl ConversationTemplate) *reasoningv1.ConversationSnapshot {
		return GenerateConversation(tmpl, ollamaCfg, client)
	}
}

// defaultRunFunc wraps pipeline.Run for use in the runner.
func defaultRunFunc(cfg pipeline.PipelineConfig) PipelineRunFunc {
	return func(snap *reasoningv1.ConversationSnapshot) *pipeline.PipelineResult {
		return pipeline.Run(snap, cfg)
	}
}

// ─── Adversarial fitness function (Deliverable 3) ──────────────────────────

// adversarialFitness generates a conversation from the template, runs it
// through the pipeline, and returns a [0, 1] fitness score.
//
// Scoring:
//   - False negative (missed real failure)  → +0.3 each
//   - False positive in clean section       → +0.2 each
//   - Borderline finding (confidence ~0.5)  → +0.1 each
//   - nil result (crash / parse failure)    → 0.0
func adversarialFitness(
	tmpl ConversationTemplate,
	generateFn GenerateFunc,
	runFn PipelineRunFunc,
) float64 {
	snap := generateFn(tmpl)
	if snap == nil {
		return 0.0
	}

	result := runFn(snap)
	if result == nil {
		return 0.0
	}

	// Build set of detected finding types.
	detected := make(map[string]bool)
	for _, f := range result.Findings {
		detected[pipeline.FindingTypeString(f.GetFindingType())] = true
	}

	// Expected finding types from the template.
	expected := make(map[string]bool)
	for _, f := range tmpl.FailureModes {
		expected[templateTypeToProto(f.Type)] = true
	}

	var score float64
	var maxScore float64

	// False negatives: expected but not detected → adversarially interesting.
	for et := range expected {
		maxScore += 0.3
		if !detected[et] {
			score += 0.3
		}
	}

	// Evaluate each detected finding.
	for _, finding := range result.Findings {
		ft := pipeline.FindingTypeString(finding.GetFindingType())

		// False positive in a clean section.
		if !expected[ft] {
			if isFindingInCleanSection(finding, tmpl.CleanSections) {
				maxScore += 0.2
				score += 0.2
			}
		}

		// Borderline finding: confidence near 0.5 (±0.15).
		conf := finding.GetConfidence()
		if conf >= 0.35 && conf <= 0.65 {
			maxScore += 0.1
			score += 0.1
		}
	}

	if maxScore == 0 {
		return 0.0
	}
	return score / maxScore
}

// isFindingInCleanSection returns true if any relevant turn of the finding
// falls within one of the template's clean sections.
func isFindingInCleanSection(finding *reasoningv1.CognitiveAssessment, clean []TurnRange) bool {
	for _, turn := range finding.GetRelevantTurns() {
		for _, cs := range clean {
			if int(turn) >= cs.Start && int(turn) <= cs.End {
				return true
			}
		}
	}
	return false
}

// templateTypeToProto converts template failure type names to the strings
// returned by pipeline.FindingTypeString (proto enum .String() values).
func templateTypeToProto(t string) string {
	return strings.ToUpper(t)
}
