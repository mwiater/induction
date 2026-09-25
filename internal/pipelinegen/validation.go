package pipelinegen

import (
	"fmt"
	"regexp"
	"strings"
)

var unsafeID = regexp.MustCompile(`[^a-z0-9]+`)

// These phrases indicate an attempt to control pipeline execution or schema,
// rather than ordinary domain content. Do not reject words such as
// "document", "image", or "model" by themselves: they are common legitimate
// deliverables and domain concepts.
var forbidden = []string{
	"attachment", "image attachment", "document attachment", "nomcp",
	"depends on", "dependson", "yaml", "execute command", "run command",
	"shell command", "runtime behavior", "model:",
}

func NormalizeID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = unsafeID.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func ValidatePlan(p *DecompositionPlan) error {
	if p == nil {
		return fmt.Errorf("planner returned a nil plan")
	}
	p.Classification = strings.ToLower(strings.TrimSpace(p.Classification))
	p.Reason, p.Objective = strings.TrimSpace(p.Reason), strings.TrimSpace(p.Objective)
	if p.Classification != "simple" && p.Classification != "composite" {
		return fmt.Errorf("planner returned unsupported classification %q", p.Classification)
	}
	if p.Objective == "" {
		return fmt.Errorf("planner returned a blank objective")
	}
	if p.Reason == "" {
		return fmt.Errorf("planner returned a blank reason")
	}
	if p.Classification == "simple" && p.DecompositionRecommended {
		return fmt.Errorf("simple plan cannot recommend decomposition")
	}
	if p.Classification == "composite" && !p.DecompositionRecommended {
		return fmt.Errorf("composite plan must recommend decomposition")
	}
	if !p.DecompositionRecommended && len(p.Tasks) != 0 {
		return fmt.Errorf("non-decomposed plan must not contain tasks")
	}
	if len(p.Tasks) > 8 {
		return fmt.Errorf("planner returned %d tasks; maximum is 8", len(p.Tasks))
	}
	if p.DecompositionRecommended && len(p.Tasks) < 2 {
		return fmt.Errorf("planner returned composite decomposition with %d tasks; at least 2 are required", len(p.Tasks))
	}
	seen := map[string]bool{}
	for i := range p.Tasks {
		t := &p.Tasks[i]
		n := NormalizeID(t.ID)
		if n == "" {
			return fmt.Errorf("task %d has an unsafe or blank id", i+1)
		}
		if n == "synthesize" || n == "validate" {
			return fmt.Errorf("generated step name %q is reserved", n)
		}
		if seen[n] {
			return fmt.Errorf("duplicate task id %q after normalization", n)
		}
		seen[n] = true
		t.ID = n
		t.Name = strings.TrimSpace(t.Name)
		t.Objective = strings.TrimSpace(t.Objective)
		if t.Name == "" {
			return fmt.Errorf("task %q has a blank name", n)
		}
		if t.Objective == "" {
			return fmt.Errorf("task %q has a blank objective", n)
		}
		if len(t.OutputRequirements) == 0 || allBlank(t.OutputRequirements) {
			return fmt.Errorf("task %q has no output requirements", n)
		}
		if len(t.AcceptanceCriteria) == 0 || allBlank(t.AcceptanceCriteria) {
			return fmt.Errorf("task %q has no acceptance criteria", n)
		}
		for _, value := range append(append(append([]string{t.Name, t.Objective}, t.Inputs...), t.OutputRequirements...), t.AcceptanceCriteria...) {
			lower := strings.ToLower(value)
			for _, word := range forbidden {
				if strings.Contains(lower, word) {
					return fmt.Errorf("task %q contains unsupported generator behavior %q", n, word)
				}
			}
		}
	}
	return nil
}
func allBlank(xs []string) bool {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return false
		}
	}
	return true
}
