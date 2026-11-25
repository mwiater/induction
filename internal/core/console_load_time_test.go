package induction

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestConsolePreloadsAndPipelineSwitchesReachSnapshots(t *testing.T) {
	states := map[string]ModelRuntimeState{"first": ModelRuntimeUnloaded, "second": ModelRuntimeUnloaded}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/models":
			_, _ = fmt.Fprintf(w, `{"data":[{"id":"first","status":%q},{"id":"second","status":%q}]}`, states["first"], states["second"])
		case "/models/load":
			var request struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode load request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			states[request.Model] = ModelRuntimeLoaded
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/models/unload":
			var request struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode unload request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			states[request.Model] = ModelRuntimeUnloaded
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
		case "/props":
			_, _ = w.Write([]byte(`{"total_slots":1}`))
		case "/slots":
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	client := NewClient(ctx, srv.URL, WithLoadWaitInterval(time.Millisecond))
	model := newConsoleModel(ctx, client, ChatRequest{Model: "first"}, time.Second, 24, consoleNonStreaming)
	if msg, ok := model.Init()().(consoleModelLoadedMsg); !ok || msg.err != nil {
		t.Fatalf("initial load = %#v", msg)
	}
	request := func(model string) *ChatRequest {
		return &ChatRequest{Model: model, Messages: []Message{{Role: "user", Content: "hello"}}}
	}
	first, err := client.GenerateSnapshot(ctx, request("first"))
	if err != nil || first.ModelLoadTime <= 0 {
		t.Fatalf("first snapshot load time = %v, err = %v", first, err)
	}
	loaded := loadModelCmd(ctx, client, time.Second, "second", "first", nil)().(modelLoadedMsg)
	if loaded.err != nil {
		t.Fatalf("switch model: %v", loaded.err)
	}
	second, err := client.GenerateSnapshot(ctx, request("second"))
	if err != nil || second.ModelLoadTime <= 0 {
		t.Fatalf("second snapshot load time = %v, err = %v", second, err)
	}
	again, err := client.GenerateSnapshot(ctx, request("second"))
	if err != nil || again.ModelLoadTime != 0 {
		t.Fatalf("already loaded snapshot load time = %v, err = %v", again, err)
	}
}
