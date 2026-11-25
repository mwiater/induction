package induction

import "sort"

type dashboardFloatPair struct {
	x float64
	y float64
}

func buildDashboardLatencyPressure(models []DashboardModelData) map[string]DashboardLatencyPressure {
	result := make(map[string]DashboardLatencyPressure, len(models))
	for _, model := range models {
		prompt, completion, context, ttft := map[dashboardFloatPair]int{}, map[dashboardFloatPair]int{}, map[dashboardFloatPair]int{}, map[dashboardFloatPair]int{}
		promptTTFT, postFirstToken := map[dashboardFloatPair]int{}, map[dashboardFloatPair]int{}
		for _, observation := range model.Observations {
			if observation.Performance != nil && observation.Tokens != nil && observation.Tokens.Prompt != nil && observation.Performance.PromptMS != nil {
				prompt[dashboardFloatPair{x: float64(*observation.Tokens.Prompt), y: *observation.Performance.PromptMS}]++
			}
			if observation.Performance != nil && observation.Tokens != nil && observation.Tokens.Completion != nil && observation.Performance.GenerationMS != nil {
				completion[dashboardFloatPair{x: float64(*observation.Tokens.Completion), y: *observation.Performance.GenerationMS}]++
			}
			if observation.Timing != nil {
				if observation.Context != nil && observation.Context.Tokens != nil {
					context[dashboardFloatPair{x: float64(*observation.Context.Tokens), y: observation.Timing.DurationMS}]++
					if observation.Timing.TimeToFirstTokenMS != nil {
						tail := observation.Timing.DurationMS - *observation.Timing.TimeToFirstTokenMS
						if tail >= 0 {
							postFirstToken[dashboardFloatPair{x: float64(*observation.Context.Tokens), y: tail}]++
						}
					}
				}
				if observation.Timing.TimeToFirstTokenMS != nil {
					if observation.Context != nil && observation.Context.Tokens != nil {
						ttft[dashboardFloatPair{x: float64(*observation.Context.Tokens), y: *observation.Timing.TimeToFirstTokenMS}]++
					}
					if observation.Tokens != nil && observation.Tokens.Prompt != nil {
						promptTTFT[dashboardFloatPair{x: float64(*observation.Tokens.Prompt), y: *observation.Timing.TimeToFirstTokenMS}]++
					}
				}
			}
		}
		result[model.ModelID] = DashboardLatencyPressure{PromptTokensToMS: latencyPoints(prompt), CompletionTokensToMS: latencyPoints(completion), ContextTokensToMS: latencyPoints(context), ContextTokensToTTFT: latencyPoints(ttft), PromptTokensToTTFT: latencyPoints(promptTTFT), ContextTokensToPostFirstTokenMS: latencyPoints(postFirstToken)}
	}
	return result
}

func buildDashboardCachePressure(models []DashboardModelData) map[string]DashboardCachePressure {
	result := make(map[string]DashboardCachePressure, len(models))
	for _, model := range models {
		cached, ratio := map[dashboardFloatPair]int{}, map[dashboardFloatPair]int{}
		for _, observation := range model.Observations {
			if observation.Tokens == nil || observation.Tokens.Prompt == nil || *observation.Tokens.Prompt <= 0 || observation.Performance == nil {
				continue
			}
			if observation.Tokens.Cached != nil && observation.Performance.PromptTokensPerSecond != nil {
				cached[dashboardFloatPair{x: float64(*observation.Tokens.Cached), y: *observation.Performance.PromptTokensPerSecond}]++
			}
			if observation.Tokens.Cached != nil && observation.Performance.PromptMS != nil {
				ratio[dashboardFloatPair{x: float64(*observation.Tokens.Cached) / float64(*observation.Tokens.Prompt), y: *observation.Performance.PromptMS}]++
			}
		}
		result[model.ModelID] = DashboardCachePressure{CachedTokensToPromptTPS: cachePoints(cached), CacheRatioToPromptMS: cachePoints(ratio)}
	}
	return result
}

func buildDashboardReasoningPressure(models []DashboardModelData) map[string]DashboardReasoningPressure {
	result := make(map[string]DashboardReasoningPressure, len(models))
	for _, model := range models {
		generation, duration := map[dashboardReasoningKey]int{}, map[dashboardReasoningKey]int{}
		for _, observation := range model.Observations {
			if observation.ReasoningTokens == nil {
				continue
			}
			if observation.Performance != nil && observation.Performance.GenerationTokensPerSecond != nil {
				generation[dashboardReasoningKey{tokens: *observation.ReasoningTokens, value: *observation.Performance.GenerationTokensPerSecond}]++
			}
			if observation.Timing != nil {
				duration[dashboardReasoningKey{tokens: *observation.ReasoningTokens, value: observation.Timing.DurationMS}]++
			}
		}
		result[model.ModelID] = DashboardReasoningPressure{ReasoningTokensToGenerationTPS: reasoningPoints(generation), ReasoningTokensToDurationMS: reasoningPoints(duration)}
	}
	return result
}

type dashboardReasoningKey struct {
	tokens int
	value  float64
}

func buildDashboardWorkloadPressure(models []DashboardModelData) map[string]DashboardWorkloadPressure {
	result := make(map[string]DashboardWorkloadPressure, len(models))
	for _, model := range models {
		messages, toolCalls, attachments := map[dashboardFloatPair]int{}, map[dashboardFloatPair]int{}, map[dashboardFloatPair]int{}
		for _, observation := range model.Observations {
			if observation.Workload == nil || observation.Timing == nil {
				continue
			}
			duration := observation.Timing.DurationMS
			messages[dashboardFloatPair{x: float64(observation.Workload.MessageCount), y: duration}]++
			toolCalls[dashboardFloatPair{x: float64(observation.Conversation.ToolCallCount), y: duration}]++
			if observation.Performance != nil && observation.Performance.PromptTokensPerSecond != nil {
				attachments[dashboardFloatPair{x: float64(observation.Workload.AttachmentCount), y: *observation.Performance.PromptTokensPerSecond}]++
			}
		}
		result[model.ModelID] = DashboardWorkloadPressure{MessagesToDurationMS: cachePoints(messages), ToolCallsToDurationMS: cachePoints(toolCalls), AttachmentsToPromptTPS: cachePoints(attachments)}
	}
	return result
}

func buildDashboardReliability(models []DashboardModelData) map[string]DashboardReliability {
	result := make(map[string]DashboardReliability, len(models))
	for _, model := range models {
		entry := DashboardReliability{ByStatus: map[string]int{}, ByFinishReason: map[string]int{}}
		for _, observation := range model.Observations {
			entry.Total++
			if observation.Outcome != nil {
				if observation.Outcome.Status != "" {
					entry.ByStatus[observation.Outcome.Status]++
				}
				if observation.Outcome.FinishReason != "" {
					entry.ByFinishReason[observation.Outcome.FinishReason]++
				}
			}
		}
		result[model.ModelID] = entry
	}
	return result
}

func latencyPoints(values map[dashboardFloatPair]int) []DashboardLatencyPoint {
	keys := sortedFloatPairs(values)
	result := make([]DashboardLatencyPoint, 0, len(keys))
	for _, key := range keys {
		result = append(result, DashboardLatencyPoint{X: key.x, MS: key.y, Count: values[key]})
	}
	return result
}

func cachePoints(values map[dashboardFloatPair]int) []DashboardCachePoint {
	keys := sortedFloatPairs(values)
	result := make([]DashboardCachePoint, 0, len(keys))
	for _, key := range keys {
		result = append(result, DashboardCachePoint{X: key.x, Y: key.y, Count: values[key]})
	}
	return result
}

func sortedFloatPairs(values map[dashboardFloatPair]int) []dashboardFloatPair {
	keys := make([]dashboardFloatPair, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].x != keys[j].x {
			return keys[i].x < keys[j].x
		}
		return keys[i].y < keys[j].y
	})
	return keys
}

func reasoningPoints(values map[dashboardReasoningKey]int) []DashboardReasoningPoint {
	keys := make([]dashboardReasoningKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].tokens != keys[j].tokens {
			return keys[i].tokens < keys[j].tokens
		}
		return keys[i].value < keys[j].value
	})
	result := make([]DashboardReasoningPoint, 0, len(keys))
	for _, key := range keys {
		result = append(result, DashboardReasoningPoint{ReasoningTokens: key.tokens, Y: key.value, Count: values[key]})
	}
	return result
}
