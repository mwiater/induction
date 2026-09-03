package induction

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

const (
	consoleFooterHeight = 1
	consoleMinMainWidth = 40
	consoleMinSidebar   = 16
)

type consoleMode int

const (
	consoleNonStreaming consoleMode = iota
	consoleStreaming
	consoleSnapshot
)

type consoleMessage struct {
	role    string
	content string
}

type consoleTurnResult struct {
	content  string
	snapshot *ModelSnapshot
	model    string
	err      error
}

type consoleChunkMsg string
type consoleOverlayMsg overlayUpdate
type consoleMCPMsg string
type consoleMCPActivityMsg string
type consoleModelLoadedMsg struct{ err error }
type consoleMonitorStartedMsg struct{ monitor *inferenceMonitor }

type sessionViewMode int

const (
	sessionViewChat sessionViewMode = iota
	sessionViewSave
	sessionViewLoad
	sessionViewConfirmLoad
	sessionViewModels
	sessionViewTools
)

type sessionSavedMsg struct {
	session *ChatSession
	err     error
	exit    bool
}
type sessionListLoadedMsg struct {
	sessions []ChatSessionSummary
	err      error
}
type sessionLoadedMsg struct {
	session *ChatSession
	err     error
}
type modelListLoadedMsg struct {
	models []RuntimeModel
	err    error
}
type modelLoadedMsg struct {
	model    string
	previous string
	monitor  *inferenceMonitor
	err      error
}

type consoleModel struct {
	ctx               context.Context
	client            *Client
	request           ChatRequest
	timeout           time.Duration
	mode              consoleMode
	persistence       *snapshotSession
	snapshots         []*ModelSnapshot
	transcript        viewport.Model
	sidebar           viewport.Model
	input             textinput.Model
	messages          []consoleMessage
	slots             SlotsData
	footer            string
	mcpFooter         string
	width             int
	height            int
	sidebarWidth      int
	sidebarOpen       bool
	waiting           bool
	loading           bool
	err               error
	session           *ChatSession
	sessionDirty      bool
	sessionView       sessionViewMode
	sessionTitleInput textinput.Model
	sessionList       []ChatSessionSummary
	sessionCursor     int
	modelList         []RuntimeModel
	modelCursor       int
	modelLoading      string
	mcpTools          []boundMCPTool
	modelOverlay      *liveMetricsOverlay
	modelMonitor      *inferenceMonitor
	sessionStatus     string
	sessionLoading    bool
	exitSaving        bool
	send              func(tea.Msg)
	complete          func()
	inferTurn         func(context.Context, *ChatRequest) (*InferenceResponse, error)
	startMonitor      func() tea.Cmd
	cancel            context.CancelFunc

	mainUserPromptStyle         lipgloss.Style
	mainAssistantReasoningStyle lipgloss.Style
	mainAssistantContentStyle   lipgloss.Style
	sidebarHeaderStyle          lipgloss.Style
	sidebarBackgroundStyle      lipgloss.Style
	sidebarTextStyle            lipgloss.Style
	footerBarMCPStyle           lipgloss.Style
	footerBarLiveMetricsStyle   lipgloss.Style
	footerBarKeyBindingsStyle   lipgloss.Style
	mainErrorStyle              lipgloss.Style
	mcpActivityStyle            lipgloss.Style
}

func canRunConsole(in io.Reader, out io.Writer) bool {
	inFile, inOK := in.(*os.File)
	outFile, outOK := out.(*os.File)
	return inOK && outOK && term.IsTerminal(int(inFile.Fd())) && term.IsTerminal(int(outFile.Fd()))
}

func newConsoleModel(ctx context.Context, client *Client, request ChatRequest, timeout time.Duration, sidebarWidth int, mode consoleMode) consoleModel {
	input := textinput.New()
	input.Prompt = " " + renderWhiteIcon(DefaultUnicode.Star) + " You: "
	input.Blur()
	input.Prompt = fmt.Sprintf(" "+renderWhiteIcon(DefaultUnicode.Star)+" You: (model %s is loading, please wait...)", request.Model)
	input.Cursor.Style = defaultConsoleUITheme.inputCursor
	input.CharLimit = 0
	titleInput := textinput.New()
	titleInput.Prompt = "> "
	titleInput.CharLimit = maxSessionTitleRunes
	titleInput.Blur()

	m := consoleModel{
		ctx:                         ctx,
		client:                      client,
		request:                     request,
		timeout:                     timeout,
		mode:                        mode,
		transcript:                  viewport.New(0, 0),
		sidebar:                     viewport.New(0, 0),
		input:                       input,
		sessionTitleInput:           titleInput,
		sidebarWidth:                sidebarWidth,
		sidebarOpen:                 true,
		loading:                     true,
		footer:                      formatModelLoadingPlain(request.Model, modelLoadProgress{}),
		mainUserPromptStyle:         defaultConsoleUITheme.mainUserPrompt,
		mainAssistantContentStyle:   defaultConsoleUITheme.mainAssistantContent,
		mainAssistantReasoningStyle: defaultConsoleUITheme.mainAssistantReasoning,
		sidebarHeaderStyle:          defaultConsoleUITheme.sidebarHeader,
		sidebarBackgroundStyle:      defaultConsoleUITheme.sidebarBackground,
		sidebarTextStyle:            defaultConsoleUITheme.sidebarText,
		footerBarMCPStyle:           defaultConsoleUITheme.footerBarMCP,
		footerBarLiveMetricsStyle:   defaultConsoleUITheme.footerBarLiveMetrics,
		footerBarKeyBindingsStyle:   defaultConsoleUITheme.footerBarKeyBindings,
		mainErrorStyle:              defaultConsoleUITheme.mainError,
		mcpActivityStyle:            defaultConsoleUITheme.mcpActivity,
	}
	return m
}

func (m consoleModel) Init() tea.Cmd {
	loadModel := func() tea.Msg {
		if m.client == nil {
			return consoleModelLoadedMsg{}
		}
		loadCtx, cancel := context.WithTimeout(m.ctx, m.timeout)
		defer cancel()
		return consoleModelLoadedMsg{err: m.client.loadModel(loadCtx, m.request.Model)}
	}
	if m.startMonitor != nil {
		// Subscribe to lifecycle events before asking the server to load the
		// model so the initial progress event cannot be missed.
		return tea.Sequence(m.startMonitor(), loadModel)
	}
	return loadModel
}

func (m consoleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case consoleModelLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			// A failed or timed-out load must never leave the chat input
			// permanently disabled. The user can retry through Ctrl-M or exit
			// immediately with Ctrl-C.
			m.loading = false
			m.input.Prompt = " " + renderWhiteIcon(DefaultUnicode.Star) + " You: "
			m.input.Focus()
			m.refreshTranscript()
			return m, textinput.Blink
		}
		m.loading = false
		if m.modelOverlay != nil {
			m.modelOverlay.ModelLoaded()
		}
		m.footer = formatModelReadyPlain(m.request.Model)
		m.input.SetValue("")
		m.input.Prompt = " " + renderWhiteIcon(DefaultUnicode.Star) + " You: "
		m.input.Focus()
		return m, textinput.Blink
	case consoleMonitorStartedMsg:
		m.modelMonitor = msg.monitor
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil
	case consoleOverlayMsg:
		update := overlayUpdate(msg)
		if update.footer != "" {
			m.footer = update.footer
		}
		if update.modelReady && m.modelLoading == "" {
			m.loading = false
			m.input.SetValue("")
			m.input.Prompt = " " + renderWhiteIcon(DefaultUnicode.Star) + " You: "
			m.input.Focus()
		}
		if len(update.slots) > 0 {
			m.slots = update.slots
			m.refreshSidebar()
		}
		return m, nil
	case consoleMCPMsg:
		m.mcpFooter = string(msg)
		m.resize()
		return m, nil
	case consoleMCPActivityMsg:
		m.messages = append(m.messages, consoleMessage{role: "mcp_activity", content: string(msg)})
		m.refreshTranscript()
		return m, nil
	case consoleChunkMsg:
		if len(m.messages) == 0 || m.messages[len(m.messages)-1].role != "assistant" {
			m.messages = append(m.messages, consoleMessage{role: "assistant"})
		}
		m.messages[len(m.messages)-1].content += string(msg)
		m.refreshTranscript()
		return m, nil
	case consoleTurnResult:
		m.waiting = false
		if msg.err != nil {
			m.err = msg.err
			m.refreshTranscript()
			return m, nil
		}
		if m.mode != consoleStreaming {
			m.messages = append(m.messages, consoleMessage{role: "assistant", content: msg.content})
		}
		m.request.Messages = append(m.request.Messages, Message{Role: "assistant", Content: msg.content})
		m.sessionDirty = true
		if msg.snapshot != nil && msg.model != "" {
			// The model is a property of this turn, not merely of the session's
			// currently selected model. Keep the stored snapshot authoritative.
			msg.snapshot.ModelID = msg.model
		}
		if m.session != nil && msg.snapshot != nil {
			m.session.Snapshots = append(m.session.Snapshots, msg.snapshot)
			m.session.Model = msg.snapshot.ModelID
		}
		if msg.snapshot != nil {
			m.snapshots = append(m.snapshots, msg.snapshot)
		}
		m.refreshTranscript()
		m.input.Focus()
		cmds := []tea.Cmd{textinput.Blink}
		if m.session != nil {
			m.syncSessionFromRequest()
			cmds = append(cmds, saveSessionCmd(m.session, false))
		}
		return m, tea.Batch(cmds...)
	case sessionSavedMsg:
		m.sessionLoading = false
		if msg.err != nil {
			m.sessionStatus = "Save failed: " + msg.err.Error()
			if msg.exit {
				m.exitSaving = false
			}
			return m, nil
		}
		m.session = msg.session
		m.sessionDirty = false
		if msg.session.Type == sessionTypeUnsaved {
			m.sessionStatus = "Session updated"
		} else {
			m.sessionStatus = "Saved: " + msg.session.Title
		}
		if msg.exit || m.exitSaving {
			m.exitSaving = false
			return m, tea.Quit
		}
		m.sessionView = sessionViewChat
		m.input.Focus()
		return m, textinput.Blink
	case sessionListLoadedMsg:
		m.sessionLoading = false
		if msg.err != nil {
			m.sessionStatus = "Load failed: " + msg.err.Error()
			m.sessionView = sessionViewChat
			m.input.Focus()
			return m, nil
		}
		m.sessionList = msg.sessions
		m.sessionCursor = 0
		m.sessionView = sessionViewLoad
		return m, nil
	case sessionLoadedMsg:
		m.sessionLoading = false
		if msg.err != nil {
			m.sessionStatus = "Load failed: " + msg.err.Error()
			m.sessionView = sessionViewChat
			m.input.Focus()
			return m, nil
		}
		m.request.Model = sessionMostRecentModel(msg.session)
		m.request.Messages = cloneMessages(msg.session.Messages)
		m.messages = consoleMessagesFromMessages(msg.session.Messages)
		m.session = msg.session
		m.sessionDirty = false
		m.sessionView = sessionViewChat
		m.sessionStatus = "Loaded: " + msg.session.Title
		m.footer = formatModelReadyPlain(m.request.Model)
		if m.modelOverlay != nil {
			m.modelOverlay.SetModel(m.request.Model)
			m.modelOverlay.ModelLoaded()
		}
		m.refreshSidebar()
		m.refreshTranscript()
		m.input.Focus()
		return m, textinput.Blink
	case modelListLoadedMsg:
		m.sessionLoading = false
		if msg.err != nil {
			m.sessionStatus = "Model list failed: " + msg.err.Error()
			m.input.Focus()
			return m, nil
		}
		m.modelList = msg.models
		m.modelCursor = 0
		m.sessionView = sessionViewModels
		return m, nil
	case modelLoadedMsg:
		m.sessionLoading = false
		m.modelLoading = ""
		if msg.err != nil {
			m.sessionStatus = "Model load failed: " + msg.err.Error()
			m.sessionView = sessionViewModels
			if m.modelOverlay != nil {
				m.modelOverlay.SetModel(msg.previous)
				m.modelOverlay.ModelLoaded()
			}
			return m, nil
		}
		m.request.Model = msg.model
		m.loading = false
		m.input.Prompt = " " + renderWhiteIcon(DefaultUnicode.Star) + " You: "
		m.footer = formatModelReadyPlain(msg.model)
		m.modelMonitor = msg.monitor
		m.sessionStatus = "Loaded model: " + msg.model
		m.sessionView = sessionViewChat
		m.input.Focus()
		m.refreshSidebar()
		m.refreshSidebar()
		return m, textinput.Blink
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			// Ctrl-C is an unconditional escape hatch. In particular, do not
			// wait for a model load or session save here: Run returns promptly,
			// allowing the caller's cleanup hook to run.
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		if key == "ctrl+1" && m.sessionView == sessionViewChat {
			m.sidebarOpen = !m.sidebarOpen
			m.resize()
			return m, nil
		}
		if m.sessionView == sessionViewChat {
			if key == "ctrl+t" && m.mcpFooter != "" {
				m.sessionView = sessionViewTools
				m.input.Blur()
				return m, nil
			}
			if key == "ctrl+s" {
				if m.waiting || m.loading {
					m.sessionStatus = "Wait for the current response to finish before saving."
					return m, nil
				}
				m.sessionTitleInput.SetValue("")
				if m.session != nil {
					m.sessionTitleInput.SetValue(m.session.Title)
				}
				m.sessionTitleInput.Focus()
				m.input.Blur()
				m.sessionView = sessionViewSave
				return m, textinput.Blink
			}
			if key == "ctrl+l" {
				if m.waiting || m.loading {
					m.sessionStatus = "Wait for the current response to finish before loading."
					return m, nil
				}
				if m.hasUnsavedSessionChanges() {
					m.sessionView = sessionViewConfirmLoad
					m.input.Blur()
					return m, nil
				}
				m.sessionLoading = true
				return m, listSessionsCmd()
			}
			// Terminals encode Ctrl-M and Enter identically as carriage return.
			// In the empty chat input, treat that event as the model-picker
			// shortcut; Enter with text continues to submit the chat message.
			if key == "ctrl+m" || (key == "enter" && strings.TrimSpace(m.input.Value()) == "") {
				if m.waiting || m.loading {
					m.sessionStatus = "Wait for the current response to finish before loading a model."
					return m, nil
				}
				m.input.Blur()
				m.sessionLoading = true
				return m, listModelsCmd(m.client, m.ctx, m.timeout)
			}
		}
		if m.sessionView == sessionViewSave {
			switch key {
			case "esc":
				m.sessionView = sessionViewChat
				m.sessionTitleInput.Blur()
				m.input.Focus()
				return m, textinput.Blink
			case "enter":
				title, err := normalizeSessionTitle(m.sessionTitleInput.Value())
				if err != nil {
					m.sessionStatus = "Save failed: " + err.Error()
					return m, nil
				}
				var session *ChatSession
				if m.session != nil {
					updated := *m.session
					session = &updated
					session.Type = sessionTypeSaved
					session.Title = title
					session.Model = m.request.Model
					session.Messages = cloneMessages(m.request.Messages)
				} else {
					session, err = newChatSession(title, m.request)
				}
				if err != nil {
					m.sessionStatus = "Save failed: " + err.Error()
					return m, nil
				}
				m.sessionLoading = true
				return m, saveSessionCmd(session, false)
			}
			m.sessionTitleInput, _ = m.sessionTitleInput.Update(msg)
			return m, nil
		}
		if m.sessionView == sessionViewConfirmLoad {
			switch key {
			case "esc":
				m.sessionView = sessionViewChat
				m.input.Focus()
				return m, textinput.Blink
			case "enter":
				m.sessionView = sessionViewChat
				m.sessionLoading = true
				return m, listSessionsCmd()
			}
			return m, nil
		}
		if m.sessionView == sessionViewTools {
			if key == "esc" {
				m.sessionView = sessionViewChat
				m.input.Focus()
				return m, textinput.Blink
			}
			return m, nil
		}
		if m.sessionView == sessionViewLoad {
			switch key {
			case "esc":
				m.sessionView = sessionViewChat
				m.input.Focus()
				return m, textinput.Blink
			case "up", "k":
				if m.sessionCursor > 0 {
					m.sessionCursor--
				}
				return m, nil
			case "down", "j":
				if m.sessionCursor+1 < len(m.sessionList) {
					m.sessionCursor++
				}
				return m, nil
			case "enter":
				if len(m.sessionList) == 0 {
					return m, nil
				}
				m.sessionLoading = true
				return m, loadSessionCmd(m.sessionList[m.sessionCursor], m.client, m.ctx, m.timeout)
			}
			return m, nil
		}
		if m.sessionView == sessionViewModels {
			switch key {
			case "esc":
				m.sessionView = sessionViewChat
				m.input.Focus()
				return m, textinput.Blink
			case "up", "k":
				if m.modelCursor > 0 {
					m.modelCursor--
				}
				return m, nil
			case "down", "j":
				if m.modelCursor+1 < len(m.modelList) {
					m.modelCursor++
				}
				return m, nil
			case "enter":
				if len(m.modelList) == 0 {
					return m, nil
				}
				m.modelLoading = m.modelList[m.modelCursor].ID
				m.sessionLoading = true
				m.loading = true
				m.input.Prompt = fmt.Sprintf(" "+renderWhiteIcon(DefaultUnicode.Star)+" You: (model %s is loading, please wait...)", m.modelLoading)
				m.input.Blur()
				m.sessionView = sessionViewChat
				if m.modelMonitor != nil {
					m.modelMonitor.Stop()
					m.modelMonitor = nil
				}
				if m.modelOverlay != nil {
					m.modelOverlay.StartModelLoad(m.modelList[m.modelCursor].ID)
				}
				return m, loadModelCmd(m.client, m.ctx, m.timeout, m.modelList[m.modelCursor].ID, m.request.Model, m.modelOverlay)
			}
			return m, nil
		}
		if m.input.Focused() && !m.loading {
			switch msg.String() {
			case "enter":
				value := strings.TrimSpace(m.input.Value())
				if value == "" || m.waiting {
					return m, nil
				}
				m.input.SetValue("")
				m.input.Blur()
				m.err = nil
				m.waiting = true
				m.messages = append(m.messages, consoleMessage{role: "user", content: value})
				m.request.Messages = append(m.request.Messages, Message{Role: "user", Content: value})
				m.sessionDirty = true
				m.refreshTranscript()
				return m, m.runTurn(m.request)
			}
		}
	}

	var cmd tea.Cmd
	if m.input.Focused() && !m.loading {
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m consoleModel) hasUnsavedSessionChanges() bool {
	return (m.session == nil && len(m.request.Messages) > 0) || (m.session != nil && m.sessionDirty)
}

func (m *consoleModel) syncSessionFromRequest() {
	if m.session != nil {
		m.session.Messages = cloneMessages(m.request.Messages)
	}
}

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

func saveSessionCmd(session *ChatSession, exit bool) tea.Cmd {
	copy := *session
	copy.Messages = cloneMessages(session.Messages)
	return func() tea.Msg {
		err := saveChatSession(&copy)
		return sessionSavedMsg{session: &copy, err: err, exit: exit}
	}
}

func listSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		sessions, err := listChatSessions()
		return sessionListLoadedMsg{sessions: sessions, err: err}
	}
}

func listModelsCmd(client *Client, ctx context.Context, timeout time.Duration) tea.Cmd {
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

func loadModelCmd(client *Client, ctx context.Context, timeout time.Duration, model, previous string, overlay *liveMetricsOverlay) tea.Cmd {
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
		err := client.loadModel(loadCtx, model)
		if err != nil && monitor != nil {
			monitor.Stop()
			monitor = nil
		}
		if err == nil && overlay != nil {
			overlay.ModelLoaded()
		}
		return modelLoadedMsg{model: model, previous: previous, monitor: monitor, err: err}
	}
}

func loadSessionCmd(summary ChatSessionSummary, client *Client, ctx context.Context, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		session, err := loadChatSession(summary.Path)
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
		}
		return sessionLoadedMsg{session: session}
	}
}

func (m consoleModel) runTurn(request ChatRequest) tea.Cmd {
	return func() tea.Msg {
		turnCtx, cancel := context.WithTimeout(m.ctx, m.timeout)
		defer cancel()
		switch m.mode {
		case consoleStreaming:
			reasoningOpen := false
			snapshot, err := m.client.withoutLiveMetricsOverlay(m.ctx).GenerateStreamingSnapshot(turnCtx, &request, func(chunk InferenceStreamChunk) error {
				for _, choice := range chunk.Choices {
					if reasoning := choice.Delta.ReasoningContent; reasoning != "" {
						if !reasoningOpen {
							if m.send != nil {
								m.send(consoleChunkMsg("<think>\n"))
							}
							reasoningOpen = true
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
						if reasoningOpen {
							if m.send != nil {
								m.send(consoleChunkMsg("\n</think>\n\n"))
							}
							reasoningOpen = false
						}
						if m.send != nil {
							m.send(consoleChunkMsg(content))
						}
					}
				}
				return nil
			})
			if reasoningOpen && m.send != nil {
				m.send(consoleChunkMsg("\n</think>"))
			}
			if err == nil && m.complete != nil {
				m.complete()
			}
			return consoleTurnResult{content: lastInteractionContent(snapshot), snapshot: snapshot, model: request.Model, err: err}
		case consoleSnapshot:
			snapshot, err := m.client.withoutLiveMetricsOverlay(turnCtx).GenerateSnapshot(turnCtx, &request)
			content := ""
			if snapshot != nil && len(snapshot.Interaction) > 0 {
				content = snapshot.Interaction[len(snapshot.Interaction)-1].Content
			}
			if err == nil && m.persistence != nil {
				all := append(append([]*ModelSnapshot(nil), m.snapshots...), snapshot)
				err = m.persistence.save(all)
			}
			if err == nil && m.complete != nil {
				m.complete()
			}
			return consoleTurnResult{content: content, snapshot: snapshot, model: request.Model, err: err}
		default:
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

func lastInteractionContent(snapshot *ModelSnapshot) string {
	if snapshot == nil || len(snapshot.Interaction) == 0 {
		return ""
	}
	return snapshot.Interaction[len(snapshot.Interaction)-1].Content
}

func (m *consoleModel) resize() {
	contentHeight := max(1, m.height-m.footerHeight())
	showSidebar := m.sidebarOpen && m.width >= consoleMinMainWidth+consoleMinSidebar
	sidebarOuter := 0
	if showSidebar {
		sidebarOuter = min(max(consoleMinSidebar, m.sidebarWidth), m.width-consoleMinMainWidth)
	}
	mainWidth := max(1, m.width-sidebarOuter)
	m.transcript.Width = mainWidth
	// Main header, input header, input row, and one blank row occupy four
	// lines; the remainder belongs to the scrolling transcript.
	m.transcript.Height = max(1, contentHeight-4)
	m.input.Width = max(1, mainWidth-lipgloss.Width(m.input.Prompt))
	m.sidebar.Width = max(1, sidebarOuter)
	// Match the main pane's final spacer so neither pane touches the footer.
	m.sidebar.Height = max(1, contentHeight-2)
	m.refreshTranscript()
	m.refreshSidebar()
}

func (m consoleModel) footerHeight() int {
	if m.mcpFooter != "" {
		return 3
	}
	return consoleFooterHeight + 1
}

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

func indentContinuationLines(content string) string {
	return strings.ReplaceAll(content, "\n", "\n ")
}

func (m *consoleModel) refreshTranscript() {
	var out strings.Builder
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
			out.WriteString(renderAssistantMessage(m.mainAssistantReasoningStyle, m.mainAssistantContentStyle, icon, label, message.content))
		} else {
			out.WriteString(" " + renderWhiteIcon(icon) + style.Render(label+indentContinuationLines(message.content)))
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

var sidebarKeys = []string{
	"n_ctx", "speculative", "is_processing", "seed", "temperature", "top_k", "top_p", "min_p",
	"repeat_last_n", "repeat_penalty", "presence_penalty", "frequency_penalty", "dry_multiplier",
	"dry_base", "dry_allowed_length", "dry_penalty_last_n", "max_tokens", "n_predict", "n_keep",
	"n_discard", "stream", "reasoning_in_content", "speculative.types",
}

func (m *consoleModel) refreshSidebar() {
	fields := sidebarFields(m.slots)
	keys := append([]string(nil), sidebarKeys...)
	sort.Strings(keys)
	lines := make([]string, 0, len(keys)+1)
	lines = append(lines, "model_id: "+m.request.Model)
	// Reapply the sidebar text/background style after each white icon. A
	// nested lipgloss style emits a reset sequence, which would otherwise
	// expose the terminal's default background for the rest of the line.
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

func (m consoleModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.sessionView != sessionViewChat {
		return m.sessionModalView()
	}
	main := strings.Join([]string{
		renderFullWidthHeader(DefaultUnicode.Home, "Chat Session", defaultConsoleUITheme.mainHeader, m.transcript.Width),
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
	footerText := m.footer
	if m.sessionStatus != "" {
		footerText += " | " + m.sessionStatus
	}
	footer := ansi.Truncate(footerText, max(0, m.width-1), "")
	footer = renderIconBar(DefaultUnicode.DoubleArrowRight, "#FFFF00", footer, m.footerBarLiveMetricsStyle.Width(m.width).MaxHeight(1))
	if m.mcpFooter != "" {
		mcpFooter := ansi.Truncate(m.mcpFooter, max(0, m.width-1), "")
		mcpFooter = renderIconBar(DefaultUnicode.DoubleArrowRight, "#FFFF00", mcpFooter, m.footerBarMCPStyle.Width(m.width).MaxHeight(1))
		footer = lipgloss.JoinVertical(lipgloss.Left, mcpFooter, footer)
	}
	keyBindingText := consoleFooterKeyBindings
	if m.mcpFooter != "" {
		keyBindingText += " | Ctrl+T List Tools"
	}
	keyBindings := renderIconBar(DefaultUnicode.DoubleArrowRight, "#FFFF00", ansi.Truncate(keyBindingText, max(0, m.width-1), ""), m.footerBarKeyBindingsStyle.Width(m.width).MaxHeight(1))
	footer = lipgloss.JoinVertical(lipgloss.Left, footer, keyBindings)
	return lipgloss.JoinVertical(lipgloss.Left, content, footer)
}

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

func runConsoleChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, mode consoleMode, options ...ClientOption) ([]*ModelSnapshot, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	request := cloneChatRequest(req)
	if request.Model == "" {
		return nil, fmt.Errorf("request model is required")
	}
	client := newClientFromConfig(ctx, cfg, options...)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	model := newConsoleModel(runCtx, client, request, time.Duration(cfg.Timeout), cfg.SidebarWidth, mode)
	model.cancel = cancel
	if mode != consoleSnapshot {
		model.session, err = newUnsavedChatSession(request)
		if err == nil {
			err = saveChatSession(model.session)
		}
		if err != nil {
			return nil, err
		}
	}
	if mode == consoleSnapshot && cfg.PersistSnapshots {
		model.persistence, err = newSnapshotSession(request.Model)
		if err != nil {
			return nil, err
		}
	}

	var program *tea.Program
	var monitor *inferenceMonitor
	overlay := &liveMetricsOverlay{startedAt: time.Now(), model: request.Model, notify: func(update overlayUpdate) {
		if program != nil {
			// Overlay methods are also called from consoleModel.Update. Sending
			// synchronously from that event loop would deadlock on Bubble Tea's
			// unbuffered message channel, so always publish asynchronously.
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
	if runErr != nil {
		return nil, runErr
	}
	result, ok := final.(consoleModel)
	if !ok {
		return nil, nil
	}
	if result.modelMonitor != nil && result.modelMonitor != monitor {
		result.modelMonitor.Stop()
	}
	return result.snapshots, result.err
}

func runConsoleMCPChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, approve MCPApprovalFunc, options ...ClientOption) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	request := cloneChatRequest(req)
	if request.Model == "" {
		return fmt.Errorf("request model is required")
	}
	timeout := time.Duration(cfg.Timeout)
	discoveryCtx, cancel := context.WithTimeout(ctx, timeout)
	tools, err := discoverMCPTools(discoveryCtx, cfg, timeout)
	cancel()
	if err != nil {
		return err
	}

	client := newClientFromConfig(ctx, cfg, append(options, WithLiveMetricsOverlay(false))...)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	model := newConsoleModel(runCtx, client, request, timeout, cfg.SidebarWidth, consoleNonStreaming)
	model.cancel = cancel
	model.session, err = newUnsavedChatSession(request)
	if err == nil {
		err = saveChatSession(model.session)
	}
	if err != nil {
		return err
	}
	model.mcpFooter = fmt.Sprintf("  [Induction: MCP] %d tools available ", len(tools))
	model.mcpTools = tools

	var program *tea.Program
	var monitor *inferenceMonitor
	status := func(message string) {
		if program != nil {
			program.Send(consoleMCPMsg(message))
			if activity, ok := formatMCPToolActivity(message, tools); ok {
				program.Send(consoleMCPActivityMsg(activity))
			}
		}
	}
	model.inferTurn = func(turnCtx context.Context, turn *ChatRequest) (*InferenceResponse, error) {
		return runMCPToolLoopWith(turnCtx, turn, tools, timeout, approve, status, func(inferCtx context.Context, toolTurn *ChatRequest) (*InferenceResponse, error) {
			return client.infer(inferCtx, toolTurn)
		})
	}

	overlay := &liveMetricsOverlay{startedAt: time.Now(), model: request.Model, notify: func(update overlayUpdate) {
		if program != nil {
			// See runConsoleChat: model updates can originate from Update itself.
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
	if runErr != nil {
		return runErr
	}
	result, ok := final.(consoleModel)
	if ok {
		if result.modelMonitor != nil && result.modelMonitor != monitor {
			result.modelMonitor.Stop()
		}
		return result.err
	}
	return nil
}

func formatMCPToolActivity(message string, tools []boundMCPTool) (string, bool) {
	const prefix = "[Induction: MCP] "
	status := strings.TrimSpace(message)
	status = strings.TrimPrefix(status, prefix)
	parts := strings.Split(status, " · ")
	if len(parts) < 2 {
		return "", false
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
		if binding.tool.Name == parts[0] {
			if binding.server.Name != "" {
				server = binding.server.Name
			}
			break
		}
	}
	return fmt.Sprintf("   [Tool Call: %s] %s: %s", label, server, parts[0]), true
}
