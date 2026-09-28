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
	Inputs *InputSet      `yaml:"inputs,omitempty"`
	Batch  *BatchConfig   `yaml:"batch,omitempty"`
	// FilePath is the source file used to load this pipeline. It is kept out
	// of the YAML representation and is used by the console UI for status.
	FilePath string `yaml:"-"`
}

// InputSet is the complete set of external inputs for one pipeline run.
// Files are intentionally generic; Images and Documents are convenience
// representations used by the current attachment adapters.
type InputSet struct {
	Files     []string `yaml:"files,omitempty" json:"files,omitempty"`
	Images    []string `yaml:"images,omitempty" json:"images,omitempty"`
	Documents []string `yaml:"documents,omitempty" json:"documents,omitempty"`
}

// BatchConfig describes independent input sets for one pipeline.
type BatchConfig struct {
	Items       []BatchItemConfig `yaml:"items" json:"items"`
	StopOnError bool              `yaml:"stop_on_error,omitempty" json:"stop_on_error,omitempty"`
}

type BatchItemConfig struct {
	ID       string `yaml:"id,omitempty" json:"id,omitempty"`
	InputSet `yaml:",inline" json:"-"`
}

// PipelineStep describes one automatically submitted turn.
type PipelineStep struct {
	Name           string                `yaml:"name"`
	Model          string                `yaml:"model"`
	UserPrompt     string                `yaml:"userPrompt"`
	SystemPrompt   string                `yaml:"systemPrompt,omitempty"`
	Image          string                `yaml:"image,omitempty"`
	Document       string                `yaml:"document,omitempty"`
	NoMCP          bool                  `yaml:"nomcp,omitempty"`
	ResponseFormat *ResponseFormat       `yaml:"responseFormat,omitempty"`
	JSONSchema     any                   `yaml:"jsonSchema,omitempty"`
	Parameters     *PipelineParameters   `yaml:"parameters,omitempty"`
	Classification *ClassificationConfig `yaml:"classification,omitempty"`
}

// ClassificationConfig configures bounded next-token classification. Each
// candidate key must be represented by exactly one model vocabulary token.
type ClassificationConfig struct {
	Candidates  map[string]string `yaml:"candidates"`
	TopLogprobs int               `yaml:"topLogprobs,omitempty"`
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
	resolveInputSetPaths(result.Inputs, base)
	if result.Batch != nil {
		for i := range result.Batch.Items {
			resolveInputSetPaths(&result.Batch.Items[i].InputSet, base)
		}
	}
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
	result.FilePath = path
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
	if p.Inputs != nil {
		if err := p.Inputs.Validate("inputs"); err != nil {
			return err
		}
	}
	if p.Batch != nil {
		if len(p.Batch.Items) == 0 {
			return fmt.Errorf("batch.items must contain at least one item")
		}
		seenIDs := make(map[string]bool, len(p.Batch.Items))
		for i := range p.Batch.Items {
			item := &p.Batch.Items[i]
			if strings.TrimSpace(item.ID) == "" {
				item.ID = item.InputSet.DerivedID(i)
			}
			if seenIDs[item.ID] {
				return fmt.Errorf("batch.items[%d] duplicates id %q", i, item.ID)
			}
			seenIDs[item.ID] = true
			if len(item.Files)+len(item.Images)+len(item.Documents) == 0 {
				return fmt.Errorf("batch.items[%d] must contain at least one input", i)
			}
		}
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
		if step.Classification != nil {
			if step.ResponseFormat != nil || step.JSONSchema != nil {
				return fmt.Errorf("steps[%d]: classification cannot be combined with responseFormat or jsonSchema", i)
			}
			if len(step.Classification.Candidates) < 2 {
				return fmt.Errorf("steps[%d]: classification requires at least two candidates", i)
			}
			for candidate, label := range step.Classification.Candidates {
				if strings.TrimSpace(candidate) == "" {
					return fmt.Errorf("steps[%d]: classification candidate key cannot be empty", i)
				}
				if strings.TrimSpace(label) == "" {
					return fmt.Errorf("steps[%d]: classification label for candidate %q cannot be empty", i, candidate)
				}
			}
			if step.Classification.TopLogprobs == 0 {
				step.Classification.TopLogprobs = 20
			}
			if step.Classification.TopLogprobs < len(step.Classification.Candidates) {
				return fmt.Errorf("steps[%d]: classification topLogprobs must be at least the number of candidates", i)
			}
			if step.Parameters != nil && step.Parameters.MaxTokens != nil && *step.Parameters.MaxTokens != 1 {
				return fmt.Errorf("steps[%d]: classification requires maxTokens=1", i)
			}
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

func resolveInputSetPaths(input *InputSet, base string) {
	if input == nil {
		return
	}
	set := input
	for _, values := range [][]string{set.Files, set.Images, set.Documents} {
		for i, value := range values {
			if value != "" && !filepath.IsAbs(value) {
				values[i] = filepath.Join(base, value)
			}
		}
	}
}

func (s InputSet) Validate(label string) error {
	if len(s.Files)+len(s.Images)+len(s.Documents) == 0 {
		return fmt.Errorf("%s must contain at least one input", label)
	}
	for _, path := range append(append(append([]string{}, s.Files...), s.Images...), s.Documents...) {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%s contains an empty input path", label)
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("%s input %q: %w", label, path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("%s input %q is a directory", label, path)
		}
		if info.Size() == 0 {
			return fmt.Errorf("%s input %q is empty", label, path)
		}
	}
	return nil
}

func (s InputSet) DerivedID(index int) string {
	paths := append(append(append([]string{}, s.Files...), s.Images...), s.Documents...)
	if len(paths) > 0 {
		return filepath.ToSlash(filepath.Clean(paths[0]))
	}
	return fmt.Sprintf("item-%03d", index+1)
}
