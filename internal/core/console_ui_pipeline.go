// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// submitPipelineStep performs the internal operation described by its signature.
// Additional context: Output constraints belong to the current step. Clear them first so a structured-output step cannot accidentally constrain a later step.
func (m *consoleModel) submitPipelineStep(index int) tea.Cmd {
	if m.pipeline == nil || index < 0 || index >= len(m.pipeline.Steps) || m.waiting || m.sessionLoading {
		return nil
	}
	step := m.pipeline.Steps[index]
	var currentItem any
	if step.ForEach != "" && m.pipelineFanoutIndex < len(m.pipelineFanoutItems) {
		currentItem = m.pipelineFanoutItems[m.pipelineFanoutIndex]
	}
	renderedPrompt, err := renderPipelineReferencesFor(step.UserPrompt, m.pipelineOutputs, m.pipelineInputs, currentItem, step.As)
	if err != nil {
		return m.failPipeline(fmt.Errorf("pipeline step %q: %w", step.Name, err))
	}
	renderedSystem, err := renderPipelineReferencesFor(step.SystemPrompt, m.pipelineOutputs, m.pipelineInputs, currentItem, step.As)
	if err != nil {
		return m.failPipeline(fmt.Errorf("pipeline step %q: %w", step.Name, err))
	}
	step.UserPrompt = renderedPrompt
	step.SystemPrompt = renderedSystem
	renderedModel, err := renderPipelineReferencesFor(step.Model, m.pipelineOutputs, m.pipelineInputs, currentItem, step.As)
	if err != nil {
		return m.failPipeline(fmt.Errorf("pipeline step %q model: %w", step.Name, err))
	}
	step.Model = renderedModel
	if step.Decision != nil {
		decision := *step.Decision
		if strings.TrimSpace(decision.CandidatesFrom) != "" {
			value, resolveErr := resolvePipelineReferenceFor(strings.TrimSpace(decision.CandidatesFrom), m.pipelineOutputs, m.pipelineInputs, currentItem, step.As)
			if resolveErr != nil {
				return m.failPipeline(fmt.Errorf("pipeline step %q decision candidates: %w", step.Name, resolveErr))
			}
			candidates, convertErr := pipelineDecisionCandidates(value)
			if convertErr != nil {
				return m.failPipeline(fmt.Errorf("pipeline step %q decision candidates: %w", step.Name, convertErr))
			}
			decision.Candidates = candidates
		}
		if state, ok := decision.State.(string); ok {
			renderedState, renderErr := renderPipelineReferencesFor(state, m.pipelineOutputs, m.pipelineInputs, currentItem, step.As)
			if renderErr != nil {
				return m.failPipeline(fmt.Errorf("pipeline step %q: decision state: %w", step.Name, renderErr))
			}
			decision.State = renderedState
		}
		if decision.Instructions != "" {
			renderedInstructions, renderErr := renderPipelineReferencesFor(decision.Instructions, m.pipelineOutputs, m.pipelineInputs, currentItem, step.As)
			if renderErr != nil {
				return m.failPipeline(fmt.Errorf("pipeline step %q: decision instructions: %w", step.Name, renderErr))
			}
			decision.Instructions = renderedInstructions
		}
		step.Decision = &decision
	}
	m.client.logf("pipeline: submitting step %d/%d name=%q model=%q system_prompt=%q user_prompt=%q image=%q document=%q response_format=%#v json_schema=%#v", index+1, len(m.pipeline.Steps), step.Name, step.Model, step.SystemPrompt, step.UserPrompt, step.Image, step.Document, step.ResponseFormat, step.JSONSchema)
	m.err = nil
	m.request.Model = step.Model

	m.request.ResponseFormat = nil
	m.request.JSONSchema = nil
	m.request.Grammar = ""
	m.request.Temperature = nil
	m.request.TopP = nil
	m.request.TopK = nil
	m.request.MaxTokens = nil
	m.request.Classification = nil
	m.request.Decision = nil
	m.request.ReasoningControl = nil
	m.request.RepeatPenalty = nil
	m.request.Seed = nil
	m.request.MinP = nil
	m.request.XTCProbability = nil
	m.request.XTCThreshold = nil
	m.request.TypicalP = nil
	m.request.PresencePenalty = nil
	m.request.FrequencyPenalty = nil
	m.request.RepeatLastN = nil
	if step.ResponseFormat != nil {
		m.request.ResponseFormat = step.ResponseFormat
	}
	if step.Output != nil {
		if strings.EqualFold(step.Output.Type, "json") {
			m.request.ResponseFormat = &ResponseFormat{Type: "json_object"}
			disableReasoning := false
			m.request.ReasoningControl = &disableReasoning
			if step.Output.JSONSchema != nil {
				m.request.JSONSchema = step.Output.JSONSchema
			}
			m.request.Grammar = step.Output.Grammar
		}
	}
	if step.JSONSchema != nil {
		m.request.JSONSchema = step.JSONSchema
	}
	if step.Parameters != nil {
		m.request.Temperature = step.Parameters.Temperature
		m.request.TopP = step.Parameters.TopP
		m.request.TopK = step.Parameters.TopK
		m.request.MaxTokens = step.Parameters.MaxTokens
		m.request.RepeatPenalty = step.Parameters.RepeatPenalty
		m.request.Seed = step.Parameters.Seed
		m.request.MinP = step.Parameters.MinP
		m.request.XTCProbability = step.Parameters.XTCProbability
		m.request.XTCThreshold = step.Parameters.XTCThreshold
		m.request.TypicalP = step.Parameters.TypicalP
		m.request.PresencePenalty = step.Parameters.PresencePenalty
		m.request.FrequencyPenalty = step.Parameters.FrequencyPenalty
		m.request.RepeatLastN = step.Parameters.RepeatLastN
	}
	if step.Temperature != nil {
		m.request.Temperature = step.Temperature
	}
	m.refreshSidebar()
	if decision := effectiveDecision(step); decision != nil {
		m.request.Decision = decision
		if step.Parameters == nil || step.Parameters.MaxTokens == nil {
			one := 1
			m.request.MaxTokens = &one
		}
	}
	m.loading = false
	m.modelLoading = ""
	m.input.SetValue("")
	m.input.Prompt = " " + renderWhiteIcon(DefaultUnicode.Star) + " You: "
	if strings.TrimSpace(step.SystemPrompt) != "" {
		m.request.Messages = addPipelineSystemPrompt(m.request.Messages, step.SystemPrompt)
	}
	content := any(step.UserPrompt)
	if index == 0 && m.pipeline.Inputs != nil {
		inputContent, imageName, documentName, err := inputContent(*m.pipeline.Inputs, step)
		if err != nil {
			m.client.logf("pipeline: step %q prepare inputs failed: %v", step.Name, err)
			return m.failPipeline(fmt.Errorf("pipeline step %q: prepare inputs: %w", step.Name, err))
		}
		m.request.ImageFilename = imageName
		m.request.DocumentFilename = documentName
		content = inputContent
	} else if step.Image != "" {
		m.request.ImageFilename = filepath.Base(step.Image)
		dataURL, err := ImageDataURL(step.Image, DefaultAttachmentMaxBytes)
		if err != nil {
			m.client.logf("pipeline: step %q prepare image %q failed: %v", step.Name, step.Image, err)
			return m.failPipeline(fmt.Errorf("pipeline step %q: prepare image: %w", step.Name, err))
		}
		content = []ContentPart{{Type: "text", Text: step.UserPrompt}, {Type: "image_url", ImageURL: &ImageURLPart{URL: dataURL, Detail: "low"}}}
	} else if step.Document != "" {
		m.request.DocumentFilename = filepath.Base(step.Document)
		documentText, err := ExtractPDFText(step.Document, DefaultAttachmentMaxBytes)
		if err != nil {
			m.client.logf("pipeline: step %q extract document %q failed: %v", step.Name, step.Document, err)
			return m.failPipeline(fmt.Errorf("pipeline step %q: extract document: %w", step.Name, err))
		}
		_, filename, err := FileDataURL(step.Document, DefaultAttachmentMaxBytes)
		if err != nil {
			m.client.logf("pipeline: step %q prepare document %q failed: %v", step.Name, step.Document, err)
			return m.failPipeline(fmt.Errorf("pipeline step %q: prepare document: %w", step.Name, err))
		}
		content = []ContentPart{{Type: "text", Text: step.UserPrompt}, {Type: "text", Text: "Document filename: " + filename + "\n\nExtracted document text:\n" + documentText}}
	}
	m.request.Messages = append(m.request.Messages, Message{Role: "user", Content: content})
	m.messages = append(m.messages, consoleMessage{role: "user", content: step.UserPrompt})
	if m.pipelineQualityFeedback != "" {
		m.request.Messages = append(m.request.Messages, Message{Role: "user", Content: m.pipelineQualityFeedback})
		m.messages = append(m.messages, consoleMessage{role: "user", content: m.pipelineQualityFeedback})
		m.pipelineQualityFeedback = ""
	}
	m.waiting = true
	if m.modelOverlay != nil {
		m.modelOverlay.StartInference(step.Model)
	}
	m.sessionDirty = true
	m.input.Blur()
	m.sessionStatus = fmt.Sprintf("Pipeline step %d/%d: %s", index+1, len(m.pipeline.Steps), step.Name)
	m.refreshTranscript()
	return m.runTurn(m.request)
}

// failPipeline records a terminal pipeline error in the session before ending
// the console run. This keeps batch wrappers from mistaking a halted pipeline
// for an interactive session that is still waiting for input.
func (m *consoleModel) failPipeline(err error) tea.Cmd {
	if err == nil {
		err = fmt.Errorf("pipeline failed")
	}
	m.pipelineRunning = false
	m.waiting = false
	m.err = err
	m.sessionStatus = "Pipeline failed: " + err.Error()
	if m.session != nil {
		m.syncSessionFromRequest()
		m.session.Error = err.Error()
		m.sessionDirty = true
		m.sessionLoading = true
		m.input.Blur()
		m.refreshTranscript()
		return saveSessionCmd(m.client, m.session, true)
	}
	m.refreshTranscript()
	return tea.Quit
}

// addPipelineSystemPrompt keeps pipeline instructions compatible with chat
// templates that allow exactly one system message at the beginning.
func addPipelineSystemPrompt(messages []Message, prompt string) []Message {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return messages
	}
	leading := 0
	var instructions []string
	for leading < len(messages) && messages[leading].Role == "system" {
		if existing, ok := messages[leading].Content.(string); ok && strings.TrimSpace(existing) != "" {
			instructions = append(instructions, strings.TrimSpace(existing))
		}
		leading++
	}
	instructions = append(instructions, "Pipeline step instructions:\n"+prompt)
	result := make([]Message, 0, len(messages)-leading+1)
	result = append(result, Message{Role: "system", Content: strings.Join(instructions, "\n\n")})
	result = append(result, messages[leading:]...)
	return result
}

// submitInput performs the internal operation described by its signature.
func (m *consoleModel) submitInput() tea.Cmd {
	value := m.input.Value()
	if !m.submittingInitialPrompt {
		value = strings.TrimSpace(value)
	}
	if value == "" || !m.inputReady() {
		return nil
	}
	m.input.SetValue("")
	m.input.Blur()
	m.err = nil
	m.client.logf("ui: submitting user turn model=%q prompt=%q", m.request.Model, value)
	m.waiting = true
	if m.modelOverlay != nil {
		m.modelOverlay.StartInference(m.request.Model)
	}
	m.messages = append(m.messages, consoleMessage{role: "user", content: value})
	if !replaceLastMultimodalUserPrompt(&m.request, value) {
		m.request.Messages = append(m.request.Messages, Message{Role: "user", Content: value})
	}
	m.sessionDirty = true
	m.refreshTranscript()
	return m.runTurn(m.request)
}

// inputReady is true only while the loaded model is idle and the chat view is
// waiting for a new user turn. Inference stages such as encoding and decoding
// are represented by waiting, so the input remains unavailable throughout the
// complete request lifecycle.
func (m consoleModel) inputReady() bool {
	return m.sessionView == sessionViewChat && !m.loading && !m.waiting && !m.sessionLoading && m.modelLoading == ""
}

// replaceLastMultimodalUserPrompt keeps an automated prompt attached to an
// image or document instead of creating a separate text-only turn.
func replaceLastMultimodalUserPrompt(req *ChatRequest, prompt string) bool {
	if req == nil || len(req.Messages) == 0 {
		return false
	}
	message := &req.Messages[len(req.Messages)-1]
	if message.Role != "user" {
		return false
	}
	parts, ok := message.Content.([]ContentPart)
	if !ok {
		return false
	}
	for i := range parts {
		if parts[i].Type == "text" {
			parts[i].Text = prompt
			message.Content = parts
			return true
		}
	}
	message.Content = append(parts, ContentPart{Type: "text", Text: prompt})
	return true
}

// hasUnsavedSessionChanges performs the internal operation described by its signature.
func (m consoleModel) hasUnsavedSessionChanges() bool {
	return (m.session == nil && len(m.request.Messages) > 0) || (m.session != nil && m.sessionDirty)
}

// syncSessionFromRequest performs the internal operation described by its signature.
func (m *consoleModel) syncSessionFromRequest() {
	if m.session != nil {
		m.session.Messages = cloneMessages(m.request.Messages)
		m.session.imageFilename = m.request.ImageFilename
		m.session.documentFilename = m.request.DocumentFilename
	}
}

// consoleMessagesFromMessages performs the internal operation described by its signature.
func consoleMessagesFromMessages(messages []Message) []consoleMessage {
	result := make([]consoleMessage, 0, len(messages))
	for _, message := range messages {
		content := ""
		if value, ok := message.Content.(string); ok {
			content = value
		} else if message.Content != nil {
			encoded, _ := json.Marshal(message.Content)
			content = string(encoded)
		}
		result = append(result, consoleMessage{role: message.Role, content: content})
	}
	return result
}

// saveSessionCmd performs the internal operation described by its signature.
func saveSessionCmd(client *Client, session *ChatSession, exit bool) tea.Cmd {
	clonedSession := *session
	clonedSession.Messages = cloneMessages(session.Messages)
	return func() tea.Msg {
		if client != nil {
			client.logf("session: persisting id=%q path=%q saved=%t model=%q messages=%d snapshots=%d", clonedSession.ID, sessionPathForLog(clonedSession.ID), clonedSession.Saved, clonedSession.Model, len(clonedSession.Messages), len(clonedSession.Snapshots))
		}
		err := saveChatSession(&clonedSession)
		if client != nil {
			if err != nil {
				client.logf("session: persistence failed id=%q path=%q error=%v", clonedSession.ID, sessionPathForLog(clonedSession.ID), err)
			} else {
				client.logf("session: persistence complete id=%q path=%q", clonedSession.ID, sessionPathForLog(clonedSession.ID))
			}
		}
		if err == nil && client != nil && client.opts.sessionSaved != nil {
			client.opts.sessionSaved(chatSessionPath(clonedSession.ID))
		}
		return sessionSavedMsg{session: &clonedSession, err: err, exit: exit}
	}
}

// sessionPathForLog performs the internal operation described by its signature.
func sessionPathForLog(id string) string {
	path := chatSessionPath(id)
	if absolute, err := filepath.Abs(path); err == nil {
		return absolute
	}
	return path
}

// sessionID performs the internal operation described by its signature.
func sessionID(session *ChatSession) string {
	if session == nil {
		return ""
	}
	return session.ID
}

// listSessionsCmd performs the internal operation described by its signature.
func listSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		sessions, err := listChatSessions()
		return sessionListLoadedMsg{sessions: sessions, err: err}
	}
}

// listModelsCmd performs the internal operation described by its signature.
func listModelsCmd(ctx context.Context, client *Client, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return modelListLoadedMsg{err: fmt.Errorf("model client is unavailable")}
		}
		listCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		status, err := client.GetRuntimeStatus(listCtx)
		if err != nil {
			return modelListLoadedMsg{err: err}
		}
		return modelListLoadedMsg{models: status.Models}
	}
}

// loadModelCmd performs the internal operation described by its signature.
func loadModelCmd(ctx context.Context, client *Client, timeout time.Duration, model, previous string, overlay *liveMetricsOverlay) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return modelLoadedMsg{model: model, previous: previous, err: fmt.Errorf("model client is unavailable")}
		}
		var monitor *inferenceMonitor
		if overlay != nil {
			monitor = client.startInferenceMonitorWithOverlay(ctx, model, false, overlay)
		}
		loadCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		client.logf("ui: loading model model=%q previous=%q", model, previous)
		var err error
		if strings.TrimSpace(previous) != "" && previous != model {
			client.logf("runtime: switching model previous=%q target=%q", previous, model)
			var result *SwitchResult
			result, err = client.SwitchModel(loadCtx, model)
			if err == nil && result.Load != nil && result.Load.Changed {
				client.recordModelLoadDuration(model, result.Load.Duration)
			}
		} else {
			err = client.loadModel(loadCtx, model)
		}
		if err != nil && monitor != nil {
			monitor.Stop()
			monitor = nil
		}
		if err == nil && overlay != nil {
			overlay.modelLoadedForModel(model)
		}
		var props *PropsData
		var slots SlotsData
		if err == nil {
			props, _ = client.fetchPropsReady(loadCtx, model)
			slots, _ = client.fetchSlotsReady(loadCtx, model)
			if monitor != nil {
				monitor.markModelReady()
			}
		}
		return modelLoadedMsg{model: model, previous: previous, monitor: monitor, props: props, slots: slots, err: err}
	}
}

// loadSessionCmd performs the internal operation described by its signature.
func loadSessionCmd(ctx context.Context, summary ChatSessionSummary, client *Client, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		session, err := loadChatSessionFromPath(summary.Path)
		if err != nil {
			return sessionLoadedMsg{err: err}
		}
		model := sessionMostRecentModel(session)
		if client != nil {
			loadCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			if err := client.loadModel(loadCtx, model); err != nil {
				return sessionLoadedMsg{err: err}
			}
			props, _ := client.fetchPropsReady(loadCtx, model)
			slots, _ := client.fetchSlotsReady(loadCtx, model)
			return sessionLoadedMsg{session: session, props: props, slots: slots}
		}
		return sessionLoadedMsg{session: session}
	}
}

// sendConsoleInferenceChunk renders both separately returned reasoning and
// assistant content as one transcript stream. The same helper is used by
// ordinary, application-tool, and MCP-backed UI inferences.
func (m consoleModel) sendConsoleInferenceChunk(chunk InferenceStreamChunk, reasoningOpen *bool) {
	for _, choice := range chunk.Choices {
		for _, toolCall := range choice.Delta.ToolCalls {
			name := toolCall.Function.Name
			if name == "" || m.streamedToolCallNames == nil || m.streamedToolCallNames[toolCall.Index] == name {
				continue
			}
			m.streamedToolCallNames[toolCall.Index] = name
			if m.send != nil {
				arguments := toolCall.Function.Arguments
				if arguments == "" {
					arguments = "…"
				}
				m.send(consoleToolMsg(fmt.Sprintf("[Tool Call: Streaming] %s(%s)", name, arguments)))
			}
		}
		if reasoning := choice.Delta.ReasoningContent; reasoning != "" {
			if !*reasoningOpen {
				if m.send != nil {
					m.send(consoleChunkMsg("<think>\n"))
				}
				*reasoningOpen = true
			}
			if m.send != nil {
				m.send(consoleChunkMsg(reasoning))
			}
		}
		content := choice.Delta.Content
		if content == "" {
			content = choice.Text
		}
		if content != "" {
			if *reasoningOpen {
				if m.send != nil {
					m.send(consoleChunkMsg("\n</think>\n\n"))
				}
				*reasoningOpen = false
			}
			if m.send != nil {
				m.send(consoleChunkMsg(content))
			}
		}
	}
}

// closeConsoleInferenceReasoning performs the internal operation described by its signature.
func (m consoleModel) closeConsoleInferenceReasoning(reasoningOpen *bool) {
	if *reasoningOpen && m.send != nil {
		m.send(consoleChunkMsg("\n</think>"))
		*reasoningOpen = false
	}
}
