// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"encoding/json"
	"time"
)

const (
	// DashboardSchemaVersion identifies a fixed value used internally by this package.
	DashboardSchemaVersion = 3
	// DefaultDashboardSessionsDirectory identifies a fixed value used internally by this package.
	DefaultDashboardSessionsDirectory = sessionDirectory
	// DefaultDashboardEvalResultsDirectory identifies a fixed value used internally by this package.
	DefaultDashboardEvalResultsDirectory = "data/evals/results"
	// DefaultDashboardEvalConfigPath identifies a fixed value used internally by this package.
	DefaultDashboardEvalConfigPath = "inspect_evals.yaml"
	// DefaultDashboardModelCatalogPath is the latest local /v1/models snapshot.
	DefaultDashboardModelCatalogPath = "data/.local/v1_models.json"
	// DefaultDashboardIgnorePath identifies a fixed value used internally by this package.
	DefaultDashboardIgnorePath = ".dashboardignore"
	// DefaultDashboardMetricsPath identifies a fixed value used internally by this package.
	DefaultDashboardMetricsPath = "data/dashboard/session_metrics.json"
	// DefaultDashboardSIAMetricsPath identifies a fixed value used internally by this package.
	DefaultDashboardSIAMetricsPath = "data/dashboard/sia.json"
	// DefaultDashboardTemplatePath identifies a fixed value used internally by this package.
	DefaultDashboardTemplatePath = "internal/templates/dashboard.template.html"
	// DefaultDashboardHTMLPath identifies a fixed value used internally by this package.
	DefaultDashboardHTMLPath = "data/dashboard/dashboard.html"
)

// DashboardGenerateOptions is an exported type in this package's public data model.
type DashboardGenerateOptions struct {
	// SessionsDirectory stores the value represented by this member.
	SessionsDirectory string
	// EvalResultsDirectory stores the value represented by this member.
	EvalResultsDirectory string
	// IgnoreFilePath stores the value represented by this member.
	IgnoreFilePath string
}

// DashboardMetrics is an exported type in this package's public data model.
type DashboardMetrics struct {
	// SchemaVersion stores the value represented by this member.
	SchemaVersion int `json:"schema_version"`
	// GeneratedAt stores the value represented by this member.
	GeneratedAt time.Time `json:"generated_at"`
	// Source stores the value represented by this member.
	Source DashboardSource `json:"source"`
	// RuntimeConfigs stores the value represented by this member.
	RuntimeConfigs []DashboardRuntimeConfig `json:"runtime_configs,omitempty"`
	// GenerationConfigs deduplicates effective request settings across observations.
	GenerationConfigs []DashboardGenerationConfig `json:"generation_configs,omitempty"`
	// LoadEvents stores the value represented by this member.
	LoadEvents []DashboardLoadEvent `json:"load_events,omitempty"`
	// Models stores the value represented by this member.
	Models []DashboardModelData `json:"models"`
	// SystemOneModels contains models whose catalog output modality is decisions.
	SystemOneModels []DashboardModelData `json:"systemone_models,omitempty"`
	// ContextPressure stores deduplicated throughput/context pairs by model.
	ContextPressure map[string]DashboardContextPressure `json:"context_pressure"`
	// LatencyPressure stores compact latency relationships by model.
	LatencyPressure map[string]DashboardLatencyPressure `json:"latency_pressure"`
	// CachePressure stores compact cache relationships by model.
	CachePressure map[string]DashboardCachePressure `json:"cache_pressure"`
	// ReasoningPressure stores compact reasoning relationships by model.
	ReasoningPressure map[string]DashboardReasoningPressure `json:"reasoning_pressure"`
	// WorkloadPressure stores compact workload relationships by model.
	WorkloadPressure map[string]DashboardWorkloadPressure `json:"workload_pressure"`
	// Reliability stores completion outcome counts by model.
	Reliability map[string]DashboardReliability `json:"reliability"`
	// SIA stores the value represented by this member.
	SIA json.RawMessage `json:"sia"`
}

// DashboardContextPressure describes the two throughput/context pair series for one model.
type DashboardContextPressure struct {
	GenerationTPS []DashboardContextPressurePair `json:"generation_tps"`
	PromptTPS     []DashboardContextPressurePair `json:"prompt_tps"`
}

// DashboardContextPressurePair is one distinct throughput and context-token pair.
type DashboardContextPressurePair struct {
	TPS           float64 `json:"tps"`
	ContextTokens int     `json:"context_tokens"`
	Count         int     `json:"count"`
}

// DashboardGenerationConfig is one deduplicated request configuration.
type DashboardGenerationConfig struct {
	ID       string         `json:"id"`
	ModelID  string         `json:"model_id"`
	Settings map[string]any `json:"settings"`
}

// DashboardLatencyPoint describes one latency relationship.
type DashboardLatencyPoint struct {
	X     float64 `json:"x"`
	MS    float64 `json:"ms"`
	Count int     `json:"count"`
}

// DashboardLatencyPressure describes latency relationships for one model.
type DashboardLatencyPressure struct {
	PromptTokensToMS                []DashboardLatencyPoint `json:"prompt_tokens_to_ms,omitempty"`
	CompletionTokensToMS            []DashboardLatencyPoint `json:"completion_tokens_to_ms,omitempty"`
	ContextTokensToMS               []DashboardLatencyPoint `json:"context_tokens_to_ms,omitempty"`
	ContextTokensToTTFT             []DashboardLatencyPoint `json:"context_tokens_to_ttft,omitempty"`
	PromptTokensToTTFT              []DashboardLatencyPoint `json:"prompt_tokens_to_ttft,omitempty"`
	ContextTokensToPostFirstTokenMS []DashboardLatencyPoint `json:"context_tokens_to_post_first_token_ms,omitempty"`
}

// DashboardCachePoint describes one cache relationship.
type DashboardCachePoint struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Count int     `json:"count"`
}

// DashboardCachePressure describes cache relationships for one model.
type DashboardCachePressure struct {
	CachedTokensToPromptTPS []DashboardCachePoint `json:"cached_tokens_to_prompt_tps,omitempty"`
	CacheRatioToPromptMS    []DashboardCachePoint `json:"cache_ratio_to_prompt_ms,omitempty"`
}

// DashboardReasoningPoint describes one reasoning relationship.
type DashboardReasoningPoint struct {
	ReasoningTokens int     `json:"reasoning_tokens"`
	Y               float64 `json:"y"`
	Count           int     `json:"count"`
}

// DashboardReasoningPressure describes reasoning relationships for one model.
type DashboardReasoningPressure struct {
	ReasoningTokensToGenerationTPS []DashboardReasoningPoint `json:"reasoning_tokens_to_generation_tps,omitempty"`
	ReasoningTokensToDurationMS    []DashboardReasoningPoint `json:"reasoning_tokens_to_duration_ms,omitempty"`
}

// DashboardWorkloadPressure describes workload relationships for one model.
type DashboardWorkloadPressure struct {
	MessagesToDurationMS   []DashboardCachePoint `json:"messages_to_duration_ms,omitempty"`
	ToolCallsToDurationMS  []DashboardCachePoint `json:"tool_calls_to_duration_ms,omitempty"`
	AttachmentsToPromptTPS []DashboardCachePoint `json:"attachments_to_prompt_tps,omitempty"`
}

// DashboardReliability stores normalized outcome counts for one model.
type DashboardReliability struct {
	Total          int            `json:"total"`
	ByStatus       map[string]int `json:"by_status,omitempty"`
	ByFinishReason map[string]int `json:"by_finish_reason,omitempty"`
}

// DashboardRuntimeConfig preserves changes in configuration across observations.
// DashboardRuntimeConfig is an exported type in this package's public data model.
type DashboardRuntimeConfig struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// ModelID stores the value represented by this member.
	ModelID string `json:"model_id"`
	// Runtime stores the value represented by this member.
	Runtime DashboardRuntime `json:"runtime"`
}

// DashboardLoadEvent is one server lifecycle transition, not one inference.
// DashboardLoadEvent is an exported type in this package's public data model.
type DashboardLoadEvent struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// ModelID stores the value represented by this member.
	ModelID string `json:"model_id"`
	// SessionID stores the value represented by this member.
	SessionID string `json:"session_id"`
	// SnapshotIndex stores the value represented by this member.
	SnapshotIndex int `json:"snapshot_index"`
	// RuntimeConfigID stores the value represented by this member.
	RuntimeConfigID string `json:"runtime_config_id"`
	// AssociatedSnapshotAt stores the value represented by this member.
	AssociatedSnapshotAt time.Time `json:"associated_snapshot_at"`
	// DurationMS stores the value represented by this member.
	DurationMS float64 `json:"duration_ms"`
	// Source stores the value represented by this member.
	Source string `json:"source"`
	// Kind stores the value represented by this member.
	Kind string `json:"kind"`
}

// DashboardSource is an exported type in this package's public data model.
type DashboardSource struct {
	// Directory stores the value represented by this member.
	Directory string `json:"directory"`
	// SessionFiles stores the value represented by this member.
	SessionFiles int `json:"session_files"`
	// SessionsLoaded stores the value represented by this member.
	SessionsLoaded int `json:"sessions_loaded"`
	// UniqueSessionIDs stores the value represented by this member.
	UniqueSessionIDs int `json:"unique_session_ids"`
	// ObservedSessionIDs stores the value represented by this member.
	ObservedSessionIDs int `json:"observed_session_ids"`
	// SessionsWithoutObservations stores the value represented by this member.
	SessionsWithoutObservations int `json:"sessions_without_observations"`
	// SnapshotsSeen stores the value represented by this member.
	SnapshotsSeen int `json:"snapshots_seen"`
	// SnapshotsIncluded stores the value represented by this member.
	SnapshotsIncluded int `json:"snapshots_included"`
	// SnapshotsSkipped stores the value represented by this member.
	SnapshotsSkipped int `json:"snapshots_skipped"`
	// Models stores the value represented by this member.
	Models int `json:"models"`
	// EvalFilesSeen stores the value represented by this member.
	EvalFilesSeen int `json:"eval_files_seen,omitempty"`
	// EvalsIncluded stores the value represented by this member.
	EvalsIncluded int `json:"evals_included,omitempty"`
	// EvalsSkipped stores the value represented by this member.
	EvalsSkipped int `json:"evals_skipped,omitempty"`
}

// DashboardModelData is an exported type in this package's public data model.
type DashboardModelData struct {
	// ModelID stores the value represented by this member.
	ModelID string `json:"model_id"`
	// SessionCount stores the value represented by this member.
	SessionCount int `json:"session_count"`
	// SnapshotCount stores the value represented by this member.
	SnapshotCount int `json:"snapshot_count"`
	// FirstObservedAt stores the value represented by this member.
	FirstObservedAt *time.Time `json:"first_observed_at,omitempty"`
	// LastObservedAt stores the value represented by this member.
	LastObservedAt *time.Time `json:"last_observed_at,omitempty"`
	// Observations stores the value represented by this member.
	Observations []DashboardSnapshotObservation `json:"observations"`
	// Evals stores the value represented by this member.
	Evals []DashboardEvalData `json:"evals,omitempty"`
}

// DashboardEvalData is an exported type in this package's public data model.
type DashboardEvalData struct {
	// RunID stores the value represented by this member.
	RunID string `json:"run_id"`
	// Suite stores the value represented by this member.
	Suite DashboardEvalSuite `json:"suite"`
	// Engine stores the value represented by this member.
	Engine DashboardEvalEngine `json:"engine"`
	// Server stores the value represented by this member.
	Server DashboardEvalServer `json:"server"`
	// CompletedAt stores the value represented by this member.
	CompletedAt time.Time `json:"completed_at"`
	// DurationMS stores the value represented by this member.
	DurationMS int64 `json:"duration_ms"`
	// Status stores the value represented by this member.
	Status string `json:"status"`
	// Benchmarks stores the value represented by this member.
	Benchmarks []DashboardEvalBenchmark `json:"benchmarks"`
	// Aggregate stores the value represented by this member.
	Aggregate *DashboardEvalAggregate `json:"aggregate,omitempty"`
}

// DashboardEvalSuite is an exported type in this package's public data model.
type DashboardEvalSuite struct {
	// Name stores the value represented by this member.
	Name string `json:"name"`
	// ConfigHash stores the value represented by this member.
	ConfigHash string `json:"config_hash,omitempty"`
}

// DashboardEvalEngine is an exported type in this package's public data model.
type DashboardEvalEngine struct {
	// Name stores the value represented by this member.
	Name string `json:"name,omitempty"`
	// Version stores the value represented by this member.
	Version string `json:"version,omitempty"`
}

// DashboardEvalServer is an exported type in this package's public data model.
type DashboardEvalServer struct {
	// Type stores the value represented by this member.
	Type string `json:"type,omitempty"`
	// URL stores the value represented by this member.
	URL string `json:"url,omitempty"`
}

// DashboardEvalBenchmark is an exported type in this package's public data model.
type DashboardEvalBenchmark struct {
	// Name stores the value represented by this member.
	Name string `json:"name"`
	// Tag stores the value represented by this member.
	Tag string `json:"tag,omitempty"`
	// Task stores the value represented by this member.
	Task string `json:"task"`
	// Limit stores the value represented by this member.
	Limit int `json:"limit"`
	// MaxTokens stores the value represented by this member.
	MaxTokens int `json:"max_tokens,omitempty"`
	// Samples stores the value represented by this member.
	Samples int `json:"samples"`
	// Score stores the value represented by this member.
	Score float64 `json:"score"`
	// Metrics stores the value represented by this member.
	Metrics map[string]float64 `json:"metrics,omitempty"`
}

// DashboardEvalAggregate is an exported type in this package's public data model.
type DashboardEvalAggregate struct {
	// Score stores the value represented by this member.
	Score float64 `json:"score"`
	// Method stores the value represented by this member.
	Method string `json:"method"`
}

// DashboardSnapshotObservation is an exported type in this package's public data model.
type DashboardSnapshotObservation struct {
	// Session stores the value represented by this member.
	Session DashboardSessionProvenance `json:"session"`
	// CollectedAt stores the value represented by this member.
	CollectedAt time.Time `json:"collected_at"`
	// InputType stores the value represented by this member.
	InputType string `json:"input_type,omitempty"`
	// OutputType stores the value represented by this member.
	OutputType string `json:"output_type,omitempty"`
	// LoadTimeMS stores the value represented by this member.
	LoadTimeMS *float64 `json:"load_time_ms,omitempty"`
	// RuntimeConfigID stores the value represented by this member.
	RuntimeConfigID string `json:"runtime_config_id,omitempty"`
	// GenerationConfigID refers to the deduplicated request-settings catalog.
	GenerationConfigID string `json:"generation_config_id,omitempty"`
	// Conversation stores the value represented by this member.
	Conversation DashboardConversation `json:"conversation"`
	// Tools stores the value represented by this member.
	Tools DashboardToolUsage `json:"tools"`
	// Response stores the value represented by this member.
	Response DashboardResponse `json:"response"`
	// Tokens stores the value represented by this member.
	Tokens *DashboardTokens `json:"tokens,omitempty"`
	// ReasoningTokens describes model-reported reasoning token usage.
	ReasoningTokens *int `json:"reasoning_tokens,omitempty"`
	// Performance stores the value represented by this member.
	Performance *DashboardPerformance `json:"performance,omitempty"`
	// Context describes observed context occupancy during inference.
	Context *SnapshotContextTelemetry `json:"context,omitempty"`
	// Request stores sanitized generation configuration.
	Request map[string]any `json:"request,omitempty"`
	// Workload stores prompt-shape metadata without prompt content.
	Workload *SnapshotWorkloadTelemetry `json:"workload,omitempty"`
	// Timing stores client-side lifecycle measurements.
	Timing *SnapshotTimingTelemetry `json:"timing,omitempty"`
	// SlotSamples stores compact timestamped slot samples.
	SlotSamples []SnapshotSlotSample `json:"slot_samples,omitempty"`
	// Outcome stores normalized completion state.
	Outcome *SnapshotOutcomeTelemetry `json:"outcome,omitempty"`
	// Speculative stores the value represented by this member.
	Speculative *DashboardSpeculative `json:"speculative,omitempty"`
	// Runtime stores the value represented by this member.
	Runtime DashboardRuntime `json:"runtime"`
	// Metrics stores the value represented by this member.
	Metrics map[string]any `json:"metrics,omitempty"`
}

// MarshalJSON keeps schema-v1-style standalone observations readable while
// schema-v2 model observations refer to the runtime catalog once.
func (o DashboardSnapshotObservation) MarshalJSON() ([]byte, error) {
	type plain DashboardSnapshotObservation
	if o.RuntimeConfigID == "" {
		return json.Marshal(plain(o))
	}
	encoded, err := json.Marshal(plain(o))
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		return nil, err
	}
	delete(object, "runtime")
	return json.Marshal(object)
}

// DashboardToolUsage is an exported type in this package's public data model.
type DashboardToolUsage struct {
	// ApplicationToolsAvailable stores the value represented by this member.
	ApplicationToolsAvailable bool `json:"application_tools_available"`
	// MCPToolsAvailable stores the value represented by this member.
	MCPToolsAvailable bool `json:"mcp_tools_available"`
	// ApplicationToolsUsed stores the value represented by this member.
	ApplicationToolsUsed bool `json:"application_tools_used"`
	// MCPToolsUsed stores the value represented by this member.
	MCPToolsUsed bool `json:"mcp_tools_used"`
	// MCPToolNames stores the value represented by this member.
	MCPToolNames []string `json:"mcp_tool_names,omitempty"`
	// MCPToolsUsedArguments stores the value represented by this member.
	MCPToolsUsedArguments map[string][]string `json:"mcp_tools_used_arguments,omitempty"`
	// ApplicationToolUseOutcome stores the value represented by this member.
	ApplicationToolUseOutcome string `json:"application_tool_use_outcome"`
	// MCPToolUseOutcome stores the value represented by this member.
	MCPToolUseOutcome string `json:"mcp_tool_use_outcome"`
}

// DashboardSessionProvenance is an exported type in this package's public data model.
type DashboardSessionProvenance struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// Type stores the value represented by this member.
	Type string `json:"type"`
	// Title stores the value represented by this member.
	Title string `json:"title,omitempty"`
	// CreatedAt stores the value represented by this member.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt stores the value represented by this member.
	UpdatedAt time.Time `json:"updated_at"`
	// SnapshotIndex stores the value represented by this member.
	SnapshotIndex int `json:"snapshot_index"`
}

// DashboardConversation is an exported type in this package's public data model.
type DashboardConversation struct {
	// MessageCount stores the value represented by this member.
	MessageCount int `json:"message_count"`
	// UserMessages stores the value represented by this member.
	UserMessages int `json:"user_messages"`
	// AssistantMessages stores the value represented by this member.
	AssistantMessages int `json:"assistant_messages"`
	// SystemMessages stores the value represented by this member.
	SystemMessages int `json:"system_messages"`
	// ToolMessages stores the value represented by this member.
	ToolMessages int `json:"tool_messages"`
	// TurnNumber stores the value represented by this member.
	TurnNumber int `json:"turn_number"`
	// HasToolCalls stores the value represented by this member.
	HasToolCalls bool `json:"has_tool_calls"`
	// ToolCallCount stores the value represented by this member.
	ToolCallCount int `json:"tool_call_count"`
}

// DashboardResponse is an exported type in this package's public data model.
type DashboardResponse struct {
	// VisibleCharacters stores the value represented by this member.
	VisibleCharacters int `json:"visible_characters"`
	// VisibleWords stores the value represented by this member.
	VisibleWords int `json:"visible_words"`
	// ReasoningCharacters stores the value represented by this member.
	ReasoningCharacters int `json:"reasoning_characters"`
	// ReasoningWords stores the value represented by this member.
	ReasoningWords int `json:"reasoning_words"`
	// HasVisibleContent stores the value represented by this member.
	HasVisibleContent bool `json:"has_visible_content"`
	// HasReasoning stores the value represented by this member.
	HasReasoning bool `json:"has_reasoning"`
	// FinishReason stores the value represented by this member.
	FinishReason string `json:"finish_reason,omitempty"`
	// SystemFingerprint stores the value represented by this member.
	SystemFingerprint string `json:"system_fingerprint,omitempty"`
}

// DashboardTokens is an exported type in this package's public data model.
type DashboardTokens struct {
	// Prompt stores the value represented by this member.
	Prompt *int `json:"prompt,omitempty"`
	// Completion stores the value represented by this member.
	Completion *int `json:"completion,omitempty"`
	// Total stores the value represented by this member.
	Total *int `json:"total,omitempty"`
	// Cached stores the value represented by this member.
	Cached *int `json:"cached,omitempty"`
}

// DashboardPerformance is an exported type in this package's public data model.
type DashboardPerformance struct {
	// PromptMS stores the value represented by this member.
	PromptMS *float64 `json:"prompt_ms,omitempty"`
	// PromptTokensPerSecond stores the value represented by this member.
	PromptTokensPerSecond *float64 `json:"prompt_tokens_per_second,omitempty"`
	// GenerationMS stores the value represented by this member.
	GenerationMS *float64 `json:"generation_ms,omitempty"`
	// GenerationTokensPerSecond stores the value represented by this member.
	GenerationTokensPerSecond *float64 `json:"generation_tokens_per_second,omitempty"`
	// MillisecondsPerGeneratedToken stores the value represented by this member.
	MillisecondsPerGeneratedToken *float64 `json:"milliseconds_per_generated_token,omitempty"`
}

// DashboardSpeculative is an exported type in this package's public data model.
type DashboardSpeculative struct {
	// DraftTokens stores the value represented by this member.
	DraftTokens *int `json:"draft_tokens,omitempty"`
	// AcceptedDraftTokens stores the value represented by this member.
	AcceptedDraftTokens *int `json:"accepted_draft_tokens,omitempty"`
	// AcceptanceRate stores the value represented by this member.
	AcceptanceRate *float64 `json:"acceptance_rate,omitempty"`
}

// DashboardRuntime is an exported type in this package's public data model.
type DashboardRuntime struct {
	// TotalSlots stores the value represented by this member.
	TotalSlots *int `json:"total_slots,omitempty"`
	// ContextSize stores the value represented by this member.
	ContextSize *int `json:"context_size,omitempty"`
	// ModelAlias stores the value represented by this member.
	ModelAlias string `json:"model_alias,omitempty"`
	// BuildInfo stores the value represented by this member.
	BuildInfo string `json:"build_info,omitempty"`
	// Modalities stores the value represented by this member.
	Modalities []string `json:"modalities,omitempty"`
	// VisionCapable stores the value represented by this member.
	VisionCapable *bool `json:"vision_capable,omitempty"`
	// GenerationSettings stores the value represented by this member.
	GenerationSettings map[string]any `json:"generation_settings,omitempty"`
}

// dashboardSessionSource is an internal data type used by this package.
type dashboardSessionSource struct {
	// path stores the value represented by this member.
	path string
	// session stores the value represented by this member.
	session *ChatSession
}

// dashboardEvalSource is an internal data type used by this package.
type dashboardEvalSource struct {
	// path stores the value represented by this member.
	path string
	// result stores the value represented by this member.
	result *dashboardEvalResult
}

// dashboardEvalResult is an internal data type used by this package.
type dashboardEvalResult struct {
	// RunID stores the value represented by this member.
	RunID string `json:"run_id"`
	// Suite stores the value represented by this member.
	Suite dashboardEvalSuiteWire `json:"suite"`
	// Model stores the value represented by this member.
	Model dashboardEvalModelWire `json:"model"`
	// Engine stores the value represented by this member.
	Engine DashboardEvalEngine `json:"engine"`
	// Server stores the value represented by this member.
	Server DashboardEvalServer `json:"server"`
	// CompletedAt stores the value represented by this member.
	CompletedAt time.Time `json:"completed_at"`
	// DurationMS stores the value represented by this member.
	DurationMS int64 `json:"duration_ms"`
	// Status stores the value represented by this member.
	Status string `json:"status"`
	// Benchmarks stores the value represented by this member.
	Benchmarks []dashboardEvalBenchmarkWire `json:"benchmarks"`
	// Aggregate stores the value represented by this member.
	Aggregate *dashboardEvalAggregateWire `json:"aggregate"`
}

// dashboardEvalSuiteWire is an internal data type used by this package.
type dashboardEvalSuiteWire struct {
	// Name stores the value represented by this member.
	Name string `json:"name"`
	// ConfigHash stores the value represented by this member.
	ConfigHash string `json:"config_hash"`
}

// dashboardEvalModelWire is an internal data type used by this package.
type dashboardEvalModelWire struct {
	// Name stores the value represented by this member.
	Name string `json:"name"`
}

// dashboardEvalBenchmarkWire is an internal data type used by this package.
type dashboardEvalBenchmarkWire struct {
	// Name stores the value represented by this member.
	Name string `json:"name"`
	// Tag stores the value represented by this member.
	Tag string `json:"tag"`
	// Task stores the value represented by this member.
	Task string `json:"task"`
	// Limit stores the value represented by this member.
	Limit int `json:"limit"`
	// MaxTokens stores the value represented by this member.
	MaxTokens int `json:"max_tokens"`
	// Samples stores the value represented by this member.
	Samples int `json:"samples"`
	// Score stores the value represented by this member.
	Score float64 `json:"score"`
	// Metrics stores the value represented by this member.
	Metrics map[string]float64 `json:"metrics"`
}

// dashboardEvalTagConfig is an internal data type used by this package.
type dashboardEvalTagConfig struct {
	// Evals stores the value represented by this member.
	Evals []struct {
		Name string `yaml:"name"`
		Tag  string `yaml:"tag"`
	} `yaml:"evals"`
}

// dashboardEvalAggregateWire is an internal data type used by this package.
type dashboardEvalAggregateWire struct {
	// Score stores the value represented by this member.
	Score float64 `json:"score"`
	// Method stores the value represented by this member.
	Method string `json:"method"`
}

// dashboardModelBuilder is an internal data type used by this package.
type dashboardModelBuilder struct {
	// ModelID stores the value represented by this member.
	ModelID string
	// SessionIDs stores the value represented by this member.
	SessionIDs map[string]struct{}
	// Observations stores the value represented by this member.
	Observations []DashboardSnapshotObservation
	// Last stores the value represented by this member.
	// First stores the value represented by this member.
	First, Last *time.Time
}
