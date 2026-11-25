// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// RunPipelineHeadless executes a pipeline without the terminal UI. It is used
// by unattended CLI workflows and preserves the same session and artifact
// boundaries as the interactive pipeline runner.
// Additional context: Benchmark answers must fit in the visible response. Reasoning models such as LFM can otherwise consume the entire output cap before emitting the required JSON object.
func RunPipelineHeadless(ctx context.Context, pipeline *Pipeline, out io.Writer, options ...ClientOption) error {
	if pipeline == nil {
		return fmt.Errorf("pipeline is nil")
	}
	if err := pipeline.Validate(); err != nil {
		return err
	}
	cfg, err := loadConfigForOptions(options)
	if err != nil {
		return err
	}
	client := newClientFromConfig(ctx, cfg, options...)
	request := ChatRequest{Model: pipeline.Steps[0].Model}
	session, err := newUnsavedChatSession(request)
	if err != nil {
		return err
	}
	session.BatchID = client.opts.batchID
	session.BatchItemID = client.opts.batchItemID
	configureSessionMode(session, pipeline)
	if err := saveChatSession(session); err != nil {
		return err
	}
	if client.opts.sessionSaved != nil {
		client.opts.sessionSaved(chatSessionPath(session.ID))
	}

	inputs, err := preparePipelineRuntimeInputs(pipeline.Inputs)
	if err != nil {
		return err
	}
	outputs := make(map[string]json.RawMessage)
	artifactContext := defaultArtifactPathContext(pipeline)
	if client.opts.artifactPathContext != nil {
		artifactContext = *client.opts.artifactPathContext
	}
	for index, configured := range pipeline.Steps {
		step := configured
		prompt, err := renderPipelineReferencesFor(step.UserPrompt, outputs, inputs, nil, step.As)
		if err != nil {
			return fmt.Errorf("pipeline step %q: %w", step.Name, err)
		}
		system, err := renderPipelineReferencesFor(step.SystemPrompt, outputs, inputs, nil, step.As)
		if err != nil {
			return fmt.Errorf("pipeline step %q: %w", step.Name, err)
		}
		request.Model = step.Model
		request.ResponseFormat = nil
		request.JSONSchema = nil
		request.Grammar = ""
		request.Temperature = nil
		request.TopP = nil
		request.TopK = nil
		request.MaxTokens = nil
		request.RepeatPenalty = nil
		request.Seed = nil
		request.Decision = nil
		request.Classification = nil
		request.ReasoningControl = nil
		if strings.TrimSpace(system) != "" {
			request.Messages = addPipelineSystemPrompt(request.Messages, system)
		}
		if step.ResponseFormat != nil {
			request.ResponseFormat = step.ResponseFormat
		}
		if step.Output != nil && strings.EqualFold(step.Output.Type, "json") {
			request.ResponseFormat = &ResponseFormat{Type: "json_object"}
			request.JSONSchema = step.Output.JSONSchema
			request.Grammar = step.Output.Grammar

			disableReasoning := false
			request.ReasoningControl = &disableReasoning
		}
		if step.JSONSchema != nil {
			request.JSONSchema = step.JSONSchema
		}
		if step.Parameters != nil {
			request.Temperature = step.Parameters.Temperature
			request.TopP = step.Parameters.TopP
			request.TopK = step.Parameters.TopK
			request.MaxTokens = step.Parameters.MaxTokens
			request.RepeatPenalty = step.Parameters.RepeatPenalty
			request.Seed = step.Parameters.Seed
		}
		if step.Temperature != nil {
			request.Temperature = step.Temperature
		}
		content := any(prompt)
		if index == 0 && pipeline.Inputs != nil {
			content, _, _, err = inputContent(*pipeline.Inputs, PipelineStep{UserPrompt: prompt})
			if err != nil {
				return fmt.Errorf("pipeline step %q: prepare inputs: %w", step.Name, err)
			}
		}
		request.Messages = append(request.Messages, Message{Role: "user", Content: content})
		turnCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Timeout))
		snapshot, inferErr := client.withoutLiveMetricsOverlay(turnCtx).GenerateSnapshot(turnCtx, &request)
		cancel()
		if inferErr != nil {
			_ = saveChatSession(session)
			return fmt.Errorf("pipeline step %q: %w", step.Name, inferErr)
		}
		if snapshot != nil {
			snapshot.ModelID = step.Model
			session.Snapshots = append(session.Snapshots, snapshot)
		}
		response := lastInteractionContent(snapshot)
		response = recoverStructuredPipelineContent(step, response, snapshot)
		request.Messages = append(request.Messages, Message{Role: "assistant", Content: response})
		session.Messages = cloneMessages(request.Messages)
		session.Model = step.Model
		updateSessionFinalOutput(session)
		if err := saveChatSession(session); err != nil {
			return err
		}
		if client.opts.sessionSaved != nil {
			client.opts.sessionSaved(chatSessionPath(session.ID))
		}
		if step.Output != nil && strings.EqualFold(step.Output.Type, "json") {
			var parsed any
			if err := json.Unmarshal([]byte(response), &parsed); err != nil || !isJSONObject(response) {
				return fmt.Errorf("pipeline step %q returned invalid JSON object", step.Name)
			}
			encoded, _ := json.Marshal(parsed)
			outputs[step.Name] = encoded
			if step.Output.Artifact != "" {
				artifactPath, err := ResolveArtifactPath(step.Output.Artifact, artifactContext)
				if err != nil {
					return fmt.Errorf("pipeline step %q: %w", step.Name, err)
				}
				if _, err := persistPipelineArtifact(session.ID, step.Name, artifactPath, "application/json", []byte(response)); err != nil {
					return err
				}
			}
		}
		if out != nil {
			_, _ = fmt.Fprintln(out, response)
		}
	}
	return nil
}
