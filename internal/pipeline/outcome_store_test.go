package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

// makeTestStore creates an OutcomeStore backed by a temp file.
func makeTestStore(t *testing.T) *OutcomeStore {
	t.Helper()
	dir := t.TempDir()
	return NewOutcomeStore(filepath.Join(dir, "outcomes.ndjson"))
}

func TestOutcomeStore_RecordAndList(t *testing.T) {
	store := makeTestStore(t)

	o := FindingOutcome{
		ID:           newUUID(),
		SessionID:    "conv-abc",
		DetectorName: "scope-guard",
		FindingType:  "SCOPE_DRIFT",
		Confidence:   0.85,
		FiredAt:      "2026-01-01T00:00:00Z",
	}
	store.Record(o)

	findings, err := store.List("", 10)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].ID != o.ID {
		t.Errorf("ID mismatch: got %q, want %q", findings[0].ID, o.ID)
	}
	if findings[0].DetectorName != "scope-guard" {
		t.Errorf("detector mismatch: got %q", findings[0].DetectorName)
	}
	if findings[0].Outcome != nil {
		t.Error("outcome should be nil before validation")
	}
}

func TestOutcomeStore_Validate_TP(t *testing.T) {
	store := makeTestStore(t)

	id := newUUID()
	store.Record(FindingOutcome{
		ID:           id,
		SessionID:    "conv-tp",
		DetectorName: "contradiction-tracker",
		FindingType:  "CONTRADICTION",
		Confidence:   0.9,
		FiredAt:      "2026-01-01T00:00:00Z",
	})

	if err := store.Validate(id, true, "confirmed in review"); err != nil {
		t.Fatalf("Validate error: %v", err)
	}

	got, err := store.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if got.Outcome == nil {
		t.Fatal("outcome should not be nil after validation")
	}
	if *got.Outcome != true {
		t.Errorf("expected TP (true), got false")
	}
	if got.OutcomeNote != "confirmed in review" {
		t.Errorf("note mismatch: %q", got.OutcomeNote)
	}
	if got.ValidatedAt == nil {
		t.Error("validated_at should be set")
	}
}

func TestOutcomeStore_Validate_FP(t *testing.T) {
	store := makeTestStore(t)

	id := newUUID()
	store.Record(FindingOutcome{
		ID:           id,
		SessionID:    "conv-fp",
		DetectorName: "anchoring-detector",
		FindingType:  "ANCHORING_BIAS",
		Confidence:   0.65,
		FiredAt:      "2026-01-01T00:00:00Z",
	})

	if err := store.Validate(id, false, "false positive — no actual anchoring"); err != nil {
		t.Fatalf("Validate error: %v", err)
	}

	got, err := store.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if *got.Outcome != false {
		t.Errorf("expected FP (false), got true")
	}
}

func TestOutcomeStore_Validate_NotFound(t *testing.T) {
	store := makeTestStore(t)
	err := store.Validate("non-existent-id", true, "")
	if err == nil {
		t.Fatal("expected error for non-existent ID")
	}
}

func TestOutcomeStore_ListFilterByDetector(t *testing.T) {
	store := makeTestStore(t)

	store.Record(FindingOutcome{ID: newUUID(), DetectorName: "scope-guard", FiredAt: "2026-01-01T00:00:00Z"})
	store.Record(FindingOutcome{ID: newUUID(), DetectorName: "contradiction-tracker", FiredAt: "2026-01-01T00:00:00Z"})
	store.Record(FindingOutcome{ID: newUUID(), DetectorName: "scope-guard", FiredAt: "2026-01-01T00:00:00Z"})

	findings, err := store.List("scope-guard", 10)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(findings) != 2 {
		t.Errorf("expected 2 scope-guard findings, got %d", len(findings))
	}
	for _, f := range findings {
		if f.DetectorName != "scope-guard" {
			t.Errorf("unexpected detector %q", f.DetectorName)
		}
	}
}

func TestOutcomeStore_Metrics(t *testing.T) {
	store := makeTestStore(t)

	id1, id2, id3 := newUUID(), newUUID(), newUUID()
	store.Record(FindingOutcome{ID: id1, DetectorName: "scope-guard", FiredAt: "2026-01-01T00:00:00Z"})
	store.Record(FindingOutcome{ID: id2, DetectorName: "scope-guard", FiredAt: "2026-01-01T00:00:00Z"})
	store.Record(FindingOutcome{ID: id3, DetectorName: "contradiction-tracker", FiredAt: "2026-01-01T00:00:00Z"})

	// Validate id1 as TP, id2 as FP.
	_ = store.Validate(id1, true, "tp")
	_ = store.Validate(id2, false, "fp")

	metrics, err := store.Metrics()
	if err != nil {
		t.Fatalf("Metrics error: %v", err)
	}
	if len(metrics) != 2 {
		t.Fatalf("expected 2 detector entries, got %d", len(metrics))
	}

	// Find scope-guard metrics.
	var sg DetectorMetrics
	for _, m := range metrics {
		if m.DetectorName == "scope-guard" {
			sg = m
			break
		}
	}

	if sg.TotalFired != 2 {
		t.Errorf("scope-guard TotalFired: got %d, want 2", sg.TotalFired)
	}
	if sg.ConfirmedTP != 1 {
		t.Errorf("scope-guard ConfirmedTP: got %d, want 1", sg.ConfirmedTP)
	}
	if sg.ConfirmedFP != 1 {
		t.Errorf("scope-guard ConfirmedFP: got %d, want 1", sg.ConfirmedFP)
	}
	const wantPrecision = 0.5
	if sg.Precision != wantPrecision {
		t.Errorf("scope-guard Precision: got %.4f, want %.4f", sg.Precision, wantPrecision)
	}
}

func TestOutcomeStore_EmptyStore(t *testing.T) {
	store := makeTestStore(t)

	findings, err := store.List("", 10)
	if err != nil {
		t.Fatalf("List on empty store: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}

	metrics, err := store.Metrics()
	if err != nil {
		t.Fatalf("Metrics on empty store: %v", err)
	}
	if len(metrics) != 0 {
		t.Errorf("expected 0 metrics, got %d", len(metrics))
	}
}

func TestOutcomeStore_ListLimit(t *testing.T) {
	store := makeTestStore(t)

	for i := 0; i < 5; i++ {
		store.Record(FindingOutcome{ID: newUUID(), DetectorName: "det", FiredAt: "2026-01-01T00:00:00Z"})
	}

	findings, err := store.List("", 3)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(findings) != 3 {
		t.Errorf("expected 3 findings (limit), got %d", len(findings))
	}
}

func TestOutcomeStore_PersistenceAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outcomes.ndjson")

	// Write with first instance.
	store1 := NewOutcomeStore(path)
	id := newUUID()
	store1.Record(FindingOutcome{ID: id, DetectorName: "det", FiredAt: "2026-01-01T00:00:00Z"})

	// Read with second instance.
	store2 := NewOutcomeStore(path)
	findings, err := store2.List("", 10)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].ID != id {
		t.Errorf("ID mismatch after persistence")
	}
}

func TestNewUUID(t *testing.T) {
	id := newUUID()
	// UUID v4 format: 8-4-4-4-12
	if len(id) != 36 {
		t.Errorf("expected UUID length 36, got %d: %q", len(id), id)
	}
	if id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		t.Errorf("UUID format invalid: %q", id)
	}
	// version nibble should be '4'
	if id[14] != '4' {
		t.Errorf("expected version nibble '4', got %q in UUID %q", string(id[14]), id)
	}

	// Two UUIDs should not be equal.
	id2 := newUUID()
	if id == id2 {
		t.Error("two consecutive UUIDs are identical — RNG broken")
	}
}

func TestOutcomeStore_NilOutcomeFile_Graceful(t *testing.T) {
	// Point to a read-only directory to simulate failure at write time.
	// The store should log but not panic.
	dir := t.TempDir()
	if err := os.Chmod(dir, 0444); err != nil {
		t.Skip("cannot chmod temp dir, skipping")
	}
	store := NewOutcomeStore(filepath.Join(dir, "sub", "outcomes.ndjson"))
	// Record should not panic even when directory creation fails.
	store.Record(FindingOutcome{ID: newUUID(), DetectorName: "det", FiredAt: "now"})
}
