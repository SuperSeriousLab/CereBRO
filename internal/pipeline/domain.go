package pipeline

// DomainContext carries optional domain hints from an upstream fact-grounding
// service (e.g. Sophrim). It is purely advisory: when nil or below the
// confidence threshold, all detectors run with defaults (no regression).
type DomainContext struct {
	PrimaryDomain string  // "philosophy", "cognitive-analysis", "technical", "code-review", "security", …
	TextEra       string  // "classical", "modern", ""
	Confidence    float64 // 0.0–1.0
}

const domainConfidenceThreshold = 0.6

// isClassical returns true when the context signals classical-era text with
// sufficient confidence.
func (dc *DomainContext) isClassical() bool {
	if dc == nil {
		return false
	}
	return dc.TextEra == "classical" && dc.Confidence > domainConfidenceThreshold
}

// isCodeReview returns true when the context signals a code review conversation
// (PR reviews, architecture discussions, implementation critique) with sufficient
// confidence.
//
// Code review conversations have distinct linguistic properties vs general modern
// conversations:
//   - Dense technical vocabulary (function names, type names, identifiers) that
//     overlaps heavily across turns even when the topic changes (e.g. refactoring
//     discussion → naming discussion share the same identifier tokens).
//   - Short, directive turns ("rename this", "extract a method") that carry little
//     topical signal for Jaccard-based scope detection.
//   - High occurrence of numeric literals (line numbers, loop counts, constants)
//     that are NOT anchoring-bias signals — they are incidental code references.
//   - Structural rather than evidential reasoning: reviewers assert facts ("this
//     will cause a data race") rather than offering accumulated evidence chains.
//
// Domain signal: PrimaryDomain == "code-review" with Confidence > 0.6.
func (dc *DomainContext) isCodeReview() bool {
	if dc == nil {
		return false
	}
	return dc.PrimaryDomain == "code-review" && dc.Confidence > domainConfidenceThreshold
}

// applyDomainContext returns a PipelineConfig adjusted for the DomainContext
// stored in cfg.DomainContext. When DomainContext is nil, or confidence is low,
// the config is returned unchanged (zero regression).
//
// # Classical adjustments (TextEra == "classical" && Confidence > 0.6)
//
//   - ScopeGuard.DriftThreshold = 0.70  (default 0.79)
//     Classical philosophical vocabulary is richer and more varied per turn —
//     inflected forms, formal registers, and rhetorical repetition all reduce
//     Jaccard distances between on-topic turns. Lowering the threshold to 0.70
//     correctly identifies genuine scope shifts (e.g. Thrasymachus's late-dialogue
//     shift from justice-definition to political advantage) while staying above
//     the within-topic vocabulary variation of philosophical dialogue.
//   - ScopeGuard.SustainedTurns = 4  (default 8; Forge Cycle 1 winner on full corpus)
//     Classical corpus entries are segmented into ~10-turn windows. With a
//     reference window of 4 turns, only ~6 turns are evaluated per entry.
//     SustainedTurns=8 (tuned for 15-turn modern conversations) can never fire
//     on 10-turn segments — mathematically impossible. Forge Cycle 1 found 4 optimal
//     on the full corpus (improves classical recall without harming modern F1).
//     Four consecutive drifting turns is a strong sustained signal; single-turn
//     vocabulary excursions (which are normal in classical dialogue) are not flagged.
//   - ConceptualAnchoring.AnchorThreshold = 0.35  (default 0.30; Forge Cycle 1 winner)
//     Raising the Jaccard overlap threshold to 0.35 reduces false positives on
//     classical philosophical restatements while preserving genuine anchoring signals.
//   - Calibrator.MinCertaintyWords = 8  (default 5)
//     Classical discourse particles are longer.
//   - Anchoring detector: removed from detector map via SkipAnchoring flag.
//     Cannot detect conceptual anchoring — only numeric anchoring.
//   - SunkCost detector: already has classical markers; just ensure activated
//     (no config change needed — activation is driven by the router).
//
// # Code Review adjustments (PrimaryDomain == "code-review" && Confidence > 0.6)
//
//   - ScopeGuard.DriftThreshold = 0.85  (default 0.79)
//     Code review turns share dense identifier vocabulary across topic shifts.
//     Raising the threshold prevents false scope-drift alerts when discussion
//     moves between aspects of the same diff (e.g. correctness → naming → style)
//     which all reference the same code symbols.
//   - ScopeGuard.SustainedTurns = 3  (default 8)
//     Code review conversations tend to be shorter and more directive. Requiring
//     only 3 consecutive drifting turns (vs 8 in general modern) ensures that
//     genuine off-topic tangents are still caught without needing a full 8-turn
//     drift run that rarely occurs in a focused PR review.
//   - Anchoring detector: disabled via SkipAnchoring flag.
//     Numeric literals in code (line numbers, loop bounds, magic constants) are
//     NOT anchoring-bias signals. The anchoring detector would fire continuously
//     on code review conversations due to incidental numeric co-occurrence.
//   - Calibrator.MinCertaintyWords = 3  (default 5)
//     Code review assertions are terse ("this will deadlock", "O(n^2) here").
//     Shorter turns still carry genuine certainty signals.
//
// NOTE: Code review and classical adjustments are mutually exclusive.
// Classical check runs first; if classical fires, code-review branch is skipped.
func applyDomainContext(cfg PipelineConfig) PipelineConfig {
	if cfg.DomainContext.isClassical() {
		// ScopeGuard: lower drift threshold for richer classical vocabulary.
		// Classical texts have higher per-turn vocabulary diversity than modern
		// conversational text, so the same Jaccard threshold would never fire.
		cfg.ScopeGuard.DriftThreshold = 0.70

		// ScopeGuard: lower sustained-turns requirement to match segment length.
		// Classical corpus entries are ~10 turns; default=8 cannot fire on these.
		// Forge Cycle 1 found 4 optimal on full corpus (classical recall improvement).
		cfg.ScopeGuard.SustainedTurns = 4

		// ConceptualAnchoring: raise overlap threshold for classical philosophical
		// restatements. Forge Cycle 1 winner: 0.35 reduces FP on within-topic repetition.
		cfg.ConceptualAnchoring.AnchorThreshold = 0.35

		// Calibrator: require longer turns before CERTAIN-level markers are counted.
		cfg.Calibrator.MinCertaintyWords = 8

		// Anchoring: skip entirely — classical texts lack numeric anchoring patterns.
		cfg.SkipAnchoring = true

		return cfg
	}

	if cfg.DomainContext.isCodeReview() {
		// ScopeGuard: raise drift threshold — code identifiers pollute Jaccard distance.
		// PR reviews discussing correctness, naming, and style all share the same
		// symbol vocabulary; default 0.79 would produce near-constant false drift alerts.
		cfg.ScopeGuard.DriftThreshold = 0.85

		// ScopeGuard: lower sustained-turns requirement for shorter, directive reviews.
		// A 3-turn drift window catches genuine off-topic tangents without requiring
		// the 8-turn run that never occurs in focused code review exchanges.
		cfg.ScopeGuard.SustainedTurns = 3

		// Anchoring: skip — numeric literals in code are not anchoring-bias signals.
		// Line numbers, loop bounds, and magic constants would otherwise trigger
		// the detector continuously on any code review conversation.
		cfg.SkipAnchoring = true

		// Calibrator: lower minimum words — code review assertions are terse.
		// "This will deadlock" and "O(n^2) here" are genuine high-certainty claims
		// that default MinCertaintyWords=5 would miss in short turns.
		cfg.Calibrator.MinCertaintyWords = 3

		return cfg
	}

	return cfg
}
