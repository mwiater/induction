package induction

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestClient_GenerateSnapshot_Success verifies the happy path snapshot flow.
func TestClient_GenerateSnapshot_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices": [{"message": {"reasoning_content":"careful thought","content": "success"}}]}`))
		case "/props":
			_, _ = w.Write([]byte(`{"total_slots": 1, "default_generation_settings": {"n_ctx": 4096}}`))
		case "/slots":
			_, _ = w.Write([]byte(`[{"id": 0, "n_ctx": 4096, "is_processing": true}]`))
		case "/metrics":
			_, _ = w.Write([]byte("llamacpp:prompt_tokens_total 12\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := NewClient(ctx, srv.URL,
		WithPollInterval(10*time.Millisecond),
		WithLoadWaitInterval(10*time.Millisecond),
	)

	req := &ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}

	snapshot, err := client.GenerateSnapshot(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateSnapshot failed: %v", err)
	}
	if snapshot.ModelID != "test-model" {
		t.Fatalf("expected ModelID 'test-model', got %s", snapshot.ModelID)
	}
	if snapshot.LoadTime < 0 {
		t.Fatalf("expected LoadTime to be non-negative, got %v", snapshot.LoadTime)
	}
	if len(snapshot.Interaction) != 1 || snapshot.Interaction[0].Content != "success" {
		t.Fatalf("expected inference content 'success', got %#v", snapshot.Interaction)
	}
	if snapshot.Interaction[0].ReasoningContent != "careful thought" {
		t.Fatalf("expected snapshot reasoning content, got %#v", snapshot.Interaction)
	}
	if len(snapshot.Messages) != 2 || snapshot.Messages[0].Role != "user" || snapshot.Messages[1].Role != "assistant" || snapshot.Messages[1].Content != "success" {
		t.Fatalf("expected complete snapshot message history, got %#v", snapshot.Messages)
	}
	if snapshot.Props == nil || snapshot.Props.TotalSlots != 1 {
		t.Fatalf("expected props to be populated, got %#v", snapshot.Props)
	}
	if len(snapshot.Slots) != 1 {
		t.Fatalf("expected 1 slot, got %d", len(snapshot.Slots))
	}
	if snapshot.Metrics == nil || snapshot.Metrics.Entries["llamacpp:prompt_tokens_total"] != float64(12) {
		t.Fatalf("expected metrics to be populated, got %#v", snapshot.Metrics)
	}
}

// TestClient_GenerateSnapshot_PollsSlotsDuringInference verifies that slot
// sampling runs during inference and stops before GenerateSnapshot returns.
func TestClient_GenerateSnapshot_PollsSlotsDuringInference(t *testing.T) {
	var inferenceActive atomic.Bool
	var activePolls atomic.Int32
	var inactivePolls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			inferenceActive.Store(true)
			time.Sleep(75 * time.Millisecond)
			inferenceActive.Store(false)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"success"}}]}`))
		case "/props":
			_, _ = w.Write([]byte(`{"total_slots":1}`))
		case "/slots":
			if inferenceActive.Load() {
				activePolls.Add(1)
			} else {
				inactivePolls.Add(1)
			}
			_, _ = w.Write([]byte(`[{"id":0,"is_processing":true,"n_decoded":218}]`))
		case "/metrics":
			_, _ = w.Write([]byte("llamacpp:prompt_tokens_total 12\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(context.Background(), srv.URL, WithPollInterval(10*time.Millisecond))
	snapshot, err := client.GenerateSnapshot(context.Background(), &ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("GenerateSnapshot failed: %v", err)
	}
	if activePolls.Load() < 2 {
		t.Fatalf("expected multiple in-progress slot polls, got %d", activePolls.Load())
	}
	if len(snapshot.Slots) != 1 {
		t.Fatalf("expected only the newest slots payload, got %#v", snapshot.Slots)
	}
	// One post-inference /slots request is expected for the existing Slots field.
	if inactivePolls.Load() != 1 {
		t.Fatalf("expected only the final slots fetch after inference, got %d", inactivePolls.Load())
	}

	pollsAtReturn := activePolls.Load() + inactivePolls.Load()
	time.Sleep(30 * time.Millisecond)
	if got := activePolls.Load() + inactivePolls.Load(); got != pollsAtReturn {
		t.Fatalf("slot polling continued after return: before=%d after=%d", pollsAtReturn, got)
	}
}

func TestClient_LoadModelSkipsPostWhenAlreadyLoaded(t *testing.T) {
	var loadPosts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"test-model","status":{"value":"loaded"}}]}`))
		case "/models/load":
			loadPosts.Add(1)
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(context.Background(), srv.URL)
	if err := client.loadModel(context.Background(), "test-model"); err != nil {
		t.Fatalf("loadModel failed: %v", err)
	}
	if loadPosts.Load() != 0 {
		t.Fatalf("already-loaded model triggered %d load requests", loadPosts.Load())
	}
}

func TestClient_LoadModelWaitsUntilLoaded(t *testing.T) {
	var loaded atomic.Bool
	var statusChecks atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			statusChecks.Add(1)
			status := "unloaded"
			if loaded.Load() {
				status = "loaded"
			}
			fmt.Fprintf(w, `{"data":[{"id":"test-model","status":{"value":%q}}]}`, status)
		case "/models/load":
			go func() {
				time.Sleep(15 * time.Millisecond)
				loaded.Store(true)
			}()
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(context.Background(), srv.URL, WithLoadWaitInterval(5*time.Millisecond))
	if err := client.loadModel(context.Background(), "test-model"); err != nil {
		t.Fatalf("loadModel failed: %v", err)
	}
	if !loaded.Load() || statusChecks.Load() < 2 {
		t.Fatalf("loadModel returned before readiness: loaded=%v checks=%d", loaded.Load(), statusChecks.Load())
	}
}

// TestClient_GenerateSnapshot_PropsFailure confirms props failures are tolerated.
func TestClient_GenerateSnapshot_PropsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices": [{"message": {"content": "success"}}]}`))
		case "/props":
			w.WriteHeader(http.StatusInternalServerError)
		case "/slots":
			_, _ = w.Write([]byte(`[]`))
		case "/metrics":
			_, _ = w.Write([]byte("llamacpp:prompt_tokens_total 12\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(context.Background(), srv.URL, WithLoadWaitInterval(10*time.Millisecond))

	req := &ChatRequest{Model: "test-model"}
	snapshot, err := client.GenerateSnapshot(context.Background(), req)
	if err != nil {
		t.Fatalf("expected snapshot to succeed despite props failure, got err: %v", err)
	}
	if snapshot.Props != nil {
		t.Fatalf("expected props to remain nil after failure, got %#v", snapshot.Props)
	}
}

// TestClient_GenerateSnapshot_SlotsFailure confirms slots failures are tolerated.
func TestClient_GenerateSnapshot_SlotsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices": [{"message": {"content": "success"}}]}`))
		case "/props":
			_, _ = w.Write([]byte(`{"total_slots": 1}`))
		case "/slots":
			w.WriteHeader(http.StatusInternalServerError)
		case "/metrics":
			_, _ = w.Write([]byte("llamacpp:prompt_tokens_total 12\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(context.Background(), srv.URL, WithLoadWaitInterval(10*time.Millisecond))

	req := &ChatRequest{Model: "test-model"}
	snapshot, err := client.GenerateSnapshot(context.Background(), req)
	if err != nil {
		t.Fatalf("expected snapshot to succeed despite slots failure, got err: %v", err)
	}
	if len(snapshot.Slots) != 0 {
		t.Fatalf("expected slots to remain empty after failure, got %#v", snapshot.Slots)
	}
}

// TestClient_GenerateSnapshot_MetricsFailure confirms metrics failures are tolerated.
func TestClient_GenerateSnapshot_MetricsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices": [{"message": {"content": "success"}}]}`))
		case "/props":
			_, _ = w.Write([]byte(`{"total_slots": 1}`))
		case "/slots":
			_, _ = w.Write([]byte(`[]`))
		case "/metrics":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(context.Background(), srv.URL, WithLoadWaitInterval(10*time.Millisecond))

	req := &ChatRequest{Model: "test-model"}
	snapshot, err := client.GenerateSnapshot(context.Background(), req)
	if err != nil {
		t.Fatalf("expected snapshot to succeed despite metrics failure, got err: %v", err)
	}
	if snapshot.Metrics != nil {
		t.Fatalf("expected metrics to be nil after failure, got %#v", snapshot.Metrics)
	}
}

// TestClient_EnsureModelLoaded_Idempotent covers the cached target shortcut.
func TestClient_EnsureModelLoaded_Idempotent(t *testing.T) {
	client := NewClient(context.Background(), "http://localhost")
	if _, err := client.ensureModelLoaded(context.Background(), "test-model"); err != nil {
		t.Fatalf("ensureModelLoaded failed: %v", err)
	}
	if _, err := client.ensureModelLoaded(context.Background(), "test-model"); err != nil {
		t.Fatalf("ensureModelLoaded should be idempotent, got: %v", err)
	}
}
