package induction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildDashboardMetricsGroupsSnapshotsBySnapshotModel(t *testing.T) {
	directory := t.TempDir()
	session := &ChatSession{Version: chatSessionVersion, ID: "00000000-0000-0000-0000-000000000001", Type: sessionTypeDirect, Model: "session-model", Snapshots: []*ModelSnapshot{
		{ModelID: "model-b", CollectedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
		{ModelID: "model-a", CollectedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ModelID: "model-a"}, nil,
	}}
	writeTestSession(t, directory, session)

	metrics, err := buildDashboardMetricsFromDirectories(directory, filepath.Join(directory, "eval-results"))
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Source.SessionsLoaded != 1 || metrics.Source.SnapshotsSeen != 4 || metrics.Source.SnapshotsIncluded != 3 || metrics.Source.SnapshotsSkipped != 1 {
		t.Fatalf("unexpected source counts: %+v", metrics.Source)
	}
	if len(metrics.Models) != 2 || metrics.Models[0].ModelID != "model-a" || metrics.Models[0].SnapshotCount != 2 || metrics.Models[0].SessionCount != 1 || metrics.Models[1].ModelID != "model-b" {
		t.Fatalf("unexpected model groups: %+v", metrics.Models)
	}
	if metrics.Models[0].Observations[0].Session.SnapshotIndex != 2 {
		t.Fatalf("observations were not sorted by collection time: %+v", metrics.Models[0].Observations)
	}
}

func TestDashboardProjectionPrunesRawTextAndExtractsTelemetry(t *testing.T) {
	session := &ChatSession{
		ID: "00000000-0000-0000-0000-000000000002", Type: sessionTypeDirect, Saved: true, Title: "title", Model: "model",
		Messages: []Message{{Role: "user", Content: "private message"}, {Role: "assistant", Content: "assistant"}},
		Snapshots: []*ModelSnapshot{{
			ModelID:           "model",
			MCPToolsAvailable: true, MCPToolNames: []string{"weather"}, MCPToolUseOutcome: "model_did_not_request",
			Messages:    []Message{{Role: "user", Content: "private message"}, {Role: "tool", Content: "tool secret"}},
			Interaction: []Interaction{{Content: "visible answer", ReasoningContent: "private reasoning", Response: `{"system_fingerprint":"fp","choices":[{"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14},"timings":{"prompt_ms":5,"predicted_n":4,"predicted_ms":20,"draft_n":8,"draft_n_accepted":6}}`}},
			Props:       &PropsData{Raw: `{"model_path":"model.gguf","context_size":4096,"secret":"full props secret"}`, DefaultGenerationSettings: map[string]interface{}{"temperature": 0.7}},
			Metrics:     &MetricsData{Raw: "private metrics", Entries: map[string]interface{}{"llamacpp:prompt_tokens_total": float64(10), "unknown": float64(1)}},
		}},
	}
	observation, ok := dashboardObservation(session, session.Snapshots[0], 0)
	if !ok {
		t.Fatal("expected observation")
	}
	encoded, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	output := string(encoded)
	for _, marker := range []string{"private message", "tool secret", "private reasoning", "full props secret", "private metrics"} {
		if strings.Contains(output, marker) {
			t.Fatalf("raw marker %q leaked: %s", marker, output)
		}
	}
	if observation.Response.VisibleCharacters != len([]rune("visible answer")) || observation.Response.ReasoningWords != 2 || observation.Conversation.ToolMessages != 1 {
		t.Fatalf("unexpected derived stats: %+v %+v", observation.Response, observation.Conversation)
	}
	if observation.Tokens == nil || observation.Tokens.Prompt == nil || *observation.Tokens.Prompt != 10 || observation.Tokens.Completion == nil || *observation.Tokens.Completion != 4 || observation.Speculative == nil || observation.Speculative.AcceptanceRate == nil || *observation.Speculative.AcceptanceRate != 0.75 {
		t.Fatalf("unexpected telemetry: %+v %+v", observation.Tokens, observation.Speculative)
	}
	if !observation.Tools.MCPToolsAvailable || observation.Tools.MCPToolsUsed || observation.Tools.MCPToolUseOutcome != "model_did_not_request" || len(observation.Tools.MCPToolNames) != 1 {
		t.Fatalf("unexpected tool telemetry: %+v", observation.Tools)
	}
}

func TestWriteDashboardMetricsCreatesIndentedAtomicJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "session_metrics.json")
	metrics := &DashboardMetrics{SchemaVersion: DashboardSchemaVersion, GeneratedAt: time.Unix(0, 0).UTC(), Source: DashboardSource{Directory: ".sessions"}, Models: []DashboardModelData{}}
	if err := WriteDashboardMetrics(path, metrics); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(contents), "\n") || !strings.Contains(string(contents), "\n  \"schema_version\"") {
		t.Fatalf("unexpected JSON formatting: %q", contents)
	}
	var decoded DashboardMetrics
	if err := json.Unmarshal(contents, &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestWriteDashboardHTMLEmbedsMetricsInTemplate(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "dashboard.template.html")
	outputPath := filepath.Join(t.TempDir(), "nested", "dashboard.html")
	if err := os.WriteFile(templatePath, []byte("<script>\nconst DASHBOARD_DATA = // insert data/dashboard/session_metrics.json;\n</script>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metrics := &DashboardMetrics{SchemaVersion: DashboardSchemaVersion, GeneratedAt: time.Unix(0, 0).UTC(), Source: DashboardSource{Directory: ".sessions"}, Models: []DashboardModelData{}}
	if err := WriteDashboardHTML(templatePath, outputPath, metrics); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	output := string(contents)
	if strings.Contains(output, "// insert data/dashboard/session_metrics.json") || !strings.Contains(output, "const DASHBOARD_DATA = {\n  \"schema_version\": 1") {
		t.Fatalf("dashboard data was not embedded: %q", output)
	}
}

func TestDashboardMetricNormalizationAndConservativeCounters(t *testing.T) {
	session := &ChatSession{ID: "00000000-0000-0000-0000-000000000003", Type: sessionTypeDirect, Model: "model", Snapshots: []*ModelSnapshot{{
		ModelID:     "model",
		Interaction: []Interaction{{Response: `{"timings":{"draft_n":12}}`}},
		Metrics: &MetricsData{Entries: map[string]interface{}{
			"llamacpp:predicted_tokens_seconds": float64(42.5),
			"llamacpp:prompt_tokens_seconds":    float64(1000),
			"llamacpp:prompt_tokens_total":      float64(999999),
		}},
	}}}
	observation, ok := dashboardObservation(session, session.Snapshots[0], 0)
	if !ok {
		t.Fatal("expected observation")
	}
	if observation.Performance == nil || observation.Performance.GenerationTokensPerSecond == nil || *observation.Performance.GenerationTokensPerSecond != 42.5 || observation.Performance.PromptTokensPerSecond == nil || *observation.Performance.PromptTokensPerSecond != 1000 {
		t.Fatalf("metric rates were not normalized: %+v", observation.Performance)
	}
	if observation.Tokens != nil && observation.Tokens.Prompt != nil {
		t.Fatalf("cumulative prompt counter was treated as request-scoped: %+v", observation.Tokens)
	}
	if observation.Speculative != nil && (observation.Speculative.DraftTokens != nil || observation.Speculative.AcceptedDraftTokens != nil || observation.Speculative.AcceptanceRate != nil) {
		t.Fatalf("partial speculative telemetry was emitted: %+v", observation.Speculative)
	}
}

func TestDashboardProjectionUsesSlotPromptTokensAsFallback(t *testing.T) {
	session := &ChatSession{ID: "00000000-0000-0000-0000-000000000005", Type: sessionTypeDirect, Model: "model", Snapshots: []*ModelSnapshot{{
		ModelID: "model",
		Slots: SlotsData{
			{"n_prompt_tokens": float64(5716), "n_prompt_tokens_cache": float64(0)},
			{"n_prompt_tokens": float64(1200), "n_prompt_tokens_cache": float64(50)},
		},
	}}}
	observation, ok := dashboardObservation(session, session.Snapshots[0], 0)
	if !ok || observation.Tokens == nil || observation.Tokens.Prompt == nil || *observation.Tokens.Prompt != 5716 || observation.Tokens.Cached == nil || *observation.Tokens.Cached != 0 {
		t.Fatalf("slot telemetry was not used as fallback: %+v", observation.Tokens)
	}
}

func TestDashboardProjectionPrefersResponsePromptTokensOverSlots(t *testing.T) {
	session := &ChatSession{ID: "00000000-0000-0000-0000-000000000006", Type: sessionTypeDirect, Model: "model", Snapshots: []*ModelSnapshot{{
		ModelID:     "model",
		Slots:       SlotsData{{"n_prompt_tokens": float64(5716)}},
		Interaction: []Interaction{{Response: `{"usage":{"prompt_tokens":10}}`}},
	}}}
	observation, ok := dashboardObservation(session, session.Snapshots[0], 0)
	if !ok || observation.Tokens == nil || observation.Tokens.Prompt == nil || *observation.Tokens.Prompt != 10 {
		t.Fatalf("response prompt telemetry was not preferred: %+v", observation.Tokens)
	}
}

func TestDashboardProjectionOmitsEmptyOptionalTelemetry(t *testing.T) {
	session := &ChatSession{ID: "00000000-0000-0000-0000-000000000004", Type: sessionTypeDirect, Model: "model", Snapshots: []*ModelSnapshot{{ModelID: "model"}}}
	observation, ok := dashboardObservation(session, session.Snapshots[0], 0)
	if !ok {
		t.Fatal("expected observation")
	}
	if observation.Tokens != nil || observation.Performance != nil || observation.Speculative != nil {
		t.Fatalf("empty optional telemetry was attached: %+v", observation)
	}
	encoded, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	output := string(encoded)
	for _, field := range []string{`"tokens"`, `"performance"`, `"speculative"`} {
		if strings.Contains(output, field) {
			t.Fatalf("empty telemetry field %s was emitted: %s", field, output)
		}
	}
}

func TestBuildDashboardMetricsMergesValidEvalsAndEvalOnlyModels(t *testing.T) {
	sessionsDirectory := t.TempDir()
	evalsDirectory := t.TempDir()
	model := "model/with:special"
	session := &ChatSession{Version: chatSessionVersion, ID: "00000000-0000-0000-0000-000000000010", Type: sessionTypeDirect, Model: model, Snapshots: []*ModelSnapshot{{ModelID: model}}}
	writeTestSession(t, sessionsDirectory, session)

	writeDashboardEvalResult(t, evalsDirectory, "model-result", &dashboardEvalResult{
		RunID: "run-2", Model: dashboardEvalModelWire{Name: model}, Suite: dashboardEvalSuiteWire{Name: "suite-b"}, Status: "success", CompletedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Benchmarks: []dashboardEvalBenchmarkWire{{Name: "task", Task: "inspect_evals/task", Samples: 5, Score: 0.8, Metrics: map[string]float64{"accuracy": 0.8}}}, Aggregate: &dashboardEvalAggregateWire{Score: 0.8, Method: "mean"},
	})
	writeDashboardEvalResult(t, evalsDirectory, "model-result-second", &dashboardEvalResult{
		RunID: "run-3", Model: dashboardEvalModelWire{Name: model}, Suite: dashboardEvalSuiteWire{Name: "suite-a"}, Status: "success", CompletedAt: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		Benchmarks: []dashboardEvalBenchmarkWire{{Name: "task", Task: "inspect_evals/task", Samples: 5, Score: 0.9}}, Aggregate: &dashboardEvalAggregateWire{Score: 0.9, Method: "mean"},
	})
	writeDashboardEvalResult(t, evalsDirectory, "eval-only", &dashboardEvalResult{
		RunID: "run-1", Model: dashboardEvalModelWire{Name: "eval-only-model"}, Suite: dashboardEvalSuiteWire{Name: "suite-a"}, Status: "success", CompletedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Benchmarks: []dashboardEvalBenchmarkWire{{Name: "task", Task: "inspect_evals/task", Samples: 5, Score: 0.6}}, Aggregate: &dashboardEvalAggregateWire{Score: 0.6, Method: "mean"},
	})

	metrics, err := buildDashboardMetricsFromDirectories(sessionsDirectory, evalsDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Source.EvalFilesSeen != 3 || metrics.Source.EvalsIncluded != 3 || metrics.Source.EvalsSkipped != 0 {
		t.Fatalf("unexpected eval source counts: %+v", metrics.Source)
	}
	if len(metrics.Models) != 2 || metrics.Models[0].ModelID != "eval-only-model" || metrics.Models[1].ModelID != model {
		t.Fatalf("unexpected model ordering: %+v", metrics.Models)
	}
	if len(metrics.Models[0].Observations) != 0 || metrics.Models[0].SessionCount != 0 || len(metrics.Models[0].Evals) != 1 {
		t.Fatalf("unexpected eval-only model: %+v", metrics.Models[0])
	}
	if len(metrics.Models[1].Evals) != 2 || metrics.Models[1].Evals[0].Suite.Name != "suite-a" || metrics.Models[1].Evals[1].Suite.Name != "suite-b" {
		t.Fatalf("eval was not merged into session model: %+v", metrics.Models[1])
	}
	encoded, err := json.Marshal(metrics)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "raw_log") {
		t.Fatalf("raw eval log leaked into dashboard metrics: %s", encoded)
	}
}

func TestBuildDashboardMetricsSkipsInvalidEvalsWithoutFailing(t *testing.T) {
	sessionsDirectory := t.TempDir()
	evalsDirectory := t.TempDir()
	writeDashboardEvalResult(t, evalsDirectory, "valid", &dashboardEvalResult{
		RunID: "valid", Model: dashboardEvalModelWire{Name: "valid-model"}, Suite: dashboardEvalSuiteWire{Name: "suite"}, Status: "success", CompletedAt: time.Now().UTC(),
		Benchmarks: []dashboardEvalBenchmarkWire{{Name: "task", Task: "inspect_evals/task", Samples: 1, Score: 0.5}}, Aggregate: &dashboardEvalAggregateWire{Score: 0.5, Method: "mean"},
	})
	writeDashboardEvalResult(t, evalsDirectory, "failed", &dashboardEvalResult{RunID: "failed", Model: dashboardEvalModelWire{Name: "failed-model"}, Suite: dashboardEvalSuiteWire{Name: "suite"}, Status: "failed"})
	writeDashboardEvalResult(t, evalsDirectory, "partial", &dashboardEvalResult{
		RunID: "partial", Model: dashboardEvalModelWire{Name: "partial-model"}, Suite: dashboardEvalSuiteWire{Name: "suite"}, Status: "failed", CompletedAt: time.Now().UTC(),
		Benchmarks: []dashboardEvalBenchmarkWire{{Name: "completed-task", Task: "inspect_evals/task", Limit: 5, Samples: 5, Score: 0.7}, {Name: "incomplete-task", Task: "inspect_evals/task", Limit: 5, Samples: 2, Score: 0.9}},
	})
	if err := os.WriteFile(filepath.Join(evalsDirectory, "malformed.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evalsDirectory, "nan.json"), []byte(`{"run_id":"nan","model":{"name":"nan-model"},"suite":{"name":"suite"},"status":"success","completed_at":"2026-01-01T00:00:00Z","benchmarks":[{"name":"task","task":"inspect_evals/task","samples":1,"score":1e999}],"aggregate":{"score":0.5,"method":"mean"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	metrics, err := buildDashboardMetricsFromDirectories(sessionsDirectory, evalsDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Source.EvalFilesSeen != 5 || metrics.Source.EvalsIncluded != 2 || metrics.Source.EvalsSkipped != 3 {
		t.Fatalf("unexpected invalid eval handling: %+v", metrics.Source)
	}
	if len(metrics.Models) != 2 || metrics.Models[0].ModelID != "partial-model" || len(metrics.Models[0].Evals[0].Benchmarks) != 1 || metrics.Models[0].Evals[0].Aggregate != nil {
		t.Fatalf("invalid evals were included: %+v", metrics.Models)
	}
}

func writeDashboardEvalResult(t *testing.T, directory, name string, result *dashboardEvalResult) {
	t.Helper()
	path := filepath.Join(directory, name+".json")
	contents, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestSession(t *testing.T, directory string, session *ChatSession) {
	t.Helper()
	contents, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, session.ID+".json"), contents, 0o600); err != nil {
		t.Fatal(err)
	}
}
