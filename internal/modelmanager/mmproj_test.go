package modelmanager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMMProjFiles(t *testing.T) {
	files := MMProjFiles([]ModelFile{{Path: "model-Q4.gguf"}, {Path: "mmproj-model-f16.gguf"}, {Path: "docs/mmproj.txt"}, {Path: "mmproj-other.GGUF"}})
	if len(files) != 2 || files[0].Path != "mmproj-model-f16.gguf" || files[1].Path != "mmproj-other.GGUF" {
		t.Fatalf("unexpected mmproj files: %#v", files)
	}
}

func TestMMProjInstalled(t *testing.T) {
	modelsPath := t.TempDir()
	destination, err := ResolveDestination(modelsPath, "org/model", "mmproj-model-f16.gguf")
	if err != nil {
		t.Fatal(err)
	}
	if MMProjInstalled(modelsPath, "org/model", "mmproj-model-f16.gguf") {
		t.Fatal("missing projector reported as installed")
	}
	if err := os.MkdirAll(filepath.Dir(destination.Artifact), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination.Artifact, []byte("projector"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !MMProjInstalled(modelsPath, "org/model", "mmproj-model-f16.gguf") {
		t.Fatal("present projector reported as missing")
	}
}
