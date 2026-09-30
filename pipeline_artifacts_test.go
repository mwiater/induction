package induction

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPersistPipelineArtifact(t *testing.T) {
	workingDirectory := t.TempDir()
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	data := []byte(`{"answer":42}`)
	artifact, err := persistPipelineArtifact("run-1", "summarize", "results/output.json", "application/json", data)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ID != artifactID("run-1", "summarize", filepath.Join("results", "output.json")) {
		t.Fatalf("artifact ID = %q", artifact.ID)
	}
	if !reflect.DeepEqual(artifact.Data, data) || artifact.SHA256 == "" {
		t.Fatalf("artifact data or hash missing: %#v", artifact)
	}
	persisted, err := os.ReadFile(filepath.Join(".pipeline-artifacts", "run-1", "results", "output.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted, data) {
		t.Fatalf("persisted data = %q, want %q", persisted, data)
	}

	if _, err := persistPipelineArtifact("run-1", "summarize", "results/output.json", "application/json", data); err != nil {
		t.Fatalf("persisting identical artifact again: %v", err)
	}
	if _, err := persistPipelineArtifact("run-1", "summarize", "results/output.json", "application/json", []byte(`{"answer":43}`)); err == nil {
		t.Fatal("expected conflicting artifact contents to fail")
	}
}

func TestPersistPipelineArtifactRejectsInvalidNames(t *testing.T) {
	if _, err := persistPipelineArtifact("", "step", "output.json", "application/json", nil); err == nil {
		t.Fatal("expected empty run ID to fail")
	}
	for _, name := range []string{"..", "../outside.json", filepath.Join("nested", "..", "..", "outside.json"), filepath.Join(string(filepath.Separator), "outside.json")} {
		if _, err := persistPipelineArtifact("run", "step", name, "application/json", nil); err == nil {
			t.Errorf("expected artifact name %q to fail", name)
		}
	}
}
