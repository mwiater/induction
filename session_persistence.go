package induction

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	sessionDirectory     = ".sessions"
	chatSessionVersion   = 1
	maxSessionTitleRunes = 120
	sessionTypeDirect    = "direct"
	sessionTypePipeline  = "pipeline"
)

// ChatSession is the persisted transcript and snapshot history for one chat.
type ChatSession struct {
	Version          int              `json:"version"`
	ID               string           `json:"id"`
	Type             string           `json:"type"`
	Saved            bool             `json:"saved"`
	Title            string           `json:"title"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	Model            string           `json:"model"`
	FinalOutput      string           `json:"final_output"`
	Error            string           `json:"error,omitempty"`
	BatchID          string           `json:"batch_id,omitempty"`
	BatchItemID      string           `json:"batch_item_id,omitempty"`
	Pipeline         *SessionPipeline `json:"pipeline,omitempty"`
	PipelineStatus   string           `json:"pipeline_status,omitempty"`
	CompletedAt      *time.Time       `json:"completed_at,omitempty"`
	Messages         []Message        `json:"messages"`
	Snapshots        []*ModelSnapshot `json:"snapshots"`
	imageFilename    string
	documentFilename string
}

// SessionPipeline identifies the exact pipeline configuration used for a run.
// File paths are absolute; SHA256 hashes the original file bytes for file
// pipelines and the YAML encoding of the in-memory configuration otherwise.
type SessionPipeline struct {
	Name   string `json:"name"`
	Source string `json:"source"` // file or inline
	Path   string `json:"path,omitempty"`
	SHA256 string `json:"sha256"`
}

// ChatSessionSummary contains the metadata shown when listing saved chats.
type ChatSessionSummary struct {
	ID           string
	Title        string
	Model        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	MessageCount int
	Path         string
}

// sessionMostRecentModel returns the model that produced the latest completed
// turn. Session.Model remains a backwards-compatible fallback for sessions
// created before per-turn snapshots were recorded.
func sessionMostRecentModel(session *ChatSession) string {
	if session == nil {
		return ""
	}
	for i := len(session.Snapshots) - 1; i >= 0; i-- {
		snapshot := session.Snapshots[i]
		if snapshot != nil && strings.TrimSpace(snapshot.ModelID) != "" {
			return snapshot.ModelID
		}
	}
	return session.Model
}

var chatSessionIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func normalizeSessionTitle(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("session title cannot be empty")
	}
	if !utf8.ValidString(value) {
		return "", errors.New("session title must contain valid UTF-8")
	}
	if len([]rune(value)) > maxSessionTitleRunes {
		return "", fmt.Errorf("session title cannot exceed %d characters", maxSessionTitleRunes)
	}
	return value, nil
}

func newChatSession(title string, request ChatRequest) (*ChatSession, error) {
	title, err := normalizeSessionTitle(title)
	if err != nil {
		return nil, err
	}
	id, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("generate chat session ID: %w", err)
	}
	now := time.Now().UTC()
	return &ChatSession{Version: chatSessionVersion, ID: id, Type: sessionTypeDirect, Saved: true, Title: title, CreatedAt: now, UpdatedAt: now, Model: request.Model, Messages: cloneMessages(request.Messages), imageFilename: request.ImageFilename, documentFilename: request.DocumentFilename}, nil
}

func newUnsavedChatSession(request ChatRequest) (*ChatSession, error) {
	id, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("generate chat session ID: %w", err)
	}
	now := time.Now().UTC()
	return &ChatSession{Version: chatSessionVersion, ID: id, Type: sessionTypeDirect, CreatedAt: now, UpdatedAt: now, Model: request.Model, Messages: cloneMessages(request.Messages), imageFilename: request.ImageFilename, documentFilename: request.DocumentFilename}, nil
}

func configureSessionMode(session *ChatSession, pipeline *Pipeline) error {
	if session == nil {
		return nil
	}
	session.Type = sessionTypeDirect
	session.Pipeline = nil
	session.PipelineStatus = ""
	session.CompletedAt = nil
	if pipeline != nil {
		provenance := &SessionPipeline{Name: pipeline.Name}
		if pipeline.FilePath != "" {
			if pipeline.FileSHA256 == "" {
				return errors.New("file-backed pipeline has no load-time SHA-256; load it with LoadPipeline")
			}
			path, err := filepath.Abs(pipeline.FilePath)
			if err != nil {
				return fmt.Errorf("resolve pipeline path: %w", err)
			}
			provenance.Source = "file"
			provenance.Path = path
			provenance.SHA256 = pipeline.FileSHA256
		} else {
			data, err := yaml.Marshal(pipeline)
			if err != nil {
				return fmt.Errorf("encode in-memory pipeline provenance: %w", err)
			}
			hash := sha256.Sum256(data)
			provenance.Source = "inline"
			provenance.SHA256 = hex.EncodeToString(hash[:])
		}
		session.Type = sessionTypePipeline
		session.PipelineStatus = "running"
		session.Title = pipeline.Name
		session.Pipeline = provenance
	}
	return nil
}

func updateSessionFinalOutput(session *ChatSession) {
	if session == nil || len(session.Snapshots) == 0 {
		return
	}
	snapshot := session.Snapshots[len(session.Snapshots)-1]
	if snapshot == nil {
		return
	}
	session.FinalOutput = ""
	for i := len(snapshot.Messages) - 1; i >= 0; i-- {
		if strings.EqualFold(snapshot.Messages[i].Role, "assistant") {
			session.FinalOutput = replayMessageContent(snapshot.Messages[i].Content)
			return
		}
	}
}

func chatSessionPath(id string) string { return filepath.Join(sessionDirectory, id+".json") }

func saveChatSession(session *ChatSession) error {
	return saveChatSessionToDir(sessionDirectory, session)
}

func saveChatSessionToDir(directory string, session *ChatSession) error {
	if session == nil {
		return errors.New("chat session is nil")
	}
	persisted := *session
	imageFilename, documentFilename := sessionAttachmentFilenames(&persisted)
	persisted.Messages = redactMessages(cloneMessages(session.Messages), imageFilename, documentFilename)
	persisted.Snapshots = make([]*ModelSnapshot, len(session.Snapshots))
	for i, snapshot := range session.Snapshots {
		if snapshot == nil {
			continue
		}
		copy := *snapshot
		copy.Messages = redactMessages(cloneMessages(snapshot.Messages), imageFilename, documentFilename)
		persisted.Snapshots[i] = &copy
	}
	if err := validateChatSession(&persisted); err != nil {
		return err
	}
	persisted.UpdatedAt = time.Now().UTC()
	session.UpdatedAt = persisted.UpdatedAt
	contents, err := json.MarshalIndent(&persisted, "", "  ")
	if err != nil {
		return fmt.Errorf("encode chat session: %w", err)
	}
	contents = append(contents, '\n')
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}
	path := filepath.Join(directory, session.ID+".json")
	temporary, err := os.CreateTemp(directory, ".sessions-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary session file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set session permissions: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write chat session: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync chat session: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close chat session: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace chat session: %w", err)
	}
	return nil
}

func sessionAttachmentFilenames(session *ChatSession) (string, string) {
	imageFilename, documentFilename := session.imageFilename, session.documentFilename
	for _, snapshot := range session.Snapshots {
		if snapshot == nil {
			continue
		}
		for _, message := range snapshot.Messages {
			parts, ok := message.Content.([]any)
			if !ok {
				continue
			}
			for _, value := range parts {
				object, ok := value.(map[string]any)
				if !ok {
					continue
				}
				kind, _ := object["type"].(string)
				if strings.EqualFold(kind, "image_url") {
					if imageURL, ok := object["image_url"].(map[string]any); ok {
						if value, _ := imageURL["url"].(string); value != "" && value != "image" && !strings.HasPrefix(strings.ToLower(value), "data:") {
							imageFilename = value
						}
					}
				}
				if strings.EqualFold(kind, "text") {
					if value, _ := object["text"].(string); strings.HasSuffix(strings.ToLower(value), ".pdf") || strings.HasSuffix(strings.ToLower(value), ".txt") || strings.HasSuffix(strings.ToLower(value), ".md") {
						documentFilename = value
					}
				}
			}
		}
	}
	return imageFilename, documentFilename
}

// LoadChatSession loads and validates a persisted chat session from path.
func LoadChatSession(path string) (*ChatSession, error) { return loadChatSessionFromPath(path) }

func loadChatSessionFromPath(path string) (*ChatSession, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read chat session: %w", err)
	}
	var session ChatSession
	if err := json.Unmarshal(contents, &session); err != nil {
		return nil, fmt.Errorf("decode chat session: %w", err)
	}
	if err := validateChatSession(&session); err != nil {
		return nil, err
	}
	session.Messages = cloneMessages(session.Messages)
	return &session, nil
}

func listChatSessions() ([]ChatSessionSummary, error) {
	return listChatSessionsFromDir(sessionDirectory)
}

func listChatSessionsFromDir(directory string) ([]ChatSessionSummary, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return []ChatSessionSummary{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list chat sessions: %w", err)
	}
	result := make([]ChatSessionSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		session, err := loadChatSessionFromPath(path)
		if err != nil {
			continue
		}
		if !session.Saved {
			continue
		}
		result = append(result, ChatSessionSummary{ID: session.ID, Title: session.Title, Model: sessionMostRecentModel(session), CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt, MessageCount: len(session.Messages), Path: path})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			if result[i].Title == result[j].Title {
				return result[i].ID < result[j].ID
			}
			return result[i].Title < result[j].Title
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

func validateChatSession(session *ChatSession) error {
	if session.Version != chatSessionVersion {
		return fmt.Errorf("unsupported chat session version %d", session.Version)
	}
	if !chatSessionIDPattern.MatchString(session.ID) {
		return errors.New("invalid chat session ID")
	}
	if session.Type != sessionTypeDirect && session.Type != sessionTypePipeline {
		return fmt.Errorf("invalid chat session type %q", session.Type)
	}
	if session.Type == sessionTypePipeline {
		if session.Pipeline == nil || strings.TrimSpace(session.Pipeline.Name) == "" {
			return errors.New("pipeline session requires pipeline provenance")
		}
		if session.Pipeline.Source != "file" && session.Pipeline.Source != "inline" {
			return errors.New("pipeline session has invalid pipeline source")
		}
		if session.Pipeline.Source == "file" && !filepath.IsAbs(session.Pipeline.Path) {
			return errors.New("file pipeline provenance requires an absolute path")
		}
		if session.Pipeline.Source == "inline" && session.Pipeline.Path != "" {
			return errors.New("inline pipeline provenance cannot have a file path")
		}
		if len(session.Pipeline.SHA256) != 64 {
			return errors.New("pipeline provenance requires a SHA-256 hash")
		}
		if _, err := hex.DecodeString(session.Pipeline.SHA256); err != nil {
			return errors.New("pipeline provenance has invalid SHA-256 hash")
		}
		switch session.PipelineStatus {
		case "": // sessions written by binaries predating explicit terminal status
			if session.CompletedAt != nil {
				return errors.New("legacy pipeline session cannot have completed_at")
			}
		case "running":
			if session.CompletedAt != nil {
				return errors.New("running pipeline session cannot have completed_at")
			}
		case "completed", "failed":
			if session.CompletedAt == nil || session.CompletedAt.IsZero() {
				return errors.New("terminal pipeline session requires completed_at")
			}
		default:
			return errors.New("pipeline session requires a valid pipeline_status")
		}
	} else if session.Pipeline != nil {
		return errors.New("direct session cannot have pipeline provenance")
	}
	if session.Saved {
		if normalized, err := normalizeSessionTitle(session.Title); err != nil {
			return err
		} else {
			session.Title = normalized
		}
	} else if session.Type == sessionTypePipeline {
		if normalized, err := normalizeSessionTitle(session.Title); err != nil {
			return err
		} else {
			session.Title = normalized
		}
	} else if session.Title != "" {
		return errors.New("unsaved chat sessions cannot have a title")
	}
	if strings.TrimSpace(session.Model) == "" {
		return errors.New("chat session model cannot be empty")
	}
	if session.Messages == nil {
		session.Messages = []Message{}
	}
	// Populate this for newly written sessions and migrate older sessions that
	// predate the field.
	updateSessionFinalOutput(session)
	return nil
}

func cloneMessages(messages []Message) []Message {
	if messages == nil {
		return nil
	}
	result := make([]Message, len(messages))
	for i, message := range messages {
		result[i] = message
		result[i].ToolCalls = append([]InferenceToolCall(nil), message.ToolCalls...)
	}
	return result
}
