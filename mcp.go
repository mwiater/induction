package induction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const maxMCPToolTurns = 8

// MCPTool describes a discovered MCP tool presented for side-effect approval.
type MCPTool struct {
	ServerName  string
	Name        string
	Description string
}

// MCPApprovalFunc decides whether a potentially side-effecting MCP tool may be
// called. Read-only tools do not invoke this hook.
type MCPApprovalFunc func(context.Context, MCPTool, json.RawMessage) (bool, error)

type boundMCPTool struct {
	server MCPServerConfig
	client *mcpClient
	tool   mcpTool
}

// InferMCP runs an application-managed MCP tool loop using the servers enabled
// in induction.yaml. Read-only tools run automatically; potentially
// side-effecting tools are denied. Use InferMCPWithApproval when an application
// needs to approve such calls explicitly.
func InferMCP(ctx context.Context, req *ChatRequest, options ...ClientOption) (*InferenceResponse, error) {
	return InferMCPWithApproval(ctx, req, nil, options...)
}

// InferMCPWithApproval is InferMCP with an explicit approval callback for tools
// that are not annotated as read-only by their MCP server.
func InferMCPWithApproval(ctx context.Context, req *ChatRequest, approve MCPApprovalFunc, options ...ClientOption) (*InferenceResponse, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Model == "" {
		return nil, errors.New("request model is required")
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	overlay, options, ownsOverlay := prepareMCPOverlay(ctx, cfg, req.Model, options)
	if ownsOverlay {
		defer overlay.Stop()
	}
	updateMCPStatus(options, "  [Induction: MCP] Discovering tools… ")
	timeout := time.Duration(cfg.Timeout)
	discoveryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	bound, err := discoverMCPTools(discoveryCtx, cfg, timeout)
	if err != nil {
		updateMCPStatus(options, "  [Induction: MCP] Tool discovery failed ")
		return nil, err
	}
	updateMCPStatus(options, fmt.Sprintf("  [Induction: MCP] %d tools available ", len(bound)))
	options = append(options, withMCPTools(mcpToolNames(bound)...))
	return runMCPToolLoop(ctx, req, bound, timeout, approve, options...)
}

func mcpToolNames(tools []boundMCPTool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.tool.Name)
	}
	return names
}

func prepareMCPOverlay(ctx context.Context, cfg *Config, model string, options []ClientOption) (*liveMetricsOverlay, []ClientOption, bool) {
	client := newClientFromConfig(ctx, cfg, options...)
	if client.opts.liveMetricsOverlay != nil {
		return client.opts.liveMetricsOverlay, options, false
	}
	if !client.opts.enableLiveMetricsOverlay {
		return nil, options, false
	}
	overlay := startMCPMetricsOverlay(model)
	if overlay == nil {
		return nil, options, false
	}
	return overlay, append(options, withLiveMetricsOverlay(overlay)), true
}

func updateMCPStatus(options []ClientOption, status string) {
	opts := &ClientOptions{}
	for _, option := range options {
		option(opts)
	}
	if opts.liveMetricsOverlay != nil {
		opts.liveMetricsOverlay.UpdateMCP(status)
	}
}

func discoverMCPTools(ctx context.Context, cfg *Config, timeout time.Duration) ([]boundMCPTool, error) {
	var bound []boundMCPTool
	seen := make(map[string]string)
	for _, server := range cfg.MCPServers {
		if !server.Allow {
			continue
		}
		client := &mcpClient{url: server.URL, httpClient: &http.Client{Timeout: timeout}}
		if err := client.initialize(ctx); err != nil {
			return nil, fmt.Errorf("MCP server %q: %w", server.Name, err)
		}
		tools, err := client.listTools(ctx)
		if err != nil {
			return nil, fmt.Errorf("MCP server %q: %w", server.Name, err)
		}
		for _, tool := range tools {
			if other, exists := seen[tool.Name]; exists {
				return nil, fmt.Errorf("MCP tool %q is exposed by both %q and %q; rename one tool to make routing unambiguous", tool.Name, other, server.Name)
			}
			seen[tool.Name] = server.Name
			bound = append(bound, boundMCPTool{server: server, client: client, tool: tool})
		}
	}
	if len(bound) == 0 {
		return nil, errors.New("configuration contains no tools from allowed MCP servers")
	}
	return bound, nil
}

func runMCPToolLoop(ctx context.Context, req *ChatRequest, tools []boundMCPTool, timeout time.Duration, approve MCPApprovalFunc, options ...ClientOption) (*InferenceResponse, error) {
	status := func(message string) { updateMCPStatus(options, message) }
	return runMCPToolLoopWith(ctx, req, tools, timeout, approve, status, func(ctx context.Context, req *ChatRequest) (*InferenceResponse, error) {
		return Infer(ctx, req, options...)
	})
}

func runMCPToolLoopWith(ctx context.Context, req *ChatRequest, tools []boundMCPTool, timeout time.Duration, approve MCPApprovalFunc, status func(string), inferTurn func(context.Context, *ChatRequest) (*InferenceResponse, error)) (*InferenceResponse, error) {
	if status == nil {
		status = func(string) {}
	}
	request := cloneChatRequest(req)
	toolByName := make(map[string]boundMCPTool, len(tools))
	request.Tools = make([]Tool, 0, len(tools))
	for _, binding := range tools {
		tool := binding.tool
		var schema map[string]any
		if !json.Valid(tool.InputSchema) || json.Unmarshal(tool.InputSchema, &schema) != nil || schema["type"] != "object" {
			return nil, fmt.Errorf("MCP server %q tool %q has malformed inputSchema", binding.server.Name, tool.Name)
		}
		toolByName[tool.Name] = binding
		request.Tools = append(request.Tools, Tool{Type: "function", Function: ToolFunction{Name: tool.Name, Description: tool.Description, Parameters: schema}})
	}
	request.ToolChoice = "auto"
	for turn := 0; turn < maxMCPToolTurns; turn++ {
		response, err := inferTurn(ctx, &request)
		if err != nil {
			return nil, fmt.Errorf("infer with MCP tools: %w", err)
		}
		if len(response.Choices) == 0 || response.Choices[0].Message == nil {
			return nil, errors.New("model returned no assistant message")
		}
		assistant := response.Choices[0].Message
		if len(assistant.ToolCalls) == 0 {
			status("  [Induction: MCP] Complete ")
			return response, nil
		}
		request.Messages = append(request.Messages, Message{Role: "assistant", Content: assistant.Content, ToolCalls: assistant.ToolCalls})
		for _, call := range assistant.ToolCalls {
			binding, ok := toolByName[call.Function.Name]
			if !ok {
				return nil, fmt.Errorf("model requested unknown MCP tool %q", call.Function.Name)
			}
			status(fmt.Sprintf("  [Induction: MCP] %s · requested ", binding.tool.Name))
			args := json.RawMessage(call.Function.Arguments)
			if !json.Valid(args) {
				return nil, fmt.Errorf("model returned malformed arguments for MCP tool %q", binding.tool.Name)
			}
			if !binding.tool.Annotations.ReadOnlyHint {
				if approve == nil {
					status(fmt.Sprintf("  [Induction: MCP] %s · denied ", binding.tool.Name))
					return nil, fmt.Errorf("MCP tool %q on server %q may have side effects and was not approved", binding.tool.Name, binding.server.Name)
				}
				approved, err := approve(ctx, MCPTool{ServerName: binding.server.Name, Name: binding.tool.Name, Description: binding.tool.Description}, args)
				if err != nil {
					return nil, fmt.Errorf("approve MCP tool %q: %w", binding.tool.Name, err)
				}
				if !approved {
					status(fmt.Sprintf("  [Induction: MCP] %s · denied ", binding.tool.Name))
					return nil, fmt.Errorf("MCP tool %q on server %q may have side effects and was not approved", binding.tool.Name, binding.server.Name)
				}
			}
			status(fmt.Sprintf("  [Induction: MCP] %s · running… ", binding.tool.Name))
			started := time.Now()
			callCtx, cancel := context.WithTimeout(ctx, timeout)
			result, err := binding.client.callTool(callCtx, binding.tool.Name, args)
			cancel()
			if err != nil {
				status(fmt.Sprintf("  [Induction: MCP] %s · failed · %s ", binding.tool.Name, err))
				return nil, err
			}
			resultStatus := "completed"
			if result.IsError {
				resultStatus = "tool error"
			}
			status(fmt.Sprintf("  [Induction: MCP] %s · %s · %s · %s ", binding.tool.Name, resultStatus, summarizeMCPResult(result), time.Since(started).Round(time.Millisecond)))
			request.Messages = append(request.Messages, Message{Role: "tool", ToolCallID: call.ID, Name: binding.tool.Name, Content: formatMCPToolResult(result)})
		}
	}
	return nil, fmt.Errorf("model exceeded the %d-turn MCP tool-call limit", maxMCPToolTurns)
}

func summarizeMCPResult(result mcpCallResult) string {
	counts := make(map[string]int)
	for _, content := range result.Content {
		kind := content.Type
		if kind == "" {
			kind = "unknown"
		}
		counts[kind]++
	}
	parts := make([]string, 0, len(counts)+1)
	for kind, count := range counts {
		parts = append(parts, fmt.Sprintf("%s×%d", kind, count))
	}
	sort.Strings(parts)
	if len(result.StructuredContent) > 0 {
		parts = append(parts, "structured")
	}
	if len(parts) == 0 {
		return "empty result"
	}
	return strings.Join(parts, " + ")
}

func formatMCPToolResult(result mcpCallResult) string {
	parts := make([]string, 0, len(result.Content)+2)
	for _, item := range result.Content {
		if item.Type == "text" {
			parts = append(parts, item.Text)
		} else {
			parts = append(parts, fmt.Sprintf("[unsupported MCP content type %q: %s]", item.Type, compactMCPJSON(item.Data)))
		}
	}
	if len(result.StructuredContent) > 0 {
		parts = append(parts, "structured_content: "+compactMCPJSON(result.StructuredContent))
	}
	if result.IsError {
		parts = append([]string{"MCP tool reported an error."}, parts...)
	}
	return joinNonEmpty(parts)
}

func compactMCPJSON(data []byte) string {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return string(data)
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func joinNonEmpty(parts []string) string {
	result := ""
	for _, part := range parts {
		if result != "" {
			result += "\n"
		}
		result += part
	}
	return result
}
