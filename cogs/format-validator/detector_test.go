package main

import (
	"strings"
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

func TestValidate_ValidSnapshot(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Hello world"},
		},
		TotalTurns: 1,
	}
	result := Validate(snap, DefaultConfig())
	if !result.GetValid() {
		t.Errorf("expected valid, got rejection: %s", result.GetRejectionReason())
	}
}

func TestValidate_NilSnapshot(t *testing.T) {
	result := Validate(nil, DefaultConfig())
	if result.GetValid() {
		t.Error("expected invalid for nil snapshot")
	}
	if result.GetRejectionReason() != "nil snapshot" {
		t.Errorf("unexpected reason: %s", result.GetRejectionReason())
	}
}

func TestValidate_EmptyTurns(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{TotalTurns: 0}
	result := Validate(snap, DefaultConfig())
	if result.GetValid() {
		t.Error("expected invalid for empty turns")
	}
}

func TestValidate_InvalidUTF8(t *testing.T) {
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "Hello \xff world"},
		},
		TotalTurns: 1,
	}
	result := Validate(snap, DefaultConfig())
	if result.GetValid() {
		t.Error("expected invalid for bad UTF-8")
	}
	if result.GetRejectionReason() != "invalid UTF-8 in turn text" {
		t.Errorf("unexpected reason: %s", result.GetRejectionReason())
	}
}

func TestValidate_OversizedInput(t *testing.T) {
	cfg := Config{MaxInputBytes: 100, MaxTurns: 500}
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: strings.Repeat("x", 200)},
		},
		TotalTurns: 1,
	}
	result := Validate(snap, cfg)
	if result.GetValid() {
		t.Error("expected invalid for oversized input")
	}
}

func TestValidate_ExactlyAtSizeLimit(t *testing.T) {
	text := strings.Repeat("x", 90)
	cfg := Config{MaxInputBytes: 100, MaxTurns: 500}
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: text},
		},
		TotalTurns: 1,
	}
	result := Validate(snap, cfg)
	if !result.GetValid() {
		t.Errorf("expected valid at size limit, got: %s", result.GetRejectionReason())
	}
}

func TestValidate_JustOverSizeLimit(t *testing.T) {
	text := strings.Repeat("x", 98)
	cfg := Config{MaxInputBytes: 100, MaxTurns: 500}
	snap := &reasoningv1.ConversationSnapshot{
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: text},
		},
		TotalTurns: 1,
	}
	result := Validate(snap, cfg)
	if result.GetValid() {
		t.Error("expected invalid just over size limit")
	}
}

func TestValidate_TooManyTurns(t *testing.T) {
	cfg := Config{MaxInputBytes: 1048576, MaxTurns: 5}
	turns := make([]*reasoningv1.Turn, 10)
	for i := range turns {
		turns[i] = &reasoningv1.Turn{TurnNumber: uint32(i + 1), Speaker: "user", RawText: "hi"}
	}
	snap := &reasoningv1.ConversationSnapshot{Turns: turns, TotalTurns: 10}
	result := Validate(snap, cfg)
	if result.GetValid() {
		t.Error("expected invalid for too many turns")
	}
}
