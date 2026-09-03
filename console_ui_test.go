package induction

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConsoleLayoutAndSidebarToggle(t *testing.T) {
	m := newConsoleModel(context.Background(), nil, ChatRequest{Model: "model"}, 0, 30, consoleNonStreaming)
	loaded := m.Init()().(consoleModelLoadedMsg)
	updated, _ := m.Update(loaded)
	m = updated.(consoleModel)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(consoleModel)

	if !m.sidebarOpen {
		t.Fatal("sidebar should default to open")
	}
	if m.transcript.Width != 70 || m.sidebar.Width != 30 {
		t.Fatalf("unexpected pane widths: transcript=%d sidebar=%d", m.transcript.Width, m.sidebar.Width)
	}
	if m.sidebar.Height != 20 || m.transcript.Height != 18 {
		t.Fatalf("unexpected pane heights: transcript=%d sidebar=%d", m.transcript.Height, m.sidebar.Height)
	}

	// Printable text always belongs to the focused chat input.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(consoleModel)
	if !m.sidebarOpen || m.input.Value() != "s" {
		t.Fatalf("focused s should type, not toggle: open=%v value=%q", m.sidebarOpen, m.input.Value())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" more text")})
	m = updated.(consoleModel)
	if m.input.Value() != "s more text" {
		t.Fatalf("input did not accept text: %q", m.input.Value())
	}
}

func TestConsoleInputDisabledWhileModelLoads(t *testing.T) {
	m := newConsoleModel(context.Background(), nil, ChatRequest{Model: "test-model"}, time.Second, 30, consoleNonStreaming)
	if !m.loading || m.input.Focused() {
		t.Fatalf("input should start disabled while loading: loading=%v focused=%v", m.loading, m.input.Focused())
	}
	if got := m.input.View(); !strings.Contains(got, "You: (model test-model is loading, please wait...)") {
		t.Fatalf("missing loading message: %q", got)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ignored")})
	m = updated.(consoleModel)
	if m.input.Value() != "" {
		t.Fatalf("disabled input accepted text: %q", m.input.Value())
	}
	updated, _ = m.Update(consoleModelLoadedMsg{})
	m = updated.(consoleModel)
	if m.loading || !m.input.Focused() || m.input.Value() != "" || m.input.Prompt != " "+DefaultUnicode.Star+" You: " {
		t.Fatalf("input was not reset and enabled after loading: %#v", m.input)
	}
}

func TestConsoleModelLoadErrorDoesNotLockInput(t *testing.T) {
	m := newConsoleModel(context.Background(), nil, ChatRequest{Model: "model"}, time.Second, 24, consoleNonStreaming)
	updated, _ := m.Update(consoleModelLoadedMsg{err: context.DeadlineExceeded})
	m = updated.(consoleModel)
	if m.loading || !m.input.Focused() || m.input.Prompt != " "+DefaultUnicode.Star+" You: " {
		t.Fatalf("model load error left input locked: loading=%v focused=%v prompt=%q", m.loading, m.input.Focused(), m.input.Prompt)
	}
}

func TestConsoleCtrlCAlwaysQuitsImmediately(t *testing.T) {
	m := newConsoleModel(context.Background(), nil, ChatRequest{Model: "model"}, time.Second, 24, consoleNonStreaming)
	m.sessionDirty = true
	m.sessionLoading = true
	m.waiting = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if updated.(consoleModel).sessionLoading != true {
		t.Fatal("Ctrl-C should not wait for session cleanup")
	}
	if cmd == nil {
		t.Fatal("Ctrl-C did not return a quit command")
	}
	quit := cmd()
	if _, ok := quit.(tea.QuitMsg); !ok {
		t.Fatalf("Ctrl-C command type = %T, want tea.QuitMsg", quit)
	}
}

func TestConsoleInitialModelLoadMarksOverlayReady(t *testing.T) {
	m := newConsoleModel(context.Background(), nil, ChatRequest{Model: "model"}, 0, 24, consoleNonStreaming)
	m.modelOverlay = &liveMetricsOverlay{model: "model", loading: true, notify: func(update overlayUpdate) {
		if update.footer != "" {
			m.footer = update.footer
		}
	}}
	updated, _ := m.Update(consoleModelLoadedMsg{})
	m = updated.(consoleModel)
	if m.loading || !strings.Contains(m.footer, "Ready") {
		t.Fatalf("initial model load did not mark overlay ready: loading=%v footer=%q", m.loading, m.footer)
	}
}

func TestConsoleOverlayReadyEnablesInput(t *testing.T) {
	m := newConsoleModel(context.Background(), nil, ChatRequest{Model: "model"}, time.Second, 24, consoleNonStreaming)
	updated, _ := m.Update(consoleOverlayMsg(overlayUpdate{
		footer:     formatModelReadyPlain("model"),
		modelReady: true,
	}))
	m = updated.(consoleModel)
	if m.loading || !m.input.Focused() || m.input.Prompt != " "+DefaultUnicode.Star+" You: " {
		t.Fatalf("overlay readiness did not enable input: loading=%v focused=%v prompt=%q", m.loading, m.input.Focused(), m.input.Prompt)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(consoleModel)
	if m.input.Value() != "x" {
		t.Fatalf("ready chat input did not accept text: %q", m.input.Value())
	}
}

func TestSidebarFieldsAreFlattenedAndStable(t *testing.T) {
	slots := SlotsData{{
		"n_ctx":         float64(262144),
		"speculative":   true,
		"is_processing": true,
		"params": map[string]interface{}{
			"seed":              float64(4294967295),
			"temperature":       float64(0.949999988079071),
			"speculative.types": "none,draft-mtp",
		},
	}}
	fields := sidebarFields(slots)
	if fields["seed"] != float64(4294967295) || fields["speculative.types"] != "none,draft-mtp" {
		t.Fatalf("nested params were not flattened: %#v", fields)
	}

	m := newConsoleModel(context.Background(), nil, ChatRequest{Model: "model"}, 0, 32, consoleNonStreaming)
	m.slots = slots
	m.width, m.height = 90, 30
	m.resize()
	content := m.sidebar.View()
	for _, want := range []string{"model_id: model", "n_ctx: 262144", "speculative: true", "seed: 4294967295", "temperature: 0.95", "speculative.types:"} {
		if !strings.Contains(content, want) {
			t.Fatalf("sidebar does not contain %q: %q", want, content)
		}
	}
	if strings.Index(content, "model_id:") > strings.Index(content, "n_ctx:") || strings.Index(content, "n_ctx:") > strings.Index(content, "seed:") {
		t.Fatalf("sidebar fields are not in stable order: %q", content)
	}
}

func TestConsoleFooterRemainsFullWidth(t *testing.T) {
	m := newConsoleModel(context.Background(), nil, ChatRequest{Model: "model"}, 0, 24, consoleNonStreaming)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m = updated.(consoleModel)
	view := m.View()
	for _, header := range []string{"Chat Session", "Model Information", "User Input", "[Induction: Live Metrics]"} {
		if !strings.Contains(view, header) {
			t.Fatalf("view does not contain header %q: %q", header, view)
		}
	}
	lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
	if len(lines) < 20 {
		t.Fatalf("view did not occupy the terminal height: got %d lines", len(lines))
	}
	if got := len([]rune(lines[len(lines)-1])); got < 80 {
		t.Fatalf("footer is not full width: got %d cells", got)
	}
	if strings.TrimSpace(lines[len(lines)-3]) != "" {
		t.Fatalf("expected a blank row between user input and footer: %q", lines[len(lines)-2])
	}
}

func TestConsoleMCPFooterAddsAThirdReservedRow(t *testing.T) {
	m := newConsoleModel(context.Background(), nil, ChatRequest{Model: "model"}, 0, 24, consoleNonStreaming)
	m.mcpFooter = "  [Induction: MCP] search_documents · running… "
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m = updated.(consoleModel)

	view := m.View()
	for _, footer := range []string{"[Induction: MCP]", "[Induction: Live Metrics]"} {
		if !strings.Contains(view, footer) {
			t.Fatalf("view does not contain footer %q: %q", footer, view)
		}
	}
	lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
	if len(lines) < 20 {
		t.Fatalf("view did not occupy the terminal height: got %d lines", len(lines))
	}
	if !strings.Contains(lines[len(lines)-3], "[Induction: MCP]") {
		t.Fatalf("MCP footer is not above metrics footer: %q", lines[len(lines)-3:])
	}
	if !strings.Contains(lines[len(lines)-2], "[Induction: Live Metrics]") {
		t.Fatalf("metrics footer is not above key bindings: %q", lines[len(lines)-3:])
	}
	if !strings.Contains(lines[len(lines)-1], "Ctrl+S Save") {
		t.Fatalf("key bindings footer is not last: %q", lines[len(lines)-3:])
	}
}

func TestOverlayPublishesIdleSlotsToSidebar(t *testing.T) {
	var update overlayUpdate
	overlay := &liveMetricsOverlay{notify: func(next overlayUpdate) { update = next }}
	slots := SlotsData{{"n_ctx": float64(4096), "is_processing": false}}
	overlay.Update(slots)
	if len(update.slots) != 1 || update.slots[0]["is_processing"] != false {
		t.Fatalf("idle slots were not published: %#v", update.slots)
	}
}

func TestConsoleSessionSaveAndLoadShortcuts(t *testing.T) {
	m := newConsoleModel(context.Background(), nil, ChatRequest{Model: "model"}, 0, 24, consoleNonStreaming)
	m.loading = false
	m.input.Focus()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(consoleModel)
	if m.sessionView != sessionViewSave || !m.sessionTitleInput.Focused() || m.input.Focused() {
		t.Fatalf("Ctrl+S did not enter save view: view=%d titleFocused=%v inputFocused=%v", m.sessionView, m.sessionTitleInput.Focused(), m.input.Focused())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(consoleModel)
	if m.sessionView != sessionViewChat || !m.input.Focused() {
		t.Fatalf("Esc did not return to chat: view=%d focused=%v", m.sessionView, m.input.Focused())
	}
	m.request.Messages = []Message{{Role: "user", Content: "unsaved"}}
	m.sessionDirty = true
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = updated.(consoleModel)
	if m.sessionView != sessionViewConfirmLoad {
		t.Fatalf("Ctrl+L did not protect unsaved chat: view=%d", m.sessionView)
	}
}

func TestConsoleSessionRestorationKeepsRequestContext(t *testing.T) {
	m := newConsoleModel(context.Background(), nil, ChatRequest{Model: "old"}, 0, 24, consoleNonStreaming)
	session := &ChatSession{Version: chatSessionVersion, ID: "11111111-1111-4111-8111-111111111111", Title: "Restored", Model: "new", Messages: []Message{{Role: "user", Content: "one"}, {Role: "assistant", Content: "two"}}}
	updated, _ := m.Update(sessionLoadedMsg{session: session})
	m = updated.(consoleModel)
	if m.request.Model != "new" || len(m.request.Messages) != 2 || len(m.messages) != 2 || m.messages[1].content != "two" {
		t.Fatalf("session restoration lost context: request=%#v messages=%#v", m.request.Messages, m.messages)
	}
}

func TestConsoleTurnRecordsTheModelUsedForThatTurn(t *testing.T) {
	m := newConsoleModel(context.Background(), nil, ChatRequest{Model: "GLM-4.7-Flash-Q4_K_M"}, time.Second, 24, consoleNonStreaming)
	m.loading = false
	m.session = &ChatSession{Version: chatSessionVersion, ID: "11111111-1111-4111-8111-111111111111", Type: sessionTypeUnsaved, Model: "GLM-4.7-Flash-Q4_K_M"}

	snapshot := &ModelSnapshot{ModelID: "GLM-4.7-Flash-Q4_K_M"}
	updated, _ := m.Update(consoleTurnResult{content: "answer", snapshot: snapshot, model: "different-model"})
	m = updated.(consoleModel)
	if snapshot.ModelID != "different-model" {
		t.Fatalf("snapshot model = %q, want turn model", snapshot.ModelID)
	}
	if m.session.Model != "different-model" || len(m.session.Snapshots) != 1 {
		t.Fatalf("session did not retain latest turn model: %#v", m.session)
	}
}

func TestConsoleSessionRestorationUsesMostRecentTurnModel(t *testing.T) {
	m := newConsoleModel(context.Background(), nil, ChatRequest{Model: "old"}, 0, 24, consoleNonStreaming)
	session := &ChatSession{
		Version:  chatSessionVersion,
		ID:       "11111111-1111-4111-8111-111111111111",
		Title:    "Restored",
		Model:    "first-model",
		Messages: []Message{{Role: "user", Content: "one"}, {Role: "assistant", Content: "two"}},
		Snapshots: []*ModelSnapshot{
			{ModelID: "first-model"},
			{ModelID: "last-model"},
		},
	}
	updated, _ := m.Update(sessionLoadedMsg{session: session})
	m = updated.(consoleModel)
	if m.request.Model != "last-model" {
		t.Fatalf("restored model = %q, want most recent turn model", m.request.Model)
	}
}

func TestConsoleModelPickerMarksLoadedModels(t *testing.T) {
	m := newConsoleModel(context.Background(), nil, ChatRequest{Model: "current"}, 0, 24, consoleNonStreaming)
	m.loading = false
	m.width, m.height = 100, 24
	m.input.Focus()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(consoleModel)
	if cmd == nil || m.sessionView != sessionViewChat || !m.sessionLoading || m.input.Focused() {
		t.Fatalf("empty Enter/Ctrl-M did not begin model picker: view=%d loading=%v focused=%v", m.sessionView, m.sessionLoading, m.input.Focused())
	}
	updated, _ = m.Update(modelListLoadedMsg{models: []RuntimeModel{{ID: "loaded-model", State: ModelRuntimeLoaded}, {ID: "idle-model", State: ModelRuntimeUnloaded}}})
	m = updated.(consoleModel)
	if m.sessionView != sessionViewModels || m.input.Focused() {
		t.Fatalf("model list did not open model picker: view=%d focused=%v", m.sessionView, m.input.Focused())
	}
	if !strings.Contains(m.View(), "loaded-model (loaded)") || !strings.Contains(m.View(), "idle-model") {
		t.Fatalf("model picker did not render loaded suffix: %q", m.View())
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(consoleModel)
	if cmd == nil || m.modelLoading != "loaded-model" || m.sessionView != sessionViewChat || !m.loading || !strings.Contains(m.View(), "model loaded-model is loading") {
		t.Fatalf("Enter did not begin model loading: loading=%q view=%q", m.modelLoading, m.View())
	}
	updated, _ = m.Update(cmd())
	m = updated.(consoleModel)
	if m.modelLoading != "" || !strings.Contains(m.sessionStatus, "Model load failed") {
		t.Fatalf("model load failure was not reported: loading=%q status=%q", m.modelLoading, m.sessionStatus)
	}
	if m.sessionView != sessionViewModels {
		t.Fatalf("failed model load did not return to picker: view=%d", m.sessionView)
	}
}
