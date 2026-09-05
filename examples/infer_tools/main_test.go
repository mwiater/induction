package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/mwiater/induction"
)

func TestRunFunctionLoop(t *testing.T) {
	old := inferChat
	defer func() { inferChat = old }()
	inferChat = func(_ context.Context, req *induction.ChatRequest, _ io.Reader, out io.Writer, handler induction.ApplicationToolHandler, _ ...induction.ClientOption) error {
		if req.Model != "test-model" || len(req.Tools) != 3 || handler == nil {
			t.Fatalf("unexpected request: %#v", req)
		}
		_, err := io.WriteString(out, "chat")
		return err
	}
	var out bytes.Buffer
	if err := runWithOptions(context.Background(), "test-model", strings.NewReader("hello\n"), &out, "hello", true, false); err != nil {
		t.Fatal(err)
	}
	if out.String() != "chat" {
		t.Fatalf("out=%q", out.String())
	}
	if !strings.Contains(string(mustJSON(applicationTools[0].Function.Parameters)), "additionalProperties") {
		t.Fatal("schema is not strict")
	}
}

func TestLocalToolsReturnSystemData(t *testing.T) {
	for _, name := range []string{systemTimeTool, diskSpaceTool, freeRAMTool} {
		result, err := runLocalTool(context.Background(), name)
		if err != nil {
			t.Fatalf("runLocalTool(%q): %v", name, err)
		}
		if !json.Valid([]byte(result)) {
			t.Fatalf("runLocalTool(%q) returned invalid JSON: %q", name, result)
		}
	}
}

func TestApplicationToolCallsChainSystemTime(t *testing.T) {
	calls := chainApplicationToolCalls([]induction.InferenceToolCall{{ID: "call-1", Function: induction.InferenceFunctionCall{Name: "some_local_tool"}}})
	if len(calls) != 2 || calls[1].Function.Name != systemTimeTool {
		t.Fatalf("chained calls = %#v", calls)
	}
	if calls = chainApplicationToolCalls([]induction.InferenceToolCall{{Function: induction.InferenceFunctionCall{Name: systemTimeTool}}}); len(calls) != 1 {
		t.Fatalf("duplicate time call was added: %#v", calls)
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
