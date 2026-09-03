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

func withLiveMetricsOverlay(overlay *liveMetricsOverlay) ClientOption {
	return func(o *ClientOptions) {
		o.liveMetricsOverlay = overlay
	}
}
