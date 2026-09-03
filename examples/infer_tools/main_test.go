package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mwiater/induction"
)

func TestRunFunctionLoop(t *testing.T) {
	old := infer
	defer func() { infer = old }()
	var requests []*induction.ChatRequest
	infer = func(_ context.Context, req *induction.ChatRequest, _ ...induction.ClientOption) (*induction.InferenceResponse, error) {
		requests = append(requests, req)
		if len(requests) == 1 {
			return &induction.InferenceResponse{Choices: []induction.InferenceChoice{{Message: &induction.InferenceResponseMessage{Role: "assistant", ToolCalls: []induction.InferenceToolCall{{ID: "call-1", Type: "function", Function: induction.InferenceFunctionCall{Name: "get_weather", Arguments: `{"city":"Paris"}`}}}}}}}, nil
		}
		return &induction.InferenceResponse{Choices: []induction.InferenceChoice{{Message: &induction.InferenceResponseMessage{Content: "Paris is sunny."}}}}, nil
	}
	var out bytes.Buffer
	if err := run(context.Background(), "test-model", &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "Paris is sunny.\n" || len(requests) != 2 {
		t.Fatalf("out=%q requests=%d", out.String(), len(requests))
	}
	if len(requests[1].Messages) != 3 || requests[1].Messages[2].ToolCallID != "call-1" || requests[1].Messages[2].Name != "get_weather" {
		t.Fatalf("continuation=%#v", requests[1].Messages)
	}
	if !strings.Contains(string(mustJSON(requests[0].Tools[0].Function.Parameters)), "additionalProperties") {
		t.Fatal("schema is not strict")
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
