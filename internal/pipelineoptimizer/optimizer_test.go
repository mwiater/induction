package pipelineoptimizer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	induction "github.com/mwiater/induction"
)

const testYAML = "name: example\nprompt: Write a short answer\nsteps:\n  - name: draft\n    model: local-model\n    userPrompt: Write a short answer\n"

type mockAdvisor struct {
	mu        sync.Mutex
	calls     int
	responses []string
}

func (m *mockAdvisor) Infer(_ context.Context, _, _, _ string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls >= len(m.responses) {
		return "", context.DeadlineExceeded
	}
	r := m.responses[m.calls]
	m.calls++
	return r, nil
}

func setupOptimizer(t *testing.T) (Options, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "pipeline.example.yaml")
	if err := os.WriteFile(path, []byte(testYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return Options{PipelinePath: path, SourceRoot: sourceRoot, OutputDir: root, SessionsDir: filepath.Join(root, ".sessions"), MaxSessions: 10, Focus: "balanced"}, root
}

func generatedResponse(yamlText string) string {
	changes, _ := json.Marshal([]string{"Clarified draft instruction"})
	data, _ := json.Marshal(generation{YAML: yamlText, Changes: changes})
	return string(data)
}

func TestOptimizeWritesNumberedValidatedPairs(t *testing.T) {
	o, root := setupOptimizer(t)
	for _, want := range []string{"pipeline.example-alt-1.yaml", "pipeline.example-alt-2.yaml"} {
		mock := &mockAdvisor{responses: []string{`{"findings":[]}`, generatedResponse(testYAML)}}
		result, err := (&Service{Advisor: mock}).Optimize(context.Background(), o)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(result.YAMLPath) != want {
			t.Fatalf("got %s, want %s", result.YAMLPath, want)
		}
		if _, err := induction.LoadPipeline(result.YAMLPath); err != nil {
			t.Fatalf("written YAML is invalid: %v", err)
		}
		if _, err := os.Stat(result.ReportPath); err != nil {
			t.Fatal(err)
		}
		if result.SessionCount != 0 {
			t.Fatalf("unexpected sessions: %d", result.SessionCount)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "pipeline.example-alt-1.analysis.md")); err != nil {
		t.Fatal(err)
	}
	o.PipelinePath = filepath.Join(root, "pipeline.example-alt-2.yaml")
	mock := &mockAdvisor{responses: []string{`{"findings":[]}`, generatedResponse(testYAML)}}
	result, err := (&Service{Advisor: mock}).Optimize(context.Background(), o)
	if err != nil || filepath.Base(result.YAMLPath) != "pipeline.example-alt-3.yaml" {
		t.Fatalf("alt lineage: %v %#v", err, result)
	}
}

func TestOptimizeReportsStageProgress(t *testing.T) {
	o, _ := setupOptimizer(t)
	var updates []string
	o.Progress = func(message string) { updates = append(updates, message) }
	mock := &mockAdvisor{responses: []string{`{"findings":[]}`, generatedResponse(testYAML)}}
	if _, err := (&Service{Advisor: mock}).Optimize(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(updates, "\n")
	for _, want := range []string{"[optimizer 1/6]", "[optimizer 2/6]", "[optimizer 3/6]", "[optimizer 4/6]", "architecture analysis", "Generating candidate", "static validation passed", "Publishing alternative"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("progress missing %q: %s", want, joined)
		}
	}
}

func TestOptimizeRepairsAndRejectsInvalidCandidate(t *testing.T) {
	o, root := setupOptimizer(t)
	bad := strings.Replace(testYAML, "userPrompt:", "unknownKey:", 1)
	mock := &mockAdvisor{responses: []string{`{"findings":[]}`, generatedResponse(bad), generatedResponse(testYAML)}}
	if _, err := (&Service{Advisor: mock}).Optimize(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if mock.calls != 3 {
		t.Fatalf("expected repair call, got %d", mock.calls)
	}
	o.DryRun = true
	mock = &mockAdvisor{responses: []string{`{"findings":[]}`, generatedResponse(testYAML)}}
	if _, err := (&Service{Advisor: mock}).Optimize(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if mock.calls != 0 {
		t.Fatalf("dry-run contacted inference advisor: %d calls", mock.calls)
	}
	if _, err := os.Stat(filepath.Join(root, "pipeline.example-alt-2.yaml")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote YAML: %v", err)
	}
	o.DryRun = false
	bad = strings.Replace(testYAML, "local-model", "unknown-model", 1)
	mock = &mockAdvisor{responses: []string{`{"findings":[]}`, generatedResponse(bad), generatedResponse(bad), generatedResponse(bad)}}
	if _, err := (&Service{Advisor: mock}).Optimize(context.Background(), o); err == nil {
		t.Fatal("unsupported candidate accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "pipeline.example-alt-2.yaml")); !os.IsNotExist(err) {
		t.Fatalf("failed generation wrote YAML: %v", err)
	}
}

func TestGenerationAcceptsStructuredChangeEntries(t *testing.T) {
	var response generation
	if err := json.Unmarshal([]byte(`{"yaml":"name: example","changes":[{"step":"draft","reason":"clarified"}]}`), &response); err != nil {
		t.Fatal(err)
	}
	changes := response.changeList()
	if len(changes) != 1 || !strings.Contains(changes[0], "clarified") {
		t.Fatalf("structured change was not retained: %#v", changes)
	}
}

func TestFindingsAcceptStructuredObservedValues(t *testing.T) {
	var response struct {
		Findings []Finding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(`{"findings":[{"id":"F1","evidence":["P1"],"observed":["step failed","invalid JSON"]}]}`), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Findings) != 1 {
		t.Fatalf("got %d findings", len(response.Findings))
	}
	if response.Findings[0].Observed != `["step failed","invalid JSON"]` {
		t.Fatalf("unexpected observed value: %q", response.Findings[0].Observed)
	}
}

func TestCompletedSessionSelectionExcludesRunning(t *testing.T) {
	o, root := setupOptimizer(t)
	pipeline, err := induction.LoadPipeline(o.PipelinePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(o.SessionsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(id, status string) {
		t.Helper()
		now := time.Now().UTC()
		s := induction.ChatSession{Version: 1, ID: id, Type: "pipeline", Title: "example", Model: "local-model", Pipeline: &induction.SessionPipeline{Name: "example", Source: "file", Path: o.PipelinePath, SHA256: pipeline.FileSHA256}, PipelineStatus: status, UpdatedAt: now, Snapshots: []*induction.ModelSnapshot{{ModelID: "local-model"}}}
		if status == "completed" {
			s.CompletedAt = &now
		}
		data, _ := json.Marshal(s)
		if err := os.WriteFile(filepath.Join(o.SessionsDir, id+".json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("11111111-1111-4111-8111-111111111111", "running")
	write("22222222-2222-4222-8222-222222222222", "completed")
	o.AutoSessions = true
	selected, _, err := collectSessions(o, pipeline, pipeline.FileSHA256, o.PipelinePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 || selected[0].ID != "22222222-2222-4222-8222-222222222222" || selected[0].Match != "exact-hash" {
		t.Fatalf("incorrect selection: %#v", selected)
	}
	o.AutoSessions = false
	selected, _, err = collectSessions(o, pipeline, pipeline.FileSHA256, o.PipelinePath)
	if err != nil || len(selected) != 0 {
		t.Fatalf("sessions none selected evidence: %v %#v", err, selected)
	}
	o.SessionIDs = []string{"11111111-1111-4111-8111-111111111111"}
	if _, _, err := collectSessions(o, pipeline, pipeline.FileSHA256, o.PipelinePath); err == nil {
		t.Fatal("explicit in-flight session was accepted")
	}
	_ = root
}

func TestLegacySessionRequiresStableTerminalEvidence(t *testing.T) {
	o, _ := setupOptimizer(t)
	pipeline, err := induction.LoadPipeline(o.PipelinePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(o.SessionsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	id := "33333333-3333-4333-8333-333333333333"
	s := induction.ChatSession{Version: 1, ID: id, Type: "pipeline", Title: "example", Model: "local-model", UpdatedAt: time.Now().UTC(), FinalOutput: "Done", Snapshots: []*induction.ModelSnapshot{{ModelID: "local-model"}}}
	data, _ := json.Marshal(s)
	path := filepath.Join(o.SessionsDir, id+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := inspectSession(path, pipeline, pipeline.FileSHA256, o.PipelinePath, false); err != nil || ok {
		t.Fatalf("fresh legacy session accepted: %v %t", err, ok)
	}
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	e, ok, err := inspectSession(path, pipeline, pipeline.FileSHA256, o.PipelinePath, false)
	if err != nil || !ok || e.Match != "same-name-unverified" {
		t.Fatalf("stable legacy session rejected: %v %t %#v", err, ok, e)
	}
}

func TestFamilyAndConcurrentPublication(t *testing.T) {
	for input, want := range map[string]string{"pipeline.a.yml": "pipeline.a", "pipeline.a-alt-2.yaml": "pipeline.a", "bad.txt": ""} {
		got, _ := familyName(input)
		if got != want {
			t.Fatalf("%s: %s != %s", input, got, want)
		}
	}
	dir := t.TempDir()
	var wg sync.WaitGroup
	paths := make(chan string, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, _, err := publish(context.Background(), dir, "family", []byte(testYAML), func(string) []byte { return []byte("report") })
			if err != nil {
				paths <- "ERROR:" + err.Error()
			} else {
				paths <- p
			}
		}()
	}
	wg.Wait()
	close(paths)
	seen := map[string]bool{}
	for p := range paths {
		if strings.HasPrefix(p, "ERROR:") || seen[p] {
			t.Fatalf("publication collision: %s", p)
		}
		seen[p] = true
	}
	if len(seen) != 2 {
		t.Fatalf("got %d outputs", len(seen))
	}
}

func TestPublicationRefusesUnrecognizedReportAndRecoversOwnOrphan(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "family-alt-1.analysis.md")
	if err := os.WriteFile(reportPath, []byte("user report"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := publish(context.Background(), dir, "family", []byte(testYAML), func(string) []byte { return []byte("report") }); err == nil {
		t.Fatal("overwrote user report")
	}
	if err := os.WriteFile(reportPath, []byte("<!-- induction-optimizer-v1 -->\ninterrupted"), 0o600); err != nil {
		t.Fatal(err)
	}
	yamlPath, _, err := publish(context.Background(), dir, "family", []byte(testYAML), func(string) []byte { return []byte("<!-- induction-optimizer-v1 -->\ncomplete") })
	if err != nil || filepath.Base(yamlPath) != "family-alt-1.yaml" {
		t.Fatalf("orphan recovery: %v %s", err, yamlPath)
	}
}

func TestOptimizeMissingSourceFailsBeforeInference(t *testing.T) {
	o, _ := setupOptimizer(t)
	o.SourceRoot = t.TempDir()
	mock := &mockAdvisor{responses: []string{`{"findings":[]}`, generatedResponse(testYAML)}}
	if _, err := (&Service{Advisor: mock}).Optimize(context.Background(), o); err == nil {
		t.Fatal("missing source accepted")
	}
	if mock.calls != 0 {
		t.Fatalf("inference happened before source validation: %d calls", mock.calls)
	}
}

func TestOutputDirectorySymlinkRejected(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	if err := safeOutputDirectory(link); err == nil {
		t.Fatal("symlink output directory accepted")
	}
}

func TestDescribeChangesUsesValidatedPipelineFields(t *testing.T) {
	original := &induction.Pipeline{Steps: []induction.PipelineStep{{Name: "draft", Model: "m", UserPrompt: "old"}, {Name: "final", Model: "m", UserPrompt: "finish"}}}
	candidate := &induction.Pipeline{Steps: []induction.PipelineStep{{Name: "draft", Model: "m", UserPrompt: "new"}, {Name: "extra", Model: "m", UserPrompt: "check"}, {Name: "final", Model: "m", UserPrompt: "finish"}}}
	changes := strings.Join(describeChanges(original, candidate), "\n")
	if !strings.Contains(changes, "Modified step `draft`: prompts") || !strings.Contains(changes, "Added step `extra`") {
		t.Fatalf("missing actual changes: %s", changes)
	}
}

func TestCandidateStaticValidationRejectsUnknownsAndReferences(t *testing.T) {
	o, _ := setupOptimizer(t)
	original, err := induction.LoadPipeline(o.PipelinePath)
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{
		strings.Replace(testYAML, "userPrompt:", "inventedKey:", 1),
		strings.Replace(testYAML, "userPrompt: Write a short answer", "userPrompt: '{{steps.missing.output}}'", 1),
		strings.Replace(testYAML, "local-model", "unavailable-model", 1),
		strings.Replace(testYAML, "name: draft", "name: other", 1),
		strings.Replace(testYAML, "prompt: Write a short answer", "prompt: Different goal", 1),
	}
	for i, candidate := range cases {
		if _, err := validateCandidate([]byte(candidate), original, o.PipelinePath, o.OutputDir); err == nil {
			t.Fatalf("case %d accepted invalid candidate", i)
		}
	}
}

func TestBatchCandidateRejectsMixedMCPRouting(t *testing.T) {
	o, _ := setupOptimizer(t)
	original := "name: example\nbatch:\n  items:\n    - id: one\n      files: [input.txt]\nsteps:\n  - name: draft\n    model: local-model\n    userPrompt: Draft\n  - name: final\n    model: local-model\n    userPrompt: Final\n"
	path := filepath.Join(o.OutputDir, "batch.yaml")
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	pipeline, err := induction.LoadPipeline(path)
	if err != nil {
		t.Fatal(err)
	}
	candidate := strings.Replace(original, "    userPrompt: Draft", "    userPrompt: Draft\n    nomcp: true", 1)
	if _, err := validateCandidate([]byte(candidate), pipeline, path, o.OutputDir); err == nil || !strings.Contains(err.Error(), "mixes nomcp") {
		t.Fatalf("mixed batch routing accepted: %v", err)
	}
}

func TestGroundedFindingsDiscardInventedEvidence(t *testing.T) {
	source := sourceEvidence{Text: "C:pipeline.go:1 type Pipeline struct{}\n"}
	findings := []Finding{{ID: "F1", Observed: "real", Evidence: []string{"P1"}}, {ID: "F2", Observed: "invented", Evidence: []string{"S:missing"}}}
	got := groundedFindings(findings, source, nil)
	if len(got) != 1 || got[0].ID != "F1" {
		t.Fatalf("unsupported finding survived: %#v", got)
	}
}
