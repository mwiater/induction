// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PipelineItemResult is the in-memory result of one complete pipeline item.
// It deliberately contains no session, snapshot, or artifact persistence.
// PipelineItemResult is an exported type in this package's public data model.
type PipelineItemResult struct {
	// Outputs stores the value represented by this member.
	Outputs map[string]json.RawMessage
}

// PipelineStepEvent reports non-sensitive progress for one pipeline step.
// It is intentionally limited to identity, timing, and outcome; prompts and
// model responses are never included.
// PipelineStepEvent is an exported type in this package's public data model.
type PipelineStepEvent struct {
	// Index stores the value represented by this member.
	Index int
	// Total stores the value represented by this member.
	Total int
	// Name stores the value represented by this member.
	Name string
	// Model stores the value represented by this member.
	Model string
	// Started stores the value represented by this member.
	Started bool
	// Duration stores the value represented by this member.
	Duration time.Duration
	// Err stores the value represented by this member.
	Err error
}

// PipelineStepObserver receives progress for each step of one item.
// PipelineStepObserver is an exported type in this package's public data model.
type PipelineStepObserver func(PipelineStepEvent)

// RunPipelineItem executes every step for one input set in order. It shares
// the same reference rendering, attachment preparation, request construction,
// model loading, and inference path used by the console pipeline runner, but
// keeps all intermediate state in memory for callers such as appliances.
func RunPipelineItem(ctx context.Context, client *Client, pipeline *Pipeline, input InputSet) (*PipelineItemResult, error) {
	return RunPipelineItemWithObserver(ctx, client, pipeline, input, nil)
}

// RunPipelineItemWithObserver is RunPipelineItem with a non-persistent,
// non-sensitive progress callback for appliance logging and other callers.
func RunPipelineItemWithObserver(ctx context.Context, client *Client, pipeline *Pipeline, input InputSet, observer PipelineStepObserver) (*PipelineItemResult, error) {
	if client == nil || pipeline == nil {
		return nil, fmt.Errorf("client and pipeline are required")
	}
	validationPipeline := *pipeline
	if validationPipeline.Batch != nil && len(validationPipeline.Batch.Items) == 0 {
		batchCopy := *validationPipeline.Batch
		batchCopy.Items = []BatchItemConfig{{ID: "runtime-image", InputSet: InputSet{Images: []string{"runtime-image.jpg"}}}}
		validationPipeline.Batch = &batchCopy
	}
	if err := validationPipeline.Validate(); err != nil {
		return nil, err
	}
	inputs, err := preparePipelineRuntimeInputs(&input)
	if err != nil {
		return nil, err
	}
	if err := preparePipelineRuntimeContext(ctx, client, pipeline, inputs); err != nil {
		return nil, err
	}

	request := ChatRequest{}
	outputs := make(map[string]json.RawMessage, len(pipeline.Steps))
	for index, original := range pipeline.Steps {
		step := original
		stepStarted := time.Now()
		if observer != nil {
			observer(PipelineStepEvent{Index: index, Total: len(pipeline.Steps), Name: step.Name, Model: step.Model, Started: true})
		}
		finish := func(err error) {
			if observer != nil {
				observer(PipelineStepEvent{Index: index, Total: len(pipeline.Steps), Name: step.Name, Model: step.Model, Duration: time.Since(stepStarted), Err: err})
			}
		}
		if step.ForEach != "" || step.Transform != "" || step.When != nil || step.Decision != nil || step.Classification != nil {
			err := fmt.Errorf("pipeline step %q uses an unsupported appliance feature", step.Name)
			finish(err)
			return nil, err
		}
		model, err := renderPipelineReferencesFor(step.Model, outputs, inputs, nil, step.As)
		if err != nil {
			err = fmt.Errorf("pipeline step %q model: %w", step.Name, err)
			finish(err)
			return nil, err
		}
		prompt, err := renderPipelineReferencesFor(step.UserPrompt, outputs, inputs, nil, step.As)
		if err != nil {
			err = fmt.Errorf("pipeline step %q prompt: %w", step.Name, err)
			finish(err)
			return nil, err
		}
		system, err := renderPipelineReferencesFor(step.SystemPrompt, outputs, inputs, nil, step.As)
		if err != nil {
			err = fmt.Errorf("pipeline step %q system prompt: %w", step.Name, err)
			finish(err)
			return nil, err
		}

		request.Model = model
		request.ResponseFormat = step.ResponseFormat
		request.JSONSchema = step.JSONSchema
		request.Grammar = ""
		request.Temperature = nil
		request.TopP = nil
		request.TopK = nil
		request.MaxTokens = nil
		request.RepeatPenalty = nil
		request.Seed = nil
		request.MinP = nil
		request.XTCProbability = nil
		request.XTCThreshold = nil
		request.TypicalP = nil
		request.PresencePenalty = nil
		request.RepeatLastN = nil
		if step.Output != nil && strings.EqualFold(step.Output.Type, "json") {
			request.ResponseFormat = &ResponseFormat{Type: "json_object"}
			if step.Output.JSONSchema != nil {
				request.JSONSchema = step.Output.JSONSchema
			}
			request.Grammar = step.Output.Grammar
		}
		if step.Parameters != nil {
			request.Temperature = step.Parameters.Temperature
			request.TopP = step.Parameters.TopP
			request.TopK = step.Parameters.TopK
			request.MaxTokens = step.Parameters.MaxTokens
			request.RepeatPenalty = step.Parameters.RepeatPenalty
			request.Seed = step.Parameters.Seed
			request.MinP = step.Parameters.MinP
			request.XTCProbability = step.Parameters.XTCProbability
			request.XTCThreshold = step.Parameters.XTCThreshold
			request.TypicalP = step.Parameters.TypicalP
			request.PresencePenalty = step.Parameters.PresencePenalty
			request.RepeatLastN = step.Parameters.RepeatLastN
		}
		if step.Temperature != nil {
			request.Temperature = step.Temperature
		}
		if index == 0 {
			content, imageName, documentName, err := inputContent(input, step)
			if err != nil {
				err = fmt.Errorf("pipeline step %q input: %w", step.Name, err)
				finish(err)
				return nil, err
			}
			request.ImageFilename = imageName
			request.DocumentFilename = documentName
			request.Messages = append(request.Messages, Message{Role: "user", Content: content})
		} else {
			request.Messages = append(request.Messages, Message{Role: "user", Content: prompt})
		}
		if strings.TrimSpace(system) != "" {
			last := len(request.Messages) - 1
			user := request.Messages[last]
			request.Messages = request.Messages[:last]
			request.Messages = addPipelineSystemPrompt(request.Messages, system)
			request.Messages = append(request.Messages, user)
		}
		snapshot, err := client.GenerateSnapshot(ctx, &request)
		if err != nil {
			err = fmt.Errorf("pipeline step %q: %w", step.Name, err)
			finish(err)
			return nil, err
		}
		if snapshot == nil || len(snapshot.Interaction) == 0 {
			err := fmt.Errorf("pipeline step %q returned no interaction", step.Name)
			finish(err)
			return nil, err
		}
		content := recoverStructuredPipelineContent(step, snapshot.Interaction[0].Content, snapshot)
		outputs[step.Name] = json.RawMessage(append([]byte(nil), []byte(content)...))
		request.Messages = append(request.Messages, Message{Role: "assistant", Content: content})
		finish(nil)
	}
	return &PipelineItemResult{Outputs: outputs}, nil
}
