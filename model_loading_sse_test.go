package induction

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseModelLoadingEvent(t *testing.T) {
	event, ok := parseModelLoadingEvent(`data: {"model":"vision-model","event":"status_change","data":{"status":"loading","progress":{"stages":["text_model","mmproj_model"],"current":"mmproj_model","value":0.5155}}}`)
	if !ok {
		t.Fatal("expected event to parse")
	}
	if event.Model != "vision-model" || event.Data.Status != "loading" {
		t.Fatalf("unexpected event: %#v", event)
	}
	if event.Data.Progress.Current != "mmproj_model" || event.Data.Progress.Value == nil || *event.Data.Progress.Value != 0.5155 {
		t.Fatalf("unexpected progress: %#v", event.Data.Progress)
	}
}

func TestParseModelLoadingEventRejectsInvalidLines(t *testing.T) {
	for _, line := range []string{"", "event: status_change", "data:a {bad json}"} {
		if _, ok := parseModelLoadingEvent(line); ok {
			t.Fatalf("expected invalid line to be rejected: %q", line)
		}
	}
}

func TestMonitorModelLoadingReturnsToMetricsAfterLoaded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/sse" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"model":"other-model","event":"status_change","data":{"status":"loading","progress":{"current":"text_model","value":0.5}}}`)
		fmt.Fprintln(w, `data: {"model":"target-model","event":"status_change","data":{"status":"loading","progress":{"current":"text_model","value":0.5}}}`)
		fmt.Fprintln(w, `data: {"model":"target-model","event":"status_change","data":{"status":"loaded"}}`)
	}))
	defer srv.Close()

	overlay := &liveMetricsOverlay{}
	client := NewClient(context.Background(), srv.URL)
	ready := make(chan struct{})
	if err := client.monitorModelLoading(context.Background(), "target-model", overlay, ready); err != nil {
		t.Fatalf("monitorModelLoading failed: %v", err)
	}
	select {
	case <-ready:
	default:
		t.Fatal("expected readiness channel to be closed")
	}
	if overlay.loading {
		t.Fatal("expected loaded event to return the overlay to metrics mode")
	}
}
