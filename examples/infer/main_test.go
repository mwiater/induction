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
	oldInfer := infer
	oldStream := inferStream
	oldChat := inferStreamChat
	oldSnapshot := inferSnapshot
	t.Cleanup(func() {
		infer = oldInfer
		inferStream = oldStream
		inferStreamChat = oldChat
		inferSnapshot = oldSnapshot
	})
}

func TestRunDefault(t *testing.T) {
	preserveFunctions(t)
	infer = func(_ context.Context, req *induction.ChatRequest, _ ...induction.ClientOption) (*induction.InferenceResponse, error) {
		assertRequest(t, req, 2)
		return &induction.InferenceResponse{ID: "response-1"}, nil
	}
	var out bytes.Buffer
	if err := run(context.Background(), "test-model", modeDefault, strings.NewReader(""), &out); err != nil || !strings.Contains(out.String(), "response-1") {
		t.Fatalf("output=%q err=%v", out.String(), err)
	}
}

func TestRunStream(t *testing.T) {
	preserveFunctions(t)
	inferStream = func(_ context.Context, req *induction.ChatRequest, out io.Writer, _ ...induction.ClientOption) error {
		assertRequest(t, req, 2)
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
	inferStreamChat = func(_ context.Context, req *induction.ChatRequest, in io.Reader, out io.Writer, _ ...induction.ClientOption) error {
		assertRequest(t, req, 1)
		if in == nil {
			t.Fatal("input is nil")
		}
		_, err := io.WriteString(out, "chat")
		return err
	}
	var out bytes.Buffer
	if err := run(context.Background(), "test-model", modeChat, strings.NewReader("hello"), &out); err != nil || !strings.Contains(out.String(), "chat") {
		t.Fatalf("output=%q err=%v", out.String(), err)
	}
}

func TestRunSnapshot(t *testing.T) {
	preserveFunctions(t)
	inferSnapshot = func(_ context.Context, req *induction.ChatRequest, _ ...induction.ClientOption) (*induction.ModelSnapshot, error) {
		assertRequest(t, req, 2)
		return &induction.ModelSnapshot{ModelID: "model-1"}, nil
	}
	var out bytes.Buffer
	if err := run(context.Background(), "test-model", modeSnapshot, strings.NewReader(""), &out); err != nil || !strings.Contains(out.String(), "model-1") {
		t.Fatalf("output=%q err=%v", out.String(), err)
	}
}

func TestRunErrors(t *testing.T) {
	preserveFunctions(t)
	infer = func(context.Context, *induction.ChatRequest, ...induction.ClientOption) (*induction.InferenceResponse, error) {
		return nil, errors.New("boom")
	}
	if err := run(context.Background(), "test-model", modeDefault, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("expected inference error")
	}
	if err := run(context.Background(), "test-model", "unknown", strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("expected unsupported mode error")
	}
}

func TestRunWriterErrors(t *testing.T) {
	preserveFunctions(t)
	infer = func(context.Context, *induction.ChatRequest, ...induction.ClientOption) (*induction.InferenceResponse, error) {
		return &induction.InferenceResponse{}, nil
	}
	if err := run(context.Background(), "test-model", modeDefault, strings.NewReader(""), errorWriter{}); err == nil {
		t.Fatal("expected response writer error")
	}
	if err := run(context.Background(), "test-model", modeChat, strings.NewReader(""), errorWriter{}); err == nil {
		t.Fatal("expected chat writer error")
	}
}

func assertRequest(t *testing.T, req *induction.ChatRequest, messages int) {
	t.Helper()
	if req.Model != "test-model" || len(req.Messages) != messages {
		t.Fatalf("unexpected request: %#v", req)
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }
