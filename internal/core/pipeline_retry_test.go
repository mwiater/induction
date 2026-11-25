package induction

import "testing"

func TestValidateRetryOutputPlainTextRules(t *testing.T) {
	r := &PipelineRetry{Validate: &PipelineValidation{Rules: []PipelineValidationRule{{Type: "minWords", Path: "", Value: 4}}}}
	if issues := validateRetryOutput("one two", r); len(issues) != 1 || issues[0].Code != "min_words" {
		t.Fatalf("issues = %#v", issues)
	}
	if issues := validateRetryOutput("one two three four", r); len(issues) != 0 {
		t.Fatalf("valid text issues = %#v", issues)
	}
}

func TestValidateRetryJSONSchemaAndUniqueIDs(t *testing.T) {
	r := &PipelineRetry{
		Validate: &PipelineValidation{JSON: true, Rules: []PipelineValidationRule{{Type: "uniqueJsonPointers", Path: "/chapters", Field: "id"}}},
		SchemaValue: map[string]any{
			"type":       "object",
			"required":   []any{"chapters"},
			"properties": map[string]any{"chapters": map[string]any{"type": "array", "minItems": float64(2)}},
		},
	}
	bad := `{"chapters":[{"id":"one"},{"id":"one"}]}`
	issues := validateRetryOutput(bad, r)
	if len(issues) != 1 || issues[0].Code != "duplicate_id" {
		t.Fatalf("issues = %#v", issues)
	}
	if issues := validateRetryOutput(`{"chapters":[{"id":"one"},{"id":"two"}]} trailing`, r); len(issues) == 0 || issues[0].Code != "json_syntax" {
		t.Fatalf("trailing JSON issues = %#v", issues)
	}
}

func TestRetryConfigRequiresBoundedGeneratorContract(t *testing.T) {
	step := PipelineStep{Name: "writer", Model: "m", UserPrompt: "write", Retry: &PipelineRetry{MaxAttempts: 2}}
	if err := validatePipelineRetry(step, "."); err == nil {
		t.Fatal("retry without validator or gate was accepted")
	}
	step.Retry.Validate = &PipelineValidation{Rules: []PipelineValidationRule{{Type: "minWords", Path: "", Value: 1}}}
	if err := validatePipelineRetry(step, "."); err != nil {
		t.Fatalf("valid retry rejected: %v", err)
	}
	step.UseMCP = true
	if err := validatePipelineRetry(step, "."); err == nil {
		t.Fatal("retry with MCP enabled was accepted")
	}
}
