package induction

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPipelineResolvesAttachmentPathsAndPreservesOrder(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "fixture.jpg")
	if err := os.WriteFile(image, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pipeline.yaml")
	contents := `name: test
steps:
  - name: first
    model: model-a
    userPrompt: first prompt
    image: fixture.jpg
  - name: second
    model: model-b
    userPrompt: second prompt
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPipeline(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 2 || p.Steps[0].Name != "first" || p.Steps[1].Model != "model-b" {
		t.Fatalf("unexpected pipeline: %#v", p)
	}
	if p.Steps[0].Image != image {
		t.Fatalf("image path = %q", p.Steps[0].Image)
	}
}

func TestLoadPipelineRejectsDuplicateSteps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipeline.yaml")
	contents := `name: test
steps:
  - name: duplicate
    model: model
    userPrompt: prompt
  - name: duplicate
    model: model
    userPrompt: prompt
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPipeline(path); err == nil {
		t.Fatal("expected duplicate step validation error")
	}
}

func TestLoadPipelineRejectsImageAfterFirstStep(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "fixture.jpg")
	if err := os.WriteFile(image, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pipeline.yaml")
	contents := `name: test
steps:
  - name: first
    model: model
    userPrompt: first prompt
  - name: second
    model: model
    userPrompt: second prompt
    image: fixture.jpg
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPipeline(path); err == nil {
		t.Fatal("expected image-after-first-step validation error")
	}
}

func TestLoadPipelineRejectsDocumentAfterFirstStep(t *testing.T) {
	dir := t.TempDir()
	document := filepath.Join(dir, "fixture.pdf")
	if err := os.WriteFile(document, []byte("document"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pipeline.yaml")
	contents := `name: test
steps:
  - name: first
    model: model
    userPrompt: first prompt
  - name: second
    model: model
    userPrompt: second prompt
    document: fixture.pdf
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPipeline(path); err == nil {
		t.Fatal("expected document-after-first-step validation error")
	}
}

func TestLoadPipelineLoadsStructuredOutputSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipeline.yaml")
	contents := `name: test
steps:
  - name: format
    model: model
    userPrompt: return JSON
    responseFormat:
      type: json_object
    jsonSchema:
      type: object
      properties:
        answer:
          type: string
      required: [answer]
      additionalProperties: false
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPipeline(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Steps[0].ResponseFormat == nil || p.Steps[0].ResponseFormat.Type != "json_object" {
		t.Fatalf("unexpected response format: %#v", p.Steps[0].ResponseFormat)
	}
	schema, ok := p.Steps[0].JSONSchema.(map[string]any)
	if !ok || schema["type"] != "object" {
		t.Fatalf("unexpected JSON schema: %#v", p.Steps[0].JSONSchema)
	}
}

func TestLoadPipelineRejectsSchemaWithoutJSONResponseFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipeline.yaml")
	contents := `name: test
steps:
  - name: format
    model: model
    userPrompt: return JSON
    jsonSchema:
      type: object
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPipeline(path); err == nil {
		t.Fatal("expected schema validation error")
	}
}

func TestLoadPipelineRejectsUnsupportedResponseFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipeline.yaml")
	contents := `name: test
steps:
  - name: format
    model: model
    userPrompt: return output
    responseFormat:
      type: xml
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPipeline(path); err == nil {
		t.Fatal("expected response format validation error")
	}
}

func TestLoadFullExamplePipeline(t *testing.T) {
	p, err := LoadPipeline("pipelines/pipeline.full-example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if p.Config != "induction.yaml" || len(p.Steps) != 2 {
		t.Fatalf("unexpected full example pipeline: %#v", p)
	}
	if !p.Steps[0].NoMCP || p.Steps[0].Parameters == nil {
		t.Fatalf("first step options were not loaded: %#v", p.Steps[0])
	}
	if p.Steps[1].ResponseFormat == nil || p.Steps[1].JSONSchema == nil {
		t.Fatalf("structured output was not loaded: %#v", p.Steps[1])
	}
}
