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
	session := &ChatSession{Version: chatSessionVersion, ID: "00000000-0000-0000-0000-000000000001", Type: sessionTypeUnsaved, Model: "session-model", Snapshots: []*ModelSnapshot{
		{ModelID: "model-b", CollectedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
		{ModelID: "model-a", CollectedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ModelID: "model-a"}, nil,
	}}
	writeTestSession(t, directory, session)

	metrics, err := BuildDashboardMetrics(directory)
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
		ID: "00000000-0000-0000-0000-000000000002", Type: sessionTypeSaved, Title: "title", Model: "model",
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
	session := &ChatSession{ID: "00000000-0000-0000-0000-000000000003", Type: sessionTypeUnsaved, Model: "model", Snapshots: []*ModelSnapshot{{
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

func TestDashboardProjectionOmitsEmptyOptionalTelemetry(t *testing.T) {
	session := &ChatSession{ID: "00000000-0000-0000-0000-000000000004", Type: sessionTypeUnsaved, Model: "model", Snapshots: []*ModelSnapshot{{ModelID: "model"}}}
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
