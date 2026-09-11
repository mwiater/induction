package induction

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInspectModelIsReadOnlyAndNormalizesRuntime(t *testing.T) {
	var loads int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"vision","path":"/models/vision.gguf","status":{"value":"loaded","args":["llama-server","--ctx-size","32768","--batch-size","2048","--ubatch-size","512","--parallel","1","--cache-type-k","q8_0","--cache-type-v","q4_0","--flash-attn","off"]},"architecture":{"input_modalities":["text","image"],"output_modalities":["text"]}}]}`))
		case "/props":
			_, _ = w.Write([]byte(`{"total_slots":1,"default_generation_settings":{"n_ctx":32768}}`))
		case "/models/load":
			loads++
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	inspection, err := NewClient(context.Background(), srv.URL).InspectModel(context.Background(), "vision")
	if err != nil {
		t.Fatalf("InspectModel failed: %v", err)
	}
	if inspection.State != ModelRuntimeLoaded || inspection.Runtime.ContextSize == nil || *inspection.Runtime.ContextSize != 32768 {
		t.Fatalf("unexpected inspection: %#v", inspection)
	}
	if !inspection.Capabilities.ImageInput || !inspection.Capabilities.TextInput || !inspection.Capabilities.TextOutput {
		t.Fatalf("unexpected capabilities: %#v", inspection.Capabilities)
	}
	if inspection.Runtime.FlashAttention == nil || *inspection.Runtime.FlashAttention {
		t.Fatal("expected explicit flash-attention false")
	}
	if len(inspection.RawModel) == 0 || loads != 0 {
		t.Fatalf("inspection was not read-only: raw=%s loads=%d", inspection.RawModel, loads)
	}
}

func TestInspectModelUnloadedDoesNotFetchProps(t *testing.T) {
	var props int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_, _ = w.Write([]byte(`[{"id":"idle","status":{"value":"unloaded"}}]`))
		case "/props":
			props++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	inspection, err := NewClient(context.Background(), srv.URL).InspectModel(context.Background(), "idle")
	if err != nil || inspection.State != ModelRuntimeUnloaded {
		t.Fatalf("unexpected result: %#v %v", inspection, err)
	}
	if props != 0 {
		t.Fatalf("unloaded inspection fetched props %d times", props)
	}
}

func TestRuntimeStatusUnknownAndFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			_, _ = w.Write([]byte(`[{"id":"broken","status":{"value":"hibernating","failed":true,"exit_code":1}}]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	status, err := NewClient(context.Background(), srv.URL).GetRuntimeStatus(context.Background())
	if err != nil || status.Models[0].State != ModelRuntimeUnknown || !status.Models[0].Failed || status.Models[0].ExitCode == nil || *status.Models[0].ExitCode != 1 {
		t.Fatalf("unexpected status: %#v %v", status, err)
	}
	if _, err := NewClient(context.Background(), srv.URL).InspectModel(context.Background(), "missing"); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("expected not-found error, got %v", err)
	}
	_ = json.Valid(status.Models[0].Raw)
}

func TestInspectServerCollectsHealthRoleModelsAndProps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/props":
			_, _ = w.Write([]byte(`{"role":"model","total_slots":2}`))
		case "/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"model-a","role":"model","status":"loaded"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	inspection, err := NewClient(context.Background(), srv.URL).InspectServer(context.Background())
	if err != nil {
		t.Fatalf("InspectServer failed: %v", err)
	}
	if !inspection.Healthy || inspection.Role != ServerRoleModel || len(inspection.Models) != 1 || len(inspection.LoadedModels) != 1 || inspection.Props == nil || inspection.Props.TotalSlots != 2 {
		t.Fatalf("unexpected inspection: %#v", inspection)
	}
}
