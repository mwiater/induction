package induction

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type modelLoadProgress struct {
	Stages  []string `json:"stages"`
	Current string   `json:"current"`
	Stage   string   `json:"stage"`
	Value   *float64 `json:"value"`
}

// ModelLoadProgress is the server-reported progress of a model lifecycle event.
type ModelLoadProgress = modelLoadProgress

// ModelLifecycleEvent is a UI-independent representation of a model SSE event.
type ModelLifecycleEvent struct {
	Model    string            `json:"model"`
	Event    string            `json:"event,omitempty"`
	State    ModelRuntimeState `json:"state"`
	Progress ModelLoadProgress `json:"progress,omitempty"`
}

type modelLoadingEvent struct {
	Model string `json:"model"`
	Event string `json:"event"`
	Data  struct {
		Status   string            `json:"status"`
		Progress modelLoadProgress `json:"progress"`
	} `json:"data"`
}

// monitorModelLoading consumes the server's model lifecycle stream and updates
// the existing overlay only for the model used by the current inference.
func (c *Client) monitorModelLoading(ctx context.Context, model string, overlay *liveMetricsOverlay, ready chan<- struct{}) error {
	signalReady := func() {
		if ready != nil {
			close(ready)
			ready = nil
		}
	}
	defer signalReady()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/models/sse", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.clientHTTP().Do(req)
	if err != nil {
		return fmt.Errorf("model loading stream request failed: %w", err)
	}
	defer resp.Body.Close()
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
			overlay.UpdateLoading(event.Progress)
		case ModelRuntimeLoaded:
			overlay.ModelLoaded()
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("read model loading stream: %w", err)
	}
	return nil
}

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
