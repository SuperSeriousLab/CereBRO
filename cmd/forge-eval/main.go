// forge-eval evaluates CereBRO's pipeline with given parameters against a corpus.
//
// Input:
//
//	--corpus=path        path to NDJSON corpus file
//	--params='{"drift_threshold":"0.79","sustained_turns":"8"}'
//	                     JSON object mapping parameter names to string values
//
// Output (stdout): JSON object
//
//	{"fitness": 0.85, "traits": {"precision": 0.9, "recall": 0.8, "f1": 0.85,
//	                             "tp": 10, "fp": 1, "fn": 2}}
//
// The binary is designed as a subprocess evaluator: GEARS' Forge launches it,
// reads stdout, and parses the fitness/traits for its evolutionary loop.
// No GEARS dependency — standalone, no reverse dependency.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func main() {
	corpusPath := flag.String("corpus", "", "path to NDJSON corpus file (required)")
	paramsJSON := flag.String("params", "{}", "JSON object of parameter name→string value overrides")
	flag.Parse()

	if *corpusPath == "" {
		log.Fatal("forge-eval: --corpus is required")
	}

	// Parse parameter overrides.
	var params map[string]string
	if err := json.Unmarshal([]byte(*paramsJSON), &params); err != nil {
		log.Fatalf("forge-eval: invalid --params JSON: %v", err)
	}

	// Build PipelineConfig from params (defaults first, then overrides).
	cfg := buildConfig(params)

	// Load corpus.
	entries, err := LoadCorpus(*corpusPath)
	if err != nil {
		log.Fatalf("forge-eval: %v", err)
	}
	if len(entries) == 0 {
		log.Fatal("forge-eval: corpus is empty or all entries were malformed")
	}

	// Evaluate.
	tp, fp, fn := evaluate(entries, cfg)

	precision := safeDiv(float64(tp), float64(tp+fp))
	recall := safeDiv(float64(tp), float64(tp+fn))
	f1 := safeDiv(2*precision*recall, precision+recall)

	out := map[string]interface{}{
		"fitness": f1,
		"traits": map[string]float64{
			"precision": precision,
			"recall":    recall,
			"f1":        f1,
			"tp":        float64(tp),
			"fp":        float64(fp),
			"fn":        float64(fn),
		},
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		log.Fatalf("forge-eval: encoding output: %v", err)
	}
}

// evaluate runs the pipeline on every corpus entry and accumulates TP/FP/FN counts.
// Each entry may declare zero or more expected finding types.
// A TP is when the pipeline emits a finding type that was expected.
// A FP is when the pipeline emits a finding type that was NOT expected.
// A FN is when an expected finding type was NOT emitted by the pipeline.
func evaluate(entries []*CorpusEntry, cfg pipeline.PipelineConfig) (tp, fp, fn int) {
	pjunmarshaler := protojson.UnmarshalOptions{DiscardUnknown: true}

	for _, entry := range entries {
		var snap reasoningv1.ConversationSnapshot
		if err := pjunmarshaler.Unmarshal(entry.Input, &snap); err != nil {
			log.Printf("forge-eval: skipping entry %q: unmarshal ConversationSnapshot: %v",
				entry.EntryID, err)
			continue
		}

		result := pipeline.Run(&snap, cfg)

		// Collect emitted finding type names.
		emitted := make(map[string]bool)
		for _, f := range result.Findings {
			name := findingTypeName(f.GetFindingType())
			if name != "" {
				emitted[name] = true
			}
		}

		// Build expected set.
		expected := make(map[string]bool)
		for _, ef := range entry.Expected {
			name := normaliseTypeName(ef.FindingType)
			if name != "" {
				expected[name] = true
			}
		}

		// TP: expected ∩ emitted
		for name := range expected {
			if emitted[name] {
				tp++
			} else {
				fn++
			}
		}
		// FP: emitted \ expected
		for name := range emitted {
			if !expected[name] {
				fp++
			}
		}
	}
	return
}

// buildConfig constructs a PipelineConfig starting from defaults and applying
// the string-valued parameter overrides supplied by the Forge.
func buildConfig(params map[string]string) pipeline.PipelineConfig {
	cfg := pipeline.DefaultPipelineConfig()

	if v, ok := params["drift_threshold"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.ScopeGuard.DriftThreshold = f
		} else {
			log.Printf("forge-eval: invalid drift_threshold %q: %v", v, err)
		}
	}
	if v, ok := params["sustained_turns"]; ok {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			cfg.ScopeGuard.SustainedTurns = uint32(n)
		} else {
			log.Printf("forge-eval: invalid sustained_turns %q: %v", v, err)
		}
	}
	if v, ok := params["reference_turns"]; ok {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			cfg.ScopeGuard.ReferenceTurns = uint32(n)
		} else {
			log.Printf("forge-eval: invalid reference_turns %q: %v", v, err)
		}
	}
	if v, ok := params["anchor_threshold"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.ConceptualAnchoring.AnchorThreshold = f
		} else {
			log.Printf("forge-eval: invalid anchor_threshold %q: %v", v, err)
		}
	}
	if v, ok := params["orbit_threshold"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.ConceptualAnchoring.OrbitThreshold = f
		} else {
			log.Printf("forge-eval: invalid orbit_threshold %q: %v", v, err)
		}
	}
	if v, ok := params["min_citations"]; ok {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			cfg.InheritedPosition.MinCitations = uint32(n)
		} else {
			log.Printf("forge-eval: invalid min_citations %q: %v", v, err)
		}
	}
	if v, ok := params["merit_ratio"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.InheritedPosition.MeritRatio = f
		} else {
			log.Printf("forge-eval: invalid merit_ratio %q: %v", v, err)
		}
	}

	return cfg
}

// findingTypeName returns the canonical all-caps name of a FindingType enum value,
// e.g. "ANCHORING_BIAS". Returns "" for FINDING_TYPE_UNSPECIFIED.
func findingTypeName(ft reasoningv1.FindingType) string {
	if ft == reasoningv1.FindingType_FINDING_TYPE_UNSPECIFIED {
		return ""
	}
	return ft.String()
}

// normaliseTypeName upper-cases a finding type string so that corpus entries
// written as "anchoring_bias" or "ANCHORING_BIAS" both match the proto enum names.
func normaliseTypeName(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

func safeDiv(num, denom float64) float64 {
	if denom == 0 {
		return 0
	}
	return num / denom
}

// outputJSON is used in tests to capture evaluation output without writing to os.Stdout.
type evalResult struct {
	Fitness float64            `json:"fitness"`
	Traits  map[string]float64 `json:"traits"`
}

func runEval(corpusPath string, params map[string]string) (*evalResult, error) {
	cfg := buildConfig(params)
	entries, err := LoadCorpus(corpusPath)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("corpus is empty or all entries were malformed")
	}

	tp, fp, fn := evaluate(entries, cfg)
	precision := safeDiv(float64(tp), float64(tp+fp))
	recall := safeDiv(float64(tp), float64(tp+fn))
	f1 := safeDiv(2*precision*recall, precision+recall)

	return &evalResult{
		Fitness: f1,
		Traits: map[string]float64{
			"precision": precision,
			"recall":    recall,
			"f1":        f1,
			"tp":        float64(tp),
			"fp":        float64(fp),
			"fn":        float64(fn),
		},
	}, nil
}
