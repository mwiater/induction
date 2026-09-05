package induction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// ApplicationToolHandler executes an application-managed tool call and
// returns the tool result to the model.
type ApplicationToolHandler func(context.Context, string, string) (string, error)

// ApplicationToolChain can add related calls to the model's requested calls.
type ApplicationToolChain func([]InferenceToolCall) []InferenceToolCall

// InferApplicationToolsChat runs a chat session with application-managed
// tools. In a terminal it uses the full console chat UI.
func InferApplicationToolsChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, handler ApplicationToolHandler, options ...ClientOption) error {
	if req == nil {
		return errors.New("request is nil")
	}
	if handler == nil {
		return errors.New("application tool handler is nil")
	}
	options = append(options, WithApplicationToolHandler(handler))
	if canRunConsole(in, out) {
		_, err := runConsoleChat(ctx, req, in, out, consoleNonStreaming, options...)
		return err
	}
	return InferChat(ctx, req, in, out, options...)
}

func runApplicationToolLoopWith(ctx context.Context, req *ChatRequest, handler ApplicationToolHandler, chain ApplicationToolChain, infer func(context.Context, *ChatRequest) (*InferenceResponse, *ModelSnapshot, error)) (*InferenceResponse, *ModelSnapshot, error) {
	request := cloneChatRequest(req)
	var loadTime time.Duration
	for turn := 0; turn < 8; turn++ {
		response, snapshot, err := infer(ctx, &request)
		if err != nil {
			return nil, snapshot, err
		}
		if snapshot != nil && snapshot.ModelLoadTime > 0 && loadTime == 0 {
			loadTime = snapshot.ModelLoadTime
		}
		if len(response.Choices) == 0 || response.Choices[0].Message == nil {
			return nil, snapshot, errors.New("model returned no assistant message")
		}
		assistant := response.Choices[0].Message
		if len(assistant.ToolCalls) == 0 {
			if snapshot != nil && snapshot.ModelLoadTime == 0 {
				snapshot.ModelLoadTime = loadTime
			}
			return response, snapshot, nil
		}
		toolCalls := append([]InferenceToolCall(nil), assistant.ToolCalls...)
		if chain != nil {
			toolCalls = chain(toolCalls)
		}
		request.Messages = append(request.Messages, Message{Role: "assistant", Content: assistant.Content, ToolCalls: toolCalls})
		for _, call := range toolCalls {
			result, err := handler(ctx, call.Function.Name, call.Function.Arguments)
			if err != nil {
				return nil, snapshot, fmt.Errorf("application tool %q: %w", call.Function.Name, err)
			}
			request.Messages = append(request.Messages, Message{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: result})
		}
	}
	return nil, nil, fmt.Errorf("model exceeded the application tool-call limit")
}

func decodeSnapshotResponse(snapshot *ModelSnapshot) (*InferenceResponse, error) {
	if snapshot == nil || len(snapshot.Interaction) == 0 {
		return nil, errors.New("snapshot contains no interaction")
	}
	var response InferenceResponse
	if err := json.Unmarshal([]byte(snapshot.Interaction[len(snapshot.Interaction)-1].Response), &response); err != nil {
		return nil, err
	}
	return &response, nil
}
