package induction

import (
	"encoding/json"
	"fmt"
	"math"
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
	Temperature    *float64              `yaml:"temperature,omitempty"`
	Classification *ClassificationConfig `yaml:"classification,omitempty"`
	Decision       *DecisionConfig       `yaml:"decision,omitempty"`
	When           *WhenCondition        `yaml:"when,omitempty"`
	Output         *StepOutputConfig     `yaml:"output,omitempty"`
	Transform      string                `yaml:"transform,omitempty"`
	Input          map[string]any        `yaml:"input,omitempty"`
	ForEach        string                `yaml:"forEach,omitempty"`
	As             string                `yaml:"as,omitempty"`
}

// StepOutputConfig describes a persisted structured result. The legacy
// responseFormat/jsonSchema fields remain supported for existing pipelines.
type StepOutputConfig struct {
	Type     string `yaml:"type,omitempty" json:"type,omitempty"`
	Artifact string `yaml:"artifact,omitempty" json:"artifact,omitempty"`
	// Path is an accepted alias for Artifact, matching the source-aware
	// artifact path terminology. Artifact remains the canonical field used by
	// the runtime and existing pipeline files.
	Path       string         `yaml:"path,omitempty" json:"path,omitempty"`
	Grammar    string         `yaml:"grammar,omitempty" json:"grammar,omitempty"`
	JSONSchema map[string]any `yaml:"jsonSchema,omitempty" json:"jsonSchema,omitempty"`
}

// ClassificationConfig configures bounded next-token classification. Each
// candidate key must be represented by exactly one model vocabulary token.
type ClassificationConfig struct {
	Candidates  map[string]string `yaml:"candidates"`
	TopLogprobs int               `yaml:"topLogprobs,omitempty"`
}

// DecisionConfig configures a bounded next-token decision.
type DecisionConfig struct {
	Candidates  map[string]string `yaml:"candidates"`
	TopLogprobs int               `yaml:"topLogprobs,omitempty"`
}

// WhenCondition gates a pipeline step on an earlier decision result.
type WhenCondition struct {
	Decision      string   `yaml:"decision"`
	Equals        string   `yaml:"equals"`
	MinConfidence *float64 `yaml:"minConfidence,omitempty"`
	MinMargin     *float64 `yaml:"minMargin,omitempty"`
}

func effectiveDecision(step PipelineStep) *DecisionConfig {
	if step.Decision != nil {
		return step.Decision
	}
	if step.Classification != nil {
		return &DecisionConfig{Candidates: step.Classification.Candidates, TopLogprobs: step.Classification.TopLogprobs}
	}
	return nil
}

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
	if result.SelectedValue != condition.Equals {
		return false, nil
	}
	if condition.MinConfidence != nil && result.Confidence < *condition.MinConfidence {
		return false, nil
	}
	if condition.MinMargin != nil && result.Margin < *condition.MinMargin {
		return false, nil
	}
	return true, nil
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
		if result.Steps[i].Image != "" && !filepath.IsAbs(result.Steps[i].Image) && !IsRemoteSource(result.Steps[i].Image) {
			result.Steps[i].Image = filepath.Join(base, result.Steps[i].Image)
		}
		if result.Steps[i].Document != "" && !filepath.IsAbs(result.Steps[i].Document) && !IsRemoteSource(result.Steps[i].Document) {
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
				item.ID = item.DerivedID(i)
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
	positions := make(map[string]int, len(p.Steps))
	for i := range p.Steps {
		positions[p.Steps[i].Name] = i
	}
	for i, step := range p.Steps {
		if strings.TrimSpace(step.Name) == "" {
			return fmt.Errorf("steps[%d].name is required", i)
		}
		if seen[step.Name] {
			return fmt.Errorf("steps[%d] duplicates step name %q", i, step.Name)
		}
		isTransform := strings.TrimSpace(step.Transform) != ""
		if isTransform {
			switch step.Transform {
			case "knowledgeGraph.entityCandidates", "knowledgeGraph.canonicalizeEntities", "knowledgeGraph.predicateCandidates", "knowledgeGraph.buildGraph", "knowledgeGraph.finalize":
			default:
				return fmt.Errorf("steps[%d]: unknown transform %q", i, step.Transform)
			}
			if strings.TrimSpace(step.Model) != "" || strings.TrimSpace(step.UserPrompt) != "" || step.SystemPrompt != "" || step.Parameters != nil || step.Temperature != nil || step.Classification != nil || step.Decision != nil || step.When != nil || step.ResponseFormat != nil || step.JSONSchema != nil {
				return fmt.Errorf("steps[%d]: transform steps cannot specify inference configuration", i)
			}
			if step.Image != "" || step.Document != "" {
				return fmt.Errorf("steps[%d]: transform steps cannot specify attachments", i)
			}
		} else {
			if strings.TrimSpace(step.Model) == "" {
				return fmt.Errorf("steps[%d].model is required", i)
			}
			if strings.TrimSpace(step.UserPrompt) == "" {
				return fmt.Errorf("steps[%d].userPrompt is required", i)
			}
		}
		if step.Transform != "" && step.ForEach != "" {
			return fmt.Errorf("steps[%d]: transform steps cannot use forEach", i)
		}
		if step.As == "" && step.ForEach != "" {
			step.As = "item"
			p.Steps[i].As = step.As
		}
		if step.Output != nil {
			if step.Output.Artifact == "" {
				step.Output.Artifact = step.Output.Path
				p.Steps[i].Output = step.Output
			} else if step.Output.Path != "" && step.Output.Path != step.Output.Artifact {
				return fmt.Errorf("steps[%d].output.artifact and output.path must match when both are set", i)
			}
			typeName := strings.ToLower(strings.TrimSpace(step.Output.Type))
			if typeName == "" {
				typeName = "text"
				step.Output.Type = typeName
			}
			if typeName != "text" && typeName != "json" {
				return fmt.Errorf("steps[%d].output.type %q is unsupported", i, step.Output.Type)
			}
			if step.Output.Grammar != "" && step.Output.JSONSchema != nil {
				return fmt.Errorf("steps[%d].output.grammar and output.jsonSchema are mutually exclusive", i)
			}
			if typeName == "json" && step.Output.Artifact == "" {
				return fmt.Errorf("steps[%d].output.artifact is required for JSON output", i)
			}
			if step.Output.Artifact != "" {
				clean := filepath.Clean(step.Output.Artifact)
				if filepath.IsAbs(step.Output.Artifact) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
					return fmt.Errorf("steps[%d].output.artifact must remain within the run artifact directory", i)
				}
			}
		}
		if step.Image != "" && step.Document != "" {
			return fmt.Errorf("steps[%d] cannot specify both image and document", i)
		}
		if step.Decision != nil && step.Classification != nil {
			return fmt.Errorf("steps[%d]: decision and classification cannot both be specified", i)
		}
		if step.Decision != nil && step.Decision.TopLogprobs == 0 {
			step.Decision.TopLogprobs = 20
			p.Steps[i].Decision = step.Decision
		}
		if step.Classification != nil && step.Classification.TopLogprobs == 0 {
			step.Classification.TopLogprobs = 20
			p.Steps[i].Classification = step.Classification
		}
		decision := effectiveDecision(step)
		if decision != nil {
			if step.ResponseFormat != nil || step.JSONSchema != nil {
				return fmt.Errorf("steps[%d]: decision cannot be combined with responseFormat or jsonSchema", i)
			}
			if len(decision.Candidates) < 2 {
				return fmt.Errorf("steps[%d]: decision requires at least two candidates", i)
			}
			for candidate, label := range decision.Candidates {
				if strings.TrimSpace(candidate) == "" {
					return fmt.Errorf("steps[%d]: decision candidate key cannot be empty", i)
				}
				if strings.TrimSpace(label) == "" {
					return fmt.Errorf("steps[%d]: decision value for candidate %q cannot be empty", i, candidate)
				}
			}
			if decision.TopLogprobs < len(decision.Candidates) {
				return fmt.Errorf("steps[%d]: decision topLogprobs must be at least the number of candidates", i)
			}
			if step.Parameters != nil && step.Parameters.MaxTokens != nil && *step.Parameters.MaxTokens != 1 {
				return fmt.Errorf("steps[%d]: decision requires maxTokens=1", i)
			}
		}
		if step.When != nil {
			condition := step.When
			if strings.TrimSpace(condition.Decision) == "" || strings.TrimSpace(condition.Equals) == "" {
				return fmt.Errorf("steps[%d]: when.decision and when.equals are required", i)
			}
			for name, threshold := range map[string]*float64{"minConfidence": condition.MinConfidence, "minMargin": condition.MinMargin} {
				if threshold != nil && (math.IsNaN(*threshold) || math.IsInf(*threshold, 0) || *threshold < 0 || *threshold > 1) {
					return fmt.Errorf("steps[%d]: when.%s must be in [0,1]", i, name)
				}
			}
			position, exists := positions[condition.Decision]
			if !exists {
				return fmt.Errorf("steps[%d]: when references unknown decision step %q", i, condition.Decision)
			}
			if position >= i {
				return fmt.Errorf("steps[%d]: when must reference an earlier decision step", i)
			}
			ref := p.Steps[position]
			refDecision := effectiveDecision(ref)
			if refDecision == nil {
				return fmt.Errorf("steps[%d]: when reference %q is not a decision step", i, condition.Decision)
			}
			known := false
			for _, value := range refDecision.Candidates {
				if value == condition.Equals {
					known = true
					break
				}
			}
			if !known {
				return fmt.Errorf("steps[%d]: when.equals %q is not a value of decision %q", i, condition.Equals, condition.Decision)
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
			if path != "" && !IsRemoteSource(path) {
				if _, err := os.Stat(path); err != nil {
					return fmt.Errorf("steps[%d].%s %q: %w", i, label, path, err)
				}
			}
		}
		if err := validatePipelineReferences(step, i, seen, positions); err != nil {
			return err
		}
		seen[step.Name] = true
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
			if value != "" && !filepath.IsAbs(value) && !IsRemoteSource(value) {
				values[i] = filepath.Join(base, value)
			}
		}
	}
}

// validatePipelineReferences deliberately accepts only the small reference
// language used by pipeline artifacts. It catches future-step references
// before Bubble Tea starts model loading.
func validatePipelineReferences(step PipelineStep, index int, prior map[string]bool, positions map[string]int) error {
	values := []string{step.UserPrompt, step.SystemPrompt, step.ForEach}
	for _, value := range values {
		for pos := 0; pos < len(value); {
			start := strings.Index(value[pos:], "{{")
			if start < 0 {
				break
			}
			start += pos
			end := strings.Index(value[start+2:], "}}")
			if end < 0 {
				return fmt.Errorf("steps[%d]: unterminated reference", index)
			}
			end += start + 2
			expr := strings.TrimSpace(value[start+2 : end])
			if expr == "item" || expr == step.As || strings.HasPrefix(expr, step.As+".") || strings.HasPrefix(expr, "inputs.documents.chunks") {
				pos = end + 2
				continue
			}
			if !strings.HasPrefix(expr, "steps.") {
				return fmt.Errorf("steps[%d]: unsupported reference %q", index, expr)
			}
			parts := strings.Split(expr, ".")
			if len(parts) < 3 || parts[2] != "output" && parts[2] != "items" {
				return fmt.Errorf("steps[%d]: unsupported reference %q", index, expr)
			}
			name := parts[1]
			if !prior[name] {
				if position, exists := positions[name]; exists && position >= index {
					return fmt.Errorf("steps[%d]: reference to future step %q", index, name)
				}
				return fmt.Errorf("steps[%d]: unknown step reference %q", index, name)
			}
			pos = end + 2
		}
	}
	for key, value := range step.Input {
		if text, ok := value.(string); ok {
			if err := validatePipelineReferences(PipelineStep{UserPrompt: text}, index, prior, positions); err != nil {
				return fmt.Errorf("steps[%d].input[%q]: %w", index, key, err)
			}
		}
	}
	return nil
}

func (s InputSet) Validate(label string) error {
	if len(s.Files)+len(s.Images)+len(s.Documents) == 0 {
		return fmt.Errorf("%s must contain at least one input", label)
	}
	for _, path := range append(append(append([]string{}, s.Files...), s.Images...), s.Documents...) {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%s contains an empty input path", label)
		}
		// HTTP(S) sources are validated when they are fetched by the
		// attachment processor. They cannot be checked with os.Stat.
		if IsRemoteSource(path) {
			continue
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
