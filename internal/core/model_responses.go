// Package induction contains the core implementation for the public Induction API.
package induction

import "time"

// ModelSnapshot is an exported type in this package's public data model.
type ModelSnapshot struct {
	// ModelID identifies the model used for the snapshot.
	ModelID string
	// InputType classifies the request input as text, vision, audio, or document.
	InputType string `json:"inputType"`
	// OutputType classifies the requested output constraint.
	OutputType string `json:"outputType"`
	// ApplicationTools reports whether an application-managed tool was used.
	ApplicationTools bool `json:"applicationTools"`
	// MCPTools reports whether an MCP tool was used.
	MCPTools bool `json:"MCPTools"`
	// ApplicationToolsAvailable stores the value represented by this member.
	ApplicationToolsAvailable bool `json:"applicationToolsAvailable"`
	// MCPToolsAvailable stores the value represented by this member.
	MCPToolsAvailable bool `json:"MCPToolsAvailable"`
	// ApplicationToolsUsed stores the value represented by this member.
	ApplicationToolsUsed bool `json:"applicationToolsUsed"`
	// MCPToolsUsed stores the value represented by this member.
	MCPToolsUsed bool `json:"MCPToolsUsed"`
	// MCPToolNames stores the value represented by this member.
	MCPToolNames []string `json:"MCPToolNames,omitempty"`
	// MCPToolsUsedArguments maps each MCP tool name to the JSON argument
	// payloads used across the interaction, in call order.
	MCPToolsUsedArguments map[string][]string `json:"MCPToolsUsedArguments,omitempty"`
	// ApplicationToolUseOutcome stores the value represented by this member.
	ApplicationToolUseOutcome string `json:"applicationToolUseOutcome"`
	// MCPToolUseOutcome stores the value represented by this member.
	MCPToolUseOutcome string `json:"MCPToolUseOutcome"`
	// ModelLoadTime records the server-reported model load transition duration
	// before inference began. It is zero when the model was already loaded or
	// the server does not expose lifecycle timing.
	ModelLoadTime time.Duration
	// CollectedAt records when the snapshot was finished.
	CollectedAt time.Time
	// Interaction stores the inference responses represented by this snapshot.
	Interaction []Interaction
	// ReasoningCutoff contains optional runtime cutoff telemetry.
	ReasoningCutoff *ReasoningCutoffSnapshot `json:"reasoning_cutoff,omitempty"`
	// ReasoningTokens stores the value represented by this member.
	ReasoningTokens *int `json:"reasoning_tokens,omitempty"`
	// ResponseTokens stores the value represented by this member.
	ResponseTokens *int `json:"response_tokens,omitempty"`
	// Context contains the highest-confidence context usage telemetry collected during inference.
	Context *SnapshotContextTelemetry `json:"context,omitempty"`
	// Request stores sanitized generation configuration without prompt content.
	Request map[string]any `json:"request,omitempty"`
	// Workload stores prompt-shape metadata and a non-reversible workload hash.
	Workload *SnapshotWorkloadTelemetry `json:"workload,omitempty"`
	// Timing stores client-side inference lifecycle timestamps and durations.
	Timing *SnapshotTimingTelemetry `json:"timing,omitempty"`
	// SlotSamples stores compact timestamped slot telemetry samples.
	SlotSamples []SnapshotSlotSample `json:"slot_samples,omitempty"`
	// Outcome stores normalized completion status and error information.
	Outcome *SnapshotOutcomeTelemetry `json:"outcome,omitempty"`
	// Messages stores the complete chat history represented by this snapshot.
	Messages []Message `json:"messages"`
	// Props stores the /props response when available.
	Props *PropsData
	// Slots stores the /slots response when available.
	Slots SlotsData
	// Metrics stores parsed metric data when available.
	Metrics *MetricsData
}

// SnapshotContextTelemetry describes context occupancy observed during one inference.
type SnapshotContextTelemetry struct {
	Tokens      *int     `json:"tokens,omitempty"`
	Capacity    *int     `json:"capacity,omitempty"`
	Percent     *float64 `json:"percent,omitempty"`
	PeakTokens  *int     `json:"peak_tokens,omitempty"`
	PeakPercent *float64 `json:"peak_percent,omitempty"`
	Shifts      *int     `json:"shifts,omitempty"`
}

// SnapshotWorkloadTelemetry describes workload shape without retaining prompt text.
type SnapshotWorkloadTelemetry struct {
	MessageCount    int    `json:"message_count"`
	UserMessages    int    `json:"user_messages"`
	ToolMessages    int    `json:"tool_messages"`
	PromptBytes     int    `json:"prompt_bytes"`
	AttachmentCount int    `json:"attachment_count"`
	ToolCount       int    `json:"tool_count"`
	InputType       string `json:"input_type,omitempty"`
	Hash            string `json:"hash,omitempty"`
}

// SnapshotTimingTelemetry records the client-side inference lifecycle.
type SnapshotTimingTelemetry struct {
	StartedAt          time.Time  `json:"started_at"`
	FirstTokenAt       *time.Time `json:"first_token_at,omitempty"`
	CompletedAt        time.Time  `json:"completed_at"`
	DurationMS         float64    `json:"duration_ms"`
	TimeToFirstTokenMS *float64   `json:"time_to_first_token_ms,omitempty"`
}

// SnapshotSlotSample is a compact, timestamped projection of /slots telemetry.
type SnapshotSlotSample struct {
	AtMS                  int64 `json:"at_ms"`
	SlotID                int   `json:"slot_id,omitempty"`
	ActiveSlots           int   `json:"active_slots"`
	ContextTokens         int   `json:"context_tokens"`
	ContextCapacity       int   `json:"context_capacity"`
	PromptTokens          int   `json:"prompt_tokens"`
	PromptTokensProcessed int   `json:"prompt_tokens_processed"`
	DecodedTokens         int   `json:"decoded_tokens"`
	CachedTokens          int   `json:"cached_tokens"`
	IsProcessing          bool  `json:"is_processing"`
}

// SnapshotOutcomeTelemetry records normalized request completion state.
type SnapshotOutcomeTelemetry struct {
	Status       string `json:"status"`
	FinishReason string `json:"finish_reason,omitempty"`
	Error        string `json:"error,omitempty"`
	Cancelled    bool   `json:"cancelled,omitempty"`
}
