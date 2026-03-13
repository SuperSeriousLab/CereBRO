package main

import (
	"unicode/utf8"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// Config holds the format validator's parameters.
type Config struct {
	MaxInputBytes uint32
	MaxTurns      uint32
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		MaxInputBytes: 1048576, // 1MB
		MaxTurns:      500,
	}
}

// Validate checks a ConversationSnapshot for structural validity.
// Returns a ValidationResult — valid=true if all checks pass.
//
// Note: The proto was designed for a future where Layer 0 sits at the network
// boundary accepting raw bytes. For now, it validates already-parsed input.
func Validate(snap *reasoningv1.ConversationSnapshot, cfg Config) *cerebrov1.ValidationResult {
	if snap == nil {
		return &cerebrov1.ValidationResult{
			Valid:           false,
			RejectionReason: "nil snapshot",
		}
	}

	// Check turn count.
	if len(snap.GetTurns()) == 0 {
		return &cerebrov1.ValidationResult{
			Valid:           false,
			RejectionReason: "no turns in snapshot",
		}
	}

	if uint32(len(snap.GetTurns())) > cfg.MaxTurns {
		return &cerebrov1.ValidationResult{
			Valid:           false,
			RejectionReason: "turn count exceeds max_turns limit",
			InputSizeBytes:  estimateSize(snap),
		}
	}

	// Check total size of all turn text.
	totalBytes := estimateSize(snap)
	if totalBytes > cfg.MaxInputBytes {
		return &cerebrov1.ValidationResult{
			Valid:           false,
			RejectionReason: "input size exceeds max_input_bytes limit",
			InputSizeBytes:  totalBytes,
		}
	}

	// Check UTF-8 validity of all turn text.
	for _, turn := range snap.GetTurns() {
		if !utf8.ValidString(turn.GetRawText()) {
			return &cerebrov1.ValidationResult{
				Valid:           false,
				RejectionReason: "invalid UTF-8 in turn text",
				InputSizeBytes:  totalBytes,
			}
		}
		if !utf8.ValidString(turn.GetSpeaker()) {
			return &cerebrov1.ValidationResult{
				Valid:           false,
				RejectionReason: "invalid UTF-8 in speaker field",
				InputSizeBytes:  totalBytes,
			}
		}
	}

	// Check objective field UTF-8.
	if !utf8.ValidString(snap.GetObjective()) {
		return &cerebrov1.ValidationResult{
			Valid:           false,
			RejectionReason: "invalid UTF-8 in objective field",
			InputSizeBytes:  totalBytes,
		}
	}

	return &cerebrov1.ValidationResult{
		Valid:          true,
		InputSizeBytes: totalBytes,
	}
}

// estimateSize returns the approximate byte size of the snapshot's text content.
func estimateSize(snap *reasoningv1.ConversationSnapshot) uint32 {
	var total int
	total += len(snap.GetObjective())
	for _, turn := range snap.GetTurns() {
		total += len(turn.GetRawText())
		total += len(turn.GetSpeaker())
	}
	return uint32(total)
}
