// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Pipeline describes an ordered sequence of chat turns.
// Pipeline is an exported type in this package's public data model.
type Pipeline struct {
	// Name stores the value represented by this member.
	Name string `yaml:"name"`
	// Config stores the value represented by this member.
	Config string `yaml:"config,omitempty"`
	// Prompt stores the value represented by this member.
	Prompt string `yaml:"prompt,omitempty"`
	// Steps stores the value represented by this member.
	Steps []PipelineStep `yaml:"steps"`
	// Inputs stores the value represented by this member.
	Inputs *InputSet `yaml:"inputs,omitempty"`
	// Batch stores the value represented by this member.
	Batch *BatchConfig `yaml:"batch,omitempty"`
	// FilePath is the source file used to load this pipeline. It is kept out
	// of the YAML representation and is used by the console UI for status.
	FilePath string `yaml:"-"`
}

// InputSet is the complete set of external inputs for one pipeline run.
// Files are intentionally generic; Images and Documents are convenience
// representations used by the current attachment adapters.
// InputSet is an exported type in this package's public data model.
type InputSet struct {
	// Files stores the value represented by this member.
	Files []string `yaml:"files,omitempty" json:"files,omitempty"`
	// Images stores the value represented by this member.
	Images []string `yaml:"images,omitempty" json:"images,omitempty"`
	// Documents stores the value represented by this member.
	Documents []string `yaml:"documents,omitempty" json:"documents,omitempty"`
	// Text stores the value represented by this member.
	Text string `yaml:"text,omitempty" json:"text,omitempty"`
}

// BatchConfig describes independent input sets for one pipeline.
// BatchConfig is an exported type in this package's public data model.
type BatchConfig struct {
	// Items stores the value represented by this member.
	Items []BatchItemConfig `yaml:"items" json:"items"`
	// StopOnError stores the value represented by this member.
	StopOnError bool `yaml:"stop_on_error,omitempty" json:"stop_on_error,omitempty"`
}

// BatchItemConfig is an exported type in this package's public data model.
type BatchItemConfig struct {
	// ID stores the value represented by this member.
	ID       string `yaml:"id,omitempty" json:"id,omitempty"`
	InputSet `yaml:",inline" json:"-"`
}

// PipelineStep describes one automatically submitted turn.
// PipelineStep is an exported type in this package's public data model.
type PipelineStep struct {
	// Name stores the value represented by this member.
	Name string `yaml:"name"`
	// Model stores the value represented by this member.
	Model string `yaml:"model"`
	// UserPrompt stores the value represented by this member.
	UserPrompt string `yaml:"userPrompt"`
	// SystemPrompt stores the value represented by this member.
	SystemPrompt string `yaml:"systemPrompt,omitempty"`
	// Image stores the value represented by this member.
	Image string `yaml:"image,omitempty"`
	// Document stores the value represented by this member.
	Document string `yaml:"document,omitempty"`
	// UseMCP stores the value represented by this member.
	UseMCP bool `yaml:"usemcp,omitempty"`
	// ResponseFormat stores the value represented by this member.
	ResponseFormat *ResponseFormat `yaml:"responseFormat,omitempty"`
	// JSONSchema stores the value represented by this member.
	JSONSchema any `yaml:"jsonSchema,omitempty"`
	// Parameters stores the value represented by this member.
	Parameters *PipelineParameters `yaml:"parameters,omitempty"`
	// Temperature stores the value represented by this member.
	Temperature *float64 `yaml:"temperature,omitempty"`
	// Classification stores the value represented by this member.
	Classification *ClassificationConfig `yaml:"classification,omitempty"`
	// Decision stores the value represented by this member.
	Decision *DecisionConfig `yaml:"decision,omitempty"`
	// When stores the value represented by this member.
	When *WhenCondition `yaml:"when,omitempty"`
	// Output stores the value represented by this member.
	Output *StepOutputConfig `yaml:"output,omitempty"`
	// Quality stores the value represented by this member.
	Quality *PipelineQuality `yaml:"quality,omitempty"`
	// Retry stores the value represented by this member.
	Retry *PipelineRetry `yaml:"retry,omitempty"`
	// Transform stores the value represented by this member.
	Transform string `yaml:"transform,omitempty"`
	// Input stores the value represented by this member.
	Input map[string]any `yaml:"input,omitempty"`
	// Contract stores the value represented by this member.
	Contract []string `yaml:"contract,omitempty"`
	// ForEach stores the value represented by this member.
	ForEach string `yaml:"forEach,omitempty"`
	// As stores the value represented by this member.
	As string `yaml:"as,omitempty"`
}

// PipelineQuality is a lightweight contract for generated text. Failed
// contracts are retried by the pipeline runner before the output is exposed
// to later steps or persisted as an artifact.
// PipelineQuality is an exported type in this package's public data model.
type PipelineQuality struct {
	// MinWords stores the value represented by this member.
	MinWords int `yaml:"minWords,omitempty"`
	// MaxWords stores the value represented by this member.
	MaxWords int `yaml:"maxWords,omitempty"`
	// ExactHeading stores the value represented by this member.
	ExactHeading string `yaml:"exactHeading,omitempty"`
	// RequiredPhrases stores the value represented by this member.
	RequiredPhrases []string `yaml:"requiredPhrases,omitempty"`
	// ForbiddenPhrases stores the value represented by this member.
	ForbiddenPhrases []string `yaml:"forbiddenPhrases,omitempty"`
	// MaxRetries stores the value represented by this member.
	MaxRetries int `yaml:"maxRetries,omitempty"`
}

// StepOutputConfig describes a persisted structured result. The legacy
// responseFormat/jsonSchema fields remain supported for existing pipelines.
// StepOutputConfig is an exported type in this package's public data model.
type StepOutputConfig struct {
	// Type stores the value represented by this member.
	Type string `yaml:"type,omitempty" json:"type,omitempty"`
	// Artifact stores the value represented by this member.
	Artifact string `yaml:"artifact,omitempty" json:"artifact,omitempty"`
	// Path is an accepted alias for Artifact, matching the source-aware
	// artifact path terminology. Artifact remains the canonical field used by
	// the runtime and existing pipeline files.
	Path string `yaml:"path,omitempty" json:"path,omitempty"`
	// Grammar stores the value represented by this member.
	Grammar string `yaml:"grammar,omitempty" json:"grammar,omitempty"`
	// JSONSchema stores the value represented by this member.
	JSONSchema map[string]any `yaml:"jsonSchema,omitempty" json:"jsonSchema,omitempty"`
}

// ClassificationConfig configures bounded next-token classification. Each
// candidate key must be represented by exactly one model vocabulary token.
// ClassificationConfig is an exported type in this package's public data model.
type ClassificationConfig struct {
	// Candidates stores the value represented by this member.
	Candidates map[string]string `yaml:"candidates"`
	// TopLogprobs stores the value represented by this member.
	TopLogprobs int `yaml:"topLogprobs,omitempty"`
}

// DecisionConfig configures a bounded next-token decision.
// DecisionConfig is an exported type in this package's public data model.
type DecisionConfig struct {
	// Candidates stores the value represented by this member.
	Candidates map[string]string `yaml:"candidates"`
	// CandidatesFrom stores the value represented by this member.
	CandidatesFrom string `yaml:"candidatesFrom,omitempty"`
	// Criteria stores the value represented by this member.
	Criteria any `yaml:"criteria,omitempty"`
	// TopLogprobs stores the value represented by this member.
	TopLogprobs int `yaml:"topLogprobs,omitempty"`
	// Endpoint selects the decision protocol. The default is the legacy
	// chat-completions token/logprob implementation; systemone uses the
	// native llama.cpp TypeSafe-compatible decision endpoint.
	Endpoint string `yaml:"endpoint,omitempty"`
	// Type stores the value represented by this member.
	Type string `yaml:"type,omitempty"`
	// State stores the value represented by this member.
	State any `yaml:"state,omitempty"`
	// Instructions stores the value represented by this member.
	Instructions string `yaml:"instructions,omitempty"`
	// QuestionID stores the value represented by this member.
	QuestionID string `yaml:"questionID,omitempty"`
}

// WhenCondition gates a pipeline step on an earlier decision result.
// WhenCondition is an exported type in this package's public data model.
type WhenCondition struct {
	// Decision stores the value represented by this member.
	Decision string `yaml:"decision"`
	// Equals stores the value represented by this member.
	Equals string `yaml:"equals"`
	// MinConfidence stores the value represented by this member.
	MinConfidence *float64 `yaml:"minConfidence,omitempty"`
	// MinMargin stores the value represented by this member.
	MinMargin *float64 `yaml:"minMargin,omitempty"`
	// MinScore stores the value represented by this member.
	MinScore *float64 `yaml:"minScore,omitempty"`
	// MaxScore stores the value represented by this member.
	MaxScore *float64 `yaml:"maxScore,omitempty"`
}

// effectiveDecision performs the internal operation described by its signature.
func effectiveDecision(step PipelineStep) *DecisionConfig {
	if step.Decision != nil {
		return step.Decision
	}
	if step.Classification != nil {
		return &DecisionConfig{Candidates: step.Classification.Candidates, TopLogprobs: step.Classification.TopLogprobs}
	}
	return nil
}

// pipelineDecisionCandidates performs the internal operation described by its signature.
func pipelineDecisionCandidates(value any) (map[string]string, error) {
	result := make(map[string]string)
	switch catalog := value.(type) {
	case map[string]string:
		for id, description := range catalog {
			result[id] = description
		}
	case map[string]any:
		for id, metadata := range catalog {
			if description, ok := metadata.(string); ok {
				result[id] = description
				continue
			}
			encoded, err := json.Marshal(metadata)
			if err != nil {
				return nil, err
			}
			result[id] = string(encoded)
		}
	default:
		return nil, fmt.Errorf("expected a model catalog object, got %T", value)
	}
	if len(result) < 2 {
		return nil, fmt.Errorf("model catalog contains fewer than two candidates")
	}
	return result, nil
}

// evaluatePipelineCondition performs the internal operation described by its signature.
func evaluatePipelineCondition(condition *WhenCondition, outputs map[string]json.RawMessage) (bool, error) {
	if condition == nil {
		return true, nil
	}
	raw, ok := outputs[condition.Decision]
	if !ok {
		return false, fmt.Errorf("when references decision %q with no result", condition.Decision)
	}
	var result DecisionResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return false, fmt.Errorf("decode decision %q result: %w", condition.Decision, err)
	}
	if result.Type != "decision" {
		return false, fmt.Errorf("step %q did not produce a decision result", condition.Decision)
	}
	if condition.Equals != "" && result.SelectedValue != condition.Equals {
		return false, nil
	}
	if condition.MinConfidence != nil && result.Confidence < *condition.MinConfidence {
		return false, nil
	}
	if condition.MinMargin != nil && result.Margin < *condition.MinMargin {
		return false, nil
	}
	if condition.MinScore != nil {
		score, err := strconv.ParseFloat(result.SelectedValue, 64)
		if err != nil || score < *condition.MinScore {
			return false, nil
		}
	}
	if condition.MaxScore != nil {
		score, err := strconv.ParseFloat(result.SelectedValue, 64)
		if err != nil || score > *condition.MaxScore {
			return false, nil
		}
	}
	return true, nil
}

// PipelineParameters contains the generation parameter overrides supported by
// the inference CLI. Nil fields preserve the model/server defaults.
// PipelineParameters is an exported type in this package's public data model.
type PipelineParameters struct {
	// Temperature stores the value represented by this member.
	Temperature *float64 `yaml:"temperature,omitempty"`
	// TopP stores the value represented by this member.
	TopP *float64 `yaml:"topP,omitempty"`
	// TopK stores the value represented by this member.
	TopK *int `yaml:"topK,omitempty"`
	// MaxTokens stores the value represented by this member.
	MaxTokens *int `yaml:"maxTokens,omitempty"`
	// RepeatPenalty stores the value represented by this member.
	RepeatPenalty *float64 `yaml:"repeatPenalty,omitempty"`
	// Seed stores the value represented by this member.
	Seed *int `yaml:"seed,omitempty"`
	// MinP stores the value represented by this member.
	MinP *float64 `yaml:"minP,omitempty"`
	// XTCProbability stores the value represented by this member.
	XTCProbability *float64 `yaml:"xtcProbability,omitempty"`
	// XTCThreshold stores the value represented by this member.
	XTCThreshold *float64 `yaml:"xtcThreshold,omitempty"`
	// TypicalP stores the value represented by this member.
	TypicalP *float64 `yaml:"typicalP,omitempty"`
	// PresencePenalty stores the value represented by this member.
	PresencePenalty *float64 `yaml:"presencePenalty,omitempty"`
	// FrequencyPenalty stores the value represented by this member.
	FrequencyPenalty *float64 `yaml:"frequencyPenalty,omitempty"`
	// RepeatLastN stores the value represented by this member.
	RepeatLastN *int `yaml:"repeatLastN,omitempty"`
}
