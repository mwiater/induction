// Package induction contains the core implementation for the public Induction API.
package induction

import (
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
)

const (
	// sessionDirectory identifies a fixed value used internally by this package.
	sessionDirectory = ".sessions"
	// chatSessionVersion identifies a fixed value used internally by this package.
	chatSessionVersion = 1
	// maxSessionTitleRunes identifies a fixed value used internally by this package.
	maxSessionTitleRunes = 120
	// sessionTypeDirect identifies a fixed value used internally by this package.
	sessionTypeDirect = "direct"
	// sessionTypePipeline identifies a fixed value used internally by this package.
	sessionTypePipeline = "pipeline"
)

// ChatSession is the persisted transcript and snapshot history for one chat.
// ChatSession is an exported type in this package's public data model.
type ChatSession struct {
	// Version stores the value represented by this member.
	Version int `json:"version"`
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// Type stores the value represented by this member.
	Type string `json:"type"`
	// Saved stores the value represented by this member.
	Saved bool `json:"saved"`
	// Title stores the value represented by this member.
	Title string `json:"title"`
	// CreatedAt stores the value represented by this member.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt stores the value represented by this member.
	UpdatedAt time.Time `json:"updated_at"`
	// Model stores the value represented by this member.
	Model string `json:"model"`
	// FinalOutput stores the value represented by this member.
	FinalOutput string `json:"final_output"`
	// Error stores the value represented by this member.
	Error string `json:"error,omitempty"`
	// BatchID stores the value represented by this member.
	BatchID string `json:"batch_id,omitempty"`
	// BatchItemID stores the value represented by this member.
	BatchItemID string `json:"batch_item_id,omitempty"`
	// Messages stores the value represented by this member.
	Messages []Message `json:"messages"`
	// Snapshots stores the value represented by this member.
	Snapshots []*ModelSnapshot `json:"snapshots"`
	// imageFilename stores the value represented by this member.
	imageFilename string
	// documentFilename stores the value represented by this member.
	documentFilename string
}

// ChatSessionSummary contains the metadata shown when listing saved chats.
// ChatSessionSummary is an exported type in this package's public data model.
type ChatSessionSummary struct {
	// ID stores the value represented by this member.
	ID string
	// Title stores the value represented by this member.
	Title string
	// Model stores the value represented by this member.
	Model string
	// CreatedAt stores the value represented by this member.
	CreatedAt time.Time
	// UpdatedAt stores the value represented by this member.
	UpdatedAt time.Time
	// MessageCount stores the value represented by this member.
	MessageCount int
	// Path stores the value represented by this member.
	Path string
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

// chatSessionIDPattern holds package-level state used internally by this package.
var chatSessionIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// normalizeSessionTitle performs the internal operation described by its signature.
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

// newChatSession performs the internal operation described by its signature.
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

// newUnsavedChatSession performs the internal operation described by its signature.
func newUnsavedChatSession(request ChatRequest) (*ChatSession, error) {
	id, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("generate chat session ID: %w", err)
	}
	now := time.Now().UTC()
	return &ChatSession{Version: chatSessionVersion, ID: id, Type: sessionTypeDirect, CreatedAt: now, UpdatedAt: now, Model: request.Model, Messages: cloneMessages(request.Messages), imageFilename: request.ImageFilename, documentFilename: request.DocumentFilename}, nil
}

// configureSessionMode performs the internal operation described by its signature.
func configureSessionMode(session *ChatSession, pipeline *Pipeline) {
	if session == nil {
		return
	}
	session.Type = sessionTypeDirect
	if pipeline != nil {
		session.Type = sessionTypePipeline
		session.Title = pipeline.Name
	}
}

// updateSessionFinalOutput performs the internal operation described by its signature.
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

// chatSessionPath performs the internal operation described by its signature.
func chatSessionPath(id string) string { return filepath.Join(sessionDirectory, id+".json") }

// saveChatSession performs the internal operation described by its signature.
func saveChatSession(session *ChatSession) error {
	return saveChatSessionToDir(sessionDirectory, session)
}

// saveChatSessionToDir performs the internal operation described by its signature.
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
		clonedSnapshot := *snapshot
		clonedSnapshot.Messages = redactMessages(cloneMessages(snapshot.Messages), imageFilename, documentFilename)
		persisted.Snapshots[i] = &clonedSnapshot
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

// sessionAttachmentFilenames performs the internal operation described by its signature.
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

// loadChatSessionFromPath performs the internal operation described by its signature.
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

// listChatSessions performs the internal operation described by its signature.
func listChatSessions() ([]ChatSessionSummary, error) {
	return listChatSessionsFromDir(sessionDirectory)
}

// listChatSessionsFromDir performs the internal operation described by its signature.
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

// validateChatSession performs the internal operation described by its signature.
// Additional context: Populate this for newly written sessions and migrate older sessions that predate the field.
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
	if session.Saved {
		normalized, err := normalizeSessionTitle(session.Title)
		if err != nil {
			return err
		}
		session.Title = normalized
	} else if session.Type == sessionTypePipeline {
		normalized, err := normalizeSessionTitle(session.Title)
		if err != nil {
			return err
		}
		session.Title = normalized
	} else if session.Title != "" {
		return errors.New("unsaved chat sessions cannot have a title")
	}
	if strings.TrimSpace(session.Model) == "" {
		return errors.New("chat session model cannot be empty")
	}
	if session.Messages == nil {
		session.Messages = []Message{}
	}

	updateSessionFinalOutput(session)
	return nil
}

// cloneMessages performs the internal operation described by its signature.
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
