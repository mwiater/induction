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

func TestRunChatUsesParameterOverrides(t *testing.T) {
	old := inferChat
	t.Cleanup(func() { inferChat = old })
	inferChat = func(_ context.Context, req *induction.ChatRequest, in io.Reader, out io.Writer, _ ...induction.ClientOption) error {
		if req.Model != "test-model" || len(req.Messages) != 2 || in == nil {
			t.Fatalf("unexpected request: %#v", req)
		}
		if req.Temperature == nil || *req.Temperature != temperature || req.TopP == nil || *req.TopP != topP || req.TopK == nil || *req.TopK != topK || req.MaxTokens == nil || *req.MaxTokens != maxTokens || req.RepeatPenalty == nil || *req.RepeatPenalty != repeatPenalty || req.Seed == nil || *req.Seed != seed {
			t.Fatalf("parameter overrides were not applied: %#v", req)
		}
		_, err := io.WriteString(out, "chat")
		return err
	}
	var out bytes.Buffer
	if err := run(context.Background(), "test-model", strings.NewReader("hello"), &out); err != nil || !strings.Contains(out.String(), "chat") {
		t.Fatalf("output=%q err=%v", out.String(), err)
	}
}

func TestRunChatErrors(t *testing.T) {
	old := inferChat
	t.Cleanup(func() { inferChat = old })
	inferChat = func(context.Context, *induction.ChatRequest, io.Reader, io.Writer, ...induction.ClientOption) error {
		return errors.New("boom")
	}
	if err := run(context.Background(), "test-model", strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("expected inference error")
	}
}
