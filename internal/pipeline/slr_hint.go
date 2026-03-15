// Copyright 2025 SuperSeriousLab
// Licensed under the Apache License, Version 2.0

// Package pipeline contains the CereBRO five-layer cognitive pipeline.
package pipeline

import "fmt"

// FormatSLRModelHint formats a GainSignal's complexity as an SLR model hint
// for use in eidos-llm Complete() calls.
//
// The returned string uses the "auto:cx=<value>" format recognised by SLR's
// gateway, e.g. "auto:cx=0.72". SLR will use the hint to select the cheapest
// viable LLM tier, but the hint is advisory — SLR may fall back if the
// preferred tier is unavailable.
//
// The composite complexity value is:
//
//	score = 0.4*urgency + 0.4*complexity + 0.2*formality
//
// All inputs are expected in the [0, 1] range. The output is clamped to [0, 1]
// before formatting.
func FormatSLRModelHint(urgency, complexity, formality float64) string {
	score := 0.4*urgency + 0.4*complexity + 0.2*formality
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return fmt.Sprintf("auto:cx=%.2f", score)
}
