// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// runTurn performs the internal operation described by its signature.
func (m consoleModel) runTurn(request ChatRequest) tea.Cmd {
	return func() tea.Msg {
		m.client.logf("inference: started model=%q messages=%d classification=%t streaming=%t", request.Model, len(request.Messages), request.Classification != nil, m.mode == consoleStreaming)
		turnCtx, cancel := context.WithTimeout(m.ctx, m.timeout)
		defer cancel()
		if request.Decision != nil || request.Classification != nil {
			snapshot, err := m.client.withoutLiveMetricsOverlay(turnCtx).GenerateSnapshot(turnCtx, &request)
			if err == nil && m.complete != nil {
				m.complete()
			}
			return consoleTurnResult{content: lastInteractionContent(snapshot), snapshot: snapshot, model: request.Model, err: err}
		}
		if m.client != nil && m.client.opts != nil && m.client.opts.applicationToolHandler != nil {
			m.streamedToolCallNames = make(map[int]string)
			response, snapshot, err := runApplicationToolLoopWithObserver(turnCtx, &request, m.client.opts.applicationToolHandler, m.client.opts.applicationToolChain, func(inferCtx context.Context, turn *ChatRequest) (*InferenceResponse, *ModelSnapshot, error) {
				reasoningOpen := false
				snapshot, err := m.client.withoutLiveMetricsOverlay(inferCtx).GenerateStreamingSnapshot(inferCtx, turn, func(chunk InferenceStreamChunk) error {
					m.sendConsoleInferenceChunk(chunk, &reasoningOpen)
					return nil
				})
				m.closeConsoleInferenceReasoning(&reasoningOpen)
				if err != nil {
					return nil, snapshot, err
				}
				response, err := decodeSnapshotResponse(snapshot)
				return response, snapshot, err
			}, func(call InferenceToolCall) {
				m.client.logf("tool call: type=application name=%q arguments=%s", call.Function.Name, call.Function.Arguments)
				if m.send != nil {
					m.send(consoleToolMsg(fmt.Sprintf("[Tool Call] %s(%s)", call.Function.Name, call.Function.Arguments)))
				}
			})
			if err == nil && m.complete != nil {
				m.complete()
			}
			return consoleTurnResult{content: inferenceResponseContent(response), snapshot: snapshot, model: request.Model, err: err}
		}
		switch m.mode {
		case consoleStreaming:
			m.client.logf("ui: streaming inference started model=%q messages=%d", request.Model, len(request.Messages))
			m.streamedToolCallNames = make(map[int]string)
			reasoningOpen := false
			snapshot, err := m.client.withoutLiveMetricsOverlay(m.ctx).GenerateStreamingSnapshot(turnCtx, &request, func(chunk InferenceStreamChunk) error {
				m.sendConsoleInferenceChunk(chunk, &reasoningOpen)
				return nil
			})
			m.closeConsoleInferenceReasoning(&reasoningOpen)
			if err == nil && m.complete != nil {
				m.complete()
			}
			return consoleTurnResult{content: lastInteractionContent(snapshot), snapshot: snapshot, model: request.Model, err: err}
		default:
			if m.inferSnapshotTurn != nil {
				response, snapshot, err := m.inferSnapshotTurn(turnCtx, &request)
				if err == nil && m.complete != nil {
					m.complete()
				}
				return consoleTurnResult{content: inferenceResponseContent(response), snapshot: snapshot, model: request.Model, err: err}
			}
			if m.inferTurn != nil {
				response, err := m.inferTurn(turnCtx, &request)
				if err == nil && m.complete != nil {
					m.complete()
				}
				return consoleTurnResult{content: inferenceResponseContent(response), model: request.Model, err: err}
			}
			snapshot, err := m.client.withoutLiveMetricsOverlay(m.ctx).GenerateSnapshot(turnCtx, &request)
			if err == nil && m.complete != nil {
				m.complete()
			}
			return consoleTurnResult{content: lastInteractionContent(snapshot), snapshot: snapshot, model: request.Model, err: err}
		}
	}
}

// lastInteractionContent performs the internal operation described by its signature.
func lastInteractionContent(snapshot *ModelSnapshot) string {
	if snapshot == nil || len(snapshot.Interaction) == 0 {
		return ""
	}
	return snapshot.Interaction[len(snapshot.Interaction)-1].Content
}

// Some llama.cpp model templates place a structured answer in the separately
// returned reasoning channel and leave message content empty. For a declared
// JSON pipeline step, use that channel only when it is itself valid JSON (or a
// valid JSON payload wrapped by think markers). Normal chat output is never
// changed by this fallback.
func recoverStructuredPipelineContent(step PipelineStep, content string, snapshot *ModelSnapshot) string {
	if step.Output == nil || !strings.EqualFold(step.Output.Type, "json") || json.Valid([]byte(content)) {
		return content
	}
	candidates := []string{content}
	if snapshot != nil && len(snapshot.Interaction) > 0 {
		candidates = append(candidates, snapshot.Interaction[len(snapshot.Interaction)-1].ReasoningContent)
	}
	for _, candidate := range candidates {
		if recovered := extractStructuredJSON(candidate); recovered != "" {
			return recovered
		}
	}
	return content
}

// extractStructuredJSON tolerates the common markdown/prose wrappers produced
// by models even when JSON output was requested. It only returns a complete,
// syntactically valid JSON value, so ordinary chat output is unaffected.
func extractStructuredJSON(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if strings.HasPrefix(content, "```") {
		if newline := strings.IndexByte(content, '\n'); newline >= 0 {
			content = strings.TrimSpace(content[newline+1:])
		}
		content = strings.TrimSpace(strings.TrimSuffix(content, "```"))
		if json.Valid([]byte(content)) {
			return content
		}
	}
	for start := 0; start < len(content); start++ {
		if content[start] != '{' && content[start] != '[' {
			continue
		}
		depth := 0
		inString := false
		escaped := false
	candidateLoop:
		for end := start; end < len(content); end++ {
			ch := content[end]
			if inString {
				if escaped {
					escaped = false
				} else if ch == '\\' {
					escaped = true
				} else if ch == '"' {
					inString = false
				}
				continue
			}
			switch ch {
			case '"':
				inString = true
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					candidate := content[start : end+1]
					if json.Valid([]byte(candidate)) {
						return candidate
					}
					break candidateLoop
				}
			}
		}
	}
	return ""
}

// previewPipelineOutput performs the internal operation described by its signature.
func previewPipelineOutput(content string) string {
	content = strings.TrimSpace(content)
	if len(content) > 240 {
		return content[:240] + "..."
	}
	return content
}

// resize performs the internal operation described by its signature.
// Additional context: Main header, input header, input row, and one blank row occupy four lines; the remainder belongs to the scrolling transcript. The main view has four fixed rows: the header, User Input header, input line, and trailing spacer. Do not force a transcript row when the terminal is too short; overflow here pushes stale-looking input and footer rows into the next render region.
// Additional context: Match the main pane's final spacer so neither pane touches the footer.
func (m *consoleModel) resize() {
	contentHeight := max(1, m.height-m.footerHeight())
	showSidebar := m.sidebarOpen && m.width >= consoleMinMainWidth+consoleMinSidebar
	sidebarOuter := 0
	if showSidebar {
		sidebarOuter = min(max(consoleMinSidebar, m.sidebarWidth), m.width-consoleMinMainWidth)
	}
	mainWidth := max(1, m.width-sidebarOuter)
	m.transcript.Width = mainWidth

	m.transcript.Height = max(0, contentHeight-4)
	m.input.Width = max(1, mainWidth-lipgloss.Width(m.input.Prompt))
	m.sidebar.Width = max(1, sidebarOuter)

	m.sidebar.Height = max(1, contentHeight-2)
	m.refreshTranscript()
	m.refreshSidebar()
}

// footerHeight performs the internal operation described by its signature.
// Additional context: live metrics and key bindings
func (m consoleModel) footerHeight() int {
	height := consoleFooterHeight + 1
	if m.mcpFooter != "" {
		height++
	}
	if m.pipelineRunning && m.pipeline != nil {
		height++
	}
	return height
}

// renderAssistantMessage performs the internal operation described by its signature.
func renderAssistantMessage(reasoningStyle, contentStyle lipgloss.Style, icon, label, content string) string {
	var out strings.Builder
	iconPrefix := " " + renderWhiteIcon(icon)
	for len(content) > 0 {
		start := strings.Index(content, "<think>")
		if start < 0 {
			out.WriteString(iconPrefix + contentStyle.Render(label+indentContinuationLines(content)))
			break
		}
		if start > 0 {
			out.WriteString(iconPrefix + contentStyle.Render(label+indentContinuationLines(content[:start])))
			iconPrefix = ""
			label = ""
		}
		content = content[start:]
		end := strings.Index(content, "</think>")
		if end < 0 {
			out.WriteString(iconPrefix + reasoningStyle.Render(label+indentContinuationLines(content)))
			break
		}
		end += len("</think>")
		out.WriteString(iconPrefix + reasoningStyle.Render(label+indentContinuationLines(content[:end])))
		iconPrefix = ""
		label = ""
		content = content[end:]
	}
	return out.String()
}

// indentContinuationLines performs the internal operation described by its signature.
func indentContinuationLines(content string) string {
	return strings.ReplaceAll(content, "\n", "\n ")
}

// refreshTranscript performs the internal operation described by its signature.
// Additional context: reserve the message icon
func (m *consoleModel) refreshTranscript() {
	var out strings.Builder
	messageWidth := max(1, m.transcript.Width-2)
	assistantContentStyle := m.mainAssistantContentStyle.Width(messageWidth)
	assistantReasoningStyle := m.mainAssistantReasoningStyle.Width(messageWidth)
	for i, message := range m.messages {
		if i > 0 {
			separator := "\n\n"
			if message.role == "mcp_activity" || m.messages[i-1].role == "mcp_activity" {
				separator = "\n"
			}
			out.WriteString(separator)
		}
		if message.role == "mcp_activity" {
			out.WriteString(m.mcpActivityStyle.Render(message.content))
			continue
		}
		style, icon, label := m.mainAssistantContentStyle, DefaultUnicode.Sparkle, " Assistant: "
		if message.role == "user" {
			style, icon, label = m.mainUserPromptStyle, DefaultUnicode.Star, " You: "
		}
		if message.role == "assistant" {
			out.WriteString(renderAssistantMessage(assistantReasoningStyle, assistantContentStyle, icon, label, message.content))
		} else {
			out.WriteString(" " + renderWhiteIcon(icon) + style.Width(messageWidth).Render(label+indentContinuationLines(message.content)))
		}
	}
	if m.err != nil {
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(m.mainErrorStyle.Render("Error: " + m.err.Error()))
	}
	m.transcript.SetContent(out.String())
	m.transcript.GotoBottom()
}

// sidebarKeys holds package-level state used internally by this package.
var sidebarKeys = []string{
	"n_ctx", "speculative", "is_processing", "seed", "temperature", "top_k", "top_p", "min_p",
	"typical_p", "xtc_probability", "xtc_threshold", "repeat_last_n", "repeat_penalty", "presence_penalty", "frequency_penalty", "dry_multiplier",
	"dry_base", "dry_allowed_length", "dry_penalty_last_n", "max_tokens", "n_predict", "n_keep",
	"n_discard", "stream", "reasoning_in_content", "speculative.types",
}

// refreshSidebar performs the internal operation described by its signature.
// Additional context: Slot and props data describe server/model defaults. Explicit request parameters take precedence in the sidebar so the UI shows the values that will be sent for this chat, rather than only the loaded defaults.
// Additional context: Reapply the sidebar text/background style after each white icon. A nested lipgloss style emits a reset sequence, which would otherwise expose the terminal's default background for the rest of the line.
func (m *consoleModel) refreshSidebar() {
	fields := sidebarFields(m.slots)
	if m.props != nil {
		for key, value := range m.props.DefaultGenerationSettings {
			if _, exists := fields[key]; !exists {
				fields[key] = value
			}
		}
	}

	if m.request.Temperature != nil {
		fields["temperature"] = *m.request.Temperature
	}
	if m.request.TopP != nil {
		fields["top_p"] = *m.request.TopP
	}
	if m.request.TopK != nil {
		fields["top_k"] = *m.request.TopK
	}
	if m.request.MaxTokens != nil {
		fields["max_tokens"] = *m.request.MaxTokens
	}
	if m.request.RepeatPenalty != nil {
		fields["repeat_penalty"] = *m.request.RepeatPenalty
	}
	if m.request.Seed != nil {
		fields["seed"] = *m.request.Seed
	}
	if m.request.MinP != nil {
		fields["min_p"] = *m.request.MinP
	}
	if m.request.XTCProbability != nil {
		fields["xtc_probability"] = *m.request.XTCProbability
	}
	if m.request.XTCThreshold != nil {
		fields["xtc_threshold"] = *m.request.XTCThreshold
	}
	if m.request.TypicalP != nil {
		fields["typical_p"] = *m.request.TypicalP
	}
	if m.request.PresencePenalty != nil {
		fields["presence_penalty"] = *m.request.PresencePenalty
	}
	if m.request.FrequencyPenalty != nil {
		fields["frequency_penalty"] = *m.request.FrequencyPenalty
	}
	if m.request.RepeatLastN != nil {
		fields["repeat_last_n"] = *m.request.RepeatLastN
	}
	keys := append([]string(nil), sidebarKeys...)
	sort.Strings(keys)
	lines := make([]string, 0, len(keys)+1)
	lines = append(lines, "model_id: "+m.request.Model)

	lineStyle := m.sidebarTextStyle
	lineStyle = lineStyle.Padding(0)
	for _, key := range keys {
		value := "n/a"
		if candidate, ok := fields[key]; ok {
			value = formatSidebarValue(candidate)
		}
		lines = append(lines, renderWhiteIcon(DefaultUnicode.DoubleArrowRight)+lineStyle.Render(" "+key+": "+value))
	}
	m.sidebar.SetContent(m.sidebarTextStyle.Width(max(1, m.sidebar.Width-2)).Render(strings.Join(lines, "\n")))
}

// sidebarFields performs the internal operation described by its signature.
func sidebarFields(slots SlotsData) map[string]interface{} {
	fields := make(map[string]interface{})
	if len(slots) == 0 {
		return fields
	}
	selected := slots[0]
	for _, slot := range slots {
		if processing, _ := slot["is_processing"].(bool); processing {
			selected = slot
			break
		}
	}
	for _, key := range []string{"n_ctx", "speculative", "is_processing"} {
		if value, ok := selected[key]; ok {
			fields[key] = value
		}
	}
	if params, ok := selected["params"].(map[string]interface{}); ok {
		for key, value := range params {
			fields[key] = value
		}
	}
	return fields
}

// formatSidebarValue performs the internal operation described by its signature.
func formatSidebarValue(value interface{}) string {
	switch value := value.(type) {
	case string:
		return value
	case bool:
		return strconv.FormatBool(value)
	case float64:
		if math.Trunc(value) == value {
			return strconv.FormatFloat(value, 'f', 0, 64)
		}
		return strconv.FormatFloat(math.Round(value*100)/100, 'f', 2, 64)
	case float32:
		converted := float64(value)
		if math.Trunc(converted) == converted {
			return strconv.FormatFloat(converted, 'f', 0, 32)
		}
		return strconv.FormatFloat(math.Round(converted*100)/100, 'f', 2, 32)
	case nil:
		return "n/a"
	default:
		encoded, err := json.Marshal(value)
		if err == nil {
			return string(encoded)
		}
		return fmt.Sprint(value)
	}
}

// View is an exported operation provided by this package.
// Additional context: Pipeline progress has its own sticky footer. Keeping it out of the live metrics line prevents the same status from being rendered twice.
func (m consoleModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.sessionView != sessionViewChat {
		return m.sessionModalView()
	}
	chatSessionTitle := "Chat Session"
	if m.session != nil && strings.TrimSpace(m.session.ID) != "" {
		chatSessionTitle = fmt.Sprintf("Chat Session (%s)", chatSessionPath(m.session.ID))
	}
	main := strings.Join([]string{
		renderFullWidthHeader(DefaultUnicode.Home, chatSessionTitle, defaultConsoleUITheme.mainHeader, m.transcript.Width),
		m.transcript.View(),
		defaultConsoleUITheme.mainHeader.Width(m.transcript.Width).Render("User Input"),
		m.input.View(),
		"",
	}, "\n")
	contentHeight := max(1, m.height-m.footerHeight())
	main = lipgloss.NewStyle().Width(m.transcript.Width).Height(contentHeight).Render(main)
	content := main
	if m.sidebarOpen && m.width >= consoleMinMainWidth+consoleMinSidebar {
		sideContent := lipgloss.JoinVertical(
			lipgloss.Left,
			renderFullWidthHeader(DefaultUnicode.Star, "Model Information", m.sidebarHeaderStyle, m.sidebar.Width),
			m.sidebar.View(),
			"",
		)
		side := m.sidebarBackgroundStyle.Width(m.sidebar.Width).Height(contentHeight).Render(sideContent)
		content = lipgloss.JoinHorizontal(lipgloss.Top, main, side)
	}
	footerText := singleConsoleLine(m.footer)

	if m.sessionStatus != "" && m.pipeline == nil {
		footerText += " | " + singleConsoleLine(m.sessionStatus)
	}
	footer := ansi.Truncate(footerText, max(0, m.width-1), "")
	footer = renderIconBar(DefaultUnicode.DoubleArrowRight, "#FFFF00", footer, m.footerBarLiveMetricsStyle.Width(m.width).MaxHeight(1))
	if m.mcpFooter != "" {
		mcpFooter := ansi.Truncate(singleConsoleLine(m.mcpFooter), max(0, m.width-1), "")
		mcpFooter = renderIconBar(DefaultUnicode.DoubleArrowRight, "#FFFF00", mcpFooter, m.footerBarMCPStyle.Width(m.width).MaxHeight(1))
		footer = lipgloss.JoinVertical(lipgloss.Left, mcpFooter, footer)
	}
	if m.pipelineRunning && m.pipeline != nil && m.pipelineStep >= 0 && m.pipelineStep < len(m.pipeline.Steps) {
		pipelineFile := filepath.Base(m.pipeline.FilePath)
		if pipelineFile == "." || pipelineFile == string(filepath.Separator) || pipelineFile == "" {
			pipelineFile = m.pipeline.Name
		}
		step := m.pipeline.Steps[m.pipelineStep]
		pipelineText := fmt.Sprintf("%s | Pipeline step %d/%d: %s", pipelineFile, m.pipelineStep+1, len(m.pipeline.Steps), step.Name)
		pipelineFooter := renderIconBar(DefaultUnicode.DoubleArrowRight, "#FFFF00", ansi.Truncate(singleConsoleLine(pipelineText), max(0, m.width-1), ""), m.footerBarPipelineStyle.Width(m.width).MaxHeight(1))
		footer = lipgloss.JoinVertical(lipgloss.Left, footer, pipelineFooter)
	}
	keyBindingText := consoleFooterKeyBindings
	if m.mcpFooter != "" {
		keyBindingText += " | Ctrl+T List Tools"
	}
	keyBindings := renderIconBar(DefaultUnicode.DoubleArrowRight, "#FFFF00", ansi.Truncate(keyBindingText, max(0, m.width-1), ""), m.footerBarKeyBindingsStyle.Width(m.width).MaxHeight(1))
	footer = lipgloss.JoinVertical(lipgloss.Left, footer, keyBindings)
	return lipgloss.JoinVertical(lipgloss.Left, content, footer)
}

// singleConsoleLine prevents dynamic status text from changing the number of
// rows owned by the footer. Newlines can otherwise make Bubble Tea's frame
// shorter or taller than the previous frame and expose stale terminal rows.
func singleConsoleLine(value string) string {
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(value)
}

// sessionModalView performs the internal operation described by its signature.
func (m consoleModel) sessionModalView() string {
	var body strings.Builder
	switch m.sessionView {
	case sessionViewSave:
		body.WriteString("Save Session\n\nSession title:\n")
		body.WriteString(m.sessionTitleInput.View())
		body.WriteString("\n\nEnter save   Esc cancel")
	case sessionViewConfirmLoad:
		body.WriteString("Load another session?\n\nUnsaved changes in the current chat will be lost.\n\nEnter: Continue   Esc: Cancel")
	case sessionViewLoad:
		body.WriteString("Load Session\n\n")
		if len(m.sessionList) == 0 {
			body.WriteString("No saved sessions found.\n\nEsc: Back")
		} else {
			for i, session := range m.sessionList {
				prefix := "  "
				if i == m.sessionCursor {
					prefix = "> "
				}
				body.WriteString(prefix + session.Title + "  (" + session.Model + ", " + session.UpdatedAt.Local().Format("Jan 2, 2006 3:04 PM") + ")\n")
			}
			body.WriteString("\n↑/↓ select   Enter load   Esc cancel")
		}
	case sessionViewModels:
		body.WriteString("Load Model\n\n")
		if m.modelLoading != "" {
			body.WriteString("Loading: " + m.modelLoading + "\n\nEsc: Back")
			break
		}
		if len(m.modelList) == 0 {
			body.WriteString("No models found.\n\nEsc: Back")
		} else {
			for i, model := range m.modelList {
				prefix := "  "
				if i == m.modelCursor {
					prefix = "> "
				}
				label := model.ID
				if model.State == ModelRuntimeLoaded {
					label += " (loaded)"
				}
				body.WriteString(prefix + label + "\n")
			}
			body.WriteString("\n↑/↓ select   Enter load   Esc cancel")
		}
	case sessionViewTools:
		body.WriteString("MCP Tools\n\n")
		if len(m.mcpTools) == 0 {
			body.WriteString("No MCP tools available.\n\nEsc: Back")
			break
		}
		groups := make(map[string][]boundMCPTool)
		groupOrder := make([]string, 0)
		for _, binding := range m.mcpTools {
			group := binding.server.Name
			if group == "" {
				group = "MCP"
			}
			if _, exists := groups[group]; !exists {
				groupOrder = append(groupOrder, group)
			}
			groups[group] = append(groups[group], binding)
		}
		for i, group := range groupOrder {
			if i > 0 {
				body.WriteString("\n")
			}
			body.WriteString(defaultConsoleUITheme.mcpToolGroup.Render(group) + "\n")
			for _, binding := range groups[group] {
				body.WriteString("  " + defaultConsoleUITheme.mcpToolName.Render(binding.tool.Name) + ": " + defaultConsoleUITheme.mcpToolDescription.Render(binding.tool.Description) + "\n")
			}
		}
		body.WriteString("\nEsc: Back")
	}
	content := lipgloss.NewStyle().Padding(2, 4).Width(max(1, m.width-8)).Height(max(1, m.height-2)).Render(body.String())
	return content
}
