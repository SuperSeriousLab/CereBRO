// OutcomeStore records finding outcomes for TP/FP tracking per detector.
//
// Storage format: NDJSON at /var/lib/cerebro/outcomes.ndjson (or a
// caller-supplied path). Each line is a JSON-encoded FindingOutcome.
// Outcome and ValidatedAt are null until a POST /findings/{id}/outcome
// call validates the finding.
//
// The store is safe for concurrent use. All writes are append-only.
// Reads scan the file linearly — acceptable for low-volume validation
// workflows (typically dozens of validations per day, not millions).
package pipeline

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// DefaultOutcomesPath is the default location for the NDJSON outcomes file.
	DefaultOutcomesPath = "/var/lib/cerebro/outcomes.ndjson"
)

// FindingOutcome is a single NDJSON line in the outcomes file.
type FindingOutcome struct {
	ID           string   `json:"id"`            // uuid v4 string
	SessionID    string   `json:"session_id"`    // conversation ID
	DetectorName string   `json:"detector_name"` // e.g. "scope-guard"
	FindingType  string   `json:"finding_type"`  // e.g. "SCOPE_DRIFT"
	Confidence   float64  `json:"confidence"`
	Severity     float64  `json:"severity"`
	FiredAt      string   `json:"fired_at"`     // RFC3339
	Outcome      *bool    `json:"outcome"`      // null until validated; true=TP, false=FP
	OutcomeNote  string   `json:"outcome_note"` // optional human note
	ValidatedAt  *string  `json:"validated_at"` // null until validated; RFC3339
}

// DetectorMetrics holds per-detector TP/FP statistics.
type DetectorMetrics struct {
	DetectorName    string  `json:"detector_name"`
	TotalFired      int     `json:"total_fired"`
	ConfirmedTP     int     `json:"confirmed_tp"`
	ConfirmedFP     int     `json:"confirmed_fp"`
	Precision       float64 `json:"precision"`        // TP / (TP + FP); NaN when no validated findings
	RecallEstimate  float64 `json:"recall_estimate"`  // always 0 — no ground truth available
}

// newUUID returns a random UUID v4 string (e.g. "550e8400-e29b-41d4-a716-446655440000").
// Uses crypto/rand. Panics only when the OS random source is broken.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("outcome-store: crypto/rand unavailable: " + err.Error())
	}
	// Set version 4 (0100xxxx) and variant bits (10xxxxxx).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// OutcomeStore manages outcome recording and validation for CereBRO findings.
type OutcomeStore struct {
	mu   sync.Mutex
	path string
}

// NewOutcomeStore returns an OutcomeStore that writes to path.
// It creates any missing parent directories. Path defaults to
// DefaultOutcomesPath when empty.
func NewOutcomeStore(path string) *OutcomeStore {
	if path == "" {
		path = DefaultOutcomesPath
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Printf("[outcome-store] failed to create directory %q: %v", filepath.Dir(path), err)
	}
	return &OutcomeStore{path: path}
}

// Record appends a new FindingOutcome with a null outcome.
// Called fire-and-forget from maybeInjectPTSFindings.
func (s *OutcomeStore) Record(outcome FindingOutcome) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.appendLine(outcome); err != nil {
		log.Printf("[outcome-store] failed to record finding %q: %v", outcome.ID, err)
	}
}

// Validate sets the outcome (true=TP, false=FP) for a finding by ID.
// Returns an error when the ID is not found.
// Strategy: scan the file, rewrite with the updated record.
func (s *OutcomeStore) Validate(id string, correct bool, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	outcomes, err := s.readAll()
	if err != nil {
		return fmt.Errorf("outcome-store: read: %w", err)
	}

	found := false
	now := time.Now().UTC().Format(time.RFC3339)
	for i, o := range outcomes {
		if o.ID == id {
			outcomes[i].Outcome = &correct
			outcomes[i].OutcomeNote = note
			outcomes[i].ValidatedAt = &now
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("outcome-store: finding %q not found", id)
	}

	return s.rewriteAll(outcomes)
}

// List returns up to limit recent FindingOutcome records (newest first),
// optionally filtered by detectorName (empty = all detectors).
func (s *OutcomeStore) List(detectorName string, limit int) ([]FindingOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.readAll()
	if err != nil {
		return nil, fmt.Errorf("outcome-store: list: %w", err)
	}

	// Reverse (newest first).
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}

	var result []FindingOutcome
	for _, o := range all {
		if detectorName != "" && o.DetectorName != detectorName {
			continue
		}
		result = append(result, o)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

// Metrics returns per-detector TP/FP statistics across all recorded findings.
func (s *OutcomeStore) Metrics() ([]DetectorMetrics, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.readAll()
	if err != nil {
		return nil, fmt.Errorf("outcome-store: metrics: %w", err)
	}

	type stats struct {
		total int
		tp    int
		fp    int
	}
	m := make(map[string]*stats)
	// Preserve insertion order using a slice.
	var order []string

	for _, o := range all {
		name := o.DetectorName
		if _, exists := m[name]; !exists {
			m[name] = &stats{}
			order = append(order, name)
		}
		st := m[name]
		st.total++
		if o.Outcome != nil {
			if *o.Outcome {
				st.tp++
			} else {
				st.fp++
			}
		}
	}

	result := make([]DetectorMetrics, 0, len(order))
	for _, name := range order {
		st := m[name]
		var precision float64
		validated := st.tp + st.fp
		if validated > 0 {
			precision = float64(st.tp) / float64(validated)
		}
		result = append(result, DetectorMetrics{
			DetectorName:   name,
			TotalFired:     st.total,
			ConfirmedTP:    st.tp,
			ConfirmedFP:    st.fp,
			Precision:      precision,
			RecallEstimate: 0, // no ground truth
		})
	}
	return result, nil
}

// GetByID returns a single FindingOutcome by ID, or an error when not found.
func (s *OutcomeStore) GetByID(id string) (FindingOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.readAll()
	if err != nil {
		return FindingOutcome{}, fmt.Errorf("outcome-store: get: %w", err)
	}
	for _, o := range all {
		if o.ID == id {
			return o, nil
		}
	}
	return FindingOutcome{}, fmt.Errorf("outcome-store: finding %q not found", id)
}

// ── internal helpers ─────────────────────────────────────────────────────────

func (s *OutcomeStore) appendLine(o FindingOutcome) error {
	data, err := json.Marshal(o)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

func (s *OutcomeStore) readAll() ([]FindingOutcome, error) {
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return nil, nil // empty store
	}
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	var outcomes []FindingOutcome
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1 MiB line buffer
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var o FindingOutcome
		if err := json.Unmarshal(line, &o); err != nil {
			log.Printf("[outcome-store] skipping malformed line: %v", err)
			continue
		}
		outcomes = append(outcomes, o)
	}
	return outcomes, scanner.Err()
}

func (s *OutcomeStore) rewriteAll(outcomes []FindingOutcome) error {
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open tmp: %w", err)
	}
	w := bufio.NewWriter(f)
	for _, o := range outcomes {
		data, err := json.Marshal(o)
		if err != nil {
			_ = f.Close()
			return fmt.Errorf("marshal: %w", err)
		}
		if _, err = w.Write(append(data, '\n')); err != nil {
			_ = f.Close()
			return fmt.Errorf("write: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return fmt.Errorf("flush: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close tmp: %w", err)
	}
	return os.Rename(tmp, s.path)
}
