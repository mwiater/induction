package induction

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRuntimeModelTransitionsAndSwitch(t *testing.T) {
	states := map[string]ModelRuntimeState{"old": ModelRuntimeLoaded, "target": ModelRuntimeUnloaded}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/models":
			_, _ = fmt.Fprintf(w, `{"data":[{"id":"old","status":%q},{"id":"target","status":%q}]}`, states["old"], states["target"])
		case "/models/load":
			states["target"] = ModelRuntimeLoaded
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/models/unload":
			states["old"] = ModelRuntimeUnloaded
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(context.Background(), srv.URL, WithLoadWaitInterval(time.Millisecond))
	result, err := client.SwitchModel(context.Background(), "target")
	if err != nil {
		t.Fatalf("SwitchModel failed: %v", err)
	}
	if len(result.Unloaded) != 1 || result.Unloaded[0].Model != "old" || result.Load == nil || result.Load.To != ModelRuntimeLoaded {
		t.Fatalf("unexpected switch result: %#v", result)
	}
	if _, err := client.LoadModel(context.Background(), "target"); err != nil {
		t.Fatalf("idempotent LoadModel failed: %v", err)
	}
	if _, err := client.UnloadModel(context.Background(), "missing"); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("expected model-not-found error, got %v", err)
	}
}

func TestSwitchModelSerializesCompoundTransitions(t *testing.T) {
	var stateMu sync.Mutex
	states := map[string]ModelRuntimeState{"old": ModelRuntimeLoaded, "target": ModelRuntimeUnloaded}
	var activeLoads atomic.Int32
	var maxActiveLoads atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			stateMu.Lock()
			defer stateMu.Unlock()
			_, _ = fmt.Fprintf(w, `{"data":[{"id":"old","status":%q},{"id":"target","status":%q}]}`, states["old"], states["target"])
		case "/models/unload":
			stateMu.Lock()
			states["old"] = ModelRuntimeUnloaded
			stateMu.Unlock()
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/models/load":
			current := activeLoads.Add(1)
			for {
				previous := maxActiveLoads.Load()
				if current <= previous || maxActiveLoads.CompareAndSwap(previous, current) {
					break
				}
			}
			if current > 1 {
				activeLoads.Add(-1)
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"message":"model limit reached, try again later"}}`))
				return
			}
			time.Sleep(5 * time.Millisecond)
			stateMu.Lock()
			states["target"] = ModelRuntimeLoaded
			stateMu.Unlock()
			activeLoads.Add(-1)
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(context.Background(), srv.URL, WithLoadWaitInterval(time.Millisecond))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.SwitchModel(context.Background(), "target")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent SwitchModel failed: %v", err)
		}
	}
	if got := maxActiveLoads.Load(); got != 1 {
		t.Fatalf("maximum concurrent model loads = %d, want 1", got)
	}
}

func TestRuntimeTransitionFailureAndTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"model","status":"unloaded"}]}`))
		case "/models/load":
			_, _ = w.Write([]byte(`{"success":false,"error":"denied"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	client := NewClient(context.Background(), srv.URL)
	_, err := client.LoadModel(context.Background(), "model")
	if !errors.Is(err, ErrModelLoadFailed) || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("unexpected load failure: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	client = NewClient(context.Background(), srv.URL, WithLoadWaitInterval(time.Millisecond))
	// The failed acknowledgement is replaced by a successful one, but the
	// server never reports the requested loaded state, exercising the timeout.
	client.opts.httpClient = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/models/load" {
			return responseWithBody(http.StatusOK, `{"success":true}`), nil
		}
		return responseWithBody(http.StatusOK, `{"data":[{"id":"model","status":"unloaded"}]}`), nil
	})}
	_, err = client.LoadModel(ctx, "model")
	if !errors.Is(err, ErrRuntimeStateTimeout) {
		t.Fatalf("expected runtime timeout, got %v", err)
	}
}

func TestModelRuntimeErrorFormatting(t *testing.T) {
	underlying := errors.New("cause")
	err := &ModelRuntimeError{Model: "model", State: ModelRuntimeLoading, ExitCode: intPtr(7), Err: underlying}
	if !strings.Contains(err.Error(), "exit code 7") || !errors.Is(err, underlying) {
		t.Fatalf("unexpected runtime error: %v", err)
	}
}

func intPtr(value int) *int { return &value }
