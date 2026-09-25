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

// InferMCPStreamChat runs a multi-turn MCP chat using streaming model
// responses while retaining the MCP tool loop between responses.
func InferMCPStreamChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, options ...ClientOption) error {
	return inferMCPStreamChatWithApproval(ctx, req, in, out, nil, options...)
}

func inferMCPStreamChatWithApproval(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, approve MCPApprovalFunc, options ...ClientOption) error {
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
	options = append(options, withMCPTools(mcpToolNames(tools)...))
	cfg, err := loadConfigForOptions(options)
	if err != nil {
		return err
	}
	configureSessionMode(session, newClientFromConfig(ctx, cfg, options...).opts.pipeline)
	if err := saveChatSession(session); err != nil {
		return err
	}
	return runChatSession(ctx, in, out, &request, func(turn *ChatRequest) (string, error) {
		assistant, streamErr := runMCPStreamToolLoopResult(ctx, turn, out, tools, timeout, approve, options...)
		if streamErr != nil {
			session.Messages = cloneMessages(turn.Messages)
			_ = saveChatSession(session)
			return "", streamErr
		}
		content := ""
		if assistant != nil {
			content = assistant.Content
		}
		session.Messages = append(cloneMessages(turn.Messages), Message{Role: "assistant", Content: content})
		if err := saveChatSession(session); err != nil {
			return "", err
		}
		return content, nil
	})
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
	options = append(options, withMCPTools(mcpToolNames(tools)...))
	cfg, err := loadConfigForOptions(options)
	if err != nil {
		return err
	}
	snapshotClient := newClientFromConfig(ctx, cfg, options...)
	configureSessionMode(session, snapshotClient.opts.pipeline)
	if err := saveChatSession(session); err != nil {
		return err
	}
	return runChatSession(ctx, in, out, &request, func(turn *ChatRequest) (string, error) {
		var latestSnapshot *ModelSnapshot
		response, err := runMCPToolLoopWith(ctx, turn, tools, timeout, approve, func(message string) { updateMCPStatus(options, message) }, func(inferCtx context.Context, toolTurn *ChatRequest) (*InferenceResponse, error) {
			snapshot, inferErr := snapshotClient.GenerateSnapshot(inferCtx, toolTurn)
			if inferErr != nil {
				return nil, inferErr
			}
			latestSnapshot = snapshot
			return decodeSnapshotResponse(snapshot)
		})
		if err != nil {
			session.Messages = cloneMessages(turn.Messages)
			_ = saveChatSession(session)
			return "", err
		}
		content := inferenceResponseContent(response)
		session.Messages = append(cloneMessages(turn.Messages), Message{Role: "assistant", Content: content})
		if latestSnapshot != nil {
			session.Snapshots = append(session.Snapshots, latestSnapshot)
			session.Model = latestSnapshot.ModelID
			updateSessionFinalOutput(session)
		}
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
	cfg, err := loadConfigForOptions(options)
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
