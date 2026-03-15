package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// miniCorpus is a 3-entry corpus written inline.
// Entry 1: anchoring bias (numeric estimates cluster around initial value).
// Entry 2: sunk cost fallacy (explicit "too far in to quit" phrasing).
// Entry 3: clean conversation — no expected findings (tests FP accounting).
const miniCorpus = `{"entry_id":"test-001","input":{"turns":[{"turn_number":1,"speaker":"alice","raw_text":"The vendor quote came in at $100,000 for the project."},{"turn_number":2,"speaker":"bob","raw_text":"That seems about right. I had $95,000 in mind."},{"turn_number":3,"speaker":"alice","raw_text":"Let's budget $102,000 and negotiate from there."},{"turn_number":4,"speaker":"bob","raw_text":"Agreed. Maybe $98,000 if we cut scope."}],"objective":"Set project budget","total_turns":4},"expected":[{"finding_type":"ANCHORING_BIAS"}]}
{"entry_id":"test-002","input":{"turns":[{"turn_number":1,"speaker":"participant-1","raw_text":"We have spent 18 months and $600,000 building this platform and it still isn't working."},{"turn_number":2,"speaker":"participant-2","raw_text":"Should we switch to an off-the-shelf solution?"},{"turn_number":3,"speaker":"participant-1","raw_text":"We have invested too much to abandon it now."},{"turn_number":4,"speaker":"participant-2","raw_text":"I understand, but continuing may cost even more."},{"turn_number":5,"speaker":"participant-1","raw_text":"We are too far in to turn back. Let us hire two more engineers."}],"objective":"Evaluate platform build vs buy decision","total_turns":5},"expected":[{"finding_type":"SUNK_COST_FALLACY"}]}
{"entry_id":"test-003","input":{"turns":[{"turn_number":1,"speaker":"alice","raw_text":"The deployment is scheduled for Friday."},{"turn_number":2,"speaker":"bob","raw_text":"I will have the tests ready by Thursday."},{"turn_number":3,"speaker":"alice","raw_text":"Perfect. I will coordinate with ops."}],"objective":"Plan deployment schedule","total_turns":3},"expected":[]}
`

func writeTempCorpus(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test-corpus.ndjson")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp corpus: %v", err)
	}
	return path
}

func TestRunEvalDefaultParams(t *testing.T) {
	corpusPath := writeTempCorpus(t, miniCorpus)

	result, err := runEval(corpusPath, map[string]string{})
	if err != nil {
		t.Fatalf("runEval: %v", err)
	}

	// Verify output is structurally valid JSON (round-trip through marshal/unmarshal).
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var decoded evalResult
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Verify required fields are present.
	requiredTraits := []string{"precision", "recall", "f1", "tp", "fp", "fn"}
	for _, key := range requiredTraits {
		if _, ok := decoded.Traits[key]; !ok {
			t.Errorf("missing trait %q in output", key)
		}
	}

	// Fitness must equal the f1 trait.
	if decoded.Fitness != decoded.Traits["f1"] {
		t.Errorf("fitness (%f) does not match traits.f1 (%f)", decoded.Fitness, decoded.Traits["f1"])
	}

	// Fitness must be in [0, 1].
	if decoded.Fitness < 0 || decoded.Fitness > 1 {
		t.Errorf("fitness %f out of range [0,1]", decoded.Fitness)
	}

	t.Logf("result: fitness=%.4f precision=%.4f recall=%.4f f1=%.4f tp=%.0f fp=%.0f fn=%.0f",
		decoded.Fitness,
		decoded.Traits["precision"],
		decoded.Traits["recall"],
		decoded.Traits["f1"],
		decoded.Traits["tp"],
		decoded.Traits["fp"],
		decoded.Traits["fn"],
	)
}

func TestRunEvalMalformedEntriesSkipped(t *testing.T) {
	// Mix of valid and invalid lines — malformed lines must be skipped silently.
	corpus := `{"entry_id":"ok-001","input":{"turns":[{"turn_number":1,"speaker":"a","raw_text":"The estimate is $50,000."},{"turn_number":2,"speaker":"b","raw_text":"I was thinking $48,000."},{"turn_number":3,"speaker":"a","raw_text":"Let us go with $51,000."},{"turn_number":4,"speaker":"b","raw_text":"Close to $50,000 works."}],"objective":"Set estimate","total_turns":4},"expected":[{"finding_type":"ANCHORING_BIAS"}]}
not valid json at all
{"entry_id":"","input":{"turns":[],"objective":"","total_turns":0},"expected":[]}
{"entry_id":"ok-002","input":{"turns":[{"turn_number":1,"speaker":"x","raw_text":"We are planning the quarterly review."},{"turn_number":2,"speaker":"y","raw_text":"Sounds good."}],"objective":"Quarterly planning","total_turns":2},"expected":[]}
`
	corpusPath := writeTempCorpus(t, corpus)
	result, err := runEval(corpusPath, map[string]string{})
	if err != nil {
		t.Fatalf("runEval with malformed entries: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Should not panic and fitness must be in range.
	if result.Fitness < 0 || result.Fitness > 1 {
		t.Errorf("fitness %f out of range", result.Fitness)
	}
}

func TestRunEvalParamOverrides(t *testing.T) {
	corpusPath := writeTempCorpus(t, miniCorpus)

	defaultResult, err := runEval(corpusPath, map[string]string{})
	if err != nil {
		t.Fatalf("default runEval: %v", err)
	}

	// Very permissive drift_threshold should not panic.
	looseResult, err := runEval(corpusPath, map[string]string{
		"drift_threshold": "0.50",
		"sustained_turns": "2",
	})
	if err != nil {
		t.Fatalf("loose runEval: %v", err)
	}
	if looseResult.Fitness < 0 || looseResult.Fitness > 1 {
		t.Errorf("loose fitness %f out of range", looseResult.Fitness)
	}

	t.Logf("default fitness=%.4f  loose fitness=%.4f",
		defaultResult.Fitness, looseResult.Fitness)
}

func TestCorpusLoading(t *testing.T) {
	corpusPath := writeTempCorpus(t, miniCorpus)
	entries, err := LoadCorpus(corpusPath)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].EntryID != "test-001" {
		t.Errorf("unexpected first entry_id: %q", entries[0].EntryID)
	}
	if len(entries[0].Expected) != 1 || entries[0].Expected[0].FindingType != "ANCHORING_BIAS" {
		t.Errorf("unexpected expected findings for test-001: %+v", entries[0].Expected)
	}
	// test-003 has no expected findings.
	if len(entries[2].Expected) != 0 {
		t.Errorf("expected 0 expected findings for test-003, got %d", len(entries[2].Expected))
	}
}

func TestBuildConfigDefaults(t *testing.T) {
	// Empty params must produce a config identical in key fields to DefaultPipelineConfig.
	cfg := buildConfig(map[string]string{})
	def := defaultScopeGuardFields()

	if cfg.ScopeGuard.DriftThreshold != def.driftThreshold {
		t.Errorf("DriftThreshold: got %f, want %f", cfg.ScopeGuard.DriftThreshold, def.driftThreshold)
	}
	if cfg.ScopeGuard.SustainedTurns != def.sustainedTurns {
		t.Errorf("SustainedTurns: got %d, want %d", cfg.ScopeGuard.SustainedTurns, def.sustainedTurns)
	}
}

type scopeGuardDefaults struct {
	driftThreshold float64
	sustainedTurns uint32
	referenceTurns uint32
}

func defaultScopeGuardFields() scopeGuardDefaults {
	// These values mirror DefaultScopeGuardConfig() — kept in sync manually.
	return scopeGuardDefaults{
		driftThreshold: 0.79,
		sustainedTurns: 8,
		referenceTurns: 4,
	}
}
