package induction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ModelRuntimeState describes the server-reported lifecycle state of a model.
type ModelRuntimeState string

const (
	// ModelRuntimeUnknown indicates that the server did not report a recognized state.
	ModelRuntimeUnknown ModelRuntimeState = "unknown"
	// ModelRuntimeUnloaded indicates that the model is not resident in memory.
	ModelRuntimeUnloaded ModelRuntimeState = "unloaded"
	// ModelRuntimeLoading indicates that the server is loading the model.
	ModelRuntimeLoading ModelRuntimeState = "loading"
	// ModelRuntimeLoaded indicates that the model is resident and ready.
	ModelRuntimeLoaded ModelRuntimeState = "loaded"
	// ModelRuntimeSleeping indicates that the server has put the model to sleep.
	ModelRuntimeSleeping ModelRuntimeState = "sleeping"
)

// ServerRole identifies whether the endpoint acts as a router or a model server.
type ServerRole string

const (
	// ServerRoleUnknown indicates that the endpoint role could not be determined.
	ServerRoleUnknown ServerRole = "unknown"
	// ServerRoleRouter identifies a router or multi-model endpoint.
	ServerRoleRouter ServerRole = "router"
	// ServerRoleModel identifies an endpoint serving model inference directly.
	ServerRoleModel ServerRole = "model"
)

// RuntimeModel is the normalized runtime state and metadata for one model.
type RuntimeModel struct {
	ID          string            `json:"id"`
	State       ModelRuntimeState `json:"state"`
	Failed      bool              `json:"failed,omitempty"`
	ExitCode    *int              `json:"exitCode,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Path        string            `json:"path,omitempty"`
	LastUsed    *int64            `json:"lastUsed,omitempty"`
	InputModes  []string          `json:"inputModalities,omitempty"`
	OutputModes []string          `json:"outputModalities,omitempty"`
	Raw         json.RawMessage   `json:"raw,omitempty"`
}

// RuntimeStatus is a point-in-time view of all models known to the server.
type RuntimeStatus struct {
	ServerRole  ServerRole     `json:"serverRole"`
	Models      []RuntimeModel `json:"models"`
	Loaded      []string       `json:"loaded"`
	Loading     []string       `json:"loading"`
	CollectedAt time.Time      `json:"collectedAt"`
}

// RuntimeOperation records one model load or unload transition.
type RuntimeOperation struct {
	Model       string            `json:"model"`
	From        ModelRuntimeState `json:"from"`
	To          ModelRuntimeState `json:"to"`
	Changed     bool              `json:"changed"`
	Duration    time.Duration     `json:"duration"`
	CompletedAt time.Time         `json:"completedAt"`
}

// SwitchOptions controls the behavior of SwitchModel.
type SwitchOptions struct{ UnloadOthers bool }

// SwitchOption modifies the options used by SwitchModel.
type SwitchOption func(*SwitchOptions)

// WithUnloadOthers controls whether SwitchModel unloads other loaded models
// before loading its target. It is enabled by default.
func WithUnloadOthers(enabled bool) SwitchOption {
	return func(o *SwitchOptions) { o.UnloadOthers = enabled }
}

// SwitchResult describes the operations performed while switching models.
type SwitchResult struct {
	Target   string             `json:"target"`
	Unloaded []RuntimeOperation `json:"unloaded,omitempty"`
	Load     *RuntimeOperation  `json:"load,omitempty"`
	Duration time.Duration      `json:"duration"`
}

var (
	// ErrModelNotFound indicates that the requested model is unknown to the server.
	ErrModelNotFound = errors.New("model not found")
	// ErrRuntimeUnsupported indicates that the endpoint does not expose runtime management.
	ErrRuntimeUnsupported = errors.New("runtime model management unsupported")
	// ErrModelLoadFailed indicates that a model could not be loaded successfully.
	ErrModelLoadFailed = errors.New("model load failed")
	// ErrModelUnloadFailed indicates that a model could not be unloaded successfully.
	ErrModelUnloadFailed = errors.New("model unload failed")
	// ErrRuntimeStateTimeout indicates that a requested lifecycle transition timed out.
	ErrRuntimeStateTimeout = errors.New("runtime state wait timed out")
)

// ModelRuntimeError reports a failed model lifecycle transition and preserves
// the underlying cause for errors.Is and errors.As inspection.
type ModelRuntimeError struct {
	Model    string
	State    ModelRuntimeState
	ExitCode *int
	Err      error
}

// Error returns a human-readable description of the failed model transition.
func (e *ModelRuntimeError) Error() string {
	if e.ExitCode != nil {
		return fmt.Sprintf("model %q (%s): %v (exit code %d)", e.Model, e.State, e.Err, *e.ExitCode)
	}
	return fmt.Sprintf("model %q (%s): %v", e.Model, e.State, e.Err)
}

// Unwrap returns the underlying lifecycle error.
func (e *ModelRuntimeError) Unwrap() error { return e.Err }

// GetRuntimeStatus returns the server-authoritative runtime state.
func (c *Client) GetRuntimeStatus(ctx context.Context) (*RuntimeStatus, error) {
	records, err := c.fetchModelRecords(ctx, false)
	if err != nil {
		return nil, err
	}
	result := &RuntimeStatus{ServerRole: c.serverRoleFromRecords(records), CollectedAt: time.Now()}
	for _, record := range records {
		model := runtimeModelFromMap(record)
		result.Models = append(result.Models, model)
		if model.State == ModelRuntimeLoaded {
			result.Loaded = append(result.Loaded, model.ID)
		}
		if model.State == ModelRuntimeLoading {
			result.Loading = append(result.Loading, model.ID)
		}
	}
	sort.Slice(result.Models, func(i, j int) bool { return result.Models[i].ID < result.Models[j].ID })
	sort.Strings(result.Loaded)
	sort.Strings(result.Loading)
	return result, nil
}

// GetRuntimeStatus returns runtime state using a convenience client for endpoint.
func GetRuntimeStatus(ctx context.Context, endpoint string, options ...ClientOption) (*RuntimeStatus, error) {
	return NewClient(ctx, endpoint, options...).GetRuntimeStatus(ctx)
}

// ServerRole returns the role reported or inferred for the client's endpoint.
func (c *Client) ServerRole(ctx context.Context) (ServerRole, error) {
	// /props is the least ambiguous source when llama.cpp reports its role.
	if role, ok := c.explicitServerRole(ctx); ok {
		return role, nil
	}
	records, err := c.fetchModelRecords(ctx, false)
	if err != nil {
		return ServerRoleUnknown, err
	}
	return c.serverRoleFromRecords(records), nil
}

// LoadModel asks the server to load model and waits for its resulting state.
func (c *Client) LoadModel(ctx context.Context, model string) (*RuntimeOperation, error) {
	operation, err := c.changeModelState(ctx, model, ModelRuntimeLoaded)
	if err == nil && operation.Changed {
		c.recordModelLoadDuration(model, operation.Duration)
	}
	return operation, err
}

// LoadModel asks endpoint to load model using a convenience client.
func LoadModel(ctx context.Context, endpoint, model string, options ...ClientOption) (*RuntimeOperation, error) {
	return NewClient(ctx, endpoint, options...).LoadModel(ctx, model)
}

// UnloadModel asks the server to unload model and waits for its resulting state.
func (c *Client) UnloadModel(ctx context.Context, model string) (*RuntimeOperation, error) {
	return c.changeModelState(ctx, model, ModelRuntimeUnloaded)
}

// UnloadModel asks endpoint to unload model using a convenience client.
func UnloadModel(ctx context.Context, endpoint, model string, options ...ClientOption) (*RuntimeOperation, error) {
	return NewClient(ctx, endpoint, options...).UnloadModel(ctx, model)
}

// SwitchModel optionally unloads other loaded models and loads target.
func (c *Client) SwitchModel(ctx context.Context, target string, options ...SwitchOption) (*SwitchResult, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("model ID is required")
	}
	settings := SwitchOptions{UnloadOthers: true}
	for _, option := range options {
		option(&settings)
	}
	start := time.Now()
	status, err := c.GetRuntimeStatus(ctx)
	if err != nil {
		return nil, err
	}
	result := &SwitchResult{Target: target}
	if settings.UnloadOthers {
		for _, model := range status.Models {
			if model.ID == target || model.State != ModelRuntimeLoaded {
				continue
			}
			op, err := c.UnloadModel(ctx, model.ID)
			if err != nil {
				return nil, fmt.Errorf("switch unload %q: %w", model.ID, err)
			}
			result.Unloaded = append(result.Unloaded, *op)
		}
	}
	op, err := c.LoadModel(ctx, target)
	if err != nil {
		return result, fmt.Errorf("switch load %q: %w", target, err)
	}
	result.Load = op
	result.Duration = time.Since(start)
	return result, nil
}

func (c *Client) changeModelState(ctx context.Context, model string, desired ModelRuntimeState) (*RuntimeOperation, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("model ID is required")
	}
	start := time.Now()
	c.runtimeMu.Lock()
	defer c.runtimeMu.Unlock()
	operationName := "loading"
	if desired == ModelRuntimeUnloaded {
		operationName = "unloading"
	}
	c.logf("runtime: %s model: %s", operationName, model)
	records, err := c.fetchModelRecords(ctx, false)
	if err != nil {
		return nil, err
	}
	current, found := runtimeModelByID(records, model)
	if !found {
		return nil, &ModelRuntimeError{Model: model, State: ModelRuntimeUnknown, Err: ErrModelNotFound}
	}
	if current.State == desired {
		return &RuntimeOperation{Model: model, From: desired, To: desired, CompletedAt: time.Now(), Duration: time.Since(start)}, nil
	}
	if desired == ModelRuntimeLoaded && current.State == ModelRuntimeLoading {
		final, err := c.waitForModelState(ctx, model, desired)
		return c.operation(start, model, current.State, final.State, err)
	}
	path := "/models/load"
	failure := ErrModelLoadFailed
	if desired == ModelRuntimeUnloaded {
		path = "/models/unload"
		failure = ErrModelUnloadFailed
	}
	payload, _ := json.Marshal(map[string]string{"model": model})
	resp, err := c.runtimeRequest(ctx, http.MethodPost, path, bytes.NewReader(payload))
	if err != nil {
		return nil, &ModelRuntimeError{Model: model, State: current.State, Err: errors.Join(failure, err)}
	}
	var ack struct {
		Success *bool  `json:"success"`
		Error   string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ack)
	resp.Body.Close()
	if ack.Success != nil && !*ack.Success {
		cause := failure
		if ack.Error != "" {
			cause = fmt.Errorf("%w: %s", failure, ack.Error)
		}
		return nil, &ModelRuntimeError{Model: model, State: current.State, Err: cause}
	}
	final, err := c.waitForModelState(ctx, model, desired)
	return c.operation(start, model, current.State, final.State, err)
}

func (c *Client) operation(start time.Time, model string, from, to ModelRuntimeState, err error) (*RuntimeOperation, error) {
	op := &RuntimeOperation{Model: model, From: from, To: to, Changed: from != to, Duration: time.Since(start), CompletedAt: time.Now()}
	if err != nil {
		return op, err
	}
	return op, nil
}

func (c *Client) waitForModelState(ctx context.Context, model string, desired ModelRuntimeState) (RuntimeModel, error) {
	interval := c.opts.loadWaitInterval
	if interval <= 0 {
		interval = time.Second
	}
	for {
		records, err := c.fetchModelRecords(ctx, true)
		if err != nil {
			return RuntimeModel{}, err
		}
		current, found := runtimeModelByID(records, model)
		if !found {
			return RuntimeModel{}, &ModelRuntimeError{Model: model, State: ModelRuntimeUnknown, Err: ErrModelNotFound}
		}
		if current.State == desired {
			return current, nil
		}
		if current.Failed {
			failure := ErrModelLoadFailed
			if desired == ModelRuntimeUnloaded {
				failure = ErrModelUnloadFailed
			}
			return current, &ModelRuntimeError{Model: model, State: current.State, ExitCode: current.ExitCode, Err: failure}
		}
		select {
		case <-ctx.Done():
			return current, fmt.Errorf("wait for model %q: %w: %v", model, ErrRuntimeStateTimeout, ctx.Err())
		case <-time.After(interval):
		}
	}
}

func (c *Client) runtimeRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.clientHTTP().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: %s", ErrRuntimeUnsupported, strings.TrimSpace(string(data)))
		}
		return nil, fmt.Errorf("runtime request returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return resp, nil
}

func (c *Client) fetchModelRecords(ctx context.Context, reload bool) ([]map[string]interface{}, error) {
	paths := []string{"/models", "/v1/models"}
	return c.fetchModelRecordsFromPaths(ctx, paths, reload)
}

func (c *Client) fetchModelRecordsAt(ctx context.Context, path string, reload bool) ([]map[string]interface{}, error) {
	return c.fetchModelRecordsFromPaths(ctx, []string{path}, reload)
}

func (c *Client) fetchModelRecordsFromPaths(ctx context.Context, paths []string, reload bool) ([]map[string]interface{}, error) {
	var last error
	for _, path := range paths {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path, nil)
		if err != nil {
			return nil, err
		}
		if reload {
			q := req.URL.Query()
			q.Set("reload", "1")
			req.URL.RawQuery = q.Encode()
		}
		resp, err := c.clientHTTP().Do(req)
		if err != nil {
			return nil, fmt.Errorf("models request failed: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read models body: %w", readErr)
		}
		if resp.StatusCode == http.StatusNotFound {
			last = fmt.Errorf("%w: %s", ErrRuntimeUnsupported, strings.TrimSpace(string(body)))
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("models request returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var envelope struct {
			Data []map[string]interface{} `json:"data"`
		}
		if err := json.Unmarshal(body, &envelope); err == nil && envelope.Data != nil {
			return envelope.Data, nil
		}
		var records []map[string]interface{}
		if err := json.Unmarshal(body, &records); err != nil {
			return nil, fmt.Errorf("failed to decode models json: %w", err)
		}
		return records, nil
	}
	return nil, last
}

func runtimeModelFromMap(m map[string]interface{}) RuntimeModel {
	status := modelStatusFromMap(m)
	raw, _ := json.Marshal(m)
	return RuntimeModel{ID: firstNonEmptyValue(stringValue(m["id"]), stringValue(m["name"])), State: status.State, Failed: status.Failed, ExitCode: status.ExitCode, Args: status.Args, Path: firstNonEmptyValue(stringValue(m["path"]), stringValue(m["model_path"])), LastUsed: status.LastUsed, InputModes: modalityStrings(architectureValue(m), "input_modalities"), OutputModes: modalityStrings(architectureValue(m), "output_modalities"), Raw: raw}
}

func runtimeModelByID(records []map[string]interface{}, id string) (RuntimeModel, bool) {
	for _, record := range records {
		model := runtimeModelFromMap(record)
		if model.ID == id {
			return model, true
		}
	}
	return RuntimeModel{}, false
}

func (c *Client) serverRoleFromRecords(records []map[string]interface{}) ServerRole {
	for _, record := range records {
		for _, key := range []string{"server_role", "serverRole", "role"} {
			role := strings.ToLower(strings.TrimSpace(stringValue(record[key])))
			if role == string(ServerRoleRouter) {
				return ServerRoleRouter
			}
			if role == string(ServerRoleModel) || role == "single-model" {
				return ServerRoleModel
			}
		}
	}
	// A /models collection containing more than one model is strong evidence
	// of router mode; a single row remains ambiguous and is intentionally not
	// guessed.
	if len(records) > 1 {
		return ServerRoleRouter
	}
	return ServerRoleUnknown
}

func (c *Client) explicitServerRole(ctx context.Context) (ServerRole, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/props", nil)
	if err != nil {
		return ServerRoleUnknown, false
	}
	resp, err := c.clientHTTP().Do(req)
	if err != nil {
		return ServerRoleUnknown, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ServerRoleUnknown, false
	}
	var value map[string]interface{}
	if json.NewDecoder(resp.Body).Decode(&value) != nil {
		return ServerRoleUnknown, false
	}
	for _, key := range []string{"server_role", "serverRole", "role"} {
		role := strings.ToLower(strings.TrimSpace(stringValue(value[key])))
		if role == string(ServerRoleRouter) {
			return ServerRoleRouter, true
		}
		if role == string(ServerRoleModel) || role == "single-model" {
			return ServerRoleModel, true
		}
	}
	for _, key := range []string{"router", "is_router", "router_mode"} {
		if enabled, ok := value[key].(bool); ok {
			if enabled {
				return ServerRoleRouter, true
			}
			return ServerRoleModel, true
		}
	}
	return ServerRoleUnknown, false
}

type normalizedModelStatus struct {
	State    ModelRuntimeState
	Failed   bool
	ExitCode *int
	Args     []string
	LastUsed *int64
}

func modelStatusFromMap(m map[string]interface{}) normalizedModelStatus {
	status := normalizedModelStatus{State: ModelRuntimeUnknown}
	hasExplicitState := false
	value := m["status"]
	if nested, ok := asMap(value); ok {
		value = firstNonEmptyValue(stringValue(nested["value"]), stringValue(nested["state"]))
		status.Failed, _ = nested["failed"].(bool)
		if code, ok := nested["exit_code"].(float64); ok {
			n := int(code)
			status.ExitCode = &n
		}
		if len(status.Args) == 0 {
			status.Args = argTokens(nested["args"])
		}
		if used, ok := nested["last_used"].(float64); ok {
			n := int64(used)
			status.LastUsed = &n
		}
	} else if _, ok := value.(string); !ok {
		value = m["state"]
	}
	if loaded, ok := m["loaded"].(bool); ok && loaded {
		status.State = ModelRuntimeLoaded
	}
	switch strings.ToLower(strings.TrimSpace(stringValue(value))) {
	case "loaded":
		status.State = ModelRuntimeLoaded
		hasExplicitState = true
	case "loading":
		status.State = ModelRuntimeLoading
		hasExplicitState = true
	case "unloaded":
		status.State = ModelRuntimeUnloaded
		hasExplicitState = true
	case "sleeping":
		status.State = ModelRuntimeSleeping
		hasExplicitState = true
	}
	if !hasExplicitState {
		if loaded, ok := m["loaded"].(bool); ok {
			if loaded {
				status.State = ModelRuntimeLoaded
			} else {
				status.State = ModelRuntimeUnloaded
			}
		}
	}
	if failed, ok := m["failed"].(bool); ok {
		status.Failed = failed
	}
	if args := argTokens(m["args"]); len(args) > 0 {
		status.Args = args
	}
	if len(status.Args) == 0 {
		status.Args = argTokens(m["arguments"])
	}
	if used, ok := m["last_used"].(float64); ok {
		n := int64(used)
		status.LastUsed = &n
	}
	return status
}
