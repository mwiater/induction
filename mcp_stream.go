package induction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"
)

// InferMCPStream runs the configured MCP tool loop with streaming model
// responses and writes generated reasoning/content to out as it arrives.
func InferMCPStream(ctx context.Context, req *ChatRequest, out io.Writer, options ...ClientOption) error {
	return InferMCPStreamWithApproval(ctx, req, out, nil, options...)
}

// InferMCPStreamWithApproval is InferMCPStream with an explicit approval hook
// for MCP tools that are not annotated as read-only.
func InferMCPStreamWithApproval(ctx context.Context, req *ChatRequest, out io.Writer, approve MCPApprovalFunc, options ...ClientOption) error {
	if req == nil {
		return errors.New("request is nil")
	}
	if out == nil {
		return errors.New("output writer is nil")
	}
	if req.Model == "" {
		return errors.New("request model is required")
	}
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	overlay, options, ownsOverlay := prepareMCPOverlay(ctx, cfg, req.Model, options)
	if ownsOverlay {
		defer overlay.Stop()
	}
	updateMCPStatus(options, "  [Induction: MCP] Discovering tools… ")
	timeout := time.Duration(cfg.Timeout)
	discoveryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	tools, err := discoverMCPTools(discoveryCtx, cfg, timeout)
	if err != nil {
		updateMCPStatus(options, "  [Induction: MCP] Tool discovery failed ")
		return err
	}
	updateMCPStatus(options, fmt.Sprintf("  [Induction: MCP] %d tools available ", len(tools)))
	return runMCPStreamToolLoop(ctx, req, out, tools, timeout, approve, options...)
}

type streamedMCPCall struct {
	index     int
	id        string
	callType  string
	name      string
	arguments string
}

type mcpStreamAccumulator struct {
	role      string
	content   string
	reasoning string
	calls     map[int]*streamedMCPCall
}

func (a *mcpStreamAccumulator) add(chunk InferenceStreamChunk) error {
	for _, choice := range chunk.Choices {
		if choice.Index != 0 {
			continue
		}
		delta := choice.Delta
		if delta.Role != "" {
			a.role = delta.Role
		}
		a.content += delta.Content
		a.reasoning += delta.ReasoningContent
		for _, update := range delta.ToolCalls {
			if update.Index < 0 {
				return fmt.Errorf("streamed MCP tool call has negative index %d", update.Index)
			}
			if a.calls == nil {
				a.calls = make(map[int]*streamedMCPCall)
			}
			call := a.calls[update.Index]
			if call == nil {
				call = &streamedMCPCall{index: update.Index}
				a.calls[update.Index] = call
			}
			if update.ID != "" {
				if call.id != "" && call.id != update.ID {
					return fmt.Errorf("conflicting IDs for streamed MCP tool call index %d", update.Index)
				}
				call.id = update.ID
			}
			if update.Type != "" {
				call.callType = update.Type
			}
			call.name += update.Function.Name
			call.arguments += update.Function.Arguments
		}
	}
	return nil
}

func (a *mcpStreamAccumulator) message() (*InferenceResponseMessage, error) {
	message := &InferenceResponseMessage{Role: a.role, Content: a.content, ReasoningContent: a.reasoning}
	indices := make([]int, 0, len(a.calls))
	for index := range a.calls {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		call := a.calls[index]
		if call.id == "" || call.name == "" {
			return nil, fmt.Errorf("incomplete streamed MCP tool call at index %d", index)
		}
		message.ToolCalls = append(message.ToolCalls, InferenceToolCall{ID: call.id, Type: call.callType, Function: InferenceFunctionCall{Name: call.name, Arguments: call.arguments}})
	}
	return message, nil
}

func runMCPStreamToolLoop(ctx context.Context, req *ChatRequest, out io.Writer, tools []boundMCPTool, timeout time.Duration, approve MCPApprovalFunc, options ...ClientOption) error {
	request := cloneChatRequest(req)
	toolByName := make(map[string]boundMCPTool, len(tools))
	request.Tools = make([]Tool, 0, len(tools))
	for _, binding := range tools {
		var schema map[string]any
		if !json.Valid(binding.tool.InputSchema) || json.Unmarshal(binding.tool.InputSchema, &schema) != nil || schema["type"] != "object" {
			return fmt.Errorf("MCP server %q tool %q has malformed inputSchema", binding.server.Name, binding.tool.Name)
		}
		toolByName[binding.tool.Name] = binding
		request.Tools = append(request.Tools, Tool{Type: "function", Function: ToolFunction{Name: binding.tool.Name, Description: binding.tool.Description, Parameters: schema}})
	}
	request.ToolChoice = "auto"
	for turn := 0; turn < maxMCPToolTurns; turn++ {
		accumulator := new(mcpStreamAccumulator)
		err := renderInferenceStream(out, func(yield func(InferenceStreamChunk) error) error {
			return InferStreamChunks(ctx, &request, func(chunk InferenceStreamChunk) error {
				if err := accumulator.add(chunk); err != nil {
					return err
				}
				return yield(chunk)
			}, options...)
		})
		if err != nil {
			return fmt.Errorf("stream inference with MCP tools: %w", err)
		}
		assistant, err := accumulator.message()
		if err != nil {
			return err
		}
		if len(assistant.ToolCalls) == 0 {
			updateMCPStatus(options, "  [Induction: MCP] Complete ")
			return nil
		}
		request.Messages = append(request.Messages, Message{Role: "assistant", Content: assistant.Content, ToolCalls: assistant.ToolCalls})
		for _, call := range assistant.ToolCalls {
			binding, ok := toolByName[call.Function.Name]
			if !ok {
				return fmt.Errorf("model requested unknown MCP tool %q", call.Function.Name)
			}
			updateMCPStatus(options, fmt.Sprintf("  [Induction: MCP] %s · requested ", binding.tool.Name))
			args := json.RawMessage(call.Function.Arguments)
			if !json.Valid(args) {
				return fmt.Errorf("model returned malformed arguments for MCP tool %q: %q", binding.tool.Name, call.Function.Arguments)
			}
			if !binding.tool.Annotations.ReadOnlyHint {
				if approve == nil {
					updateMCPStatus(options, fmt.Sprintf("  [Induction: MCP] %s · denied ", binding.tool.Name))
					return fmt.Errorf("MCP tool %q on server %q may have side effects and was not approved", binding.tool.Name, binding.server.Name)
				}
				approved, err := approve(ctx, MCPTool{ServerName: binding.server.Name, Name: binding.tool.Name, Description: binding.tool.Description}, args)
				if err != nil {
					return fmt.Errorf("approve MCP tool %q: %w", binding.tool.Name, err)
				}
				if !approved {
					updateMCPStatus(options, fmt.Sprintf("  [Induction: MCP] %s · denied ", binding.tool.Name))
					return fmt.Errorf("MCP tool %q on server %q may have side effects and was not approved", binding.tool.Name, binding.server.Name)
				}
			}
			updateMCPStatus(options, fmt.Sprintf("  [Induction: MCP] %s · running… ", binding.tool.Name))
			started := time.Now()
			callCtx, cancel := context.WithTimeout(ctx, timeout)
			result, err := binding.client.callTool(callCtx, binding.tool.Name, args)
			cancel()
			if err != nil {
				updateMCPStatus(options, fmt.Sprintf("  [Induction: MCP] %s · failed · %s ", binding.tool.Name, err))
				return err
			}
			resultStatus := "completed"
			if result.IsError {
				resultStatus = "tool error"
			}
			updateMCPStatus(options, fmt.Sprintf("  [Induction: MCP] %s · %s · %s · %s ", binding.tool.Name, resultStatus, summarizeMCPResult(result), time.Since(started).Round(time.Millisecond)))
			request.Messages = append(request.Messages, Message{Role: "tool", ToolCallID: call.ID, Name: binding.tool.Name, Content: formatMCPToolResult(result)})
		}
	}
	return fmt.Errorf("model exceeded the %d-turn MCP tool-call limit", maxMCPToolTurns)
}
