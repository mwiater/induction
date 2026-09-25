package eval

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeModelNameAndResultPath(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "slashes", model: " org/model:v1 ", want: "org__model__v1"},
		{name: "empty", model: "  ", want: "unknown-model"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := SafeModelName(test.model); got != test.want {
				t.Fatalf("SafeModelName(%q) = %q, want %q", test.model, got, test.want)
			}
		})
	}
	if got, want := ResultPath("/tmp/root", "org/model", "suite"), filepath.Join("/tmp/root", "data", "evals", "results", "org__model", "suite.json"); got != want {
		t.Fatalf("ResultPath() = %q, want %q", got, want)
	}
}

func TestSaveAndLoadResult(t *testing.T) {
	root := t.TempDir()
	result := &Result{Model: ModelResult{Name: "org/model"}, Suite: SuiteResult{Name: "smoke"}}
	path, err := SaveResult(root, result)
	if err != nil {
		t.Fatalf("SaveResult() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved result missing: %v", err)
	}
	loaded, err := LoadResult(root, "org/model", "smoke")
	if err != nil {
		t.Fatalf("LoadResult() error = %v", err)
	}
	if loaded.Model.Name != result.Model.Name || loaded.Suite.Name != result.Suite.Name {
		t.Fatalf("loaded result = %#v, want %#v", loaded, result)
	}
	missing, err := LoadResult(root, "other", "suite")
	if err != nil || missing != nil {
		t.Fatalf("missing LoadResult() = (%#v, %v), want (nil, nil)", missing, err)
	}
}

func TestSaveResultRejectsNilAndLoadResultRejectsMalformedJSON(t *testing.T) {
	if _, err := SaveResult(t.TempDir(), nil); err == nil {
		t.Fatal("SaveResult(nil) returned nil error")
	}
	root := t.TempDir()
	path := ResultPath(root, "model", "suite")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadResult(root, "model", "suite"); err == nil {
		t.Fatal("LoadResult() accepted malformed JSON")
	}
}
