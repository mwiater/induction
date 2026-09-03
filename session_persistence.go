package induction

import (
	"bytes"
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
	sessionDirectory     = ".sessions"
	chatSessionVersion   = 1
	maxSessionTitleRunes = 120
	sessionTypeSaved     = "saved"
	sessionTypeUnsaved   = "unsaved"
)

// ChatSession is the persisted transcript and snapshot history for one chat.
type ChatSession struct {
	Version   int              `json:"version"`
	ID        string           `json:"id"`
	Type      string           `json:"type"`
	Title     string           `json:"title"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Model     string           `json:"model"`
	Messages  []Message        `json:"messages"`
	Snapshots []*ModelSnapshot `json:"snapshots"`
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
	return &ChatSession{Version: chatSessionVersion, ID: id, Type: sessionTypeSaved, Title: title, CreatedAt: now, UpdatedAt: now, Model: request.Model, Messages: cloneMessages(request.Messages)}, nil
}

func newUnsavedChatSession(request ChatRequest) (*ChatSession, error) {
	id, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("generate chat session ID: %w", err)
	}
	now := time.Now().UTC()
	return &ChatSession{Version: chatSessionVersion, ID: id, Type: sessionTypeUnsaved, CreatedAt: now, UpdatedAt: now, Model: request.Model, Messages: cloneMessages(request.Messages)}, nil
}

func chatSessionPath(id string) string { return filepath.Join(sessionDirectory, id+".json") }

func saveChatSession(session *ChatSession) error {
	return saveChatSessionToDir(sessionDirectory, session)
}

func saveChatSessionToDir(directory string, session *ChatSession) error {
	if session == nil {
		return errors.New("chat session is nil")
	}
	if err := validateChatSession(session); err != nil {
		return err
	}
	session.UpdatedAt = time.Now().UTC()
	contents, err := json.MarshalIndent(session, "", "  ")
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
	defer os.Remove(temporaryPath)
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

func loadChatSession(path string) (*ChatSession, error) { return loadChatSessionFromPath(path) }

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

func loadChatSessionByID(id string) (*ChatSession, error) {
	if !chatSessionIDPattern.MatchString(id) {
		return nil, errors.New("invalid chat session ID")
	}
	return loadChatSession(chatSessionPath(id))
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
		if session.Type != sessionTypeSaved {
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

// cleanupNullSnapshotSessions removes sessions that never recorded an
// inference snapshot. A null snapshots field is intentionally distinct from
// an empty array, which represents a valid session with an initialized
// snapshot collection.
func cleanupNullSnapshotSessions() (int, error) {
	return cleanupNullSnapshotSessionsFromDir(sessionDirectory)
}

func cleanupNullSnapshotSessionsFromDir(directory string) (int, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("list sessions for cleanup: %w", err)
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(contents, &fields); err != nil {
			continue
		}
		snapshots, ok := fields["snapshots"]
		if !ok || !bytes.Equal(bytes.TrimSpace(snapshots), []byte("null")) {
			continue
		}
		if _, err := loadChatSessionFromPath(path); err != nil {
			continue
		}
		if err := os.Remove(path); err != nil {
			return removed, fmt.Errorf("remove session %q: %w", path, err)
		}
		removed++
	}
	return removed, nil
}

func validateChatSession(session *ChatSession) error {
	if session.Version != chatSessionVersion {
		return fmt.Errorf("unsupported chat session version %d", session.Version)
	}
	if !chatSessionIDPattern.MatchString(session.ID) {
		return errors.New("invalid chat session ID")
	}
	if session.Type == "" {
		// Files written before explicit saved/unsaved classification are named
		// sessions and remain loadable.
		session.Type = sessionTypeSaved
	}
	if session.Type != sessionTypeSaved && session.Type != sessionTypeUnsaved {
		return fmt.Errorf("invalid chat session type %q", session.Type)
	}
	if session.Type == sessionTypeSaved {
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
