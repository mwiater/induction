package induction

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DashboardSchemaVersion            = 1
	DefaultDashboardSessionsDirectory = sessionDirectory
	DefaultDashboardMetricsPath       = "data/dashboard/session_metrics.json"
	DefaultDashboardTemplatePath      = "dashboard.template.html"
	DefaultDashboardHTMLPath          = "data/dashboard/dashboard.html"
)

type DashboardGenerateOptions struct{ SessionsDirectory string }

type DashboardMetrics struct {
	SchemaVersion int                  `json:"schema_version"`
	GeneratedAt   time.Time            `json:"generated_at"`
	Source        DashboardSource      `json:"source"`
	Models        []DashboardModelData `json:"models"`
}
type DashboardSource struct {
	Directory         string `json:"directory"`
	SessionFiles      int    `json:"session_files"`
	SessionsLoaded    int    `json:"sessions_loaded"`
	SnapshotsSeen     int    `json:"snapshots_seen"`
	SnapshotsIncluded int    `json:"snapshots_included"`
	SnapshotsSkipped  int    `json:"snapshots_skipped"`
	Models            int    `json:"models"`
}
type DashboardModelData struct {
	ModelID         string                         `json:"model_id"`
	SessionCount    int                            `json:"session_count"`
	SnapshotCount   int                            `json:"snapshot_count"`
	FirstObservedAt *time.Time                     `json:"first_observed_at,omitempty"`
	LastObservedAt  *time.Time                     `json:"last_observed_at,omitempty"`
	Observations    []DashboardSnapshotObservation `json:"observations"`
}
type DashboardSnapshotObservation struct {
	Session      DashboardSessionProvenance `json:"session"`
	CollectedAt  time.Time                  `json:"collected_at"`
	LoadTimeMS   *float64                   `json:"load_time_ms,omitempty"`
	Conversation DashboardConversation      `json:"conversation"`
	Tools        DashboardToolUsage         `json:"tools"`
	Response     DashboardResponse          `json:"response"`
	Tokens       *DashboardTokens           `json:"tokens,omitempty"`
	Performance  *DashboardPerformance      `json:"performance,omitempty"`
	Speculative  *DashboardSpeculative      `json:"speculative,omitempty"`
	Runtime      DashboardRuntime           `json:"runtime"`
	Metrics      map[string]any             `json:"metrics,omitempty"`
}
type DashboardToolUsage struct {
	ApplicationToolsAvailable bool     `json:"application_tools_available"`
	MCPToolsAvailable         bool     `json:"mcp_tools_available"`
	ApplicationToolsUsed      bool     `json:"application_tools_used"`
	MCPToolsUsed              bool     `json:"mcp_tools_used"`
	MCPToolNames              []string `json:"mcp_tool_names,omitempty"`
	ApplicationToolUseOutcome string   `json:"application_tool_use_outcome"`
	MCPToolUseOutcome         string   `json:"mcp_tool_use_outcome"`
}
type DashboardSessionProvenance struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	Title         string    `json:"title,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	SnapshotIndex int       `json:"snapshot_index"`
}
type DashboardConversation struct {
	MessageCount      int  `json:"message_count"`
	UserMessages      int  `json:"user_messages"`
	AssistantMessages int  `json:"assistant_messages"`
	SystemMessages    int  `json:"system_messages"`
	ToolMessages      int  `json:"tool_messages"`
	TurnNumber        int  `json:"turn_number"`
	HasToolCalls      bool `json:"has_tool_calls"`
	ToolCallCount     int  `json:"tool_call_count"`
}
type DashboardResponse struct {
	VisibleCharacters   int    `json:"visible_characters"`
	VisibleWords        int    `json:"visible_words"`
	ReasoningCharacters int    `json:"reasoning_characters"`
	ReasoningWords      int    `json:"reasoning_words"`
	HasVisibleContent   bool   `json:"has_visible_content"`
	HasReasoning        bool   `json:"has_reasoning"`
	FinishReason        string `json:"finish_reason,omitempty"`
	SystemFingerprint   string `json:"system_fingerprint,omitempty"`
}
type DashboardTokens struct {
	Prompt     *int `json:"prompt,omitempty"`
	Completion *int `json:"completion,omitempty"`
	Total      *int `json:"total,omitempty"`
	Cached     *int `json:"cached,omitempty"`
}
type DashboardPerformance struct {
	PromptMS                      *float64 `json:"prompt_ms,omitempty"`
	PromptTokensPerSecond         *float64 `json:"prompt_tokens_per_second,omitempty"`
	GenerationMS                  *float64 `json:"generation_ms,omitempty"`
	GenerationTokensPerSecond     *float64 `json:"generation_tokens_per_second,omitempty"`
	MillisecondsPerGeneratedToken *float64 `json:"milliseconds_per_generated_token,omitempty"`
}
type DashboardSpeculative struct {
	DraftTokens         *int     `json:"draft_tokens,omitempty"`
	AcceptedDraftTokens *int     `json:"accepted_draft_tokens,omitempty"`
	AcceptanceRate      *float64 `json:"acceptance_rate,omitempty"`
}
type DashboardRuntime struct {
	TotalSlots         *int           `json:"total_slots,omitempty"`
	ContextSize        *int           `json:"context_size,omitempty"`
	ModelPath          string         `json:"model_path,omitempty"`
	ModelAlias         string         `json:"model_alias,omitempty"`
	BuildInfo          string         `json:"build_info,omitempty"`
	ChatTemplate       string         `json:"chat_template,omitempty"`
	Modalities         []string       `json:"modalities,omitempty"`
	GenerationSettings map[string]any `json:"generation_settings,omitempty"`
}

type dashboardSessionSource struct {
	path    string
	session *ChatSession
}
type dashboardModelBuilder struct {
	ModelID      string
	SessionIDs   map[string]struct{}
	Observations []DashboardSnapshotObservation
	First, Last  *time.Time
}

// BuildDashboardMetrics builds the rebuildable dashboard projection without contacting a server.
func BuildDashboardMetrics(sessionsDirectory string) (*DashboardMetrics, error) {
	if sessionsDirectory == "" {
		sessionsDirectory = DefaultDashboardSessionsDirectory
	}
	sources, err := scanDashboardSessions(sessionsDirectory)
	if err != nil {
		return nil, err
	}
	source := DashboardSource{Directory: sessionsDirectory, SessionFiles: len(sources)}
	for _, item := range sources {
		source.SessionsLoaded++
		source.SnapshotsSeen += len(item.session.Snapshots)
	}
	result, err := buildDashboardMetrics(sources, source)
	if err != nil {
		return nil, err
	}
	result.GeneratedAt = time.Now().UTC()
	return result, nil
}

func scanDashboardSessions(directory string) ([]dashboardSessionSource, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return []dashboardSessionSource{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("generate dashboard: list sessions directory %q: %w", directory, err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Strings(paths)
	result := make([]dashboardSessionSource, 0, len(paths))
	seen := make(map[string]string)
	for _, path := range paths {
		session, loadErr := loadChatSessionFromPath(path)
		if loadErr != nil {
			return nil, fmt.Errorf("generate dashboard: load session %q: %w", path, loadErr)
		}
		if previous, exists := seen[session.ID]; exists {
			return nil, fmt.Errorf("generate dashboard: duplicate session ID %q in %q and %q", session.ID, previous, path)
		}
		seen[session.ID] = path
		result = append(result, dashboardSessionSource{path: path, session: session})
	}
	return result, nil
}

func buildDashboardMetrics(sources []dashboardSessionSource, source DashboardSource) (*DashboardMetrics, error) {
	groups := make(map[string]*dashboardModelBuilder)
	for _, item := range sources {
		for index, snapshot := range item.session.Snapshots {
			if snapshot == nil || strings.TrimSpace(snapshot.ModelID) == "" {
				source.SnapshotsSkipped++
				continue
			}
			observation, ok := dashboardObservation(item.session, snapshot, index)
			if !ok {
				source.SnapshotsSkipped++
				continue
			}
			group := groups[snapshot.ModelID]
			if group == nil {
				group = &dashboardModelBuilder{ModelID: snapshot.ModelID, SessionIDs: make(map[string]struct{})}
				groups[snapshot.ModelID] = group
			}
			group.SessionIDs[item.session.ID] = struct{}{}
			group.Observations = append(group.Observations, observation)
			if !snapshot.CollectedAt.IsZero() {
				t := snapshot.CollectedAt
				if group.First == nil || t.Before(*group.First) {
					group.First = &t
				}
				if group.Last == nil || t.After(*group.Last) {
					group.Last = &t
				}
			}
		}
	}
	models := finalizeDashboardModels(groups)
	source.SnapshotsIncluded = source.SnapshotsSeen - source.SnapshotsSkipped
	source.Models = len(models)
	return &DashboardMetrics{SchemaVersion: DashboardSchemaVersion, Source: source, Models: models}, nil
}

func dashboardObservation(session *ChatSession, snapshot *ModelSnapshot, index int) (DashboardSnapshotObservation, bool) {
	if snapshot == nil || strings.TrimSpace(snapshot.ModelID) == "" {
		return DashboardSnapshotObservation{}, false
	}
	conversation := dashboardConversationStats(snapshot.Messages)
	response, telemetry := dashboardResponseStats(snapshot.Interaction)
	applyDashboardMetricTelemetry(snapshot.Metrics, &telemetry)
	finalizeDashboardSpeculative(&telemetry.speculative)
	observation := DashboardSnapshotObservation{Session: DashboardSessionProvenance{ID: session.ID, Type: session.Type, Title: session.Title, CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt, SnapshotIndex: index}, CollectedAt: snapshot.CollectedAt, Conversation: conversation, Tools: DashboardToolUsage{ApplicationToolsAvailable: snapshot.ApplicationToolsAvailable, MCPToolsAvailable: snapshot.MCPToolsAvailable, ApplicationToolsUsed: snapshot.ApplicationToolsUsed, MCPToolsUsed: snapshot.MCPToolsUsed, MCPToolNames: append([]string(nil), snapshot.MCPToolNames...), ApplicationToolUseOutcome: snapshot.ApplicationToolUseOutcome, MCPToolUseOutcome: snapshot.MCPToolUseOutcome}, Response: response, Tokens: dashboardTokensIfPopulated(telemetry.tokens), Performance: dashboardPerformanceIfPopulated(telemetry.performance), Speculative: dashboardSpeculativeIfPopulated(telemetry.speculative), Runtime: extractDashboardRuntime(snapshot.Props), Metrics: extractDashboardMetrics(snapshot.Metrics)}
	if snapshot.ModelLoadTime != 0 {
		value := float64(snapshot.ModelLoadTime) / float64(time.Millisecond)
		observation.LoadTimeMS = &value
	}
	return observation, true
}

func dashboardTokensIfPopulated(value DashboardTokens) *DashboardTokens {
	if value.Prompt == nil && value.Completion == nil && value.Total == nil && value.Cached == nil {
		return nil
	}
	return &value
}

func dashboardPerformanceIfPopulated(value DashboardPerformance) *DashboardPerformance {
	if value.PromptMS == nil && value.PromptTokensPerSecond == nil && value.GenerationMS == nil && value.GenerationTokensPerSecond == nil && value.MillisecondsPerGeneratedToken == nil {
		return nil
	}
	return &value
}

func dashboardSpeculativeIfPopulated(value DashboardSpeculative) *DashboardSpeculative {
	if value.DraftTokens == nil && value.AcceptedDraftTokens == nil && value.AcceptanceRate == nil {
		return nil
	}
	return &value
}

func dashboardConversationStats(messages []Message) DashboardConversation {
	result := DashboardConversation{MessageCount: len(messages)}
	for _, message := range messages {
		switch strings.ToLower(message.Role) {
		case "user":
			result.UserMessages++
		case "assistant":
			result.AssistantMessages++
		case "system":
			result.SystemMessages++
		case "tool":
			result.ToolMessages++
		}
		if len(message.ToolCalls) > 0 {
			result.HasToolCalls = true
			result.ToolCallCount += len(message.ToolCalls)
		}
	}
	result.TurnNumber = result.UserMessages
	return result
}

type dashboardTelemetry struct {
	tokens      DashboardTokens
	performance DashboardPerformance
	speculative DashboardSpeculative
}

func dashboardResponseStats(interactions []Interaction) (DashboardResponse, dashboardTelemetry) {
	var result DashboardResponse
	telemetry := dashboardTelemetry{}
	for _, interaction := range interactions {
		result.VisibleCharacters += utf8.RuneCountInString(interaction.Content)
		result.VisibleWords += len(strings.Fields(interaction.Content))
		result.ReasoningCharacters += utf8.RuneCountInString(interaction.ReasoningContent)
		result.ReasoningWords += len(strings.Fields(interaction.ReasoningContent))
		result.HasVisibleContent = result.HasVisibleContent || strings.TrimSpace(interaction.Content) != ""
		result.HasReasoning = result.HasReasoning || strings.TrimSpace(interaction.ReasoningContent) != ""
		var raw any
		if json.Unmarshal([]byte(interaction.Response), &raw) != nil {
			continue
		}
		payloads := []map[string]any{}
		switch value := raw.(type) {
		case map[string]any:
			payloads = append(payloads, value)
		case []any:
			for _, item := range value {
				if object, ok := item.(map[string]any); ok {
					payloads = append(payloads, object)
				}
			}
		}
		for _, payload := range payloads {
			applyDashboardTelemetry(payload, &result, &telemetry)
		}
	}
	return result, telemetry
}

func applyDashboardTelemetry(payload map[string]any, response *DashboardResponse, telemetry *dashboardTelemetry) {
	if value, ok := payload["system_fingerprint"].(string); ok && value != "" {
		response.SystemFingerprint = value
	}
	if choices, ok := payload["choices"].([]any); ok {
		for _, raw := range choices {
			if choice, ok := raw.(map[string]any); ok {
				if reason, ok := choice["finish_reason"].(string); ok && reason != "" && response.FinishReason == "" {
					response.FinishReason = reason
				}
			}
		}
	}
	if usage, ok := payload["usage"].(map[string]any); ok {
		setIntFromMap(usage, "prompt_tokens", &telemetry.tokens.Prompt)
		setIntFromMap(usage, "completion_tokens", &telemetry.tokens.Completion)
		setIntFromMap(usage, "total_tokens", &telemetry.tokens.Total)
	}
	if timings, ok := payload["timings"].(map[string]any); ok {
		applyDashboardTiming(timings, telemetry)
	}
	applyDashboardTiming(payload, telemetry)
}

func applyDashboardTiming(values map[string]any, telemetry *dashboardTelemetry) {
	setIntFromMap(values, "prompt_n", &telemetry.tokens.Prompt)
	setIntFromMap(values, "predicted_n", &telemetry.tokens.Completion)
	setIntFromMap(values, "cache_n", &telemetry.tokens.Cached)
	setIntFromMap(values, "draft_n", &telemetry.speculative.DraftTokens)
	setIntFromMap(values, "draft_n_accepted", &telemetry.speculative.AcceptedDraftTokens)
	setFloatFromMap(values, "prompt_ms", &telemetry.performance.PromptMS)
	setFloatFromMap(values, "prompt_per_second", &telemetry.performance.PromptTokensPerSecond)
	setFloatFromMap(values, "predicted_ms", &telemetry.performance.GenerationMS)
	setFloatFromMap(values, "predicted_per_second", &telemetry.performance.GenerationTokensPerSecond)
	if telemetry.performance.GenerationMS == nil {
		setFloatFromMap(values, "generation_ms", &telemetry.performance.GenerationMS)
	}
	if telemetry.performance.GenerationTokensPerSecond == nil {
		setFloatFromMap(values, "generation_tokens_per_second", &telemetry.performance.GenerationTokensPerSecond)
	}
	if telemetry.performance.GenerationMS != nil && telemetry.tokens.Completion != nil && *telemetry.tokens.Completion > 0 {
		value := *telemetry.performance.GenerationMS / float64(*telemetry.tokens.Completion)
		telemetry.performance.MillisecondsPerGeneratedToken = &value
	} else if telemetry.performance.GenerationTokensPerSecond != nil && *telemetry.performance.GenerationTokensPerSecond > 0 {
		value := 1000 / *telemetry.performance.GenerationTokensPerSecond
		telemetry.performance.MillisecondsPerGeneratedToken = &value
	}
}

// applyDashboardMetricTelemetry only promotes metrics that are direct rates.
// The token-total metric is a Prometheus counter and is intentionally not used
// as a per-request token count.
func applyDashboardMetricTelemetry(metrics *MetricsData, telemetry *dashboardTelemetry) {
	if metrics == nil {
		return
	}
	setMetricFloat(metrics.Entries, "llamacpp:prompt_tokens_seconds", &telemetry.performance.PromptTokensPerSecond)
	setMetricFloat(metrics.Entries, "llamacpp:predicted_tokens_seconds", &telemetry.performance.GenerationTokensPerSecond)
}

func setMetricFloat(values map[string]interface{}, key string, target **float64) {
	if *target != nil {
		return
	}
	if value, ok := dashboardNumber(values[key]); ok && !math.IsNaN(value) && !math.IsInf(value, 0) {
		*target = &value
	}
}

func finalizeDashboardSpeculative(speculative *DashboardSpeculative) {
	// Partial telemetry is not useful: absent accepted-token data must not look
	// like a zero acceptance rate (and vice versa).
	if speculative.DraftTokens == nil || speculative.AcceptedDraftTokens == nil {
		*speculative = DashboardSpeculative{}
		return
	}
	if *speculative.DraftTokens > 0 {
		rate := float64(*speculative.AcceptedDraftTokens) / float64(*speculative.DraftTokens)
		speculative.AcceptanceRate = &rate
	}
}

func setIntFromMap(values map[string]any, key string, target **int) {
	if value, ok := dashboardNumber(values[key]); ok {
		converted := int(value)
		if converted >= 0 {
			*target = &converted
		}
	}
}
func setFloatFromMap(values map[string]any, key string, target **float64) {
	if value, ok := dashboardNumber(values[key]); ok && !math.IsNaN(value) && !math.IsInf(value, 0) {
		*target = &value
	}
}
func dashboardNumber(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case int:
		return float64(value), true
	case json.Number:
		n, err := value.Float64()
		return n, err == nil
	}
	return 0, false
}

func extractDashboardRuntime(props *PropsData) DashboardRuntime {
	var result DashboardRuntime
	if props == nil {
		return result
	}
	if props.TotalSlots != 0 {
		result.TotalSlots = &props.TotalSlots
	}
	result.GenerationSettings = dashboardSafeSettings(props.DefaultGenerationSettings)
	if props.Raw == "" {
		return result
	}
	var raw map[string]any
	if json.Unmarshal([]byte(props.Raw), &raw) != nil {
		return result
	}
	for _, key := range []string{"context_size", "n_ctx", "ctx_size"} {
		if value, ok := findNumber(raw, key); ok {
			converted := int(value)
			result.ContextSize = &converted
			break
		}
	}
	for key, target := range map[string]*string{"model_path": &result.ModelPath, "model_alias": &result.ModelAlias, "build_info": &result.BuildInfo, "chat_template": &result.ChatTemplate} {
		if value, ok := findString(raw, key); ok {
			*target = value
		}
	}
	if value, ok := findStrings(raw, "modalities"); ok {
		result.Modalities = value
	}
	return result
}
func dashboardSafeSettings(settings map[string]interface{}) map[string]any {
	if len(settings) == 0 {
		return nil
	}
	result := make(map[string]any)
	for key, value := range settings {
		switch value.(type) {
		case nil, string, bool, float64, float32, int, int64, uint, uint64, []any, []string:
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
func findNumber(value any, key string) (float64, bool) {
	if object, ok := value.(map[string]any); ok {
		if result, ok := dashboardNumber(object[key]); ok {
			return result, true
		}
		for _, child := range object {
			if result, ok := findNumber(child, key); ok {
				return result, true
			}
		}
	}
	return 0, false
}
func findString(value any, key string) (string, bool) {
	if object, ok := value.(map[string]any); ok {
		if result, ok := object[key].(string); ok && result != "" {
			return result, true
		}
		for _, child := range object {
			if result, ok := findString(child, key); ok {
				return result, true
			}
		}
	}
	return "", false
}
func findStrings(value any, key string) ([]string, bool) {
	if object, ok := value.(map[string]any); ok {
		if values, ok := object[key].([]any); ok {
			result := make([]string, 0, len(values))
			for _, item := range values {
				if stringValue, ok := item.(string); ok {
					result = append(result, stringValue)
				}
			}
			return result, len(result) > 0
		}
		for _, child := range object {
			if result, ok := findStrings(child, key); ok {
				return result, true
			}
		}
	}
	return nil, false
}

var dashboardMetricAllowlist = map[string]struct{}{"llamacpp:prompt_tokens_total": {}, "llamacpp:predicted_tokens_total": {}, "llamacpp:prompt_tokens_seconds": {}, "llamacpp:predicted_tokens_seconds": {}, "llamacpp:kv_cache_usage": {}, "llamacpp:kv_cache_capacity": {}, "llamacpp:context_shift": {}, "llamacpp:requests_processing": {}, "llamacpp:requests_deferred": {}, "llamacpp:tokens_cached": {}, "llamacpp:draft_tokens": {}, "llamacpp:accepted_draft_tokens": {}}

func extractDashboardMetrics(metrics *MetricsData) map[string]any {
	if metrics == nil || len(metrics.Entries) == 0 {
		return nil
	}
	result := make(map[string]any)
	for key, value := range metrics.Entries {
		if _, ok := dashboardMetricAllowlist[key]; ok {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func finalizeDashboardModels(groups map[string]*dashboardModelBuilder) []DashboardModelData {
	models := make([]DashboardModelData, 0, len(groups))
	for _, group := range groups {
		sort.SliceStable(group.Observations, func(i, j int) bool {
			left, right := group.Observations[i], group.Observations[j]
			if !left.CollectedAt.Equal(right.CollectedAt) {
				return left.CollectedAt.Before(right.CollectedAt)
			}
			if left.Session.ID != right.Session.ID {
				return left.Session.ID < right.Session.ID
			}
			return left.Session.SnapshotIndex < right.Session.SnapshotIndex
		})
		models = append(models, DashboardModelData{ModelID: group.ModelID, SessionCount: len(group.SessionIDs), SnapshotCount: len(group.Observations), FirstObservedAt: group.First, LastObservedAt: group.Last, Observations: group.Observations})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ModelID < models[j].ModelID })
	return models
}

// WriteDashboardMetrics atomically writes an indented dashboard artifact.
func WriteDashboardMetrics(path string, metrics *DashboardMetrics) error {
	if metrics == nil {
		return errors.New("dashboard metrics are nil")
	}
	contents, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dashboard metrics: %w", err)
	}
	contents = append(contents, '\n')
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create dashboard directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".session_metrics-*.tmp")
	if err != nil {
		return fmt.Errorf("create dashboard temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set dashboard permissions: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write dashboard metrics: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync dashboard metrics: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close dashboard metrics: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace dashboard metrics: %w", err)
	}
	return nil
}

const dashboardDataPlaceholder = "const DASHBOARD_DATA = // insert data/dashboard/session_metrics.json;"

// WriteDashboardHTML embeds the dashboard metrics in the HTML template and
// atomically writes the resulting self-contained dashboard artifact.
func WriteDashboardHTML(templatePath, path string, metrics *DashboardMetrics) error {
	if metrics == nil {
		return errors.New("dashboard metrics are nil")
	}
	contents, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("read dashboard template: %w", err)
	}
	if bytes.Count(contents, []byte(dashboardDataPlaceholder)) != 1 {
		return fmt.Errorf("dashboard template must contain exactly one %q placeholder", dashboardDataPlaceholder)
	}
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dashboard data: %w", err)
	}
	replacement := append([]byte("const DASHBOARD_DATA = "), data...)
	replacement = append(replacement, ';')
	contents = bytes.Replace(contents, []byte(dashboardDataPlaceholder), replacement, 1)

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create dashboard directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".dashboard-*.tmp")
	if err != nil {
		return fmt.Errorf("create dashboard temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set dashboard permissions: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write dashboard HTML: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync dashboard HTML: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close dashboard HTML: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace dashboard HTML: %w", err)
	}
	return nil
}

// GenerateDashboardMetrics builds and writes the default dashboard artifacts.
func GenerateDashboardMetrics(options DashboardGenerateOptions) (*DashboardMetrics, error) {
	metrics, err := BuildDashboardMetrics(options.SessionsDirectory)
	if err != nil {
		return nil, err
	}
	if err := WriteDashboardMetrics(DefaultDashboardMetricsPath, metrics); err != nil {
		return nil, err
	}
	if err := WriteDashboardHTML(DefaultDashboardTemplatePath, DefaultDashboardHTMLPath, metrics); err != nil {
		return nil, err
	}
	return metrics, nil
}
