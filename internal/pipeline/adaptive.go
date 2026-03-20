// Domain-adaptive pipeline variant selection.
//
// RunAdaptive selects the optimal pipeline variant based on domain context:
//   - Classical text (era="classical", confidence>0.6) → E-pre-cortex (higher recall)
//   - Everything else → D-inhibitor-only (higher precision, lower latency)
//
// This is the primary entry point for production use when the caller has
// upstream domain signals (e.g. from Sophrim). The clean interface —
// DomainContext in, PipelineResult out — lets any system that can produce a
// DomainContext plug into domain-adaptive behaviour without knowing the
// pipeline's internal architecture.
package pipeline

import (
	"errors"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// RunAdaptive selects the optimal pipeline variant based on domain context.
//
// Classical text (era="classical", confidence>0.6) → E-pre-cortex (higher recall).
// Code review (PrimaryDomain="code-review", confidence>0.6) → D-inhibitor-only with
// code-review detector adjustments (raised scope-drift threshold, SkipAnchoring,
// lower MinCertaintyWords).
// Everything else → D-inhibitor-only with default thresholds.
//
// Domain selection logic:
//   - nil domain                              → D-inhibitor-only (safe default)
//   - modern era                              → D-inhibitor-only
//   - classical, low confidence (≤0.6)        → D-inhibitor-only
//   - classical, confidence > 0.6             → E-pre-cortex
//   - code-review, low confidence (≤0.6)      → D-inhibitor-only
//   - code-review, confidence > 0.6           → D-inhibitor-only + code-review adjustments
//
// The DomainContext is applied on top of the selected config so that
// domain-specific detector threshold adjustments are always in effect.
//
// ptsEndpoint (optional) enables fire-and-forget PTS anomaly signals. Pass ""
// to disable. Mirrors the PTSEndpoint field on PipelineConfig.
// store (optional variadic) enables outcome recording for TP/FP tracking.
func RunAdaptive(snap *reasoningv1.ConversationSnapshot, domain *DomainContext, ptsEndpoint string, store ...*OutcomeStore) (*PipelineResult, error) {
	if snap == nil {
		return nil, errors.New("RunAdaptive: snap must not be nil")
	}

	var cfg PipelineConfig
	if domain.isClassical() {
		// E-pre-cortex: detectors → aggregator unfiltered (higher recall on classical)
		cfg = PreCortexConfig()
	} else {
		// D-inhibitor-only: detectors + inhibitor + aggregator (higher precision).
		// Used for modern conversations, code review, and default (nil domain).
		cfg = InhibitorOnlyConfig()
	}

	// Wire domain context so applyDomainContext inside Run can adjust
	// detector thresholds for the specific domain's vocabulary characteristics.
	cfg.DomainContext = domain

	// Wire PTS endpoint for fire-and-forget anomaly signals.
	cfg.PTSEndpoint = ptsEndpoint

	// Wire outcome store for TP/FP tracking (optional).
	if len(store) > 0 {
		cfg.OutcomeStore = store[0]
	}

	result := Run(snap, cfg)
	return result, nil
}

// AdaptiveVariantName returns the name of the variant that RunAdaptive would
// select for the given domain. Useful for logging and testing.
//
// Code review domain maps to D-inhibitor-only (same variant as modern, with
// domain-specific threshold adjustments applied inside Run via applyDomainContext).
func AdaptiveVariantName(domain *DomainContext) string {
	if domain.isClassical() {
		return PreCortexInfo().Name
	}
	return InhibitorOnlyInfo().Name
}
