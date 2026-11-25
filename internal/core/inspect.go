// Package induction contains the core implementation for the public Induction API.
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
// ServerInspection is an exported type in this package's public data model.
type ServerInspection struct {
	// Endpoint stores the value represented by this member.
	Endpoint string `json:"endpoint"`
	// Role stores the value represented by this member.
	Role ServerRole `json:"role"`
	// Healthy stores the value represented by this member.
	Healthy bool `json:"healthy"`
	// Models stores the value represented by this member.
	Models []RuntimeModel `json:"models"`
	// LoadedModels stores the value represented by this member.
	LoadedModels []string `json:"loadedModels"`
	// Props stores the value represented by this member.
	Props *PropsData `json:"props,omitempty"`
	// CollectedAt stores the value represented by this member.
	CollectedAt time.Time `json:"collectedAt"`
}

// ModelCapabilities contains capabilities explicitly reported by the server.
// ModelCapabilities is an exported type in this package's public data model.
type ModelCapabilities struct {
	// TextInput stores the value represented by this member.
	TextInput bool `json:"textInput"`
	// ImageInput stores the value represented by this member.
	ImageInput bool `json:"imageInput"`
	// AudioInput stores the value represented by this member.
	AudioInput bool `json:"audioInput"`
	// TextOutput stores the value represented by this member.
	TextOutput bool `json:"textOutput"`
}

// ModelRuntimeConfig contains normalized, optional runtime settings.
// ModelRuntimeConfig is an exported type in this package's public data model.
type ModelRuntimeConfig struct {
	// ContextSize stores the value represented by this member.
	ContextSize *int `json:"contextSize,omitempty"`
	// BatchSize stores the value represented by this member.
	BatchSize *int `json:"batchSize,omitempty"`
	// UBatchSize stores the value represented by this member.
	UBatchSize *int `json:"ubatchSize,omitempty"`
	// Parallel stores the value represented by this member.
	Parallel *int `json:"parallel,omitempty"`
	// CacheTypeK stores the value represented by this member.
	CacheTypeK string `json:"cacheTypeK,omitempty"`
	// CacheTypeV stores the value represented by this member.
	CacheTypeV string `json:"cacheTypeV,omitempty"`
	// FlashAttention stores the value represented by this member.
	FlashAttention *bool `json:"flashAttention,omitempty"`
	// Temperature stores the value represented by this member.
	Temperature *float64 `json:"temperature,omitempty"`
	// TopK stores the value represented by this member.
	TopK *int `json:"topK,omitempty"`
	// TopP stores the value represented by this member.
	TopP *float64 `json:"topP,omitempty"`
	// RepeatLastN stores the value represented by this member.
	RepeatLastN *int `json:"repeatLastN,omitempty"`
	// RepeatPenalty stores the value represented by this member.
	RepeatPenalty *float64 `json:"repeatPenalty,omitempty"`
}

// ModelInspection is a read-only snapshot of one runtime model.
// ModelInspection is an exported type in this package's public data model.
type ModelInspection struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// State stores the value represented by this member.
	State ModelRuntimeState `json:"state"`
	// Failed stores the value represented by this member.
	Failed bool `json:"failed,omitempty"`
	// ExitCode stores the value represented by this member.
	ExitCode *int `json:"exitCode,omitempty"`
	// Path stores the value represented by this member.
	Path string `json:"path,omitempty"`
	// Args stores the value represented by this member.
	Args []string `json:"args,omitempty"`
	// Capabilities stores the value represented by this member.
	Capabilities ModelCapabilities `json:"capabilities"`
	// Runtime stores the value represented by this member.
	Runtime ModelRuntimeConfig `json:"runtime"`
	// Props stores the value represented by this member.
	Props *PropsData `json:"props,omitempty"`
	// Slots stores the value represented by this member.
	Slots SlotsData `json:"slots,omitempty"`
	// RawModel stores the value represented by this member.
	RawModel json.RawMessage `json:"rawModel,omitempty"`
	// CollectedAt stores the value represented by this member.
	CollectedAt time.Time `json:"collectedAt"`
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
		props, err := c.fetchPropsReady(ctx, model)
		if err != nil {
			c.logf("inspect: fetch props for %q failed: %v", model, err)
		} else {
			inspection.Props = props
		}
	}
	return inspection, nil
}

// recordByID performs the internal operation described by its signature.
func recordByID(records []map[string]interface{}, id string) (map[string]interface{}, bool) {
	for _, record := range records {
		if firstNonEmptyValue(stringValue(record["id"]), stringValue(record["name"])) == id {
			return record, true
		}
	}
	return nil, false
}

// modelRuntimeConfigFromMap performs the internal operation described by its signature.
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

// intValue performs the internal operation described by its signature.
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

// floatValue performs the internal operation described by its signature.
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

// boolValue performs the internal operation described by its signature.
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

// argValue performs the internal operation described by its signature.
func argValue(args map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := args[key]; value != "" {
			return value
		}
	}
	return ""
}

// capabilitiesFromMap performs the internal operation described by its signature.
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

// checkHealthContext performs the internal operation described by its signature.
func (c *Client) checkHealthContext(ctx context.Context) error {
	var lastErr error
	for _, path := range []string{"/health", "/v1/health"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path, http.NoBody)
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
