// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// modelLoadProgress is an internal data type used by this package.
type modelLoadProgress struct {
	// Stages stores the value represented by this member.
	Stages []string `json:"stages"`
	// Current stores the value represented by this member.
	Current string `json:"current"`
	// Stage stores the value represented by this member.
	Stage string `json:"stage"`
	// Value stores the value represented by this member.
	Value *float64 `json:"value"`
}

// ModelLoadProgress is the server-reported progress of a model lifecycle event.
// ModelLoadProgress is an exported type in this package's public data model.
type ModelLoadProgress = modelLoadProgress

// ModelLifecycleEvent is a UI-independent representation of a model SSE event.
// ModelLifecycleEvent is an exported type in this package's public data model.
type ModelLifecycleEvent struct {
	// Model stores the value represented by this member.
	Model string `json:"model"`
	// Event stores the value represented by this member.
	Event string `json:"event,omitempty"`
	// State stores the value represented by this member.
	State ModelRuntimeState `json:"state"`
	// Progress stores the value represented by this member.
	Progress ModelLoadProgress `json:"progress,omitempty"`
}

// modelLoadingEvent is an internal data type used by this package.
type modelLoadingEvent struct {
	// Model stores the value represented by this member.
	Model string `json:"model"`
	// Event stores the value represented by this member.
	Event string `json:"event"`
	// Data stores the value represented by this member.
	Data struct {
		Status   string            `json:"status"`
		Progress modelLoadProgress `json:"progress"`
	} `json:"data"`
}

// monitorModelLoading consumes the server's model lifecycle stream and updates
// the existing overlay only for the model used by the current inference.
// Additional context: Loading is a one-shot lifecycle phase. Keep slot polling alive for inference metrics, but stop consuming stale lifecycle events.
func (c *Client) monitorModelLoading(ctx context.Context, model string, overlay *liveMetricsOverlay, ready chan<- struct{}) error {
	signalReady := func() {
		if ready != nil {
			close(ready)
			ready = nil
		}
	}
	defer signalReady()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/models/sse", http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.clientHTTP().Do(req)
	if err != nil {
		return fmt.Errorf("model loading stream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("model loading stream returned %d", resp.StatusCode)
	}
	signalReady()

	scanner := bufio.NewScanner(resp.Body)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		event, ok := parseModelLifecycleEvent(scanner.Text())
		if !ok || event.Model != model {
			continue
		}
		switch event.State {
		case ModelRuntimeLoading:
			c.logf("runtime: model loading model=%q stage=%q current=%q progress=%v", event.Model, event.Progress.Stage, event.Progress.Current, progressValue(event.Progress.Value))
			overlay.updateLoadingForModel(model, event.Progress)
		case ModelRuntimeLoaded:
			c.logf("runtime: model loaded model=%q", event.Model)
			overlay.modelLoadedForModel(model)

			return nil
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("read model loading stream: %w", err)
	}
	return nil
}

// progressValue performs the internal operation described by its signature.
func progressValue(value *float64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

// parseModelLoadingEvent performs the internal operation described by its signature.
func parseModelLoadingEvent(line string) (modelLoadingEvent, bool) {
	event, ok := parseModelLifecycleEvent(line)
	if !ok {
		return modelLoadingEvent{}, false
	}
	legacy := modelLoadingEvent{Model: event.Model, Event: event.Event}
	legacy.Data.Status = string(event.State)
	legacy.Data.Progress = event.Progress
	return legacy, true
}

// parseModelLifecycleEvent performs the internal operation described by its signature.
func parseModelLifecycleEvent(line string) (ModelLifecycleEvent, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data:") {
		return ModelLifecycleEvent{}, false
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	var raw struct {
		Model string `json:"model"`
		Event string `json:"event"`
		State string `json:"state"`
		Data  struct {
			Status   string            `json:"status"`
			Progress ModelLoadProgress `json:"progress"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return ModelLifecycleEvent{}, false
	}
	state := raw.State
	if state == "" {
		state = raw.Data.Status
	}
	event := ModelLifecycleEvent{Model: raw.Model, Event: raw.Event, State: ModelRuntimeState(strings.ToLower(state)), Progress: raw.Data.Progress}
	return event, event.Model != ""
}
