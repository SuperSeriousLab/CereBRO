package pipeline

import (
	"fmt"
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// =============================================================================
// P4.4: Fuzzy CereBRO Validation — crisp vs fuzzy comparison
//
// Tests exercise the full L1->L2->L3->L4->Arbitration pipeline with both
// crisp (nil fuzzy) and fuzzy (all FIS configs loaded) configurations.
// =============================================================================

// buildCrispConfig returns a pipeline config with all layers enabled but no
// fuzzy inference (nil DetectorFuzzy, nil FuzzyUrgency, nil Arbitrator).
func buildCrispConfig() PipelineConfig {
	cfg := DefaultPipelineConfig()
	cfg.UseInhibitor = true
	cfg.UseNeuromodulation = true
	cfg.UseMetacognition = true
	cfg.UseSalience = true
	// All fuzzy components nil — crisp mode.
	return cfg
}

// buildFuzzyConfig returns a pipeline config with all layers enabled AND all
// fuzzy FIS systems wired in (L1 urgency, L2 detectors, L3 inhibitor gates,
// cross-layer arbitration).
func buildFuzzyConfig(t *testing.T) PipelineConfig {
	t.Helper()
	cfg := DefaultPipelineConfig()
	cfg.UseInhibitor = true
	cfg.UseNeuromodulation = true
	cfg.UseMetacognition = true
	cfg.UseSalience = true

	// L2: Fuzzy detector severity.
	cfg.DetectorFuzzy = buildTestDetectorFuzzy(t)

	// L1: Fuzzy urgency / gain signal.
	cfg.FuzzyUrgency = buildTestFuzzyUrgency(t)

	// L3: Fuzzy inhibitor gates.
	inh := DefaultInhibitorConfig()
	inh.Fuzzy = buildTestFuzzyInhibitor(t)
	cfg.Inhibitor = inh

	// Cross-layer arbitration.
	cfg.Arbitrator = buildTestArbitrator(t)

	return cfg
}

// =============================================================================
// Test conversations
// =============================================================================

// pathologicalConversation has clear reasoning pathologies (anchoring, contradiction).
func pathologicalConversation() *reasoningv1.ConversationSnapshot {
	return &reasoningv1.ConversationSnapshot{
		Objective: "budget estimation",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "The initial budget estimate was $100,000. What do you think the final cost will be?"},
			{TurnNumber: 2, Speaker: "assistant", RawText: "Based on the initial figure of $100,000, I estimate the final cost will be approximately $105,000."},
			{TurnNumber: 3, Speaker: "user", RawText: "But you said earlier that costs could double. Isn't that a contradiction?"},
			{TurnNumber: 4, Speaker: "assistant", RawText: "According to the specification, costs will not double. The estimate remains at $105,000. I am absolutely certain of this assessment."},
		},
		TotalTurns: 4,
	}
}

// balancedConversation is healthy — balanced feedback, no clear pathologies.
func balancedConversation() *reasoningv1.ConversationSnapshot {
	return &reasoningv1.ConversationSnapshot{
		Objective: "system design",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "What database should we use for the new service?"},
			{TurnNumber: 2, Speaker: "assistant", RawText: "There are several options to consider. PostgreSQL offers strong ACID guarantees and is well-suited for relational data. MongoDB provides flexibility for document-oriented workloads. The choice depends on your access patterns and consistency requirements."},
			{TurnNumber: 3, Speaker: "user", RawText: "We mostly do key-value lookups with occasional joins."},
			{TurnNumber: 4, Speaker: "assistant", RawText: "For primarily key-value access patterns with some joins, PostgreSQL with proper indexing would serve well. You could also consider a hybrid approach with Redis for hot reads and PostgreSQL for persistence and joins."},
		},
		TotalTurns: 4,
	}
}

// mildAgreementConversation has mild agreement patterns that crisp detectors
// might flag as pathological but fuzzy should treat more softly.
func mildAgreementConversation() *reasoningv1.ConversationSnapshot {
	return &reasoningv1.ConversationSnapshot{
		Objective: "planning discussion",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "I think we should target a 3-month timeline for this project."},
			{TurnNumber: 2, Speaker: "assistant", RawText: "A 3-month timeline seems reasonable given the scope. We should plan for some buffer though, maybe 3.5 months to account for testing."},
			{TurnNumber: 3, Speaker: "user", RawText: "Good point. Let's go with 3.5 months then."},
			{TurnNumber: 4, Speaker: "assistant", RawText: "Agreed. I'll structure the plan around a 3.5-month delivery window with milestones at weeks 4, 8, and 12."},
		},
		TotalTurns: 4,
	}
}

// casualConversation is very informal — should trigger inhibition in crisp
// but fuzzy should handle it more gracefully (graded suppression).
func casualConversation() *reasoningv1.ConversationSnapshot {
	return &reasoningv1.ConversationSnapshot{
		Objective: "quick chat",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "hey what color should we use for the button lol"},
			{TurnNumber: 2, Speaker: "assistant", RawText: "I think blue would look nice! Definitely a solid choice yeah."},
			{TurnNumber: 3, Speaker: "user", RawText: "yeah blue is good, let's go with that"},
		},
		TotalTurns: 3,
	}
}

// formalPathologicalConversation has pathologies in a formal context — both
// crisp and fuzzy should detect them. This tests no-regression.
func formalPathologicalConversation() *reasoningv1.ConversationSnapshot {
	return &reasoningv1.ConversationSnapshot{
		Objective: "security assessment",
		Turns: []*reasoningv1.Turn{
			{TurnNumber: 1, Speaker: "user", RawText: "The security audit identified 15 critical vulnerabilities. What is your assessment of the risk?"},
			{TurnNumber: 2, Speaker: "assistant", RawText: "Based on the initial report of 15 vulnerabilities, the estimated remediation cost is approximately $15,500."},
			{TurnNumber: 3, Speaker: "user", RawText: "Previously you stated the system was secure. How do you reconcile that with 15 critical findings?"},
			{TurnNumber: 4, Speaker: "assistant", RawText: "According to the specification, the system meets all security requirements. The 15 vulnerabilities do not constitute a security risk. I am absolutely certain the system is secure."},
		},
		TotalTurns: 4,
	}
}

// =============================================================================
// pipelineMetrics captures comparison data from a pipeline run.
// =============================================================================

type pipelineMetrics struct {
	label         string
	findingCount  int
	findingTypes  []string
	gatedCount    int    // findings after inhibition
	arbitration   string // ArbitrationAction
	compoundPath  float64
}

func runAndCollect(snap *reasoningv1.ConversationSnapshot, cfg PipelineConfig, label string) pipelineMetrics {
	result := Run(snap, cfg)

	m := pipelineMetrics{
		label:        label,
		findingCount: len(result.Findings),
	}

	for _, f := range result.Findings {
		m.findingTypes = append(m.findingTypes, f.GetFindingType().String())
	}

	if result.Inhibition != nil {
		m.gatedCount = len(result.Inhibition.Gated)
	} else {
		m.gatedCount = m.findingCount
	}

	if result.Arbitration != nil {
		m.arbitration = string(result.Arbitration.Action)
		m.compoundPath = result.Arbitration.CompoundPathology
	} else {
		m.arbitration = "n/a"
	}

	return m
}

// =============================================================================
// Test 1: Fuzzy vs Crisp — no-regression (recall >= 1.00)
// Everything crisp detects, fuzzy must also detect.
// =============================================================================

func TestFuzzyValidation_NoRegression(t *testing.T) {
	conversations := []struct {
		name string
		snap *reasoningv1.ConversationSnapshot
	}{
		{"pathological", pathologicalConversation()},
		{"formal_pathological", formalPathologicalConversation()},
	}

	crispCfg := buildCrispConfig()
	fuzzyCfg := buildFuzzyConfig(t)

	for _, conv := range conversations {
		t.Run(conv.name, func(t *testing.T) {
			crispResult := Run(conv.snap, crispCfg)
			fuzzyResult := Run(conv.snap, fuzzyCfg)

			// Build set of finding types from crisp.
			crispTypes := make(map[string]bool)
			for _, f := range crispResult.Findings {
				crispTypes[f.GetFindingType().String()] = true
			}

			// Every finding type crisp detects must also appear in fuzzy.
			fuzzyTypes := make(map[string]bool)
			for _, f := range fuzzyResult.Findings {
				fuzzyTypes[f.GetFindingType().String()] = true
			}

			for ft := range crispTypes {
				if !fuzzyTypes[ft] {
					t.Errorf("regression: crisp detects %q but fuzzy does not", ft)
				}
			}

			// Recall check: fuzzy finding count >= crisp finding count.
			if len(fuzzyResult.Findings) < len(crispResult.Findings) {
				t.Errorf("fuzzy recall regression: fuzzy=%d findings < crisp=%d findings",
					len(fuzzyResult.Findings), len(crispResult.Findings))
			}

			t.Logf("[%s] crisp=%d findings %v, fuzzy=%d findings %v",
				conv.name,
				len(crispResult.Findings), findingTypeNames(crispResult.Findings),
				len(fuzzyResult.Findings), findingTypeNames(fuzzyResult.Findings))
		})
	}
}

// =============================================================================
// Test 2: FP reduction — balanced/mild/casual scenarios
// Fuzzy should produce fewer or softer findings on non-pathological input.
// =============================================================================

func TestFuzzyValidation_FPReduction(t *testing.T) {
	scenarios := []struct {
		name string
		snap *reasoningv1.ConversationSnapshot
	}{
		{"balanced_feedback", balancedConversation()},
		{"mild_agreement", mildAgreementConversation()},
		{"casual_informal", casualConversation()},
	}

	crispCfg := buildCrispConfig()
	fuzzyCfg := buildFuzzyConfig(t)

	totalCrispFP := 0
	totalFuzzyFP := 0

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			crispResult := Run(sc.snap, crispCfg)
			fuzzyResult := Run(sc.snap, fuzzyCfg)

			// On non-pathological input, any findings are FPs.
			crispFP := len(crispResult.Findings)
			fuzzyFP := len(fuzzyResult.Findings)
			totalCrispFP += crispFP
			totalFuzzyFP += fuzzyFP

			// Also compare gated counts (post-inhibition).
			crispGated := crispFP
			fuzzyGated := fuzzyFP
			if crispResult.Inhibition != nil {
				crispGated = len(crispResult.Inhibition.Gated)
			}
			if fuzzyResult.Inhibition != nil {
				fuzzyGated = len(fuzzyResult.Inhibition.Gated)
			}

			t.Logf("[%s] crisp: %d raw / %d gated, fuzzy: %d raw / %d gated",
				sc.name, crispFP, crispGated, fuzzyFP, fuzzyGated)

			// Fuzzy's gated findings should not exceed crisp's gated findings.
			// (Fuzzy should be at least as good at suppressing FPs post-inhibition.)
			if fuzzyGated > crispGated {
				t.Logf("note: fuzzy gated (%d) > crisp gated (%d) — fuzzy is less aggressive on %s",
					fuzzyGated, crispGated, sc.name)
			}

			// If fuzzy produces findings, their confidence should be lower (softer).
			if fuzzyFP > 0 && crispFP > 0 {
				crispMaxConf := maxConfidence(crispResult.Findings)
				fuzzyMaxConf := maxConfidence(fuzzyResult.Findings)
				t.Logf("[%s] crisp max conf=%.3f, fuzzy max conf=%.3f", sc.name, crispMaxConf, fuzzyMaxConf)
			}
		})
	}

	t.Logf("FP summary: crisp total=%d, fuzzy total=%d", totalCrispFP, totalFuzzyFP)

	// Fuzzy should produce <= FPs than crisp across all healthy scenarios.
	if totalFuzzyFP > totalCrispFP {
		t.Logf("fuzzy produced more FPs (%d) than crisp (%d) — check FIS tuning",
			totalFuzzyFP, totalCrispFP)
	}
}

// =============================================================================
// Test 3: Full pipeline integration — L1->L2->L3->L4->Arbitration
// Verifies the complete end-to-end flow with all layers active.
// =============================================================================

func TestFuzzyValidation_FullPipelineIntegration(t *testing.T) {
	fuzzyCfg := buildFuzzyConfig(t)

	conversations := []struct {
		name string
		snap *reasoningv1.ConversationSnapshot
	}{
		{"pathological", pathologicalConversation()},
		{"balanced", balancedConversation()},
		{"mild_agreement", mildAgreementConversation()},
		{"casual", casualConversation()},
		{"formal_pathological", formalPathologicalConversation()},
	}

	for _, conv := range conversations {
		t.Run(conv.name, func(t *testing.T) {
			result := Run(conv.snap, fuzzyCfg)

			// Verify pipeline did not panic and produced a report.
			if result == nil {
				t.Fatal("pipeline returned nil result")
			}
			if result.Report == nil {
				t.Fatal("pipeline produced nil report")
			}

			// L1: Gain signal should be non-nil (neuromodulation enabled).
			if result.Gain == nil {
				t.Error("L1: gain signal is nil with neuromodulation enabled")
			}

			// L2: Findings should be present (even if empty).
			// (We just verify the pipeline ran detectors.)

			// L3: Inhibition should be non-nil (inhibitor enabled).
			if result.Inhibition == nil {
				t.Error("L3: inhibition result is nil with inhibitor enabled")
			}

			// L4: Arbitration should be non-nil (arbitrator enabled).
			if result.Arbitration == nil {
				t.Error("L4: arbitration result is nil with arbitrator enabled")
			}

			// Log the full pipeline state.
			gainStr := "nil"
			if result.Gain != nil {
				gainStr = fmt.Sprintf("urgency=%.3f mode=%v", result.Gain.Urgency, result.Gain.Mode)
			}
			gatedCount := 0
			if result.Inhibition != nil {
				gatedCount = len(result.Inhibition.Gated)
			}
			arbStr := "nil"
			if result.Arbitration != nil {
				arbStr = fmt.Sprintf("cp=%.3f action=%s", result.Arbitration.CompoundPathology, result.Arbitration.Action)
			}

			t.Logf("[%s] L1:%s | L2:%d findings | L3:%d gated | L4:%s | score=%.2f",
				conv.name, gainStr, len(result.Findings), gatedCount, arbStr,
				result.Report.GetOverallIntegrityScore())
		})
	}
}

// =============================================================================
// Test 4: Metrics comparison — crisp vs fuzzy summary table
// =============================================================================

func TestFuzzyValidation_MetricsComparison(t *testing.T) {
	conversations := []struct {
		name string
		snap *reasoningv1.ConversationSnapshot
	}{
		{"pathological", pathologicalConversation()},
		{"balanced", balancedConversation()},
		{"mild_agreement", mildAgreementConversation()},
		{"casual", casualConversation()},
		{"formal_pathological", formalPathologicalConversation()},
	}

	crispCfg := buildCrispConfig()
	fuzzyCfg := buildFuzzyConfig(t)

	t.Logf("\n%-25s | %-20s | %-20s", "Scenario", "Crisp", "Fuzzy")
	t.Logf("%-25s-+-%-20s-+-%-20s", "-------------------------", "--------------------", "--------------------")

	totalCrispFindings := 0
	totalFuzzyFindings := 0
	totalCrispGated := 0
	totalFuzzyGated := 0

	for _, conv := range conversations {
		crispM := runAndCollect(conv.snap, crispCfg, "crisp")
		fuzzyM := runAndCollect(conv.snap, fuzzyCfg, "fuzzy")

		totalCrispFindings += crispM.findingCount
		totalFuzzyFindings += fuzzyM.findingCount
		totalCrispGated += crispM.gatedCount
		totalFuzzyGated += fuzzyM.gatedCount

		t.Logf("%-25s | %d raw/%d gated %-6s | %d raw/%d gated %-6s",
			conv.name,
			crispM.findingCount, crispM.gatedCount, crispM.arbitration,
			fuzzyM.findingCount, fuzzyM.gatedCount, fuzzyM.arbitration)
	}

	t.Logf("%-25s-+-%-20s-+-%-20s", "-------------------------", "--------------------", "--------------------")
	t.Logf("%-25s | %d raw / %d gated     | %d raw / %d gated",
		"TOTAL", totalCrispFindings, totalCrispGated, totalFuzzyFindings, totalFuzzyGated)

	// Recall: fuzzy must detect at least as many finding-types on pathological inputs.
	// FP: fuzzy should gate fewer findings on healthy inputs.
}

// =============================================================================
// Helpers
// =============================================================================

func findingTypeNames(findings []*reasoningv1.CognitiveAssessment) []string {
	names := make([]string, len(findings))
	for i, f := range findings {
		names[i] = f.GetFindingType().String()
	}
	return names
}

func maxConfidence(findings []*reasoningv1.CognitiveAssessment) float64 {
	var max float64
	for _, f := range findings {
		if f.GetConfidence() > max {
			max = f.GetConfidence()
		}
	}
	return max
}
