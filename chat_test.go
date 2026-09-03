package induction

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestRunChatSessionAccumulatesTranscript(t *testing.T) {
	original := &ChatRequest{
		Model: "model-id",
		Messages: []Message{
			{Role: "system", Content: "be helpful"},
		},
	}
	request := cloneChatRequest(original)
	var output bytes.Buffer
	turn := 0
	err := runChatSession(context.Background(), strings.NewReader("first\nsecond\n"), &output, &request, func(req *ChatRequest) (string, error) {
		turn++
		wantMessages := 2
		if turn == 2 {
			wantMessages = 4
			if got := req.Messages[2]; got.Role != "assistant" || got.Content != "answer 1" {
				t.Fatalf("assistant response not retained: %#v", got)
			}
			if got := req.Messages[3]; got.Role != "user" || got.Content != "second" {
				t.Fatalf("user response not retained: %#v", got)
			}
		}
		if len(req.Messages) != wantMessages {
			t.Fatalf("turn %d has %d messages, want %d", turn, len(req.Messages), wantMessages)
		}
		answer := "answer " + string(rune('0'+turn))
		_, _ = io.WriteString(&output, answer+"\n")
		return answer, nil
	})
	if err != nil {
		t.Fatalf("runChatSession: %v", err)
	}
	if turn != 2 {
		t.Fatalf("got %d turns, want 2", turn)
	}
	if len(original.Messages) != 1 {
		t.Fatalf("original request was modified: %#v", original.Messages)
	}
	for _, want := range []string{
		" " + DefaultUnicode.Star + " You: \n " + DefaultUnicode.Sparkle + " Assistant: answer 1",
		" " + DefaultUnicode.Star + " You: \n " + DefaultUnicode.Sparkle + " Assistant: answer 2",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output does not contain %q: %q", want, output.String())
		}
	}
}

func TestRunChatSessionCancellationAndErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := ChatRequest{}
	if err := runChatSession(ctx, strings.NewReader(""), io.Discard, &request, func(*ChatRequest) (string, error) {
		return "", context.Canceled
	}); err != nil {
		t.Fatalf("cancellation should end cleanly: %v", err)
	}

	want := errors.New("boom")
	if err := runChatSession(context.Background(), strings.NewReader("hello\n"), io.Discard, &request, func(*ChatRequest) (string, error) {
		return "", want
	}); !errors.Is(err, want) {
		t.Fatalf("expected inference error, got %v", err)
	}
}

func TestInferenceResponseContent(t *testing.T) {
	if got := inferenceResponseContent(nil); got != "" {
		t.Fatalf("nil response content = %q", got)
	}
	if got := inferenceResponseContent(&InferenceResponse{Choices: []InferenceChoice{{Text: "completion"}}}); got != "completion" {
		t.Fatalf("completion content = %q", got)
	}
	if got := inferenceResponseContent(&InferenceResponse{Choices: []InferenceChoice{{Message: &InferenceResponseMessage{Content: "chat"}}}}); got != "chat" {
		t.Fatalf("chat content = %q", got)
	}
}

func TestChatMonitorStopsPollingWithoutRemovingOverlay(t *testing.T) {
	var terminal bytes.Buffer
	footer := &stickyFooter{
		writer: &terminal,
		size:   func() (int, int, error) { return 80, 24, nil },
		width:  80,
		height: 24,
		active: true,
	}
	overlay := &liveMetricsOverlay{footer: footer}
	_, cancel := context.WithCancel(context.Background())
	monitor := &inferenceMonitor{cancel: cancel, overlay: overlay}

	monitor.stopKeepingOverlay()
	if !footer.active || overlay.stopped {
		t.Fatal("chat monitor removed the overlay before application cleanup")
	}
}

func TestChatEntryPointRejectsInvalidArguments(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		call func() error
	}{
		{"nil request", func() error { return InferChat(ctx, nil, strings.NewReader(""), io.Discard) }},
		{"nil input", func() error { return InferChat(ctx, &ChatRequest{}, nil, io.Discard) }},
		{"nil output", func() error { return InferChat(ctx, &ChatRequest{}, strings.NewReader(""), nil) }},
		{"stream nil request", func() error { return InferStreamChat(ctx, nil, strings.NewReader(""), io.Discard) }},
		{"snapshot nil request", func() error { _, err := InferSnapshotChat(ctx, nil, strings.NewReader(""), io.Discard); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatal("expected argument validation error")
			}
		})
	}
}
