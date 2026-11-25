package induction

import (
	"io"
	"log"
	"net/http"
	"testing"
	"time"
)

// TestOptions verifies each functional option mutates ClientOptions as expected.
func TestOptions(t *testing.T) {
	customClient := &http.Client{Timeout: 10 * time.Second}
	customLogger := log.New(io.Discard, "", 0)

	opts := ClientOptions{
		httpClient:       &http.Client{},
		pollInterval:     1 * time.Second,
		loadWaitInterval: 1 * time.Second,
	}

	optFuncs := []ClientOption{
		WithHTTPClient(customClient),
		WithPollInterval(5 * time.Second),
		WithLoadWaitInterval(3 * time.Second),
		WithLogger(customLogger),
	}

	for _, fn := range optFuncs {
		fn(&opts)
	}

	if opts.httpClient != customClient {
		t.Errorf("expected custom HTTP client, got default")
	}
	if opts.pollInterval != 5*time.Second {
		t.Errorf("expected 5s poll interval, got %v", opts.pollInterval)
	}
	if opts.loadWaitInterval != 3*time.Second {
		t.Errorf("expected 3s wait interval, got %v", opts.loadWaitInterval)
	}
	if opts.logger != customLogger {
		t.Errorf("expected custom logger")
	}
}
