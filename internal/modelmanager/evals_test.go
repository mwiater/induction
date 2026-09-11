package modelmanager

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfiguredTokenOverridesHFEnvironmentForCommands(t *testing.T) {
	env := commandEnvWithToken("configured-token")
	found := false
	for _, value := range env {
		if strings.HasPrefix(value, "HF_TOKEN=") {
			found = true
			if value != "HF_TOKEN=configured-token" {
				t.Fatalf("unexpected token environment: %q", value)
			}
		}
	}
	if !found {
		t.Fatal("configured HF_TOKEN was not added to command environment")
	}
}

type evalFakeHub struct {
	files map[string][]ModelFile
	meta  map[string]ModelMetadata
	data  map[string][]byte
}

func (f *evalFakeHub) Search(context.Context, string, int, string) ([]SearchResult, error) {
	return nil, nil
}
func (f *evalFakeHub) ListFiles(_ context.Context, id string) (string, []ModelFile, error) {
	return "rev-1", f.files[id], nil
}
func (f *evalFakeHub) ModelMetadata(_ context.Context, id string) (ModelMetadata, error) {
	return f.meta[id], nil
}
func (f *evalFakeHub) DownloadFile(_ context.Context, _, _, remote, dir string) (string, error) {
	path := filepath.Join(dir, filepath.FromSlash(remote))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, f.data[remote], 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func TestNormalizeModelIdentity(t *testing.T) {
	cases := map[string]string{
		"Qwen3-8B-GGUF":                     "qwen3-8b",
		"Qwen3-8B-Q4_K_M-GGUF":              "qwen3-8b",
		"Qwen3-Coder-30B-A3B-Instruct-GGUF": "qwen3-coder-30b-a3b-instruct",
		"Qwen3-8B-Instruct-GGUF":            "qwen3-8b-instruct",
		"Model-7B":                          "model-7b",
	}
	for input, want := range cases {
		if got := NormalizeModelIdentity(input); got != want {
			t.Errorf("NormalizeModelIdentity(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestEvalSearchQueryPreservesVersionPunctuation(t *testing.T) {
	if got := evalSearchQuery("GLM-4.7-Flash-GGUF"); got != "GLM-4.7-Flash" {
		t.Fatalf("eval search query = %q", got)
	}
	if got := evalSearchQuery("Qwen3-8B-Q4_K_M-GGUF"); got != "Qwen3-8B" {
		t.Fatalf("quantized eval search query = %q", got)
	}
}

func TestNormalizedIdentityUsesSharedModelTag(t *testing.T) {
	left := ModelMetadata{ID: "unsloth/GLM-4.7-Flash-GGUF", Tags: []string{"glm4_moe_lite", "gguf"}}
	right := ModelMetadata{ID: "zai-org/GLM-4.7-Flash", Tags: []string{"glm4_moe_lite", "transformers"}}
	if relationship, _ := relationship(left, right); relationship != EvalRelationshipNormalizedModel {
		t.Fatalf("relationship = %q", relationship)
	}
}

func TestEvalFilesAndPrimaryReconciliation(t *testing.T) {
	files := []ModelFile{{Path: "README.md"}, {Path: ".eval_results/z.yml"}, {Path: ".eval_results/a.yaml"}, {Path: ".eval_results/ignored.json"}, {Path: "../.eval_results/no.yaml"}}
	got := EvalFiles(files)
	if paths := []string{got[0].Path, got[1].Path}; !reflect.DeepEqual(paths, []string{".eval_results/a.yaml", ".eval_results/z.yml"}) {
		t.Fatalf("eval files = %v", paths)
	}

	base := t.TempDir()
	fake := &evalFakeHub{files: map[string][]ModelFile{"org/repo": {{Path: ".eval_results/results.yaml", Size: 12}}}, data: map[string][]byte{".eval_results/results.yaml": []byte("results:\n  - benchmark: test\n    value: 1.5\n")}}
	result, err := ReconcileEvals(context.Background(), fake, base, "org/repo")
	if err != nil || result.Status != EvalStatusCreatedPrimary {
		t.Fatalf("reconcile = %+v, %v", result, err)
	}
	destination, _ := ResolveEvalDestination(base, "org/repo")
	if _, err := os.Stat(filepath.Join(destination.SourceDir, "results.yaml")); err != nil {
		t.Fatal(err)
	}
	manifest, err := ReadEvalManifest(base, "org/repo")
	if err != nil || manifest.Authority != EvalAuthorityPrimary {
		t.Fatalf("manifest = %+v, %v", manifest, err)
	}
	result, err = ReconcileEvals(context.Background(), fake, base, "org/repo")
	if err != nil || result.Status != EvalStatusUnchanged {
		t.Fatalf("second reconcile = %+v, %v", result, err)
	}
}
