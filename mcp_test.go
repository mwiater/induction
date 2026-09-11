package induction

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type mcpFixture struct {
	t       *testing.T
	server  *httptest.Server
	mu      sync.Mutex
	methods []string
	calls   int
	tool    mcpTool
}

func newMCPFixture(t *testing.T, name string, readOnly bool) *mcpFixture {
	f := &mcpFixture{t: t, tool: mcpTool{Name: name, Description: "lookup", InputSchema: json.RawMessage(`{"type":"object","properties":{"key":{"type":"string"}}}`)}}
	f.tool.Annotations.ReadOnlyHint = readOnly
	f.server = httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.server.Close)
	return f
}

func (f *mcpFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		f.t.Errorf("decode MCP request: %v", err)
		return
	}
	f.mu.Lock()
	f.methods = append(f.methods, request.Method)
	f.mu.Unlock()
	if request.Method == "notifications/initialized" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	var result any
	switch request.Method {
	case "initialize":
		w.Header().Set("Mcp-Session-Id", "fixture-session")
		result = map[string]any{"protocolVersion": mcpProtocolVersion, "capabilities": map[string]any{}}
	case "tools/list":
		result = map[string]any{"tools": []mcpTool{f.tool}}
	case "tools/call":
		f.mu.Lock()
		f.calls++
		f.mu.Unlock()
		result = map[string]any{"content": []map[string]any{{"type": "text", "text": "fixture result"}}}
	default:
		f.t.Errorf("unexpected MCP method %q", request.Method)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
}

func TestDiscoverMCPToolsSkipsDisabledAndInitializesAllowed(t *testing.T) {
	fixture := newMCPFixture(t, "lookup", true)
	cfg := &Config{MCPServers: []MCPServerConfig{
		{Allow: false, Name: "BLOCKED", URL: "http://127.0.0.1:1/mcp"},
		{Allow: true, Name: "FEXR", URL: fixture.server.URL},
	}}
	tools, err := discoverMCPTools(context.Background(), cfg, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].server.Name != "FEXR" || tools[0].tool.Name != "lookup" {
		t.Fatalf("unexpected discovered tools: %#v", tools)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if strings.Join(fixture.methods, ",") != "initialize,notifications/initialized,tools/list" {
		t.Fatalf("wrong protocol order: %v", fixture.methods)
	}
}

func TestRunMCPToolLoopConvertsCallsAndReturnsFinalResponse(t *testing.T) {
	fixture := newMCPFixture(t, "lookup", true)
	client := &mcpClient{url: fixture.server.URL, httpClient: fixture.server.Client(), sessionID: "fixture-session"}
	inferenceCalls := 0
	httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		inferenceCalls++
		var request ChatRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		body := `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{\"key\":\"x\"}"}}]}}]}`
		if inferenceCalls == 2 {
			last := request.Messages[len(request.Messages)-1]
			if len(request.Tools) != 1 || last.Role != "tool" || last.ToolCallID != "call-1" || last.Content != "fixture result" {
				t.Fatalf("bad continuation request: %#v", request)
			}
			body = `{"id":"final","choices":[{"message":{"role":"assistant","content":"done"}}]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(body))}, nil
	})}
	response, err := runMCPToolLoop(context.Background(), &ChatRequest{Model: "model", Messages: []Message{{Role: "user", Content: "question"}}}, []boundMCPTool{{server: MCPServerConfig{Name: "FEXR"}, client: client, tool: fixture.tool}}, time.Second, nil, WithHTTPClient(httpClient), WithLiveMetricsOverlay(false))
	if err != nil || response.ID != "final" || inferenceCalls != 2 {
		t.Fatalf("response=%#v calls=%d err=%v", response, inferenceCalls, err)
	}
}

func TestRunMCPToolLoopDeniesSideEffectsWithoutApproval(t *testing.T) {
	fixture := newMCPFixture(t, "write", false)
	httpClient := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		body := `{"choices":[{"message":{"tool_calls":[{"id":"call-1","function":{"name":"write","arguments":"{}"}}]}}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	_, err := runMCPToolLoop(context.Background(), &ChatRequest{Model: "model", Messages: []Message{{Role: "user", Content: "write"}}}, []boundMCPTool{{server: MCPServerConfig{Name: "FEXR"}, client: &mcpClient{}, tool: fixture.tool}}, time.Second, nil, WithHTTPClient(httpClient), WithLiveMetricsOverlay(false))
	if err == nil || !strings.Contains(err.Error(), "was not approved") {
		t.Fatalf("unexpected error: %v", err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.calls != 0 {
		t.Fatal("side-effecting tool was called")
	}
}

func TestRunMCPStreamToolLoopReconstructsToolCalls(t *testing.T) {
	fixture := newMCPFixture(t, "lookup", true)
	client := &mcpClient{url: fixture.server.URL, httpClient: fixture.server.Client(), sessionID: "fixture-session"}
	streamCalls := 0
	httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		streamCalls++
		var request ChatRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		var chunks []InferenceStreamChunk
		if streamCalls == 1 {
			chunks = []InferenceStreamChunk{
				{Choices: []InferenceStreamChoice{{Index: 0, Delta: InferenceStreamDelta{Role: "assistant", ToolCalls: []InferenceStreamToolCall{{Index: 0, ID: "call-1", Type: "function", Function: InferenceFunctionCall{Name: "look", Arguments: `{"key":`}}}}}}},
				{Choices: []InferenceStreamChoice{{Index: 0, Delta: InferenceStreamDelta{ToolCalls: []InferenceStreamToolCall{{Index: 0, Function: InferenceFunctionCall{Name: "up", Arguments: `"x"`}}}}}}},
				{Choices: []InferenceStreamChoice{{Index: 0, Delta: InferenceStreamDelta{ToolCalls: []InferenceStreamToolCall{{Index: 0, Function: InferenceFunctionCall{Arguments: `}`}}}}}}},
			}
		} else {
			last := request.Messages[len(request.Messages)-1]
			if last.Role != "tool" || last.ToolCallID != "call-1" || last.Content != "fixture result" {
				t.Fatalf("bad streamed continuation: %#v", request)
			}
			chunks = []InferenceStreamChunk{{Choices: []InferenceStreamChoice{{Index: 0, Delta: InferenceStreamDelta{Role: "assistant", Content: "final "}}}}, {Choices: []InferenceStreamChoice{{Index: 0, Delta: InferenceStreamDelta{Content: "answer"}}}}}
		}
		var body bytes.Buffer
		for _, chunk := range chunks {
			encoded, _ := json.Marshal(chunk)
			body.WriteString("data: ")
			body.Write(encoded)
			body.WriteString("\n\n")
		}
		body.WriteString("data: [DONE]\n\n")
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(bytes.NewReader(body.Bytes()))}, nil
	})}
	var out bytes.Buffer
	err := runMCPStreamToolLoop(context.Background(), &ChatRequest{Model: "model", Messages: []Message{{Role: "user", Content: "question"}}}, &out, []boundMCPTool{{server: MCPServerConfig{Name: "FEXR"}, client: client, tool: fixture.tool}}, time.Second, nil, WithHTTPClient(httpClient), WithLiveMetricsOverlay(false))
	if err != nil || streamCalls != 2 || out.String() != "final answer" {
		t.Fatalf("output=%q calls=%d err=%v", out.String(), streamCalls, err)
	}
}

func TestMCPStreamAccumulatorRejectsConflictingIDs(t *testing.T) {
	acc := new(mcpStreamAccumulator)
	first := InferenceStreamChunk{Choices: []InferenceStreamChoice{{Index: 0, Delta: InferenceStreamDelta{ToolCalls: []InferenceStreamToolCall{{Index: 0, ID: "one"}}}}}}
	second := InferenceStreamChunk{Choices: []InferenceStreamChoice{{Index: 0, Delta: InferenceStreamDelta{ToolCalls: []InferenceStreamToolCall{{Index: 0, ID: "two"}}}}}}
	if err := acc.add(first); err != nil {
		t.Fatal(err)
	}
	if err := acc.add(second); err == nil {
		t.Fatal("expected conflicting streamed tool-call ID error")
	}
}

func TestSummarizeMCPResultReportsContentTypes(t *testing.T) {
	result := mcpCallResult{
		Content:           []mcpContent{{Type: "text"}, {Type: "image"}, {Type: "text"}},
		StructuredContent: json.RawMessage(`{"answer":42}`),
	}
	if got, want := summarizeMCPResult(result), "image×1 + text×2 + structured"; got != want {
		t.Fatalf("summary=%q want %q", got, want)
	}
	if got := summarizeMCPResult(mcpCallResult{}); got != "empty result" {
		t.Fatalf("empty summary=%q", got)
	}
}

func TestCompactMCPJSON(t *testing.T) {
	if got := compactMCPJSON([]byte(`{"answer": 42}`)); got != `{"answer":42}` {
		t.Fatalf("compact JSON = %q", got)
	}
	if got := compactMCPJSON([]byte("not json")); got != "not json" {
		t.Fatalf("invalid JSON fallback = %q", got)
	}
}
