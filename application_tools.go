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
		_, err := runConsoleChat(ctx, req, in, out, consoleStreaming, options...)
		return err
	}
	return InferStreamChat(ctx, req, in, out, options...)
}

func runApplicationToolLoopWith(ctx context.Context, req *ChatRequest, handler ApplicationToolHandler, chain ApplicationToolChain, infer func(context.Context, *ChatRequest) (*InferenceResponse, *ModelSnapshot, error)) (*InferenceResponse, *ModelSnapshot, error) {
	return runApplicationToolLoopWithObserver(ctx, req, handler, chain, infer, nil)
}

func runApplicationToolLoopWithObserver(ctx context.Context, req *ChatRequest, handler ApplicationToolHandler, chain ApplicationToolChain, infer func(context.Context, *ChatRequest) (*InferenceResponse, *ModelSnapshot, error), observe func(InferenceToolCall)) (*InferenceResponse, *ModelSnapshot, error) {
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
		toolCalls := normalizedToolCallIDs(assistant.ToolCalls)
		if chain != nil {
			toolCalls = chain(toolCalls)
		}
		request.Messages = append(request.Messages, Message{Role: "assistant", Content: assistant.Content, ToolCalls: toolCalls})
		for _, call := range toolCalls {
			if observe != nil {
				observe(call)
			}
			result, err := handler(ctx, call.Function.Name, call.Function.Arguments)
			if err != nil {
				return nil, snapshot, fmt.Errorf("application tool %q: %w", call.Function.Name, err)
			}
			request.Messages = append(request.Messages, Message{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: result})
		}
	}
	return nil, nil, fmt.Errorf("model exceeded the application tool-call limit")
}

// normalizedToolCallIDs gives tool calls template-safe IDs. Several local
// model templates require IDs to be exactly nine alphanumeric characters,
// while other models return IDs such as "call_xxx".
func normalizedToolCallIDs(calls []InferenceToolCall) []InferenceToolCall {
	normalized := append([]InferenceToolCall(nil), calls...)
	for i := range normalized {
		normalized[i].ID = fmt.Sprintf("toolcal%02d", i+1)
	}
	return normalized
}

func decodeSnapshotResponse(snapshot *ModelSnapshot) (*InferenceResponse, error) {
	if snapshot == nil || len(snapshot.Interaction) == 0 {
		return nil, errors.New("snapshot contains no interaction")
	}
	var response InferenceResponse
	raw := []byte(snapshot.Interaction[len(snapshot.Interaction)-1].Response)
	if err := json.Unmarshal(raw, &response); err == nil {
		return &response, nil
	}
	// Streaming snapshots persist the stream as an array of chunks. Rebuild
	// the assistant message so application tool loops can inspect tool calls
	// exactly as they do for a non-streaming response.
	var chunks []InferenceStreamChunk
	if err := json.Unmarshal(raw, &chunks); err != nil {
		return nil, err
	}
	accumulator := new(mcpStreamAccumulator)
	for _, chunk := range chunks {
		if err := accumulator.add(chunk); err != nil {
			return nil, err
		}
	}
	message, err := accumulator.message()
	if err != nil {
		return nil, err
	}
	return &InferenceResponse{Choices: []InferenceChoice{{Index: 0, Message: message}}}, nil
}
