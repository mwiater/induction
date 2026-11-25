package induction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

func snapshotRequestMetadata(req *ChatRequest) map[string]any {
	if req == nil {
		return nil
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return nil
	}
	var result map[string]any
	if json.Unmarshal(encoded, &result) != nil {
		return nil
	}
	for _, key := range []string{"messages", "prompt", "tools", "json_schema", "grammar", "logit_bias", "image_data", "response_format"} {
		delete(result, key)
	}
	result["tool_count"] = len(req.Tools)
	result["has_response_format"] = req.ResponseFormat != nil
	result["has_grammar"] = req.Grammar != ""
	return result
}

func applyEffectiveGenerationSettings(request map[string]any, defaults map[string]interface{}) {
	if len(request) == 0 || len(defaults) == 0 {
		return
	}
	effective := make(map[string]any)
	for key, value := range defaults {
		effective[key] = value
	}
	if params, ok := defaults["params"].(map[string]interface{}); ok {
		effective = make(map[string]any, len(params)+1)
		for key, value := range params {
			effective[key] = value
		}
		if value, ok := defaults["n_ctx"]; ok {
			effective["n_ctx"] = value
		}
	}
	for _, key := range []string{"stream", "reasoning_control", "max_tokens", "max_completion_tokens", "n_predict", "stop", "seed", "ignore_eos", "temperature", "top_p", "top_k", "min_p", "presence_penalty", "frequency_penalty", "repeat_penalty", "repeat_last_n", "penalize_nl", "tfs_z", "typical_p", "mirostat", "mirostat_tau", "mirostat_eta", "samplers", "xtc_threshold", "xtc_probability", "dry_multiplier", "dry_base", "dry_allowed_length", "dry_penalty_last_n", "dry_sequence_breakers", "dynatemp_range", "dynatemp_exponent", "cache_prompt", "slot_id", "n_keep"} {
		if value, ok := request[key]; ok {
			effective[key] = value
		}
	}
	request["effective_generation_settings"] = effective
}

func completeSnapshotTiming(snapshot *ModelSnapshot, startedAt time.Time, firstTokenAt *time.Time) {
	if snapshot == nil {
		return
	}
	completedAt := time.Now().UTC()
	duration := completedAt.Sub(startedAt).Seconds() * 1000
	timing := &SnapshotTimingTelemetry{StartedAt: startedAt.UTC(), CompletedAt: completedAt, DurationMS: duration}
	if firstTokenAt != nil {
		value := firstTokenAt.Sub(startedAt).Seconds() * 1000
		timing.FirstTokenAt = firstTokenAt
		timing.TimeToFirstTokenMS = &value
	}
	snapshot.Timing = timing
}

func completeSnapshotOutcome(snapshot *ModelSnapshot, status string, err error) {
	if snapshot == nil {
		return
	}
	outcome := &SnapshotOutcomeTelemetry{Status: status}
	if err != nil {
		outcome.Error = err.Error()
		outcome.Cancelled = err == context.Canceled || err == context.DeadlineExceeded
	}
	for _, interaction := range snapshot.Interaction {
		var payload any
		if json.Unmarshal([]byte(interaction.Response), &payload) != nil {
			continue
		}
		outcome.FinishReason = firstFinishReason(payload)
		if outcome.FinishReason != "" {
			break
		}
	}
	snapshot.Outcome = outcome
}

func firstFinishReason(payload any) string {
	switch value := payload.(type) {
	case map[string]any:
		choices, _ := value["choices"].([]any)
		for _, raw := range choices {
			if choice, ok := raw.(map[string]any); ok {
				if reason, ok := choice["finish_reason"].(string); ok && reason != "" {
					return reason
				}
			}
		}
	case []any:
		for _, item := range value {
			if reason := firstFinishReason(item); reason != "" {
				return reason
			}
		}
	}
	return ""
}

func snapshotWorkloadMetadata(req *ChatRequest) *SnapshotWorkloadTelemetry {
	if req == nil {
		return nil
	}
	encoded, _ := json.Marshal(req.Messages)
	result := &SnapshotWorkloadTelemetry{MessageCount: len(req.Messages), ToolCount: len(req.Tools), PromptBytes: len(encoded)}
	for _, message := range req.Messages {
		switch strings.ToLower(message.Role) {
		case "user":
			result.UserMessages++
		case "tool":
			result.ToolMessages++
		}
		if parts, ok := message.Content.([]ContentPart); ok {
			for _, part := range parts {
				if part.ImageURL != nil || part.File != nil {
					result.AttachmentCount++
				}
			}
		}
	}
	hash := sha256.Sum256(encoded)
	result.Hash = hex.EncodeToString(hash[:])
	return result
}
