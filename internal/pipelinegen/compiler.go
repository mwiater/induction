package pipelinegen

import (
	"fmt"
	"regexp"
	"strings"

	induction "github.com/mwiater/induction"
)

func Compile(p *DecompositionPlan, model, originalPrompt string, includeValidation bool) (*induction.Pipeline, error) {
	if err := ValidatePlan(p); err != nil {
		return nil, err
	}
	if !p.DecompositionRecommended {
		return nil, nil
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("model is required")
	}
	pipe := &induction.Pipeline{Name: "generated-" + slug(p.Objective), Steps: make([]induction.PipelineStep, 0, len(p.Tasks)+2)}
	for i, task := range p.Tasks {
		prompt := taskPrompt(p, task, originalPrompt, i == 0)
		pipe.Steps = append(pipe.Steps, induction.PipelineStep{Name: task.ID, Model: model, SystemPrompt: TaskSystemPrompt, UserPrompt: prompt})
	}
	pipe.Steps = append(pipe.Steps, induction.PipelineStep{Name: "synthesize", Model: model, SystemPrompt: SynthesisSystemPrompt, UserPrompt: synthesisPrompt(p)})
	if includeValidation {
		pipe.Steps = append(pipe.Steps, induction.PipelineStep{Name: "validate", Model: model, SystemPrompt: ValidationSystemPrompt, UserPrompt: "[VALIDATION]\n\nValidate the synthesized answer immediately above against the original user prompt and the pipeline's stated objective, constraints, and deliverables."})
	}
	if err := pipe.Validate(); err != nil {
		return nil, fmt.Errorf("generated pipeline failed validation: %w", err)
	}
	return pipe, nil
}

func list(label string, xs []string) string {
	var b strings.Builder
	if len(xs) == 0 {
		return label + "\n- None\n"
	}
	b.WriteString(label + "\n")
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(x))
		}
	}
	return b.String()
}
func taskPrompt(p *DecompositionPlan, t DecomposedTask, original string, first bool) string {
	var b strings.Builder
	if first {
		b.WriteString("[ORIGINAL USER PROMPT]\n<USER_PROMPT>\n" + original + "\n</USER_PROMPT>\n\n")
	}
	b.WriteString("[PIPELINE CONTEXT]\n\nOriginal objective:\n" + p.Objective + "\n\n")
	b.WriteString(list("Original constraints:", p.Constraints) + "\n" + list("Requested deliverables:", p.Deliverables) + "\n")
	b.WriteString("[CURRENT TASK]\n\nTask:\n" + t.Name + "\n\nObjective:\n" + t.Objective + "\n\n" + list("Inputs:", t.Inputs) + "\n" + list("Required output:", t.OutputRequirements) + "\n" + list("Acceptance criteria:", t.AcceptanceCriteria) + "\nComplete this task only.\n")
	return b.String()
}
func synthesisPrompt(p *DecompositionPlan) string {
	return "[FINAL SYNTHESIS]\n\nUsing the original request and the completed intermediate work above, produce the final response requested by the user.\n\nOriginal objective:\n" + p.Objective + "\n\n" + list("Required deliverables:", p.Deliverables) + "\n" + list("Constraints:", p.Constraints) + "\nThe original user prompt earlier in the conversation remains the source of truth."
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonWord.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 64 {
		s = s[:64]
		s = strings.Trim(s, "-")
	}
	if s == "" {
		s = "prompt"
	}
	return s
}
