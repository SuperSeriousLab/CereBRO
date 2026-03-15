package pipeline

// DomainContext carries optional domain hints from an upstream fact-grounding
// service (e.g. Sophrim). It is purely advisory: when nil or below the
// confidence threshold, all detectors run with defaults (no regression).
type DomainContext struct {
	PrimaryDomain string  // "philosophy", "cognitive-analysis", "technical", "security", …
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

// applyDomainContext returns a PipelineConfig adjusted for the DomainContext
// stored in cfg.DomainContext. When DomainContext is nil, or confidence is low,
// or TextEra is not "classical", the config is returned unchanged (zero regression).
//
// Classical adjustments (TextEra == "classical" && Confidence > 0.6):
//   - ScopeGuard.DriftThreshold = 0.70  (default 0.79)
//     Classical philosophical vocabulary is richer and more varied per turn —
//     inflected forms, formal registers, and rhetorical repetition all reduce
//     Jaccard distances between on-topic turns. Lowering the threshold to 0.70
//     correctly identifies genuine scope shifts (e.g. Thrasymachus's late-dialogue
//     shift from justice-definition to political advantage) while staying above
//     the within-topic vocabulary variation of philosophical dialogue.
//   - ScopeGuard.SustainedTurns = 3  (default 8)
//     Classical corpus entries are segmented into ~10-turn windows. With a
//     reference window of 4 turns, only ~6 turns are evaluated per entry.
//     SustainedTurns=8 (tuned for 15-turn modern conversations) can never fire
//     on 10-turn segments — mathematically impossible. Lowering to 3 preserves
//     the "sustained drift" discriminator while matching the segment length.
//     Three consecutive drifting turns is still a strong signal; single-turn
//     vocabulary excursions (which are normal in classical dialogue) are not flagged.
//   - Calibrator.MinCertaintyWords = 8  (default 5)
//     Classical discourse particles are longer.
//   - Anchoring detector: removed from detector map via SkipAnchoring flag.
//     Cannot detect conceptual anchoring — only numeric anchoring.
//   - SunkCost detector: already has classical markers; just ensure activated
//     (no config change needed — activation is driven by the router).
func applyDomainContext(cfg PipelineConfig) PipelineConfig {
	if !cfg.DomainContext.isClassical() {
		return cfg
	}

	// ScopeGuard: lower drift threshold for richer classical vocabulary.
	// Classical texts have higher per-turn vocabulary diversity than modern
	// conversational text, so the same Jaccard threshold would never fire.
	cfg.ScopeGuard.DriftThreshold = 0.70

	// ScopeGuard: lower sustained-turns requirement to match segment length.
	// Classical corpus entries are ~10 turns; default=8 cannot fire on these.
	// Three consecutive drifting turns is a strong enough sustained signal.
	cfg.ScopeGuard.SustainedTurns = 3

	// Calibrator: require longer turns before CERTAIN-level markers are counted.
	cfg.Calibrator.MinCertaintyWords = 8

	// Anchoring: skip entirely — classical texts lack numeric anchoring patterns.
	cfg.SkipAnchoring = true

	return cfg
}
