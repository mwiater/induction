// Shuffle-Invariance diagnostic harness. Beta / work in progress.
// Standard library only.
// Canonical invocation: go run ./internal/sia/eval.go --url URL --model MODEL
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// baselinePrefix identifies a fixed value used internally by this package.
const baselinePrefix = `Answer each multiple-choice question using only its letter (A, B, C, or D).

Question: What is 2 + 3?
A. 4
B. 5
C. 6
D. 7
Answer: B

Question: Which animal is a mammal?
A. Trout
B. Sparrow
C. Dolphin
D. Lizard
Answer: C

Question: Which number is smallest?
A. 9
B. 6
C. 3
D. 1
Answer: D

`

// balancedPrefix identifies a fixed value used internally by this package.
const balancedPrefix = `Answer each multiple-choice question using only its letter (A, B, C, or D).

Question: First example.
A. correct
B. wrong
C. wrong
D. wrong
Answer: A

Question: Second example.
A. wrong
B. correct
C. wrong
D. wrong
Answer: B

Question: Third example.
A. wrong
B. wrong
C. correct
D. wrong
Answer: C

Question: Fourth example.
A. wrong
B. wrong
C. wrong
D. correct
Answer: D

`

// Item is an exported type in this package's public data model.
type Item struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// Question stores the value represented by this member.
	Question string `json:"question"`
	// Options stores the value represented by this member.
	Options [4]string `json:"options"`
	// Correct stores the value represented by this member.
	Correct int `json:"correct"`
	// Difficulty stores the value represented by this member.
	Difficulty string `json:"difficulty"`
}

// itemWire is an internal data type used by this package.
type itemWire struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// Question stores the value represented by this member.
	Question string `json:"question"`
	// Options stores the value represented by this member.
	Options []string `json:"options"`
	// Correct stores the value represented by this member.
	Correct *int `json:"correct"`
	// Difficulty stores the value represented by this member.
	Difficulty string `json:"difficulty"`
}

// Token is an exported type in this package's public data model.
type Token struct {
	// ID stores the value represented by this member.
	ID int `json:"id"`
	// Token stores the value represented by this member.
	Token string `json:"token"`
	// Bytes stores the value represented by this member.
	Bytes []int `json:"bytes,omitempty"`
	// Logprob stores the value represented by this member.
	Logprob *float64 `json:"logprob,omitempty"`
}

// ProbStep is an exported type in this package's public data model.
type ProbStep struct {
	// Top stores the value represented by this member.
	Top []Token `json:"top_logprobs"`
}

// Completion is an exported type in this package's public data model.
type Completion struct {
	// Content stores the value represented by this member.
	Content string `json:"content"`
	// Model stores the value represented by this member.
	Model string `json:"model"`
	// Probs stores the value represented by this member.
	Probs []ProbStep `json:"probs"`
	// CompletionProbabilities stores the value represented by this member.
	CompletionProbabilities []ProbStep `json:"completion_probabilities"`
	// Truncated stores the value represented by this member.
	Truncated bool `json:"truncated"`
	// Predicted stores the value represented by this member.
	Predicted *int `json:"tokens_predicted,omitempty"`
	// Timings stores the value represented by this member.
	Timings json.RawMessage `json:"timings,omitempty"`
}

// Rotation is an exported type in this package's public data model.
type Rotation struct {
	// PermutationSet stores the value represented by this member.
	PermutationSet string `json:"permutation_set"`
	// Prompt stores the value represented by this member.
	Prompt string `json:"prompt"`
	// LetterToOption stores the value represented by this member.
	LetterToOption [4]int `json:"letter_to_option_index"`
	// CorrectOption stores the value represented by this member.
	CorrectOption int `json:"correct_option"`
	// PromptSHA256 stores the value represented by this member.
	PromptSHA256 string `json:"prompt_sha256"`
}

// PlanItem is an exported type in this package's public data model.
type PlanItem struct {
	// Item stores the value represented by this member.
	Item Item `json:"item"`
	// Rotations stores the value represented by this member.
	Rotations []Rotation `json:"rotations"`
}

// EvaluationPlan is an exported type in this package's public data model.
type EvaluationPlan struct {
	// Items stores the value represented by this member.
	Items []PlanItem `json:"items"`
	// Controls stores the value represented by this member.
	Controls []string `json:"controls"`
	// Prefixes stores the value represented by this member.
	Prefixes map[string]string `json:"prefixes"`
	// QuestionsSHA256 stores the value represented by this member.
	QuestionsSHA256 string `json:"questions_file_sha256"`
	// PlanSHA256 stores the value represented by this member.
	PlanSHA256 string `json:"plan_sha256"`
}

// Result is an exported type in this package's public data model.
type Result struct {
	// ItemID stores the value represented by this member.
	ItemID string `json:"item_id"`
	// Rotation stores the value represented by this member.
	Rotation int `json:"rotation"`
	// PermutationSet stores the value represented by this member.
	PermutationSet string `json:"permutation_set"`
	// Control stores the value represented by this member.
	Control string `json:"control"`
	// Repeat stores the value represented by this member.
	Repeat int `json:"repeat"`
	// CorrectLetter stores the value represented by this member.
	CorrectLetter string `json:"correct_letter"`
	// LetterToOption stores the value represented by this member.
	LetterToOption [4]int `json:"letter_to_option_index"`
	// Prompt stores the value represented by this member.
	Prompt string `json:"prompt"`
	// LetterMass stores the value represented by this member.
	LetterMass [4]float64 `json:"letter_mass"`
	// Normalized stores the value represented by this member.
	Normalized [4]float64 `json:"normalized"`
	// CandidateMass stores the value represented by this member.
	CandidateMass float64 `json:"candidate_mass"`
	// Missing stores the value represented by this member.
	Missing []string `json:"missing_letters,omitempty"`
	// CapturedMass stores the value represented by this member.
	CapturedMass float64 `json:"captured_top_n_mass"`
	// OffLetterObserved stores the value represented by this member.
	OffLetterObserved float64 `json:"observed_off_letter_mass"`
	// UnobservedMass stores the value represented by this member.
	UnobservedMass float64 `json:"unobserved_mass"`
	// CorrectProbability stores the value represented by this member.
	CorrectProbability *float64 `json:"correct_probability,omitempty"`
	// TopPickContent stores the value represented by this member.
	TopPickContent string `json:"top_pick_content,omitempty"`
	// TopPickOption stores the value represented by this member.
	TopPickOption *int `json:"top_pick_option,omitempty"`
	// Generated stores the value represented by this member.
	Generated string `json:"generated"`
	// TopTokens stores the value represented by this member.
	TopTokens []Token `json:"top_tokens,omitempty"`
	// Timings stores the value represented by this member.
	Timings json.RawMessage `json:"timings,omitempty"`
	// HTTPMS stores the value represented by this member.
	HTTPMS int64 `json:"http_ms"`
	// Error stores the value represented by this member.
	Error string `json:"error,omitempty"`
	// FormatAdherent stores the value represented by this member.
	FormatAdherent *bool `json:"answer_format_adherent,omitempty"`
}

// CompactTiming is an exported type in this package's public data model.
type CompactTiming struct {
	// CacheTokens stores the value represented by this member.
	CacheTokens *int `json:"cache_tokens,omitempty"`
	// PromptTokens stores the value represented by this member.
	PromptTokens *int `json:"prompt_tokens,omitempty"`
	// PromptMS stores the value represented by this member.
	PromptMS *float64 `json:"prompt_ms,omitempty"`
	// PredictedTokens stores the value represented by this member.
	PredictedTokens *int `json:"predicted_tokens,omitempty"`
	// PredictedMS stores the value represented by this member.
	PredictedMS *float64 `json:"predicted_ms,omitempty"`
	// HTTPMS stores the value represented by this member.
	HTTPMS int64 `json:"http_ms"`
}

// CompactResult is an exported type in this package's public data model.
type CompactResult struct {
	// ItemID stores the value represented by this member.
	ItemID string `json:"item_id"`
	// Rotation stores the value represented by this member.
	Rotation int `json:"rotation"`
	// PermutationSet stores the value represented by this member.
	PermutationSet string `json:"permutation_set"`
	// Control stores the value represented by this member.
	Control string `json:"control"`
	// Repeat stores the value represented by this member.
	Repeat int `json:"repeat"`
	// LetterToOption stores the value represented by this member.
	LetterToOption [4]int `json:"letter_to_option_index"`
	// LetterMass stores the value represented by this member.
	LetterMass [4]float64 `json:"letter_mass"`
	// CapturedMass stores the value represented by this member.
	CapturedMass float64 `json:"captured_top_n_mass"`
	// Missing stores the value represented by this member.
	Missing []string `json:"missing_letters,omitempty"`
	// Timing stores the value represented by this member.
	Timing CompactTiming `json:"timing"`
	// Error stores the value represented by this member.
	Error string `json:"error,omitempty"`
	// ErrorCategory stores the value represented by this member.
	ErrorCategory string `json:"error_category,omitempty"`
	// FormatAdherent stores the value represented by this member.
	FormatAdherent *bool `json:"answer_format_adherent,omitempty"`
}

// Metrics is an exported type in this package's public data model.
type Metrics struct {
	// SIA stores the value represented by this member.
	SIA float64 `json:"sia"`
	// UnconditionalSIA stores the value represented by this member.
	UnconditionalSIA float64 `json:"unconditional_correct_letter_probability"`
	// ChanceCorrectedSIA stores the value represented by this member.
	ChanceCorrectedSIA float64 `json:"chance_corrected_sia"`
	// FormatAdherence stores the value represented by this member.
	FormatAdherence float64 `json:"answer_format_adherence"`
	// FormatAdherentCount stores the value represented by this member.
	FormatAdherentCount int `json:"answer_format_adherent_count"`
	// FormatAdherenceExpected stores the value represented by this member.
	FormatAdherenceExpected int `json:"answer_format_adherence_expected"`
	// StrictConsistency stores the value represented by this member.
	StrictConsistency float64 `json:"strict_consistency"`
	// FlipRate stores the value represented by this member.
	FlipRate float64 `json:"flip_rate"`
	// AlwaysCorrect stores the value represented by this member.
	AlwaysCorrect float64 `json:"always_correct"`
	// TopPickAccuracy stores the value represented by this member.
	TopPickAccuracy float64 `json:"top_pick_accuracy"`
	// MeanOrderSensitivityRange stores the value represented by this member.
	MeanOrderSensitivityRange float64 `json:"mean_order_sensitivity_range"`
	// SlotBias stores the value represented by this member.
	SlotBias [4]float64 `json:"slot_bias"`
	// ScoredItems stores the value represented by this member.
	ScoredItems int `json:"scored_items"`
	// ExpectedItems stores the value represented by this member.
	ExpectedItems int `json:"expected_items"`
	// MeanCandidateMass stores the value represented by this member.
	MeanCandidateMass float64 `json:"mean_candidate_mass"`
	// MeanObservedOffLetter stores the value represented by this member.
	MeanObservedOffLetter float64 `json:"mean_observed_off_letter_mass"`
	// MeanUnobservedTail stores the value represented by this member.
	MeanUnobservedTail float64 `json:"mean_unobserved_tail_mass"`
	// DistractorOrderSensitivity stores the value represented by this member.
	DistractorOrderSensitivity float64 `json:"mean_distractor_order_sensitivity"`
	// DistractorTopPickChangeRate stores the value represented by this member.
	DistractorTopPickChangeRate float64 `json:"distractor_top_pick_change_rate"`
	// DistractorPairs stores the value represented by this member.
	DistractorPairs int `json:"distractor_pairs"`
}

// ModelReport is an exported type in this package's public data model.
type ModelReport struct {
	// Status stores the value represented by this member.
	Status string `json:"status"`
	// RequestedModel stores the value represented by this member.
	RequestedModel string `json:"requested_model"`
	// VerifiedModel stores the value represented by this member.
	VerifiedModel string `json:"verified_model,omitempty"`
	// ModelIdentityStatus stores the value represented by this member.
	ModelIdentityStatus string `json:"model_identity_status"`
	// StartedAt stores the value represented by this member.
	StartedAt string `json:"started_at"`
	// FinishedAt stores the value represented by this member.
	FinishedAt string `json:"finished_at,omitempty"`
	// Complete stores the value represented by this member.
	Complete int `json:"complete"`
	// Incomplete stores the value represented by this member.
	Incomplete int `json:"incomplete"`
	// Metrics stores the value represented by this member.
	Metrics *Metrics `json:"metrics"`
	// Results stores the value represented by this member.
	Results []CompactResult `json:"results"`
	// Warnings stores the value represented by this member.
	Warnings []string `json:"warnings,omitempty"`
	// Errors stores the value represented by this member.
	Errors []string `json:"errors,omitempty"`
	// OutputFingerprint stores the value represented by this member.
	OutputFingerprint string `json:"output_fingerprint,omitempty"`
}

// Settings is an exported type in this package's public data model.
type Settings struct {
	// NProbs stores the value represented by this member.
	NProbs int `json:"n_probs"`
	// Seed stores the value represented by this member.
	Seed int `json:"seed"`
	// NPredict stores the value represented by this member.
	NPredict int `json:"n_predict"`
	// Temperature stores the value represented by this member.
	Temperature float64 `json:"temperature"`
	// PostSamplingProbs stores the value represented by this member.
	PostSamplingProbs bool `json:"post_sampling_probs"`
	// Control stores the value represented by this member.
	Control string `json:"control"`
	// Repeat stores the value represented by this member.
	Repeat int `json:"repeat"`
}

// Report is an exported type in this package's public data model.
type Report struct {
	// SchemaVersion stores the value represented by this member.
	SchemaVersion int `json:"schema_version"`
	// RunID stores the value represented by this member.
	RunID string `json:"run_id"`
	// Timestamp stores the value represented by this member.
	Timestamp string `json:"timestamp"`
	// Endpoint stores the value represented by this member.
	Endpoint string `json:"endpoint"`
	// QuestionsPath stores the value represented by this member.
	QuestionsPath string `json:"questions_path"`
	// QuestionsSHA256 stores the value represented by this member.
	QuestionsSHA256 string `json:"questions_file_sha256"`
	// PlanSHA256 stores the value represented by this member.
	PlanSHA256 string `json:"plan_sha256"`
	// Prefixes stores the value represented by this member.
	Prefixes map[string]string `json:"prefixes"`
	// Items stores the value represented by this member.
	Items int `json:"items"`
	// RotationsPerItem stores the value represented by this member.
	RotationsPerItem int `json:"rotations_per_item"`
	// PermutationSets stores the value represented by this member.
	PermutationSets []string `json:"permutation_sets"`
	// ModelOrder stores the value represented by this member.
	ModelOrder []string `json:"model_order"`
	// Settings stores the value represented by this member.
	Settings Settings `json:"settings"`
	// QuestionManifest stores the value represented by this member.
	QuestionManifest []Item `json:"question_manifest"`
	// Models stores the value represented by this member.
	Models map[string]*ModelReport `json:"models"`
	// DuplicateOutputGroups stores the value represented by this member.
	DuplicateOutputGroups map[string][]string `json:"duplicate_output_groups,omitempty"`
	// Notes stores the value represented by this member.
	Notes []string `json:"notes"`
	// Rescore stores the value represented by this member.
	Rescore *RescoreInfo `json:"rescore,omitempty"`
}

// RawRecord is an exported type in this package's public data model.
type RawRecord struct {
	// ModelKey stores the value represented by this member.
	ModelKey string `json:"model_key"`
	// ItemID stores the value represented by this member.
	ItemID string `json:"item_id"`
	// Rotation stores the value represented by this member.
	Rotation int `json:"rotation"`
	// PermutationSet stores the value represented by this member.
	PermutationSet string `json:"permutation_set"`
	// Control stores the value represented by this member.
	Control string `json:"control"`
	// Repeat stores the value represented by this member.
	Repeat int `json:"repeat"`
	// Timestamp stores the value represented by this member.
	Timestamp string `json:"timestamp"`
	// Request stores the value represented by this member.
	Request json.RawMessage `json:"request"`
	// Response stores the value represented by this member.
	Response json.RawMessage `json:"response,omitempty"`
	// HTTPStatus stores the value represented by this member.
	HTTPStatus int `json:"http_status,omitempty"`
	// HTTPMS stores the value represented by this member.
	HTTPMS int64 `json:"http_ms"`
	// Error stores the value represented by this member.
	Error string `json:"error,omitempty"`
}

// RawArchive is an exported type in this package's public data model.
type RawArchive struct {
	// ArchiveSchemaVersion stores the value represented by this member.
	ArchiveSchemaVersion int `json:"archive_schema_version"`
	// Capture stores the value represented by this member.
	Capture string `json:"capture"`
	// RunID stores the value represented by this member.
	RunID string `json:"run_id"`
	// PlanSHA256 stores the value represented by this member.
	PlanSHA256 string `json:"plan_sha256"`
	// Records stores the value represented by this member.
	Records []RawRecord `json:"records"`
}

// RescoreInfo is an exported type in this package's public data model.
type RescoreInfo struct {
	// SourceFile stores the value represented by this member.
	SourceFile string `json:"source_file"`
	// SourceSHA256 stores the value represented by this member.
	SourceSHA256 string `json:"source_sha256"`
	// OriginalRunID stores the value represented by this member.
	OriginalRunID string `json:"original_run_id"`
	// ScoringVersion stores the value represented by this member.
	ScoringVersion string `json:"scoring_version"`
	// Timestamp stores the value represented by this member.
	Timestamp string `json:"timestamp"`
}

var (
	// logLevel holds package-level state used internally by this package.
	logLevel = 1
	// logMu holds package-level state used internally by this package.
	logMu sync.Mutex
)

// logStep performs the internal operation described by its signature.
func logStep(level int, format string, args ...any) {
	if level > logLevel {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	fmt.Fprintf(os.Stderr, "%s [shuffle] %s\n", time.Now().UTC().Format(time.RFC3339Nano), fmt.Sprintf(format, args...))
}

// hashBytes performs the internal operation described by its signature.
func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// jsonHash performs the internal operation described by its signature.
func jsonHash(v any) string { b, _ := json.Marshal(v); return hashBytes(b) }

// cleanControl performs the internal operation described by its signature.
func cleanControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// loadQuestions performs the internal operation described by its signature.
func loadQuestions(path string, maxItems int) ([]Item, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("load questions %q: %w", path, err)
	}
	h := hashBytes(b)
	scanner := bufio.NewScanner(bytes.NewReader(b))
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	seen := map[string]bool{}
	var items []Item
	line := 0
	for scanner.Scan() {
		line++
		raw := bytes.TrimSpace(scanner.Bytes())
		if line == 1 {
			raw = bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
		}
		if len(raw) == 0 || raw[0] == '#' {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		var w itemWire
		if err := dec.Decode(&w); err != nil {
			return nil, "", fmt.Errorf("questions %q line %d: %w", path, line, err)
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return nil, "", fmt.Errorf("questions %q line %d: expected one JSON object", path, line)
		}
		if strings.TrimSpace(w.ID) == "" || !cleanControl(w.ID) || strings.TrimSpace(w.Question) == "" {
			return nil, "", fmt.Errorf("questions %q line %d: id and question are required and must be clean", path, line)
		}
		if seen[w.ID] {
			return nil, "", fmt.Errorf("questions %q line %d: duplicate id %q", path, line, w.ID)
		}
		seen[w.ID] = true
		if len(w.Options) != 4 {
			return nil, "", fmt.Errorf("questions %q line %d: options must contain exactly 4 entries", path, line)
		}
		if w.Correct == nil || *w.Correct < 0 || *w.Correct >= 4 {
			return nil, "", fmt.Errorf("questions %q line %d: correct must be an explicit index 0..3", path, line)
		}
		if w.Difficulty != "easy" && w.Difficulty != "medium" && w.Difficulty != "hard" {
			return nil, "", fmt.Errorf("questions %q line %d: difficulty must be easy, medium, or hard", path, line)
		}
		var it Item
		it.ID, it.Question, it.Correct, it.Difficulty = w.ID, w.Question, *w.Correct, w.Difficulty
		opts := map[string]bool{}
		for i, o := range w.Options {
			o = strings.TrimSpace(o)
			if o == "" || !cleanControl(o) || opts[strings.ToLower(o)] {
				return nil, "", fmt.Errorf("questions %q line %d: options must be nonempty and distinct", path, line)
			}
			opts[strings.ToLower(o)] = true
			it.Options[i] = o
		}
		items = append(items, it)
	}
	if err := scanner.Err(); err != nil {
		return nil, "", fmt.Errorf("questions %q: %w", path, err)
	}
	if len(items) == 0 {
		return nil, "", fmt.Errorf("questions %q: dataset is empty", path)
	}
	if maxItems < 0 {
		return nil, "", errors.New("max-items must not be negative")
	}
	if maxItems > 0 && maxItems < len(items) {
		items = items[:maxItems]
	}
	return items, h, nil
}

// prefixFor performs the internal operation described by its signature.
func prefixFor(control string) (string, error) {
	switch control {
	case "baseline":
		return baselinePrefix, nil
	case "fewshot-balanced":
		return balancedPrefix, nil
	case "no-fewshot":
		return "Answer each multiple-choice question using only its letter (A, B, C, or D).\n\n", nil
	default:
		return "", fmt.Errorf("unknown control %q", control)
	}
}

// buildPlan performs the internal operation described by its signature.
func buildPlan(items []Item, controls []string, questionsHash string, settings Settings) (EvaluationPlan, error) {
	permutationSets := []string{"cyclic", "distractor-reversed"}
	p := EvaluationPlan{Controls: controls, Prefixes: map[string]string{}, QuestionsSHA256: questionsHash}
	for _, c := range controls {
		prefix, err := prefixFor(c)
		if err != nil {
			return p, err
		}
		p.Prefixes[c] = prefix
	}
	for _, it := range items {
		pi := PlanItem{Item: it}
		for _, c := range controls {
			for _, permutationSet := range permutationSets {
				for rot := 0; rot < 4; rot++ {
					var opts [4]string
					var mapping [4]int
					if permutationSet == "cyclic" {
						for i := 0; i < 4; i++ {
							mapping[(i+rot)%4] = i
							opts[(i+rot)%4] = it.Options[i]
						}
					} else {
						correctSlot := (it.Correct + rot) % 4
						mapping[correctSlot] = it.Correct
						opts[correctSlot] = it.Options[it.Correct]
						wrong := make([]int, 0, 3)
						for slot := 0; slot < 4; slot++ {
							original := (slot - rot + 4) % 4
							if original != it.Correct {
								wrong = append(wrong, original)
							}
						}
						for i, j := 0, len(wrong)-1; i < len(wrong); i, j = i+1, j-1 {
							slot := i
							if slot >= correctSlot {
								slot++
							}
							mapping[slot] = wrong[j]
							opts[slot] = it.Options[wrong[j]]
						}
					}
					prompt := p.Prefixes[c] + "Question: " + it.Question + "\n"
					for i, o := range opts {
						prompt += fmt.Sprintf("%c. %s\n", 'A'+i, o)
					}
					prompt += "Answer:"
					pi.Rotations = append(pi.Rotations, Rotation{PermutationSet: permutationSet, Prompt: prompt, LetterToOption: mapping, CorrectOption: it.Correct, PromptSHA256: hashBytes([]byte(prompt))})
				}
			}
		}
		p.Items = append(p.Items, pi)
	}
	planMaterial := struct {
		Items    []PlanItem
		Controls []string
		Settings Settings
	}{p.Items, p.Controls, settings}
	p.PlanSHA256 = jsonHash(planMaterial)
	return p, nil
}

// parseModels performs the internal operation described by its signature.
func parseModels(single, multiple string) ([]string, error) {
	if single != "" && multiple != "" {
		return nil, errors.New("--model cannot be combined with --models")
	}
	raw := single
	if multiple != "" {
		raw = multiple
	}
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("--model or --models is required")
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, errors.New("model list contains an empty entry")
		}
		if seen[s] {
			return nil, fmt.Errorf("duplicate model %q", s)
		}
		seen[s] = true
		out = append(out, s)
	}
	return out, nil
}

// loadAllModels performs the internal operation described by its signature.
func loadAllModels(ctx context.Context, endpoint string, client *http.Client) ([]string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/v1/models", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create models request: %w", err)
	}
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch models from %q: %w", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch models from %q: unexpected status %s", endpoint, response.Status)
	}
	b, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read models response: %w", err)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, fmt.Errorf("parse models response: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("models endpoint %q returned no models", endpoint)
	}
	seen := map[string]bool{}
	models := make([]string, 0, len(payload.Data))
	for _, model := range payload.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, id)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("models endpoint %q returned no usable model IDs", endpoint)
	}
	return models, nil
}

// tokenText performs the internal operation described by its signature.
func tokenText(t Token) string {
	if len(t.Bytes) == 0 {
		return t.Token
	}
	b := make([]byte, len(t.Bytes))
	for i, v := range t.Bytes {
		if v < 0 || v > 255 {
			return ""
		}
		b[i] = byte(v)
	}
	return string(b)
}

// letterIndex performs the internal operation described by its signature.
func letterIndex(s string) int {
	s = strings.TrimLeft(s, " \t\r\n")
	if len(s) != 1 || s[0] < 'A' || s[0] > 'D' {
		return -1
	}
	return int(s[0] - 'A')
}

// post performs the internal operation described by its signature.
func post(ctx context.Context, client *http.Client, endpoint string, body any, debug bool) (Completion, int64, []byte, int, error) {
	var out Completion
	b, err := json.Marshal(body)
	if err != nil {
		return out, 0, nil, 0, err
	}
	if debug {
		logStep(2, "request POST %s body=%s", endpoint, bounded(string(b), 4000))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return out, 0, nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	started := time.Now()
	resp, err := client.Do(req)
	wall := time.Since(started).Milliseconds()
	if err != nil {
		logStep(1, "response POST %s failed after %dms: %v", endpoint, wall, err)
		return out, wall, nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return out, wall, raw, resp.StatusCode, err
	}
	if debug {
		logStep(2, "response POST %s status=%s body=%s", endpoint, resp.Status, bounded(string(raw), 4000))
	}
	if resp.StatusCode != http.StatusOK {
		return out, wall, raw, resp.StatusCode, fmt.Errorf("HTTP %s: %.500s", resp.Status, raw)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, wall, raw, resp.StatusCode, fmt.Errorf("decode response: %w", err)
	}
	if len(out.Probs) == 0 {
		out.Probs = out.CompletionProbabilities
	}
	return out, wall, raw, resp.StatusCode, nil
}

// bounded performs the internal operation described by its signature.
func bounded(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…[truncated]"
}

// errorCategory performs the internal operation described by its signature.
func errorCategory(err string) string {
	s := strings.ToLower(err)
	switch {
	case strings.Contains(s, "candidate_coverage"):
		return "candidate_coverage"
	case strings.Contains(s, "invalid distribution"):
		return "invalid_distribution"
	case strings.Contains(s, "truncated"):
		return "truncated"
	case strings.Contains(s, "http "):
		return "server_error"
	case strings.Contains(s, "timeout"), strings.Contains(s, "deadline exceeded"):
		return "timeout"
	default:
		return "request_error"
	}
}

// compactResult performs the internal operation described by its signature.
func compactResult(r Result) CompactResult {
	c := CompactResult{ItemID: r.ItemID, Rotation: r.Rotation, PermutationSet: r.PermutationSet, Control: r.Control, Repeat: r.Repeat, LetterToOption: r.LetterToOption, LetterMass: r.LetterMass, CapturedMass: r.CapturedMass, Missing: r.Missing, Error: r.Error, Timing: CompactTiming{HTTPMS: r.HTTPMS}, FormatAdherent: r.FormatAdherent}
	if r.Error != "" {
		c.ErrorCategory = errorCategory(r.Error)
	}
	if len(r.Timings) > 0 {
		var timing struct {
			CacheN      *int     `json:"cache_n"`
			PromptN     *int     `json:"prompt_n"`
			PromptMS    *float64 `json:"prompt_ms"`
			PredictedN  *int     `json:"predicted_n"`
			PredictedMS *float64 `json:"predicted_ms"`
		}
		if json.Unmarshal(r.Timings, &timing) == nil {
			c.Timing.CacheTokens, c.Timing.PromptTokens, c.Timing.PromptMS = timing.CacheN, timing.PromptN, timing.PromptMS
			c.Timing.PredictedTokens, c.Timing.PredictedMS = timing.PredictedN, timing.PredictedMS
		}
	}
	return c
}

// resultKey performs the internal operation described by its signature.
func resultKey(itemID string, rotation int, permutationSet, control string, repeat int) string {
	return fmt.Sprintf("%s/%d/%s/%s/%d", itemID, rotation, permutationSet, control, repeat)
}

// expandCompactResult performs the internal operation described by its signature.
func expandCompactResult(compact CompactResult, item Item) (Result, error) {
	r := Result{
		ItemID: compact.ItemID, Rotation: compact.Rotation, PermutationSet: compact.PermutationSet,
		Control: compact.Control, Repeat: compact.Repeat, LetterToOption: compact.LetterToOption,
		LetterMass: compact.LetterMass, CapturedMass: compact.CapturedMass, Missing: compact.Missing,
		Error: compact.Error, HTTPMS: compact.Timing.HTTPMS, FormatAdherent: compact.FormatAdherent,
	}
	for _, mass := range r.LetterMass {
		r.CandidateMass += mass
	}
	if r.Error != "" || len(r.Missing) > 0 || r.CandidateMass <= 0 {
		return r, nil
	}
	for i, mass := range r.LetterMass {
		r.Normalized[i] = mass / r.CandidateMass
	}
	best := 0
	for i := 1; i < 4; i++ {
		if r.LetterMass[i] > r.LetterMass[best] {
			best = i
		}
	}
	if best < 0 || best >= len(r.LetterToOption) {
		return Result{}, fmt.Errorf("invalid top-letter index %d for item %q", best, item.ID)
	}
	pick := r.LetterToOption[best]
	if pick < 0 || pick >= len(item.Options) {
		return Result{}, fmt.Errorf("invalid option mapping for item %q", item.ID)
	}
	r.TopPickOption = &pick
	r.TopPickContent = item.Options[pick]
	correctLetter := -1
	for letter, option := range r.LetterToOption {
		if option == item.Correct {
			correctLetter = letter
			break
		}
	}
	if correctLetter < 0 {
		return Result{}, fmt.Errorf("invalid correct option mapping for item %q", item.ID)
	}
	r.CorrectLetter = string(rune('A' + correctLetter))
	probability := r.Normalized[correctLetter]
	r.CorrectProbability = &probability
	return r, nil
}

// answerFormatAdherent performs the internal operation described by its signature.
func answerFormatAdherent(content string) bool {
	return letterIndex(content) >= 0
}

// modelOutputFingerprint performs the internal operation described by its signature.
func modelOutputFingerprint(results []CompactResult) string {
	return jsonHash(results)
}

// score performs the internal operation described by its signature.
func score(c Completion, plan Rotation, item Item, control string, repeat int, wall int64) Result {
	format := answerFormatAdherent(c.Content)
	r := Result{ItemID: item.ID, Rotation: 0, PermutationSet: plan.PermutationSet, Control: control, Repeat: repeat, CorrectLetter: string(rune('A' + plan.LetterToOptionIndexForCorrect(item.Correct))), LetterToOption: plan.LetterToOption, Prompt: plan.Prompt, Generated: c.Content, Timings: c.Timings, HTTPMS: wall, FormatAdherent: &format}
	if c.Truncated {
		r.Error = "prompt truncated by server"
		return r
	}
	if len(c.Probs) == 0 || len(c.Probs[0].Top) == 0 {
		r.Error = "candidate_coverage: no top_logprobs"
		return r
	}
	r.TopTokens = c.Probs[0].Top
	seen := map[int]bool{}
	for _, t := range r.TopTokens {
		if seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		if t.Logprob == nil || math.IsNaN(*t.Logprob) || math.IsInf(*t.Logprob, 0) {
			r.Error = "invalid distribution: missing or nonfinite logprob"
			return r
		}
		p := math.Exp(*t.Logprob)
		if p < 0 || p > 1.000001 {
			r.Error = "invalid distribution: probability out of range"
			return r
		}
		r.CapturedMass += p
		if idx := letterIndex(tokenText(t)); idx >= 0 {
			r.LetterMass[idx] += p
		}
	}
	if r.CapturedMass > 1.0001 {
		r.Error = "invalid distribution: captured mass exceeds one"
		return r
	}
	r.UnobservedMass = math.Max(0, 1-r.CapturedMass)
	for i, v := range r.LetterMass {
		if v == 0 {
			r.Missing = append(r.Missing, string(rune('A'+i)))
		}
	}
	if len(r.Missing) > 0 {
		r.Error = "candidate_coverage: missing letters " + strings.Join(r.Missing, ",")
		return r
	}
	for _, v := range r.LetterMass {
		r.CandidateMass += v
	}
	if r.CandidateMass <= 0 {
		r.Error = "candidate_coverage: zero candidate mass"
		return r
	}
	best := 0
	for i, v := range r.LetterMass {
		r.Normalized[i] = v / r.CandidateMass
		if v > r.LetterMass[best] {
			best = i
		}
	}
	pick := plan.LetterToOption[best]
	r.TopPickOption = &pick
	r.TopPickContent = item.Options[pick]
	correctLetter := 0
	for i, v := range plan.LetterToOption {
		if v == item.Correct {
			correctLetter = i
		}
	}
	r.CorrectLetter = string(rune('A' + correctLetter))
	cp := r.Normalized[correctLetter]
	r.CorrectProbability = &cp
	return r
}

// LetterToOptionIndexForCorrect is an exported operation provided by this package.
func (r Rotation) LetterToOptionIndexForCorrect(correct int) int {
	for i, v := range r.LetterToOption {
		if v == correct {
			return i
		}
	}
	return 0
}

// verifyModel performs the internal operation described by its signature.
func verifyModel(ctx context.Context, client *http.Client, base, requested string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/models", http.NoBody)
	if err != nil {
		return "", "unverified", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "unverified", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", "unverified", fmt.Errorf("model identity endpoint returned %s", resp.Status)
	}
	var v struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return "", "unverified", errors.New("model identity response was not JSON")
	}
	ids := []string{}
	for _, m := range v.Data {
		ids = append(ids, m.ID)
	}
	for _, m := range v.Models {
		ids = append(ids, m.ID)
	}
	for _, id := range ids {
		if id == requested {
			return id, "verified", nil
		}
	}
	return "", "mismatch", fmt.Errorf("requested model %q was not listed by server", requested)
}

// atomicWrite performs the internal operation described by its signature.
func atomicWrite(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".shuffle-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// writeRawArchive performs the internal operation described by its signature.
func writeRawArchive(path string, archive RawArchive) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".shuffle-raw-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	gw := gzip.NewWriter(f)
	if err := json.NewEncoder(gw).Encode(archive); err != nil {
		_ = gw.Close()
		_ = f.Close()
		return err
	}
	if err := gw.Close(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// loadResumableReport performs the internal operation described by its signature.
func loadResumableReport(paths []string, questionsHash, planHash, endpoint string, settings Settings, items int) (*Report, string, error) {
	var lastErr error
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			lastErr = fmt.Errorf("read resume report %q: %w", path, err)
			continue
		}
		var report Report
		if err := json.Unmarshal(b, &report); err != nil {
			lastErr = fmt.Errorf("parse resume report %q: %w", path, err)
			continue
		}
		if report.SchemaVersion != 4 || report.QuestionsSHA256 != questionsHash || report.PlanSHA256 != planHash ||
			strings.TrimRight(report.Endpoint, "/") != strings.TrimRight(endpoint, "/") || report.Items != items || report.Settings != settings {
			lastErr = fmt.Errorf("resume report %q does not match the current SIA plan", path)
			continue
		}
		if report.Models == nil {
			report.Models = map[string]*ModelReport{}
		}
		return &report, path, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no resume report found")
	}
	return nil, "", lastErr
}

// modelRunComplete performs the internal operation described by its signature.
func modelRunComplete(model *ModelReport, expected int) bool {
	return model != nil && model.Status == "complete" && model.Metrics != nil && model.Complete == expected && model.Incomplete == 0
}

// aggregate performs the internal operation described by its signature.
func aggregate(results []Result, expected int) *Metrics {
	groups := map[string][]Result{}
	for _, r := range results {
		if r.Control == "baseline" && r.Repeat == 0 && r.PermutationSet == "cyclic" {
			groups[r.ItemID] = append(groups[r.ItemID], r)
		}
	}
	expectedItems := expected / 4
	if len(groups) != expectedItems {
		return nil
	}
	m := &Metrics{ExpectedItems: expectedItems}
	completeRotations := 0
	consistent, alwaysCorrect := 0, 0
	for _, rs := range groups {
		if len(rs) != 4 {
			return nil
		}
		m.ScoredItems++
		minP, maxP := math.Inf(1), math.Inf(-1)
		same, allCorrect := true, true
		firstPick := -1
		for _, r := range rs {
			if r.Error != "" || r.CorrectProbability == nil || r.TopPickOption == nil {
				return nil
			}
			completeRotations++
			p := *r.CorrectProbability
			m.SIA += p
			correctLetter := int(r.CorrectLetter[0] - 'A')
			if correctLetter >= 0 && correctLetter < 4 {
				m.UnconditionalSIA += r.LetterMass[correctLetter]
			}
			if p < minP {
				minP = p
			}
			if p > maxP {
				maxP = p
			}
			if firstPick < 0 {
				firstPick = *r.TopPickOption
			} else if firstPick != *r.TopPickOption {
				same = false
			}
			if *r.TopPickOption != r.LetterToOption[correctLetter] {
				allCorrect = false
			}
			for i := range r.Normalized {
				m.SlotBias[i] += r.Normalized[i]
			}
			m.MeanCandidateMass += r.CandidateMass
			m.MeanObservedOffLetter += math.Max(0, r.CapturedMass-r.CandidateMass)
			m.MeanUnobservedTail += math.Max(0, 1-r.CapturedMass)
			if r.FormatAdherent != nil {
				if *r.FormatAdherent {
					m.FormatAdherentCount++
				}
				m.FormatAdherenceExpected++
			}
		}
		m.MeanOrderSensitivityRange += maxP - minP
		if same {
			consistent++
		}
		if allCorrect {
			alwaysCorrect++
		}
	}
	if completeRotations == 0 || m.ScoredItems != expectedItems {
		return nil
	}
	m.SIA /= float64(completeRotations)
	m.UnconditionalSIA /= float64(completeRotations)
	m.ChanceCorrectedSIA = (m.SIA - .25) / .75
	m.StrictConsistency = float64(consistent) / float64(m.ScoredItems)
	m.FlipRate = 1 - m.StrictConsistency
	m.AlwaysCorrect = float64(alwaysCorrect) / float64(m.ScoredItems)
	for _, rs := range groups {
		for _, r := range rs {
			if r.TopPickOption != nil {
				correctLetter := int(r.CorrectLetter[0] - 'A')
				if correctLetter >= 0 && correctLetter < 4 && *r.TopPickOption == r.LetterToOption[correctLetter] {
					m.TopPickAccuracy++
				}
			}
		}
	}
	m.TopPickAccuracy /= float64(completeRotations)
	m.MeanOrderSensitivityRange /= float64(m.ScoredItems)
	for i := range m.SlotBias {
		m.SlotBias[i] /= float64(completeRotations)
	}
	m.MeanCandidateMass /= float64(completeRotations)
	m.MeanObservedOffLetter /= float64(completeRotations)
	m.MeanUnobservedTail /= float64(completeRotations)
	if m.FormatAdherenceExpected > 0 {
		m.FormatAdherence = float64(m.FormatAdherentCount) / float64(m.FormatAdherenceExpected)
	}
	cyclic, distractor := map[string]Result{}, map[string]Result{}
	for _, r := range results {
		if r.Control != "baseline" || r.Repeat != 0 || r.Error != "" || r.CorrectProbability == nil {
			continue
		}
		key := fmt.Sprintf("%s/%d", r.ItemID, r.Rotation)
		if r.PermutationSet == "cyclic" {
			cyclic[key] = r
		}
		if r.PermutationSet == "distractor-reversed" {
			distractor[key] = r
		}
	}
	for key, base := range cyclic {
		other, ok := distractor[key]
		if !ok || other.TopPickOption == nil || base.TopPickOption == nil || other.CorrectProbability == nil {
			continue
		}
		m.DistractorOrderSensitivity += math.Abs(*base.CorrectProbability - *other.CorrectProbability)
		if *base.TopPickOption != *other.TopPickOption {
			m.DistractorTopPickChangeRate++
		}
		m.DistractorPairs++
	}
	if m.DistractorPairs > 0 {
		m.DistractorOrderSensitivity /= float64(m.DistractorPairs)
		m.DistractorTopPickChangeRate /= float64(m.DistractorPairs)
	}
	return m
}

// rescoreReport performs the internal operation described by its signature.
func rescoreReport(path, out, questionsPath string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read rescore input %q: %w", path, err)
	}
	var report Report
	if err := json.Unmarshal(b, &report); err != nil {
		return fmt.Errorf("parse rescore input %q: %w", path, err)
	}
	if report.SchemaVersion != 3 && report.SchemaVersion != 4 {
		return fmt.Errorf("unsupported report schema_version %d; expected 3 or 4", report.SchemaVersion)
	}
	if len(report.QuestionManifest) == 0 {
		items, hash, loadErr := loadQuestions(questionsPath, 0)
		if loadErr != nil {
			return fmt.Errorf("schema-%d rescore requires question manifest or matching --questions: %w", report.SchemaVersion, loadErr)
		}
		if report.QuestionsSHA256 != "" && report.QuestionsSHA256 != hash {
			return fmt.Errorf("question bank hash %s does not match report hash %s", hash, report.QuestionsSHA256)
		}
		report.QuestionManifest = items
	}
	items := map[string]Item{}
	for _, item := range report.QuestionManifest {
		items[item.ID] = item
	}
	for modelID, model := range report.Models {
		var rawResults []Result
		for _, compact := range model.Results {
			item, ok := items[compact.ItemID]
			if !ok {
				return fmt.Errorf("model %q result references unknown item %q", modelID, compact.ItemID)
			}
			permutationSet := compact.PermutationSet
			if permutationSet == "" {
				permutationSet = "cyclic"
			}
			r := Result{ItemID: compact.ItemID, Rotation: compact.Rotation, PermutationSet: permutationSet, Control: compact.Control, Repeat: compact.Repeat, LetterToOption: compact.LetterToOption, LetterMass: compact.LetterMass, CapturedMass: compact.CapturedMass, Missing: compact.Missing, Error: compact.Error, HTTPMS: compact.Timing.HTTPMS, FormatAdherent: compact.FormatAdherent}
			for _, mass := range r.LetterMass {
				r.CandidateMass += mass
			}
			if r.Error == "" && len(r.Missing) == 0 && r.CandidateMass > 0 {
				for i, mass := range r.LetterMass {
					r.Normalized[i] = mass / r.CandidateMass
				}
				best := 0
				for i := 1; i < 4; i++ {
					if r.Normalized[i] > r.Normalized[best] {
						best = i
					}
				}
				pick := r.LetterToOption[best]
				r.TopPickOption = &pick
				r.TopPickContent = item.Options[pick]
				correctLetter := -1
				for letter, option := range r.LetterToOption {
					if option == item.Correct {
						correctLetter = letter
						break
					}
				}
				if correctLetter < 0 {
					return fmt.Errorf("model %q result %s rotation %d has invalid option mapping", modelID, r.ItemID, r.Rotation)
				}
				letter := string(rune('A' + correctLetter))
				r.CorrectLetter = letter
				probability := r.Normalized[correctLetter]
				r.CorrectProbability = &probability
			}
			rawResults = append(rawResults, r)
		}
		model.Metrics = aggregate(rawResults, report.Items*4)
		if len(model.Results) > 0 {
			model.OutputFingerprint = modelOutputFingerprint(model.Results)
		}
		for _, raw := range rawResults {
			for i := range model.Results {
				if model.Results[i].ItemID == raw.ItemID && model.Results[i].Rotation == raw.Rotation && model.Results[i].PermutationSet == raw.PermutationSet && model.Results[i].Control == raw.Control && model.Results[i].Repeat == raw.Repeat {
					model.Results[i].CapturedMass = raw.CapturedMass
					model.Results[i].Missing = raw.Missing
					model.Results[i].Error = raw.Error
					model.Results[i].ErrorCategory = errorCategory(raw.Error)
					break
				}
			}
		}
	}
	duplicateGroups := map[string][]string{}
	for modelID, model := range report.Models {
		if model.OutputFingerprint != "" {
			duplicateGroups[model.OutputFingerprint] = append(duplicateGroups[model.OutputFingerprint], modelID)
		}
	}
	for fingerprint, ids := range duplicateGroups {
		if len(ids) < 2 {
			delete(duplicateGroups, fingerprint)
		}
	}
	if len(duplicateGroups) > 0 {
		report.DuplicateOutputGroups = duplicateGroups
	}
	report.SchemaVersion = 4
	report.Rescore = &RescoreInfo{SourceFile: path, SourceSHA256: hashBytes(b), OriginalRunID: report.RunID, ScoringVersion: "compact-v1", Timestamp: time.Now().UTC().Format(time.RFC3339)}
	return atomicWrite(out, &report)
}

// main performs the internal operation described by its signature.
// Additional context: Support the legacy `go run <directory> eval.go` form, which passes the source filename as the first program argument.
// Additional context: Preserve the first 501 result and avoid issuing the remaining doomed requests.
func main() {
	logStep(1, "step: parse command line")
	var url, model, models, questions, out, control, rawOut, rescore, resumeFrom string
	var nprobs, seed, maxItems, repeat, verbose int
	var timeout time.Duration
	var allow, failFast, modelsAll, raw, resume bool
	flag.StringVar(&url, "url", "http://127.0.0.1:9998", "llama.cpp server base URL")
	flag.StringVar(&model, "model", "", "single model ID")
	flag.StringVar(&models, "models", "", "comma-separated model IDs")
	flag.BoolVar(&modelsAll, "models-all", false, "evaluate every model ID returned by /v1/models")
	flag.BoolVar(&raw, "raw", false, "write a separate lossless gzip diagnostics archive")
	flag.StringVar(&rawOut, "raw-out", "", "raw gzip archive path; requires --raw")
	flag.StringVar(&rescore, "rescore", "", "offline-rescore an existing compact or supported legacy report")
	flag.StringVar(&questions, "questions", "data/sia/shuffle_questions_320.jsonl", "JSONL question bank")
	flag.StringVar(&out, "out", "data/dashboard/sia.json", "output JSON path")
	flag.StringVar(&control, "control", "baseline", "baseline, fewshot-balanced, no-fewshot, or all")
	flag.IntVar(&nprobs, "nprobs", 100, "top tokens returned per request")
	flag.IntVar(&seed, "seed", 42, "fixed seed")
	flag.IntVar(&maxItems, "max-items", 0, "select first N validated items; zero means all")
	flag.IntVar(&repeat, "repeat", 1, "positive repetitions")
	flag.DurationVar(&timeout, "timeout", 3*time.Minute, "per-request timeout")
	flag.BoolVar(&allow, "allow-unverified-model", false, "allow provisional scoring when identity cannot be verified")
	flag.BoolVar(&failFast, "fail-fast", false, "stop after the first model failure")
	flag.BoolVar(&resume, "resume", false, "reuse complete model runs from an existing compatible report")
	flag.StringVar(&resumeFrom, "resume-from", "", "additional report path to use when resuming")
	flag.IntVar(&verbose, "verbose", 1, "0=errors, 1=concise, 2=debug HTTP bodies")

	if len(os.Args) > 1 && filepath.Base(os.Args[1]) == "eval.go" {
		os.Args = append(os.Args[:1], os.Args[2:]...)
		logStep(1, "step: ignored compatibility argument eval.go")
	}
	flag.Parse()
	logLevel = verbose
	if rescore != "" {
		if raw {
			fmt.Fprintln(os.Stderr, "--raw cannot be combined with --rescore")
			os.Exit(2)
		}
		if rawOut != "" {
			fmt.Fprintln(os.Stderr, "--raw-out cannot be combined with --rescore")
			os.Exit(2)
		}
		if out == "data/dashboard/sia.json" && filepath.Clean(rescore) == filepath.Clean(out) {
			out = "data/dashboard/sia.rescored.json"
		}
		if filepath.Clean(rescore) == filepath.Clean(out) {
			fmt.Fprintln(os.Stderr, "--rescore input and --out must be different paths")
			os.Exit(2)
		}
		if err := rescoreReport(rescore, out, questions); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("Rescored results:", out)
		return
	}
	if rawOut != "" && !raw {
		fmt.Fprintln(os.Stderr, "--raw-out requires --raw")
		os.Exit(2)
	}
	if raw {
		if rawOut == "" {
			rawOut = strings.TrimSuffix(out, filepath.Ext(out)) + ".raw.json.gz"
		}
		if filepath.Clean(rawOut) == filepath.Clean(out) {
			fmt.Fprintln(os.Stderr, "--raw-out must differ from --out")
			os.Exit(2)
		}
	}
	if nprobs < 4 {
		fmt.Fprintln(os.Stderr, "--nprobs must be >=4")
		os.Exit(2)
	}
	if repeat <= 0 {
		fmt.Fprintln(os.Stderr, "--repeat must be positive")
		os.Exit(2)
	}
	if maxItems < 0 {
		fmt.Fprintln(os.Stderr, "--max-items must not be negative")
		os.Exit(2)
	}
	if modelsAll && (strings.TrimSpace(model) != "" || strings.TrimSpace(models) != "") {
		fmt.Fprintln(os.Stderr, "--models-all cannot be combined with --model or --models")
		os.Exit(2)
	}
	var modelsList []string
	var err error
	if modelsAll {
		modelsList, err = loadAllModels(context.Background(), url, nil)
	} else {
		modelsList, err = parseModels(model, models)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if control != "all" {
		if _, err = prefixFor(control); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	items, qhash, err := loadQuestions(questions, maxItems)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	settings := Settings{NProbs: nprobs, Seed: seed, NPredict: 1, Temperature: -1, PostSamplingProbs: false, Control: control, Repeat: repeat}
	controls := []string{control}
	if control == "all" {
		controls = []string{"baseline", "fewshot-balanced", "no-fewshot"}
	}
	plan, err := buildPlan(items, controls, qhash, settings)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	logStep(1, "step: loaded %d validated items questions_sha256=%s plan_sha256=%s", len(items), qhash, plan.PlanSHA256)
	fmt.Printf("Progress: %d models total, %d items/model, %d permutation sets, %d rotation(s)/set\n", len(modelsList), len(items), 2, 4)
	permutationSetNames := []string{"cyclic", "distractor-reversed"}
	report := &Report{SchemaVersion: 4, RunID: time.Now().UTC().Format("20060102T150405Z"), Timestamp: time.Now().UTC().Format(time.RFC3339), Endpoint: strings.TrimRight(url, "/"), QuestionsPath: questions, QuestionsSHA256: qhash, PlanSHA256: plan.PlanSHA256, Prefixes: plan.Prefixes, Items: len(items), RotationsPerItem: 8, PermutationSets: permutationSetNames, ModelOrder: modelsList, Settings: settings, QuestionManifest: items, Models: map[string]*ModelReport{}, Notes: []string{"Compact report omits prompts, generated tokens, and top-token arrays by default.", "Each item has four cyclic letter-position rotations plus four distractor-reversed rotations.", "Use --raw for a separate lossless diagnostic archive.", "With only compact letter masses, tokenizer classification cannot be revisited during rescore.", "Cross-model comparisons are descriptive only."}}
	if resume {
		resumePaths := []string{out, resumeFrom}
		if existing, source, loadErr := loadResumableReport(resumePaths, qhash, plan.PlanSHA256, report.Endpoint, settings, len(items)); loadErr == nil {
			report = existing
			report.Endpoint = strings.TrimRight(url, "/")
			report.QuestionsPath = questions
			report.ModelOrder = modelsList
			logStep(1, "resuming compatible report %s", source)
		} else if loadErr != nil {
			logStep(1, "no compatible report to resume: %v", loadErr)
		}
	}
	for _, id := range modelsList {
		if _, exists := report.Models[id]; !exists {
			report.Models[id] = &ModelReport{Status: "pending", RequestedModel: id, ModelIdentityStatus: "unverified"}
		}
	}
	_ = atomicWrite(out, report)
	client := &http.Client{Timeout: timeout}
	ctx := context.Background()
	failed := false
	rawArchive := RawArchive{ArchiveSchemaVersion: 1, Capture: "full_diagnostic_report", RunID: report.RunID, PlanSHA256: report.PlanSHA256}
	totalModels := len(modelsList)
	for modelIndex, id := range modelsList {
		mr := report.Models[id]
		if resume && modelRunComplete(mr, len(items)*len(controls)*repeat*8) {
			fmt.Printf("Model %d/%d skip: %s status=complete (already evaluated)\n", modelIndex+1, totalModels, id)
			continue
		}
		modelResults := make([]Result, 0, len(items)*4)
		existingResults := map[string]int{}
		for index, compact := range mr.Results {
			existingResults[resultKey(compact.ItemID, compact.Rotation, compact.PermutationSet, compact.Control, compact.Repeat)] = index
		}
		mr.Complete = 0
		mr.Incomplete = 0
		mr.Errors = nil
		mr.Metrics = nil
		mr.Status = "running"
		mr.StartedAt = time.Now().UTC().Format(time.RFC3339)
		fmt.Printf("Model %d/%d start: %s\n", modelIndex+1, totalModels, id)
		logStep(1, "model %d/%d start requested=%q", modelIndex+1, totalModels, id)
		verified, status, verr := verifyModel(ctx, client, url, id)
		mr.ModelIdentityStatus = status
		if verr == nil {
			mr.VerifiedModel = verified
		} else {
			mr.Errors = append(mr.Errors, verr.Error())
		}
		if status != "verified" && !allow {
			mr.Status = "unverified"
			mr.Errors = append(mr.Errors, "model identity could not be confirmed; use --allow-unverified-model for provisional scoring")
			failed = true
			mr.FinishedAt = time.Now().UTC().Format(time.RFC3339)
			_ = atomicWrite(out, report)
			fmt.Printf("Model %d/%d finish: %s status=%s complete=%d incomplete=%d\n", modelIndex+1, totalModels, id, mr.Status, mr.Complete, mr.Incomplete)
			if failFast {
				break
			}
			continue
		}
		modelUnsupported := false
	modelRun:
		for itemIndex, pi := range plan.Items {
			for repeatID := 0; repeatID < repeat; repeatID++ {
				for controlID, c := range controls {
					for permutationID, permutationSet := range permutationSetNames {
						for rot := 0; rot < 4; rot++ {
							idx := controlID*8 + permutationID*4 + rot
							rotation := pi.Rotations[idx]
							key := resultKey(pi.Item.ID, rot, permutationSet, c, repeatID)
							if resume {
								if existingIndex, exists := existingResults[key]; exists {
									compact := mr.Results[existingIndex]
									if compact.Error == "" {
										if existingResult, expandErr := expandCompactResult(compact, pi.Item); expandErr == nil && existingResult.Error == "" {
											modelResults = append(modelResults, existingResult)
											mr.Complete++
											continue
										}
									}
								}
							}
							payload := map[string]any{"prompt": rotation.Prompt, "n_predict": 1, "n_probs": nprobs, "temperature": -1, "post_sampling_probs": false, "top_k": 0, "top_p": 1.0, "min_p": 0, "typical_p": 1.0, "repeat_penalty": 1.0, "presence_penalty": 0, "frequency_penalty": 0, "dry_multiplier": 0, "xtc_probability": 0, "seed": seed, "cache_prompt": true, "stream": false, "return_tokens": true, "model": id}
							logStep(1, "step: model %d/%d item %d/%d control %d/%d repeat %d/%d permutation %s rotation %d/%d request model=%q item=%s control=%s", modelIndex+1, totalModels, itemIndex+1, len(plan.Items), controlID+1, len(controls), repeatID+1, repeat, permutationSet, rot+1, 4, id, pi.Item.ID, c)
							response, wall, responseBody, httpStatus, callErr := post(ctx, client, report.Endpoint+"/completion", payload, verbose >= 2)
							var result Result
							if callErr != nil {
								result = Result{ItemID: pi.Item.ID, Rotation: rot, PermutationSet: permutationSet, Control: c, Repeat: repeatID, LetterToOption: rotation.LetterToOption, Prompt: rotation.Prompt, Error: callErr.Error(), HTTPMS: wall}
							} else {
								result = score(response, rotation, pi.Item, c, repeatID, wall)
								result.Rotation = rot
							}
							modelResults = append(modelResults, result)
							compact := compactResult(result)
							if existingIndex, exists := existingResults[key]; exists {
								mr.Results[existingIndex] = compact
							} else {
								existingResults[key] = len(mr.Results)
								mr.Results = append(mr.Results, compact)
							}
							if raw {
								requestBody, _ := json.Marshal(payload)
								rawArchive.Records = append(rawArchive.Records, RawRecord{ModelKey: id, ItemID: pi.Item.ID, Rotation: rot, PermutationSet: permutationSet, Control: c, Repeat: repeatID, Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Request: requestBody, Response: responseBody, HTTPStatus: httpStatus, HTTPMS: wall, Error: result.Error})
							}
							if result.Error != "" {
								mr.Incomplete++
							} else {
								mr.Complete++
							}
							_ = atomicWrite(out, report)
							if httpStatus == http.StatusNotImplemented {
								modelUnsupported = true
								failed = true
								mr.Errors = append(mr.Errors, "server does not support /completion for this model or request")
								break modelRun
							}
							if result.Error != "" && failFast {
								break
							}
						}
					}
				}
			}
		}
		if modelUnsupported {
			mr.Status = "unsupported"
			mr.Incomplete = len(items)*len(controls)*repeat*8 - mr.Complete
		} else {
			mr.Metrics = aggregate(modelResults, len(items)*4)
		}
		if !modelUnsupported {
			if mr.Metrics == nil {
				mr.Status = "incomplete"
				failed = true
			} else {
				mr.Status = "complete"
			}
		}
		if len(mr.Results) > 0 {
			mr.OutputFingerprint = modelOutputFingerprint(mr.Results)
		}
		mr.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		_ = atomicWrite(out, report)
		fmt.Printf("Model %d/%d finish: %s status=%s complete=%d incomplete=%d\n", modelIndex+1, totalModels, id, mr.Status, mr.Complete, mr.Incomplete)
		logStep(1, "model %d/%d finish requested=%q status=%s complete=%d incomplete=%d", modelIndex+1, totalModels, id, mr.Status, mr.Complete, mr.Incomplete)
		if failFast && mr.Status != "complete" {
			break
		}
	}
	duplicateGroups := map[string][]string{}
	for _, id := range modelsList {
		if fingerprint := report.Models[id].OutputFingerprint; fingerprint != "" {
			duplicateGroups[fingerprint] = append(duplicateGroups[fingerprint], id)
		}
	}
	for fingerprint, ids := range duplicateGroups {
		if len(ids) < 2 {
			delete(duplicateGroups, fingerprint)
		}
	}
	if len(duplicateGroups) > 0 {
		report.DuplicateOutputGroups = duplicateGroups
	}
	if err := atomicWrite(out, report); err != nil {
		fmt.Fprintln(os.Stderr, "save:", err)
		os.Exit(1)
	}
	if raw {
		if err := writeRawArchive(rawOut, rawArchive); err != nil {
			fmt.Fprintf(os.Stderr, "raw archive failed after compact report was saved: %v\n", err)
			os.Exit(1)
		}
	}
	for _, id := range modelsList {
		m := report.Models[id]
		fmt.Printf("Model %s: status=%s complete=%d incomplete=%d\n", id, m.Status, m.Complete, m.Incomplete)
		if m.Metrics != nil {
			fmt.Printf("  SIA=%.4f unconditional=%.4f format=%.2f%% accuracy=%.2f%% consistency=%.2f sensitivity=%.4f distractor_sensitivity=%.4f distractor_pick_change=%.2f%%\n", m.Metrics.SIA, m.Metrics.UnconditionalSIA, m.Metrics.FormatAdherence*100, m.Metrics.TopPickAccuracy*100, m.Metrics.StrictConsistency, m.Metrics.MeanOrderSensitivityRange, m.Metrics.DistractorOrderSensitivity, m.Metrics.DistractorTopPickChangeRate*100)
		}
	}
	info, _ := os.Stat(out)
	if info != nil {
		fmt.Printf("Compact results: %s (%d bytes)\n", out, info.Size())
	} else {
		fmt.Println("Compact results:", out)
	}
	if raw {
		if rawInfo, statErr := os.Stat(rawOut); statErr == nil {
			fmt.Printf("Raw archive: %s (%d bytes)\n", rawOut, rawInfo.Size())
		}
	}
	if failed {
		os.Exit(1)
	}
}
