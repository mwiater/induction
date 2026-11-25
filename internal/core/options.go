// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"net/http"
	"time"
)

// WithPollInterval sets how often /slots is sampled while inference is active.
func WithPollInterval(d time.Duration) ClientOption {
	return func(o *ClientOptions) {
		o.pollInterval = d
	}
}

// WithLoadWaitInterval sets the wait interval used while a model is loading.
func WithLoadWaitInterval(d time.Duration) ClientOption {
	return func(o *ClientOptions) {
		o.loadWaitInterval = d
	}
}

// WithLiveMetricsOverlay controls the terminal overlay shown while snapshots
// are collecting an inference response.
func WithLiveMetricsOverlay(enabled bool) ClientOption {
	return func(o *ClientOptions) {
		o.liveMetricsOverlayEnabled = enabled
	}
}

// WithHTTPClient injects a custom HTTP client into the Induction client.
func WithHTTPClient(c *http.Client) ClientOption {
	return func(o *ClientOptions) {
		o.httpClient = c
	}
}

// WithLogger routes Induction messages through the application's logger.
// Induction is silent unless a logger is supplied.
func WithLogger(logger Logger) ClientOption {
	return func(o *ClientOptions) {
		o.logger = logger
	}
}

// WithConfigPath selects the YAML configuration file used by config-driven
// inference. The default remains induction.yaml in the working directory.
func WithConfigPath(path string) ClientOption {
	return func(o *ClientOptions) { o.configPath = path }
}

// WithInitialChatPrompt pre-fills the console chat input after the initial
// model is ready. If autoSubmit is true, the prompt is submitted immediately
// using the same path as pressing Enter.
func WithInitialChatPrompt(prompt string, autoSubmit bool) ClientOption {
	return func(o *ClientOptions) {
		o.initialChatPrompt = prompt
		o.initialChatPromptAutoSubmit = autoSubmit
	}
}

// WithInitialModelSelection opens the existing model selector before the
// initial prompt is submitted. The prompt remains pending until a model is
// selected and loaded successfully.
func WithInitialModelSelection(enabled bool) ClientOption {
	return func(o *ClientOptions) { o.initialModelSelection = enabled }
}

// WithAutoExitAfterInitialChat exits the console after the automated initial
// chat turn and its session snapshot have been saved.
func WithAutoExitAfterInitialChat(enabled bool) ClientOption {
	return func(o *ClientOptions) {
		o.autoExitAfterInitialChat = enabled
	}
}

// WithSessionSaved registers a callback invoked with the path of a session
// after it has been written successfully.
func WithSessionSaved(callback func(string)) ClientOption {
	return func(o *ClientOptions) { o.sessionSaved = callback }
}

// WithBatchMetadata attaches durable batch and item identifiers to a child
// session created for a batch execution.
func WithBatchMetadata(batchID, itemID string) ClientOption {
	return func(o *ClientOptions) {
		o.batchID = batchID
		o.batchItemID = itemID
	}
}

// WithInferenceCompleted registers a callback invoked after a UI inference
// turn completes. It is intended for UI-backed application workflows that
// need to consume the structured result after Bubble Tea has rendered it.
func WithInferenceCompleted(callback func(*ModelSnapshot, string)) ClientOption {
	return func(o *ClientOptions) { o.inferenceCompleted = callback }
}

// WithPipeline enables sequential, automatically submitted chat steps in the
// console UI. Pipeline mode exits after the final step is saved.
func WithPipeline(pipeline *Pipeline) ClientOption {
	return func(o *ClientOptions) { o.pipeline = pipeline }
}

// WithMCPFooter controls the MCP status bar for direct inference. It does not
// enable MCP tool calls; callers must select the MCP execution path separately.
func WithMCPFooter(enabled bool) ClientOption {
	return func(o *ClientOptions) {
		o.mcpFooterConfigured = true
		o.mcpFooterEnabled = enabled
	}
}

// WithArtifactPathContext supplies runtime source and batch metadata for
// artifact path interpolation.
func WithArtifactPathContext(context ArtifactPathContext) ClientOption {
	return func(o *ClientOptions) { o.artifactPathContext = &context }
}

// withMCPTools performs the internal operation described by its signature.
func withMCPTools(names ...string) ClientOption {
	return func(o *ClientOptions) {
		o.mcpTools = true
		o.mcpToolNames = append([]string(nil), names...)
	}
}

// WithApplicationToolHandler supplies the local implementation for tools in
// ChatRequest.Tools when using InferApplicationToolsChat.
func WithApplicationToolHandler(handler ApplicationToolHandler) ClientOption {
	return func(o *ClientOptions) { o.applicationToolHandler = handler }
}

// WithApplicationToolChain adds related calls to model-requested application
// tool calls before their results are returned to the model.
func WithApplicationToolChain(chain ApplicationToolChain) ClientOption {
	return func(o *ClientOptions) { o.applicationToolChain = chain }
}

// withLiveMetricsOverlay performs the internal operation described by its signature.
func withLiveMetricsOverlay(overlay *liveMetricsOverlay) ClientOption {
	return func(o *ClientOptions) {
		o.liveMetricsOverlay = overlay
	}
}

// withResourceBudget performs the internal operation described by its signature.
func withResourceBudget(budget *ResourceBudgetConfig) ClientOption {
	return func(o *ClientOptions) { o.resourceBudget = budget }
}

// withProgressOverlay performs the internal operation described by its signature.
func withProgressOverlay(overlay *liveMetricsOverlay) ClientOption {
	return func(o *ClientOptions) { o.progressOverlay = overlay }
}
