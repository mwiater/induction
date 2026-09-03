package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/mwiater/induction"
)

func TestRun(t *testing.T) {
	old := inferSnapshot
	defer func() { inferSnapshot = old }()
	inferSnapshot = func(_ context.Context, req *induction.ChatRequest, _ ...induction.ClientOption) (*induction.ModelSnapshot, error) {
		if req.Model != "test-model" || len(req.Messages) != 2 {
			t.Fatalf("unexpected request: %#v", req)
		}
		if req.Temperature == nil || *req.Temperature != temperature ||
			req.TopP == nil || *req.TopP != topP ||
			req.TopK == nil || *req.TopK != topK ||
			req.MaxTokens == nil || *req.MaxTokens != maxTokens ||
			req.RepeatPenalty == nil || *req.RepeatPenalty != repeatPenalty ||
			req.Seed == nil || *req.Seed != seed {
			t.Fatalf("parameter overrides were not applied: %#v", req)
		}
		return &induction.ModelSnapshot{ModelID: "model-1"}, nil
	}
	var output bytes.Buffer
	if err := run(context.Background(), "test-model", &output); err != nil || !bytes.Contains(output.Bytes(), []byte("model-1")) {
		t.Fatalf("run output=%q err=%v", output.String(), err)
	}
	inferSnapshot = func(context.Context, *induction.ChatRequest, ...induction.ClientOption) (*induction.ModelSnapshot, error) {
		return nil, errors.New("boom")
	}
	if err := run(context.Background(), "test-model", &bytes.Buffer{}); err == nil {
		t.Fatal("expected inference error")
	}
}

func TestRunWriterError(t *testing.T) {
	old := inferSnapshot
	defer func() { inferSnapshot = old }()
	inferSnapshot = func(context.Context, *induction.ChatRequest, ...induction.ClientOption) (*induction.ModelSnapshot, error) {
		return &induction.ModelSnapshot{}, nil
	}
	if err := run(context.Background(), "test-model", errorWriter{}); err == nil {
		t.Fatal("expected writer error")
	}
}

func TestMain(t *testing.T) {
	oldRun, oldFatal := runMain, fatal
	defer func() { runMain, fatal = oldRun, oldFatal }()
	called := false
	fatal = func(...any) { called = true }
	runMain = func(context.Context, string, io.Writer) error { return nil }
	main()
	runMain = func(context.Context, string, io.Writer) error { return errors.New("boom") }
	main()
	if !called {
		t.Fatal("expected fatal call")
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }
