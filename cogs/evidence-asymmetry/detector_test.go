package main

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// newSnap is a helper to build a ConversationSnapshot from turn data.
func newSnap(turns []struct {
	num     uint32
	speaker string
	text    string
}) *reasoningv1.ConversationSnapshot {
	var ts []*reasoningv1.Turn
	for _, t := range turns {
		ts = append(ts, &reasoningv1.Turn{
			TurnNumber: t.num,
			Speaker:    t.speaker,
			RawText:    t.text,
		})
	}
	return &reasoningv1.ConversationSnapshot{
		Turns:      ts,
		TotalTurns: uint32(len(ts)),
	}
}

// TestRun_ConfidenceMiscalibration verifies the detector fires when the assistant
// makes high-certainty positive claims with NO evidence, while separate negative
// turns contain proper evidence links. This is the structural signature of gen4_78+gen4_86.
//
// Turn design:
//   - Positive turns: ONLY certainty assertions, NO "however"/"downside" markers
//   - Negative turns: ONLY counter-claims WITH evidence markers
func TestRun_ConfidenceMiscalibration(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "I'm getting high latency on my API. Response times around 300ms."},
		// Positive turn: certainty markers, zero evidence links
		{2, "assistant", "It is definitely the database. The root cause is missing indexes. It is certainly the query layer. Almost certainly a statistics issue."},
		{3, "user", "I added an index but latency is still 280ms. Could it be the network?"},
		// Negative turn: counter-claim WITH evidence markers
		{4, "assistant", "The downside of focusing only on indexes is that it misses other causes. The challenge is that the query plan changes since statistics are stale. The concern is that autovacuum has not run recently, because outdated stats cause the optimizer to choose nested loops."},
		{5, "user", "Are you sure? The database logs show no slow queries."},
		// Positive turn: more certainty, zero evidence
		{6, "assistant", "It is definitely a statistics issue. The root cause is clearly the vacuum settings. Almost certainly the configuration needs tuning. Is always the case with heavy write workloads."},
		{7, "user", "What about caching?"},
		// Negative turn: counter WITH evidence
		{8, "assistant", "The downside of caching is that it masks the root cause. The risk is data staleness since caches serve stale data when the database changes. The concern is invalidation complexity, because cache coherence requires additional infrastructure due to distributed writes."},
	})

	cfg := DefaultConfig()
	result := Run(snap, cfg)

	if result == nil {
		t.Fatal("expected a finding for confidence_miscalibration conversation, got nil")
	}
	if result.GetFindingType() != reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION {
		t.Errorf("expected CONFIDENCE_MISCALIBRATION, got %v", result.GetFindingType())
	}
	if result.GetConfidence() < 0.4 {
		t.Errorf("expected confidence >= 0.4, got %.3f", result.GetConfidence())
	}
	if result.GetDetectorName() != "evidence-asymmetry-detector" {
		t.Errorf("expected detector name 'evidence-asymmetry-detector', got %q", result.GetDetectorName())
	}
}

// TestRun_Healthy verifies the detector does NOT fire on a well-balanced
// conversation where the assistant provides evidence for both positive and
// negative claims proportionally.
func TestRun_Healthy(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "I'm designing an API for our e-commerce platform with Node.js."},
		// Positive with evidence
		{2, "assistant", "Node.js is an excellent choice for this use case because it handles concurrent connections efficiently. However, the downside is that CPU-bound tasks are slow since Node is single-threaded."},
		{3, "user", "We're thinking about Kafka for messaging."},
		// Positive with evidence; negative with evidence
		{4, "assistant", "Kafka is definitely the right choice for resilient messaging because it provides at-least-once delivery guarantees. However, the challenge is operational complexity since you need Zookeeper and because partition management requires careful tuning."},
		{5, "user", "What about data consistency with eventual consistency?"},
		// Balanced: positive with evidence, negative with reason
		{6, "assistant", "It is definitely the best model for high-scale systems because eventual consistency reduces coordination overhead. However, the problem is stale reads since you may see outdated data due to replication lag."},
	})

	cfg := DefaultConfig()
	result := Run(snap, cfg)

	if result != nil {
		t.Errorf("expected nil for healthy conversation, got finding: %v (confidence=%.3f, explanation=%s)",
			result.GetFindingType(), result.GetConfidence(), result.GetExplanation())
	}
}

// TestRun_NilSnapshot verifies nil safety.
func TestRun_NilSnapshot(t *testing.T) {
	cfg := DefaultConfig()
	result := Run(nil, cfg)
	if result != nil {
		t.Error("expected nil result for nil snapshot")
	}
}

// TestRun_InsufficientTurns verifies the detector does not fire on very short
// conversations where there is only one assistant turn (below MinAssistantTurns=2).
func TestRun_InsufficientTurns(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "What is the best database?"},
		{2, "assistant", "It is definitely PostgreSQL. It is certainly the best. It is clearly the most reliable."},
	})

	cfg := DefaultConfig()
	result := Run(snap, cfg)

	// With only 1 assistant turn (below MinAssistantTurns=2), should not fire.
	if result != nil {
		t.Errorf("expected nil for single-turn conversation, got %v", result.GetFindingType())
	}
}

// TestRun_ExtremeAsymmetry verifies CRITICAL severity on extreme ratios where
// the agent makes many unevidenced positive claims and rich-evidence negatives.
func TestRun_ExtremeAsymmetry(t *testing.T) {
	snap := newSnap([]struct {
		num     uint32
		speaker string
		text    string
	}{
		{1, "user", "Should I use microservices?"},
		// Many positive claims — zero evidence
		{2, "assistant", "Microservices is certainly the best approach. It is definitely correct for your scale. It is clearly the right architecture. It is always the answer for e-commerce. It is undoubtedly superior."},
		{3, "user", "What are the downsides?"},
		// Many negative claims — rich in evidence links
		{4, "assistant", "However, the problem is operational complexity since each service needs its own deployment pipeline. On the other hand, the challenge is observability because distributed tracing is required and since log aggregation adds overhead. The downside is latency given that network calls cross process boundaries. Unfortunately, service discovery is needed since DNS resolution must be managed and because each service registers independently."},
		{5, "user", "Is there a better option?"},
		// More positive (no evidence), more negative (evidence)
		{6, "assistant", "Monolith is certainly not better. It is definitely the worst approach. It is clearly outdated. However, since monoliths do perform better for simple apps because database calls are local, and given that deployment is simpler due to a single artifact, they have merits when the team is small."},
	})

	cfg := DefaultConfig()
	result := Run(snap, cfg)

	if result == nil {
		t.Fatal("expected finding for extreme asymmetry, got nil")
	}
	if result.GetFindingType() != reasoningv1.FindingType_CONFIDENCE_MISCALIBRATION {
		t.Errorf("expected CONFIDENCE_MISCALIBRATION, got %v", result.GetFindingType())
	}
}

// TestDefaultConfig verifies the default configuration has sensible values.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MiscalibrationThreshold != 1.5 {
		t.Errorf("expected MiscalibrationThreshold=1.5, got %.1f", cfg.MiscalibrationThreshold)
	}
	if cfg.BorderlineThreshold != 1.0 {
		t.Errorf("expected BorderlineThreshold=1.0, got %.1f", cfg.BorderlineThreshold)
	}
	if cfg.MinAssistantTurns != 2 {
		t.Errorf("expected MinAssistantTurns=2, got %d", cfg.MinAssistantTurns)
	}
}

// corpusEntry is the subset of fields needed from the NDJSON test corpus.
type corpusEntry struct {
	EntryID string `json:"entry_id"`
	Input   struct {
		Turns []struct {
			TurnNumber uint32 `json:"turn_number"`
			Speaker    string `json:"speaker"`
			RawText    string `json:"raw_text"`
		} `json:"turns"`
	} `json:"input"`
	Metadata struct {
		PathologyType  string `json:"pathology_type"`
		IsPathological bool   `json:"is_pathological"`
	} `json:"metadata"`
}

// TestCorpus_FireRate verifies the detector fires on confidence_miscalibration
// entries and does not fire excessively on healthy entries.
// This test reads the full-v4.ndjson corpus from the CereBRO data directory.
func TestCorpus_FireRate(t *testing.T) {
	corpusPath := "../../data/corpus/full-v4.ndjson"
	f, err := os.Open(corpusPath)
	if err != nil {
		t.Skipf("corpus not available at %s: %v", corpusPath, err)
	}
	defer f.Close()

	cfg := DefaultConfig()

	cmFired := 0
	cmTotal := 0
	hFP := 0
	hTotal := 0

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, len(buf))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry corpusEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Logf("skip malformed entry: %v", err)
			continue
		}

		var turns []*reasoningv1.Turn
		for _, tr := range entry.Input.Turns {
			turns = append(turns, &reasoningv1.Turn{
				TurnNumber: tr.TurnNumber,
				Speaker:    tr.Speaker,
				RawText:    tr.RawText,
			})
		}
		snap := &reasoningv1.ConversationSnapshot{
			Turns:      turns,
			TotalTurns: uint32(len(turns)),
		}

		result := Run(snap, cfg)
		fired := result != nil

		pt := entry.Metadata.PathologyType
		switch pt {
		case "confidence_miscalibration":
			cmTotal++
			if fired {
				cmFired++
			} else {
				t.Logf("MISS: %s (confidence_miscalibration)", entry.EntryID)
			}
		case "healthy":
			hTotal++
			if fired {
				hFP++
				t.Logf("FALSE POSITIVE: %s (healthy)", entry.EntryID)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

	if cmTotal == 0 {
		t.Fatal("no confidence_miscalibration entries found in corpus")
	}
	if hTotal == 0 {
		t.Fatal("no healthy entries found in corpus")
	}

	fireRate := float64(cmFired) / float64(cmTotal)
	fpRate := float64(hFP) / float64(hTotal)

	t.Logf("Confidence miscalibration fire rate: %d/%d (%.0f%%)", cmFired, cmTotal, fireRate*100)
	t.Logf("Healthy false positive rate: %d/%d (%.0f%%)", hFP, hTotal, fpRate*100)

	// Accept any fire rate > 0 on miscalibration as a pass (detector adds signal)
	// and FP rate <= 50% on healthy entries.
	if cmFired == 0 {
		t.Errorf("detector fired 0/%d times on confidence_miscalibration entries", cmTotal)
	}
	if fpRate > 0.5 {
		t.Errorf("false positive rate %.0f%% exceeds 50%% threshold on healthy entries", fpRate*100)
	}
}
