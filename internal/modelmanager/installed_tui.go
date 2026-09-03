package modelmanager

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// InstalledAction selects the operation offered for an installed model.
type InstalledAction string

const (
	// ActionDetails displays installation metadata.
	ActionDetails InstalledAction = "details"
	// ActionVerify checks the installed artifact digest.
	ActionVerify InstalledAction = "verify"
	// ActionUpdate downloads a newer artifact revision.
	ActionUpdate InstalledAction = "update"
	// ActionRemove deletes the selected installation.
	ActionRemove InstalledAction = "remove"
)

type installedItem struct{ Installation }

func (i installedItem) Title() string { return InstallationModelID(i.Installation) }
func (i installedItem) Description() string {
	size := i.Manifest.SizeBytes
	if i.Manifest.SchemaVersion == 2 {
		size = 0
		for _, file := range i.Manifest.Files {
			size += file.SizeBytes
		}
	}
	return fmt.Sprintf("revision %s  •  %s", shortRevision(i.Manifest.Revision), formatBytes(size))
}
func (i installedItem) FilterValue() string { return i.Title() }

type installedOperationMsg struct {
	text string
	err  error
}

// InstalledModel is the Bubble Tea model for managing local installations.
type InstalledModel struct {
	ctx      context.Context
	client   *HFCLIClient
	options  InteractiveOptions
	action   InstalledAction
	list     list.Model
	spinner  spinner.Model
	selected Installation
	screen   string
	result   string
	err      error
	cancel   context.CancelFunc
}

// NewInstalledModel creates an installed-model management UI.
func NewInstalledModel(ctx context.Context, client *HFCLIClient, options InteractiveOptions, action InstalledAction, index InstalledIndex, initial string) InstalledModel {
	items := make([]list.Item, len(index.Installations))
	selected := 0
	for i, item := range index.Installations {
		items[i] = installedItem{item}
		if initial != "" && InstallationMatches(item, initial) {
			selected = i
		}
	}
	models := list.New(items, list.NewDefaultDelegate(), 0, 0)
	models.Title = "Installed models — " + string(action)
	models.SetShowHelp(false)
	models.Select(selected)
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	return InstalledModel{ctx: ctx, client: client, options: options, action: action, list: models, spinner: spin, screen: "list"}
}

// Init implements tea.Model and performs no startup command.
func (m InstalledModel) Init() tea.Cmd { return nil }
func (m *InstalledModel) operationCmd() tea.Cmd {
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	selected, action, client, options := m.selected, m.action, m.client, m.options
	return func() tea.Msg {
		switch action {
		case ActionVerify:
			verification, err := Verify(ctx, selected)
			if err != nil {
				return installedOperationMsg{err: err}
			}
			return installedOperationMsg{text: fmt.Sprintf("%s\nExpected: %s\nActual: %s", verification.Status, verification.ExpectedSHA256, verification.ActualSHA256)}
		case ActionRemove:
			if err := RemoveInstallation(options.ModelsPath, selected); err != nil {
				return installedOperationMsg{err: err}
			}
			return installedOperationMsg{text: "Removed only the artifact(s) owned by the selected manifest."}
		case ActionUpdate:
			if selected.Manifest.SchemaVersion == 2 {
				return installedOperationMsg{err: fmt.Errorf("interactive sharded update requires selecting replacement artifacts manually")}
			}
			revision, files, err := client.ListFiles(ctx, selected.Manifest.RepositoryID)
			if err != nil {
				return installedOperationMsg{err: err}
			}
			var remote *ModelFile
			for i := range files {
				if files[i].Path == selected.Manifest.ModelFile {
					remote = &files[i]
					break
				}
			}
			if remote == nil {
				return installedOperationMsg{err: fmt.Errorf("remote artifact no longer exists; select a replacement manually")}
			}
			if revision == selected.Manifest.Revision {
				return installedOperationMsg{text: "CURRENT — no update is available."}
			}
			manifest, err := UpdateInstallation(ctx, client.Path, options.ModelsPath, selected, revision, *remote)
			if err != nil {
				return installedOperationMsg{err: err}
			}
			return installedOperationMsg{text: fmt.Sprintf("Updated to %s\nSHA-256: %s", manifest.Revision, manifest.SHA256)}
		}
		return installedOperationMsg{err: fmt.Errorf("unsupported action %q", action)}
	}
}

// Update implements tea.Model and advances the installed-model workflow.
func (m InstalledModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(max(10, msg.Width-2), max(4, msg.Height-4))
		return m, nil
	case installedOperationMsg:
		m.cancel = nil
		if msg.err != nil {
			recordInteraction("installed_action_failed", "action="+string(m.action), "model="+InstallationModelID(m.selected), "error="+msg.err.Error())
			m.err = msg.err
			m.screen = "error"
		} else {
			recordInteraction("installed_action_completed", "action="+string(m.action), "model="+InstallationModelID(m.selected))
			m.result = msg.text
			m.screen = "result"
		}
		return m, nil
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			recordInteraction("installed_action_cancelled", "action="+string(m.action))
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		switch m.screen {
		case "list":
			if key == "q" || key == "esc" {
				return m, tea.Quit
			}
			if key == "enter" && len(m.list.Items()) > 0 {
				m.selected = m.list.SelectedItem().(installedItem).Installation
				recordInteraction("installed_model_selected", "action="+string(m.action), "model="+InstallationModelID(m.selected))
				if m.action == ActionDetails {
					m.screen = "details"
					return m, nil
				}
				if m.action == ActionVerify {
					m.screen = "working"
					return m, tea.Batch(m.spinner.Tick, m.operationCmd())
				}
				m.screen = "confirm"
				return m, nil
			}
		case "details":
			if key == "esc" || key == "backspace" {
				m.screen = "list"
				return m, nil
			}
			if key == "q" {
				return m, tea.Quit
			}
		case "confirm":
			if key == "n" || key == "esc" {
				recordInteraction("installed_action_declined", "action="+string(m.action), "model="+InstallationModelID(m.selected))
				m.screen = "list"
				return m, nil
			}
			if key == "y" {
				recordInteraction("installed_action_confirmed", "action="+string(m.action), "model="+InstallationModelID(m.selected))
				m.screen = "working"
				return m, tea.Batch(m.spinner.Tick, m.operationCmd())
			}
		case "working":
			if key == "esc" {
				if m.cancel != nil {
					m.cancel()
				}
				m.screen = "list"
				return m, nil
			}
		case "result", "error":
			if key == "enter" || key == "esc" {
				m.screen = "list"
				return m, nil
			}
			if key == "q" {
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	if m.screen == "list" {
		m.list, cmd = m.list.Update(msg)
	} else if m.screen == "working" {
		m.spinner, cmd = m.spinner.Update(msg)
	}
	return m, cmd
}

// View implements tea.Model and renders the installed-model workflow.
func (m InstalledModel) View() string {
	if len(m.list.Items()) == 0 && m.screen == "list" {
		return "No installed models were found.\n\nq quit\n"
	}
	switch m.screen {
	case "list":
		return m.list.View() + "\nenter select • q/esc quit\n"
	case "details":
		return installedDetails(m.selected) + "\n\nesc back • q quit\n"
	case "confirm":
		verb := "Update"
		if m.action == ActionRemove {
			verb = "Remove"
		}
		return fmt.Sprintf("%s this installed model?\n\n%s\n\ny confirm • n cancel\n", verb, installedDetails(m.selected))
	case "working":
		return fmt.Sprintf("%s Running %s for %s…\n\nesc cancel\n", m.spinner.View(), m.action, InstallationModelID(m.selected))
	case "result":
		return m.result + "\n\nenter back • q quit\n"
	case "error":
		return fmt.Sprintf("Operation failed: %v\n\nenter back • q quit\n", m.err)
	}
	return ""
}
func installedDetails(item Installation) string {
	lines := []string{"Model: " + InstallationModelID(item), "Repository: " + item.Manifest.RepositoryID, "Revision: " + item.Manifest.Revision, "Manifest: " + item.ManifestPath}
	if item.Manifest.SchemaVersion == 1 {
		lines = append(lines, "Artifact: "+item.ArtifactPath, "Size: "+formatBytes(item.Manifest.SizeBytes), "SHA-256: "+unknown(item.Manifest.SHA256))
	} else {
		for _, file := range item.Manifest.Files {
			lines = append(lines, fmt.Sprintf("Artifact: %s (%s)\nSHA-256: %s", file.ModelFile, formatBytes(file.SizeBytes), unknown(file.SHA256)))
		}
	}
	return strings.Join(lines, "\n")
}
func unknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
func shortRevision(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

// InstallationModelID returns the stable repository/model identifier for item.
func InstallationModelID(item Installation) string {
	if item.Manifest.SchemaVersion == 1 {
		return item.Manifest.RepositoryID + "/" + item.Manifest.ModelFile
	}
	if len(item.Manifest.Files) > 0 {
		stem, _, err := ShardSet(ModelFile{Path: item.Manifest.Files[0].ModelFile}, manifestModelFiles(item.Manifest.Files))
		if err == nil && stem != "" {
			return item.Manifest.RepositoryID + "/" + stem
		}
		return item.Manifest.RepositoryID + "/" + item.Manifest.Files[0].ModelFile
	}
	return item.Manifest.RepositoryID + "/" + strings.TrimSuffix(filepath.Base(item.ManifestPath), ".json")
}

// InstallationMatches reports whether item identifies model.
func InstallationMatches(item Installation, model string) bool {
	if InstallationModelID(item) == model {
		return true
	}
	for _, file := range item.Manifest.Files {
		if item.Manifest.RepositoryID+"/"+file.ModelFile == model {
			return true
		}
	}
	return false
}
func manifestModelFiles(files []ManifestFile) []ModelFile {
	result := make([]ModelFile, len(files))
	for i, file := range files {
		result[i] = ModelFile{Path: file.ModelFile, Size: file.SizeBytes}
	}
	return result
}

// RunInstalledInteractive runs an installed-model operation UI.
func RunInstalledInteractive(ctx context.Context, in io.Reader, out io.Writer, client *HFCLIClient, options InteractiveOptions, action InstalledAction, initial string) error {
	index, err := BuildInstalledIndex(options.ModelsPath)
	if err != nil {
		return err
	}
	model := NewInstalledModel(ctx, client, options, action, index, initial)
	_, err = tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out)).Run()
	return err
}
