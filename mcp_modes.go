package induction

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// InferMCPChat runs a multi-turn, non-streaming chat session with the MCP tools
// enabled in induction.yaml. Read-only tools run automatically.
func InferMCPChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, options ...ClientOption) error {
	return InferMCPChatWithApproval(ctx, req, in, out, nil, options...)
}

// InferMCPChatWithApproval is InferMCPChat with an explicit approval callback
// for tools that are not annotated as read-only by their MCP server.
func InferMCPChatWithApproval(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, approve MCPApprovalFunc, options ...ClientOption) error {
	if req == nil {
		return errors.New("request is nil")
	}
	if in == nil {
		return errors.New("input reader is nil")
	}
	if out == nil {
		return errors.New("output writer is nil")
	}
	if canRunConsole(in, out) {
		return runConsoleMCPChat(ctx, req, in, out, approve, options...)
	}
	tools, timeout, overlay, ownsOverlay, options, err := configuredMCPTools(ctx, req, options)
	if err != nil {
		return err
	}
	if ownsOverlay {
		defer overlay.Stop()
	}
	request := cloneChatRequest(req)
	session, err := newUnsavedChatSession(request)
	if err != nil {
		return err
	}
	if err := saveChatSession(session); err != nil {
		return err
	}
	options = append(options, withMCPTools(mcpToolNames(tools)...))
	return runChatSession(ctx, in, out, &request, func(turn *ChatRequest) (string, error) {
		response, err := runMCPToolLoop(ctx, turn, tools, timeout, approve, options...)
		if err != nil {
			session.Messages = cloneMessages(turn.Messages)
			_ = saveChatSession(session)
			return "", err
		}
		content := inferenceResponseContent(response)
		session.Messages = append(cloneMessages(turn.Messages), Message{Role: "assistant", Content: content})
		if err := saveChatSession(session); err != nil {
			return "", err
		}
		if _, err := fmt.Fprintln(out, content); err != nil {
			return "", fmt.Errorf("write assistant response: %w", err)
		}
		return content, nil
	})
}

func configuredMCPTools(ctx context.Context, req *ChatRequest, options []ClientOption) ([]boundMCPTool, time.Duration, *liveMetricsOverlay, bool, []ClientOption, error) {
	if req.Model == "" {
		return nil, 0, nil, false, options, errors.New("request model is required")
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil, 0, nil, false, options, err
	}
	overlay, options, ownsOverlay := prepareMCPOverlay(ctx, cfg, req.Model, options)
	updateMCPStatus(options, "  [Induction: MCP] Discovering tools… ")
	timeout := time.Duration(cfg.Timeout)
	discoveryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	tools, err := discoverMCPTools(discoveryCtx, cfg, timeout)
	if err != nil {
		updateMCPStatus(options, "  [Induction: MCP] Tool discovery failed ")
	} else {
		updateMCPStatus(options, fmt.Sprintf("  [Induction: MCP] %d tools available ", len(tools)))
	}
	return tools, timeout, overlay, ownsOverlay, options, err
}
