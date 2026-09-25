package induction

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAllPipelineExamples(t *testing.T) {
	paths, err := filepath.Glob("pipelines/pipeline*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no pipeline examples found")
	}

	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			if _, err := LoadPipeline(path); err != nil {
				t.Fatalf("pipeline example is invalid: %v", err)
			}
		})
	}
}

func TestImagePipelineModelCoverage(t *testing.T) {
	want := map[string]bool{
		"Qwen-3.5-9B-MTP-Coding-Q8_0":          false,
		"Qwen-3.5-9B-MTP-General-Q8_0":         false,
		"Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL":  false,
		"Qwen-3.6-35B-A3B-MTP-General-Q8_K_XL": false,
		"Qwen-3.8-27B-Non-Reasoning-Q4_K_M":    false,
		"Qwen-3.8-27B-Reasoning-Q4_K_M":        false,
	}

	paths, err := filepath.Glob("pipelines/pipeline*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		pipeline, err := LoadPipeline(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		for _, step := range pipeline.Steps {
			if step.Image == "" {
				continue
			}
			relative, err := filepath.Rel("data/fixtures/images", step.Image)
			if err != nil || relative == ".." || len(relative) > 3 && relative[:3] == "../" {
				t.Errorf("%s uses image outside data/fixtures/images: %s", path, step.Image)
			}
			if _, ok := want[step.Model]; ok {
				want[step.Model] = true
			}
		}
	}

	for model, found := range want {
		if !found {
			t.Errorf("no image pipeline represents model %q", model)
		}
	}
}

func TestImagePipelinesHaveAtLeastTwoSteps(t *testing.T) {
	paths, err := filepath.Glob("pipelines/pipeline.image*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		pipeline, err := LoadPipeline(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		if len(pipeline.Steps) < 2 {
			t.Errorf("%s has %d step(s), want at least 2", path, len(pipeline.Steps))
		}
	}
}

func TestDocumentPipelinesHaveAtLeastTwoSteps(t *testing.T) {
	paths, err := filepath.Glob("pipelines/pipeline.document*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		pipeline, err := LoadPipeline(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		if len(pipeline.Steps) < 2 {
			t.Errorf("%s has %d step(s), want at least 2", path, len(pipeline.Steps))
		}
		if pipeline.Steps[0].Document == "" {
			t.Errorf("%s does not ingest a document in its first step", path)
		}
	}
}

func TestMCPPipelinesHaveAtLeastTwoSteps(t *testing.T) {
	paths, err := filepath.Glob("pipelines/pipeline.mcp*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		pipeline, err := LoadPipeline(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		if len(pipeline.Steps) < 2 {
			t.Errorf("%s has %d step(s), want at least 2", path, len(pipeline.Steps))
		}
	}
}

func TestTextPipelinesHaveAtLeastTwoStepsAndNoAttachments(t *testing.T) {
	paths, err := filepath.Glob("pipelines/pipeline.text*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		pipeline, err := LoadPipeline(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		if len(pipeline.Steps) < 2 {
			t.Errorf("%s has %d step(s), want at least 2", path, len(pipeline.Steps))
		}
		for i, step := range pipeline.Steps {
			if step.Image != "" || step.Document != "" {
				t.Errorf("%s step %d has an attachment; text pipelines must be text-only", path, i)
			}
		}
	}
}

func TestPipelineExampleTypeCounts(t *testing.T) {
	want := map[string]int{
		"pipelines/pipeline.document-*.yaml":            5,
		"pipelines/pipeline.image-[0-9][0-9].yaml":      5,
		"pipelines/pipeline.image-text-*.yaml":          5,
		"pipelines/pipeline.mcp-*.yaml":                 5,
		"pipelines/pipeline.prompt-optimization-*.yaml": 5,
		"pipelines/pipeline.text-*.yaml":                5,
	}
	for pattern, expected := range want {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) != expected {
			t.Errorf("%s has %d examples, want %d", pattern, len(paths), expected)
		}
	}
}

func TestPipelineModelCoverage(t *testing.T) {
	want := map[string]bool{
		"Agents-A1-MTP-Apex-I-Quality":         false,
		"GLM-4.7-Flash-Q4_K_M":                 false,
		"LFM-2.5-8B-A1B-UD-Q8_K_XL":            false,
		"Ornith-1.0-35B-UD-Q4_K_M":             false,
		"Qwen-3-Coder-Next-Q4_K_M":             false,
		"Qwen-3.5-9B-MTP-Coding-Q8_0":          false,
		"Qwen-3.5-9B-MTP-General-Q8_0":         false,
		"Qwen-3.6-35B-A3B-MTP-Coding-Q8_K_XL":  false,
		"Qwen-3.6-35B-A3B-MTP-General-Q8_K_XL": false,
		"Qwen-3.8-27B-Non-Reasoning-Q4_K_M":    false,
		"Qwen-3.8-27B-Reasoning-Q4_K_M":        false,
	}

	paths, err := filepath.Glob("pipelines/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		pipeline, err := LoadPipeline(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		for _, step := range pipeline.Steps {
			if _, ok := want[step.Model]; !ok {
				t.Errorf("%s uses model %q outside the current model list", path, step.Model)
				continue
			}
			want[step.Model] = true
		}
	}

	for model, found := range want {
		if !found {
			t.Errorf("no pipeline uses current model %q", model)
		}
	}
}

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
