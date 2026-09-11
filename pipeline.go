package induction

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Pipeline describes an ordered sequence of chat turns.
type Pipeline struct {
	Name   string         `yaml:"name"`
	Config string         `yaml:"config,omitempty"`
	Steps  []PipelineStep `yaml:"steps"`
}

// PipelineStep describes one automatically submitted turn.
type PipelineStep struct {
	Name           string              `yaml:"name"`
	Model          string              `yaml:"model"`
	UserPrompt     string              `yaml:"userPrompt"`
	SystemPrompt   string              `yaml:"systemPrompt,omitempty"`
	Image          string              `yaml:"image,omitempty"`
	Document       string              `yaml:"document,omitempty"`
	NoMCP          bool                `yaml:"nomcp,omitempty"`
	ResponseFormat *ResponseFormat     `yaml:"responseFormat,omitempty"`
	JSONSchema     any                 `yaml:"jsonSchema,omitempty"`
	Parameters     *PipelineParameters `yaml:"parameters,omitempty"`
}

// PipelineParameters contains the generation parameter overrides supported by
// the inference CLI. Nil fields preserve the model/server defaults.
type PipelineParameters struct {
	Temperature   *float64 `yaml:"temperature,omitempty"`
	TopP          *float64 `yaml:"topP,omitempty"`
	TopK          *int     `yaml:"topK,omitempty"`
	MaxTokens     *int     `yaml:"maxTokens,omitempty"`
	RepeatPenalty *float64 `yaml:"repeatPenalty,omitempty"`
	Seed          *int     `yaml:"seed,omitempty"`
}

// LoadPipeline reads, validates, and normalizes a pipeline. Attachment paths
// are resolved relative to the pipeline file.
func LoadPipeline(path string) (*Pipeline, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("pipeline path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pipeline %q: %w", path, err)
	}
	var result Pipeline
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode pipeline %q: %w", path, err)
	}
	base := filepath.Dir(path)
	for i := range result.Steps {
		if result.Steps[i].Image != "" && !filepath.IsAbs(result.Steps[i].Image) {
			result.Steps[i].Image = filepath.Join(base, result.Steps[i].Image)
		}
		if result.Steps[i].Document != "" && !filepath.IsAbs(result.Steps[i].Document) {
			result.Steps[i].Document = filepath.Join(base, result.Steps[i].Document)
		}
	}
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("validate pipeline %q: %w", path, err)
	}
	return &result, nil
}

func (p *Pipeline) Validate() error {
	if p == nil {
		return fmt.Errorf("pipeline is nil")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}
	seen := make(map[string]bool, len(p.Steps))
	for i, step := range p.Steps {
		if strings.TrimSpace(step.Name) == "" {
			return fmt.Errorf("steps[%d].name is required", i)
		}
		if seen[step.Name] {
			return fmt.Errorf("steps[%d] duplicates step name %q", i, step.Name)
		}
		seen[step.Name] = true
		if strings.TrimSpace(step.Model) == "" {
			return fmt.Errorf("steps[%d].model is required", i)
		}
		if strings.TrimSpace(step.UserPrompt) == "" {
			return fmt.Errorf("steps[%d].userPrompt is required", i)
		}
		if step.Image != "" && step.Document != "" {
			return fmt.Errorf("steps[%d] cannot specify both image and document", i)
		}
		if step.ResponseFormat != nil {
			switch strings.ToLower(strings.TrimSpace(step.ResponseFormat.Type)) {
			case "json", "json_object", "json_schema", "text":
			default:
				return fmt.Errorf("steps[%d].responseFormat.type %q is unsupported", i, step.ResponseFormat.Type)
			}
		}
		if step.JSONSchema != nil {
			if step.ResponseFormat == nil {
				return fmt.Errorf("steps[%d].jsonSchema requires responseFormat", i)
			}
			typeName := strings.ToLower(strings.TrimSpace(step.ResponseFormat.Type))
			if typeName != "json" && typeName != "json_object" && typeName != "json_schema" {
				return fmt.Errorf("steps[%d].jsonSchema requires a JSON responseFormat", i)
			}
			encoded, err := json.Marshal(step.JSONSchema)
			if err != nil {
				return fmt.Errorf("steps[%d].jsonSchema is not valid JSON: %w", i, err)
			}
			var schema map[string]any
			if err := json.Unmarshal(encoded, &schema); err != nil || schema == nil {
				return fmt.Errorf("steps[%d].jsonSchema must be a JSON object", i)
			}
		}
		if i > 0 && step.Image != "" {
			return fmt.Errorf("steps[%d].image is only allowed on the first pipeline step", i)
		}
		if i > 0 && step.Document != "" {
			return fmt.Errorf("steps[%d].document is only allowed on the first pipeline step", i)
		}
		for label, path := range map[string]string{"image": step.Image, "document": step.Document} {
			if path != "" {
				if _, err := os.Stat(path); err != nil {
					return fmt.Errorf("steps[%d].%s %q: %w", i, label, path, err)
				}
			}
		}
	}
	return nil
}
