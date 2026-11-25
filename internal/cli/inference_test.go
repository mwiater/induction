package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	induction "github.com/mwiater/induction"
)

func TestPrepareDocuments(t *testing.T) {
	dir := t.TempDir()
	fixture, err := os.ReadFile("../../data/fixtures/documents/fixture-01.pdf")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"b.pdf", "a.PDF"} {
		if err := os.WriteFile(filepath.Join(dir, name), fixture, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}

	filename, text, err := prepareDocuments(dir)
	if err != nil {
		t.Fatal(err)
	}
	if filename != "a.PDF, b.pdf" {
		t.Fatalf("filenames = %q, want sorted PDF filenames", filename)
	}
	if strings.Count(text, "Extracted document text:") != 2 {
		t.Fatalf("combined text = %q, want one labeled section per PDF", text)
	}
}

func TestPrepareDocumentsRejectsNonDirectoryAndEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := prepareDocuments(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected missing directory to fail")
	}
	if _, _, err := prepareDocuments(dir); err == nil || !strings.Contains(err.Error(), "no PDF files") {
		t.Fatalf("empty directory error = %v", err)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareDocuments(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("file path error = %v", err)
	}
}

func TestDocumentFlagsAreMutuallyExclusive(t *testing.T) {
	err := runInference(context.Background(), inferenceFlags{model: "model", document: "one.pdf", documents: "docs"}, strings.NewReader(""), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "--document and --documents") {
		t.Fatalf("document conflict error = %v", err)
	}
}

func TestPipelineInitialPromptPrecedence(t *testing.T) {
	pipeline := &induction.Pipeline{
		Prompt: " embedded prompt ",
		Steps:  []induction.PipelineStep{{UserPrompt: "first step prompt"}},
	}
	if got := pipelineInitialPrompt(pipeline, "flag prompt"); got != "embedded prompt" {
		t.Fatalf("embedded prompt = %q", got)
	}
	pipeline.Prompt = ""
	if got := pipelineInitialPrompt(pipeline, " flag prompt "); got != "flag prompt" {
		t.Fatalf("flag prompt = %q", got)
	}
	if got := pipelineInitialPrompt(pipeline, ""); got != "first step prompt" {
		t.Fatalf("first step prompt = %q", got)
	}
}

func TestApplyPipelineModelOverrides(t *testing.T) {
	decision := &induction.DecisionConfig{Candidates: map[string]string{"A": "one", "B": "two"}}
	classification := &induction.ClassificationConfig{Candidates: map[string]string{"A": "one", "B": "two"}}
	pipeline := &induction.Pipeline{Steps: []induction.PipelineStep{
		{Name: "ordinary", Model: "repository-generation-model"},
		{Name: "templated-ordinary", Model: "{{ steps.selector.output }}"},
		{Name: "decision", Model: "repository-decision-model", Decision: decision},
		{Name: "classification", Model: "{{ steps.selector.output }}", Classification: classification},
	}}

	applyPipelineModelOverrides(pipeline, "user-generation-model", "user-decision-model")

	if got := pipeline.Steps[0].Model; got != "user-generation-model" {
		t.Fatalf("ordinary model = %q, want user-generation-model", got)
	}
	if got := pipeline.Steps[1].Model; got != "{{ steps.selector.output }}" {
		t.Fatalf("templated ordinary model = %q, want template preserved", got)
	}
	if got := pipeline.Steps[2].Model; got != "user-decision-model" {
		t.Fatalf("decision model = %q, want user-decision-model", got)
	}
	if got := pipeline.Steps[3].Model; got != "user-decision-model" {
		t.Fatalf("classification model = %q, want user-decision-model", got)
	}
}

func TestApplyOutputFormat(t *testing.T) {
	req := &induction.ChatRequest{}
	err := applyOutputFormat(req, inferenceFlags{
		responseFormat: "json_object",
		jsonSchema:     `{"type":"object","properties":{"answer":{"type":"string"}}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_object" {
		t.Fatalf("unexpected response format: %#v", req.ResponseFormat)
	}
	if req.JSONSchema == nil {
		t.Fatal("expected JSON schema")
	}
}

func TestInferenceHelpers(t *testing.T) {
	if pipelineUsesMCP(nil) || pipelineUsesMCP(&induction.Pipeline{Steps: []induction.PipelineStep{{}}}) {
		t.Fatal("unexpected MCP detection")
	}
	if !pipelineUsesMCP(&induction.Pipeline{Steps: []induction.PipelineStep{{UseMCP: true}}}) {
		t.Fatal("expected MCP detection")
	}

	req := &induction.ChatRequest{}
	flags := inferenceFlags{parameterOverrides: true, parameterSet: map[string]bool{
		"temperature": true, "top-p": true, "top-k": true, "max-tokens": true, "repeat-penalty": true, "seed": true,
	}, temperature: 0.2, topP: 0.8, topK: 4, maxTokens: 12, repeatPenalty: 1.1, seed: 7}
	applyParameters(req, flags)
	if req.Temperature == nil || *req.Temperature != 0.2 || req.TopK == nil || *req.TopK != 4 || req.Seed == nil || *req.Seed != 7 {
		t.Fatalf("parameters not applied: %#v", req)
	}
}

func TestApplyOutputFormatRejectsSchemaWithoutJSONFormat(t *testing.T) {
	err := applyOutputFormat(&induction.ChatRequest{}, inferenceFlags{
		jsonSchema: `{"type":"object"}`,
	})
	if err == nil {
		t.Fatal("expected schema without response format to fail")
	}
}

func TestApplyOutputFormatRejectsInvalidSchema(t *testing.T) {
	err := applyOutputFormat(&induction.ChatRequest{}, inferenceFlags{
		responseFormat: "json_object",
		jsonSchema:     `{"type":`,
	})
	if err == nil {
		t.Fatal("expected invalid schema to fail")
	}
}
