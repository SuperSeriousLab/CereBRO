package pipeline

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// PipelineConfig holds all detector configurations.
type PipelineConfig struct {
	Router              RouterConfig
	Anchoring           AnchoringConfig
	AnchoringContext    AnchoringContextConfig // context-aware variant (competition winner)
	UseContextAnchoring bool                   // if true, use context-aware anchoring detector
	SkipAnchoring       bool                   // if true, anchoring detector is omitted entirely (set by domain wiring)
	SunkCost            SunkCostConfig
	Contradiction       ContradictionConfig
	ScopeGuard          ScopeGuardConfig
	Calibrator          CalibratorConfig
	Ledger              LedgerConfig
	Inhibitor           InhibitorConfig // Phase 1: Context Inhibitor
	UseInhibitor        bool            // if true, run Context Inhibitor before Aggregator
	Urgency             UrgencyConfig   // Phase 2: Urgency Assessor
	Modulator           ModulatorConfig // Phase 2: Threshold Modulator
	UseNeuromodulation  bool            // if true, run Urgency Assessor + Threshold Modulator
	Layer0              Layer0Config    // Phase 3: Layer 0 reflexes
	UseLayer0           bool            // if true, run Layer 0 before Stage 1
	SelfConfidence      SelfConfidenceConfig // Phase 4: Self-Confidence Assessor
	Feedback            FeedbackConfig       // Phase 4: Feedback Evaluator
	UseMetacognition    bool                 // if true, run Self-Confidence + Feedback
	Salience            SalienceConfig       // Phase 5: Salience Filter
	UseSalience         bool                 // if true, run Salience Filter after Inhibitor
	Consolidator        *Consolidator        // Phase 5: Memory Consolidator (nil = disabled)
	MLEnricher          MLEnricherConfig     // ML Enricher configuration (Ollama)
	MLClient            *http.Client         // HTTP client for ML enricher (nil = use default)
	MLEnrichment        *cerebrov1.MLEnrichment // populated at runtime by Stage 1.3 (merged view)
	DomainContext       *DomainContext           // optional domain hint from upstream (e.g. Sophrim); nil = defaults
	SophrimEndpoint     string                   // if non-empty, fetch DomainContext before pipeline run (advisory, 200ms timeout)
	ConceptualAnchoring ConceptualAnchoringConfig // Tier 2: conceptual anchoring detector
	InheritedPosition   InheritedPositionConfig   // Tier 2: inherited-position detector

	// Sophrim feedback (Connection A of the Lamarckian Loop).
	// Set by the caller with metadata from SLR / Sophrim grounding.
	SophrimFeedbackEndpoint string  // if non-empty, send retrieval quality feedback after pipeline
	GroundingFactIDs        []int64 // fact IDs from Sophrim grounding
	GroundingQuery          string  // original query used for grounding

	// PTSEndpoint is the base URL for the Problem Tracking System.
	// When non-empty, anomalous pipeline results (score=0, low confidence,
	// metacognitive review flag, Layer 0 rejection) are reported via POST
	// /cog/signal in a fire-and-forget goroutine.
	// Example: "http://192.168.14.68:9746"
	// Override via PTS_ENDPOINT environment variable at process startup, or
	// set directly when constructing PipelineConfig.
	PTSEndpoint string

	// Logger is an optional zerolog.Logger for structured training data events.
	// When set, a single "cerebro_pipeline_run" Info event is emitted at the
	// end of every Run() call with full pipeline telemetry.
	// Use zerolog.Nop() (or leave as zero value — same effect) to disable logging.
	// Logging is skipped when Logger.GetLevel() == zerolog.Disabled.
	Logger zerolog.Logger
}

// DefaultPipelineConfig returns all-default detector configurations.
// Uses competition winners: context-aware anchoring, reference-window scope guard.
func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		Router:              DefaultRouterConfig(),
		Anchoring:           DefaultAnchoringConfig(),
		AnchoringContext:    DefaultAnchoringContextConfig(),
		UseContextAnchoring: true, // competition winner
		SunkCost:            DefaultSunkCostConfig(),
		Contradiction:       DefaultContradictionConfig(),
		ScopeGuard:          DefaultScopeGuardConfig(),
		Calibrator:          DefaultCalibratorConfig(),
		Ledger:              DefaultLedgerConfig(),
		SelfConfidence:      DefaultSelfConfidenceConfig(),
		Feedback:            DefaultFeedbackConfig(),
		ConceptualAnchoring: DefaultConceptualAnchoringConfig(),
		InheritedPosition:   DefaultInheritedPositionConfig(),
	}
}

// PipelineResult holds the complete pipeline execution results.
type PipelineResult struct {
	Report       *reasoningv1.ReasoningReport
	Routing      RoutingDecision
	Findings     []*reasoningv1.CognitiveAssessment
	Inhibition   *InhibitorResult               // nil if inhibitor not enabled
	Decisions    []*cerebrov1.InhibitionDecision  // nil if inhibitor not enabled
	Gain         *GainSignal                     // nil if neuromodulation not enabled
	Adjustments  *ThresholdAdjustments           // nil if neuromodulation not enabled
	Layer0       *Layer0Result                   // nil if Layer 0 not enabled
	Rejected     bool                            // true if Layer 0 rejected the input
	SelfConf     *cerebrov1.SelfConfidenceReport  // nil if metacognition not enabled
	Feedback     *FeedbackResult                 // nil if metacognition not enabled or feedback not triggered
	Salience     *SalienceResult                 // nil if salience not enabled
	Consolidated bool                            // true if memory consolidation occurred
	ConsolidationTrigger cerebrov1.ConsolidationTrigger // what triggered consolidation
	MLEnrichments []*cerebrov1.MLEnrichment       // nil if ML enricher not enabled
}

// Run executes the full cognitive pipeline on a ConversationSnapshot:
//
//	Stage 0:   Layer 0 Reflexes (format → toxicity → language) — Phase 3
//	Stage 1:   Intake (enrich)
//	Stage 1.5: Urgency Assessor (produce GainSignal) — Phase 2
//	Stage 2:   Router (classify, select detectors)
//	Stage 2.5: Threshold Modulator (adjust detector thresholds) — Phase 2
//	Stage 3:   Detectors (run with adjusted thresholds)
//	Stage 4:   Context Inhibitor (5-gate gating, using real GainSignal) — Phase 1
//	Stage 4.5: Salience Filter (novelty + actionability scoring) — Phase 5
//	Stage 5:   Aggregator (synthesize findings)
//	Stage 6:   Self-Confidence Assessor (metacognitive monitoring) — Phase 4
//	Stage 7:   Feedback Evaluator (bounded re-evaluation loop) — Phase 4
//	Stage 8:   Memory Consolidator (sparse indexing → corpus) — Phase 5
func Run(snap *reasoningv1.ConversationSnapshot, cfg PipelineConfig) *PipelineResult {
	// Stage 0: Layer 0 Reflexes (Phase 3)
	if cfg.UseLayer0 {
		l0 := RunLayer0(snap, cfg.Layer0)
		if !l0.Accepted {
			rejected := &PipelineResult{
				Layer0:   l0,
				Rejected: true,
			}
			// PTS anomaly signal for Layer 0 rejection (fire-and-forget).
			maybeSendPTSSignals(rejected, cfg.PTSEndpoint)
			return rejected
		}
	}

	// Pre-pipeline: Sophrim domain-hint fetch (advisory, 200ms budget).
	// If SophrimEndpoint is set and DomainContext was not explicitly provided,
	// ask Sophrim for domain hints. Failure (network, timeout, no hints) leaves
	// DomainContext nil — the pipeline runs with defaults, no regression.
	if cfg.SophrimEndpoint != "" && cfg.DomainContext == nil {
		sc := NewSophrimClient(cfg.SophrimEndpoint, 200*time.Millisecond)
		cfg.DomainContext = sc.FetchDomainContext(context.Background(), conversationSummary(snap))
	}

	// Stage 1: Intake enrichment
	snap = Enrich(snap)

	// Stage 1.3: ML Enrichment (if enabled)
	var mlEnrichments []*cerebrov1.MLEnrichment
	if cfg.MLEnricher.Enabled {
		client := cfg.MLClient
		if client == nil {
			client = http.DefaultClient
		}
		mlEnrichments = EnrichML(snap, cfg.MLEnricher, client)
		cfg.MLEnrichment = MergeMLEnrichments(mlEnrichments)
	}

	// Stage 1.5: Urgency Assessment (Phase 2)
	var gain *GainSignal
	var adjustments *ThresholdAdjustments

	if cfg.UseNeuromodulation {
		if cfg.MLEnrichment != nil {
			gain = AssessUrgencyML(snap, cfg.Urgency, cfg.MLEnrichment)
		} else {
			gain = AssessUrgency(snap, cfg.Urgency)
		}

		// Stage 2.5 (computed early so we can apply offsets to detectors)
		adjustments = Modulate(gain, cfg.Modulator)

		// Apply gain offsets to detector configs
		cfg = applyGainOffsets(cfg, adjustments)
	}

	// Domain context wiring: adjust detector configs for classical text (Sophrim hint).
	// Must run after gain offsets (which adjust thresholds) so domain overrides win.
	// Must run before buildDetectorMap so SkipAnchoring and config fields are visible.
	cfg = applyDomainContext(cfg)

	// Stage 2: Router — determine which detectors to activate
	routing := Route(snap, cfg.Router)

	// Stage 3: Run activated detectors (with possibly adjusted thresholds).
	// Detectors are called through a uniform DetectorFunc interface.
	detectors := buildDetectorMap(cfg)
	var findings []*reasoningv1.CognitiveAssessment

	for _, det := range routing.Activated {
		if fn, ok := detectors[det]; ok {
			if assessment := fn(snap); assessment != nil {
				findings = append(findings, assessment)
			}
		}
	}

	// Stage 4: Context Inhibitor (Phase 1)
	var inhibition *InhibitorResult
	aggregateFindings := findings

	if cfg.UseInhibitor {
		if cfg.UseNeuromodulation && gain != nil {
			// Phase 2: pass real GainSignal to inhibitor
			inhibition = InhibitWithGain(findings, snap, cfg.Inhibitor, gain)
		} else {
			// Phase 1 fallback: inline formality computation, stubbed urgency
			inhibition = Inhibit(findings, snap, cfg.Inhibitor)
		}
		aggregateFindings = inhibition.Gated // Only disinhibited findings pass
	}

	// Stage 4.5: Salience Filter (Phase 5)
	var salienceResult *SalienceResult
	if cfg.UseSalience {
		salienceResult = FilterSalience(aggregateFindings, cfg.Salience)
		aggregateFindings = salienceResult.Salient // Only salient findings pass to Aggregator
	}

	// Stage 5: Aggregate
	report := Aggregate(aggregateFindings, snap.GetObjective())

	// Stage 6: Self-Confidence Assessor (Phase 4)
	var selfConf *cerebrov1.SelfConfidenceReport
	var feedbackResult *FeedbackResult

	if cfg.UseMetacognition {
		selfConf = AssessConfidence(report, cfg.SelfConfidence)

		// Stage 7: Feedback Evaluator (Phase 4)
		// If confidence is low, re-evaluate weakest findings with peer context.
		updatedFindings, fbResult := EvaluateFeedback(
			aggregateFindings, selfConf, snap, report, cfg.Feedback, detectors,
		)
		feedbackResult = fbResult

		if fbResult.Applied {
			// Re-run inhibitor on updated findings if enabled.
			if cfg.UseInhibitor {
				if cfg.UseNeuromodulation && gain != nil {
					inhibition = InhibitWithGain(updatedFindings, snap, cfg.Inhibitor, gain)
				} else {
					inhibition = Inhibit(updatedFindings, snap, cfg.Inhibitor)
				}
				updatedFindings = inhibition.Gated
			}

			// Re-run salience filter on second pass if enabled.
			if cfg.UseSalience {
				salienceResult = FilterSalience(updatedFindings, cfg.Salience)
				updatedFindings = salienceResult.Salient
			}

			// Re-aggregate with updated findings (second pass).
			report = Aggregate(updatedFindings, snap.GetObjective())
		}
	}

	result := &PipelineResult{
		Report:        report,
		Routing:       routing,
		Findings:      findings, // all raw findings (pre-inhibition)
		Inhibition:    inhibition,
		Gain:          gain,
		Adjustments:   adjustments,
		SelfConf:      selfConf,
		Feedback:      feedbackResult,
		Salience:      salienceResult,
		MLEnrichments: mlEnrichments,
	}

	// Stage 8: Memory Consolidator (Phase 5)
	// Synchronous — just a JSON marshal + file append.
	if cfg.Consolidator != nil {
		fbApplied := feedbackResult != nil && feedbackResult.Applied
		cr := cfg.Consolidator.Consolidate(&ConsolidationInput{
			ConversationID: report.GetConversationId(),
			Report:         report,
			Inhibition:     inhibition,
			SelfConf:       selfConf,
			FeedbackApplied: fbApplied,
			Gain:           gain,
			Snap:           snap,
		})
		if cr != nil && cr.Consolidated {
			result.Consolidated = true
			result.ConsolidationTrigger = cr.Trigger
		}
	}

	// Stage 9: Sophrim Feedback (Connection A of the Lamarckian Loop)
	// Fire-and-forget — never blocks the pipeline response.
	if cfg.SophrimFeedbackEndpoint != "" && len(cfg.GroundingFactIDs) > 0 {
		sender := NewFeedbackSender(cfg.SophrimFeedbackEndpoint, 5*time.Second)
		signal := "negative" // default: no findings = grounding wasn't helpful
		fbContext := "no_findings"
		if len(result.Findings) > 0 {
			signal = "positive"
			fbContext = fmt.Sprintf("findings=%d,types=%s", len(result.Findings), findingTypes(result))
		}
		go sender.SendFeedback(context.Background(), cfg.GroundingQuery, cfg.GroundingFactIDs, signal, fbContext)
	}

	// Stage 10: PTS Anomaly Signals (fire-and-forget)
	// Report zero-score, low-confidence, or metacognitive-review-flagged results
	// to the Problem Tracking System for human triage.
	maybeSendPTSSignals(result, cfg.PTSEndpoint)

	// Stage 11: Training data log event (optional — zero Logger = no-op).
	// Emits one structured event per Run() with full pipeline telemetry for
	// offline analysis, dataset construction, and performance tracking.
	if cfg.Logger.GetLevel() != zerolog.Disabled {
		// Collect finding types and confidences from raw findings (pre-inhibition).
		ftypes := make([]string, len(result.Findings))
		fconfs := make([]float64, len(result.Findings))
		for i, f := range result.Findings {
			ftypes[i] = f.GetFindingType().String()
			fconfs[i] = f.GetConfidence()
		}

		// Count findings after inhibition (post-gating).
		afterInhibition := len(result.Findings)
		if result.Inhibition != nil {
			afterInhibition = len(result.Inhibition.Gated)
		}

		// Count findings after salience filter.
		afterSalience := afterInhibition
		if result.Salience != nil {
			afterSalience = len(result.Salience.Salient)
		}

		// Aggregate metrics — safe when report is nil (rejected early).
		var integrityScore float64
		var criticalCount, warningCount, cautionCount uint32
		if result.Report != nil {
			integrityScore = result.Report.GetOverallIntegrityScore()
			criticalCount = result.Report.GetCriticalCount()
			warningCount = result.Report.GetWarningCount()
			cautionCount = result.Report.GetCautionCount()
		}

		// Self-confidence score — zero when metacognition was not enabled.
		var selfConfScore float64
		if result.SelfConf != nil {
			selfConfScore = result.SelfConf.GetOverallConfidence()
		}

		// Feedback applied flag.
		fbApplied := result.Feedback != nil && result.Feedback.Applied

		// Total text length across all turns.
		var totalTextLen int
		for _, t := range snap.GetTurns() {
			totalTextLen += len(t.GetRawText())
		}

		cfg.Logger.Info().
			Str("event", "cerebro_pipeline_run").
			Int("message_count", int(snap.GetTotalTurns())).
			Int("total_text_length", totalTextLen).
			Strs("detectors_activated", routingActivatedNames(result.Routing)).
			Int("findings_raw", len(result.Findings)).
			Int("findings_after_inhibition", afterInhibition).
			Int("findings_after_salience", afterSalience).
			Strs("finding_types", ftypes).
			Floats64("finding_confidences", fconfs).
			Float64("integrity_score", integrityScore).
			Uint32("critical_count", criticalCount).
			Uint32("warning_count", warningCount).
			Uint32("caution_count", cautionCount).
			Bool("ml_enricher_enabled", cfg.MLEnricher.Enabled).
			Int("ml_enrichments_count", len(result.MLEnrichments)).
			Float64("self_confidence", selfConfScore).
			Bool("feedback_applied", fbApplied).
			Bool("consolidated", result.Consolidated).
			Msg("pipeline: run logged")
	}

	return result
}

// applyGainOffsets creates a modified PipelineConfig with detector thresholds adjusted
// by the gain offsets from the Threshold Modulator.
func applyGainOffsets(cfg PipelineConfig, adj *ThresholdAdjustments) PipelineConfig {
	if adj == nil {
		return cfg
	}

	// scope-guard: DriftThreshold is Forge-optimized (0.80) and excluded from
	// gain modulation. Adjusting it causes both false positives (lowered threshold
	// triggers on stable conversations) and false negatives (raised threshold
	// masks real drift). The Forge sweep already found the optimal value.

	// anchoring-detector-context: ProximityThreshold (competition winner)
	if offset, ok := adj.Adjustments["anchoring-detector-context"]; ok {
		cfg.AnchoringContext.ProximityThreshold = ApplyGainOffset(cfg.AnchoringContext.ProximityThreshold, offset)
	}

	// anchoring-detector: ProximityThreshold (fallback)
	if offset, ok := adj.Adjustments["anchoring-detector"]; ok {
		cfg.Anchoring.ProximityThreshold = ApplyGainOffset(cfg.Anchoring.ProximityThreshold, offset)
	}

	// sunk-cost-detector: MinConfidence
	if offset, ok := adj.Adjustments["sunk-cost-detector"]; ok {
		cfg.SunkCost.MinConfidence = ApplyGainOffset(cfg.SunkCost.MinConfidence, offset)
	}

	// contradiction-tracker: MinOverlap
	if offset, ok := adj.Adjustments["contradiction-tracker"]; ok {
		cfg.Contradiction.MinOverlap = ApplyGainOffset(cfg.Contradiction.MinOverlap, offset)
	}

	// confidence-calibrator: MinMiscalibration
	if offset, ok := adj.Adjustments["confidence-calibrator"]; ok {
		cfg.Calibrator.MinMiscalibration = ApplyGainOffset(cfg.Calibrator.MinMiscalibration, offset)
	}

	// decision-ledger: TopicSimilarityThreshold
	if offset, ok := adj.Adjustments["decision-ledger"]; ok {
		cfg.Ledger.TopicSimilarityThreshold = ApplyGainOffset(cfg.Ledger.TopicSimilarityThreshold, offset)
	}

	return cfg
}

// DetectorFunc is the uniform calling convention for all cognitive detectors.
// Each detector's config is bound via closure when building the detector map.
// Phase 4 feedback re-evaluation reuses the same DetectorFunc signature — detectors
// are re-run on the same snapshot, then applyFeedbackAdjustment adjusts confidence
// based on peer findings from the first pass.
type DetectorFunc func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment

// buildDetectorMap creates a uniform map of detector names to functions.
// Config is captured by closure so all detectors share the same signature.
// When cfg.MLEnrichment is non-nil, ML-enhanced variants are used.
func buildDetectorMap(cfg PipelineConfig) map[Detector]DetectorFunc {
	ml := cfg.MLEnrichment // captured by closures; nil when ML disabled

	m := map[Detector]DetectorFunc{
		DetectorSunkCost: func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
			if ml != nil {
				return DetectSunkCostML(snap, cfg.SunkCost, ml)
			}
			return DetectSunkCost(snap, cfg.SunkCost)
		},
		DetectorContradiction: func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
			return DetectContradiction(snap, cfg.Contradiction)
		},
		DetectorScopeGuard: func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
			return DetectScopeDrift(snap, cfg.ScopeGuard)
		},
		DetectorCalibrator: func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
			if ml != nil {
				return DetectConfidenceMiscalibrationML(snap, cfg.Calibrator, ml)
			}
			return DetectConfidenceMiscalibration(snap, cfg.Calibrator)
		},
		DetectorLedger: func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
			return DetectSilentRevision(snap, cfg.Ledger)
		},
	}
	// Anchoring: omit entirely when SkipAnchoring is set (e.g. classical domain context).
	if !cfg.SkipAnchoring {
		if cfg.UseContextAnchoring {
			m[DetectorAnchoring] = func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
				if ml != nil {
					return DetectAnchoringContextML(snap, cfg.AnchoringContext, ml)
				}
				return DetectAnchoringContext(snap, cfg.AnchoringContext)
			}
		} else {
			m[DetectorAnchoring] = func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
				return DetectAnchoring(snap, cfg.Anchoring)
			}
		}
	}

	// Conceptual Anchoring: ALWAYS registered — not skipped by classical domain context.
	// This is the propositional variant that fires on classical text where numeric
	// anchoring is absent. cfg.SkipAnchoring only suppresses the numeric detector.
	m[DetectorConceptualAnchoring] = func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
		return DetectConceptualAnchoring(snap, cfg.ConceptualAnchoring)
	}

	// Inherited Position: ALWAYS registered — fires on authority-citation patterns
	// without independent justification. Distinct from sunk-cost and conceptual anchoring.
	m[DetectorInheritedPosition] = func(snap *reasoningv1.ConversationSnapshot) *reasoningv1.CognitiveAssessment {
		return DetectInheritedPosition(snap, cfg.InheritedPosition)
	}

	return m
}

// routingActivatedNames converts []Detector to []string for zerolog Strs().
func routingActivatedNames(r RoutingDecision) []string {
	names := make([]string, len(r.Activated))
	for i, d := range r.Activated {
		names[i] = string(d)
	}
	return names
}

// ToCerebroReport converts a PipelineResult into the proto CerebroReport message.
// Used at the gateway boundary when serializing pipeline output.
// Returns nil if the pipeline rejected the input at Layer 0 (no assessment was performed).
func (r *PipelineResult) ToCerebroReport() *cerebrov1.CerebroReport {
	if r.Rejected {
		return nil
	}

	passCount := uint32(1)
	feedbackApplied := false
	if r.Feedback != nil && r.Feedback.Applied {
		passCount = 2
		feedbackApplied = true
	}

	cr := &cerebrov1.CerebroReport{
		BaseReport:      r.Report,
		PassCount:       passCount,
		FeedbackApplied: feedbackApplied,
	}

	if r.Report != nil {
		cr.AssessedAt = r.Report.AssessedAt
		cr.ConversationId = r.Report.ConversationId
	}

	if r.Inhibition != nil {
		cr.InhibitionLog = r.Inhibition.Decisions
	}

	if r.Gain != nil {
		cr.GainSignal = &cerebrov1.GainSignal{
			Urgency:    r.Gain.Urgency,
			Complexity: r.Gain.Complexity,
			Formality:  r.Gain.Formality,
			Mode:       r.Gain.Mode,
		}
	}

	if r.Adjustments != nil {
		cr.ThresholdAdjustments = &cerebrov1.ThresholdAdjustments{
			Adjustments: r.Adjustments.Adjustments,
		}
		if r.Gain != nil {
			cr.ThresholdAdjustments.SourceMode = r.Gain.Mode
		}
	}

	if r.SelfConf != nil {
		cr.SelfConfidence = r.SelfConf
	}

	// Phase 5: Salience + Consolidation
	if r.Salience != nil {
		cr.SalienceScores = r.Salience.Scores
	}
	cr.Consolidated = r.Consolidated
	cr.ConsolidationTrigger = r.ConsolidationTrigger
	cr.MlEnrichments = r.MLEnrichments

	return cr
}
