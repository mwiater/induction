// Package induction contains the core implementation for the public Induction API.
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
		if result.Steps[i].Retry != nil {
			normalizePipelineRetry(result.Steps[i].Retry)
			if err := validatePipelineRetry(result.Steps[i], base); err != nil {
				return nil, fmt.Errorf("validate pipeline %q: steps[%d]: %w", path, i, err)
			}
		}
		if result.Steps[i].Image != "" && !filepath.IsAbs(result.Steps[i].Image) && !IsRemoteSource(result.Steps[i].Image) {
			result.Steps[i].Image = filepath.Join(base, result.Steps[i].Image)
		}
		if result.Steps[i].Document != "" && !filepath.IsAbs(result.Steps[i].Document) && !IsRemoteSource(result.Steps[i].Document) {
			result.Steps[i].Document = filepath.Join(base, result.Steps[i].Document)
		}
	}
	result.FilePath = path
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("validate pipeline %q: %w", path, err)
	}
	return &result, nil
}

// Validate is an exported operation provided by this package.
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
			if len(item.Files)+len(item.Images)+len(item.Documents) == 0 && strings.TrimSpace(item.Text) == "" {
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
			case "text.concat", "text.select", "knowledgeGraph.entityCandidates", "knowledgeGraph.canonicalizeEntities", "knowledgeGraph.predicateCandidates", "knowledgeGraph.buildGraph", "knowledgeGraph.finalize":
			default:
				return fmt.Errorf("steps[%d]: unknown transform %q", i, step.Transform)
			}
			if strings.TrimSpace(step.Model) != "" || strings.TrimSpace(step.UserPrompt) != "" || step.SystemPrompt != "" || step.Parameters != nil || step.Temperature != nil || step.Classification != nil || step.Decision != nil || step.When != nil || step.ResponseFormat != nil || step.JSONSchema != nil {
				return fmt.Errorf("steps[%d]: transform steps cannot specify inference configuration", i)
			}
			if step.Image != "" || step.Document != "" {
				return fmt.Errorf("steps[%d]: transform steps cannot specify attachments", i)
			}
			if step.Quality != nil {
				return fmt.Errorf("steps[%d]: transform steps cannot specify quality", i)
			}
		} else {
			if strings.TrimSpace(step.Model) == "" {
				return fmt.Errorf("steps[%d].model is required", i)
			}
			if strings.TrimSpace(step.UserPrompt) == "" {
				return fmt.Errorf("steps[%d].userPrompt is required", i)
			}
		}
		if step.Quality != nil {
			if step.Quality.MinWords < 0 || step.Quality.MaxWords < 0 || (step.Quality.MaxWords > 0 && step.Quality.MaxWords < step.Quality.MinWords) {
				return fmt.Errorf("steps[%d].quality word bounds are invalid", i)
			}
			if step.Quality.MaxRetries < 0 || step.Quality.MaxRetries > 10 {
				return fmt.Errorf("steps[%d].quality.maxRetries must be between 0 and 10", i)
			}
			if strings.TrimSpace(step.Quality.ExactHeading) == "" && len(step.Quality.RequiredPhrases) == 0 && len(step.Quality.ForbiddenPhrases) == 0 && step.Quality.MinWords == 0 && step.Quality.MaxWords == 0 {
				return fmt.Errorf("steps[%d].quality must define at least one check", i)
			}
		}
		if step.Contract != nil && len(step.Contract) == 0 {
			return fmt.Errorf("steps[%d].contract must contain at least one criterion", i)
		}
		if step.Retry != nil {
			if err := validatePipelineRetry(step, "."); err != nil {
				return fmt.Errorf("steps[%d]: %w", i, err)
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
		if step.Decision != nil && !strings.EqualFold(strings.TrimSpace(step.Decision.Endpoint), "systemone") && step.Decision.TopLogprobs == 0 {
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
			if strings.EqualFold(strings.TrimSpace(decision.Endpoint), "systemone") {
				if err := validateSystemOneDecisionConfig(decision); err != nil {
					return fmt.Errorf("steps[%d]: %w", i, err)
				}
			} else {
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
			}
			if !strings.EqualFold(strings.TrimSpace(decision.Endpoint), "systemone") && decision.TopLogprobs < len(decision.Candidates) {
				return fmt.Errorf("steps[%d]: decision topLogprobs must be at least the number of candidates", i)
			}
			if step.Parameters != nil && step.Parameters.MaxTokens != nil && *step.Parameters.MaxTokens != 1 {
				return fmt.Errorf("steps[%d]: decision requires maxTokens=1", i)
			}
		}
		if step.When != nil {
			condition := step.When
			if strings.TrimSpace(condition.Decision) == "" {
				return fmt.Errorf("steps[%d]: when.decision and when.equals are required", i)
			}
			if strings.TrimSpace(condition.Equals) == "" && condition.MinScore == nil && condition.MaxScore == nil {
				return fmt.Errorf("steps[%d]: when.decision and when.equals are required", i)
			}
			for name, threshold := range map[string]*float64{"minConfidence": condition.MinConfidence, "minMargin": condition.MinMargin} {
				if threshold != nil && (math.IsNaN(*threshold) || math.IsInf(*threshold, 0) || *threshold < 0 || *threshold > 1) {
					return fmt.Errorf("steps[%d]: when.%s must be in [0,1]", i, name)
				}
			}
			if condition.MinScore != nil && (math.IsNaN(*condition.MinScore) || math.IsInf(*condition.MinScore, 0) || *condition.MinScore < 0 || *condition.MinScore > 10) {
				return fmt.Errorf("steps[%d]: when.minScore must be between 0 and 10", i)
			}
			if condition.MaxScore != nil && (math.IsNaN(*condition.MaxScore) || math.IsInf(*condition.MaxScore, 0) || *condition.MaxScore < 0 || *condition.MaxScore > 10) {
				return fmt.Errorf("steps[%d]: when.maxScore must be between 0 and 10", i)
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
			if condition.Equals != "" {
				known := false
				if strings.EqualFold(strings.TrimSpace(refDecision.Endpoint), "systemone") {
					_, known = refDecision.Candidates[condition.Equals]
					if !known && strings.EqualFold(strings.TrimSpace(refDecision.Type), "noul") && (condition.Equals == "true" || condition.Equals == "false") {
						known = true
					}
				} else {
					for _, value := range refDecision.Candidates {
						if value == condition.Equals {
							known = true
							break
						}
					}
				}
				if !known && refDecision.CandidatesFrom == "" {
					return fmt.Errorf("steps[%d]: when.equals %q is not a value of decision %q", i, condition.Equals, condition.Decision)
				}
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
