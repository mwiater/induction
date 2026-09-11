package cli

import (
	"testing"

	induction "github.com/mwiater/induction"
)

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
