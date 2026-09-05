package induction

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestLoadConfig verifies YAML parsing, singleton behavior, and configured client construction.
func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "induction.yaml")
	contents := []byte(`
server: http://localhost:9998
timeout: 20m
poll_interval: 2s
load_wait_interval: 1s
enableLiveMetricsOverlay: true
sidebarWidth: 40
MCPServers:
  - MCPServerAllow: true
    MCPServerName: FEXR
    MCPServerURL: http://192.168.0.239:4002/mcp
  - MCPServerAllow: false
    MCPServerName: DISABLED
    MCPServerURL: https://example.test/mcp
log:
  path: application.log
  console: false
  prefix: "test: "
  microseconds: true
`)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	first, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	second, err := LoadConfig("ignored-after-first-load.yaml")
	if err != nil {
		t.Fatalf("second LoadConfig failed: %v", err)
	}
	if first != second {
		t.Fatal("expected the singleton config instance")
	}
	if first.Server != "http://localhost:9998" {
		t.Fatalf("unexpected config: %#v", first)
	}
	if time.Duration(first.Timeout) != 20*time.Minute {
		t.Fatalf("unexpected timeout: %s", time.Duration(first.Timeout))
	}
	if !first.EnableLiveMetricsOverlay {
		t.Fatal("expected live metrics overlay to be enabled")
	}
	if first.SidebarWidth != 40 {
		t.Fatalf("unexpected sidebar width: %d", first.SidebarWidth)
	}
	if len(first.MCPServers) != 2 || !first.MCPServers[0].Allow || first.MCPServers[0].Name != "FEXR" || first.MCPServers[0].URL != "http://192.168.0.239:4002/mcp" || first.MCPServers[1].Allow {
		t.Fatalf("unexpected MCP server configuration: %#v", first.MCPServers)
	}

	client, err := NewClientFromConfig(context.Background())
	if err != nil {
		t.Fatalf("NewClientFromConfig failed: %v", err)
	}
	if client.endpoint != first.Server {
		t.Fatalf("expected endpoint %q, got %q", first.Server, client.endpoint)
	}
	if !client.opts.enableLiveMetricsOverlay {
		t.Fatal("expected overlay setting to propagate to the client")
	}
}

func TestConfigValidatesMCPServers(t *testing.T) {
	valid := func() Config {
		return Config{Server: "http://localhost:9998", Timeout: Duration(time.Second), PollInterval: Duration(time.Second), LoadWaitInterval: Duration(time.Second), Log: LogConfig{Path: "app.log"}}
	}
	tests := []struct {
		name    string
		servers []MCPServerConfig
	}{
		{"missing name", []MCPServerConfig{{Allow: true, URL: "http://localhost/mcp"}}},
		{"missing URL", []MCPServerConfig{{Allow: true, Name: "one"}}},
		{"non HTTP URL", []MCPServerConfig{{Allow: true, Name: "one", URL: "file:///tmp/mcp"}}},
		{"duplicate name", []MCPServerConfig{{Name: "one", URL: "http://one/mcp"}, {Name: "one", URL: "http://two/mcp"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid()
			cfg.MCPServers = tt.servers
			if err := cfg.validate(); err == nil {
				t.Fatal("expected MCP configuration validation error")
			}
		})
	}
	cfg := valid()
	cfg.MCPServers = []MCPServerConfig{{Allow: true, Name: "FEXR", URL: "http://192.168.0.239:4002/mcp"}}
	if err := cfg.validate(); err != nil {
		t.Fatalf("valid MCP configuration rejected: %v", err)
	}
}

func TestModelManagerNormalizeAndValidate(t *testing.T) {
	modelsPath := filepath.Join(t.TempDir(), "models")
	cfg := ModelManagerConfig{
		PreferredProviders: []string{" Acme ", "acme", "", "Other"},
		ModelsPath:         modelsPath,
		AvailableRAM:       "8GiB",
		AvailableVRAM:      "2GB",
	}
	if err := cfg.NormalizeAndValidate(); err != nil {
		t.Fatalf("NormalizeAndValidate failed: %v", err)
	}
	if cfg.SearchResults != 10 || cfg.ModelsPath != modelsPath || len(cfg.PreferredProviders) != 2 || cfg.PreferredProviders[0] != "Acme" || cfg.PreferredProviders[1] != "Other" {
		t.Fatalf("unexpected normalized config: %#v", cfg)
	}
	if _, err := os.Stat(modelsPath); err != nil {
		t.Fatalf("models path was not created: %v", err)
	}

	cases := []ModelManagerConfig{
		{ModelsPath: modelsPath, SearchResults: 101},
		{ModelsPath: modelsPath, IncludePatterns: []string{"[bad"}},
		{ModelsPath: modelsPath, AvailableRAM: "12"},
		{ModelsPath: "", SearchResults: 1},
	}
	for i := range cases {
		if err := cases[i].NormalizeAndValidate(); err == nil {
			t.Errorf("case %d unexpectedly validated", i)
		}
	}
}

func TestParseMemorySize(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  int64
	}{
		{"1KB", 1000}, {"2MiB", 2 << 20}, {"1.5GB", 1500000000},
	} {
		got, err := parseMemorySize(tc.input)
		if err != nil || got != tc.want {
			t.Errorf("parseMemorySize(%q) = %d, %v; want %d", tc.input, got, err, tc.want)
		}
	}
	for _, input := range []string{"", "12", "-1GB", "nopeGB"} {
		if _, err := parseMemorySize(input); err == nil {
			t.Errorf("parseMemorySize(%q) unexpectedly succeeded", input)
		}
	}
}

func TestInferReturnsTypedOpenAIResponse(t *testing.T) {
	var requestPath string
	var requestModel string
	var sawDeadline atomic.Bool
	httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestPath = req.URL.Path
		if deadline, ok := req.Context().Deadline(); ok && time.Until(deadline) > 0 && time.Until(deadline) <= 20*time.Minute {
			sawDeadline.Store(true)
		}
		var request ChatRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Fatalf("decode inference request: %v", err)
		}
		requestModel = request.Model
		body := `{"id":"chatcmpl-1","object":"chat.completion","created":123,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"thinking","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		}, nil
	})}

	response, err := Infer(context.Background(), &ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}, WithHTTPClient(httpClient), WithLiveMetricsOverlay(false))
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}
	if requestPath != "/v1/chat/completions" || requestModel != "test-model" || !sawDeadline.Load() {
		t.Fatalf("request was not configured: path=%q model=%q deadline=%v", requestPath, requestModel, sawDeadline.Load())
	}
	if response.ID != "chatcmpl-1" || len(response.Choices) != 1 || response.Choices[0].Message == nil || response.Choices[0].Message.Content != "hello" {
		t.Fatalf("unexpected typed response: %#v", response)
	}
	if response.Choices[0].Message.ReasoningContent != "thinking" {
		t.Fatalf("reasoning content was not preserved: %#v", response.Choices[0].Message)
	}
	if response.Usage == nil || response.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage: %#v", response.Usage)
	}
}

func TestInferUsesCompletionsEndpointForPrompt(t *testing.T) {
	var requestPath string
	httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestPath = req.URL.Path
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"choices":[{"index":0,"text":"hello"}]}`)),
			Header:     make(http.Header),
		}, nil
	})}

	response, err := Infer(context.Background(), &ChatRequest{Model: "test-model", Prompt: "hello"}, WithHTTPClient(httpClient), WithLiveMetricsOverlay(false))
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}
	if requestPath != "/v1/completions" || len(response.Choices) != 1 || response.Choices[0].Text != "hello" {
		t.Fatalf("unexpected completion response: path=%q response=%#v", requestPath, response)
	}
}

func TestInferRejectsNilRequest(t *testing.T) {
	if _, err := Infer(context.Background(), nil); err == nil {
		t.Fatal("expected nil request error")
	}
}

func TestInferStreamChunksAndContent(t *testing.T) {
	var requestPath string
	var requestModel string
	var streamEnabled bool
	httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestPath = req.URL.Path
		var request ChatRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Fatalf("decode inference request: %v", err)
		}
		requestModel = request.Model
		streamEnabled = request.Stream != nil && *request.Stream
		body := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"I \"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n" +
			"data: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		}, nil
	})}

	var chunks []InferenceStreamChunk
	err := InferStreamChunks(context.Background(), &ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}, func(chunk InferenceStreamChunk) error {
		chunks = append(chunks, chunk)
		return nil
	}, WithHTTPClient(httpClient), WithLiveMetricsOverlay(false))
	if err != nil {
		t.Fatalf("InferStreamChunks failed: %v", err)
	}
	if len(chunks) != 4 || chunks[0].Choices[0].Delta.ReasoningContent != "I " || chunks[1].Choices[0].Delta.ReasoningContent != "think" || chunks[2].Choices[0].Delta.Content != "hello" || chunks[3].Choices[0].Delta.Content != " world" {
		t.Fatalf("unexpected typed chunks: %#v", chunks)
	}
	if requestPath != "/v1/chat/completions" || requestModel != "test-model" || !streamEnabled {
		t.Fatalf("stream request was not configured: path=%q model=%q stream=%v", requestPath, requestModel, streamEnabled)
	}

	var output bytes.Buffer
	err = InferStream(context.Background(), &ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}, &output, WithHTTPClient(httpClient), WithLiveMetricsOverlay(false))
	if err != nil {
		t.Fatalf("InferStream failed: %v", err)
	}
	if output.String() != "<think>\nI think\n</think>\n\nhello world" {
		t.Fatalf("expected reasoning block and generated content, got %q", output.String())
	}
}

func TestInferStreamChunksRejectsNilHandler(t *testing.T) {
	err := InferStreamChunks(context.Background(), &ChatRequest{}, nil)
	if err == nil {
		t.Fatal("expected nil chunk handler error")
	}
}

func TestInferStreamRejectsNilOutput(t *testing.T) {
	err := InferStream(context.Background(), &ChatRequest{}, nil)
	if err == nil {
		t.Fatal("expected nil output error")
	}
}

func TestInferStreamClosesReasoningOnlyBlock(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		body := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking\"}}]}\n\n" +
			"data: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		}, nil
	})}
	var output bytes.Buffer
	err := InferStream(context.Background(), &ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}, &output, WithHTTPClient(httpClient), WithLiveMetricsOverlay(false))
	if err != nil {
		t.Fatalf("InferStream failed: %v", err)
	}
	if output.String() != "<think>\nthinking\n</think>" {
		t.Fatalf("reasoning block was not closed: %q", output.String())
	}
}
