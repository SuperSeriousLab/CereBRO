// Architecture Competition — Phase 6
//
// Runs 5 pipeline variants against the test corpus, measures accuracy + performance
// + complexity traits, scores under 4 trait profiles, computes Pareto frontier.
package pipeline

import (
	"math"
	"sort"
	"time"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// findingTypeStr returns the string name of a finding type for scoring.
func findingTypeStr(ft reasoningv1.FindingType) string {
	return ft.String()
}

// CompetitionEntry is a test case with expected findings.
type CompetitionEntry struct {
	ID       string
	Snap     *reasoningv1.ConversationSnapshot
	Expected []string // expected finding type strings
}

// VariantResult holds measured traits for one variant.
type VariantResult struct {
	Info   VariantInfo
	Traits map[string]float64 // trait name → value
}

// CompetitionResult holds the full competition output.
type CompetitionResult struct {
	Variants       []VariantResult
	ProfileWinners map[string]string // profile name → winner variant name
	Frontier       []string          // Pareto-optimal variant names
}

// TraitDirection indicates whether a trait should be maximized or minimized.
type TraitDirection int

const (
	Maximize TraitDirection = iota
	Minimize
)

// TraitSpec defines a trait with its optimization direction.
type TraitSpec struct {
	Name      string
	Direction TraitDirection
}

// AllTraitSpecs returns the trait specifications for the competition.
func AllTraitSpecs() []TraitSpec {
	return []TraitSpec{
		{"precision", Maximize},
		{"recall", Maximize},
		{"f1", Maximize},
		{"false_positive_rate", Minimize},
		{"latency_mean_ms", Minimize},
		{"latency_p95_ms", Minimize},
		{"latency_p99_ms", Minimize},
		{"stage_count", Minimize},
		{"cog_count", Minimize},
	}
}

// TraitProfile defines weighted scoring for a deployment scenario.
type TraitProfile struct {
	Name    string
	Weights map[string]float64 // trait name → weight (must sum to ~1.0)
}

// AllProfiles returns the 4 evaluation profiles.
func AllProfiles() []TraitProfile {
	return []TraitProfile{
		{
			Name: "balanced",
			Weights: map[string]float64{
				"f1":              0.35,
				"precision":       0.15,
				"recall":          0.15,
				"latency_mean_ms": 0.15,
				"latency_p95_ms":  0.10,
				"stage_count":     0.05,
				"cog_count":       0.05,
			},
		},
		{
			Name: "precision-first",
			Weights: map[string]float64{
				"precision":          0.40,
				"f1":                 0.20,
				"false_positive_rate": 0.20,
				"recall":             0.10,
				"latency_mean_ms":    0.05,
				"stage_count":        0.05,
			},
		},
		{
			Name: "recall-first",
			Weights: map[string]float64{
				"recall":          0.40,
				"f1":              0.25,
				"precision":       0.15,
				"latency_mean_ms": 0.10,
				"stage_count":     0.05,
				"cog_count":       0.05,
			},
		},
		{
			Name: "minimal",
			Weights: map[string]float64{
				"f1":              0.25,
				"latency_mean_ms": 0.20,
				"stage_count":     0.20,
				"cog_count":       0.15,
				"precision":       0.10,
				"recall":          0.10,
			},
		},
	}
}

// RunCompetition executes all variants against the given entries and returns results.
func RunCompetition(entries []CompetitionEntry, variants []ArchVariant) *CompetitionResult {
	var results []VariantResult

	for _, v := range variants {
		traits := measureVariant(v, entries)
		results = append(results, VariantResult{
			Info:   v.Info,
			Traits: traits,
		})
	}

	// Score under each profile.
	specs := AllTraitSpecs()
	profiles := AllProfiles()
	profileWinners := make(map[string]string)

	for _, profile := range profiles {
		winner := scoreProfile(results, profile, specs)
		profileWinners[profile.Name] = winner
	}

	// Compute Pareto frontier.
	frontier := computePareto(results, specs)

	return &CompetitionResult{
		Variants:       results,
		ProfileWinners: profileWinners,
		Frontier:       frontier,
	}
}

// measureVariant runs all entries through a variant and collects traits.
func measureVariant(v ArchVariant, entries []CompetitionEntry) map[string]float64 {
	var totalTP, totalFP, totalFN int
	var cleanEntries, cleanWithFindings int
	var latencies []float64

	for _, entry := range entries {
		start := time.Now()
		result := Run(entry.Snap, v.Config)
		elapsed := time.Since(start)
		latencies = append(latencies, float64(elapsed.Microseconds())/1000.0) // ms

		// Collect actual finding types.
		actualTypes := make(map[string]bool)
		for _, finding := range result.Findings {
			actualTypes[findingTypeStr(finding.FindingType)] = true
		}

		// If inhibitor is enabled, use the report findings (post-inhibition).
		if v.Config.UseInhibitor && result.Report != nil {
			actualTypes = make(map[string]bool)
			for _, finding := range result.Report.GetFindings() {
				actualTypes[findingTypeStr(finding.FindingType)] = true
			}
		}

		expectedTypes := make(map[string]bool)
		for _, et := range entry.Expected {
			expectedTypes[et] = true
		}

		// TP, FN, FP
		for et := range expectedTypes {
			if actualTypes[et] {
				totalTP++
			} else {
				totalFN++
			}
		}
		for at := range actualTypes {
			if !expectedTypes[at] {
				totalFP++
			}
		}

		// Track clean entries for FPR.
		if len(entry.Expected) == 0 {
			cleanEntries++
			if len(actualTypes) > 0 {
				cleanWithFindings++
			}
		}
	}

	// Compute accuracy traits.
	precision := 0.0
	if totalTP+totalFP > 0 {
		precision = float64(totalTP) / float64(totalTP+totalFP)
	}
	recall := 0.0
	if totalTP+totalFN > 0 {
		recall = float64(totalTP) / float64(totalTP+totalFN)
	}
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	fpr := 0.0
	if cleanEntries > 0 {
		fpr = float64(cleanWithFindings) / float64(cleanEntries)
	}

	// Compute latency traits.
	sort.Float64s(latencies)
	latencyMean := mean(latencies)
	latencyP95 := percentile(latencies, 0.95)
	latencyP99 := percentile(latencies, 0.99)

	return map[string]float64{
		"precision":          precision,
		"recall":             recall,
		"f1":                 f1,
		"false_positive_rate": fpr,
		"latency_mean_ms":    latencyMean,
		"latency_p95_ms":     latencyP95,
		"latency_p99_ms":     latencyP99,
		"stage_count":        float64(v.Info.StageCount),
		"cog_count":          float64(v.Info.CogCount),
		"true_positives":     float64(totalTP),
		"false_positives":    float64(totalFP),
		"false_negatives":    float64(totalFN),
	}
}

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

// scoreProfile scores all variants under a profile and returns the winner's name.
func scoreProfile(results []VariantResult, profile TraitProfile, specs []TraitSpec) string {
	// Build trait direction map.
	dirMap := make(map[string]TraitDirection)
	for _, s := range specs {
		dirMap[s.Name] = s.Direction
	}

	// Find max values for normalization.
	maxVals := make(map[string]float64)
	for _, trait := range specs {
		for _, r := range results {
			if v, ok := r.Traits[trait.Name]; ok && v > maxVals[trait.Name] {
				maxVals[trait.Name] = v
			}
		}
	}

	// Score each variant.
	bestScore := -1.0
	bestName := ""

	for _, r := range results {
		score := 0.0
		for trait, weight := range profile.Weights {
			raw := r.Traits[trait]
			maxV := maxVals[trait]
			norm := normalizeTrait(raw, maxV, dirMap[trait])
			score += weight * norm
		}
		if score > bestScore {
			bestScore = score
			bestName = r.Info.Name
		}
	}
	return bestName
}

// ScoreAllProfiles returns per-variant scores for each profile.
func ScoreAllProfiles(results []VariantResult) map[string]map[string]float64 {
	specs := AllTraitSpecs()
	dirMap := make(map[string]TraitDirection)
	for _, s := range specs {
		dirMap[s.Name] = s.Direction
	}

	maxVals := make(map[string]float64)
	for _, trait := range specs {
		for _, r := range results {
			if v, ok := r.Traits[trait.Name]; ok && v > maxVals[trait.Name] {
				maxVals[trait.Name] = v
			}
		}
	}

	out := make(map[string]map[string]float64) // profile → variant → score
	for _, profile := range AllProfiles() {
		scores := make(map[string]float64)
		for _, r := range results {
			score := 0.0
			for trait, weight := range profile.Weights {
				raw := r.Traits[trait]
				maxV := maxVals[trait]
				norm := normalizeTrait(raw, maxV, dirMap[trait])
				score += weight * norm
			}
			scores[r.Info.Name] = score
		}
		out[profile.Name] = scores
	}
	return out
}

func normalizeTrait(raw, maxVal float64, dir TraitDirection) float64 {
	if maxVal == 0 {
		return 0
	}
	norm := raw / maxVal
	if dir == Minimize {
		norm = 1 - norm
	}
	return norm
}

// computePareto returns variant names on the Pareto frontier.
// A variant is dominated if another beats or ties it on ALL traits
// and strictly beats it on at least one.
func computePareto(results []VariantResult, specs []TraitSpec) []string {
	dirMap := make(map[string]TraitDirection)
	for _, s := range specs {
		dirMap[s.Name] = s.Direction
	}

	// Normalize all trait values.
	maxVals := make(map[string]float64)
	for _, s := range specs {
		for _, r := range results {
			if v := r.Traits[s.Name]; v > maxVals[s.Name] {
				maxVals[s.Name] = v
			}
		}
	}

	type normalized struct {
		name  string
		norms map[string]float64
	}

	var norms []normalized
	for _, r := range results {
		n := normalized{name: r.Info.Name, norms: make(map[string]float64)}
		for _, s := range specs {
			n.norms[s.Name] = normalizeTrait(r.Traits[s.Name], maxVals[s.Name], dirMap[s.Name])
		}
		norms = append(norms, n)
	}

	// Check domination.
	var frontier []string
	for i, candidate := range norms {
		dominated := false
		for j, other := range norms {
			if i == j {
				continue
			}
			if dominates(other.norms, candidate.norms, specs) {
				dominated = true
				break
			}
		}
		if !dominated {
			frontier = append(frontier, candidate.name)
		}
	}
	return frontier
}

// dominates returns true if a beats b on every normalized trait (≥) and strictly on at least one (>).
func dominates(a, b map[string]float64, specs []TraitSpec) bool {
	const eps = 1e-9
	strictlyBetter := false
	for _, s := range specs {
		av := a[s.Name]
		bv := b[s.Name]
		if av < bv-eps {
			return false // a is worse on this trait
		}
		if av > bv+eps {
			strictlyBetter = true
		}
	}
	return strictlyBetter
}
