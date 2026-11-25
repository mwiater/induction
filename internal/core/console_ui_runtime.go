// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// runConsoleChat performs the internal operation described by its signature.
// Additional context: Bubble Tea owns the terminal in interactive mode. Keep diagnostics in the configured file instead of mirroring them onto the UI's stderr.
// Additional context: Overlay methods are also called from consoleModel.Update. Sending synchronously from that event loop would deadlock on Bubble Tea's unbuffered message channel, so always publish asynchronously.
func runConsoleChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, mode consoleMode, options ...ClientOption) ([]*ModelSnapshot, error) {
	cfg, err := loadConfigForOptions(options)
	if err != nil {
		return nil, err
	}
	request := cloneChatRequest(req)
	clientOptions := append([]ClientOption(nil), options...)

	clientOptions = append(clientOptions, WithLogger(configuredUILogger(cfg.Log)))
	client := newClientFromConfig(ctx, cfg, clientOptions...)
	if request.Model == "" && !client.opts.initialModelSelection {
		return nil, fmt.Errorf("request model is required")
	}
	client.logf("ui: chat session starting model=%q mode=%d pipeline=%t steps=%d", request.Model, mode, client.opts.pipeline != nil, pipelineStepCount(client.opts.pipeline))
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	model := newConsoleModel(runCtx, client, request, time.Duration(cfg.Timeout), cfg.SidebarWidth, mode)
	model.cancel = cancel
	if client.opts.initialModelSelection {
		model.sessionView = sessionViewModels
		model.loading = false
		model.input.Blur()
		model.footer = "Select a model"
	}
	if client.opts.pipeline != nil {
		model.pipelineInputs, err = preparePipelineRuntimeInputs(client.opts.pipeline.Inputs)
		if err != nil {
			return nil, err
		}
		if prompt := strings.TrimSpace(client.opts.initialChatPrompt); prompt != "" {
			model.pipelineInputs["prompt"] = prompt
		}
		if err := preparePipelineRuntimeContext(runCtx, client, client.opts.pipeline, model.pipelineInputs); err != nil {
			return nil, err
		}
		if len(client.opts.pipeline.Steps) > 0 {
			model.mcpFooter = formatPipelineMCPFooter(0, client.opts.pipeline.Steps[0].UseMCP)
		}
	} else if client.opts.mcpFooterConfigured {
		model.mcpFooter = formatPipelineMCPFooter(0, client.opts.mcpFooterEnabled)
	}
	model.session, err = newUnsavedChatSession(request)
	if client.opts.initialModelSelection {
		model.session = nil
		err = nil
	} else {
		if model.session != nil {
			model.session.BatchID = client.opts.batchID
			model.session.BatchItemID = client.opts.batchItemID
		}
		configureSessionMode(model.session, client.opts.pipeline)
		if model.session != nil {
			model.pipelineRunID = model.session.ID
			client.logf("session: creating id=%q path=%q saved=%t pipeline=%t", model.session.ID, sessionPathForLog(model.session.ID), model.session.Saved, client.opts.pipeline != nil)
		}
		if err == nil {
			err = saveChatSession(model.session)
		}
	}
	if err != nil {
		client.logf("session: initial persistence failed id=%q error=%v", sessionID(model.session), err)
		return nil, err
	}
	if model.session != nil {
		client.logf("session: initial persistence complete id=%q path=%q", model.session.ID, sessionPathForLog(model.session.ID))
		if client.opts.sessionSaved != nil {
			client.opts.sessionSaved(chatSessionPath(model.session.ID))
		}
	}

	var program *tea.Program
	var monitor *inferenceMonitor
	overlay := &liveMetricsOverlay{startedAt: time.Now(), model: request.Model, notify: func(update overlayUpdate) {
		if program != nil {

			go program.Send(consoleOverlayMsg(update))
		}
	}}
	model.complete = overlay.Complete
	model.modelOverlay = overlay
	model.send = func(msg tea.Msg) {
		if program != nil {
			program.Send(msg)
		}
	}
	model.startMonitor = func() tea.Cmd {
		if strings.TrimSpace(model.request.Model) == "" {
			return nil
		}
		return func() tea.Msg {
			monitor = client.startInferenceMonitorWithOverlay(runCtx, request.Model, false, overlay)
			return consoleMonitorStartedMsg{monitor: monitor}
		}
	}
	program = tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(out), tea.WithContext(runCtx), tea.WithAltScreen())
	final, runErr := program.Run()
	if monitor != nil {
		monitor.Stop()
	}
	result, ok := final.(consoleModel)
	if runErr != nil {
		if ok && result.err != nil {
			return nil, result.err
		}
		return nil, runErr
	}
	if !ok {
		return nil, nil
	}
	if result.modelMonitor != nil && result.modelMonitor != monitor {
		result.modelMonitor.Stop()
	}
	return result.snapshots, result.err
}

// pipelineStepCount performs the internal operation described by its signature.
func pipelineStepCount(pipeline *Pipeline) int {
	if pipeline == nil {
		return 0
	}
	return len(pipeline.Steps)
}

// runConsoleMCPChat performs the internal operation described by its signature.
// Additional context: pipelineStep is maintained by the pipeline scheduler. Inferring it from message history is incorrect for fan-out steps and for turns containing system/profile messages.
// Additional context: See runConsoleChat: model updates can originate from Update itself.
func runConsoleMCPChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, approve MCPApprovalFunc, options ...ClientOption) error {
	cfg, err := loadConfigForOptions(options)
	if err != nil {
		return err
	}
	request := cloneChatRequest(req)
	timeout := time.Duration(cfg.Timeout)
	discoveryCtx, cancel := context.WithTimeout(ctx, timeout)
	tools, err := discoverMCPTools(discoveryCtx, cfg, timeout)
	cancel()
	if err != nil {
		return err
	}

	clientOptions := append([]ClientOption(nil), options...)
	clientOptions = append(clientOptions, WithLiveMetricsOverlay(false), WithLogger(configuredUILogger(cfg.Log)))
	client := newClientFromConfig(ctx, cfg, clientOptions...)
	if request.Model == "" && !client.opts.initialModelSelection {
		return fmt.Errorf("request model is required")
	}
	client.opts.mcpTools = true
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	model := newConsoleModel(runCtx, client, request, timeout, cfg.SidebarWidth, consoleNonStreaming)
	model.cancel = cancel
	if client.opts.initialModelSelection {
		model.sessionView = sessionViewModels
		model.loading = false
		model.input.Blur()
		model.footer = "Select a model"
	}
	if client.opts.pipeline != nil {
		model.pipelineInputs, err = preparePipelineRuntimeInputs(client.opts.pipeline.Inputs)
		if err != nil {
			return err
		}
		if prompt := strings.TrimSpace(client.opts.initialChatPrompt); prompt != "" {
			model.pipelineInputs["prompt"] = prompt
		}
		if err := preparePipelineRuntimeContext(runCtx, client, client.opts.pipeline, model.pipelineInputs); err != nil {
			return err
		}
	}
	model.session, err = newUnsavedChatSession(request)
	if client.opts.initialModelSelection {
		model.session = nil
		err = nil
	} else {
		configureSessionMode(model.session, client.opts.pipeline)
		if model.session != nil {
			model.pipelineRunID = model.session.ID
			client.logf("session: creating id=%q path=%q saved=%t mcp=true", model.session.ID, sessionPathForLog(model.session.ID), model.session.Saved)
		}
		if err == nil {
			err = saveChatSession(model.session)
		}
	}
	if err != nil {
		client.logf("session: initial persistence failed id=%q error=%v", sessionID(model.session), err)
		return err
	}
	if model.session != nil {
		client.logf("session: initial persistence complete id=%q path=%q", model.session.ID, sessionPathForLog(model.session.ID))
		if client.opts.sessionSaved != nil {
			client.opts.sessionSaved(chatSessionPath(model.session.ID))
		}
	}
	model.mcpFooter = fmt.Sprintf("  [Induction: MCP] %d tools available ", len(tools))
	model.mcpTools = tools
	client.opts.mcpToolNames = mcpToolNames(tools)
	if model.pipeline != nil && len(model.pipeline.Steps) > 0 {
		model.mcpFooter = formatPipelineMCPFooter(len(tools), model.pipeline.Steps[0].UseMCP)
	}

	var program *tea.Program
	var monitor *inferenceMonitor
	status := func(message string) {
		client.logf("tool call: type=mcp status=%q", strings.TrimSpace(message))
		if program != nil {
			program.Send(consoleMCPMsg(message))
			if activity, ok := formatMCPToolActivity(message, tools); ok {
				program.Send(consoleMCPActivityMsg(activity))
			}
		}
	}
	model.inferSnapshotTurn = func(turnCtx context.Context, turn *ChatRequest) (*InferenceResponse, *ModelSnapshot, error) {

		pipelineStep := model.pipelineStep
		if model.pipeline != nil && pipelineStep >= 0 && pipelineStep < len(model.pipeline.Steps) && !model.pipeline.Steps[pipelineStep].UseMCP {
			reasoningOpen := false
			snapshot, err := client.withoutLiveMetricsOverlay(turnCtx).GenerateStreamingSnapshot(turnCtx, turn, func(chunk InferenceStreamChunk) error {
				model.sendConsoleInferenceChunk(chunk, &reasoningOpen)
				return nil
			})
			model.closeConsoleInferenceReasoning(&reasoningOpen)
			if err != nil {
				return nil, snapshot, err
			}
			response, err := decodeSnapshotResponse(snapshot)
			return response, snapshot, err
		}
		var finalSnapshot *ModelSnapshot
		var modelLoadTime time.Duration
		response, err := runMCPToolLoopWith(turnCtx, turn, tools, timeout, approve, status, func(inferCtx context.Context, toolTurn *ChatRequest) (*InferenceResponse, error) {
			reasoningOpen := false
			snapshot, err := client.withoutLiveMetricsOverlay(inferCtx).GenerateStreamingSnapshot(inferCtx, toolTurn, func(chunk InferenceStreamChunk) error {
				model.sendConsoleInferenceChunk(chunk, &reasoningOpen)
				return nil
			})
			model.closeConsoleInferenceReasoning(&reasoningOpen)
			if err != nil {
				return nil, err
			}
			if snapshot == nil || len(snapshot.Interaction) == 0 {
				return nil, fmt.Errorf("MCP snapshot contains no interaction")
			}
			if snapshot.ModelLoadTime > modelLoadTime {
				modelLoadTime = snapshot.ModelLoadTime
			}
			if snapshot.ModelLoadTime == 0 {
				snapshot.ModelLoadTime = modelLoadTime
			}
			response, err := decodeSnapshotResponse(snapshot)
			if err != nil {
				return nil, fmt.Errorf("decode MCP snapshot response: %w", err)
			}
			finalSnapshot = snapshot
			return response, nil
		})
		return response, finalSnapshot, err
	}

	overlay := &liveMetricsOverlay{startedAt: time.Now(), model: request.Model, notify: func(update overlayUpdate) {
		if program != nil {

			go program.Send(consoleOverlayMsg(update))
		}
	}}
	model.complete = overlay.Complete
	model.modelOverlay = overlay
	model.send = func(msg tea.Msg) {
		if program != nil {
			program.Send(msg)
		}
	}
	model.startMonitor = func() tea.Cmd {
		if strings.TrimSpace(model.request.Model) == "" {
			return nil
		}
		return func() tea.Msg {
			monitor = client.startInferenceMonitorWithOverlay(runCtx, request.Model, false, overlay)
			return consoleMonitorStartedMsg{monitor: monitor}
		}
	}
	program = tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(out), tea.WithContext(runCtx), tea.WithAltScreen())
	final, runErr := program.Run()
	if monitor != nil {
		monitor.Stop()
	}
	result, ok := final.(consoleModel)
	if runErr != nil {
		if ok && result.err != nil {
			return result.err
		}
		return runErr
	}
	if ok {
		if result.modelMonitor != nil && result.modelMonitor != monitor {
			result.modelMonitor.Stop()
		}
		return result.err
	}
	return nil
}

// formatMCPToolActivity performs the internal operation described by its signature.
func formatMCPToolActivity(message string, tools []boundMCPTool) (string, bool) {
	const prefix = "[Induction: MCP] "
	status := strings.TrimSpace(message)
	status = strings.TrimPrefix(status, prefix)
	parts := strings.Split(status, " · ")
	if len(parts) < 2 {
		return "", false
	}
	descriptor := parts[0]
	toolName := descriptor
	arguments := ""
	if open := strings.IndexByte(descriptor, '('); open >= 0 && strings.HasSuffix(descriptor, ")") {
		toolName = descriptor[:open]
		arguments = descriptor[open+1 : len(descriptor)-1]
	}
	label := map[string]string{
		"requested":  "Requested",
		"running…":   "Running",
		"completed":  "Completed",
		"denied":     "Denied",
		"failed":     "Failed",
		"tool error": "Error",
	}[parts[1]]
	if label == "" {
		return "", false
	}
	server := "MCP"
	for _, binding := range tools {
		if binding.tool.Name == toolName {
			if binding.server.Name != "" {
				server = binding.server.Name
			}
			break
		}
	}
	return fmt.Sprintf("   [Tool Call: %s] %s: %s(%s)", label, server, toolName, arguments), true
}
