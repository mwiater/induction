package inspect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildArgs(t *testing.T) {
	got := Runner{}.BuildArgs(Command{Task: "inspect_evals/arc_easy", Model: "Foo", Limit: 5, Temperature: 0, MaxTokens: 512, LogDir: "logs"})
	want := []string{"eval", "inspect_evals/arc_easy", "--model", "openai-api/induction/Foo", "--limit", "5", "--temperature", "0", "--max-tokens", "512", "--checkpoint=turn:1", "--log-format", "json", "--display", "plain", "--log-buffer", "1", "--log-dir", "logs"}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%v", got)
		}
	}
}

func TestBuildArgsTaskArgsAreDeterministic(t *testing.T) {
	got := Runner{}.BuildArgs(Command{Task: "inspect_evals/mmlu_0_shot", Model: "Foo", TaskArgs: map[string]string{
		"max_non_cot_tokens": "4096",
		"cot":                "false",
	}})
	want := []string{"eval", "inspect_evals/mmlu_0_shot", "--model", "openai-api/induction/Foo", "--temperature", "0", "-T", "cot=false", "-T", "max_non_cot_tokens=4096", "--checkpoint=turn:1", "--log-format", "json", "--display", "plain", "--log-buffer", "1"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestBuildArgsHumanEvalUsesLocalSandbox(t *testing.T) {
	got := Runner{}.BuildArgs(Command{Task: "inspect_evals/humaneval", Model: "Foo"})
	for i := 0; i < len(got)-1; i++ {
		if got[i] == "--sandbox" && got[i+1] == "local" {
			return
		}
	}
	t.Fatalf("HumanEval args do not select the local sandbox: %v", got)
}

func TestBuildArgsUsesLimitRangeForContinuation(t *testing.T) {
	got := Runner{}.BuildArgs(Command{Task: "inspect_evals/arc_easy", Model: "Foo", Start: 5, Limit: 50})
	for i := 0; i < len(got)-1; i++ {
		if got[i] == "--limit" && got[i+1] == "5-50" {
			return
		}
	}
	t.Fatalf("continuation args do not use the requested range: %v", got)
}

func TestMergeEnvOverridesExistingValues(t *testing.T) {
	got := mergeEnv([]string{"PATH=/system", "A=old"}, []string{"PATH=/shim", "A=new", "B=added"})
	want := []string{"PATH=/shim", "A=new", "B=added"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestRunnerRetry(t *testing.T) {
	called := false
	r := Runner{LookPath: func(string) (string, error) { return "/inspect", nil }, RunCommand: func(_ context.Context, _ string, args, _ []string) ([]byte, []byte, error) {
		called = true
		want := []string{"eval-retry", "logs/partial.json", "--checkpoint=turn:1", "--display", "plain", "--log-buffer", "1", "--log-dir", "logs"}
		if len(args) != len(want) {
			t.Fatalf("%v", args)
		}
		for i := range args {
			if args[i] != want[i] {
				t.Fatalf("%v", args)
			}
		}
		return nil, nil, nil
	}}
	if _, err := r.Retry(context.Background(), "logs/partial.json", nil); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("retry command was not called")
	}
}

func TestProgress(t *testing.T) {
	if done, total, ok := Progress("Samples 3/5 completed"); !ok || done != 3 || total != 5 {
		t.Fatalf("%d/%d %v", done, total, ok)
	}
	for _, input := range []string{"", "3/0", "6/5", "not progress"} {
		if _, _, ok := Progress(input); ok {
			t.Errorf("Progress(%q) unexpectedly matched", input)
		}
	}
}

func TestCheckAvailableAndFindLatestLog(t *testing.T) {
	r := Runner{LookPath: func(string) (string, error) { return "", errors.New("missing") }}
	if _, err := r.CheckAvailable(context.Background()); err == nil {
		t.Fatal("missing inspect should fail")
	}
	r = Runner{LookPath: func(string) (string, error) { return "/inspect", nil }, RunCommand: func(context.Context, string, []string, []string) ([]byte, []byte, error) {
		return []byte("Inspect 1.2\n"), nil, nil
	}}
	if got, err := r.CheckAvailable(context.Background()); err != nil || got != "Inspect 1.2" {
		t.Fatalf("CheckAvailable = %q, %v", got, err)
	}
	dir := t.TempDir()
	old := filepath.Join(dir, "old.eval")
	newer := filepath.Join(dir, "new.json")
	if err := os.WriteFile(old, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-time.Minute)
	if err := os.Chtimes(old, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if got, err := FindLatestLog(dir); err != nil || got != newer {
		t.Fatalf("FindLatestLog = %q, %v", got, err)
	}
	if _, err := FindLatestLog(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing log directory should fail")
	}
}
func TestRunnerEnvironment(t *testing.T) {
	r := Runner{LookPath: func(string) (string, error) { return "/inspect", nil }, RunCommand: func(_ context.Context, _ string, args, env []string) ([]byte, []byte, error) {
		if args[0] != "eval" {
			t.Fatal(args)
		}
		if len(env) != 2 {
			t.Fatal(env)
		}
		return nil, nil, nil
	}}
	if _, e := r.Run(context.Background(), Command{Task: "inspect_evals/x", Model: "m", LogDir: t.TempDir()}, []string{"INDUCTION_BASE_URL=x", "INDUCTION_API_KEY=y"}); e != nil {
		t.Fatal(e)
	}
}
