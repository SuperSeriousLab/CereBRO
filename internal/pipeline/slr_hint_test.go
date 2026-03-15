// Copyright 2025 SuperSeriousLab
// Licensed under the Apache License, Version 2.0

package pipeline

import (
	"strconv"
	"strings"
	"testing"
)

// parseHintScore extracts the numeric score from an "auto:cx=<f>" string.
func parseHintScore(t *testing.T, hint string) float64 {
	t.Helper()
	const prefix = "auto:cx="
	if !strings.HasPrefix(hint, prefix) {
		t.Fatalf("hint %q does not start with %q", hint, prefix)
	}
	score, err := strconv.ParseFloat(hint[len(prefix):], 64)
	if err != nil {
		t.Fatalf("cannot parse score from hint %q: %v", hint, err)
	}
	return score
}

func TestFormatSLRModelHint_HighComplexity(t *testing.T) {
	// High urgency + high complexity → composite > 0.7
	hint := FormatSLRModelHint(0.9, 0.9, 0.8)
	score := parseHintScore(t, hint)
	if score <= 0.7 {
		t.Errorf("high urgency+complexity: expected score > 0.7, got %.4f (hint=%q)", score, hint)
	}
}

func TestFormatSLRModelHint_LowComplexity(t *testing.T) {
	// Low everything → composite < 0.3
	hint := FormatSLRModelHint(0.1, 0.1, 0.1)
	score := parseHintScore(t, hint)
	if score >= 0.3 {
		t.Errorf("low everything: expected score < 0.3, got %.4f (hint=%q)", score, hint)
	}
}

func TestFormatSLRModelHint_MidRange(t *testing.T) {
	// Mixed values → composite in [0.3, 0.7]
	// 0.4*0.5 + 0.4*0.5 + 0.2*0.5 = 0.5
	hint := FormatSLRModelHint(0.5, 0.5, 0.5)
	score := parseHintScore(t, hint)
	if score < 0.3 || score > 0.7 {
		t.Errorf("mixed (0.5, 0.5, 0.5): expected score in [0.3, 0.7], got %.4f (hint=%q)", score, hint)
	}
}

func TestFormatSLRModelHint_Format(t *testing.T) {
	hint := FormatSLRModelHint(0.5, 0.5, 0.5)
	if !strings.HasPrefix(hint, "auto:cx=") {
		t.Errorf("hint must start with 'auto:cx=', got %q", hint)
	}
}

func TestFormatSLRModelHint_Clamp(t *testing.T) {
	// Values that would exceed 1.0 should clamp at 1.0.
	hint := FormatSLRModelHint(1.0, 1.0, 1.0)
	score := parseHintScore(t, hint)
	if score > 1.0 {
		t.Errorf("expected score clamped at 1.0, got %.4f", score)
	}

	// Zero inputs should produce exactly 0.00.
	hint = FormatSLRModelHint(0, 0, 0)
	score = parseHintScore(t, hint)
	if score != 0 {
		t.Errorf("expected score 0.00 for all-zero inputs, got %.4f", score)
	}
}
