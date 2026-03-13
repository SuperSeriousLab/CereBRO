// Memory Consolidator — creates sparse index entries from pipeline results
// and appends them to the Forge corpus in NDJSON format.
//
// Consolidation triggers:
//   - HIGH_CONFIDENCE: all findings above threshold → auto-confirm
//   - NOVEL_PATTERN: finding combination not seen in pattern index
//   - USER_FEEDBACK: explicit user confirm/reject via SubmitFeedback
//
// Rate-limited by cooldown and per-session entry cap to avoid corpus bloat.
//
// Phase 5 deliverable. Brain analogue: hippocampal memory consolidation
// (transferring short-term working memory to long-term corpus).
package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// ConsolidatorConfig holds the Memory Consolidator's tunable parameters.
type ConsolidatorConfig struct {
	CorpusOutputPath     string
	MinConfidenceForAuto float64
	MinAgreementForAuto  float64 // reserved for future use
	CooldownSec          int
	MaxEntriesPerSession int
}

// DefaultConsolidatorConfig returns the default configuration.
func DefaultConsolidatorConfig() ConsolidatorConfig {
	return ConsolidatorConfig{
		CorpusOutputPath:     "data/corpus/consolidated.ndjson",
		MinConfidenceForAuto: 0.8,
		MinAgreementForAuto:  0.7,
		CooldownSec:          60,
		MaxEntriesPerSession: 10,
	}
}

// Consolidator manages sparse index entry creation and corpus appends.
type Consolidator struct {
	mu              sync.Mutex
	cfg             ConsolidatorConfig
	patternIndex    *PatternIndex
	lastConsolidate time.Time
	sessionEntries  int
	// Store last result per conversation for user feedback.
	lastResults map[string]*ConsolidationInput
}

// ConsolidationInput holds data needed for consolidation.
type ConsolidationInput struct {
	ConversationID  string
	Report          *reasoningv1.ReasoningReport
	Inhibition      *InhibitorResult                 // may be nil
	SelfConf        *cerebrov1.SelfConfidenceReport   // may be nil
	FeedbackApplied bool
	Gain            *GainSignal                      // may be nil
	Snap            *reasoningv1.ConversationSnapshot
}

// ConsolidationResult holds the output of a consolidation attempt.
type ConsolidationResult struct {
	Consolidated bool
	Trigger      cerebrov1.ConsolidationTrigger
	Entry        *consolidationJSON // nil if not consolidated
}

// consolidationJSON is the NDJSON entry format, compatible with LoadCorpus/LoadPatternIndex.
type consolidationJSON struct {
	EntryID            string         `json:"entry_id"`
	Timestamp          string         `json:"timestamp"` // RFC3339
	Trigger            string         `json:"trigger"`
	FindingTypes       []string       `json:"finding_types"`
	FindingConfidences []float64      `json:"finding_confidences"`
	FindingSeverities  []string       `json:"finding_severities"`
	InhibitedCount     int            `json:"inhibited_count"`
	DisinhibitedCount  int            `json:"disinhibited_count"`
	InhibitionReasons  []string       `json:"inhibition_reasons"`
	TurnCount          int            `json:"turn_count"`
	Formality          float64        `json:"formality"`
	Urgency            float64        `json:"urgency"`
	Outcome            string         `json:"outcome"`
	OutcomeConfidence  float64        `json:"outcome_confidence"`
	DetectorPattern    string         `json:"detector_pattern"`
	SelfConfidence     float64        `json:"self_confidence"`
	FeedbackApplied    bool           `json:"feedback_applied"`
	Expected           []expectedEntry `json:"expected"`
}

type expectedEntry struct {
	FindingType string `json:"finding_type"`
}

// NewConsolidator creates a new Memory Consolidator.
func NewConsolidator(cfg ConsolidatorConfig, patternIndex *PatternIndex) *Consolidator {
	return &Consolidator{
		cfg:          cfg,
		patternIndex: patternIndex,
		lastResults:  make(map[string]*ConsolidationInput),
	}
}

// Consolidate attempts to create a sparse index entry from pipeline results.
// Returns a ConsolidationResult indicating whether consolidation occurred.
func (c *Consolidator) Consolidate(input *ConsolidationInput) *ConsolidationResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check cooldown.
	if !c.lastConsolidate.IsZero() &&
		time.Since(c.lastConsolidate) < time.Duration(c.cfg.CooldownSec)*time.Second {
		return &ConsolidationResult{Consolidated: false}
	}

	// Check max entries per session.
	if c.sessionEntries >= c.cfg.MaxEntriesPerSession {
		return &ConsolidationResult{Consolidated: false}
	}

	// Determine trigger.
	trigger, triggerEnum := c.determineTrigger(input)
	if trigger == "" {
		// Store for potential user feedback.
		c.lastResults[input.ConversationID] = input
		return &ConsolidationResult{Consolidated: false}
	}

	// Build entry.
	entry := c.buildEntry(input, trigger)

	// Append to file.
	if err := c.appendToFile(entry); err != nil {
		// Silently fail — consolidation is best-effort.
		return &ConsolidationResult{Consolidated: false}
	}

	// Update pattern index.
	pattern := ExtractFindingPattern(input.Report.GetFindings())
	c.patternIndex.AddEntry(pattern)

	// Update rate limits.
	c.lastConsolidate = time.Now()
	c.sessionEntries++

	// Store for user feedback lookup.
	c.lastResults[input.ConversationID] = input

	return &ConsolidationResult{
		Consolidated: true,
		Trigger:      triggerEnum,
		Entry:        entry,
	}
}

// SubmitFeedback records explicit user feedback for a conversation.
// Does not enforce cooldown — explicit feedback is always accepted.
func (c *Consolidator) SubmitFeedback(conversationID string, outcome string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	input, ok := c.lastResults[conversationID]
	if !ok {
		return fmt.Errorf("no pending result for conversation %q", conversationID)
	}

	entry := c.buildEntry(input, "USER_FEEDBACK")
	entry.Outcome = outcome

	if err := c.appendToFile(entry); err != nil {
		return fmt.Errorf("failed to write feedback entry: %w", err)
	}

	// Update pattern index.
	pattern := ExtractFindingPattern(input.Report.GetFindings())
	c.patternIndex.AddEntry(pattern)

	// Remove from pending results.
	delete(c.lastResults, conversationID)

	return nil
}

// determineTrigger returns the trigger string and enum value, or ("", 0) if none.
func (c *Consolidator) determineTrigger(input *ConsolidationInput) (string, cerebrov1.ConsolidationTrigger) {
	findings := input.Report.GetFindings()

	// HIGH_CONFIDENCE: all findings above threshold (requires at least one finding).
	if len(findings) > 0 {
		allHigh := true
		for _, f := range findings {
			if f.GetConfidence() < c.cfg.MinConfidenceForAuto {
				allHigh = false
				break
			}
		}
		if allHigh {
			return "HIGH_CONFIDENCE", cerebrov1.ConsolidationTrigger_CONSOLIDATION_HIGH_CONFIDENCE
		}
	}

	// NOVEL_PATTERN: finding pattern not in index.
	pattern := ExtractFindingPattern(findings)
	if _, found := c.patternIndex.Lookup(pattern); !found {
		return "NOVEL_PATTERN", cerebrov1.ConsolidationTrigger_CONSOLIDATION_NOVEL_PATTERN
	}

	return "", cerebrov1.ConsolidationTrigger_CONSOLIDATION_TRIGGER_UNSPECIFIED
}

// buildEntry constructs a consolidationJSON from the input.
func (c *Consolidator) buildEntry(input *ConsolidationInput, trigger string) *consolidationJSON {
	findings := input.Report.GetFindings()

	var findingTypes []string
	var findingConfidences []float64
	var findingSeverities []string
	var expected []expectedEntry

	for _, f := range findings {
		ft := f.GetFindingType().String()
		findingTypes = append(findingTypes, ft)
		findingConfidences = append(findingConfidences, f.GetConfidence())
		findingSeverities = append(findingSeverities, f.GetSeverity().String())
		expected = append(expected, expectedEntry{FindingType: ft})
	}

	var inhibitedCount, disinhibitedCount int
	reasonSet := make(map[string]bool)
	if input.Inhibition != nil {
		for _, d := range input.Inhibition.Decisions {
			switch d.GetAction() {
			case cerebrov1.InhibitionAction_INHIBITED:
				inhibitedCount++
			case cerebrov1.InhibitionAction_DISINHIBITED:
				disinhibitedCount++
			}
			if r := d.GetReason(); r != "" {
				reasonSet[r] = true
			}
		}
	}
	var inhibitionReasons []string
	for r := range reasonSet {
		inhibitionReasons = append(inhibitionReasons, r)
	}
	sort.Strings(inhibitionReasons)

	var formality, urgency float64
	if input.Gain != nil {
		formality = input.Gain.Formality
		urgency = input.Gain.Urgency
	}

	var outcomeConfidence, selfConfidence float64
	if input.SelfConf != nil {
		outcomeConfidence = input.SelfConf.GetOverallConfidence()
		selfConfidence = input.SelfConf.GetOverallConfidence()
	}

	pattern := ExtractFindingPattern(findings)

	// Default slices to empty (not nil) for clean JSON.
	if findingTypes == nil {
		findingTypes = []string{}
	}
	if findingConfidences == nil {
		findingConfidences = []float64{}
	}
	if findingSeverities == nil {
		findingSeverities = []string{}
	}
	if inhibitionReasons == nil {
		inhibitionReasons = []string{}
	}
	if expected == nil {
		expected = []expectedEntry{}
	}

	return &consolidationJSON{
		EntryID:            fmt.Sprintf("consolidated-%d", time.Now().UnixNano()),
		Timestamp:          time.Now().UTC().Format(time.RFC3339),
		Trigger:            trigger,
		FindingTypes:       findingTypes,
		FindingConfidences: findingConfidences,
		FindingSeverities:  findingSeverities,
		InhibitedCount:     inhibitedCount,
		DisinhibitedCount:  disinhibitedCount,
		InhibitionReasons:  inhibitionReasons,
		TurnCount:          int(input.Snap.GetTotalTurns()),
		Formality:          formality,
		Urgency:            urgency,
		Outcome:            "auto_confirmed",
		OutcomeConfidence:  outcomeConfidence,
		DetectorPattern:    pattern,
		SelfConfidence:     selfConfidence,
		FeedbackApplied:    input.FeedbackApplied,
		Expected:           expected,
	}
}

// appendToFile marshals an entry to JSON and appends it to the corpus file.
func (c *Consolidator) appendToFile(entry *consolidationJSON) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal consolidation entry: %w", err)
	}

	f, err := os.OpenFile(c.cfg.CorpusOutputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open corpus file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write consolidation entry: %w", err)
	}
	return nil
}

// Suppress unused import warnings — these are used by the implementation.
var (
	_ = strings.Join
	_ = sort.Strings
)
