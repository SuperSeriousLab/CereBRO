package pipeline

import (
	"fmt"
	"math"
	"testing"
)

// TestForgeOptimization simulates Forge-style parameter optimization
// by sweeping evolvable config fields and measuring pipeline F1.
func TestForgeOptimization(t *testing.T) {
	convs := loadTestConversations(t)

	type configTrial struct {
		name    string
		cfg     PipelineConfig
		metrics struct {
			tp, fn, fp int
			precision, recall, f1 float64
		}
	}

	// Sweep scope guard parameters.
	var trials []configTrial

	for _, threshold := range []float64{0.80, 0.85, 0.90, 0.95} {
		for _, sustained := range []uint32{5, 6, 7, 8} {
			for _, refTurns := range []uint32{2, 3, 4} {
				cfg := DefaultPipelineConfig()
				cfg.ScopeGuard.DriftThreshold = threshold
				cfg.ScopeGuard.SustainedTurns = sustained
				cfg.ScopeGuard.ReferenceTurns = refTurns

				trial := configTrial{
					name: fmt.Sprintf("sg(t=%.2f,s=%d,r=%d)", threshold, sustained, refTurns),
					cfg:  cfg,
				}
				trials = append(trials, trial)
			}
		}
	}

	// Also sweep anchoring context threshold.
	for _, ctxThresh := range []float64{0.1, 0.15, 0.2, 0.25, 0.3} {
		cfg := DefaultPipelineConfig()
		cfg.AnchoringContext.ContextThreshold = ctxThresh
		trial := configTrial{
			name: fmt.Sprintf("anc(ctx=%.2f)", ctxThresh),
			cfg:  cfg,
		}
		trials = append(trials, trial)
	}

	// Evaluate each trial.
	bestF1 := 0.0
	bestTrial := ""
	var bestCfg PipelineConfig

	for i := range trials {
		trial := &trials[i]
		tp, fn, fp := 0, 0, 0

		for _, conv := range convs {
			result := Run(conv.snapshot, trial.cfg)

			actualTypes := make(map[string]bool)
			for _, finding := range result.Findings {
				actualTypes[findingTypeString(finding.FindingType)] = true
			}
			expectedTypes := make(map[string]bool)
			for _, et := range conv.expectedTypes {
				expectedTypes[et] = true
			}

			for et := range expectedTypes {
				if actualTypes[et] {
					tp++
				} else {
					fn++
				}
			}
			for at := range actualTypes {
				if !expectedTypes[at] {
					fp++
				}
			}
		}

		trial.metrics.tp = tp
		trial.metrics.fn = fn
		trial.metrics.fp = fp
		if tp+fp > 0 {
			trial.metrics.precision = float64(tp) / float64(tp+fp)
		}
		if tp+fn > 0 {
			trial.metrics.recall = float64(tp) / float64(tp+fn)
		}
		if trial.metrics.precision+trial.metrics.recall > 0 {
			trial.metrics.f1 = 2 * trial.metrics.precision * trial.metrics.recall / (trial.metrics.precision + trial.metrics.recall)
		}

		// Only consider trials with recall >= 0.8.
		if trial.metrics.recall >= 0.8 && trial.metrics.f1 > bestF1 {
			bestF1 = trial.metrics.f1
			bestTrial = trial.name
			bestCfg = trial.cfg
		}
	}

	t.Log("\n========== FORGE OPTIMIZATION RESULTS ==========")
	t.Logf("  Total trials: %d", len(trials))
	t.Logf("  Best config: %s", bestTrial)
	t.Logf("  Best F1: %.2f (recall >= 0.8 constraint)", bestF1)

	// Show top 5 configs.
	type ranked struct {
		name string
		f1   float64
		recall float64
		precision float64
		fp int
	}
	var top []ranked
	for _, trial := range trials {
		if trial.metrics.recall >= 0.8 {
			top = append(top, ranked{trial.name, trial.metrics.f1, trial.metrics.recall, trial.metrics.precision, trial.metrics.fp})
		}
	}

	// Sort by F1 desc.
	for i := 0; i < len(top)-1; i++ {
		for j := i + 1; j < len(top); j++ {
			if top[j].f1 > top[i].f1 || (math.Abs(top[j].f1-top[i].f1) < 0.001 && top[j].fp < top[i].fp) {
				top[i], top[j] = top[j], top[i]
			}
		}
	}

	limit := 10
	if limit > len(top) {
		limit = len(top)
	}
	t.Logf("\n  Top %d configs (recall >= 0.8):", limit)
	t.Logf("  %-35s %7s %7s %7s %5s", "Config", "Prec", "Recall", "F1", "FP")
	for i := 0; i < limit; i++ {
		t.Logf("  %-35s %7.2f %7.2f %7.2f %5d",
			top[i].name, top[i].precision, top[i].recall, top[i].f1, top[i].fp)
	}

	// Validate the best config produces improvement.
	if bestF1 > 0.64 {
		t.Logf("\n  Forge optimization improved F1: 0.64 → %.2f (+%.2f)", bestF1, bestF1-0.64)
	}

	// Run the best config once more for detailed output.
	t.Log("\n  --- Best config detailed results ---")
	totalTP, totalFN, totalFP := 0, 0, 0
	for _, conv := range convs {
		result := Run(conv.snapshot, bestCfg)
		actualTypes := make(map[string]bool)
		for _, finding := range result.Findings {
			actualTypes[findingTypeString(finding.FindingType)] = true
		}
		expectedTypes := make(map[string]bool)
		for _, et := range conv.expectedTypes {
			expectedTypes[et] = true
		}
		tp, fn, fp := 0, 0, 0
		for et := range expectedTypes {
			if actualTypes[et] {
				tp++
			} else {
				fn++
			}
		}
		for at := range actualTypes {
			if !expectedTypes[at] {
				fp++
			}
		}
		totalTP += tp
		totalFN += fn
		totalFP += fp

		status := "PASS"
		if fn > 0 {
			status = "MISS"
		}
		if fp > 0 && len(conv.expectedTypes) == 0 {
			status = "FP"
		}
		t.Logf("    [%s] %-20s TP=%d FN=%d FP=%d", status, conv.id, tp, fn, fp)
	}
	t.Logf("    TOTAL: TP=%d FN=%d FP=%d", totalTP, totalFN, totalFP)
}
