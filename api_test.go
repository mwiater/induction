package induction

import (
	"bytes"
	"context"
	"io"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// captureLogger returns an application logger and its in-memory destination.
func captureLogger() (*log.Logger, *bytes.Buffer) {
	var output bytes.Buffer
	return log.New(&output, "", 0), &output
}

// newTestClient returns a client configured for deterministic unit tests.
func newTestClient(endpoint string) *Client {
	return &Client{
		endpoint: endpoint,
		opts: &ClientOptions{
			httpClient:       &http.Client{Timeout: 2 * time.Minute},
			pollInterval:     10 * time.Millisecond,
			loadWaitInterval: 10 * time.Millisecond,
		},
	}
}

// captureStdout runs fn and returns everything it prints to standard output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	_ = w.Close()
	output, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}

	return string(output)
}

// TestAPI_DoInference exercises the chat-completions inference path.
func TestAPI_DoInference(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices": [{"message": {"content": "success"}}]}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	req := &ChatRequest{Model: "test-model", Messages: []Message{{Role: "user", Content: "hello"}}}

	interaction, err := client.doInference(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if interaction.Content != "success" {
		t.Fatalf("expected content 'success', got %q", interaction.Content)
	}
	if interaction.Response == "" {
		t.Fatal("expected raw response, got empty string")
	}
}

// TestAPI_DoInference_CompletionPath exercises the plain completion endpoint.
func TestAPI_DoInference_CompletionPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/completion" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices": [{"text": "completion-success"}]}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	req := &ChatRequest{Model: "test-model"}

	interaction, err := client.doInference(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if interaction.Content != "completion-success" {
		t.Fatalf("expected completion text, got %q", interaction.Content)
	}
}

// TestAPI_DoInference_MarshalError exercises the request marshalling error branch.
func TestAPI_DoInference_MarshalError(t *testing.T) {
	client := newTestClient("http://example.com")
	temperature := math.NaN()
	req := &ChatRequest{Model: "test-model", Temperature: &temperature}
	if _, err := client.doInference(context.Background(), req); err == nil {
		t.Fatal("expected marshal error for NaN temperature")
	}
}

// TestAPI_DoInference_RequestError exercises the request construction error branch.
func TestAPI_DoInference_RequestError(t *testing.T) {
	client := newTestClient("http://[::1")
	req := &ChatRequest{Model: "test-model"}
	if _, err := client.doInference(context.Background(), req); err == nil {
		t.Fatal("expected request creation to fail")
	}
}

// TestAPI_FetchProps exercises the /props success path.
func TestAPI_FetchProps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/props" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("model"); got != "test-model" {
			t.Fatalf("missing or incorrect model query param: %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"total_slots": 4, "default_generation_settings": {"n_ctx": 4096}}`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	props, err := client.fetchProps(context.Background(), "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props.TotalSlots != 4 {
		t.Fatalf("expected 4 total slots, got %d", props.TotalSlots)
	}
	if props.DefaultGenerationSettings["n_ctx"] != float64(4096) {
		t.Fatalf("expected n_ctx 4096, got %v", props.DefaultGenerationSettings["n_ctx"])
	}
}

// TestAPI_FetchProps_RequestError exercises the request construction error branch.
func TestAPI_FetchProps_RequestError(t *testing.T) {
	client := newTestClient("http://[::1")
	if _, err := client.fetchProps(context.Background(), "test-model"); err == nil {
		t.Fatal("expected fetchProps to fail on bad endpoint")
	}
}

// TestAPI_FetchProps_InvalidJSON exercises the props decoding error branch.
func TestAPI_FetchProps_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	if _, err := client.fetchProps(context.Background(), "test-model"); err == nil {
		t.Fatal("expected fetchProps to fail on invalid JSON")
	}
}

// TestAPI_FetchProps_HTTPError exercises the /props error path.
func TestAPI_FetchProps_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	if _, err := client.fetchProps(context.Background(), "test-model"); err == nil {
		t.Fatal("expected fetchProps to fail on 500 response")
	}
}

// TestAPI_FetchSlots exercises the /slots success path.
func TestAPI_FetchSlots(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/slots" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id": 0, "n_ctx": 4096, "is_processing": true}]`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	slots, err := client.fetchSlots(context.Background(), "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(slots) != 1 {
		t.Fatalf("expected 1 slot, got %d", len(slots))
	}
	if slots[0]["n_ctx"] != float64(4096) {
		t.Fatalf("expected n_ctx 4096, got %v", slots[0]["n_ctx"])
	}
}

// TestAPI_FetchSlots_RequestError exercises the request construction error branch.
func TestAPI_FetchSlots_RequestError(t *testing.T) {
	client := newTestClient("http://[::1")
	if _, err := client.fetchSlots(context.Background(), "test-model"); err == nil {
		t.Fatal("expected fetchSlots to fail on bad endpoint")
	}
}

// TestAPI_FetchSlots_InvalidJSON exercises the slots decoding error branch.
func TestAPI_FetchSlots_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	if _, err := client.fetchSlots(context.Background(), "test-model"); err == nil {
		t.Fatal("expected fetchSlots to fail on invalid JSON")
	}
}

// TestAPI_FetchSlots_HTTPError exercises the /slots error path.
func TestAPI_FetchSlots_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	if _, err := client.fetchSlots(context.Background(), "test-model"); err == nil {
		t.Fatal("expected fetchSlots to fail on 500 response")
	}
}

// TestAPI_FetchMetrics exercises the /metrics success path.
func TestAPI_FetchMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("# HELP llamacpp:prompt_tokens_total\nllamacpp:prompt_tokens_total 12\n"))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	metrics, err := client.fetchMetrics(context.Background(), "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(metrics.Raw, "llamacpp:prompt_tokens_total") {
		t.Fatalf("unexpected raw metrics output: %q", metrics.Raw)
	}
	if metrics.Entries["llamacpp:prompt_tokens_total"] != float64(12) {
		t.Fatalf("expected parsed metric value 12, got %v", metrics.Entries["llamacpp:prompt_tokens_total"])
	}
}

// TestAPI_FetchMetrics_RequestError exercises the request construction error branch.
func TestAPI_FetchMetrics_RequestError(t *testing.T) {
	client := newTestClient("http://[::1")
	if _, err := client.fetchMetrics(context.Background(), "test-model"); err == nil {
		t.Fatal("expected fetchMetrics to fail on bad endpoint")
	}
}

// TestAPI_FetchMetrics_HTTPError exercises the /metrics error path.
func TestAPI_FetchMetrics_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	if _, err := client.fetchMetrics(context.Background(), "test-model"); err == nil {
		t.Fatal("expected fetchMetrics to fail on 500 response")
	}
}

// TestListModels_PrintsTable exercises the envelope-shaped /v1/models response.
func TestListModels_PrintsTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": [
				{
					"id": "model-a",
					"loaded": true,
					"temperature": 0.7,
					"repeat-last-n": 64,
					"repeat-penalty": 1.1,
					"top-k": 40,
					"top-p": 0.95,
					"architecture": {
						"input_modalities": ["text", "image"],
						"output_modalities": ["text"]
					}
				},
				{
					"id": "model-b",
					"status": {"value": "loaded"},
					"parameters": {"temperature": 0.2}
				}
			]
		}`))
	}))
	defer srv.Close()

	logger, logOutput := captureLogger()
	if err := ListModels(srv.URL, WithLogger(logger)); err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	output := logOutput.String()

	headerLine := strings.SplitN(output, "\n", 2)[0]
	expectedHeader := []string{"MODEL", "STATUS", "CTX", "BATCH", "UBATCH", "PARALLEL", "CACHE-K", "CACHE-V", "FLASH-ATTN", "TEMPERATURE", "TOP-K", "TOP-P", "REPEAT-LAST-N", "REPEAT-PENALTY", "INPUT MODALITIES", "OUTPUT MODALITIES"}
	pos := 0
	for _, want := range expectedHeader {
		idx := strings.Index(headerLine[pos:], want)
		if idx < 0 {
			t.Fatalf("expected header to contain %q in order, got %q", want, headerLine)
		}
		pos += idx + len(want)
	}

	for _, want := range []string{"model-a", "model-b", "0.7", "40", "0.95", "64", "1.1", "text,image", "text"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, output)
		}
	}
	if !strings.Contains(output, "LOADED") {
		t.Fatalf("expected loaded model marker in output, got:\n%s", output)
	}
}

// TestListModels_PrintsTableFromRawArray exercises the raw-array /v1/models shape.
func TestListModels_PrintsTableFromRawArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{
				"id": "array-model",
				"args": {"top_p": 0.75}
			}
		]`))
	}))
	defer srv.Close()

	logger, logOutput := captureLogger()
	if err := ListModels(srv.URL, WithLogger(logger)); err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	output := logOutput.String()

	if !strings.Contains(output, "array-model") {
		t.Fatalf("expected array model in output, got:\n%s", output)
	}
	if !strings.Contains(output, "0.75") {
		t.Fatalf("expected parsed top_p value in output, got:\n%s", output)
	}
}

// TestListModels_HTTPError exercises the non-200 response branch.
func TestListModels_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := ListModels(srv.URL); err == nil {
		t.Fatal("expected ListModels to fail on 500 response")
	}
}

// TestRenderModelTable_NoRows ensures the header still renders when no rows exist.
func TestRenderModelTable_NoRows(t *testing.T) {
	output := captureStdout(t, func() {
		renderModelTable(os.Stdout, nil)
	})
	if !strings.Contains(output, "MODEL") {
		t.Fatalf("expected header in output, got %q", output)
	}
}

// failingBody is an io.ReadCloser that always fails when read.
type failingBody struct{}

// Read returns a read error immediately so read-all error paths can be exercised.
func (failingBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// Close is a no-op for the failing test body.
func (failingBody) Close() error { return nil }

// TestAPI_DoInference_ReadError exercises the response-body read error branch.
func TestAPI_DoInference_ReadError(t *testing.T) {
	client := newTestClient("http://example.com")
	client.opts.httpClient = &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}}, nil
		}),
	}
	_, err := client.doInference(context.Background(), &ChatRequest{Model: "test-model"})
	if err == nil {
		t.Fatal("expected response read error")
	}
}

// TestAPI_FetchProps_ReadError exercises the response-body read error branch.
func TestAPI_FetchProps_ReadError(t *testing.T) {
	client := newTestClient("http://example.com")
	client.opts.httpClient = &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}}, nil
		}),
	}
	_, err := client.fetchProps(context.Background(), "test-model")
	if err == nil {
		t.Fatal("expected response read error")
	}
}

// TestAPI_FetchSlots_ReadError exercises the response-body decode error branch.
func TestAPI_FetchSlots_ReadError(t *testing.T) {
	client := newTestClient("http://example.com")
	client.opts.httpClient = &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}}, nil
		}),
	}
	_, err := client.fetchSlots(context.Background(), "test-model")
	if err == nil {
		t.Fatal("expected response decode error")
	}
}

// TestAPI_FetchMetrics_ReadError exercises the response-body read error branch.
func TestAPI_FetchMetrics_ReadError(t *testing.T) {
	client := newTestClient("http://example.com")
	client.opts.httpClient = &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}}, nil
		}),
	}
	_, err := client.fetchMetrics(context.Background(), "test-model")
	if err == nil {
		t.Fatal("expected response read error")
	}
}

// roundTripperFunc turns a function into an http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper.
func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// TestAPI_DoInference_InvalidJSON exercises the non-JSON response branch.
func TestAPI_DoInference_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	interaction, err := client.doInference(context.Background(), &ChatRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if interaction.Content != "" {
		t.Fatalf("expected empty content for invalid JSON, got %q", interaction.Content)
	}
}

// TestAPI_DoInference_DoError exercises the HTTP client error branch.
func TestAPI_DoInference_DoError(t *testing.T) {
	client := newTestClient("http://example.com")
	client.opts.httpClient = &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, io.ErrUnexpectedEOF
		}),
	}
	if _, err := client.doInference(context.Background(), &ChatRequest{Model: "test-model"}); err == nil {
		t.Fatal("expected inference request error")
	}
}

// TestAPI_FetchProps_DoError exercises the HTTP client error branch.
func TestAPI_FetchProps_DoError(t *testing.T) {
	client := newTestClient("http://example.com")
	client.opts.httpClient = &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, io.ErrUnexpectedEOF
		}),
	}
	if _, err := client.fetchProps(context.Background(), "test-model"); err == nil {
		t.Fatal("expected props request error")
	}
}

// TestAPI_FetchSlots_DoError exercises the HTTP client error branch.
func TestAPI_FetchSlots_DoError(t *testing.T) {
	client := newTestClient("http://example.com")
	client.opts.httpClient = &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, io.ErrUnexpectedEOF
		}),
	}
	if _, err := client.fetchSlots(context.Background(), "test-model"); err == nil {
		t.Fatal("expected slots request error")
	}
}

// TestAPI_FetchMetrics_DoError exercises the HTTP client error branch.
func TestAPI_FetchMetrics_DoError(t *testing.T) {
	client := newTestClient("http://example.com")
	client.opts.httpClient = &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, io.ErrUnexpectedEOF
		}),
	}
	if _, err := client.fetchMetrics(context.Background(), "test-model"); err == nil {
		t.Fatal("expected metrics request error")
	}
}

// TestListModels_RequestError exercises the request construction error branch.
func TestListModels_RequestError(t *testing.T) {
	if err := ListModels("http://[::1"); err == nil {
		t.Fatal("expected ListModels to fail on bad endpoint")
	}
}

// TestListModels_ReadError exercises the response-body read error branch.
func TestListModels_ReadError(t *testing.T) {
	client := newTestClient("http://example.com")
	client.opts.httpClient = &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}}, nil
		}),
	}
	if err := client.ListModels(); err == nil {
		t.Fatal("expected ListModels to fail on unreadable body")
	}
}

// TestListModels_ParsesArgsArray exercises the flat args-array response shape.
func TestListModels_ParsesArgsArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": [
				{
					"id": "Granite-4.1-30B-Q4_K_M",
					"loaded": true,
					"args": [
						"C:\\ai\\bin\\llama.cpp\\llama-server.exe",
						"--repeat-last-n", "256",
						"--repeat-penalty", "1.02",
						"--temperature", "0.0",
						"--top-k", "1",
						"--top-p", "1.0",
						"--mlock"
					],
					"architecture": {
						"input_modalities": ["text", "image"],
						"output_modalities": ["text"]
					}
				}
			]
		}`))
	}))
	defer srv.Close()

	logger, logOutput := captureLogger()
	if err := ListModels(srv.URL, WithLogger(logger)); err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	output := logOutput.String()

	for _, want := range []string{"Granite-4.1-30B-Q4_K_M", "0.0", "256", "1.02", "4", "1", "1.0", "text,image", "text"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, output)
		}
	}
	if !strings.Contains(output, "LOADED") {
		t.Fatalf("expected loaded model marker in output, got:\n%s", output)
	}
}

// TestListModels_ParsesNestedStatusArgs exercises the sample endpoint shape.
func TestListModels_ParsesNestedStatusArgs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": [
				{
					"id": "Agents-A1-35B-Q4_K_M",
					"status": {
						"value": "unloaded",
						"args": [
							"C:\\ai\\bin\\llama.cpp\\llama-server.exe",
							"--repeat-last-n", "256",
							"--repeat-penalty", "1.0",
							"--temperature", "0.85",
							"--top-k", "20",
							"--top-p", "0.95"
						]
					},
					"architecture": {
						"input_modalities": ["text", "image"],
						"output_modalities": ["text"]
					}
				}
			]
		}`))
	}))
	defer srv.Close()

	logger, logOutput := captureLogger()
	if err := ListModels(srv.URL, WithLogger(logger)); err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	output := logOutput.String()

	for _, want := range []string{"0.85", "256", "1.0", "4", "20", "0.95", "text,image", "text"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, output)
		}
	}
}
