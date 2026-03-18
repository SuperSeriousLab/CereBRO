package pipeline

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// TestV7_FPDiagnosis runs the v5 and v7 detectors directly on each test conversation
// to see which one triggers the extra FP.
func TestV7_FPDiagnosis(t *testing.T) {
	type corpusEntry struct {
		EntryID  string `json:"entry_id"`
		Input    struct {
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

	loadFile := func(path string) []corpusEntry {
		f, err := os.Open(path)
		if err != nil {
			t.Logf("skip %s: %v", path, err)
			return nil
		}
		defer f.Close()
		var entries []corpusEntry
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var e corpusEntry
			if err2 := json.Unmarshal([]byte(line), &e); err2 != nil {
				continue
			}
			entries = append(entries, e)
		}
		return entries
	}

	buildSnap := func(e corpusEntry) *reasoningv1.ConversationSnapshot {
		turns := make([]*reasoningv1.Turn, len(e.Input.Turns))
		for i, tt := range e.Input.Turns {
			turns[i] = &reasoningv1.Turn{
				TurnNumber: uint32(tt.TurnNumber),
				Speaker:    tt.Speaker,
				RawText:    tt.RawText,
			}
		}
		return &reasoningv1.ConversationSnapshot{Turns: turns, TotalTurns: uint32(len(turns))}
	}

	cfgV5 := DefaultSustainedConvictionConfig()
	cfgV7 := DefaultSustainedConvictionWideConfig()

	testFiles := []string{
		"../../data/test-conversations/01-anchoring.ndjson",
		"../../data/test-conversations/02-sunk-cost.ndjson",
		"../../data/test-conversations/03-contradiction.ndjson",
		"../../data/test-conversations/04-scope-drift.ndjson",
		"../../data/test-conversations/05-confidence.ndjson",
		"../../data/test-conversations/06-multi-failure.ndjson",
		"../../data/test-conversations/07-clean.ndjson",
		"../../data/test-conversations/08-borderline.ndjson",
		"../../data/test-conversations/09-feedback-trigger.ndjson",
	}

	fmt.Println("entry_id                    path            v5-fired  v7-fired  v7-conf")
	for _, path := range testFiles {
		entries := loadFile(path)
		for _, e := range entries {
			snap := buildSnap(e)
			r5 := DetectSustainedConviction(snap, cfgV5)
			r7 := DetectSustainedConviction(snap, cfgV7)
			v5f := r5 != nil
			v7f := r7 != nil
			label := "healthy"
			if e.Metadata.IsPathological {
				label = e.Metadata.PathologyType
			}
			v7conf := 0.0
			if r7 != nil {
				v7conf = float64(r7.GetConfidence())
			}
			fmt.Printf("%-28s [%-12s] v5=%-5v v7=%-5v v7conf=%.3f\n",
				e.EntryID, label, v5f, v7f, v7conf)
		}
	}
}

// TestV7_PipelineFPDiagnosis runs the full pipeline (PreCortex config) on each test
// conversation and shows which one produces an extra FP.
func TestV7_PipelineFPDiagnosis(t *testing.T) {
	convDir := "../../data/test-conversations"
	if _, err := os.Stat(convDir); os.IsNotExist(err) {
		t.Skip("test-conversations not found")
	}

	entries := loadCompetitionEntries(t, convDir)
	cfg := PreCortexConfig()

	fmt.Println("\nPipeline FP diagnosis:")
	for _, e := range entries {
		tp, fp, fn := runAndScore(e.Snap, cfg, e.Expected)
		fmt.Printf("  %s: TP=%d FP=%d FN=%d\n", e.ID, tp, fp, fn)
	}
}
