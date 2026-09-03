package induction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pterm/pterm"
	"gopkg.in/yaml.v3"
)

func TestInferErrorPaths(t *testing.T) {
	nan := math.NaN()
	client := newTestClient("http://example.com")
	if _, err := client.infer(context.Background(), &ChatRequest{Temperature: &nan}); err == nil {
		t.Fatal("expected marshal error")
	}
	if _, err := newTestClient(":").infer(context.Background(), &ChatRequest{}); err == nil {
		t.Fatal("expected request construction error")
	}

	for name, transport := range map[string]roundTripperFunc{
		"transport": func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
		"read": func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}}, nil
		},
		"status": func(*http.Request) (*http.Response, error) {
			return responseWithBody(http.StatusBadRequest, "bad request"), nil
		},
		"decode": func(*http.Request) (*http.Response, error) {
			return responseWithBody(http.StatusOK, "not-json"), nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			client := newTestClient("http://example.com")
			client.opts.httpClient = &http.Client{Transport: transport}
			if _, err := client.infer(context.Background(), &ChatRequest{}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestInferStreamChunkErrorPaths(t *testing.T) {
	nan := math.NaN()
	client := newTestClient("http://example.com")
	if err := client.inferStreamChunks(context.Background(), &ChatRequest{Temperature: &nan}, func(InferenceStreamChunk) error { return nil }); err == nil {
		t.Fatal("expected marshal error")
	}
	if err := newTestClient(":").inferStreamChunks(context.Background(), &ChatRequest{}, func(InferenceStreamChunk) error { return nil }); err == nil {
		t.Fatal("expected request construction error")
	}

	tests := []struct {
		name      string
		transport roundTripperFunc
		yield     func(InferenceStreamChunk) error
	}{
		{"transport", func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") }, func(InferenceStreamChunk) error { return nil }},
		{"status", func(*http.Request) (*http.Response, error) {
			return responseWithBody(http.StatusBadRequest, "bad"), nil
		}, func(InferenceStreamChunk) error { return nil }},
		{"decode", func(*http.Request) (*http.Response, error) {
			return responseWithBody(http.StatusOK, "data: nope\n"), nil
		}, func(InferenceStreamChunk) error { return nil }},
		{"handler", func(*http.Request) (*http.Response, error) {
			return responseWithBody(http.StatusOK, "event: message\n\ndata: {}\n"), nil
		}, func(InferenceStreamChunk) error { return errors.New("boom") }},
		{"read", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}}, nil
		}, func(InferenceStreamChunk) error { return nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient("http://example.com")
			client.opts.httpClient = &http.Client{Transport: tc.transport}
			if err := client.inferStreamChunks(context.Background(), &ChatRequest{}, tc.yield); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestConfigValidationBranches(t *testing.T) {
	valid := Config{Server: "server", Timeout: 1, PollInterval: 1, LoadWaitInterval: 1, Log: LogConfig{Path: "log"}}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	tests := []func(*Config){
		func(c *Config) { c.Server = "" },
		func(c *Config) { c.Timeout = 0 },
		func(c *Config) { c.PollInterval = 0 },
		func(c *Config) { c.LoadWaitInterval = 0 },
		func(c *Config) { c.Log.Path = "" },
	}
	for i, mutate := range tests {
		cfg := valid
		mutate(&cfg)
		if err := cfg.validate(); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
	var duration Duration
	if err := duration.UnmarshalYAML(&yaml.Node{Value: "invalid"}); err == nil {
		t.Fatal("expected duration parse error")
	}
}

func TestStreamAndContentParsingBranches(t *testing.T) {
	for input, want := range map[string]string{
		"":             "",
		"data: [DONE]": doneStreamToken,
		`data: {"choices":[{"delta":{"content":"delta"}}]}`: "delta",
		`{"choices":[{"message":{"content":"message"}}]}`:   "message",
		`{"choices":[{"text":"text"}]}`:                     "text",
		`{"content":"content"}`:                             "content",
		`{"text":"root"}`:                                   "root",
		"plain":                                             "plain",
	} {
		if got := streamedContent(input); got != want {
			t.Errorf("streamedContent(%q)=%q, want %q", input, got, want)
		}
	}
	for input, want := range map[string]string{
		`{"choices":[{"message":{"content":"chat"}}]}`: "chat",
		`{"choices":[{"text":"completion"}]}`:          "completion",
		`{"content":"native"}`:                         "native",
		`not-json`:                                     "",
		`{}`:                                           "",
	} {
		if got := extractInteractionContent([]byte(input)); got != want {
			t.Errorf("extractInteractionContent(%q)=%q, want %q", input, got, want)
		}
	}
	for input, want := range map[string]string{
		`{"choices":[{"message":{"reasoning_content":"message reasoning"}}]}`: "message reasoning",
		`{"choices":[{"reasoning_content":"choice reasoning"}]}`:              "choice reasoning",
		`{"reasoning_content":"root reasoning"}`:                              "root reasoning",
		`not-json`:                                                            "",
	} {
		if got := extractInteractionReasoningContent([]byte(input)); got != want {
			t.Errorf("extractInteractionReasoningContent(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestLiveMetricsOverlayLifecycleAndNumericHelpers(t *testing.T) {
	pterm.DisableOutput()
	started := startLiveMetricsOverlay("model")
	started.Stop()
	pterm.EnableOutput()

	var output bytes.Buffer
	footer := &stickyFooter{writer: &output, size: func() (int, int, error) { return 80, 24, nil }, width: 80, height: 24, active: true}
	overlay := &liveMetricsOverlay{footer: footer, startedAt: time.Now().Add(-time.Second), model: "model"}
	overlay.Update(nil)
	overlay.UpdateLoading(modelLoadProgress{Stage: "spec_model"})
	overlay.Update(SlotsData{{"n_ctx": 100, "n_prompt_tokens_processed": 10}})
	overlay.ModelLoaded()
	overlay.Update(SlotsData{{"n_ctx": 100, "n_prompt_tokens_processed": 10, "n_decoded": 2}})
	overlay.lastAt = time.Now().Add(-time.Second)
	overlay.lastPrompt, overlay.lastGenerated, overlay.hasMeasurement = 20, 10, true
	overlay.Update(SlotsData{{"n_ctx": 100, "n_prompt_tokens_processed": 10, "n_decoded": 5}})
	overlay.Stop()
	if output.Len() == 0 || footer.active {
		t.Fatal("expected rendered and stopped footer")
	}

	if got := loadingStageLabel("unknown"); got != "unknown" {
		t.Fatalf("unexpected stage label: %q", got)
	}
	if got := formatModelLoading("model", modelLoadProgress{}); !strings.Contains(got, "Loading: Model") {
		t.Fatalf("unexpected loading fallback: %q", got)
	}
	for _, value := range []any{float64(1), float32(1), int(1), int64(1), json.Number("1")} {
		if got, ok := number(value); !ok || got != 1 {
			t.Fatalf("number(%T)=%v,%v", value, got, ok)
		}
	}
	if _, ok := number("nope"); ok || nonNegative(-1) != 0 || nonNegative(1) != 1 {
		t.Fatal("numeric fallback failed")
	}
	if _, ok := number(json.Number("bad")); ok {
		t.Fatal("expected invalid JSON number")
	}
}

func TestStickyFooterGuardBranches(t *testing.T) {
	var footer *stickyFooter
	footer.Update("ignored")
	footer.Stop()
	if got, ok := newStickyFooter(nil); ok || got != nil {
		t.Fatal("nil output should not create footer")
	}
	inactive := &stickyFooter{active: false}
	inactive.Update("ignored")
	inactive.Stop()
	var output bytes.Buffer
	active := &stickyFooter{writer: &output, size: func() (int, int, error) { return 0, 0, errors.New("size") }, width: 80, height: 24, active: true}
	active.Update("content")
	if !strings.Contains(output.String(), "content") {
		t.Fatal("expected update to retain existing dimensions after size error")
	}
}

func TestLegacyInferenceErrorPaths(t *testing.T) {
	client := newTestClient("http://example.com")
	if _, err := client.doInferenceAt(context.Background(), nil, "/completion"); err == nil {
		t.Fatal("expected nil request error")
	}
	if _, err := client.doStreamInferenceAt(context.Background(), nil, "/completion", io.Discard); err == nil {
		t.Fatal("expected nil request error")
	}
	nan := math.NaN()
	if _, err := client.doInferenceAt(context.Background(), &ChatRequest{Temperature: &nan}, "/completion"); err == nil {
		t.Fatal("expected marshal error")
	}
	if _, err := client.doStreamInferenceAt(context.Background(), &ChatRequest{Temperature: &nan}, "/completion", io.Discard); err == nil {
		t.Fatal("expected stream marshal error")
	}
	if _, err := newTestClient(":").doInferenceAt(context.Background(), &ChatRequest{}, "/completion"); err == nil {
		t.Fatal("expected request construction error")
	}
	if _, err := newTestClient(":").doStreamInferenceAt(context.Background(), &ChatRequest{}, "/completion", io.Discard); err == nil {
		t.Fatal("expected stream request construction error")
	}

	for name, transport := range map[string]roundTripperFunc{
		"transport": func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
		"read": func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}}, nil
		},
	} {
		t.Run("inference_"+name, func(t *testing.T) {
			client := newTestClient("http://example.com")
			client.opts.httpClient = &http.Client{Transport: transport}
			if _, err := client.doInferenceAt(context.Background(), &ChatRequest{}, "/completion"); err == nil {
				t.Fatal("expected error")
			}
		})
	}

	streamCases := []struct {
		name      string
		transport roundTripperFunc
		out       io.Writer
	}{
		{"transport", func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") }, io.Discard},
		{"status", func(*http.Request) (*http.Response, error) {
			return responseWithBody(http.StatusBadRequest, "bad"), nil
		}, io.Discard},
		{"read", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}}, nil
		}, io.Discard},
		{"write", func(*http.Request) (*http.Response, error) {
			return responseWithBody(http.StatusOK, "data: {\"content\":\"hello\"}\n"), nil
		}, errorWriterForCoverage{}},
	}
	for _, tc := range streamCases {
		t.Run("stream_"+tc.name, func(t *testing.T) {
			client := newTestClient("http://example.com")
			client.opts.httpClient = &http.Client{Transport: tc.transport}
			if _, err := client.doStreamInferenceAt(context.Background(), &ChatRequest{}, "/completion", tc.out); err == nil {
				t.Fatal("expected error")
			}
		})
	}

	client.opts.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return responseWithBody(http.StatusOK, "data: [DONE]\n"), nil
	})}
	if _, err := client.doStreamInferenceAt(context.Background(), &ChatRequest{}, "/completion", nil); err != nil {
		t.Fatalf("nil output should use discard: %v", err)
	}
}

func TestGenerateSnapshotAndPollingGuards(t *testing.T) {
	client := newTestClient("http://example.com")
	if _, err := client.GenerateSnapshot(context.Background(), nil); err == nil {
		t.Fatal("expected nil request error")
	}
	client.opts.pollInterval = 0
	if samples := client.pollSlots(context.Background(), "model", nil); samples != nil {
		t.Fatalf("expected no samples: %#v", samples)
	}

	var output bytes.Buffer
	overlay := &liveMetricsOverlay{
		footer:    &stickyFooter{writer: &output, size: func() (int, int, error) { return 80, 24, nil }, width: 80, height: 24, active: true},
		startedAt: time.Now().Add(-time.Second),
	}
	calls := 0
	client.opts.pollInterval = time.Millisecond
	client.opts.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("boom")
		}
		return responseWithBody(http.StatusOK, `[{"n_ctx":100,"n_prompt_tokens_processed":1}]`), nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Millisecond)
	defer cancel()
	samples := client.pollSlots(ctx, "model", overlay)
	if calls < 2 || len(samples) == 0 || output.Len() == 0 {
		t.Fatalf("polling branches not exercised: calls=%d samples=%d output=%d", calls, len(samples), output.Len())
	}

	client.opts.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})}
	if _, err := client.GenerateSnapshot(context.Background(), &ChatRequest{Model: "model"}); err == nil {
		t.Fatal("expected inference failure")
	}
}

type errorWriterForCoverage struct{}

func (errorWriterForCoverage) Write([]byte) (int, error) { return 0, errors.New("boom") }

func TestModelLoadingStreamErrors(t *testing.T) {
	overlay := &liveMetricsOverlay{}
	ready := make(chan struct{})
	if err := newTestClient(":").monitorModelLoading(context.Background(), "model", overlay, ready); err == nil {
		t.Fatal("expected request error")
	}
	select {
	case <-ready:
	default:
		t.Fatal("ready should close on request error")
	}

	for name, transport := range map[string]roundTripperFunc{
		"transport": func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") },
		"status": func(*http.Request) (*http.Response, error) {
			return responseWithBody(http.StatusBadRequest, "bad"), nil
		},
		"read": func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			client := newTestClient("http://example.com")
			client.opts.httpClient = &http.Client{Transport: transport}
			if err := client.monitorModelLoading(context.Background(), "model", overlay, nil); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestModelAndHealthRequestErrors(t *testing.T) {
	if err := newTestClient(":").CheckHealth(); err == nil {
		t.Fatal("expected health request construction error")
	}
	client := newTestClient("http://example.com")
	client.opts.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})}
	if err := client.CheckHealth(); err == nil {
		t.Fatal("expected health transport error")
	}
	if err := client.ListLoadedModels(); err == nil {
		t.Fatal("expected loaded-model transport error")
	}
	if _, err := client.fetchModelRows(context.Background()); err == nil {
		t.Fatal("expected model-row transport error")
	}
	if err := client.ListModels(); err == nil {
		t.Fatal("expected list-model transport error")
	}

	if _, err := newTestClient(":").fetchModelRows(context.Background()); err == nil {
		t.Fatal("expected model-row request construction error")
	}
	client.opts.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return responseWithBody(http.StatusBadRequest, "bad"), nil
	})}
	if _, err := client.fetchModelRows(context.Background()); err == nil {
		t.Fatal("expected model-row status error")
	}
	if err := client.ListModels(); err == nil {
		t.Fatal("expected list-model status error")
	}

	if got := filterLoadedModelRows(nil); got != nil {
		t.Fatalf("expected nil filtered rows: %#v", got)
	}
	if got := argTokens("--one two"); len(got) != 2 {
		t.Fatalf("unexpected string args: %#v", got)
	}
	if got := argTokens([]string{"one"}); len(got) != 1 {
		t.Fatalf("unexpected string slice args: %#v", got)
	}
}

func TestGenerateSnapshotWithLiveOverlay(t *testing.T) {
	pterm.DisableOutput()
	defer pterm.EnableOutput()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/models/sse":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"model\":\"model\",\"data\":{\"status\":\"loading\",\"progress\":{\"stage\":\"text_model\",\"value\":0.5}}}\n\n")
			_, _ = io.WriteString(w, "data: {\"model\":\"model\",\"data\":{\"status\":\"loaded\"}}\n\n")
		case "/v1/chat/completions":
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
		case "/props":
			_, _ = io.WriteString(w, `{}`)
		case "/slots":
			_, _ = io.WriteString(w, `[]`)
		case "/metrics":
			_, _ = io.WriteString(w, "")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(context.Background(), srv.URL, WithLiveMetricsOverlay(true), WithPollInterval(0))
	snapshot, err := client.GenerateSnapshot(context.Background(), &ChatRequest{
		Model:    "model",
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("GenerateSnapshot with overlay failed: %v", err)
	}
	if len(snapshot.Interaction) != 1 || snapshot.Interaction[0].Content != "ok" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestLiveOverlayRunsForAllStandardInferenceModes(t *testing.T) {
	pterm.DisableOutput()
	defer pterm.EnableOutput()

	var loadingStreams atomic.Int32
	var slotPolls atomic.Int32
	httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/models/sse":
			loadingStreams.Add(1)
			return responseWithBody(http.StatusOK, "data: {\"model\":\"test-model\",\"data\":{\"status\":\"loaded\"}}\n\n"), nil
		case "/slots":
			slotPolls.Add(1)
			return responseWithBody(http.StatusOK, `[{"n_ctx":100,"n_prompt_tokens_processed":1}]`), nil
		case "/v1/chat/completions":
			time.Sleep(8 * time.Millisecond)
			if req.Header.Get("Accept") == "text/event-stream" {
				return responseWithBody(http.StatusOK, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"), nil
			}
			return responseWithBody(http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`), nil
		default:
			return responseWithBody(http.StatusNotFound, ""), nil
		}
	})}

	options := []ClientOption{
		WithHTTPClient(httpClient),
		WithLiveMetricsOverlay(true),
		WithPollInterval(time.Millisecond),
	}
	req := &ChatRequest{Model: "test-model", Messages: []Message{{Role: "user", Content: "hello"}}}
	if _, err := Infer(context.Background(), req, options...); err != nil {
		t.Fatalf("Infer failed: %v", err)
	}
	if err := InferStreamChunks(context.Background(), req, func(InferenceStreamChunk) error { return nil }, options...); err != nil {
		t.Fatalf("InferStreamChunks failed: %v", err)
	}
	var output bytes.Buffer
	if err := InferStream(context.Background(), req, &output, options...); err != nil {
		t.Fatalf("InferStream failed: %v", err)
	}
	if output.String() != "ok" {
		t.Fatalf("unexpected streamed content: %q", output.String())
	}
	if loadingStreams.Load() != 3 || slotPolls.Load() == 0 {
		t.Fatalf("monitoring did not run for every mode: loading=%d slots=%d", loadingStreams.Load(), slotPolls.Load())
	}
}

func responseWithBody(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
