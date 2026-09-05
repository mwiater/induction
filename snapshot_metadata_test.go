package induction

import (
	"encoding/json"
	"testing"
)

func TestSnapshotMetadataDefaultsToText(t *testing.T) {
	var snapshot ModelSnapshot
	initializeSnapshotMetadata(&snapshot, &ChatRequest{})
	if snapshot.InputType != "text" || snapshot.OutputType != "text" {
		t.Fatalf("defaults = input %q, output %q", snapshot.InputType, snapshot.OutputType)
	}
	if snapshot.ApplicationTools || snapshot.MCPTools {
		t.Fatal("tool flags should default to false")
	}
}

func TestSnapshotMetadataClassifiesImageAndOutput(t *testing.T) {
	req := &ChatRequest{
		Messages:       []Message{{Role: "user", Content: []ContentPart{{Type: "text", Text: "describe"}, {Type: "image_url", ImageURL: &ImageURLPart{URL: "data:image/png;base64,abc"}}}}},
		ResponseFormat: &ResponseFormat{Type: "json_schema", JSONSchema: map[string]any{"type": "object"}},
	}
	var snapshot ModelSnapshot
	initializeSnapshotMetadata(&snapshot, req)
	if snapshot.InputType != "image" || snapshot.OutputType != "json_schema" {
		t.Fatalf("classification = input %q, output %q", snapshot.InputType, snapshot.OutputType)
	}
}

func TestSnapshotMetadataOutputTypes(t *testing.T) {
	tests := []struct {
		name   string
		req    ChatRequest
		output string
	}{
		{name: "json object", req: ChatRequest{ResponseFormat: &ResponseFormat{Type: "json_object"}}, output: "json"},
		{name: "json schema field", req: ChatRequest{JSONSchema: map[string]any{"type": "object"}}, output: "json_schema"},
		{name: "grammar", req: ChatRequest{Grammar: "root ::= \"ok\""}, output: "grammar"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var snapshot ModelSnapshot
			initializeSnapshotMetadata(&snapshot, &test.req)
			if snapshot.OutputType != test.output {
				t.Fatalf("output type = %q, want %q", snapshot.OutputType, test.output)
			}
		})
	}
}

func TestSnapshotMetadataOnlyMarksApplicationToolsWhenUsed(t *testing.T) {
	declared := &ChatRequest{Tools: []Tool{{Type: "function", Function: ToolFunction{Name: "lookup"}}}}
	var snapshot ModelSnapshot
	initializeSnapshotMetadata(&snapshot, declared)
	if snapshot.ApplicationTools {
		t.Fatal("declared tools must not mark applicationTools")
	}

	used := &ChatRequest{Messages: []Message{
		{Role: "assistant", ToolCalls: []InferenceToolCall{{ID: "call-1", Function: InferenceFunctionCall{Name: "lookup"}}}},
		{Role: "tool", ToolCallID: "call-1", Name: "lookup", Content: "result"},
	}}
	initializeSnapshotMetadata(&snapshot, used)
	if !snapshot.ApplicationTools {
		t.Fatal("executed application tool should mark applicationTools")
	}
}

func TestSnapshotMetadataClassifiesMCPToolsSeparately(t *testing.T) {
	req := &ChatRequest{Messages: []Message{
		{Role: "assistant", ToolCalls: []InferenceToolCall{{ID: "call-1", Function: InferenceFunctionCall{Name: "weather"}}}},
		{Role: "tool", ToolCallID: "call-1", Name: "weather", Content: "sunny"},
	}}
	var snapshot ModelSnapshot
	initializeSnapshotMetadataForMCPWithNames(&snapshot, req, true, []string{"weather"})
	if !snapshot.MCPTools || snapshot.ApplicationTools || !snapshot.MCPToolsAvailable || !snapshot.MCPToolsUsed || snapshot.ApplicationToolsAvailable {
		t.Fatalf("MCP tool metadata = %#v", snapshot)
	}
	if snapshot.MCPToolUseOutcome != "used_successfully" || snapshot.ApplicationToolUseOutcome != "not_available" || len(snapshot.MCPToolNames) != 1 {
		t.Fatalf("MCP tool outcomes = %#v", snapshot)
	}
}

func TestSnapshotMetadataRecordsAvailableButUnusedMCPTools(t *testing.T) {
	var snapshot ModelSnapshot
	initializeSnapshotMetadataForMCPWithNames(&snapshot, &ChatRequest{Messages: []Message{{Role: "user", Content: "current weather"}}}, true, []string{"weather"})
	if !snapshot.MCPToolsAvailable || snapshot.MCPToolsUsed || snapshot.MCPToolUseOutcome != "model_did_not_request" {
		t.Fatalf("unused MCP tool metadata = %#v", snapshot)
	}
}

func TestSnapshotMetadataUsesRequestedJSONTags(t *testing.T) {
	data, err := json.Marshal(ModelSnapshot{InputType: "text", OutputType: "text"})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(data)
	for _, field := range []string{"\"inputType\"", "\"outputType\"", "\"applicationTools\"", "\"MCPTools\""} {
		if !containsJSONField(encoded, field) {
			t.Fatalf("missing JSON field %s in %s", field, encoded)
		}
	}
}

func containsJSONField(encoded, field string) bool {
	return len(encoded) >= len(field) && stringContains(encoded, field)
}

func stringContains(value, want string) bool {
	for i := 0; i+len(want) <= len(value); i++ {
		if value[i:i+len(want)] == want {
			return true
		}
	}
	return false
}
