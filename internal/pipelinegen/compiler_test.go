package pipelinegen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mwiater/induction"
)

type staticPlanner struct{ plan *DecompositionPlan }

func (s staticPlanner) Plan(context.Context, string, string) (*DecompositionPlan, error) {
	return s.plan, nil
}

func validPlan() *DecompositionPlan {
	return &DecompositionPlan{Classification: "composite", DecompositionRecommended: true, Reason: "multiple useful stages", Objective: "Produce a design", Deliverables: []string{"A design"}, Tasks: []DecomposedTask{
		{ID: "Analyze Requirements", Name: "Analyze requirements", Objective: "Identify requirements", OutputRequirements: []string{"Requirements"}, AcceptanceCriteria: []string{"Concrete"}},
		{ID: "Design Solution", Name: "Design solution", Objective: "Design the solution", Inputs: []string{"Requirements"}, OutputRequirements: []string{"Design"}, AcceptanceCriteria: []string{"Coherent"}},
	}}
}

func TestCompileProducesLoadablePipeline(t *testing.T) {
	p, err := Compile(validPlan(), "test-model", "Original request", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 4 || p.Steps[0].Name != "analyze-requirements" || p.Steps[3].Name != "validate" {
		t.Fatalf("unexpected steps: %#v", p.Steps)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "generated.yaml")
	if err := WritePipeline(path, p, false); err != nil {
		t.Fatal(err)
	}
	loaded, err := induction.LoadPipeline(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Steps[0].Model != "test-model" || !strings.Contains(loaded.Steps[0].UserPrompt, "<USER_PROMPT>\nOriginal request") {
		t.Fatalf("unexpected loaded pipeline: %#v", loaded.Steps[0])
	}
	if err := WritePipeline(path, p, false); err == nil {
		t.Fatal("expected overwrite refusal")
	}
	if err := WritePipeline(path, p, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestSimplePlanDoesNotCompile(t *testing.T) {
	p := &DecompositionPlan{Classification: "simple", Reason: "one task", Objective: "Explain it", Deliverables: []string{"Explanation"}}
	compiled, plan, err := Generate(context.Background(), staticPlanner{p}, "model", "prompt", false)
	if err != nil || compiled != nil || plan != p {
		t.Fatalf("unexpected result: pipeline=%v plan=%v err=%v", compiled, plan, err)
	}
}

func TestValidatePlanRejectsReservedAndContradictoryOutput(t *testing.T) {
	p := validPlan()
	p.Tasks[0].ID = "synthesize"
	if err := ValidatePlan(p); err == nil {
		t.Fatal("expected reserved name rejection")
	}
	p = validPlan()
	p.Classification = "simple"
	if err := ValidatePlan(p); err == nil {
		t.Fatal("expected contradictory plan rejection")
	}
}

func TestValidatePlanAllowsOrdinaryDocumentDeliverable(t *testing.T) {
	p := validPlan()
	p.Tasks[0].OutputRequirements = []string{"A structured document of requirements"}
	if err := ValidatePlan(p); err != nil {
		t.Fatalf("ordinary document deliverable was rejected: %v", err)
	}
}
