package adversarial

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"time"

	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

// ─── Evolutionary loop (Deliverable 4) ─────────────────────────────────────

// EvolutionConfig controls the genetic algorithm parameters.
type EvolutionConfig struct {
	PopSize     int     // default 20
	Generations int     // default 10
	EliteCount  int     // elitism: top N preserved unchanged (default 2)
	TournSize   int     // tournament selection size (default 3)
	MutateRate  float64 // probability of applying each mutation operator (default 0.3)
	Seed        int64   // RNG seed; 0 = use time
}

// DefaultEvolutionConfig returns sensible defaults.
func DefaultEvolutionConfig() EvolutionConfig {
	return EvolutionConfig{
		PopSize:     20,
		Generations: 10,
		EliteCount:  2,
		TournSize:   3,
		MutateRate:  0.3,
		Seed:        0,
	}
}

// Individual is a template paired with its fitness score.
type Individual struct {
	Template ConversationTemplate
	Fitness  float64
}

// EvolutionResult holds the final population and per-generation statistics.
type EvolutionResult struct {
	FinalPopulation []Individual
	BestFitness     []float64 // best fitness per generation
	MeanFitness     []float64 // mean fitness per generation
}

// RunEvolution executes the full genetic loop.
func RunEvolution(
	evoCfg EvolutionConfig,
	ollamaCfg OllamaConfig,
	pipelineCfg pipeline.PipelineConfig,
	client *http.Client,
) *EvolutionResult {
	seed := evoCfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	generateFn := defaultGenerateFunc(ollamaCfg, client)
	runFn := defaultRunFunc(pipelineCfg)

	// Initialise population.
	templates := randomTemplates(evoCfg.PopSize, rng)
	pop := make([]Individual, len(templates))
	for i, t := range templates {
		pop[i] = Individual{Template: t}
	}

	result := &EvolutionResult{}

	for gen := 0; gen < evoCfg.Generations; gen++ {
		// Evaluate fitness for individuals without a score yet.
		for i := range pop {
			if pop[i].Fitness == 0 {
				pop[i].Fitness = adversarialFitness(pop[i].Template, generateFn, runFn)
			}
		}

		// Sort descending by fitness.
		sort.Slice(pop, func(i, j int) bool {
			return pop[i].Fitness > pop[j].Fitness
		})

		best := pop[0].Fitness
		mean := meanFitness(pop)
		result.BestFitness = append(result.BestFitness, best)
		result.MeanFitness = append(result.MeanFitness, mean)

		log.Printf("[adversarial] gen %d/%d  best=%.3f  mean=%.3f\n",
			gen+1, evoCfg.Generations, best, mean)

		if gen == evoCfg.Generations-1 {
			break // don't breed on the last generation
		}

		// Build next generation.
		nextPop := make([]Individual, 0, evoCfg.PopSize)

		// Elitism: carry top N unchanged.
		eliteN := evoCfg.EliteCount
		if eliteN > len(pop) {
			eliteN = len(pop)
		}
		nextPop = append(nextPop, pop[:eliteN]...)

		// Fill remainder via selection, crossover, mutation.
		for len(nextPop) < evoCfg.PopSize {
			parentA := tournamentSelect(pop, evoCfg.TournSize, rng)
			parentB := tournamentSelect(pop, evoCfg.TournSize, rng)
			child := crossover(parentA.Template, parentB.Template, rng)
			child = mutate(child, evoCfg.MutateRate, rng)
			nextPop = append(nextPop, Individual{Template: child})
		}

		pop = nextPop
	}

	// Final fitness evaluation pass.
	for i := range pop {
		if pop[i].Fitness == 0 {
			pop[i].Fitness = adversarialFitness(pop[i].Template, generateFn, runFn)
		}
	}
	sort.Slice(pop, func(i, j int) bool {
		return pop[i].Fitness > pop[j].Fitness
	})

	result.FinalPopulation = pop
	return result
}

// ─── Selection ─────────────────────────────────────────────────────────────

func tournamentSelect(pop []Individual, tournSize int, rng *rand.Rand) Individual {
	if len(pop) == 0 {
		return Individual{}
	}
	best := pop[rng.Intn(len(pop))]
	for i := 1; i < tournSize; i++ {
		candidate := pop[rng.Intn(len(pop))]
		if candidate.Fitness > best.Fitness {
			best = candidate
		}
	}
	return best
}

// ─── Crossover ─────────────────────────────────────────────────────────────

// crossover swaps failure specs between two parent templates.
func crossover(a, b ConversationTemplate, rng *rand.Rand) ConversationTemplate {
	child := ConversationTemplate{
		Topic:         a.Topic,
		Formality:     a.Formality,
		TurnCount:     a.TurnCount,
		Speakers:      append([]string(nil), a.Speakers...),
		Distractors:   append([]string(nil), a.Distractors...),
		CleanSections: append([]TurnRange(nil), a.CleanSections...),
	}

	// Swap the failure specs: interleave from both parents.
	combined := make([]FailureSpec, 0, len(a.FailureModes)+len(b.FailureModes))
	combined = append(combined, a.FailureModes...)
	combined = append(combined, b.FailureModes...)

	// Deduplicate by type — keep first occurrence.
	seen := make(map[string]bool)
	var unique []FailureSpec
	for _, f := range combined {
		if !seen[f.Type] {
			seen[f.Type] = true
			unique = append(unique, f)
		}
	}

	// Choose a random subset (1–3 failure modes).
	rng.Shuffle(len(unique), func(i, j int) { unique[i], unique[j] = unique[j], unique[i] })
	n := 1 + rng.Intn(3)
	if n > len(unique) {
		n = len(unique)
	}
	child.FailureModes = unique[:n]

	// Pick topic from either parent.
	if rng.Float64() < 0.5 {
		child.Topic = b.Topic
	}

	return child
}

// ─── Mutation operators ─────────────────────────────────────────────────────

// mutate applies random perturbations to a template.
func mutate(tmpl ConversationTemplate, rate float64, rng *rand.Rand) ConversationTemplate {
	// Perturb severity.
	if rng.Float64() < rate {
		for i := range tmpl.FailureModes {
			tmpl.FailureModes[i].Severity = clampF(tmpl.FailureModes[i].Severity+rng.NormFloat64()*0.2, 0, 1)
		}
	}

	// Shift onset_turn.
	if rng.Float64() < rate {
		for i := range tmpl.FailureModes {
			shift := rng.Intn(3) - 1 // -1, 0, +1
			tmpl.FailureModes[i].OnsetTurn = clampI(tmpl.FailureModes[i].OnsetTurn+shift, 1, tmpl.TurnCount)
		}
	}

	// Change technique.
	if rng.Float64() < rate {
		for i := range tmpl.FailureModes {
			tmpl.FailureModes[i].Technique = techniques[rng.Intn(len(techniques))]
		}
	}

	// Add a failure mode.
	if rng.Float64() < rate && len(tmpl.FailureModes) < 4 {
		existing := make(map[string]bool)
		for _, f := range tmpl.FailureModes {
			existing[f.Type] = true
		}
		var candidates []string
		for _, ft := range failureTypes {
			if !existing[ft] {
				candidates = append(candidates, ft)
			}
		}
		if len(candidates) > 0 {
			ft := candidates[rng.Intn(len(candidates))]
			onset := 1 + rng.Intn(tmpl.TurnCount)
			tmpl.FailureModes = append(tmpl.FailureModes, FailureSpec{
				Type:      ft,
				Severity:  rng.Float64(),
				OnsetTurn: onset,
				Duration:  1 + rng.Intn(3),
				Technique: techniques[rng.Intn(len(techniques))],
			})
		}
	}

	// Remove a failure mode (if more than 1).
	if rng.Float64() < rate && len(tmpl.FailureModes) > 1 {
		idx := rng.Intn(len(tmpl.FailureModes))
		tmpl.FailureModes = append(tmpl.FailureModes[:idx], tmpl.FailureModes[idx+1:]...)
	}

	// Change formality.
	if rng.Float64() < rate {
		tmpl.Formality = clampF(tmpl.Formality+rng.NormFloat64()*0.2, 0, 1)
	}

	// Change topic.
	if rng.Float64() < rate {
		tmpl.Topic = domains[rng.Intn(len(domains))]
	}

	return tmpl
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func meanFitness(pop []Individual) float64 {
	if len(pop) == 0 {
		return 0
	}
	sum := 0.0
	for _, ind := range pop {
		sum += ind.Fitness
	}
	return sum / float64(len(pop))
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampI(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// TopN returns the top n individuals from a final population (already sorted).
func TopN(result *EvolutionResult, n int) []Individual {
	if result == nil {
		return nil
	}
	if n > len(result.FinalPopulation) {
		n = len(result.FinalPopulation)
	}
	return result.FinalPopulation[:n]
}

// ─── Export (Deliverable: Export function) ──────────────────────────────────

// CorpusEntry matches the NDJSON format used in data/corpus/*.ndjson.
type CorpusEntry struct {
	EntryID  string      `json:"entry_id"`
	Input    CorpusInput `json:"input"`
	Expected []Expected  `json:"expected"`
}

// CorpusInput is the conversation snapshot serialised for the corpus.
type CorpusInput struct {
	Turns     []CorpusTurn `json:"turns"`
	Objective string       `json:"objective,omitempty"`
	TotalTurns int         `json:"total_turns"`
}

// CorpusTurn is a single turn in the corpus format.
type CorpusTurn struct {
	TurnNumber int    `json:"turn_number"`
	Speaker    string `json:"speaker"`
	RawText    string `json:"raw_text"`
}

// Expected is an expected finding in the corpus format.
type Expected struct {
	FindingType string `json:"finding_type"`
}

// ExportCorpusEntries converts top-N individuals to corpus entries ready for NDJSON output.
// Each individual must have its conversation snapshot pre-generated; since we store
// templates rather than snapshots, we regenerate them here using the provided generate function.
func ExportCorpusEntries(
	individuals []Individual,
	generateFn GenerateFunc,
	prefix string,
) []CorpusEntry {
	var entries []CorpusEntry
	for i, ind := range individuals {
		snap := generateFn(ind.Template)
		if snap == nil {
			continue
		}

		var turns []CorpusTurn
		for _, t := range snap.GetTurns() {
			turns = append(turns, CorpusTurn{
				TurnNumber: int(t.GetTurnNumber()),
				Speaker:    t.GetSpeaker(),
				RawText:    t.GetRawText(),
			})
		}

		var expected []Expected
		for _, f := range ind.Template.FailureModes {
			expected = append(expected, Expected{FindingType: templateTypeToProto(f.Type)})
		}

		entries = append(entries, CorpusEntry{
			EntryID: fmt.Sprintf("%s-%03d", prefix, i+1),
			Input: CorpusInput{
				Turns:      turns,
				Objective:  snap.GetObjective(),
				TotalTurns: len(turns),
			},
			Expected: expected,
		})
	}
	return entries
}
