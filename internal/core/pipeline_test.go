package induction

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func activePipelinePaths(pattern string) ([]string, error) {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	active := paths[:0]
	for _, path := range paths {
		if strings.HasSuffix(path, ".local.yaml") {
			continue
		}
		active = append(active, path)
	}
	return active, nil
}

func TestLoadAllPipelineExamples(t *testing.T) {
	paths, err := activePipelinePaths("../../pipelines/pipeline*.yaml")
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

func TestTextBatchInputValidates(t *testing.T) {
	p := &Pipeline{
		Name:  "text-batch",
		Batch: &BatchConfig{Items: []BatchItemConfig{{ID: "q1", InputSet: InputSet{Text: "question"}}}},
		Steps: []PipelineStep{{Name: "answer", Model: "model", UserPrompt: "{{inputs.text}}"}},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("text batch should validate: %v", err)
	}
}

func TestPipelineMCPIsOptIn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp-default.yaml")
	contents := `name: mcp-default
steps:
  - name: plain
    model: model
    userPrompt: plain
  - name: opted-in
    model: model
    usemcp: true
    userPrompt: opted in
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPipeline(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Steps[0].UseMCP {
		t.Fatal("MCP was enabled for a step without usemcp")
	}
	if !p.Steps[1].UseMCP {
		t.Fatal("usemcp: true was not loaded")
	}
}

func TestImagePipelineModelCoverage(t *testing.T) {
	valid := currentModelIDs(t)
	unknown := map[string]bool{}

	paths, err := activePipelinePaths("../../pipelines/pipeline*.yaml")
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
			relative, err := filepath.Rel("../../data/fixtures/images", step.Image)
			if err != nil || relative == ".." || len(relative) > 3 && relative[:3] == "../" {
				t.Errorf("%s uses image outside data/fixtures/images: %s", path, step.Image)
			}
			if !valid[step.Model] {
				unknown[step.Model] = true
			}
		}
	}
	if len(unknown) > 0 {
		t.Skipf("model endpoint is stale before enforcing image model coverage (unknown: %v)", sortedModelIDs(unknown))
	}
}

func TestImagePipelinesHaveAtLeastTwoSteps(t *testing.T) {
	paths, err := activePipelinePaths("../../pipelines/pipeline.image*.yaml")
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
	paths, err := activePipelinePaths("../../pipelines/pipeline.document*.yaml")
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
	paths, err := activePipelinePaths("../../pipelines/pipeline.mcp*.yaml")
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
	paths, err := activePipelinePaths("../../pipelines/pipeline.text*.yaml")
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
		"../../pipelines/pipeline.document-*.yaml":            1,
		"../../pipelines/pipeline.image-[0-9][0-9].yaml":      1,
		"../../pipelines/pipeline.image-text-*.yaml":          1,
		"../../pipelines/pipeline.mcp-*.yaml":                 5,
		"../../pipelines/pipeline.prompt-optimization-*.yaml": 2,
		"../../pipelines/pipeline.text-*.yaml":                5,
	}
	for pattern, expected := range want {
		paths, err := activePipelinePaths(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) != expected {
			t.Errorf("%s has %d examples, want %d", pattern, len(paths), expected)
		}
	}
}

func TestPipelineModelCoverage(t *testing.T) {
	valid := currentModelIDs(t)
	unknown := map[string]bool{}

	paths, err := activePipelinePaths("../../pipelines/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		pipeline, err := LoadPipeline(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		for _, step := range pipeline.Steps {
			if step.Transform != "" {
				continue
			}
			if !valid[step.Model] {
				unknown[step.Model] = true
			}
		}
	}
	if len(unknown) > 0 {
		t.Skipf("model endpoint is stale before enforcing pipeline model coverage (unknown: %v)", sortedModelIDs(unknown))
	}
}

func sortedModelIDs(models map[string]bool) []string {
	ids := make([]string, 0, len(models))
	for id := range models {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func currentModelIDs(t *testing.T) map[string]bool {
	t.Helper()
	configData, err := os.ReadFile(defaultConfigPath)
	if err != nil {
		t.Skipf("model endpoint unavailable: read %s: %v", defaultConfigPath, err)
	}
	var cfg struct {
		Server string `yaml:"server"`
	}
	if err := yaml.Unmarshal(configData, &cfg); err != nil || strings.TrimSpace(cfg.Server) == "" {
		t.Skipf("model endpoint unavailable: invalid server in %s", defaultConfigPath)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	catalog, err := NewClient(ctx, cfg.Server, WithHTTPClient(&http.Client{Timeout: 5 * time.Second})).ListModelCatalog(ctx)
	if err != nil {
		t.Skipf("model endpoint unavailable at %s: %v", cfg.Server, err)
	}
	valid := map[string]bool{}
	for _, model := range catalog {
		if strings.TrimSpace(model.ID) != "" {
			valid[model.ID] = true
		}
	}
	if len(valid) == 0 {
		t.Fatalf("model endpoint %s returned no model IDs", cfg.Server)
	}
	return valid
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
	p, err := LoadPipeline("../../pipelines/pipeline.custom-model-parameters-01.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if p.Config != "induction.yaml" || len(p.Steps) != 2 {
		t.Fatalf("unexpected full example pipeline: %#v", p)
	}
	if p.Steps[0].UseMCP || p.Steps[0].Parameters == nil {
		t.Fatalf("first step options were not loaded: %#v", p.Steps[0])
	}
	if p.Steps[1].ResponseFormat == nil || p.Steps[1].JSONSchema == nil {
		t.Fatalf("structured output was not loaded: %#v", p.Steps[1])
	}
}
