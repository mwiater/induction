package induction

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestListLoadedModels_PrintsOnlyLoadedRows verifies the loaded-model filter helper.
func TestListLoadedModels_PrintsOnlyLoadedRows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": [
				{"id": "loaded-model", "loaded": true},
				{"id": "idle-model", "loaded": false}
			]
		}`))
	}))
	defer srv.Close()

	logger, logOutput := captureLogger()
	if err := ListLoadedModels(srv.URL, WithLogger(logger)); err != nil {
		t.Fatalf("ListLoadedModels failed: %v", err)
	}
	output := logOutput.String()

	if !strings.Contains(output, "loaded-model") {
		t.Fatalf("expected loaded model in output, got:\n%s", output)
	}
	if strings.Contains(output, "idle-model") {
		t.Fatalf("expected unloaded model to be filtered out, got:\n%s", output)
	}
	if !strings.Contains(output, "LOADED") {
		t.Fatalf("expected loaded column marker in output, got:\n%s", output)
	}
}

// TestCheckHealth verifies the health probe succeeds on the primary endpoint.
func TestCheckHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := CheckHealth(srv.URL); err != nil {
		t.Fatalf("CheckHealth failed: %v", err)
	}
}

// TestCheckHealth_FallsBackToV1Health verifies the fallback probe path.
func TestCheckHealth_FallsBackToV1Health(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusNotFound)
		case "/v1/health":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	if err := CheckHealth(srv.URL); err != nil {
		t.Fatalf("CheckHealth fallback failed: %v", err)
	}
}

// TestGenerateSnapshot_Wrapper verifies the convenience snapshot helper.
func TestGenerateSnapshot_Wrapper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices": [{"message": {"content": "snapshot-success"}}]}`))
		case "/props":
			_, _ = w.Write([]byte(`{"total_slots": 1, "default_generation_settings": {"n_ctx": 4096}}`))
		case "/slots":
			_, _ = w.Write([]byte(`[{}]`))
		case "/metrics":
			_, _ = w.Write([]byte("llamacpp:prompt_tokens_total 12\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	snapshot, err := GenerateSnapshot(context.Background(), srv.URL, &ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("GenerateSnapshot failed: %v", err)
	}
	if snapshot == nil || snapshot.ModelID != "test-model" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if snapshot.LoadTime < 0 {
		t.Fatalf("unexpected load time: %v", snapshot.LoadTime)
	}
	if len(snapshot.Interaction) != 1 || snapshot.Interaction[0].Content != "snapshot-success" {
		t.Fatalf("unexpected interaction: %#v", snapshot.Interaction)
	}
	if snapshot.Props == nil || snapshot.Props.TotalSlots != 1 {
		t.Fatalf("unexpected props: %#v", snapshot.Props)
	}
}

// TestChatAndComplete_Wrappers verify the convenience request helpers.
func TestChatAndComplete_Wrappers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices": [{"message": {"content": "chat-success"}}]}`))
		case "/completion":
			_, _ = w.Write([]byte(`{"choices": [{"text": "complete-success"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	chat, err := Chat(context.Background(), srv.URL, &ChatRequest{Model: "test-model", Messages: []Message{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if chat == nil || chat.Content != "chat-success" {
		t.Fatalf("unexpected chat response: %#v", chat)
	}

	complete, err := Complete(context.Background(), srv.URL, &ChatRequest{Model: "test-model", Prompt: "hello"})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if complete == nil || complete.Content != "complete-success" {
		t.Fatalf("unexpected completion response: %#v", complete)
	}
}

// TestStreamChatAndStreamComplete_Wrappers verify the streaming request helpers.
func TestStreamChatAndStreamComplete_Wrappers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"hel"}}]}

`))
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"lo"}}]}

`))
			_, _ = w.Write([]byte(`data: [DONE]

`))
		case "/completion":
			_, _ = w.Write([]byte(`data: {"choices":[{"text":"hel"}]}

`))
			_, _ = w.Write([]byte(`data: {"choices":[{"text":"lo"}]}

`))
			_, _ = w.Write([]byte(`data: [DONE]

`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	var chatOut bytes.Buffer
	chat, err := StreamChat(context.Background(), srv.URL, &ChatRequest{Model: "test-model", Messages: []Message{{Role: "user", Content: "hello"}}}, &chatOut)
	if err != nil {
		t.Fatalf("StreamChat failed: %v", err)
	}
	if chat == nil || chat.Content != "hello" || chatOut.String() != "hello" {
		t.Fatalf("unexpected stream chat response: %#v output=%q", chat, chatOut.String())
	}

	var completeOut bytes.Buffer
	complete, err := StreamComplete(context.Background(), srv.URL, &ChatRequest{Model: "test-model", Prompt: "hello"}, &completeOut)
	if err != nil {
		t.Fatalf("StreamComplete failed: %v", err)
	}
	if complete == nil || complete.Content != "hello" || completeOut.String() != "hello" {
		t.Fatalf("unexpected stream completion response: %#v output=%q", complete, completeOut.String())
	}
}
