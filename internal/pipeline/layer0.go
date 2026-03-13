// Layer 0: Brainstem Reflexes — fast input validation before cognitive processing.
//
// Three checks run in sequence. If any rejects, the pipeline short-circuits
// immediately — no Stages 1-5 execute.
//
//  1. Format Validator: structure, encoding, size limits
//  2. Toxicity Gate: blocklist screening with word boundary matching
//  3. Language Detector: trigram frequency language identification
//
// Note: The cerebro.v1 proto messages (ValidationResult, ToxicityResult,
// LanguageResult) were designed for a future where Layer 0 sits at the network
// boundary accepting raw bytes. For now, it validates already-parsed
// ConversationSnapshot fields.
package pipeline

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	reasoningv1 "github.com/SuperSeriousLab/CereBRO/gen/go/cog/reasoning/v1"
)

// Layer0Config holds all Layer 0 configurations.
type Layer0Config struct {
	Format   FormatValidatorConfig
	Toxicity ToxicityGateConfig
	Language LanguageDetectorConfig
}

// DefaultLayer0Config returns the default Layer 0 configuration.
func DefaultLayer0Config() Layer0Config {
	return Layer0Config{
		Format:   DefaultFormatValidatorConfig(),
		Toxicity: DefaultToxicityGateConfig(),
		Language: DefaultLanguageDetectorConfig(),
	}
}

// LoadLayer0Config loads a complete Layer0Config from disk paths.
// Intended for server/gateway startup.
func LoadLayer0Config(blocklistPath, profileDir string) (Layer0Config, error) {
	cfg := DefaultLayer0Config()

	if blocklistPath != "" {
		blocklist, err := LoadBlocklist(blocklistPath)
		if err != nil {
			return cfg, err
		}
		cfg.Toxicity.Blocklist = blocklist
	}

	if profileDir != "" {
		profiles, err := LoadLangProfiles(profileDir)
		if err != nil {
			return cfg, err
		}
		cfg.Language.Profiles = profiles
	}

	return cfg, nil
}

// Layer0Result holds the combined Layer 0 output.
type Layer0Result struct {
	Accepted   bool
	Validation *cerebrov1.ValidationResult
	Toxicity   *cerebrov1.ToxicityResult
	Language   *cerebrov1.LanguageResult
	Reason     string // non-empty if rejected
}

// RunLayer0 executes all three Layer 0 checks in sequence.
// Returns immediately on the first rejection.
func RunLayer0(snap *reasoningv1.ConversationSnapshot, cfg Layer0Config) *Layer0Result {
	// 1. Format validation.
	validation := ValidateFormat(snap, cfg.Format)
	if !validation.GetValid() {
		return &Layer0Result{
			Accepted:   false,
			Validation: validation,
			Reason:     "format: " + validation.GetRejectionReason(),
		}
	}

	// 2. Toxicity screening.
	toxicity := ScreenToxicity(snap, cfg.Toxicity.Blocklist, cfg.Toxicity.CaseSensitive)
	if toxicity.GetToxic() {
		return &Layer0Result{
			Accepted:   false,
			Validation: validation,
			Toxicity:   toxicity,
			Reason:     "toxicity: matched " + strings.Join(toxicity.GetMatchedPatterns(), ", "),
		}
	}

	// 3. Language detection.
	language := DetectLanguage(snap, cfg.Language.Profiles, cfg.Language)
	if !language.GetSupported() {
		return &Layer0Result{
			Accepted:   false,
			Validation: validation,
			Toxicity:   toxicity,
			Language:   language,
			Reason:     "language: unsupported language " + language.GetLanguageCode(),
		}
	}

	return &Layer0Result{
		Accepted:   true,
		Validation: validation,
		Toxicity:   toxicity,
		Language:   language,
	}
}

// ============================================================
// Format Validator
// ============================================================

// FormatValidatorConfig holds format validation parameters.
type FormatValidatorConfig struct {
	MaxInputBytes uint32
	MaxTurns      uint32
}

// DefaultFormatValidatorConfig returns the default format validator config.
func DefaultFormatValidatorConfig() FormatValidatorConfig {
	return FormatValidatorConfig{
		MaxInputBytes: 1048576, // 1MB
		MaxTurns:      500,
	}
}

// ValidateFormat checks a ConversationSnapshot for structural validity.
func ValidateFormat(snap *reasoningv1.ConversationSnapshot, cfg FormatValidatorConfig) *cerebrov1.ValidationResult {
	if snap == nil {
		return &cerebrov1.ValidationResult{
			Valid:           false,
			RejectionReason: "nil snapshot",
		}
	}

	if len(snap.GetTurns()) == 0 {
		return &cerebrov1.ValidationResult{
			Valid:           false,
			RejectionReason: "no turns in snapshot",
		}
	}

	if uint32(len(snap.GetTurns())) > cfg.MaxTurns {
		return &cerebrov1.ValidationResult{
			Valid:           false,
			RejectionReason: "turn count exceeds max_turns limit",
			InputSizeBytes:  estimateSnapshotSize(snap),
		}
	}

	totalBytes := estimateSnapshotSize(snap)
	if totalBytes > cfg.MaxInputBytes {
		return &cerebrov1.ValidationResult{
			Valid:           false,
			RejectionReason: "input size exceeds max_input_bytes limit",
			InputSizeBytes:  totalBytes,
		}
	}

	for _, turn := range snap.GetTurns() {
		if !utf8.ValidString(turn.GetRawText()) {
			return &cerebrov1.ValidationResult{
				Valid:           false,
				RejectionReason: "invalid UTF-8 in turn text",
				InputSizeBytes:  totalBytes,
			}
		}
		if !utf8.ValidString(turn.GetSpeaker()) {
			return &cerebrov1.ValidationResult{
				Valid:           false,
				RejectionReason: "invalid UTF-8 in speaker field",
				InputSizeBytes:  totalBytes,
			}
		}
	}

	if !utf8.ValidString(snap.GetObjective()) {
		return &cerebrov1.ValidationResult{
			Valid:           false,
			RejectionReason: "invalid UTF-8 in objective field",
			InputSizeBytes:  totalBytes,
		}
	}

	return &cerebrov1.ValidationResult{
		Valid:          true,
		InputSizeBytes: totalBytes,
	}
}

func estimateSnapshotSize(snap *reasoningv1.ConversationSnapshot) uint32 {
	var total int
	total += len(snap.GetObjective())
	for _, turn := range snap.GetTurns() {
		total += len(turn.GetRawText())
		total += len(turn.GetSpeaker())
	}
	return uint32(total)
}

// ============================================================
// Toxicity Gate
// ============================================================

// ToxicityGateConfig holds toxicity screening parameters.
type ToxicityGateConfig struct {
	Blocklist     []string // pre-loaded blocklist terms
	CaseSensitive bool
}

// DefaultToxicityGateConfig returns the default toxicity config (empty blocklist).
// Callers should use LoadBlocklist to populate Blocklist from a file.
func DefaultToxicityGateConfig() ToxicityGateConfig {
	return ToxicityGateConfig{
		CaseSensitive: false,
	}
}

// LoadBlocklist reads a blocklist file (one term per line, # comments).
func LoadBlocklist(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var terms []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		terms = append(terms, line)
	}
	return terms, scanner.Err()
}

// ScreenToxicity checks all turn text against the blocklist using word boundaries.
func ScreenToxicity(snap *reasoningv1.ConversationSnapshot, blocklist []string, caseSensitive bool) *cerebrov1.ToxicityResult {
	if snap == nil || len(blocklist) == 0 {
		return &cerebrov1.ToxicityResult{Toxic: false}
	}

	normalized := make([]string, len(blocklist))
	for i, term := range blocklist {
		if caseSensitive {
			normalized[i] = term
		} else {
			normalized[i] = strings.ToLower(term)
		}
	}

	var matched []string
	for _, turn := range snap.GetTurns() {
		text := turn.GetRawText()
		if !caseSensitive {
			text = strings.ToLower(text)
		}
		for _, pattern := range normalized {
			if matchWordBoundary(text, pattern) {
				if !sliceContains(matched, pattern) {
					matched = append(matched, pattern)
				}
			}
		}
	}

	if len(matched) == 0 {
		return &cerebrov1.ToxicityResult{Toxic: false}
	}

	score := float64(len(matched)) / float64(len(normalized))
	if score > 1.0 {
		score = 1.0
	}
	return &cerebrov1.ToxicityResult{
		Toxic:           true,
		MatchedPatterns: matched,
		ToxicityScore:   score,
	}
}

// matchWordBoundary checks if pattern appears in text at word boundaries.
func matchWordBoundary(text, pattern string) bool {
	start := 0
	for {
		idx := strings.Index(text[start:], pattern)
		if idx < 0 {
			return false
		}
		matchStart := start + idx
		matchEnd := matchStart + len(pattern)

		leftOK := matchStart == 0 || !isWordRuneBefore(text, matchStart)
		rightOK := matchEnd >= len(text) || !isWordRuneAt(text, matchEnd)

		if leftOK && rightOK {
			return true
		}
		start = matchStart + 1
		if start >= len(text) {
			return false
		}
	}
}

// isWordRuneBefore decodes the UTF-8 rune ending at byte position pos and
// returns whether it is a letter or digit.
func isWordRuneBefore(text string, pos int) bool {
	r, _ := utf8.DecodeLastRuneInString(text[:pos])
	return r != utf8.RuneError && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

// isWordRuneAt decodes the UTF-8 rune starting at byte position pos and
// returns whether it is a letter or digit.
func isWordRuneAt(text string, pos int) bool {
	r, _ := utf8.DecodeRuneInString(text[pos:])
	return r != utf8.RuneError && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

func sliceContains(s []string, v string) bool {
	for _, item := range s {
		if item == v {
			return true
		}
	}
	return false
}

// ============================================================
// Language Detector
// ============================================================

// LanguageDetectorConfig holds language detection parameters.
type LanguageDetectorConfig struct {
	Profiles           map[string]LangProfile // language_code → trigram profile
	SupportedLanguages []string
	MinConfidence      float64
	FallbackLanguage   string
	MinSampleChars     uint32
}

// LangProfile maps trigrams to normalized frequencies.
type LangProfile map[string]float64

// DefaultLanguageDetectorConfig returns the default language detector config.
// Callers should use LoadLangProfiles to populate Profiles from a directory.
func DefaultLanguageDetectorConfig() LanguageDetectorConfig {
	return LanguageDetectorConfig{
		SupportedLanguages: []string{"en"},
		MinConfidence:      0.55,
		FallbackLanguage:   "en",
		MinSampleChars:     20,
	}
}

// LoadLangProfiles loads all language profiles from a directory.
func LoadLangProfiles(dir string) (map[string]LangProfile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	profiles := make(map[string]LangProfile)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		lang := strings.TrimSuffix(entry.Name(), ".json")
		data, err := os.ReadFile(dir + "/" + entry.Name())
		if err != nil {
			return nil, err
		}
		var profile LangProfile
		if err := json.Unmarshal(data, &profile); err != nil {
			return nil, err
		}
		profiles[lang] = profile
	}
	return profiles, nil
}

// DetectLanguage identifies the dominant language of the conversation.
func DetectLanguage(snap *reasoningv1.ConversationSnapshot, profiles map[string]LangProfile, cfg LanguageDetectorConfig) *cerebrov1.LanguageResult {
	if snap == nil || len(profiles) == 0 {
		return &cerebrov1.LanguageResult{
			LanguageCode: cfg.FallbackLanguage,
			Confidence:   0.5,
			Supported:    langIsSupported(cfg.FallbackLanguage, cfg.SupportedLanguages),
		}
	}

	sample := langExtractSample(snap, 2000)
	if uint32(len(sample)) < cfg.MinSampleChars {
		return &cerebrov1.LanguageResult{
			LanguageCode: cfg.FallbackLanguage,
			Confidence:   0.5,
			Supported:    langIsSupported(cfg.FallbackLanguage, cfg.SupportedLanguages),
		}
	}

	sampleProfile := langCountTrigrams(sample)

	bestLang := ""
	bestScore := 0.0
	for lang, profile := range profiles {
		score := langProfileScore(sampleProfile, profile)
		if score > bestScore {
			bestScore = score
			bestLang = lang
		}
	}

	// Below confidence threshold — treat as unreliable detection.
	// Mark as unsupported so Layer 0 rejects rather than passing
	// a low-confidence guess to expensive downstream processing.
	if bestScore < cfg.MinConfidence {
		return &cerebrov1.LanguageResult{
			LanguageCode: bestLang,
			Confidence:   bestScore,
			Supported:    false,
		}
	}

	return &cerebrov1.LanguageResult{
		LanguageCode: bestLang,
		Confidence:   bestScore,
		Supported:    langIsSupported(bestLang, cfg.SupportedLanguages),
	}
}

func langExtractSample(snap *reasoningv1.ConversationSnapshot, maxChars int) string {
	var b strings.Builder
	for _, turn := range snap.GetTurns() {
		text := turn.GetRawText()
		remaining := maxChars - b.Len()
		if remaining <= 0 {
			break
		}
		if len(text) > remaining {
			text = text[:remaining]
		}
		b.WriteString(text)
		b.WriteByte(' ')
	}
	return strings.ToLower(strings.TrimSpace(b.String()))
}

func langCountTrigrams(text string) LangProfile {
	profile := make(LangProfile)
	runes := []rune(text)
	if len(runes) < 3 {
		return profile
	}
	total := 0
	for i := 0; i <= len(runes)-3; i++ {
		trigram := string(runes[i : i+3])
		profile[trigram]++
		total++
	}
	if total > 0 {
		for k, v := range profile {
			profile[k] = v / float64(total)
		}
	}
	return profile
}

// langProfileScore combines cosine similarity with profile overlap for robust scoring.
func langProfileScore(sample, ref LangProfile) float64 {
	cos := langCosineSimilarity(sample, ref)

	overlap := 0.0
	for k, va := range sample {
		if _, ok := ref[k]; ok {
			overlap += va
		}
	}

	liftedCos := math.Sqrt(cos)
	score := 0.4*liftedCos + 0.6*overlap
	if score > 1.0 {
		score = 1.0
	}
	return score
}

func langCosineSimilarity(a, b LangProfile) float64 {
	var dot, magA, magB float64
	for k, va := range a {
		magA += va * va
		if vb, ok := b[k]; ok {
			dot += va * vb
		}
	}
	for _, vb := range b {
		magB += vb * vb
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}

func langIsSupported(lang string, supported []string) bool {
	for _, s := range supported {
		if s == lang {
			return true
		}
	}
	return false
}
