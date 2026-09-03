package induction

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ServerInspection is a read-only snapshot of a llama.cpp server.
type ServerInspection struct {
	Endpoint     string         `json:"endpoint"`
	Role         ServerRole     `json:"role"`
	Healthy      bool           `json:"healthy"`
	Models       []RuntimeModel `json:"models"`
	LoadedModels []string       `json:"loadedModels"`
	Props        *PropsData     `json:"props,omitempty"`
	CollectedAt  time.Time      `json:"collectedAt"`
}

// ModelCapabilities contains capabilities explicitly reported by the server.
type ModelCapabilities struct {
	TextInput  bool `json:"textInput"`
	ImageInput bool `json:"imageInput"`
	AudioInput bool `json:"audioInput"`
	TextOutput bool `json:"textOutput"`
}

// ModelRuntimeConfig contains normalized, optional runtime settings.
type ModelRuntimeConfig struct {
	ContextSize    *int     `json:"contextSize,omitempty"`
	BatchSize      *int     `json:"batchSize,omitempty"`
	UBatchSize     *int     `json:"ubatchSize,omitempty"`
	Parallel       *int     `json:"parallel,omitempty"`
	CacheTypeK     string   `json:"cacheTypeK,omitempty"`
	CacheTypeV     string   `json:"cacheTypeV,omitempty"`
	FlashAttention *bool    `json:"flashAttention,omitempty"`
	Temperature    *float64 `json:"temperature,omitempty"`
	TopK           *int     `json:"topK,omitempty"`
	TopP           *float64 `json:"topP,omitempty"`
	RepeatLastN    *int     `json:"repeatLastN,omitempty"`
	RepeatPenalty  *float64 `json:"repeatPenalty,omitempty"`
}

// ModelInspection is a read-only snapshot of one runtime model.
type ModelInspection struct {
	ID           string             `json:"id"`
	State        ModelRuntimeState  `json:"state"`
	Failed       bool               `json:"failed,omitempty"`
	ExitCode     *int               `json:"exitCode,omitempty"`
	Path         string             `json:"path,omitempty"`
	Args         []string           `json:"args,omitempty"`
	Capabilities ModelCapabilities  `json:"capabilities"`
	Runtime      ModelRuntimeConfig `json:"runtime"`
	Props        *PropsData         `json:"props,omitempty"`
	Slots        SlotsData          `json:"slots,omitempty"`
	RawModel     json.RawMessage    `json:"rawModel,omitempty"`
	CollectedAt  time.Time          `json:"collectedAt"`
}

// InspectServer collects health, role, and model metadata from the endpoint.
func (c *Client) InspectServer(ctx context.Context) (*ServerInspection, error) {
	if err := c.checkHealthContext(ctx); err != nil {
		return nil, err
	}
	status, err := c.GetRuntimeStatus(ctx)
	if err != nil {
		return nil, err
	}
	inspection := &ServerInspection{Endpoint: c.endpoint, Role: status.ServerRole, Healthy: true, Models: status.Models, LoadedModels: status.Loaded, CollectedAt: time.Now()}
	props, err := c.fetchProps(ctx, "")
	if err != nil {
		c.logf("inspect: fetch server props failed: %v", err)
	} else {
		inspection.Props = props
	}
	return inspection, nil
}

// InspectModel collects runtime, capability, and telemetry data for model.
func (c *Client) InspectModel(ctx context.Context, model string) (*ModelInspection, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("model ID is required")
	}
	records, err := c.fetchModelRecords(ctx, false)
	if err != nil {
		return nil, err
	}
	record, ok := recordByID(records, model)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrModelNotFound, model)
	}
	runtime := runtimeModelFromMap(record)
	inspection := &ModelInspection{ID: runtime.ID, State: runtime.State, Failed: runtime.Failed, ExitCode: runtime.ExitCode, Path: runtime.Path, Args: runtime.Args, Capabilities: capabilitiesFromMap(record), Runtime: modelRuntimeConfigFromMap(record), RawModel: runtime.Raw, CollectedAt: time.Now()}
	if runtime.State == ModelRuntimeLoaded {
		props, err := c.fetchProps(ctx, model)
		if err != nil {
			c.logf("inspect: fetch props for %q failed: %v", model, err)
		} else {
			inspection.Props = props
		}
	}
	return inspection, nil
}

func recordByID(records []map[string]interface{}, id string) (map[string]interface{}, bool) {
	for _, record := range records {
		if firstNonEmptyValue(stringValue(record["id"]), stringValue(record["name"])) == id {
			return record, true
		}
	}
	return nil, false
}

func modelRuntimeConfigFromMap(m map[string]interface{}) ModelRuntimeConfig {
	args := parsedModelArgs(extractModelArgs(m))
	result := ModelRuntimeConfig{
		CacheTypeK: firstNonEmptyValue(lookupModelValue(m, "cache-type-k", "cache_type_k"), args["cache-type-k"]),
		CacheTypeV: firstNonEmptyValue(lookupModelValue(m, "cache-type-v", "cache_type_v"), args["cache-type-v"]),
	}
	result.ContextSize = intValue(m, args, []string{"ctx-size", "ctx_size", "n_ctx"})
	result.BatchSize = intValue(m, args, []string{"batch-size", "batch_size"})
	result.UBatchSize = intValue(m, args, []string{"ubatch-size", "ubatch_size"})
	result.Parallel = intValue(m, args, []string{"parallel"})
	result.TopK = intValue(m, args, []string{"top-k", "top_k"})
	result.RepeatLastN = intValue(m, args, []string{"repeat-last-n", "repeat_last_n"})
	result.Temperature = floatValue(m, args, []string{"temperature"})
	result.TopP = floatValue(m, args, []string{"top-p", "top_p"})
	result.RepeatPenalty = floatValue(m, args, []string{"repeat-penalty", "repeat_penalty"})
	result.FlashAttention = boolValue(m, args, []string{"flash-attn", "flash_attn"})
	return result
}

func intValue(m map[string]interface{}, args map[string]string, keys []string) *int {
	value := firstNonEmptyValue(lookupModelValue(m, keys...), argValue(args, keys...))
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return &parsed
}
func floatValue(m map[string]interface{}, args map[string]string, keys []string) *float64 {
	value := firstNonEmptyValue(lookupModelValue(m, keys...), argValue(args, keys...))
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	return &parsed
}
func boolValue(m map[string]interface{}, args map[string]string, keys []string) *bool {
	value := firstNonEmptyValue(lookupModelValue(m, keys...), argValue(args, keys...))
	if value == "" {
		return nil
	}
	switch strings.ToLower(value) {
	case "on", "enabled", "enable":
		value = "true"
	case "off", "disabled", "disable":
		value = "false"
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func argValue(args map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := args[key]; value != "" {
			return value
		}
	}
	return ""
}

func capabilitiesFromMap(m map[string]interface{}) ModelCapabilities {
	arch := architectureValue(m)
	input := modalityStrings(arch, "input_modalities")
	output := modalityStrings(arch, "output_modalities")
	result := ModelCapabilities{}
	for _, mode := range input {
		switch strings.ToLower(mode) {
		case "text":
			result.TextInput = true
		case "image", "vision":
			result.ImageInput = true
		case "audio":
			result.AudioInput = true
		}
	}
	for _, mode := range output {
		if strings.EqualFold(mode, "text") {
			result.TextOutput = true
		}
	}
	return result
}

func (c *Client) checkHealthContext(ctx context.Context) error {
	var lastErr error
	for _, path := range []string{"/health", "/v1/health"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path, nil)
		if err != nil {
			return err
		}
		resp, err := c.clientHTTP().Do(req)
		if err != nil {
			return fmt.Errorf("health request failed: %w", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		lastErr = fmt.Errorf("health endpoint %s returned %d", path, resp.StatusCode)
	}
	return lastErr
}
