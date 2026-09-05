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
		o.enableLiveMetricsOverlay = enabled
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

// WithInitialChatPrompt pre-fills the console chat input after the initial
// model is ready. If autoSubmit is true, the prompt is submitted immediately
// using the same path as pressing Enter.
func WithInitialChatPrompt(prompt string, autoSubmit bool) ClientOption {
	return func(o *ClientOptions) {
		o.initialChatPrompt = prompt
		o.initialChatPromptAutoSubmit = autoSubmit
	}
}

// WithAutoExitAfterInitialChat exits the console after the automated initial
// chat turn and its session snapshot have been saved.
func WithAutoExitAfterInitialChat(enabled bool) ClientOption {
	return func(o *ClientOptions) {
		o.autoExitAfterInitialChat = enabled
	}
}

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

func withLiveMetricsOverlay(overlay *liveMetricsOverlay) ClientOption {
	return func(o *ClientOptions) {
		o.liveMetricsOverlay = overlay
	}
}
