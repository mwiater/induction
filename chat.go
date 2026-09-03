package induction

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

var (
	chatUserPrompt      = "\n " + renderWhiteIcon(DefaultUnicode.Star) + " You: "
	chatAssistantPrompt = " " + renderWhiteIcon(DefaultUnicode.Sparkle) + " Assistant: "
)

// InferChat runs a multi-turn, non-streaming chat session. It prompts for the
// first user message before sending a request, then retains the accumulated
// transcript on every turn. Cancel ctx (normally with Ctrl-C) to end the
// session.
func InferChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, options ...ClientOption) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if in == nil {
		return fmt.Errorf("input reader is nil")
	}
	if out == nil {
		return fmt.Errorf("output writer is nil")
	}
	if canRunConsole(in, out) {
		_, err := runConsoleChat(ctx, req, in, out, consoleNonStreaming, options...)
		return err
	}

	client, request, monitor, timeout, err := startChatMonitor(ctx, req, options...)
	if err != nil {
		return err
	}
	defer monitor.stopKeepingOverlay()
	session, err := newUnsavedChatSession(request)
	if err != nil {
		return err
	}
	if err := saveChatSession(session); err != nil {
		return err
	}
	snapshotClient := client.withoutLiveMetricsOverlay(ctx)
	return runChatSession(ctx, in, out, &request, func(turn *ChatRequest) (string, error) {
		turnCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		snapshot, err := snapshotClient.GenerateSnapshot(turnCtx, turn)
		if err != nil {
			session.Messages = cloneMessages(turn.Messages)
			_ = saveChatSession(session)
			return "", err
		}
		monitor.overlay.Complete()
		content := lastInteractionContent(snapshot)
		session.Snapshots = append(session.Snapshots, snapshot)
		session.Messages = cloneMessages(snapshot.Messages)
		if err := saveChatSession(session); err != nil {
			return "", err
		}
		if _, err := fmt.Fprintln(out, content); err != nil {
			return "", fmt.Errorf("write assistant response: %w", err)
		}
		return content, nil
	})
}

// InferStreamChat runs a multi-turn streaming chat session. It has the same
// transcript and cancellation behavior as InferChat, but writes each assistant
// response as it arrives.
func InferStreamChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, options ...ClientOption) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if in == nil {
		return fmt.Errorf("input reader is nil")
	}
	if out == nil {
		return fmt.Errorf("output writer is nil")
	}
	if canRunConsole(in, out) {
		_, err := runConsoleChat(ctx, req, in, out, consoleStreaming, options...)
		return err
	}

	client, request, monitor, timeout, err := startChatMonitor(ctx, req, options...)
	if err != nil {
		return err
	}
	defer monitor.stopKeepingOverlay()
	session, err := newUnsavedChatSession(request)
	if err != nil {
		return err
	}
	if err := saveChatSession(session); err != nil {
		return err
	}
	snapshotClient := client.withoutLiveMetricsOverlay(ctx)
	return runChatSession(ctx, in, out, &request, func(turn *ChatRequest) (string, error) {
		turnCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		reasoningOpen := false
		snapshot, err := snapshotClient.GenerateStreamingSnapshot(turnCtx, turn, func(chunk InferenceStreamChunk) error {
			for _, choice := range chunk.Choices {
				if reasoning := choice.Delta.ReasoningContent; reasoning != "" {
					if !reasoningOpen {
						if _, err := io.WriteString(out, "<think>\n"); err != nil {
							return err
						}
						reasoningOpen = true
					}
					if _, err := io.WriteString(out, reasoning); err != nil {
						return err
					}
				}
				content := choice.Delta.Content
				if content == "" {
					content = choice.Text
				}
				if content != "" {
					if reasoningOpen {
						if _, err := io.WriteString(out, "\n</think>\n\n"); err != nil {
							return err
						}
						reasoningOpen = false
					}
					if _, err := io.WriteString(out, content); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if reasoningOpen {
			if _, writeErr := io.WriteString(out, "\n</think>"); err == nil {
				err = writeErr
			}
		}
		if err != nil {
			session.Messages = cloneMessages(turn.Messages)
			_ = saveChatSession(session)
			return "", err
		}
		monitor.overlay.Complete()
		if _, err := fmt.Fprintln(out); err != nil {
			return "", fmt.Errorf("write assistant response: %w", err)
		}
		content := lastInteractionContent(snapshot)
		session.Snapshots = append(session.Snapshots, snapshot)
		session.Messages = cloneMessages(snapshot.Messages)
		if err := saveChatSession(session); err != nil {
			return "", err
		}
		return content, nil
	})
}

// InferSnapshotChat runs a multi-turn, non-streaming chat session and returns
// one telemetry snapshot for every completed assistant response. The returned
// slice remains available when the session ends through cancellation or EOF.
func InferSnapshotChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, options ...ClientOption) ([]*ModelSnapshot, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if in == nil {
		return nil, fmt.Errorf("input reader is nil")
	}
	if out == nil {
		return nil, fmt.Errorf("output writer is nil")
	}
	if canRunConsole(in, out) {
		return runConsoleChat(ctx, req, in, out, consoleSnapshot, options...)
	}

	client, request, monitor, timeout, err := startChatMonitor(ctx, req, options...)
	if err != nil {
		return nil, err
	}
	defer monitor.stopKeepingOverlay()
	snapshotClient := client.withoutLiveMetricsOverlay(ctx)
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	var persistence *snapshotSession
	if cfg.PersistSnapshots {
		persistence, err = newSnapshotSession(request.Model)
		if err != nil {
			return nil, err
		}
	}

	var snapshots []*ModelSnapshot
	err = runChatSession(ctx, in, out, &request, func(turn *ChatRequest) (string, error) {
		turnCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		snapshot, err := snapshotClient.GenerateSnapshot(turnCtx, turn)
		if err != nil {
			return "", err
		}
		monitor.overlay.Complete()
		snapshots = append(snapshots, snapshot)
		if persistence != nil {
			if err := persistence.save(snapshots); err != nil {
				return "", err
			}
		}
		content := ""
		if len(snapshot.Interaction) > 0 {
			content = snapshot.Interaction[len(snapshot.Interaction)-1].Content
		}
		if _, err := fmt.Fprintln(out, content); err != nil {
			return "", fmt.Errorf("write assistant response: %w", err)
		}
		return content, nil
	})
	return snapshots, err
}

func (c *Client) withoutLiveMetricsOverlay(ctx context.Context) *Client {
	return NewClient(ctx, c.endpoint,
		WithHTTPClient(c.opts.httpClient),
		WithPollInterval(c.opts.pollInterval),
		WithLoadWaitInterval(c.opts.loadWaitInterval),
		WithLogger(c.opts.logger),
		WithLiveMetricsOverlay(false),
	)
}

func startChatMonitor(ctx context.Context, req *ChatRequest, options ...ClientOption) (*Client, ChatRequest, *inferenceMonitor, time.Duration, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, ChatRequest{}, nil, 0, err
	}
	request := cloneChatRequest(req)
	if request.Model == "" {
		return nil, ChatRequest{}, nil, 0, fmt.Errorf("request model is required")
	}
	client := newClientFromConfig(ctx, cfg, options...)
	monitor := client.startInferenceMonitor(ctx, request.Model, false)
	return client, request, monitor, time.Duration(cfg.Timeout), nil
}

func runChatSession(ctx context.Context, in io.Reader, out io.Writer, req *ChatRequest, inferTurn func(*ChatRequest) (string, error)) error {
	lines := make(chan string)
	readErrors := make(chan error, 1)
	go scanChatInput(in, lines, readErrors)

	firstTurn := true
	for {
		prompt := chatUserPrompt
		if firstTurn {
			prompt = " " + renderWhiteIcon(DefaultUnicode.Star) + " You: "
		}
		if _, err := io.WriteString(out, prompt); err != nil {
			return fmt.Errorf("write user prompt: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErrors:
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read chat input: %w", err)
		case line, ok := <-lines:
			if !ok {
				return nil
			}
			req.Messages = append(req.Messages, Message{Role: "user", Content: line})
		}

		if _, err := io.WriteString(out, "\n"+chatAssistantPrompt); err != nil {
			return fmt.Errorf("write assistant prompt: %w", err)
		}
		content, err := inferTurn(req)
		if err != nil {
			if errors.Is(err, context.Canceled) && ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("chat inference: %w", err)
		}
		req.Messages = append(req.Messages, Message{Role: "assistant", Content: content})
		firstTurn = false
	}
}

func scanChatInput(in io.Reader, lines chan<- string, readErrors chan<- error) {
	defer close(lines)
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		lines <- scanner.Text()
	}
	if err := scanner.Err(); err != nil {
		readErrors <- err
		return
	}
	readErrors <- io.EOF
}

func cloneChatRequest(req *ChatRequest) ChatRequest {
	clone := *req
	clone.Messages = append([]Message(nil), req.Messages...)
	return clone
}

func inferenceResponseContent(response *InferenceResponse) string {
	if response == nil || len(response.Choices) == 0 {
		return ""
	}
	choice := response.Choices[0]
	if choice.Message != nil {
		return choice.Message.Content
	}
	return choice.Text
}
