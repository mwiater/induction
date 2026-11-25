// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

const (
	// consoleFooterHeight identifies a fixed value used internally by this package.
	consoleFooterHeight = 1
	// consoleMinMainWidth identifies a fixed value used internally by this package.
	consoleMinMainWidth = 40
	// consoleMinSidebar identifies a fixed value used internally by this package.
	consoleMinSidebar = 16
)

// consoleMode is an internal data type used by this package.
type consoleMode int

const (
	// consoleNonStreaming identifies a fixed value used internally by this package.
	consoleNonStreaming consoleMode = iota
	// consoleStreaming identifies a fixed value used internally by this package.
	consoleStreaming
)

// consoleMessage is an internal data type used by this package.
type consoleMessage struct {
	// role stores the value represented by this member.
	role string
	// content stores the value represented by this member.
	content string
}

// consoleTurnResult is an internal data type used by this package.
type consoleTurnResult struct {
	// content stores the value represented by this member.
	content string
	// snapshot stores the value represented by this member.
	snapshot *ModelSnapshot
	// model stores the value represented by this member.
	model string
	// err stores the value represented by this member.
	err error
}

// consoleChunkMsg is an internal data type used by this package.
type consoleChunkMsg string

// consoleToolMsg is an internal data type used by this package.
type consoleToolMsg string

// consoleOverlayMsg is an internal data type used by this package.
type consoleOverlayMsg overlayUpdate

// consoleMCPMsg is an internal data type used by this package.
type consoleMCPMsg string

// consoleMCPActivityMsg is an internal data type used by this package.
type consoleMCPActivityMsg string

// pipelineRetryState is an internal data type used by this package.
type pipelineRetryState struct {
	// step stores the value represented by this member.
	step int
	// attempt stores the value represented by this member.
	attempt int
	// candidate stores the value represented by this member.
	candidate string
	// gate stores the value represented by this member.
	gate bool
}

// consoleModelLoadedMsg is an internal data type used by this package.
type consoleModelLoadedMsg struct {
	// err stores the value represented by this member.
	err error
	// props stores the value represented by this member.
	props *PropsData
	// slots stores the value represented by this member.
	slots SlotsData
}

// consoleMonitorStartedMsg is an internal data type used by this package.
type consoleMonitorStartedMsg struct { // monitor carries the monitor-start event.
	monitor *inferenceMonitor
}

// sessionViewMode is an internal data type used by this package.
type sessionViewMode int

const (
	// sessionViewChat identifies a fixed value used internally by this package.
	sessionViewChat sessionViewMode = iota
	// sessionViewSave identifies a fixed value used internally by this package.
	sessionViewSave
	// sessionViewLoad identifies a fixed value used internally by this package.
	sessionViewLoad
	// sessionViewConfirmLoad identifies a fixed value used internally by this package.
	sessionViewConfirmLoad
	// sessionViewModels identifies a fixed value used internally by this package.
	sessionViewModels
	// sessionViewTools identifies a fixed value used internally by this package.
	sessionViewTools
)

// sessionSavedMsg is an internal data type used by this package.
type sessionSavedMsg struct {
	// session stores the value represented by this member.
	session *ChatSession
	// err stores the value represented by this member.
	err error
	// exit stores the value represented by this member.
	exit bool
}

// sessionListLoadedMsg is an internal data type used by this package.
type sessionListLoadedMsg struct {
	// sessions stores the value represented by this member.
	sessions []ChatSessionSummary
	// err stores the value represented by this member.
	err error
}

// sessionLoadedMsg is an internal data type used by this package.
type sessionLoadedMsg struct {
	// session stores the value represented by this member.
	session *ChatSession
	// props stores the value represented by this member.
	props *PropsData
	// slots stores the value represented by this member.
	slots SlotsData
	// err stores the value represented by this member.
	err error
}

// modelListLoadedMsg is an internal data type used by this package.
type modelListLoadedMsg struct {
	// models stores the value represented by this member.
	models []RuntimeModel
	// err stores the value represented by this member.
	err error
}

// modelLoadedMsg is an internal data type used by this package.
type modelLoadedMsg struct {
	// model stores the value represented by this member.
	model string
	// previous stores the value represented by this member.
	previous string
	// monitor stores the value represented by this member.
	monitor *inferenceMonitor
	// props stores the value represented by this member.
	props *PropsData
	// slots stores the value represented by this member.
	slots SlotsData
	// err stores the value represented by this member.
	err error
}

// consoleModel is an internal data type used by this package.
type consoleModel struct {
	// ctx stores the value represented by this member.
	ctx context.Context
	// client stores the value represented by this member.
	client *Client
	// request stores the value represented by this member.
	request ChatRequest
	// timeout stores the value represented by this member.
	timeout time.Duration
	// mode stores the value represented by this member.
	mode consoleMode
	// snapshots stores the value represented by this member.
	snapshots []*ModelSnapshot
	// transcript stores the value represented by this member.
	transcript viewport.Model
	// sidebar stores the value represented by this member.
	sidebar viewport.Model
	// input stores the value represented by this member.
	input textinput.Model
	// messages stores the value represented by this member.
	messages []consoleMessage
	// slots stores the value represented by this member.
	slots SlotsData
	// footer stores the value represented by this member.
	footer string
	// mcpFooter stores the value represented by this member.
	mcpFooter string
	// width stores the value represented by this member.
	width int
	// height stores the value represented by this member.
	height int
	// sidebarWidth stores the value represented by this member.
	sidebarWidth int
	// sidebarOpen stores the value represented by this member.
	sidebarOpen bool
	// waiting stores the value represented by this member.
	waiting bool
	// streamedTurn stores the value represented by this member.
	streamedTurn bool
	// loading stores the value represented by this member.
	loading bool
	// initialPrompt stores the value represented by this member.
	initialPrompt string
	// autoSubmitInitialPrompt stores the value represented by this member.
	autoSubmitInitialPrompt bool
	// initialPromptApplied stores the value represented by this member.
	initialPromptApplied bool
	// initialModelSelection stores the value represented by this member.
	initialModelSelection bool
	// submittingInitialPrompt stores the value represented by this member.
	submittingInitialPrompt bool
	// autoExitAfterInitialChat stores the value represented by this member.
	autoExitAfterInitialChat bool
	// pipeline stores the value represented by this member.
	pipeline *Pipeline
	// pipelineStep stores the value represented by this member.
	pipelineStep int
	// pipelineRunning stores the value represented by this member.
	pipelineRunning bool
	// err stores the value represented by this member.
	err error
	// session stores the value represented by this member.
	session *ChatSession
	// sessionDirty stores the value represented by this member.
	sessionDirty bool
	// sessionView stores the value represented by this member.
	sessionView sessionViewMode
	// sessionTitleInput stores the value represented by this member.
	sessionTitleInput textinput.Model
	// sessionList stores the value represented by this member.
	sessionList []ChatSessionSummary
	// sessionCursor stores the value represented by this member.
	sessionCursor int
	// modelList stores the value represented by this member.
	modelList []RuntimeModel
	// modelCursor stores the value represented by this member.
	modelCursor int
	// modelLoading stores the value represented by this member.
	modelLoading string
	// mcpTools stores the value represented by this member.
	mcpTools []boundMCPTool
	// streamedToolCallNames stores the value represented by this member.
	streamedToolCallNames map[int]string
	// modelOverlay stores the value represented by this member.
	modelOverlay *liveMetricsOverlay
	// modelMonitor stores the value represented by this member.
	modelMonitor *inferenceMonitor
	// props stores the value represented by this member.
	props *PropsData
	// sessionStatus stores the value represented by this member.
	sessionStatus string
	// sessionLoading stores the value represented by this member.
	sessionLoading bool
	// pipelineOutputs stores the value represented by this member.
	pipelineOutputs map[string]json.RawMessage
	// pipelineInputs stores the value represented by this member.
	pipelineInputs map[string]any
	// pipelineRunID stores the value represented by this member.
	pipelineRunID string
	// artifactPathContext stores the value represented by this member.
	artifactPathContext ArtifactPathContext
	// pipelineFanoutItems stores the value represented by this member.
	pipelineFanoutItems []any
	// pipelineFanoutOutputs stores the value represented by this member.
	pipelineFanoutOutputs []json.RawMessage
	// pipelineFanoutIndex stores the value represented by this member.
	pipelineFanoutIndex int
	// pipelineFanoutStep stores the value represented by this member.
	pipelineFanoutStep int
	// pipelineQualityRetries stores the value represented by this member.
	pipelineQualityRetries map[int]int
	// pipelineQualityFeedback stores the value represented by this member.
	pipelineQualityFeedback string
	// pipelineRetryState stores the value represented by this member.
	pipelineRetryState *pipelineRetryState
	// pipelineRetryGateRequest stores the value represented by this member.
	pipelineRetryGateRequest *ChatRequest
	// exitSaving stores the value represented by this member.
	exitSaving bool
	// send stores the value represented by this member.
	send func(tea.Msg)
	// complete stores the value represented by this member.
	complete func()
	// inferTurn stores the value represented by this member.
	inferTurn func(context.Context, *ChatRequest) (*InferenceResponse, error)
	// inferSnapshotTurn stores the value represented by this member.
	inferSnapshotTurn func(context.Context, *ChatRequest) (*InferenceResponse, *ModelSnapshot, error)
	// startMonitor stores the value represented by this member.
	startMonitor func() tea.Cmd
	// cancel stores the value represented by this member.
	cancel context.CancelFunc

	// mainUserPromptStyle stores the value represented by this member.
	mainUserPromptStyle lipgloss.Style
	// mainAssistantReasoningStyle stores the value represented by this member.
	mainAssistantReasoningStyle lipgloss.Style
	// mainAssistantContentStyle stores the value represented by this member.
	mainAssistantContentStyle lipgloss.Style
	// sidebarHeaderStyle stores the value represented by this member.
	sidebarHeaderStyle lipgloss.Style
	// sidebarBackgroundStyle stores the value represented by this member.
	sidebarBackgroundStyle lipgloss.Style
	// sidebarTextStyle stores the value represented by this member.
	sidebarTextStyle lipgloss.Style
	// footerBarMCPStyle stores the value represented by this member.
	footerBarMCPStyle lipgloss.Style
	// footerBarLiveMetricsStyle stores the value represented by this member.
	footerBarLiveMetricsStyle lipgloss.Style
	// footerBarPipelineStyle stores the value represented by this member.
	footerBarPipelineStyle lipgloss.Style
	// footerBarKeyBindingsStyle stores the value represented by this member.
	footerBarKeyBindingsStyle lipgloss.Style
	// mainErrorStyle stores the value represented by this member.
	mainErrorStyle lipgloss.Style
	// mcpActivityStyle stores the value represented by this member.
	mcpActivityStyle lipgloss.Style
}

// canRunConsole performs the internal operation described by its signature.
func canRunConsole(in io.Reader, out io.Writer) bool {
	inFile, inOK := in.(*os.File)
	outFile, outOK := out.(*os.File)
	return inOK && outOK && term.IsTerminal(int(inFile.Fd())) && term.IsTerminal(int(outFile.Fd()))
}

// newConsoleModel performs the internal operation described by its signature.
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
		footerBarPipelineStyle:      defaultConsoleUITheme.footerBarPipeline,
		footerBarKeyBindingsStyle:   defaultConsoleUITheme.footerBarKeyBindings,
		mainErrorStyle:              defaultConsoleUITheme.mainError,
		mcpActivityStyle:            defaultConsoleUITheme.mcpActivity,
		streamedToolCallNames:       make(map[int]string),
	}
	m.pipelineFanoutStep = -1
	if client != nil && client.opts != nil {
		m.initialPrompt = client.opts.initialChatPrompt
		m.autoSubmitInitialPrompt = client.opts.initialChatPromptAutoSubmit
		m.initialModelSelection = client.opts.initialModelSelection
		m.autoExitAfterInitialChat = client.opts.autoExitAfterInitialChat
		m.pipeline = client.opts.pipeline
		m.pipelineRunning = m.pipeline != nil
		m.pipelineOutputs = make(map[string]json.RawMessage)
		m.pipelineInputs = make(map[string]any)
		m.artifactPathContext = defaultArtifactPathContext(m.pipeline)
		if client.opts.artifactPathContext != nil {
			m.artifactPathContext = *client.opts.artifactPathContext
		}
	}
	return m
}

// Init is an exported operation provided by this package.
// Additional context: Subscribe to lifecycle events before asking the server to load the model so the initial progress event cannot be missed.
func (m consoleModel) Init() tea.Cmd {
	if m.initialModelSelection {
		return listModelsCmd(m.ctx, m.client, m.timeout)
	}
	loadModel := func() tea.Msg {
		if m.client == nil {
			return consoleModelLoadedMsg{}
		}
		loadCtx, cancel := context.WithTimeout(m.ctx, m.timeout)
		defer cancel()
		loadTime, err := m.client.ensureModelLoaded(loadCtx, m.request.Model)
		if err != nil {
			return consoleModelLoadedMsg{err: err}
		}
		if loadTime > 0 {
			m.client.recordModelLoadDuration(m.request.Model, loadTime)
		}
		props, _ := m.client.fetchPropsReady(loadCtx, m.request.Model)
		slots, _ := m.client.fetchSlotsReady(loadCtx, m.request.Model)
		return consoleModelLoadedMsg{props: props, slots: slots}
	}
	if m.startMonitor != nil {

		return tea.Sequence(m.startMonitor(), loadModel)
	}
	return loadModel
}
