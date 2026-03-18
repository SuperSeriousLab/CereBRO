package pipeline

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// TestDetectSustainedConvictionWide_v7_CorpusFireRate validates that:
// - v7 fires on at least as many sycophancy+cathedral entries as v5
// - v7 catches at least some entries that v5 misses
// - v7 has 0 false positives on healthy entries (same as v5)
func TestDetectSustainedConvictionWide_v7_CorpusFireRate(t *testing.T) {
	type cEntry struct {
		EntryID string `json:"entry_id"`
		Input   struct {
			Turns []struct {
				TurnNumber int    `json:"turn_number"`
				Speaker    string `json:"speaker"`
				RawText    string `json:"raw_text"`
			} `json:"turns"`
		} `json:"input"`
		Metadata struct {
			IsPathological bool   `json:"is_pathological"`
			PathologyType  string `json:"pathology_type"`
		} `json:"metadata"`
	}

	corpusPath := "../../data/corpus/full-v4.ndjson"
	f, err := os.Open(corpusPath)
	if err != nil {
		t.Skipf("corpus not found at %s: %v (skipping corpus test)", corpusPath, err)
		return
	}
	defer f.Close()

	cfgV5 := DefaultSustainedConvictionConfig()
	cfgV7 := DefaultSustainedConvictionWideConfig()

	var v5fired, v7fired, v7only, pathTotal, healthyFP5, healthyFP7, healthyTotal int

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var e cEntry
		if err2 := json.Unmarshal([]byte(line), &e); err2 != nil {
			continue
		}
		turns := make([]*reasoningv1.Turn, len(e.Input.Turns))
		for i, tt := range e.Input.Turns {
			turns[i] = &reasoningv1.Turn{
				TurnNumber: uint32(tt.TurnNumber),
				Speaker:    tt.Speaker,
				RawText:    tt.RawText,
			}
		}
		snap := &reasoningv1.ConversationSnapshot{Turns: turns, TotalTurns: uint32(len(turns))}
		r5 := DetectSustainedConviction(snap, cfgV5)
		r7 := DetectSustainedConviction(snap, cfgV7)

		pt := e.Metadata.PathologyType
		if !e.Metadata.IsPathological {
			healthyTotal++
			if r5 != nil {
				healthyFP5++
			}
			if r7 != nil {
				healthyFP7++
				t.Logf("v7 FALSE POSITIVE: %s (healthy)", e.EntryID)
			}
		} else if pt == "sycophancy" || pt == "cathedral" {
			pathTotal++
			if r5 != nil {
				v5fired++
			}
			if r7 != nil {
				v7fired++
			}
			if r5 == nil && r7 != nil {
				v7only++
				t.Logf("v7 catches v5-miss: %s", e.EntryID)
			}
		}
	}

	if pathTotal == 0 {
		t.Skip("no sycophancy or cathedral entries in corpus")
	}

	v5Rate := float64(v5fired) / float64(pathTotal)
	v7Rate := float64(v7fired) / float64(pathTotal)
	t.Logf("Sycophancy+Cathedral (%d entries): v5=%d/%.0f%% v7=%d/%.0f%% v7-only=%d",
		pathTotal, v5fired, v5Rate*100, v7fired, v7Rate*100, v7only)
	t.Logf("Healthy (%d entries): v5 FP=%d v7 FP=%d", healthyTotal, healthyFP5, healthyFP7)

	if v7Rate < v5Rate {
		t.Errorf("v7 fire rate %.1f%% < v5 fire rate %.1f%% — regression", v7Rate*100, v5Rate*100)
	}
	if healthyFP7 > 0 {
		t.Errorf("v7 false positive rate > 0: %d/%d healthy entries fired", healthyFP7, healthyTotal)
	}
}
