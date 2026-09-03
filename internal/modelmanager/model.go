package modelmanager

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Screen identifies the current page in the interactive model manager.
type Screen int

const (
	// ScreenQuery is the repository search page.
	ScreenQuery Screen = iota
	// ScreenSearching indicates that a repository search is in progress.
	ScreenSearching
	// ScreenRepositories displays matching repositories.
	ScreenRepositories
	// ScreenLoadingFiles indicates that repository files are being fetched.
	ScreenLoadingFiles
	// ScreenFiles displays selectable model files.
	ScreenFiles
	// ScreenConfirm asks for download confirmation.
	ScreenConfirm
	// ScreenDownloading indicates that selected files are being downloaded.
	ScreenDownloading
	// ScreenComplete indicates a successful download.
	ScreenComplete
	// ScreenError displays an operation error.
	ScreenError
)

// InteractiveOptions configures the interactive model-manager screens.
type InteractiveOptions struct {
	SearchResults                                                                int
	PreferredProviders, PreferredQuantizations, IncludePatterns, ExcludePatterns []string
	ModelsPath, HFPath                                                           string
}
type repositoryItem struct{ SearchResult }

func (i repositoryItem) Title() string { return i.ID }
func (i repositoryItem) Description() string {
	return fmt.Sprintf("%s  downloads %d  likes %d  modified %s  gated=%t private=%t", i.Provider, i.Downloads, i.Likes, i.LastModified.Format("2006-01-02"), i.Gated, i.Private)
}
func (i repositoryItem) FilterValue() string { return i.ID }

type fileItem struct{ ModelFile }

func (i fileItem) Title() string { return i.Path }
func (i fileItem) Description() string {
	quant := i.Quantization
	if quant == "" {
		quant = "unknown quantization"
	}
	return fmt.Sprintf("%s  •  %s", formatBytes(i.Size), quant)
}
func (i fileItem) FilterValue() string { return i.Path }

type searchCompletedMsg struct {
	query   string
	results []SearchResult
	err     error
}
type filesCompletedMsg struct {
	repository, revision string
	files                []ModelFile
	err                  error
}
type downloadCompletedMsg struct {
	manifest     Manifest
	manifestPath string
	err          error
}
type downloadProgressMsg struct{ ProgressUpdate }

// Model is the Bubble Tea state for the interactive model-manager workflow.
type Model struct {
	ctx                 context.Context
	client              HubClient
	options             InteractiveOptions
	Screen              Screen
	Query               textinput.Model
	Repositories, Files list.Model
	Spinner             spinner.Model
	Progress            progress.Model
	Help                help.Model
	err                 error
	cancel              context.CancelFunc
	width, height       int
	selectedRepository  SearchResult
	revision            string
	allFiles            []ModelFile
	selectedFile        ModelFile
	destination         Destination
	overwrite           bool
	manifest            Manifest
	manifestPath        string
	transfer            <-chan tea.Msg
	transferProgress    ProgressUpdate
	progressLastAt      time.Time
	progressLastBytes   int64
	progressRate        float64
	progressPhase       TransferPhase
}

// NewInteractiveModel creates a model-manager UI with an optional initial query.
func NewInteractiveModel(ctx context.Context, client HubClient, options InteractiveOptions, initialQuery string) Model {
	input := textinput.New()
	input.Placeholder = "Search Hugging Face"
	input.SetValue(initialQuery)
	input.Focus()
	repos := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	repos.Title = "Repositories"
	repos.SetShowHelp(false)
	files := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	files.Title = "Exact artifacts"
	files.SetShowHelp(false)
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	bar := progress.New(progress.WithDefaultGradient())
	m := Model{ctx: ctx, client: client, options: options, Screen: ScreenQuery, Query: input, Repositories: repos, Files: files, Spinner: spin, Progress: bar, Help: help.New()}
	if initialQuery != "" {
		m.Screen = ScreenSearching
	}
	return m
}

// NewModel creates the model-manager UI with explicit search settings.
func NewModel(ctx context.Context, client HubClient, limit int, providers []string, initialQuery string) Model {
	return NewInteractiveModel(ctx, client, InteractiveOptions{SearchResults: limit, PreferredProviders: providers}, initialQuery)
}

// Init implements tea.Model and starts the initial repository search when configured.
func (m Model) Init() tea.Cmd {
	if m.Screen == ScreenSearching {
		return tea.Batch(m.Spinner.Tick, m.searchCmd(m.Query.Value()))
	}
	return textinput.Blink
}
func (m *Model) operationContext() context.Context {
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	return ctx
}
func (m *Model) searchCmd(query string) tea.Cmd {
	ctx := m.operationContext()
	return func() tea.Msg {
		results, err := SearchRanked(ctx, m.client, query, m.options.SearchResults, m.options.PreferredProviders)
		return searchCompletedMsg{query, results, err}
	}
}
func (m *Model) filesCmd(repository string) tea.Cmd {
	ctx := m.operationContext()
	return func() tea.Msg {
		revision, files, err := m.client.ListFiles(ctx, repository)
		if err == nil {
			files = FilterFiles(files, m.options.IncludePatterns, m.options.ExcludePatterns, m.options.PreferredQuantizations, false)
		}
		return filesCompletedMsg{repository, revision, files, err}
	}
}
func (m *Model) downloadCmd() tea.Cmd {
	ctx := m.operationContext()
	repository, revision, file, all, options, overwrite := m.selectedRepository.ID, m.revision, m.selectedFile, m.allFiles, m.options, m.overwrite
	return func() tea.Msg {
		_, shards, err := ShardSet(file, all)
		if err != nil {
			return downloadCompletedMsg{err: err}
		}
		if len(shards) > 1 {
			manifest, path, err := DownloadMulti(ctx, options.HFPath, options.ModelsPath, repository, revision, shards)
			return downloadCompletedMsg{manifest, path, err}
		}
		manifest, err := Download(ctx, options.HFPath, DownloadRequest{Repository: repository, File: file.Path, Revision: revision, ModelsPath: options.ModelsPath, Size: file.Size, ETag: file.ETag, LFSOID: file.LFSOID, Overwrite: overwrite})
		return downloadCompletedMsg{manifest, m.destination.Manifest, err}
	}
}

func (m *Model) startProgressDownload(shards []ModelFile) tea.Cmd {
	ctx := m.operationContext()
	events := make(chan tea.Msg, 32)
	m.transfer = events
	request := DownloadRequest{Repository: m.selectedRepository.ID, File: m.selectedFile.Path, Revision: m.revision, ModelsPath: m.options.ModelsPath, Size: m.selectedFile.Size, ETag: m.selectedFile.ETag, LFSOID: m.selectedFile.LFSOID, Overwrite: m.overwrite}
	manifestPath := m.destination.Manifest
	modelsPath, repository, revision, overwrite := m.options.ModelsPath, m.selectedRepository.ID, m.revision, m.overwrite
	go func() {
		callback := func(update ProgressUpdate) {
			select {
			case events <- downloadProgressMsg{update}:
			case <-ctx.Done():
			}
		}
		var manifest Manifest
		var err error
		if len(shards) > 1 {
			manifest, manifestPath, err = DownloadMultiHTTP(ctx, nil, modelsPath, repository, revision, shards, overwrite, callback)
		} else {
			manifest, err = DownloadHTTP(ctx, nil, request, callback)
		}
		events <- downloadCompletedMsg{manifest, manifestPath, err}
		close(events)
	}()
	return waitTransfer(events)
}
func waitTransfer(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		message, ok := <-events
		if !ok {
			return nil
		}
		return message
	}
}

// Update implements tea.Model and advances the model-manager workflow.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		w, h := max(10, msg.Width-2), max(4, msg.Height-4)
		m.Repositories.SetSize(w, h)
		m.Files.SetSize(w, h)
		return m, nil
	case searchCompletedMsg:
		if msg.query != m.Query.Value() {
			return m, nil
		}
		m.cancel = nil
		if msg.err != nil {
			recordInteraction("search_failed", "query="+msg.query, "error="+msg.err.Error())
			m.err = msg.err
			m.Screen = ScreenError
			return m, nil
		}
		items := make([]list.Item, len(msg.results))
		for i := range msg.results {
			items[i] = repositoryItem{msg.results[i]}
		}
		m.Repositories.SetItems(items)
		recordInteraction("search_completed", "query="+msg.query, fmt.Sprintf("results=%d", len(msg.results)))
		m.Screen = ScreenRepositories
		return m, nil
	case filesCompletedMsg:
		if msg.repository != m.selectedRepository.ID {
			return m, nil
		}
		m.cancel = nil
		if msg.err != nil {
			recordInteraction("files_failed", "repository="+msg.repository, "error="+msg.err.Error())
			m.err = msg.err
			m.Screen = ScreenError
			return m, nil
		}
		m.revision, m.allFiles = msg.revision, msg.files
		items := make([]list.Item, len(msg.files))
		for i := range msg.files {
			items[i] = fileItem{msg.files[i]}
		}
		m.Files.SetItems(items)
		recordInteraction("files_loaded", "repository="+msg.repository, "revision="+msg.revision, fmt.Sprintf("files=%d", len(msg.files)))
		m.Screen = ScreenFiles
		return m, nil
	case downloadProgressMsg:
		now := time.Now()
		if msg.Phase != m.progressPhase || msg.CompletedBytes < m.progressLastBytes {
			m.progressPhase = msg.Phase
			m.progressLastAt = now
			m.progressLastBytes = msg.CompletedBytes
			m.progressRate = 0
		} else if !m.progressLastAt.IsZero() {
			elapsed := now.Sub(m.progressLastAt).Seconds()
			delta := msg.CompletedBytes - m.progressLastBytes
			if elapsed > 0 && delta > 0 {
				instant := float64(delta) / elapsed
				if m.progressRate == 0 {
					m.progressRate = instant
				} else {
					m.progressRate = .2*instant + .8*m.progressRate
				}
			}
			m.progressLastAt = now
			m.progressLastBytes = msg.CompletedBytes
		}
		m.transferProgress = msg.ProgressUpdate
		return m, waitTransfer(m.transfer)
	case downloadCompletedMsg:
		m.cancel = nil
		if msg.err != nil {
			recordInteraction("download_failed", "repository="+m.selectedRepository.ID, "file="+m.selectedFile.Path, "error="+msg.err.Error())
			m.err = msg.err
			m.Screen = ScreenError
			return m, nil
		}
		m.manifest, m.manifestPath = msg.manifest, msg.manifestPath
		recordInteraction("download_completed", "repository="+m.selectedRepository.ID, "file="+m.selectedFile.Path, "revision="+m.revision, "manifest="+msg.manifestPath)
		m.Screen = ScreenComplete
		return m, nil
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			recordInteraction("interaction_cancelled", "screen="+fmt.Sprint(m.Screen))
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		switch m.Screen {
		case ScreenQuery:
			if key == "esc" {
				return m, tea.Quit
			}
			if key == "enter" {
				recordInteraction("search_submitted", "query="+m.Query.Value())
				m.Screen = ScreenSearching
				return m, tea.Batch(m.Spinner.Tick, m.searchCmd(m.Query.Value()))
			}
		case ScreenSearching, ScreenLoadingFiles, ScreenDownloading:
			if key == "esc" {
				if m.cancel != nil {
					m.cancel()
				}
				if m.Screen == ScreenSearching {
					m.Screen = ScreenQuery
				} else if m.Screen == ScreenLoadingFiles {
					m.Screen = ScreenRepositories
				} else {
					m.Screen = ScreenFiles
				}
				return m, nil
			}
		case ScreenRepositories:
			if key == "q" {
				return m, tea.Quit
			}
			if key == "/" {
				m.Screen = ScreenQuery
				m.Query.Focus()
				return m, textinput.Blink
			}
			if key == "enter" && len(m.Repositories.Items()) > 0 {
				m.selectedRepository = m.Repositories.SelectedItem().(repositoryItem).SearchResult
				recordInteraction("repository_selected", "repository="+m.selectedRepository.ID)
				m.Screen = ScreenLoadingFiles
				return m, tea.Batch(m.Spinner.Tick, m.filesCmd(m.selectedRepository.ID))
			}
		case ScreenFiles:
			if key == "q" {
				return m, tea.Quit
			}
			if key == "esc" || key == "backspace" {
				m.Screen = ScreenRepositories
				return m, nil
			}
			if key == "enter" && len(m.Files.Items()) > 0 {
				m.selectedFile = m.Files.SelectedItem().(fileItem).ModelFile
				recordInteraction("artifact_selected", "repository="+m.selectedRepository.ID, "file="+m.selectedFile.Path)
				destination, err := ResolveDestination(m.options.ModelsPath, m.selectedRepository.ID, m.selectedFile.Path)
				if err != nil {
					m.err = err
					m.Screen = ScreenError
					return m, nil
				}
				m.destination = destination
				m.overwrite = fileExists(destination.Artifact) || fileExists(destination.Manifest)
				if _, shards, shardErr := ShardSet(m.selectedFile, m.allFiles); shardErr == nil && len(shards) > 1 {
					for _, shard := range shards {
						shardDestination, resolveErr := ResolveDestination(m.options.ModelsPath, m.selectedRepository.ID, shard.Path)
						if resolveErr == nil && (fileExists(shardDestination.Artifact) || fileExists(shardDestination.Manifest)) {
							m.overwrite = true
							break
						}
					}
				}
				m.Screen = ScreenConfirm
				return m, nil
			}
		case ScreenConfirm:
			if key == "n" || key == "esc" {
				recordInteraction("download_cancelled", "repository="+m.selectedRepository.ID, "file="+m.selectedFile.Path)
				m.Screen = ScreenFiles
				return m, nil
			}
			if key == "y" {
				recordInteraction("download_confirmed", "repository="+m.selectedRepository.ID, "file="+m.selectedFile.Path, fmt.Sprintf("overwrite=%t", m.overwrite))
				m.Screen = ScreenDownloading
				_, shards, err := ShardSet(m.selectedFile, m.allFiles)
				if err != nil {
					m.err = err
					m.Screen = ScreenError
					return m, nil
				}
				m.transferProgress = ProgressUpdate{}
				m.progressPhase = ""
				m.progressLastAt = time.Time{}
				m.progressLastBytes = 0
				m.progressRate = 0
				return m, tea.Batch(m.Spinner.Tick, m.startProgressDownload(shards))
			}
		case ScreenComplete:
			if key == "q" || key == "enter" {
				return m, tea.Quit
			}
			if key == "esc" {
				m.Screen = ScreenFiles
				return m, nil
			}
		case ScreenError:
			if key == "esc" {
				m.Screen = ScreenFiles
				if m.selectedRepository.ID == "" {
					m.Screen = ScreenQuery
				}
				return m, nil
			}
			if key == "q" {
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	switch m.Screen {
	case ScreenQuery:
		m.Query, cmd = m.Query.Update(msg)
	case ScreenSearching, ScreenLoadingFiles, ScreenDownloading:
		m.Spinner, cmd = m.Spinner.Update(msg)
	case ScreenRepositories:
		m.Repositories, cmd = m.Repositories.Update(msg)
	case ScreenFiles:
		m.Files, cmd = m.Files.Update(msg)
	}
	return m, cmd
}

// View implements tea.Model and renders the current model-manager screen.
func (m Model) View() string {
	switch m.Screen {
	case ScreenQuery:
		return "Search models\n\n" + m.Query.View() + "\n\nenter search • esc quit\n"
	case ScreenSearching:
		return fmt.Sprintf("%s Searching for %q…\n\nesc cancel\n", m.Spinner.View(), m.Query.Value())
	case ScreenRepositories:
		return m.Repositories.View() + "\n/ new search • enter select • q quit\n"
	case ScreenLoadingFiles:
		return fmt.Sprintf("%s Loading exact artifacts for %s…\n\nesc cancel\n", m.Spinner.View(), m.selectedRepository.ID)
	case ScreenFiles:
		if len(m.Files.Items()) == 0 {
			return "No downloadable files matched the configured filters.\n\nesc back • q quit\n"
		}
		return m.Files.View() + "\nenter review download • esc repositories • q quit\n"
	case ScreenConfirm:
		action, warning := "Download", ""
		if m.overwrite {
			action = "Overwrite"
			warning = "\nWARNING: the artifact or manifest already exists.\n"
		}
		url, _ := DownloadURL(m.selectedRepository.ID, m.revision, m.selectedFile.Path)
		return fmt.Sprintf("%s exact artifact?\n\nRepository: %s\nRevision: %s\nFile: %s\nSize: %s\nURL: %s\nDestination: %s\nManifest: %s\n%s\ny confirm • n cancel\n", action, m.selectedRepository.ID, m.revision, m.selectedFile.Path, formatBytes(m.selectedFile.Size), url, m.destination.Artifact, m.destination.Manifest, warning)
	case ScreenDownloading:
		phase := string(m.transferProgress.Phase)
		if phase == "" {
			phase = "preparing"
		}
		if m.transferProgress.TotalBytes > 0 {
			percent := float64(m.transferProgress.CompletedBytes) / float64(m.transferProgress.TotalBytes)
			if percent > 1 {
				percent = 1
			}
			return fmt.Sprintf("%s %s\n\n%s\n%s / %s  %.1f%%\nRate: %s/s  ETA: %s\n\nctrl+c cancel\n", strings.ToUpper(phase[:1])+phase[1:], m.selectedFile.Path, m.Progress.ViewAs(percent), formatBytes(m.transferProgress.CompletedBytes), formatBytes(m.transferProgress.TotalBytes), percent*100, formatRate(m.progressRate), formatETA(m.transferProgress.TotalBytes-m.transferProgress.CompletedBytes, m.progressRate))
		}
		return fmt.Sprintf("%s %s %s…\n\nTransferred: %s\nctrl+c cancel\n", m.Spinner.View(), strings.ToUpper(phase[:1])+phase[1:], m.selectedFile.Path, formatBytes(m.transferProgress.CompletedBytes))
	case ScreenComplete:
		return fmt.Sprintf("Installed and verified.\n\nArtifact: %s\nManifest: %s\nSHA-256: %s\n\nenter/q quit • esc files\n", m.destination.Artifact, m.manifestPath, m.manifest.SHA256)
	case ScreenError:
		return fmt.Sprintf("Operation failed: %v\n\nesc back • q quit\n", m.err)
	}
	return ""
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }
func formatBytes(size int64) string {
	if size <= 0 {
		return "unknown"
	}
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}
func formatRate(rate float64) string {
	if rate <= 0 {
		return "calculating…"
	}
	return formatBytes(int64(rate))
}
func formatETA(remaining int64, rate float64) string {
	if remaining <= 0 {
		return "0s"
	}
	if rate <= 0 {
		return "calculating…"
	}
	seconds := time.Duration(float64(remaining) / rate * float64(time.Second))
	if seconds < time.Second {
		seconds = time.Second
	}
	seconds = seconds.Round(time.Second)
	if seconds >= time.Hour {
		return fmt.Sprintf("%dh%02dm", int(seconds.Hours()), int(seconds.Minutes())%60)
	}
	if seconds >= time.Minute {
		return fmt.Sprintf("%dm%02ds", int(seconds.Minutes()), int(seconds.Seconds())%60)
	}
	return seconds.String()
}

// RunInteractive runs the model-manager UI until completion or cancellation.
func RunInteractive(ctx context.Context, in io.Reader, out io.Writer, client HubClient, options InteractiveOptions, initialQuery string) error {
	if options.SearchResults == 0 {
		options.SearchResults = 10
	}
	if options.HFPath == "" {
		if concrete, ok := client.(*HFCLIClient); ok {
			options.HFPath = concrete.Path
		}
	}
	if options.ModelsPath != "" {
		options.ModelsPath = filepath.Clean(options.ModelsPath)
	}
	model := NewInteractiveModel(ctx, client, options, initialQuery)
	_, err := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out)).Run()
	return err
}
