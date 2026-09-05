package main

import (
	"bytes"
	"context"
	"errors"
	"github.com/mwiater/induction"
	"io"
	"strings"
	"testing"
)

func TestRunChat(t *testing.T) {
	old := inferChat
	t.Cleanup(func() { inferChat = old })
	inferChat = func(_ context.Context, req *induction.ChatRequest, in io.Reader, out io.Writer, _ ...induction.ClientOption) error {
		if req.Model != "test-model" || len(req.Messages) != 1 || in == nil {
			t.Fatalf("unexpected request: %#v", req)
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

func TestParseArgs(t *testing.T) {
	model, prompt, autosubmit, autoexit, err := parseArgs([]string{"--model", "model", "--prompt", "hello", "--autosubmit", "--autoexit"})
	if err != nil || model != "model" || prompt != "hello" || !autosubmit || !autoexit {
		t.Fatalf("parseArgs = model=%q prompt=%q autosubmit=%v autoexit=%v err=%v", model, prompt, autosubmit, autoexit, err)
	}
}

func TestParseArgsRejectsAutosubmitWithoutPrompt(t *testing.T) {
	if _, _, _, _, err := parseArgs([]string{"--model", "model", "--autosubmit"}); err == nil {
		t.Fatal("expected --autosubmit validation error")
	}
	if _, _, _, _, err := parseArgs([]string{"--model", "model", "--prompt", "  ", "--autosubmit"}); err == nil {
		t.Fatal("expected whitespace-only prompt validation error")
	}
	if _, _, _, _, err := parseArgs([]string{"--model", "model", "--autoexit"}); err == nil {
		t.Fatal("expected --autoexit validation error")
	}
}
