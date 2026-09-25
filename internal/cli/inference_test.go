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
	fixture, err := os.ReadFile("../../data/fixtures/documents/fixture.pdf")
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
	if pipelineUsesMCP(nil) || pipelineUsesMCP(&induction.Pipeline{Steps: []induction.PipelineStep{{NoMCP: true}}}) {
		t.Fatal("unexpected MCP detection")
	}
	if !pipelineUsesMCP(&induction.Pipeline{Steps: []induction.PipelineStep{{NoMCP: false}}}) {
		t.Fatal("expected MCP detection")
	}

	req := withLocalTools(&induction.ChatRequest{Model: "model"})
	if len(req.Tools) != 3 || req.ToolChoice != "auto" {
		t.Fatalf("local tools not attached: %#v", req)
	}
	calls := []induction.InferenceToolCall{{Function: induction.InferenceFunctionCall{Name: "other"}}}
	chained := chainLocalTools(calls)
	if len(chained) != 2 || chained[1].Function.Name != "current_system_date_time" {
		t.Fatalf("expected automatic time tool: %#v", chained)
	}
	if len(chainLocalTools([]induction.InferenceToolCall{{Function: induction.InferenceFunctionCall{Name: "current_system_date_time"}}})) != 1 {
		t.Fatal("existing time tool should not be duplicated")
	}

	flags := inferenceFlags{parameterOverrides: true, parameterSet: map[string]bool{
		"temperature": true, "top-p": true, "top-k": true, "max-tokens": true, "repeat-penalty": true, "seed": true,
	}, temperature: 0.2, topP: 0.8, topK: 4, maxTokens: 12, repeatPenalty: 1.1, seed: 7}
	applyParameters(req, flags)
	if req.Temperature == nil || *req.Temperature != 0.2 || req.TopK == nil || *req.TopK != 4 || req.Seed == nil || *req.Seed != 7 {
		t.Fatalf("parameters not applied: %#v", req)
	}
	if _, err := localToolResult(context.Background(), "unknown"); err == nil {
		t.Fatal("unknown local tool should fail")
	}
	if value, err := localToolResult(context.Background(), "current_system_date_time"); err != nil || !strings.Contains(value, "date_time") {
		t.Fatalf("date/time tool = %q, err=%v", value, err)
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
