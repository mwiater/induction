// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// buildDashboardMetrics performs the internal operation described by its signature.
// Additional context: Tool loops can propagate the original lifecycle duration to their final snapshot. Export that repeated duration as one event.
func buildDashboardMetrics(sources []dashboardSessionSource, source DashboardSource, ignoredModels map[string]bool) (*DashboardMetrics, error) {
	groups := make(map[string]*dashboardModelBuilder)
	configs := make(map[string]DashboardRuntimeConfig)
	generationConfigs := make(map[string]DashboardGenerationConfig)
	loadEvents := []DashboardLoadEvent{}
	seenSessions := make(map[string]struct{})
	for _, item := range sources {
		included := false
		lastLoadKey := ""
		for index, snapshot := range item.session.Snapshots {
			if snapshot == nil || strings.TrimSpace(snapshot.ModelID) == "" {
				source.SnapshotsSkipped++
				continue
			}
			if ignoredModels[snapshot.ModelID] {
				source.SnapshotsSkipped++
				continue
			}
			observation, ok := dashboardObservation(item.session, snapshot, index)
			if !ok {
				source.SnapshotsSkipped++
				continue
			}
			included = true
			seenSessions[item.session.ID] = struct{}{}
			configJSON, err := json.Marshal(observation.Runtime)
			if err != nil {
				return nil, fmt.Errorf("encode runtime config: %w", err)
			}
			configSum := sha256.Sum256(append([]byte(snapshot.ModelID+"\x00"), configJSON...))
			configID := hex.EncodeToString(configSum[:12])
			configs[configID] = DashboardRuntimeConfig{ID: configID, ModelID: snapshot.ModelID, Runtime: observation.Runtime}
			observation.RuntimeConfigID = configID
			if len(observation.Request) > 0 {
				requestJSON, err := json.Marshal(observation.Request)
				if err != nil {
					return nil, fmt.Errorf("encode generation config: %w", err)
				}
				requestSum := sha256.Sum256(append([]byte(snapshot.ModelID+"\x00"), requestJSON...))
				requestID := hex.EncodeToString(requestSum[:12])
				generationConfigs[requestID] = DashboardGenerationConfig{ID: requestID, ModelID: snapshot.ModelID, Settings: observation.Request}
				observation.GenerationConfigID = requestID
				observation.Request = nil
			}
			// Slot timelines remain in the raw session archive. The dashboard projection
			// keeps only the context summary and derived pressure relationships.
			observation.SlotSamples = nil

			if observation.LoadTimeMS != nil && *observation.LoadTimeMS > 0 {
				loadKey := fmt.Sprintf("%s\x00%s\x00%g", snapshot.ModelID, configID, *observation.LoadTimeMS)
				if lastLoadKey != loadKey {
					loadEvents = append(loadEvents, DashboardLoadEvent{ID: fmt.Sprintf("%s:%d:%s", item.session.ID, index, snapshot.ModelID), ModelID: snapshot.ModelID, SessionID: item.session.ID, SnapshotIndex: index, RuntimeConfigID: configID, AssociatedSnapshotAt: snapshot.CollectedAt, DurationMS: *observation.LoadTimeMS, Source: "client_observed_lifecycle_transition", Kind: "unknown"})
				} else {
					observation.LoadTimeMS = nil
				}
				lastLoadKey = loadKey
			} else {
				lastLoadKey = ""
			}
			group := groups[snapshot.ModelID]
			if group == nil {
				group = &dashboardModelBuilder{ModelID: snapshot.ModelID, SessionIDs: make(map[string]struct{})}
				groups[snapshot.ModelID] = group
			}
			group.SessionIDs[item.session.ID] = struct{}{}
			group.Observations = append(group.Observations, observation)
			if !snapshot.CollectedAt.IsZero() {
				t := snapshot.CollectedAt
				if group.First == nil || t.Before(*group.First) {
					group.First = &t
				}
				if group.Last == nil || t.After(*group.Last) {
					group.Last = &t
				}
			}
		}
		if !included {
			source.SessionsWithoutObservations++
		}
	}
	models := finalizeDashboardModels(groups)
	contextPressure := buildDashboardContextPressure(models)
	latencyPressure := buildDashboardLatencyPressure(models)
	cachePressure := buildDashboardCachePressure(models)
	reasoningPressure := buildDashboardReasoningPressure(models)
	workloadPressure := buildDashboardWorkloadPressure(models)
	reliability := buildDashboardReliability(models)
	source.SnapshotsIncluded = source.SnapshotsSeen - source.SnapshotsSkipped
	source.Models = len(models)
	source.UniqueSessionIDs = len(sources)
	source.ObservedSessionIDs = len(seenSessions)
	orderedConfigs := make([]DashboardRuntimeConfig, 0, len(configs))
	for _, config := range configs {
		orderedConfigs = append(orderedConfigs, config)
	}
	sort.Slice(orderedConfigs, func(i, j int) bool { return orderedConfigs[i].ID < orderedConfigs[j].ID })
	orderedGenerationConfigs := make([]DashboardGenerationConfig, 0, len(generationConfigs))
	for _, config := range generationConfigs {
		orderedGenerationConfigs = append(orderedGenerationConfigs, config)
	}
	sort.Slice(orderedGenerationConfigs, func(i, j int) bool { return orderedGenerationConfigs[i].ID < orderedGenerationConfigs[j].ID })
	return &DashboardMetrics{SchemaVersion: DashboardSchemaVersion, Source: source, RuntimeConfigs: orderedConfigs, GenerationConfigs: orderedGenerationConfigs, LoadEvents: loadEvents, Models: models, ContextPressure: contextPressure, LatencyPressure: latencyPressure, CachePressure: cachePressure, ReasoningPressure: reasoningPressure, WorkloadPressure: workloadPressure, Reliability: reliability, SIA: json.RawMessage("[]")}, nil
}

func buildDashboardContextPressure(models []DashboardModelData) map[string]DashboardContextPressure {
	result := make(map[string]DashboardContextPressure, len(models))
	for _, model := range models {
		generation := make(map[DashboardContextPressurePair]int)
		prompt := make(map[DashboardContextPressurePair]int)
		for _, observation := range model.Observations {
			if observation.Context == nil || observation.Context.Tokens == nil {
				continue
			}
			contextTokens := *observation.Context.Tokens
			if observation.Performance != nil {
				if observation.Performance.GenerationTokensPerSecond != nil {
					key := DashboardContextPressurePair{TPS: *observation.Performance.GenerationTokensPerSecond, ContextTokens: contextTokens}
					generation[key]++
				}
				if observation.Performance.PromptTokensPerSecond != nil {
					key := DashboardContextPressurePair{TPS: *observation.Performance.PromptTokensPerSecond, ContextTokens: contextTokens}
					prompt[key]++
				}
			}
		}
		entry := DashboardContextPressure{GenerationTPS: pressurePairs(generation), PromptTPS: pressurePairs(prompt)}
		result[model.ModelID] = entry
	}
	return result
}

func pressurePairs(values map[DashboardContextPressurePair]int) []DashboardContextPressurePair {
	result := make([]DashboardContextPressurePair, 0, len(values))
	for value := range values {
		value.Count = values[value]
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ContextTokens != result[j].ContextTokens {
			return result[i].ContextTokens < result[j].ContextTokens
		}
		return result[i].TPS < result[j].TPS
	})
	return result
}

// dashboardObservation performs the internal operation described by its signature.
func dashboardObservation(session *ChatSession, snapshot *ModelSnapshot, index int) (DashboardSnapshotObservation, bool) {
	if snapshot == nil || strings.TrimSpace(snapshot.ModelID) == "" {
		return DashboardSnapshotObservation{}, false
	}
	conversation := dashboardConversationStats(snapshot.Messages)
	response, telemetry := dashboardResponseStats(snapshot.Interaction)
	applyDashboardMetricTelemetry(snapshot.Metrics, &telemetry)
	applyDashboardSlotSampleTelemetry(snapshot.SlotSamples, &telemetry)
	applyDashboardSlotTelemetry(snapshot.Slots, &telemetry)
	if telemetry.tokens.Completion == nil && snapshot.ResponseTokens != nil {
		value := *snapshot.ResponseTokens
		telemetry.tokens.Completion = &value
	}
	finalizeDashboardSpeculative(&telemetry.speculative)
	observation := DashboardSnapshotObservation{Session: DashboardSessionProvenance{ID: session.ID, Type: session.Type, Title: session.Title, CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt, SnapshotIndex: index}, CollectedAt: snapshot.CollectedAt, InputType: normalizeDashboardInputType(snapshot.InputType), OutputType: snapshot.OutputType, Conversation: conversation, Tools: DashboardToolUsage{ApplicationToolsAvailable: snapshot.ApplicationToolsAvailable, MCPToolsAvailable: snapshot.MCPToolsAvailable, ApplicationToolsUsed: snapshot.ApplicationToolsUsed, MCPToolsUsed: snapshot.MCPToolsUsed, MCPToolNames: append([]string(nil), snapshot.MCPToolNames...), ApplicationToolUseOutcome: snapshot.ApplicationToolUseOutcome, MCPToolUseOutcome: snapshot.MCPToolUseOutcome}, Response: response, Tokens: dashboardTokensIfPopulated(telemetry.tokens), ReasoningTokens: snapshot.ReasoningTokens, Performance: dashboardPerformanceIfPopulated(telemetry.performance), Context: dashboardContextTelemetry(snapshot), Request: snapshot.Request, Workload: snapshot.Workload, Timing: snapshot.Timing, SlotSamples: snapshot.SlotSamples, Outcome: snapshot.Outcome, Speculative: dashboardSpeculativeIfPopulated(telemetry.speculative), Runtime: extractDashboardRuntime(snapshot.Props), Metrics: extractDashboardMetrics(snapshot.Metrics)}
	if snapshot.ModelLoadTime != 0 {
		value := float64(snapshot.ModelLoadTime) / float64(time.Millisecond)
		observation.LoadTimeMS = &value
	}
	return observation, true
}

func dashboardContextTelemetry(snapshot *ModelSnapshot) *SnapshotContextTelemetry {
	if snapshot == nil {
		return nil
	}
	if snapshot.Context != nil {
		return snapshot.Context
	}
	_, _, used, capacity, ok := activeSlotMetrics(snapshot.Slots)
	if !ok || capacity <= 0 {
		return nil
	}
	tokens, limit := int(used), int(capacity)
	peak := tokens
	percent := used / capacity * 100
	peakPercent := percent
	shifts := 0
	if snapshot.Metrics != nil {
		if value, found := dashboardNumber(snapshot.Metrics.Entries["llamacpp:context_shift"]); found {
			shifts = int(value)
		}
	}
	return &SnapshotContextTelemetry{Tokens: &tokens, Capacity: &limit, Percent: &percent, PeakTokens: &peak, PeakPercent: &peakPercent, Shifts: &shifts}
}

// normalizeDashboardInputType performs the internal operation described by its signature.
func normalizeDashboardInputType(inputType string) string {
	if strings.EqualFold(strings.TrimSpace(inputType), "image") {
		return "vision"
	}
	return strings.ToLower(strings.TrimSpace(inputType))
}

// cloneToolArguments performs the internal operation described by its signature.
func cloneToolArguments(value map[string][]string) map[string][]string {
	if len(value) == 0 {
		return nil
	}
	result := make(map[string][]string, len(value))
	for name, arguments := range value {
		result[name] = append([]string(nil), arguments...)
	}
	return result
}

// dashboardTokensIfPopulated performs the internal operation described by its signature.
func dashboardTokensIfPopulated(value DashboardTokens) *DashboardTokens {
	if value.Prompt == nil && value.Completion == nil && value.Total == nil && value.Cached == nil {
		return nil
	}
	return &value
}

// dashboardPerformanceIfPopulated performs the internal operation described by its signature.
func dashboardPerformanceIfPopulated(value DashboardPerformance) *DashboardPerformance {
	if value.PromptMS == nil && value.PromptTokensPerSecond == nil && value.GenerationMS == nil && value.GenerationTokensPerSecond == nil && value.MillisecondsPerGeneratedToken == nil {
		return nil
	}
	return &value
}

// dashboardSpeculativeIfPopulated performs the internal operation described by its signature.
func dashboardSpeculativeIfPopulated(value DashboardSpeculative) *DashboardSpeculative {
	if value.DraftTokens == nil && value.AcceptedDraftTokens == nil && value.AcceptanceRate == nil {
		return nil
	}
	return &value
}

// dashboardConversationStats performs the internal operation described by its signature.
func dashboardConversationStats(messages []Message) DashboardConversation {
	result := DashboardConversation{MessageCount: len(messages)}
	for _, message := range messages {
		switch strings.ToLower(message.Role) {
		case "user":
			result.UserMessages++
		case "assistant":
			result.AssistantMessages++
		case "system":
			result.SystemMessages++
		case "tool":
			result.ToolMessages++
		}
		if len(message.ToolCalls) > 0 {
			result.HasToolCalls = true
			result.ToolCallCount += len(message.ToolCalls)
		}
	}
	result.TurnNumber = result.UserMessages
	return result
}

// dashboardTelemetry is an internal data type used by this package.
type dashboardTelemetry struct {
	// tokens stores the value represented by this member.
	tokens DashboardTokens
	// performance stores the value represented by this member.
	performance DashboardPerformance
	// speculative stores the value represented by this member.
	speculative DashboardSpeculative
}

// dashboardResponseStats performs the internal operation described by its signature.
func dashboardResponseStats(interactions []Interaction) (DashboardResponse, dashboardTelemetry) {
	var result DashboardResponse
	telemetry := dashboardTelemetry{}
	for _, interaction := range interactions {
		result.VisibleCharacters += utf8.RuneCountInString(interaction.Content)
		result.VisibleWords += len(strings.Fields(interaction.Content))
		result.ReasoningCharacters += utf8.RuneCountInString(interaction.ReasoningContent)
		result.ReasoningWords += len(strings.Fields(interaction.ReasoningContent))
		result.HasVisibleContent = result.HasVisibleContent || strings.TrimSpace(interaction.Content) != ""
		result.HasReasoning = result.HasReasoning || strings.TrimSpace(interaction.ReasoningContent) != ""
		var raw any
		if json.Unmarshal([]byte(interaction.Response), &raw) != nil {
			continue
		}
		payloads := []map[string]any{}
		switch value := raw.(type) {
		case map[string]any:
			payloads = append(payloads, value)
		case []any:
			for _, item := range value {
				if object, ok := item.(map[string]any); ok {
					payloads = append(payloads, object)
				}
			}
		}
		for _, payload := range payloads {
			applyDashboardTelemetry(payload, &result, &telemetry)
		}
	}
	return result, telemetry
}

// applyDashboardTelemetry performs the internal operation described by its signature.
func applyDashboardTelemetry(payload map[string]any, response *DashboardResponse, telemetry *dashboardTelemetry) {
	if value, ok := payload["system_fingerprint"].(string); ok && value != "" {
		response.SystemFingerprint = value
	}
	if choices, ok := payload["choices"].([]any); ok {
		for _, raw := range choices {
			if choice, ok := raw.(map[string]any); ok {
				if reason, ok := choice["finish_reason"].(string); ok && reason != "" && response.FinishReason == "" {
					response.FinishReason = reason
				}
			}
		}
	}
	if usage, ok := payload["usage"].(map[string]any); ok {
		setIntFromMap(usage, "prompt_tokens", &telemetry.tokens.Prompt)
		setIntFromMap(usage, "completion_tokens", &telemetry.tokens.Completion)
		setIntFromMap(usage, "total_tokens", &telemetry.tokens.Total)
		if details, ok := usage["prompt_tokens_details"].(map[string]any); ok {
			setIntFromMap(details, "cached_tokens", &telemetry.tokens.Cached)
		}
	}
	if timings, ok := payload["timings"].(map[string]any); ok {
		applyDashboardTiming(timings, telemetry)
	}
	applyDashboardTiming(payload, telemetry)
}

// applyDashboardTiming performs the internal operation described by its signature.
func applyDashboardTiming(values map[string]any, telemetry *dashboardTelemetry) {
	setIntFromMap(values, "prompt_n", &telemetry.tokens.Prompt)
	setIntFromMap(values, "predicted_n", &telemetry.tokens.Completion)
	setIntFromMap(values, "cache_n", &telemetry.tokens.Cached)
	setIntFromMap(values, "tokens_cached", &telemetry.tokens.Cached)
	setIntFromMap(values, "n_prompt_tokens_cache", &telemetry.tokens.Cached)
	setIntFromMap(values, "draft_n", &telemetry.speculative.DraftTokens)
	setIntFromMap(values, "draft_n_accepted", &telemetry.speculative.AcceptedDraftTokens)
	setFloatFromMap(values, "prompt_ms", &telemetry.performance.PromptMS)
	setFloatFromMap(values, "prompt_per_second", &telemetry.performance.PromptTokensPerSecond)
	setFloatFromMap(values, "predicted_ms", &telemetry.performance.GenerationMS)
	setFloatFromMap(values, "predicted_per_second", &telemetry.performance.GenerationTokensPerSecond)
	if telemetry.performance.GenerationMS == nil {
		setFloatFromMap(values, "generation_ms", &telemetry.performance.GenerationMS)
	}
	if telemetry.performance.GenerationTokensPerSecond == nil {
		setFloatFromMap(values, "generation_tokens_per_second", &telemetry.performance.GenerationTokensPerSecond)
	}
	if telemetry.performance.GenerationMS != nil && telemetry.tokens.Completion != nil && *telemetry.tokens.Completion > 0 {
		value := *telemetry.performance.GenerationMS / float64(*telemetry.tokens.Completion)
		telemetry.performance.MillisecondsPerGeneratedToken = &value
	} else if telemetry.performance.GenerationTokensPerSecond != nil && *telemetry.performance.GenerationTokensPerSecond > 0 {
		value := 1000 / *telemetry.performance.GenerationTokensPerSecond
		telemetry.performance.MillisecondsPerGeneratedToken = &value
	}
}

// applyDashboardMetricTelemetry only promotes metrics that are direct rates.
// The token-total metric is a Prometheus counter and is intentionally not used
// as a per-request token count.
func applyDashboardMetricTelemetry(metrics *MetricsData, telemetry *dashboardTelemetry) {
	if metrics == nil {
		return
	}
	setMetricFloat(metrics.Entries, "llamacpp:prompt_tokens_seconds", &telemetry.performance.PromptTokensPerSecond)
	setMetricFloat(metrics.Entries, "llamacpp:predicted_tokens_seconds", &telemetry.performance.GenerationTokensPerSecond)
}

// applyDashboardSlotTelemetry supplies a prompt-token fallback when the
// response does not include request-scoped usage/timing data. The /slots
// endpoint reports the number of prompt tokens currently held by each slot;
// use the largest populated slot because a snapshot normally represents one
// request, while avoiding a sum that would double-count unrelated slots.
func applyDashboardSlotTelemetry(slots SlotsData, telemetry *dashboardTelemetry) {
	var promptTokens, cachedTokens *int
	for _, slot := range slots {
		if value, ok := dashboardNumber(slot["n_prompt_tokens"]); ok && value >= 0 {
			candidate := int(value)
			if promptTokens == nil || candidate > *promptTokens {
				promptTokens = &candidate
				if cached, ok := dashboardNumber(slot["n_prompt_tokens_cache"]); ok && cached >= 0 {
					cachedValue := int(cached)
					cachedTokens = &cachedValue
				} else {
					cachedTokens = nil
				}
			}
		}
	}
	if telemetry.tokens.Prompt == nil && promptTokens != nil {
		telemetry.tokens.Prompt = promptTokens
	}
	if telemetry.tokens.Cached == nil && cachedTokens != nil {
		telemetry.tokens.Cached = cachedTokens
	}
}

// applyDashboardSlotSampleTelemetry preserves request-scoped cache telemetry
// from an active slot sample. Final /slots responses commonly contain the
// now-idle slot, whose cache counter has already reset to zero.
func applyDashboardSlotSampleTelemetry(samples []SnapshotSlotSample, telemetry *dashboardTelemetry) {
	if len(samples) == 0 {
		return
	}
	var selected *SnapshotSlotSample
	for index := range samples {
		sample := &samples[index]
		if !sample.IsProcessing {
			continue
		}
		if selected == nil || sample.PromptTokens > selected.PromptTokens || (sample.PromptTokens == selected.PromptTokens && sample.CachedTokens > selected.CachedTokens) {
			selected = sample
		}
	}
	if selected == nil {
		return
	}
	if telemetry.tokens.Prompt == nil && selected.PromptTokens >= 0 {
		value := selected.PromptTokens
		telemetry.tokens.Prompt = &value
	}
	if telemetry.tokens.Cached == nil && selected.CachedTokens >= 0 {
		value := selected.CachedTokens
		telemetry.tokens.Cached = &value
	}
}

// setMetricFloat performs the internal operation described by its signature.
func setMetricFloat(values map[string]interface{}, key string, target **float64) {
	if *target != nil {
		return
	}
	if value, ok := dashboardNumber(values[key]); ok && !math.IsNaN(value) && !math.IsInf(value, 0) {
		*target = &value
	}
}

// finalizeDashboardSpeculative performs the internal operation described by its signature.
// Additional context: Partial telemetry is not useful: absent accepted-token data must not look like a zero acceptance rate (and vice versa).
func finalizeDashboardSpeculative(speculative *DashboardSpeculative) {

	if speculative.DraftTokens == nil || speculative.AcceptedDraftTokens == nil {
		*speculative = DashboardSpeculative{}
		return
	}
	if *speculative.DraftTokens > 0 {
		rate := float64(*speculative.AcceptedDraftTokens) / float64(*speculative.DraftTokens)
		speculative.AcceptanceRate = &rate
	}
}

// setIntFromMap performs the internal operation described by its signature.
func setIntFromMap(values map[string]any, key string, target **int) {
	if value, ok := dashboardNumber(values[key]); ok {
		converted := int(value)
		if converted >= 0 {
			*target = &converted
		}
	}
}

// setFloatFromMap performs the internal operation described by its signature.
func setFloatFromMap(values map[string]any, key string, target **float64) {
	if value, ok := dashboardNumber(values[key]); ok && !math.IsNaN(value) && !math.IsInf(value, 0) {
		*target = &value
	}
}

// dashboardNumber performs the internal operation described by its signature.
func dashboardNumber(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case int:
		return float64(value), true
	case json.Number:
		n, err := value.Float64()
		return n, err == nil
	}
	return 0, false
}

// extractDashboardRuntime performs the internal operation described by its signature.
func extractDashboardRuntime(props *PropsData) DashboardRuntime {
	var result DashboardRuntime
	if props == nil {
		return result
	}
	if props.TotalSlots != 0 {
		result.TotalSlots = &props.TotalSlots
	}
	result.GenerationSettings = dashboardSafeSettings(props.DefaultGenerationSettings)
	if props.Raw == "" {
		return result
	}
	var raw map[string]any
	if json.Unmarshal([]byte(props.Raw), &raw) != nil {
		return result
	}
	for _, key := range []string{"context_size", "n_ctx", "ctx_size"} {
		if value, ok := findNumber(raw, key); ok {
			converted := int(value)
			result.ContextSize = &converted
			break
		}
	}
	for key, target := range map[string]*string{"model_alias": &result.ModelAlias, "build_info": &result.BuildInfo} {
		if value, ok := findString(raw, key); ok {
			*target = value
		}
	}
	if value, ok := findStrings(raw, "modalities"); ok {
		result.Modalities = value
	}
	if modalities, ok := raw["modalities"].(map[string]any); ok {
		if vision, exists := modalities["vision"].(bool); exists {
			result.VisionCapable = &vision
		}
	}
	return result
}

// dashboardSafeSettings performs the internal operation described by its signature.
func dashboardSafeSettings(settings map[string]interface{}) map[string]any {
	if len(settings) == 0 {
		return nil
	}
	result := make(map[string]any)
	for key, value := range settings {
		switch value.(type) {
		case nil, string, bool, float64, float32, int, int64, uint, uint64, []any, []string:
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// findNumber performs the internal operation described by its signature.
func findNumber(value any, key string) (float64, bool) {
	if object, ok := value.(map[string]any); ok {
		if result, ok := dashboardNumber(object[key]); ok {
			return result, true
		}
		for _, child := range object {
			if result, ok := findNumber(child, key); ok {
				return result, true
			}
		}
	}
	return 0, false
}

// findString performs the internal operation described by its signature.
func findString(value any, key string) (string, bool) {
	if object, ok := value.(map[string]any); ok {
		if result, ok := object[key].(string); ok && result != "" {
			return result, true
		}
		for _, child := range object {
			if result, ok := findString(child, key); ok {
				return result, true
			}
		}
	}
	return "", false
}

// findStrings performs the internal operation described by its signature.
func findStrings(value any, key string) ([]string, bool) {
	if object, ok := value.(map[string]any); ok {
		if values, ok := object[key].([]any); ok {
			result := make([]string, 0, len(values))
			for _, item := range values {
				if stringValue, ok := item.(string); ok {
					result = append(result, stringValue)
				}
			}
			return result, len(result) > 0
		}
		for _, child := range object {
			if result, ok := findStrings(child, key); ok {
				return result, true
			}
		}
	}
	return nil, false
}

// dashboardMetricAllowlist holds package-level state used internally by this package.
var dashboardMetricAllowlist = map[string]struct{}{"llamacpp:prompt_tokens_total": {}, "llamacpp:predicted_tokens_total": {}, "llamacpp:prompt_tokens_seconds": {}, "llamacpp:predicted_tokens_seconds": {}, "llamacpp:kv_cache_usage": {}, "llamacpp:kv_cache_capacity": {}, "llamacpp:context_shift": {}, "llamacpp:requests_processing": {}, "llamacpp:requests_deferred": {}, "llamacpp:tokens_cached": {}, "llamacpp:draft_tokens": {}, "llamacpp:accepted_draft_tokens": {}}

// extractDashboardMetrics performs the internal operation described by its signature.
func extractDashboardMetrics(metrics *MetricsData) map[string]any {
	if metrics == nil || len(metrics.Entries) == 0 {
		return nil
	}
	result := make(map[string]any)
	for key, value := range metrics.Entries {
		if _, ok := dashboardMetricAllowlist[key]; ok {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// finalizeDashboardModels performs the internal operation described by its signature.
func finalizeDashboardModels(groups map[string]*dashboardModelBuilder) []DashboardModelData {
	models := make([]DashboardModelData, 0, len(groups))
	for _, group := range groups {
		sort.SliceStable(group.Observations, func(i, j int) bool {
			left, right := group.Observations[i], group.Observations[j]
			if !left.CollectedAt.Equal(right.CollectedAt) {
				return left.CollectedAt.Before(right.CollectedAt)
			}
			if left.Session.ID != right.Session.ID {
				return left.Session.ID < right.Session.ID
			}
			return left.Session.SnapshotIndex < right.Session.SnapshotIndex
		})
		models = append(models, DashboardModelData{ModelID: group.ModelID, SessionCount: len(group.SessionIDs), SnapshotCount: len(group.Observations), FirstObservedAt: group.First, LastObservedAt: group.Last, Observations: group.Observations})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ModelID < models[j].ModelID })
	return models
}
