package pipeline

import (
	"fmt"

	"github.com/SuperSeriousLab/fugo"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// FIS config name for cross-layer arbitration.
const FISArbitration = "cross_layer_arbitration"

// ArbitrationAction is the recommended action from cross-layer arbitration.
type ArbitrationAction string

const (
	ArbitrationDismiss     ArbitrationAction = "dismiss"     // compound_pathology <= 0.35
	ArbitrationMonitor     ArbitrationAction = "monitor"     // 0.35 < compound_pathology <= 0.65
	ArbitrationInvestigate ArbitrationAction = "investigate" // compound_pathology > 0.65
)

// ArbitrationResult holds the output of cross-layer arbitration.
type ArbitrationResult struct {
	CompoundPathology float64           // [0, 1] — overall session health assessment
	Action            ArbitrationAction // derived from compound_pathology thresholds
	FindingCount      int               // total findings considered
	InhibitedCount    int               // findings that were inhibited
}

// CrossLayerArbitrator performs compound pathology assessment across L2 detector
// severities and L3 inhibition results using fuzzy inference.
type CrossLayerArbitrator struct {
	Engine *fugo.FuzzyEngine
}

// BuildCrossLayerArbitrator constructs an arbitrator from a FIS config.
func BuildCrossLayerArbitrator(cfg *fugo.FisConfig) (*CrossLayerArbitrator, error) {
	e, err := cfg.BuildEngine()
	if err != nil {
		return nil, fmt.Errorf("arbitration FIS: %w", err)
	}
	return &CrossLayerArbitrator{Engine: e}, nil
}

// BuildCrossLayerArbitratorFromRegistry constructs an arbitrator from a FisRegistry.
func BuildCrossLayerArbitratorFromRegistry(reg *fugo.FisRegistry) (*CrossLayerArbitrator, error) {
	e, err := reg.BuildEngine(FISArbitration)
	if err != nil {
		return nil, fmt.Errorf("arbitration FIS: %w", err)
	}
	return &CrossLayerArbitrator{Engine: e}, nil
}

// Arbitrate produces a compound pathology assessment from raw findings and
// inhibition results. When the arbitrator is nil, all findings pass through
// with a neutral compound_pathology of 0.0 (no assessment).
func (a *CrossLayerArbitrator) Arbitrate(
	findings []*reasoningv1.CognitiveAssessment,
	inhibitionResult *InhibitorResult,
) *ArbitrationResult {
	if a == nil || a.Engine == nil {
		// Nil arbitrator → passthrough, no compound assessment.
		return &ArbitrationResult{
			CompoundPathology: 0.0,
			Action:            ArbitrationDismiss,
			FindingCount:      len(findings),
		}
	}

	if len(findings) == 0 {
		return &ArbitrationResult{
			CompoundPathology: 0.0,
			Action:            ArbitrationDismiss,
		}
	}

	// Compute inputs for the FIS.
	maxSev := computeMaxSeverity(findings)
	density := computeFindingDensity(findings)
	inhRatio := computeInhibitionRatio(findings, inhibitionResult)

	outputs, err := a.Engine.Evaluate(map[string]float64{
		"max_severity":    maxSev,
		"finding_density": density,
		"inhibition_ratio": inhRatio,
	})
	if err != nil {
		// FIS evaluation error — return neutral assessment.
		return &ArbitrationResult{
			CompoundPathology: 0.0,
			Action:            ArbitrationDismiss,
			FindingCount:      len(findings),
		}
	}

	cp, ok := outputs["compound_pathology"]
	if !ok {
		return &ArbitrationResult{
			CompoundPathology: 0.0,
			Action:            ArbitrationDismiss,
			FindingCount:      len(findings),
		}
	}

	inhibitedCount := 0
	if inhibitionResult != nil {
		inhibitedCount = len(findings) - len(inhibitionResult.Gated)
		if inhibitedCount < 0 {
			inhibitedCount = 0
		}
	}

	return &ArbitrationResult{
		CompoundPathology: cp,
		Action:            classifyAction(cp),
		FindingCount:      len(findings),
		InhibitedCount:    inhibitedCount,
	}
}

// classifyAction maps compound_pathology to an ArbitrationAction.
func classifyAction(cp float64) ArbitrationAction {
	if cp > 0.65 {
		return ArbitrationInvestigate
	}
	if cp > 0.35 {
		return ArbitrationMonitor
	}
	return ArbitrationDismiss
}

// computeMaxSeverity returns the highest confidence among all findings,
// normalized to [0, 1]. Uses confidence as a proxy for severity strength
// since L2 fuzzy detectors replace severity with graded confidence.
func computeMaxSeverity(findings []*reasoningv1.CognitiveAssessment) float64 {
	var maxConf float64
	for _, f := range findings {
		conf := f.GetConfidence()
		// Also factor in proto severity ordinal if confidence is not already fuzzy.
		sevBoost := float64(f.GetSeverity()) * 0.25 // INFO=0.25, CAUTION=0.5, WARNING=0.75, CRITICAL=1.0
		effective := conf
		if sevBoost > effective {
			effective = sevBoost
		}
		if effective > maxConf {
			maxConf = effective
		}
	}
	return clamp(maxConf, 0.0, 1.0)
}

// computeFindingDensity normalizes finding count to [0, 1].
// 0 findings → 0.0, 4+ findings → 1.0. Linear interpolation between.
func computeFindingDensity(findings []*reasoningv1.CognitiveAssessment) float64 {
	n := float64(len(findings))
	return clamp(n/4.0, 0.0, 1.0)
}

// computeInhibitionRatio returns the fraction of findings that were inhibited.
// 0.0 = no inhibition (all passed), 1.0 = all inhibited.
func computeInhibitionRatio(findings []*reasoningv1.CognitiveAssessment, inhResult *InhibitorResult) float64 {
	if inhResult == nil || len(findings) == 0 {
		return 0.0 // no inhibition data
	}
	gated := len(inhResult.Gated)
	total := len(findings)
	inhibited := total - gated
	if inhibited < 0 {
		inhibited = 0
	}
	return float64(inhibited) / float64(total)
}
