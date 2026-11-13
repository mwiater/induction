package induction

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPurgeInferenceDataRemovesContentsAndPreservesDirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range inferenceDataDirectories {
		directory := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Join(directory, "nested"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "nested", "data.json"), []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	result, err := purgeInferenceDataFromRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Directories != len(inferenceDataDirectories) || result.Entries != len(inferenceDataDirectories) {
		t.Fatalf("unexpected purge result: %+v", result)
	}
	for _, name := range inferenceDataDirectories {
		entries, err := os.ReadDir(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("%s still contains entries: %v", name, entries)
		}
	}
}

func TestPurgeInferenceDataIgnoresMissingDirectories(t *testing.T) {
	result, err := purgeInferenceDataFromRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if result != (InferenceDataPurgeResult{}) {
		t.Fatalf("unexpected purge result: %+v", result)
	}
}
