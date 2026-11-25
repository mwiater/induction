// Package induction contains the core implementation for the public Induction API.
package induction

// Message represents a single chat message sent to a completion endpoint.
// Message is an exported type in this package's public data model.
type Message struct {
	// Role identifies the speaker, such as "system", "user", or "assistant".
	Role string `json:"role"`
	// Content accepts either text or an array of multimodal content objects.
	Content any `json:"content"`
	// ToolCalls carries function calls requested by an assistant message.
	ToolCalls []InferenceToolCall `json:"tool_calls,omitempty"`
	// ToolCallID associates a tool result with the assistant call that requested it.
	ToolCallID string `json:"tool_call_id,omitempty"`
	// Name identifies the function that produced a tool result.
	Name string `json:"name,omitempty"`
}

// ContentPart is one item in a multimodal message. Set exactly one of Text,
// ImageURL, or File for the corresponding Type ("text", "image_url", or
// "file"). It intentionally models the OpenAI-compatible content shape so
// callers do not need to build raw maps.
// ContentPart is an exported type in this package's public data model.
type ContentPart struct {
	// Type stores the value represented by this member.
	Type string `json:"type"`
	// Text stores the value represented by this member.
	Text string `json:"text,omitempty"`
	// ImageURL stores the value represented by this member.
	ImageURL *ImageURLPart `json:"image_url,omitempty"`
	// File stores the value represented by this member.
	File *FileContentPart `json:"file,omitempty"`
}

// ImageURLPart identifies a public image URL or a data URL. Detail is an
// optional server-dependent hint such as "low", "high", or "auto".
// ImageURLPart is an exported type in this package's public data model.
type ImageURLPart struct {
	// URL stores the value represented by this member.
	URL string `json:"url"`
	// Detail stores the value represented by this member.
	Detail string `json:"detail,omitempty"`
}

// FileContentPart identifies an attached document. Servers commonly support
// one of FileID, FileURL, or FileData; Filename is required with FileData.
// FileContentPart is an exported type in this package's public data model.
type FileContentPart struct {
	// FileID stores the value represented by this member.
	FileID string `json:"file_id,omitempty"`
	// FileURL stores the value represented by this member.
	FileURL string `json:"file_url,omitempty"`
	// FileData stores the value represented by this member.
	FileData string `json:"file_data,omitempty"`
	// Filename stores the value represented by this member.
	Filename string `json:"filename,omitempty"`
}

// ChatRequest defines the payload sent to llama.cpp-compatible completion endpoints.
// It supports both /v1/chat/completions and completion-style requests.
// ChatRequest is an exported type in this package's public data model.
type ChatRequest struct {
	// Messages carries a chat transcript for chat-completion-style requests.
	Messages []Message `json:"messages,omitempty"`
	// Prompt accepts a string or an array of token IDs for completion requests.
	Prompt any `json:"prompt,omitempty"`
	// Model stores the value represented by this member.
	Model string `json:"model,omitempty"`
	// Stream stores the value represented by this member.
	Stream *bool `json:"stream,omitempty"`
	// ReasoningControl stores the value represented by this member.
	ReasoningControl *bool `json:"reasoning_control,omitempty"`
	// Logprobs stores the value represented by this member.
	Logprobs *bool `json:"logprobs,omitempty"`
	// TopLogprobs stores the value represented by this member.
	TopLogprobs *int `json:"top_logprobs,omitempty"`
	// ChatTemplateKwargs contains llama.cpp per-request chat-template options.
	// Classification uses it to disable model thinking when supported.
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`

	// MaxTokens stores the value represented by this member.
	MaxTokens *int `json:"max_tokens,omitempty"`
	// MaxCompletionTokens stores the value represented by this member.
	MaxCompletionTokens *int `json:"max_completion_tokens,omitempty"`
	// NPredict stores the value represented by this member.
	NPredict *int `json:"n_predict,omitempty"`
	// Stop accepts either a string or an array of strings.
	Stop any `json:"stop,omitempty"`
	// Seed stores the value represented by this member.
	Seed *int `json:"seed,omitempty"`
	// NProbs stores the value represented by this member.
	NProbs *int `json:"n_probs,omitempty"`
	// IgnoreEOS stores the value represented by this member.
	IgnoreEOS *bool `json:"ignore_eos,omitempty"`

	// Temperature stores the value represented by this member.
	Temperature *float64 `json:"temperature,omitempty"`
	// TopP stores the value represented by this member.
	TopP *float64 `json:"top_p,omitempty"`
	// TopK stores the value represented by this member.
	TopK *int `json:"top_k,omitempty"`
	// MinP stores the value represented by this member.
	MinP *float64 `json:"min_p,omitempty"`
	// PresencePenalty stores the value represented by this member.
	PresencePenalty *float64 `json:"presence_penalty,omitempty"`
	// FrequencyPenalty stores the value represented by this member.
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	// RepeatPenalty stores the value represented by this member.
	RepeatPenalty *float64 `json:"repeat_penalty,omitempty"`
	// RepeatLastN stores the value represented by this member.
	RepeatLastN *int `json:"repeat_last_n,omitempty"`
	// PenalizeNL stores the value represented by this member.
	PenalizeNL *bool `json:"penalize_nl,omitempty"`
	// LogitBias stores the value represented by this member.
	LogitBias map[string]float64 `json:"logit_bias,omitempty"`

	// TfsZ stores the value represented by this member.
	TfsZ *float64 `json:"tfs_z,omitempty"`
	// TypicalP stores the value represented by this member.
	TypicalP *float64 `json:"typical_p,omitempty"`
	// Mirostat stores the value represented by this member.
	Mirostat *int `json:"mirostat,omitempty"`
	// MirostatTau stores the value represented by this member.
	MirostatTau *float64 `json:"mirostat_tau,omitempty"`
	// MirostatEta stores the value represented by this member.
	MirostatEta *float64 `json:"mirostat_eta,omitempty"`
	// Samplers stores the value represented by this member.
	Samplers []string `json:"samplers,omitempty"`

	// XTCThreshold stores the value represented by this member.
	XTCThreshold *float64 `json:"xtc_threshold,omitempty"`
	// XTCProbability stores the value represented by this member.
	XTCProbability *float64 `json:"xtc_probability,omitempty"`

	// DryMultiplier stores the value represented by this member.
	DryMultiplier *float64 `json:"dry_multiplier,omitempty"`
	// DryBase stores the value represented by this member.
	DryBase *float64 `json:"dry_base,omitempty"`
	// DryAllowedLength stores the value represented by this member.
	DryAllowedLength *int `json:"dry_allowed_length,omitempty"`
	// DryPenaltyLastN stores the value represented by this member.
	DryPenaltyLastN *int `json:"dry_penalty_last_n,omitempty"`
	// DrySequenceBreakers stores the value represented by this member.
	DrySequenceBreakers []string `json:"dry_sequence_breakers,omitempty"`

	// DynatempRange stores the value represented by this member.
	DynatempRange *float64 `json:"dynatemp_range,omitempty"`
	// DynatempExponent stores the value represented by this member.
	DynatempExponent *float64 `json:"dynatemp_exponent,omitempty"`

	// ResponseFormat stores the value represented by this member.
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	// Grammar stores the value represented by this member.
	Grammar string `json:"grammar,omitempty"`
	// JSONSchema stores the value represented by this member.
	JSONSchema any `json:"json_schema,omitempty"`
	// Tools stores the value represented by this member.
	Tools []Tool `json:"tools,omitempty"`
	// ToolChoice accepts "auto", "none", or a specific tool choice object.
	ToolChoice any `json:"tool_choice,omitempty"`

	// CachePrompt stores the value represented by this member.
	CachePrompt *bool `json:"cache_prompt,omitempty"`
	// SlotID stores the value represented by this member.
	SlotID *int `json:"slot_id,omitempty"`
	// NKeep stores the value represented by this member.
	NKeep *int `json:"n_keep,omitempty"`
	// ImageData stores the value represented by this member.
	ImageData []ImageData `json:"image_data,omitempty"`
	// Attachment filenames are retained for snapshot redaction and are not
	// sent to the inference server.
	ImageFilename string `json:"-"`
	// DocumentFilename stores the value represented by this member.
	DocumentFilename string `json:"-"`
	// Classification is an internal pipeline execution mode and is never sent
	// as part of the llama.cpp request payload.
	Classification *ClassificationConfig `json:"-"`
	// Decision stores the value represented by this member.
	Decision *DecisionConfig `json:"-"`
}

// ResponseFormat configures JSON-object or JSON-schema constrained output.
// ResponseFormat is an exported type in this package's public data model.
type ResponseFormat struct {
	// Type stores the value represented by this member.
	Type string `json:"type" yaml:"type"`
	// JSONSchema stores the value represented by this member.
	JSONSchema any `json:"json_schema,omitempty" yaml:"jsonSchema,omitempty"`
}

// Tool describes a tool available to the model.
// Tool is an exported type in this package's public data model.
type Tool struct {
	// Type stores the value represented by this member.
	Type string `json:"type"`
	// Function stores the value represented by this member.
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable function and its JSON Schema parameters.
// ToolFunction is an exported type in this package's public data model.
type ToolFunction struct {
	// Name stores the value represented by this member.
	Name string `json:"name"`
	// Description stores the value represented by this member.
	Description string `json:"description,omitempty"`
	// Parameters stores the value represented by this member.
	Parameters any `json:"parameters,omitempty"`
}

// ImageData carries a base64-encoded image for multimodal inference.
// ImageData is an exported type in this package's public data model.
type ImageData struct {
	// Data stores the value represented by this member.
	Data string `json:"data"`
	// ID stores the value represented by this member.
	ID int `json:"id"`
}

// InferenceResponse is the OpenAI-compatible response returned by Infer.
// Choices supports both chat-completion messages and completion text.
// InferenceResponse is an exported type in this package's public data model.
type InferenceResponse struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// Object stores the value represented by this member.
	Object string `json:"object"`
	// Created stores the value represented by this member.
	Created int64 `json:"created"`
	// Model stores the value represented by this member.
	Model string `json:"model"`
	// SystemFingerprint stores the value represented by this member.
	SystemFingerprint string `json:"system_fingerprint,omitempty"`
	// Choices stores the value represented by this member.
	Choices []InferenceChoice `json:"choices"`
	// Usage stores the value represented by this member.
	Usage *InferenceUsage `json:"usage,omitempty"`
}

// InferenceChoice is one generated choice from a chat or completion response.
// InferenceChoice is an exported type in this package's public data model.
type InferenceChoice struct {
	// Index stores the value represented by this member.
	Index int `json:"index"`
	// Message stores the value represented by this member.
	Message *InferenceResponseMessage `json:"message,omitempty"`
	// Text stores the value represented by this member.
	Text string `json:"text,omitempty"`
	// Logprobs stores the value represented by this member.
	Logprobs *ChoiceLogprobs `json:"logprobs,omitempty"`
	// FinishReason stores the value represented by this member.
	FinishReason *string `json:"finish_reason,omitempty"`
}

// TokenLogprob is an exported type in this package's public data model.
type TokenLogprob struct {
	// Token stores the value represented by this member.
	Token string `json:"token"`
	// Logprob stores the value represented by this member.
	Logprob float64 `json:"logprob"`
	// Bytes stores the value represented by this member.
	Bytes []int `json:"bytes,omitempty"`
}

// TokenLogprobPosition is an exported type in this package's public data model.
type TokenLogprobPosition struct {
	// Token stores the value represented by this member.
	Token string `json:"token"`
	// Logprob stores the value represented by this member.
	Logprob float64 `json:"logprob"`
	// Bytes stores the value represented by this member.
	Bytes []int `json:"bytes,omitempty"`
	// TopLogprobs stores the value represented by this member.
	TopLogprobs []TokenLogprob `json:"top_logprobs"`
}

// ChoiceLogprobs is an exported type in this package's public data model.
type ChoiceLogprobs struct {
	// Content stores the value represented by this member.
	Content []TokenLogprobPosition `json:"content"`
}

// InferenceResponseMessage is an assistant message returned by the model.
// InferenceResponseMessage is an exported type in this package's public data model.
type InferenceResponseMessage struct {
	// Role stores the value represented by this member.
	Role string `json:"role"`
	// Content stores the value represented by this member.
	Content string `json:"content"`
	// ReasoningContent stores the value represented by this member.
	ReasoningContent string `json:"reasoning_content,omitempty"`
	// Refusal stores the value represented by this member.
	Refusal string `json:"refusal,omitempty"`
	// ToolCalls stores the value represented by this member.
	ToolCalls []InferenceToolCall `json:"tool_calls,omitempty"`
}

// InferenceToolCall describes a function call requested by the model.
// InferenceToolCall is an exported type in this package's public data model.
type InferenceToolCall struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// Type stores the value represented by this member.
	Type string `json:"type"`
	// Function stores the value represented by this member.
	Function InferenceFunctionCall `json:"function"`
}

// InferenceFunctionCall contains a requested function name and JSON arguments.
// InferenceFunctionCall is an exported type in this package's public data model.
type InferenceFunctionCall struct {
	// Name stores the value represented by this member.
	Name string `json:"name"`
	// Arguments stores the value represented by this member.
	Arguments string `json:"arguments"`
}

// InferenceUsage contains OpenAI-compatible token counts.
// InferenceUsage is an exported type in this package's public data model.
type InferenceUsage struct {
	// PromptTokens stores the value represented by this member.
	PromptTokens int `json:"prompt_tokens"`
	// CompletionTokens stores the value represented by this member.
	CompletionTokens int `json:"completion_tokens"`
	// TotalTokens stores the value represented by this member.
	TotalTokens int `json:"total_tokens"`
}

// InferenceStreamChunk is one OpenAI-compatible streaming response object.
// InferenceStreamChunk is an exported type in this package's public data model.
type InferenceStreamChunk struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// Object stores the value represented by this member.
	Object string `json:"object"`
	// Created stores the value represented by this member.
	Created int64 `json:"created"`
	// Model stores the value represented by this member.
	Model string `json:"model"`
	// SystemFingerprint stores the value represented by this member.
	SystemFingerprint string `json:"system_fingerprint,omitempty"`
	// Choices stores the value represented by this member.
	Choices []InferenceStreamChoice `json:"choices"`
	// Usage stores the value represented by this member.
	Usage *InferenceUsage `json:"usage,omitempty"`
}

// InferenceStreamChoice contains a chat delta or completion text fragment.
// InferenceStreamChoice is an exported type in this package's public data model.
type InferenceStreamChoice struct {
	// Index stores the value represented by this member.
	Index int `json:"index"`
	// Delta stores the value represented by this member.
	Delta InferenceStreamDelta `json:"delta,omitempty"`
	// Text stores the value represented by this member.
	Text string `json:"text,omitempty"`
	// Logprobs stores the value represented by this member.
	Logprobs *ChoiceLogprobs `json:"logprobs,omitempty"`
	// FinishReason stores the value represented by this member.
	FinishReason *string `json:"finish_reason,omitempty"`
}

// InferenceStreamDelta contains incremental assistant content and tool calls.
// InferenceStreamDelta is an exported type in this package's public data model.
type InferenceStreamDelta struct {
	// Role stores the value represented by this member.
	Role string `json:"role,omitempty"`
	// Content stores the value represented by this member.
	Content string `json:"content,omitempty"`
	// ReasoningContent stores the value represented by this member.
	ReasoningContent string `json:"reasoning_content,omitempty"`
	// Refusal stores the value represented by this member.
	Refusal string `json:"refusal,omitempty"`
	// ToolCalls stores the value represented by this member.
	ToolCalls []InferenceStreamToolCall `json:"tool_calls,omitempty"`
	// Optional llama.cpp extensions used for authoritative cutoff accounting.
	ReasoningTokens *int `json:"reasoning_tokens,omitempty"`
	// ContextTokens stores the value represented by this member.
	ContextTokens *int `json:"context_tokens,omitempty"`
	// ContextSize stores the value represented by this member.
	ContextSize *int `json:"context_size,omitempty"`
}

// InferenceStreamToolCall contains an incremental tool-call update.
// InferenceStreamToolCall is an exported type in this package's public data model.
type InferenceStreamToolCall struct {
	// Index stores the value represented by this member.
	Index int `json:"index"`
	// ID stores the value represented by this member.
	ID string `json:"id,omitempty"`
	// Type stores the value represented by this member.
	Type string `json:"type,omitempty"`
	// Function stores the value represented by this member.
	Function InferenceFunctionCall `json:"function,omitempty"`
}

// Interaction stores the response body and best-effort extracted text content.
// Interaction is an exported type in this package's public data model.
type Interaction struct {
	// Content stores the extracted assistant text when it can be parsed.
	Content string `json:"content"`
	// ReasoningContent stores separately returned model reasoning when present.
	ReasoningContent string `json:"reasoning_content,omitempty"`
	// Response stores the raw response body returned by the server.
	Response string `json:"response"`
}

// PropsData represents the server's /props response payload.
// PropsData is an exported type in this package's public data model.
type PropsData struct {
	// Raw keeps the full unmodified response body.
	Raw string `json:"raw,omitempty"`
	// TotalSlots reports the number of slots exposed by the server.
	TotalSlots int `json:"total_slots,omitempty"`
	// DefaultGenerationSettings holds the parsed generation settings map.
	DefaultGenerationSettings map[string]interface{} `json:"default_generation_settings,omitempty"`
}

// SlotsData is a slice alias for slot telemetry records.
// SlotsData is an exported type in this package's public data model.
type SlotsData []map[string]interface{}

// MetricsData holds the raw Prometheus text and parsed metric entries.
// MetricsData is an exported type in this package's public data model.
type MetricsData struct {
	// Raw keeps the original metrics payload.
	Raw string `json:"raw"`
	// Entries stores parsed metric values keyed by metric name.
	Entries map[string]interface{} `json:"entries"`
}

// ModelSnapshot aggregates all telemetry and inference data for a request.
