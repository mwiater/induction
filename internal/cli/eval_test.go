package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mwiater/induction/internal/eval"
	"github.com/spf13/cobra"
)

func TestEvalCommandRequiresFlags(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"eval"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected missing flag error")
	}
	root = NewRootCommand()
	root.SetArgs([]string{"eval", "--eval-config", "missing.yaml"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected missing model error")
	}
}

func TestEvalAndModelCommandsRequireBetaAcknowledgement(t *testing.T) {
	root := NewRootCommand()
	for _, name := range []string{"eval", "models"} {
		command, _, err := root.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		assertBetaTree(t, command)
	}

	root = NewRootCommand()
	root.SetArgs([]string{"eval"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "required flag") {
		t.Fatalf("eval without --beta error = %v", err)
	}
	root = NewRootCommand()
	root.SetArgs([]string{"models", "list"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "required flag") {
		t.Fatalf("models list without --beta error = %v", err)
	}
}

func assertBetaTree(t *testing.T, command *cobra.Command) {
	t.Helper()
	flag := command.Flags().Lookup("beta")
	values, marked := []string(nil), false
	if flag != nil {
		values, marked = flag.Annotations[cobra.BashCompOneRequiredFlag]
	}
	if flag == nil || !marked || len(values) != 1 || values[0] != "true" {
		t.Fatalf("%s does not have a required --beta flag", command.CommandPath())
	}
	for _, child := range command.Commands() {
		assertBetaTree(t, child)
	}
}

func TestRenderEvalStatus(t *testing.T) {
	suite := &eval.Config{
		Name: "suite",
		Evals: []eval.Definition{
			{Name: "finished", Task: "inspect_evals/finished", Limit: 2},
			{Name: "partial", Task: "inspect_evals/partial", Limit: 4},
			{Name: "missing", Task: "inspect_evals/missing", Limit: 1},
		},
	}
	result := &eval.Result{Benchmarks: []eval.BenchmarkResult{
		{Name: "finished", Task: "inspect_evals/finished", Limit: 2, Samples: 2},
		{Name: "partial", Task: "inspect_evals/partial", Limit: 4, Samples: 1},
	}}
	var output bytes.Buffer
	if err := renderEvalStatus(&output, suite, "model", result); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{
		"Evaluation Status",
		"finished  complete  2        2",
		"partial   missing   1        4",
		"missing   missing   0        1",
		"Complete: 1/3",
		"Missing: 2",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("status output missing %q:\n%s", want, text)
		}
	}
}
