// Package pipelinegen plans and compiles prompt decomposition pipelines.
package pipelinegen

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	induction "github.com/mwiater/induction"
	"gopkg.in/yaml.v3"
)

// DecompositionPlan is the planner's deliberately small intermediate representation.
type DecompositionPlan struct {
	Classification           string           `json:"classification"`
	DecompositionRecommended bool             `json:"decomposition_recommended"`
	Reason                   string           `json:"reason"`
	Objective                string           `json:"objective"`
	Constraints              []string         `json:"constraints"`
	Deliverables             []string         `json:"deliverables"`
	Tasks                    []DecomposedTask `json:"tasks"`
}

type DecomposedTask struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Objective          string   `json:"objective"`
	Inputs             []string `json:"inputs"`
	OutputRequirements []string `json:"output_requirements"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

// Planner is the only dependency needed by the generator CLI, making planning testable.
type Planner interface {
	Plan(context.Context, string, string) (*DecompositionPlan, error)
}

// LLMPlanner uses Induction's existing inference client for one structured call.
type LLMPlanner struct{ Client *induction.Client }

func (p LLMPlanner) Plan(ctx context.Context, model, prompt string) (*DecompositionPlan, error) {
	if p.Client == nil {
		return nil, fmt.Errorf("planner client is nil")
	}
	req := &induction.ChatRequest{Model: model, Messages: []induction.Message{
		{Role: "system", Content: PlannerSystemPrompt},
		{Role: "user", Content: "[ORIGINAL USER PROMPT]\n<USER_PROMPT>\n" + prompt + "\n</USER_PROMPT>"},
	}}
	// Some llama.cpp builds fail while initializing the grammar sampler for
	// json_schema, even though they support JSON-object mode. The response is
	// still a constrained structured response at the protocol level; strict
	// schema and semantic validation remain enforced below in Go.
	req.ResponseFormat = &induction.ResponseFormat{Type: "json_object"}
	snapshot, err := p.Client.GenerateSnapshot(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("planning inference: %w", err)
	}
	if snapshot == nil || len(snapshot.Interaction) == 0 {
		return nil, fmt.Errorf("planner returned no interaction")
	}
	interaction := snapshot.Interaction[0]
	for _, candidate := range []string{interaction.Content, interaction.ReasoningContent} {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		var plan DecompositionPlan
		dec := json.NewDecoder(strings.NewReader(candidate))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&plan); err == nil {
			return &plan, nil
		}
	}
	response := strings.TrimSpace(interaction.Response)
	if len(response) > 500 {
		response = response[:500] + "..."
	}
	if response == "" {
		return nil, fmt.Errorf("planner returned an empty structured response; the model may not support JSON-schema output")
	}
	return nil, fmt.Errorf("planner returned no valid structured response; server response: %s", response)
}

// Generate validates a plan and compiles it into an ordinary pipeline.
func Generate(ctx context.Context, planner Planner, model, prompt string, includeValidation bool) (*induction.Pipeline, *DecompositionPlan, error) {
	if planner == nil {
		return nil, nil, fmt.Errorf("planner is required")
	}
	plan, err := planner.Plan(ctx, model, prompt)
	if err != nil {
		return nil, nil, err
	}
	if err := ValidatePlan(plan); err != nil {
		return nil, nil, err
	}
	if !plan.DecompositionRecommended {
		return nil, plan, nil
	}
	p, err := Compile(plan, model, prompt, includeValidation)
	if err != nil {
		return nil, plan, err
	}
	return p, plan, nil
}

// WritePipeline validates and atomically writes a generated pipeline as YAML.
func WritePipeline(path string, pipeline *induction.Pipeline, force bool) error {
	if pipeline == nil {
		return fmt.Errorf("pipeline is nil")
	}
	if err := pipeline.Validate(); err != nil {
		return fmt.Errorf("generated pipeline failed validation: %w", err)
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("output path is required")
	}
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("output %q already exists; use --force to overwrite", path)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("check output %q: %w", path, err)
	}
	data, err := yaml.Marshal(pipeline)
	if err != nil {
		return fmt.Errorf("serialize generated pipeline: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".pipeline-*.yaml")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0644); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write generated pipeline: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install generated pipeline: %w", err)
	}
	return nil
}
