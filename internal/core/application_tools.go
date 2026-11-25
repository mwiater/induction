// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// ApplicationToolHandler executes an application-managed tool call and
// returns the tool result to the model.
// ApplicationToolHandler is an exported type in this package's public data model.
type ApplicationToolHandler func(context.Context, string, string) (string, error)

// ApplicationToolChain can add related calls to the model's requested calls.
// ApplicationToolChain is an exported type in this package's public data model.
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

// runApplicationToolLoopWith performs the internal operation described by its signature.
func runApplicationToolLoopWith(ctx context.Context, req *ChatRequest, handler ApplicationToolHandler, chain ApplicationToolChain, infer func(context.Context, *ChatRequest) (*InferenceResponse, *ModelSnapshot, error)) (*InferenceResponse, *ModelSnapshot, error) {
	return runApplicationToolLoopWithObserver(ctx, req, handler, chain, infer, nil)
}

// runApplicationToolLoopWithObserver performs the internal operation described by its signature.
func runApplicationToolLoopWithObserver(ctx context.Context, req *ChatRequest, handler ApplicationToolHandler, chain ApplicationToolChain, infer func(context.Context, *ChatRequest) (*InferenceResponse, *ModelSnapshot, error), observe func(InferenceToolCall)) (*InferenceResponse, *ModelSnapshot, error) {
	request := cloneChatRequest(req)
	lastToolCall := ""
	repeatedToolCalls := 0
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
			callKey := call.Function.Name + "\x00" + call.Function.Arguments
			if callKey == lastToolCall {
				repeatedToolCalls++
			} else {
				lastToolCall = callKey
				repeatedToolCalls = 1
			}
			result, err := handler(ctx, call.Function.Name, call.Function.Arguments)
			if err != nil {
				// Tool execution failures are part of the tool conversation. Return
				// the failure to the model so it can adjust the request or explain
				// that the requested resource is unavailable, just as it can with a
				// tool-reported error from an MCP server.
				result = fmt.Sprintf("application tool %q failed: %v", call.Function.Name, err)
				if repeatedToolCalls > 1 {
					result += "; this is a repeated failure, so do not retry the same call"
				}
			}
			request.Messages = append(request.Messages, Message{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: result})
		}
	}
	if lastToolCall != "" {
		return nil, nil, fmt.Errorf("model exceeded the application tool-call limit while repeating %q", strings.SplitN(lastToolCall, "\x00", 2)[0])
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

// decodeSnapshotResponse performs the internal operation described by its signature.
// Additional context: Streaming snapshots persist the stream as an array of chunks. Rebuild the assistant message so application tool loops can inspect tool calls exactly as they do for a non-streaming response.
func decodeSnapshotResponse(snapshot *ModelSnapshot) (*InferenceResponse, error) {
	if snapshot == nil || len(snapshot.Interaction) == 0 {
		return nil, errors.New("snapshot contains no interaction")
	}
	var response InferenceResponse
	raw := []byte(snapshot.Interaction[len(snapshot.Interaction)-1].Response)
	if err := json.Unmarshal(raw, &response); err == nil {
		return &response, nil
	}

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
