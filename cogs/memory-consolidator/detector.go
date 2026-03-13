// Package main implements the Memory Consolidator COG.
//
// PURE deterministic — no LLM calls. Creates sparse index entries from pipeline
// results and appends them to the Forge corpus in NDJSON format for the
// Lamarckian learning loop.
// Phase 5, Layer 5 (Memory Consolidation).
//
// The implementation delegates to internal/pipeline.Consolidator.
package main

import (
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// Config holds the Memory Consolidator's tunable parameters.
type Config = pipeline.ConsolidatorConfig

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return pipeline.DefaultConsolidatorConfig()
}

// RunConsolidator runs the memory consolidator on pipeline results.
func RunConsolidator(input *pipeline.ConsolidationInput, consolidator *pipeline.Consolidator) *pipeline.ConsolidationResult {
	return consolidator.Consolidate(input)
}

// SubmitFeedback submits user feedback for a conversation.
func SubmitFeedback(consolidator *pipeline.Consolidator, conversationID, outcome string) error {
	return consolidator.SubmitFeedback(conversationID, outcome)
}
