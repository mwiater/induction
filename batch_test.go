package induction

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBatchExecutionCountAndResume(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(file, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &Pipeline{Name: "test", Batch: &BatchConfig{Items: []BatchItemConfig{{ID: "a", InputSet: InputSet{Files: []string{file}}}, {ID: "b", InputSet: InputSet{Files: []string{file}}}, {ID: "c", InputSet: InputSet{Files: []string{file}}}}}, Steps: []PipelineStep{{Name: "one", Model: "m", UserPrompt: "x"}}}
	count := 0
	exec := func(_ context.Context, _ *Pipeline, _ InputSet, item *BatchItem) error {
		count++
		if item.ID == "a" {
			item.PipelineRunID = "run-a"
		}
		return nil
	}
	batch, err := RunBatch(context.Background(), p, filepath.Join(dir, "batches"), exec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 || batch.Status != BatchCompleted {
		t.Fatalf("count=%d status=%s", count, batch.Status)
	}
	_, err = RunBatch(context.Background(), p, filepath.Join(dir, "batches"), exec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("completed items reran: count=%d", count)
	}
}

func TestBatchInvalidItemDoesNotBlockValidItems(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(file, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &Pipeline{Name: "test", Batch: &BatchConfig{Items: []BatchItemConfig{{ID: "valid", InputSet: InputSet{Files: []string{file}}}, {ID: "invalid", InputSet: InputSet{Files: []string{filepath.Join(dir, "missing")}}}}}, Steps: []PipelineStep{{Name: "one", Model: "m", UserPrompt: "x"}}}
	count := 0
	batch, err := RunBatch(context.Background(), p, filepath.Join(dir, "batches"), func(context.Context, *Pipeline, InputSet, *BatchItem) error { count++; return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || batch.Status != BatchCompletedWithErrors || batch.Items[1].Status != BatchItemInvalid {
		t.Fatalf("unexpected batch: count=%d status=%s items=%+v", count, batch.Status, batch.Items)
	}
}

func TestBatchExamplesLoad(t *testing.T) {
	for _, path := range []string{
		"pipelines/pipeline.multi-image-analysis-01.yaml",
		"pipelines/pipeline.batch-image-analysis-01.yaml",
		"pipelines/pipeline.multi-document-analysis-01.yaml",
		"pipelines/pipeline.batch-document-analysis-01.yaml",
	} {
		if _, err := LoadPipeline(path); err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
	}
}
