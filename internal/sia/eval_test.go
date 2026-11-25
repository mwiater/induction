package main

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func writeQuestions(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "questions.jsonl")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadQuestionsValidatesAndPreservesOrder(t *testing.T) {
	path := writeQuestions(t, "\ufeff# smoke\n\n"+string(mustJSON(t, map[string]any{"id": "b", "question": "q", "options": []string{"1", "2", "3", "4"}, "correct": 2, "difficulty": "hard"}))+"\n"+string(mustJSON(t, map[string]any{"id": "a", "question": "q", "options": []string{"w", "x", "y", "z"}, "correct": 0, "difficulty": "easy"}))+"\n")
	items, hash, err := loadQuestions(path, 0)
	if err != nil || len(items) != 2 || items[0].ID != "b" || items[1].ID != "a" || len(hash) != 64 {
		t.Fatalf("load: items=%#v hash=%q err=%v", items, hash, err)
	}
	if _, _, err := loadQuestions(writeQuestions(t, `{"id":"x","question":"q","options":["a","b","c","d"],"difficulty":"easy"}`), 0); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("missing correct should identify line: %v", err)
	}
	if _, _, err := loadQuestions(writeQuestions(t, `{"id":"x","question":"q","options":["a","a","c","d"],"correct":0,"difficulty":"easy"}`), 0); err == nil {
		t.Fatal("duplicate options should fail")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseModels(t *testing.T) {
	got, err := parseModels("", " A, B ")
	if err != nil || strings.Join(got, ",") != "A,B" {
		t.Fatalf("models=%v err=%v", got, err)
	}
	for _, pair := range [][2]string{{"A", "B"}, {"", "A,,B"}, {"", "A,A"}} {
		if _, err := parseModels(pair[0], pair[1]); err == nil {
			t.Fatalf("expected model-list error for %#v", pair)
		}
	}
}

func TestLoadAllModels(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"one"},{"id":"two"},{"id":"one"},{"id":""}]}`)), Header: make(http.Header)}, nil
	})}
	models, err := loadAllModels(context.Background(), "http://llama", client)
	if err != nil || strings.Join(models, ",") != "one,two" {
		t.Fatalf("models=%v err=%v", models, err)
	}
}

func TestBuildPlanRotationMapping(t *testing.T) {
	items := []Item{{ID: "x", Question: "question", Options: [4]string{"zero", "one", "two", "three"}, Correct: 2, Difficulty: "easy"}}
	plan, err := buildPlan(items, []string{"baseline"}, "qhash", Settings{NProbs: 100, Seed: 42})
	if err != nil || len(plan.Items[0].Rotations) != 8 {
		t.Fatalf("plan=%#v err=%v", plan, err)
	}
	for rot, r := range plan.Items[0].Rotations[:4] {
		if r.LetterToOption[(2+rot)%4] != 2 || !strings.Contains(r.Prompt, "Answer:") {
			t.Fatalf("rotation %d mapping=%v prompt=%q", rot, r.LetterToOption, r.Prompt)
		}
	}
	for rot, r := range plan.Items[0].Rotations[4:] {
		if r.PermutationSet != "distractor-reversed" || r.LetterToOption[(2+rot)%4] != 2 {
			t.Fatalf("distractor rotation %d mapping=%v", rot, r.LetterToOption)
		}
	}
}

func TestScoreCompletionAndMass(t *testing.T) {
	item := Item{ID: "x", Question: "q", Options: [4]string{"zero", "one", "two", "three"}, Correct: 0, Difficulty: "easy"}
	plan, err := buildPlan([]Item{item}, []string{"baseline"}, "qhash", Settings{NProbs: 4})
	if err != nil {
		t.Fatal(err)
	}
	probs := []Token{{ID: 1, Token: " A", Logprob: ptr(math.Log(.4))}, {ID: 2, Token: " B", Logprob: ptr(math.Log(.3))}, {ID: 3, Token: " C", Logprob: ptr(math.Log(.2))}, {ID: 4, Token: " D", Logprob: ptr(math.Log(.1))}}
	r := score(Completion{Content: " A", Probs: []ProbStep{{Top: probs}}}, plan.Items[0].Rotations[0], item, "baseline", 0, 12)
	if r.Error != "" || r.CorrectProbability == nil || math.Abs(*r.CorrectProbability-.4) > 1e-9 || math.Abs(r.CandidateMass-1) > 1e-9 {
		t.Fatalf("score=%#v", r)
	}
	if got := score(Completion{Probs: []ProbStep{{Top: probs[:3]}}}, plan.Items[0].Rotations[0], item, "baseline", 0, 0); !strings.Contains(got.Error, "candidate_coverage") {
		t.Fatalf("missing candidate error=%q", got.Error)
	}
}

func TestAggregateMetricsUseCompleteItemGroups(t *testing.T) {
	results := make([]Result, 0, 4)
	for rotation := 0; rotation < 4; rotation++ {
		correctLetter := rotation
		mapping := [4]int{0, 1, 2, 3}
		mapping[correctLetter], mapping[0] = mapping[0], mapping[correctLetter]
		format := true
		p := .7
		mass := [4]float64{.1, .1, .1, .1}
		mass[correctLetter] = .7
		results = append(results, Result{
			ItemID: "item", Rotation: rotation, PermutationSet: "cyclic", Control: "baseline", Repeat: 0,
			CorrectLetter: string(rune('A' + correctLetter)), LetterToOption: mapping,
			LetterMass: mass, Normalized: mass,
			CandidateMass: 1, CapturedMass: 1, CorrectProbability: &p,
			TopPickOption: ptrInt(0), FormatAdherent: &format,
		})
	}
	m := aggregate(results, 4)
	if m == nil || math.Abs(m.SIA-.7) > 1e-9 || math.Abs(m.UnconditionalSIA-.7) > 1e-9 || m.StrictConsistency != 1 || m.TopPickAccuracy != 1 || m.AlwaysCorrect != 1 || m.MeanOrderSensitivityRange != 0 || m.FormatAdherence != 1 {
		t.Fatalf("metrics=%#v", m)
	}
	if got := aggregate(results[:3], 4); got != nil {
		t.Fatalf("incomplete item group should have nil headline metrics: %#v", got)
	}
}

func ptr(v float64) *float64 { return &v }
func ptrInt(v int) *int      { return &v }

func TestAtomicWriteRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "report.json")
	if err := atomicWrite(path, map[string]any{"schema_version": 3}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "schema_version") {
		t.Fatalf("read atomic report: %v %s", err, b)
	}
	if err := atomicWrite(path, map[string]any{"schema_version": 4, "run": "second"}); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), `"schema_version": 4`) || strings.Contains(string(b), `"schema_version": 3`) {
		t.Fatalf("atomic replacement failed: %v %s", err, b)
	}
}

func TestLoadResumableReportRequiresACompatibleReport(t *testing.T) {
	if report, source, err := loadResumableReport([]string{filepath.Join(t.TempDir(), "missing.json")}, "questions", "plan", "http://server", Settings{}, 1); err == nil || report != nil || source != "" {
		t.Fatalf("missing resume report = report=%#v source=%q err=%v", report, source, err)
	}
	if modelRunComplete(nil, 1) {
		t.Fatal("nil model must not be considered complete")
	}
	if !modelRunComplete(&ModelReport{Status: "complete", Complete: 8, Metrics: &Metrics{}}, 8) {
		t.Fatal("complete model was not recognized")
	}
	if modelRunComplete(&ModelReport{Status: "complete", Complete: 7, Metrics: &Metrics{}}, 8) {
		t.Fatal("partial model was considered complete")
	}
}
