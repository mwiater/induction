package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mwiater/induction"
)

func preserveFunctions(t *testing.T) {
	t.Helper()
	oldInfer := inferMCP
	oldStream := inferMCPStream
	oldChat := inferMCPChat
	oldSnapshot := inferMCPSnapshot
	t.Cleanup(func() {
		inferMCP = oldInfer
		inferMCPStream = oldStream
		inferMCPChat = oldChat
		inferMCPSnapshot = oldSnapshot
	})
}

func TestRunDefault(t *testing.T) {
	preserveFunctions(t)
	inferMCP = func(_ context.Context, req *induction.ChatRequest, _ ...induction.ClientOption) (*induction.InferenceResponse, error) {
		assertRequest(t, req)
		return &induction.InferenceResponse{ID: "mcp-response"}, nil
	}
	var out bytes.Buffer
	if err := run(context.Background(), "test-model", modeDefault, strings.NewReader(""), &out); err != nil || !strings.Contains(out.String(), "mcp-response") {
		t.Fatalf("output=%q err=%v", out.String(), err)
	}
}

func TestRunStream(t *testing.T) {
	preserveFunctions(t)
	inferMCPStream = func(_ context.Context, req *induction.ChatRequest, out io.Writer, _ ...induction.ClientOption) error {
		assertRequest(t, req)
		_, err := io.WriteString(out, "streamed")
		return err
	}
	var out bytes.Buffer
	if err := run(context.Background(), "test-model", modeStream, strings.NewReader(""), &out); err != nil || out.String() != "streamed" {
		t.Fatalf("output=%q err=%v", out.String(), err)
	}
}

func TestRunChat(t *testing.T) {
	preserveFunctions(t)
	inferMCPChat = func(_ context.Context, req *induction.ChatRequest, in io.Reader, out io.Writer, _ ...induction.ClientOption) error {
		if req.Model != "test-model" || len(req.Messages) != 0 || in == nil {
			t.Fatalf("unexpected chat request: %#v", req)
		}
		_, err := io.WriteString(out, "chat")
		return err
	}
	var out bytes.Buffer
	if err := run(context.Background(), "test-model", modeChat, strings.NewReader("question"), &out); err != nil || !strings.Contains(out.String(), "chat") {
		t.Fatalf("output=%q err=%v", out.String(), err)
	}
}

func TestRunSnapshot(t *testing.T) {
	preserveFunctions(t)
	inferMCPSnapshot = func(_ context.Context, req *induction.ChatRequest, _ ...induction.ClientOption) (*induction.ModelSnapshot, error) {
		assertRequest(t, req)
		return &induction.ModelSnapshot{ModelID: "snapshot-model"}, nil
	}
	var out bytes.Buffer
	if err := run(context.Background(), "test-model", modeSnapshot, strings.NewReader(""), &out); err != nil || !strings.Contains(out.String(), "snapshot-model") {
		t.Fatalf("output=%q err=%v", out.String(), err)
	}
}

func TestRunErrors(t *testing.T) {
	preserveFunctions(t)
	inferMCP = func(context.Context, *induction.ChatRequest, ...induction.ClientOption) (*induction.InferenceResponse, error) {
		return nil, errors.New("boom")
	}
	if err := run(context.Background(), "test-model", modeDefault, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("expected inference error")
	}
	if err := run(context.Background(), "test-model", "unknown", strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("expected unsupported mode error")
	}
}

func assertRequest(t *testing.T, req *induction.ChatRequest) {
	t.Helper()
	if req.Model != "test-model" || len(req.Messages) != 1 || len(req.Tools) != 0 {
		t.Fatalf("example should only configure the prompt: %#v", req)
	}
}
