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
		var snapshot *ModelSnapshot
		if client.opts.applicationToolHandler != nil {
			_, snapshot, err = runApplicationToolLoopWith(turnCtx, turn, client.opts.applicationToolHandler, client.opts.applicationToolChain, func(inferCtx context.Context, toolTurn *ChatRequest) (*InferenceResponse, *ModelSnapshot, error) {
				toolSnapshot, inferErr := snapshotClient.GenerateSnapshot(inferCtx, toolTurn)
				if inferErr != nil {
					return nil, toolSnapshot, inferErr
				}
				response, decodeErr := decodeSnapshotResponse(toolSnapshot)
				return response, toolSnapshot, decodeErr
			})
		} else {
			snapshot, err = snapshotClient.GenerateSnapshot(turnCtx, turn)
		}
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

func (c *Client) withoutLiveMetricsOverlay(ctx context.Context) *Client {
	client := NewClient(ctx, c.endpoint,
		WithHTTPClient(c.opts.httpClient),
		WithPollInterval(c.opts.pollInterval),
		WithLoadWaitInterval(c.opts.loadWaitInterval),
		WithLogger(c.opts.logger),
		WithLiveMetricsOverlay(false),
		func(o *ClientOptions) { o.mcpTools = c.opts.mcpTools },
		func(o *ClientOptions) { o.mcpToolNames = append([]string(nil), c.opts.mcpToolNames...) },
	)
	client.pendingModelLoadDurations = c.pendingModelLoadDurations
	return client
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
